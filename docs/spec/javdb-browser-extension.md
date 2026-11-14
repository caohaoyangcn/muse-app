# JavDB 浏览器扩展：卡片一键查收藏

> 状态：已实现。源码在 `browser-extension/`，服务端在 `backend/server/extension.go`，
> 查询能力在 `backend/api/library.go`（`MatchJavCodes`）。

## 1. 目标

在 JavDB 的影片卡片上直接回答一个问题：**这个番号是否已经收藏到本机 Muse？**

用户不必切回 Muse 逐条搜索，悬停角标即可看到结果，点开还能看到命中的是哪些文件、
在哪个盘或网盘上、是否只在回收站里。

非目标：

- 不在 JavDB 上提供任何写操作（加收藏、标心想看、触发刮削）。
- 不把扩展做成 JavDB 的下载器或磁链工具。
- 不支持手机浏览器（移动端浏览器不能安装扩展）。

## 2. 设计决定：番号来自页面，不在扩展里解析

**扩展不解析任何"从标题里猜番号"的规则。** JavDB 已经把它渲染成独立元素
（卡片标题里的 `<strong>`），扩展读那个元素的文本即可。

这条决定不是图省事，而是正确性要求：

| 做法 | 后果 |
|---|---|
| 从标题文本猜番号 | 猜错就把一部作品标成另一部；同一个标题字符串里往往还有分辨率、站点前缀等噪声 |
| 读页面渲染的片号字段 | 区分的是页面自己认定的作品，与站点的分类、番号列表页一致 |

扩展侧对读到的值只做**字面整理**，不改变它的含义：

- 去掉零宽空格 / BOM；
- 去掉首尾空白（含 `\u00a0` 不间断空格与 `\u3000` 全角空格）；
- 转大写。

页面没有片号字段时角标显示 `?`，**不发起查询**。

服务端 `jav.LookupKey` 执行同一套整理后再比对，两侧因此不会对"同一个番号字符串"
产生分歧。`\u00a0` 出现在番号中间时不会被吞掉（`ABP\u00a0576` 归一为 `ABP 576`），
因为那是页面自己的写法，库里的记录也按刮削到的写法存。

## 3. 整体结构

```
JavDB 页面（content script）
  │ 1. 读卡片 .video-title strong 的文本 → 字面整理
  │ 2. 悬停 / 可见卡片 → 合并成一批（200 ms 窗口）
  ▼
扩展 service worker（background.js）
  │ POST /api/extension/check      （host permission: 127.0.0.1:8899）
  ▼
Muse 局域网服务（backend/server）
  │ withExtensionCORS → handleExtensionCheck
  ▼
api.Library.MatchJavCodes(codes)  ← 只读，不写库
  └─ 已刮削：javCode 精确匹配（按番号+上下集归键）
  └─ 未刮削：jav.ExtractNameCodes(fileName) 的第一个候选
```

为什么由后端匹配而不是扩展自己筛：

1. 番号比较必须同时覆盖"已刮削的番号"和"未刮削收藏的文件名候选"，后者是 Muse
   既有的提取器（与查重同一套规则）。放在 Go 里只有一份实现。
2. `GET /api/videos` 会返回整库（含备注、哈希等），扩展只需要"有没有、是哪几条"。
3. 一页 40–60 张卡片合并成 1 个请求，避免 N 次整库读。

## 4. 接口契约

```
POST /api/extension/check
Content-Type: application/json
Origin: https://javdb.com

{"codes": ["ABP-576", "FC2-PPV-1234567"]}
```

```json
200 {
  "codes": [
    {
      "code": "ABP-576",
      "matches": [
        {
          "code": "ABP-576",
          "videoId": 42,
          "inTrash": false,
          "fileName": "ABP-576 1080p.mp4",
          "label": "ABP-576",
          "javCode": "ABP-576",
          "width": 1920,
          "height": 1080,
          "fileSize": 4294967296,
          "actors": ["演员名"],
          "coverPath": "covers/jav/42.jpg",
          "sourceName": "",
          "volumeOffline": false,
          "status": "available",
          "createdAt": 1763000000000
        }
      ]
    },
    { "code": "FC2-1234567", "matches": [] }
  ]
}
```

