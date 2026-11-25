# 精彩时刻：时刻截图与时间段循环视频

**状态：已实现（2026-11-25，schema v31）。**

**目标：** 在影片详情中记录「精彩时刻」。用户输入一个时刻，生成一帧截图；输入一个时间段，生成循环视频（MP4）。生成走后台任务队列，不阻塞界面，进度出现在现有「后台任务」看板。

**核心约束：** 仍然遵守「收藏 ≠ 文件」。只读取源视频生成派生图片，绝不复制、移动、修改或删除源视频；删除精彩时刻只删记录与生成文件。

---

## 1. 已确认的产品行为

| 决策 | 结论 |
| --- | --- |
| 增删平台 | 桌面端新增、重试、删除；移动端只读，不新增 HTTP 写端点 |
| 数量 | 每部影片不设上限，全局队列不设硬上限 |
| 输入 | 时/分/秒三个数字框 × 开始（必填）+ 结束（可选） |
| 识别 | 结束三个框全空 = 时刻截图；任一格有值 = 时间段循环视频 |
| 精度 | 整数秒；分/秒 0–59，小时不限 |
| 校验 | 实时提示 + 禁用保存，不自动改写输入；范围 1–20 秒、结束晚于开始、不超视频时长 |
| 展示 | 只在详情页，网格卡片不出现 |
| 生成 | 后台单 worker，保存后立即关闭弹窗 |
| 快速时长 | 填好开始时间后可用 +5 / +10 / +15 / +20 秒一键设结束时间（不可用则置灰，不夹取） |
| 预览 | 保存前可用同一条 ffmpeg 指令生成一次真实预览；确认后保存直接采纳该文件，不再重复编码 |
| 取消 | 删除排队中/生成中的条目即取消；不提供独立取消按钮 |
| 恢复 | `queued` / `running` 重启后自动重排；连续中断 3 次标失败 |
| 失败 | 桌面保留失败卡 + 重试/删除；移动端隐藏失败条目 |
| 编辑 | 不支持编辑时间，只能删除后重新添加 |
| 时间段 | MP4（H.264，静音循环）/ 960 宽（不超过源宽）/ 60 fps / CRF 22；一次编码，不做体积回退 |
| 任务看板 | 新增「精彩时刻生成」行，完成后保留至下一轮或应用重启 |
| 手机刷新 | 仅存在排队/生成中条目时每 3 秒轮询 |

---

## 2. 用户流程

### 2.1 新增时刻（截图）

1. 桌面详情页「精彩时刻」区域点击「新增精彩时刻」。
2. 填写开始时间，结束时间留空。
3. 输入器实时显示「将生成截图：1:30」。
4. （可选）点「预览」，弹窗里出现这一帧的截图，标题写明时刻与文件大小；确认效果后保存。
5. 点击保存，弹窗立即关闭：**用预览保存时**图片当场出现（预览文件被采纳，不再编码一次）；
   没预览就保存时，详情出现「排队中」占位卡，后台任务看板出现进度行，worker 完成后替换为截图。
   标签「时刻 1:30」。

### 2.2 新增时间段（循环视频）

1. 填写开始时间；结束时间可以手填，也可以点 +5 / +10 / +15 / +20 秒快速设置。范围 1–20 秒。
2. 输入器实时显示「将生成循环视频：1:30–1:35（5 秒）」。
3. （可选）点「预览」：用正式保存时的同一条 ffmpeg 指令（同样 60 fps）生成一次，
   弹窗里直接播放循环视频，并标出「时间段 1:30–1:35 · 60 fps · 1.6 MB」。
4. 保存后：采纳预览则立即出现同一条目；否则走后台上生成。标签「时间段 1:30–1:35 · 5 秒」。
5. 改时间 = 换了问题：旧预览当场作废并删除，需要重新预览。关闭弹窗（取消 / Esc / 遮罩）
   也会丢弃未保存的预览。

### 2.3 失败与重试

- 生成失败时，桌面显示「生成失败」卡片，附错误文案、「重试」「删除」。
- 重试沿用原开始/结束时间，重新进入队列。
- 移动端不展示失败条目；任务看板仍会统计失败。
- 应用崩溃/退出导致的中断会自动重排；同一条超过 3 次中断后标失败，等待手动重试。

### 2.4 删除

- 删除任意状态都需二次确认，文案说明只删除生成的截图/视频，不影响源视频。
- 删除排队中条目：直接移出队列。
- 删除生成中条目：当前 ffmpeg 跑完但结果丢弃并清理。
- 删除已完成条目：删除记录与文件。

### 2.5 源文件不可用

- 影片缺失、外接盘未接入、网盘离线时，已生成的精彩时刻照常展示（它们是本地文件）。
- 「新增精彩时刻」按钮禁用并说明原因。
- 已在队列中的任务在 worker 执行时发现源不可用，标失败并给出对应原因。

---

## 3. 时间输入器与校验

### 3.1 输入器结构

`components/HighlightMomentDialog.tsx` 使用两行结构化输入：

```text
开始时间（必填）   [ 时 ] : [ 分 ] : [ 秒 ]
结束时间（可选）   [ 时 ] : [ 分 ] : [ 秒 ]   [清空结束时间]
常用时长                                     [ +5 秒 ] [ +10 秒 ] [ +15 秒 ] [ +20 秒 ]
```

