# 片名中文翻译（LLM）技术设计

**Status:** Implemented
**Scope:** 刮削成功后，用 OpenAI 兼容的 LLM 接口把原片名翻译成简体中文，在详情页增加「中文译名」
**Target:** Windows/macOS Wails Desktop App
**Version:** v1

---

## 1. 目标

刮削已经拿到的 `video_jav.title` 通常是日文。用户在详情页需要一栏可直接读懂的中文译名：

```text
刮削成功（task = succeeded）
        |
        +--> 入队「中文译名」翻译（独立队列，最多 5 个 in-flight 请求）
                |
                +--> LLM 返回译名
                        |
                        +--> video_jav.title_zh
                        +--> 详情页显示「中文译名」
                        +--> 发出 jav:scrape:updated，前端自动刷新
```

硬约束：

- **不阻塞、不改变刮削结果。** 翻译在刮削任务成功之后入队；LLM 未配置、超时、鉴权失败只影响译名，任务仍是 `succeeded`。
- **不在翻译流程里裸写 HTTP。** 请求/响应交给 `github.com/sashabaranov/go-openai`；Muse 只负责配置、提示词和输出清洗。
- **凭证加密保存。** API Key 与 JavBus / FC2 Cookie 一样交给平台安全存储：Windows 用 DPAPI，macOS 用登录钥匙串（`security` CLI，密码经 stdin 传入，不出现在进程参数里）。没有明文兜底，也不回传前端。

## 2. 数据模型（Migration v27–v28）

`video_jav` 增加三列，纯增量、无回填：

| 列 | 含义 |
| --- | --- |
| `title_zh` | 成功翻译后的简体中文片名；空字符串表示还没有译名 |
| `title_zh_error` | 最近一次翻译失败的原因；成功或从未尝试时为空 |
| `title_zh_version` | 生成当前 `title_zh` 的指纹（系统提示词 + 原标题 + 模型）；空字符串表示旧版本未知，下一次「重新翻译全部译名」会刷新一次 |

`title_zh_version` 由 `llm.Client` 计算：`sha256("muse-title-translation-v1" + NUL + systemPrompt + NUL + sanitizeTitle(title) + NUL + model)` 取前 16 字节的十六进制。它让批量重译只处理真正受影响的条目，而不是每次点击都覆盖全库。

成功与失败通常互斥：`SetTitleTranslationVersion` 每次写入都会清掉另一列。例外是「重新翻译已有译名时失败」：`SetTitleTranslationFailure` 只更新错误列、保留旧译名与它的版本，此时两列同时非空，`titleZhStatus` 按 `failed` 报告并在详情页显示失败原因，旧译名仍然可读；下一次成功会清掉错误并写入新版本。读取时由 `titleZhStatus` 统一解释：

| `title_zh` | `title_zh_error` | 状态 |
| --- | --- | --- |
| 任意 | 非空 | `failed`（旧译名可能仍在，见上） |
| 非空 | 空 | `succeeded` |
| 空 | 空 | `none` |

`pending`（翻译中）**不落库**：它只是服务内存里 `titlePending` 集合的投影。这样进程崩溃不会留下永远停在「翻译中…」的行；重启后该影片回到 `none`，由下一次刮削或批量翻译重新排队。

重新刮削会替换 `title`，因此 `SaveSuccess` 同时清空 `title_zh` / `title_zh_error` / `title_zh_version`，避免旧译名继续描述新标题。

## 3. 配置

`settings.Store` 新增（均复用通用 key/value 表）：

| key | 内容 |
| --- | --- |
| `llm_enabled` | `"0"` 表示暂停自动翻译；缺省视为启用 |
| `llm_resume_missing_titles` | `"1"` 表示启动时自动补齐缺失译名；缺省（或 `"0"`）关闭 |
| `llm_base_url` | OpenAI 兼容接口根地址；缺省 `https://api.deepseek.com/v1` |
| `llm_model` | 模型名；缺省 `deepseek-chat` |
| `llm_api_key_dpapi` | 平台安全存储的引用（Windows DPAPI 密文 / macOS Keychain 账号）；键名保留历史命名 |

