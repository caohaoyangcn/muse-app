# 添加视频来源弹窗实施计划

**状态：** In progress（本地文件 / 扫描文件夹已实现；网盘待接入）
**范围：** 桌面端顶部「添加视频」入口、来源选择弹窗、扫描文件夹与目录监控的统一、网盘入口的交接
**非范围：** 网盘/WebDAV 后端（见 [Muse WebDAV 云盘片源](webdav-cloud-source.md)）、OpenList BitQiu 驱动（见 [OpenList fork 与 BitQiu 驱动](openlist-bitqiu-driver.md)）、移动端
**依赖：** 网盘卡片依赖 [Muse WebDAV 云盘片源](webdav-cloud-source.md) 的连接与浏览 API；本地/扫描两项已经独立可用
**最后更新：** 2026-11-05

---

## 1. 背景与已确认决策

现状：顶部 `＋ 添加视频` 原本直接调用 `pickVideoFiles()`，只支持本地文件。目录监控只在设置里，网盘入口尚不存在。

目标：点「添加视频」先进入一个来源选择弹窗，把「本地文件 / 扫描文件夹 / 从网盘添加」放在同一个入口下。

已确认的交互决策：

- 选择「本地文件」或「扫描文件夹」后，**先关闭来源弹窗，再打开系统选择器**；取消选择器就回到主界面，不保留弹窗。
- 弹窗内**不再做拖拽区**；保留现有「把文件拖进窗口」的全局能力。
- 没有任何网盘连接时，**仍然显示「从网盘添加」卡片**，进入后给「去设置 → 网盘」入口，不强迫安装 OpenList。
- **「扫描文件夹」不是一次性动作**：它等同于「设置 → 目录监控 → 添加目录」，会持久化监控目录、立即扫描，并持续收录之后新增的视频。来源弹窗只是把这个动作放到添加入口旁边。
- 不采用分裂按钮/下拉菜单；这个入口就是弹窗。

> 2026-11-05 修订：最初实现过一次性 `watch.ScanOnce`，现按上述决策删除；扫描文件夹统一复用 `watch.Service.Add` / `AddWatchDir`。

## 2. 目标与非目标

### 目标

- 一个来源无关的添加入口，三种来源的文案、状态、下一步一致。
- 本地高频操作仍然快：弹窗打开后本地卡片默认聚焦，`1`/`Enter` 即可进入文件选择器。
- 「扫描文件夹」与「目录监控」是同一套语义和同一套持久化：添加后出现在设置 → 目录监控列表，之后新增文件自动收录。
- 没有网盘或 OpenList 的用户完全不受影响，不出现安装提示或错误。
- 弹窗打开时拖入文件仍能添加，且不出现两层遮罩或重复添加。

### 非目标

- 不在弹窗里直接列出文件、目录或网盘内容；选择后进入各自流程。
- 不做一次性扫描模式；不再维护第二条「只扫描、不监控」的代码路径。
- 不改变本地文件选择器、拖拽添加、目录监控的既有行为（除了新增一个入口）。
- 不改变移动端（移动端本来就不提供添加入口）。

## 3. 交互与视觉规格

### 3.1 流程

```text
＋ 添加视频
   ↓
AddSourceDialog
   ├─ 本地文件   → 关闭弹窗 → pickVideoFiles() → addVideos()
   ├─ 扫描文件夹 → 关闭弹窗 → pickWatchDir()   → addWatchDir()
   │                              （写入目录监控，后台扫描，watch:scanned 报告结果）
   └─ 从网盘添加 → 关闭弹窗 → CloudBrowserDialog
```

- 本地/扫描选择后不保留来源弹窗；系统选择器取消 = 本次操作结束，无错误提示。
- 扫描文件夹添加成功后立即提示「已加入目录监控，正在扫描…」；扫描结果由现有的 `watch:scanned` 事件提示并刷新列表。
- 重复选择已在监控清单中的目录：提示「该目录已在目录监控中」，不重复添加。
- 网盘选择后来源弹窗关闭，再打开网盘浏览弹窗，避免两层遮罩叠加。

### 3.2 弹窗布局

```text
┌──────────────────────────────────────┐
│ 添加视频                          ×  │
│ 选择来源                             │
│                                      │
│ [FileVideo]  本地文件            1 › │
│              选择一个或多个视频文件   │
│                                      │
│ [FolderSearch] 扫描文件夹        2 › │
│              加入目录监控并立即扫描   │
│                                      │
│ [Cloud]      从网盘添加      可选 3 › │
│              未配置；进入后可去设置   │
│                                      │
│ 1 本地 · 2 文件夹 · 3 网盘  Esc 关闭 │
│ 也可以直接把文件拖进窗口              │
└──────────────────────────────────────┘
```

