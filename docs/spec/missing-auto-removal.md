# Muse 缺失收藏自动清理设计

**Status:** Implemented (v2: 自动清理改为移入回收站)
**Scope:** 卷已接入且文件确认不存在时，自动把这条收藏移入回收站（记录、缩略图、分组归属、刮削数据都保留），**绝不触碰源视频**
**Target:** 桌面端（Wails）与局域网移动端（共用 `api.Library` 门面；移动端下一次读取即一致）
**Version:** v2
**依赖:** 无新增依赖；判据是 `os.Stat` 的 `ErrNotExist` + 卷在线 + 可识别卷根

---

## 0. 问题

收藏是引用，不是文件（`spec-v1.md` §41）。引用失效时，应用原先只把 `status` 标成 `missing`，卡片仍留在列表里，等用户手动移除。

实际使用中更常见的路径是：

1. 外接盘拔掉时删除了若干文件；
2. 盘插回来后，`volume-offline` 逻辑只把“已经是 `missing`”的行改回 `available`，**不会重新检查磁盘离线期间消失的文件**；
3. 于是这些卡片一直显示为可用，打开时才报“找不到文件”。

用户希望：**硬盘接入时发现文件缺失，就自动把影片从列表清理掉**，不需要逐个手动移除。

## 1. 目标与非目标

v2 目标：

- 启动时、硬盘从离线恢复为在线时，对每块在线卷做一次整卷对账；
- 文件确认不存在 → 自动把收藏移入回收站（软删除），保留数据库记录、缩略图、分组归属、`video_jav` / `jav_scrape_tasks` 与刮削封面；
- 回收站保留期默认 14 天，超期自动彻底清理；详见 `docs/spec/trash.md`；
- 文件只是不可读（权限 / IO）→ 不删，标记 `missing` 保持可见；
- 卷未接入 → 不删、不改状态；
- 能识别“整块盘都不对劲”的情况并拒绝批量删除；
- 移入回收站后桌面端收到汇总提示，明确“可在回收站恢复，源文件未被删除”。

v2 非目标：

- 不做运行中的逐文件监控：没有卷插拔事件时，运行期间删除的文件仍由“打开失败 / `RefreshStatus`”的原有对话框处理，或等下一次启动；
- 不做卷序列号 / 磁盘指纹，盘符复用只靠“整卷全部缺失”保护；
- 不做 macOS `/Volumes/<name>` 挂载点识别；
- 不提供开关：这是用户确认的默认行为。

## 2. 判定条件

只有同时满足以下条件才会移入回收站：

| 条件 | 实现 |
| --- | --- |
| 卷在线 | `volumeState.offline(root) == false`；调用方只在 `Changed && Online` 或启动探测后进入 |
| 卷可识别 | `filepath.VolumeName(path) != ""`：Windows 盘符 / UNC 为真；macOS/Linux 的 `/` 为假，因此自动清理在 Unix 不启用 |
| 文件确认不存在 | `errors.Is(statErr, os.ErrNotExist)`；权限、IO 等错误不算缺失 |
| 卷可信 | 该卷上至少一条收藏仍能 stat 成功，或该卷只有这一条收藏 |

## 3. 触发时机

| 时机 | 行为 |
| --- | --- |
| 启动 `RefreshAllStatuses` | 登记卷根并 `probe()` 一次，再对每个在线卷调用 `reconcileVolume(root, true)`；离线卷跳过 |
| 手动「检测文件」（`ScanFileStatuses` / `POST /api/maintenance/files/scan`） | 与启动完全相同的一次探测 + 整卷对账，后台执行并报告进度与结果（见 §4.2） |
| 后台 `WatchVolumes` / 前端首选 `RefreshVolumes` | 仅当卷从离线翻转为在线（`changed && online`）时调用 `reconcileVolume(root, false)` |
| 打开番号查重页 `ListJavDuplicates` | 对页面上列出的版本调用 `ReconcileMissing(ids)`：确认缺失的自动移除，然后重建分组，页面不再展示已经消失的版本 |
| 单文件 `Open` / `OpenContainingFolder` / `RefreshStatus` | 保持原行为：标 `missing` 并提示，不自动删除 |

`announce` 决定状态变化是否逐行发 `video:status:changed`：启动时需要（首屏可能已经画出来），插回时不需要（`video:volume` 已经是这次刷新信号）。