- 只接受数字，`inputMode="numeric"`。
- 开始时间三个框全空 → 保存禁用，提示「请填写开始时间」。
- 结束三个框全空 → 时刻。
- 结束任一框有值（包括 0）→ 时间段；空着的子字段按 0 解释。
- 「清空结束时间」把三个结束框恢复为空，回到时刻模式。
- 不做自动跳格，避免输入分钟时被强行移走焦点。
- 开始时间是合法时刻后，「常用时长」那一行才出现：点 +N 秒把结束时间设成「开始 + N 秒」
  （写进那三格，走与手填完全相同的校验路径）。会超出视频结尾或超过 20 秒的按钮**置灰并说明原因**，
  不做夹取——把 +20 秒悄悄改成 +7 秒，等于保存了一段用户没要的时长。

### 3.2 实时校验

每次输入变化都计算：

| 条件 | 提示 |
| --- | --- |
| 开始全空 | 请填写开始时间 |
| 分或秒 > 59 | 分/秒需在 0–59 |
| 结束有值且 `end <= start` | 结束时间必须晚于开始时间 |
| 时间段长度 < 1 秒 | 时间段至少 1 秒 |
| 时间段长度 > 20 秒 | 时间段最长 20 秒（当前 N 秒） |
| 已知视频时长且 `start >= duration` | 开始时间超出视频时长 |
| 已知视频时长且 `end > duration` | 结束时间超出视频时长 |

- 任一错误立即标红并禁用保存；不等提交。
- 不自动夹取或改写用户输入。
- 提交时后端再次校验，防止绕过前端。

### 3.3 后端校验

`video.AddHighlight` 使用与前端相同的规则，返回中文错误：

- `startMs < 0`
- 已知时长时 `startMs >= durationMs`
- `endMs > 0` 时：`endMs <= startMs`、`endMs-startMs < 1000`、`> 20000`
- 已知时长时 `endMs > durationMs`

常量：

```go
const (
    MinHighlightRangeMs int64 = 1000
    MaxHighlightRangeMs int64 = 20000
)
```

---

## 4. 数据模型与迁移

### 4.1 schema v31

`backend/database/migration.go`，`currentSchemaVersion = 31`。

> 设计定稿时写的是 v29，但 v29（马赛克破解标记）与 v30（删除分组表）在实现前已经落地，
> 所以精彩时刻表顺延为 **v31**。迁移内容与下面的 SQL 完全一致。

```sql
CREATE TABLE IF NOT EXISTS video_highlights (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    video_id      INTEGER NOT NULL,
    kind          TEXT NOT NULL CHECK (kind IN ('moment','range')),
    start_ms      INTEGER NOT NULL,
    end_ms        INTEGER NOT NULL DEFAULT 0,
    media_path    TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'queued'
                  CHECK (status IN ('queued','running','ready','failed')),
    error         TEXT NOT NULL DEFAULT '',
    interruptions INTEGER NOT NULL DEFAULT 0,
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL,
    finished_at   INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_video_highlights_video ON video_highlights(video_id);
CREATE INDEX IF NOT EXISTS idx_video_highlights_status ON video_highlights(status);
```

- 迁移可重复执行，无数据回填。
- 不依赖外键（现有数据库未开启 `foreign_keys`）。
- 永久删除影片时，`sqliteRepository.Delete` 在一个事务里先删该影片的 `video_highlights`，再删 `videos`。
- 回收站保留记录与生成文件；恢复后 `ready` 条目照常展示。
- 不迁移、不删除旧数据；旧版本代码忽略这张表，也从不清扫 `highlights/`，文件会保留到升级。

### 4.2 Go 模型

`backend/video/highlight.go`：

```go
type Highlight struct {
    ID            int64  `json:"id"`
    VideoID       int64  `json:"videoId"`
    Kind          string `json:"kind"`       // moment | range
    StartMs       int64  `json:"startMs"`
    EndMs         int64  `json:"endMs"`      // moment 为 0
    MediaPath     string `json:"mediaPath"`  // 相对数据目录
    Status        string `json:"status"`     // queued | running | ready | failed
    Error         string `json:"error"`
    Interruptions int    `json:"interruptions,omitempty"`
    CreatedAt     int64  `json:"createdAt"`
    UpdatedAt     int64  `json:"updatedAt"`
    FinishedAt    int64  `json:"finishedAt"`
}

const (
    HighlightKindMoment   = "moment"
    HighlightKindRange    = "range"
    HighlightStatusQueued = "queued"
    HighlightStatusRunning = "running"
    HighlightStatusReady  = "ready"
    HighlightStatusFailed = "failed"
)
```

### 4.3 仓库接口

扩展 `VideoRepository`，并同步补齐测试用 `stubVideoRepo`、`memVideoRepo`：

```go
CreateHighlight(h *Highlight) error
GetHighlight(id int64) (*Highlight, error)                  // worker 用
GetVideoHighlight(videoID, id int64) (*Highlight, error)    // 归属校验
ListHighlights(videoID int64) ([]Highlight, error)          // 按 start_ms, id
ListQueuedHighlightIDs() ([]int64, error)
ListHighlightMediaPaths() ([]string, error)                 // 清扫用，含 pending
ClaimHighlight(id int64) (bool, error)                      // queued -> running
FinishHighlight(id int64, status, errMsg string, finishedAt int64) (bool, error)
RetryHighlight(videoID, id int64) (bool, error)             // failed -> queued
RecoverHighlights(maxInterruptions int) (requeue []int64, failed int, err error)
DeleteHighlight(videoID, id int64) error
```

`FinishHighlight` 返回 false 表示行已被删除；worker 据此丢弃生成文件，避免给已删除的条目写回状态。

---

## 5. 后台生成器

### 5.1 生命周期

`video.Service` 新增：

```go
func (s *Service) StartHighlights()
func (s *Service) CloseHighlights()
```

