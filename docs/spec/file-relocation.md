# Muse 文件移动重连（Hash 身份）设计

**Status:** Implemented (v1：入库时按字节重连；「主动发现移动」仍为非目标)
**Scope:** 一个刚入库的文件，其字节与某条「失踪」收藏一致时，把该收藏的路径引用改到新文件，而不是新建一条收藏
**Target:** 桌面端（Wails）+ 局域网移动端（共用 `api.Library` 门面）
**Version:** v1
**依赖:** 无新增依赖；判据复用 `content-hash-dedup.md` 的部分哈希

---

## 0. 问题

收藏是引用，不是文件（`spec-v1.md` §41）。文件一旦被移动，引用指向的路径就不存在了：`RefreshAllStatuses` 在启动时、`Open` / `RefreshStatus` 在打开前把它标成 `missing`（§20、§21），界面显示 `⚠ 文件缺失`。

在此之前，用户唯一的出路是「移除收藏」再重新添加。代价是 **id 变了**——而分组、点赞、刮削数据（番号/演员/封面）、自定义标题与备注全部挂在 id 上，于是这些数据一起丢失。`spec-v1.md` §40 把 File relocation detection / Hash based identity 列为 V2。

本设计用已有的内容指纹（部分哈希）判定「新入库的文件就是那条失踪收藏的文件」，并把引用改过去；id 与所有用户数据保持不变。

## 1. 目标与非目标

v1 目标：

- 新入库文件的字节等于某条失踪收藏记录的字节时，自动重连该记录；
- 代价有界：库里没有「文件已不在」的候选时，入库不额外读任何字节；
- 不误判：字节不同绝不重连；同一份字节存在多条收藏且无法区分时，宁可新建也不猜；
- 磁盘上的文件一个字节都不动（全局原则）。

v1 非目标：

- **不主动发现移动**：不去遍历磁盘找失踪文件的去处（那需要全盘扫描；见 §8）；
- **不匹配没有指纹的收藏**：没有身份就没有判据，见 §5；
- **不推断未挂载卷上的文件**：拔掉的盘不是「移动」，见 §2.2；
- 不做相似度匹配：不同压制、不同码率不算同一份字节，那是番号查重的职责。

## 2. 触发点与判据

### 2.1 触发点：入库

只有一个入口：`video.Service.addVideo`（`AddVideo` 与 `AddVideos` 的共同实现）。手动选择、拖拽、目录监控的 `watch.scanRoot` 全部经过它，因此移动重连对三条路径同时生效。

目录监控在交给这个入口之前还有一道**可读性门禁**（`watch.importable`）：文件当前必须能被打开读取，才会被导入。下载客户端仍独占着文件时导入，会得到一条指纹算不出来的收藏；等文件被改名，这条就没有任何身份可供重连，于是变成两张卡片。门禁让它稍后重试（`importRetry` 一次延迟扫描，之后由 5 分钟兜底扫描接手），而不是先记一条注定对不上的引用。

时间点决定了判据不能只看状态：移动后的新路径通常由目录监控在 1 秒级发现，而旧路径要被标成 `missing` 需要一次对账（启动时，或 5 分钟兜底扫描）。若只认 `status = 'missing'`，最典型的场景——**运行中把目录改名**——恰好落在两者之间的窗口里。

### 2.2 候选筛选（便宜 → 贵）

1. **同 size 且有指纹**：`ListRelocationCandidates` 一条 SQL 取 `file_size` 相等、`content_hash` 非空、`hash_version = FingerprintVersion` 的行。移动不改变字节，所以只有同 size 的收藏可能是它；这一步让健康库上的开销降到一次查询。
2. **文件真的不在了**：`status = 'missing'` 直接入选；否则要求 `stat(path)` 失败 **且** 卷根（`filepath.VolumeName` + 分隔符）`stat` 成功。后半句是拔盘保护：路径暂时不可达的收藏不是「移动」，重连它会让收藏脱离一块还会插回来的盘。
3. **读新文件一次**：仅当上一步仍留有候选时，才对入库文件算一次部分哈希（约 2 MiB，`Fingerprint`）。
4. **digest 相等**：字节一致才入选。
5. **消歧**：若多条收藏的 digest 相同，取 `file_name`（大小写不敏感）与新文件相同的那条；仍有歧义（含两条同名）则放弃重连、按新收藏记录，并写日志。

判定错误的代价是不对称的：漏一次重连只是多一条记录（查重面板会显示），错一次重连会把一条收藏指向别的视频。因此第 2 步和第 5 步都选择保守。

### 2.3 重连写入

`RelinkPath` 是**一条** UPDATE：`path`、`file_name`、`status = available`、`file_size`、`hash_size`、`hash_mtime`、`hash_at`、`hash_error = ''`、`updated_at`。

- `content_hash` / `hash_version` 原样写回：摘要描述的正是这些字节，这正是重连成立的原因；
- `full_hash` 保留：它是同一份字节的完整校验结果；
- 用户数据列（title / notes / like_count / 封面 / 分组成员 / 刮削数据）一律不碰，分组等表通过 id 关联，天然跟随。

