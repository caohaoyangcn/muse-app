# 从代码到 Figma：固化与回流手册

目标：把 Muse Video 当前的设计**变成 Figma 里可编辑、可追踪的资产**，然后在这份资产上打磨 UI，改动只从一条路回流到代码。

## 0. 现状（2026-11-26）

**已完成并验证的：**

- 应用在浏览器里以真实组件渲染（dev-only 的 mock 传输层，未改动任何生产逻辑）：[preview.tsx](../../frontend/src/design/preview.tsx)、[mock/](../../frontend/src/design/mock/)、[design.preview.config.ts](../../frontend/design.preview.config.ts)。
- 26 张真实界面截图 + 1 张组件基元长图：[screenshots/](screenshots/)，每次运行 `capture-design.mjs` 可重拍。
- token 的机器可读镜像（可直接进 Figma Variables）：[figma-tokens.json](figma-tokens.json)。
- 界面清单与源码映射：[screen-inventory.md](screen-inventory.md)。

**还没做的，别当成已完成：**

- **没有任何 Figma 文件被创建或写入。** 本会话没有 Figma MCP 服务器、没有 Figma API token、没有打开 Figma 桌面端，因此「导入 Figma」这一步只能由你在本地执行，或用下文的 MCP 路线交给有 Figma 权限的 agent。
- 截图是**视觉参照**，不是可编辑图层。谁拿截图当设计稿交付，就等于交付了一张图。

## 1. 三条路线，按你的工具条件选

| 路线 | 前提 | 产出 | 适合 |
|---|---|---|---|
| **A. html.to.design 插件**（推荐先试） | Figma 桌面/网页版 + 插件；本机能跑 `npm run dev:design` | 真实文本层、Auto Layout、Styles、**Variables** | 想一次性铺完 20+ 屏，再整理组件 |
| **B. Figma MCP write-to-canvas** | Figma 付费席位（Full seat）+ 远端 MCP + 有 `use_figma` 的客户端 | 由 agent 按清单直接生成 frame / 组件 / 变量，命名可控 | 想让「清单 → 画布」可复现，且接受 beta |
| **C. 手工重建** | 只要 Figma | 最干净的结构，但工时最高 | 只打磨 3–5 屏，或 A/B 的效果都不能接受 |

无论哪条路线，**顺序都是**：先导 [组件基元页](screenshots/20-design-board.png) 建组件库，再导页面、用组件替换实例。反过来做会得到一屏一屏的重复图层。

### 路线 A：html.to.design

1. `cd frontend && npm run dev:design`，浏览器打开
   `http://localhost:5199/src/design/preview.html`（桌面）与 `.../preview-mobile.html`（移动）。
2. 确认 `--ui-zoom` 是 1（预览入口已经强制为 1，界面里不要按 `+`/`-`）。
3. Figma 里运行 html.to.design，选 **Web** 页签，粘贴上面的 URL：
   - 视口：桌面填 `1440×900`（再用 `1100×720` 导一档对照最小可用宽度），移动填 `390×844`。
   - 主题：Dark。语言：zh-CN。
   - 导入选项全部打开：**Auto Layout**、**Styles**、**Components**、**Variables**。只开 Auto Layout 不开 Styles，你会得到一堆硬编码颜色。
4. 需要「点开之后才有」的状态（详情页、右键菜单、筛选面板、编辑对话框、多选、抽屉），浏览器插件模式（Browser Extension）里先点到那个状态再抓；插件模式抓的是当前 DOM。
5. 顺序：`20-design-board` → `01..19` → `21..25` → `26`。
6. 每屏导入后立刻做的事：删除纯装饰的包裹组、把重复卡片转成组件实例、按第 3 节改名。

### 路线 B：Figma MCP（write-to-canvas）

前提：Figma 桌面端打开目标文件、MCP 已连接、客户端里有 `use_figma`。把下面这段连同 `screen-inventory.md` 一起给 agent：

