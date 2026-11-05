# Muse 内容重复（部分哈希去重）设计

**Status:** Implemented (v1，含 T2 完整校验；物理盘调度仍为按盘符的近似)
**Scope:** 识别收藏中「同一份字节存了两处」的重复文件，给出分组让用户留一份或全留
**Target:** 桌面端（Wails）+ 局域网移动端（共用 `api.Library` 门面）
**Version:** v1
**依赖:** 无新增第三方库（哈希用标准库 `crypto/sha256`）

---

## 0.1 实现记录（与初版计划的差异）

计划与实现的差异，都是实现时做出的选择：

- `Fingerprint(path)` 一次返回 digest 与它读取时的 size/mtime（`FingerprintResult`），而不是接受调用方传入的 size：摘要和它所描述的事实必须来自同一个句柄，不可能互相矛盾。
- `hash_mtime` 存 **unix 纳秒**而不是秒：同一秒内的替换否则看起来没变。
- T0 剪枝用 `os.Stat` 的当前 size，并且**size 冲突的判定要跨全部行**，不只是「待哈希」的行——否则「已指纹的 A + 新加入的同 size B」这一对会被剪掉（实现细节见 `runContentScan`）。
- **2026-10-25 修订：T1 覆盖全部「没有当前指纹」的收藏，不再只算 size 冲突的文件。** 原因不是查重，而是移动重连：`file-relocation.md` 靠内容摘要认出「同一份字节换了路径」，没有摘要的收藏在改名后必然变成两张卡片。此前的 size 剪枝让「体积独一无二」的收藏永远没有指纹（旧库 1976 行里只有 56 行有指纹），扫描这个本该修复它的入口反而跳过了它们。代价是首次扫描要读全部文件的头尾（1976 个文件约 4 GB，机械盘分钟级），此后只补新增/变更的行。T0 仍逐文件 `stat`，负责发现不可读文件与判断指纹是否过期。
- `UpdateFingerprint` 顺带用同一份事实修正过期的 `file_size`，并清掉随之失效的 `full_hash`。
- 未挂载的盘：扫描 stat 失败时记录 `hash_error` 并清掉指纹（§3.2 的过期规则），面板以「N 个文件无法读取」呈现；因此可移动盘拔掉时不会留下不可校验的结论。
- 物理盘调度未实现：按卷（`filepath.VolumeName`）并行在该库上已等价（E/G/H/I 是四块不同盘），同一物理盘挂两个盘符仍是已知近似。
- **运行中进度按两个阶段各自计数**（首次真实运行后补上）：T0 只 stat、T1 才算指纹，而在「几乎每个文件体积都独一无二」的库里 T0 就是整轮扫描的全部工作。原计划只有 T1 的 `done/total`，于是界面在整个耗时阶段一个数字都不动。现在 `ContentScanStatus` 带 `phase`（`examining` / `comparing`）、`examined`/`examineTotal`（T0）与 `total`/`done`（T1），界面显示当前阶段的「已处理 / 本轮预计总数」，如 `正在检查文件大小… 412/899`，结束后汇总为 `上次扫描：检查 899 个 · 比对 4 个`。阶段切换点写入日志（`content scan examining finished`）。
- 实测数据（2026-10-04，该库）：899 个可用收藏里只有 4 个与其他文件同体积，895 个被 T0 剪枝；2 组内容重复共 20.04 GiB，可释放 10.02 GiB。这印证了上面的判断——只看 T1 计数的话，整轮扫描的进度条永远不会动。
- ignore 表的清理沿用 jav 的做法：只在 `IgnoreContentDuplicate` 发现成员少于 2 个时删除（没有额外的读路径清扫）。

---

## 0. 与现有查重的关系

`duplicate-detection.md` 用 **番号** 分组，覆盖「同一作品的不同版本（不同清晰度、不同压制组、带与不带字幕）」，其 §8 明确把「基于文件哈希的自动判定」列为非目标。两者是**不同维度，互相不能替代**：

