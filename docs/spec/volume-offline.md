# Muse 磁盘未接入（外接盘拔掉）识别设计

**Status:** Implemented (v1)
**Scope:** 区分「文件不在了」与「文件所在的盘没插」：后者不写 `missing`、不降级卡片，只在卡片右下角标注「⚠ 磁盘未接入」
**Target:** 桌面端（Wails）+ 局域网移动端（共用 `api.Library` 门面）
**Version:** v1
**依赖:** 无新增依赖；判据是卷根是否可访问（`filepath.VolumeName`）

---

## 0. 问题

收藏是引用，不是文件（`spec-v1.md` §41）。引用失效有两种，此前被合并成一种：

1. **文件不在了**（删除 / 移动），卷仍在 → `status = 'missing'`，界面 `⚠ 文件缺失`，这是正确的；
2. **卷不在了**（移动硬盘被拔掉）→ 现有 `RefreshAllStatuses` 在启动时对每条收藏 `os.Stat(path)`，失败就写成 `missing`。

第 2 种写法有两个后果：

- 用户明明只是拔了盘，却被标记成「文件缺失」；
- 更糟的是，盘重新插回来之后状态仍然是 `missing`（下次启动前不会自己恢复），除非用户手动重新添加。

而且把盘拔掉当成文件缺失，会让「移动重连」（`file-relocation.md`）的候选判断变复杂：`missing` 与「卷未挂载」必须能被区分开。

## 1. 目标与非目标

v1 目标：

- 整个卷不可访问时，认出这是「磁盘未接入」而不是「文件缺失」；
- 卡片**照常展示**（缩略图、标题、元数据、点赞都在），只多一个右下角提示；
- 启动时、运行中（后台轮询）都能识别；盘插回来，状态与提示自动恢复；
- 拔盘**不写数据库**：`status` 保持原值，卷状态只存在于内存与读模型里。

v1 非目标：

- 不区分「U 盘没插」与「网络共享离线」之外的其他不可达原因（权限、坏道）；
- 不做逐文件轮询：只按卷探测一次；
- 不主动发现文件移动（见 `file-relocation.md` §8）；
- 不改「失效视频自动检测 / 回收站」的整体方案（`TODO.md`）。

## 2. 判定：卷根是否可访问

`backend/video/volume.go`

