# 后台任务看板（统一进度视图）技术设计

**状态：第一阶段已实现（桌面端），移动端页面与行内动作留待第二阶段。** 实现与本文档的差异见 §12。

**目标：** 一个界面回答四个问题——现在有什么在后台跑、跑到哪了、还剩多少、上一次结果如何。首个必须覆盖的任务是「片名翻译」批量补齐（目前只能看日志和数据库），同时把已有的刮削、查重扫描、文件检测、演员补齐收进同一张视图。

**非目标：** 不重写各任务的执行器（尤其刮削）；不做通用任务调度器；不新增表；不为翻译队列做持久化。

---

## 1. 现状盘点

| 任务 | 执行器 | 状态来源 | 现在能看到什么 | 持久化 |
| --- | --- | --- | --- | --- |
| 刮削批量 | `jav.Service` 的 2 个 worker | `jav_scrape_tasks` 表 | 看板按 进行中/已完成/失败 分组，每部影片一条 | 是（可恢复、重试、租约） |
| 片名翻译 | `jav.Service` 的 1 个 title worker | 内存 `titleJobs` + `titlePending` | **什么都没有**，只有日志 | 否（队列不落库，只有 `title_zh` 结果落库） |
| 内容查重扫描 | `video.Service` | 内存 `ContentScanStatus` | 查重页一条 `ProgressBar`（两阶段计数） | 否（可续跑） |
| 文件检测 | `video.Service` | 内存 `FileScanStatus` | 工具栏按钮 + 结果通知 | 否 |
| 演员资料补齐 | `jav.Service` | 内存 `ActorFillStatus` | 演员索引页一行摘要 | 否（`actor_fill` 状态落库） |
| 缩略图/元数据回填 | `video.Service` | 无计数（`BackfillProcessing` 只返回数量） | 无 | 结果落库 |
| 目录监控扫描 / 图片清扫 / 云盘健康 | `watch` / `video` / `cloud` | 一次性或周期轮询 | 事件 + toast | — |

观察：除了翻译，其余任务都已经有**某种**进度表示，只是分散在各自页面里；翻译是唯一完全没有进度视图的。

## 2. 关键判断：统一「视图」，不统一「执行器」

「把刮削任务看成一种后台任务」在**展示层**成立，在执行层不成立。刮削任务不是一个队列，而是一台持久状态机：

- `jav_scrape_tasks` 有 `status / attempts / lease_until / next_retry_at / error_code`，服务启动时 `recoverPending()` 回收中断任务，扫描器按冷却时间重试；
- 看板的语义是「每部影片的当前任务」，不是「一批任务」——它没有批次 id，也没有总进度；
- 它有两端入口（桌面看板 + 移动端看板）、HTTP 写端点和一批已有测试。

把它抽象成通用「后台任务队列」意味着重新定义租约、重试、错误码、明细与恢复语义，并迁移现有数据——收益只是多一个进度条，风险远大于收益。

**方案：`后台任务` 是一层只读聚合读模型。** 每个执行器保留自己的实现与明细页，各自暴露一个「状态快照」；`api.Library` 把快照拼成一张列表；看板只负责展示与跳转。这层关系与现有的 `TaskOverview`（任务表 → 看板投影）同构，只是投影范围从 jav 扩大到全库。

```
 jav.ScrapeTaskCounts ─┐
 jav.TitleTranslationStatus ─┤
 video.ContentScanStatus ────┼─→ api.Library.BackgroundTasks() ─→ Wails / GET /api/tasks
 video.FileScanStatus ───────┤
 jav.ActorFillStatus ────────┘
```

## 3. 统一数据模型

新包 `backend/tasks`，只放类型与排序辅助，不含业务依赖（避免 `jav`/`video` 反向依赖 api）。

```go
package tasks

// Kind 决定行的图标与默认动作。
type Kind string // "batch" | "scan"

// State 是展示层状态：running 表示结束前，done/failed 表示上一轮已经结束。
type State string // "running" | "done" | "failed"

type Task struct {
    ID        string  `json:"id"`        // "title-translation" | "scrape" | "content-scan" | …
    Title     string  `json:"title"`     // 片名翻译
    Kind      Kind    `json:"kind"`
    State     State   `json:"state"`
    Done      int     `json:"done"`
    Total     int     `json:"total"`     // 0 = 总数未知（进度条走 indeterminate）
    Failed    int     `json:"failed"`    // 不改变 State，只作为说明的一部分
    Unit      string  `json:"unit"`      // 条 / 部 / 个文件
    Current   string  `json:"current,omitempty"` // 当前条目：番号或文件名
    Message   string  `json:"message,omitempty"` // 阶段说明 / 最近一次结果
    Error     string  `json:"error,omitempty"`
    StartedAt int64   `json:"startedAt,omitempty"` // unix ms
    UpdatedAt int64   `json:"updatedAt,omitempty"`
    Detail    string  `json:"detail,omitempty"`    // 深链："scrapeTasks" | "duplicates" | …
}

type Snapshot struct {
    Tasks []Task `json:"tasks"`
}
```

