// JavDB card helper.
//
// The one rule that matters: this script never guesses a 番号. JavDB renders
// the code of every card as its own element (the <strong> inside the card's
// title); the script reads that element's text and hands it to Muse, which
// already knows how to answer "is this collected?". There is deliberately no
// pattern matching here — a wrong guess would badge a card for a different
// work, and the page already tells us which work it is.
(() => {
  // Site structure. Only these two selectors describe where the code lives;
  // a JavDB redesign should be a one-line change to CARD_SELECTOR and one to
  // CODE_SELECTOR.
  //
  // CARD_SELECTOR deliberately does not name a list container (.movie-list,
  // #videos): JavDB renders the same card markup on search, actor, ranking and
  // home pages, and pinning a container makes the extension silently do
  // nothing on every page that uses another one. A card is "an /v/ link whose
  // title carries the rendered code", which is the property actually relied on
  // — and it excludes the site's many other /v/ links (nav, recommendations)
  // because those have no .video-title.
  const CARD_SELECTOR = "a.box[href^='/v/']";
  const CODE_SELECTOR = ".video-title strong";
  const TITLE_SELECTOR = ".video-title";

  // A page that renders no cards is either not a listing page or a redesign;
  // the observer keeps watching instead of giving up.
  const RESCAN_DELAY_MS = 10000;
  // Cards are queried as soon as they render; this window only folds one burst
  // of DOM inserts into a single request.
  const BATCH_WINDOW_MS = 200;
  // The endpoint answers at most this many codes per request.
  const MAX_BATCH = 100;
  const CACHE_TTL_MS = 60000;

  /** @type {Map<string, {result: any, at: number}>} */
  const cache = new Map();
  /** Cards currently being looked at: element -> { code, state }. */
  const cardState = new WeakMap();
  let popover = null;
  let pendingCodes = new Set();
  let pendingTimer = null;
  let offlineAnnounced = false;

  // ---------- reading the page ----------

  /**
   * Reads one card's 番号 from the element JavDB renders it in. Whitespace and
   * zero-width characters are stripped and letters upper-cased: that is a
   * literal clean-up of the same value, not a parse.
   */
  function readCode(card) {
    const node = card.querySelector(CODE_SELECTOR);
    if (!node) return "";
    // Keep the value literal: trim the padding JavDB adds, drop zero-width
    // characters, and leave every other character alone. The backend applies
    // the same clean-up, so both sides compare the same string; a separator
    // written differently on the page is still the page's own spelling.
    return String(node.textContent ?? "")
      .replace(/[\u200b\ufeff]/g, "")
      .replace(/^[\s\u00a0\u3000]+|[\s\u00a0\u3000]+$/g, "")
      .toUpperCase();
  }

  function readTitle(card) {
    const node = card.querySelector(TITLE_SELECTOR);
    if (!node) return "";
    // Drop the code node's own text so the title reads as the work's name.
    let text = "";
    for (const child of node.childNodes) {
      if (child.nodeType === Node.ELEMENT_NODE && child.matches("strong")) continue;
      text += child.textContent ?? "";
    }
    return text.replace(/\s+/g, " ").trim();
  }

  // ---------- badge ----------

  function ensureBadge(card) {
    let badge = card.querySelector(":scope > .muse-javdb-badge");
    if (badge) return badge;
    badge = document.createElement("span");
    badge.className = "muse-javdb-badge";
    badge.setAttribute("role", "button");
    badge.setAttribute("tabindex", "0");
    card.appendChild(badge);
    card.classList.add("muse-javdb-card");
    return badge;
  }

  function setBadge(badge, state, label, title) {
    badge.dataset.state = state;
    badge.textContent = label;
    badge.title = title;
  }

  function stateFor(result) {
    if (!result) return { state: "idle", label: "·", title: "Muse：等待查询" };
    if (result.ok === false) {
      if (result.reason === "noPermission") {
        return {
          state: "offline",
          label: "⚠",
          title: `扩展没有访问 ${result.base} 的权限：请打开扩展「选项」，点「允许访问该地址」`,
        };
      }
      if (result.reason === "offline" || result.reason === "timeout") {
        return {
          state: "offline",
          label: "⚠",
          title: `Muse 未连接（${result.base ?? museBase}）：请确认 Muse 正在运行，并在扩展「选项」里填写正确地址`,
        };
      }
      return { state: "offline", label: "⚠", title: `Muse 返回错误：${result.message ?? result.status}` };
    }
    if (!result.check) return { state: "idle", label: "·", title: "Muse" };
    const count = result.check.matches.length;
    if (count === 0) return { state: "none", label: "✗", title: "Muse 中未收藏该番号" };
    const inTrash = result.check.matches.every((m) => m.inTrash);
    if (inTrash) return { state: "trash", label: "🗑", title: `仅在 Muse 回收站中（${count} 个）` };
    return { state: "found", label: `✓${count > 1 ? count : ""}`, title: `Muse 已收藏（${count} 个版本）` };
  }

  // ---------- querying ----------

  function cached(code) {
    const entry = cache.get(code);
    if (!entry) return null;
    if (Date.now() - entry.at > CACHE_TTL_MS) {
      cache.delete(code);
      return null;
    }
    return entry.result;
  }

  /**
   * Queues a code for the next batch. Cards are queryable as soon as they are
   * rendered, so a listing page asks about all of them at once; the 200 ms
   * window only coalesces the burst into as few requests as the server accepts.
   */
  function enqueue(code, card) {
    pendingCodes.add(code);
    card.dataset.musePending = "1";
    if (pendingTimer != null) return;
    pendingTimer = window.setTimeout(() => {
      pendingTimer = null;
      const queued = Array.from(pendingCodes);
      pendingCodes = new Set();
      // The endpoint caps a batch; a long page simply takes more than one round
      // trip instead of being rejected.
      for (let i = 0; i < queued.length; i += MAX_BATCH) {
        void runBatch(queued.slice(i, i + MAX_BATCH));
      }
    }, BATCH_WINDOW_MS);
  }

  async function runBatch(codes) {
    let response;
    try {
      response = await chrome.runtime.sendMessage({ type: "check", codes });
    } catch {
      response = { ok: false, reason: "offline", message: "扩展后台不可用" };
    }
    if (!response || response.ok === false) {
      for (const code of codes) cache.set(code, { result: response, at: Date.now() });
      announceOfflineOnce();
      refreshBadges();
      return;
    }
    const byCode = new Map((response.codes ?? []).map((entry) => [entry.code, entry]));
    for (const code of codes) {
      cache.set(code, { result: { ok: true, check: byCode.get(code) ?? null }, at: Date.now() });
    }
    refreshBadges();
  }

  function announceOfflineOnce() {
    if (offlineAnnounced) return;
    offlineAnnounced = true;
    // Do not nag: one toast per page, and the badges keep saying 未连接. The
    // message names the configured address, because a wrong port is the usual
    // cause and the default one is not necessarily what the user runs.
    toast(`Muse 未连接：请确认 Muse 正在运行，并在扩展选项里填写正确的服务地址（当前 ${museBase}）。`);
  }

  /** Repaints every badge on the page from the cache. */
  function refreshBadges() {
    for (const card of document.querySelectorAll(CARD_SELECTOR)) {
      const badge = ensureBadge(card);
      const code = readCode(card);
      if (!code) {
        setBadge(badge, "unknown", "?", "该卡片没有番号字段");
        cardState.set(card, { code: "", result: null });
        continue;
      }
      const result = cached(code);
      if (!result) continue; // still queued or in flight; the next round paints it
      const view = stateFor(result);
      setBadge(badge, view.state, view.label, view.title);
      cardState.set(card, { code, result });
      if (card.dataset.musePending) delete card.dataset.musePending;
    }
  }

  function mark(card, view) {
    setBadge(ensureBadge(card), view.state, view.label, view.title);
  }

  // ---------- popover ----------

  function closePopover() {
    if (popover) {
      popover.remove();
      popover = null;
    }
  }

  function formatSize(bytes) {
    if (!bytes) return "";
    const gb = bytes / (1 << 30);
    if (gb >= 1) return `${gb.toFixed(2)} GB`;
    return `${Math.max(1, Math.round(bytes / (1 << 20)))} MB`;
  }

  function openPopover(card, anchor) {
    closePopover();
    const { code, result } = cardState.get(card) ?? {};
    if (!code) {
      toast("该卡片没有番号字段，无法判断是否已收藏。");
      return;
    }
    if (!result) {
      mark(card, { state: "loading", label: "…", title: "查询 Muse 中…" });
      enqueue(code, card);
      return;
    }
    if (result.ok === false) {
      toast("Muse 未连接。请启动 Muse，并确认局域网服务处于开启状态（默认端口 8899）。");
      return;
    }
    const check = result.check;
    if (!check || check.matches.length === 0) {
      toast(`Muse 中未收藏 ${code}。`);
      return;
    }

    const box = document.createElement("div");
    box.className = "muse-javdb-popover";
    const header = document.createElement("div");
    header.className = "muse-javdb-popover-title";
    header.textContent = `${code} · ${check.matches.length} 个版本`;
    box.appendChild(header);

    for (const match of check.matches) {
      const row = document.createElement("button");
      row.type = "button";
      row.className = "muse-javdb-match";
      row.title = "在 Muse 中打开";

      const cover = document.createElement("img");
      cover.alt = "";
      // Muse serves the cover and the generated thumbnail from the same base
      // URL the query went to; both files are addressed by their data-dir
      // relative path ("covers/jav/<id>.jpg" or "thumbnails/<id>.jpg").
      cover.src = `${museBase}/${match.coverPath || `thumbnails/${match.videoId}.jpg`}`;
      cover.loading = "lazy";
      cover.addEventListener("error", () => cover.remove(), { once: true });
      row.appendChild(cover);

      const meta = document.createElement("div");
      meta.className = "muse-javdb-match-meta";
      const name = document.createElement("div");
      name.className = "muse-javdb-match-name";
      name.textContent = match.label || match.fileName;
      meta.appendChild(name);
      const detail = document.createElement("div");
      detail.className = "muse-javdb-match-detail";
      detail.textContent = [
        match.width && match.height ? `${match.width}×${match.height}` : "",
        formatSize(match.fileSize),
        (match.actors ?? []).slice(0, 2).join(" / "),
      ]
        .filter(Boolean)
        .join(" · ");
      meta.appendChild(detail);
      const tags = [];
      if (match.inTrash) tags.push("回收站");
      if (match.sourceName) tags.push(`网盘：${match.sourceName}`);
      if (match.volumeOffline) tags.push("磁盘未接入");
      if (match.status === "missing") tags.push("文件缺失");
      if (tags.length > 0) {
        const tagLine = document.createElement("div");
        tagLine.className = "muse-javdb-match-tags";
        tagLine.textContent = tags.join(" · ");
        meta.appendChild(tagLine);
      }
      row.appendChild(meta);

      row.addEventListener("click", (event) => {
        event.preventDefault();
        event.stopPropagation();
        closePopover();
        void chrome.runtime.sendMessage({ type: "openVideo", videoId: match.videoId });
      });
      box.appendChild(row);
    }

    document.body.appendChild(box);
    const rect = anchor.getBoundingClientRect();
    const top = rect.bottom + window.scrollY + 6;
    const left = Math.min(
      rect.left + window.scrollX,
      window.scrollX + document.documentElement.clientWidth - box.offsetWidth - 12,
    );
    box.style.top = `${top}px`;
    box.style.left = `${Math.max(8, left)}px`;
    popover = box;
  }

  // The assets base follows the configured Muse address, which is stored (not
  // secret) and readable here synchronously after the first read.
  let museBase = "http://127.0.0.1:8899";
  chrome.storage.local
    .get("museBaseUrl")
    .then((stored) => {
      if (stored && stored.museBaseUrl) museBase = String(stored.museBaseUrl).replace(/\/+$/, "");
    })
    .catch(() => undefined);

  // ---------- toast ----------

  let toastNode = null;
  function toast(text) {
    if (!toastNode) {
      toastNode = document.createElement("div");
      toastNode.className = "muse-javdb-toast";
      document.body.appendChild(toastNode);
    }
    toastNode.textContent = text;
    toastNode.classList.add("muse-javdb-toast-visible");
    window.clearTimeout(toastNode.museTimer);
    toastNode.museTimer = window.setTimeout(() => {
      toastNode?.classList.remove("muse-javdb-toast-visible");
    }, 4000);
  }

  // ---------- wiring ----------

  /**
   * Paints a badge for every card on the page and queues the ones that have not
   * been answered yet. The status is the point of the extension, so it is
   * fetched eagerly: a page of 40 cards costs one batched request, and the user
   * never has to hover to learn anything.
   */
  function decorate() {
    const cards = document.querySelectorAll(CARD_SELECTOR);
    if (cards.length === 0) return 0;
    for (const card of cards) {
      const badge = ensureBadge(card);
      const code = readCode(card);
      if (!code) {
        setBadge(badge, "unknown", "?", "该卡片没有番号字段");
        cardState.set(card, { code: "", result: null });
        continue;
      }
      const result = cached(code);
      if (!result) {
        setBadge(badge, "loading", "…", "查询 Muse 中…");
        cardState.set(card, { code, result: null });
        enqueue(code, card);
      } else {
        const view = stateFor(result);
        setBadge(badge, view.state, view.label, view.title);
        cardState.set(card, { code, result });
      }
    }
    return cards.length;
  }

  // Hover is only a hint for a card whose query is still in flight; it never
  // gates the query itself.
  function onCardEnter(event) {
    const card = event.target instanceof Element ? event.target.closest(CARD_SELECTOR) : null;
    if (!card) return;
    const state = cardState.get(card);
    if (!state || !state.code || state.result) return;
    enqueue(state.code, card);
  }

  function onBadgeClick(event) {
    const badge = event.target instanceof Element ? event.target.closest(".muse-javdb-badge") : null;
    if (!badge) return;
    const card = badge.closest(CARD_SELECTOR);
    if (!card) return;
    // The badge lives inside the card's link; never follow it or open the work.
    event.preventDefault();
    event.stopPropagation();
    openPopover(card, badge);
  }

  function onBadgeKey(event) {
    if (event.key !== "Enter" && event.key !== " ") return;
    onBadgeClick(event);
  }

  document.addEventListener("mouseover", onCardEnter, true);
  document.addEventListener("click", onBadgeClick, true);
  document.addEventListener("keydown", onBadgeKey, true);
  document.addEventListener("mouseover", (event) => {
    // Clicking anywhere else dismisses the popover.
    if (popover && !(event.target instanceof Element && event.target.closest(".muse-javdb-popover"))) {
      closePopover();
    }
  });
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible") {
      cache.clear();
      decorate();
    }
  });

  // New cards appear on pagination and infinite scroll; repaint after each
  // mutation burst instead of on every node change.
  let repaintTimer = null;
  const observer = new MutationObserver(() => {
    if (repaintTimer != null) return;
    repaintTimer = window.setTimeout(() => {
      repaintTimer = null;
      decorate();
    }, 120);
  });
  observer.observe(document.body, { childList: true, subtree: true });

  if (decorate() === 0) {
    // Not a listing page yet (or a redesign): keep watching, and retry once in
    // case the cards arrive without a mutation we can see.
    window.setTimeout(() => {
      decorate();
    }, RESCAN_DELAY_MS);
  }
})();
