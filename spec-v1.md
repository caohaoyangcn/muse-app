# Video Favorites — V1 技术设计 Spec

## 1. 背景

Windows 原生文件管理器缺少一个适合“收藏单个本地文件”的轻量机制。

目标用户在日常使用电脑时，经常会遇到：

- 视频文件分散在不同磁盘、不同目录；
- 某些视频值得长期保留，但不希望移动到统一目录；
- 希望维护一个“精选视频清单”；
- 希望从一个 App 中快速查看收藏的视频，并直接打开原文件。

因此实现一个 Windows Desktop App：

> **Video Favorites：一个面向本地视频文件的收藏管理器。**

App 本身不负责存储视频，只保存视频文件的引用和少量元数据。

---

# 2. V1 目标

## 2.1 核心目标

V1 必须完整支持以下流程：

```text
在 Explorer 中选择视频
        ↓
加入收藏
        ↓
Video Favorites 中出现
        ↓
看到视频标题 / 缩略图
        ↓
点击
        ↓
使用系统默认播放器打开原视频
```

以及：

```text
打开 Video Favorites
        ↓
查看全部收藏视频
        ↓
搜索
        ↓
打开视频
```

## 2.2 非目标

V1 明确不实现：

- 视频复制 / 移动
- 视频转码
- 内置播放器
- 自动扫描整个磁盘
- 自动生成分类
- 标签系统
- 云同步
- 多设备同步
- 用户登录
- 视频内容分析
- AI 自动分类
- NAS / SMB 专门支持
- Explorer 右键菜单扩展

这些功能留到后续版本。

---

# 3. 技术选型

## 3.1 总体技术栈

```text
Frontend
  React
  TypeScript
  Vite

Desktop Runtime
  Wails

Backend
  Go

Database
  SQLite

Media Metadata
  FFmpeg / ffprobe

Packaging
  Wails Windows build
```

推荐版本：

```text
Go        1.24+
Node      22+
React     19+
TypeScript
SQLite    3.x
Wails     v2
FFmpeg    8.x 或兼容版本
```

具体版本不要求强绑定，以开发时稳定版本为准。

---

# 4. 系统架构

整体采用三层结构：

```text
┌─────────────────────────────────────┐
│              UI Layer               │
│                                     │
│ React + TypeScript                  │
│                                     │
│ Library / Search / Detail / Dialog  │
└─────────────────┬───────────────────┘
                  │
               Wails RPC
                  │
┌─────────────────▼───────────────────┐
│             Application             │
│                                     │
│ VideoService                        │
│ FavoriteService                     │
│ MetadataService                     │
│ ThumbnailService                    │
└─────────────────┬───────────────────┘
                  │
       ┌──────────┼───────────┐
       ▼          ▼           ▼
    SQLite      FFmpeg     OS APIs
```

设计原则：

> React 负责展示和交互，Go 负责业务逻辑、文件系统以及持久化。

---

# 5. 数据模型

V1 只需要一张主表。

## 5.1 videos

```sql
CREATE TABLE videos (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    path            TEXT NOT NULL UNIQUE,
    file_name       TEXT NOT NULL,
    file_size       INTEGER NOT NULL,
    duration_ms     INTEGER,
    thumbnail_path  TEXT,
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL,
    last_opened_at  INTEGER,
    status          TEXT NOT NULL DEFAULT 'available'
);
```

字段说明：

| 字段           | 含义                |
| -------------- | ------------------- |
| id             | 内部 ID             |
| path           | 视频绝对路径        |
| file_name      | 文件名              |
| file_size      | 文件大小            |
| duration_ms    | 视频时长            |
| thumbnail_path | 缩略图路径          |
| created_at     | 加入收藏时间        |
| updated_at     | 数据更新时间        |
| last_opened_at | 最近一次打开时间    |
| status         | available / missing |

### 为什么保存 `file_name` 而不是每次从 path 获取？

因为：

1. UI 不需要频繁访问文件系统；
2. 文件丢失后仍然可以显示原始名称；
3. 方便后续实现文件迁移检测。

---

# 6. 文件引用模型

核心设计原则：

> **数据库只保存引用，不拥有视频。**

例如用户收藏：

```text
D:\Videos\Movies\Interstellar.mkv
```

数据库只记录：

```text
path =
D:\Videos\Movies\Interstellar.mkv
```

而不会：

```text
copy video
move video
import video
```

