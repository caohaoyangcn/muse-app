# FC2CMADB 浏览器结构调查报告

> 调查日期：2026-09-05  
> 调查方式：真实浏览器 / OpenCLI 观察；报告只记录脱敏后的结构、字段和结论，不保存 Cookie、认证信息或完整真实 HTML。  
> 样本：`https://fc2cmadb.com/articles/4749992`

## 1. 结论摘要

1. 样本文章页 HTTP 状态为 `200`，页面标题正常；浏览器 DOM 中年龄确认复选框 `#age-verification-modal` 未勾选，正文没有年龄提示。因此这次样本不是年龄门，也不是 Cloudflare challenge。
2. 初始 HTML 包含以下 Inertia 启动数据节点：

   ```html
   <script type="application/json" data-page="app">…</script>
   ```

   其 JSON 顶层包含 `component`、`version`、`props`、`sharedProps`、`deferredProps`。`component` 为 `Articles/Show`；`props.article` 已经是完整文章对象。
3. `deferredProps.default` 为 `["actresses"]`。浏览器随后对同一文章 URL 发起的 Inertia JSON 响应只带 `props.actresses`，这是 deferred/partial props，不是文章主体的唯一来源。
4. 当前实现的主要缺口是没有读取 `data-page` Inertia envelope，而不是需要另找一个独立的 article API。首选方案是解析初始 HTML 中的 `props.article`；演员列表通过 deferred props best-effort 获取。
5. 当前 `fc2BlockedPage` 对脚本中的 `age verification` 宽泛匹配，会把正常页面误判为年龄门。应改为基于实际控件/正文或明确 challenge 结构的精确分类。

## 2. 真实浏览器观察结果

### 2.1 页面和年龄确认状态

| 项目 | 观察结果 |
| --- | --- |
| 请求 URL | `https://fc2cmadb.com/articles/4749992` |
| HTTP 状态 | `200` |
| 页面标题 | 正常（未记录完整标题文本） |
| 年龄复选框 | `#age-verification-modal` 存在但未勾选 |
| 页面正文 | 无年龄提示 |
| 页面结论 | 正常文章页，不应分类为 `age_gate` |

页面脚本中可能出现与年龄验证相关的 JavaScript 文本，不能仅凭整页脚本文本出现 `age verification` 就判定用户遇到年龄门。应区分“脚本/应用代码中的字符串”和“可见年龄门控件或明确阻断页面”。

### 2.2 Inertia 初始 envelope

初始 HTML 中存在 `script[type="application/json"][data-page="app"]`。脱敏后的结构如下；此处仅展示字段形状，不包含完整真实 JSON：

```json
{
  "component": "Articles/Show",
  "version": "<inertia-version>",
  "props": {
    "article": {
      "id": 730132,
      "title": "<已脱敏标题>",
      "video_id": 4749992,
      "censored": "無",
      "uncensored_image": "<可能为空或已脱敏>",
      "not_found": false,
      "status": "<正常状态>",
      "release_date": "2025-08-29",
      "duration": "01:12:49",
      "writer_id": "<脱敏>",
      "image_url": "/storage/images/article/no-image.jpg",
      "sale_percentage": 50,
      "sale_limite_date": "<未记录>",
      "love_reactant_id": "<未记录>",
      "bookmark_count": "<未记录>",
      "like_count": 8,
      "dislike_count": 0,
      "model_name": "<未记录>",
      "writer": {
        "slug": "<已脱敏 writer slug>",
        "name": "<已脱敏 writer name>"
      },
      "tags": "<14 个标签，名称未逐一记录>",
      "affiliate_links": "<未记录>",
      "comments": "<未记录>",
      "love_reactant": "<未记录>"
    }
  },
  "sharedProps": "<结构存在，内容未记录>",
  "deferredProps": {
    "default": ["actresses"]
  }
}
```

已确认 `props.article` 是完整文章对象，字段集合为：

