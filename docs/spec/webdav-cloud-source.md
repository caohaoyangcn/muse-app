# Muse WebDAV 云盘片源实施计划

**状态：** In progress（M4 已完成）
**范围：** Muse Go 后端 `backend/cloud`、v24 schema、WebDAV 只读访问、云端片源的添加/元数据/封面/播放/局域网串流、桌面端设置与浏览弹窗
**非范围：** OpenList BitQiu 驱动本身（见 [OpenList fork 与 BitQiu 驱动](openlist-bitqiu-driver.md)）、Muse 内重命名/移动/删除/上传网盘文件、远程目录监控、远程内容哈希去重
**依赖：** 真实比特球联调依赖 [OpenList BitQiu 驱动](openlist-bitqiu-driver.md)；本地可先用 `httptest` WebDAV 或任意 OpenList/AList 实例开发
**入口：** 桌面端的[添加来源弹窗](add-video-source-dialog.md)
**最后更新：** 2026-11-06

---

## 1. 背景与决策

比特球云盘没有公开 WebDAV，它的网盘能力由 AList 3.55.0 的 `drivers/bitqiu` 通过私有 HTTP 接口封装；OpenList 主线目前没有 BitQiu 驱动。因此：

- 运行时**不使用 AList**；由 OpenList fork 提供 BitQiu → WebDAV。
- Muse 只实现**标准 WebDAV 只读客户端**，不直接接触比特球私有 API。
- 网盘是「添加来源」之一，入口在来源选择弹窗；Muse 内只读，管理动作跳 OpenList 网页。
- OpenList 是**可选组件**，默认不随 Muse 安装包分发；只用硬盘的用户完全不受影响。

已确认的产品决策：

- Muse 内只读：浏览目录、多选视频、加入收藏；不改名/移动/删除/上传。
- 桌面播放采用**先缓存后播放**；缩略图/元数据走本地代理按需读取，不下载完整文件。
- 远程文件不参与内容指纹去重、移动重连和启动时批量文件检测。
- 远程 404 标记 `missing`，但保留记录；网络/鉴权失败只标记来源离线，不删收藏。

## 2. 目标与非目标

### 目标

- 配置任意 WebDAV/OpenList 连接，测试连通性，密码加密存储。
- 浏览远程目录，把选中的视频加入收藏；重复添加有明确提示。
- 生成缩略图、时长、分辨率、编码信息；卡片显示来源与离线状态。
- 远程视频可缓存后播放；可一键打开 OpenList 网页管理页。
- 局域网移动端能播放远程收藏（通过 LAN 服务代理 Range）。
- OpenList/网络离线时，收藏保留、状态可恢复、绝不误删。

### 非目标

- 不向 WebDAV 发 PUT/DELETE/MOVE/COPY/MKCOL。
- 不实现比特球私有 API 直连。
- 不做远程目录监控（fsnotify 只适用于本地）。
- 不把远程文件纳入内容哈希/查重/移动重连。
- 不在 v1 做远程批量「文件检测」；单文件在打开/详情刷新时校验。
- 不把 OpenList 打包进默认 Muse 包。

## 3. 架构

### 3.1 包结构

```text
backend/cloud/
  model.go        Connection / ConnectionInput / ConnectionView / Entry / TestResult
  errors.go       ErrUnauthorized / ErrNotFound / ErrUnreachable / ErrUnsupported
  repository.go   cloud_connections 的 SQLite 读写
  webdav.go       PROPFIND / GET Range / HEAD 客户端
  service.go      连接 CRUD、浏览、Stat、OpenRange、健康检查
  proxy.go        仅 127.0.0.1 的媒体代理（带随机 token）
  cache.go        下载缓存、断点续传、LRU、清理
  stream.go       供本地代理与 LAN 服务复用的 Range 转发
  *_test.go
```

新增 `backend/secret`（从 `backend/settings/secret_*.go` 抽出）：

- `Protect/Unprotect/Available`；Windows DPAPI，非 Windows 返回 `ErrUnsupported`。
- `settings` 改为调用 `secret`，现有 cookie 存储行为不变。

### 3.2 数据模型（schema v24）

`backend/database/migration.go` 增加 v24：

