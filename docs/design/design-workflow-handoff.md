# 设计工作流交接：现状、已踩的坑、下一步

> 写于 2026-11-27。接手的人（或 agent）先读这一篇，再按需跳到配套文档。
> 配套：[`ui-change-process.md`](ui-change-process.md)（变更流程与评审清单）、
> [`penpot-baseline.md`](penpot-baseline.md)（设计文件结构、刷新方式、导入硬规则）、
> [`figma-handoff.md`](figma-handoff.md)（最初的导出规范与命名表）。

## 结论先行

1. **已经建成**：一份能准确镜像当前实现的 Penpot 基线 —— 25 屏（桌面 20 + 移动 5）、33 个组件、
   11 个颜色 token、全量 `Sarasa UI SC`。01 屏与真实截图逐像素比较：平均通道差 **3.03/255**，
   **97.3%** 的像素差 ≤ 24/255。
2. **还没达成**：这份基线**改不动**。所有图层是绝对坐标，挪侧栏不会带动右侧，改标题不会把
   内容推下去；33 个组件与基线屏是两套东西，基线里一个实例都没有。
3. **卡点**：把"实现"翻译成"可编辑设计稿"（下文代号 B）需要一套 **CSS 布局语义映射**
   （block/flex/grid、fill/hug、百分比、grid 行高……）。试到第四轮仍然"每修一处换一处错"，
   已停，失败的屏已从设计文件删除，没有半成品残留。
4. **下一步**：反过来 —— **在 Penpot 里设计新 UI，评审通过后按稿改代码**。这条路不需要"翻译"，
   因为布局是设计时写下的，天然可编辑。

## 0. 换机器后怎么接上（约 15 分钟）

### 代码侧

```bash
git clone https://github.com/caohaoyangcn/muse-app.git
cd muse-app
npm --prefix frontend ci
npm --prefix frontend run dev:design   # 预览：http://localhost:5199/src/design/preview.html
```

- 预览是**真组件 + mock 传输层**（`frontend/src/design/`），不是截图服务：
  桌面 `preview.html`、移动 `preview-mobile.html`、基元页 `board.html`。
- 截图与 DOM 抓取都驱动这个预览，需要 Playwright（仓库**不依赖**它，只靠环境变量指路）：

```bash
PLAYWRIGHT_MODULE=/path/to/playwright node frontend/scripts/capture-design.mjs
PLAYWRIGHT_MODULE=/path/to/playwright node frontend/scripts/dump-design-dom.mjs
```

### 设计文件侧

- 稿子在 Penpot 云端，同一账号即可看到，**不需要拷贝文件**；链接与凭证**不入库**
  （仓库是公开的），取用方式见 [`penpot-baseline.md`](penpot-baseline.md) 的
  「在另一台机器接入（同一账号，同一份稿）」。
- 打开设计稿所在的**页面**（插件只在文件页激活），确认 `File → MCP Server` 显示已连接。
- 验证连的是同一份稿：

  ```js
  return penpot.root.children.map((c) => c.name);
  // 期望：["Muse Video · 设计基元", "Components / 从截图临摹", "Screens / HiFi（DOM 实测）"]
  ```

- **同一时刻只有一个标签页能持有连接**。两台机器同时开会互相抢占，切换时先关掉一边。
- 浏览器会挂起后台标签页（插件心跳断掉），写入时 Penpot 必须在前台，别切走。

## 1. 现状

### 1.1 仓库里的资产（可信来源，都可重建）