```text
id, title, video_id, censored, uncensored_image, not_found, status,
release_date, duration, writer_id, image_url, sale_percentage,
sale_limite_date, love_reactant_id, bookmark_count, like_count,
dislike_count, model_name, writer, tags, affiliate_links, comments,
love_reactant
```

### 2.3 后续 Inertia 请求

浏览器网络中观察到打开文章页后，同一 URL 还有后续 Inertia JSON 响应。该响应只包含 `props.actresses`，与初始页面的 `deferredProps.default = ["actresses"]` 相符。

这说明：

- `props.article` 在初始 `data-page` envelope 中已经提供；
- 后续响应是 deferred/partial props，用来补充演员列表；
- 后续响应不能被当成文章主体 API；
- 没有证据表明需要另找独立的 article API；
- 即使演员补充请求失败，也应尽量保留已解析的文章元数据。

具体网络请求中的 Cookie 值、session 值和完整响应正文不写入本报告。

## 3. 样本字段的有限引用

以下只保留实现验证所需的最小、脱敏或有限信息：

| 字段 | 样本观察 | 处理说明 |
| --- | --- | --- |
| `article.id` | `730132` | 站内文章主键；不要误当作请求用的 FC2 video ID |
| `article.video_id` | `4749992` | 请求路径使用该数字；落库 Code 仍建议为 `FC2-4749992` |
| `article.title` | 存在，实际文本脱敏 | 作为标题主来源 |
| `article.release_date` | `2025-08-29` | 转为 `ReleaseUnix` |
| `article.duration` | `01:12:49` | 转为现有模型的分钟字段；推荐固定采用向下取整策略，即约 72 分钟 |
| `article.censored` | `無` | 表示未打码时可映射为 `IsUncensored=true`；未知值不得强行推断 |
| `article.image_url` | `/storage/images/article/no-image.jpg` | 相对 URL，需按文章 URL 解析；这是一个 no-image 占位图，应允许后续封面策略跳过或降级 |
| `article.uncensored_image` | 字段存在，实际值未记录 | 有有效值时可作为封面候选 |
| `article.sale_percentage` | `50` | 当前 `jav.Metadata` 没有对应持久化字段，暂不映射 |
| `article.like_count` | `8` | 当前 `jav.Metadata` 没有对应字段 |
| `article.dislike_count` | `0` | 当前 `jav.Metadata` 没有对应字段 |
| `article.tags` | `14` 个标签 | 提取名称并去重后映射到 `Tags` |
| `article.writer` | 有 slug/name | 仅保留为文章源字段或诊断信息；不要错误映射为演员 |

本报告不记录真实用户 Cookie、邮箱、IP、session、完整认证信息或完整页面响应。

## 4. 当前代码对照

目标文件：`backend/jav/providers/fc2cmadb.go`

### 4.1 `Lookup` 目前只有普通 GET

当前 `Lookup` 的流程是：

1. 规范化输入编号；
2. 对 `/articles/{id}` 发起一次普通 `GET`；
3. 做状态码和页面阻断分类；
4. 调用 `parseFC2Metadata` 解析初始 body；
5. 没有执行浏览器 JavaScript，也没有读取 Inertia `props.article` 的嵌套对象；
6. 没有实现 deferred props 的 best-effort 请求。

因此当前实现不能完整利用样本页已经返回的文章数据。

### 4.2 `parseFC2Metadata` / `parseFC2JSONScripts` 的缺口

当前解析器能够处理：

- OpenGraph / Twitter meta 标签；
- 标题、标签、演员等 HTML 文本或链接；
- `application/ld+json` 或部分扁平文章 JSON。

但 `parseFC2JSONScripts` 将 JSON 直接反序列化为扁平的 `fc2ArticleJSON`，没有识别：

```text
script[data-page="app"]
  -> top-level props
  -> props.article
```

因此即使脚本 JSON 已经含有完整文章对象，现有代码也不会稳定提取 `title`、`release_date`、`duration`、`tags` 等字段。

