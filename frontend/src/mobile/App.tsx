import { useCallback, useEffect, useRef, useState } from "react";

import type { Video } from "../types";
import { cn } from "../lib/utils";
import ErrorBoundary from "../components/ErrorBoundary";
import { invalidate } from "../services/live";
import { ApiError, likeVideo, markVideoOpened, removeVideo, updateVideoInfo } from "../services/http";
import { ActorFilmsView } from "./components/ActorFilmsView";
import { ActorIndexView } from "./components/ActorIndexView";
import { DuplicateView } from "./components/DuplicateView";
import { EditInfoSheet } from "./components/EditInfoSheet";
import { ExternalPlayerSheet } from "./components/ExternalPlayerSheet";
import { LibraryView } from "./components/LibraryView";
import { ScrapeTaskView } from "./components/ScrapeTaskView";
import { TrashView } from "./components/TrashView";
import { ConfirmSheet } from "../ui/sheet";
import { ToastStack, type ToastItem } from "../ui/toast";
import { VideoActionSheet } from "./components/VideoActionSheet";
import { VideoDetailView } from "./components/VideoDetailView";
import {
  copyText,
  externalStreamURL,
  isIOSDevice,
  loadPreferredPlayer,
  playerByID,
  savePreferredPlayer,
  type ExternalPlayerID,
} from "./externalPlayers";
import { parseRoute, routeHref, type Route, type Tab } from "./router";
import { newScrollKey, rememberScroll, restoreScroll } from "./scrollMemory";
import { videoName } from "./videoDisplay";

/** The browser-history payload each entry carries. */
type NavState = { museDepth: number; museModal?: boolean; museKey?: string };

function navState(): NavState {
  const raw = window.history.state as {
    museDepth?: unknown;
    museModal?: unknown;
    museKey?: unknown;
  } | null;
  return {
    museDepth: typeof raw?.museDepth === "number" ? raw.museDepth : 0,
    museModal: raw?.museModal === true,
    museKey: typeof raw?.museKey === "string" ? raw.museKey : undefined,
  };
}

function currentHref(): string {
  return window.location.pathname + window.location.search;
}

/**
 * The mobile shell. Three bottom tabs mirror the desktop's workspace concept
 * (library / actor index / scrape board); video detail and actor film lists are
 * pushed screens with a visible back arrow.
 *
 * Navigation is owned by the browser's own history rather than an in-memory
 * stack: every push writes the destination to the address bar, so the browser
 * back and forward buttons, the Android back gesture and the header arrow all
 * unwind the same list. A pushed screen's parent is simply the entry the user
 * came from, which also makes 收藏 → 详情 → 演员 → 详情 in four back steps.
 */
