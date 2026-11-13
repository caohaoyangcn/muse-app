# 桌面端与移动端共享改造计划

> 状态：进行中（阶段 0 实施中）。本文是这次改造的**唯一方向锚点**：范围、分层、阶段划分与验收标准都以此为准；每完成一个阶段，更新 §7 进度看板与 §9 变更记录。
>
> 修订关系：本文**修正** `mobile-web-design.md` §2 的结论。该文档当时判定"不做响应式"，因此移动端独立成一个应用、只共享类型与工具函数。结论中"两套壳必须分开"仍然成立，但"UI 层完全不共享"已被证明代价过高，本文改为共享 L1-L4、保留 L5。

## 1. 问题

同一仓库里实际存在两个应用，而它们本应有 80% 共享。

| 维度 | 桌面端 | 移动端 | 共享 |
|---|---|---|---|
| 入口 | `index.html` → `main.tsx` → `App.tsx` | `mobile.html` → `mobile/main.tsx` → `mobile/App.tsx` | 否 |
| 数据 | `services/api.ts` 410 行 / 91 个导出（Wails 绑定） | `mobile/api.ts` 223 行 / 30 个导出（fetch） | 否，重复实现 |
| 主题 | `style.css` 内 `:root` 12 个变量 | `mobile.css` 内同样 12 个变量（注释自述 "mirror the desktop app"） | 否，手工同步 |
| 组件 | `components/` 40 个 | `mobile/components/` 19 个 | 仅 3 个叶子组件 |
| 导航 | 内存 `backStack`/`forwardStack` + 工作区标签 | 浏览器 history + 底部标签 + 入栈屏 | 否（**应当保持不同**） |

三个具体症状：

1. **桌面端没有响应式能力**。全仓只有 3 个文件使用真实断点类，`useMediaQuery` / `matchMedia` 零出现。桌面布局在窄视口下是"坏掉"，不是"没调好"。
2. **功能面缺失**。设置页、云浏览、新手引导、分组建议在移动端为 0，割裂感有一半来自功能缺口而非排版。
3. **结构卡点**。`pages/Library.tsx` 2010 行承载约 90 个 state，同时是应用壳、视图路由与全部视图；`mobile/App.tsx` 607 行与其对应。任何适配都要先拆开它。

## 2. 目标与非目标

**目标**

- 新增一个功能时，业务逻辑与展示基元只写一遍；平台差异收敛在两个壳和少数形态差异上。
- 让"移动端缺什么"成为可查清的清单，而不是靠用户发现。
- 建立防止再次漂移的机械约束（目录规则、清单文件、对照页）。

**非目标**

- 不合并构建入口。Wails 绑定、`go:embed webui` 产物、浏览器 history 路由这三处分歧是真实的。
- 不把桌面端改造成响应式单页。桌面窗口可以拉窄，但不是这次的目标；若日后需要，属于 L5 的独立需求。
- 不追求 100% 共享。导航壳、视频播放、外部播放器、safe-area 本就应当分离。
- 不引入新的 UI 组件库。自定义基元继续自持。

## 3. 目标架构

```
L5 壳（不共享，允许分叉）
   src/App.tsx            桌面：侧栏 + 工作区标签 + 右侧详情面板
   src/mobile/App.tsx     移动：底部标签 + 压栈屏 + 底部 Sheet

L4 原子（共享）           src/ui/
   Button Input Dialog Sheet Toast ActionMenu ListRow EmptyState Spinner …

L3 视图模型（共享）       src/views/<name>/
   use<Name>.ts           取数、筛选、排序、变更操作，无 JSX
   <Name>Desktop.tsx      桌面展示
   <Name>Mobile.tsx       移动展示

L2 数据（共享接口，双实现）src/services/
   client.ts              MuseClient 接口（唯一契约）
   client.wails.ts        Wails 绑定实现（桌面入口注入）
   client.http.ts         fetch /api 实现（移动入口注入）

L1 主题（共享）           src/theme.css  唯一 token 来源
```