### 4.3 `fc2BlockedPage` 的误判

当前页面分类逻辑会扫描整页 body，并将以下字符串作为阻断标记之一：

```text
age verification
age gate
```

正常 Inertia/Laravel 前端脚本可能包含这些字符串。对本样本而言，浏览器确认年龄复选框未勾选、正文无年龄提示，但宽泛脚本匹配仍可能返回 `age_gate`。分类逻辑应改为：

- 年龄门：要求可见/结构化年龄确认控件或明确年龄提示，并结合实际阻断页面状态；
- Cloudflare challenge：要求 challenge 页面结构/响应特征，而不是普通应用脚本中的字符串；
- 初始 shell 没有可解析文章：归类为 `parse` 或专门的 `deferred_data_missing`；
- 不因 `article`、`age verification` 或普通 `cloudflare` 字样单独阻断。

## 5. 建议的数据字段映射

现有目标模型为 `backend/jav/model.go` 中的 `jav.Metadata`。建议按以下优先级映射：

| FC2CMADB 来源 | `jav.Metadata` 目标 | 建议 |
| --- | --- | --- |
| `video_id` | `Code` 间接使用 | 请求路径使用数字；最终 Code 为 `FC2-` + 数字，例如 `FC2-4749992` |
| `title` | `Title` | 首选来源；再按现有 `cleanFC2Title` 清理站点后缀 |
| `release_date` | `ReleaseUnix` | 解析 `YYYY-MM-DD`，按 UTC 生成日期时间；非法日期视为缺失 |
| `duration` | `DurationMin` | 解析 `HH:MM:SS`、分钟数或数字；模型只有整数分钟，推荐向下取整并在测试中固定行为 |
| `censored` | `IsUncensored` | `無` 可视为 true；只对明确枚举值映射，未知值保持 nil |
| `uncensored_image` | `CoverURL` 候选 1 | 有效且非占位值时优先使用 |
| `image_url` | `CoverURL` 候选 2 | 解析相对 URL；`no-image.jpg` 应识别为占位图并允许降级 |
| `tags` | `Tags` | 支持字符串或对象的 `name`/`title`/`label`，去重、清理空值 |
| `actresses`（deferred） | `Actors` | 第二阶段请求 best-effort；缺失或失败不应抹掉文章主体 |
| `writer` | 无直接目标 | 不映射为 `Actors`；如未来需要应增加明确的 writer 字段，而不是复用演员 |
| `id` | 无直接目标 | 作为诊断/源数据字段；不能替换 `video_id` |
| `status`, `not_found` | 解析/错误分类 | 文章状态异常或 `not_found=true` 时参与分类，不当作标题等元数据 |
| `sale_percentage`, `sale_limite_date` | 无直接目标 | 仅在未来扩展销售字段时使用 |
| `like_count`, `dislike_count`, `bookmark_count` | 无直接目标 | 不写入现有作品元数据 |
| `writer_id`, `love_reactant_id`, `love_reactant` | 无直接目标 | 不为满足字段数量而强行映射 |
| `affiliate_links`, `comments`, `model_name` | 无直接目标 | 当前刮削目标不需要；避免抓取或持久化无关内容 |

`article.id=730132` 与 `video_id=4749992` 必须在测试中明确区分，防止把文章主键保存成番号。

## 6. 推荐请求顺序和伪代码

### 6.1 请求策略

推荐把一次 Lookup 设计为“初始文档 + 可选 deferred 补充”两阶段：

1. 对文章 URL 发起初始文档 GET；
2. 校验 HTTP 状态和最终 URL；
3. 从 `script[type="application/json"][data-page="app"]` 解码 envelope；
4. 直接解析 `props.article`，只要文章对象有效，就先生成主体元数据；
5. 发现 `deferredProps.default` 含 `actresses` 时，按真实浏览器的 Inertia partial 约定对同一 URL 发起第二请求；
6. 第二请求只用于补充 `props.actresses`，失败时保留第一阶段结果；
7. 所有响应都使用现有超时、大小上限、同站点重定向限制和限速器。

