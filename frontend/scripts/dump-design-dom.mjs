// Dumps the rendered DOM of every design-preview screen as geometry + styles.
//
// It replays exactly the same interaction script as `capture-design.mjs`, but
// records `getBoundingClientRect()` + `getComputedStyle()` + text + images +
// inline SVG for every painted node instead of taking a screenshot.
//
// The output is what the Penpot baseline is built from: the design file is a
// *mirror* of the running app, so it must be regenerated from the DOM rather
// than redrawn by hand (see docs/design/penpot-baseline.md).
//
// Usage (from the repository root):
//   npm --prefix frontend run dev:design              # in another shell
//   PLAYWRIGHT_MODULE=/path/to/playwright node frontend/scripts/dump-design-dom.mjs
//
// PLAYWRIGHT_MODULE exists because this repository does not depend on Playwright
// (the app has no e2e suite); point it at any installed copy, or install
// Playwright next to the project and omit the variable.
import { createRequire } from "node:module";
import { mkdir, writeFile } from "node:fs/promises";
import path from "node:path";

const require = createRequire(import.meta.url);
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || "playwright");

const BASE =
  process.env.PREVIEW_URL || "http://localhost:5199/src/design/preview.html";
const OUT = process.env.DOM_DIR || ".design-dom";
const VIEWPORT = { width: 1440, height: 900 };

const DONE = [];
const FAILED = [];

/**
 * Runs in the page. Returns every node that actually paints something:
 * a background, a border, text, an image or an SVG icon. Wrapper divs are
 * skipped — they carry no pixels, and the layout they express is already
 * encoded in the children's absolute rects.
 */