- `main.go` 在 `javSvc.Start()` 附近调用 `videoSvc.StartHighlights()`。
- 关闭步骤放在 thumbnail processor 之前：`shutdownStep{"highlight worker", videoSvc.CloseHighlights}`。
- 启动时 `RecoverHighlights(3)`：
  - `running` 行 `interruptions+1`；达到 3 次标 `failed`（「生成被多次中断」）；否则改回 `queued`。
  - `queued` 行直接重新入队。
  - 恢复的条目组成新一轮任务计数。
- 队列是内存 `workQueue[int64]`，数据库行是持久事实；单 worker。
- 关闭时取消服务 context、关闭队列、等待 worker。正在生成的行保持 `running`，留给下次启动恢复，不标失败。

### 5.2 新增

`AddHighlight(videoID, startMs, endMs)`：

1. `repo.Get(videoID)`，不存在返回 `ErrNotFound`。
2. 校验时间（见 §3.3）。
3. `processor.MediaPath(v)` 确认媒体可读；缺失、卷离线、远程离线返回明确中文错误。
4. 生成唯一媒体路径：
   `highlights/h-<videoID>-<unixNano>-<seq>.<jpg|mp4>`。
   `seq` 为进程内原子序号。路径在入队前写入数据库，清扫会把生成中的文件视为有主文件。
5. 插入 `status=queued` 行；入队；`noteQueued` 启动或延续本轮任务计数；发出 `video:highlight:updated` 和节流的 `tasks:updated`。
6. 立即返回该行；弹窗关闭。

若队列在关闭过程中拒绝入队，行保持 `queued`，下次启动恢复。

### 5.3 重试与删除

`RetryHighlight(videoID, id)`：

- 仅 `failed` 行；清空 `error`，置 `queued`，重新入队，计入当前/新一轮任务。
- 明确失败（源损坏、超时）不自动重试；只有崩溃中断自动重排。

`DeleteHighlight(videoID, id)`：

1. 读取行；不存在按幂等成功处理。
2. 删除记录。
3. 删除 `media_path` 对应文件（尽力而为）。
4. 若是 `queued`/`running`，计入本轮「取消」并结束 pending；worker 之后通过 rows-affected 判断，避免重复计数。
5. 若是 `ready`/`failed`，不改任务计数。

### 5.4 Worker

`processHighlight(id)`：

1. `GetHighlight(id)`；不存在或非 `queued` → 结束（若被删除，计取消）。
2. `ClaimHighlight(id)` 原子 queued → running；设置任务 `Current`（文件名 + 时间标签）。
3. `repo.GetAny(videoID)`：
   - 影片不存在 → 行已被级联删除，结束。
   - 影片在回收站 → 标 failed「影片在回收站中，恢复后可重试」。
   - 文件缺失 / 卷离线 / 远程离线 → 标 failed 对应中文原因。
4. 调用生成器（服务 context）：
   - 时刻：截图。
   - 时间段：MP4（H.264，静音，60 fps，960 宽且不超过源宽）。一次编码，没有体积回退——20 秒上限下实测约 1.6 MB，降帧降质都换不回有意义的东西。
5. 若服务 context 被取消（退出）→ 不更新行，保持 `running`。
6. 成功：`FinishHighlight(ready)`；返回 false 表示行已被删，删除生成文件；否则计完成。
7. 失败：删除半成品；`FinishHighlight(failed, error)`；返回 false 时删除文件；否则计失败。
8. 状态变化发出 `video:highlight:updated`；任务事件节流到最多 1 次/秒。

### 5.5 错误文案

统一中文，桌面失败卡直接展示：

- `生成精彩时刻失败：<ffmpeg 首行>`
- `源文件缺失，无法生成精彩时刻`
- `磁盘未接入，无法生成精彩时刻`
- `网盘离线，无法生成精彩时刻`
- `影片在回收站中，恢复后可重试`
- `生成被多次中断`

---

## 6. 任务看板

### 6.1 状态结构

`backend/video` 暴露只读快照：

```go
type HighlightTaskStatus struct {
    Active    bool
    Total     int
    Done      int // 成功 + 取消
    Failed    int
    Canceled  int
    Current   string
    StartedAt int64
    UpdatedAt int64
}
```

- 不变式：`Done + Failed + 当前 pending == Total`。
- 取消计入 `Done`，另记 `Canceled` 用于文案。
- 队列空闲时开始新一轮并重置计数；结束后保留到下一轮或应用重启（与片名翻译一致）。

### 6.2 投影

`api.Library.BackgroundTasks()` 新增 `highlightTask()`：

```go
state := tasks.StateDone
if status.Active {
    state = tasks.StateRunning
}

tasks.Task{
    ID:        "highlight-moments",
    Title:     "精彩时刻生成",
    Kind:      tasks.KindBatch,
    State:     state,
    Done:      status.Done,
    Total:     status.Total,
    Failed:    status.Failed,
    Unit:      "条",
    Current:   status.Current,
    Message:   "生成中 1 · 排队 N" / "完成 N · 失败 M · 取消 K",
    StartedAt: status.StartedAt,
    UpdatedAt: status.UpdatedAt,
}
```

- 无活动且本轮没有计数时不显示。
- 不提供 `Detail` 深链；当前没有专门的精彩时刻任务明细页。
- 执行器仍归 `video.Service`；看板只做只读投影，符合 `background-tasks.md` 的「统一视图，不统一执行器」。

### 6.3 事件与刷新

- `tasks:updated`：任务计数变化，节流 1 次/秒。
- `video:highlight:updated`：单条状态变化。
- 两者都加入 `frontend/src/services/events.ts` 的失效列表；桌面端 `useLiveQuery` 自动刷新详情和看板。
- 移动端没有推送，靠详情页轮询（见 §8.4）。