- 使用现有 [Dialog](../../frontend/src/components/ui/dialog.tsx) 原语；宽度 `520px`，最大宽度 `calc(100vw - 48px)`，居中，最大高度内滚动。
- 内容 `p-0`：header `px-5 pt-5 pb-3`、列表 `px-5 pb-2`、footer `border-t px-5 py-3`；header 右侧留 `pr-10` 给关闭按钮。
- 来源卡片整行是 `<button>`，最小高度 `64px`，`rounded-[var(--radius)]`，`border border-[var(--border)]`，`bg-[var(--panel)]`。
- hover/focus：`border-[var(--accent)]`，背景 `--panel-2`；不用整块紫色填充。
- 图标底座 `40×40`，`rounded-[10px] bg-[var(--panel-2)] text-[var(--accent)]`，lucide 18px：`FileVideo`、`FolderSearch`、`Cloud`。
- 标题 14px 中等字重；描述 12px `--muted`；右侧 `ChevronRight` 16px，hover 变 accent。
- 卡片间距 `gap-2`；网盘卡片右上角一个小号「可选」标签，保持低调。
- footer 使用 11–12px `--muted`；键位用现有 `<kbd>` 样式（与设置页快捷键一致）。
- 不新增装饰性动效；沿用 Dialog 原语显隐，尊重 `prefers-reduced-motion`。
- 窄窗口：描述允许换行，footer 允许换行，不横向溢出。

### 3.3 文案

| 来源 | 标题 | 描述 |
| --- | --- | --- |
| 本地 | 本地文件 | 选择一个或多个视频文件 |
| 扫描 | 扫描文件夹 | 加入目录监控并立即扫描；之后新增的视频会自动收录 |
| 网盘 | 从网盘添加 | 有连接：`N 个连接可用`；无连接：`未配置；进入后可去设置`；加载中：`正在检查网盘连接…`；读取失败：`无法读取网盘连接；仍可进入设置` |

- 标题：`添加视频`；描述：`选择来源`。
- footer：`1 本地 · 2 文件夹 · 3 网盘`，右侧 `Esc 关闭`；下面一行 `也可以直接把文件拖进窗口`。
- 不使用「必须安装 OpenList」「立即下载」等强制性文案。

### 3.4 键盘与焦点

- 打开时焦点落在「本地文件」卡片。
- `Tab`/`Shift+Tab` 或上下方向键在三张卡片间移动。
- `1`/`2`/`3` 直接选择；`Enter`/`Space` 激活当前卡片；`Esc` 关闭。
- 关闭后焦点回到 `＋ 添加视频` 按钮（Radix Dialog 默认行为）。
- 卡片有清晰 `focus-visible` 环；图标和文字均包含在按钮可点击区域内。

### 3.5 状态与边界

| 场景 | 行为 |
| --- | --- |
| 系统文件/文件夹选择器取消 | 弹窗已关闭，回到主界面，无错误 |
| 没有任何网盘连接 | 网盘卡片显示「未配置」，可进入空状态并去设置 |
| 网盘连接全部离线 | 显示「N 个连接 · 离线」，仍可进入查看/重试 |
| 网盘连接读取失败 | 显示「无法读取网盘连接；仍可进入设置」 |
| 弹窗打开时拖入文件 | 关闭弹窗，走现有全局拖拽添加；不重复添加 |
| 扫描文件夹已存在监控清单 | 提示「该目录已在目录监控中」，不重复添加 |
| 扫描文件夹添加成功 | 立即提示「已加入目录监控，正在扫描…」；结果由 `watch:scanned` 汇总 |
| 目录很大 | 扫描在后台进行，不阻塞来源弹窗；设置 → 目录监控可看到上次扫描时间 |

## 4. 组件与代码结构

### 4.1 前端

`frontend/src/components/AddSourceDialog.tsx`：

```ts
interface AddSourceDialogProps {
  open: boolean;
  onPickLocal: () => void;
  onPickFolder: () => void;
  onPickCloud?: () => void; // WebDAV 接入后启用
  cloud?: CloudSummary | null;
  onClose: () => void;
}
```

组件只负责展示、键盘和焦点；不直接调用 Wails 绑定，便于后续测试/复用。扫描文件夹卡片不再有「扫描中」禁用态，因为它现在走目录监控的异步扫描。

`frontend/src/pages/Library.tsx`：

