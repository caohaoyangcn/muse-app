// Jump page: the service worker opens this extension page with the Muse URL in
// the fragment, and this page navigates there.
//
// The indirection exists because tabs.create() is subject to the extension's
// host permissions for the target URL, while navigating an extension page from
// itself is not. It also keeps the target out of the tab strip's history: the
// fragment never becomes a bookmarkable address.
const link = document.getElementById("link");
const target = document.getElementById("target");
const fallback = document.getElementById("fallback");

function safeTarget() {
  const raw = decodeURIComponent(String(location.hash ?? "").replace(/^#/, ""));
  if (!raw) return "";
  try {
    const url = new URL(raw);
    // Only an http(s) Muse address may be opened; anything else (javascript:,
    // file:, a data: URL) is refused rather than rendered.
    return url.protocol === "http:" || url.protocol === "https:" ? url.toString() : "";
  } catch {
    return "";
  }
}

const destination = safeTarget();
if (destination) {
  link.href = destination;
  target.textContent = destination;
  window.setTimeout(() => {
    location.replace(destination);
  }, 60);
} else {
  target.textContent = "（无效的地址）";
  fallback.hidden = false;
}