function extract() {
  const px = (value) => Math.round(value * 10) / 10;
  const rgba = (color) => {
    const match = color && color.match(/rgba?\(([^)]+)\)/);
    if (!match) return null;
    const parts = match[1].split(",").map((part) => parseFloat(part.trim()));
    const hex =
      "#" +
      [0, 1, 2]
        .map((i) => parts[i].toString(16).padStart(2, "0"))
        .join("")
        .toUpperCase();
    return { hex, a: parts.length > 3 ? parts[3] : 1 };
  };

  const images = [];
  const imageIndex = new Map();
  const nodes = [];

  const walk = (el, depth) => {
    if (nodes.length > 1400 || depth > 50) return;
    const tag = el.tagName.toLowerCase();
    if (tag === "script" || tag === "style" || tag === "noscript") return;

    const style = getComputedStyle(el);
    if (style.display !== "none" && style.visibility !== "hidden") {
      const rect = el.getBoundingClientRect();
      const visible =
        rect.width >= 4 &&
        rect.height >= 4 &&
        rect.right > -200 &&
        rect.left < innerWidth + 200 &&
        rect.bottom > -400 &&
        rect.top < innerHeight + 500;

      if (visible) {
        const background = rgba(style.backgroundColor);
        const borderWidth = parseFloat(style.borderTopWidth) || 0;
        const borderColor = borderWidth > 0 ? rgba(style.borderTopColor) : null;

        // Own text nodes only: the meta line of a card sits next to a like
        // button inside the same div, so "leaf element" alone misses it.
        const ownText = Array.from(el.childNodes)
          .filter((node) => node.nodeType === 3)
          .map((node) => node.textContent.trim())
          .filter(Boolean)
          .join(" ");
        let text =
          ownText ||
          (el.childElementCount === 0 ? (el.textContent || "").trim() : "");
        if (!text && tag === "input" && el.placeholder) text = el.placeholder;

        // CSS 用 `text-overflow: ellipsis` 把长文本截断，Penpot 没有等价能力。
        // 这里按浏览器实际可容纳的宽度把字符串裁成带省略号的版本，导入后就不必
        // 依赖设计工具去猜截断位置。
        if (text && el.scrollWidth > el.clientWidth + 1 && el.clientWidth > 0) {
          const limit = el.clientWidth;
          const fits = (candidate) => {
            const probe = el.cloneNode(false);
            probe.textContent = candidate;
            probe.style.position = "absolute";
            probe.style.visibility = "hidden";
            probe.style.whiteSpace = "nowrap";
            probe.style.width = "auto";
            document.body.appendChild(probe);
            const width = probe.getBoundingClientRect().width;
            probe.remove();
            return width <= limit;
          };
          if (!fits(text)) {
            let low = 0;
            let high = text.length;
            while (low < high) {
              const mid = Math.ceil((low + high) / 2);
              if (fits(text.slice(0, mid) + "…")) low = mid;
              else high = mid - 1;
            }
            text = text.slice(0, low).trimEnd() + "…";
          }
        }

        const src = tag === "img" ? el.currentSrc || el.src : null;
        const hasPaint =
          (background && background.a > 0.02) ||
          borderColor ||
          tag === "svg" ||
          text ||
          src;

        // 透明但承担布局的 flex 包裹层也必须输出：层级和方向信息全在它们身上。
        // 只导出"画得出像素"的节点，重建出来的就只剩一堆绝对坐标，
        // 也就是为什么第一版设计稿改不动。
        // 只保留"有意义"的包裹层：包住多个子元素，或自己带 padding/gap。
        // 单子节点、零内边距的 pass-through 层折掉，否则一份设计稿会被
        // Tailwind 的包装 div 灌到上万图层。
        const numPad = (v) => Math.abs(parseFloat(v) || 0);
        const inset =
          numPad(style.paddingTop) + numPad(style.paddingRight) +
          numPad(style.paddingBottom) + numPad(style.paddingLeft);
        const spacing = numPad(style.rowGap) + numPad(style.columnGap);
        const isGrid = style.display === "grid" || style.display === "inline-grid";
        // 普通 block 容器（不是 flex 也不是 grid）同样是布局的一部分：它的
        // padding 决定子元素的起点，跳过它会让子元素挂到隔壁兄弟节点下面。
        // 导入时按"纵向流"还原成 column flex。
        const isLayoutWrapper =
          !hasPaint &&
          el.childElementCount > 0 &&
          (el.childElementCount > 1 || true) &&
          el.childElementCount > 0 &&
          (el.childElementCount > 1 || inset > 0 || spacing > 0);

        if (hasPaint || isLayoutWrapper) {
          const node = {
            // 深度用于把预序输出还原成父子树；没有它只能拿到一堆扁平矩形。
            d: depth,
            t: tag,
            x: px(rect.x),
            y: px(rect.y),
            w: px(rect.width),
            h: px(rect.height),
            bg: background && background.a > 0.02 ? background.hex : null,
            ba: background ? background.a : 1,
            bc: borderColor ? borderColor.hex : null,
            bw: borderWidth || 0,
            rad: parseFloat(style.borderTopLeftRadius) || 0,
            fs: parseFloat(style.fontSize) || 0,
            fw: parseInt(style.fontWeight, 10) || 400,
            color: rgba(style.color) ? rgba(style.color).hex : null,
            txt: text ? text.slice(0, 140) : null,
            op: parseFloat(style.opacity),
            ...(hasPaint ? {} : { structural: true }),
          };
          // 布局属性：视觉几何说明"现在长什么样"，这些说明"为什么长这样"。
          // 有了它们才能把 DOM 的 flex 容器还原成设计工具里的 auto-layout，
          // 而不是把每个子元素钉死在坐标上（钉死之后改版=重排一遍）。
          const display = style.display;
          const numOf = (v) => Math.round((parseFloat(v) || 0) * 10) / 10;
          if (display === "grid" || display === "inline-grid") {
            // 设计工具普遍没有 grid，导入时按"列容器 + 每个网格行一条 flex row"
            // 还原；子元素的数量变化仍会驱动重排。
            node.grid = true;
            node.layout = {
              dir: "grid",
              wrap: false,
              row: numOf(style.rowGap),
              col: numOf(style.columnGap),
              pad: [
                numOf(style.paddingTop),
                numOf(style.paddingRight),
                numOf(style.paddingBottom),
                numOf(style.paddingLeft),
              ],
              ai: style.alignItems,
              jc: style.justifyContent,
            };
          }
          const isLeafMedia = tag === "svg" || tag === "img";
          if (!isLeafMedia && (display === "block" || display === "flow-root" || display === "list-item")) {
            if (el.childElementCount > 0) {
              node.layout = {
                dir: "block",
                wrap: false,
                row: 0,
                col: 0,
                pad: [
                  numOf(style.paddingTop),
                  numOf(style.paddingRight),
                  numOf(style.paddingBottom),
                  numOf(style.paddingLeft),
                ],
                ai: "stretch",
                jc: "start",
              };
            }
          }
          if (display === "flex" || display === "inline-flex") {
            const num = (v) => Math.round((parseFloat(v) || 0) * 10) / 10;
            node.layout = {
              dir: style.flexDirection === "column" ? "column" : "row",
              wrap: style.flexWrap === "wrap" || style.flexWrap === "wrap-reverse",
              row: num(style.rowGap),
              col: num(style.columnGap),
              pad: [
                num(style.paddingTop),
                num(style.paddingRight),
                num(style.paddingBottom),
                num(style.paddingLeft),
              ],
              ai: style.alignItems,
              jc: style.justifyContent,
            };
          }
          // 父容器是 flex 时，子项怎么分宽度：0 = 不抢，1 = 吃掉剩余空间。
          const parent = el.parentElement;
          if (parent && /flex/.test(getComputedStyle(parent).display)) {
            node.flexGrow = parseFloat(style.flexGrow) || 0;
            node.flexShrink = parseFloat(style.flexShrink) || 0;
            node.flexBasis = style.flexBasis === "auto" ? null : Math.round(parseFloat(style.flexBasis) || 0);
          }

          if (tag === "svg" && rect.width >= 8) {
            node.svg = el.outerHTML.slice(0, 2600);
          }
          if (src) {
            if (!imageIndex.has(src)) {
              imageIndex.set(src, images.length);
              images.push(src);
            }
            node.img = imageIndex.get(src);
          }
          nodes.push(node);
        }
      }
    }

    for (const child of el.children) walk(child, depth + 1);
  };

  walk(document.body, 0);
  return { nodes, images };
}

