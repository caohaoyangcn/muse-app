# Muse 后续迭代 TODO

> 记录待实现的功能与改进项。完成一项即删除或勾选一项（`[x]`）。

## V2 计划

## macOS 适配（当前实施）

> 目标：自用 macOS App，从 `/Applications` 启动；支持 macOS 13+、Apple Silicon（`darwin/arm64`）。
> UI 重构已完成，当前进入平台适配实施阶段。
>
> 已确认范围：Windows/macOS 使用独立数据目录，不做数据迁移；不做 DMG/PKG、安装器、自动更新、签名、公证、Universal Binary；不启用 App Sandbox。

### 阶段 1：平台编译与系统操作

- [x] 将 `backend/platform/windows.go` 拆为 Windows 与 macOS 实现，确保 Windows 行为不回归。
- [x] macOS `Command` 使用普通 `exec.Command`，移除 Windows 专属 `HideWindow` / syscall。
- [x] 默认播放器改为 `open <path>`。
- [x] “打开所在文件夹”改为 `open -R <path>`，在 Finder 中定位文件。
- [x] 剪贴板使用 Wails API，`pbcopy` 作为 macOS 兜底；移除 `cmd /c clip` 路径。
- [x] 局域网服务在 macOS 跳过 `netsh` 防火墙规则，保留系统提示和手动排障说明。
- [x] 验收：`GOOS=darwin GOARCH=arm64 go test ./...` 通过编译与测试。

### 阶段 2：路径、存储与外部工具

- [x] 将 `%LOCALAPPDATA%\\Muse` 改为 macOS 用户目录（macOS 使用 `~/Library/Application Support/Muse`，保留现有子目录结构）。
- [x] 启动失败日志提示使用运行时实际路径，不再出现 Windows 路径。
- [x] `FindTool` 优先检查 `/opt/homebrew/bin`，再检查进程 `PATH`；支持无扩展名的 `ffmpeg` / `ffprobe`。
- [x] 演员 venv 从 `venv/Scripts/python.exe` 改为 `venv/bin/python`，bootstrap 使用 `python3`。（已随人脸识别功能一并移除）
- [x] 构建时将 `backend/pyactor/server.py`、`bootstrap.py`、`requirements.txt` 放入 `.app/Contents/Resources/pyactor/`。（已随人脸识别功能一并移除）
- [x] 保持现有约束：默认大小写不敏感 APFS/HFS+；拒绝 symlink/Finder Alias；watcher 跳过隐藏目录。
- [ ] 验收：从 `/Applications` 启动时，FFmpeg 不依赖当前工作目录。

### 阶段 3：Wails 与桌面交互回归

- [ ] 在真实 macOS 上验证多文件选择器、目录选择器、Finder 拖拽、窗口最小尺寸和 Retina 缩放。
- [ ] 验证视频播放、缩略图、元数据、目录监控、剪贴板、Finder 定位和 LAN 访问。
- [ ] 验证 macOS 文件权限拒绝时的错误提示；不启用 App Sandbox。
- [ ] 保持降级原则：外部工具（ffmpeg）不可用时，媒体库核心功能继续可用。

### 阶段 4：本地构建与文档

- [x] 配置 macOS Bundle Identifier：`com.caohaoyang.muse`。
- [x] 验证 `wails build --platform darwin/arm64` 产出可从 `/Applications` 启动的 `.app`。
- [x] 文档补充 Homebrew 前提：`ffmpeg`、`ffprobe`。
- [ ] 更新 spec 中的 Windows-only 表述、数据目录和 macOS 排障路径（README 已更新；spec 仍保留 Windows 基线）。
- [ ] 增加最小 macOS smoke check：构建、启动、导入一个视频、生成缩略图、打开视频、停止退出。