### 6.2 Go 风格伪代码

```go
func lookupFC2(ctx context.Context, articleURL *url.URL) (*jav.Metadata, error) {
    htmlBody, response, err := getDocument(ctx, articleURL)
    if err != nil {
        return nil, classifyNetworkError(err)
    }
    if err := classifyHTTP(response); err != nil {
        return nil, err
    }
    if classifyVisibleBlock(response, htmlBody) != "ok" {
        return nil, classifyProviderBlock(response, htmlBody)
    }

    page, ok := parseInertiaAppScript(htmlBody)
    if !ok {
        // 保留 JSON-LD / OpenGraph / HTML fallback。
        return parseFallbackOrParseError(htmlBody, articleURL)
    }

    article, ok := page.Props.Article
    if !ok || article.NotFound {
        return nil, lookupParseError("props.article missing or not found")
    }

    metadata := mapFC2Article(article, articleURL)

    if contains(page.DeferredProps.Default, "actresses") {
        // 同一 URL 的 Inertia partial/deferred 请求；不另造 article API。
        if actresses, err := fetchDeferredActresses(ctx, articleURL, page); err == nil {
            metadata.Actors = uniqueFC2(actresses)
        }
        // 演员是 best-effort，第二请求失败不覆盖主体文章结果。
    }
    return metadata, nil
}
```

解析 `data-page` 时应只选择 `type="application/json"` 且 `data-page="app"` 的 script，先做 JSON 解码和类型校验，再访问 `props.article`；不能对任意包含 `article` 字符串的脚本直接尝试扁平反序列化。

### 6.3 请求头建议

以下是实现 Inertia partial 时的建议，不是把认证值写入代码或 fixture：

初始文档 GET 继续使用现有浏览器风格请求头：

```text
User-Agent: <configured browser-like UA>
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8
Accept-Language: zh-CN,zh;q=0.9,en;q=0.8
Referer: https://fc2cmadb.com/
Sec-Fetch-Dest: document
Sec-Fetch-Mode: navigate
Sec-Fetch-Site: same-origin
```

第二阶段 Inertia 请求建议：

```text
Accept: text/html, application/xhtml+xml
X-Inertia: true
X-Inertia-Version: <从初始 data-page.version 读取>
X-Requested-With: XMLHttpRequest
Referer: https://fc2cmadb.com/articles/4749992
```

实现时应以真实浏览器脱敏网络记录为准，尤其是 Inertia 版本和 partial/deferred 所需的查询参数或头字段。Cookie 只能沿用现有运行时安全配置，不能出现在日志、伪代码中的真实值、fixture、提交记录或报告中。

## 7. 错误处理和限速

建议保留并细化现有分类：

| 场景 | 建议分类 | 行为 |
| --- | --- | --- |
| HTTP 404 或明确 `not_found` | `not_found` | 不重试 |
| HTTP 429 | `retryable` / `rate_limited` | 尊重 `Retry-After`（若有），指数退避并限制最大次数 |
| HTTP 5xx | `retryable` | 退避重试；不要立即重复请求 |
| HTTP 401/403 | `provider-auth` | 报告认证/权限问题，不伪装成 parse failure |
| 明确年龄确认控件/正文阻断 | `provider-auth` 或 `age_gate` | 仅在证据明确时分类 |
| 明确 Cloudflare challenge 页面 | `provider-auth` / challenge | 不实现验证码或 JS challenge 绕过 |
| 200 但缺少 `data-page` | `parse`，然后 JSON-LD/HTML fallback | 不因缺少 Inertia 就直接判年龄门 |
| 200 且缺少有效 `props.article` | `parse` / `deferred_data_missing` | 记录结构性失败；不要把 deferred `actresses` 当文章主体 |
| deferred `actresses` 请求失败 | 非致命 | 返回主体文章，演员列表为空或保留已有 fallback |
| JSON 超限、解码失败、字段类型错误 | `parse` | 响应大小有限制，字段按类型严格校验 |

