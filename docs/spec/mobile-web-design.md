# 移动端 Web UI 设计

> 状态：已实现。适用于 `backend/server/webui/`（构建产物）与 `frontend/src/mobile/`（源码）。

## 1. 背景与目标

桌面端（Wails + React）是完整的收藏管理器，但它依赖 Wails 运行时（`window.go`）、原生文件选择器、系统播放器、剪贴板与拖拽，无法直接在手机浏览器中运行。局域网服务因此需要一套独立的移动端界面。

移动端的目标是**浏览对齐桌面 + 轻编辑**：让用户在手机上完成"查看与播放"，以及少量低风险的数据编辑。重操作（刮削、封面选帧、分组结构维护）保留在桌面端。

## 2. 架构

```
桌面：Wails 绑定 ──┐
                   ├──► api.Library（唯一业务门面）
移动：fetch(/api) ─┘
```

- 移动端**不引入新的业务逻辑**。Go 侧仅把 `api.Library` 已有方法暴露为 HTTP 端点，校验与编排全部沿用门面（符合仓库约定：新业务逻辑一律经由 `api.Library`）。
- 移动端是独立的 React 应用（`frontend/src/mobile/`），通过第二份 Vite 配置构建，产物写入 `backend/server/webui/`，由 `//go:embed webui` 打进二进制。
- 局域网服务随应用启动自动监听固定端口（设置 → 服务器，默认 8899），手机网址无需再手动开启；端口被占用时只记警告，桌面功能照常可用，头部按钮仍可停止或重试启动。
- 桌面端与移动端共享 `frontend/src/types.ts`（纯类型，无运行时副作用）以及 `frontend/src/utils/`（格式化）。

### 为什么不把桌面端改成响应式

桌面端满屏是固定 200px 侧栏、右键菜单、hover 交互、HTML5 拖拽与键盘快捷键，且 `services/api.ts` 直连 Wails 绑定、没有 HTTP 抽象层。改造它需要同时新增传输抽象层与全站响应式重写，改动面远大于新建一个移动端应用，且会让两个平台的能力被迫耦合。

## 3. 构建

```bash
npm run build:mobile      # frontend/ 下
```

流程：`tsc` 类型检查 → `vite build --config vite.mobile.config.ts` → `scripts/finalize-mobile.mjs`。

- `outDir` 指向 `../backend/server/webui`，`emptyOutDir: true`。
- 输出**固定文件名** `assets/app.js` / `assets/app.css`。Go 静态服务已对全部资源设置 `Cache-Control: no-cache`，内容哈希没有收益，只会增加提交噪音。
- Vite 以入口文件名输出 HTML（`mobile.html`），`finalize-mobile.mjs` 将其重命名为 `index.html`，以匹配 `serveWebUI` 的 `/` 路由。
- 静态图片（`favicon.png`、`muse-logo.png`）位于 `frontend/mobile-public/assets/`，作为构建输入；若直接放在 `webui/` 会被 `emptyOutDir` 清空。
- `wails.json` 的 `frontend:build` 调用单一入口 `npm run build:all`，由该脚本串联 `npm run build && npm run build:mobile`；Wails 会按空格切分构建命令，因此不能直接把 `&&` 写进 `frontend:build`。
- 构建产物**入库提交**：`go:embed webui` 要求目录非空，且这样新克隆上 `go build` / `go test ./...` 无需先跑 npm。

## 4. 信息架构

底部四个标签页：

| 标签 | 内容 |
|---|---|
| 收藏 | 网格/列表、搜索、分组筛选（选中分组后可再按关键字筛组内）、排序 |
| 演员 | 演员索引（含合并别名与刮削到的头像），点入查看该演员作品（作品列表可按关键字筛选；演员页副标题带生日/身高三围等档案，见 `actor-profiles.md`） |
| 刮削 | 刮削任务看板（进行中 / 已完成 / 失败） |
| 查重 | 同一番号多个版本的成组列表；保留某一个版本或保留全部（见 `duplicate-detection.md`） |