| | 番号查重（已实现） | 内容查重（本计划） |
| --- | --- | --- |
| 判据 | `video_jav.code` + 集数标记 | 文件字节 |
| 抓出 | 同一作品的多份不同压制 | 同一份字节的多份拷贝（改名、换目录、跨盘备份） |
| 抓不到 | 没有可靠番号的文件 | 同一作品的不同压制/重封装 |
| 判定错误的方向 | 拆成幻影作品或漏提示 | **把两份不同视频判成同一份** |

最后一行决定了设计取向：内容查重的结论会被用户当作「可以删掉哪个文件」的依据，所以判定必须是**字节一致**，不做相似度或感知哈希，并且要能对结论做完整性确认。

本机实况（§7 有测量）：收藏 898 个可解析文件、6.31 TB，中位大小 5.39 GB，其中大量是 `*.restored.mp4`、`FC2-Collection`、`[BT-btt.com]` 这类番号缺失或重名严重的文件——番号查重覆盖不到的部分。

## 1. 目标与非目标

v1 目标：

- 不读整个文件（部分哈希）就能给出整库的「内容重复」分组；
- 面板列出分组、重复占用的空间，让用户留一份或全留；
- 首次全库扫描在分钟级完成，新增文件即时补算；
- 判定结论可被完整哈希确认（T2，见 §2.2）；结论已过期（§3.2）的成员，在重新校验完成前不允许据其删除。

v1 非目标：

- **不做相似度/感知哈希**：不同压制、不同码率、remux 过的同一作品不算重复（那是番号查重的职责）；
- **不删除、不移动源文件**（全局原则，`spec-v1.md` §41）：应用只删收藏记录，文件由用户在资源管理器处理；
- **不扫描收藏之外的文件**：那是目录监控的职责；
- **不引入第三方哈希库**：`xxhash` / `blake3` 在 2 MiB 样本上省下的时间可以忽略，而确认哈希受磁盘带宽限制；多一个依赖不划算；
- **不做自动批量清理**：每组的取舍需要人工判断；
- 不做启动时自动全库扫描（在设置里给开关也留到 v2，避免开机 IO 争抢）。

## 2. 判定规则：部分哈希（fingerprint）

### 2.1 摘要配方（recipe v1）

```text
digest = sha256( "muse-fp1" || u64le(fileSize) || head || tail )
head   = 文件前 1 MiB
tail   = 文件后 1 MiB
size <= head+tail 时：整个文件只读一次，作为唯一区域参与摘要
```

- **把 size 放进摘要输入**：否则「同头同尾、长度不同」的两个文件会碰撞；
- **头尾都要读**：MP4 的 `moov` 可能在文件头也可能在尾（faststart 与否），MKV 的 Cues 在尾部；只看头或只看尾都会漏；
- **1 MiB 足够**：视频首尾各 1 MiB 已包含容器头与首尾帧数据，「两份不同作品的这两段完全相同且总长也相同」的概率可忽略；
- **配方可版本化**：区域大小、加入新区域都算新配方，靠 `hash_version` 列让旧指纹整体失效（见 §3）；
- 明确后果：同一作品的 remux / 重压版本 → 头尾不同 → **不判重**。这是与番号查重的分工，不是缺陷。

### 2.2 分层剪枝（成本递增）

| 层 | 判据 | 读盘量 | 触发 |
| --- | --- | --- | --- |
| T0 | 磁盘上当前的 size/mtime（`os.Stat`，只读目录项） | 0（不读内容） | 始终 |
| T1 | 部分哈希（头尾 2 MiB） | 2 MiB/文件 | 对没有当前指纹的文件（含从未算过的） |
| T2 | 完整 SHA-256 | 全文件 | 用户对某一组手动点「确认」 |

T1 的范围是**身份**而不是「同 size 才需要」：摘要同时是查重的判据和移动重连的判据，库里的每个收藏都应当有。T0 因此不再承担剪枝职责，只回答「文件还在吗、指纹是否仍描述它」，并负责把过期/失败的行挑出来。

T0 用的是 `os.Stat` 拿到的**当前** size/mtime，而不是数据库 `videos.file_size` 列——后者是导入时记的，文件被同名替换后已经过期（§3.2），照它判断才会漏掉。stat 只碰目录项，不触发文件内容读。

