# Astryx 全量替换方案

> 状态：Phase 1–4 已实施（2026-11-16）；Phase 5/6 部分完成，Phase 7 待继续。  
> 范围：`frontend/src/` 的桌面端与移动端；产品深色 token、`ui/` 唯一接缝、文件零改动原则不变。

## 0. 结论先行

**能做到的**：业务文件里不再出现自绘的交互组件——按钮、卡片、菜单、底部弹层、标签、空态、加载态、导航行、分页/标签条全部来自 `ui/` 对 Astryx 的适配层。

**做不到、也不建议做的**：把每一个 `<div>`、每一段布局类名都换成 Astryx 组件。Astryx 的 `Stack`/`Grid`/`Section` 能覆盖大部分布局，但视频详情、分组树、时间线等业务组合最终还是“由 Astryx 基元拼出来的 Muse 视图”。那属于业务视图，不属于自绘组件。

真正需要保留的例外只有四类：

1. `ui/error-boundary.tsx`：React 错误边界是 class 逻辑，Astryx 没有等价物；只把 fallback 换成 `EmptyState`/`Button`。
2. `frontend/src/hooks/useVirtualRows.ts`：长列表窗口化。Astryx 的 `useListWindow` 是 core 私有实现，没有公开导出；保留这个 hook，只把它渲染的卡片/行换成 Astryx。
3. `GuidedTour` 的聚光灯遮罩：Astryx 没有 tour/spotlight 组件，只有 `Overlay`/`Popover`。要么保留这一块自绘，要么接受去掉聚光灯、改成普通 popover 步骤。
4. 拖拽语义（分组排序、嵌套、把视频拖进分组）：Astryx 的 `SideNavItem`/`TreeList` 没有内置这套业务 DnD；视觉行可以换，拖拽状态机和 drop 规则仍留在 Muse。

**建议目标**：业务层零自绘交互组件；`ui/` 保留薄适配/组合层；上述四类作为显式例外写进检查脚本和文档。

---

## 1. 现状盘点

### 1.1 已经走 Astryx 的 `ui/` 适配

| 文件 | Astryx |
|---|---|
| `ui/button.tsx` | `Button` |
| `ui/input.tsx` | `TextInput` |
| `ui/dialog.tsx` | `Dialog` |
| `ui/drawer.tsx` | `Drawer` |
| `ui/sheet.tsx` | `BottomSheet` + `Dialog` |
| `ui/toast.tsx` | `Toast` + `ToastViewport` |
| `ui/theme.tsx` | `Theme` + `theme-neutral` |

这些不需要“迁移”，只需要在替换其它组件时保持它们的公开 API 不变。

### 1.2 仍自绘的 `ui/`

| 文件 | 现状 | Astryx 替代 |
|---|---|---|
| `ui/NavIcon.tsx` | `?raw` SVG + `dangerouslySetInnerHTML` | `Icon` + `registerIcons`；内置名直接映射，Muse 图标注册为 `muse:*` |
| `ui/backup-badge.tsx` | 自绘胶囊 | `Badge variant="success"` |
| `ui/restored-badge.tsx` | 自绘胶囊 | `Badge variant="purple"` |
| `ui/menu.tsx` | 已改为 `DropdownMenu` 的 1×1 坐标触发器适配层 | `DropdownMenu`/`DropdownMenuItem`/`DropdownMenuSubMenu`；已删除旧键盘/定位逻辑 |
| `ui/flyout-shell.tsx` | 已删除 | `DropdownMenuSubMenu` 承担锚点、翻转与 hover-intent |
| `ui/sidebar.tsx` | 手写 aside/nav/section | `SideNav` + `SideNavSection` + `SideNavItem` |
| `ui/sticky-bar.tsx` | 手写 sticky div | `Toolbar className="sticky top-0"`（或 `Section` + `Stack`） |
| `ui/error-boundary.tsx` | class + 自绘 fallback | 保留 class；fallback 改用 `EmptyState`/`Banner` + `Button` |

### 1.3 自绘业务组件

移动端 `mobile/components/`：

- `LibraryView` 工具栏、筛选 Chip、结果计数
- `FilterSheet` 的分区、Chip、排序方向
- `VideoGrid`/`VideoList` 的海报卡、卡片菜单、列表行
- `States` 的 Loading/Empty/Error
- `App.tsx` 的 header、drawer 内容、裸按钮
- `ActorIndexView`、`ActorFilmsView`、`DuplicateView`、`ContentDuplicateView`、`ScrapeTaskView`、`TrashView`、`VideoDetailView` 的列表/卡片/分区
- `VideoActionSheet`、`ExternalPlayerSheet`、`EditInfoSheet` 的行与表单

