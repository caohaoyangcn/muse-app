# OpenList fork 与 BitQiu 驱动实施计划

**状态：** Proposed / Spike
**范围：** 在 OpenList fork 中新增 `drivers/bitqiu`，把 AList v3.55 的比特球驱动移植到 OpenList v4；Windows 构建、测试、可选分发
**非范围：** Muse 的 WebDAV 客户端与云端片源 UI（见 [Muse WebDAV 云盘片源](webdav-cloud-source.md)）、Muse 直连比特球私有 API
**仓库：** OpenList fork（独立仓库/独立进程），不把 OpenList 代码复制进 Muse
**许可证：** OpenList 为 AGPL-3.0；fork 与其分发的二进制必须满足 AGPL
**最后更新：** 2026-11-05

---

## 1. 调研结论

### 1.1 比特球没有公开 WebDAV

比特球云盘的官方客户端/网页没有公开 WebDAV 入口。AList 3.55.0 新增的 `drivers/bitqiu` 走的是 `pan.bitqiu.com` 的私有 HTTP 接口：

- 登录：`POST /loginServer/login`，密码先做 MD5；
- 用户信息 / 根目录：`/user/getInfo` → `rootDirId`；
- 列目录：`/apiToken/cfi/fs/resources/pages`，按 `parentId` 分页；
- 下载直链：`/download/getUrl`，返回临时 URL；
- 新建目录：`/resource/create`；
- 改名：`/resource/rename`；
- 移动：`/resource/remove`（命名可疑，需真实账号验证）；
- 复制：`/apiToken/cfi/fs/async/copy` + `/apiToken/cfi/fs/async/manager` 轮询；
- 删除：`/resource/delete`；
- 上传：`/apiToken/cfi/fs/upload/v2/initialize` + `/upload/v2/complete`，分片 1 MiB；
- 通用请求头包含 `user-platform`（UUID，首次自动生成并回写）、`x-kl-saas-ajax-request`、`x-requested-with` 等。

