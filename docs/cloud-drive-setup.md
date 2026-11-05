# 云盘片源设置（WebDAV / OpenList / 比特球）

Muse 通过**标准 WebDAV 只读**访问网盘。比特球没有公开 WebDAV，所以需要一个可选的
OpenList 实例把比特球挂载成 WebDAV，再让 Muse 连接它。只用本地硬盘的用户不需要安装
OpenList，也不需要看这一页。

> 核心原则：**收藏 ≠ 文件**。Muse 不会改名、移动、删除或上传网盘文件；删除收藏只删除
> 数据库记录、缩略图和本地缓存副本。

## 1. 组成

```text
比特球账号 ──> OpenList（可选，独立进程）── WebDAV ──> Muse
                                                │
                                                ├─ 浏览/多选添加
                                                ├─ 元数据/封面（本地代理）
                                                ├─ 播放（先缓存到本地）
                                                └─ 手机同 LAN 播放
```

- Muse 只连接 WebDAV，不直接保存比特球账号，也不调用比特球私有 API。
- OpenList 以独立 zip/进程分发，默认**不**打包进 Muse；它使用 AGPL-3.0，Muse 使用 MIT。
- OpenList 的挂载、改名、移动等管理操作在 OpenList 网页完成，Muse 只提供「打开管理页」。

## 2. 准备 OpenList + 比特球