```
把 docs/design/screen-inventory.md 里的界面按顺序建成 Figma frames。
约束：
1. 先建 Variables（docs/design/figma-tokens.json 的 color/radius/font/space/shadow），
   Collection 名「Muse / Dark」，mode = Dark。
2. 再建组件库页：AppShell / Sidebar / Topbar / MediaToolbar / MediaCard /
   MediaPoster / MediaMetadata / FilterChip / Dialog / Dropdown / ListRow / Pagination。
   组件名必须和 docs/design/figma-handoff.md 第 3 节的映射表一致。
3. 再建页面 frames，1440×900，用组件实例搭建，不允许 flatten 组。
4. 每一屏旁放一张对应截图（docs/design/screenshots/）作为比对参照，标注来源文件名。
5. 完成后输出：屏数、组件数、变量数、与截图的已知差异清单。
```

### 路线 C：手工重建

用截图做位置与间距参考，直接摆 [board 页](screenshots/20-design-board.png) 里的基元。先只做「01 网格 / 03 详情 / 15 设置」三屏，验证命名与组件规范，再铺其余。

## 2. 导入参数（三条路线通用）

| 参数 | 值 | 原因 |
|---|---|---|
| 桌面视口 | 1440×900，另加 1100×720 | 1100×720 是 `main.go` 的默认窗口；960×600 是最小尺寸，顶栏会换行 |
| 移动视口 | 390×844 | 移动壳按此宽度设计 |
| 缩放 | 100%（`--ui-zoom: 1`） | CSS `zoom` 会改变所有元素的实际尺寸，非 1 时导入会整体放大/缩小 |
| 字体 | **Sarasa UI SC** | 代码字体栈是 `"Sarasa UI SC", system-ui, sans-serif`，字体随包分发（`frontend/src/assets/fonts/`），Penpot 团队字体库同名；三端同字，不必再为字体回落做位置补偿 |
| 主题 | Dark | 应用只有暗色主题（`color-scheme: dark`） |

## 3. 图层与组件命名（Figma ↔ 代码一张表）

命名规则：`区域 / 组件 / 变体`，用 `/` 分层，英文 PascalCase，和 `ui/` 的模块名对齐。**改这张表就等于改约定**，先改表再改画布。