## 4. 整卷对账

`backend/video/volume.go` 的 `reconcileVolume(root, announce)`：

1. `reconcileMu` 串行化，避免启动对账与轮询同时处理同一批记录；
2. 再次检查卷是否离线（探测与对账之间可能被拔出）；
3. 取出该卷上的所有收藏，逐条 `stat`：
   - 成功 → 若状态为 `missing`，改回 `available`；
   - `ErrNotExist` 且卷可识别 → 加入待移入回收站候选；
   - 其他错误 → 标记 `missing`，不删除；
4. 候选为空 → 结束；
5. **整卷保护**：`没有一条候选以外的收藏存在` 且该卷收藏数 `> 1` → 不移入回收站，全部标 `missing`，记录警告，发 `skipped` 汇总事件；
6. 否则逐条调用注入的软删除函数；失败则保留记录并标 `missing`，下次重试；
7. 有条目移入回收站时发 `video:missing:swept` 汇总事件。

软删除函数由门面构造时注入（`backend/api/library.go`）：

```go
videos.SetMissingRemover(func(id int64) error {
    return l.trashVideo(id, video.TrashReasonMissing)
})
```

因此自动清理与手动移除走同一条回收站编排（记录保留、原因标记），不会出现两份行为；桌面端、局域网服务与测试拿到的都是同一条路径。

### 4.1 单条读路径：`ReconcileMissing`

番号查重页在读取分组后，会把页面上所有版本 id 交给 `video.Service.ReconcileMissing`。它复用同一套判定与整卷保护，但只处理传入的 id：

- 卷根不可 stat → 视为未接入，保留记录（页面显示「磁盘未接入」）；
- 文件存在 → 若状态是 `missing`，改回 `available`；
- `ErrNotExist` 且卷可识别 → 候选；
- 其他错误 → 标 `missing`，不删；
- 该卷上还有任意一条非候选收藏存在 → 信任该卷并把候选移入回收站；否则整卷保护生效，全部标 `missing`；
- 返回被移入回收站的 id；`api.Library.ListJavDuplicates` 据此重建分组，避免继续展示已经不存在的版本。

内容查重页（`ListContentDuplicates`）不在这条路径上：它仍按原有设计标记缺失、在下一次读取时从工作清单里去掉该成员，记录本身由启动/插回对账清理。

### 4.2 手动全库检测（检测文件）

启动时的整卷对账现在也能按需触发：`video.Service.ScanFileStatuses` → `api.Library.ScanFileStatuses` → `App.ScanFileStatuses`（桌面）/ `POST /api/maintenance/files/scan`（局域网）。它复用 `RefreshAllStatuses` 的同一段编排，不另写一份判定——在线卷整卷对账、整卷保护、自动移除全部保持一致。

- 状态只放内存（`video.FileScanStatus`，与内容扫描一致，不建表）：`running`、`total`/`checked`（在线卷上的收藏数 / 已 stat 数）、`removed`/`skipped`/`revived`、`offline`（未接入卷上的收藏，不计入检测）、`error`、`startedAt`/`finishedAt`；`removed` 现在表示「已移入回收站」的数量（字段名保留以兼容既有客户端）；
- 每 16 个文件发一次 `video:filescan:progress`，结束时发一次 `video:filescan:done`；启动那次检查写入同一份状态，所以「上次检测」不需要再扫一遍；
- 正在跑时再次调用只返回当前状态（连点按钮不会扫两遍），与内容扫描的「加入进行中的扫描」同义；
- 桌面入口在收藏列表工具栏：按钮显示 `检测中 N/M`，结束后 toast 汇总「检查 N 个 / 清理 M 个 / 恢复 K 个 / 未接入磁盘 X 个」，仅检查无变化也提示一次（否则按钮看起来没反应）；
- 局域网侧只提供端点（维护类接口，与 `POST /api/maintenance/orphaned-images` 同级），移动端界面未加入口。

为什么不做「运行中持续检测」：那需要 fsnotify 双向对账或定期全库 stat（见 `TODO.md`「失效视频自动检测」）。手动入口解决的是「刚在资源管理器里删了文件，不想重启应用」这个最常见的场景，代价是用户需要点一下。