| 路径 | 作用 |
|---|---|
| `frontend/src/design/` | 设计预览壳：真实页面 + mock 传输层 + 基元展示页 |
| `frontend/design.preview.config.ts` | 预览专用 Vite 配置（端口 5199，独立入口） |
| `frontend/scripts/capture-design.mjs` | 抓 25 屏 + 基元长图到 `docs/design/screenshots/`（2× JPEG） |
| `frontend/scripts/dump-design-dom.mjs` | 抓每屏的几何 / 样式 / 文本 / 图标 / 图片，外加 flex/grid/block 布局与 depth，输出 `.design-dom/`（已 gitignore） |
| `docs/design/screenshots/` | 26 张：25 屏 + `20-design-board.png` 基元长图 |
| `docs/design/figma-tokens.json` | DTCG token：color / alpha / semantic / radius / font / space / shadow |
| `docs/design/screen-inventory.md` | 26 屏清单、已知界面问题 |
| `frontend/src/assets/fonts/` | Sarasa UI SC 三个字重（WOFF2，24.3 MB）+ OFL 许可 |
| `docs/design/penpot-baseline.md` | 设计文件结构、刷新三步、导入 8 条硬规则、换机接入、故障表 |
| `docs/design/ui-change-process.md` | 变更流程与 8 条评审清单 |
| `docs/design/changes/sarasa-font/` | 字体替换的 before/after 与实测字宽对比 |

### 1.2 Penpot 里有什么

基线文件（Penpot 上的 `Muse Video`，维护者账号私有，草稿里那个含「设计基元 / Components / Screens」三块的就是它）：

| 区块 | 内容 | 能否手改 |
|---|---|---|
| `Muse Video · 设计基元` | 色板、字阶、间距、圆角、Button/Input/Badge/Chip/Cover/Pagination 展示 | 可以（token 的说明页） |
| `Components / 从截图临摹` | 33 个组件（Topbar、Sidebar、MediaToolbar、MediaCard、FilterChip、Dialog、ListRow、Pagination…） | 可以，按需修 |
| `Screens / HiFi（DOM 实测）` | 25 屏，每屏一个 board，约 2700–2800 个图层 | **禁止手改**，只能由 DOM 重导覆盖 |

补充三件事，避免接手时找不到或误判：

- **11 个库颜色**对应 token 的 `color/*`（bg、panel、panel-2、card、border、text、muted、accent、
  accent-hover、danger、on-accent）。`radius / space / font / shadow` **不是变量**，只是基元页上的标注
  —— 这版 Penpot（2.19.3）的 `library.local.tokens.addSet()` 用 7 种入参都被拒，见 3.3。
- **25 屏的文本层全部是 `Sarasa UI SC`**，不是设计工具默认字体；重导时若不显式写 `fontFamily`，
  字宽会漂回默认字体，之前修好的换行又会错。
- 另有为 B 准备的两个**空文件**：`Muse Video · 基线 · 核心`、`Muse Video · 基线 · 移动端`。
  B 停了，它们现在是空的 —— 不是导失败，是没往里写。

### 1.3 保真度（可复现的数字，不是印象）

| 屏 | 平均通道差 | ≤24/255 | >48/255 | 说明 |
|---|---|---|---|---|
| 01 桌面 收藏库（1440×900） | **3.03** | **97.3%** | 1.7% | 布局、颜色、圆角、间距全部取自实测值；残差主要在文字抗锯齿 |
| 21 移动 收藏库（390×844） | 9.27 | 80.3% | 12.4% | 截图是 3×（1170×2532），画布导出 1×，缩放后字形差异被放大，**不能与桌面同标准比** |

复现方式：把画布单屏导出 PNG，与 `docs/design/screenshots/` 对应图缩到同尺寸后逐像素比较。
文字边缘的差异来自渲染管线，不是布局错位 —— 判断布局对不对要看盒子的位置与尺寸，不要看字形。

## 2. 这几天走过的路

