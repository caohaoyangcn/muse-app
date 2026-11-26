# 字体替换：Sarasa UI SC

## 目标

把产品字体从当前的 `"Segoe UI", system-ui, -apple-system, sans-serif` 换成
**Sarasa UI SC**（[更纱黑体](https://github.com/be5invis/sarasa-gothic)，SIL OFL 1.1）。

动机：现在三处渲染各不相同 —— Windows 生产用 Segoe UI + 微软雅黑，macOS 开发回退到 SF + 苹方，
Penpot 设计稿用默认字体。改成一个自带 CJK 与拉丁的字体后，**设计稿、开发机、生产环境三者同字**，
"稿 = 实现"才有像素级意义。

## 选型依据

README 原文：`a CJK composite font based on Inter, Iosevka and Source Han Sans`。

| 变体家族 | 拉丁 | 用途 |
|---|---|---|
| **Sarasa Gothic / UI** | **Inter** | 界面、正文 ← 选这个 |
| Sarasa Mono / Term / Fixed | Iosevka | 代码、终端 |

`Gothic` 与 `UI` 的差别只有引号：Gothic 的 `“”` 是全角，UI 是窄引号。界面里全角引号过宽，
**取 `Sarasa UI SC`**（SC = 简体区域字形）。

## 影响面

- `frontend/src/theme.css` 的字体栈
- 应用需打包 3 个字重的 WOFF2：**24.3 MB**（Regular 8.0 / SemiBold 8.1 / Bold 8.2）
- Penpot 团队字体库需上传 WOFF（12 MB × 3，团队级资源，只能人工上传）
- 全部 25 屏截图与 Penpot 基线需要重导一次

## 实测字宽变化（1440×900，Playwright 量 `offsetWidth`）

| 样本 | 当前字体 | Sarasa UI SC | 差异 |
|---|---|---|---|
| `【MUSE-101】盛夏海边，与你` 15px | 206.2 px | 208.1 px | +0.9 % |
| `2:28:00 · 1920×1080 · 6.4 GB` 12px | 167.4 px | 169.1 px | +1.0 % |
| `归档与回收站` 13px | 78.0 px | 78.0 px | 0 |
| `局域网访问` 13px | 65.0 px | 65.0 px | 0 |
| `添加时间` 12px | 48.0 px | 48.0 px | 0 |

结论：**纯中文宽度完全不变**（CJK 在固定字号下等宽），只有中英混排的行因为 Inter 的数字/字母
略宽 1 %。**布局风险很低**，不需要改任何尺寸。

## 待决

1. **24.3 MB 体积**是否可以接受（分发包本来就带 FFmpeg，判断上不算大）。
2. **字体栈首位**：建议 `"Sarasa UI SC", "Segoe UI", …` —— 打包字体优先，避免 Windows 上有
   Segoe UI 的用户看到另一套字形，导致设计稿与生产再次分叉。
3. 附带 OFL 许可文件（`frontend/public/fonts/OFL.txt`），OFL 要求随字体分发。

## 对照图

- `01-before.jpg` —— 当前字体
- `02-after.jpg` —— 注入 Sarasa UI SC 后（同一屏、同一状态、同一视口）

## 评审结论

- 日期：2026-11-26
- 结论：**通过**。维护者确认：不再使用 Segoe UI；接受 24.3 MB 的字体包体积。
- 实现：字体已打进 `src/assets/fonts/`（Vite 资源管线，构建后落在 `dist/assets/*.woff2`），
  `theme.css` 定义三个 `@font-face` 并把 `--font-family-body/heading` 指向 `Sarasa UI SC`，
  桌面 `style.css`、移动 `mobile.css` 同步；`capture-design.mjs` 与 `dump-design-dom.mjs`
  增加 `document.fonts.ready` 等待，避免抓到回退字形。
- 效果：01 屏与真实截图的像素差 平均 3.32 → **3.03**，≤24/255 由 96.9% → **97.3%**，
  大偏差（>48）由 2.0% → **1.7%**。
- Penpot 基线 25 屏全部重导完成，文本层显式 `fontFamily: "Sarasa UI SC"`，无残留默认字体。
- 导入器这次补了两条规则（详见 `docs/design/penpot-baseline.md`）：封面按 `object-fit` 语义
  裁剪（竖版露右半，横版等比填充）；长文本按浏览器实测宽度截断成 `…`，因为 Penpot 没有
  CSS 的 `text-overflow: ellipsis`。
- 已知残留：移动端像素差（01 桌面 mean 3.03 / ≤24 97.3%；21 移动 mean 9.27 / ≤24 80.3%）
  偏高的主因是 3× 截图画布压到 1× 对比时的重采样差异，不是布局错位。
