# Muse JAV 元数据刮削技术设计

**Status:** Draft
**Scope:** 从视频文件名提取番号，后台抓取 JAV 作品元数据，关联视频，下载封面，并提供 ffmpeg 封面兜底
**Target:** Windows/macOS Wails Desktop App
**Version:** v1

---

## 1. 目标

导入视频后，Muse 应完成以下流程：

```text
视频导入成功（不等待网络）
        |
        +--> ffprobe / ffmpeg：提取本地媒体信息和自动帧封面
        |
        +--> JAV 后台任务：提取番号、查询 Provider、保存元数据
                              |
                              +--> 下载 JAV 封面
                              +--> 关联视频
                              +--> 通知前端刷新
```

用户在刮削完成前始终能看到 ffmpeg 自动帧。刮削成功后，JAV 封面默认成为当前封面；用户可以按影片关闭该替换行为，使手动封面继续保留。

本功能是视频收藏的可选 enrichment：网络失败、番号识别失败、Provider 被限制，都不能阻塞视频导入、缩略图生成、播放或人脸识别。

## 2. 与现有 Muse 的关系

Muse 当前的导入入口已经统一：

- 手工选择和拖拽经过 `api.Library.AddVideo/AddVideos`；
- 目录监控 `backend/watch.Service` 也调用同一个 `Library.AddVideos`；
- `backend/video.Processor` 负责 ffprobe 和 ffmpeg 缩略图；
- `backend/actor.Worker` 负责 ffmpeg 抽帧和人脸识别。

因此 JAV 服务应由 `api.Library` 编排，但使用自己的队列和 worker，不应加入 `video.Processor` 或 `actor.Worker`。

现有 `backend/actor` 的演员是视觉识别/人工确认领域，JAV Provider 返回的作品演员是外部元数据。两者必须使用不同的表和 API，不得写入 `actors` 或 `video_actors`。

## 3. 整体架构

```text
Library.AddVideos
    |
    +-- video.Service.AddVideo(s)
    |       +-- INSERT videos
    |       +-- Processor.Enqueue(videoID)
    |       +-- ffprobe + auto_frame
    |
    +-- JavService.Enqueue(videoID)
            +-- 读取视频文件名
            +-- 检查手动覆盖/跳过状态
            +-- ExtractCodes(fileName)
            +-- 查询 video_jav 中已有成功结果
            +-- Provider.Lookup(code)
            +-- 保存元数据和封面状态
            +-- 建立 video_jav 关联
            +-- Emit("jav:scrape:updated")
```

任务入队必须是非阻塞的。队列满时可以丢弃任务，由启动补偿或用户手动重试恢复；不得让导入调用等待远程请求。

## 4. 数据模型

### 4.1 首期只增加 `video_jav` 业务表

首期不引入 `jav_items`。当前需求是“每个视频显示其刮削结果”，一张表可以减少迁移、Repository 和详情接口的复杂度。现有 `videos` 仍会增加封面来源字段，封面当前状态属于视频本身，不属于外部作品。

`jav_items` 只有在需要作品级复用时再增加，例如同一番号对应多个文件、按作品统一刷新封面、作品级筛选或独立作品详情页。

### 4.2 Migration v13

Muse 当前数据库 schema 为 v15。新增 v16 migration，不修改既有 migration；v16 为 `videos` 增加按影片保存的 `allow_scraped_cover`。

```sql
CREATE TABLE video_jav (
    video_id          INTEGER PRIMARY KEY,
    code              TEXT NOT NULL DEFAULT '',
    title             TEXT NOT NULL DEFAULT '',
    studio            TEXT NOT NULL DEFAULT '',
    series            TEXT NOT NULL DEFAULT '',
    release_unix      INTEGER,
    duration_min      INTEGER,
    is_uncensored     INTEGER,
    actors_json       TEXT NOT NULL DEFAULT '[]',
    tags_json         TEXT NOT NULL DEFAULT '[]',
    cover_url         TEXT NOT NULL DEFAULT '',
    cover_status      TEXT NOT NULL DEFAULT 'none',
    provider          TEXT NOT NULL DEFAULT '',
    code_source       TEXT NOT NULL DEFAULT 'filename',
    override_mode     TEXT NOT NULL DEFAULT 'auto',
    scrape_status     TEXT NOT NULL DEFAULT 'queued',
    attempts          INTEGER NOT NULL DEFAULT 0,
    last_error        TEXT NOT NULL DEFAULT '',
    next_retry_at     INTEGER,
    fetched_at        INTEGER,
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL,
    FOREIGN KEY (video_id) REFERENCES videos(id) ON DELETE CASCADE
);

CREATE INDEX idx_video_jav_code ON video_jav(code);
CREATE INDEX idx_video_jav_scrape_status ON video_jav(scrape_status);
```