---

## 7. 生成规格与压缩

### 7.1 截图

- 输出 `<base>.jpg`，宽 1280，`-q:v 3`。
- ffmpeg 参数：

```text
-y -hide_banner -loglevel error -ss <at> -i <video>
-frames:v 1 -vf scale=1280:-2 -q:v 3 <out.jpg>
```

### 7.2 时间段（循环视频）

- 输出 `<base>.mp4`，宽 960（超过源宽时保持源宽），60 fps，H.264（yuv420p），静音，可循环播放。
- ffmpeg 参数：

```text
-y -hide_banner -loglevel error
-ss <start> -t <duration> -i <video>
-an
-vf "fps=60,scale='min(960,iw)':-2:flags=lanczos,setsar=1"
-c:v libx264 -preset medium -crf 22 -pix_fmt yuv420p
-movflags +faststart
<out.mp4>
```

- 一次编码，没有体积回退：20 秒、960 宽、60 fps 实测约 1.6 MB（2.5 秒完成），远低于 GIF 时代
  触发降级的量级，降帧只会换来一段更差的片段。
- `setsar=1` 去掉非方形象素比，浏览器不会把画面拉宽。
- `+faststart` 把索引搬到文件头，手机在局域网里边下边放。
- 超时：本地 60 秒，远程 90 秒。
- 失败删除半成品文件。

### 7.3 为什么是 MP4 而不是 GIF

第一版是 GIF。它能用，但有两个封顶：**帧率**和**体积**。

- **帧率**：GIF 每帧延迟以 1/100 秒存储，50 fps 正好 2 cs，是浏览器认可的最短延迟；再快就写成
  1 cs / 2 cs 交替，而小于 2 cs 会被 Chromium / Firefox 钳制成 10 cs（100 ms），于是每隔一帧
  冻结 0.1 秒，比 25 fps 还卡。60 fps 的 GIF 是做不到的，不是参数问题。
- **体积**：GIF 没有真正的帧间预测（只有"这块没变就透明"），熵编码是 LZW，128 色 + 抖动又把
  画面打成高频噪点。同样 10 秒、480 宽、MP4 60 fps 比 GIF 25 fps 小 **20 倍上下**（普通画面
  1.5 MB → 77 KB；满屏颗粒 18 MB → 762 KB）。

换 MP4 后 60 fps 是真 60 fps，全彩，无抖动，编码也快得多。代价是播放端从 `<img>` 变成
`<video autoplay muted loop playsinline>`：静音是浏览器自动播放的前提，`playsinline` 防止 iOS
抢全屏，低电量模式下自动播放被拒时盖一个播放按钮兜底（`ui/clip.tsx` 的 `LoopingClip`）。

**升级兼容**：MP4 之前生成的 `.gif` 条目照常显示（媒体路由与清扫的白名单保留 `gif`）。只有
被重试或重启恢复的旧条目会在生成前换成新的 `.mp4` 文件名——否则 MP4 字节会顶着 `.gif` 扩展名
被服务成错误的 Content-Type。
---

## 8. 传输与前端

### 8.1 门面与 Wails

`backend/api/library.go`：

```go
ListVideoHighlights(videoID int64) ([]video.Highlight, error)
AddVideoHighlight(videoID, startMs, endMs int64) (*video.Highlight, error)
RetryVideoHighlight(videoID, highlightID int64) error
DeleteVideoHighlight(videoID, highlightID int64) error
```

`app.go` 暴露同名 Wails 方法。`frontend/wailsjs` 由 `wails generate module` 重新生成，不手改。

### 8.2 局域网 HTTP（只读）

- `GET /api/videos/{id}/highlights` → 返回全部状态（移动端自行过滤 failed）。
- `GET /highlights/{file}` → 严格匹配 `^h-\d+-\d+-\d+\.(jpg|gif|mp4)$` 后 `http.ServeFile`，`Cache-Control: no-cache`。
- 两个端点都自动受 `backend/server/auth.go` 的 `withLANAuth` 保护，不申请公开豁免；移动端沿用现有局域网访问口令。
- 不新增 HTTP 写端点。

### 8.3 桌面资源服务

`main.go` 的 `thumbnailHandler` 增加 `highlightDir` 和 `/highlights/` 路由，使用同一严格正则；启动时创建 `<dataDir>/highlights`。

### 8.4 前端契约与共享工具

- `types.ts`：`export type Highlight = video.Highlight;`
- `services/client.ts`：`listVideoHighlights(videoId): Promise<Highlight[]>`，两端共用。
- `client.wails.ts` / `client.http.ts` 分别实现；`services/http.ts` 增加 GET。
- `services/api.ts` / `services/desktop.ts`：`addVideoHighlight`、`retryVideoHighlight`、`deleteVideoHighlight`（仅桌面）。
- `utils/highlightTime.ts`：
  - `timePartsToMs(parts)`：把时/分/秒字符串转毫秒；全空返回 `null`。
  - `validateHighlightTimes(startMs, endMs, durationMs)`：返回中文错误或 `null`。
  - `formatHighlightTime(ms)`：`0:00` / `1:30` / `1:02:03`。
  - `highlightLabel(h)`：`时刻 1:30` / `时间段 1:30–1:35 · 5 秒`。
  - `isHighlightPending(h)`、`isHighlightReady(h)`、`mobileVisibleHighlights(list)`（过滤 `failed`）。
- `utils/format.ts` 增加 `highlightURL(h)`：
  `/${h.mediaPath}?v=${h.updatedAt || h.createdAt}`；只在 `ready` 时调用。
