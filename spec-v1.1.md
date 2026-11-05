# V1.1 添加视频：Drag & Drop

## 1. 功能目标

允许用户直接从 Windows File Explorer 将一个或多个视频文件拖拽到 Video Favorites 窗口中，松开鼠标后自动添加到收藏。

典型操作：

```text
Windows Explorer
      │
      │ drag video.mp4
      ▼
┌─────────────────────────┐
│                         │
│    Drop video here      │
│                         │
└─────────────────────────┘
      │
      ▼
   Add to Favorites
```

该功能与 File Picker、未来的 Explorer 右键菜单共同作为视频添加入口。

---

## 2. 支持的拖拽来源

V1 主要支持：

```text
Windows File Explorer
```

拖拽对象：

* 单个视频文件
* 多个视频文件

例如：

```text
Explorer
├── video1.mp4
├── video2.mkv
└── video3.webm
```

用户可以一次选中三个文件并拖入 App。

---

## 3. 不支持

V1 暂不支持：

* 拖入文件夹
* 从网页拖入视频 URL
* 拖入网络 URL
* 拖入文本
* 拖入快捷方式 `.lnk`
* 从其他应用拖入未知对象

如果拖入不支持的对象，应显示明确提示，而不是静默失败。

---

## 4. UI 行为

当用户将文件拖到 App 窗口上方时，进入 Drag Over 状态。

正常：

```text
┌────────────────────────────┐
│                            │
│       Video Favorites      │
│                            │
│    ┌──────────────────┐    │
│    │                  │    │
│    │   Your videos    │    │
│    │                  │    │
│    └──────────────────┘    │
│                            │
└────────────────────────────┘
```

拖入有效视频时：

```text
┌────────────────────────────┐
│                            │
│                            │
│     Drop videos here       │
│                            │
│     + Add to Favorites     │
│                            │
└────────────────────────────┘
```

视觉上应明显提示当前窗口接受文件。

---

## 5. 拖拽生命周期

前端处理以下事件：

```text
dragenter
    ↓
dragover
    ↓
drop
```

核心流程：

```text
dragenter
    → 检测 DataTransfer 是否包含文件

dragover
    → 阻止默认行为
    → 显示 Drop Zone

drop
    → 获取 File[]
    → 提取本地路径
    → 调用 AddVideos(paths)
```

---

## 6. Frontend API

不要逐个调用：

```typescript
AddVideo(path)
AddVideo(path)
AddVideo(path)
```

V1 直接提供批量 API：

```typescript
AddVideos(paths: string[]): Promise<AddVideoResult[]>
```

例如：

```typescript
const result = await AddVideos([
    "D:\\Videos\\a.mp4",
    "D:\\Videos\\b.mkv",
    "D:\\Videos\\c.webm",
])
```

---

## 7. 批量添加结果

每个文件单独返回结果。

```typescript
interface AddVideoResult {
    path: string
    success: boolean
    videoId?: number
    error?: string
}
```

例如：

```json
[
    {
        "path": "D:\\Videos\\a.mp4",
        "success": true,
        "videoId": 101
    },
    {
        "path": "D:\\Videos\\b.mkv",
        "success": false,
        "error": "Already in favorites"
    },
    {
        "path": "D:\\Videos\\c.txt",
        "success": false,
        "error": "Unsupported file type"
    }
]
```

这样一个文件失败不会影响其他文件。

---

## 8. Backend API

Go：

```go
type VideoService struct {
    repo      VideoRepository
    metadata  MetadataService
    thumbnail ThumbnailService
}

func (s *VideoService) AddVideos(paths []string) []AddVideoResult
```

内部：

```text
AddVideos
    │
    ├── path 1 → AddVideo
    ├── path 2 → AddVideo
    ├── path 3 → AddVideo
    └── ...
```

业务规则仍然统一复用：

```go
func (s *VideoService) AddVideo(path string) (*Video, error)
```

即：

> **Drag & Drop 只是输入方式，不应该产生独立的业务逻辑。**

---

## 9. 文件类型判断

根据文件扩展名进行第一层过滤：

```go
var supportedExtensions = map[string]struct{}{
    ".mp4":  {},
    ".mkv":  {},
    ".webm": {},
    ".mov":  {},
    ".m4v":  {},
    ".avi":  {},
}
```

大小写不敏感：

```text
.MP4
.mp4
.Mp4
```

都视为有效。

---

## 10. 拖入文件夹

如果用户拖入：

```text
D:\Videos\
```

V1 不自动扫描。

UI 提示：

```text
Folders are not supported yet.

Please drop video files instead.
```

这样可以避免 V1 无意中演变成：

```text
Drag Folder
    ↓
Recursive Scan
    ↓
Thousands of Videos
```

---

## 11. 大量文件拖入

用户可能一次拖入：

```text
200 videos
```

UI 不应等待所有文件处理完成。

流程：

```text
Drop 200 files
       ↓
Validate
       ↓
Batch Add
       ↓
UI immediately updates
       ↓
Background metadata / thumbnail jobs
```

例如：

```text
Added 187 videos
Skipped 13
```

---

## 12. Duplicate 行为

如果拖入的视频已经存在：

```text
D:\Videos\a.mp4
```

不创建新的数据库记录。

返回：

```text
Already in favorites
```

UI 可以显示：

```text
187 added
13 skipped
```

而不是弹出 13 个对话框。

---

## 13. 错误展示

批量导入完成后显示一个简短结果：

```text
Added 8 videos

Skipped:
• movie.mkv — Already in Favorites
• test.exe — Unsupported File Type
```

不要因为一个文件失败就打断整个批量操作。

---

## 14. File Picker 与 Drag & Drop 统一

三个添加入口：

```text
┌───────────────────────┐
│                       │
│     + Add Video       │
│                       │
│  ┌─────────────────┐  │
│  │ Drop videos     │  │
│  │      here       │  │
│  └─────────────────┘  │
│                       │
└───────────────────────┘
```

最终全部进入：

```text
                    ┌── File Picker
                    │
Add Video Input ────┼── Drag & Drop
                    │
                    └── Explorer Context Menu
                              ↓
                         AddVideos()
                              ↓
                         AddVideo()
                              ↓
                    ┌─────────┴─────────┐
                    ▼                   ▼
                 SQLite             Background
                                    Thumbnail
```

这样可以保证三种入口的行为完全一致。

---

## 15. Explorer 右键菜单

Explorer Shell Extension 不属于 V1 实现内容，但架构上预留：

```text
Explorer
    │
    └── Add to Video Favorites
              │
              ▼
        AddVideos(paths)
```

因此未来增加右键菜单时：

**不需要修改核心业务逻辑。**

---

## 16. V1 Definition of Done 更新

在原有 Definition of Done 上增加：

### Add Video

* [ ] File Picker 添加单个视频
* [ ] Drag & Drop 添加单个视频
* [ ] Drag & Drop 添加多个视频
* [ ] 拖入非视频文件时给出提示
* [ ] 拖入文件夹时给出提示
* [ ] 重复文件不会产生重复记录
* [ ] 批量添加时单个失败不会影响其他文件
* [ ] 添加完成后显示汇总结果
* [ ] Thumbnail 异步生成

### Architecture

* [ ] File Picker 和 Drag & Drop 共用 `AddVideos()`
* [ ] `AddVideos()` 共用 `AddVideo()`
* [ ] 后续 Explorer Context Menu 可直接调用同一套业务逻辑