`video_jav` 是视频到作品元数据的一对零或一关系：同一个番号可以出现在多条视频记录中，但首期允许元数据重复。

### 4.3 状态值

`override_mode`：

```text
auto       从文件名自动提取
manual     使用用户指定的 code
skip       不对该视频执行自动刮削
```

`scrape_status`：

```text
queued       已入队
running      正在抓取
succeeded    元数据抓取成功
no_code      没有识别到番号
not_found    Provider 未找到
retryable    timeout、429、5xx 等可重试错误
blocked      403、验证码、JS challenge 或页面无法解析
disabled     用户跳过
```

`cover_status` 独立于 `scrape_status`：

```text
none
pending
succeeded
failed
```

元数据成功但封面下载失败时，不能把整条任务标为失败；应保留元数据并继续使用 ffmpeg 兜底封面。

## 5. 番号提取

移植 JavBoss 的 `internal/util/jav_util.go` 里的纯函数逻辑到 `backend/jav/extractor.go`。

处理规则：

1. 取 `filepath.Base(path)`；
2. 去掉扩展名；
3. 按候选顺序匹配 `PREFIX-NNN`、`PREFIX_NNN`、`PREFIXNNN`；
4. 支持 2～6 位字母前缀和 2～5 位数字；4 位及以上数字段去除前导 `0`，3 位以内保留原样；文件名中紧贴数字段的字母按分段标记处理，不并入番号；
5. 统一大写并标准化为 `PREFIX-N`，或对 3 位以内数字保留其原始位数；
6. 去重后依次查询候选；
7. 没有候选时记录 `no_code`，不执行网络重试。

首期不复制无码混合前缀、纯数字组合等复杂规则，避免把分辨率、编码参数等普通文件名片段误判为番号。后续可以逐步加入 JavBoss 的 `ExtractUncensoredCodesFromName` 规则，并补充表驱动测试。

JavBoss 中“时长小于 900 秒就跳过”的规则不移植到 Muse。Muse 允许管理短视频，不能因时长启发式直接放弃刮削。

## 6. Provider

### 6.1 抽象

```go
type Provider interface {
    Name() string
    Lookup(ctx context.Context, code string) (*Metadata, error)
}
```

`Metadata` 至少包含：

```text
Title
Code
Studio
Series
ReleaseUnix
DurationMin
Actors
Tags
CoverURL
IsUncensored
Provider
```

Provider 只负责 HTTP、解析和 Provider 特有的会话逻辑；任务状态、重试、数据库和封面优先级由 `JavService` 负责。

### 6.2 首期 Provider

首期只接 JavBus，完成最小闭环后再增加 fallback。当前 Avmoo fallback 的具体实施计划见 [`docs/spec/avmoo-fallback-implementation-plan.md`](avmoo-fallback-implementation-plan.md)：

```text
GANA-*   JavMenu -> JavBus
STARS-*  JavBus -> Avmoo
AP-*     Avmoo
无码候选  AVSOX
普通番号 JavBus
```

这些路由规则来自 JavBoss 的 `internal/service/video_location_jav_linker.go`，不应散落在前端或视频服务中。

### 6.3 反爬边界

JavBoss 当前实现的是基础防护，Muse 首期复用这些原则：

- 每个 Provider 独立限速，JavBus 全局每分钟最多 5 次请求；详情页和封面下载共用额度；
- 使用浏览器风格 `User-Agent`、`Referer` 和 `Accept`；
- 需要时发送年龄 Cookie；
- timeout、429、5xx 在第 1、2、3 次失败后分别等待 20 秒、40 秒、60 秒重试，最多重试 3 次；
- 使用 `http.ProxyFromEnvironment`；
- 允许在 `settings` 中增加手动 HTTP proxy；
- 成功结果可缓存 90 天，未找到结果可缓存 7 天；
- 已有成功的同番号记录时直接复用，不访问网络。

不实现浏览器 JS challenge 破解、验证码处理、IP 轮换、代理池或账号池。遇到这类情况标记 `blocked`，提供手动番号、切换 Provider 和重试入口。

## 7. 后台刮削服务

建议新增：

```text
backend/jav/model.go
backend/jav/extractor.go
backend/jav/provider.go
backend/jav/providers/javbus.go
backend/jav/repository.go
backend/jav/service.go
backend/jav/service_test.go
```