- 新增 `views/highlights/useVideoHighlights.ts`：
  - `useLiveQuery`，key `highlights:${videoId}`。
  - 可选 `pollWhilePending`：存在 `queued`/`running` 时每 3 秒 `reload()`，否则不建定时器。
  - 桌面传 `false`（事件刷新），移动端传 `true`。
- `views/manifest.json` 增加 `video-detail` 视图条目，并在 gaps 记录：移动端不支持新增/重试/删除，失败条目隐藏。

### 8.5 桌面新增弹窗

`components/HighlightMomentDialog.tsx`：

- 两行时间器，实时结果显示与错误（见 §3）。
- 保存按钮在任一错误时禁用。
- 弹窗**头尾固定、中间滚动**（`Dialog` 传 `maxHeight`，内容 `max-h` 再小一档 + 内部
  `overflow-y-auto`，页脚 `shrink-0`）：预览把内容撑长之后，「保存」仍然留在视口里。
  宽度固定 560，不随预览出现而变宽。
- **预览**：`预览` 按钮调用 `previewVideoHighlight`，用与保存完全相同的 ffmpeg 指令渲染一次
  （同一帧率、同一条编码指令），结果用 `<video>` 播放（`LoopingClip`），下方标出
  「时间段 1:30–1:35 · 60 fps · 1.6 MB」/「时刻 1:30 · 340 KB」。
  预览是**同步**调用（本地 1–2 秒、最坏十几秒），期间按钮显示「生成预览中…」，
  此时保存禁用，避免保存一张还没确认的图。
- 改时间、清空结束时间都会作废并删除旧预览（`discardVideoHighlightPreview`），
  一张预览最多对应一个请求；预览回来时如果用户已经改了时间，结果直接丢弃。
- 保存：
  - 有与当前输入匹配的预览 → `addVideoHighlightFromPreview`，文件移入 `highlights/`、
    行直接是 `ready`，不再编码第二遍；
  - 采纳失败（预览已被清理）→ 退回 `addVideoHighlight` 走后台队列，保存不因此失败；
  - 没有预览 → `addVideoHighlight`（原有路径）。
- 成功后 `invalidate()` 并立即关闭；关闭（取消 / Esc / 遮罩）会丢弃未保存的预览。
- API 层错误（DB、媒体路径、生成失败）显示在弹窗内，保留输入。
- 源不可用时预览与保存都禁用并说明原因。

### 8.6 桌面详情

`components/HighlightMomentsSection.tsx`，挂载在 `VideoDetailPanel` 顶部封面/元数据之后：

- 网格展示全部状态：
  - `ready`：时间段用 `<LoopingClip src={highlightURL(h)}>`（`<video autoplay muted loop playsinline>`，
    只在该卡片可见时播放），时刻用 `<img src={highlightURL(h)} loading="lazy">`；点击打开大图 Dialog。
  - `queued`：「排队中」+ Spinner。
  - `running`：「正在生成视频…」或「正在截取…」+ Spinner。
  - `failed`：「生成失败」+ 错误文案 + 「重试」「删除」。
- 每条提供删除，经 `ConfirmDialog` 确认；排队/生成中删除即取消。
- 空状态「还没有精彩时刻」；读取失败显示 Banner + 重试。
- 新增按钮在 `status === "missing"`、`volumeOffline`、`sourceOffline` 时禁用。

### 8.7 移动详情

`mobile/components/HighlightMoments.tsx`：

- `useVideoHighlights(videoId, { pollWhilePending: true })`。
- 只渲染 `ready` 与 `queued`/`running`；`failed` 过滤掉。
- `ready` 两列网格 + 点击全屏；pending 占位 + Spinner。
- 无可见条目时整段不渲染。
- 不提供新增、重试、删除入口。

---

## 9. 资源生命周期

### 9.1 文件布局

```text
<dataDir>/highlights/h-<videoID>-<unixNano>-<seq>.jpg            # 时刻
<dataDir>/highlights/h-<videoID>-<unixNano>-<seq>.mp4            # 时间段
<dataDir>/highlights/h-<videoID>-<unixNano>-<seq>.gif            # MP4 之前生成的时间段，只读兼容
<dataDir>/preview/p-<videoID>-<startMs>-<endMs>-<unixNano>-<seq>.<jpg|mp4>
```

- 归属由 `video_highlights.media_path` 声明，而不是文件名里的 id。
- 路径在插入 queued 行时写入，生成中的文件不会被清扫误删。
- 同一路径重试时覆盖，不再新增行；`.gif` 旧条目被重试或恢复时，会在生成前换成新的
  `.mp4` 文件名（`SetHighlightMediaPath`），避免 MP4 字节顶着 GIF 扩展名被服务成错误的类型。
- `preview/` 里的文件**不属于任何行**：它只在弹窗打开期间有意义。文件名里带着请求
  （`videoID`/`startMs`/`endMs`），所以"采纳"这一步可以校验"这张图是不是为这组时间生成的"，
  而不必相信前端。
- 采纳预览时文件被 `os.Rename` 到 `highlights/` 并换成正式命名，行直接写 `ready`——
  同一段区间不会编码两遍。

### 9.2 孤儿清扫

扩展 `video.RemoveOrphanedImages`：

1. 读取 `ListHighlightMediaPaths()`，建立 basename 集合。
2. 扫描 `highlights/`，只处理 `^h-\d+-\d+-\d+\.(jpg|gif|mp4)$`。
3. 保留集合内文件；删除孤儿；其他命名与目录一律保留。
4. `Scanned / Removed / Bytes` 计入总结果。

`preview/` **不参与清扫**：那里的文件天生没有行，清扫会误删一个正开着的弹窗的预览。
它由两条规则管住——弹窗关闭/改时间时显式删除，应用启动时清空上一次进程留下的所有预览
（`purgeStalePreviews`，只删符合预览命名的文件）。

