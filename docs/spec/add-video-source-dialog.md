# 添加视频来源弹窗实施计划

**状态：** Proposed（未实现）
**范围：** 桌面端顶部「添加视频」入口、来源选择弹窗、一次性目录扫描、网盘入口的交接
**非范围：** 网盘/WebDAV 后端（见 [Muse WebDAV 云盘片源](webdav-cloud-source.md)）、OpenList BitQiu 驱动（见 [OpenList fork 与 BitQiu 驱动](openlist-bitqiu-driver.md)）、移动端
**依赖：** 网盘卡片依赖 [Muse WebDAV 云盘片源](webdav-cloud-source.md) 的连接与浏览 API；本地/扫描两项可先独立交付
**最后更新：** 2026-11-05

---

## 1. 背景与已确认决策

现状：顶部 `＋ 添加视频` 直接调用 `pickVideoFiles()`，只支持本地文件。目录监控在设置里，网盘入口尚不存在。

目标：点「添加视频」先进入一个来源选择弹窗，把「本地文件 / 扫描文件夹 / 从网盘添加」放在同一个入口下。

已确认的交互决策：

- 选择「本地文件」或「扫描文件夹」后，**先关闭来源弹窗，再打开系统选择器**；取消选择器就回到主界面，不保留弹窗。
- 弹窗内**不再做拖拽区**；保留现有「把文件拖进窗口」的全局能力。
- 没有任何网盘连接时，**仍然显示「从网盘添加」卡片**，进入后给「去设置 → 网盘」入口，不强迫安装 OpenList。
- 「扫描文件夹」是**一次性导入**，不写入目录监控；持续监控仍在「设置 → 目录监控」。
- 不采用分裂按钮/下拉菜单；这个入口就是弹窗。

## 2. 目标与非目标

### 目标

- 一个来源无关的添加入口，三种来源的文案、状态、下一步一致。
- 本地高频操作仍然快：弹窗打开后本地卡片默认聚焦，`1`/`Enter` 即可进入文件选择器。
- 目录扫描与目录监控语义分开，不让用户以为「扫一次」就多了一个后台监控目录。
- 没有网盘或 OpenList 的用户完全不受影响，不出现安装提示或错误。
- 弹窗打开时拖入文件仍能添加，且不出现两层遮罩或重复添加。

### 非目标

- 不在弹窗里直接列出文件、目录或网盘内容；选择后进入各自流程。
- 不做扫描进度条/取消（v1）；扫描期间只显示状态。
- 不改变本地文件选择器、拖拽添加、目录监控的既有行为。
- 不改变移动端（移动端本来就不提供添加入口）。

## 3. 交互与视觉规格

### 3.1 流程

```text
＋ 添加视频
   ↓
AddSourceDialog
   ├─ 本地文件   → 关闭弹窗 → pickVideoFiles() → addVideos()
   ├─ 扫描文件夹 → 关闭弹窗 → pickScanDir()   → scanDirectoryOnce()
   └─ 从网盘添加 → 关闭弹窗 → CloudBrowserDialog
```

- 本地/扫描选择后不保留来源弹窗；系统选择器取消 = 本次操作结束，无错误提示。
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
│              一次性导入；持续监控去设置│
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
| 扫描 | 扫描文件夹 | 一次性导入文件夹中的视频；持续监控请到设置 |
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
| 扫描目录很大 | 显示「正在扫描 <目录名>…」；禁止再次启动扫描；v1 不做取消 |
| 扫描到 0 个新视频 | 成功提示「未发现新视频」 |
| 扫描中文件被占用 | 计入失败并在汇总中列出；不自动重试 |

## 4. 组件与代码结构

### 4.1 前端

新增 `frontend/src/components/AddSourceDialog.tsx`：

```ts
interface AddSourceDialogProps {
  open: boolean;
  cloud: {
    loading: boolean;
    count: number;
    onlineCount: number;
    error: string | null;
  };
  scanning: boolean;
  onPickLocal: () => void;
  onPickFolder: () => void;
  onPickCloud: () => void;
  onClose: () => void;
}
```