约定：

- **不伪造总数。** `Total = 0` 时前端显示不确定进度（旋转 + 计数），而不是百分比。
- **不倒退。** `Total` 只能单调增加（翻译运行中允许单条入队），`Done` 单调不减。
- **结束保留一轮。** 任务结束后 `State` 变为 `done/failed`，保留计数与 `StartedAt`，直到下一轮开始被覆盖；这样「上次补了 297 条」不需要持久化。
- **无批次语义的任务不给百分比。** 刮削任务没有批次 id，硬凑百分比会在刷新时跳变、倒退，见 §4.2。
- 排序：`running` 在前（按 `StartedAt` 升序），随后 `done/failed`（按 `UpdatedAt` 倒序）；`Total == 0 && Done == 0` 的空闲任务不进列表。

## 4. 各任务的进度来源

### 4.1 片名翻译（核心新增）

在 `backend/jav/title_translation.go` 增加一个纯内存状态，由 `titleMu` 保护，随队列自然形成「一轮」：

```go
type TitleTranslationStatus struct {
    Active    bool   `json:"active"`
    Total     int    `json:"total"`
    Done      int    `json:"done"`
    Failed    int    `json:"failed"`
    Queued    int    `json:"queued"`    // 排队中（不含在飞）
    Current   string `json:"current,omitempty"` // 正在翻译的番号/片名
    StartedAt int64  `json:"startedAt,omitempty"`
    UpdatedAt int64  `json:"updatedAt,omitempty"`
}
func (s *Service) TitleTranslationStatus() TitleTranslationStatus
```

轮次规则（在 `enqueueTitleTranslation` / `translateTitle` / `finishTitleTranslation` 中维护）：

1. 入队时若 `len(titlePending) == 0`（上一次已空闲），重置 `Done/Failed/Total` 并记 `StartedAt`；随后 `Total++`（批量入队 525 次即 `Total = 525`，`Queued = 525`）。
2. worker 取到任务后设置 `Current`（`Detail.Code`，为空时退回片名截断），开始请求。
3. `finishTitleTranslation` 成功 `Done++` / 失败 `Failed++`，更新 `UpdatedAt`，清 `Current`。
4. `Active = len(titlePending) > 0`；队列排空后本轮的计数保留到下一轮被覆盖。
5. 事件：`finishTitleTranslation` 已有 `emitUpdate(video)`。再发一个节流后的 `tasks:updated`（≤1 次/秒，合并突发），供看板与其他视图刷新；不改变现有明细页行为。

与「启动时自动补齐」开关天然衔接：开关打开时 `Start()` 的续跑会让看板在启动几秒内出现 `200/525` 这样的行；开关关闭时看板为空态，与队列不持久的事实一致。

### 4.2 刮削任务（只给队列深度，不给百分比）

`jav.Repository` 新增一个廉价聚合查询（`GROUP BY`），避免每次刷新把全库任务行（`TaskOverview`）读回来：

```go
type ScrapeTaskCounts struct {
    Queued    int `json:"queued"`
    Running   int `json:"running"`
    Retryable int `json:"retryable"`
    Failed    int `json:"failed"`
    Succeeded int `json:"succeeded"`
    LastFinishedAt int64 `json:"lastFinishedAt"`
}
```

看板行显示：`进行中 4（刮削中 1 / 排队 2 / 待重试 1）· 最近完成 19:48`，`Total = 0`，进度条 indeterminate；「查看明细」跳到现有刮削任务看板（`filter.kind = "scrapeTasks"`）。

**为什么不给 `300/812`：** 刮削没有「这一轮」的概念，用户按「刮削缺失」时入队的是当时的候选集，运行期间可以不断追加；任何分母都会在刷新时变化，进度条会倒退。看板的分组计数已经回答了「还剩多少」，不需要在总览里重复。
### 4.3 其余任务（直接复用现有状态）

| 任务 | 映射 |
| --- | --- |
| 内容查重扫描 | `examining` 阶段：`Done=Examined, Total=ExamineTotal`；`comparing` 阶段：`Done=Done, Total=Total`；`Message=阶段名` |
| 文件检测 | `Done=Checked, Total=Total, Failed=Removed`，`Message` 带上 offline/skipped 数量 |
| 演员资料补齐 | `Done=Succeeded+NotFound+Failed, Total=Total, Failed=Failed, Current=Current` |