视频详情与演员作品列表以**压栈屏幕**形式在标签内打开，带返回箭头；压栈期间隐藏底部标签栏，使返回成为唯一出口。导航由浏览器历史驱动，而不是内存中的 `Screen[]`：每次切换标签或打开屏幕都写入地址栏，返回箭头调用 `history.back()`，所以浏览器前进/后退、Android 返回手势与页内返回箭头走的是同一份历史。

路由定义在 `frontend/src/mobile/router.ts`：

| URL | 屏幕 |
|---|---|
| `/` | 收藏 |
| `/actors` | 演员索引 |
| `/scrape` | 刮削 |
| `/duplicates` | 查重 |
| `/videos/<id>` | 视频详情 |
| `/actor?name=<演员名>` | 演员作品列表 |

- **扁平路由**：详情与演员作品不把父级标签编码进 URL。父级就是打开屏幕时浏览器记录的上一个条目，因此「收藏 → 详情 → 演员 → 详情」是四个历史条目，逐次返回即可还原；刷新后仍停留在当前屏幕（详情按 id 重新拉取收藏）。没有应用内历史时（直接打开深链，或刷新后处于首个条目），页内返回箭头回到该屏幕的自然根：详情 → 收藏，演员作品 → 演员索引。
- **演员名走 query**：与 §6 的 API 原因相同——演员名是任意字符串（CJK、标点、甚至 `/`），作为路径段会被浏览器按 `/` 切开。`/actor?name=` 由 `URLSearchParams` 解码，`/` 不会损坏路由。
- **Sheet 也是历史条目**：打开动作面板 / 编辑 / 播放器 / 确认框时压入一个同 URL 的条目，Android 返回或浏览器后退先关面板；从面板直接跳到详情时复用该条目，避免多出一次空返回。
- **滚动位置按历史条目保存**：每条历史记录带一个 `museKey`，离开时记录带 `data-scroll-key` 的主滚动面板偏移，返回该条目时恢复（`frontend/src/mobile/scrollMemory.ts`）。滚动发生在应用内的面板而不是文档上，所以关闭浏览器原生的 `history.scrollRestoration`；恢复会等列表异步加载到足够高度后再落位，恢复尚未完成就再次离开时不会用 0 覆盖已保存的位置。
- **静态服务回退**：`serveWebUI` 对不存在的文件路径返回 `index.html`，所以直接打开或刷新上述任意 URL 不会 404。

## 5. 触屏交互

移动端不照搬桌面交互，而是为触屏提供原生原语：

| 桌面 | 移动端 |
|---|---|
| 右键上下文菜单 | 卡片「⋯」按钮 → 底部动作面板 |
| 悬浮 tooltip | 直接展示元信息，无悬浮态 |
| 侧栏分组树 | 「分组 ▾」按钮 → 底部选择面板（缩进子分组） |
| Radix 模态框 | 底部 sheet（`BottomSheet`）/ 居中确认框 |
| 快捷键 | 不提供；浏览器前进/后退、Android 返回手势与页内返回箭头共用 URL 历史 |
| HTML5 拖拽 | 不提供 |
| 框选（长按拖出选择框） | 不提供；两端均以复选框多选取代（见第 8 节） |
| 网格多列固定 | `repeat(auto-fill, minmax(150px,1fr))` 自适应（小/中/大预设分别取 110 / 150 / 210px） |

安全区适配：`viewport-fit=cover` + `env(safe-area-inset-*)` + `100dvh`；`overscroll-behavior: none` 避免页面级橡皮筋；输入框恢复 `user-select: text`（全局禁用选择以免长按选中 UI 文本）。

## 6. HTTP API

全部为 `api.Library` 方法的薄封装。除标注外，读端点返回 JSON。

**读**