因此：

```text
原视频位置：
D:\Videos\Movies\Interstellar.mkv

App：
          └── reference ──→ 原文件
```

删除收藏只删除数据库记录，不删除真实视频。

这是整个系统非常重要的安全原则。

---

# 7. 收藏操作

## 7.1 添加视频

用户在 App 中点击：

```text
Add Video
```

打开 Windows 文件选择器。

允许选择：

```text
.mp4
.mkv
.webm
.mov
.avi
.m4v
```

选择完成之后：

```text
Validate File
      ↓
Check duplicate
      ↓
Read metadata
      ↓
Create thumbnail
      ↓
Insert database
      ↓
Refresh UI
```

---

# 8. 文件校验

添加视频时必须检查：

### 8.1 是否存在

```go
os.Stat(path)
```

不存在：

```text
File does not exist
```

直接拒绝添加。

### 8.2 是否为普通文件

拒绝：

- directory
- symbolic link（V1 可直接拒绝）
- device file

### 8.3 是否重复

通过：

```sql
UNIQUE(path)
```

保证同一个绝对路径不会出现多次。

如果重复：

```text
This video is already in Favorites.
```

---

# 9. 视频元数据

添加视频时调用：

```text
ffprobe
```

获取：

```text
duration
width
height
codec
```

V1 实际只需要：

```text
duration
width
height
```

例如：

```json
{
  "duration": 7423.42,
  "width": 3840,
  "height": 2160
}
```

转换为：

```text
duration_ms = 7423420
```

---

# 10. 缩略图

视频收藏库使用 Grid UI，因此缩略图是 V1 的核心体验。

流程：

```text
video.mp4
   ↓
ffmpeg
   ↓
frame extraction
   ↓
thumbnail.jpg
```

默认：

> 取视频 10% 位置的画面作为缩略图。

原因：

- 起始位置经常是黑屏；
- 10% 通常比第一帧更有内容；
- 算法简单。

---

# 11. Thumbnail Storage

缩略图不要直接存 SQLite BLOB。

统一放在 App Data：

```text
%LOCALAPPDATA%\VideoFavorites\
```

目录：

```text
VideoFavorites/
├── app.db
└── thumbnails/
    ├── 1.jpg
    ├── 2.jpg
    ├── 3.jpg
    └── ...
```

数据库只记录：

```text
thumbnail_path
```

例如：

```text
thumbnails/123.jpg
```

---

# 12. Thumbnail 生成策略

缩略图属于异步任务。

添加视频时：

```text
Create database record
        ↓
status = processing
        ↓
UI immediately displays item
        ↓
background thumbnail generation
        ↓
thumbnail generated
        ↓
update database
        ↓
frontend refresh
```

这样不会因为视频比较大而阻塞 UI。

---

# 13. 删除收藏

UI：

```text
Right Click
    ↓
Remove from Favorites
```

执行：

```text
DELETE FROM videos WHERE id = ?
```

同时删除：

```text
thumbnail
```

**绝对禁止删除原视频。**

移除只看数据库记录，不看磁盘：原文件不存在、甚至这条记录已经被删掉了，移除都应当成功
（记录不存在 = 目标状态已达成）。界面永远不会因此卡在一条删不掉的卡片上——列表过期时，
后端返回成功、前端照常刷新并对齐真实数据。

生成的缩略图与刮削封面（`<dataDir>/thumbnails/<id>.jpg`、`<dataDir>/covers/jav/<id>.jpg`）不靠
"删记录时顺手删文件"保证不残留，而是定期与 `videos` 表对账：启动后台一次、每次移除后台一次。
详见 `docs/spec/generated-images.md`。

---

# 14. 打开视频

用户点击卡片：

```text
OpenVideo(id)
```

Go：

```go
cmd := exec.Command("cmd", "/c", "start", "", path)
```

或者使用 Windows Shell API。

推荐最终封装成：

```go
OpenWithDefaultApp(path string) error
```

实现：

```text
VideoFavorites
      ↓
Windows Shell
      ↓
系统默认播放器
```

例如用户默认播放器是：

```text
PotPlayer
```

则直接使用 PotPlayer。

---

# 15. “打开所在文件夹”

视频右键菜单：

```text
Open
Open Containing Folder
Copy Path
Remove Favorite
```

其中：

```text
Open Containing Folder
```

执行 Explorer：