一条语句同时写完路径与事实，行不可能处在「新路径 + 旧事实」的中间态。

## 3. 为什么留着的指纹是安全的

失踪收藏保留 `content_hash` 是重连的前提，而这与内容查重的安全规则不冲突：**所有查重读路径都过滤 `status = available`**（`ListFingerprinted`、`ListByContentHash`、`ListNeedingFingerprint`，以及内容扫描的 available 集合），所以失踪行上的摘要进不了查重面板——它是「记得的字节」，不是「当前的结论」。

反向的规则也保留：指纹器读不到文件时会清掉摘要（`UpdateFingerprintFailure`，`fingerprinter_test.go` 固定了该行为），这只会让那行失去重连资格，不会造成误连。

## 4. 同路径复活（文件回到原处）

`GetByPath` 命中且该行是 `missing` 时，不再返回「该文件已添加」：标回 `available`，并重新入队处理器与指纹器。路径没变，所以不需要摘要判据；但路径背后的字节可能已经换了，因此元数据、封面、指纹都必须重新推导（`reviveFavorite`）。

## 5. 已知边界

- **没有指纹就认不出**：库里指纹来自入库时的入队哈希和「内容扫描」；扫描现在会为**每一个**没有当前指纹的收藏补算（不再只算 size 冲突的行），所以跑一次扫描就能让全库具备重连能力。功能上线前导入、且此后没跑过扫描的行仍会先漏一次重连。
- **多条收藏共享同一份字节且文件名都不同** → 不重连，新建记录，交给查重面板。
- **卷未挂载时不重连**（§2.2 第 2 步），即使文件确实不在了；这类行由启动对账标成 `missing`，之后仍可在重连窗口内被认回。
- **不处理仅大小写不同的重命名**：`videos.path` 是 `UNIQUE COLLATE NOCASE`，`GetByPath` 会直接命中，走 §4 的复活路径。
- **重连结果不参与自动分组**：`AutoTagVideos` 跳过 `Relocated`——这些字节首次入库时已经过同一套规则，再应用一次只会撤销用户后来手工去掉的分组。桌面端的「分组建议」同样跳过。
- 重连后若该收藏从未完成过处理（文件在缩略图/元数据完成前就消失了），`needsProcessing` 会把它重新入队；已经处理过的不会重复 ffprobe / 抽帧，因为字节完全相同。

## 6. 接口与事件

| 位置 | 变化 |
| --- | --- |
| `video.Service.addVideo` | 入库时先尝试重连（`relocate.go`）；命中则返回既有记录 |
| `video.AddVideoResult.Relocated` | 报告该文件是「重连」而非「新增」 |
| `video:relinked` | `{id, from, path}`；桌面端据此重新加载网格 |
| `watch.ScanSummary.Relocated` / `watch:scanned` | 扫描把重连与新增分开计数（`{path, added, relocated, failed}`），toast 分别提示 |
| `api.Library.AutoTagVideos` | 跳过 `Relocated` 结果 |
| `frontend/wailsjs/go/models.ts` | 由 `wails generate module` 重新生成（勿手改） |

移动端无新增端点：移动端没有添加入口，重连结果通过既有的 `GET /api/videos` 可见（HTTP 无事件通道，见 `mobile-web-design.md`）。

## 7. 测试

- `backend/video/relocate_test.go`
  - 同字节换路径 → 复用同一条收藏（id、path、status、指纹事实、`hash_error` 全部正确），收藏数不变；
  - 同 size 不同字节 → 各自成条，失踪行不动；
  - 用户数据（标题/备注/点赞）在重连后保留；
  - 目录监控先看到新文件、旧路径尚未标 missing → 仍能重连（卷在）；
  - 卷不可达（`stat` 桩：文件与卷根都失败）→ 不重连，且**不读**新文件；
  - 没有指纹的失踪行 → 不重连；
  - 库里无候选 → 不读新文件；
  - 文件回到原路径 → 复活同一条记录；
  - `AddVideos` 逐条报告 `Relocated`；
  - `pickRelinkTarget` 的表驱动用例（无匹配 / 单匹配 / 文件名消歧 / 两种歧义）；
  - `RelinkPath` 与 `ListRelocationCandidates` 的仓库级列行为。
- `backend/watch/service_test.go`：一次扫描里重连 + 新增各计一次并写进事件；下一次扫描不重复计数；重连结果不被自动分组。
- 前端：`npm run lint`、`npm run build`（`frontend/`）。

## 8. 后续（本设计未做）

- **主动「查找丢失文件」**：按 size → digest 在用户指定的目录里找失踪文件的去处（正向匹配；本设计只做了入库时的反向匹配）。
- **整目录前缀重写**：整个目录搬家时按前缀一次性改写，零 IO、最高置信度。
- **移动的感知（而非重连）**：fsnotify 双向对账 + 定期校验，让 `missing` 在移动后立刻出现，见 `TODO.md` 的「失效视频自动检测」。