| Figma 组件 | 变体（建议） | 代码来源 | 说明 |
|---|---|---|---|
| `AppShell` | 桌面 / 移动 | [pages/Library.tsx](../../frontend/src/pages/Library.tsx)、[mobile/App.tsx](../../frontend/src/mobile/App.tsx) | 顶栏 + 侧栏/抽屉 + 内容区 |
| `Topbar` | 默认 / 详情态 / 设置态 | Library 的 `<header>` | 含 Logo、前进后退、`+ 添加视频`、局域网状态、搜索、`?`、`⚙ 设置` |
| `Sidebar` | 展开 | [ui/sidebar.tsx](../../frontend/src/ui/sidebar.tsx)、[components/Sidebar.tsx](../../frontend/src/components/Sidebar.tsx) | 三个分区：浏览 / 标签 / 工具 |
| `SidebarRow` | 默认 / 选中 / 悬停 | 同上 | 选中 = accent 12% 底 + 左侧 2px 竖条 |
| `MediaToolbar` | 全部收藏 / 详情 / 筛选已选 / 任务 | Library 工具栏 | 标题 + 计数 + 筛选/检测 + 排序 chip + 卡片大小 + 视图切换 |
| `MediaCard` | 网格（默认/选中/缺失/磁盘离线/远端）/ 列表行 | [components/VideoCard.tsx](../../frontend/src/components/VideoCard.tsx)、[VideoList.tsx](../../frontend/src/components/VideoList.tsx) | 封面 16:9、标题、`时长 · 分辨率 · 大小`、点赞数 |
| `MediaPoster` | landscape（封面+封底）/ portrait（右裁封面） | [ui/cover.tsx](../../frontend/src/ui/cover.tsx) | 两种裁切是产品决策，不要合并成一个 |
| `MediaMetadata` | 两列 | `MetadataList` / `MetadataListItem`（[ui/astryx.tsx](../../frontend/src/ui/astryx.tsx)） | 12px 键 + 13px 值，键在值的上方 |
| `FilterChip` | 未选 / 已选 / 带图标 | `toggleChipClass`（[ui/astryx.tsx](../../frontend/src/ui/astryx.tsx)）、[views/library/FilterChips.tsx](../../frontend/src/views/library/FilterChips.tsx) | 胶囊、描边、已选实心 accent |
| `Badge` | neutral / success / warning / error / purple / 紧凑 | `Badge` + `compactBadgeClass` | 紧凑版高度 16px，不改变行高 |
| `Button` | primary / secondary / ghost / destructive × sm/md/lg | [ui/button.tsx](../../frontend/src/ui/button.tsx) | 代码里 `default` 映射到 Astryx `secondary` |
| `IconButton` | 默认 / 选中 | [ui/button.tsx](../../frontend/src/ui/button.tsx) | 纯图标必须有 tooltip / aria-label |
| `Input` | 空 / 已填 / 带标签 / 数字 | [ui/input.tsx](../../frontend/src/ui/input.tsx) | 高度 32，聚焦描边 accent |
| `Dialog` | 表单（编辑信息）/ 确认（移入回收站） | [ui/dialog.tsx](../../frontend/src/ui/dialog.tsx) | 头部标题 + 可选批量操作 + 底部按钮；宽度按内容 |
| `Dropdown` | 卡片右键 / 标签右键 | [ui/menu.tsx](../../frontend/src/ui/menu.tsx)、[ContextMenu.tsx](../../frontend/src/components/ContextMenu.tsx) | 6 项，末项 danger |
| `Drawer` | 侧滑（移动导航）/ 底部（播放器选项） | [ui/drawer.tsx](../../frontend/src/ui/drawer.tsx)、[ui/sheet.tsx](../../frontend/src/ui/sheet.tsx) | 移动端 |
| `Pagination` | 多页 / 单页隐藏 | [ui/pagination.tsx](../../frontend/src/ui/pagination.tsx) | 首尾 + 当前页前后 5 页，`…` 表示跳空 |
| `Toast` / `ReportBanner` | success / info / error | [ui/toast.tsx](../../frontend/src/ui/toast.tsx)、[ui/report.tsx](../../frontend/src/ui/report.tsx) | 短确认走 Toast，长结果走 Banner |
| `EmptyState` | 无数据 / 无结果 / 未播放 | `EmptyState` | 标题 + 说明 + 可选主按钮 |
| `WorkspaceTabs` | 库 / 影片 / 演员 | [components/WorkspaceTabs.tsx](../../frontend/src/components/WorkspaceTabs.tsx) | 扁平标签，组色用前置圆点 |

## 4. Figma 里的 Design System

与 [figma-tokens.json](figma-tokens.json) 一一对应，**不要另起一套名字**：

- **Variables**：Collection `Muse / Dark`，分组 `color/*`、`radius/*`、`space/*`、`font/size/*`。深层语义（`semantic.background.*` 等）建第二组，值引用第一组 —— 这样「换皮」才只需要改一处。
- **Text styles**：`Badge 10`、`Caption 11`、`Body 13`、`Toolbar 15`、`Title 17`、`Heading 20`、`Display 22`，字体 Sarasa UI SC，行高 1.5（标题 1.35）。
- **Effect styles**：`Card Hover`、`Selection Glow`、`Popover`，对应 tokens 里的 shadow。
- **Rule**：画布里出现 `#7c5cff` 这样的裸值就算失败 —— 一律绑变量。这条和代码里「业务组件不写魔法值」是同一条规矩的两面。

## 5. 回流：打磨完之后怎么回到代码

允许的改动只有两条路（和仓库已有的分层规则一致）：