### 目录规则（可机械检查）

| 规则 | 理由 |
|---|---|
| `src/ui/**` 只能 import React、`lib/utils`、自身 | 原子层不得依赖业务与平台 |
| `src/views/*/use*.ts` 只能 import `services/client`、`types`、`utils`、`hooks` | 视图模型与平台解耦 |
| `src/mobile/**` 不得 import `src/components/**` | 防止反向依赖桌面实现 |
| `src/components/**` 视为桌面专属 | 需要共享的一律下沉到 `ui/` 或 `views/` |
| 新组件必须先考虑 `src/ui/` | 禁止在业务文件里手搭基元 |

### 允许分叉的清单

导航与历史模型、视频播放与外部播放器调用、safe-area 与触屏手势、Wails 原生能力（文件选择器、剪贴板、系统文件夹）、桌面键盘快捷键与拖拽。以上差异必须在文档里写明，不允许以"顺手复制一份"的方式扩散到其他层。

## 4. 阶段划分

每个阶段独立可验收、独立提交。验收命令一律为 `cd frontend && npm run lint && npm run build && npm run build:mobile`。

### 阶段 0：止血（基础设施）

| 步骤 | 内容 | 验收 |
|---|---|---|
| 0.1 | 抽 `src/theme.css`，桌面 `style.css` 与移动 `mobile.css` 各自 `@import`；平台专属变量（`--ui-zoom` / `--safe-*`）留在各自文件 | 两份 token 定义变一份，构建产物视觉无变化 |
| 0.2 | `components/ui/` → `src/ui/`，更新全部引用 | `tsc` 通过；`grep -r "components/ui" src` 为空 |
| 0.3 | 缺失原子**按需创建**（`Sheet`、`Toast`、`ActionMenu`、`ListRow`、`EmptyState`、`Spinner`）：随阶段 2/3 的真实消费者落地，不预先建空组件 | 每个新原子都有实际消费者、props 类型与无障碍属性 || 0.4 | 目录规则写入 `AGENTS.md` 与 `docs/DEVELOPMENT.md` | 文档含规则表 |

### 阶段 1：数据层统一

| 步骤 | 内容 |
|---|---|
| 1.0 | 把 LAN 传输从 `mobile/api.ts` 移到 `services/http.ts`，让 L2 全部落在 `services/`，避免 `services → mobile` 的反向依赖 |
| 1.1 | `services/client.ts` 定义 `MuseClient` 接口：**两平台交集的 26 个能力**，不是桌面 91 个导出的并集 |
| 1.2 | `client.wails.ts` 包装现有绑定，`services/api.ts` 及其辅助函数保持不变 |
| 1.3 | `client.http.ts` 包装 `services/http.ts`，负责移动端命名（`javDetail` → `videoJav` 等）的映射 |
| 1.4 | 两个入口注入各自实现；`TrashView` 两平台改为通过 `client.*` 取数，作为接线验收样板 |
| 1.5 | 错误类型统一：移动端 `ApiError` 与 Wails 拒绝字符串目前不共享，视图只能按 `Error.message` 处理 |

验收：视图不再 import 任何传输模块（`services/api.ts` 或 `services/http.ts`），只 import `services/client.ts`；移动端 bundle 中不出现任何 Wails 绑定符号。

**接口边界的判定规则**：只有当两个平台都能执行、且返回形状一致时才进 `MuseClient`。已知的三个例外及其处置见 §8。

### 阶段 2：按视图迁移（主体）

按只读优先排序，每个视图一个提交：

1. `TrashView`（桌面 414 / 移动 295 行，先做样板）
2. `DuplicateView`（434 / 250）
3. `ContentDuplicateView`（581 / 365）
4. `ActorIndexView`（329 / 移动版）
5. `ActorFilmsView`
6. `ScrapeTaskView`