| 阶段 | 做法 | 结果与结论 |
|---|---|---|
| 1. Figma MCP | 按官方文档配远程 MCP | 服务端只支持 OAuth，客户端动态注册受限；当时的账号没有画布写入权限 → **Figma 只能读，不能当生产工具** |
| 2. 转 Penpot | 用个人 key 走 URL 的 MCP，插件在页面内执行 | 通了，可读写；从此设计稿在 Penpot |
| 3. 截图入画布 | 把 26 张真实截图当 frame 铺进去 | 看着像设计稿，其实是"截图墙"。`figma-handoff.md` 第 0 节早就写了"截图是参照、不是可编辑图层" → 全部删除 |
| 4. 手搭 01/03/15 | 读 JSX、按类名猜尺寸 | 结构像、数值错（卡片 509×372 vs 实际 500×341.6，圆角 10 vs 12，正文 13 vs 14）→ **读代码不等于读布局** |
| 5. DOM 实测导入 | 浏览器里量 `getBoundingClientRect` + `getComputedStyle`，把数值写进画布 | 保真达标（01 屏 94.2% 像素差 ≤8/255），25 屏铺满 → **看的问题解决了** |
| 6. 字体统一 | 随包分发 Sarasa UI SC，截图与基线重做 | 设计稿、开发机、生产同字；01 屏平均差 3.32 → 3.03 → 字宽漂移这条线结案 |
| 7. B：auto-layout | 想按 DOM 的 flex/grid 生成真布局，让稿子"改得动" | 失败：补结构节点后 01 屏 164 → 291、25 屏约 4800；卡片被 `fill` 拉成 1205 宽、宽度不回退、整页偏移。**每修一处换一处错** → 停，删掉半成品 |
| 8. 反向 | 不再翻译实现，改为先设计新 UI 再实现 | 当前方向，见第 4 节 |

相关提交（按时间）：

| 提交 | 内容 |
|---|---|
| `5fe0fb2` | 设计交付包：预览壳、截图管线、token、屏清单、导出规范 |
| `89a0cfa` | 修预览 mock 缺 `listVideoHighlights`（详情页「精彩时刻」报错） |
| `b99815b` | UI 变更流程：先交互稿、评审、再编码 |
| `81b9d8f` | 字体换 Sarasa UI SC（含 25 屏重拍） |
| `956c169` `c8d9fe1` | 长文本在抓取时按 `clientWidth` 截断；导入规则补两条 |
| `033ffa3` `0419e08` `d41b9f6` `69c38e6` | B 的前置：抓取器补 flex / 结构层 / depth / grid+block 容器 |

## 3. 碰到的问题

### 3.1 根本矛盾：像素保真 ≠ 可编辑

绝对定位能把"看"做到 97%，但代价是**失去布局关系**：

1. 改一个容器尺寸，邻居不重排，要逐个挪；
2. 没法回答"窄屏怎么办"——它只在一个像素宽度下成立；
3. 内容变长就压住邻居，而不是把下面的推下去（这也是「元信息压到心形数上」的直接原因）；
4. 25 屏 × 平均 110 图层 ≈ 2800 个裸对象，组件完全没用上；
5. 文件重，写入慢，504 频发；
6. 改版成本 = 重摆一遍坐标，而不是改一个 `flex` 属性。

结论：**保真解决"看"，可编辑解决"改"**。目标是改版，所以只做前者是走偏的。

### 3.2 B 为什么失败（具体到可复现）

- 第一版抓取器只导出"有像素贡献"的节点，透明但承担布局的 flex 包裹层全被丢掉：
  01 屏只抓到 30 个 flex 容器，其中只有 1 个 column —— 这种数据重建不出层级。
- 补上 `structural`、`depth`、`grid`、`block` 之后，01 屏 164 → 291 节点，25 屏约 4800。
  也就是说：**auto-layout 会让文件更重，不是更轻**。
- 层级挂错：`main` 是普通 block，被当成"无视觉贡献"跳过，卡片网格于是挂到了侧栏 `aside` 下。
  改用"矩形包含关系 + depth"双重判定后 320 个节点全部挂对，但下一层问题立刻出现。
- 尺寸语义错：`width:100%` 该是 fill，卡片该是 fix；判错就拉成全宽（1205 宽），改成 fix 后
  宽度又不会回退，手工 resize 之后整页偏移、顶栏侧栏跑到画外。