```sql
CREATE TABLE cloud_connections (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  name         TEXT NOT NULL,
  kind         TEXT NOT NULL DEFAULT 'webdav',
  endpoint     TEXT NOT NULL,
  web_url      TEXT NOT NULL DEFAULT '',
  username     TEXT NOT NULL DEFAULT '',
  password_enc TEXT NOT NULL DEFAULT '',
  root_path    TEXT NOT NULL DEFAULT '/',
  verify_tls   INTEGER NOT NULL DEFAULT 1,
  created_at   INTEGER NOT NULL,
  updated_at   INTEGER NOT NULL,
  last_ok_at   INTEGER,
  last_error   TEXT NOT NULL DEFAULT ''
);

ALTER TABLE videos ADD COLUMN source_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE videos ADD COLUMN remote_path TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX idx_videos_remote ON videos(source_id, remote_path) WHERE source_id > 0;
CREATE INDEX idx_videos_source ON videos(source_id);
```

`backend/video/model.go` 新增：

| 字段 | 含义 |
| --- | --- |
| `SourceID` | 0 = 本地；>0 = `cloud_connections.id` |
| `RemotePath` | WebDAV 上的绝对路径，POSIX 风格，百分号解码后保存 |
| `SourceName` | 只读投影：连接名称 |
| `SourceOffline` | 只读投影：连接最近一次探测失败 |

路径策略：

- 远程视频的 `path` 使用合成值 `cloud://<connectionID>/<sha1(remotePath)>`，避免 WebDAV 大小写敏感路径被 `COLLATE NOCASE` 误判。
- 真实路径放 `remote_path`，由 `(source_id, remote_path)` 的 BINARY 唯一索引去重。
- `path` 只作为稳定主键/调试用；界面显示 `SourceName + RemotePath`。
- 本地视频 `source_id=0`、`remote_path=''`，所有现有行为不变。

仓储改动：

- `videoColumns` / `scanVideo` / `Create` 增加两列。
- 新增 `GetByRemote(sourceID, remotePath)`。
- `Search` 同时匹配 `file_name` 与 `remote_path`。
- `ListNeedingFingerprint`、`ListSameSizePeers`、`ListRelocationCandidates`、`ListFingerprinted`、`ListByContentHash` 增加 `source_id = 0` 过滤。
- 迁移测试覆盖新表、列、索引和旧库升级。

### 3.3 WebDAV 客户端

标准库实现，不引入第三方依赖：

- `PROPFIND Depth 0/1`，请求 `resourcetype/getcontentlength/getlastmodified/getcontenttype/displayname`。
- `GET` 带 `Range`，`HEAD` 用于快速存在性检查。
- Basic Auth；每连接可选 `verify_tls`；连接/请求超时；支持中文与百分号编码路径。
- 路径规范化：`path.Clean`、必须以 `/` 开头、拒绝 `..`、不能越过连接根。
- 错误映射：401/403 → `ErrUnauthorized`；404 → `ErrNotFound`；DNS/超时/TLS → `ErrUnreachable`。
- `Entry`：`Name/Path/IsDir/Size/Modified/ContentType`。

### 3.4 连接服务

- `ListConnections()` 返回 `ConnectionView`：不包含密码，只带 `HasPassword`、`Online`、`LastError`、`VideoCount`。
- `SaveConnection(input)`：创建/更新；密码为空时保留原密码；显式清空需单独标志。
- `DeleteConnection(id)`：如果 `videos.source_id = id` 还有收藏，拒绝并返回数量。
- `TestConnection(input)`：PROPFIND 根目录，返回延迟与错误；错误文案中文。
- `Browse(id, path)`：Depth 1 列表；目录在前，文件按名称排序。
- `Stat(id, path)`：Depth 0。
- `OpenRange(ctx, id, path, offset, length)`：GET Range；206 直接返回；200 退化为完整流。
- 健康检查：后台每 60 秒 PROPFIND 根目录；成功/失败更新内存状态并发出 `cloud:status`；列表读取不访问网络。

### 3.5 媒体代理

启动一个仅绑定 `127.0.0.1` 的随机端口 HTTP 服务，路径：

```text
http://127.0.0.1:<port>/media/<random-token>/<videoID>
```

- 代理把 `Range` 转发给 WebDAV，转发 `Content-Range/Content-Length/Content-Type`。
- token 每次进程启动随机生成，只允许 GET/HEAD。
- 供 ffprobe/ffmpeg 读取远程视频；避免把 WebDAV 密码放进命令行。
- 服务端不支持 Range 时退化为完整流；日志记录一次降级。
- 代理不缓存完整文件；缩略图/元数据只读需要的字节。