每个视图的固定动作：抽出 `use<Name>.ts`；桌面与移动各留 150~200 行展示壳；删除重复的状态与取数逻辑。

**中间态约定**：阶段 2 只下沉逻辑（`views/<name>/use<Name>.ts`），展示壳暂时留在 `components/` 与 `mobile/components/` 原位。等六个视图都迁完，再统一决定是否把展示壳搬进 `views/<name>/`，避免同一个视图在迁移途中跨两次目录。

验收：同名视图两份展示合计行数显著低于原两份实现之和；两平台行为一致（人工对照 `/__gallery`）。

### 阶段 3：交互形态自适应

- `ui/ActionMenu`：内部按 `useMediaQuery('(max-width: 767px)')` 渲染 Popover 或底部 Sheet，业务侧只写一份调用。
- 替换手写组件：三个右键菜单（437 行）、`Toasts`（42 行）、`mobile/components/Sheet`（143 行）。
- 同步补齐 ARIA：`role`、`aria-label`、`aria-live`、焦点回收。

验收：`src/components/ContextMenu.tsx`、`GroupContextMenu.tsx`、`TabContextMenu.tsx` 删除或被薄封装取代；键盘可完整操作菜单与提示。

### 阶段 4：防漂移机制

- `src/views/manifest.ts`：每个视图在 desktop / mobile 的支持状态，缺失显式标 `missing`。
- dev-only `/__gallery` 路由：双栏 iframe 同时渲染两平台，肉眼对照。
- 脚本 `scripts/check-shared-ui.mjs`：检查目录规则、裸 `<button>` 数量、同名视图行数差。
- 上述检查接入 `npm run lint` 之后的手动流程与 `AGENTS.md`。

## 5. 度量

| 指标 | 现状 | 目标 |
|---|---|---|
| 视图逻辑份数 | 2 份/视图 | 1 份/视图 |
| `ui/` 之外裸 `<button>` | 80 | 0 |
| 同名视图两平台行数差 | 30%~45% | 仅展示层差异 |
| 移动端缺失功能 | 4 个（设置、云浏览、引导、分组建议） | 显式记录在 manifest，允许长期缺失但必须可见 |
| `aria-*` / `role` 覆盖 | 零散十几处 | 交互组件全覆盖 |
| 重复 token 定义 | 2 份 | 1 份 |

**已完成的实测**

| 阶段 | 指标 | 结果 |
|---|---|---|
| 0.1 | 重复 token 定义 | 2 份 → 1 份（`src/theme.css`） |
| 0.2 | 跨平台基元位置 | `components/ui/` → `src/ui/`，构建产物哈希不变 |
| 1.0 | 传输层位置 | `mobile/api.ts` → `services/http.ts`，产物字节不变 |
| 1.4 | 传输隔离 | 移动 bundle 中 Wails 绑定符号计数为 0 |
| 1.5 | 错误处理分支 | 视图不再出现 `String(e)` / `instanceof ApiError`（新代码） |
| 2 TrashView | 两平台实现行数 | 414 + 295 → 337 + 247 + 150（共享逻辑），重复的取数与写操作归零 |
| 2 DuplicateView | 两平台实现行数 | 434 + 250 → 371 + 224 + 124（共享逻辑），取数、复核、保留/忽略全部单一实现 |
| 2 ContentDuplicateView | 两平台实现行数 | 581 + 365 → 457 + 250 + 231（共享逻辑），扫描状态机与保留/忽略/重校验单一实现 |

## 6. 风险与回滚

| 风险 | 应对 |
|---|---|
| 移动端构建产物入库，改动会带来大 diff | 每个阶段单独提交，产物随源码同次提交 |
| 抽 `client.ts` 时接口设计过度 | 以"现有 91 个导出"为上限，不做前瞻抽象 |
| 视图迁移期间两平台行为分叉 | 一次只迁一个视图，其余保持原样；迁移完成的视图删除旧实现 |
| `Library.tsx` 拆分牵动过多状态 | 先迁独立视图，最后才动 `Library.tsx` 本体 |
| 阶段 0 的重命名造成大面积 import 改动 | 机械替换 + `tsc` 校验；如失败整体回滚该提交 |

