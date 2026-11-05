# FC2CMADB 服务端刮削

## 文件名与查询号

Muse 对包含 FC2 标记的文件名优先使用 FC2 专用解析器，支持 `FC2-PPV-4970405`、`FC2PPV4970405`、`FC2-4970405` 及下划线/空格变体。内部候选号统一为 `FC2-4970405`，请求 FC2CMADB 时只取数字并访问：

```text
https://fc2cmadb.com/articles/4970405
```

命中 FC2 标记但没有合法数字时，不会回退到通用字母+数字正则，避免把 `FC2`、分辨率或编码参数误当成普通番号。FC2 结果也不会回退到 JavBus/Avmoo。

## 登录态

FC2CMADB 文章页需要已完成年龄确认的登录态。设置页接收浏览器请求中的完整 `Cookie` 请求头，例如包含 `ageVerified=true`、站点 session 以及必要的 Cloudflare 会话值。

- Windows 使用 DPAPI 加密后存储在 SQLite settings 表中；数据库不保存明文。
- 运行时只将解密后的值保存在 FC2 Provider 实例内存中。
- 前端只显示“已配置/未配置”，不会回显 Cookie。
- 清除设置会同时删除密文并清空运行中的 Provider；重新复制 Cookie 后无需重启。
- 非 Windows 构建没有明文回退，当前会报告安全存储不可用。

Cookie 不写入日志、错误文本、测试 fixture 或网络测试命令行。环境变量 `MUSE_FC2_COOKIE` 仅作为启动时的临时覆盖来源，生产部署应优先使用设置页的 DPAPI 存储。

## 站点与错误处理

Provider 使用实例级 HTTP client、限速器、超时和 Cookie。默认每次请求之间留出约 1.5 秒间隔，减少批量扫描触发 Cloudflare 429 的概率；遇到 429/5xx 交由任务服务的 retryable 退避处理。

| 情况 | 任务分类 |
| --- | --- |
| 文章不存在 | `not_found` |
| 429、5xx | `retryable` |
| 401/403、年龄确认、登录页、Cloudflare challenge | `blocked` / `provider_auth` |
| 结构改变、没有作品标题 | `parse` |

不实现 JS challenge 破解、验证码绕过、IP 轮换、代理池或账号池。登录态失效时重新录入 Cookie，再对失败任务执行重试。

## 元数据与封面

Provider 优先解析 Inertia/JSON-LD 和 OpenGraph，再回退到标题、带标签字段、标签/演员链接。保存的 Provider 名称为 `fc2cmadb`，Code 为 `FC2-数字`。封面下载由现有 JavService 负责图片解码、JPEG 转换、临时文件和原子替换。

文章页和 FC2CMADB 同站封面请求可以携带 FC2 Cookie；外部图片 CDN 不携带 Cookie。重定向逐跳检查目标，跨站跳转会拒绝或清除 Cookie，避免会话泄露。

## 运行与验证

默认 `go test ./...` 完全离线，Provider 测试使用 `httptest.Server`。如需真实站点验证，应显式 opt-in，使用脱敏日志和环境变量，不得把 Cookie 写入源码、fixture、命令行或 CI 输出。

## 当前阻塞与后续处理记录（2026-09-05）

### 已确认的事实

- FC2 不会回退到 JavBus/Avmoo。组合路由已按 `FC2-` 前缀选择 `fc2cmadb`；此前日志里的 `provider=javbus` 是组合 Provider 名称，诊断版已改为 `jav-router`，并额外记录 `route=fc2cmadb`。
- 文件名提取规则已修复：支持 6～7 位数字、`FC2-PPV-<id>_1`/`-1`/`_hd`、`hhd800.com@` 前缀及 `(FC2)(<id>)`。此前数据库中 70 个 FC2 `no_code` 任务里，69 个是数字后 `_part` 导致 Go `\b` 边界失败；修复后需强制重刮这些历史任务。
- Cookie 设置使用 Windows DPAPI，运行时 Cookie 只在 Provider 内存中保存。设置层、Provider 构造和热更新都剥离可选的 `Cookie:` 前缀。
- 诊断版日志会输出但不会泄露 Cookie 值：`cookieConfigured`、`cookieSent`、`hasAgeVerified`、`hasSession`、请求 host/path、HTTP status、最终 host/path、是否重定向和页面分类。
- FC2 请求已按浏览器请求补齐 Edge 152 相关 headers：完整 `Accept`、`Accept-Language`、`Cache-Control`、`DNT`、`Priority`、`Sec-CH-UA*`、`Sec-Fetch-*`、`Upgrade-Insecure-Requests`、Edge 152 User-Agent。