- 现有 `handleAdd` 改为 `openAddSource`。
- 新增 `addSourceOpen` 状态。
- `handleAddLocal`：关闭弹窗 → `pickVideoFiles` → `addVideos` → `showAddSummary` / `showGroupSuggestions` / `invalidate`。
- `handleAddFolder`：关闭弹窗 → `pickWatchDir` → `addWatchDir`；成功 toast「已加入目录监控，正在扫描…」；重复时提示已在监控中；结果由 `watch:scanned` 事件处理。
- `handleAddCloud`：关闭弹窗 → 打开 `CloudBrowserDialog`（待 WebDAV 后端）。
- 全局拖拽 handler：如果 `addSourceOpen` 为真，先关闭弹窗再走原添加逻辑。
- 空收藏库的 CTA 仍打开同一个来源弹窗，不再直接调用本地选择器。

`frontend/src/services/api.ts`：

- 复用 `pickWatchDir`、`addWatchDir`，不新增一次性扫描 API。
- 网盘相关沿用 `listCloudConnections` / `browseCloud` / `addCloudVideos`。

### 4.2 后端：复用目录监控

不再新增一次性扫描接口。`扫描文件夹` 直接调用现有的：

- `backend/watch.Service.Add(path)`：规范化目录、写入 `watched_dirs`、注册 fsnotify、入队首次扫描、发出 `watch:changed`。
- `backend/api.Library.AddWatchDir(path)`。
- `app.go` 的 `App.AddWatchDir` 与 `App.PickWatchDir`（标题「选择要监控的目录」）。

保留的纯重构：`scanRoot` 的遍历逻辑抽成 `collectCandidates(root, existing)`，让首次扫描、周期扫描和 fsnotify 触发共用同一套跳过规则；它不再服务于第二套一次性扫描。

## 5. 实施步骤

### Phase 0：文档与接口约定

- [x] 本计划评审通过，确认三个来源的文案与语义。
- [x] 确认「扫描文件夹」= 添加目录监控，不新增一次性 API。
- [x] 删除 `ScanOnce` / `ScanOnceResult` / `ScanDirectoryOnce` / `PickScanDir` 及其测试/绑定。

### Phase 1：扫描文件夹复用目录监控

- [x] 抽出 `collectCandidates`，保证 `scanRoot` 行为不变。
- [x] 删除一次性 `watch.ScanOnce` 与相关测试。
- [x] `Library.handleAddFolder` 改为 `pickWatchDir` + `addWatchDir`。
- [x] 重复目录、成功/失败 toast。
- [x] `go test ./backend/watch ./backend/api ./...`。

### Phase 2：来源弹窗组件

- [x] 新建 `AddSourceDialog.tsx`，实现本地/扫描卡片、键盘、焦点、footer。
- [x] 接入 `Dialog` 原语，保持现有深色 token。
- [ ] 网盘卡片：组件已预留 `onPickCloud` / `cloud` 与 loading/count/offline/error 文案，等网盘后端接入后启用。
- [x] 窄窗口检查。

### Phase 3：接入 Library

- [x] `addSourceOpen` 状态与打开/关闭逻辑。
- [x] 本地、扫描 handler；网盘 handler 等 Phase 4。
- [x] 拖拽时关闭弹窗并继续添加。
- [x] 空库 CTA 复用同一入口。
- [x] 扫描文件夹的异步结果交给现有 `watch:scanned` 事件，不新增前端扫描状态。

### Phase 4：网盘交接

- [ ] `CloudBrowserDialog` 空状态加「去设置 → 网盘」。
- [ ] 连接数读取在弹窗打开时触发，失败不阻塞。
- [ ] 从来源弹窗进入网盘后，关闭来源弹窗，不叠加两层遮罩。

### Phase 5：文案、无障碍、引导

- [x] 更新新手引导中「添加视频」的说明为「选择来源」，并说明扫描文件夹会持续监控。
- [x] 检查 `aria-describedby`、焦点环、Esc、键盘顺序。
- [x] 更新 `PRODUCT.md` 的能力清单（来源选择弹窗）。

### Phase 6：验证

- [x] `cd frontend && npm run lint && npm run build`。
- [x] `go test ./...`。
- [x] Headless Chromium 布局/键盘检查。
- [x] 实现完成后运行一次 Impeccable 机械检测：`impeccable detect --json frontend/src/components/AddSourceDialog.tsx frontend/src/pages/Library.tsx`（结果为空）。
- [ ] Windows 原生窗口实机检查。

## 6. 测试与验收

### 手测矩阵