```text
explorer.exe /select,"D:\Videos\movie.mp4"
```

用户可以直接看到文件在 Explorer 中的位置。

---

# 16. UI 设计

主界面：

```text
┌───────────────────────────────────────────────────┐
│ Video Favorites                         Search 🔍 │
├───────────────┬───────────────────────────────────┤
│               │                                   │
│ ⭐ Favorites  │    128 Videos                     │
│               │                                   │
│               │   ┌─────────┐ ┌─────────┐        │
│               │   │         │ │         │        │
│               │   │ thumb   │ │ thumb   │        │
│               │   │         │ │         │        │
│               │   ├─────────┤ ├─────────┤        │
│               │   │ Video A │ │ Video B │        │
│               │   │ 12:32   │ │ 01:03   │        │
│               │   └─────────┘ └─────────┘        │
│               │                                   │
└───────────────┴───────────────────────────────────┘
```

---

# 17. 首页功能

V1 首页只有一个 Library：

```text
All Favorites
```

排序：

```text
Recently Added
```

默认：

```sql
ORDER BY created_at DESC
```

暂时不做：

```text
Tags
Folders
Categories
Smart Collections
```

---

# 18. Search

搜索框支持：

```text
Video title
File name
File path
```

例如：

```text
搜索：
interstellar
```

SQL：

```sql
SELECT *
FROM videos
WHERE file_name LIKE '%' || ? || '%'
   OR path LIKE '%' || ? || '%'
ORDER BY created_at DESC;
```

V1 不需要 Elasticsearch / FTS5。

数据量预计只有：

```text
100 ~ 10,000 videos
```

SQLite 普通查询完全够用。

---

# 19. 状态管理

视频可能被用户移动或者删除。

例如数据库：

```text
D:\Videos\movie.mp4
```

真实文件已经不存在。

此时 UI：

```text
┌─────────────────┐
│ thumbnail       │
│                 │
├─────────────────┤
│ movie.mp4       │
│ ⚠ File Missing  │
└─────────────────┘
```

状态：

```text
available
missing
```

---

# 20. 文件状态检查

V1 不实现实时 FileSystem Watcher。

而是在以下时机检查：

### App 启动

```text
Load favorites
     ↓
Check file existence
```

### 打开文件之前

```text
if !exists(path)
    status = missing
```

这样实现简单，并避免 Windows 大量文件监听带来的复杂性。

---

# 21. Missing File 行为

如果文件不存在：

```text
Open
```

显示：

```text
The file could not be found.

D:\Videos\movie.mp4
```

提供：

```text
Remove Favorite
```

V1 暂不实现：

```text
Relocate
Find New Location
```

---

# 22. Backend API

Wails 暴露以下 API：

```go
type VideoService struct {}
```

### List

```go
ListVideos() ([]Video, error)
```

### Search

```go
SearchVideos(keyword string) ([]Video, error)
```

### Add

```go
AddVideo(path string) (*Video, error)
```

### Remove

```go
RemoveVideo(id int64) error
```

### Open

```go
OpenVideo(id int64) error
```

### Open Folder

```go
OpenContainingFolder(id int64) error
```

### Copy Path

```go
CopyPath(id int64) error
```

### Refresh Status

```go
RefreshStatus(id int64) error
```

---

# 23. Frontend API

React 不直接操作 SQLite 或 Windows API。

统一：

```text
React
  ↓
Wails Bindings
  ↓
Go Service
```

例如：

```typescript
const videos = await ListVideos();

await AddVideo(path);

await OpenVideo(id);
```

这样 UI 与平台能力完全解耦。

---

# 24. 推荐目录结构

```text
video-favorites/
│
├── frontend/
│   ├── src/
│   │   ├── components/
│   │   │   ├── VideoCard.tsx
│   │   │   ├── VideoGrid.tsx
│   │   │   ├── SearchBar.tsx
│   │   │   └── ContextMenu.tsx
│   │   │
│   │   ├── pages/
│   │   │   └── Library.tsx
│   │   │
│   │   ├── hooks/
│   │   ├── services/
│   │   ├── types/
│   │   └── App.tsx
│   │
│   └── package.json
│
├── backend/
│   ├── video/
│   │   ├── service.go
│   │   ├── repository.go
│   │   ├── model.go
│   │   └── metadata.go
│   │
│   ├── thumbnail/
│   │   └── service.go
│   │
│   ├── database/
│   │   ├── db.go
│   │   └── migration.go
│   │
│   └── platform/
│       └── windows.go
│
├── build/
├── app.go
├── wails.json
└── README.md
```