T2 单独成层的原因见 §7：一组两个 5 GB 文件在 USB 机械盘上要 80 秒，不能默认做，但它是「敢不敢删」的信任来源，必须在同一面板里可用。

## 3. 存储（schema v22）

`backend/database/migration.go`：`currentSchemaVersion` 21 → 22，新增一步迁移：

```sql
ALTER TABLE videos ADD COLUMN content_hash TEXT;      -- 部分哈希 hex；NULL/空 = 未算
ALTER TABLE videos ADD COLUMN hash_version INTEGER;   -- recipe 版本；低于 FingerprintVersion 视为过期
ALTER TABLE videos ADD COLUMN hash_size    INTEGER;   -- 计算时磁盘上的 size
ALTER TABLE videos ADD COLUMN hash_mtime   INTEGER;   -- 计算时的文件 mtime（unix 纳秒）
ALTER TABLE videos ADD COLUMN hash_at      INTEGER;   -- 计算时间（unix 毫秒）
ALTER TABLE videos ADD COLUMN hash_error   TEXT;      -- 最后一次失败原因；成功为 ''
ALTER TABLE videos ADD COLUMN full_hash    TEXT;      -- T2 完整 SHA-256（确认后写入）
ALTER TABLE videos ADD COLUMN full_hash_at INTEGER;   -- T2 确认时间（unix 毫秒）
CREATE INDEX IF NOT EXISTS idx_videos_content_hash ON videos(content_hash);

CREATE TABLE IF NOT EXISTS content_duplicate_ignores (
    dup_key    TEXT PRIMARY KEY,   -- = content_hash
    video_ids  TEXT NOT NULL,      -- 忽略时该组成员的 id 升序列表（逗号分隔）
    created_at INTEGER NOT NULL
);
```

`MigrationResult` 新增 `BackfillFingerprints bool`（`version < 22` 时为 true）。照 `BackfillMetadata` 的做法，启动时把缺指纹的行入队，而不是在迁移里同步跑 IO。

`jav_duplicate_ignores` 的 `dup_key/video_ids/created_at` 形状与本表完全一致，但**不复用**：表名不撒谎、两个域的清理时机不同（§10 有取舍说明）。

### 3.1 模型字段：内部状态不外发

`Video` 新增两个可序列化字段和一个**不序列化**的内部状态：

```go
ContentHash string `json:"contentHash"` // 空 = 未算
HashVersion int    `json:"hashVersion"`
// HashState 是校验用的内部记账，永远不出进程（json:"-"）。
HashState FingerprintState `json:"-"`
```

`FingerprintState` 装 `Size / Mtime / At / Error / FullHash / FullHashAt` 六个字段。`repository.go` 的 `videoColumns` 与 `scanVideo` 一次性带上全部 8 列，避免为去重读路径复制一份 23 列的 SELECT；前端只看到 `contentHash`/`hashVersion`（详情面板可以显示「已计算内容指纹」），校验细节不外泄。

T2 的完整哈希必须落库：一组两个 5 GB 文件在 USB 机械盘上要 80 秒，重启后不能要求用户再等一次。`full_hash` 与 `content_hash` 一样按 §3.2 的规则判过期。

### 3.2 指纹过期（关键正确性点）

`Service.RefreshStatus` 只更新 `status`，**不更新 `file_size`**。文件在应用之外被同名替换时数据库完全不知道，旧指纹会继续参与分组 → 用户可能据此删错文件。因此：

- 记 `hash_size` + `hash_mtime` 作为「算指纹那一刻的磁盘事实」；
- 面板每次加载对每个成员做一次 `os.Stat`（与番号查重面板每个版本都 `RefreshStatus` 同一哲学）；
- 任一不符 → 该成员标 `NeedsReverify`，面板显示「⚠ 内容已变化，结论不可信」，并**禁止**在重新校验完成前做「保留此版本」，同时重新入队哈希；
- 文件缺失 → 标 `Status = missing`，允许用户直接移除这条记录（文件已经不在了）。

## 4. 计算：`Fingerprinter`

新增 `backend/video/fingerprint.go`（纯函数）与 `backend/video/fingerprinter.go`（工作池）。

