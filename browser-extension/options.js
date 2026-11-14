// Options page: the Muse address, a connection test, and the install notes.
import { DEFAULT_MUSE_BASE, loadMuseBaseURL, normalizeBaseURL, saveMuseBaseURL } from "./config.js";

const baseInput = document.getElementById("base");
const saveButton = document.getElementById("save");
const resetButton = document.getElementById("reset");
const testButton = document.getElementById("test");
const status = document.getElementById("status");

function setStatus(kind, text) {
  status.dataset.kind = kind;
  status.textContent = text;
}

function describe(result) {
  if (!result || result.ok === false) {
    switch (result?.reason) {
      case "offline":
        return ["error", "无法连接：请确认 Muse 正在运行，且局域网服务处于开启状态。"];
      case "timeout":
        return ["error", "连接超时：Muse 没有在 3 秒内应答。"];
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
    count > 0
      ? `连接成功 ✓ 查询链路可用（库中 ABP-576 命中 ${count} 条）`
      : "连接成功 ✓ 查询链路可用（库中没有 ABP-576）",
  ];
}

async function test() {
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
  baseInput.value = await loadMuseBaseURL();
  if (!baseInput.value) baseInput.value = DEFAULT_MUSE_BASE;
  baseInput.placeholder = DEFAULT_MUSE_BASE;
}

saveButton.addEventListener("click", async () => {
  const normalized = normalizeBaseURL(baseInput.value);
  const saved = await saveMuseBaseURL(normalized);
  baseInput.value = saved;
  if (normalized !== saved) {
    setStatus("error", `只接受 http(s) 地址，已回退到 ${saved}`);
    return;
  }
  setStatus("ok", "已保存 ✓");
  // Muse may be reachable at the new address; verify instead of assuming.
  await test();
});

resetButton.addEventListener("click", async () => {
  baseInput.value = await saveMuseBaseURL(DEFAULT_MUSE_BASE);
  setStatus("ok", "已恢复默认地址");
  await test();
});

testButton.addEventListener("click", () => void test());

void init();