组件只负责展示、键盘和焦点；不直接调用 Wails 绑定，便于后续测试/复用。

`frontend/src/pages/Library.tsx`：

- 现有 `handleAdd` 改为 `openAddSource`。
- 新增 `addSourceOpen`、`scanningDir` 状态。
- `handleAddLocal`：关闭弹窗 → `pickVideoFiles` → `addVideos` → `showAddSummary` / `showGroupSuggestions` / `invalidate`。
- `handleScanDirectory`：关闭弹窗 → `pickScanDir` → `scanDirectoryOnce` → 汇总/建议/刷新。
- `handleAddCloud`：关闭弹窗 → 打开 `CloudBrowserDialog`。
- 全局拖拽 handler：如果 `addSourceOpen` 为真，先关闭弹窗再走原添加逻辑。
- 空收藏库的 CTA 仍打开同一个来源弹窗，不再直接调用本地选择器。

`frontend/src/services/api.ts`：

- 新增 `pickScanDir`、`scanDirectoryOnce`。
- 网盘相关沿用 `listCloudConnections` / `browseCloud` / `addCloudVideos`。

### 4.2 后端：一次性目录扫描

新增接口放在 `backend/watch`，因为它已经拥有目录遍历、跳过系统目录、可读性判断和 `AddVideos` 入口：

```go
type ScanOnceResult struct {
    Path      string
    Added     int
    Relocated int
    Failed    int
    Results   []video.AddVideoResult
}

func (s *Service) ScanOnce(path string) (ScanOnceResult, error)
```

实现要点：

- 把 `scanRoot` 中的遍历逻辑抽成 `collectCandidates(root string, existing map[string]struct{})`，供 `scanRoot` 和 `ScanOnce` 共用。
- `ScanOnce` 使用 `normalizeDir`、`existingPaths`、`collectCandidates`，调用 `lib.AddVideos`，统计成功/重连/失败并返回逐文件结果。
- **不写入 `watched_dirs`、不注册 fsnotify、不更新 `last_scan_at`、不发 `watch:scanned`。**
- 不调用 `AutoTagVideos`；一次性扫描按手动批量添加处理，由前端展示分组建议。
- 复用 `canImport`：读不到的文件计入失败，不重试。
- 在 `backend/api/watch.go` 增加 `ScanDirectoryOnce(path)`；在 `app.go` 增加 `ScanDirectoryOnce`、`PickScanDir`（标题「选择要扫描的目录」），不改 `PickWatchDir`。

## 5. 实施步骤

### Phase 0：文档与接口约定

- [ ] 本计划评审通过，确认三个来源的文案与语义。
- [ ] 确认 `ScanOnceResult` 字段；前端汇总可直接复用 `AddVideoResult`。
- [ ] 确认 `PickScanDir` 与 `PickWatchDir` 分开，避免选择器标题串台。

### Phase 1：一次性目录扫描后端

- [ ] 抽出 `collectCandidates`，保证 `scanRoot` 行为不变。
- [ ] 实现 `watch.Service.ScanOnce`。
- [ ] `api.Library.ScanDirectoryOnce`。
- [ ] `App.ScanDirectoryOnce` / `App.PickScanDir`。
- [ ] 单测：添加、跳过已收藏、跳过隐藏/系统目录、失败计数、不创建监控项。
- [ ] `go test ./backend/watch ./backend/api ./...`。

### Phase 2：来源弹窗组件

- [ ] 新建 `AddSourceDialog.tsx`，实现三张卡片、键盘、焦点、footer。
- [ ] 接入 `Dialog` 原语，保持现有深色 token。
- [ ] 网盘卡片的 loading/count/offline/error 四种描述。
- [ ] 窄窗口检查。

### Phase 3：接入 Library

- [ ] `addSourceOpen` 状态与打开/关闭逻辑。
- [ ] 本地、扫描、网盘三个 handler。
- [ ] 拖拽时关闭弹窗并继续添加。
- [ ] 扫描中的状态提示与禁止重复扫描。
- [ ] 空库 CTA 复用同一入口。