桌面端 `components/` + `pages/`：

- `VideoCard`、`VideoGrid`、`VideoList`、`RecentlyPlayedView`
- `SortBar`、`ViewControls`、`DiskFilterBar`、`SearchBar`、`ServerBar`、`FileScanButton`
- `Sidebar`、`WorkspaceTabs`
- `ContextMenu`、`GroupContextMenu`、`TabContextMenu`、`GroupFlyout`、`GroupRelationFlyout`、`MetadataPopup`
- `ActorIndexView`、`ActorFilmsView`、`DuplicateView`、`ContentDuplicateView`、`ScrapeTaskView`、`TrashView`、`VideoDetailPanel`
- `AddSourceDialog`、`CloudBrowserDialog`、`EditVideoDialog`、`GroupDialog`、`GroupSuggestDialog`、`ConfirmDialog`
- `GuidedTour`、`DropOverlay`
- `pages/Library.tsx`、`pages/Settings.tsx`（两个大页面）

---

## 2. 替代映射

### 2.1 图标、徽标、状态、工具栏

| 现在 | 替换为 | 说明 |
|---|---|---|
| `NavIcon` | `Icon` | 内置图标直接用 `close`/`menu`/`moreHorizontal`/`funnel`/`chevronDown`/`check`/`warning`；Muse 专有图标在 `ui/icon.tsx` 里 `registerIcons` 成 `muse:star`、`muse:grid`、`muse:list`、`muse:heart`、`muse:folder` 等。调用点保持 `<NavIcon name=...>` 的语义，内部换成 Astryx `Icon`。 |
| `backup-badge` / `restored-badge` | `Badge` | 语义映射：已备份 → `success`，马赛克破解 → `purple`；缺失/离线 → `error`/`warning`。`ui/badge.tsx` 统一收口。 |
| `States.Loading` | `Spinner` | 居中 `Spinner size="lg"`，可选 `label`。 |
| `States.EmptyState` | `EmptyState` | `title`/`description`/`actions`/`isCompact` 直接对应。 |
| `States.ErrorState` | `EmptyState` + `actions` 或 `Banner status="error"` | 重试按钮用 `Button variant="primary"`。 |
| 首屏骨架 | `Skeleton` | 海报卡和列表行都可以做骨架波次。 |
| `SortBar` / `ViewControls` | `Toolbar` + `ToggleButtonGroup` / `SegmentedControl` + `Slider` | 排序字段是多选一（可再点取消/翻转方向），用 `ToggleButtonGroup type="single"` 或 `ButtonGroup`；视图切换用 `SegmentedControl`；卡片尺寸用 `Slider`。 |
| `DiskFilterBar` | `ToggleButtonGroup type="multiple"` | 每个硬盘一个 `ToggleButton`，`value` 为 `diskKey`。 |
| `FilterSheet` 的 Chip 区 | `ToggleButtonGroup type="multiple"` | 分组、演员、硬盘各一个组；数量放进 `label`，离线标记用 `icon`/`endContent`。若 `ToggleButtonGroup` 不支持自动换行，改用 `CheckboxList` + `CheckboxListItem`。 |
| 筛选分区 | `Section variant="transparent"` + `Divider` | 分区标题、说明、间距交给 Astryx。 |
| 排序方向 | `SegmentedControl` 或 `ButtonGroup` | “降序/升序”两个互斥按钮。 |
| `SearchBar` | `Input` + `IconButton`（或 `InputGroup`） | 搜索图标/清除按钮用 `Icon`/`IconButton`。 |

### 2.2 卡片与集合

