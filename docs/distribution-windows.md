# 打包分发给 Windows 用户

Muse 是 Wails 应用：一个 `Muse.exe` + 一个内嵌的前端产物，运行时只用 WebView2，
不需要安装运行时、VC++ 运行库或任何服务端。真正需要额外操心的只有一件事——
**ffmpeg / ffprobe 必须和 exe 放在同一目录**（`backend/platform/tools.go` 的查找顺序是
「exe 同目录 → PATH」）。所以对朋友的交付物不是一个裸 exe，而是一个小文件夹。

## 一条命令产出可发的包

```bash
scripts/package-windows.sh
```

前置条件（只装一次）：

```bash
brew install go node            # 已有则跳过
go install github.com/wailsapp/wails/v2/cmd/wails@v2.15.0
```

脚本做的事：

1. `wails build -platform windows/amd64 -clean -ldflags "-s -w"` 交叉编译出 `build/bin/Muse.exe`；
2. 首次运行从 [BtbN/FFmpeg-Builds](https://github.com/BtbN/FFmpeg-Builds) 下载 **LGPL** 版
   Windows FFmpeg 并缓存在 `.cache/ffmpeg-win64-shared/`（约 78 MB，之后复用）；
3. 组装 `dist/Muse-<版本>-win64/`，写入 CRLF 换行的 `README.txt` 和许可证文件，并把
   `browser-extension/` 一起复制进去（打包前会跑 `node scripts/check-extension.mjs`）；
4. 压成 `dist/Muse-<版本>-win64.zip`，打印 sha256。

在 Apple Silicon Mac 上交叉编译是**已验证可用**的：Wails v2.15 的 Windows 端
WebView2Loader 是纯 Go 实现，不需要 mingw-w64，也不需要 Windows 机器或虚拟机。
一次完整打包约 4 分钟（首次多一次 FFmpeg 下载）。

## 交付物长这样

```
Muse-1.0.0-win64/
├── Muse.exe               # 17 MB，应用本体
├── ffmpeg.exe             # 缩略图 / 封面选帧
├── ffprobe.exe            # 时长、分辨率、编码信息
├── avcodec-63.dll 等 7 个 DLL   # 仅 shared 版本需要
├── README.txt             # 给朋友看的首次运行说明（含 SmartScreen、防火墙）
├── FFmpeg-LICENSE.txt     # LGPL 许可证，再分发时必须保留
├── THIRD-PARTY-NOTICES.txt
└── browser-extension/     # JavDB 浏览器扩展（可选，Chrome「加载已解压的扩展程序」）
```

两种 FFmpeg 打包方式，按分发渠道挑一个：

| `MUSE_FFMPEG_FLAVOR` | 文件数 | 解压后 | zip | 适合 |
| --- | --- | --- | --- | --- |
| `shared`（默认） | 14 | 170 MB | **约 70 MB** | 微信 / 网盘发给朋友 |
| `static` | 6 | 290 MB | 约 130 MB | 追求「只有两个 exe」的极简文件夹 |

```bash
MUSE_FFMPEG_FLAVOR=static scripts/package-windows.sh
```

其它开关：

```bash
MUSE_SKIP_BUILD=1 scripts/package-windows.sh        # 复用已有 build/bin/Muse.exe，只重新打包
MUSE_NO_FFMPEG=1 scripts/package-windows.sh         # 不打包 FFmpeg（对方自己装）
MUSE_FFMPEG_ZIP=/path/to/ffmpeg-...-lgpl-shared.zip scripts/package-windows.sh  # 用本地 zip，不联网
```

## 版本号只有一个来源

`wails.json` 的 `info.productVersion` 决定 exe 的版本资源、安装包版本、文件夹名和 zip 名。
发新版本就改这一处，然后重跑脚本：

```json
"info": {
  "companyName": "Muse",
  "productName": "Muse",
  "productVersion": "1.1.0",
  "copyright": "Copyright © 2026 caohaoyangcn",
  "comments": "Local video library manager"
}
```

数据目录是 `%LOCALAPPDATA%\Muse\`，和程序文件夹无关。朋友升级时只要用新文件夹
覆盖旧的就行，收藏库会自动迁移（`backend/database/migration.go`）。

## GitHub Release 流水线

仓库带一条 GitHub Actions 流水线（`.github/workflows/release.yml`）：**推 tag 或手动触发都会打包并发布
Release**。CI 不重复实现打包逻辑，调的还是 `scripts/package-windows.sh`，产物和本机打的一模一样。

```bash
# 方式一：手动触发（Actions → Release (Windows x64) → Run workflow）
# 按 wails.json 的 info.productVersion 创建/更新 v<版本> Release；
# tag 不存在时自动创建在当前提交上。

# 方式二：先改 wails.json 的 info.productVersion（例如 1.0.1）并提交，再打 tag
git tag v1.0.1
git push origin v1.0.1
```

流水线（`windows-latest` + Git Bash，和本机同一套工具链）依次做：

1. 从 `go.mod` 读 Wails 版本并 `go install` 对应 CLI —— 版本不写死在 workflow 里；
2. 推 tag 时校验 tag 与 `wails.json` 的 `info.productVersion` 一致，不一致直接失败（版本号仍然只有一个来源）；
3. 前端构建 + `npm run lint`，再 `go test ./...`（根包 `//go:embed all:frontend/dist`，全新检出没有
   `dist/`，所以必须先构建前端），任何一步红了就不打包；
4. 跑 `scripts/package-windows.sh`，FFmpeg 走 Actions cache，命中时省掉 170 MB 下载；
5. zip 作为 workflow artifact 上传（保留 1 天），并用 `gh release create` 创建或更新 Release，附上 zip 与 sha256 说明。

细节：

- **手动触发**（Actions → Release (Windows x64) → Run workflow）会构建、测试、打包并发布/更新 `v<版本>`
  Release；适合首次发版、补发或同版本重跑。tag 已存在但指向别的提交时不会移动 tag。
- **同一 tag 重跑**会覆盖已上传的 zip 并更新 Release 说明，不会因为「Release 已存在」失败。
- **权限**：workflow 里声明了 `permissions: contents: write`；如果 Release 步骤报权限错误，
  去仓库 Settings → Actions → General → Workflow permissions 选「Read and write permissions」。
- 用的是 runner 自带的 `gh` CLI，没有引入第三方 release action。
- **artifact 保留 1 天**；Release 附件才是正式产物，不需要长期保留 Actions artifact。
- 想先看一遍再发布：给 `gh release create` 的两条分支都加上 `--draft`。

## 怎么把包发出去

- **微信 / QQ 直接传 zip**：别传 exe。聊天工具对 `.exe` 会拦截或自动改名，zip 不会。
  70 MB 仍在微信单文件的上限内。
- **网盘 / 蓝奏云 / 阿里云盘**：链接更稳定，适合发多人。
- **GitHub Release**：仓库是 public 时最省事——推一个 `v*` tag 或在 Actions 手动触发，流水线会把 zip 挂上去（见上一节）；也可以手动 Releases → Draft a new release → 上传 zip。`dist/` 已在 `.gitignore` 里，不要提交进仓库。
- 顺手把 sha256 一起发出去，对方可以用 `certutil -hashfile Muse-1.0.0-win64.zip SHA256`
  核对（也能证明「不是被人替换过的包」）。

## 朋友那边会遇到的三件事

这三个都已经写进 `packaging/README-Windows.txt`，会随包一起发出去：

1. **SmartScreen**：没买代码签名证书时，第一次运行会弹「Windows 已保护你的电脑」。
   点「更多信息 → 仍要运行」。要彻底消掉这个提示只能买证书（OV/EV 或 Azure Trusted
   Signing），个人分享没必要。
2. **杀软误报**：Go 编译的未签名 exe 常被启发式规则标黄。可以让朋友把文件夹加入信任。
3. **WebView2**：Win11 和较新的 Win10 自带；缺失时 Wails 会弹窗引导安装
   （`wails build` 默认 `-webview2 download`）。

局域网手机访问还要过一次防火墙：应用会用 `netsh` 自己加放行规则，但需要管理员权限。
所以 README 里写的是「右键 → 以管理员身份运行一次」。端口固定为 8899（可在「设置 → 服务器」中修改），以界面顶部显示的地址为准。

## 可选：做成安装包

`wails build -nsis` 能生成 NSIS 安装包，但 Wails 自带的模板（`build/windows/installer/project.nsi`）
只打包 `Muse.exe`，**不会带上 ffmpeg**——装完就是没有缩略图的版本。要做得改模板：

1. 先 `wails build -platform windows/amd64`，把 ffmpeg 的文件复制进 `build/bin/`；
2. 在 `project.nsi` 的 `!insertmacro wails.files` 之后补 `File` 行（shared 版要把 DLL 一起列上）；
3. 手动跑 `makensis`（需要 Windows 机器装 NSIS），传 `-DARG_WAILS_AMD64_BINARY=..\..\bin\Muse.exe`
   和 `INFO_PROJECTNAME/INFO_COMPANYNAME/INFO_PRODUCTNAME/INFO_PRODUCTVERSION` 等 define，
   否则 `wails_tools.nsh` 里会留着 `{{.Name}}` 占位符。

对几个朋友来说 ZIP 已经够用，安装包只是「开始菜单快捷方式 + 卸载项」，不建议先做这个。

## 排障

- **朋友双击没反应**：先看同目录是不是少了 `ffmpeg.exe` / DLL；日志在
  `%LOCALAPPDATA%\Muse\logs\app.log`。
- **缩略图一直是灰的**：九成是 ffmpeg 缺失或被单独挪出了文件夹。
- **打包脚本报 `wails CLI not found`**：`go install github.com/wailsapp/wails/v2/cmd/wails@v2.15.0`
  后把 `$(go env GOPATH)/bin` 加进 PATH（脚本也会去 `~/go/bin` 找）。
- **打包后 `git status` 有改动**：`wails build` 会重写 `frontend/wailsjs/`，macOS 上还会把文件
  权限从 644 改成 755。内容没变时 `git checkout -- frontend/wailsjs` 即可。
