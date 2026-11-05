import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  AlertCircle,
  ChevronRight,
  Cloud,
  FileVideo,
  Folder,
  Loader2,
  RefreshCw,
} from "lucide-react";
import { Button } from "./ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "./ui/dialog";
import { addCloudVideos, browseCloud, listCloudConnections } from "../services/api";
import type { AddVideoResult, CloudConnection, CloudEntry } from "../types";
import { formatFileSize } from "../utils/format";
import { cn } from "../lib/utils";

const VIDEO_EXTENSION = /\.(mp4|mkv|webm|mov|avi|m4v)$/i;

interface CloudBrowserDialogProps {
  open: boolean;
  onClose: () => void;
  /** Open Settings → 网盘 when no connection exists yet. */
  onOpenSettings: () => void;
  /** Report the per-file add outcome; the library shows the summary. */
  onAdded: (results: AddVideoResult[]) => void;
}

/**
 * Read-only WebDAV browser behind 添加视频 → 从网盘添加. It browses one
 * connection at a time, multi-selects video files and hands the paths to the
 * backend, which re-checks each one before creating a favorite. It never offers
 * rename/delete/upload: management belongs in the OpenList web UI.
 */
export default function CloudBrowserDialog({
  open,
  onClose,
  onOpenSettings,
  onAdded,
}: CloudBrowserDialogProps) {
  const [connections, setConnections] = useState<CloudConnection[] | null>(null);
  const [connectionID, setConnectionID] = useState<number | null>(null);
  const [path, setPath] = useState("/");
  const [entries, setEntries] = useState<CloudEntry[] | null>(null);
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const [loadingConnections, setLoadingConnections] = useState(false);
  const [loadingEntries, setLoadingEntries] = useState(false);
  const [adding, setAdding] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const requestSeq = useRef(0);

  const enabledConnections = useMemo(
    () => connections?.filter((item) => item.enabled) ?? null,
    [connections],
  );
  const connection = useMemo(
    () => enabledConnections?.find((item) => item.id === connectionID) ?? null,
    [enabledConnections, connectionID],
  );

  const loadConnections = useCallback(async () => {
    setLoadingConnections(true);
    setError(null);
    try {
      const list = await listCloudConnections();
      setConnections(list);
      const usable = list.filter((item) => item.enabled);
      setConnectionID((current) => {
        if (current != null && usable.some((item) => item.id === current)) return current;
        return usable[0]?.id ?? null;
      });
    } catch (e) {
      setError(String(e));
    } finally {
      setLoadingConnections(false);
    }
  }, []);

  useEffect(() => {
    if (!open) return;
    setEntries(null);
    setSelected(new Set());
    setConnectionID(null);
    void loadConnections();
  }, [open, loadConnections]);

  // Keep the browse path on the connection's configured root when the user
  // switches connection. A status refresh replaces the object but must not
  // yank them back to the root while they are browsing.
  const connectionRoot = connection?.rootPath ?? "/";
  useEffect(() => {
    setPath(normalizeRoot(connectionRoot));
    setSelected(new Set());
  }, [connectionID, connectionRoot]);

  const loadEntries = useCallback(async (sourceID: number, target: string) => {
    const seq = ++requestSeq.current;
    setLoadingEntries(true);
    setError(null);
    try {
      const list = await browseCloud(sourceID, target);
      if (seq !== requestSeq.current) return;
      setEntries(list);
    } catch (e) {
      if (seq !== requestSeq.current) return;
      setEntries([]);
      setError(String(e));
    } finally {
      if (seq === requestSeq.current) setLoadingEntries(false);
    }
  }, []);

  useEffect(() => {
    if (!open || connection == null) return;
    void loadEntries(connection.id, path);
  }, [open, connection, path, loadEntries]);

  const navigate = (target: string) => {
    setPath(target);
    setSelected(new Set());
  };

  const root = connection ? normalizeRoot(connection.rootPath) : "/";
  const crumbs = useMemo(() => buildCrumbs(connection, root, path), [connection, root, path]);
  const parent = parentPath(root, path);

  const visibleEntries = useMemo(
    () =>
      (entries ?? []).filter(
        (entry) => entry.isDir || VIDEO_EXTENSION.test(entry.name),
      ),
    [entries],
  );
  const videoEntries = visibleEntries.filter((entry) => !entry.isDir);

  const toggle = (entryPath: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(entryPath)) next.delete(entryPath);
      else next.add(entryPath);
      return next;
    });
  };

  const toggleAll = () => {
    setSelected((prev) => {
      if (prev.size > 0) return new Set();
      return new Set(videoEntries.map((entry) => entry.path));
    });
  };

  const addSelected = async () => {
    if (connection == null || selected.size === 0 || adding) return;
    setAdding(true);
    setError(null);
    try {
      const results = await addCloudVideos(connection.id, [...selected]);
      onAdded(results);
      setSelected(new Set());
      onClose();
    } catch (e) {
      setError(String(e));
    } finally {
      setAdding(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="flex h-[620px] w-[780px] flex-col gap-0 p-0">
        <DialogHeader className="shrink-0 border-b border-[var(--border)] px-5 pb-3.5 pt-5 pr-12">
          <DialogTitle className="flex items-center gap-2">
            <Cloud className="h-[18px] w-[18px] text-[var(--accent)]" aria-hidden />
            从网盘添加
          </DialogTitle>
          <DialogDescription>
            浏览 WebDAV / OpenList 中的视频，加入收藏后仍由网盘保存原文件。Muse 不会修改网盘内容。
          </DialogDescription>
        </DialogHeader>

        {connections !== null && (enabledConnections?.length ?? 0) === 0 ? (
          <div className="flex flex-1 flex-col items-center justify-center px-8 text-center">
            <span className="mb-3 flex h-12 w-12 items-center justify-center rounded-full bg-[var(--panel)] text-[var(--muted)]">
              <Cloud className="h-5 w-5" aria-hidden />
            </span>
            <div className="text-sm font-medium">
              {connections.length === 0 ? "还没有配置网盘" : "没有启用的网盘"}
            </div>
            <p className="mt-1 max-w-[360px] text-xs leading-relaxed text-[var(--muted)]">
              {connections.length === 0
                ? "先在设置里添加一个 WebDAV 连接。比特球请先通过 OpenList 挂载，再填 OpenList 的 WebDAV 地址。"
                : "已停用的网盘只能查看归档，不能继续浏览或添加。到设置里重新启用后再回来。"}
            </p>
            <Button
              className="mt-4"
              variant="primary"
              onClick={() => {
                onClose();
                onOpenSettings();
              }}
            >
              去设置 → 网盘
            </Button>
          </div>
        ) : (
          <>
            <div className="flex shrink-0 items-center gap-2.5 border-b border-[var(--border)] px-5 py-3">
              <label className="sr-only" htmlFor="cloud-browser-connection">
                网盘连接
              </label>
              <select
                id="cloud-browser-connection"
                className="h-9 min-w-[170px] max-w-[240px] rounded-[var(--radius)] border border-[var(--border)] bg-[var(--panel)] px-2.5 text-sm text-[var(--text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]"
                value={connectionID ?? ""}
                onChange={(event) => setConnectionID(Number(event.target.value))}
                disabled={loadingConnections || adding}
              >
                {(enabledConnections ?? []).map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.name}
                    {item.online ? "" : "（离线）"}
                  </option>
                ))}
              </select>

              <div className="flex min-w-0 flex-1 items-center gap-0.5 overflow-hidden">
                {crumbs.map((crumb, index) => (
                  <span key={crumb.path} className="flex min-w-0 items-center">
                    {index > 0 && (
                      <ChevronRight className="mx-0.5 h-3.5 w-3.5 shrink-0 text-[var(--muted)]" aria-hidden />
                    )}
                    <button
                      type="button"
                      className={cn(
                        "truncate rounded px-1.5 py-0.5 text-xs transition-colors hover:bg-[var(--panel)] hover:text-[var(--text)]",
                        index === crumbs.length - 1 ? "text-[var(--text)]" : "text-[var(--muted)]",
                      )}
                      title={crumb.path}
                      onClick={() => navigate(crumb.path)}
                    >
                      {crumb.label}
                    </button>
                  </span>
                ))}
              </div>

              <Button
                size="sm"
                variant="ghost"
                className="shrink-0"
                onClick={() => connection && void loadEntries(connection.id, path)}
                disabled={connection == null || loadingEntries || adding}
                aria-label="刷新目录"
              >
                <RefreshCw className={cn("h-3.5 w-3.5", loadingEntries && "animate-spin")} aria-hidden />
                刷新
              </Button>
            </div>

            {connection?.online === false && (
              <div className="flex shrink-0 items-start gap-2 border-b border-[var(--border)] bg-[#e0b64f]/10 px-5 py-2 text-xs leading-relaxed text-[#e0b64f]">
                <AlertCircle className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden />
                <span>
                  连接当前显示为离线
                  {connection.lastError ? `：${connection.lastError}` : ""}。已收藏的视频不会删除，恢复连接后刷新即可。
                </span>
              </div>
            )}

            <div className="min-h-0 flex-1 overflow-y-auto px-3 py-2">
              {loadingEntries && entries === null ? (
                <div className="flex h-full items-center justify-center gap-2 text-sm text-[var(--muted)]">
                  <Loader2 className="h-4 w-4 animate-spin" aria-hidden />
                  正在读取目录…
                </div>
              ) : error ? (
                <div className="flex h-full flex-col items-center justify-center px-6 text-center">
                  <AlertCircle className="mb-2 h-5 w-5 text-[var(--danger)]" aria-hidden />
                  <p className="max-w-[460px] text-sm leading-relaxed text-[var(--danger)]">{error}</p>
                  <div className="mt-3 flex gap-2">
                    <Button
                      size="sm"
                      onClick={() => connection && void loadEntries(connection.id, path)}
                    >
                      重试
                    </Button>
                    <Button size="sm" variant="ghost" onClick={loadConnections}>
                      重新读取连接
                    </Button>
                  </div>
                </div>
              ) : visibleEntries.length === 0 ? (
                <div className="flex h-full flex-col items-center justify-center text-center text-[var(--muted)]">
                  <Folder className="mb-2 h-6 w-6 opacity-60" aria-hidden />
                  <div className="text-sm">这个目录里没有视频</div>
                  <div className="mt-0.5 text-xs">可以返回上一级，或把视频放进网盘后再刷新</div>
                </div>
              ) : (
                <ul className="m-0 flex list-none flex-col gap-0.5 p-0">
                  {parent && (
                    <li>
                      <button
                        type="button"
                        className="flex w-full items-center gap-2.5 rounded-[var(--radius)] px-3 py-2 text-left text-sm text-[var(--muted)] transition-colors hover:bg-[var(--panel)] hover:text-[var(--text)]"
                        onClick={() => navigate(parent)}
                      >
                        <Folder className="h-4 w-4" aria-hidden />
                        返回上一级
                      </button>
                    </li>
                  )}
                  {visibleEntries.map((entry) =>
                    entry.isDir ? (
                      <li key={entry.path}>
                        <button
                          type="button"
                          className="flex w-full items-center gap-2.5 rounded-[var(--radius)] px-3 py-2 text-left transition-colors hover:bg-[var(--panel)]"
                          onClick={() => navigate(entry.path)}
                          title={entry.path}
                        >
                          <Folder className="h-4 w-4 shrink-0 text-[var(--accent)]" aria-hidden />
                          <span className="min-w-0 flex-1 truncate text-sm">{entry.name}</span>
                          <ChevronRight className="h-4 w-4 shrink-0 text-[var(--muted)]" aria-hidden />
                        </button>
                      </li>
                    ) : (
                      <li key={entry.path}>
                        <label
                          className={cn(
                            "flex cursor-pointer items-center gap-2.5 rounded-[var(--radius)] px-3 py-2 transition-colors hover:bg-[var(--panel)]",
                            selected.has(entry.path) && "bg-[var(--panel)]",
                          )}
                          title={entry.path}
                        >
                          <input
                            type="checkbox"
                            className="h-3.5 w-3.5 shrink-0 accent-[var(--accent)]"
                            checked={selected.has(entry.path)}
                            onChange={() => toggle(entry.path)}
                          />
                          <FileVideo className="h-4 w-4 shrink-0 text-[var(--muted)]" aria-hidden />
                          <span className="min-w-0 flex-1 truncate text-sm">{entry.name}</span>
                          <span className="shrink-0 text-[11px] tabular-nums text-[var(--muted)]">
                            {formatFileSize(entry.size)}
                          </span>
                        </label>
                      </li>
                    ),
                  )}
                </ul>
              )}
            </div>

            <DialogFooter className="shrink-0 items-center border-t border-[var(--border)] px-5 py-3 sm:justify-between">
              <div className="flex items-center gap-3 text-xs text-[var(--muted)]">
                <button
                  type="button"
                  className="rounded px-1 py-0.5 transition-colors hover:text-[var(--text)] disabled:opacity-50"
                  onClick={toggleAll}
                  disabled={videoEntries.length === 0 || adding}
                >
                  {selected.size > 0 ? "取消全选" : "全选本页视频"}
                </button>
                <span>已选 {selected.size} 个</span>
              </div>
              <div className="flex items-center gap-2">
                <Button variant="ghost" onClick={onClose} disabled={adding}>
                  取消
                </Button>
                <Button
                  variant="primary"
                  onClick={() => void addSelected()}
                  disabled={selected.size === 0 || adding}
                >
                  {adding ? "正在加入…" : `加入收藏${selected.size > 0 ? `（${selected.size}）` : ""}`}
                </Button>
              </div>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}

function normalizeRoot(root: string): string {
  const trimmed = (root || "/").trim();
  if (!trimmed.startsWith("/")) return "/";
  if (trimmed === "/") return "/";
  return trimmed.replace(/\/+$/, "");
}

function parentPath(root: string, current: string): string | null {
  if (current === root) return null;
  const index = current.lastIndexOf("/");
  if (index <= 0) return "/";
  const next = current.slice(0, index) || "/";
  return next.length < root.length ? root : next;
}

function buildCrumbs(
  connection: CloudConnection | null,
  root: string,
  current: string,
): { label: string; path: string }[] {
  if (!connection) return [];
  const out = [{ label: connection.name, path: root }];
  if (current === root) return out;
  const relative = current.slice(root === "/" ? 1 : root.length + 1);
  let walked = root === "/" ? "" : root;
  for (const segment of relative.split("/").filter(Boolean)) {
    walked = `${walked}/${segment}`;
    out.push({ label: segment, path: walked });
  }
  return out;
}
