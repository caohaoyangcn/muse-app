# 相似影片推荐（详情页底部）技术设计

**Status:** 里程碑 1（打分内核）与里程碑 3（两端展示）已实现；里程碑 2（标题 embedding）待做
**Scope:** 影片详情页底部增加「类似影片」推荐栏，综合系列、风格标签、演员、厂牌、发行时间与片名 embedding
**Target:** Windows/macOS Wails Desktop App + 局域网移动端
**Version:** v1

---

## 1. 目标与位置

详情页最底部增加一排可横向滚动的推荐卡，默认 8 张，每张带一条可读的推荐理由：

```text
视频详情
    |
    +-- 封面 / JavBus 元数据
    +-- 演员 / 标签
    +-- 刮削状态
    +-- 番号与刮削模式
    +-- 文件信息
    +-- 类似影片   <-- 新增
          卡片：封面 · 【番号】译名
          理由：同系列 / 同演员 / 相似标题 / 同厂牌 / 同年发行
```

硬约束：

- **推荐是读路径的副产品，不阻塞详情页首屏。** 详情页先用已有数据渲染，推荐栏独立取数、独立 loading。
- **不联网也能出结果。** embedding 未配置、请求失败、条目未嵌入，都只让「标题」这一项退出打分，权重按可用信号重归一化，绝不报错、绝不空栏。
- **纯读。** 不写 `videos` / `video_jav`，唯一例外是 embedding 缓存表。
- **召回集永远排除**：影片自身、已删除/回收站条目、同一作品的其他副本（`jav_duplicate_ignores` 视为已处理）。

## 2. 信号与清洗

| 信号 | 来源 | 归一化规则 | 权重 |
| --- | --- | --- | --- |
| 演员 | `video_jav.actors_json`（经 `actor_aliases` 合并后的规范名） | 去空白、按规范名去重 | 0.28 |
| 风格标签 | `video_jav.tags_json` | 剔除「格式标签」后按 IDF 加权 | 0.24 |
| 系列 | `video_jav.series` + 番号前缀 | 全角转半角、去空格、小写 | 0.18 |
| 片名 embedding | `title_zh`（优先）或 `title` | 清洗后嵌入，见 §4 | 0.16 |
| 厂牌 | `video_jav.studio` | 小写、去空白 | 0.08 |
| 发行时间 | `video_jav.release_unix` | 指数衰减，见 §3.3 | 0.06 |

### 2.1 格式标签不是风格

标签里混着「拍摄/发行格式」，它们不描述内容，若参与相似度会让整个厂牌互相相似：

```text
單體作品 / HDTV / 高清 / 數位馬賽克 / 無碼破解 / LTX破解 / 獨占配信 / 生写真
```

第一版用**显式剔除列表**（`formatTags`）而不是靠 IDF 自动压低：它们在某些子集里 DF 可能并不高（例如「LTX破解」只出现在破码片里），IDF 压不住，反而会制造一个假的强维度。

### 2.2 有无码是硬分区

`is_uncensored` 不同的两部片不放进同一排推荐——破码/无码片与有码片混排会让整排看起来「不对」。「無碼破解」标签**不**参与这个判断：它描述的是本地这份文件被破解过，而不是作品本身有没有码。同侧候选不足 `limit` 时，用另一侧的候选静默补满（结果里不标记：排序与理由不变，只有入围者变了）。

## 3. 打分模型

三段式：召回 → 排序 → 多样性。

### 3.1 召回

召回读取的是**整库特征**，不是一次 SQL 并集：`Corpus` 必须先看到所有条目才能算出 IDF，几千条规模下一次全表读取（约 2–5 ms）比"先查候选再查统计"更简单也更快。真正的召回是评分前的一道闸门：

- 必须有内容信号重叠（演员 / 标签 / 系列 / 厂牌）；仅发行时间相近不算相似；
- 排除自身、同番号（`query.code == candidate.code`）、已删除与回收站条目；
- 有无码分区，同侧候选优先，不足 `limit` 时用另一侧补满；
- 候选规模由 `limit` 决定（默认 8、上限 40），不需要额外的固定截断。

### 3.2 各信号产出 [0,1]

```text
s_actor  = IDF 加权余弦(canonical actors)
s_tag    = IDF 加权余弦(style tags)
s_series = 1.0  同 series
         | 0.6  只有番号前缀相同（SSIS/SONE/IPZ…，视作作品线）
         | 0.0  否则
s_studio = 1.0 同 studio，否则 0
s_title  = clamp(余弦(emb_a, emb_b), 0, 1)
s_time   = exp(-|Δdays| / 1095)
```