async function settle(page) {
  await page.waitForTimeout(500);
}

async function dump(page, name) {
  try {
    await settle(page);
    const data = await page.evaluate(extract);
    const images = [];
    for (const src of data.images) {
      try {
        const response = await page.request.get(src);
        const buffer = await response.body();
        const head = buffer.slice(0, 200).toString("utf8").trimStart();
        images.push({
          b64: buffer.toString("base64"),
          type: head.startsWith("<svg") ? "svg" : "png",
        });
      } catch {
        images.push(null);
      }
    }
    await writeFile(
      path.join(OUT, `${name}.json`),
      JSON.stringify({
        w: page.viewportSize().width,
        h: page.viewportSize().height,
        nodes: data.nodes,
        imgs: images,
      }),
    );
    DONE.push(name);
    console.log(`✓ ${name} (${data.nodes.length} nodes, ${images.length} images)`);
  } catch (error) {
    FAILED.push(`${name}: ${error.message}`);
    console.error(`✗ ${name} — ${error.message}`);
  }
}

/** Fresh page per screen: every dump starts from the same state. */
async function fresh(browser) {
  const page = await browser.newPage({
    viewport: VIEWPORT,
    deviceScaleFactor: 1,
    colorScheme: "dark",
    locale: "zh-CN",
    timezoneId: "Asia/Shanghai",
  });
  page.on("pageerror", (error) => console.error(`  pageerror: ${error.message}`));
  await page.goto(BASE, { waitUntil: "domcontentloaded" });
  await page.waitForSelector("[data-video-id]", { timeout: 25000 });
  await page.evaluate(() => document.fonts.ready);
  await settle(page);
  return page;
}

async function openMobile(browser) {
  const page = await browser.newPage({
    viewport: { width: 390, height: 844 },
    deviceScaleFactor: 1,
    colorScheme: "dark",
    locale: "zh-CN",
    timezoneId: "Asia/Shanghai",
  });
  page.on("pageerror", (error) => console.error(`  mobile pageerror: ${error.message}`));
  await page.goto(BASE.replace("preview.html", "preview-mobile.html"), {
    waitUntil: "domcontentloaded",
  });
  await page.evaluate(() => document.fonts.ready);
  await page.waitForTimeout(1400);
  return page;
}