## 5. 事件

| 事件 | 载荷 | 说明 |
| --- | --- | --- |
| `video:trashed` | `{id, reason}` | 每条被自动清理的记录进入回收站；列表/卡片实时消失 |
| `video:removed` | `{id}` | 保留语义：仅彻底删除时发出 |
| `video:missing:swept` | `{root, removed, skipped}` | 每块卷一次汇总；`removed` 表示移入回收站条数，桌面端提示“已移入回收站 N 条 / N 条被保护未处理” |
| `video:filescan:progress` | `FileScanStatus` | 手动或启动检测的进度：计划总数、已 stat 数、未接入卷上的收藏数 |
| `video:filescan:done` | `FileScanStatus` | 检测结束；桌面端据此提示结果并刷新列表 |
| `video:volume` | `{root, online}` | 原有卷状态事件；插回时同时触发前端整体刷新 |

桌面端在 `frontend/src/services/events.ts` 统一订阅并 `invalidate()`；`Library.tsx` 只额外用 `video:missing:swept` 弹提示。移动端没有事件通道，下一次 `/api/videos` 读取自然一致。

## 6. 已知边界

- **移动重连代价已解决**：记录进入回收站而不是被删除，`content_hash` 的记忆仍在。文件被移动后重新添加会通过内容指纹接回原收藏，id、分组、点赞与刮削资料都不丢；详见 `docs/spec/trash.md`。
- **macOS / Unix**：`VolumeRoot` 一律返回 `/`，拔掉外接盘与删除文件无法区分，因此自动清理不启用；只在启动/插回时标记 `missing`。
- **整卷保护是启发式**：盘符被另一块盘复用且新盘上恰好有同路径文件时，仍可能误判；保护只覆盖“整卷收藏全部缺失”这一最常见情况。
- **网络共享**：共享根可访问但文件瞬时不可达时，`ErrNotExist` 可能误报；整卷保护只能在全部缺失时兜底。
- **运行中删除**：没有卷插拔事件，不会自动清理；下次启动或插回时才处理。
- **启动竞态**：启动时的汇总事件可能早于前端挂载而丢失；列表本身仍会正确（移入发生在读取之前，或通过 `video:trashed` 刷新）。
- **与 `PRODUCT.md` 原则 2 的关系**：清理后缺失项从活动列表消失，但会显性进入回收站并标明原因；这是比“直接删除”更可解释的落点。

## 7. 测试

`backend/video/volume_test.go`：

- 启动时在线卷上单条缺失 → 记录进入回收站；
- 同一卷一条存在、一条缺失 → 只把缺失的移入回收站；
- 权限错误 → 保留并标 `missing`；
- 整卷收藏全部缺失 → 全部保留、`skipped` 汇总；
- 不可识别卷 → 保留并标 `missing`；
- 盘拔掉期间删文件、插回 → 自动移入回收站；
- 文件回来 → 复活，不删除（既有测试）；
- 注入的软删除函数被调用；失败保留记录；汇总事件载荷正确；
- `ReconcileMissing`：只移入传入的确认缺失项；离线卷、权限错误、不可识别卷、整卷全部缺失均保留。

`backend/api/library_test.go`：

- `ListJavDuplicates` 在版本文件被删除后返回不含该版本的分组，并把其收藏记录移入回收站（Windows；卷根可识别才启用）。

`backend/video/filescan_test.go`（手动检测与状态）：

- 启动检测写入 `FileScanStatus`：`checked == total`、确认缺失的计入 `removed`、时间戳齐全；
- 未接入卷上的收藏计入 `offline`，不计入 `checked`，也不移除记录；
- 整卷全部缺失 → `skipped` 计数、`removed == 0`；
- `ScanFileStatuses` 立即返回、后台完成；运行中再次调用不新起一轮（`startedAt` 不变）；
- `video:filescan:progress` 的首个事件是计划（`total` 已知、`checked == 0`），结束时发 `video:filescan:done`；
- 读取收藏失败时 `error` 有值且状态仍然关闭（UI 不会永远转圈）。

`backend/server/server_integration_test.go`：

- `POST|GET /api/maintenance/files/scan` 启动并汇报检测结果。

既有 `service_test.go` / `relocate_test.go` / `dedup_test.go` 回归通过。