| 现在 | 替换为 | 说明 |
|---|---|---|
| 移动端海报卡 `VideoGrid` | `Grid` + `ClickableCard` + `AspectRatio` + `Badge` + `MoreMenu` | `ClickableCard` 的隐藏 button + 嵌套交互正好覆盖“整卡点开、卡内更多菜单”；`AspectRatio ratio={3/4} fit="cover"` 保证统一比例；`MoreMenu presentation="adaptive"` 在手机上出 bottom sheet、桌面出锚点菜单。 |
| 桌面 `VideoCard` | `SelectableCard` + `AspectRatio` + `Badge` + `MoreMenu`/`HoverCard` | `SelectableCard` 自带 checkbox/radio 语义和选中环，替换手写左上角勾选框；嵌套菜单/点赞按钮可独立点击。 |
| 桌面 `VideoGrid` | `Grid` 承载卡片；窗口化保留 | `useVirtualRows` 负责只挂载可见行，`Grid` 只负责列/间距；`columns={columns}` 或 `Grid` 的响应式 `columns={{minWidth, max}}`。 |
| 桌面 `VideoList` / 移动端列表 | `List` + `ListItem` | 缩略图放 `startContent`，标题/元信息用 `label`/`description`，菜单放 `endContent`；仍由 `useVirtualRows` 决定渲染哪几行。 |
| 演员索引 | `List` + `ListItem` + `Avatar` | 头像/别名/计数分别放 `startContent`、`description`、`endContent`。 |
| 回收站、查重、刮削任务 | `List` + `ListItem` + `Badge`/`ProgressBar`/`Card` | 行操作放 `endContent`；成组查重用 `Card`/`Section`。 |
| `DropOverlay` | `Overlay` + `Center`/`Section` + `Text` | 只保留拖拽状态，不再手写全屏遮罩。 |

> 海报卡有一个 Astryx 官方取舍：`ClickableCard` 文档不建议用 `variant="transparent"` + `padding={0}` 来“借”按压缩放模型，建议改用 `useClickableContainer`。但本产品的海报卡本来就拥有自己的盒子（封面、渐变、圆角、比例），`ClickableCard padding={0} variant="transparent"` 是最接近“零自绘”的做法。若视觉不可接受，就只能在 `ui/media-card.tsx` 里用 Astryx 的 `useClickableContainer` hook 组合；这是唯一的“薄组合层”例外。

### 2.3 菜单与浮层

| 现在 | 替换为 | 说明 |
|---|---|---|
| `ui/menu.tsx` 的右键菜单 | `ContextMenu` | 直接吃光标位置，键盘/外点/Escape/翻转/自适应 bottom sheet 都由 Astryx 处理。 |
| `ui/flyout-shell.tsx` | `DropdownMenuSubMenu` | 子菜单支持 `presentation="flyout" \| "drill-in" \| "adaptive"`，`header`/`footer` 可放过滤输入或自定义内容，正好覆盖分组关系面板。 |
| `ContextMenu.tsx` / `GroupContextMenu.tsx` / `TabContextMenu.tsx` | `ContextMenu` + `items` 数据 | 保留为“把业务 action 映射成 items”的小函数，不再输出 DOM。 |
| `GroupFlyout` / `GroupRelationFlyout` | `DropdownMenuSubMenu` + `TreeList`/`CheckboxList` | 分组树用 `TreeList`，关系/祖先/后代用 `CheckboxList` 或 `ListItem`。 |
| `MetadataPopup` | `HoverCard` | 悬浮延迟、锚点、触摸降级由 Astryx 管。 |
| `GuidedTour` | 无直接替代 | 保留自绘，或降级为 `Popover` + `Overlay` 的普通步骤（失去聚光灯）。 |

### 2.4 导航、标签、外壳

| 现在 | 替换为 | 说明 |
|---|---|---|
| `ui/sidebar.tsx` + 桌面 `Sidebar.tsx` | `SideNav` + `SideNavSection` + `SideNavItem` + `TreeList` | 选中态/图标/嵌套/收起由 `SideNav` 负责；DnD 仍挂在 item 的宿主事件上，嵌套 drop 规则留在业务层。 |
| 移动端 `App.tsx` 抽屉 | `MobileNav` + `SideNavItem` | `MobileNav` 自带 dialog、遮罩、焦点陷阱、滑入滑出；内容用 `SideNavSection`/`SideNavItem`。 |
| `WorkspaceTabs` | `TabList` + `Tab`（`endContent` 放关闭按钮）+ `TabMenu` | 横向滚动、键盘 roving tabindex、选中态由 Astryx 负责；分组色仍可用 `Tab className`/CSS 变量表达。 |
| 移动端 header | `TopNav`（`heading`/`endContent`） | 搜索框放 `endContent`；抽屉按钮用 `IconButton`/`MobileNavToggle`。 |
| 桌面整体外壳 | `AppShell` + `TopNav` + `SideNav` | `pages/Library.tsx` 的布局分栏、历史、滚动还原仍留在业务层；壳的语义/响应式交给 `AppShell`。 |
| `ui/sticky-bar.tsx` | `Toolbar` + `className="sticky top-0"` | 不单独保留 sticky 组件。 |

### 2.5 表单、弹窗、详情

