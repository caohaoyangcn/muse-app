# 云盘片源与添加入口路线图

**状态：** Active（实施中）
**用途：** 把三块工作拆成可独立追踪、可分别提交的文档；进度以各文档的 checklist、提交表和本文的状态表为准。
**最后更新：** 2026-11-06

三份实施计划：

1. [添加视频来源弹窗](add-video-source-dialog.md) —— 前端入口，以及扫描文件夹与目录监控的统一。
2. [Muse WebDAV 云盘片源](webdav-cloud-source.md) —— 连接、浏览、添加、元数据、播放、离线语义。
3. [OpenList fork 与 BitQiu 驱动](openlist-bitqiu-driver.md) —— 私有 API 移植、构建、可选分发。

## 依赖关系

```text
OpenList BitQiu 驱动 ──┐
                       ├── 真实比特球端到端联调
Muse WebDAV 客户端 ────┘
        │
        └── 网盘浏览/添加 API ── 添加来源弹窗的「从网盘添加」卡片

本地目录监控入口 ── 添加来源弹窗的「扫描文件夹」卡片（复用 AddWatchDir）
```

- 添加来源弹窗的本地/扫描部分**不依赖**网盘；可以先交付，网盘卡片先显示「未配置」。
- WebDAV 客户端可以用 `httptest` 或任意 OpenList/AList 实例开发，不必等 BitQiu 驱动。
- BitQiu 驱动可以在 OpenList fork 仓库独立完成；联调时再与 Muse 对接。
- OpenList 默认不随 Muse 安装包分发；需要网盘的用户单独下载。

## 建议顺序

1. **OpenList BitQiu 驱动（可并行）**：先确认私有 API 能登录、列目录、下载；移动/上传等写操作可在只读链路验证后再补。
2. **Muse WebDAV 客户端 + 连接/浏览**：用测试 WebDAV 或 OpenList 实例打通。
3. **添加来源弹窗（本地 + 扫描）**：不依赖网盘，先把入口改好。
4. **网盘卡片接入来源弹窗**：接上浏览/添加。
5. **媒体代理 + 播放委托 OpenList + 离线/归档语义**。
6. **端到端联调**：OpenList fork + BitQiu + Muse。
7. **文档与可选分发**：OpenList 独立 zip、源码包、AGPL 清单。

## 状态表

| 工作流 | 文档 | 状态 | 当前里程碑 | 下一个提交 |
| --- | --- | --- | --- | --- |
| 添加来源弹窗 | [add-video-source-dialog.md](add-video-source-dialog.md) | In progress | 三来源已接通；仅剩 Windows 原生窗口检查 | 文档收尾 |
| WebDAV 云盘片源 | [webdav-cloud-source.md](webdav-cloud-source.md) | In progress | M9 + 无缓存播放/来源归档/备份标记完成；真实 OpenList/BitQiu 待联调 | OpenList fork 联调 |
| OpenList BitQiu 驱动 | [openlist-bitqiu-driver.md](openlist-bitqiu-driver.md) | Proposed / Spike | Phase 1 最小可读驱动 | `Add BitQiu driver`（OpenList fork） |

> 状态只在本表和各文档的「进度记录」中更新；不要用代码注释代替进度记录。

## 如何更新进度

每次开始/完成一个阶段：

1. 在对应文档的 checklist 勾选已完成项；
2. 在该文档的「进度记录」追加一行：日期、阶段、状态、实际结果、提交短 SHA；
3. 更新本文状态表的「当前里程碑 / 下一个提交」；
4. 如果实际实现与计划不同，在文档的「风险与取舍」或「实现记录」中写明原因，不要静默改计划。

提交信息保持单一职责，按各文档的「提交拆分」；一个提交只完成一个可验证的边界。

## 跨工作流的固定决策