- [x] **生成的图片不会残留**
  - 缩略图与刮削封面按 `<id>.jpg` 命名，与 `videos` 表定期对账：启动后台一次 + 每次移除后台一次 + 每 24 小时一次；HTTP 侧 `POST /api/maintenance/orphaned-images`。
  - 设计：`docs/spec/generated-images.md`。
  - ⚠️ 本机 `<dataDir>/ai/`（866 MB）与 `<dataDir>/faces/`（58 MB）是**已删除的人脸识别功能**留下的文件，相关表已在迁移 21 中 DROP，无代码引用。它们不在图片清扫范围内（清扫只认 `thumbnails/` 与 `covers/jav/`），需要一次显式的一次性迁移来清理，属未决事项。

- [x] **失效视频自动检测 + 回收站**
  - 当检测到收藏列表中某个视频文件已经不存在（如被移动/删除/外接盘未挂载）时，自动将其移入「回收站」；手动移除也走同一条软删除路径。
  - 回收站标明移入原因、移入时间、剩余天数与**曾经所属的 group**；可恢复、彻底删除或清空。
  - 默认保留 14 天（可配置 1–365 天），启动时与运行中每小时自动清理超期条目；只删记录与生成图片/缓存，不碰源视频。
  - ✅ 外接盘未挂载已单独识别（`docs/spec/volume-offline.md`）：不写 `missing`、卡片照常展示，仅在右下角标「⚠ 磁盘未接入」；盘插回后自动恢复。
  - ✅ 卷在线 + 文件确认缺失时自动移入回收站（启动、盘插回、手动「检测文件」与打开番号查重页时触发；整卷全部缺失时只标 `missing` 防盘符复用）。设计见 `docs/spec/missing-auto-removal.md` 与 `docs/spec/trash.md`。
  - ✅ 被移动的文件重新添加时通过内容指纹接回回收站中的原记录，id、分组、点赞与刮削资料不丢。
  - ⏳ 仍未做：运行中无卷插拔、且没有打开查重页时的持续检测。现在可随时手动重跑启动时的整卷对账（收藏列表工具栏「检测文件」，或 `POST /api/maintenance/files/scan`），但不会自动感知运行中的删除。

- [x] **Web 端访问（局域网浏览 + 外部播放器）**
  - ✅ 桌面端启动时自动开启 LAN HTTP 服务（端口冲突只记警告、不阻断启动），头部「📱 局域网访问」按钮可停止/重新启动。
  - ✅ 移动端 Web UI：独立 React 应用（`frontend/src/mobile/`），构建产物入库到 `backend/server/webui/`。
  - ✅ 原始文件串流：HTTP Range + 按扩展名推断 Content-Type；服务端不 remux/转码。
  - ✅ Web API 读端点：`/api/videos`（`?search=`）、`/api/videos/{id}`、`/api/videos/{id}/jav`、`/api/videos/{id}/jav/task`、`/api/groups`、`/api/groups/{id}/videos`、`/api/jav/tasks`、`/api/actors`、`/api/actors/videos?name=`。
  - ✅ Web API 写端点：`POST /api/videos`、`DELETE /api/videos/{id}`、`PUT /api/videos/{id}/info`、`POST|DELETE /api/videos/{id}/likes`、`POST|PUT|DELETE /api/groups...`、`POST /api/groups/{id}/videos`。
  - ✅ 移动端界面：浏览/搜索/分组筛选/排序/网格列表切换、详情（元数据/演员/标签/文件信息）、iOS 外部播放器跳转（Infuse/VLC）、演员索引与作品页、刮削任务看板、标题备注编辑、点赞、移除收藏。
  - ⚠️ 写端点无鉴权（见下方「局域网服务」）；桌面专属能力（发起刮削、封面选帧、演员合并、分组结构维护、添加收藏、目录监控、设置）在移动端不提供。
  - 设计文档：`docs/spec/mobile-web-design.md`。
  - 后续可扩展：鉴权/口令、HTTPS、更多具备公开播放协议的专用客户端。