约定：

- `codes` 按请求顺序返回，请求里重复的番号合并成一个结果槽位；`code` 是归一后的值
  （所以客户端不能用原样字符串去配，应该用返回的 `code`）。
- 未命中的番号返回 `"matches": []`（数组，不是 `null`）。
- 一次最多 100 个番号，超出返回 `400`（不静默截断，客户端需要知道）。
- 请求体不是合法 JSON、或 `codes` 缺失 → `400`；空数组 → `200` + 空结果。
- 单个字符串查不到、`jav` 服务未配置（自动化测试或纯文件名书签）都不是错误：
  前者空结果，后者退回文件名匹配。

### 匹配规则

| 来源 | 键 | 说明 |
|---|---|---|
| 已刮削收藏 | `jav.LookupKey(javCode)` | `override_mode='skip'` 的收藏不参与（与库内展示一致） |
| 未刮削收藏 | `jav.DuplicateKey(LookupKey(首个候选番号), Part)` | 只取文件名的**第一个**候选，避免 `hhd800.com@ABW-180.mp4` 这类前缀误报 |
| 请求侧 | 番号本身；若番号带上下集标记，同时映射到"番号+集数"键 | 只有带集数标记的文件会命中该键，所以请求单集不会报裸番号的作品 |

- 同一收藏被多个键命中时只报一次。
- 结果按 `createdAt` 降序、`videoId` 降序。
- 回收站与归档条目照常返回并标 `inTrash: true`——"记录还在回收站里"和"没收藏"
  是两个不同的答案，只有调用方知道该怎么展示。
- FC2：JavDB 写作 `FC2-PPV-1234567`，Muse 存 `FC2-1234567`（FC2 提供方与文件名
  提取器都用这个写法），`LookupKey` 负责折叠。

## 5. CORS 面

只有 `/api/extension/check` 带 CORS，且只在来源属于下列之一时回写
`Access-Control-Allow-Origin`：

- `javdb.com`（永久域名）
- `javdb<数字>.com`、`app.javdb<数字>.com`（官方轮换的镜像）
- `jav.app`、`*.jav.app`
- `localhost` / `127.0.0.1` / `[::1]`（开发时的本地样例页）

其余来源一律不写头（浏览器自行拦截）；来自非法来源的预检回 `403`。响应始终带
`Vary: Origin`，且**不允许凭据**（端点不收 Cookie，服务端也没有会话）。

**域名白名单有两处，必须同步修改**：

1. `browser-extension/manifest.json` 的 `content_scripts[].matches`
2. `backend/server/extension.go` 的 `extensionOriginAllowed`

`scripts/check-extension.mjs` 会检查两者是否对得上（CI 与打包脚本都会跑）。

现有 `/api/*` 保持同源行为，有回归测试锁死（`TestAPIExtensionCORSIsScoped`）。

## 6. 扩展行为

| 场景 | 表现 |
|---|---|
| 卡片有番号、未查询 | 灰色 `·`，悬停后查询 |
| 查询中 | `…` |
| 命中 | 绿色 `✓`（多版本显示数量） |
| 未命中 | 灰色 `✗` |
| 仅在回收站 | 黄色 `🗑` |
| Muse 未运行 / 超时 / HTTP 错误 | 红色 `⚠`，整页最多提示一次 |
| 卡片没有番号字段 | 虚线 `?`，点击说明原因，不发请求 |

索引与节流：

- 悬停触发，200 ms 合并窗口；同一批内重复番号只查一次。
- 同番号结果缓存 60 s；切换回标签页时清空并重扫。
- 只在悬停时查询：页面加载、滚动本身不产生任何请求，因此没有可见卡片预取。
- `MutationObserver`（`childList + subtree`，120 ms 防抖）覆盖翻页与无限滚动。
- 一页 40–60 张卡片、悬停 10 张以内 → 通常 1–2 个请求。

