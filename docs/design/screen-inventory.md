# Muse Video · 界面清单（Figma 固化范围）

这份清单是 Figma 导入的工单：一行 = 一个要进画布的 frame。截图全部来自**真实组件在浏览器里的渲染**（不重绘、不美化），数据来自 dev-only 的 mock 传输层（[src/design/](../../frontend/src/design/)），所以结构与间距和线上一致。

- 抓取方式：`npm run dev:design` + [capture-design.mjs](../../frontend/scripts/capture-design.mjs)
- 桌面端：1440×900 @2×（应用默认窗口 1100×720，截图取更大视口以便看清信息密度）
- 移动端：390×844 @3×（iPhone 14 逻辑宽度）
- 字体：`Segoe UI`。抓取机是 macOS，浏览器回落到 Helvetica/SF，**字宽与 Windows 上略有差异**；Figma 里请把字体显式设为 `Segoe UI`，再用截图做位置参考而非逐像素对齐。

## 桌面端（Wails 壳）

| # | 截图 | 界面 | 入口 | 源码 | 要点 |
|---|---|---|---|---|---|
| 01 | [01-library-grid](screenshots/01-library-grid.jpg) | 收藏库 · 网格 | 启动默认 | [pages/Library.tsx](../../frontend/src/pages/Library.tsx)、[VideoGrid.tsx](../../frontend/src/components/VideoGrid.tsx) | 顶栏（Logo / 后退前进 / 添加 / 局域网 / 搜索 / ? / 设置）+ 侧栏 + 工具栏（排序、卡片大小、视图）+ 卡片网格 |
| 02 | [02-library-list](screenshots/02-library-list.jpg) | 收藏库 · 列表 | 视图切换 → 列表 | [VideoList.tsx](../../frontend/src/components/VideoList.tsx) | 行式布局：封面 16:9 + 标题 + 元信息 + 点赞 |
| 03 | [03-library-detail](screenshots/03-library-detail.jpg) | 影片详情 | 点卡片 → 打开详情 tab | [VideoDetailPanel.tsx](../../frontend/src/components/VideoDetailPanel.tsx) | 工作区标签切换、封面 + 刮削元数据两列、演员/标签、刮削模式单选、文件信息 |
| 04 | [04-card-context-menu](screenshots/04-card-context-menu.jpg) | 卡片右键菜单 | 右键卡片 | [ContextMenu.tsx](../../frontend/src/components/ContextMenu.tsx) | 6 项，最后一项 danger；浮层阴影与圆角 |
| 05 | [05-library-multi-select](screenshots/05-library-multi-select.jpg) | 多选与批量条 | hover 卡片 → 勾选 | [VideoCard.tsx](../../frontend/src/components/VideoCard.tsx) | 选中描边 + 外发光、工具栏批量操作条（移入回收站等） |
| 06 | [06-filter-dialog](screenshots/06-filter-dialog.jpg) | 筛选面板 | 工具栏「筛选」/ 侧栏漏斗 | [FilterDialog.tsx](../../frontend/src/components/FilterDialog.tsx) | 标签 / 演员 / 硬盘三组多选、搜索、已选 chip |
| 07 | [07-edit-dialog](screenshots/07-edit-dialog.jpg) | 编辑信息 | 右键 → 编辑信息… | [EditVideoDialog.tsx](../../frontend/src/components/EditVideoDialog.tsx) | 标题 / 备注 / 封面重选入口；表单型对话框 |
| 08 | [08-recently-played](screenshots/08-recently-played.jpg) | 最近播放 | 侧栏「最近播放」 | [RecentlyPlayedView.tsx](../../frontend/src/components/RecentlyPlayedView.tsx) | 按 1 天 / 3 天 / 7 天 / 14 天 / 更早分组 |
| 09 | [09-actors](screenshots/09-actors.jpg) | 演员索引 | 侧栏「演员」 | [ActorIndexView.tsx](../../frontend/src/components/ActorIndexView.tsx) | 演员 chip（头像/艺名/数量）、搜索、补全进度 |
| 10 | [10-scrape-tasks](screenshots/10-scrape-tasks.jpg) | 刮削任务 | 侧栏「刮削任务」 | [ScrapeTaskView.tsx](../../frontend/src/components/ScrapeTaskView.tsx) | 队列分组、失败原因、重试 |
| 11 | [11-background-tasks](screenshots/11-background-tasks.jpg) | 后台任务 | 侧栏「后台任务」 | [BackgroundTasksView.tsx](../../frontend/src/components/BackgroundTasksView.tsx) | 多任务进度条、当前条目 |
| 12 | [12-duplicates](screenshots/12-duplicates.jpg) | 查重 | 侧栏「查重」 | [DuplicateView.tsx](../../frontend/src/components/DuplicateView.tsx)、[ContentDuplicateView.tsx](../../frontend/src/components/ContentDuplicateView.tsx) | 同番号 / 内容重复两组结论、保留/清理动作 |
| 13 | [13-trash](screenshots/13-trash.jpg) | 归档与回收站 | 侧栏「归档与回收站」 | [TrashView.tsx](../../frontend/src/components/TrashView.tsx) | 行式条目、恢复 / 彻底删除、保留天数说明 |
| 14 | [14-actor-films](screenshots/14-actor-films.jpg) | 演员作品 | 演员索引 → 点演员 | [ActorFilmsView.tsx](../../frontend/src/components/ActorFilmsView.tsx) | 演员档案 + 作品网格 |
| 15 | [15-settings-server](screenshots/15-settings-server.jpg) | 设置 · 服务器 | ⚙ 设置 | [pages/Settings.tsx](../../frontend/src/pages/Settings.tsx) | 设置左分类 + 右表单的双栏骨架 |
| 16 | [16-settings-cloud](screenshots/16-settings-cloud.jpg) | 设置 · 网盘 | 设置分类 | 同上 | 连接列表、表单、状态徽标 |
| 17 | [17-settings-llm](screenshots/17-settings-llm.jpg) | 设置 · 片名翻译 | 设置分类 | 同上 | 开关 + 输入 + 测试按钮 + 说明文案 |
| 18 | [18-settings-guide](screenshots/18-settings-guide.jpg) | 设置 · 新手引导 | ⚙ 设置 或 `?` | 同上 + [GuidedTour.tsx](../../frontend/src/components/GuidedTour.tsx) | 引导入口与快捷键表 |
| 19 | [19-settings-watch](screenshots/19-settings-watch.jpg) | 设置 · 目录监控 | 设置分类 | 同上 | 目录列表行 + 移除按钮 |
| 26 | [26-library-tag](screenshots/26-library-tag.jpg) | 标签筛选结果 | 侧栏点标签 | [Sidebar.tsx](../../frontend/src/components/Sidebar.tsx) | 已选 chip、计数文案「匹配 10 / 共 18」、清除 |

