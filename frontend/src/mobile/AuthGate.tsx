import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";

import { ApiError, loginWithKey, onSessionLost, sessionKey } from "../services/web";
import { Button } from "../ui/button";
import { Input } from "../ui/input";

type GateState = "checking" | "needed" | "ready";

/** The key the desktop was opened with, if the URL carried one. */
function keyFromURL(): string {
  return new URLSearchParams(window.location.search).get("key") ?? "";
}

/**
 * Removes the key from the address bar without adding a history entry. A key
 * left in the URL would end up in the browser's history, in a shared link, and
 * in the referrer of any link the user taps from the save.
 */
function forgetKeyInURL() {
  const current = new URL(window.location.href);
  if (!current.searchParams.has("key")) return;
  current.searchParams.delete("key");
  window.history.replaceState(
    window.history.state,
    "",
    current.pathname + current.search + current.hash,
  );
}

/**
 * The LAN shell's front door.
 *
 * The phone authenticates once with the key shown on the desktop; the server
 * answers with an HttpOnly cookie, so every later request (including `<img>`
 * thumbnails and covers) is authenticated without the key appearing in a URL.
 * A URL that carries `?key=` — the handoff the desktop header copies — logs in
 * automatically, then drops the parameter.
 *
 * The gate also owns the reaction to a 401 from any view: the key was rotated
 * or expired, so the shell unmounts and the form returns. Data is never shown
 * to an unauthenticated tab.
 */
export function AuthGate({ children }: { children: ReactNode }) {
  const [state, setState] = useState<GateState>("checking");
  const [key, setKey] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const autoKey = useRef(keyFromURL());
  const started = useRef(false);

  const enter = useCallback(async (candidate: string): Promise<boolean> => {
    setBusy(true);
    setError("");
    try {
      await loginWithKey(candidate);
      forgetKeyInURL();
      setState("ready");
      return true;
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "登录失败，请重试");
      return false;
    } finally {
      setBusy(false);
    }
  }, []);

  useEffect(() => {
    if (started.current) return;
    started.current = true;
    let cancelled = false;
    void (async () => {
      // A URL carrying the key is the desktop's "open on phone" handoff. Try it
      // first; a stale key falls through to the session check and then the
      // form, so a bad link still has a recovery path.
      if (autoKey.current && (await enter(autoKey.current))) return;
      try {
        const existing = await sessionKey();
        if (!cancelled) setState(existing !== null ? "ready" : "needed");
      } catch (e) {
        if (cancelled) return;
        setError(e instanceof ApiError ? e.message : "无法连接到服务");
        setState("needed");
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [enter]);

  // A view that gets a 401 while mounted (key rotated on the desktop, cookie
  // cleared) brings the form back and unmounts the shell.
  useEffect(
    () =>
      onSessionLost(() => {
        setError("访问口令已失效，请重新输入");
        setState("needed");
      }),
    [],
  );

  const submit = useCallback(async () => {
    if (await enter(key)) setKey("");
  }, [enter, key]);

  if (state === "checking") {
    return (
      <div className="flex h-[100dvh] items-center justify-center bg-[var(--bg)] text-[var(--muted)]">
        正在连接…
      </div>
    );
  }

  if (state === "ready") return <>{children}</>;

  return (
    <div className="flex h-[100dvh] items-center justify-center bg-[var(--bg)] px-5 pb-[var(--safe-bottom)] pt-[var(--safe-top)]">
      <form
        className="w-full max-w-sm rounded-[var(--radius)] border border-[var(--border)] bg-[var(--panel)] p-6"
        onSubmit={(event) => {
          event.preventDefault();
          void submit();
        }}
      >
        <h1 className="text-lg font-semibold">Muse Video</h1>
        <p className="mt-2 text-sm leading-relaxed text-[var(--muted)]">
          局域网收藏需要访问口令。口令显示在桌面端顶部的「局域网访问」或「设置 → 服务器」。
        </p>
        <label className="mt-5 block text-xs text-[var(--muted)]" htmlFor="lan-access-key">
          访问口令
        </label>
        <Input
          autoComplete="off"
          autoFocus
          className="mt-1.5"
          id="lan-access-key"
          onChange={(event) => {
            setKey(event.target.value);
            setError("");
          }}
          placeholder="xxxx-xxxx-xxxx-xxxx"
          value={key}
        />
        {error ? (
          <p className="mt-3 text-[13px] leading-relaxed text-[var(--danger)]" role="alert">
            {error}
          </p>
        ) : null}
        <Button
          className="mt-5 w-full py-3"
          disabled={busy || key.trim() === ""}
          type="submit"
          variant="primary"
        >
          {busy ? "验证中…" : "进入"}
        </Button>
      </form>
    </div>
  );
}