安全约束：

- 只向 `127.0.0.1:8899` / `localhost:8899` 发请求（写死在 `host_permissions`）。
- 只读，不修改页面 DOM 的属性与内容，只追加自己的角标 / 浮层 / toast 节点。
- 角标在卡片链接内部，点击一律 `preventDefault + stopPropagation`，不会误打开作品页。
- 跳转 Muse 详情走扩展自己的 `open.html` 跳板页，只接受 `http:` / `https:` 目标。

## 7. 站点改版兜底

`content.js` 顶部两个常量是唯一描述站点结构的地方：

```js
const CARD_SELECTOR = ".movie-list .item a.box, #videos .item a.box, …";
const CODE_SELECTOR = ".video-title strong";
```

页面渲染出 0 张卡片时不放弃：`MutationObserver` 继续观察，另外 10 s 后再重扫一次。

## 8. 已知限制

- 端口固定 8899。MV3 的站点权限只能按具体端口声明，动态申请会给每次安装增加一次
  授权弹窗，因此不做运行时授权；改端口需要改 `manifest.json` 并重新加载扩展。
  桌面端「设置 → 浏览器扩展」会提示这一约束。
- Muse 未运行时无法查询（角标显示未连接），不做离线缓存。
- 仅桌面 Chrome / Edge（MV3），Firefox 需要单独的 `browser_specific_settings` 适配。
- JavDB 页面若把片号渲染成图片或拆成多个文本节点，`CODE_SELECTOR` 需要调整。

## 9. 验收清单

代码层面（可自动执行）：

```bash
go test ./...                          # 门面、端点、CORS 白名单
node scripts/check-extension.mjs       # manifest/脚本/来源白名单一致性
( cd frontend && npm run lint && npm run build )
```

手工验收（需要一个能访问 JavDB 的浏览器）：

1. Muse 运行中，打开 JavDB 搜索页 → 卡片出现角标，悬停显示 `✓` / `✗`。
2. `设置 → 浏览器扩展 → 测试查询` → 显示「查询成功」。
3. 停止局域网服务 → 角标变 `⚠`，页面只弹一次提示，不刷屏。
4. 翻页 / 向下滚动加载更多 → 新卡片照样有角标。
5. 40 条的一页，DevTools Network 中 `/api/extension/check` 请求 ≤ 3。
6. 点击命中项 → 新标签页打开 `http://127.0.0.1:8899/videos/<id>`。
7. 打开任意其他站点 → 没有任何角标（内容脚本不注入）。
8. 在 JavDB 上 `curl` 一个没有来源头的请求（或另一个站点的脚本）→ 浏览器拦截，
   服务端不返回 `Access-Control-Allow-Origin`。

## 10. 相关文件

| 文件 | 作用 |
|---|---|
| `browser-extension/manifest.json` | MV3 清单、权限、内容脚本匹配（白名单之一） |
| `browser-extension/content.js` | 读页面番号、角标、浮层、批合并与缓存 |
| `browser-extension/background.js` | 唯一联网的地方：`check` / `openVideo` / `testConnection` |
| `browser-extension/config.js` | Muse 地址与超时（三个上下文共用） |
| `browser-extension/options.html` / `.js` | 地址配置、连接测试、安装说明 |
| `browser-extension/open.html` / `.js` | 打开 Muse 详情的跳板页 |
| `backend/server/extension.go` | 端点、CORS 包装、来源白名单（白名单之二） |
| `backend/api/library.go` | `MatchJavCodes`：番号匹配与结果投影 |
| `backend/jav/check.go` | `JavCodeCheck` / `JavCodeMatch` / `LookupKey` |
| `frontend/src/pages/Settings.tsx` | 「浏览器扩展」设置页 |
| `scripts/check-extension.mjs` | 扩展自检（打包与 CI 都跑） |