参考：[AList BitQiu 文档](https://alistgo.com/zh/guide/drivers/bitqiu.html)、[AList drivers/bitqiu](https://github.com/AlistGo/alist/tree/main/drivers/bitqiu)。

### 1.2 OpenList 主线没有 BitQiu 驱动

截至 2026-11-05，OpenList `drivers/all.go` 没有 `bitqiu` 导入（[drivers/all.go](https://github.com/OpenListTeam/OpenList/blob/main/drivers/all.go)）。因此用 OpenList 直接挂比特球不可行，需要 fork 并移植。

### 1.3 两个项目的差异

| AList v3.55 实现 | OpenList v4 适配 |
| --- | --- |
| 模块 `github.com/alist-org/alist/v3` | `github.com/OpenListTeam/OpenList/v4` |
| `MakeDir/Move/Rename/Copy/Put` 返回 `(model.Obj, error)` | OpenList 支持 `driver.MkdirResult/MoveResult/RenameResult/CopyResult/PutResult`，保留返回值并加编译期断言 |
| `streamPkg.CacheFullInTempFileAndHash(file, utils.MD5)` | `stream.CacheFullAndHash(file, nil, utils.MD5)` |
| `baseURL` 常量 | 改成 `apiBase` 字段，默认 `https://pan.bitqiu.com`，便于 `httptest` 注入 |
| `utils.SliceConvert` / `utils.GetMD5EncodeStr` / `op.MustSaveDriverStorage` / `base.NewRestyClient` | OpenList 均有同名能力，替换导入路径即可 |
| 登录遇到 `10401/10404` 重新登录 | 增加最大重试次数，避免接口异常时死循环 |
| `moveResourceURL = "/resource/remove"` | 先保留原行为；真实账号验证移动，若不符按抓包修正 |

### 1.4 OpenList 现成驱动快照（2026-11-05）

在决定 fork 之前，先确认一件事：**除了比特球，OpenList 主线已经内置了大量现成网盘驱动**。如果用户只是要接其他网盘，直接使用 OpenList 主线即可，不需要为了 BitQiu 维护 fork。

以下清单来自 `drivers/all.go` 与各驱动 `meta.go` 的 `Name` 字段，按「添加存储」页面显示名整理；`*Share` / `*Link` 通常只处理分享链接，能力可能只读。

**国内常用网盘**

| 网盘 | 驱动名 |
| --- | --- |
| 115 网盘 | `115 Cloud` / `115 Open` / `115 Share` |
| 123 云盘 | `123Pan` / `123 Open` / `123PanShare` / `123PanLink` |
| 百度网盘 / 相册 | `BaiduNetdisk` / `BaiduPhoto` |
| 阿里云盘 | `Aliyundrive` / `AliyundriveOpen` / `AliyundriveShare` |
| 钉钉文档 | `AliDoc` |
| 夸克 / UC | `Quark` / `UC` / `QuarkOpen` / `QuarkTV` |
| 迅雷云盘 | `Thunder` / `ThunderX` / `ThunderBrowser` |
| 天翼云盘 | `189Cloud` / `189CloudPC` / `189CloudTV` |
| 中国移动云盘 | `139Yun` |
| 联通云盘 | `WoPan` |
| 腾讯微云 | `WeiYun` |
| 蓝奏云 | `Lanzou` / `ILanZou` |
| 城通网盘 | `CtFile` |
| 超星 | `ChaoXingGroupDrive` |
| 豆包 | `Doubao` / `DoubaoNew` / `DoubaoShare` |
| WPS | `WPS` |
| Teambition | `Teambition` |
| 联想 NAS 分享 | `LenovoNasShare` |
| MoPan | `MoPan` |
| 光雅盘 | `GuangYaPan` |

**海外 / 国际网盘**

- `GoogleDrive` / `GooglePhoto`
- `Onedrive` / `OnedriveAPP` / `Onedrive Sharelink`
- `Dropbox`
- `Mega_nz`
- `PikPak` / `PikPakShare`
- `ProtonDrive`
- `YandexDisk`
- `MediaFire`
- `Terabox`
- `Degoo`
- `FebBox`
- `MediaTrack`
- `HalalCloud` / `HalalCloudOpen`

**对象存储、协议与自建服务（不是公有网盘）**

- 对象存储：`S3`、`Azure Blob Storage`、`Bunny Storage`、`USS`（又拍云）
- 协议：`WebDAV`、`FTP`、`SFTP`、`SMB`
- 自建：`Local`、`Cloudreve` / `Cloudreve V4`、`KodBox`、`Seafile`、`Emby`、`OpenList` / `OpenListShare`、`AList V3`、`Teldrive`
- 工具型：`Alias`、`Crypt`、`Chunk`、`Strm`、`URL Tree`、`Virtual`、`AutoIndex`、`GitHub API`、`GitHub Releases`、`CNB Releases`、`IPFS API`

**对本计划的含义**

- **BitQiu 仍然不在主线**：上述清单没有 `BitQiu`，`drivers/all.go` 也没有注册它；Muse 接比特球仍需要 OpenList fork。
- **fork 只补 BitQiu 一个驱动**：不需要复制/重写其他网盘；合并上游时只保留 `drivers/bitqiu`、`drivers/all.go` 的一行注册和（如需要）构建/文档改动。
- **可以先用其他网盘验证 Muse**：在 BitQiu 驱动完成前，可以用 OpenList 主线挂 115、123、阿里云盘、夸克、OneDrive、WebDAV 等，跑通 Muse 的浏览/添加/封面/播放链路。
- **主线在持续增加驱动**：实现 fork 前先 rebase 到当时最新 main，并重新核对 `drivers/all.go`；不要以本快照作为永久列表。

来源：[drivers 目录](https://github.com/OpenListTeam/OpenList/tree/main/drivers) · [drivers/all.go](https://github.com/OpenListTeam/OpenList/blob/main/drivers/all.go) · [README 功能列表](https://github.com/OpenListTeam/OpenList/blob/main/README.md) · [官方驱动文档](https://doc.oplist.org/guide/drivers)

## 2. 目标与非目标

### 目标

- OpenList fork 可在 Windows amd64 构建，包含 BitQiu 驱动。
- 添加 BitQiu 存储后可登录、列目录、分页、下载/预览、上传、新建目录、改名、移动、复制、删除。
- BitQiu 存储可通过 OpenList `/dav` 暴露为 WebDAV，供 Muse 只读访问。
- 驱动行为尽量与 AList v3.55 一致，差异有记录、有测试。
- 构建产物可独立分发；源码、许可证、构建说明齐全。

### 非目标

- 不把 BitQiu 驱动合并进 Muse。
- 不在 Muse 里保存比特球账号密码；密码只存在 OpenList 配置中。
- 不保证私有 API 永久稳定；接口变化时按上游/抓包维护。
- 不要求 OpenList 默认随 Muse 安装包分发（见 §7）。

## 3. 仓库与分支策略

- 在 Muse 仓库同级克隆：`../OpenList`，分支 `feat/bitqiu-driver`。
- 如本机已登录 GitHub，推送到个人 fork；否则保留本地分支并生成 patch。
- **不把 OpenList 作为 Muse 的 git submodule，不复制其源码进 Muse。**
- 固定基线：记录 OpenList 上游 commit（当前调研基线 `ef5d8c8`，2026-11-04）和 AList 参考 commit（`92e7453`，2026-10-21）。实现时以 fork 的实际基线为准，并写入 `SOURCE.txt`。
- 上游 PR 作为可选项；合入前使用 fork。

## 4. 驱动设计

### 4.1 文件

```text
drivers/bitqiu/
  meta.go        Addition、driver.Config、init() 注册
  types.go       Response / Resource / ResourcePage / DownloadData / UserInfoData /
                 CreateDirData / AsyncManagerData / UploadInitData / ChunkUploadResponse
  util.go        Resource.toObject、时间解析、改名后路径更新
  driver.go      Init/Drop/List/Link/MakeDir/Move/Rename/Copy/Remove/Put
  driver_test.go httptest 单测
```

`drivers/all.go` 增加：

```go
_ "github.com/OpenListTeam/OpenList/v4/drivers/bitqiu"
```

### 4.2 Addition

```go
type Addition struct {
    driver.RootID
    Username     string `json:"username" required:"true"`
    Password     string `json:"password" required:"true"`
    UserPlatform string `json:"user_platform" help:"Optional device identifier; auto-generated if empty."`
    OrderType    string `json:"order_type" type:"select" options:"updateTime,createTime,name,size" default:"updateTime"`
    OrderDesc    bool   `json:"order_desc"`
    PageSize     string `json:"page_size" default:"24" help:"Number of entries to request per page."`
    UserAgent    string `json:"user_agent" default:"..."`
}
```

`driver.Config`：

```go
var config = driver.Config{
    Name:        "BitQiu",
    DefaultRoot: "0",
}
```

### 4.3 关键实现点

- `Init`：生成/复用 `UserPlatform`（UUID），用 `op.MustSaveDriverStorage` 回写；创建带 cookie jar 的 resty client；登录。
- `login`：密码 MD5；成功后保存 `userID`；`ensureRootFolderID` 在根为 `0`/空时取 `rootDirId` 并回写。
- `List`：按 `parentId` 分页，`hasNext` 停止；把 `Resource` 转成 `*Object`；`ResourceType==1` 为目录。
- `Link`：`/download/getUrl` 返回临时 URL；`10401/10404` 重新登录后重试一次。
- `MakeDir/Move/Rename/Copy/Remove/Put`：保持 AList 行为；`Copy` 轮询异步任务，超时/失败有明确错误。
- `Put`：先 `stream.CacheFullAndHash` 缓存并算 MD5；小文件直接上传，大文件分片；进度回调 `up`。
- 重试：所有接口最多 2 次登录重试；`postForm` 不忽略 HTTP 状态；错误消息带操作名。
- 日志：不记录密码、token、下载直链。

### 4.4 与 OpenList 的接口对齐

- 编译期断言：
  ```go
  var _ driver.Driver = (*BitQiu)(nil)
  var _ driver.PutResult = (*BitQiu)(nil)
  ```
- 写操作返回 `(model.Obj, error)` 的，同时满足 OpenList 的 `*Result` 接口；如果某方法 OpenList 只接受 `error`，则实现无返回值的版本并保持内部返回对象用于缓存。
- `Object` 内嵌 `model.Object`，实现 `GetID/GetPath/SetPath/GetName/IsDir/ModTime/CreateTime/GetSize/GetHash`。
- `op.MustSaveDriverStorage` 只在真实 OpenList 运行时调用；单测里通过设置非零 `RootFolderID` 跳过。

## 5. 实施步骤

### Phase 0：调研与基线

- [ ] 固定 OpenList 上游 commit、AList 参考 commit。
- [ ] 记录 AList 驱动端点、请求头、错误码。
- [ ] 确认 OpenList 的 `driver.*Result`、`stream.CacheFullAndHash`、`op.MustSaveDriverStorage` 可用。
- [ ] 确认 Windows 构建需要 Go 1.26.4+ 与 OpenList-Frontend 产物。

### Phase 1：移植最小可读驱动

- [ ] `meta.go` / `types.go` / `util.go`。
- [ ] `driver.go` 的 `Init/Drop/List/Link`。
- [ ] `apiBase` 可注入，便于 httptest。
- [ ] `drivers/all.go` 注册。
- [ ] `go build ./...`。

### Phase 2：写操作与上传

- [ ] `MakeDir/Move/Rename/Copy/Remove`。
- [ ] `Put`（小文件 + 分片 + 进度）。
- [ ] 复制任务轮询与失败处理。
- [ ] 重试上限、错误消息、脱敏日志。

### Phase 3：测试

- [ ] `httptest` 覆盖登录（MD5、headers）、分页、`hasNext`、直链、`10401` 重登、根目录 ID。
- [ ] `toObject` 的目录/文件/时间/MD5。
- [ ] 写操作请求体与返回对象。
- [ ] `go test ./drivers/bitqiu`、`go vet`/`gofmt`。
- [ ] 真实账号手动矩阵（见 §6）。

### Phase 4：构建与分发

- [ ] 固定 OpenList-Frontend 版本，构建 `public/dist`。
- [ ] Windows amd64 构建：`CGO_ENABLED=0`、`-trimpath -ldflags "-s -w"`。
- [ ] 产出 `OpenList-fork-<版本>-win64.zip`、`OpenList-source-<版本>.zip`、SHA256。
- [ ] 编写 `LICENSE.txt`、`SOURCE.txt`、`README.txt`、`Start-OpenList.bat`（见 §7）。

### Phase 5：Muse 联调

- [ ] Muse 添加 WebDAV 连接 `http://127.0.0.1:5244/dav`。
- [ ] 浏览 BitQiu 目录、添加视频、生成封面/时长。
- [ ] 播放缓存、OpenList 网页跳转。
- [ ] 记录端到端结果到 `webdav-cloud-source.md` 的进度表。

## 6. 测试与手动验证

### 单测

| 用例 | 断言 |
| --- | --- |
| 登录 | 表单 `passport`、MD5 密码、`user-platform`、`org_channel` |
| 登录失败 | 返回 `login failed: ...`，不泄露密码 |
| 用户信息 | `rootDirId` 写回 `RootFolderID` |
| 分页 | `hasNext=true` 时继续，第二页拼接正确 |
| 直链 | 返回 `model.Link{URL}`；空 URL 报错 |
| 10401/10404 | 重新登录一次后成功；超过重试上限报错 |
| 对象转换 | 目录/文件、size、create/update time、MD5 |
| 写操作 | MakeDir/Move/Rename/Copy/Remove 请求体与返回对象 |
| 上传 | 小文件直接返回；分片 offset/len/hash 正确；进度单调 |

### 真实账号手动矩阵

1. 添加 BitQiu 存储，登录成功。
2. 列根目录、进入子目录、分页加载。
3. 下载/预览一个文件。
4. 新建目录。
5. 上传一个小文件（<1 MiB）和一个大文件（跨分片）。
6. 改名、移动、复制、删除。
7. 在 OpenList 中开启 WebDAV，用 Muse 连接 `/dav`。
8. 记录移动接口是否符合预期；若 `/resource/remove` 不是移动，按抓包修正。

> 真实账号信息只放在本地 OpenList 配置，不提交到仓库、不写进测试或日志。

## 7. 构建、分发与 AGPL

### 7.1 构建

- OpenList `go.mod` 要求 Go 1.26.4、toolchain 1.27.3，明显高于 Muse 的 1.25；**不要**在 `wails build` 里顺带编译 OpenList。
- 在 OpenList fork 仓库独立构建：
  - 固定 OpenList-Frontend 版本，生成 `public/dist`；
  - `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o openlist.exe`。
- 构建产物校验：PE32+、版本、SHA256。

### 7.2 分发形态

按已确认的产品决策，OpenList **不作为默认安装包的一部分**：

| 资产 | 内容 | 默认 |
| --- | --- | --- |
| `Muse-<版本>-win64.zip` | Muse + FFmpeg，不含 OpenList | 是 |
| `OpenList-fork-<版本>-win64.zip` | openlist.exe、Start-OpenList.bat、README、AGPL LICENSE、SOURCE | 否 |
| `OpenList-source-<版本>.zip` | 对应源码 | 否 |

- 需要网盘的用户自行下载 OpenList 包，放在 `Muse.exe` 同目录的 `OpenList/`，或在 Muse 设置里填写外部 WebDAV 地址。
- 可选整合包只在明确需要时构建，不设为默认。
- `.bat` 使用 `--data "%LOCALAPPDATA%\Muse\openlist"`，保留首次启动管理员密码输出。

### 7.3 AGPL 清单

- `LICENSE.txt`：AGPL-3.0 全文。
- `SOURCE.txt`：fork URL、commit、相对上游修改、构建命令、前端版本、源码包名。
- Release 同时提供 `OpenList-source-<版本>.zip`。
- 不静态链接进 Muse，不复制 OpenList 代码进 Muse，不移除其版权/许可证。
- 主包不含 OpenList；整合包如发布，按同一套要求处理。
- 发布前建议法务确认“独立进程 + WebDAV”的聚合定性；以上是工程清单，不构成法律意见。

## 8. Muse 侧接口约定

- OpenList 默认 WebDAV：`http://127.0.0.1:5244/dav`；管理页：`http://127.0.0.1:5244`。
- Muse 连接使用 OpenList 账号（建议专用只读用户），不保存比特球凭据。
- “打开 OpenList”打开连接配置的 `web_url`；为空时从 endpoint 去掉 `/dav` 推导。
- 驱动支持写操作，但 Muse 只读；文件管理在 OpenList 网页完成。

## 9. 提交拆分（OpenList fork）

| 顺序 | 提交信息（建议） | 内容 |
| --- | --- | --- |
| 1 | `Add BitQiu driver` | `drivers/bitqiu` 四个文件、`drivers/all.go` |
| 2 | `Add BitQiu driver tests` | httptest 单测、对象转换、重试 |
| 3 | `Fix BitQiu move endpoint after live verification` | 仅在真实账号验证后需要时 |
| 4 | `Add BitQiu Windows release workflow` | 固定 Go/Frontend、产物、SHA256 |
| 5 | `Document BitQiu setup and AGPL source` | README、SOURCE、许可证 |

## 10. 风险

- **私有 API 不稳定**：接口/字段/风控变化会导致驱动失效；维护依赖真实账号和抓包。
- **移动接口可疑**：AList 的 `moveResourceURL` 指向 `/resource/remove`，必须实测；错误会导致移动变成删除，需优先验证。
- **上传复杂度**：分片、hash、serverCode、异步复制任务都容易在真实账号上暴露差异；先小文件再大文件。
- **Go/Frontend 工具链**：OpenList 需要更高 Go 版本和固定前端产物；构建脚本必须锁定版本。
- **AGPL 合规**：二进制/源码/许可证必须同步；不能把 OpenList 当闭源组件分发。
- **上游分叉**：长期需要 rebase；优先向上游提 PR，降低维护成本。

## 11. 进度记录

| 日期 | 阶段 | 状态 | 说明 | 提交 |
| --- | --- | --- | --- | --- |
| 2026-11-05 | 调研 | Done | 确认 AList 私有 API、OpenList 无 BitQiu 驱动、接口差异；补充 OpenList 现成驱动清单（见 §1.4） | 本提交 |
| 2026-11-05 | 计划 | Planned | 计划文档建立 | `<pending>` |
