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
        +--> 入队「中文译名」翻译（独立队列，单 worker）
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

## 2. 数据模型（Migration v27）

`video_jav` 增加两列，纯增量、无回填：

| 列 | 含义 |
| --- | --- |
| `title_zh` | 成功翻译后的简体中文片名；空字符串表示还没有译名 |
| `title_zh_error` | 最近一次翻译失败的原因；成功或从未尝试时为空 |

成功与失败通常互斥：`SetTitleTranslation` 每次写入都会清掉另一列。例外是「重新翻译已有译名时失败」：`SetTitleTranslationFailure` 只更新错误列、保留旧译名，此时两列同时非空，`titleZhStatus` 按 `failed` 报告并在详情页显示失败原因，旧译名仍然可读；下一次成功会清掉错误。读取时由 `titleZhStatus` 统一解释：

| `title_zh` | `title_zh_error` | 状态 |
| --- | --- | --- |
| 任意 | 非空 | `failed`（旧译名可能仍在，见上） |
| 非空 | 空 | `succeeded` |
| 空 | 空 | `none` |

`pending`（翻译中）**不落库**：它只是服务内存里 `titlePending` 集合的投影。这样进程崩溃不会留下永远停在「翻译中…」的行；重启后该影片回到 `none`，由下一次刮削或批量翻译重新排队。

重新刮削会替换 `title`，因此 `SaveSuccess` 同时清空 `title_zh` / `title_zh_error`，避免旧译名继续描述新标题。

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

- 客户端：`backend/llm.Client`，实现 `jav.TitleTranslator`（`TranslateTitle` + `Configured` + `Available`）。
- 每次翻译都重新读取设置，因此改配置无需重启；HTTP 复用刮削代理（`netproxy.Config.Transport()`），LLM 与元数据请求走同一条网络路径。
- 单次请求 60 秒超时；提示词要求只输出最终简体中文译文（不输出原文、翻译过程、词汇解释或前言），并保留原文的语气、情绪与表达强度。
- 输出清洗：取第一行、去掉 `中文译名：` 之类前缀与包裹引号、上限 200 字符；空结果按失败处理。
- 错误映射成可操作的中文：401/403 提示检查 API Key，404 提示接口/模型，429 提示稍后重试，5xx 提示服务不可用，网络错误提示无法连接。

## 5. 队列与状态流转

`jav.Service` 增加一条独立的单 worker 队列（与刮削 worker 分开）：

- 刮削成功后，若 provider 已配置且 `title` 非空，则 `enqueueTitleTranslation(id)`；`titlePending` 去重，重复入队无效。
- `Get` 读取详情时，如果 id 在 `titlePending` 中，把状态覆盖成 `pending`，供详情页显示「翻译中…」。
- worker 完成后先写库、再清 `titlePending`、最后发 `jav:scrape:updated`。顺序很重要：前端收到事件会立即重读，pending 必须先清除，否则已完成的译名会继续显示成翻译中。
- 失败写入 `title_zh_error` 并发事件；不自动重试，避免用错误的配置反复请求。
- 批量入队（`TranslateMissingTitles`）只发一次结果事件，避免 N 条「待刷新」事件同时打到前端。
- 「重新翻译全部译名」（`RetranslateAllTitles`）用更宽的候选查询：所有已成功刮削、带标题的行都入队，**包括已有译名**。这条动作是给「改了提示词 / 换了模型」之后整库刷新用的：旧译名继续显示，直到新结果落库；单条刷新失败时只更新 `title_zh_error` 并保留旧译名（`finishTitleTranslationFailure`），详情页会显示失败原因，避免一次限流把整库译名清空。单条「重新翻译」按钮也遵循同样的保留行为。
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

- **设置 → 片名翻译**：接口地址、模型、API Key、启用开关、「启动时自动补齐缺失的译名」开关、保存/清除 Key、「测试连接」，以及「翻译缺少译名的片名」批量入口。另提供「重新翻译全部译名」：确认后把已有的译名也重新排队，用于调整提示词或更换模型后整库刷新。测试成功会直接显示示例译名（连接成功，示例译名：…），失败显示 401/404/429/网络等具体原因。状态只显示「已配置 / 已暂停 / 未配置」。
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
  - 写入侧本来也有限制：送模型的原标题 ≤400 字符，模型返回的译名入库前截到 ≤200 字符，提示词要求 ≤40 汉字。

## 8. 测试

- `backend/llm`：默认值与校验、输出清洗、请求体（模型 + 提示词）、401 映射、设置实时生效、设置读取失败视为未配置；`Check` 用显式配置（不读已存设置、暂停状态也可测）并返回示例译名，缺 Key / 鉴权失败给出可操作错误。
- `backend/settings`：默认配置、往返读写、无效地址拒绝、Key 加密存储（Windows DPAPI / macOS 钥匙串）、无安全存储平台上的降级行为。
- `backend/secret`：Windows DPAPI 往返；macOS 真实钥匙串写入/读取/覆盖隔离/幂等删除；不支持的平台返回 `ErrUnsupported`。
- `backend/database`：v26 → v27 增量升级、已有刮削数据保留、译名跨再次迁移保留。
- `backend/jav`：刮削后自动翻译、未配置时跳过、失败不改变刮削状态、批量去重、整库重译替换旧译名、重译失败保留旧译名、`pending` 覆盖、重新刮削替换旧译名、仓库层候选查询（缺失 / 全部两套工作列表）；`ListTitles` / `SearchTitleIDs` 的投影、跳过与回收站边界、关键词大小写与通配符转义。
- `backend/api`：`SearchVideos` 合并刮削字段（番号/原标题/译名）、不重复返回、并投影 `javTitle` / `javTitleZh`。
- 前端：Astryx `Text` 的截断 tooltip 用真实浏览器验证过（hover 被截断的卡片标题后出现 `role="tooltip"`，内容为完整片名）。

## 9. 非目标

- 不做批量并发翻译（单 worker，避免触发 provider 限流）。
- 不做译名人工编辑（译名是 LLM 派生的只读字段；重新翻译即可）。
- 不做翻译结果缓存/共享（同一标题的两部影片各调用一次）。
- 不把译名写入 `videos.title` 或卡片显示名，避免覆盖用户可编辑的标题。