> 不用比特球时，可以直接用 OpenList 主线挂其他网盘：115、123、百度、阿里云盘、夸克/UC、迅雷、天翼、移动、联通、Google Drive、OneDrive、Dropbox、PikPak 等都有现成驱动。完整清单见 [OpenList BitQiu 驱动 §1.4](spec/openlist-bitqiu-driver.md#14-openlist-现成驱动快照2026-11-05)。

1. 从 OpenList fork 的发布页下载对应平台的 OpenList（需要包含 BitQiu 驱动的构建；见
   [OpenList fork 与 BitQiu 驱动](spec/openlist-bitqiu-driver.md)）。
2. 启动 OpenList，在网页管理页登录并添加「比特球」存储，填写比特球账号。
3. 确认 OpenList 的 WebDAV 已启用，记下地址，通常是：

   ```text
   http://127.0.0.1:5244/dav
   ```

   OpenList 默认监听 `5244`；如果你改了端口或反向代理，使用实际地址。
4. 如需账号密码，在 OpenList 的 WebDAV 设置里配置；Muse 支持 Basic Auth。
5. 如果使用自签名 HTTPS，先确认浏览器能打开 OpenList；Muse 里需要关闭「校验 HTTPS 证书」。

> OpenList 只监听 `127.0.0.1` 时，Muse 必须运行在同一台电脑上。不要把没有密码的
> OpenList 直接暴露到公网。

## 3. 在 Muse 里添加连接

1. 打开 **设置 → 网盘（可选）**。
2. 点 **+ 添加网盘连接**，填写：

   | 字段 | 说明 |
   | --- | --- |
   | 名称 | 自己识别的名字，例如「比特球」 |
   | WebDAV 地址 | OpenList 的 WebDAV 地址，例如 `http://127.0.0.1:5244/dav` |
   | OpenList 网页地址 | 可选；留空时从 WebDAV 地址去掉 `/dav` 推导 |
   | 用户名 / 密码 | 可选；匿名 WebDAV 留空。密码只在 Windows 用 DPAPI 加密保存 |
   | 根目录 | 限制浏览范围，默认 `/`。填 `/BitQiu` 就只看该目录 |
   | 校验 HTTPS 证书 | 默认开启；仅自签名证书时关闭 |

3. 点 **测试连接**，确认显示「连接成功」和延迟。
4. 点 **保存连接**。

> 非 Windows 开发版不能安全保存密码，只能使用匿名 WebDAV；这是刻意的，不存在明文回退。

## 4. 添加网盘视频

1. 顶部 **＋ 添加视频** → **从网盘添加**。
2. 选择连接，进入目录，勾选一个或多个视频，点 **加入收藏**。
3. Muse 会重新读取每个文件的元数据，生成封面、时长、分辨率和编码信息。
4. 重复添加同一个 WebDAV 路径会提示「已在收藏中」，不会新建记录。

支持扩展名：`.mp4`、`.mkv`、`.webm`、`.mov`、`.avi`、`.m4v`。目录和图片不会出现在可选列表里。

## 5. 播放与缓存

- 双击远程收藏（或点播放）时，Muse 先把文件下载到本地缓存，再用系统默认播放器打开。
- 下载进度显示在右下角，可取消；已下载的部分会保留，下次继续（服务端支持 Range 时）。
- 默认缓存上限 **10 GiB**，超出后按最久未使用淘汰。设置 → 网盘 → 本地缓存 可以查看和清理。
- 缓存位置：
  - Windows：`%LOCALAPPDATA%\Muse\cloud-cache`
  - macOS / Linux 开发环境：`~/Library/Application Support/Muse/cloud-cache` 或 `$XDG_CONFIG_HOME/Muse/cloud-cache`
- 删除收藏会删除它的缓存副本；清理缓存不会删除网盘源文件。
- 「在 OpenList 中打开」/右键「在 OpenList 中打开」会在浏览器打开管理页。
- 手机在同一局域网访问 Muse 的 LAN 地址时，远程收藏会由 Muse 代理 Range 串流播放。

## 6. 离线与文件变化

| 情况 | Muse 的行为 |
| --- | --- |
| OpenList 未启动 / 网络超时 | 连接标记离线，收藏保留；已缓存副本仍可播放；恢复后自动恢复 |
| 账号密码错误（401/403） | 连接标记离线，提示检查账号密码；不删收藏 |
| 文件被删除（404） | 该收藏标记「文件缺失」，记录保留，可手动移除 |
| 远程文件大小/修改时间变化 | 下次播放重新下载缓存 |
| 服务端不支持 Range | 元数据/播放退化为完整流，仍可工作但较慢 |
| 删除连接时仍有收藏 | 拒绝删除，提示先移除该连接下的收藏 |

Muse 不会在启动时批量探测网盘文件；远程收藏不参与本地文件检测、内容指纹去重、移动重连。
只有打开/详情刷新时才做单文件校验。

## 7. 局域网手机访问

1. 顶部开启 **局域网访问**，记下地址（例如 `http://192.168.1.10:8899`）。
2. 手机浏览器打开该地址。
3. 远程收藏和本地收藏一样出现在列表中；播放时 Muse 负责把 WebDAV 的 Range 转发给手机。
4. 来源离线时，手机会看到「网盘离线」并禁用播放，恢复连接后刷新即可。

LAN API 目前没有登录鉴权，只应在可信任的家庭/办公局域网内开启。

## 8. 故障排查

| 现象 | 检查 |
| --- | --- |
| 测试连接 401/403 | OpenList 的 WebDAV 账号密码；地址是否以 `/dav` 结尾 |
| 测试连接超时 | OpenList 是否启动、端口是否被占用、防火墙；自签名证书是否关闭校验 |
| 能浏览但不能生成封面 | ffmpeg/ffprobe 是否在 PATH（Muse 包通常自带）；OpenList 是否支持 Range |
| 播放一直下载 | 文件较大或网络慢；查看右下角进度，必要时取消后重试 |
| 手机能打开列表但播放失败 | OpenList 是否允许局域网访问；手机与电脑是否同一网段；来源是否离线 |
| 缓存占用过大 | 设置 → 网盘 → 本地缓存 → 清理缓存；上限默认 10 GiB |

## 9. 代码与计划

- 实施计划：[Muse WebDAV 云盘片源](spec/webdav-cloud-source.md)
- OpenList/BitQiu：[OpenList fork 与 BitQiu 驱动](spec/openlist-bitqiu-driver.md)
- 添加来源弹窗：[添加视频来源弹窗](spec/add-video-source-dialog.md)
- 路线图：[云盘片源与添加入口路线图](spec/cloud-drive-roadmap.md)
