import { useCallback, useEffect, useRef, useState } from "react";
import { serverStatus, startServer, stopServer } from "../services/desktop";
import type { ServerStartInfo } from "../types";
import { Button } from "../ui/button";

/**
 * Header control for the LAN server mode. Lets the user start/stop the HTTP
 * server and see/copy the URL a phone on the same network can open for the
 * mobile web UI.
 */
export default function ServerBar() {
  const [info, setInfo] = useState<ServerStartInfo | null>(null);
  const [starting, setStarting] = useState(false);
  const [copied, setCopied] = useState(false);
  const checkTimer = useRef<number | null>(null);

  const refresh = useCallback(async () => {
    try {
      setInfo(await serverStatus());
    } catch {
      /* server polling failures are non-fatal */
    }
  }, []);

  useEffect(() => {
    void refresh();
    // While running, poll lightly so the port/IP stays current the moment the
    // machine's LAN address changes.
    checkTimer.current = window.setInterval(() => void refresh(), 5000);
    return () => {
      if (checkTimer.current != null) window.clearInterval(checkTimer.current);
    };
  }, [refresh]);

  const handleToggle = async () => {
    if (info?.running) {
      try {
        await stopServer();
        setInfo({ running: false, urls: [], local: "", port: 0 });
      } catch (e) {
        setInfo({ running: false, urls: [], local: "", port: 0, error: String(e) });
      }
      return;
    }
    setStarting(true);
    try {
      setInfo(await startServer(0));
    } catch (e) {
      setInfo({ running: false, urls: [], local: "", port: 0, error: String(e) });
    } finally {
      setStarting(false);
    }
  };

  const copy = async (url: string) => {
    try {
      await navigator.clipboard.writeText(url);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      /* clipboard unavailable */
    }
  };

  const url = info?.urls?.find((u) => !u.startsWith("http://localhost:"));
  // The address the phone should open carries the key: the mobile shell logs in
  // on arrival and then drops the parameter from the address bar, so the user
  // never has to type the 16 characters. The bare IP still works, but lands on
  // the login form.
  const loginURL = (() => {
    if (!url) return "";
    try {
      const target = new URL(`${url}/`);
      if (info?.key) target.searchParams.set("key", info.key);
      return target.toString();
    } catch {
      return url;
    }
  })();
  const noLanAddress = info?.running && !url;
  const startError = info?.error && info.error !== "not running" ? info.error : "";

  return (
    <div className="flex items-center gap-2" data-tour="server">
      {info?.running ? (
        <>
          {url ? (
            <Button
              className="max-w-[260px] truncate border-[var(--border)] text-xs"
              disabled={copied}
              onClick={() => void copy(loginURL)}
              size="sm"
              title={`${loginURL} — 点击复制含访问口令的手机地址`}
              variant="ghost"
            >
              {copied ? "已复制（含口令）✓" : `📱 ${url.replace(/^https?:\/\//, "")}`}
            </Button>
          ) : (
            <span
              className="text-xs text-[#e0b64f]"
              title="未检测到可与手机互通的局域网 IP，请检查电脑是否已连接网络，以及手机和电脑是否在同一局域网"
            >
              ⚠ 未找到局域网地址
            </span>
          )}
          <Button
            variant="ghost"
            className="border-[var(--border)]"
            onClick={() => void handleToggle()}
            aria-label="停止局域网访问"
          >
            ⏹ 停止
          </Button>
          {noLanAddress && (
            <span className="text-xs text-[var(--muted)]">
              <a href={info.local || undefined} target="_blank" rel="noreferrer">
                本机
              </a>{" "}
              可访问
            </span>
          )}
        </>
      ) : (
        <>
          <Button
            variant="default"
            className=""
            onClick={() => void handleToggle()}
            disabled={starting}
            title="启动局域网 Web 服务，可在同一网络下的手机上访问收藏库"
          >
            {starting ? "启动中…" : "📱 局域网访问"}
          </Button>
          {startError && (
            <span className="max-w-[280px] truncate text-xs text-[var(--danger)]" title={startError}>
              ⚠ {startError}
            </span>
          )}
        </>
      )}
    </div>
  );
}
