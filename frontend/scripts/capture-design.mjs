// Captures the design preview into docs/design/screenshots/.
//
// Usage (from the repository root):
//   npm --prefix frontend run dev:design          # in another shell
//   PLAYWRIGHT_MODULE=/path/to/playwright node frontend/scripts/capture-design.mjs
//
// PLAYWRIGHT_MODULE exists because this repository does not depend on Playwright
// (the app has no e2e suite); point it at any installed copy, or install
// Playwright next to the project and omit the variable.
import { createRequire } from "node:module";
import { mkdir } from "node:fs/promises";
import path from "node:path";

const require = createRequire(import.meta.url);
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || "playwright");

const BASE =
  process.env.PREVIEW_URL || "http://localhost:5199/src/design/preview.html";
const OUT = process.env.SHOT_DIR || "docs/design/screenshots";
const VIEWPORT = { width: 1440, height: 900 };

const TAKEN = [];
const FAILED = [];

async function clickText(page, text) {
  const byRole = page.getByText(text, { exact: true }).first();
  await byRole.click({ timeout: 4000 });
}

async function settle(page) {
  await page.waitForTimeout(450);
}

async function shot(page, name, options = {}) {
  try {
    await settle(page);
    // Screens ship as JPEG (2× of a 1440-wide window is a lot of PNG), the
    // design sheet stays PNG because it is read for token values.
    const png = options.png === true;
    const file = path.join(OUT, `${name}.${png ? "png" : "jpg"}`);
    await page.screenshot({
      path: file,
      animations: "disabled",
      type: png ? "png" : "jpeg",
      ...(png ? {} : { quality: 92 }),
    });
    TAKEN.push(name);
    console.log(`✓ ${name}`);
  } catch (error) {
    FAILED.push(`${name}: ${error.message}`);
    console.error(`✗ ${name} — ${error.message}`);
  }
}

/** Fresh page per screen: every capture starts from the same state. */
async function fresh(browser) {
  const page = await browser.newPage({
    viewport: VIEWPORT,
    deviceScaleFactor: 2,
    colorScheme: "dark",
    locale: "zh-CN",
    timezoneId: "Asia/Shanghai",
  });
  page.on("console", (message) => {
    if (message.type() === "warning" || message.type() === "error") {
      console.log(`  console.${message.type()}: ${message.text()}`);
    }
  });
  page.on("pageerror", (error) => console.error(`  pageerror: ${error.message}`));
  await page.goto(BASE, { waitUntil: "networkidle" });
  await page.waitForSelector("[data-video-id]", { timeout: 15000 });
  await page.evaluate(() => document.fonts.ready);
  await settle(page);
  return page;
}

/** The phone shell: same mock library, 390×844, device-pixel ratio 3. */
async function captureMobile(browser) {
  const url = BASE.replace("preview.html", "preview-mobile.html");

  async function open() {
    const page = await browser.newPage({
      viewport: { width: 390, height: 844 },
      deviceScaleFactor: 3,
      colorScheme: "dark",
      locale: "zh-CN",
      timezoneId: "Asia/Shanghai",
    });
    page.on("pageerror", (error) => console.error(`  mobile pageerror: ${error.message}`));
    await page.goto(url, { waitUntil: "networkidle" });
    await page.evaluate(() => document.fonts.ready);
    await page.waitForTimeout(900);
    return page;
  }

  /** Every destination starts from a fresh page: the drawer is a modal layer,
   *  and driving it twice in one session fights the overlay. */
  async function nav(label, name) {
    const page = await open();
    try {
      await page.getByRole("button", { name: "打开导航" }).click({ timeout: 4000 });
      await settle(page);
      if (name === "22-mobile-navigation") {
        await shot(page, name);
      } else {
        await page
          .getByRole("dialog")
          .getByRole("button", { name: label })
          .first()
          .click({ timeout: 4000 });
        await settle(page);
        await shot(page, name);
      }
    } catch (error) {
      FAILED.push(`${name}: ${error.message}`);
    }
    await page.close();
  }

  {
    const page = await open();
    await shot(page, "21-mobile-library");
    try {
      await page.getByText("【MUSE-101】", { exact: false }).first().click({ timeout: 4000 });
      await settle(page);
      await shot(page, "23-mobile-detail");
    } catch (error) {
      FAILED.push(`23-mobile-detail: ${error.message}`);
    }
    await page.close();
  }

  await nav("", "22-mobile-navigation");
  await nav("演员", "24-mobile-actors");
  await nav("归档与回收站", "25-mobile-trash");
}