```go
// 纯函数，无 ffmpeg 依赖，可用临时文件做表驱动测试。返回的 size/mtime 与
// digest 取自同一个句柄，不可能互相矛盾。
func Fingerprint(path string) (FingerprintResult, error)

const FingerprintVersion = 1
const fingerprintRegion = 1 << 20 // 1 MiB
```

`Fingerprinter` 与 `Processor` 结构对称（`jobs chan int64`、`Enqueue` 非阻塞且队列满时告警、`Close` 排空、`emit` 事件、`wg`），默认 **2 个 worker**（即最多同时压在 2 个卷上；本机 4 个卷，可放宽到 4）——每个 worker 只读 2 MiB，不会和 ffmpeg 抢 CPU。

调度：按**卷**（`filepath.VolumeName`）分组，卷内按路径顺序处理，卷之间并行。这台机器的库正好落在 E / G / H / I 四块不同物理盘上（`E:`=SATA、`G:`=T7 SSD、`H:`=TOSHIBA USB、`I:`=Seagate USB），所以按盘符分卷已经等价于按物理盘调度；**同一物理盘挂两个盘符**（本机 `C:`/`D:` 就是）是已知近似，留到 v2 用 `IOCTL_STORAGE_GET_DEVICE_NUMBER`（`backend/platform/windows.go`）修正。

失败处理：文件缺失/权限 → 写 `hash_error`，不重试，emit `fingerprint:failed`。断开的盘（数据库里有 `K:\...` 这类当前未挂载的路径）会集中落到这里，面板据此显示「N 个文件读取失败」，而不是静默当成无重复。

全库扫描流程（`ScanContentDuplicates`）：**第一遍只 stat**（T0），逐文件确认可读性并判断指纹是否仍描述当前文件；**第二遍对全部「没有当前指纹」的文件算指纹**（T1）。第二遍既服务查重，也服务 `file-relocation.md` 的移动重连：没有摘要的收藏在改名后无法被认回。首次扫描因此会读每个文件的头尾（约 2 MiB），此后只有新增或被替换的文件需要重算。

**同 size 预取（保留）**：新加入的文件 B 若有同 size 的旧行 A（A 可能是功能上线前导入的），`AddVideo` 会把 A 一并入队，让 A 在「有人提到它」时立刻获得身份。这条不依赖扫描，也不会误判：

- 仓库新增 `ListSameSizePeers(size int64, excludeID int64) ([]int64, error)`；
- `AddVideo` 入队自己之后，再用**刚 stat 到的 size** 查一次同 size 的已有行，把命中行一并入队；
- 扫描本身已覆盖所有缺指纹的行，因此这条只是「更早一步」的补充。

入库时 `AddVideo` 已经 `os.Stat` 过（`NormalizeAndValidate` 返回 `info`），所以这条检查不产生额外 IO。

接线（`main.go`）：

- `video.NewFingerprinter(repo, app.Emit, log, 2)`，与 `processor` 并列构造，并交给 `shutdown` 管理器 `Close`；
- `Service` 新增可选的 `SetFingerprinter`（镜像 `SetWatchService`），**不改 `NewService` 签名**，现有测试不用动；
- `Service.AddVideo` 里紧挨 `s.processor.Enqueue(v.ID)` 调 `s.fingerprinter.Enqueue(v.ID)`：新增视频立即有指纹，代价 ~几十毫秒，`watch` 导入的路径也自然覆盖；
- 启动时 `BackfillFingerprints()`（镜像 `BackfillMetadata`）。

并发与 SQLite：`SetMaxOpenConns(1)` 是单写连接，worker 只写自己那一行，短暂持锁即可，不需要额外串行化。

## 5. 分组与读路径

