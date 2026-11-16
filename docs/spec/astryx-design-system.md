# Astryx 设计系统接入

决定：引入 Meta 的 Astryx（`@astryxdesign/core` 0.7.15 + `theme-neutral`）作为设计系统，但**只从 `frontend/src/ui/` 暴露**，业务组件不直接 import。本文记录已验证的事实、必须遵守的两条规则，以及分批采用的顺序。

> 全量替换自绘组件的逐项映射与分阶段方案见 [`astryx-migration-plan.md`](./astryx-migration-plan.md)。本文记录当前已采用的边界。2026-11-16 已完成 Phase 1 + Phase 2（移动端片库与原子层），桌面卡片/菜单/导航仍按计划迁移。

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
| `button.tsx` | `Button` | 不变（`variant` / `size` / children） | 文字经 `children` 传入；`label` 取 children 的纯文本，图标+文字按钮由 Astryx 自动产出 `aria-label`。`default→secondary`、`danger→destructive` |
| `input.tsx` | `TextInput` | 不变（原生 input props） | 事件转回原生 `onChange(event)`；`id` 透传给真输入框，外部 `<label htmlFor>` 关联保留；`number` 无对应字段，降级为 `text + inputMode=numeric` |
| `dialog.tsx` | `Dialog` | 不变（`open`/`onOpenChange` + Header/Title/Description/Footer） | 宽度改为 `<Dialog width>`（Astryx 用内联自定义属性写宽度，`w-[…]` 类打不过）；关闭按钮与 `aria-labelledby` 由本层补 |
| `toast.tsx` | `Toast` + `ToastViewport` | 不变（受控 `toasts`） | 卡片用 `renderContent` 保住左色条与详情列表；堆叠与进动画交给 Astryx，锚点仍由本层定位（viewport 不脱离文档流） |
| `drawer.tsx` | `Drawer` | 不变 | `side` 映射 `start`/`end`；行改为 `Button` |
| `sheet.tsx` | `BottomSheet` + `Dialog` | 不变 | `ConfirmSheet` 改用 Astryx Dialog；行改为 `Button` |
| `astryx.tsx` | `Badge`、`EmptyState`、`Spinner`、`Skeleton`、`Banner`、`Toolbar`、`SegmentedControl`、`ToggleButtonGroup`、`Slider`、`ClickableCard`、`AspectRatio`、`Grid`、`List`/`ListItem`、`Section`、`Divider` 等 | 直接 re-export | 业务层只经这个 barrel 取用；`toggleChipClass` 统一多选 Chip 的静态样式 |
| `icon.tsx` | `Icon` + `registerIcons` | `name` 仍用 kebab-case | Muse SVG 注册为 `muse:*`；menu/close/warning/funnel/chevron 等直接映射 Astryx 内置语义名 |
| `muse-icons.tsx` | — | — | Muse 自有 SVG 路径转成组件，交给 `Icon` 渲染；资产仍是唯一图形来源 |

随之删除：`ui/NavIcon.tsx`、`ui/backup-badge.tsx`、`ui/restored-badge.tsx`、`ui/sticky-bar.tsx`、`mobile/components/States.tsx`、`components/EmptyState.tsx`。

随之删除的历史文件：`ui/overlay.ts`（聚焦陷阱与 Escape 由 Astryx 负责）、`ui/sheet.css`、`ui/drawer.css`、`@radix-ui/react-dialog` 依赖。

## 刻意没有替换的

- **`ui/sidebar.tsx`**：分组树的拖拽排序/嵌套/投放规则是业务状态机；视觉行后续换 `SideNav`/`TreeList`（Phase 5）。
- **`WorkspaceTabs`**：标签组色、折叠/展开、关闭按钮与拖拽顺序是业务状态；Astryx `TabList`/`Tab` 的关闭 `endContent` 会形成嵌套按钮，迁移留到 Phase 5 处理。
- **`ui/error-boundary.tsx`**：React 错误边界必须是 class，Astryx 没有等价物；fallback 已改为 `EmptyState` + `Button`。
- 其它已替换的 `NavIcon`、业务徽标、StickyBar、移动端 States、旧 `menu`/`flyout-shell`、`MetadataPopup` 自绘定位、`SheetAction` 不再保留。
- Phase 3/4 的桌面卡片/列表、菜单、HoverCard、移动端动作面板与表单已改用 Astryx；Phase 5/6 的 `Sidebar`、`WorkspaceTabs`、业务页面仍在收尾。

## 复验方法

`scripts/ui-check.mjs` + 一个 spike 页面是一次性验证工具，未进版本库：起 dev server 后在 1× / 1.5× 下打开对话框、底部抽屉、侧边抽屉、toast，并断言 `dialog:modal`、`aria-labelledby` 指向标题、`label[for]` 关联、受控输入回写、Escape 关闭。要复验时照本文件第一二节的 layer 顺序起页面即可。