## 组件基元（导入成 Figma 组件库的那一页）

| # | 截图 | 内容 |
|---|---|---|
| 20 | [20-design-board](./screenshots/20-design-board.png) | Color / Typography / Spacing / Radius / Button（variant × size）/ IconButton / Input / Badge & Chip / Cover（portrait / landscape / 卡片组合）/ Pagination / Feedback / Metadata。页面本身是 [board.tsx](../../frontend/src/design/board.tsx)，由真实组件渲染 |

## 移动端（局域网 Web）

| # | 截图 | 界面 | 入口 | 源码 |
|---|---|---|---|---|
| 21 | [21-mobile-library](screenshots/21-mobile-library.jpg) | 收藏 · 网格 | 默认 | [mobile/App.tsx](../../frontend/src/mobile/App.tsx)、[mobile/components/LibraryView.tsx](../../frontend/src/mobile/components/LibraryView.tsx) |
| 22 | [22-mobile-navigation](screenshots/22-mobile-navigation.jpg) | 抽屉导航 | 左上 ☰ | 同上（`Drawer`） |
| 23 | [23-mobile-detail](screenshots/23-mobile-detail.jpg) | 影片详情 | 点卡片 | [mobile/components/VideoDetailView.tsx](../../frontend/src/mobile/components/VideoDetailView.tsx) |
| 24 | [24-mobile-actors](screenshots/24-mobile-actors.jpg) | 演员 | 抽屉 → 演员 | [mobile/components/ActorIndexView.tsx](../../frontend/src/mobile/components/ActorIndexView.tsx) |
| 25 | [25-mobile-trash](screenshots/25-mobile-trash.jpg) | 归档与回收站 | 抽屉 → 归档与回收站 | [mobile/components/TrashView.tsx](../../frontend/src/mobile/components/TrashView.tsx) |

## 未覆盖（有意或待补）

| 界面 | 原因 / 下一步 |
|---|---|
| 播放器 | 应用内没有播放器画面：播放交给系统默认播放器或外部播放器（`openVideo`）。桌面端只有「▶ 播放」按钮，移动端有「播放器选项」抽屉。Figma 里不需要画播放器，需要的话画的是**外部播放器的选择抽屉** |
| 新手引导浮层（GuidedTour 运行态） | 它是覆盖层 + 分步定位，`?` 可复现；抓取时关闭了自动弹出。需要单独一帧时在预览里点 `?` → 新手引导 → 开始 |
| 「添加视频 / 添加来源」对话框 | 依赖原生文件选择器，桌面端专属；如需固化，可在预览里直接调 `AddSourceDialog` open 状态 |
| 云端浏览 / 网盘连接对话框 | 同上，依赖后端网络请求；结构可参考设置页的网盘分区 |
| 空库 / 加载中 / 错误 | 预览默认有数据；把 `mock/data.ts` 的 `VIDEOS` 置空并截图即可得到空库态 |
| 移动端设置页 | 移动壳没有独立设置页（只有抽屉 + 各视图），设置只在桌面端 |

## 已知的界面问题（打磨时优先看）

1. **移动端详情页把刮削状态原样打印**：`状态：done`（[VideoDetailView.tsx:246](../../frontend/src/mobile/components/VideoDetailView.tsx)），桌面端是中文标签。属于「界面悄悄不对」，与 Figma 无关但值得记一笔。
2. **网格卡片密度**：1440 宽下默认卡片尺寸只排 2 列（01），信息密度偏低；卡片大小滑杆（03 工具栏）是唯一调节手段。
3. **顶栏元素顺序**：Logo、前进后退、添加、局域网、搜索、帮助、设置挤在一行，搜索框宽度随窗口伸缩，窄窗口（接近 960 最小宽）会换行——Figma 里建议同时画 1100 与 1440 两档。
