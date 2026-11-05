# 演员档案（Actor Profile）

## 背景

演员索引（`演员`）过去只有名字：`video_jav.actors_json` 保存作品演员列表，索引在读取时按名字聚合，并支持「演员合并」。名字之外的演员信息（头像、生日、身高三围）一直没有来源。

数据源其实早就给了。Avmoo 的作品详情接口 `POST /jav/data/api/getMovie` 返回的 `star[]` 是**完整演员对象**，而 `avmoo.go` 只取了名字字段：

```json
{
  "starId": "nxyzzbk",
  "starName": "明里つむぎ",
  "starName_ja": "明里つむぎ",
  "starName_en": "Akari Tsumugi",
  "avatarUrl": "https://jp.netcdn.space/mono/actjpgs/akari_tumugi.jpg",
  "birthday": "1998-03-31",
  "bloodType": "O",
  "hometown": "神奈川県",
  "hobby": "ネットサーフィン",
  "movieCount": 470,
  "lastReleaseDate": "2026-10-30",
  "size": { "T": "155", "B": "80", "C": "B", "W": "58", "H": "83" }
}
```

因此本功能分两段：**A) 刮削时顺带保存（零额外请求）**，**B) 对已有名字做后台补全**。

## 数据源（2026-11 实测）

| 端点 | 用途 | 备注 |
| --- | --- | --- |
| `POST /jav/data/api/getMovie` `["<movieId>","cn"]` | 作品详情，含 `star[]` 完整档案 | 现有刮削已在调用 |
| `POST /jav/data/api/getStar` `["<starId>","cn"]` | 单个演员档案 | 与 `star[]` 元素同构 |
| `POST /jav/data/api/search` `[{"search":"<演员名>","lang":"cn"},20,1]` | 按名字搜作品 | 结果的 `starId` 是 id 串，要拿到档案仍需 `getMovie` |
| `POST /jav/data/api/getStars` `[lang, limit, offset]` | 全量演员列表 | 参数是「条数 / 偏移」，**没有名字过滤**，不能当名字索引 |

约束：

- `javbus` 只给名字，`fc2cmadb` 是 FC2 卖家而非演员，所以 FC2 收藏永远没有演员档案。
- 头像 `jp.netcdn.space` 可直接 GET（无需 Referer），但仍缓存到本地：离线可用，也不受图床防盗链变化影响。
- 所有请求都走 provider 自己的限速器与代理（与封面一致）。

## 数据模型

`schemaV23`：

- `actor_profiles`：以**刮削到的名字**为主键（与 `actors_json`、演员索引同一把钥匙），`star_id` 是数据源的稳定 id，头像文件名由它决定。可选数值列（身高/胸/腰/臀/出演数/最近发行）用 `NULL` 表示数据源没给。
- `actor_fill_tasks`：补全队列，一行一个待解析的名字。

写入规则（`UpsertActorProfiles`）：档案是「增强」而不是「覆盖」——后一次刮削没给的字段保留旧值（空串与 `NULL` 不覆盖非空），只有真正带回来的事实才更新。头像路径在头像下载完成后单独回填（`SetActorAvatarPath`），所以下载失败不会回滚档案。

头像落盘：`<dataDir>/avatars/<starId>.jpg`，写完才原子替换，与 `covers/jav/<id>.jpg` 同一条 `saveJPEG` 路径。

## 读取路径

- `ListActors` 在聚合结果上附加 `profile`：优先规范名，规范名没有档案时用已合并艺名的档案（合并不会丢头像）。
- `ActorProfile(name)` 先按合并关系解析到规范名，再按「规范名 → 各艺名」顺序找档案。
- 档案缺失不是错误：名字可能来自 `javbus`，或还没跑过补全。

## 补全任务（B）

`Service.StartActorFill()`：

1. 取演员索引聚合结果，跳过已有档案的演员（含其艺名），其余名字入队（`EnqueueActorFills`）。
2. 单个 worker 逐个 `ClaimActorFill` → `ActorResolver.ResolveActor(name)`：
   - Avmoo 实现：按名字搜作品（最多取 20 条），逐个 `getMovie`（最多 4 部）读 `star[]`，按「去除空白 + 忽略大小写」匹配任意一个名字变体；命中即返回档案，并把数据源的写法加进 `aliases`。
3. 结果落库：`succeeded` / `not_found`（终态，不再重试）/ `failed`（blocked、parse、预算耗尽）/ `queued`（瞬时错误，带 60s 冷却重试，最多 3 次）。
4. 重启时 `ResetActorFills` 把残留的 `running` 重新排回队列；30s 的重试扫描同时唤醒 worker，让冷却到期的名字继续。

`ActorFillStatus` 汇总队列计数、当前名字与是否仍在运行，演员索引据此显示进度。

## UI

- 演员索引：每个 chip 显示缓存头像（无档案时用名字首字），表头右侧「补全演员信息」按钮（运行中禁用），下方一行进度（已补全 / 未收录 / 失败 / 队列 / 正在查询）。
- 演员作品页：档案头（头像、生日、身高、三围、出演数、出身、数据源别名），缺项不显示空标签。
- 移动端：同一份 `ActorSummary`（已带 `profile`）由 LAN 端点返回，索引显示头像、演员页副标题带档案；只读，不提供补全入口（与演员合并一致，属桌面专属能力）。
- 事件：`jav:actors:updated` 与其它后端事件一样只做一次全量失效，视图不需要知道细节。

## 不做的事

- 不自动合并数据源给出的多语言别名：`演员合并` 仍是用户决定，档案里的 `aliases` 只作为提示与搜索线索。
- 不把演员档案写入 `actors` / `video_actors`（视觉识别域），与 `jav-scrape-design.md` 的边界一致。
- 不为 FC2 推断演员。
- 不做演员页面的剧照集、粉丝数等未在数据源稳定提供的内容。

## 验证

- `backend/jav/actor_profile_test.go`：部分刷新的保留语义、合并后取档案、坏 JSON 容错、未知名字。
- `backend/jav/actor_profile_service_test.go`：刮削落库 + 头像下载、下载失败仍保留档案、未配置头像目录。
- `backend/jav/actor_fill_test.go`：解析成功 / 未收录 / 瞬时失败冷却 / 预算耗尽 / 无 resolver。
- `backend/jav/providers/avmoo_test.go`：`star[]` → 档案映射、按名字解析并改用请求方写法、未命中。
- `backend/server/server_integration_test.go`：LAN 头像路由与非法文件名。
