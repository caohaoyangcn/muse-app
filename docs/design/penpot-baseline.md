# Penpot 基线：设计稿的"现状镜像"

设计文件：Penpot 上的 `Muse Video`（维护者账号私有）。不做成公开链接——文件里有未发布的设计，
需要访问权时向维护者索取邀请。

它是**当前实现的镜像**，不是自由创作的文件。UI 变更从这个文件出发改（见
[`ui-change-process.md`](ui-change-process.md)），改完编码、编码完再回写。

现状（做到了什么、哪些路已证明走不通、下一步怎么走、换机器怎么接上）见
[`design-workflow-handoff.md`](design-workflow-handoff.md)。

## 三层结构

| 区块 | 内容 | 能否手改 |
|---|---|---|
| `Muse Video · 设计基元` | 色板、字阶、间距、圆角、Button/Input/Badge/Chip/Cover/Pagination 等基元展示 | 可以（这是 token 的说明页） |
| `Components / 从截图临摹` | 33 个组件（Topbar、Sidebar、MediaToolbar、MediaCard、FilterChip、Dialog、ListRow、Pagination…） | 可以，按需修 |
| `Screens / HiFi（DOM 实测）` | 25 屏（桌面 20 + 移动 5），每屏一个 board，图层 = 实测值 | **禁止手改**，只能由 DOM 重导覆盖 |

基线段禁止手改的原因很实际：手改一次，画布与实现就出现一个无法追溯的差异，
下一个人不知道该信哪边。要改基线，就改代码再重导。

## 刷新流程

源码是唯一真相，画布是派生物。三步：

```bash
# 1. 起预览（真组件 + mock 传输层，两端同一份源码）
npm --prefix frontend run dev:design

# 2. 把每屏的几何与样式抓成 JSON（镜像 capture-design.mjs 的交互步骤）
PLAYWRIGHT_MODULE=/path/to/playwright node frontend/scripts/dump-design-dom.mjs

# 3. 用 Penpot MCP 把 .design-dom/*.json 写成 board（见下节规则）
```

`.design-dom/` 是中间产物，已在 `.gitignore` 里，不要提交。

## 写进 Penpot 时的八条硬规则

这些是踩过坑得出的，改导入逻辑时逐条核对：

1. **坐标是页面绝对坐标**。`shape.x/y` 不是相对父级的；先算好 `board.x + node.x`，
   否则子元素全落在画框外、导出是空白。
2. **父容器有 flex 就会覆盖坐标**。往带 flex 的容器里放画框，必须先
   `layoutChild.absolute = true`，**再**写 `x/y`；顺序反了会被布局到 (0,0)。
3. **`resize()` 不接受 0**。高度未知时给 1，或改用 `horizontalSizing: 'fill'`。
4. **背景 alpha 不能丢**。`rgba(124,92,255,.12)` 要写成 `{ fillColor, fillOpacity: .12 }`，
   直接取 RGB 会让侧栏选中态变成整块实心紫。
5. **图标导成矢量，不要用字符代替**。`penpot.createShapeFromSvg(svg.outerHTML)`，
   之后把 path 的 `fills` 清空、只保留 stroke（lucide 是描边图标）。
6. **文本框宽度按实测值给，单行文本按字符估算兜底**。若设计工具与运行时的字体不同，
   按实测宽度会换行（「保存」会竖排）。宽度取 `max(实测宽, 按字符估算宽) + 4`，
   CJK 按 `fs × 1.02`、拉丁按 `fs × 0.54` 估；或者干脆把字体随包分发，两边同字。
7. **长文本要在抓取时截断**。CSS 的 `text-overflow: ellipsis` 在设计工具里没有等价能力，
   直接把完整字符串导进去只会折行或溢出。`dump-design-dom.mjs` 会按浏览器实测的
   `clientWidth` 二分裁剪并补 `…`，导入后位置和长度都与实现一致。
8. **封面要按 `object-fit` 语义裁剪**。`CoverImage` 的 `portrait` 只露右半、`landscape`
   等比填充。导入时用一个固定尺寸的裁剪容器包住海报：竖框按高度缩放并右对齐，横框按
   `max(宽比, 高比)` 缩放并居中，否则海报里的文字会跑到卡片中间。

## 在另一台机器接入（同一账号，同一份稿）

设计稿在 Penpot 云端，不在任何一台机器上。换机器不需要拷贝文件，只需要**同一个 Penpot
账号 + 一份 MCP 凭证**。

### 前置条件

- Penpot 账号（文件属主维护者的账号）。**别人的账号默认看不到** —— 文件在「草稿」里，
  草稿是私有的；要协作得先把它移进团队项目并邀请成员。
- 能访问 <https://design.penpot.app>。
- Penpot 的 MCP 服务器在其免费/Professional 套餐里可用，不需要额外付费。

### 第 1 步：确认能看到稿子

浏览器登录 Penpot → 打开 `Muse Video`。打不开就说明账号不对或没被邀请，
先解决这个，后面的凭证没有意义。

### 第 2 步：取 MCP 凭证（没有凭证时）

Penpot 的 MCP 用**个人 key 走 URL**，不做 OAuth：

1. 登录后点右上角头像 → **Account → Integrations → MCP Server**
2. 启用后**生成 key**（**只显示一次**，关掉就再也看不到，只能重新生成）
3. 复制给出的 **server URL**，形如：

   ```
   https://design.penpot.app/mcp/stream?userToken=<token>
   ```

那个 `userToken` 就是凭证本身。它绑定你的账号，**不会过期**，但会随「重新生成」失效。

### 第 3 步：配置客户端

任何支持 remote MCP 的客户端都一样：把第 2 步的完整 URL 填进它的 `url` 字段，不需要 header。

```jsonc
// Claude Desktop / Cursor / Codex 等：按各自格式填 mcpServers
{ "mcpServers": { "penpot": { "url": "https://design.penpot.app/mcp/stream?userToken=…" } } }
```

> 维护者本人用 DeepSeek Harness：key 存在 harness 的凭据库里
> （`refs.PENPOT_MCP_KEY`，权限 600），profile 里的 MCP 条目用 `${credential:…}`
> 模板展开 URL，避免明文入库。这是本机配置，不属于本仓库。

### 第 4 步：验证接上了

1. 浏览器里**打开设计稿的那个页面**（插件只在文件页激活）；
2. Penpot 里 `File → MCP Server` 确认显示已连接；
3. 调一次工具，例如 `mcp__penpot__execute_code` 跑：

   ```js
   return penpot.root.children.map((c) => c.name);
   ```

   返回 `["Muse Video · 设计基元", "Components / 从截图临摹", "Screens / HiFi（DOM 实测）"]`
   就说明连的是同一份稿。

### 同一时刻只有一个连接

插件绑定在**一个浏览器标签页**上。两台机器同时连会互相抢占：需要来回切换时，
先在 A 机器关掉插件（或关掉标签页），再去 B 机器操作。别指望并发编辑。

## 常见故障

| 现象 | 原因 | 处理 |
|---|---|---|
| 写入/导出返回 **Cloudflare 504** | 文件已 2700+ 图层，源站处理超时 | 等 120 秒重试；**不要**重跑整批（改动通常已经落盘），先读一次确认再继续 |
| 插件报 `no heartbeat` | 浏览器把 Penpot 标签页挂起 | 切到前台点一下；ego-browser 里开预览页会把 Penpot tab 挤到后台 |
| 导出高分辨率很慢 | 单 board 上千图层 | 只导单屏；整页导出意义不大 |
| 基线与代码不一致 | 有人手改了基线屏 | 以代码为准重导该屏，不要反向改代码去迁就画布 |