- 仓库：`ListFingerprinted()` → `SELECT <videoColumns + 指纹列> FROM videos WHERE content_hash IS NOT NULL AND content_hash <> ''`。分组放 Go 里做（与 `jav` 的分组同一做法），因为成员要带完整 `Video`，SQL 里 `GROUP BY` 反而要二次取行。
- `ListNeedingFingerprint()` → `WHERE content_hash IS NULL OR content_hash = '' OR hash_version IS NULL OR hash_version <> ?`，支撑首次全库扫描、配方升级后的自动重算、以及单条重新校验。
- 服务 `ListContentDuplicates(includeIgnored bool) ([]ContentDuplicateGroup, error)`：
  - 按 `content_hash` 分组，只返回成员数 ≥ 2 的组（一份拷贝不算重复）；
  - 与 `content_duplicate_ignores` 比对：**成员 id 集合一致**才算已忽略——新增一份拷贝或多删一份都会让旧决定失效、该组重新出现（与 `duplicate-detection.md` §3 的语义逐字一致，避免两套「忽略」语义分叉）；
  - 组排序按组内最新成员（与收藏库默认顺序一致），组内按 `created_at DESC`；
  - 每组附带 `TotalBytes` / `ReclaimableBytes`（成员数 −1 的总和），供面板显示「重复占用 5.4 GB」；
  - 成员带 `NeedsReverify`、`HashVersion`、`Status`；
  - 组内成员全部删除后清理 ignore 行，避免残留标记压掉将来无关的同内容组。

## 6. API 与界面

门面（`backend/api/library.go`）：

| 方法 | 说明 |
| --- | --- |
| `ListContentDuplicates(includeIgnored bool)` | 内容重复工作列表 |
| `IgnoreContentDuplicate(hash string)` | 保留全部（写 `content_duplicate_ignores`） |
| `ScanContentDuplicates() (ContentScanStatus, error)` | 启动或复用后台全库扫描，立即返回 |
| `ContentScanStatus() (ContentScanStatus, error)` | `running/phase/examined/examineTotal`（T0）与 `total/done/hashed/skipped/failed/startedAt/finishedAt`（T1） |
| `ReverifyContentDuplicate(hash string) error` | 重新 stat + 重新入队该组成员 |
| `ConfirmContentDuplicate(hash string) error` | 可选 T2：后台做完整 SHA-256，完成后写成员的 `full_hash`/`full_hash_at`，组带 `Confirmed` |

扫描状态只放内存（不建表）：重启后 `ListNeedingFingerprint` 天然让扫描可续跑，因此不需要持久化进度。

移除成员复用既有入口 `RemoveVideo` / `DELETE /api/videos/{id}`，组归属清理与缩略图删除行为与收藏库完全一致。

HTTP（`backend/server/server.go`）：

| 端点 | 说明 |
| --- | --- |
| `GET /api/duplicates/content[?includeIgnored=1]` | 工作列表 |
| `POST /api/duplicates/content/ignore` | `{"hash": "..."}` |
| `GET /api/duplicates/content/scan` | 扫描状态 |
| `POST /api/duplicates/content/scan` | 启动扫描 |
| `POST /api/duplicates/content/confirm` | `{"hash": "..."}`，后台执行，靠状态轮询取结果 |

事件：`fingerprint:updated`、`fingerprint:failed`、`fingerprint:scan:progress`。

界面：

- 桌面端在既有 `DuplicateView.tsx` 里加 tab：**「同番号多版本 | 内容重复」**，复用面板外壳（关键字框、显示已忽略开关、卡片右键菜单、保留此版本 / 保留全部、确认对话框），保证两套判据的交互不分叉。补充方案（不采纳）：侧边栏单开一个「重复文件」入口——同一个概念两个入口，用户要先想清楚判据才知道点哪个。
- 内容 tab 顶部：扫描状态条 —— 运行中显示当前阶段的「已处理 / 本轮预计总数」（`正在检查文件大小… 412/899`、`正在比对内容指纹… 2/4`）并配一条进度条，结束后汇总为「上次扫描：检查 899 个 · 比对 4 个 · N 个无法读取」；另有「重新扫描」按钮与失败文件数。空态区分「从未扫描」（引导按钮）与「扫描完成，无重复」。
  - 状态只在内存里：**重启后**状态条会回到「尚未扫描过内容指纹」，即使指纹早已建立。判断「扫过没有」要看指纹列或日志（`app.log` 的 `content scan finished`），不要只看状态条。彻底修掉它需要在 `settings` 表里留一条上次扫描摘要，v1 未做。
