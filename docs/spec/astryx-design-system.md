# Astryx 设计系统接入

决定：引入 Meta 的 Astryx（`@astryxdesign/core` 0.7.15 + `theme-neutral`）作为设计系统，但**只从 `frontend/src/ui/` 暴露**，业务组件不直接 import。本文记录已验证的事实、必须遵守的两条规则，以及分批采用的顺序。

> 全量替换自绘组件的逐项映射与分阶段方案见 [`astryx-migration-plan.md`](./astryx-migration-plan.md)，逐组件“现状 vs 文档建议”的完整对照见 [`astryx-usage-audit.md`](./astryx-usage-audit.md)。本文记录当前已采用的边界。2026-11-16 已完成 Phase 1–9（Phase 9 为原生语义收尾：`TabList`、`MoreMenu`、`ClickableCard` 列表行、`Banner` 报告 + 原生 Toast）。

## 为什么只做接缝，不整体换皮

Astryx 有 174 个组件入口，是一套完整设计语言（排版、间距、圆角、动效、a11y）。Muse 有自己的产品 token（`src/theme.css`）、CSS `zoom` 外壳（`--ui-zoom`，0.8–1.5）、自绘滚动条、中文界面。整体替换的代价不是 import 语句，而是第二套视觉语言。

真正值得用它的地方是那些自己写要几百行、还要处理键盘与无障碍的组件：类型提前联想、令牌输入、多选、分组树、可排序表格、命令面板、底部抽屉。

## 已核实（2026-11-14，spike 分支实测）

| 关注点 | 结论 |
|---|---|
| React 19 / TS | 兼容；包自带 `.d.ts`，`strict` 下 `tsc` 通过 |
| 离线可用 | `astryx.css` 内 `url(` 出现 0 次，无外链字体；Wails 内嵌无需额外资源 |
| 中文 | `locales/zh-CN.json` 齐全，内置文案有中文 |
| **CSS `zoom` 外壳** | **无冲突**。zoom 0.8 / 1.0 / 1.5 下，Astryx Dialog（`showModal()` 顶层）、Popover（CSS anchor positioning）与现有 Radix Dialog 三者均居中、均在视口内，遮罩始终等于视口 |
| 组件是否吃 `className` | **取决于 layer 顺序**（见规则一） |
| 主题 token | 可由 CSS 变量桥接，`--color-accent` 实测解析为 Muse 的 `#7c5cff`，按钮圆角等于 `--radius` |

## 规则一：layer 顺序必须显式声明

Tailwind v4 的 `@import "tailwindcss"` 会把 `utilities` 排在 Astryx 的 layer 之前，结果是**任何传给 Astryx 组件的 Tailwind 类都失效**（实测：`h-[80px]` 的 Astryx Button 高度仍是 32px，`rounded-full` 无效果）。两个入口统一用拆开的形式声明顺序：

```css
@layer theme, base, components, reset, astryx-base, astryx-theme, utilities;

@import "tailwindcss/theme.css" layer(theme);
@import "tailwindcss/preflight.css" layer(base);
@import "./range.css";
@import "./theme.css";            /* 内含 Astryx 三张样式表 */
@import "tailwindcss/utilities.css" layer(utilities);
```

`src/style.css` 与 `src/mobile/mobile.css` 必须保持一致——桌面端能赢的类，移动端不能输。

## 规则二：设计系统只经 `ui/` 暴露

`src/ui/theme.tsx` 是唯一的 provider（`<AppTheme>`，两个入口都包上），token 桥接写在 `src/theme.css` 的 `[data-astryx-theme]` 块里（unlayered，才能盖住 Astryx 自己的 `@layer astryx-theme`）。

业务层（`components/`、`pages/`、`mobile/`、`views/`）不得直接 import `@astryxdesign/*`：`scripts/check-shared-ui.mjs` 会拦截。要用的组件先在 `ui/` 里包一层，**保持现有 `ui/` 对外 props 不变**，调用点才不用改（现在有 41 个文件、78 处依赖 `ui/`）。

token 也不在 TS 里重复定义：`defineTheme()` 会把颜色搬进 TypeScript，与 `theme.css` 形成两份调色板。除非将来需要 Astryx 从品牌色推导完整色阶，再用 `astryx theme build` 生成主题包。

## 已完成的替换（`frontend/src/ui/`）