| 端点 | 门面方法 | 备注 |
|---|---|---|
| `GET /api/videos` | `ListVideos` / `SearchVideos` | `?search=` 存在时走搜索 |
| `GET /api/videos/{id}` | `Video` | 不存在 → 404 |
| `GET /api/videos/{id}/jav` | `VideoJav` | 未刮削 → **200 + `null`** |
| `GET /api/videos/{id}/jav/task` | `VideoJavTask` | 未排过队 → 200 + `null` |
| `GET /api/groups` | `ListGroups` | |
| `GET /api/groups/{id}/videos` | `VideosInGroup` | |
| `GET /api/jav/tasks` | `ListJavTaskOverviews` | 刮削看板 |
| `GET /api/jav/duplicates` | `ListJavDuplicates` | 查重；`?includeIgnored=1` 显示已忽略 |
| `GET /api/actors` | `ListActors` | 含合并别名 |
| `GET /api/actors/videos?name=` | `VideosByActor` | 演员名走 query，见下 |

**写**

| 端点 | 门面方法 |
|---|---|
| `POST /api/videos` | `AddVideos` |
| `DELETE /api/videos/{id}` | `RemoveVideo` |
| `PUT /api/videos/{id}/info` | `UpdateVideoInfo` |
| `POST /api/videos/{id}/likes` | `LikeVideo` |
| `DELETE /api/videos/{id}/likes` | `ClearVideoLikes` |
| `POST /api/groups` / `PUT /api/groups/{id}` / `DELETE /api/groups/{id}` | `CreateGroup` / `RenameGroup` / `DeleteGroup` |
| `POST /api/groups/{id}/videos` | `AddVideoToGroup` |
| `POST /api/jav/duplicates/ignore` | `IgnoreJavDuplicate` |

**媒体**

| 端点 | 说明 |
|---|---|
| `GET /stream/video/{id}` | 原始文件 Range 串流；Muse 不解码、不重封装，内容由手机上的专用播放器处理 |
| `GET /thumbnails/{id}` | 自动缩略图 |
| `GET /covers/jav/{id}` | 刮削封面 |

### 设计要点

- **未刮削返回 `null` 而非 404**：桌面端把 `sql.ErrNoRows` 解释为"尚无数据"，移动端需要区分"未刮削"与"视频不存在"。因此 `/jav` 与 `/jav/task` 在无数据时返回 `200` + `null`，只有视频本身不存在才是 `404`。
- **演员名走 query 参数**：演员名是任意字符串（CJK、标点、甚至 `/`），作为路径段会被截断或需要额外转义，因此用 `?name=`。
- **写端点返回更新后的 `Video`**：与桌面端行为一致，移动端据此直接重绘，无需回查。
- **串流只负责传输**：Range 让原生播放器自行 seek，Muse 不读取视频帧、不 remux、不 transcode，也不保存播放进度。
- **播放交给专用客户端**：移动 Web 只负责生成完整 LAN URL，并通过 URL Scheme 启动 Infuse 或 VLC；编码、容器、字幕和音轨全部由客户端处理。
- **默认播放器**：首次点击播放打开选择面板，用户选择后写入 `localStorage`；后续优先自动尝试该客户端，2 秒内未观察到页面隐藏则回退到选择面板。
- **安装检测边界**：浏览器没有枚举已安装 App 的 API，因此不能宣称“已检测到客户端”；页面隐藏只表示很可能成功交给客户端，失败时仍提供重试、切换和复制地址。

## 7. 与桌面端的能力差异

| 能力 | 桌面 | 移动 |
|---|---|---|
| 浏览 / 搜索 / 分组筛选 / 排序 | ✅ | ✅ |
| 组内与演员作品关键字筛选 | ✅ | ✅ |
| 播放 | 系统默认播放器（编码无关） | iOS 上交给 Infuse/VLC；Muse 只传原始文件，客户端负责解码 |
| 详情（JAV 元数据 / 演员 / 标签 / 文件信息） | ✅ | ✅ 只读 |
| 标题 / 备注编辑 | ✅ | ✅ |
| 点赞 | ✅ | ✅ |
| 移除收藏 | ✅ | ✅ |
| 刮削任务查看 | ✅ | ✅ |
| 查重（保留某个版本 / 保留全部） | ✅ | ✅ |
| 发起刮削 / 覆盖番号 | ✅ | ❌ 桌面专属 |
| 演员合并 | ✅ | ❌ 桌面专属 |
| 封面选帧 | ✅ 输入时间后由 ffmpeg 抽帧，无内嵌视频预览 | ❌ 桌面专属 |
| 分组增删改 / 嵌套 / 拖拽排序 | ✅ | ❌ 桌面专属 |
| 打开系统播放器 / 定位文件 / 复制路径 | ✅ | ❌ 作用于服务器文件系统，对手机无意义 |
| 添加收藏 / 目录监控 / 设置 | ✅ | ❌ 移动端不渲染入口（写端点仍可达） |