| 现在 | 替换为 |
|---|---|
| `AddSourceDialog`、`CloudBrowserDialog`、`EditVideoDialog`、`GroupDialog`、`GroupSuggestDialog` | 已通过 `ui/dialog` 的 Astryx `Dialog`；内部改为 `FormLayout`/`Field`/`TextInput`/`TextArea`/`Selector`/`TabList`/`List`/`Breadcrumbs` |
| 移动端 `EditInfoSheet`、`ExternalPlayerSheet`、`VideoActionSheet` | `BottomSheet`（已是）+ `FormLayout`/`Button`；动作列表改用 `List`/`ListItem`，或 `MoreMenu` 的 bottom-sheet 展示 |
| `VideoDetailPanel` / `VideoDetailView` | `Section` + `Layout`/`Stack` + `MetadataList` + `AspectRatio` + `Badge` + `HoverCard` |
| `Settings.tsx` | `FormLayout` + `Section` + `Card` + `Switch` + `Selector`/`RadioList` + `Input` + `Button` |

---

## 3. 不可替换清单

这些不是“懒得换”，而是没有现成等价物：

1. **`ErrorBoundary` class**：React 的错误边界只能 class；只替换 fallback 的 UI。
2. **`useVirtualRows`**：Astryx `useListWindow` 未公开导出；保留 hook，卡片/行用 Astryx 组件。
3. **`GuidedTour` 聚光灯**：Astryx 没有 tour/spotlight；若要“零自绘”，只能去掉聚光灯改成普通 `Popover` 步骤，这是产品取舍。
4. **拖拽分组/嵌套/投放到分组**：视觉行可以换 `SideNavItem`/`TreeList`，但拖拽状态机和嵌套判定是业务逻辑，不是 UI 组件。
5. **品牌 SVG 资产**：图标仍由 Muse 拥有，但通过 `registerIcons` 交给 Astryx `Icon` 渲染；资产不是组件。
6. **布局组合**：`Stack`/`Grid`/`Section` 能替换大部分 flex/grid，但视频详情的时间线、分组树、分栏等业务组合最终仍是 Muse 视图；这是视图，不是自绘组件。

---

## 4. 分阶段落地

### Phase 0：守卫与基线

- 新增检查：业务文件不得出现裸 `<button>`/`<input>`/`<select>`/`<textarea>`/`role="menu"`；`ui/` 外不得 import `@astryxdesign/*`（已检查）。
- 先设“只减不增”的基线，迁移一个文件就调低一次上限。
- 产出本方案 + `astryx-design-system.md` 的链接。

**退出条件**：lint 通过，基线有数字。

### Phase 1：原子层（低风险）— ✅ 已完成

- `NavIcon` → `Icon` + `registerIcons`
- `backup-badge` / `restored-badge` → `Badge`
- `States` → `EmptyState`/`Spinner`/`Skeleton`
- `ui/sticky-bar` → `Toolbar`
- `SortBar`/`ViewControls`/`DiskFilterBar` → `Toolbar`/`SegmentedControl`/`ToggleButtonGroup`/`Slider`
- `ui/error-boundary` fallback → `EmptyState`/`Banner` + `Button`

**退出条件**：320/390/768 与桌面截图一致；裸按钮数量下降。

### Phase 2：移动端片库（可视化试点）— ✅ 已完成

- 海报卡 → `Grid` + `ClickableCard` + `AspectRatio` + `Badge`（完成）
- 列表 → `List` + `ListItem`（完成）
- 筛选面板 → `BottomSheet` + `Section` + `ToggleButtonGroup` + `Divider`（完成）
- 工具栏 → `Toolbar` + `SegmentedControl`（完成）
- 卡片菜单暂用 Astryx `Button` 触发既有 `VideoActionSheet`；`MoreMenu` 与动作面板合并放到 Phase 4。

**结果**：320/390/768 视觉通过；`ClickableCard padding=0` + 透明 Card 保留了全出血海报；筛选多选、关闭保留、返回键、海报比例、列表/网格切换与虚拟滚动已回归。卡片菜单/动作面板仍未替换，留在 Phase 4。

### Phase 3：桌面卡片与工具栏 — ✅ 已完成

- `VideoCard` → `SelectableCard` + `AspectRatio` + `Badge` + `MoreMenu`
- `VideoGrid`/`VideoList` → `Grid`/`List` + 保留窗口化
- `SortBar`/`ViewControls`/`DiskFilterBar`/`SearchBar` → 同 Phase 1 的 Astryx 控件

