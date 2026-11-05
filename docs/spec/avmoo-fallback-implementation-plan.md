# Avmoo 数据源与 JavBus Fallback 实施计划

**状态：** Implementation plan / implementation in progress  
**范围：** Muse Go 后端 `backend/jav` 与本说明文档  
**非范围：** 数据库 schema、前端 Provider 配置、JavMenu、AVSOX 和其他 Provider

## 1. 背景和目标

Muse 当前由 `JavService` 通过 JavBus Provider 获取番号元数据。JavBus 遇到年龄验证、403、driver-verify 或挑战页时，任务会直接进入 blocked。目标是新增 Avmoo Provider，并把默认链路调整为：

```text
JavBus
  └─ 当 Lookup 返回 LookupError{Kind: "blocked"} 或 "not_found" 时
       └─ Avmoo
```

timeout、429、5xx、网络错误和解析失败不触发 fallback，继续由现有任务状态机处理。成功结果中的 `Metadata.Provider` 必须反映实际来源。

## 2. 当前 Muse 刮削链路

视频导入后，`api.Library` 异步调用 `JavService.Enqueue`。Service 从手动覆盖或文件名提取番号，Claim 任务后调用 `jav.Provider.Lookup`，保存 `video_jav` 元数据，独立下载远程封面，最后把任务置为 succeeded。网络和封面失败不阻塞视频导入、缩略图生成和播放。

文件名可能同时包含站点域名和真实番号，例如 `hhd800.com@ABW-180.mp4` 会提取出 `HHD-800`、`ABW-180` 两个候选。Service 按出现顺序尝试候选；只有候选最终返回 `not_found` 时才继续下一个候选，blocked、retryable、network、parse 和 http 等其他错误立即结束任务。整个候选序列共用一次任务 lease 和一次 attempt。

Provider 接口保持最小化：

```go
type Provider interface {
    Name() string
    Lookup(context.Context, string) (*Metadata, error)
}
```

封面请求继续使用 Service 侧可选 fetcher 接口，以保持已有测试 Provider 和第三方实现的源兼容性。

## 3. JavBoss Avmoo 实现参考

参考实现位于 JavBoss 的 `internal/jav/avmoo.go`。Muse 只移植 API 查询所需的行为，不移植 JavBoss 的作品缓存、其他 Provider 路由或 HTML legacy parser：

- 默认地址 `https://avmoo.shop`，当前 API 语言 `cn`；
- `/{lang}/search/{code}` 页面建立 CSRF/session；
- `POST /jav/data/api/search` 返回候选；
- `POST /jav/data/api/getMovie` 返回详情；
- 搜索结果按规范化后的 `movieFanHao` 精确匹配，并要求 `movieId` 非空。

## 4. Avmoo Provider

实现文件：`backend/jav/providers/avmoo.go`。

### 4.1 Session 和请求

每个 Avmoo Provider 实例缓存一个 session（CSRF token、Cookie、Referer）30 分钟。缓存不使用全局状态，避免测试 Server 或不同运行实例互相污染。Session 页面和 API 使用浏览器风格 User-Agent、Accept、Accept-Language、Referer、Origin、`X-Requested-With` 和 `X-CSRF-Token`。HTTP transport 继续使用 `http.ProxyFromEnvironment`；测试构造函数支持注入 `httptest.Server` 和自定义 client。

每个 Provider 使用独立的约 1.5 秒 interval limiter，详情、API、session 和封面请求共用该 Provider 的额度。响应体由 `io.LimitReader` 限制在 8 MiB 内。

当 API 返回 HTTP/API 状态 400、401、403 或 419 时，当前 session 被清理，重新建立 Session，并把完整 search/detail 流程重试一次。不会把 Cookie 或 CSRF token 写入日志或错误消息。

### 4.2 API 和错误边界

Search body（API 语言参数为 `cn`）：

```json
[{"search":"IPX-228","lang":"cn"},30,1]
```

Detail body：

```json
["<movie-id>","tw"]
```

响应 envelope 为 `{code,message,data}`。API 请求对 timeout、429、5xx 做最多三次有限重试；无结果、400/401/403/419、解析失败不在该循环中重试。无结果返回 `LookupError{Kind: "not_found"}`；拒绝访问返回 `blocked`；429/5xx/timeout 返回 `retryable`；其他网络问题返回 `network`；响应解析问题返回 `parse`。组合 Provider 对 JavBus 的 `blocked` 和 `not_found` 继续查询 Avmoo。

番号规范化为大写后只保留 ASCII 字母和数字，再做全字符串相等比较，因此 `IPX-228` 不会命中 `IPX-228R`。

### 4.3 Metadata 映射

Avmoo 结果映射到 Muse `jav.Metadata`：