## 8. 数据与刷新

- 拉全量 `GET /api/videos` + 客户端排序（与桌面端一致）。
- 分组视图走 `GET /api/groups/{id}/videos`；搜索始终跨全库。
- **两级关键字**：顶部「搜索」跨全库（走 `?search=`），分组视图与演员作品页各自另有一个输入框，只筛当前已加载的列表，纯客户端过滤、无防抖、不产生请求。两者相互独立、同时生效。筛选字段为 `title` / `fileName` / `path`（大小写不敏感子串，与后端 `SearchVideos` 的 LIKE 字段一致并加上自定义标题），因此本地筛选不会藏掉全局搜索刚命中的结果。切换分组会清空组内筛选；命中数不足时表头显示「匹配 N / 共 M」。
- 排序字段、网格/列表与图标尺寸（小/中/大）偏好存 `localStorage`（键 `muse.mobile.*`）。图标尺寸对两种模式同时生效：网格改变卡片最小宽度、列表改变行内缩略图宽度。
- **查重页不带筛选框**：桌面端因为顶部搜索框常驻，必须在该视图里自己提供筛选；移动端的顶部搜索框本来就只在「收藏」标签显示，查重页不会出现一个看似可用却无效的输入框，因此不需要再放一个。列表本就不长，「显示已忽略」开关已足够复查先前的决定。
- **行虚拟化**：收藏库可能上千条，卡片全量挂载会让每次重排（缩放窗口、改字号）代价随条数线性增长，拖拽缩放窗口时明显卡顿。两端共用 `frontend/src/hooks/useVirtualRows.ts`，只挂载视口（含 600px 预载带）附近的行，其余高度由外层占位元素支撑；列数、卡高与间距均从 DOM 实测量取（桌面根字号 14px、移动端 16px，类名推不出像素）。条数低于 100 时仍全量渲染。因此框选被移除：它逐卡片读 `getBoundingClientRect()`，与只挂载可见行的做法冲突。
- **无推送**：桌面端依赖 Wails 事件（如 `jav:scrape:updated`）实时刷新，HTTP 无此能力，移动端在打开详情时拉取。这是有意的简化。
- **只有屏幕进 URL**：标签、详情 id 与演员名写入地址栏；搜索关键字、分组筛选、排序与网格/列表偏好仍留在组件状态或 `localStorage`。搜索是逐字符输入，若每敲一个字都压入历史，前进/后退会被输入过程淹没，也会让返回一次只退一个字符。

## 9. 已知限制

- **写端点无鉴权**：同网段任何设备都能调用写端点（含 `DELETE /api/videos/{id}`）。这是既有缺口（见 `TODO.md`），移动端新增的移除入口扩大了暴露面，应优先补口令。
- **仅支持 iOS 外部播放器**：首版支持 iPhone/iPad 上的 Infuse、VLC；Android、PC 浏览器不执行自动跳转，只提供复制地址。
- **无法真正检测安装状态**：iOS 不允许网页枚举已安装 App；未安装、用户取消系统提示或启动过慢时，都会进入回退选择面板。
- **网易爆米花暂不支持直跳**：官方公开的是添加 WebDAV/Emby 数据源的 Universal Link，没有单视频 URL 播放协议，因此不作为播放器入口。
- **旧重封装缓存不自动清理**：新版本不再创建或使用 `<数据目录>/web/`，升级后该目录可由用户安全删除。
- **两条 UI 代码路径**：移动端与桌面端各自维护，长期存在漂移风险；能力差异以本文档为准。