**结果**：`VideoCard` 用 `ClickableCard` + `CheckboxInput`（保留“点卡片打开、勾选框多选”的原行为，比 `SelectableCard` 的“点卡片即选中”更贴合产品）；`VideoGrid` 用 `Grid` 承载固定宽度轨道；`VideoList` 用 `List`/`ListItem`；`SearchBar` 用 `Input` + `IconButton`。卡片尺寸滑块、窗口化、右键、双击、拖拽、多选均已保留。

### Phase 4：菜单与浮层（高风险）— ✅ 已完成

- `ui/menu` + `ui/flyout-shell` → `ContextMenu`/`DropdownMenuSubMenu`
- `GroupFlyout`/`GroupRelationFlyout`/`MetadataPopup` → 子菜单/`HoverCard`
- 移动端 `VideoActionSheet` → `MoreMenu` adaptive

**结果**：`ui/menu.tsx` 现在是 Astryx `DropdownMenu` 的坐标适配层（1×1 隐藏触发器，保留任意坐标右键），`GroupFlyout`/`GroupRelationFlyout` 改为 `DropdownMenuSubMenu` + `DropdownMenuCheckboxItem`/`DropdownMenuRadioGroup`，`MetadataPopup` 改为 `HoverCard`，移动端动作/播放器/编辑表单改为 `List`/`ListItem`/`Input`/`TextArea`/`Button`。桌面菜单的键盘、外点关闭、Escape、翻转、子菜单 hover 由 Astryx 负责。`VideoActionSheet` 仍是 `BottomSheet` 组合（未改成 `MoreMenu` 的 adaptive 展示），但内部行已全部是 Astryx。

### Phase 5：导航与外壳（部分完成）

- 桌面 `Sidebar` → `SideNav`/`TreeList`
- 移动端 drawer/header → `MobileNav`/`TopNav`
- `WorkspaceTabs` → `TabList`/`Tab`
- `pages/Library.tsx` → `AppShell` + `TopNav` + `SideNav`（布局逻辑保留）

已完成：移动端 header 的菜单/关闭按钮改为 `IconButton`，搜索框改为 `Input`。仍待迁移：桌面 `Sidebar`、`WorkspaceTabs`、`pages/Library.tsx` 外壳，以及 `MobileNav`/`TopNav`/`AppShell`。

### Phase 6：业务页面（部分完成）

- 演员索引/作品、查重（番号 + 内容）、刮削、回收站、视频详情、设置
已完成：移动端演员索引（`List`/`ListItem`/`Avatar`）、刮削看板（`SegmentedControl`/`List`/`ListItem`）、动作/播放器/编辑表单。仍待迁移：桌面演员/查重/回收站/刮削/详情/设置，以及移动端查重/回收站/详情。

- 统一 `Section`/`List`/`Card`/`Badge`/`EmptyState`/`Banner`/`FormLayout`

### Phase 7：清理（待继续）

- 删除已无引用的自绘文件与 CSS
- 裸按钮上限降到 0（或只保留例外清单）
- 更新 `astryx-design-system.md` 的“刻意没有替换”表
- 全量视觉回归 + 构建 + 测试

---

## 5. 验证矩阵

至少覆盖：

- 尺寸：320 / 390 / 430 / 768 / 桌面 1100+/缩放 0.8–1.5
- 输入：鼠标右键、双击、拖拽；触摸长按/滑动；键盘 Tab/方向键/Escape
- 状态：加载、空、错误、缺失、离线、备份、破解、选中、禁用
- 浮层：右击坐标、子菜单 hover/键盘、手机 bottom sheet、返回手势、焦点归还
- 性能：上千条收藏的虚拟滚动、窗口缩放、筛选切换
- 构建：`npm run lint`、`npm run build`、`npm run build:mobile`、`go test ./...`

---

## 6. 建议

先做 **Phase 1 + Phase 2**，在移动端形成一个完整垂直切片：图标/徽标/状态/筛选/海报卡/列表全部换成 Astryx。这个切片能回答两个关键问题：

1. Astryx 的深色 token 下，`ClickableCard`/`Badge`/`ToggleButtonGroup` 的视觉是否达到现有“cinematic library”水平；
2. 菜单/子菜单/自适应 bottom sheet 是否足以替代现有手写交互。

如果这两点通过，再按 Phase 3–7 推进；如果海报卡必须保留自定义叠层，就把 `ui/media-card.tsx` 记为唯一薄组合例外，其余仍可全量替换。