- [x] **单一 API 门面（backend/api）**
  - ✅ 新增 `api.Library`：封装读+写编排，桌面(Wails `app.go`) 与 局域网 HTTP (`server.go`) 共用同一份业务逻辑，杜绝行为漂移。
  - ✅ `RemoveVideo`（含清除组成员）、`VideosInGroup`（id→完整对象）等跨服务编排收敛到门面。
  - ✅ 平台专属（打开播放器/Explorer/剪贴板/文件选择器）与 Web 专属（Range 串流/静态 SPA）保留在对应层。
  - ✅ Wails 前端 bindings/方法签名不变，前端零改动。

## 功能优化点清单（2026-09-03 代码梳理）

> 来源：对照 `spec-v1.md` / `spec-v1.1.md` / 现有代码逐项核对后整理，按领域与优先级分组。完成一项即勾选并保留一行结论。

### 核心收藏体验

- [x] **最近播放视图**
  - ✅ 侧边栏「最近播放」：只列已播放收藏，按 1 天内访问过 / 3 天内 / 7 天内 / 14 天内 / 更早之前分组，组内按播放时间倒序；空组隐藏。
  - ✅ 排序栏新增「最近播放」（桌面、移动端、演员作品页）：降序最近在前，升序最早在前，从未播放的始终排在已播放之后。
  - ✅ 播放后即时更新：服务端写 `last_opened_at` 并发 `video:opened`；移动端把外部播放器/OpenList 的播放上报到 `POST /api/videos/{id}/opened`。
  - 源码：`frontend/src/pages/Library.tsx`、`components/Sidebar.tsx`、`components/RecentlyPlayedView.tsx`、`utils/recent.ts`、`utils/sortVideos.ts`、`backend/video/service.go`、`backend/server/server.go`。
- [ ] **搜索排序按相关度加权**
  - 现 `Search()`（`backend/video/repository.go` `Search`）为纯 `LIKE %kw%` + `ORDER BY created_at DESC`，与关键词相关度无关。
  - 建议：文件名命中 > 路径命中、前缀命中 > 包含命中，简单加权即可（数据量 1 万以内无需 FTS5）。
- [ ] **多选批量操作**
  - 当前网格/右键菜单全部为单条操作（打开/复制路径/移除/打标）。
  - 建议：网格多选后批量移除、批量加入分组、批量复制路径。
- [x] **多选批量操作（拖拽标记分组）**
  - ✅ 卡片左上角常驻复选框（悬停显示），点击勾选/取消，不触发打开。
  - ✅ 选中卡片高亮（accent 描边 + 微光），内容头出现「已选 N 个」工具栏（取消选择 / 新建分组）。
  - ✅ 拖拽选中的多张卡片到侧边栏分组：批量加入分组（MIME_VIDEO_DRAG_MULTI 携带 id 列表）。
  - ✅ 单卡片拖拽行为不变（MIME_VIDEO_DRAG）；拖拽幽灵图在多选时显示批量数量。
  - ✅ 交互模型改为点选驱动：单击卡片=勾选/取消（高亮），双击=打开；多选可连点或用复选框累积，批量操作不变。
  - 源码：`frontend/src/pages/Library.tsx`、`components/VideoCard.tsx`、`components/Sidebar.tsx`、`components/VideoGrid.tsx`、`types.ts`、`style.css`。
- [ ] **元数据 / 缩略图失败重试与可见性**
  - `Processor.process()`（`backend/video/processor.go`）中 ffprobe / ffmpeg 失败仅写日志，前端不可见且无重试。
  - 建议：带退避的重试 + 在卡片上暴露「处理中 / 缺元数据 / 缺缩略图」状态。