`llm.Normalize` 负责校验：只接受 `http(s)`、必须带主机名、拒绝查询参数/片段/换行；地址与模型为空时补默认值。保存表单时 API Key 留空表示**保留原 Key**，因此修改接口或模型不需要重新输入密钥。

`baseURL && model && apiKey` 齐备时 `llm.Client.Available()` 为真，手动翻译（详情页按钮、批量补齐）可用；再加上 `enabled` 才算 `Configured()`，刮削成功后才自动翻译。暂停自动翻译不会挡住手动操作，刮削流程也直接跳过，不写任何失败状态。

`llm_resume_missing_titles` 与 `enabled` 相互独立：它只决定**启动时**要不要把仍未翻译的片名重新排进队列，默认关闭，因为每次翻译都是一次付费请求。打开后即使自动翻译处于暂停状态也会补齐（与手动批量一致）；设置读取失败按关闭处理，绝不影响启动。

配置表单提供「测试连接」（`llm.Client.Check`）：用**当前表单里的**接口地址、模型与 Key（Key 留空时取已保存的 Key）翻译一条固定示例片名 `となりの人妻`，返回真实译名或可操作错误。它不写数据库，所以配错的值不会被保存；即使自动翻译处于暂停状态也能测试。

## 4. 请求与输出

- 客户端：`backend/llm.Client`，实现 `jav.TitleTranslator`（`TranslateTitle`、`TranslateTitles` + `Configured` + `Available`）以及可选的 `jav.TitleVersioner`（`TitleVersions`）。
- 每次翻译都重新读取设置，因此改配置无需重启；HTTP 复用刮削代理（`netproxy.Config.Transport()`），LLM 与元数据请求走同一条网络路径。
- **版本指纹**：`TitleVersions` 用当前设置计算每条标题的 `title_zh_version`，不触发请求。批量重译只把「没有译名」或「存储版本 ≠ 当前版本」的行入队；提示词、原标题或模型任一变化都会让对应行变旧，版本没变则整批跳过。详情页的单条「重新翻译」始终绕过版本检查。
- **批量打包**：单条标题每次都要重发一大段系统提示词，批量任务因此把最多 10 条标题塞进一次请求。用户消息按 `1. 标题` 编号，要求模型严格按编号逐行输出 `编号. 译文`；解析器按编号解包，支持 `1.` / `1、` / `1：` / 全角标点 / Markdown 粗体编号。模型漏行或格式不可用时，只对缺失的那几条单独重试，绝不按顺序猜测——错位的译名比没有译名更难发现。
- 单条请求 60 秒超时，打包请求 150 秒超时（外层 3 分钟兜底）；提示词要求只输出最终简体中文译文（不输出原文、翻译过程、词汇解释或前言），并保留原文的语气、情绪与表达强度。
- 输出清洗：每条取第一行、去掉 `中文译名：` 之类前缀与包裹引号、上限 200 字符；空结果按失败处理。批量请求中的标题会先压成单行（防止换行打乱编号）并各自限长。
- 错误映射成可操作的中文：401/403 提示检查 API Key，404 提示接口/模型，429 提示稍后重试，5xx 提示服务不可用，网络错误提示无法连接。

## 5. 队列与状态流转

`jav.Service` 增加一个独立的翻译工作池（与刮削 worker 分开）：5 个 worker 共享队列，最多同时有 5 个打包请求在等 provider：