缩略图/元数据回填先不进看板：启动后几秒内完成，且当前只有数量没有总数（需要另加计数），留到第二阶段。

## 5. 后端装配与 API

- `api.Library.BackgroundTasks() tasks.Snapshot`：依次读各服务状态，拼成 `[]tasks.Task`；任何单个来源读失败只记日志并跳过该行，不影响整张视图。
- Wails：`app.go` 增加 `BackgroundTasks() (tasks.Snapshot, error)`（只读）。
- HTTP：`GET /api/tasks`（只读，移动端用）；`server_test` 覆盖 200 与空列表。
- 不新增写端点。停止/取消属于第二阶段（见 §8）。

## 6. 前端设计

### 6.1 取数位置与共享规则

`check-shared-ui.mjs` 规定：`views/<x>/use*.ts` 形式的共享视图模型必须**两端都被引用**，并在 `views/manifest.json` 里认领。因此第一阶段（仅桌面）不要把取数放进 `views/`：直接在 `components/BackgroundTasksView.tsx` 里用 `useLiveQuery` 取 `client.backgroundTasks`；第二阶段做移动端页面时，再把取数抽到 `views/tasks/useBackgroundTasks.ts` 并登记 manifest。两侧取数逻辑本身很简单（读一次快照 + 轮询/事件刷新），提前抽象反而会触发规则检查。

数据契约放在 `services/client.ts` 的 `MuseClient`（两端都能读），实现分别在 `client.wails.ts`（Wails 绑定）与 `client.http.ts`（`GET /api/tasks`）；桌面组件经门面取数，符合既有分层。

### 6.2 桌面端

- **入口**：`Sidebar.tsx` 的「工具」区新增「后台任务」（复用 `filter.kind = "tasks"` 的既有机制，与「刮削任务」「查重」并列）。
- **面板**：`components/BackgroundTasksView.tsx`，无任务时显示空态「目前没有后台任务」+ 「最近完成」区块（保留的上一轮）。
- **行样式**：标题 + 状态徽标 + `ProgressBar`（`Total == 0` 时不画进度条，改为 `Spinner` + 计数）+ 当前条目 + 说明 + 「查看明细」。
- **头部指示**：有 `running` 行时，头部显示一个「任务 N」的小指示（`Spinner` + 数字），点击切到任务页；空闲时不渲染，避免打扰。
- **刷新**：桌面端由 `tasks:updated` / `jav:scrape:updated` 等事件触发 `invalidate()`；`useLiveQuery` 的 200ms 合并已经存在。
- 组件只用 `ui/` 里的 `ProgressBar`、`Badge`、`Spinner`、`Button`，不新增裸控件。

### 6.3 浏览器端（第二阶段）

`mobile/App.tsx` 的 `工具` 区新增「任务」tab，复用同一个取数（抽出 `views/` 之后）；只读，不提供停止/重试。浏览器没有 Wails 事件：有 `running` 行时 2 秒轮询一次，全部结束即停（照抄 `views/duplicates/useContentDuplicates.ts` 的做法）。

速率与 ETA 放在共享工具 `utils/tasks.ts`：`Done >= 3` 且跨度 > 5s 才显示「约 10 条/分 · 预计 33 分钟」，否则只显示 `200/525`。

## 7. 刷新与开销

| 端 | 机制 | 开销 |
| --- | --- | --- |
| 桌面 | 事件 `tasks:updated`（节流 1/s）→ `invalidate()` → `useLiveQuery` 再合并 200ms | 每次推进最多 1 次读，响应 2 条 `GROUP BY` + 内存快照 |
| 浏览器 | 运行中 2s 轮询 | 每次一个轻量 JSON |
| 结束 | 最后一条结果落库后发一次事件 | 看板立即显示「上次：297 条完成，0 失败」 |

`GET /api/tasks` 不读影片列表、不读 `videos` 表，只读状态与计数，成本与任务量无关。

## 8. 分阶段

**第一阶段（建议先做，覆盖本次需求）：**

1. `backend/tasks` 类型 + `jav.TitleTranslationStatus`（含测试：轮次重置、单条入队、成功/失败计数、排空后保留）。
2. `jav.ScrapeTaskCounts` + `api.Library.BackgroundTasks`（翻译、刮削、查重、文件检测、演员补齐五行）+ HTTP 端点 + Wails 绑定。
3. `tasks:updated` 节流事件 + `events.ts` 注册。
4. 桌面：侧边栏入口、`BackgroundTasksView`（组件内取数）、头部指示、ETA 工具。
5. 文档：本文档更新状态与实现差异。