- [x] **收藏编辑（自定义标题 / 备注 / 自定义封面）**
  - ✅ 卡片右键菜单新增「编辑信息…」（`ContextMenu.tsx` → `EditVideoDialog.tsx`）：自定义标题（留空回退文件名）与备注，写 `videos.title / notes`（schema v11）。
  - ✅ 自定义封面：输入秒数（含 ±0.1s/±1s 微调），保存时由 ffmpeg 在指定时间点抽帧覆盖 `thumbnails/<id>.jpg`；不再在桌面 WebView 内嵌视频预览。
  - ✅ 封面标记 `videos.custom_cover_at` 保护：后台处理器不再覆盖自定义封面；「恢复默认封面」清标记并重新自动抽帧。
  - 源码：`EditVideoDialog.tsx`、`ContextMenu.tsx`、`Library.tsx`、`VideoCard/VideoList`、`thumbnail.go`（`ExtractAt`）、`video/service|repository|processor`。
- [x] **列表 / 网格视图切换**
  - ✅ 排序栏右侧新增「▦ 网格 / ☰ 列表」分段切换（`frontend/src/components/SortBar.tsx`），偏好经 localStorage（`muse.view`）跨会话记忆。
  - ✅ 新增列表视图（`frontend/src/components/VideoList.tsx`）：行式布局（复选框 + 小缩略图 + 文件名 + 元信息，悬浮 title 显示完整路径），点击打开 / 右键菜单 / 多选 / 拖拽（单选与多选批量）与网格完全一致。
  - ✅ 两种视图共享排序、选中集与全部交互（`Library.tsx` `videoListProps` 统一接线），切换即时生效。
- [ ] **键盘快捷键**（部分完成：卡片键盘导航已上线）
  - ✅ 已有：单选卡片后 ↑/↓/←/→ 移动选中（网格上下按整行跳转并自动滚动到可见）、Enter 打开、Del 弹出移除确认、Esc 清除选中（`Library.tsx` keydown，输入框/按钮聚焦或弹窗打开时不劫持）。
  - ✅ Ctrl/Cmd+F：从任意标签页回到影片库列表并聚焦顶部搜索框；演员索引、查重、刮削任务面板会回到「全部收藏」，全部收藏/分组/最近播放视图保持当前列表。已有输入内容会被全选，便于直接输入新关键词。
  - spec §40 其余仍缺：空格打开（当前空格是编辑信息）、以及更多全局快捷键。
- [x] **排序选项**
  - ✅ 列表顶部新增排序栏（`frontend/src/components/SortBar.tsx`）：添加时间 / 文件大小 / 文件名三个字段，点击切换字段、再点切换升降序，活动字段显示 ↑/↓。
  - ✅ 前端排序（`Library.tsx` `sortedVideos`），对全部/分组/搜索结果统一生效；默认「添加时间 降序」与后端 `created_at DESC, id DESC` 一致；文件名用 `Intl.Collator`（numeric，中文/英文与 "2"<"10" 均正确）。
  - ✅ 「最近播放」排序已支持（桌面端与移动端）；仍可选：按时长排序。

### 局域网服务

- [ ] **写端点鉴权 / 默认不开放写**（优先级提升）
  - `backend/server/server.go` 暴露了全部写端点（`DELETE /api/videos/{id}`、`PUT /api/videos/{id}/info`、`POST /api/videos/{id}/likes` 等）。同网段任何设备可直接调用，包括删除收藏。
  - 移动端重建后已把这些写操作**变成了用户可见的 UI 入口**（详情页点赞/编辑、卡片菜单移除），暴露面比此前「无 UI 按钮」时更大，因此建议优先处理。
  - 建议：默认关闭写端点或加简单口令（读与串流保持开放即可满足浏览需求）。
- [x] **移动端外部播放器跳转（Infuse / VLC）**
  - ✅ `GET /stream/video/{id}` 始终返回原始文件并支持 Range；删除服务端 remux、转码、状态接口和本地 `/media` 预览服务。
  - ✅ iOS 首次播放选择默认客户端，之后自动尝试；无法确认启动时回退到选择面板，可重试、切换或复制播放地址。
  - ✅ ffprobe 仍记录并展示 `video_codec` / `audio_codec`，但不再参与服务端播放决策。