| 模块 | Astryx 组件 | 对外 API | 适配说明 |
|---|---|---|---|
| `astryx.tsx` | 再导出 | — | 业务文件只从这里取 Astryx 组件；`compactBadgeClass` / `toggleChipClass` 收口紧凑徽标与 Chip 的房规 |
| `icon.tsx` / `muse-icons.tsx` | `Icon` + `registerIcons` | `<Icon name="…"/>` | 内置名映射到 Astryx，Muse 专有 SVG 注册为 `muse:*`；自定义图标只以 ReactNode 注册 |
| `menu.tsx` | `DropdownMenu` | `MenuSurface({x,y,label,onClose,children})` | 1×1 隐藏触发器放在光标坐标，保留任意坐标右键；菜单内容由业务传 `DropdownMenuItem`/`DropdownMenuSubMenu` |
| `button.tsx` | `Button` | 不变（`variant` / `size` / children）+ `icon` / `tooltip` | 文字经 `children` 传入；`label` 取 children 的纯文本。图标必须走 `icon` 插槽（不能塞 children，否则会掉进 block 级 label 里、失去间距）；原生 `title` 自动转成 Astryx `tooltip`。`default→secondary`、`danger→destructive`；纯图标按钮改 `IconButton`。 |
| `input.tsx` | `TextInput` | 原生 input props + `label` / `isLabelHidden` | 事件转回原生 `onChange(event)`；`id` 透传给真输入框。传 `label` 时显示 Astryx 可见标签，否则标签隐藏、靠外部 `<label htmlFor>` / `aria-label` 命名；`number` 无对应字段，降级为 `text + inputMode=numeric` |
| `dialog.tsx` | `Dialog` + `DialogHeader` + `AlertDialog` | 不变（`open`/`onOpenChange` + `DialogHeader`/`Footer`） | 标题、`aria-labelledby`、聚焦与关闭按钮全部交给 Astryx `DialogHeader`；`purpose` 透传（表单对话框用 `form`）。确认类对话框改 `AlertDialog`；无标题的图片灯箱用 `DialogCloseButton`。宽度/内边距走 props，不用 class |
| `toast.tsx` | `Toast` + `ToastViewport` | 不变（受控 `toasts`） | 纯原生卡片，无 `details`；长结果走 `ReportBanner` |
| `WorkspaceTabs.tsx` | `TabList` + `Tab` | 不变 | 扁平化标签；组色改为前置圆点；关闭走右键/中键 |
| `mobile` 卡片菜单 | `MoreMenu` | — | 底部面板；点赞次数进 label，底部说明移除 |
| `VideoList` / 移动列表 | `ClickableCard` | — | 行本身可点，内部勾选/点赞/更多独立 |
| `ui/report.tsx` | `Banner` | `<ReportBanner>` | 长结果常驻；Toast 只留一句话 |
| `drawer.tsx` | `Drawer` + `DrawerHeader` | 不变 | `side` 映射 `start`/`end`；头部改用 Astryx `DrawerHeader`（标题、副标题、关闭按钮），行改为 `Button` 并把图标放进 `icon` 插槽 |
| `sheet.tsx` | `BottomSheet` + `Dialog` | 不变 | `height` 使用 Astryx 的 `hug` / `capped` / `tall` 起始高度；`ConfirmSheet` 改用 Astryx Dialog；行改为 `Button` |
| `astryx.tsx` | `AlertDialog`、`Badge`、`EmptyState`、`Spinner`、`Skeleton`、`Banner`、`Toolbar`、`SegmentedControl`、`ToggleButtonGroup`、`Slider`、`ClickableCard`、`AspectRatio`、`Grid`、`List`/`ListItem`、`Section`、`Divider`、`Text` 等 | 直接 re-export | 业务层只经这个 barrel 取用；`toggleChipClass` 统一多选 Chip 的静态样式 |
| `icon.tsx` | `Icon` + `registerIcons` | `name` 仍用 kebab-case | Muse SVG 注册为 `muse:*`；menu/close/warning/funnel/chevron 等直接映射 Astryx 内置语义名 |
| `muse-icons.tsx` | — | — | Muse 自有 SVG 路径转成组件，交给 `Icon` 渲染；资产仍是唯一图形来源 |

随之删除：`ui/NavIcon.tsx`、`ui/backup-badge.tsx`、`ui/restored-badge.tsx`、`ui/sticky-bar.tsx`、`mobile/components/States.tsx`、`components/EmptyState.tsx`。

随之删除的历史文件：`ui/overlay.ts`（聚焦陷阱与 Escape 由 Astryx 负责）、`ui/sheet.css`、`ui/drawer.css`、`@radix-ui/react-dialog` 依赖。

## 2026-11-16 逐组件复核

对照包内每个组件的 `*.doc.mjs`（props、bestPractices、anatomy）逐个检查了业务层的实际调用，修掉了这些系统性问题：

