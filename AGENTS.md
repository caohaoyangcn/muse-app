# 仓库指南

Muse Video 是一个面向 Windows 本地视频的轻量收藏管理器，基于 Wails v2 构建（Go 后端 + React 19/TypeScript 前端 + SQLite）。核心原则：**收藏 ≠ 文件** —— 数据库只保存引用，绝不复制、移动或删除源视频。

## 项目结构与模块组织

- `main.go`、`app.go` —— 程序入口与 Wails 绑定 API（`ListVideos`、`AddVideos`、`OpenVideo` 等）
- `backend/api/` —— 唯一的业务门面 `Library`，桌面端与局域网 HTTP 服务共用
- `backend/database/` —— SQLite 打开与 schema 迁移
- `backend/video/`、`backend/group/`、`backend/jav/`、`backend/watch/`、`backend/thumbnail/`、`backend/settings/` —— 各领域模型的 model/repository/service 代码，测试（`*_test.go`）与源码同目录
- `backend/server/` —— 局域网 Web 服务（SPA + JSON API + Range 串流）；`webui/` 内嵌静态资源
- `frontend/src/` —— 两种客户端共用一份源码：`ui/` 是跨平台基元层（`Button`、`Input`、`Dialog` 等），`components/` 是桌面专属业务组件，`mobile/` 是局域网移动壳；`services/api.ts` 是 Wails 绑定的唯一封装层，`types.ts`、`utils/`、`hooks/` 为共享代码；`frontend/wailsjs/` 为生成产物，勿手改
- `docs/spec/` 与根目录 `spec-v1.md` / `spec-v1.1.md` —— 设计文档

## 构建、测试与开发命令

环境要求：Go 1.25+、Node 22+、Wails v2 CLI、ffmpeg/ffprobe 在 PATH 中。

命令行工具约定：优先使用 Git Bash 执行命令；只有在 Git Bash 无法完成任务时才使用 PowerShell，并说明原因。

- `wails dev` —— 热重载开发模式；自动重新生成 `frontend/wailsjs/` 绑定
- `wails build` —— 产出 `build/bin/Muse.exe`
- `scripts/package-windows.sh` —— 交叉编译并打包 Windows 分发 zip（含 FFmpeg、首次运行说明），产出 `dist/Muse-<版本>-win64.zip`；细节见 `docs/distribution-windows.md`
- `go test ./...` —— 后端单元测试（默认不跑集成测试）
- 每次修改代码后必须运行对应范围的静态检查；修改 Go 代码必须运行 `golangci-lint run ./...`，修改前端代码必须运行 `npm run lint` 和 `npm run build`（均在 `frontend/` 下），并在提交前处理所有静态检查问题
- 每完成一个边界清晰且可验证的模块，必须立即运行该模块的针对性验证并单独提交；提交应保持单一职责，不要把多个已完成模块攒成一个提交
- `go test -tags integration ./...` —— 集成测试（真实 ffmpeg，显式指定才运行）
- `npm run build`（在 `frontend/` 下）—— `tsc` 类型检查 + Vite 构建
- `npm run lint`（在 `frontend/` 下）—— ESLint 检查 `src/` 中的 TypeScript、React Hooks 与 Fast Refresh 规则，随后跑 `scripts/check-shared-ui.mjs` 校验跨平台目录规则（`ui/` 不依赖业务、`mobile/` 不 import 桌面组件、两端各引自己的传输层、每个共享视图模型两端都被引用、裸 `<button>` 不新增）；lint 必须无错误、无警告

## 编码风格与命名约定

- Go：遵循 `gofmt` 格式与 `go vet` 规范；包名为单个小写单词（`video`、`group`）；文件用 snake_case（`repository.go`）
- 前端：组件用 PascalCase（`VideoCard.tsx`），变量/处理器用 camelCase；构建时由 `tsc` 强制类型检查

## 测试指南

- 后端：使用 Go `testing`，按行为编写表驱动测试，共享辅助代码放在 `testhelpers_test.go`
- 集成测试（-tags integration）会使用真实 SQLite + ffmpeg，需要这些工具在 PATH 中
- 运行 `go test ./...`；覆盖率用 `go test ./... -cover`
- 前端尚未配置测试框架

## 提交与 Pull Request 规范

- 历史提交采用短小祈使句并带类型前缀：`Add …`、`Fix …`、`Refactor …`、`Support …`（例如 `Fix saving state stuck after assign`）
- 每个提交聚焦单一改动；默认使用简洁英文
- 仓库未配置 CI：PR 必须说明改了什么、为什么改，关联受影响的 spec/文档，并报告 `go test ./...` 结果

## 开发注意事项

- 新业务逻辑一律经由 `api.Library`，避免桌面端与 HTTP 行为漂移；平台专属能力保留在各自分层中
- 删除收藏只删除数据库记录与生成的缩略图——绝不触碰底层视频文件
- 跨平台基元一律放 `frontend/src/ui/`，桌面 `components/` 与移动 `mobile/` 都从这里取；不要在业务文件里手搭按钮、对话框、提示条。`ui/` 只依赖 React 与 `lib/utils`，不得 import 业务或平台模块
- `frontend/src/mobile/` 不得 import `frontend/src/components/`：需要两平台复用的实现下沉到 `ui/`（展示）或 `views/`（取数与状态）。共享改造的阶段与进度见 `docs/spec/cross-platform-ui-plan.md`
- 前端取数一律走 `useLiveQuery`（`frontend/src/hooks/useLiveQuery.ts`），不要自己写「只加载一次 + 接收 refreshToken」：写操作成功后调用一次 `invalidate()`（`frontend/src/services/live.ts`），所有已挂载视图会自行静默重读。桌面端后端事件只在 `services/events.ts` 统一订阅，视图不要 `EventsOn` 数据类事件