### 3.6 视频服务集成

`backend/video` 不导入 `backend/cloud`，通过钩子解耦：

```go
func (s *Service) SetRemoteSupport(
    open func(context.Context, *Video) error,
    check func(context.Context, *Video) (bool, error),
    mediaURL func(*Video) (string, error),
    online func(*Video) bool,
)
```

- `Open`：远程走 `open`；本地保持现状。
- `RefreshStatus`：远程走 `check`；404 标 `missing`；网络/鉴权失败不修改 `status`，`SourceOffline` 由读模型按连接健康状态填。
- `OpenContainingFolder`：远程改为打开 OpenList 网页（或返回「网盘文件没有本地目录」）。
- `FileScan`：只扫描本地；`FileScanStatus` 增加 `Remote` 计数；远程不参与 `reconcileVolume`。
- `volume.go` 的 `stamp`/`probe`/`reconcile` 跳过 `source_id > 0`。
- `Fingerprinter`/`ContentScan`/`Relocate` 的查询排除远程。
- `Processor` 增加 `SetMediaPathResolver`：本地返回 `Path`，远程返回代理 URL；远程 ffprobe 超时放宽到 60–90 秒。

### 3.7 API 与绑定

`backend/api/library.go`：

```text
ListCloudConnections
SaveCloudConnection
DeleteCloudConnection
TestCloudConnection
BrowseCloud
AddCloudVideos
OpenCloudPage
CancelCloudDownload
ClearCloudCache
CloudCacheInfo
```

`app.go` 暴露同名 Wails 绑定；`main.go` 完成 wiring：

```text
cloudSvc → api.Library.SetCloudService
         → videoSvc.SetRemoteSupport
         → processor.SetMediaPathResolver
         → server.SetCloudService
```

### 3.8 局域网服务

`backend/server/server.go` 的 `handleStreamVideo`：

- 本地：保持 `streaming.ServeFile`。
- 远程：调用 `cloud.ServeVideo(w, r, sourceID, remotePath, fileName)`，复用 Range 转发。
- 离线：503 + 可读中文错误；404：404。
- 手机端 `/stream/video/{id}` 不变，远程收藏可直接播放。

### 3.9 前端

- `frontend/src/pages/Settings.tsx` 新增「网盘（可选）」：
  - 连接列表：名称、WebDAV 地址、在线状态、收藏数、编辑/测试/删除/打开 OpenList。
  - 表单：名称、WebDAV 地址、网页地址（可选）、用户名、密码、根目录、允许自签名证书。
  - 文案明确：Muse 只读；比特球需先由 OpenList fork 挂载。
- `frontend/src/components/CloudBrowserDialog.tsx`：
  - 连接选择、面包屑、目录/视频列表、多选、添加、加载/错误/空状态。
  - 无连接时显示「去设置 → 网盘」。
- `frontend/src/components/AddSourceDialog.tsx`：网盘卡片入口（见 `add-video-source-dialog.md`）。
- `VideoCard`/详情：云端角标、`SourceOffline` 警告、`missing` 提示。
- `ContextMenu`：远程视频显示「在 OpenList 中打开」；复制路径复制 `RemotePath`。
- `services/api.ts` 增加绑定封装；`services/events.ts` 接入 `cloud:status`；`cloud:download` 由 Library 视图订阅。

### 3.10 播放与缓存

- 远程「打开」：下载到 `dataDir/cloud-cache/<videoID>-<安全文件名>`，进度通过 `cloud:download` 上报，完成后调用系统默认播放器。
- 支持取消、`Range` 断点续传、大小 + 远端修改时间校验。
- 默认缓存上限 10 GiB，LRU 淘汰；设置页提供「清理缓存」。
- 删除收藏时删除对应缓存副本（只删生成物，不碰网盘源文件）。
- 「在 OpenList 中打开」使用连接 `web_url`；为空则从 endpoint 去掉 `/dav` 推导。
- v1 不做边下边播；后续可基于代理 URL 增加 `.m3u`/流式选项，不改数据模型。

## 4. 关键数据流

### 添加

```text
CloudBrowserDialog
  → BrowseCloud(connID, path)
  → 选中文件
  → AddCloudVideos(connID, paths)
  → cloud.Stat 校验存在/扩展名/大小
  → video.AddRemoteVideo
  → videos 行（source_id, remote_path, 合成 path）
  → processor 入队
  → 代理 URL 给 ffprobe/ffmpeg
  → 元数据 + 缩略图
  → 前端 invalidate()
```