---

# 25. Repository 层

不要让 Service 直接写 SQL。

采用：

```text
Service
   ↓
Repository
   ↓
SQLite
```

例如：

```go
type VideoRepository interface {
    Create(video *Video) error
    Get(id int64) (*Video, error)
    List() ([]Video, error)
    Search(keyword string) ([]Video, error)
    Delete(id int64) error
    UpdateStatus(id int64, status string) error
}
```

这样未来切换数据库或者增加缓存时，不需要修改业务层。

---

# 26. Service 层

核心业务逻辑：

```go
type VideoService struct {
    repo      VideoRepository
    metadata  MetadataService
    thumbnail ThumbnailService
}
```

例如：

```go
func (s *VideoService) AddVideo(path string) (*Video, error)
```

内部：

```text
ValidatePath
    ↓
NormalizePath
    ↓
CheckDuplicate
    ↓
ExtractMetadata
    ↓
CreateDBRecord
    ↓
GenerateThumbnailAsync
    ↓
Return
```

---

# 27. Path Normalization

这是 Windows 应用里需要特别注意的问题。

例如：

```text
D:\Videos\movie.mp4
D:/Videos/movie.mp4
```

逻辑上可能是同一个文件。

添加视频时统一：

```text
Clean
Absolute
Normalize
```

最终数据库保存 canonical path。

V1 可以使用 Go：

```go
filepath.Abs()
filepath.Clean()
```

---

# 28. 并发模型

Thumbnail 生成不能阻塞主线程。

推荐：

```text
Worker Pool

Queue
  ↓
2 workers
  ↓
ffmpeg
```

V1 可以先使用：

```go
goroutine
```

实现简单版本。

例如：

```go
go thumbnail.Generate(video)
```

后续收藏量增加之后再引入 Job Queue。

---

# 29. 错误处理

错误分为三类。

### User Error

例如：

```text
File does not exist
Unsupported file type
Already added
```

直接返回可读错误。

### System Error

例如：

```text
SQLite unavailable
Permission denied
ffprobe failed
```

记录日志并向 UI 返回错误。

### Non-critical Error

例如：

```text
Thumbnail generation failed
```

不应该导致收藏失败。

即：

```text
Video Favorite = success
Thumbnail = optional
```

即使缩略图失败，视频仍然存在于收藏列表。

---

# 30. Logging

使用 Go 标准：

```go
log/slog
```

日志目录：

```text
%LOCALAPPDATA%\VideoFavorites\logs\
```

例如：

```text
2026-08-29T18:30:01Z INFO add video path=...
2026-08-29T18:30:02Z INFO thumbnail generated id=12
2026-08-29T18:30:05Z ERROR ffprobe failed ...
```

---

# 31. 配置

V1 不需要复杂配置系统。

固定：

```text
Database:
%LOCALAPPDATA%\VideoFavorites\app.db

Thumbnail:
%LOCALAPPDATA%\VideoFavorites\thumbnails

Logs:
%LOCALAPPDATA%\VideoFavorites\logs
```

未来再增加：

```text
settings.json
```

---

# 32. 性能目标

对于：

```text
1,000 videos
```

应该满足：

### 启动

```text
< 1 second
```

不包括首次启动初始化。

### Library Loading

```text
< 200 ms
```

### Search

```text
< 100 ms
```

### Add Video

数据库写入不应等待 thumbnail 完成。

目标：

```text
< 300 ms
```

然后异步生成缩略图。

---

# 33. 安全设计

App 对用户视频只做：

```text
read
```

不会：

```text
delete
move
overwrite
rename
```

除非未来显式加入文件管理功能。

这个约束非常重要。

尤其是：

```text
Remove Favorite
```

必须保证只删除：

```text
数据库记录
缩略图
```

而不会删除：

```text
original video
```

---

# 34. UX 原则

V1 UI 遵循：

> **少操作、低打扰。**

核心操作控制在：

```text
Add
Search
Open
Remove
```

四个动作。

不要在 V1 引入过多设置。

---

# 35. MVP User Journey

完整用户流程：