- `Code` ← `movieFanHao`；
- `Title` ← `title`、`title_tw`、`title_cn`、`title_ja`、`title_en`；
- `Studio`、`Series` ← 对应 nested name 字段；
- `ReleaseUnix` ← `releaseDate`；
- `DurationMin` ← `length`；
- `Actors` ← `star[]`；
- `Tags` ← `genre[]`；
- `CoverURL` ← `posterLarge` 再回退 `posterSmall`；
- `IsUncensored=false`；
- `Provider="avmoo"`。

Actors 和 Tags 按响应顺序去重并丢弃空值。

## 5. Fallback Provider

实现文件：`backend/jav/providers/fallback.go`。

组合 Provider 先调用 JavBus。只有错误可以 `errors.As` 为 `*jav.LookupError` 且 `Kind` 为 `blocked` 或 `not_found` 时才调用 Avmoo。JavBus 成功时 Avmoo 调用次数为零；`retryable`、`network`、`parse`、`http` 不触发 fallback。Avmoo 的最终错误原样交给 `JavService` 状态映射。文件名候选由 `JavService` 依次尝试；候选级 `not_found` 才会继续下一个候选，不会把 blocked 或可重试错误吞掉。

组合 Provider 的 `Name()` 返回 primary 名称（保持现有日志和兼容性）。fallback 成功结果的 `Provider` 保留 Avmoo 名称；组合日志额外记录 primary、fallback 和番号。

## 6. 封面请求路由

`JavService.processCover` 和 `downloadCover` 接收 `providerName`。新增可选接口：

```go
type namedURLResponseFetcher interface {
    FetchFor(context.Context, providerName, rawURL string) (*http.Response, error)
}
```

组合 Provider 按 `providerName` 选择 JavBus 或 Avmoo。JavBus 继续使用详情页 Referer、Cookie 和原有 limiter；Avmoo 使用自己的 client、limiter 和 headers，不向外部图片 CDN 转发 Avmoo Session Cookie。未实现命名接口的旧测试 Provider 继续走原有 FetchCover/Fetch/默认 HTTP client 兼容路径。

封面失败仍只更新 `cover_status=failed`，元数据任务继续 succeeded。

## 7. 应用启动 wiring

`main.go` 保留 JavBus Cookie 的读取、持久化、设置和清除逻辑，且 `app.setJavBusCookieProvider` 仍绑定 JavBus 实例。启动时分别创建 JavBus、Avmoo 和 JavBus→Avmoo 组合 Provider；`jav.NewService` 接收组合 Provider。Avmoo 生产地址固定，不增加设置项或环境变量。

## 8. 测试计划

### Avmoo Provider

使用 `httptest.Server` 验证：

- Session 页面 token、Cookie、请求头和 30 分钟复用；
- search/detail 方法、路径、payload、headers；
- 完整 metadata 字段、演员/标签顺序与去重；
- `IPX-228` 不匹配 `IPX-228R`；
- not_found、401/403/419 session refresh、429/5xx/timeout 分类；
- response body 上限；
- Avmoo cover 使用独立请求头且不发送 Session Cookie；
- 错误和日志不暴露 Cookie/CSRF。

### Fallback 和 Service

验证 JavBus 成功不访问 Avmoo；blocked 或 not_found 时 fallback；其他错误不 fallback；fallback 成功持久化 `provider=avmoo` 且封面走 Avmoo；Avmoo 封面失败不改变元数据成功状态；现有 fake/metadata Provider 和封面覆盖逻辑保持兼容。

## 9. 验收标准

1. JavBus 正常时只访问 JavBus。
2. JavBus 拒绝访问类错误自动尝试 Avmoo。
3. timeout、429、5xx、普通解析失败不触发 Avmoo；JavBus 的 not_found 会触发 Avmoo fallback。
4. Avmoo 能保存番号、标题、片商、系列、日期、时长、演员、标签和封面 URL。
5. 数据库 provider 准确保存 javbus 或 avmoo。
6. Avmoo 成功后的封面使用 Avmoo 请求配置。
7. JavBus Cookie 设置/清除只影响 JavBus。
8. Provider 失败不阻塞视频导入、缩略图或播放。
9. 现有 JavBus 测试和 Go 测试通过。
10. 默认测试不访问真实 Avmoo；真实网络验证必须显式 opt-in。

## 10. 非目标和剩余风险

不实现验证码处理、JS challenge 破解、代理池、IP 轮换、账号池、数据库 schema、前端 Provider 切换或其他 Provider。Avmoo API/HTML 结构变化、站点不可用、TLS/网络策略和封面 CDN 访问限制仍可能导致任务失败；这些情况由现有 retry、blocked、cover_failed 状态和手动重试处理。