### 播放

```text
OpenVideo(id)
  → 远程：cloud.Open
  → 缓存有效：直接打开
  → 缓存无效：下载（进度事件）→ 完成 → 系统播放器
  → 用户也可「在 OpenList 中打开」
```

### 离线

```text
健康检查失败
  → 连接 Online=false
  → cloud:status
  → 远程卡片 SourceOffline=true
  → 打开失败提示「网盘未连接」
  → 不修改 status、不删记录
  → 恢复后下次探测成功，警告消失
```

## 5. 实施里程碑

### M1：schema 与仓储

- [x] v24 迁移、模型字段、仓储列/查询。
- [x] 迁移测试。
- [x] 现有 `go test ./...` 通过。

### M2：WebDAV 客户端

- [x] PROPFIND 解析（中文/转义 href）。
- [x] Range GET / HEAD。
- [x] 路径越界拒绝。
- [x] 错误映射。
- [x] `httptest` 单测。

### M3：连接服务

- [x] `backend/secret` 抽取，`settings` 迁移。
- [x] 连接 CRUD、测试连接、删除保护。
- [x] 健康缓存与 `cloud:status`。
- [x] 服务层单测。

### M4：浏览与添加

- [x] `BrowseCloud` / `AddCloudVideos`。
- [x] `video.AddRemoteVideo`、去重、复活。
- [x] 读模型 `SourceName/SourceOffline` 附加。
- [x] API/Wails 绑定。
- [x] 前端网盘设置与浏览弹窗。

### M5：媒体代理与元数据

- [ ] 127.0.0.1 代理 + token。
- [ ] `Processor.SetMediaPathResolver`。
- [ ] 远程 ffprobe/缩略图超时。
- [ ] 生成封面与元数据手测。

### M6：播放与缓存

- [ ] 下载器、进度、取消、断点续传。
- [ ] LRU 清理、删除收藏清缓存。
- [ ] `OpenCloudPage`。
- [ ] 桌面播放手测。

### M7：离线与检测语义

- [ ] 文件检测跳过远程，增加 `Remote` 计数。
- [ ] 卷逻辑跳过远程。
- [ ] 指纹/查重/重连排除远程。
- [ ] 404 vs 网络失败行为测试。

### M8：LAN 与移动端

- [ ] `server.handleStreamVideo` 远程分支。
- [ ] 手机端播放远程收藏。
- [ ] 离线 503 文案。

### M9：文档与端到端

- [ ] `docs/cloud-drive-setup.md`。
- [ ] OpenList fork 联调。
- [ ] 全套静态检查。

## 6. 边界与失败模式

| 场景 | 行为 |
| --- | --- |
| WebDAV 401/403 | 连接测试失败，提示检查账号/密码；不删收藏 |
| WebDAV 404 | 目录不存在或文件已删；收藏标 `missing`，保留记录 |
| 网络超时/DNS 失败 | 连接离线；保留收藏；后台重试 |
| 服务端不支持 Range | 代理退化为完整流；缓存仍可完成 |
| 重复添加 | 返回「已在收藏中」，不新建记录 |
| 连接下仍有收藏却删除连接 | 拒绝，提示先移除收藏 |
| 缓存盘满/超上限 | LRU 淘汰；单文件过大时提示手动清理 |
| 远程大小/修改时间变化 | 缓存失效，重新下载 |
| 非 Windows 保存密码 | 返回「当前平台不支持安全保存密码」；匿名 WebDAV 可测试 |
| 自签名 HTTPS | 连接级开关，默认校验；关闭时 UI 警告 |
| 批量添加 | 逐个校验，限并发 2–3 |
| 远程路径大小写不同 | `(source_id, remote_path)` BINARY 唯一索引区分；合成 path 用哈希 |
| OpenList 网页地址与 WebDAV 不同 | 允许单独填写 `web_url` |
| 远程文件被删 | 标 `missing`，不自动删 |
| OpenList 离线启动 | 无阻塞、无弹窗；卡片显示离线 |

## 7. 测试与验收

### 单测

- `backend/cloud`：PROPFIND 解析、Range 206/200、401/404 映射、路径越界、连接 CRUD、删除保护、健康缓存、加密 codec。
- `backend/video`：v24 迁移、远程添加/去重、文件检测跳过远程、卷逻辑忽略远程、fingerprint 查询排除远程、Open/RefreshStatus 钩子。
- `backend/api`：`AddCloudVideos` 校验、读模型附加。
- `backend/server`：远程 `/stream/video/{id}` 走代理、离线 503。