### 9.3 删除与回收站

- 永久删除影片：事务级联删行；随后清扫删除文件。
- 删除单条精彩时刻：删行 + 删文件。
- 回收站：保留行与文件；恢复后照常展示。
- 失败/取消：worker 删除半成品；清扫兜底。

### 9.4 回滚

- v29 表对旧版本代码无害；旧版本不会清扫 `highlights/`，文件会保留到升级。
- 不迁移、不删除旧数据。

---

## 10. 测试计划

### 后端

- 迁移：v28 → v29 建表/索引/约束；重复迁移幂等。
- 仓库：增删查、Claim/Finish/Retry、Recover（running 中断计数、3 次失败）、级联删除、媒体路径列表。
- 服务：
  - 新增校验（负数、结束 ≤ 开始、< 1 秒、> 20 秒、超时长）；
  - 插入 queued、入队、立即返回；
  - worker 成功 → ready，文件存在，事件与任务计数；
  - worker 失败 → failed + error，半成品清理；
  - 重试 failed → queued → 再次执行；
  - 删除 queued/running/ready/failed 的文件与计数；
  - worker 发现行被删时丢弃结果；
  - 退出取消保持 running，重启恢复；
  - 时间段只编码一次（12 MB 的文件也不触发第二次尝试），帧率固定为 60，落盘扩展名是 `.mp4`；
  - 任务状态：运行/完成/失败/取消计数与文案。
- 预览：
  - 渲染不产生行、不进队列、不动任务计数；
  - 预览带回了请求与帧率，失败时不留半成品；
  - 采纳预览：文件移入 `highlights/`、行直接 `ready`、没有第二次编码；
  - 时间不匹配 / 文件不存在 / 路径穿越一律拒绝且不写行；
  - 丢弃只删预览目录里的文件；启动清空旧预览；孤儿清扫不碰 `preview/`。
- 清扫：queued/running 文件保留、孤儿删除、非应用命名保留。
- `thumbnail` 集成测试：真实 ffmpeg 生成 JPEG 与 MP4，断言 JPEG 魔数、MP4 的 `ftyp`、
  h264 / 60 fps / 帧数 / 时长，以及「没有音轨」。
- 服务集成测试（`highlights_integration_test.go`，`-tags integration`）：真实 ffmpeg 经
  `video.Service` 生成 2 秒区间，断言 `.mp4` 魔数、h264、60 fps 与帧数——常量对了不等于
  ffmpeg 写对了。
- API/服务：`BackgroundTasks` 包含「精彩时刻生成」行。
- HTTP：列表返回状态、媒体文件可访问、非法文件名 404。

### 前端

- `utils/highlightTime.test.ts`：
  - 时/分/秒转毫秒；
  - 结束 ≤ 开始、> 20 秒、超时长、分/秒越界；常用时长的可用性（超出视频结尾或超过上限时置灰）；
  - 标签格式；
  - 预览与请求的匹配（改开始 / 改结束 / 时刻）；
  - 常用时长按钮的可用性（超出视频结尾或上限时置灰）；
  - 毫秒 → 时/分/秒的拆解（含与 `timePartsToMs` 的互逆）；
  - 时间段 / 时刻的判定（决定用 `<video>` 还是 `<img>`）；
  - pending/ready 判断；
  - 移动端过滤 failed。
- `npm run lint` 校验 manifest 与跨平台目录规则。
- `npm test`、`npm run build:all`；重建 `backend/server/webui/`。
- 绑定生成后检查 `App.{js,d.ts}` 与 `models.ts` 包含 `Highlight`、status/error 字段与 retry 方法。

---

## 11. 实施清单

- [x] 数据库迁移 v31、`Highlight` 模型、仓库方法与迁移/仓库测试。
- [x] `backend/thumbnail` 截图/循环视频生成器与集成测试。
- [x] `video.Service` 校验、队列、worker、恢复、任务状态与服务测试。
- [x] `api.Library` 门面、`BackgroundTasks` 投影、`app.go`、`main.go` 启停与资源路由。
- [x] HTTP 只读端点/媒体服务与服务器测试。
- [x] 重新生成 Wails 绑定。
- [x] `utils/highlightTime`、共享 hook、桌面弹窗/详情、移动端、事件与 manifest。
- [x] 文档更新（本文、`generated-images.md`、`background-tasks.md`、`mobile-web-design.md`、`DEVELOPMENT.md`、`README.md`）。
- [x] 重建 `backend/server/webui/`。
- [x] 全量验证：

```bash
gofmt -l .
go vet ./...
golangci-lint run ./...
go test ./...
go test -tags integration ./backend/thumbnail ./backend/video ./backend/server

cd frontend
npm run lint
npm test
npm run build:all
```

---

## 12. 风险与非目标

- **不做通用任务调度器。** 任务看板仍是只读投影；精彩时刻的执行器归 `video.Service`，状态落在自己的表里。
- **移动端不提供写操作。** 新增、重试、删除只在桌面端；失败条目在移动端隐藏。
- **不自动重试明确失败。** 源损坏、超时等确定性错误交给用户手动重试；只有崩溃中断自动重排。
- **没有体积回退。** 20 秒、960 宽、60 fps 的 MP4 约 1.6 MB，降帧降质换不回有意义的东西；
  第一版 GIF 的 8 MB 软上限随格式一起删掉了。
- **不做多分辨率/多码率。** 960 宽是固定档（并且从不放大源）：想更清晰只能整体上调这个常量。
- **不支持编辑。** 修改时间只能删除后重新添加。
- **不设数量上限。** 20 秒上限与用户逐条输入控制体积。
- **回滚不清理新目录。** 旧版本不会删除 `highlights/`，升级后继续使用。

