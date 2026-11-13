# Muse Video 开发文档

面向贡献者与二次开发。用户安装与使用说明见 [README](../README.md)。

Muse 是一个面向 Windows / macOS 的本地视频收藏管理器，核心原则是**收藏 ≠ 文件**：数据库只保存引用，绝不复制、移动或删除源视频。

## 技术栈

Wails v2.15 · Go 1.25 · React 19 + TypeScript + Vite · SQLite（`modernc.org/sqlite`，纯 Go 免 CGO）· FFmpeg / ffprobe

## 环境要求

- Go 1.25+
- Node 22+
- Wails CLI v2.15.0
- `ffmpeg` / `ffprobe` 在 `PATH` 中，或与可执行文件同目录
- Windows 10/11 或 macOS 13+（Apple Silicon）
- 打包脚本需要 Git Bash；Windows 上优先使用 Git Bash，只有无法完成时才用 PowerShell

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.15.0
```

确保 `$(go env GOPATH)/bin` 在 `PATH` 中。

## 常用命令

```bash
wails dev                          # 热重载开发模式，自动重新生成 frontend/wailsjs/ 绑定
wails build                        # 产出 build/bin/Muse.exe
go test ./...                      # 单元测试（默认不跑集成测试）
go test -tags integration ./...    # 集成测试（真实 ffmpeg + SQLite，显式指定才运行）
golangci-lint run ./...            # Go 静态检查
```

前端命令均在 `frontend/` 下执行：

```bash
npm install
npm run dev             # Vite 开发服务器
npm run build           # tsc 类型检查 + 桌面端构建
npm run build:mobile    # 构建移动端，产物写入 backend/server/webui/
npm run lint            # ESLint + 跨平台目录规则检查（必须无错误、无警告）
npm run gaps            # 列出移动端相对桌面端还缺哪些能力（读 src/views/manifest.json）
```

`wails build` 的 `frontend:build` 配置为 `npm run build:all`，会同时构建桌面端与移动端；`frontend/wailsjs/` 是生成产物，不要手改。

根包通过 `go:embed` 内嵌 `frontend/dist`，全新检出的仓库需要先构建前端，`go test ./...` 才能通过：

```bash
cd frontend
npm ci
npm run build:all
```

## 测试与静态检查

- 后端使用 Go `testing`，按行为编写表驱动测试；共享辅助代码放在 `testhelpers_test.go`。
- 集成测试使用真实 SQLite 与 ffmpeg，需要这些工具在 `PATH` 中。
- 修改 Go 代码必须运行 `golangci-lint run ./...`。
- 修改前端代码必须运行 `npm run lint` 与 `npm run build`，提交前处理所有问题。
- 每完成一个边界清晰且可验证的模块，立即运行针对性验证并单独提交，不要把多个模块攒成一个提交。
- 覆盖率：`go test ./... -cover`。
- 仓库目前没有 PR CI，只有打 tag 时的 Release 流水线；合并前请自行确认上述检查通过。

## 架构

桌面 App（Wails 绑定）与局域网 HTTP 服务共用同一个 `api.Library` 门面，业务逻辑（校验、编排、事务）只有一份：

```text
api.Library ──► video.Service / group.Service ──► SQLite
  ▲                    ▲
  │                    │
  ├─ app.go (Wails)    平台专属操作（打开播放器/文件管理器/剪贴板/文件选择器）
  └─ server.go (HTTP)  Web 专属能力（Range 串流/静态 SPA/图片 URL）