- 组卡片：`content_hash` 前 8 位、成员数、重复占用空间、「已完整校验」标记（组成员都有 `full_hash` 且两两相等）、成员卡片上「文件缺失」/「⚠ 内容已变化，需重新校验」标记；后者存在时禁用保留操作，只提供「重新校验」。
- 每个成员卡片带与收藏库完全相同的右键菜单，「打开所在文件夹 / 复制路径」是删除重复文件的正规路径——与 `duplicate-detection.md` §6.1 的工作流一字不差：**文件由用户在资源管理器删除，应用绝不代劳**。
- 移动端 `frontend/src/mobile/components/DuplicateView.tsx` 同样加 tab，走 §6 的 HTTP 端点；移动端不提供「打开所在文件夹 / 复制路径」（作用于服务器文件系统，见 `mobile-web-design.md` 的能力对照表）。
- `frontend/wailsjs/` 是生成产物，靠 `wails dev` / `wails build` 重新生成，不手改。

## 7. 性能预算

本机实测（本次会话现场测量：i5-13400F / 16 GB / Go `crypto/sha256` 走 SHA-NI / 8 MB 缓冲，读的是本库真实文件，每盘采样 2–4 GB 冷读）：

| 盘 | 设备 | 顺序读+哈希 | 单个 5 GiB |
| --- | --- | --- | --- |
| `E:` | 内置 SATA HDD `ST2000DM008` | 182–199 MB/s | 26–28 s |
| `H:` | USB HDD `TOSHIBA 4TB` | 133 MB/s | 39 s |
| `I:` | USB HDD `Seagate Expansion` | 127 MB/s | 40 s |
| `G:` | USB SSD `Samsung PSSD T7` | 400 MB/s | 13 s |
| `C:`/`D:` | NVMe `UMIS 512G` | 1287 MB/s（哈希已是瓶颈） | 4 s |

由实测推导（估算部分已标注）：

- **T0 单文件成本**：`os.Stat` 只读目录项，几百个文件是秒级；机械盘若已休眠，首个 stat 会触发一次唤醒（秒级），之后整卷连续 stat 很快。这是**估算**；
- **T1 单文件成本**：2 MiB 传输 + 两次寻道 ≈ 50–70 ms（USB 机械盘）、≈ 5–10 ms（SSD）。这是**估算**，实现后要用真实库校准一次；
- **首次全库扫描**（898 文件）：全部文件都算指纹，约 1.8 GB 读取；按卷并行后由最大卷决定，分钟级；1976 文件的库约 4 GB，机械盘上视盘速约数分钟；
- **T2 完整确认**：一组两个 5 GB 文件 → `H:` 上 80 s、T7 上 26 s、NVMe 上 8 s。因此只做手动、单组触发，并显示进度；
- 扫描期间与播放/缩略图共存：指纹 worker 只读 2 MiB 且不占 CPU，量化影响是「机械盘上的顺序读会被打断一次」，可接受。

## 8. 测试

- `backend/video/fingerprint_test.go`：表驱动纯函数测试——相同内容不同路径同 digest；同头尾不同 size 不同 digest；空文件；刚好等于区域大小的边界；小于阈值（只读一次）；大于阈值（头尾不重叠）；改名不改 digest。
- `backend/video/fingerprinter_test.go`：镜像 `processor_test.go`——Enqueue 非阻塞、Close 排空、单文件失败不阻塞其他、缺失文件记为 `hash_error`、队列满时丢弃并告警。
- `backend/video/service_test.go` 扩展：**加入与旧文件同 size 的拷贝时，旧文件被一并入队（同 size 预取）**；扫描为每个没有当前指纹的收藏补上指纹。
- `backend/video/service_test.go` 扩展：分组只保留 ≥2 成员；ignore 后隐藏；新增拷贝后重新出现；成员全删后清理 ignore 行；size/mtime 不符的成员标 `NeedsReverify` 且不进入可保留状态；T2 确认后组带 `Confirmed`，成员被替换（size/mtime 变化）后 `Confirmed` 失效。
- `backend/video/contentscan_test.go`：扫描只给缺指纹/过期的行算指纹；`TestContentScanIdentifiesEveryUnidentifiedFile` 固定「体积独一无二也要有身份」。
- `backend/database/migration_test.go`：21 → 22（新列、索引、`content_duplicate_ignores`；老库升级后 `hash_version` 为 NULL 触发回填）。
- `backend/api/library_test.go`：经门面走通「扫描 → 列表 → 忽略 → 移除」，以及未扫描时的空列表与状态。
- 前端：`npm run lint` + `npm run build`（在 `frontend/` 下），并手动验证桌面/移动两个 tab、右键菜单、空态。
- 验证门槛：`go test ./...`、`golangci-lint run ./...`，每阶段单独跑针对性测试。