### 真实请求证据：FC2-4749992

使用应用数据库中 DPAPI 解密出的 Cookie，对 `https://fc2cmadb.com/articles/4749992` 做过一次单次真实请求。脱敏结果：

```text
status=200
final_url=https://fc2cmadb.com/articles/4749992
redirected=false
cookieConfigured=true
cookieSent=true
hasAgeVerified=true
hasSession=true
classification=age_gate   # 这是当前旧分类，后续页面检查证明它是误判
```

进一步检查同一响应（仅输出结构摘要，没有保存或打印完整 HTML）：

```text
Content-Type=text/html; charset=utf-8
body_bytes=95204
title=FC2CMADB
h1/h2/h3/button/form/img=none
scripts=6
contains[18歳以上です]=false
contains[年齢確認]=false
contains[cf-chl-]=false
contains[challenge-platform]=false
contains[just a moment]=false
contains[checking your browser]=false
contains[inertia]=true
contains[article]=true
visible_text begins with FC2CMADB + Laravel/Ziggy route configuration
```

### 当前主阻塞：只拿到 Inertia/Laravel 前端壳，没有文章数据

当前 Go Provider 只对 `/articles/{id}` 发起一次普通 HTML GET，然后直接解析初始 HTML。真实返回页面是一个约 95 KB 的 Laravel/Inertia SPA 壳：没有文章标题、hero、图片、按钮或表单，只有 6 个 script、Ziggy 路由配置和前端应用代码。文章数据很可能由页面 JavaScript 启动后通过 Inertia deferred props 或后续 XHR/fetch 请求加载。

因此当前失败的核心不是番号、Cookie 或重定向，而是：

1. 首次 HTML 请求成功返回 200；
2. Cookie 已发送且包含 `ageVerified=true` 和站点 session；
3. 初始 HTML 不包含可直接保存的作品元数据；
4. Provider 没有执行浏览器 JavaScript，也没有复现后续 Inertia/XHR 数据请求；
5. 页面正文的 JavaScript 字符串包含 `age verification`，导致旧版宽泛检测误判为 `age_gate`。

### 需要在下一会话解决的技术任务

1. **先逆向 deferred 数据请求，而不是继续放宽 HTML marker**
   - 使用浏览器开发者工具或脱敏网络记录，观察打开 `/articles/4749992` 后的 XHR/fetch 请求。
   - 记录请求方法、路径、查询参数、`X-Inertia`/`X-Inertia-Version`/`X-Requested-With`、`Accept`、Referer 和响应 JSON 结构。
   - 不把 Cookie 值写入 fixture；使用字段名、状态码和脱敏 JSON 结构即可。

2. **实现 Provider 的第二阶段请求**
   - 首次页面响应只负责提取 Inertia version、CSRF/meta 信息和必要的页面状态。
   - 按真实浏览器请求调用 deferred/XHR endpoint，使用同一实例 Cookie 和浏览器 headers。
   - 对 JSON 响应设置大小上限、超时和精确字段映射。
   - 继续禁止跨站重定向和 Cookie 外泄。

3. **收紧页面分类**
   - 不要因为整页 HTML/JavaScript 中出现 `age verification`、`article` 或 `cloudflare` 字符串就判定年龄门。
   - 只有明确的年龄确认控件/文案、Cloudflare challenge 页面结构、401/403 或最终 URL challenge 才标记 `provider_auth`。
   - “页面壳但没有元数据”应分类为 `parse` 或专门的 `deferred_data_missing`，不能伪装成年龄验证。

4. **补充真实结构的离线 fixture**
   - 初始 Inertia shell fixture；
   - deferred JSON/XHR fixture；
   - 真实文章元数据映射；
   - 年龄门、Cloudflare challenge、登录页和正常 shell 的区分；
   - 验证 `4749992` 的数字 ID只用于请求路径，落库 Code 仍为 `FC2-4749992`。

5. **历史任务恢复**
   - 修复 Provider 后，对 `blocked/provider_auth` 和旧 `no_code` FC2 任务执行强制重刮。
   - `blocked` 任务不会自动重试；保存 Cookie 也不会自动重刷全库，这是为了避免意外触发大量 FC2 请求和限流。

### 当前诊断版

当前诊断可执行文件：

```text
D:\muse-v2\muse-app\build\bin\Muse-fc2-diagnostics.exe
```

它只增加脱敏诊断日志，不绕过年龄验证或 Cloudflare challenge。临时真实请求探针和页面检查探针均在使用后删除，未保存 Cookie 或完整响应。
