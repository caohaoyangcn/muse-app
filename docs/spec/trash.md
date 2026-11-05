# Muse 收藏回收站设计

**Status:** Implemented (v1; v26 增加网盘归档)
**Scope:** 收藏的软删除、恢复、保留期与自动清理，以及网盘来源停用后的长期归档；只涉及数据库记录与 Muse 生成的图片，**绝不触碰源视频或网盘文件，也不保存视频内容**
**Target:** 桌面端（Wails）、局域网移动端（共用 `api.Library` 门面）、LAN HTTP API
**Version:** v1（schema v25；v26 增加网盘归档字段）
**依赖:** 无新增依赖

---

## 0. 问题

在回收站之前，「移除收藏」直接删除 `videos` 行，并清空分组归属与刮削资料。这个动作对用户是不可逆的：

- 误点移除后，标题、备注、点赞、分组和刮削结果一起消失；
- 文件只是被移动时，自动缺失清理也会删除记录，之后重新添加只能得到一条新收藏；
- 文件暂时不可达（网络共享抖动）时，直接删除的代价过高。

缺失自动清理本身是对的——库不应该长期显示一个已经不存在的东西——但它需要一个**可撤销的落点**。

## 1. 目标与非目标

v1 目标：

- 手动「移入回收站」是软删除：记录、缩略图/封面、分组归属、点赞、刮削资料全部保留，可恢复；
- 文件缺失的自动清理也先进入回收站，并标记原因，不再直接销毁记录；
- 回收站保留期默认 14 天，可配置为 1–365 天；
- 超期条目在应用启动、运行中每小时、以及修改保留天数后自动彻底清理；
- 桌面端与移动端都能查看、恢复、彻底删除；
- 回收站显示条目原分组、移入原因、移入时间与剩余天数。

v1 非目标：

- 不做文件系统回收站：不移动、不复制、不重命名任何源视频；
- 不在回收站里保存视频副本；磁盘成本只有数据库行与已生成的图片；
- 不做按条目自定义保留期（保留期是全局策略）；
- 不做回收站搜索/筛选（条目量小，先保持简单）；
- 不做「文件重新出现时自动恢复回收站记录」：文件回到原路径后，重新添加会恢复；移走后的文件通过内容指纹重连恢复。

## 2. 数据模型

`videos` 表新增两列（schema v25）；v26 另加 `archive_source_name`，`cloud_connections` 加 `enabled` / `disabled_at`：

| 列 | 含义 |
| --- | --- |
| `deleted_at INTEGER NULL` | 移入回收站的 unix 毫秒；`NULL` = 活动收藏 |
| `delete_reason TEXT NOT NULL DEFAULT ''` | `user`（手动移除）、`missing`（文件缺失自动移入）或 `archived`（网盘来源停用归档） |
| `archive_source_name TEXT NOT NULL DEFAULT ''` | 停用来源时的名称快照；连接删除后归档仍可显示来源 |

新增索引 `idx_videos_deleted_at`。

**为什么保留原行，而不是复制到回收站表：**

- 缩略图/封面文件名由收藏 id 决定（`thumbnails/<id>.jpg`、`covers/jav/<id>.jpg`），保留 id 才不会丢失图片；
- 分组归属、`video_jav`、`jav_scrape_tasks` 都以 `video_id` 关联，保留原行让恢复变成一次 `UPDATE`；
- `videos.path` 的唯一约束由回收站继续持有，因此再次添加同一路径可以恢复原收藏，而不是产生重复或冲突。

## 3. 读模型边界

所有**活动读**一律过滤 `deleted_at IS NULL`：

- 收藏列表、搜索、单条读取；
- 元数据/指纹回填候选、同尺寸 peer、内容查重读路径；
- 分组视频列表与成员计数（递归计数同样过滤）；
- 刮削任务看板、番号查重候选、演员聚合、按演员取片；
- 文件检测与整卷对账（回收站条目不反复探测）。

有意不过滤：

- `GetByPath` / `GetByRemote`：用于识别「同一路径/同一网盘文件重新添加」，需要看到回收站行并恢复它；
- `ListVideoIDs`：图片清理需要知道回收站仍拥有哪些图片，否则会把封面当孤儿删除；
- `ListRelocationCandidates`：服务层需要看到因缺失而移入回收站的记录，才能把移动后的文件接回原收藏。

## 4. 生命周期

### 4.1 移入

手动移除与文件缺失自动清理都调用 `video.Service.Trash(id, reason)`：

1. `GetAny` 读取（不存在 = 成功：过期卡片也能消失）；
2. 已在回收站 = 幂等，不重置 `deleted_at`；
3. 活动行写入 `deleted_at` / `delete_reason`，发 `video:trashed {id, reason}`；
4. 源视频、缩略图、封面、分组、点赞、刮削资料都不动；Muse 不保存视频内容。

### 4.1.1 网盘归档（v26）

停用网盘来源时，门面把该来源所有活动收藏在同一用户动作里转成回收站条目：

```text
deleted_at = now
delete_reason = 'archived'
archive_source_name = 连接名快照
```

- 归档条目长期保留，**不参与**普通回收站的保留期自动清理；
- 「归档与回收站」入口分为「归档 / 回收站」两个 tab；
- 重新启用连接不会自动恢复归档；
- 归档可以单条恢复（来源必须已启用）、单条彻底删除、按来源清空或「清空归档」；
- 普通「清空回收站」不包含归档；
- 连接删除后，归档靠 `archive_source_name` 继续显示来源。

### 4.2 恢复

`video.Service.Restore(id)`：