---

## 13. 实现差异与落地细节

实现时在保持行为不变的前提下做了几个设计上没有写死的决定，记录在这里，便于以后改动时
判断哪些是"规则"、哪些只是"当时的写法"。

### 13.1 表版本顺延为 v31

见 §4.1 的说明：v29 / v30 在实现前已被占用（马赛克破解标记、删除分组表）。SQL 不变。

### 13.2 任务计数的 Total 由 pending 集合推导

§6.1 的不变式（`Done + Failed + pending == Total`）没有靠"每处记得同步加一"来维持，而是让
`video.Service` 保存**本轮接受的 id 集合**，`Total = Done + Failed + len(pending)` 现算：

- 排队是**幂等**的：启动恢复不会把本进程已经入队的 id 再算一次（这是实现中真实踩到的
  双重计数，见 `TestBackgroundTasksProjectsHighlightRound`）；
- 取消按 id 结算，删掉一条不在本轮的旧行不会让 `Done` 虚增；
- 结果迟到（worker 写完时行已被删）时 `resolve` 找不到 id，计数自然不动。

### 13.3 永久删除时先读后删

`video.Service.Remove` 在删记录**之前**读一遍该影片处于 `queued`/`running` 的条目 id，级联删除后
按 id 记取消。否则"删影片"会留下永远 pending 的计数，看板会一直显示在跑。生成的文件不在这里
删除：`media_path` 随行消失，孤儿清扫会兜底。

### 13.4 队列类型在 `video` 内单独一份

`workQueue[T]` 与 `backend/jav/job_queue.go` 的版本实现相同。跨包复用需要把它导出，代价大于
四十行重复代码；两处注释互相指路。

### 13.5 可替换的接缝

`video.Service` 的 `highlightState` 持有两个 ffmpeg 生成函数和一个 `fileSize`，默认分别接到
`backend/thumbnail` 与 `os.Stat`。测试据此描述"成功 / 失败 / 卡住直到关闭"，不必真的跑 ffmpeg，
也不必写几 MB 的文件。

### 13.6 生成目录按需创建

`highlights/` 在 `AddHighlight` 时确保存在，而不是只在 `main.go` 启动时创建：数据目录被单独恢复
（只带回 `app.db`）时，行仍然能写入文件。

### 13.7 事件

- `video:highlight:updated`：单条状态变化（新增、就绪、失败、删除都发），payload 是
  `{id, videoId, status}`；删除时 `status` 为 `deleted`。
- `tasks:updated`：看板计数变化，1 秒尾触发节流，与片名翻译共用同一语义。

两者都在 `frontend/src/services/events.ts` 的失效列表里。

### 13.8 局域网媒体路由与桌面资源路由共用一条命名规则

`video.ValidHighlightFileName` 是唯一实现，桌面 `AssetServer` 的 `/highlights/` 与局域网
`GET /highlights/{name}` 都调用它，清扫用的也是同一个正则——三处各写一遍迟早会漂移。

### 13.9 帧率：10 → 25 → 50 fps（2026-11-25，**已被 13.11 取代**）

这一节记录的是 GIF 时代的历史：先 10 fps，因运动画面"一顿一顿"提到 25，再按用户要求提到
**50 fps——GIF 的物理上限**（每帧正好 2 cs，浏览器认可的最短延迟）。60 fps 在 GIF 里写不出来，
原因见下表，这也是后来换 MP4 的直接动机。

**为什么不能 60 fps**（实测同一段 5 秒素材，`fps=60`）：

| 请求 | ffmpeg 实际写入的每帧延迟 | 浏览器里的表现 |
| --- | --- | --- |
| 33 fps | 3 cs / 4 cs 交替 | ≈30 fps，轻微抖动 |
| 50 fps | 2 cs | 50 fps ✅ |
| 60 fps | **1 cs / 2 cs 交替** | 1 cs 的帧被钳制成 10 cs，每隔一帧冻结 0.1 s ❌ |

Chromium 与 Firefox 都有一条硬规则：延迟小于 2 cs 的帧按 10 cs 播放。所以 60 fps 的 GIF 比
25 fps 更卡，这是容器的限制，不是参数没调好。真需要 60 fps 只能换容器（静音循环 MP4/WebM），
代价是两端播放元素、媒体扩展白名单与 iOS 低电量模式的自动播放兜底。

体积与耗时（10 秒区间实测）：普通画面 GIF 50 fps 1.73 MB / 1.6 s；满屏颗粒最坏情况 21.9 MB /
13 s（超过 8 MB，会回退到 25 fps 再生成一次）。回退值也从 8 fps 一路提到 25，不再出现"幻灯片"。
如果要再往上，GIF 的下一档就是 50 封顶；超过它只能换格式。

### 13.10 保存前预览（2026-11-25 追加）

用户提出"想先看动图效果再决定是否保存"。实现方式是把**正式生成的那条 ffmpeg 指令**提前跑
一次，产物放进 `preview/`，而不是另写一套"低质量预览"——预览低质量就失去了确认的意义。

三个决定值得记下来：

1. **预览不是记录。** 没有行、没有状态、不进队列、不进任务看板。因此它也不会被"取消生成"
   之类的语义纠缠：关掉弹窗就删文件。
2. **保存采纳预览，而不是重新生成。** `AddHighlightFromPreview` 校验文件名里的
   `videoID/startMs/endMs` 与请求一致后 `os.Rename` 到 `highlights/`，直接写一条 `ready`
   行。所以"确认的那张图"与"存下来的那张图"是同一个文件，也不会把同一段区间编码两遍。
   校验失败（预览已被清理）时前端退回普通 `addVideoHighlight`，保存不会因此失败。
