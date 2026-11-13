#!/usr/bin/env node
// Guards the directory rules in docs/spec/cross-platform-ui-plan.md §3.
//
// This is a grep-shaped check on purpose: the rules are about *which directory
// may import which*, and a violation is cheapest to catch here rather than in a
// review where a `../components/Foo` import looks perfectly ordinary.
//
// Run from frontend/: `npm run check:shared`
import { readFileSync, readdirSync, statSync } from "node:fs";
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

// 视图模型不得关心渲染层。
forbid({ rule: "views/ 不得 import 渲染层", inDir: "views", forbidden: ["components", "mobile"] });

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

// 裸 <button> 是"绕过 ui/Button"的信号。它只许减少，不许变多。
const BUTTON_CEILING = 80;
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

notes.push(`共享视图模型：${models.length} 个（两端均已引用）`);

for (const note of notes) console.log(`  ${note}`);
if (problems.length > 0) {
  console.error("\n目录规则检查未通过：");
  for (const problem of problems) console.error(`  ✗ ${problem}`);
  process.exit(1);
}
console.log("目录规则检查通过。");