`JavService` 使用独立的有界队列和 1～2 个 worker。每个视频同一时间只能有一个任务，重复入队直接丢弃。

核心算法：

```text
读取视频
  -> override_mode=skip        => disabled
  -> 已有 succeeded 结果且非 force => 直接返回
  -> manual code                => 使用指定番号
  -> auto                       => 从 FileName 提取候选
  -> video_jav 中已有成功 code   => 复制/关联本地结果
  -> Provider 依次查询
  -> 事务保存元数据、状态和 cover_url
  -> 下载封面（独立状态）
  -> 如影片允许刮削封面替换，则切换当前封面；否则只保存为备用封面
```

强制刮削时，如果番号变化，应先清除旧的作品字段，避免详情页短暂展示旧作品。

## 8. 封面设计

### 8.1 优先级

```text
custom_frame（显式禁止刮削替换时） > scraped > auto_frame
```

现有 `videos.thumbnail_path` 表示当前实际展示的封面，继续保留。建议给 `videos` 增加：

```sql
ALTER TABLE videos ADD COLUMN cover_source TEXT NOT NULL DEFAULT 'auto_frame';
ALTER TABLE videos ADD COLUMN scraped_cover_path TEXT;
ALTER TABLE videos ADD COLUMN allow_scraped_cover INTEGER NOT NULL DEFAULT 1;
```

`video_jav.cover_url` 保存远程来源，`videos.scraped_cover_path` 保存本地下载路径。封面来源和是否允许刮削替换都属于视频级状态，因为用户可以只为某个文件选择特定帧或关闭替换。

### 8.2 生命周期

```text
导入                  auto_frame
刮削中                auto_frame
元数据和封面成功      scraped（影片允许替换时）
用户选择视频帧        custom_frame
用户重置自定义封面    scraped（允许替换且有刮削封面时），否则 auto_frame
```

现有 `backend/video/service.go` 的 `SetCover` 和 `ResetCover` 需要扩展：

- `SetCover` 设置 `cover_source=custom_frame`，继续设置 `custom_cover_at`；
- `ResetCover` 有刮削封面时恢复 `scraped`，否则重新入队 ffmpeg；
- 默认允许刮削封面替换当前封面；影片级偏好关闭后，用户自定义封面继续保持当前封面，刮削结果只保存为备用封面。

推荐文件布局：

```text
data/
├── thumbnails/<videoID>.jpg       # ffmpeg 自动帧
├── thumbnails/custom-<videoID>.jpg # 用户帧（可选）
└── covers/jav/<videoID>.jpg       # JAV 下载封面
```

下载必须先写临时文件，成功后原子 rename；文件类型、大小和图片解码应经过校验。`thumbnailHandler` 需要增加受限的 `/covers/jav/` 静态读取路径，禁止任意路径访问。

### 8.3 并发竞态

ffmpeg 和网络刮削并行时，必须使用条件更新，避免旧的 ffmpeg 任务覆盖新封面：

```sql
UPDATE videos
SET thumbnail_path = ?, cover_source = 'auto_frame', updated_at = ?
WHERE id = ?
  AND cover_source = 'auto_frame'
  AND custom_cover_at IS NULL;
```

刮削安装封面也必须检查 `allow_scraped_cover = 1`。不要只依赖任务开始时读取的旧 Video 对象。

## 9. API 和事件

### 9.1 `api.Library`

新增：

```go
VideoJav(videoID int64) (*jav.Detail, error)
ScrapeVideoJav(videoID int64, force bool) error
SetVideoJavOverride(videoID int64, mode, code string) error
```

`Library.AddVideo/AddVideos` 在视频成功入库后调用 `JavService.Enqueue`。目录监控不需要新增特殊逻辑，因为它已经调用同一个 façade。

### 9.2 Wails

根目录 `app.go` 只做薄包装，Wails 重新生成 `frontend/wailsjs`。生成文件禁止手改。

`frontend/src/services/api.ts` 增加对应封装，`frontend/src/types.ts` 转发生成模型。

### 9.3 事件

```text
jav:scrape:updated
{
  "videoId": 123,
  "code": "IPX-633",
  "scrapeStatus": "succeeded",
  "coverStatus": "succeeded",
  "coverSource": "scraped"
}
```

`frontend/src/pages/Library.tsx` 像现有 `thumbnail:generated`、`actor:changed` 一样监听事件。列表只需刷新受影响视频；详情窗口刷新当前视频即可。

局域网 API 如需同步支持：