## 7. 进度看板

| 阶段 | 步骤 | 状态 | 提交 | 日期 |
|---|---|---|---|---|
| 0 | 0.1 抽 `theme.css` | 已完成 | `b12a67d` | 2026-11-13 |
| 0 | 0.2 `ui/` 迁移 | 已完成 | `6118dfc` | 2026-11-13 |
| 0 | 0.3 补齐原子 | 已改判为按需创建（随 2/3 落地） | — | — |
| 0 | 0.4 规则入文档 | 已完成 | 随本文档提交 | 2026-11-13 |
| 1 | 1.0 传输归位 | 已完成 | `207ef70` | 2026-11-13 |
| 1 | 1.1 接口定义 | 已完成 | `e8bcffd` | 2026-11-13 |
| 1 | 1.2 Wails adapter | 已完成 | `e8bcffd` | 2026-11-13 |
| 1 | 1.3 HTTP adapter | 已完成 | `e8bcffd` | 2026-11-13 |
| 1 | 1.4 入口注入 + TrashView 接线 | 已完成 | `1f462dd` | 2026-11-13 |
| 1 | 1.5 错误类型统一 | 已完成 | `6a7823f` | 2026-11-13 |
| 2 | TrashView | 已完成 | `6a7823f` | 2026-11-13 |
| 2 | DuplicateView（同番号多版本） | 已完成 | `481bcc6` | 2026-11-13 |
| 2 | ContentDuplicateView | 已完成 | `5d92cc6` | 2026-11-13 |
| 2 | ActorIndexView | 待开始 | — | — |
| 2 | ActorFilmsView | 待开始 | — | — |
| 2 | ScrapeTaskView | 待开始 | — | — |
| 3 | ActionMenu / 替换手写 | 待开始 | — | — |
| 4 | manifest / gallery / 检查脚本 | 待开始 | — | — |

状态取值：待开始 / 进行中 / 已完成 / 已放弃（附原因）。

## 8. 决策记录

