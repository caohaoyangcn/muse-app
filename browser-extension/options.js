// Options page: the Muse host and port, access to it, a connection test, and
// the install notes.
import {
  DEFAULT_HOST,
  DEFAULT_PORT,
  buildBaseURL,
  hasOriginPermission,
  loadMuseBaseURL,
  parseBaseURL,
  parsePort,
  requestOriginPermission,
  saveMuseAddress,
} from "./config.js";

const hostInput = document.getElementById("host");
const portInput = document.getElementById("port");
const preview = document.getElementById("preview");
const saveButton = document.getElementById("save");
const resetButton = document.getElementById("reset");
const grantButton = document.getElementById("grant");
const testButton = document.getElementById("test");
const status = document.getElementById("status");
const access = document.getElementById("access");

function setStatus(kind, text) {
  status.dataset.kind = kind;
  status.textContent = text;
}

/** The address the form currently describes. */
function currentBase() {
  const port = parsePort(portInput.value) || DEFAULT_PORT;
  return buildBaseURL(hostInput.value, port);
}

function renderPreview() {
  preview.textContent = currentBase();
  portInput.setAttribute("aria-invalid", parsePort(portInput.value) ? "false" : "true");
}

function describe(result) {
  const base = result?.base ?? currentBase();
  if (!result || result.ok === false) {
    switch (result?.reason) {
      case "noPermission":
        return ["warn", `还没有访问 ${base} 的权限：点「允许访问该地址」后重试。`];
      case "offline":
        return [
          "error",
          `连不上 ${base}：请确认 Muse 正在运行，且端口与「设置 → 服务器」里的一致。`,
        ];
      case "timeout":
        return ["error", `${base} 在 3 秒内没有响应。`];
      case "http":
        return ["error", `Muse 返回错误（HTTP ${result.status}）：${result.message ?? ""}`];
      default:
        return ["error", "连接失败。"];
    }
  }
  const entry = (result.codes ?? [])[0];
  const count = entry?.matches?.length ?? 0;
  return [
    "ok",
    `连接成功 ✓ ${base}（库中 ABP-576 ${count > 0 ? `命中 ${count} 条` : "没有命中"}）`,
  ];
}

/** Shows whether the extension may call the address in the form right now. */
async function refreshAccess() {
  const base = currentBase();
  const granted = await hasOriginPermission(base);
  access.dataset.kind = granted ? "ok" : "warn";
  access.textContent = granted
    ? `已获准访问 ${base}`
    : `${base} 还需要授权（浏览器要求用户亲自确认非默认端口）。`;
  grantButton.hidden = granted;
  return granted;
}

async function runTest() {
  testButton.disabled = true;
  setStatus("pending", "测试中…");
  try {
    const result = await chrome.runtime.sendMessage({ type: "testConnection" });
    setStatus(...describe(result));
  } catch (error) {
    setStatus("error", `测试失败：${String(error)}`);
  } finally {
    testButton.disabled = false;
  }
}

async function init() {
  const { host, port } = parseBaseURL(await loadMuseBaseURL());
  hostInput.value = host;
  portInput.value = String(port);
  renderPreview();
  await refreshAccess();
  await runTest();
}

saveButton.addEventListener("click", async () => {
  const port = parsePort(portInput.value);
  if (!port) {
    setStatus("error", `端口必须在 1-65535 之间（当前填写的是「${portInput.value}」）。`);
    return;
  }
  const saved = await saveMuseAddress(hostInput.value, port);
  const parts = parseBaseURL(saved);
  hostInput.value = parts.host;
  portInput.value = String(parts.port);
  renderPreview();
  setStatus("ok", `已保存 ${saved}`);
  const granted = await refreshAccess();
  if (!granted) {
    setStatus("warn", `已保存 ${saved}。点「允许访问该地址」让扩展真正连上它。`);
    return;
  }
  await runTest();
});

// chrome.permissions.request() only resolves from a user gesture, which is why
// a non-default port needs this one click rather than a manifest entry.
grantButton.addEventListener("click", async () => {
  const base = currentBase();
  grantButton.disabled = true;
  try {
    const granted = await requestOriginPermission(base);
    if (!granted) {
      setStatus("error", "授权被取消，扩展仍无法访问该地址。");
      return;
    }
    setStatus("ok", `已获准访问 ${base}`);
    await runTest();
  } finally {
    grantButton.disabled = false;
    await refreshAccess();
  }
});

resetButton.addEventListener("click", async () => {
  hostInput.value = DEFAULT_HOST;
  portInput.value = String(DEFAULT_PORT);
  renderPreview();
  await saveMuseAddress(DEFAULT_HOST, DEFAULT_PORT);
  setStatus("ok", `已恢复默认 ${buildBaseURL(DEFAULT_HOST, DEFAULT_PORT)}`);
  await refreshAccess();
  await runTest();
});

for (const input of [hostInput, portInput]) {
  input.addEventListener("input", renderPreview);
  input.addEventListener("change", () => void refreshAccess());
}

testButton.addEventListener("click", () => void runTest());

void init();