export default function App() {
  const [route, setRoute] = useState<Route>(() =>
    parseRoute(window.location.pathname, window.location.search),
  );
  const [keyword, setKeyword] = useState("");
  const [search, setSearch] = useState("");
  const [filterGroupId, setFilterGroupId] = useState<number | "all">("all");

  const [playerVideo, setPlayerVideo] = useState<Video | null>(null);
  const [playerNotice, setPlayerNotice] = useState<string | undefined>();
  const [preferredPlayer, setPreferredPlayer] = useState<ExternalPlayerID | null>(() => loadPreferredPlayer());
  const [menuVideo, setMenuVideo] = useState<Video | null>(null);
  const [editVideo, setEditVideo] = useState<Video | null>(null);
  const [removeTarget, setRemoveTarget] = useState<Video | null>(null);
  const [busy, setBusy] = useState(false);
  const [toasts, setToasts] = useState<ToastItem[]>([]);
  const toastSeq = useRef(0);
  const launchTimer = useRef<number | null>(null);
  // The history entry currently on screen. Scroll offsets are filed under this
  // key when the entry is left and replayed when it is entered again.
  const currentKey = useRef("");

  const toast = useCallback((message: string, tone: ToastItem["kind"] = "info") => {
    const id = ++toastSeq.current;
    setToasts((prev) => [...prev, { id, message, kind: tone }]);
    setTimeout(() => setToasts((prev) => prev.filter((t) => t.id !== id)), 2600);
  }, []);

  // Debounce the search box (matching the desktop's 300ms behaviour).
  useEffect(() => {
    const t = setTimeout(() => setKeyword(search.trim()), 300);
    return () => clearTimeout(t);
  }, [search]);

  const activeTab: Tab = route.kind === "tab" ? route.tab : "library";
  // The header box searches the library, so it only makes sense while the
  // library list is on screen. The actor views carry their own keyword box, and
  // leaving the library search visible there would present a second, inert
  // field right above it.
  const showSearch = route.kind === "tab" && route.tab === "library";

  /** Writes a route to the address bar and the UI in one step. */
  const navigate = useCallback((next: Route, options?: { replace?: boolean }) => {
    const state = navState();
    // File the leaving screen's offsets before its pane is unmounted.
    rememberScroll(state.museKey);

    const href = routeHref(next);
    if (href === currentHref()) {
      // Re-selecting the active tab needs no entry.
      const key = state.museKey ?? newScrollKey();
      window.history.replaceState({ museDepth: state.museDepth, museKey: key }, "", href);
      currentKey.current = key;
    } else if (state.museModal) {
      // A sheet is an entry on top of the URL it was opened from. The
      // destination takes its place, but still counts as one screen above that
      // URL, so back returns to the URL the sheet was opened from. It is a new
      // screen, so it starts with its own (empty) scroll memory.
      const key = newScrollKey();
      window.history.replaceState({ museDepth: state.museDepth + 1, museKey: key }, "", href);
      currentKey.current = key;
    } else if (options?.replace) {
      const key = newScrollKey();
      window.history.replaceState({ museDepth: state.museDepth, museKey: key }, "", href);
      currentKey.current = key;
    } else {
      const key = newScrollKey();
      window.history.pushState({ museDepth: state.museDepth + 1, museKey: key }, "", href);
      currentKey.current = key;
    }
    setRoute(next);
  }, []);

  /**
   * Back from a pushed screen. The browser history is the source of truth; a
   * deep link (or a refresh) has no in-app parent entry, so the screen's
   * natural root replaces it instead of sending the user out of the app.
   */
  const goBack = useCallback(
    (fallback: Route) => {
      rememberScroll(currentKey.current);
      if (navState().museDepth > 0) {
        window.history.back();
      } else {
        navigate(fallback, { replace: true });
      }
    },
    [navigate],
  );

  // Normalise an unknown or non-canonical deep link once, without adding an
  // entry. The route itself is already parsed by the state initialiser.
  useEffect(() => {
    // The shell owns scroll restoration: the browser only knows the document's
    // scroll, while every screen scrolls inside its own pane.
    if ("scrollRestoration" in window.history) {
      window.history.scrollRestoration = "manual";
    }

    const initial = parseRoute(window.location.pathname, window.location.search);
    const href = routeHref(initial);
    const state = navState();
    const key = state.museKey ?? newScrollKey();
    currentKey.current = key;
    // Always replace the initial entry: this canonicalises the URL, gives the
    // entry its scroll key, and drops a stale sheet marker left by a reload.
    window.history.replaceState({ museDepth: state.museDepth, museKey: key }, "", href);
    setRoute(initial);
  }, []);

  // Replay the entering entry's scroll offsets. A pushed screen has a fresh key
  // (nothing to restore); back/forward find the offsets filed when the entry was
  // left. restoreScroll keeps retrying while the screen fetches its rows.
  useEffect(() => restoreScroll(currentKey.current), [route]);

  // Browser/gesture back and forward: the URL is the destination, so parse it
  // and repaint. Open sheets belong to the entry being left, not the one being
  // entered.
  useEffect(() => {
    const onPop = (event: PopStateEvent) => {
      // The old screen is still mounted at this point, so its scroll offsets can
      // be filed before React swaps the pane out.
      rememberScroll(currentKey.current);

      const state = event.state as Partial<NavState> | null;
      const key = typeof state?.museKey === "string" ? state.museKey : newScrollKey();
      currentKey.current = key;
      if (typeof state?.museKey !== "string") {
        // An entry created outside the shell (or by an older build) still needs
        // a key so it can be remembered when the user leaves it again.
        window.history.replaceState(
          { museDepth: state?.museDepth ?? 0, museKey: key },
          "",
          currentHref(),
        );
      }

      setRoute(parseRoute(window.location.pathname, window.location.search));
      setMenuVideo(null);
      setEditVideo(null);
      setRemoveTarget(null);
      setPlayerVideo(null);
      setPlayerNotice(undefined);

      if (state?.museModal) {
        // Forward can land on a sheet entry, but the sheet's data is gone.
        // Collapse the entry so it does not become a dead step.
        window.history.replaceState(
          { museDepth: state.museDepth ?? 0, museKey: key },
          "",
          currentHref(),
        );
      }
    };
    window.addEventListener("popstate", onPop);
    return () => window.removeEventListener("popstate", onPop);
  }, []);

  // Sheets are history entries too, so Android's back gesture closes the sheet
  // before it leaves the screen — matching native navigation. They share the
  // screen's scroll key: closing a sheet must not disturb the pane underneath.
  const overlayOpen =
    playerVideo !== null || editVideo !== null || menuVideo !== null || removeTarget !== null;
  useEffect(() => {
    const state = navState();
    if (overlayOpen && !state.museModal) {
      window.history.pushState(
        { museDepth: state.museDepth, museModal: true, museKey: state.museKey },
        "",
        currentHref(),
      );
    } else if (!overlayOpen && state.museModal) {
      window.history.back();
    }
  }, [overlayOpen]);

  const cancelPendingLaunch = useCallback(() => {
    if (launchTimer.current != null) {
      window.clearTimeout(launchTimer.current);
      launchTimer.current = null;
    }
  }, []);

  // iOS offers no API that enumerates installed apps. A hidden/pagehide signal
  // is the best available evidence that Safari handed control to the player.
  useEffect(() => {
    const launched = () => cancelPendingLaunch();
    const onVisibility = () => {
      if (document.visibilityState === "hidden") launched();
    };
    document.addEventListener("visibilitychange", onVisibility);
    window.addEventListener("pagehide", launched);
    window.addEventListener("blur", launched);
    return () => {
      document.removeEventListener("visibilitychange", onVisibility);
      window.removeEventListener("pagehide", launched);
      window.removeEventListener("blur", launched);
      cancelPendingLaunch();
    };
  }, [cancelPendingLaunch]);

  // Returning from an external player (or another app) is when the playback
  // just recorded becomes visible to the list: re-read then, the same way a
  // local write does, so 最近播放 and its date groups are current.
  useEffect(() => {
    const onVisible = () => {
      if (document.visibilityState === "visible") invalidate();
    };
    document.addEventListener("visibilitychange", onVisible);
    return () => document.removeEventListener("visibilitychange", onVisible);
  }, []);

  const launchPlayer = useCallback(
    (video: Video, playerID: ExternalPlayerID) => {
      savePreferredPlayer(playerID);
      setPreferredPlayer(playerID);
      cancelPendingLaunch();

      if (!isIOSDevice()) {
        setPlayerNotice("当前设备不执行 iOS 播放器跳转，可复制播放地址到已安装的播放器中打开。");
        setPlayerVideo(video);
        return;
      }

      markVideoOpened(video.id);

      const uri = playerByID(playerID).buildURL(video, externalStreamURL(video.id));
      launchTimer.current = window.setTimeout(() => {
        launchTimer.current = null;
        if (document.visibilityState === "hidden") return;
        setPlayerNotice("未确认播放器已打开。可重试、切换播放器，或复制地址后手动打开。");
        setPlayerVideo(video);
      }, 2000);

      try {
        window.location.href = uri;
      } catch {
        cancelPendingLaunch();
        setPlayerNotice("无法启动播放器。请确认客户端已安装，或复制播放地址后手动打开。");
        setPlayerVideo(video);
      }
    },
    [cancelPendingLaunch],
  );

  const requestPlay = useCallback(
    (video: Video) => {
      setPlayerNotice(undefined);
      // Remote playback belongs to OpenList. Open its file page instead of
      // handing a Muse stream URL to a native player.
      if (video.sourceId > 0) {
        const url = video.cloudUrl;
        if (!url) {
          toast("没有可用的 OpenList 地址", "error");
          return;
        }
        markVideoOpened(video.id);
        window.location.href = url;
        return;
      }
      if (!isIOSDevice()) {
        setPlayerNotice("当前仅支持 iPhone/iPad 自动跳转外部播放器，可复制播放地址后手动打开。");
        setPlayerVideo(video);
        return;
      }
      if (!preferredPlayer) {
        setPlayerVideo(video);
        return;
      }
      launchPlayer(video, preferredPlayer);
    },
    [launchPlayer, preferredPlayer, toast],
  );

  const choosePlayer = useCallback(
    (playerID: ExternalPlayerID) => {
      if (!playerVideo) return;
      const video = playerVideo;
      setPlayerVideo(null);
      setPlayerNotice(undefined);
      launchPlayer(video, playerID);
    },
    [launchPlayer, playerVideo],
  );

  const copyPlaybackURL = useCallback(async () => {
    if (!playerVideo) return;
    const copied = await copyText(
      playerVideo.sourceId > 0 && playerVideo.cloudUrl
        ? playerVideo.cloudUrl
        : externalStreamURL(playerVideo.id),
    );
    toast(copied ? "已复制播放地址" : "复制失败，请手动复制", copied ? "info" : "error");
  }, [playerVideo, toast]);

  const openVideo = useCallback(
    (video: Video) => navigate({ kind: "detail", videoId: video.id }),
    [navigate],
  );
  const openActor = useCallback(
    (name: string) => navigate({ kind: "actor", name }),
    [navigate],
  );

  const confirmRemove = useCallback(async () => {
    if (!removeTarget) return;
    const removedID = removeTarget.id;
    // Close the sheet and drop the target from every screen that could still
    // show it. Done on failure too: the backend answers "already gone" as
    // success, so a failure means the list on screen is out of date and would
    // otherwise keep offering a removal that cannot succeed.
    const forget = () => {
      setRemoveTarget(null);
      setMenuVideo(null);
      setPlayerVideo((cur) => (cur && cur.id === removedID ? null : cur));
      setEditVideo((cur) => (cur && cur.id === removedID ? null : cur));
      invalidate();
    };
    setBusy(true);
    try {
      await removeVideo(removedID);
      toast(`已将「${videoName(removeTarget)}」移入回收站`);
      forget();
    } catch (e) {
      forget();
      toast(e instanceof ApiError ? e.message : "移入回收站失败", "error");
    } finally {
      setBusy(false);
    }
  }, [removeTarget, toast]);

  const like = useCallback(
    async (target: Video) => {
      try {
        const next = await likeVideo(target.id);
        toast(`已点赞「${videoName(next)}」♥ ${next.likeCount}`);
        invalidate();
        return next;
      } catch (e) {
        toast(e instanceof ApiError ? e.message : "点赞失败", "error");
        return null;
      }
    },
    [toast],
  );

  // The tab strip switches the base content; a pushed screen is not present on
  // a tab root, so selecting a tab is a plain history push.
  const selectTab = useCallback(
    (next: Tab) => {
      setKeyword("");
      setSearch("");
      navigate({ kind: "tab", tab: next });
    },
    [navigate],
  );

  // Reset the content boundary whenever the routed screen changes, so a broken
  // screen never traps the user: switching tabs or going back recovers.
  const contentKey =
    route.kind === "detail"
      ? `detail:${route.videoId}`
      : route.kind === "actor"
        ? `actor:${route.name}`
        : `tab:${activeTab}`;

  return (
    <div className="flex h-[100dvh] flex-col pt-[var(--safe-top)]">
      {/* Header */}
      <header className="flex items-center gap-3 border-b border-[var(--border)] bg-[var(--panel)] px-4 py-3">
        <img alt="" className="size-9 rounded-[10px] object-cover" src="/assets/muse-logo.png" />
        <span className="shrink-0 font-semibold">Muse Video</span>
        {showSearch ? (
          <div className="relative ml-auto min-w-0 flex-1">
            <input
              className="w-full rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg)] px-3 py-2 outline-none focus:border-[var(--accent)]"
              onChange={(e) => setSearch(e.target.value)}
              placeholder="搜索"
              type="search"
              value={search}
            />
          </div>
        ) : null}
      </header>

      {/* Body: the routed screen wins over the active tab. Wrapped so a broken
          screen degrades to a retry card instead of blanking the whole app. */}
      <ErrorBoundary scope="当前页面" resetKey={contentKey} variant="region">
      {route.kind === "detail" ? (
        <VideoDetailView
          key={route.videoId}
          onBack={() => goBack({ kind: "tab", tab: "library" })}
          onChoosePlayer={(v) => {
            setPlayerNotice(undefined);
            setPlayerVideo(v);
          }}
          onEdit={setEditVideo}
          onPlay={requestPlay}
          onSelectActor={openActor}
          videoId={route.videoId}
        />
      ) : route.kind === "actor" ? (
        <ActorFilmsView
          key={route.name}
          name={route.name}
          onBack={() => goBack({ kind: "tab", tab: "actors" })}
          onMenu={setMenuVideo}
          onOpenVideo={openVideo}
        />
      ) : activeTab === "library" ? (
        <LibraryView
          filterGroupId={filterGroupId}
          keyword={keyword}
          onFilterGroupChange={setFilterGroupId}
          onOpenMenu={setMenuVideo}
          onOpenVideo={openVideo}
        />
      ) : activeTab === "actors" ? (
        <ActorIndexView onSelect={openActor} />
      ) : activeTab === "duplicates" ? (
        <DuplicateView
          onOpenVideo={openVideo}
          onToast={toast}
        />
      ) : activeTab === "trash" ? (
        <TrashView onToast={toast} />
      ) : (
        <ScrapeTaskView onOpenVideo={openVideo} />
      )}
      </ErrorBoundary>

      {/* Bottom tabs — hidden while a screen is pushed so the back arrow is the
          single way out, matching native navigation. */}
      {route.kind === "tab" ? (
        <nav className="flex border-t border-[var(--border)] bg-[var(--panel)] pb-[var(--safe-bottom)]">
          {(
            [
              ["library", "收藏"],
              ["actors", "演员"],
              ["scrape", "刮削"],
              ["duplicates", "查重"],
              ["trash", "回收站"],
            ] as [Tab, string][]
          ).map(([id, label]) => (
            <button
              className={cn(
                "flex-1 py-3 text-sm",
                activeTab === id ? "text-[var(--accent)]" : "text-[var(--muted)]",
              )}
              key={id}
              onClick={() => selectTab(id)}
              type="button"
            >
              {label}
            </button>
          ))}
        </nav>
      ) : null}

      {/* Overlays */}
      <VideoActionSheet
        onClose={() => setMenuVideo(null)}
        onDetails={() => {
          if (menuVideo) openVideo(menuVideo);
          setMenuVideo(null);
        }}
        onEdit={() => {
          setEditVideo(menuVideo);
          setMenuVideo(null);
        }}
        onLike={() => {
          const target = menuVideo;
          setMenuVideo(null);
          if (target) void like(target);
        }}
        onPlay={() => {
          if (menuVideo) requestPlay(menuVideo);
          setMenuVideo(null);
        }}
        onChoosePlayer={() => {
          setPlayerNotice(undefined);
          setPlayerVideo(menuVideo);
          setMenuVideo(null);
        }}
        onRemove={() => {
          setRemoveTarget(menuVideo);
          setMenuVideo(null);
        }}
        open={menuVideo !== null}
        video={menuVideo}
      />

      {editVideo ? (
        <EditInfoSheet
          busy={busy}
          onClose={() => setEditVideo(null)}
          onSave={async (title, notes) => {
            setBusy(true);
            try {
              await updateVideoInfo(editVideo.id, title, notes);
              invalidate();
              setEditVideo(null);
              toast("已保存");
            } catch (e) {
              toast(e instanceof ApiError ? e.message : "保存失败", "error");
            } finally {
              setBusy(false);
            }
          }}
          open
          video={editVideo}
        />
      ) : null}

      <ConfirmSheet
        busy={busy}
        confirmLabel="移入回收站"
        danger
        message="源视频文件不会被删除；移入后可在回收站恢复。"
        onClose={() => setRemoveTarget(null)}
        onConfirm={() => void confirmRemove()}
        open={removeTarget !== null}
        title={`移入回收站「${removeTarget ? videoName(removeTarget) : ""}」？`}
      />

      {playerVideo ? (
        <ExternalPlayerSheet
          notice={playerNotice}
          onChoose={choosePlayer}
          onClose={() => {
            setPlayerVideo(null);
            setPlayerNotice(undefined);
          }}
          onCopy={() => void copyPlaybackURL()}
          open
          preferred={preferredPlayer}
          video={playerVideo}
        />
      ) : null}

      <ToastStack placement="bottom" toasts={toasts} />
    </div>
  );
}
