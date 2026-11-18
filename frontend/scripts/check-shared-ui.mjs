#!/usr/bin/env node
// Guards the directory rules in docs/spec/cross-platform-ui-plan.md §3.
//
// This is a grep-shaped check on purpose: the rules are about *which directory
// may import which*, and a violation is cheapest to catch here rather than in a
// review where a `../components/Foo` import looks perfectly ordinary.
//
// Run from frontend/: `npm run check:shared`
import { existsSync, readFileSync, readdirSync, statSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";

const ROOT = resolve(import.meta.dirname, "..");
const SRC = join(ROOT, "src");

/** Every .ts/.tsx under src/, with its repo-relative path. */
function walk(dir) {
  const out = [];
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) out.push(...walk(path));
    else if (/\.tsx?$/.test(entry)) out.push(path);
  }
  return out;
}

const files = walk(SRC).map((path) => ({
  path,
  rel: relative(ROOT, path).replaceAll("\\", "/"),
  source: readFileSync(path, "utf8"),
}));

/** Import specifiers of a file, relative or not. */
function importsOf(source) {
  return [...source.matchAll(/from\s+["']([^"']+)["']|import\s*\(\s*["']([^"']+)["']/g)]
    .map((m) => m[1] ?? m[2])
    .filter(Boolean);
}

/** Does a resolved path point at this module (with or without the extension)? */
function isModule(target, module) {
  return target === `${SRC}/${module}` || target === `${SRC}/${module.replace(/\.tsx?$/, "")}`;
}

/** Absolute path a relative specifier points at, or null for a bare package. */
function resolveSpec(file, spec) {
  if (!spec.startsWith(".")) return null;
  return resolve(dirname(file.path), spec).replaceAll("\\", "/");
}

const problems = [];
const notes = [];

/**
 * Fails when a file under `inDir` imports something that resolves into one of
 * the `forbidden` directories. Resolution (not string matching) is what makes
 * `./components/Foo` legal inside mobile/ while `../components/Foo` is not.
 */
function forbid({ rule, inDir, forbidden }) {
  const targets = forbidden.map((dir) => `${SRC}/${dir}/`);
  for (const file of files) {
    if (!file.rel.startsWith(`src/${inDir}/`)) continue;
    for (const spec of importsOf(file.source)) {
      const target = resolveSpec(file, spec);
      if (target && targets.some((dir) => target.startsWith(dir))) {
        problems.push(`${file.rel}: ${rule} (imports "${spec}")`);
      }
    }
  }
}

// L4 原子层只能依赖 React 与 lib/utils：它要能同时出现在两个 bundle 里。
forbid({
  rule: "ui/ 不得依赖业务或平台模块",
  inDir: "ui",
  forbidden: ["services", "views", "components", "mobile", "hooks"],
});

// 移动端不得 import 桌面组件：需要共享就下沉到 ui/ 或 views/。
forbid({ rule: "mobile/ 不得 import 桌面组件", inDir: "mobile", forbidden: ["components"] });

// 设计系统只经 ui/ 暴露。裸包名不走 resolveSpec（相对路径才有目标），所以单独查
// 一次：业务层直接 import @astryxdesign，ui/ 这道接缝就名存实亡了。
for (const file of files) {
  if (file.rel.startsWith("src/ui/")) continue;
  for (const spec of importsOf(file.source)) {
    if (spec.startsWith("@astryxdesign/")) {
      problems.push(`${file.rel}: 不得直接 import 设计系统（改为经 src/ui/ 暴露）`);
    }
  }
}

// 视图模型不得关心渲染层。
forbid({ rule: "views/ 不得 import 渲染层", inDir: "views", forbidden: ["components", "mobile"] });

// 唯一入口：壳只经门面取数。直接 import 传输模块的，只能是 services/ 内部
// （两个 adapter 与两个门面本身）。
for (const file of files) {
  if (file.rel.startsWith("src/services/")) continue;
  for (const spec of importsOf(file.source)) {
    const target = resolveSpec(file, spec);
    if (isModule(target, "services/api.ts") || isModule(target, "services/http.ts")) {
      problems.push(
        `${file.rel}: 不得直接 import 传输模块（改用 services/desktop.ts 或 services/web.ts）`,
      );
    }
  }
}

// 门面不能拿错：移动端不该看到桌面专属能力，桌面端也不该引 Web 专有实现。
for (const file of files) {
  const mobile = file.rel.startsWith("src/mobile/");
  const desktopShell = /^src\/(components|pages)\//.test(file.rel);
  if (!mobile && !desktopShell) continue;
  for (const spec of importsOf(file.source)) {
    const target = resolveSpec(file, spec);
    if (mobile && isModule(target, "services/desktop.ts")) {
      problems.push(`${file.rel}: 移动端不得 import 桌面门面 services/desktop.ts`);
    }
    if (desktopShell && isModule(target, "services/web.ts")) {
      problems.push(`${file.rel}: 桌面端不得 import 移动门面 services/web.ts`);
    }
  }
}

// 传输层隔离：两个入口各引各的 adapter，混了就会把 Wails 打进 Web bundle。
const entries = {
  "src/main.tsx": { mustNot: ["client.http", "services/http"], must: ["client.wails"] },
  "src/mobile/main.tsx": { mustNot: ["client.wails", "services/api"], must: ["client.http"] },
};
for (const [rel, rule] of Object.entries(entries)) {
  const file = files.find((f) => f.rel === rel);
  if (!file) {
    problems.push(`${rel}: 入口文件不存在`);
    continue;
  }
  for (const bad of rule.mustNot) {
    if (importsOf(file.source).some((spec) => spec.includes(bad))) {
      problems.push(`${rel}: 不得引入 "${bad}"`);
    }
  }
  for (const good of rule.must) {
    if (!importsOf(file.source).some((spec) => spec.includes(good))) {
      problems.push(`${rel}: 必须引入 "${good}" 以绑定传输层`);
    }
  }
}

// 每个共享视图模型都必须真的被两个平台用到；只被一边用，说明迁移做了一半。
const models = files.filter((f) => /^src\/views\/[^/]+\/use[A-Z]\w*\.ts$/.test(f.rel));
for (const model of models) {
  const specifier = model.rel.replace(/^src\//, "").replace(/\.ts$/, "");
  const importers = files.filter(
    (f) => f !== model && importsOf(f.source).some((spec) => spec.endsWith(specifier)),
  );
  const desktop = importers.some((f) => /^src\/(components|pages)\//.test(f.rel));
  const mobile = importers.some((f) => f.rel.startsWith("src/mobile/"));
  if (!desktop || !mobile) {
    problems.push(
      `${model.rel}: 共享视图模型只在${desktop ? "" : "桌面"}${mobile ? "" : "移动"}端被引用（两端都要）`,
    );
  }
}

// 视图清单：文件必须存在、每个共享视图模型都要被某个视图认领、gap 指向已知视图。
// 清单本身是给人看的（移动端缺什么），这一节保证它不会烂掉。
const manifestPath = join(SRC, "views/manifest.json");
let manifest = null;
try {
  manifest = JSON.parse(readFileSync(manifestPath, "utf8"));
} catch {
  problems.push("src/views/manifest.json 缺失或不是合法 JSON");
}

if (manifest && process.argv.includes("--gaps")) {
  // 人类入口：看移动端还缺什么，一条命令列出来。
  for (const view of manifest.views ?? []) {
    const gaps = (manifest.gaps ?? []).filter((gap) => gap.view === view.id);
    console.log(`\n${view.title}（${view.id}）${view.mobile ? "" : " — 移动端无此视图"}`);
    for (const gap of gaps) {
      console.log(`  · [${gap.platform}] ${gap.capability}：${gap.reason}`);
    }
  }
  process.exit(0);
}

if (manifest) {
  const viewIds = new Set();
  const claimedModels = new Set();
  for (const view of manifest.views ?? []) {
    viewIds.add(view.id);
    for (const key of ["desktop", "mobile", "model"]) {
      const rel = view[key];
      if (!rel) continue;
      if (!existsSync(join(ROOT, rel))) {
        problems.push(`manifest: 视图 "${view.id}" 的 ${key} 指向不存在的文件 ${rel}`);
      }
      if (key === "model") claimedModels.add(rel);
    }
  }
  for (const model of models) {
    if (!claimedModels.has(model.rel)) {
      problems.push(`manifest: 共享视图模型 ${model.rel} 没有在视图清单里认领`);
    }
  }
  for (const gap of manifest.gaps ?? []) {
    if (!viewIds.has(gap.view)) {
      problems.push(`manifest: gap 指向未知视图 "${gap.view}"`);
    }
    if (gap.platform !== "desktop" && gap.platform !== "mobile") {
      problems.push(`manifest: gap 的 platform 只能是 desktop 或 mobile（收到 "${gap.platform}"）`);
    }
  }
  notes.push(
    `视图清单：${(manifest.views ?? []).length} 个视图、${(manifest.gaps ?? []).length} 处已知差异`,
  );
}

// 裸 <button> 是"绕过 ui/Button"的信号。它只许减少，不许变多。
// 只剩下 GuidedTour 聚光灯的两个控制（Astryx 无 tour/spotlight 等价物，
// 见 docs/spec/astryx-migration-plan.md 的显式例外）。
const BUTTON_CEILING = 2;
const bareButtons = files.filter(
  (f) => !f.rel.startsWith("src/ui/") && f.rel.endsWith(".tsx"),
);
const buttonCount = bareButtons.reduce(
  (sum, f) => sum + (f.source.match(/<button[\s>]/g)?.length ?? 0),
  0,
);
if (buttonCount > BUTTON_CEILING) {
  problems.push(`裸 <button> 数量 ${buttonCount} 超过上限 ${BUTTON_CEILING}（新代码请用 ui/button）`);
} else {
  notes.push(`裸 <button>：${buttonCount} / 上限 ${BUTTON_CEILING}`);
}

// 原生 <input>/<select>/<textarea> 也应收口到 ui/input 与 Astryx 表单组件；
// ui/ 自身是适配层，不计入。当前业务层应为 0。
const NATIVE_FORM_CEILING = 0;
const nativeFormCount = files
  .filter((f) => !f.rel.startsWith("src/ui/") && f.rel.endsWith(".tsx"))
  .reduce((sum, f) => sum + (f.source.match(/<(input|select|textarea)[\s>]/g)?.length ?? 0), 0);
if (nativeFormCount > NATIVE_FORM_CEILING) {
  problems.push(`裸表单控件 ${nativeFormCount} 超过上限 ${NATIVE_FORM_CEILING}（请用 ui/input 或 Astryx 表单组件）`);
} else {
  notes.push(`裸表单控件：${nativeFormCount} / 上限 ${NATIVE_FORM_CEILING}`);
}

// 带框封面必须经 ui/cover 的 CoverImage：竖版（右半封面）与横版（完整封面+
// 封底）是产品规则，散落成手写 <img> 就会有人再居中裁一刀、切出半张封面。
// 按原比例展示整张图的场景（详情大图、封面查看器、编辑预览）没有框，也没有
// 裁切决策，所以显式列白名单；新增一处裸封面 <img> 必须改动这份清单，让评审
// 看见"这里为什么不用 CoverImage"。
const RAW_COVER_ALLOWLIST = {
  "src/components/VideoDetailPanel.tsx": 2,
  "src/components/EditVideoDialog.tsx": 1,
  "src/mobile/components/VideoDetailView.tsx": 2,
};
const COVER_SRC = /videoCoverURL\(|coverURL\(|\/thumbnails\/|\/covers\//;
const rawCoverCounts = new Map();
for (const file of files) {
  if (!file.rel.endsWith(".tsx") || file.rel.startsWith("src/ui/")) continue;
  const count = [...file.source.matchAll(/<img\b[\s\S]*?\/>/g)].filter((tag) =>
    COVER_SRC.test(tag[0]),
  ).length;
  if (count > 0) rawCoverCounts.set(file.rel, count);
}
for (const [rel, count] of rawCoverCounts) {
  const allowed = RAW_COVER_ALLOWLIST[rel] ?? 0;
  if (count > allowed) {
    problems.push(
      `${rel}: 裸封面 <img> ${count} 处，超过允许的 ${allowed} 处` +
        `（新封面请用 ui/cover 的 CoverImage：portrait 竖版右半封面 / landscape 横版完整封面+封底）`,
    );
  } else if (count < allowed) {
    problems.push(
      `${rel}: 裸封面 <img> 只剩 ${count} 处，白名单记的是 ${allowed} 处；请同步这份清单`,
    );
  }
}
for (const rel of Object.keys(RAW_COVER_ALLOWLIST)) {
  if (!rawCoverCounts.has(rel)) {
    problems.push(`封面白名单 ${rel} 已没有裸封面 <img>，请从 check-shared-ui.mjs 删除这项`);
  }
}
const rawCoverTotal = [...rawCoverCounts.values()].reduce((sum, n) => sum + n, 0);
notes.push(`裸封面 <img>：${rawCoverTotal} 处（仅详情大图 / 查看器 / 编辑预览）`);

notes.push(`共享视图模型：${models.length} 个（两端均已引用）`);

for (const note of notes) console.log(`  ${note}`);
if (problems.length > 0) {
  console.error("\n目录规则检查未通过：");
  for (const problem of problems) console.error(`  ✗ ${problem}`);
  process.exit(1);
}
console.log("目录规则检查通过。");