### 集成/手测

- 启动任意 WebDAV（OpenList/AList）→ 添加连接 → 浏览 → 添加 → 封面/时长 → 播放 → 停服务验证离线 → 恢复验证。
- 用 OpenList fork 挂 BitQiu 做真实端到端（见 `openlist-bitqiu-driver.md`）。
- 手机同 LAN 打开远程收藏播放。

### 验收

- 连接 CRUD/测试/删除保护可用，密码不出现在 JSON/日志。
- 浏览与多选添加可用，重复/缺失有清晰提示。
- 远程封面、时长、编码信息正确。
- 播放缓存有进度、可取消、可清理。
- 离线不误删、不把网络失败当 missing。
- LAN 可播放远程收藏。
- `go test ./...`、`golangci-lint run ./...`、`npm run lint`、`npm run build` 通过。

## 8. 提交拆分

| 顺序 | 提交信息（建议） | 内容 |
| --- | --- | --- |
| 1 | `Add cloud source schema and repository` | v24、模型、仓储、测试 |
| 2 | `Add WebDAV client for cloud sources` | `backend/cloud/webdav.go`、单测 |
| 3 | `Add cloud connection service and secret storage` | `backend/secret`、连接 CRUD、健康 |
| 4 | `Support remote videos in the library` | video 钩子、读模型、文件检测/卷/指纹排除 |
| 5 | `Add cloud media proxy and cache playback` | 代理、缓存、播放、API |
| 6 | `Stream cloud videos over the LAN server` | `handleStreamVideo` 远程分支 |
| 7 | `Add cloud source settings and browser UI` | Settings、CloudBrowserDialog、卡片/菜单 |
| 8 | `Add cloud source to add-source dialog` | 与 `add-video-source-dialog.md` 对接 |
| 9 | `Document cloud drive setup` | 安装/配置/故障排查 |

## 9. 风险与开放问题

- **比特球私有 API 稳定性**：由 OpenList fork 承担；Muse 只依赖 WebDAV，驱动失效不影响本地功能。
- **DPAPI 仅 Windows**：macOS 开发端无法保存密码；非 Windows 只支持匿名 WebDAV。是否要 macOS Keychain 支持，留到有实际需求时。
- **缓存占用**：默认 10 GiB；是否需要按连接/全局配置，实现时再定。
- **大文件预览**：依赖 OpenList Range；不支持时退化完整流，UI 提示可能较慢。
- **远程批量检测**：v1 不做；如果用户需要「云端 200 个文件是否还在」，后续单独设计，避免启动时几百个 PROPFIND。
- **OpenList 可选安装**：默认不随包分发；Muse 只连接外部 WebDAV，不依赖本机 OpenList。

## 10. 进度记录

| 日期 | 里程碑 | 状态 | 说明 | 提交 |
| --- | --- | --- | --- | --- |
| 2026-11-05 | 计划 | Planned | 计划文档建立 | `9da1586` |
| 2026-11-06 | M4 | Done | 远程视频 `cloud://<id>/<sha1>` 合成路径、去重/复活；`BrowseCloud`/`AddCloudVideos` 与 Wails 绑定；`SourceName/SourceOffline` 读模型；设置「网盘（可选）」、`CloudBrowserDialog`、卡片/详情角标；headless Chromium 验证浏览/多选/设置表单。 | `edcb7a7` |
| 2026-11-06 | M3 | Done | `backend/secret`（DPAPI / 非 Windows 拒绝）；连接 CRUD、密码保留/清空、删除保护、连接测试与 60 秒健康检查、`cloud:status`；服务层用 `httptest` 和 SQLite 覆盖。 | `efa5e34` |
| 2026-11-06 | M2 | Done | 只读 WebDAV 客户端：PROPFIND Depth 0/1（中文/转义/相对 href）、HEAD、Range 206/200、TLS 开关、根目录约束、401/403/404/5xx 错误映射；`httptest` 覆盖。 | `74d4126` |
| 2026-11-06 | M1 | Done | v24：`cloud_connections`、`videos.source_id/remote_path`、部分唯一索引；模型与 `GetByRemote`；本地指纹/重连查询排除远程；迁移与仓储测试。`go test ./...`、`go vet ./...` 通过。 | `6629ebc` |