**第二阶段（按需）：**

- 移动端「任务」tab；
- 行内动作：停止本次翻译（清空 `titlePending`、取消在飞请求）、从任务页直达批量入口；
- 缩略图回填进度、最近完成历史（最多 N 条的环形缓冲，不落库）；
- 深链：翻译行跳到 `设置 → 片名翻译`（需要 Settings 支持按 section 打开）。

## 9. 风险与取舍

- **内存进度重启即失。** 与「队列不持久」一致，看板重启后为空态，不假装续跑；打开自动补齐开关时会从新一轮开始。
- **ETA 可能不准。** 只在样本足够时显示，且标注「约」；不做倒计时。
- **总数不确定的任务不给百分比**，避免阶段切换（查重 examining → comparing）时进度条倒退。
- **多端同开**：移动端轮询延迟最多 2s，可接受（只读）。
- **`api.Library` 变胖**：聚合逻辑只做映射与容错，不承载业务规则；各任务的语义仍留在各自服务里。
- **事件风暴**：`tasks:updated` 必须节流，否则 525 条翻译会打 525 次前端刷新（现有 `useLiveQuery` 200ms 合并仍不足以抵消事件驱动的重复读取）。

## 10. 待 review 的决策点

| # | 问题 | 建议 |
| --- | --- | --- |
| 1 | 是否把刮削改造成通用任务执行器？ | **否**，只做只读聚合（§2） |
| 2 | 后台任务入口放哪？ | 侧边栏「工具」区新增一项，复用现有 tab/filter 机制；头部只做运行中指示 |
| 3 | 刮削行是否显示百分比？ | 不显示，只给队列深度与明细跳转（§4.2） |
| 4 | 是否显示 ETA？ | 显示「速率 + 预计」，样本 ≥3 条才出现 |
| 5 | 第一阶段是否包含停止/取消？ | 不包含，先只读；取消需要与 worker 的 context 和队列一起设计 |
| 6 | 移动端第一阶段是否要页面？ | 端点先做，页面放第二阶段 |
| 7 | 翻译结束后是否保留「上次结果」？ | 保留到下一轮开始（内存即可，不落库） |

## 11. 涉及文件（预估）

后端：`backend/tasks/tasks.go`（新）、`backend/jav/title_translation.go`、`backend/jav/service.go`、`backend/jav/repository.go`、`backend/api/library.go`、`app.go`、`backend/server/server.go`。

前端（第一阶段）：`frontend/src/services/client.ts` + `client.wails.ts` + `client.http.ts` + `api.ts` + `desktop.ts`、`frontend/src/types.ts`、`frontend/src/components/BackgroundTasksView.tsx`（新）、`frontend/src/pages/Library.tsx`、`frontend/src/components/Sidebar.tsx`、`frontend/src/services/events.ts`、`frontend/src/utils/tasks.ts`（新）。

前端（第二阶段）：`frontend/src/views/tasks/useBackgroundTasks.ts`（新，从组件里抽出取数）、`frontend/src/views/manifest.json`、`frontend/src/mobile/components/BackgroundTasksView.tsx`（新）、`frontend/src/mobile/App.tsx`。

测试：`backend/jav` 翻译状态与刮削计数、`backend/api` 聚合与容错、`backend/server` `GET /api/tasks`；前端无测试框架，靠 lint/build 与手动核对。

## 12. 实现差异（第一阶段）

- **翻译计数已适配批量 + 并发实现**：worker 现在每次向 provider 发一批最多 10 条、最多 5 个请求并发（见 `46f6b78`、`8983a1d`）。进度仍按「行」计数（`Done`/`Failed`），`InFlight` 报告同时在飞的请求数，`Current` 是最近一批的标签（「IPX-633 等 10 部」）。跳过（记录已不存在）的行计入 `Failed`，保证 `Done + Failed == Total`。
- **事件节流**：`tasks:updated` 在 `jav.Service` 内按 1 秒窗口合并（尾触发），不是每个结果一条。
- **头部运行中指示未做**：留在第二阶段；侧边栏入口与页面本身已包含「N 个进行中」。
- **入口**：侧边栏「工具 → 后台任务」（`filter.kind = "tasks"`），与刮削看板、查重并列。
- **HTTP 端点**：`GET /api/tasks` 已就绪（移动端页面第二阶段再接入）。
- **深链**：刮削行 → 刮削看板；查重扫描 → 查重页；演员补齐 → 演员索引。翻译行暂不提供深链。
- 测试位置：`backend/jav/background_tasks_test.go`、`backend/api/background_tasks_test.go`、`backend/tasks/tasks_test.go`、`frontend/src/utils/tasks.test.ts`。