- 清空 `deleted_at` / `delete_reason`，保留 id 与全部关系；
- 本地收藏在卷在线时 stat 一次：文件存在 → `available`，缺失/不可读 → `missing`；卷离线则保持原状态；
- 网盘收藏不主动联网探测，保持原状态，在线时重新排队处理；
- 文件可用时按需重新排队缩略图/元数据与内容指纹；
- 发 `video:restored {id}`。

### 4.3 彻底删除

只有以下三种情况会删除行：

- 用户在回收站点「彻底删除」；
- 用户点「清空回收站」；
- 保留期到期，后台清理。

统一走 `api.Library.DeleteVideoPermanently`：

1. `video.Service.Remove` 删除 `videos` 行并删除缩略图；
2. 删除 `video_group_members` 中的归属；
3. `jav.Delete` 删除 `video_jav` / `jav_scrape_tasks`；
4. 发 `video:removed {id}`；图片清理把残留封面按「无记录」回收。

`videos` 行删除后，路径唯一键释放，用户可以重新添加同一个文件。

## 5. 保留期与自动清理

- 默认：`api.DefaultTrashRetentionDays = 14`；
- 范围：`1–365` 天；非法/损坏的设置回退默认值；
- 过期判据：`deleted_at <= now - retentionDays * 24h`，且 `delete_reason <> 'archived'`；
- 触发：
  - 应用启动后立即一次；
  - 运行中每小时一次（`api.Library.StartTrashPurge`）；
  - 保存新的保留天数后立即一次（缩短保留期会马上清理已超期条目）。

保留期按 `deleted_at` 动态计算，因此调整设置会同时影响已经在回收站里的条目。

## 6. 重新添加与文件移动

- **同一路径重新添加**：`GetByPath` 看到回收站行 → 恢复原收藏（同 id、同标题/备注/点赞/分组/刮削），不新建。
- **网盘同一路径重新添加**：`GetByRemote` 同样恢复。
- **文件被移动**：如果原收藏是因文件缺失进入回收站（`delete_reason = missing`），新位置的同内容文件会通过内容指纹重连到它；`RelinkPath` 同时清空回收站标记。用户手动移入回收站且文件仍在原地的条目**不会**被自动重连，避免悄悄撤销用户的决定。

## 7. 事件

| 事件 | 载荷 | 说明 |
| --- | --- | --- |
| `video:trashed` | `{id, reason}` | 收藏进入回收站；活动视图移除卡片 |
| `cloud:source:archived` | `{sourceId, name, count}` | 网盘来源停用并批量归档 |
| `video:restored` | `{id}` | 收藏从回收站恢复 |
| `video:removed` | `{id}` | 记录被彻底删除（保留原有语义） |
| `video:missing:swept` | `{root, removed, skipped}` | `removed` 现表示移入回收站的条数 |
| `video:filescan:done` | `FileScanStatus` | `removed` 同样按「移入回收站」理解 |

桌面端在 `frontend/src/services/events.ts` 统一订阅并 `invalidate()`；移动端下一次读取自然一致。

## 8. API

### Wails（`app.go`）

```go
ListTrash() (api.TrashSnapshot, error)
RestoreVideo(id int64) error
DeleteVideoPermanently(id int64) error
EmptyTrash() (int, error)
PurgeExpiredTrash() (int, error)
GetTrashRetentionDays() int
SetTrashRetentionDays(days int) error
```

`RemoveVideo` 保留原名字，但语义改为移入回收站。

### LAN HTTP

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/trash` | 回收站快照：`{videos, retentionDays}` |
| POST | `/api/trash/{id}/restore` | 恢复 |
| DELETE | `/api/trash/{id}` | 彻底删除 |
| DELETE | `/api/trash` | 清空，返回 `{count}` |
| POST | `/api/maintenance/trash/purge` | 清理超期，返回 `{count}` |

`DELETE /api/videos/{id}` 保持存在，但变更为软删除。

## 9. UI

- 桌面侧边栏「归档与回收站」，视图分为「归档 / 回收站」两个 tab；
- 回收站显示封面、名称、原因、移入时间、剩余天数、原分组与路径；每条提供「恢复」「彻底删除」，支持「清空回收站」；
- 归档按来源分组，显示归档时间、长期保留和来源名；提供「恢复」「彻底删除」「清空归档」；
- 设置 → 回收站可调整保留天数、恢复默认、立即清理超期条目；
- 移动端新增「回收站」底部 tab，操作相同（不提供保留天数设置，遵守移动端不做设置的约束）；
- 移除相关文案统一为「移入回收站」，并保留「源文件不会被删除」的提示。

## 10. 已知边界

- **回收站不是备份**：只保留 Muse 的元数据与生成图片，不保留视频内容；源文件被外部删除后无法从回收站找回视频。
- **不自动恢复回收站里的缺失项**：文件重新出现在原路径后，需要重新添加（会自动恢复）或手动恢复；这样用户主动移入回收站的条目不会因为磁盘事件被悄悄复活。
- **保留期不是精确定时器**：应用关闭期间不会清理，下一次启动或后台 tick 才执行；到期条目在两次 tick 之间可能短暂仍可见。
- **自动清理是启发式保护**：整卷全部缺失时仍然跳过（复用缺失自动移除的整卷保护），所以部分「文件真的没了」的收藏会停留在列表里标 ⚠，不会被自动移入回收站。
- **网盘归档**：归档只是记录状态，不是备份；网盘源文件仍由 OpenList 保存，Muse 不保存视频内容。
- **无鉴权 LAN**：与现有写端点一致，同网段设备可以恢复或彻底删除回收站条目；README 的安全提示不变。
