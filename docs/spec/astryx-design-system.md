# Astryx 设计系统接入

决定：引入 Meta 的 Astryx（`@astryxdesign/core` 0.7.15 + `theme-neutral`）作为设计系统，但**只从 `frontend/src/ui/` 暴露**，业务组件不直接 import。本文记录已验证的事实、必须遵守的两条规则，以及分批采用的顺序。

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
| `button.tsx` | `Button` | 不变（`variant` / `size` / children） | 文字经 `children` 传入；`label` 取 children 的纯文本，图标+文字按钮由 Astryx 自动产出 `aria-label`。`default→secondary`、`danger→destructive` |
| `input.tsx` | `TextInput` | 不变（原生 input props） | 事件转回原生 `onChange(event)`；`id` 透传给真输入框，外部 `<label htmlFor>` 关联保留；`number` 无对应字段，降级为 `text + inputMode=numeric` |
| `dialog.tsx` | `Dialog` | 不变（`open`/`onOpenChange` + Header/Title/Description/Footer） | 宽度改为 `<Dialog width>`（Astryx 用内联自定义属性写宽度，`w-[…]` 类打不过）；关闭按钮与 `aria-labelledby` 由本层补 |
| `toast.tsx` | `Toast` + `ToastViewport` | 不变（受控 `toasts`） | 卡片用 `renderContent` 保住左色条与详情列表；堆叠与进动画交给 Astryx，锚点仍由本层定位（viewport 不脱离文档流） |
| `drawer.tsx` | `Drawer` | 不变 | `side` 映射 `start`/`end`；行改为 `Button` |
| `sheet.tsx` | `BottomSheet` + `Dialog` | 不变 | `ConfirmSheet` 改用 Astryx Dialog；行改为 `Button` |

随之删除：`ui/overlay.ts`（聚焦陷阱与 Escape 由 Astryx 负责）、`ui/sheet.css`、`ui/drawer.css`、`@radix-ui/react-dialog` 依赖。

## 刻意没有替换的

- **`ui/menu.tsx` + `ui/flyout-shell.tsx`**：右键菜单在**任意坐标**打开，且带 hover-intent 子面板（`renderFlyout(anchor)` 自定义 JSX）。Astryx 的 `ContextMenu` 是「在触发区域右键」，子菜单走条目数据，替换等于重写三个菜单包装器与两个 flyout 组件，收益不抵风险。行本身已用 Astryx `Button`。
- **`ui/sidebar.tsx`、`ui/sticky-bar.tsx`、`ui/NavIcon.tsx`、`backup-badge.tsx`、`restored-badge.tsx`、`error-boundary.tsx`**：这些是 Muse 自己的外壳与业务徽标，不是设计系统里的基元，替换会改掉信息架构而不是样式。

## 复验方法

`scripts/ui-check.mjs` + 一个 spike 页面是一次性验证工具，未进版本库：起 dev server 后在 1× / 1.5× 下打开对话框、底部抽屉、侧边抽屉、toast，并断言 `dialog:modal`、`aria-labelledby` 指向标题、`label[for]` 关联、受控输入回写、Escape 关闭。要复验时照本文件第一二节的 layer 顺序起页面即可。
