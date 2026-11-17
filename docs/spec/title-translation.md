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
- **凭证加密保存。** API Key 与 JavBus / FC2 Cookie 一样走 Windows DPAPI，没有明文兜底，也不回传前端。

## 2. 数据模型（Migration v27）

`video_jav` 增加两列，纯增量、无回填：

| 列 | 含义 |
| --- | --- |
| `title_zh` | 成功翻译后的简体中文片名；空字符串表示还没有译名 |
| `title_zh_error` | 最近一次翻译失败的原因；成功或从未尝试时为空 |

成功与失败互斥：`SetTitleTranslation` 每次写入都会清掉另一列，读取时由 `titleZhStatus` 统一解释：

| `title_zh` | `title_zh_error` | 状态 |
| --- | --- | --- |
| 非空 | 空 | `succeeded` |
| 空 | 非空 | `failed` |
| 空 | 空 | `none` |

`pending`（翻译中）**不落库**：它只是服务内存里 `titlePending` 集合的投影。这样进程崩溃不会留下永远停在「翻译中…」的行；重启后该影片回到 `none`，由下一次刮削或批量翻译重新排队。

重新刮削会替换 `title`，因此 `SaveSuccess` 同时清空 `title_zh` / `title_zh_error`，避免旧译名继续描述新标题。

## 3. 配置

`settings.Store` 新增（均复用通用 key/value 表）：

| key | 内容 |
| --- | --- |
| `llm_enabled` | `"0"` 表示暂停自动翻译；缺省视为启用 |
| `llm_base_url` | OpenAI 兼容接口根地址；缺省 `https://api.deepseek.com/v1` |
| `llm_model` | 模型名；缺省 `deepseek-chat` |
| `llm_api_key_dpapi` | DPAPI 加密后的 API Key |

`llm.Normalize` 负责校验：只接受 `http(s)`、必须带主机名、拒绝查询参数/片段/换行；地址与模型为空时补默认值。保存表单时 API Key 留空表示**保留原 Key**，因此修改接口或模型不需要重新输入密钥。

`baseURL && model && apiKey` 齐备时 `llm.Client.Available()` 为真，手动翻译（详情页按钮、批量补齐）可用；再加上 `enabled` 才算 `Configured()`，刮削成功后才自动翻译。暂停自动翻译不会挡住手动操作，刮削流程也直接跳过，不写任何失败状态。

## 4. 请求与输出

- 客户端：`backend/llm.Client`，实现 `jav.TitleTranslator`（`TranslateTitle` + `Configured` + `Available`）。
- 每次翻译都重新读取设置，因此改配置无需重启；HTTP 复用刮削代理（`netproxy.Config.Transport()`），LLM 与元数据请求走同一条网络路径。
- 单次请求 60 秒超时；提示词要求「只输出译名、保留番号/系列/人名、已是中文则原样返回、40 字以内」。
- 输出清洗：取第一行、去掉 `中文译名：` 之类前缀与包裹引号、上限 200 字符；空结果按失败处理。
- 错误映射成可操作的中文：401/403 提示检查 API Key，404 提示接口/模型，429 提示稍后重试，5xx 提示服务不可用，网络错误提示无法连接。

## 5. 队列与状态流转

`jav.Service` 增加一条独立的单 worker 队列（与刮削 worker 分开）：

- 刮削成功后，若 provider 已配置且 `title` 非空，则 `enqueueTitleTranslation(id)`；`titlePending` 去重，重复入队无效。
- `Get` 读取详情时，如果 id 在 `titlePending` 中，把状态覆盖成 `pending`，供详情页显示「翻译中…」。
- worker 完成后先写库、再清 `titlePending`、最后发 `jav:scrape:updated`。顺序很重要：前端收到事件会立即重读，pending 必须先清除，否则已完成的译名会继续显示成翻译中。
- 失败写入 `title_zh_error` 并发事件；不自动重试，避免用错误的配置反复请求。
- 批量入队（`TranslateMissingTitles`）只发一次结果事件，避免 N 条「待刷新」事件同时打到前端。

## 6. API

`api.Library`（两端共享的业务门面）：

- `TranslateJavTitle(videoID) error`
- `TranslateMissingJavTitles() (int, error)`

Wails（仅桌面端设置/操作）：

- `GetLLMSettings() (LLMSettings, error)` —— 永远不返回 Key，只给 `hasApiKey`
- `SetLLMSettings(LLMSettingsInput) error`
- `ClearLLMAPIKey() error`
- `TranslateVideoTitle(videoID) error`
- `TranslateMissingTitles() (int, error)`

`jav.Detail` 新增 `titleZh` / `titleZhStatus` / `titleZhError`，因此桌面详情、局域网 JSON API 和手机详情页都自动拿到译名，不需要额外端点。

## 7. 前端

- **设置 → 片名翻译**：接口地址、模型、API Key、启用开关、保存/清除 Key，以及「翻译缺少译名的片名」批量入口。状态只显示「已配置 / 已暂停 / 未配置」。
- **桌面详情页**：元数据表新增「中文译名」一行；标题存在时提供「翻译片名 / 重新翻译」按钮，失败时在按钮下方显示原因。
- **手机详情页**：只读显示「中文译名」（翻译中 / 翻译失败 / 尚未翻译），不提供触发入口。

## 8. 测试

- `backend/llm`：默认值与校验、输出清洗、请求体（模型 + 提示词）、401 映射、设置实时生效、设置读取失败视为未配置。
- `backend/settings`：默认配置、往返读写、无效地址拒绝、Key 加密存储（Windows）、非 Windows 平台无安全存储时的降级行为。
- `backend/database`：v26 → v27 增量升级、已有刮削数据保留、译名跨再次迁移保留。
- `backend/jav`：刮削后自动翻译、未配置时跳过、失败不改变刮削状态、批量去重、`pending` 覆盖、重新刮削替换旧译名、仓库层候选查询。

## 9. 非目标

- 不做批量并发翻译（单 worker，避免触发 provider 限流）。
- 不做译名人工编辑（译名是 LLM 派生的只读字段；重新翻译即可）。
- 不做翻译结果缓存/共享（同一标题的两部影片各调用一次）。
- 不把译名写入 `videos.title` 或卡片显示名，避免覆盖用户可编辑的标题。
