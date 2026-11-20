# 局域网访问口令（LAN access key）

> 状态：已实现。源码在 `backend/accesskey`、`backend/server/auth.go`、
> `backend/settings/lan_key.go`；移动端登录闸门在 `frontend/src/mobile/AuthGate.tsx`，
> 桌面展示与轮换在 `frontend/src/components/ServerBar.tsx` 与
> `frontend/src/pages/Settings.tsx`。

## 1. 问题

局域网服务随应用自动开启并监听 `0.0.0.0`，方便手机访问；在加入口令之前，
`/api/*`（含 `DELETE /api/videos/{id}`）、串流和生成图片对同网段任何设备开放。
移动端又把写操作做成了可见按钮，暴露面比早期更大。

目标是：**同一局域网里没有口令的设备既不能读库也不能写库**，而手机首次访问仍然
只需要打开桌面给出的一个链接。

## 2. 设计决定

| 决定 | 理由 |
|---|---|
| 一个随机口令，不是用户自定义密码 | 80 bit 随机值无法被猜中；用户不需要想密码，也不存在弱口令。桌面负责展示/复制/轮换。 |
| 口令明文存在 `settings` 表 | 它本来就要显示在桌面「设置 → 服务器」上；威胁模型是同网段设备，不是能读本机数据目录的攻击者。DPAPI/Keychain 只会增加跨平台失败面。 |
| 继续监听 `0.0.0.0` | 这是手机可达的前提；问题不是“端口能连上”，而是“连上后没有门槛”。口令接管访问控制。 |
| 保护**全部数据端点**，不只写 | 收藏库本身（文件名、路径、封面）就是隐私；读开放会让任意同网段设备看到整库。 |
| 扩展只读端点免口令 | `/api/extension/check` 不写库，且已有 loopback/LAN Host 检查 + JavDB 来源白名单；见 §9。 |
| 不做 HTTPS | 明文口令对主动嗅探不设防，但实现与自签证书负担大。文档明确说明只应在可信局域网使用。 |

## 3. 口令生命周期

- 格式：`crypto/rand` 10 字节 → base32 无填充 → **16 个小写字符**（80 bit）。
  展示时分成 `xxxx-xxxx-xxxx-xxxx` 四组；`accesskey.Normalize` 忽略大小写、连字符
  与空白，所以手输 `ABCD-EFGH-IJKL-MNOP` 和 URL 里的原始形式等价。
- 生成：`settings.LanAccessKey()` 在首次读取时生成并持久化；读取到损坏/空值时
  自动换成新值（宁可一次锁死设备，也不让服务裸奔）。
- 轮换：`settings.RotateLanAccessKey()` + `server.SetAccessKey()`。旧 Cookie 与已
  复制的播放 URL 立即失效；手机下一次请求收到 401，回到登录页。
- 恢复：口令无法找回，只能轮换。桌面「设置 → 服务器 → 访问口令」是唯一查看入口。

## 4. 服务端认证

`backend/server/auth.go` 的 `withLANAuth` 包裹整个 mux，默认拒绝：

| 凭据 | 使用方 | 位置 |
|---|---|---|
| Cookie `muse_lan_key` | 手机浏览器 | 登录成功后下发，`HttpOnly; SameSite=Lax; Path=/; Max-Age=1 年` |
| `Authorization: Bearer <key>` | curl / 脚本 | 请求头 |
| `?key=<key>` | Infuse / VLC | **只接受 `/stream/` 前缀**，其他路径带 query 口令仍 401 |

补充规则：

- 比较用 `subtle.ConstantTimeCompare`，且先规范化；错误口令统一返回
  `401 {"error":"需要访问口令"}`。
- 非 GET/HEAD 的受保护请求若带 `Origin`，必须与 `Host` 同源，否则 403
  （SameSite=Lax 之外的第二道锁；扩展端点不走此中间件）。
- 登录失败按客户端 IP 限流：10 次 / 10 分钟，超出返回 429 + `Retry-After`；
  成功登录清零。计数只在内存里，重启即失效。
- 响应头：`Referrer-Policy: no-referrer`、`X-Content-Type-Options: nosniff`、
  `X-Frame-Options: DENY`；`/api/*` 额外 `Cache-Control: no-store`。
- 请求日志只记 method/path，不记 query，因此 `?key=` 不会进日志。
- 口令为空表示“未配置”，只用于测试/嵌入场景；`App.StartServer` 在监听前从
  settings 读口令，读取失败则拒绝启动，桌面应用不会跑在无口令状态。`Start`
  仍会为此状态写一条 warning。

## 5. 路由面

| 路径 | 口令 |
|---|---|
| `/`、`/assets/*`、SPA 深链接（如 `/videos/5`） | 公开（否则登录页加载不出来） |
| `GET/POST/DELETE /api/session` | 公开（登录握手，失败限流） |
| `POST/OPTIONS /api/extension/check` | 公开（只读 + loopback Host + JavDB CORS） |
| `/api/*` 其余（含所有写端点） | 需要 |
| `/stream/*` | 需要（唯一接受 `?key=` 的前缀） |
| `/thumbnails/*`、`/covers/*`、`/avatars/*` | 需要（同源 `<img>` 自动带 Cookie） |