## 9. 分阶段提交

每步边界清晰、可独立验证、单独提交（`AGENTS.md` 的提交规范）：

1. **`Add content fingerprint storage`** — schema v22 + 迁移测试 + `Video` 模型字段 + repository 读写；无业务逻辑，`go test ./...` 即可验证。
2. **`Add partial-hash fingerprinting`** — `fingerprint.go` 纯函数 + 表驱动测试（不接线，纯新增）。
3. **`Add fingerprint worker and backfill`** — `Fingerprinter` + `Service.SetFingerprinter` + `AddVideo` 入队 + 启动回填 + `main.go` 装配与 shutdown 注册；用 `go test` 与一次真实库扫描验证。
4. **`Add content duplicate panel`** — 分组服务 + `api.Library` + HTTP 端点 + 桌面 tab + 移动 tab。这一步最大，建议再拆成三个提交：后端分组与 API / 桌面 UI / 移动 UI。
5. （可选）**`Add full-hash confirmation`** — T2 完整哈希确认。
6. （可选）**`Map fingerprint scheduling to physical disks`** — 卷 → 物理盘映射，修掉同盘双盘符的近似。

## 10. 风险与取舍

- **误判导致删错文件**：四层防护——size 精确剪枝、头尾样本、可选的完整哈希确认、`NeedsReverify` 过期拦截；并且应用永远不删文件，删除动作只由用户在资源管理器执行。这是本设计里唯一有数据损失的路径，所以防护冗余是刻意的。
- **目录监控回灌**：与 `duplicate-detection.md` §6.1 / §8 完全一致——文件还在磁盘上时「从收藏中移除」会在 5 分钟内被重扫加回来，所以面板文案必须复用同一套引导（打开所在文件夹 → 用户删掉文件 → 记录再移除）。不新增「已舍弃路径」排除表：那是目录监控的能力，应单独设计。
- **同一组同时出现在两个 tab**：同一作品的两份完全相同的拷贝会同时命中番号分组和内容分组。这是对的，不是 bug，面板不做交叉去重——两者回答的是不同问题（「留哪个版本」vs「这两份是不是同一份字节」）。
- **复用 vs 新建 ignore 表**：选择新建 `content_duplicate_ignores`，代价是多一张几乎同形状的表和多一处几乎相同的比对代码；收益是两个域的清理时机与语义可以各自演进（例如 T2 确认状态将来可能参与 ignore 判定），且表名与域一致。若评审认为重复代码不可接受，可改为 `duplicate_ignores(kind, dup_key, video_ids, created_at)`，但那要迁移并重写已实现且已测试的 jav 忽略逻辑——属于另一个提交，不建议和本功能混在一起。
- **T0 的隐含假设**：不依赖数据库的 `file_size`，而是扫描时 `os.Stat` 当前 size/mtime（§2.2）。若某成员在两次扫描之间被替换，下一次扫描会重算它的指纹。
- **扫描不再剪枝的代价**：首次扫描为全部缺指纹的收藏读头尾（2 MiB/文件）；换来的是移动重连在全库可用。若将来库大到分钟级不可接受，应改成「后台分批补指纹」，而不是退回按 size 剪枝——那会让改名重连对大多数文件失效。
- **未挂载的盘**：指纹失败会显式露出（`hash_error` + 面板计数），不静默跳过，否则「盘没插」会被误读成「没有重复」。