- 刮削成功后，若 provider 已配置且 `title` 非空，则 `enqueueTitleTranslation(id)`；`titlePending` 去重，重复入队无效。
- 队列里装的是**批次**（`[]int64`）：单条操作入队一个单元素批次，`TranslateMissingTitles` / `RetranslateAllTitles` 先为全部 id 预留 `titlePending`，再按 10 条一组切块入队。这样 worker 每次 pop 到的就是固定大小的包，不会因为边入队边消费而退化成一条一请求。
- worker 数量固定为 5：每个 worker 一次只处理一个包，服务因此最多有 5 个 HTTP 请求在飞，既能重叠 provider 的等待时间，又不会形成不可控的并发突刺。
- `Get` 读取详情时，如果 id 在 `titlePending` 中，把状态覆盖成 `pending`，供详情页显示「翻译中…」。
- worker 每个包先重新读取一次详情（防止重新刮削后还用旧标题），再调用 `TranslateTitles` 一次；每个槽位独立落库。完成后先写库、再清 `titlePending`、最后发 `jav:scrape:updated`。顺序很重要：前端收到事件会立即重读，pending 必须先清除，否则已完成的译名会继续显示成翻译中。
- 包内某一条失败只影响那一条：首次翻译失败写入 `title_zh_error`；已有译名的刷新失败保留旧译名并记录错误（`finishTitleTranslationFailure`）。整包请求失败（鉴权、限流、网络）则包内每条都按失败处理，不再逐条重试同一错误。
- 批量入队（`TranslateMissingTitles`）只发一次结果事件，避免 N 条「待刷新」事件同时打到前端；每个包完成时再按条目发结果事件。
- 「重新翻译全部译名」（`RetranslateAllTitles`）先读取全部已成功刮削、带标题的行及其存储版本，再用 `TitleVersions` 计算当前版本：**只把译名缺失、上次失败、或版本不一致的行入队**。因此相同提示词 + 原标题 + 模型下再点一次会跳过成功行，但仍会重试失败行，不消耗成功行的请求；提示词或模型改动后只刷新受影响的行。旧译名继续显示，直到新结果落库；单条刷新失败时只更新 `title_zh_error` 并保留旧译名与旧版本（`finishTitleTranslationFailure`），下一次批量重译仍会把这条当作待刷新。详情页的「重新翻译」不受版本检查影响，始终覆盖该条。
- 队列只在内存里，退出或崩溃即丢失。`llm_resume_missing_titles` 打开时，`jav.Service.Start()` 会在后台调用一次 `TranslateMissingTitles`：已保存的译名不受影响，候选查询只返回 `title_zh` 为空的行，因此重启后是「从断点继续」而不是「从头再译」；强退时正在请求的那一条没有落库，会重译一次。开关关闭时则完全依赖设置页的批量按钮。

## 6. API

`api.Library`（两端共享的业务门面）：

- `TranslateJavTitle(videoID) error`
- `TranslateMissingJavTitles() (int, error)`
- `RetranslateAllJavTitles() (int, error)`

Wails（仅桌面端设置/操作）：

- `GetLLMSettings() (LLMSettings, error)` —— 永远不返回 Key，只给 `hasApiKey`
- `SetLLMSettings(LLMSettingsInput) error`
- `ClearLLMAPIKey() error`
- `TranslateVideoTitle(videoID) error`
- `TranslateMissingTitles() (int, error)`
- `RetranslateAllTitles() (int, error)` —— 设置页「重新翻译全部译名」；先弹确认，避免误触后消耗整库请求
- `TestLLMSettings(input LLMSettingsInput) (string, error)` —— 设置页「测试连接」；只读表单值，不落库

`jav.Detail` 新增 `titleZh` / `titleZhStatus` / `titleZhError`，因此桌面详情、局域网 JSON API 和手机详情页都自动拿到译名，不需要额外端点。

## 7. 前端

- **设置 → 片名翻译**：接口地址、模型、API Key、启用开关、「启动时自动补齐缺失的译名」开关、保存/清除 Key、「测试连接」，以及「翻译缺少译名的片名」批量入口。另提供「重新翻译全部译名」：确认后只重译译名缺失、上次失败、或提示词/模型版本已变化的影片，相同版本且无失败记录会跳过；需要强制刷新单条时用详情页的「重新翻译」。测试成功会直接显示示例译名（连接成功，示例译名：…），失败显示 401/404/429/网络等具体原因。状态只显示「已配置 / 已暂停 / 未配置」。
- **桌面详情页**：元数据表新增「中文译名」一行；标题存在时提供「翻译片名 / 重新翻译」按钮，失败时在按钮下方显示原因。
- **手机详情页**：只读显示「中文译名」（翻译中 / 翻译失败 / 尚未翻译），不提供触发入口。
- **卡片主标题（两端统一）**：`videoDisplayName` 的输出规则为
  1. 用户自定义标题；
  2. 刮削成功：`【番号】译名`；翻译未就绪时用抓取到的原标题占位；
  3. 没有刮削标题（未刮削成功、跳过或只有手填番号）：回落到 `fileName`。