```

开发时遵守以下边界：

- 新业务逻辑一律经由 `api.Library`，避免桌面端与 HTTP 行为漂移。
- 平台专属能力保留在各自分层（`backend/platform/`、`backend/server/`）。
- 跨平台基元放在 `frontend/src/ui/`，桌面 `components/` 与移动 `mobile/` 都从这里取；`ui/` 只依赖 React 与 `lib/utils`。
- `frontend/src/mobile/` 不得 import `frontend/src/components/`；需要复用的实现下沉到 `ui/`（展示）或 `views/`（取数与状态）。阶段与进度见 `docs/spec/cross-platform-ui-plan.md`。
- 移除收藏只把记录移入回收站；彻底清理只删除数据库记录与生成的图片，绝不触碰底层视频文件。
- 前端取数一律走 `useLiveQuery`（`frontend/src/hooks/useLiveQuery.ts`）；写操作成功后调用一次 `invalidate()`（`frontend/src/services/live.ts`），所有已挂载视图会自行静默重读。
- 桌面端数据类后端事件只在 `services/events.ts` 统一订阅，视图不要直接 `EventsOn`。

## 目录结构

```text
backend/
├── api/        # 单一业务门面（Library）：读写编排，桌面(Wails)与 HTTP 共用
├── database/   # SQLite 打开 + schema 迁移
├── video/      # 模型 / 仓库 / 服务 / ffprobe / 后台处理队列 / 指纹与文件重连
├── group/      # 分组（分组树、成员关系）
├── jav/        # 刮削：JavBus / Avmoo / FC2CMADB provider、任务队列、封面与头像
├── watch/      # 目录监控：递归导入 + fsnotify 事件
├── thumbnail/  # ffmpeg 帧提取
├── server/     # 局域网 Web 服务：移动端 SPA + JSON API + Range 串流（webui/ 内嵌）
└── platform/   # Windows/macOS shell 集成（默认播放器 / 文件管理器 / 工具查找）

frontend/src/
├── pages/Library.tsx        # 桌面端主页面（列表/搜索/事件/拖拽）
├── pages/Settings.tsx       # 设置：新手引导、服务器、目录监控、代理、刮削站点
├── components/              # 桌面端组件：VideoCard / Grid / ContextMenu / 弹窗
├── mobile/                  # 移动端 React 应用（局域网 Web UI）
│   ├── App.tsx              # 底部标签栏 + 压栈式导航
│   ├── api.ts               # fetch 传输层（Wails 绑定的移动端替代）
│   └── components/          # 触屏组件
├── services/api.ts          # 桌面端 Wails 绑定的唯一封装层
├── services/events.ts       # 桌面端后端事件订阅
└── types.ts                 # 由 wailsjs 生成的模型转发（两端共用）

app.go        # 绑定到前端的 API（ListVideos / AddVideos / OpenVideo / ...）
main.go       # 启动、日志、缩略图 AssetServer Handler、拖拽配置
```

## 数据目录与迁移

```text
Windows: %LOCALAPPDATA%\Muse\
macOS:   ~/Library/Application Support/Muse/