- `VolumeRoot(path)`：`filepath.VolumeName` + 分隔符（`D:\`、`\\server\share\`）；没有卷名的路径落在文件系统根（`/`），与 `relocate.go` 原先的私有实现一致，已收敛为这一个函数。
- 一个卷「接入」= `stat(VolumeRoot(path))` 成功。盘符不存在时 Windows 立即返回错误；这里不看具体错误码，理由是「能 stat 通」才是收藏可用的唯一判据。
- 卷状态按 `strings.ToLower(root)` 归一化后缓存（`volumeState`），**永不落库**。

## 3. 读模型：`volumeOffline` / `volumeRoot`

`Video` 新增两个只读字段（`backend/video/model.go`）：

| 字段 | 含义 |
| --- | --- |
| `volumeOffline` | 该视频所在卷当前未接入。零值 `false` 表示正常，因此未装饰的记录不会被误标。 |
| `volumeRoot` | 该视频所在的挂载根，UI 用它说明「哪块盘没插」。 |

所有读路径（`List` / `Search` / `Get`）只做**盖章**（`volumeState.stamp`）：把缓存里的答案写到记录上，同时登记卷根供下一次探测。**读路径不 stat 磁盘**，所以行数上万、或某个网络共享无响应时，列表也不会被拖住。

`Get` 与 `RefreshStatus` 都会盖章；`RefreshStatus` 是一次单文件操作，允许直接问磁盘（见 §4）。

## 4. 何时探测

| 时机 | 行为 |
| --- | --- |
| 启动 `RefreshAllStatuses` | 先登记卷根并 `probe()` 一次，再对每个在线卷做整卷对账（见下行）；卷未接入的行**直接跳过**，不改 `status` |
| 后台 `WatchVolumes`（5s） | 每次探测所有已知卷；只有「状态翻转」才发 `video:volume` 事件 |
| 前端挂载后 `RefreshVolumes` | 主动探测一次，保证首屏就能看到提示（否则要等第一次轮询） |
| 单文件失败（`Open` / `OpenContainingFolder` / `RefreshStatus`） | 直接 `stat` 卷根；若未接入，报「磁盘未接入」，**不写 missing**，并把这次观察记回缓存 |
| 卷恢复接入（翻转 online） | `reconcileVolume` 对整卷对账：回来的文件改回 `available`；确认缺失的文件自动移除（见 `missing-auto-removal.md`）；整卷全部缺失时只标 `missing`，不删除 |

探测由 `probeMu` 串行化：多个调用者同时要求刷新，磁盘只被问一次；读者不拿这把锁。`WatchVolumes` 的停止函数不等待正在进行的探测（可能卡在网络共享上），程序退出不会被它拖住。

## 5. 界面

- 卡片（网格 / 列表）：`volumeOffline === true` 时不再降级、不再显示「文件缺失」，缩略图与元数据照常，缩略图右下角加 `⚠ 磁盘未接入`（`title` 里带卷根与完整路径）；时长徽标与它并排。
- 收藏列表（桌面端网格 / 列表）：列表上方提供「硬盘」多选筛选，选项来自当前已加载影片的 `volumeRoot`（未接入的卷带 ⚠，仍可选中）。未选择时显示全部；多选取并集；纯前端过滤，不新增请求。已选但当前列表没有的卷以 0 计数保留，便于清除，避免筛选被静默取消。
- 「文件缺失」徽标只在 `status === 'missing'` 且卷已接入时出现。
- 详情面板：文件状态显示「磁盘未接入」，播放按钮禁用并说明原因。
- 播放失败：前端先看 `RefreshStatus` 的结果；卷未接入时只提示，不弹「移除收藏」对话框（避免用户把还能找回的收藏删掉）。
- 移动端：网格/列表徽标、详情页播放禁用；`/stream/video/{id}` 对该情况返回 404 `磁盘未接入`。
- 番号查重面板（`DuplicateView`，桌面与移动端）：版本卡片同样把「磁盘未接入」与「文件缺失」分开显示。

## 6. 与查重 / 扫描的关系（同一不变式）

「盘没插」不是「文件不可读」，因此：

- **内容扫描**（`contentscan.go`）：T0 stat 失败但卷未接入时跳过该行，**不写 `hash_error`、不清指纹**——否则拔盘一次就会忘掉这些字节的身份，插回来后无法再重连；
- **指纹器**（`fingerprinter.go`）在 stat 失败时同样先问卷：卷未接入就不记录失败；
- **内容查重面板**（`dedup.go`）：成员 stat 失败但卷未接入时标 `volumeOffline`，不标 `missing`，也不把该行同步成 `missing`——否则「清理重复」会诱使用户删掉安全的数据记录。

## 7. 事件与接口

| 位置 | 变化 |
| --- | --- |
| `video.VolumeChange` | `{root, online, changed}`；`changed=false` 表示首次观察 |
| `video:volume` | 仅在翻转时发出；桌面端据此重新加载列表 |
| `video:missing:swept` | 整卷对账后按卷汇总 `{root, removed, skipped}`；桌面端提示自动清理结果（见 `missing-auto-removal.md`） |
| `App.RefreshVolumes` / `api.Library.RefreshVolumes` | 主动探测并返回所有卷的观察结果；卷插回时会触发整卷对账，因此可能删除确认缺失的收藏记录（不触碰源文件） |
| `GET /api/videos` | 每个视频自带 `volumeOffline` / `volumeRoot`（JSON 直接透出） |

## 8. 已知边界

- 首次探测之前的一瞬间，卡片按「已接入」显示；前端挂载后立即调用 `RefreshVolumes`，正常启动不会看到这个窗口。
- 网络共享无响应时，探测可能长时间阻塞；它只发生在后台轮询或显式的 `RefreshVolumes` 里，不阻塞列表读取。
- 只按卷判断：同一块盘上的文件被删除，卷在线且 `stat` 返回 `ErrNotExist` 时会自动移除记录；该卷所有收藏都缺失时只标 `missing`，防盘符复用或挂载未就绪。
- 自动移除只删除记录、缩略图、分组归属与刮削数据，**绝不删除源文件**；被移除的记录不再保留 `content_hash`，之后无法再参与移动重连。
- macOS / Unix 上 `VolumeRoot` 一律是 `/`，无法区分「拔盘」与「删除」，因此自动清理不启用，只标 `missing`。
- 卷恢复后不重新刮削/抽帧：路径没变，字节大概率没变；需要时由「重新扫描 / 重新刮削」显式触发。

## 9. 测试

- `backend/video/volume_test.go`
  - `VolumeRoot` 规则（有卷名 / 无卷名）；
  - 盖章不读磁盘；未探测的卷不报 warning；
  - 探测只把真实翻转算作 `changed`；
  - 单文件观察（`record`）能教会缓存；
  - 启动对账跳过未接入卷（`status` 保持 `available`，读模型带 `volumeOffline`）；
  - `Open` 报「磁盘未接入」且不改 `status`；
  - 卷恢复后 `RefreshVolumes` 把 `missing` 改回 `available`；
  - 卷恢复后确认缺失的文件被自动移除；整卷全部缺失、权限错误、不可识别卷只标 `missing`（见 `missing-auto-removal.md`）；
  - `WatchVolumes` 在拔盘时发出一次 `video:volume`。
- 既有 `relocate_test.go` / `service_test.go` / `dedup_test.go` 回归通过。