现有约 1.5 秒请求间隔应继续保留。第二请求会增加请求数，因此应确保：

- 只有确实声明了 `actresses` deferred prop 时才请求第二阶段；
- 任务级重试和请求级限速同时生效；
- 429 不通过提高并发或循环重试解决；
- 遇到持续失败时停止并留下可诊断的脱敏错误。

## 8. 测试与 fixture 建议

新增离线测试时不要保存真实页面、Cookie 或认证头。建议最小化为以下 fixture：

1. **初始 shell fixture**：只保留一个短 HTML 文档，其中有脱敏后的 `data-page` JSON，包含 `component`、`version`、`props.article`、`deferredProps.default`；
2. **partial fixture**：只保留同 URL Inertia JSON 的结构，响应中只有 `props.actresses`；
3. **fallback fixture**：没有 Inertia 节点但有 JSON-LD/OpenGraph，验证旧 fallback 仍然可用；
4. **正常页面阻断回归 fixture**：脚本文本包含 `age verification`，但没有实际年龄控件或正文年龄提示，必须判为正常/可解析；
5. **明确年龄门 fixture**：包含实际 `#age-verification-modal` 和明确提示，才判为年龄门；
6. **Cloudflare challenge fixture**：只验证明确 challenge 结构的分类；
7. **错误响应 fixture**：404、401/403、429、5xx、超大 body、非法 JSON。

至少增加以下行为测试：

- 初始 HTML 请求能从 `props.article` 得到标题、日期、时长、封面、标签和未打码状态；
- `article.id` 与 `video_id` 不混淆；
- deferred 第二请求的目标仍是同一文章 URL，响应只补充演员；
- 第二请求失败不使主体文章 Lookup 失败；
- 请求头包含 Inertia 标记但测试日志不包含 Cookie；
- 正常页面脚本中的 `age verification` 不会误报 `age_gate`；
- 限速器和 429/`Retry-After` 处理符合预期。

推荐用 `httptest.Server` 模拟“文档 + partial”两次请求，并断言请求次数、路径、关键头和响应分类。测试内容应使用占位值，不要复制真实用户认证态。

## 9. 安全边界

- 本调查不保存 Cookie、邮箱、IP、session、完整认证头或完整真实 HTML。
- 文章页和同站封面请求如需运行时认证，只能使用已有安全存储解密后的内存值；不得写入日志、错误文本、fixture、命令行参数或本报告。
- 跨站重定向必须拒绝，或在允许的封面重定向场景逐跳清除 Cookie；文章请求不能把会话带到外站。
- 不实现 JS challenge 破解、验证码绕过、IP 轮换、代理池或账号池。
- 页面内容只作为不可信输入处理：限制响应大小和 JSON 深度/类型，解析 URL 时限制允许的站点，避免把任意外部链接当成站内封面请求。
- 评论、联盟链接、互动计数等非刮削必需字段不应为了“完整保存”而扩大敏感数据收集范围。

## 10. 后续实现优先级

1. 在 `fc2cmadb.go` 增加精确的 `data-page` Inertia envelope 解码和 `props.article` 映射。
2. 保留 JSON-LD、OpenGraph 和 HTML fallback，但把 Inertia 文章对象置于首选路径。
3. 按 `deferredProps.default` 对 `actresses` 做同 URL best-effort partial 请求，不把它设计成独立 article API。
4. 收紧年龄门和 Cloudflare 分类，修复脚本字符串导致的误判。
5. 增加离线 shell + partial fixture、两请求及请求头测试、429/限速测试。
6. 在真实验证中只输出状态码、路径、字段形状和分类，不输出 Cookie 或完整响应。

以上结论均来自 2026-09-05 对样本 URL 的真实浏览器/OpenCLI 观察；实现前如站点前端版本变化，应重新进行最小化、脱敏的结构验证。