| 问题 | 表现 | 处理 |
|---|---|---|
| 图标塞进 `children` | Astryx `Button` 会把 children 放进 block 级 label span，图标和文字没有间距（侧栏、工具栏、列表行都中过） | 统一走 `Button icon` 插槽；纯图标按钮改 `IconButton` |
| 原生 `title` 当提示 | 浏览器 tooltip，和 Astryx 的无障碍/主题不一致 | `ui/button.tsx` 把 `title` 映射到 Astryx `tooltip`，调用点也可显式写 `tooltip` |
| `IconButton` 缺 `tooltip` | 文档明确要求图标按钮给视觉用户 hover 提示 | 全部补齐；`label` 只留给读屏 |
| 手绘对话框头 | 标题是裸 `<h2>`，关闭按钮是 `Button` 包 `<X>`，`aria-labelledby` 由适配层自己接 | 改用 Astryx `DialogHeader`（自动聚焦标题、命名、关闭按钮带 tooltip） |
| 确认框不是 `AlertDialog` | 破坏性操作只用普通 Dialog + 两个按钮，遮罩可点、没有 alertdialog 语义 | `ConfirmDialog` 改 `AlertDialog`；取消是初始焦点，遮罩不可关 |
| 表单对话框没 `purpose` | 误点遮罩会丢输入 | 表单/编辑类 Dialog 传 `purpose="form"` |
| 底部抽屉高度写死 | `height="auto"` / `85dvh` 不是 Astryx 的高度语义 | 改为 `hug` / `capped` / `tall` |
| `Field` 包 `TextInput` | 文档明确说 TextInput 已有 Field 壳，不能再包一层 | 删掉 Settings 的 `Field` 包装，`Input` 直接传 `label` |
| `Toolbar` 子按钮各自设 `size` | 文档要求 size 设在 Toolbar 上一次级联 | Toolbar 统一 `size="sm"`，子控件去掉 size |
| 用 `Button` + `aria-pressed` 当开关 | 文档要求持久开关用 `ToggleButton` | 硬盘「全部」改 `ToggleButton`；显示 Cookie 是瞬时动作，去掉 `aria-pressed` 保留 `aria-label` |

复核后仍有五类**产品取舍**（不是 API 用错，未强行改）：

1. **标签关闭不再有可见入口**：`Tab` 是一个按钮，`endContent` 不能放关闭按钮，所以每个标签上的 X 去掉了；关闭只剩右键菜单（关闭/关闭其他/关闭右侧）和中键。分组容器也随 `TabList` 扁平化，组色改为前置圆点，组不能再折叠。
2. **`MoreMenu` 没有 footer/标题**：移动动作面板的视频名标题、点赞次数说明和“源视频不会被删除”底部说明不再出现在菜单里；点赞次数并入菜单项文字，删除说明由确认框承担。
3. **`Badge` 承载元数据**：时长、数量、哈希、版本数用了 Badge，文档建议元数据用普通文本。它们同时承担分组/识别的视觉作用，暂按分类标签处理。
4. **导航仍用 `Button`**：桌面侧栏、移动抽屉、Settings 分类导航的导航行是 `Button`；文档建议用链接/`SideNav`/`NavItem`。侧栏的拖拽、分组嵌套是业务状态机，已在迁移计划里记为显式例外。
5. **虚拟滚动网格用数字 `columns`**：列数必须和虚拟行窗口的数学一致，保留为性能例外；`Grid` 的响应式 `{minWidth,max}` 暂不冒险。

## 刻意没有替换的

- **`components/Sidebar.tsx` 的导航行**：分组树的拖拽排序/嵌套/投放规则是业务状态机；视觉行仍用 `Button`，后续若要换 `SideNav` 需先验证 DnD。`ui/sidebar.tsx` 只是无交互的 aside/nav/section 容器。
- **`ui/error-boundary.tsx`**：React 错误边界必须是 class，Astryx 没有等价物；fallback 已改为 `EmptyState` + `Button`。
- 其它已替换的 `NavIcon`、业务徽标、StickyBar、移动端 States、旧 `menu`/`flyout-shell`、`MetadataPopup` 自绘定位、`SheetAction` 不再保留。
- Phase 1–9 已完成；业务层裸 `<button>` 只剩 `GuidedTour` 的 2 个聚光灯控制，裸 `<input>`/`<select>`/`<textarea>` 为 0（`ui/input.tsx` 除外）。

## 复验方法

`scripts/ui-check.mjs` + 一个 spike 页面是一次性验证工具，未进版本库：起 dev server 后在 1× / 1.5× 下打开对话框、底部抽屉、侧边抽屉、toast，并断言 `dialog:modal`、`aria-labelledby` 指向标题、`label[for]` 关联、受控输入回写、Escape 关闭。要复验时照本文件第一二节的 layer 顺序起页面即可。