```text
启动 App
   ↓
Library 空状态
   ↓
点击 Add Video
   ↓
选择 movie.mp4
   ↓
App 验证文件
   ↓
提取 metadata
   ↓
写 SQLite
   ↓
UI 出现 VideoCard
   ↓
后台生成 thumbnail
   ↓
缩略图出现
   ↓
用户点击
   ↓
系统默认播放器打开
```

---

# 36. 空状态

没有收藏时：

```text
┌───────────────────────────────┐
│                               │
│        No favorites yet       │
│                               │
│   Add your favorite videos    │
│                               │
│        [ + Add Video ]        │
│                               │
└───────────────────────────────┘
```

---

# 37. Video Card

每个卡片包含：

```text
thumbnail
file name
duration
status
```

例如：

```text
┌─────────────────────┐
│                     │
│      thumbnail      │
│                     │
├─────────────────────┤
│ Interstellar.mkv    │
│ 02:49:16            │
└─────────────────────┘
```

右键：

```text
Open
Open Containing Folder
Copy Path
Remove from Favorites
```

---

# 38. V1 Definition of Done

以下全部完成后，V1 才算完成：

### Core

- [ ] 可以添加视频
- [ ] 可以删除收藏
- [ ] 可以查看全部收藏
- [ ] 可以搜索视频
- [ ] 可以打开视频
- [ ] 可以打开所在文件夹
- [ ] 可以复制文件路径

### Metadata

- [ ] 获取 duration
- [ ] 获取 resolution
- [ ] 生成 thumbnail

### File State

- [ ] 文件存在状态检测
- [ ] Missing 状态展示
- [ ] 删除文件不会导致 App 崩溃

### Persistence

- [ ] SQLite 持久化
- [ ] App 重启后收藏仍存在
- [ ] thumbnail 不因重启丢失

### UX

- [ ] 空状态
- [ ] Loading 状态
- [ ] Error message
- [ ] Context menu
- [ ] Search

### Packaging

- [ ] 可以生成 Windows `.exe`
- [ ] 在没有开发环境的机器上能够运行
- [ ] App data 使用 `%LOCALAPPDATA%`

---

# 39. V1 不应该做什么

为了控制项目规模，第一版明确拒绝这些需求：

```text
❌ 自动扫描所有磁盘
❌ 标签
❌ 分类
❌ 播放历史
❌ 播放进度同步
❌ 内置播放器
❌ 视频编辑
❌ NAS
❌ 云同步
❌ 登录
❌ 用户系统
❌ AI 视频理解
❌ Explorer Shell Extension
❌ 文件自动迁移
```

尤其是：

> **不要一开始就做“视频管理器”。**

V1 的定位应该一直是：

> **Favorite Manager for Local Videos**

---

# 40. 后续版本路线

## V1.1

```text
Recently Played
Sort
Grid/List View
Keyboard shortcuts
```

## V1.2

```text
Tags
Collections
Multiple categories
```

## V1.3

```text
Folder Watch
Automatic import
```

## V1.4

```text
Explorer right-click
"Add to Favorites"
```

## V2

```text
File relocation detection
Hash based identity
Smart collections
Advanced search
```

## V3

```text
NAS
Cloud sync
Multi-device
```

---

# 41. 一个关键架构决策

V1 最重要的设计不是 Wails、React 或 SQLite。

而是：

> **Favorite ≠ File**

Favorite 是一个独立实体：

```text
Favorite
   │
   └──── references ────> File
```

因此未来可以自然扩展成：

```text
Favorite
├── file reference
├── title
├── tags
├── notes
├── rating
├── collections
└── metadata
```

而原始文件仍然属于：

```text
Windows File System
```

这会让整个项目以后非常容易从：

```text
“收藏夹”
```

演变成：

```text
“个人本地媒体索引”
```

而不需要推翻第一版架构。

---

# 42. 第一阶段开发顺序

建议实际编码严格按这个顺序：

```text
1. Wails 项目初始化
        ↓
2. SQLite + migration
        ↓
3. Video model / repository
        ↓
4. Add Video
        ↓
5. List Video
        ↓
6. React Grid
        ↓
7. Open Video
        ↓
8. Remove Video
        ↓
9. Search
        ↓
10. ffprobe metadata
        ↓
11. thumbnail
        ↓
12. missing detection
        ↓
13. packaging
```

**不要先做 UI 动画，也不要先做复杂的状态管理。**

先把这条链打通：

```text
Add File
   ↓
SQLite
   ↓
List
   ↓
Open
```

这就是整个产品的最小闭环。