- [ ] **网易爆米花直跳**
  - 官方目前只提供添加 WebDAV/Emby 数据源的 Universal Link，没有公开单视频播放协议；若后续提供可用深链，再按播放器适配层接入。
- [ ] **ServerBar 停止 5 秒轮询**
  - `frontend/src/components/ServerBar.tsx` 每 5 秒 `serverStatus()` 轮询一次；服务器启停已有明确按钮，正常使用无需定时轮询（可改为事件或启动时单次拉取）。

### 工程与技术债

- [ ] **`VideosInGroup` 全量拉表再内存过滤**
  - `backend/api/library.go` `VideosInGroup` 先 `videos.List()` 全量再 map 过滤；应改为仓库层 SQL JOIN 直接取完整对象。
- [ ] **前端 `reload()` 全量重查**
  - `frontend/src/pages/Library.tsx` 大多数 handler 都 `loadGroups()` + `load()` 双重全量 fetch（`handleSaveGroups` / `handleDropVideo` / `confirmRemove` 等）；可引入轻量缓存 / 增量更新。
- [ ] **设置页提供「打开日志 / 数据目录」入口**
  - 排查问题（防火墙、刮削失败）时用户只能去文件管理器找应用数据目录下的 `logs/app.log`。

### 路线图预留（spec 已列）

- [x] 文件夹导入 / Folder Watch（spec §40 V1.3）
  - ✅ 设置 → 目录监控：目录清单管理（添加 / 移除 / 立即扫描），目录经 Wails 原生选择器选取；移除仅停止监控，不删除已收藏视频。
  - ✅ 后端 `backend/watch` 包：`watched_dirs` 表（schema v10）、repository/service、fsnotify 递归监控；经 `api.Library` 门面暴露（`ListWatchDirs` / `AddWatchDir` / `RemoveWatchDir` / `RescanWatchDirs`）。
  - ✅ 递归自动导入：添加目录即全量扫描（跳过隐藏目录、`$RECYCLE.BIN`、`System Volume Information`）；之后新文件由 fsnotify 事件触发（1.5s 防抖合并复制风暴，新建子目录自动纳入监控），另有 5 分钟周期兜底重扫。导入复用 `AddVideos`（路径去重），并自动进入缩略图 / 4K 分组 / 刮削流水线。
  - ✅ 前端监听 `watch:scanned` 事件：自动刷新网格，并 toast 提示导入 / 失败数量。
  - 测试：`backend/watch/repository_test.go`、`service_test.go`（含真实 fsnotify 实时导入回路测试）。
- [ ] Explorer 右键菜单「Add to Favorites」（spec §40 V1.4；架构已预留，直接调 `AddVideos(paths)` 即可）
- [ ] 标签 / 备注 / 评分（spec §41 架构预留）
- [ ] **文件移动检测 / Hash 身份**（spec §40 V2；设计：`docs/spec/file-relocation.md`）
  - ✅ Hash 身份（入库重连）：新入库文件的部分哈希与某条「失踪」收藏一致时，自动把该收藏的路径引用改到新文件，id 与分组 / 点赞 / 刮削数据 / 标题备注全部保留；文件回到原路径时也复活原记录（不再报「已添加」）。源码 `backend/video/relocate.go`、`repository.go`（`RelinkPath` / `ListRelocationCandidates`）；测试 `backend/video/relocate_test.go`；添加汇总与 `watch:scanned` 区分「新增 / 重连」，重连结果不重复自动分组。
  - [ ] 文件移动检测（主动发现）：缺失状态目前仍只在启动与打开前检查（spec-v1.md §20），运行中移动文件后要等一次对账才会显示缺失；可考虑 fsnotify 双向对账 + 定期校验，与上方「失效视频自动检测」一并做。
- [ ] 高级搜索（spec §40 V2）