async function main() {
  await mkdir(OUT, { recursive: true });
  const browser = await chromium.launch();

  // 01 grid, 02 list, 04 context menu, 03 detail, 05 multi-select.
  {
    const page = await fresh(browser);
    await dump(page, "01-library-grid");
    try {
      await page.getByRole("radio", { name: "列表" }).click({ timeout: 5000 });
      await dump(page, "02-library-list");
    } catch (error) {
      FAILED.push(`02-library-list: ${error.message}`);
    }
    try {
      await page.getByRole("radio", { name: "网格" }).click({ timeout: 5000 });
      await settle(page);
      await page.locator("[data-video-id]").nth(2).click({ button: "right" });
      await dump(page, "04-card-context-menu");
      await page.keyboard.press("Escape");
    } catch (error) {
      FAILED.push(`04-card-context-menu: ${error.message}`);
    }
    try {
      await page.locator("[data-video-id]").first().click();
      await page.locator("h2").first().waitFor({ timeout: 8000 });
      await dump(page, "03-library-detail");
    } catch (error) {
      FAILED.push(`03-library-detail: ${error.message}`);
    }
    try {
      await page.goto(BASE, { waitUntil: "domcontentloaded" });
      await page.waitForSelector("[data-video-id]");
      await settle(page);
      const boxes = page.locator('[data-video-id] input[type="checkbox"]');
      await boxes.nth(0).click({ force: true });
      await boxes.nth(1).click({ force: true });
      await boxes.nth(1).hover();
      await dump(page, "05-library-multi-select");
    } catch (error) {
      FAILED.push(`05-library-multi-select: ${error.message}`);
    }
    await page.close();
  }

  // 06 filter dialog.
  {
    const page = await fresh(browser);
    try {
      await page.getByText("筛选", { exact: true }).first().click({ timeout: 5000 });
      await page.getByRole("dialog").first().waitFor({ timeout: 5000 });
      await dump(page, "06-filter-dialog");
    } catch (error) {
      FAILED.push(`06-filter-dialog: ${error.message}`);
    }
    await page.close();
  }

  // 07 edit dialog, reached the way a user reaches it.
  {
    const page = await fresh(browser);
    try {
      await page.locator("[data-video-id]").first().click({ button: "right" });
      await settle(page);
      await page.getByText("编辑信息…", { exact: true }).first().click({ timeout: 5000 });
      await page.getByRole("dialog").first().waitFor({ timeout: 5000 });
      await dump(page, "07-edit-dialog");
    } catch (error) {
      FAILED.push(`07-edit-dialog: ${error.message}`);
    }
    await page.close();
  }

  // 08..13 and 26: sidebar destinations.
  {
    const page = await fresh(browser);
    const steps = [
      ["最近播放", "08-recently-played"],
      ["演员", "09-actors"],
      ["刮削任务", "10-scrape-tasks"],
      ["后台任务", "11-background-tasks"],
      ["查重", "12-duplicates"],
      ["归档与回收站", "13-trash"],
      ["全部收藏", null],
      ["剧情", "26-library-tag"],
    ];
    for (const [label, name] of steps) {
      try {
        await page.getByText(label, { exact: true }).first().click({ timeout: 6000 });
        if (name) await dump(page, name);
      } catch (error) {
        FAILED.push(`${name ?? label}: ${error.message}`);
      }
    }
    await page.close();
  }

  // 14 actor films.
  {
    const page = await fresh(browser);
    try {
      await page.getByText("演员", { exact: true }).first().click({ timeout: 6000 });
      await settle(page);
      await page.getByRole("button", { name: /水野あかり/ }).first().click({ timeout: 8000 });
      await settle(page);
      await dump(page, "14-actor-films");
    } catch (error) {
      FAILED.push(`14-actor-films: ${error.message}`);
    }
    await page.close();
  }

  // 15..19 settings sections.
  {
    const page = await fresh(browser);
    try {
      await page.getByRole("button", { name: "打开设置" }).click({ timeout: 6000 });
      await page.getByRole("navigation", { name: "设置分类" }).waitFor({ timeout: 8000 });
      await dump(page, "15-settings-server");
      for (const [label, name] of [
        ["网盘（可选）", "16-settings-cloud"],
        ["片名翻译", "17-settings-llm"],
        ["新手引导", "18-settings-guide"],
        ["目录监控", "19-settings-watch"],
      ]) {
        try {
          await page.getByText(label, { exact: true }).first().click({ timeout: 6000 });
          await dump(page, name);
        } catch (error) {
          FAILED.push(`${name}: ${error.message}`);
        }
      }
    } catch (error) {
      FAILED.push(`15-settings-server: ${error.message}`);
    }
    await page.close();
  }

  // 21..25 mobile shell.
  {
    const page = await openMobile(browser);
    await dump(page, "21-mobile-library");
    try {
      await page.getByText("【MUSE-101】", { exact: false }).first().click({ timeout: 6000 });
      await dump(page, "23-mobile-detail");
    } catch (error) {
      FAILED.push(`23-mobile-detail: ${error.message}`);
    }
    await page.close();

    for (const [label, name] of [
      ["", "22-mobile-navigation"],
      ["演员", "24-mobile-actors"],
      ["归档与回收站", "25-mobile-trash"],
    ]) {
      const mobile = await openMobile(browser);
      try {
        await mobile.getByRole("button", { name: "打开导航" }).click({ timeout: 6000 });
        await settle(mobile);
        if (name === "22-mobile-navigation") {
          await dump(mobile, name);
        } else {
          await mobile
            .getByRole("dialog")
            .getByRole("button", { name: label })
            .first()
            .click({ timeout: 6000 });
          await settle(mobile);
          await dump(mobile, name);
        }
      } catch (error) {
        FAILED.push(`${name}: ${error.message}`);
      }
      await mobile.close();
    }
  }

  await browser.close();
  console.log(`\n${DONE.length} screens → ${OUT}`);
  if (FAILED.length > 0) {
    console.log(`\n${FAILED.length} failed:\n - ${FAILED.join("\n - ")}`);
    process.exitCode = 1;
  }
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