- 根因不是某行代码，而是 **CSS 的布局语义在设计工具的 flex 模型里没有一一对应**：
  块级元素默认撑满、`flex:1`、`min-width`、百分比宽度、grid 的列与行高由内容决定……每一条都要单独翻译，
  而翻译错了只能靠几何自检发现。

结论：继续 B 等于写一个独立的"CSS → 设计工具"布局映射引擎，并用计算几何逐屏自检。
那是独立工程，不是收尾项。**已停，且没有回退成"再调两次参试试"。**

### 3.3 工具级限制（改不掉，只能绕）

| 限制 | 影响 | 现在的绕法 |
|---|---|---|
| 无 Variables API | `radius/space/font/shadow` 落不成变量 | 只落 11 个库颜色，其余是基元页标注 |
| 无 `text-overflow: ellipsis` | 长标题折行、溢出、压住邻居 | 抓取时按浏览器实测 `clientWidth` 二分截断并补 `…`，导入的就是截断后的串 |
| 无 `object-fit` | 竖版封面（只露右半）被塞进整张横版海报 | 导入时用固定尺寸裁剪容器：竖框按高度缩放并右对齐，横框等比居中 |
| 服务端渲染文字，不认系统字体 | 原先设计稿字体 ≠ 实现字体 | 随包分发 Sarasa UI SC（三个字重），截图与基线同字 |
| 文档体量大时 Cloudflare 504 | 写入/导出超时，偶发未落盘改动被丢弃 | 等 120 秒后**先读一次确认**再继续；不要整批重跑；单屏导出 |
| 单标签页连接 | 换文件/换机器互相抢占 | 一次只留一个 Penpot 标签页在前台 |

### 3.4 流程与环境问题（都已处理，但会复发）

- **仓库截图曾落后于代码**：`5fe0fb2` 里那批截图固化的是一份更早的 UI（搜索框还在顶栏），
  照它导进画布，画布自然就是旧的。现在以 `capture-design.mjs` 重拍为准；**改完 UI 必须重拍 + 重导**，
  否则"稿 = 实现"立刻失效。
- **预览 mock 与真实契约脱节**：`design/mock/client.ts` 漏了 `listVideoHighlights`，详情页整块报错
  `is not a function`，而这个错误状态被一起抓进了设计稿。已修；新增 mock 方法时对照
  `services/client.ts` 的契约逐项补。
- **字体回退**：预览配置里 `publicDir: false`，字体放 `public/` 根本取不到（`/fonts/*` 返回的是
  index.html），曾导致截图与基线全是回退字形。改走 `src/assets/` 的 Vite 资源管线，并让抓图/抓 DOM
  都等 `document.fonts.ready`。
- **公开仓库不能放的东西**：Penpot 链接（含 team/file id）、账号邮箱、本机 harness 路径与凭证。
  文档只写"向维护者索取"和获取步骤。
- **浏览器节流**：后台标签页会被挂起，插件心跳断掉；用浏览器自动化打开预览页时，Penpot 标签页会被挤到后台。

## 4. 下一步

### 4.1 主线：设计先行（推荐）

| 步 | 谁 | 产出 |
|---|---|---|
| 1 | 我 | 新设计文件里建 `设计系统` 页：token + 基元组件，**auto-layout**，真组件不是死图层 |
| 2 | 我 | **01 收藏库 + 03 影片详情**两屏各出 2–3 个方向（桌面 1440×900 + 移动 390×844） |
| 3 | 你 | 选方向，逐条过 8 条评审清单，结论写回 `docs/design/changes/<slug>/README.md` |
| 4 | 我 | 按评审出定稿，补齐全状态（默认/空/加载/错误/超长/未接入） |
| 5 | 我 | 改代码，映射到 `ui/` 现有基元；PR 链接目录并写「与交互稿的差异」 |
| 6 | 我 | 合并后重拍截图 + 重导基线，保持"稿 = 实现" |

先做两屏而不是 25 屏：一个列表页 + 一个详情页覆盖 80% 的组件类型，链路跑通再铺开。