async function main() {
  await mkdir(OUT, { recursive: true });
  const browser = await chromium.launch();

  // 1. Library, grid (the default view).
  {
    const page = await fresh(browser);
    await shot(page, "01-library-grid");

    // 2. Library, list rows.
    try {
      await page.getByRole("radio", { name: "列表" }).click({ timeout: 3000 });
      await settle(page);
      await shot(page, "02-library-list");
    } catch (error) {
      FAILED.push(`02-library-list: ${error.message}`);
    }

    // 3. Context menu on a card (the grid is still on screen here).
    try {
      await page.getByRole("radio", { name: "网格" }).click({ timeout: 3000 });
      await page.locator("[data-video-id]").nth(2).click({ button: "right" });
      await settle(page);
      await shot(page, "04-card-context-menu");
      await page.keyboard.press("Escape");
    } catch (error) {
      FAILED.push(`04-card-context-menu: ${error.message}`);
    }

    // 4. Detail panel: a card click turns the workspace into the video tab.
    try {
      await page.locator("[data-video-id]").first().click();
      await page.locator("h2").first().waitFor({ timeout: 5000 });
      await settle(page);
      await shot(page, "03-library-detail");
    } catch (error) {
      FAILED.push(`03-library-detail: ${error.message}`);
    }

    // 5. Multi-select state.
    try {
      await page.goto(BASE, { waitUntil: "networkidle" });
      await page.waitForSelector("[data-video-id]");
      const boxes = page.locator('[data-video-id] input[type="checkbox"]');
      await boxes.nth(0).click({ force: true });
      await boxes.nth(1).click({ force: true });
      await boxes.nth(1).hover();
      await settle(page);
      await shot(page, "05-library-multi-select");
    } catch (error) {
      FAILED.push(`05-library-multi-select: ${error.message}`);
    }

    await page.close();
  }

  // 6. Filter dialog.
  {
    const page = await fresh(browser);
    try {
      await clickText(page, "筛选");
      await page.getByRole("dialog").first().waitFor({ timeout: 5000 });
      await shot(page, "06-filter-dialog");
    } catch (error) {
      FAILED.push(`06-filter-dialog: ${error.message}`);
    }
    await page.close();
  }

  // 7. Edit dialog, reached the way a user reaches it.
  {
    const page = await fresh(browser);
    try {
      await page.locator("[data-video-id]").first().click({ button: "right" });
      await settle(page);
      await clickText(page, "编辑信息…");
      await page.getByRole("dialog").first().waitFor({ timeout: 5000 });
      await shot(page, "07-edit-dialog");
    } catch (error) {
      FAILED.push(`07-edit-dialog: ${error.message}`);
    }
    await page.close();
  }

  // 8. Sidebar destinations.
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
        await clickText(page, label);
        // A step without a name only resets the view for the next capture.
        if (name) await shot(page, name);
      } catch (error) {
        FAILED.push(`${name ?? label}: ${error.message}`);
      }
    }
    await page.close();
  }

  // 9. Actor films, opened from the actor index.
  {
    const page = await fresh(browser);
    try {
      await clickText(page, "演员");
      await settle(page);
      await page.getByRole("button", { name: /水野あかり/ }).first().click({ timeout: 5000 });
      await shot(page, "14-actor-films");
    } catch (error) {
      FAILED.push(`14-actor-films: ${error.message}`);
    }
    await page.close();
  }

  // 10. Settings sections.
  {
    const page = await fresh(browser);
    try {
      await page.getByRole("button", { name: "打开设置" }).click({ timeout: 4000 });
      await page.getByRole("navigation", { name: "设置分类" }).waitFor();
      await shot(page, "15-settings-server");
      for (const [label, name] of [
        ["网盘（可选）", "16-settings-cloud"],
        ["片名翻译", "17-settings-llm"],
        ["新手引导", "18-settings-guide"],
        ["目录监控", "19-settings-watch"],
      ]) {
        try {
          await clickText(page, label);
          await shot(page, name);
        } catch (error) {
          FAILED.push(`${name}: ${error.message}`);
        }
      }
    } catch (error) {
      FAILED.push(`15-settings-server: ${error.message}`);
    }
    await page.close();
  }

  await captureMobile(browser);

  await browser.close();

  console.log(`\n${TAKEN.length} captured → ${OUT}`);
  if (FAILED.length > 0) {
    console.log(`\n${FAILED.length} failed:`);
    for (const line of FAILED) console.log(` - ${line}`);
    process.exitCode = 1;
  }
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