- **2026-11-13**：不合并构建入口。理由：Wails 绑定、`go:embed` 产物、history 路由三处分歧真实存在，合并会把平台判断塞进每个组件。
- **2026-11-13**：不做桌面端响应式改造。桌面窗口拉窄是独立需求，不在本计划内。
- **2026-11-13**：不引入第三方组件库（含 Astryx）。理由见评估：Beta 0.7.x 版本锁死、+45KB gzip 全局 CSS、视频墙/右键菜单/拖拽等核心场景它不覆盖，收益集中在可选交互组件，而自建 `ui/` 已能覆盖。
- **2026-11-13**：进度类事件统一为**轮询**，只有"逐字节校验的逐条计数"保留桌面事件通道。扫描进度原本两端实现不同（桌面 Wails 事件、移动 1.5 秒轮询），现在都由视图模型轮询：两端行为一致，代价是 1.5 秒延迟，换掉的是两套进度通路。完整校验的 `done/total` 只有桌面能精确拿到，仍走 `EventsOn`，通过 `subscribeConfirmProgress` 端口注入；浏览器靠 `group.confirming` 与 3 秒重读表示"还在校验"。
- **2026-11-13**：平台能力以**可选参数**注入视图模型，不在 hook 内部分支。桌面查重会逐条调 `RefreshStatus` 复核文件是否还在（Wails 绑定），浏览器没有这个端点：`useJavDuplicates({ refreshVersion })` 由桌面传入、移动不传。这样"这台机器能做、那台不能"的差异在调用处一眼可见，而不是藏在 `if (isDesktop)` 里。
- **2026-11-13**：共享层**不输出 toast 类型**。`useTrash` 发的是中性通知 `{kind, text, details}`，桌面映射成三态 + 详情列表的 `ToastItem`，移动映射成两态 tone + 首条详情拼进正文。理由：两端的 toast 能力不同，让视图模型依赖任何一端的类型都会把另一端的形状带歪。
- **2026-11-13**：确认框的开关状态归**展示层**，不进视图模型。数据与写操作在 hook 里，哪一次点击先关框再执行由各壳自己决定，两端行为一致但组件不同（`ConfirmDialog` / `ConfirmSheet`）。
- **2026-11-13**：错误统一走 `services/errors.ts` 的 `errorMessage(error, fallback)`，不用异常子类做区分。Wails 以字符串或 `Error` 拒绝，HTTP 抛 `ApiError`，函数把两者都归一为用户可读文本，视图不再写 `String(e)` 或 `instanceof ApiError`。
- **2026-11-13**：`MuseClient` 取两平台**交集**（26 项），不复制桌面 91 个导出。理由：接口一旦收录浏览器做不到的能力，移动端就只能返回假的 `Unsupported`，反而把"缺失"藏起来；平台专属能力留在各自模块，缺失是显式的 import 不存在。
- **2026-11-13**：`videosInGroup` 不进接口。桌面返回 `number[]`（id 列表，配合已加载的全量列表在内存里过滤），移动端返回 `Video[]`（服务端直接给对象）。形状不一致，等阶段 2 做 Library 视图模型时再定统一形状，避免现在拍脑袋。
- **2026-11-13**：`markVideoOpened` 不进接口。桌面录制播放由 `OpenVideo` 在 Go 侧完成，移动端因为播放交给外部 App，需要单独 `POST /opened`。这是播放链路的合理分叉，记入 §3 允许分叉清单。
- **2026-11-13**：`components/ui/` 提升为 `src/ui/`。理由：让"共享层"在目录上可见，并可被机械检查。

## 9. 变更记录

| 日期 | 变更 |
|---|---|
| 2026-11-13 | 初稿：确立分层、阶段 0-4、度量与目录规则 |
| 2026-11-13 | 阶段 0 完成：抽出 `frontend/src/theme.css`（两份 token 定义合一，构建产物哈希不变）；`components/ui/` 提升为 `src/ui/`（40 个文件的 import 已改写，产物哈希不变）；目录规则写入 `AGENTS.md` 与 `docs/DEVELOPMENT.md` |
| 2026-11-13 | 阶段 2 第三个视图：`ContentDuplicateView` 逻辑下沉到 `views/duplicates/useContentDuplicates.ts`（231 行，含扫描状态机与两个纯函数 `scanStatusText` / `scanProgress`），桌面壳 581→457 行、移动壳 365→250 行 |
| 2026-11-13 | 阶段 2 第二个视图：`DuplicateView`（同番号多版本）逻辑下沉到 `views/duplicates/useJavDuplicates.ts`（124 行），桌面壳 434→371 行、移动壳 250→224 行；中性通知类型提到 `views/notify.ts`，被两个视图模型共用 |
| 2026-11-13 | 阶段 2 首个视图完成：`TrashView` 逻辑下沉到 `views/trash/useTrash.ts`（150 行），桌面壳 414→337 行、移动壳 295→247 行，两平台重复的取数与写操作归零；同时新增 `services/errors.ts`，消掉两端各自的错误处理分支 |
| 2026-11-13 | 阶段 1 主体完成（1.5 错误类型统一留待后续）：`mobile/api.ts` → `services/http.ts`（纯移动，产物字节不变）；新增 `services/client.ts`（26 项交集契约）与两个 adapter；两入口注入；`TrashView` 两平台改写为 `client.*`，桌面 +2.1 KB / 移动 +2.1 KB，移动 bundle 仍不含 Wails 符号 |
| 2026-11-13 | 0.3 改判：不再预先补齐六个空原子，改为随阶段 2/3 的真实消费者落地，避免留下无人调用的组件 |