- **搜索**：`api.Library.SearchVideos` 在文件名/路径之外合并 `video_jav` 的番号、原标题与译名；客户端的局部筛选（`videoMatchesKeyword`）同样匹配这五个字段，避免“卡片看得到、搜索找不到”。
- **长译名的截断与回看**：标题统一走 `ui/` 暴露的 Astryx `Text`（`type="inherit"` / `color="inherit"`，不引入第二套排版）：
  - 桌面网格卡片、桌面列表行、手机列表行 `maxLines={1}`；手机网格卡片 `maxLines={2}`（海报底部）；手机详情页标题栏 `maxLines={1}`。
  - `maxLines` 自带**仅在真正被截断时**才出现的悬浮 tooltip，显示完整 `【番号】译名`，所以窄卡片不再丢信息。
  - 完整文本另有两处始终可读：桌面悬浮信息卡（`break-all` 换行）与详情页「中文译名」一行（换行不截断）。
  - 写入侧本来也有限制：送模型的原标题 ≤400 字符，模型返回的译名入库前截到 ≤200 字符。

## 8. 测试

- `backend/llm`：默认值与校验、输出清洗、请求体（模型 + 提示词）、401 映射、设置实时生效、设置读取失败视为未配置；版本指纹随提示词/标题/模型变化、当前模型实时生效、缺少 provider 时拒绝；批量请求的编号提示词、编号解包（标点/粗体/缺行/乱序/代码块）、缺行单独重试、整包错误填充每个槽位；`Check` 用显式配置（不读已存设置、暂停状态也可测）并返回示例译名，缺 Key / 鉴权失败给出可操作错误。
- `backend/settings`：默认配置、往返读写、无效地址拒绝、Key 加密存储（Windows DPAPI / macOS 钥匙串）、无安全存储平台上的降级行为。
- `backend/secret`：Windows DPAPI 往返；macOS 真实钥匙串写入/读取/覆盖隔离/幂等删除；不支持的平台返回 `ErrUnsupported`。
- `backend/database`：v26 → v27 译名列、v27 → v28 版本列、已有刮削/译名数据保留、版本跨再次迁移保留。
- `backend/jav`：刮削后自动翻译、未配置时跳过、失败不改变刮削状态、批量去重、多个入队条目合并成一个打包请求、5 个请求的并发上限、版本相同跳过、版本变化入队、失败行重试、单条显式重译绕过版本、失败保留旧译名、`pending` 覆盖、重新刮削替换旧译名与版本、仓库层候选查询（缺失 / 全部两套工作列表）；`ListTitles` / `SearchTitleIDs` 的投影、跳过与回收站边界、关键词大小写与通配符转义。
- `backend/api`：`SearchVideos` 合并刮削字段（番号/原标题/译名）、不重复返回、并投影 `javTitle` / `javTitleZh`。
- 前端：Astryx `Text` 的截断 tooltip 用真实浏览器验证过（hover 被截断的卡片标题后出现 `role="tooltip"`，内容为完整片名）。

## 9. 非目标

- 不做无上限并发：固定 5 个 worker，每个一次只发一个打包请求，避免把批量任务变成 provider 的并发突刺。
- 不做译名人工编辑（译名是 LLM 派生的只读字段；重新翻译即可）。
- 不做翻译结果缓存/共享（同一标题的两部影片各调用一次）。
- 不把译名写入 `videos.title` 或卡片显示名，避免覆盖用户可编辑的标题。