| 场景 | 期望 |
| --- | --- |
| 点添加视频 | 打开来源弹窗，不直接弹系统文件选择器 |
| 点本地文件 | 弹窗关闭，系统文件选择器打开；取消后无错误 |
| 点扫描文件夹 | 弹窗关闭，系统目录选择器打开；选择后目录出现在「设置 → 目录监控」 |
| 扫描一个含 3 个新视频的目录 | 后台扫描完成后 `watch:scanned` 提示「已自动添加 3 个视频」 |
| 再次选择同一目录 | 提示「该目录已在目录监控中」，不重复写入 |
| 扫描后在目录里新增一个视频 | 目录监控自动收录（fsnotify 或周期扫描） |
| 无网盘连接时点网盘 | 进入空状态，能到设置；不出现安装强制提示 |
| 弹窗打开时拖入文件 | 弹窗关闭，文件被添加；无重复卡片 |
| 键盘 1/2/3、方向键、Enter、Esc | 全部可用，焦点可见 |
| 520px/窄窗口 | 卡片和 footer 不溢出、不重叠 |

### 验收标准

- `＋ 添加视频` 不再直接打开本地文件选择器。
- 三种来源都能完成各自流程，取消路径无副作用。
- 「扫描文件夹」与「设置 → 目录监控 → 添加目录」产生同一条记录、同一套后续自动收录行为。
- 无网盘/OpenList 的用户不被打扰。
- lint/build 通过；无新增控制台错误。

## 7. 提交拆分

| 顺序 | 提交信息（建议） | 内容 | 状态 |
| --- | --- | --- | --- |
| 1 | `Add one-off directory scan for add-source flow` | 初版 `watch.ScanOnce`（已被下一步替代） | `0800c72` |
| 2 | `Add add-source chooser dialog` | `AddSourceDialog.tsx`、Library 接入 | `d40a9ac` |
| 3 | `Record add-source implementation progress` | 文档进度 | `4c63d61` |
| 4 | `Unify scan folder with folder watch` | 删除一次性扫描，改用 `addWatchDir` | 本次提交 |

## 8. 风险与取舍

- **多一步点击**：这是用户明确要求。用默认聚焦、数字键、Enter、拖拽兜底降低摩擦；不再额外加「最近使用」之类的隐藏状态。
- **弹窗疲劳**：这个弹窗只在添加入口出现，且三项都是真实分叉；不把文件列表/网盘浏览塞进来，避免变成万能弹窗。
- **后台扫描不可见**：添加监控目录后扫描在后台进行，结果只在 `watch:scanned` 时提示；用户可以在设置 → 目录监控查看列表和上次扫描时间。v1 不新增进度条。
- **语义收拢的代价**：来源弹窗不再提供「只扫一次」选项。如果用户只想临时导入，可以先添加监控、扫完再在设置里移除；这比维护两套扫描语义更可预测。
- **云盘可选**：没有连接时卡片仍然出现，可能让本地用户多看到一个「可选」项；这是为了让网盘功能可发现，但不强制。

## 9. 进度记录

| 日期 | 状态 | 说明 | 提交 |
| --- | --- | --- | --- |
| 2026-11-05 | Planned | 计划文档建立 | `9da1586` |
| 2026-11-05 | In progress | 一次性扫描 + 来源弹窗 + Library 接入 | `0800c72` / `d40a9ac` |
| 2026-11-05 | Revised | 扫描文件夹改为添加目录监控，删除一次性扫描路径 | 本次提交 |

## 10. 实现记录（2026-11-05）

- 后端：保留 `collectCandidates` 作为 `scanRoot` 的提取；删除 `watch.ScanOnce`、`ScanOnceResult`、`api.ScanDirectoryOnce`、`App.ScanDirectoryOnce`、`App.PickScanDir` 及对应测试/绑定。
- 前端：`AddSourceDialog` 只有本地/扫描两张卡片；扫描卡片不再有「扫描中」禁用态。网盘卡片代码仍预留，等 WebDAV 后端接入。
- 入口：顶部 `＋ 添加视频` 和空库 CTA 都打开来源弹窗；拖拽仍直接添加，并在弹窗打开时先关闭弹窗。
- 扫描文件夹：`pickWatchDir` → `addWatchDir`；成功提示「已加入目录监控，正在扫描…」，结果由现有 `watch:scanned` 事件提示并刷新。
- 已完成：引导文案、`PRODUCT.md` 能力清单、`go test` / `go vet` / 前端 `lint` + `build`、headless Chromium 布局/键盘检查；点击扫描文件夹确认调用 `PickWatchDir` + `AddWatchDir`，重复目录提示正确。
- 尚未完成：网盘交接、Windows 原生窗口实机检查。