`Δdays` 用 1095 天（约 3 年）作时间常数：同年 = 1.0，±3 年 ≈ 0.37，±10 年 ≈ 0.04。时间只提供**轻微的「同年代制作风格」先验**，不允许压过演员或系列——一部 2007 年的片和 2024 年的片只要演员/标签对上，仍然应该排前面。

### 3.3 融合

```go
score = Σ(w_i · s_i)   // 缺失的信号贡献 0，不做分母归一
```

**不做权重重归一化。** 重归一化会把"只有厂牌相同"的影片放大成 1.0——厂牌是查询唯一可用的信号，它填满了整个分母——而这恰恰应该是最弱的匹配。缺失的信号是**丢失的证据**，不是中性的证据。

代价是绝对分数随数据完整度变化：里程碑 1 尚未接入标题通道，满分是 0.84（演员 + 标签 + 系列 + 厂牌 + 时间）；embedding 落地后自动回到 1.0，权重无需重调。分数只用于排序与测试，界面不展示。

### 3.4 多样性与保底

纯分数排序会出现「整排同一个演员」或「整排同一个系列」。排序末尾套一层 MMR：

```go
// λ = 0.7；similarity = max(标签余弦, 演员余弦)，系列重复交给上面的上限处理
next = argmax( λ·score(i) - (1-λ)·max_{j∈selected} sim(i, j) )
```

MMR 只在按分数截出的短名单（`8 × limit + 24` 条）上跑，不对全库做：推荐栏只有 8 个位置，在全库上做多样性是纯浪费。留下余量是为了在上限生效时仍有足够候选可挑。

再加两条上限（产品判断，不是可调参数）：

- 同一演员默认最多 4 张
- 同一系列默认最多 3 张（`series` 为空时退化为番号前缀）

上限是**排序偏好**而不是配额：达到上限的候选会被排到未达上限的候选之后；当整库都没有足够候选时，行尾仍会用它们补满，推荐栏不为凑数留空位。

### 3.5 理由

`Reasons []string` 由触发的信号生成，按贡献（`w·s`）降序，最多 2 条：

```text
同演员 · 同题材 · 同系列 · 同作品线 · 同厂牌 · 同年发行
```

`同题材` 仅在标签余弦 ≥ 0.5 时给出；`同作品线` 指番号前缀相同而 `series` 不同（或缺失）；`同年发行` 按日历年份精确相等。`相似标题` 留到里程碑 2（余弦 ≥ 0.75 时给出）。前端只显示文字，不显示分数。

## 4. 片名 embedding

### 4.1 嵌入什么文本

```text
text = clean(title_zh)  if title_zh != ""   // 优先中文译名：跨语言可比、用户可读
     = clean(title)      otherwise
```

`clean` 在嵌入前必须做，否则 embedding 会编码这些噪声：

| 清洗项 | 例子 | 原因 |
| --- | --- | --- |
| 番号 | `SSIS-762`、`[1KRMV-279]` | 纯标识符 |
| 演员名 | `河北彩伽`、`渚光希` | 演员已是独立维度，重复编码会让标题通道退化成演员通道 |
| 营销样板 | `（ブルーレイディスク）`、`生写真3枚付き`、`独占配信`、`高画質`、`4K`、`N分`、`特典映像` | 同厂牌批量后缀，会把整批片压成一个点 |
| 控制字符 / 重复空白 | — | — |

清洗后不足 4 个字符的标题不嵌入（`text_hash` 记为不可嵌入），该条目永久退出标题通道。

### 4.2 存储（Migration v28）

不复用 `video_jav`，单开一张表：换模型、换维度、换文本时只动这一张。

```sql
CREATE TABLE IF NOT EXISTS video_embeddings (
    video_id    INTEGER PRIMARY KEY,
    model       TEXT NOT NULL,
    dim         INTEGER NOT NULL,
    vector      BLOB NOT NULL,        -- float32 little-endian，已 L2 归一化
    text_hash   TEXT NOT NULL,        -- sha256(clean(text))，失效判断依据
    source      TEXT NOT NULL,        -- 'title_zh' | 'title'
    created_at  INTEGER NOT NULL,
    FOREIGN KEY (video_id) REFERENCES videos(id) ON DELETE CASCADE
);
```