├── app.db          # SQLite 收藏库
├── thumbnails\     # 缩略图（thumbnails/<id>.jpg）
├── covers\jav\     # 刮削封面（covers/jav/<id>.jpg）
├── avatars\        # 演员头像（avatars/<star_id>.jpg）
└── logs\app.log    # 运行日志（超过 10MB 启动时自动清空）
```

- 数据目录与程序目录无关；升级时覆盖程序文件夹即可，schema 迁移在启动时自动执行（`backend/database/migration.go`）。
- 旧版本生成的 `web/` 重封装缓存已不再使用，可安全手动删除。
- 图片清理逻辑只删除孤立的生成图片，不会触碰源视频。

## 移动端 Web UI

移动端是**独立的 React 应用**（`frontend/src/mobile/`），不是桌面端的响应式变体：桌面端依赖 Wails 运行时与原生能力，无法直接在手机浏览器运行。移动端通过 HTTP 调用同一 `api.Library` 门面。

```bash
cd frontend
npm run build:mobile   # 产物写入 backend/server/webui/（入库，go:embed 依赖）
```

移动端刻意不做：发起刮削、封面选帧、演员合并、分组增删改与拖拽排序、添加收藏、目录监控与设置。这些依赖原生文件对话框、ffmpeg 交互或服务器文件系统。

播放由客户端完成：`GET /stream/video/{id}` 始终以 HTTP Range 传输原始文件，Muse 不做服务端转码或重封装；iPhone/iPad 可跳转 Infuse / VLC，其他设备复制播放地址。

信息架构、触屏交互、端点清单与能力差异见 [`mobile-web-design.md`](spec/mobile-web-design.md)。

## Web API（局域网）

读写端点均为 `api.Library` 门面的薄封装，桌面端与移动端共用同一份业务逻辑。

**读**

- `GET /api/videos`（支持 `?search=`）、`GET /api/videos/{id}`
- `GET /api/videos/{id}/jav`、`GET /api/videos/{id}/jav/task`（未刮削返回 `200` + `null`）
- `GET /api/groups`、`GET /api/groups/{id}/videos`
- `GET /api/jav/tasks`（刮削任务看板）、`GET /api/jav/duplicates`（查重，`?includeIgnored=1` 显示已忽略）、`GET /api/actors`、`GET /api/actors/videos?name=`
- `GET /api/duplicates/content`（内容重复，`?includeIgnored=1` 显示已忽略）

**写**

- `POST /api/videos`、`DELETE /api/videos/{id}`
- `PUT /api/videos/{id}/info`（标题/备注）、`POST|DELETE /api/videos/{id}/likes`
- `POST /api/groups`、`PUT|DELETE /api/groups/{id}`、`POST /api/groups/{id}/videos`
- `POST /api/jav/duplicates/ignore`（保留重复分组全部版本）
- `POST /api/duplicates/content/ignore`、`POST /api/duplicates/content/confirm`

**媒体**

- `GET /stream/video/{id}`（Range；始终返回原始文件，由专用播放器客户端解码）
- `GET /thumbnails/{id}`、`GET /covers/jav/{id}`、`GET /avatars/{name}`

> ⚠️ 写端点**无鉴权**，同网段任何设备可调用（见 `TODO.md`）。

## 构建 macOS 版

```bash
wails build --clean --platform darwin/arm64
open build/bin/Muse.app
```

前提：Apple Silicon Mac，通过 Homebrew 安装 `ffmpeg`、`ffprobe`。构建后可将 `Muse.app` 复制到 `/Applications`。工具查找顺序为 **`/opt/homebrew/bin` → App 资源目录 → PATH**。

## Windows 打包与发布

发给 Windows 用户的应该是**一个文件夹**（`Muse.exe` + `ffmpeg.exe` / `ffprobe.exe` + 说明），而不是单个 exe——应用只按 **exe 同目录 → PATH** 找 FFmpeg。

```bash
scripts/package-windows.sh        # → dist/Muse-<版本>-win64.zip（约 70 MB，含 FFmpeg）
```

脚本会交叉编译 Windows x64、下载并缓存 LGPL 版 FFmpeg、写入首次运行说明、打包并打印 sha256。版本号只改 `wails.json` 的 `info.productVersion` 一处。

推一个 `v*` tag 或在 Actions 页面手动触发，都会由 GitHub Actions（[`.github/workflows/release.yml`](../.github/workflows/release.yml)）跑同一个脚本，并创建/更新对应的 Release：

- 手动触发按 `wails.json` 的 `info.productVersion` 发布 `v<版本>`；tag 不存在时自动创建在当前提交上。
- 推 tag 时会校验 tag 与 `wails.json` 版本一致，不一致直接失败。
- artifact 只保留 1 天；正式产物以 Release 附件为准。

SmartScreen、杀软误报、防火墙、`shared` / `static` 两种 FFmpeg 打包方式、可选 NSIS 安装包等细节，见 [Windows 分发文档](distribution-windows.md)。

## 设计文档

- [`spec-v1.md`](../spec-v1.md)：V1 完整技术设计
- [`spec-v1.1.md`](../spec-v1.1.md)：Drag & Drop 批量添加增量
- [`mobile-web-design.md`](spec/mobile-web-design.md)：移动端 Web UI
- [`duplicate-detection.md`](spec/duplicate-detection.md)：查重（同番号多版本）
- [`content-hash-dedup.md`](spec/content-hash-dedup.md)：内容指纹与重复检测
- [`file-relocation.md`](spec/file-relocation.md)：文件移动重连
- [`actor-profiles.md`](spec/actor-profiles.md)：演员档案
- [`guided-tour.md`](spec/guided-tour.md)：新手引导
- [`volume-offline.md`](spec/volume-offline.md)：卷离线与缺失文件
- [`generated-images.md`](spec/generated-images.md)：生成图片的生命周期

## 提交约定

- 历史提交采用短小祈使句并带类型前缀：`Add …`、`Fix …`、`Refactor …`、`Support …`（例如 `Fix saving state stuck after assign`）。
- 每个提交聚焦单一改动；默认使用简洁英文。
- PR 需要说明改了什么、为什么改，关联受影响的 spec/文档，并报告 `go test ./...` 结果。
- 仓库未配置 PR CI；合并前请自行确认测试、lint 与构建通过。