### Phase 4：网盘交接

- [ ] `CloudBrowserDialog` 空状态加「去设置 → 网盘」。
- [ ] 连接数读取在弹窗打开时触发，失败不阻塞。
- [ ] 从来源弹窗进入网盘后，关闭来源弹窗，不叠加两层遮罩。

### Phase 5：文案、无障碍、引导

- [ ] 更新新手引导中「添加视频」的说明为「选择来源」。
- [ ] 检查 `aria-describedby`、焦点环、Esc、键盘顺序。
- [ ] 更新 `PRODUCT.md` 的能力清单（来源选择弹窗）。

### Phase 6：验证

- [ ] `cd frontend && npm run lint && npm run build`。
- [ ] `go test ./...`。
- [ ] 手测矩阵见 §6。
- [ ] 实现完成后运行一次 Impeccable 机械检测：`impeccable detect --json frontend/src/components/AddSourceDialog.tsx frontend/src/pages/Library.tsx`。

## 6. 测试与验收

### 手测矩阵

| 场景 | 期望 |
| --- | --- |
| 点添加视频 | 打开来源弹窗，不直接弹系统文件选择器 |
| 点本地文件 | 弹窗关闭，系统文件选择器打开；取消后无错误 |
| 点扫描文件夹 | 弹窗关闭，系统目录选择器打开；选择后开始扫描 |
| 扫描一个含 3 个新视频的目录 | 汇总「已添加 3 个视频」；目录监控列表不增加 |
| 扫描一个空目录 | 提示「未发现新视频」 |
| 重复扫描同一目录 | 不重复添加；提示未发现新视频 |
| 无网盘连接时点网盘 | 进入空状态，能到设置；不出现安装强制提示 |
| 弹窗打开时拖入文件 | 弹窗关闭，文件被添加；无重复卡片 |
| 键盘 1/2/3、方向键、Enter、Esc | 全部可用，焦点可见 |
| 520px/窄窗口 | 卡片和 footer 不溢出、不重叠 |

### 验收标准

- `＋ 添加视频` 不再直接打开本地文件选择器。
- 三种来源都能完成各自流程，取消路径无副作用。
- 扫描是一次性的；现有目录监控行为完全不变。
- 无网盘/OpenList 的用户不被打扰。
- lint/build 通过；无新增控制台错误。

## 7. 提交拆分

| 顺序 | 提交信息（建议） | 内容 |
| --- | --- | --- |
| 1 | `Add one-off directory scan for add-source flow` | `watch.ScanOnce`、API、bindings、测试 |
| 2 | `Add add-source chooser dialog` | `AddSourceDialog.tsx`、样式、键盘 |
| 3 | `Wire add-source chooser into library` | `Library.tsx` handlers、拖拽、扫描状态 |
| 4 | `Connect cloud source to add-source dialog` | `CloudBrowserDialog` 空状态、网盘交接 |
| 5 | `Document add-source chooser` | 本文件状态更新、PRODUCT.md、引导文案 |

每个提交单独跑对应范围的测试；文档提交可以与前一项合并，但不要把后端扫描和前端弹窗攒成一个大提交。

## 8. 风险与取舍

- **多一步点击**：这是用户明确要求。用默认聚焦、数字键、Enter、拖拽兜底降低摩擦；不再额外加「最近使用」之类的隐藏状态。
- **弹窗疲劳**：这个弹窗只在添加入口出现，且三项都是真实分叉；不把文件列表/网盘浏览塞进来，避免变成万能弹窗。
- **扫描阻塞**：v1 同步遍历，大目录可能等待较久；先显示状态，后续再加进度/取消。
- **语义混淆**：扫描 vs 监控必须在文案上分开；扫描结果里不出现「已加入监控」。
- **云盘可选**：没有连接时卡片仍然出现，可能让本地用户多看到一个「可选」项；这是为了让网盘功能可发现，但不强制。

## 9. 进度记录

| 日期 | 状态 | 说明 | 提交 |
| --- | --- | --- | --- |
| 2026-11-05 | Planned | 计划文档建立 | `<pending>` |