失效规则：`text_hash` 不匹配、`model` 与当前配置不符、`dim` 不符 → 视为缺失，重新排队。写入方向只有一条：成功嵌入后 `UPSERT`；失败不落库（避免把「失败」写成永久状态，与 `title_zh` 的处理一致）。

### 4.3 什么时候算

| 触发 | 行为 |
| --- | --- |
| 刮削成功且译名可用 | 入队（与翻译队列同级，最多 2 个 in-flight） |
| `title_zh` 翻译成功 | 入队重算（`source` 从 `title` 变 `title_zh`） |
| 启动 | 只做**懒计算**：不主动全库补齐，避免每次启动都触发付费请求 |
| 用户点「补齐推荐向量」 | 后台批量跑，UI 显示进度（复用 `actor_fill_tasks` 的进度模型） |
| 详情页取推荐时发现查询影片缺向量 | 单飞（single-flight）嵌入该条，超时 3s，失败即降级 |

### 4.4 配置必须独立于翻译

`llm` 的默认端点是 DeepSeek，而 **DeepSeek 不提供 `/v1/embeddings`**。因此 embedding 需要自己的一组设置，不能直接继承翻译配置：

| key | 默认 |
| --- | --- |
| `embedding_base_url` | 空 → 不启用（**不**回落到 `llm_base_url`） |
| `embedding_model` | `text-embedding-3-small` |
| `embedding_api_key_dpapi` | 空 → 不启用 |

保存时做一次**能力探测**（`POST /v1/embeddings` 一条 1 token 的文本），探测失败直接拒绝保存并说明原因，而不是让用户在一个永远不工作的配置上排一整批任务。

### 4.5 成本

`text-embedding-3-small` 约 $0.02 / 1M token，单条标题 ~30 token：

```text
2945 条 × 30 token ≈ 88k token ≈ $0.0018
```

一次性全库嵌入不到 1 分钱，查询向量按需懒计算。因此「embedding 贵不贵」不构成设计约束，真正的约束是**离线可用性与失败降级**。

### 4.6 内存与检索

启动时把全部向量读进内存（2945 × 1536 × 4B ≈ 18 MB），`dim` 混存时按 `model` 分桶。相似度就是归一化向量的点积：

```text
2945 × 1536 ≈ 450 万次乘加 ≈ 2–4 ms（单核）
```

**不引入 HNSW / 向量库。** 超过 2 万条再考虑，届时先试 int8 量化（18 MB → 4.5 MB，精度损失可忽略），最后才上 ANN。

## 5. 接口

```go
// backend/api/library.go
type SimilarVideo struct {
    Video   video.Video `json:"video"`
    Score   float64     `json:"score"`
    Reasons []string    `json:"reasons"`
}

// SimilarVideos 返回与 videoID 相似的收藏，最多 limit 条（默认 8，上限 40）。
// 影片不存在、未刮削、库内过小时返回空切片而不是错误。
func (l *Library) SimilarVideos(videoID int64, limit int) ([]SimilarVideo, error)
```

桌面与局域网共用这一个门面（`api.Library` 是唯一业务入口）：

| 传输 | 形状 |
| --- | --- |
| Wails | `App.SimilarVideos(videoID, limit)` → 生成的 `frontend/wailsjs` 绑定 |
| HTTP | `GET /api/videos/{id}/similar?limit=N`；`limit` 缺失或非法时用默认条数，未知/未刮削收藏返回 200 + `[]` |

前端：

```text
views/similar/useSimilarVideos.ts   // 共享取数与状态（两端复用）
components/SimilarVideos.tsx        // 桌面：横向卡片排，末尾的独立 section
mobile/components/SimilarVideos.tsx // 移动：同一份数据，小一号卡片
```

- 取数走 `useLiveQuery(fetch, `similar:${videoId}:${limit}`)`：条数进查询键，换条数就是换一个问题。
- 读失败与首屏都归一成空数组：推荐是附属信息，不为它弹错误。
- `invalidate()` 后自然重读（刮削成功、译名更新）；分组变化不触发（分组不参与打分）。
- 卡片点击复用现有导航：桌面端 `openDetail` 开新标签页，移动端 `openVideo` 压栈；不新增路由。

## 6. 性能与阈值