```text
GET  /api/videos/{id}/jav
POST /api/videos/{id}/jav/scrape
PUT  /api/videos/{id}/jav/override
```

这些端点仍然必须通过 `api.Library`。

## 10. 前端体验

Muse 当前双击视频直接打开播放器，没有完整详情页。因此迁移不能只做后端关联。

首期建议：

- `VideoCard` 显示小型番号和刮削状态；
- 扩展 `MetadataPopup` 显示标题、片商、系列、作品演员、标签和封面来源；
- 右键菜单增加“查看详情”；
- 新增轻量 `VideoDetailsDialog`，提供重新刮削、手动指定番号、跳过刮削和选择视频帧；
- 明确区分“视觉识别演员”和“作品演员”。

现有 `videos.title` 是用户自定义标题，不应被 JAV 标题覆盖。显示优先级为：

```text
用户自定义标题 > 刮削得到的权威番号 > 文件名
```

`番号` 来自 `video_jav.code`：一次刮削成功后保存的作品身份就是它，文件名的杂乱写法不应替代它。`api.Library` 在所有 Video 读取路径上把该番号填充为只读字段 `video.Video.JavCode`（不是 `videos` 表的列），桌面端与局域网端因此看到同一套命名规则；没有刮削结果的收藏仍回退到文件名。JAV 标题只作为外部作品元数据展示，不参与命名。

## 11. 删除、重命名和幂等

- 删除视频时由外键级联删除 `video_jav`；同时清理本地 JAV 封面文件；原始视频不删除；
- 重复导入同一路径不创建新任务；
- 已有 `succeeded` 结果时自动模式跳过网络；
- 文件名在库外被改名时，不自动修改已有番号；用户可手动重新刮削；
- 强制刮削允许更换番号，并清除旧关联；
- Provider 的失败状态带 `next_retry_at`，避免启动后无限重试。

## 12. 分阶段实施

### Phase 1：最小闭环

- schema v13 和 `video_jav`；
- 普通番号提取；
- JavBus Provider；
- 独立队列和状态机；
- 自动关联和详情展示；
- ffmpeg 自动帧兜底；
- JAV 封面下载和封面优先级；
- 手动番号、跳过、强制重试、选择视频帧。

### Phase 2：稳定性和覆盖率

- 成功/未找到缓存；
- Provider 限速、退避和手动 proxy；
- JavMenu、Avmoo、AVSOX fallback；
- `GANA-*`、`STARS-*`、`AP-*` 和无码规则；
- 启动补偿未完成任务；
- 单独重试封面。

### Phase 3：作品级能力

- 拆出 `jav_items`，去重同番号元数据；
- `jav_idols`、`jav_tags`、`jav_studios`、`jav_series`；
- 按作品演员、片商、系列筛选；
- 作品级封面和样品图缓存；
- 外部作品演员与 Muse 视觉演员的人工映射。

## 13. 测试和验收

后端：

- `extractor_test.go`：`IPX-633`、`ipx633`、后缀、边界和误识别样例；
- Provider HTML fixture：成功、未找到、解析失败、429/5xx；
- Service 测试：入队幂等、已有结果复用、手动覆盖、跳过、重试状态；
- 封面竞态测试：ffmpeg 不能覆盖 scraped/custom；
- migration 测试：v12 升级到 v13 后表、索引、外键正确；
- API façade 测试：手工导入和目录监控都能触发同一条刮削路径。

验证命令：

```bash
go test ./...
cd frontend && npm run build
```

验收标准：

1. 添加视频 API 不等待远程 Provider；
2. 刮削成功前显示 ffmpeg 自动帧；
3. 刮削成功后自动切换到 JAV 封面；
4. 用户指定帧不会被 ffmpeg 或刮削覆盖；
5. 元数据成功但封面失败时仍保留元数据和兜底封面；
6. 重启应用后关联和封面来源仍然存在；
7. Provider 不可用时视频仍能正常导入和播放；
8. 详情页能区分视觉识别演员和作品演员。

## 14. 非目标和风险

- 不实现验证码破解、JS challenge 绕过、IP 轮换或代理池；
- 不把 JAV 演员自动写入现有 `actors`；
- 不用 JAV 标题覆盖用户自定义标题；
- 不在首期实现完整 JAV 作品管理后台；
- Provider HTML 结构和反爬策略可能变化，Provider 必须保持可替换；
- 文件名提取是启发式逻辑，必须保留人工番号覆盖入口；
- 封面下载涉及外部站点访问，应遵守目标站点条款并提供关闭自动刮削的设置。