- **收藏 ≠ 文件**：Muse 不复制/移动/删除网盘源文件；删除收藏只删记录、缩略图和封面。
- **Muse 只读 WebDAV**：不在 Muse 内改名/移动/删除/上传；管理动作跳 OpenList 网页。
- **比特球经 OpenList 桥接**：Muse 不直连比特球私有 API，不保存比特球账号。
- **OpenList 可选**：默认 Muse 包只含本地能力；OpenList 以独立 zip/独立进程分发。
- **OpenList 现成网盘优先**：115、123、百度、阿里、夸克/UC、迅雷、天翼、移动、联通、Google Drive、OneDrive、Dropbox、PikPak 等主线已内置；只有比特球需要 fork。清单见 [OpenList BitQiu 驱动 §1.4](openlist-bitqiu-driver.md#14-openlist-现成驱动快照2026-11-05)。
- **Muse 不保存视频内容**：远程播放打开 OpenList；本地代理只用于缩略图/元数据。
- **远程不参与本地指纹/查重/移动重连**：避免网络读全文件和误判。
- **离线不删收藏**：网络/鉴权失败只标来源离线；只有确认 404 才标 missing，且仍保留记录。
- **停用来源 = 归档**：活动收藏进入回收站的 `archived` 原因；长期保留、不自动清理、可清空归档；重新启用不自动恢复。

## 发布门槛

每个工作流独立满足以下条件才可标记完成：

- [ ] 对应文档的 checklist 全部完成。
- [ ] 单元测试覆盖新增行为；`go test ./...` 通过。
- [ ] 修改 Go 代码的运行 `golangci-lint run ./...`；修改前端运行 `npm run lint` 与 `npm run build`。
- [ ] 手测矩阵记录在文档的「进度记录」中。
- [ ] 提交历史按文档的「提交拆分」保持单一职责。
- [ ] OpenList fork 的 AGPL 许可证/源码说明随二进制发布。

## 进度记录

| 日期 | 事件 | 说明 | 提交 |
| --- | --- | --- | --- |
| 2026-11-05 | 计划建立 | 三份实施计划与路线图写入 `docs/spec/` | `9da1586` |
| 2026-11-05 | 本地/扫描入口 | 初版 `watch.ScanOnce` + `AddSourceDialog` + Library 接入；后收拢为目录监控 | `0800c72` / `d40a9ac` |
| 2026-11-05 | 语义收拢 | 扫描文件夹改为 `pickWatchDir` + `addWatchDir`，删除一次性 `ScanOnce` | `e25c96c` |
| 2026-11-05 | OpenList 调研 | 记录主线现成驱动清单；确认 BitQiu 仍需 fork | 本提交 |
| 2026-11-06 | WebDAV M1 | v24 schema、远程视频字段、仓储查询与过滤、迁移/仓储测试 | `6629ebc` |
| 2026-11-06 | WebDAV M2 | 只读 WebDAV 客户端与 `httptest` 覆盖 | `74d4126` |
| 2026-11-06 | WebDAV M3 | secret 抽取、连接 CRUD/删除保护/健康检查 | `efa5e34` |
| 2026-11-06 | WebDAV M4 | 远程收藏、浏览/添加 API、设置与网盘浏览弹窗 | `edcb7a7` |
| 2026-11-06 | WebDAV M5 | 127.0.0.1 媒体代理、远程元数据/封面 resolver | `b0b059a` |
| 2026-11-06 | WebDAV M6 | 下载缓存、LRU、OpenList 管理页、播放进度 | `ba947c0` |
| 2026-11-06 | WebDAV M7 | 本地文件检测/卷/指纹/重连排除远程；404 与网络失败分流 | `7973434` |
| 2026-11-06 | WebDAV M8 | LAN 远程 Range 转发与移动端离线状态 | `a58bb3b` |
| 2026-11-06 | WebDAV M9 | 设置文档、本地 WebDAV 集成测试、静态检查与构建 | `1492a37` / `2e030d6` |
| 2026-11-06 | 产品修正 | 移除缓存；播放跳 OpenList；来源停用归档；跨层备份徽章；手机归档 tab | `3172b46` / `703b308` |