3. **预览是同步调用。** 普通内容 1–3 秒，最坏的 20 秒区间约一两秒（h264 比 GIF 快得多）；期间弹窗显示
   「生成预览中…」并禁用保存，避免存下一张还没确认的图。同一时刻只允许一个预览在跑
   （`previewMu`），两个弹窗同时预览只会让双方都更慢。

文件名的形状（`p-<videoID>-<startMs>-<endMs>-<nano>-<seq>.<ext>`）同时解决了两个问题：
服务端能验证"这张图是为这组时间生成的"，而不用相信前端；桌面资源路由也能用一条正则
只放行预览文件。

`preview/` 因此不能交给孤儿清扫（那里的文件天生没有行）。它的生命周期是：关闭弹窗/改时间
即时删除 → 应用启动清空上一个进程的残留。`TestImageSweepLeavesPreviewsAlone` 钉住前者与
清扫的边界。

### 13.11 换成 MP4、上限提到 20 秒、加常用时长（2026-11-26）

用户在看过 GIF 之后问「gif 反而比 mp4 要大吗？」，随后要求换成 MP4、最长允许 20 秒，并能在
填好开始时间后用 5 / 10 / 15 / 20 秒一键设置结束时间。三处一起改：

1. **容器**：时间段从 GIF 改为 MP4（H.264，yuv420p，静音，60 fps，960 宽，CRF 22，`+faststart`）。
   60 fps 现在是真的 60 fps；体积同内容小 20 倍上下（10 秒实测：GIF 50 fps 1.73 MB → MP4 60 fps
   77 KB～750 KB，取决于画面）。`backend/thumbnail` 的 `GenerateHighlightClip` 取代了
   `GenerateHighlightGIF`，滤镜链、调色板与 `-gifflags` 一并删除。
2. **上限 10 → 20 秒**：`MaxHighlightRangeMs = 20000`，前后端与文案同步。20 秒 60 fps 实测
   750 KB、编码 0.9 秒，所以 **8 MB 软上限与降帧重试被删掉**——一个只为 GIF 的体积问题而存在的
   机制。`generateHighlight` 现在只编码一次，并把实际帧率返回给预览显示。
3. **常用时长**：`HIGHLIGHT_DURATION_PRESETS = [5,10,15,20]`，点一下把结束时间设成
   「开始 + N 秒」，写入同样的三个输入框，走同样的校验。会超出视频结尾或上限的按钮置灰
   （`canUseHighlightPreset`），**不夹取**——悄悄把 +20 秒改成 +7 秒等于保存了用户没要的时长。

播放端随之从 `<img>` 换成 `ui/clip.tsx` 的 `LoopingClip`（`<video muted loop playsInline>`）：
静音是自动播放的前提，`playsInline` 防止 iOS 抢全屏，自动播放被拒时盖一个播放按钮，滚出视口
就暂停（网格里同时播放十几个 60 fps 片段会白烧 CPU）。时刻仍是 `<img>`——静止图用视频没有意义。

**兼容**：MP4 之前生成的 `.gif` 条目照常显示，`ValidHighlightFileName` 与清扫正则都保留 `gif`；
被重试/恢复的旧条目在生成前用 `SetHighlightMediaPath` 换成新的 `.mp4` 名字，否则 MP4 字节会
顶着 `.gif` 扩展名被服务成错误的 Content-Type。

### 13.12 画质与弹窗高度（2026-11-26 同日修订）

换成 MP4 后第一版沿用了 GIF 时代的 **480 宽 / CRF 26**，两点反馈随之而来，都修了。

**画面太模糊。** 480 宽的片段在弹窗（约 520 px）和大图查看器（约 1000 px）里都被**放大**显示，
放大才是不清楚的主因；CRF 26 又把剩下的细节抹了一层。改为 **960 宽 + CRF 22**，并且用
`scale='min(960,iw)'` **不放大源**：1080p 的片子缩到 960 更锐利，640 宽的片子保持原样而不是被
吹成更模糊、更大的文件。实测 1080p → 960x540、20 秒：CRF 26 1.22 MB / CRF 24 1.61 MB /
CRF 22 2.32 MB，编码 2.5 秒；原来 480/CRF 26 是 377 KB。体积翻了几倍但仍在 MB 级，
没有任何上限会被触发。`TestGenerateHighlightClipIntegration` 里钉了"320 宽源产出 320 宽"，
防止有人把 `min()` 去掉又变成放大。

**保存按钮被顶出视口。** 预览出现后弹窗内容变长（预览视频 + 元信息 + 可能的错误条），而弹窗
既不限高也不滚动，页脚就跟着内容一起溢出，「保存」再也点不到。根因还有一层：Astryx 的 `Dialog`
默认把**弹窗盒子本身**限死在 `75dvh` 并裁掉溢出部分，所以只给内容加滚动仍然会撞上这个上限。
改为**头尾固定、中间滚动**：

- `ui/dialog.tsx` 新增并转发 `maxHeight`（以前只能吃库的 75dvh 默认值）；
- 弹窗 `maxHeight="calc(100vh - 40px)"`，`DialogContent` 的 `max-h` 再小 100 px，
  保证头尾都在盒子内；
- 中间那段 `flex-1 overflow-y-auto`，`DialogFooter` 加 `shrink-0` 留在底部；
- 预览媒体本身也限高（`clamp(150px,30vh,280px)`），不再按 16:9 铺满整宽；
- 宽度不再随预览出现而从 520 跳到 720——弹窗几何变化本身就是一次干扰。