设计时必须守三条，否则稿子落不了地：

1. **只用现有 token**：颜色取 `figma-tokens.json`，间距 4/8/12/16/20/24/32，圆角 5/6/10/12/999，
   字号 10/11/13/15/17/20/22（badge / caption / body / toolbar / title / heading / display）；
2. **组件对应 `ui/` 基元**：`frontend/src/ui/` 里的 Button、Input、Dialog、Menu、CoverImage、
   Pagination 等，要新增基元就在评审里单列并写进 `ui/`（`scripts/check-shared-ui.mjs` 会拦裸按钮与裸封面）；
3. **状态齐、两端齐**：桌面 1440×900、移动 390×844，缺一不可。

### 4.2 备选（如果主线推进不顺）

| 方案 | 做法 | 代价 | 结果 |
|---|---|---|---|
| **C：骨架 flex** | 只把 Topbar / Sidebar / 主区 / 工具栏 / 卡片行做成 flex + 组件实例，其余保持绝对 | 半屏工作量 | 改侧栏宽度、改卡片数量能重排；卡片内部不重排 |
| **B′：继续全量 auto-layout** | 先写布局映射引擎 + 逐屏几何自检，再拆文件重导 | 独立工程 | 画布真可编辑，但 4800 图层仍会 504 |
| **A：只当镜像** | 基线保持现状，新 UI 在另一个文件里从零搭组件化页面 | 0 | 快，但基线永远只是"照片" |

### 4.3 明确不再做

- 不再"给实现拍一张可编辑的照片"，再把坐标当设计；
- 不再把截图当设计交付（`figma-handoff.md` 第 0 节的原文警告）；
- 不再在约 2700 图层的旧文件上做大批量写入；
- 不再靠读 JSX 或看截图猜布局 —— 要看就量 `getBoundingClientRect` 与 `getComputedStyle`。

## 5. 待办

- [ ] 定视觉方向：保持暗色媒体库做演进，还是重新定调（决定草案幅度）
- [ ] 定设计文件：新建 `Muse Video · 新 UI`，还是复用空的 `Muse Video · 基线 · 核心`
- [ ] 设计系统页（token + 基元，auto-layout）
- [ ] 01 / 03 两屏方向草案（桌面 + 移动）
- [ ] 评审 → 定稿 → 编码 → 回写基线
- [ ] 可选：把 25 屏按文件拆开（核心 4 屏 / 移动 5 屏 / 其余按需），降低 504 概率
- [ ] 可选：把重复结构（卡片、侧栏行、工具条）换成实例，让改版只改主组件

## 6. 评审时别当成设计沿用的既有缺陷

这些是实现里已知的问题，设计稿不该照着画：

- 移动端详情页 `状态：done` 中英混排（`screen-inventory.md` 已记，应中文化）；
- 1440×900 下卡片网格只排 2 列（约 500 宽/张），密度偏低，尺寸滑块是唯一控制；
- 顶栏在约 960 宽开始换行 → 至少同时看 1100×720 与 1440×900 两个宽度；
- `frontend/src/design/board.tsx` 的字阶把 13px 标成"正文默认"，而根元素是 14px，
  同一界面里 13/14 混用 —— 基元页的字号只能当参考，规格以 computed style 为准。

## 7. 相关文档

- [`ui-change-process.md`](ui-change-process.md) —— 变更流程、评审清单、例外
- [`penpot-baseline.md`](penpot-baseline.md) —— 设计文件结构、刷新三步、导入 8 条硬规则、换机接入、故障表
- [`figma-handoff.md`](figma-handoff.md) —— 命名表（`区域 / 组件 / 变体`）、导出要求、已知界面问题
- [`screen-inventory.md`](screen-inventory.md) —— 26 屏清单与截图对应关系
- [`figma-tokens.json`](figma-tokens.json) —— 设计 token（DTCG）
- `AGENTS.md` 的「UI 变更流程」 —— 硬约束：没通过评审的交互稿不进 `frontend/src/`
