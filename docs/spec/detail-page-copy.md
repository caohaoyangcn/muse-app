# 详情页文本可选中复制

> 桌面 `VideoDetailPanel` 与局域网移动端 `VideoDetailView` 的正文文本可以用鼠标拖选或长按选中，再用 `Ctrl+C` / 系统「拷贝」带走。壳层其余部分不参与本文的规则。

## 背景

壳层在 `frontend/src/style.css` 与 `frontend/src/mobile/mobile.css` 里对 `html, body` 设置了 `user-select: none`：收藏卡片、侧栏、标签条在点击、拖拽和滚动时不该出现蓝色选区。详情页是**阅读面**，用户最常做的动作反而是把番号、标题、路径抄走；全局禁选让这件事做不到。

## 行为

- 详情页正文整体恢复 `user-select: text`：片名 / 番号 / 标题 / 中文译名 / 制作商 / 系列 / 演员 / 标签 / 刮削状态 / 文件信息 / 路径 / 备注。
- 桌面端用鼠标拖选或双击选词，再按 `Ctrl+C` 复制（Wails 生产构建关闭了 WebView 的默认右键菜单，所以没有右键「复制」；`Ctrl+C` 由 WebView 自己处理）；移动端长按选中，走系统的「拷贝」菜单（iOS Safari / Android Chrome 的原生行为）。
- 全局禁选保持不变：只给详情页根节点加 `select-text`，不修改 `body`，所以列表、卡片、侧栏、对话框的交互面不受影响。
- 路径在桌面端同时是「在文件管理器中定位 / 在 OpenList 中打开」的按钮。拖选结束后浏览器仍会向按钮派发一次 `click`；点击处理先检查当前选区，**选区非空时不执行定位**，这样复制路径不会顺带弹出资源管理器。普通点击（按下即松开，选区已折叠）仍然定位文件。
- 移动端不提供「定位 / 打开所在文件夹」（作用于服务器文件系统），因此没有点击消歧问题。

## 实现位置

- `frontend/src/components/VideoDetailPanel.tsx`：根节点 `select-text`；路径按钮的 `hasActiveSelection()` 守卫。
- `frontend/src/mobile/components/VideoDetailView.tsx`：根节点 `select-text`。
- `frontend/src/mobile/mobile.css`：全局 `user-select: none` 的原有规则不变（输入框早有一条恢复选择的例外）。
- `docs/spec/mobile-web-design.md` §5 安全区一行同步说明详情页例外。

## 非目标

- 不为单个字段加「复制」按钮：系统选区与复制菜单已经够用，再加按钮会让信息密度变高的详情页继续膨胀。
- 不放开卡片、列表、侧栏、标签条的选择：那里要的是点击与拖拽手感。
- 不新增右键菜单：桌面 Wails 生产构建关闭了 WebView 默认右键菜单，本次不为复制另开一套菜单；复制走 `Ctrl+C`。移动端交给系统长按菜单。
- 不改变 `Ctrl+A` 的语义：它仍由 WebView / 浏览器处理，Muse 不把它接到「全选整个收藏库」之类的应用动作上。