1. **改 token** → 改 [theme.css](../../frontend/src/theme.css) 的 11 个变量，同步 [figma-tokens.json](figma-tokens.json)。两端壳自动一致。
2. **改基元** → 改 `ui/` 下的组件（props / 样式），业务文件不动。Astryx 的对外 props 保持不变，调用点才不用改（见 [astryx-design-system.md](../spec/astryx-design-system.md)）。

不允许的：在 `components/` 里为一个界面写一次性样式、绕过 `ui/` 直接 import `@astryxdesign/*`、把 Figma 里的新颜色硬编码进 TSX。`scripts/check-shared-ui.mjs` 会拦住其中一部分。

每次回流后：

```bash
cd frontend && npm run lint && npm test && npm run build
# 重拍对照图（需另开 npm run dev:design）
cd .. && PLAYWRIGHT_MODULE=/path/to/playwright node frontend/scripts/capture-design.mjs
```

用重拍的截图和 Figma 里的帧对账，差异写进 commit message，别留在脑子里。

## 6. 复现与文件清单

```bash
# 1. 起预览（内置 mock 数据与占位封面，不需要 Wails、不需要后端）
cd frontend && npm run dev:design
#   http://localhost:5199/src/design/preview.html         桌面
#   http://localhost:5199/src/design/preview-mobile.html  移动
#   http://localhost:5199/src/design/board.html           组件基元

# 2. 重拍截图
PLAYWRIGHT_MODULE=/path/to/playwright node frontend/scripts/capture-design.mjs
#   产物：docs/design/screenshots/*.jpg（界面）与 20-design-board.png（基元）
```

| 文件 | 作用 |
|---|---|
| [frontend/src/design/preview.tsx](../../frontend/src/design/preview.tsx) | 桌面预览入口：装平台 stub → 绑 mock client → 渲染真实 App |
| [frontend/src/design/preview-mobile.tsx](../../frontend/src/design/preview-mobile.tsx) | 移动预览入口：mock `fetch` + 同一份 mock client |
| [frontend/src/design/board.tsx](../../frontend/src/design/board.tsx) | 组件基元长图 |
| [frontend/src/design/mock/](../../frontend/src/design/mock/) | 假数据与 `MuseClient` 实现（28 个方法），写操作会触发真实的 `invalidate()` |
| [frontend/design.preview.config.ts](../../frontend/design.preview.config.ts) | 预览 dev server：`/thumbnails`、`/covers`、`/avatars` 返回动态生成的占位封面 |
| [frontend/scripts/capture-design.mjs](../../frontend/scripts/capture-design.mjs) | Playwright 截图脚本（Playwright 需外部安装，用 `PLAYWRIGHT_MODULE` 指向） |
| [screen-inventory.md](screen-inventory.md) | 界面 × 状态 × 源码清单 |
| [figma-tokens.json](figma-tokens.json) | token 镜像（Tokens Studio / Figma Variables） |

`npm run dev:design` 是新增脚本；预览与截图都**不进入** `wails build` 与 `build:mobile` 的产物（两个入口都没有 import 它们）。

## 7. 验证清单（导入后逐条打勾）

- [ ] Variables 里改 `color/accent`，画布上的主按钮、选中描边、chip 一起变（说明绑定生效，没有裸值）。
- [ ] 卡片是 `MediaCard` 的实例，改组件内边距时所有页面同步。
- [ ] 桌面 1440 与 1100 两档都不出现横向滚动；顶栏在 1100 下不换行（换行就是间距/宽度没按 Auto Layout 做）。
- [ ] 详情页、右键菜单、筛选面板、编辑对话框、抽屉五类覆盖层各有一帧，且用了对应的 Dialog / Dropdown / Drawer 组件。
- [ ] 每个 frame 的图层名与第 3 节表格一致；没有 `Group 12`、`Rectangle 45`。
- [ ] 设计稿旁贴着对应的截图文件名（`01-library-grid.jpg` 等），任何人能反查到代码。