| 项 | 目标 |
| --- | --- |
| 单次 `SimilarVideos` 端到端 | < 30 ms（几千条库）；实测 3000 条时打分内核（建语料 + 排序 + MMR）**9.3 ms**，未含一次全表读取 |
| 向量常驻内存 | < 25 MB（3000 条 × 1536） |
| 详情页首屏 | 完全不受影响（推荐栏独立 loading） |
| 库规模上限 | 2 万条内暴力扫描；超过后单独立项 |

## 7. 校准

权重不是拍脑袋定的，用离线脚本校准（复用本次调研的 `_probe_title_similarity.py` 思路，落成 `scripts/eval-similar.py`）：

1. **代理真值**：以「同系列」与「同演员」为伪标签，计算推荐 top-8 的 hit rate / recall。
2. **对照**：逐项消融（去掉 title、去掉 series、去掉 time），确认每项都有正贡献；没有正贡献的项直接删掉。
3. **人工抽检**：随机 30 部片，人工标记 top-8 里「合理」的比例，目标 ≥ 70%。
4. **权重网格**：在代理真值上粗搜权重，再人工抽检确认没有被指标带偏（指标高但结果单调是常见陷阱）。

第一版权重就先按 §2 的表，等校准脚本跑通再调。

## 8. 测试计划

里程碑 1 已完成，共 16 条测试：

`backend/similar/scoring_test.go`（纯打分，11 条）

- 演员 > 系列 > 厂牌 的排序
- 只有发行时间相近、无内容重叠 → 不推荐
- 格式标签（無碼破解/單體作品/HDTV）不构成相似
- 只有厂牌相同时分数恰为 0.08（不被重归一化放大）
- 排除同番号副本；未知 id → 空
- 有无码分区优先，同侧为空时回退到另一侧
- 同一演员超过 4 张后排到其他候选之后，行仍补满
- MMR：等分时"新题材"排在"重复题材"之前
- 稀有标签比常见标签更重
- `limit` 边界（0、负数、超大）与空库

`backend/jav/similar_test.go`（读模型，2 条）

- 演员别名合并后只算一个人，字段（系列/厂牌/发行日/有无码）如实传递
- 回收站条目不出现在特征集里

`backend/api/similar_test.go`（门面，5 条）

- 排序 + 理由 + `【番号】` 读模型投影
- 排除同一作品的其他副本与已删除条目
- `演员合并` 生效前无推荐、生效后命中「同演员」
- 无刮削记录 / 无 jav 服务 → 返回空切片而不是 nil 或错误
- `limit` 夹取与未知 id

`text_hash` / 重嵌相关测试随里程碑 2 补。

`backend/server/similar_test.go`（HTTP 路由，3 条）

- 路由返回与 Wails 绑定同形的行（含 `javCode` 读模型投影与「同演员」理由）
- 未知收藏 → 200 + `[]`（不是 `null`，也不是错误页）
- `limit` 非法 → 忽略并回落到默认条数

前端（Vitest，`views/similar/useSimilarVideos.test.tsx`，4 条）

- 首屏给空数组而不是 `null`（展示层可以直接跳过渲染）
- 读失败后仍是空数组：详情页不因推荐报错
- 读到的结果原样交给展示层
- 条数进查询键：改条数会重新取值

## 9. 里程碑

三个可独立验证、独立提交的模块：

1. **打分内核（无 embedding）** —— ✅ 已完成：`backend/similar` + `jav.SimilarFeatures` + `Library.SimilarVideos` + 16 条测试，`golangci-lint` 无告警。
2. **标题 embedding**：Migration v28 + 独立设置项 + 能力探测 + 队列 + 内存索引 + 降级路径，把标题通道接进内核。
3. **前端与传输层** —— ✅ 已完成：`App.SimilarVideos` + `GET /api/videos/{id}/similar` + 共享 `useSimilarVideos` + 桌面/移动横排 + 3 条路由测试 + 4 条 Vitest；`npm run lint` / `build` / `test` 与 `wails build` 均通过。

## 10. 未决问题

- **译名覆盖率**：导出数据里 61% 的「中文标题」其实还是日文（2945 条中 1806 条两列相同）。译名没补齐时标题通道会退化到日文原文，跨语言可比性差。是否要在「补齐推荐向量」里顺带触发翻译，需要在实现前定。
- **是否做整体内容 embedding**：把「系列 + 标签 + 标题」拼成一句嵌入，通常比单独嵌标题更强，但可解释性和消融都会变差。留到 v2 评估。
- **时间权重是否要按厂牌区分**：老厂牌作品跨度大，时间先验可能有害；先全局一个常数，校准后再说。
