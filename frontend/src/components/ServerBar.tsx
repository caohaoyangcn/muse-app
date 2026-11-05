import { useCallback, useEffect, useRef, useState } from "react";
import { serverStatus, startServer, stopServer } from "../services/api";
import type { ServerStartInfo } from "../types";
import { Button } from "./ui/button";

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
  const noLanAddress = info?.running && !url;
  const startError = info?.error && info.error !== "not running" ? info.error : "";

  return (
    <div className="flex items-center gap-2" data-tour="server">
      {info?.running ? (
        <>
          {url ? (
            <button
              className="max-w-[260px] truncate rounded-[var(--radius)] border border-[var(--border)] bg-[var(--panel)] px-2.5 py-1.5 text-xs text-[var(--text)] hover:border-[var(--accent)] disabled:opacity-70"
              title={`${url} — 点击复制`}
              onClick={() => void copy(url)}
              disabled={copied}
            >
              {copied ? "已复制 ✓" : `📱 ${url.replace(/^https?:\/\//, "")}`}
            </button>
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