## 6. 移动端登录流程

`AuthGate` 包住整个移动壳：

1. 地址带 `?key=`（桌面顶部复制的链接）→ `POST /api/session` 自动登录 →
   `history.replaceState` 把 key 从地址栏移除（保留 hash 路由）→ 挂载壳。
2. 裸地址 → `GET /api/session`；已登录挂载壳，否则显示 16 位口令表单。
3. 任意视图收到 401 → 卸下壳、显示“口令已失效，请重新输入”。
4. 缩略图/封面是同一来源的 `<img>`，Cookie 自动携带；只有
   `externalStreamURL` 需要把 `currentAccessKey()` 追加为 `?key=`，因为原生播放器
   不共享浏览器 Cookie。
5. `GET /api/session` 在已认证时返回 key 本身，供第 4 步使用；未认证只返回 401。

## 7. 桌面端

- `server.StartInfo` 增加 `key`（只在运行中填充，且只经 Wails 绑定返回，不走 LAN）。
- 顶部 ServerBar 复制的是 `http://<lan-ip>:<port>/?key=<key>`，手机打开即登录。
- 设置 → 服务器新增「访问口令」：显示/隐藏、复制、更换口令（二次确认）。
  轮换后提示所有设备需要重新输入。
- Wails 绑定：`GetLanAccessKey`、`RotateLanAccessKey`（生成产物，不手改）。

## 8. 口令与设置存储

`backend/settings/lan_key.go` 只做三件事：读取（必要时生成）、轮换、规范化校验。
数据库键为 `lan_access_key`。没有单独的迁移：`settings` 表本来就存在。

## 9. 浏览器扩展豁免

`/api/extension/check` 是唯一不带口令的数据端点：

- 只读，不修改任何状态；
- `extensionHostAllowed` 只接受 loopback 与私网地址，挡住 DNS rebinding；
- CORS 只对 JavDB 域名与 localhost 回写，且不允许凭据；
- 测试锁死：带口令的服务器上，该端点仍返回 200（`backend/server/auth_test.go`）。

代价：扩展「打开详情」跳转到的 `/videos/{id}` 是 SPA 外壳，会显示登录页；同一
浏览器登录一次后 Cookie 长期有效。若将来要求扩展完全免登录，可给扩展选项页增加
口令字段并发送 Bearer，但这会扩大 CORS/请求头面，暂不做。

## 10. 测试

- `backend/accesskey`：生成长度/字符集/唯一性、规范化（大小写、连字符、非法字符、超长）。
- `backend/settings`：首次生成并持久化、轮换、损坏值修复、展示形式。
- `backend/server`：无口令时数据路由 401、外壳公开、登录握手与 Cookie 属性、
  Bearer、query 口令仅 `/stream/`、扩展豁免、失败限流、轮换使旧 Cookie 失效、
  跨站写 403、安全响应头。
- 前端：`externalStreamURL` 在登录后带 `?key=`；`npm run lint` / `build` /
  `build:mobile` / `test`。
- 手动：桌面启动 → 手机打开带口令链接自动登录；裸地址要求输入；错口令 401/429；
  重启后会话仍在；轮换后手机重新登录；Infuse/VLC 链接可播放；curl 无口令 401、
  Bearer 200；扩展查询仍可用。

## 11. 非目标与已知限制

- 不做 HTTPS、客户端证书、多用户、按设备撤销、二维码、口令找回。
- 口令是共享秘密：知道口令的任何设备都拥有完整读写权限。
- 明文 HTTP 传输；同网段的主动嗅探者仍可看到口令与视频内容。
- 口令在数据库中明文保存；能读本机数据目录的攻击者本来就能读整库。
- 轮换口令会让所有设备与已复制的播放链接一起失效，这是有意为之。

## 12. 文件索引

| 文件 | 作用 |
|---|---|
| `backend/accesskey/accesskey.go` | 生成与规范化 |
| `backend/settings/lan_key.go` | 持久化、首次生成、轮换 |
| `backend/server/auth.go` | 中间件、会话端点、限流、安全头 |
| `backend/server/server.go` | 路由接线、`StartInfo.Key`、启动 warning |
| `backend/server/auth_test.go` | 鉴权矩阵与回归 |
| `app.go` | `GetLanAccessKey` / `RotateLanAccessKey` / 启动前读取 |
| `frontend/src/mobile/AuthGate.tsx` | 登录闸门 |
| `frontend/src/services/http.ts` | 会话请求、401 通知、当前 key |
| `frontend/src/mobile/externalPlayers.ts` | 串流 URL 追加 `?key=` |
| `frontend/src/components/ServerBar.tsx` | 复制含口令地址 |
| `frontend/src/pages/Settings.tsx` | 查看/复制/轮换口令 |
