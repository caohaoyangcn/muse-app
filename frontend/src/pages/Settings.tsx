import { useCallback, useEffect, useState } from "react";
import {
  getServerPort,
  serverStatus,
  setServerPort,
  listWatchDirs,
  addWatchDir,
  removeWatchDir,
  pickWatchDir,
  rescanWatchDirs,
  getJavBusCookieStatus,
  setJavBusCookie,
  clearJavBusCookie,
  getFC2CookieStatus,
  setFC2Cookie,
  clearFC2Cookie,
  getScrapeProxy,
  setScrapeProxy,
  getTrashRetentionDays,
  setTrashRetentionDays,
  listTrash,
  purgeExpiredTrash,
  scrapeVideosJav,
  listCloudConnections,
  archiveCloudConnection,
  enableCloudConnection,
  saveCloudConnection,
  deleteCloudConnection,
  testCloudConnection,
  cloudStorageAvailable,
} from "../services/api";
import type {
  CloudConnection,
  CloudConnectionInput,
  CloudTestResult,
  WatchedDir,
} from "../types";
import { BrowserOpenURL } from "../../wailsjs/runtime";
import { invalidate } from "../services/live";
import ConfirmDialog from "../components/ConfirmDialog";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";
import { cn } from "../lib/utils";
import { ArrowRight, Cloud, Keyboard } from "lucide-react";

type SectionKey = "guide" | "server" | "watch" | "trash" | "cloud" | "proxy" | "javbus" | "fc2";

const DEFAULT_PORT = 8899;
const DEFAULT_TRASH_RETENTION_DAYS = 14;

// The submenu, in reading order. The first entry is where a new user should
// land: a one-page orientation plus a replay of the tour.
const NAV: { key: SectionKey; label: string }[] = [
  { key: "guide", label: "新手引导" },
  { key: "server", label: "服务器" },
  { key: "watch", label: "目录监控" },
  { key: "trash", label: "回收站" },
  { key: "cloud", label: "网盘（可选）" },
  { key: "proxy", label: "网络代理" },
  { key: "javbus", label: "JavBus" },
  { key: "fc2", label: "FC2CMADB" },
];

/**
 * Settings page with a left submenu / right detail layout. Besides the LAN
 * server port, it exposes the folder-watch list (目录监控) whose video files
 * are auto-imported into the favorites.
 *
 * The active submenu is controlled by the shell so the guided tour can open
 * 目录监控 / 网络代理 itself, and leave the user on a usable page afterwards.
 */
export default function Settings({
  onClose,
  onStartTour,
  section,
  onSectionChange,
}: {
  onClose: () => void;
  onStartTour: () => void;
  section: string;
  onSectionChange: (section: string) => void;
}) {
  // A section name from outside this page (the tour, a future deep link) is
  // only trusted when it is one this page can render; anything else lands on
  // the orientation page rather than an empty pane.
  const active: SectionKey = NAV.some((item) => item.key === section)
    ? (section as SectionKey)
    : "guide";

  return (
    <div className="flex h-full flex-col">
      <header className="flex items-center gap-3 border-b border-[var(--border)] bg-[var(--panel)] px-5 py-3">
        <h1 className="m-0 text-[17px] font-semibold tracking-[0.2px]">设置</h1>
        <Button className="ml-auto" onClick={onClose} aria-label="返回">
          ← 返回
        </Button>
      </header>
      <div className="flex min-h-0 flex-1">
        <nav
          data-tour="settings-nav"
          className="flex w-[200px] shrink-0 flex-col gap-0.5 border-r border-[var(--border)] bg-[var(--panel)] p-2.5"
          aria-label="设置分类"
        >
          {NAV.map((item) => (
            <Button
              key={item.key}
              variant="ghost"
              data-tour={`settings-${item.key}`}
              className={cn(
                "justify-start",
                active === item.key && "bg-[var(--panel-2)] text-[var(--text)]",
              )}
              onClick={() => onSectionChange(item.key)}
              aria-current={active === item.key ? "page" : undefined}
            >
              {item.label}
            </Button>
          ))}
        </nav>
        <main className="min-w-0 flex-1 overflow-y-auto p-6 pb-8">
          {active === "guide" && <GuideSection onStartTour={onStartTour} />}
          {active === "server" && <ServerSection />}
          {active === "watch" && <WatchSection />}
          {active === "trash" && <TrashSection />}
          {active === "cloud" && <CloudSection />}
          {active === "proxy" && <ProxySection />}
          {active === "javbus" && <JavBusSection />}
          {active === "fc2" && <FC2Section />}
        </main>
      </div>
    </div>
  );
}

/**
 * 新手引导: what the app is for, the two operations worth learning first, and a
 * replay. It is deliberately short — the tour does the walking, this page is
 * the place to come back to when the tour is long gone.
 */
function GuideSection({ onStartTour }: { onStartTour: () => void }) {
  return (
    <section className="max-w-[640px]">
      <h2 className="mb-1.5 text-base font-semibold">新手引导</h2>
      <p className="mb-5 max-w-[560px] leading-relaxed text-[var(--muted)]">
        用一两分钟把界面和常用操作过一遍。引导会依次指给你看分组、添加视频、刮削、手机访问，
        以及目录监控和代理这两处新手最容易卡住的地方。
      </p>

      <div className="flex flex-wrap items-center gap-3">
        <Button variant="primary" onClick={onStartTour}>
          重新播放新手引导
          <ArrowRight className="h-4 w-4" />
        </Button>
        <span className="text-[13px] text-[var(--muted)]">随时可以按 Esc 退出</span>
      </div>

      <div className="mt-6 rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg)] p-4">
        <h3 className="mb-2.5 flex items-center gap-2 text-sm font-semibold">
          <Keyboard className="h-4 w-4 text-[var(--accent)]" aria-hidden />
          常用快捷键
        </h3>
        <dl className="m-0 grid gap-2 text-[13px]">
          <Shortcut keys="Ctrl + F" action="把光标放进搜索框" />
          <Shortcut keys="Ctrl + A" action="全选当前列表里的视频" />
          <Shortcut keys="Enter" action="打开选中卡片的详情" />
          <Shortcut keys="空格" action="编辑选中卡片的信息" />
          <Shortcut keys="Alt + ← / →" action="后退 / 前进（在工作区标签之间切换）" />
          <Shortcut keys="Esc" action="关闭弹窗、菜单或新手引导" />
        </dl>
      </div>

      <p className="mt-5 max-w-[560px] text-[13px] leading-relaxed text-[var(--muted)]">
        整个引导只在第一次打开应用时自动出现一次；之后想重看，回到这里点上面的按钮就行。
      </p>
    </section>
  );
}

function Shortcut({ keys, action }: { keys: string; action: string }) {
  return (
    <div className="flex items-center gap-3">
      <dt className="shrink-0">
        <kbd className="rounded-[5px] border border-[var(--border)] bg-[var(--panel-2)] px-2 py-0.5 font-sans text-xs text-[var(--text)]">
          {keys}
        </kbd>
      </dt>
      <dd className="m-0 text-[var(--muted)]">{action}</dd>
    </div>
  );
}

function ServerSection() {
  const [port, setPort] = useState<string>("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [running, setRunning] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const [current, status] = await Promise.all([getServerPort(), serverStatus()]);
      setPort(String(current));
      setRunning(status?.running ?? false);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const save = async () => {
    const value = Number.parseInt(port.trim(), 10);
    if (!Number.isInteger(value) || value < 1 || value > 65535) {
      setError("请输入 1-65535 之间的端口号");
      return;
    }
    setError(null);
    setSaved(false);
    setSaving(true);
    try {
      await setServerPort(value);
      setSaved(true);
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  };

  return (
    <section className="max-w-[640px]">
      <h2 className="mb-1.5 text-base font-semibold">服务器</h2>
      <p className="mb-5 max-w-[560px] leading-relaxed text-[var(--muted)]">
        应用启动时会自动开启局域网 Web 服务器，顶部会显示手机可访问的地址。服务器使用固定端口，方便你记住访问地址；修改端口后需重新启动服务器才生效。
      </p>

      {loading ? (
        <div className="flex min-h-[320px] h-full items-center justify-center text-[var(--muted)]">加载中…</div>
      ) : (
        <div className="max-w-[440px]">
          <label className="mb-2 block font-medium" htmlFor="server-port">
            固定端口
          </label>
          <div className="flex items-center gap-2.5">
            <Input
              id="server-port"
              className="w-[150px]"
              type="number"
              min={1}
              max={65535}
              inputMode="numeric"
              value={port}
              onChange={(e) => {
                setPort(e.target.value);
                setSaved(false);
              }}
              placeholder="例：8899"
              disabled={running}
            />
            <Button variant="primary" onClick={() => void save()} disabled={saving || running}>
              {saving ? "保存中…" : "保存"}
            </Button>
            <Button
              onClick={() => {
                setPort(String(DEFAULT_PORT));
                setSaved(false);
              }}
              disabled={running}
            >
              恢复默认
            </Button>
          </div>
          {running ? (
            <p className="mt-2.5 text-[13px] text-[#e0b64f]">
              服务器正在运行，请先停止后再修改端口。
            </p>
          ) : (
            <p className="mt-2.5 text-[13px] text-[var(--muted)]">默认端口为 8899。</p>
          )}
          {saved && <p className="mt-2.5 text-[13px] text-[#7ad9a2]">已保存 ✓</p>}
          {error && <p className="mt-2.5 text-[13px] text-[var(--danger)]">{error}</p>}
        </div>
      )}
    </section>
  );
}

function TrashSection() {
  const [days, setDays] = useState("");
  const [count, setCount] = useState(0);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [purging, setPurging] = useState(false);
  const [saved, setSaved] = useState(false);
  const [purged, setPurged] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const [retention, trash] = await Promise.all([getTrashRetentionDays(), listTrash()]);
      setDays(String(retention));
      setCount(trash.videos.length);
      setError(null);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const save = async () => {
    const value = Number.parseInt(days.trim(), 10);
    if (!Number.isInteger(value) || value < 1 || value > 365) {
      setError("保留天数必须在 1-365 之间");
      return;
    }
    setError(null);
    setSaved(false);
    setPurged(null);
    setSaving(true);
    try {
      await setTrashRetentionDays(value);
      setDays(String(await getTrashRetentionDays()));
      setSaved(true);
      // A shorter retention may have made entries immediately due; the backend
      // purges in the background, so re-read the bin after a moment.
      window.setTimeout(() => void load(), 400);
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  };

  const purge = async () => {
    setError(null);
    setPurged(null);
    setPurging(true);
    try {
      const n = await purgeExpiredTrash();
      setPurged(n);
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setPurging(false);
    }
  };

  return (
    <section className="max-w-[640px]">
      <h2 className="mb-1.5 text-base font-semibold">回收站</h2>
      <p className="mb-5 max-w-[560px] leading-relaxed text-[var(--muted)]">
        从收藏中移除的影片会先进入回收站，而不是立刻销毁记录。回收站保留收藏的标题、点赞、分组和刮削资料；
        超过保留期后，应用会在启动时或后台定时检查中自动彻底清理。清理只删除 Muse 的记录与生成的缩略图/封面，
        <strong className="text-[var(--text)]">原始视频文件永远不会被删除</strong>。
      </p>

      {loading ? (
        <div className="flex min-h-[200px] items-center justify-center text-[var(--muted)]">加载中…</div>
      ) : (
        <div className="max-w-[460px]">
          <div className="mb-3 text-[13px] text-[var(--muted)]">当前回收站：{count} 项</div>
          <label className="mb-2 block font-medium" htmlFor="trash-retention">
            保留天数
          </label>
          <div className="flex items-center gap-2.5">
            <Input
              id="trash-retention"
              className="w-[150px]"
              type="number"
              min={1}
              max={365}
              inputMode="numeric"
              value={days}
              onChange={(e) => {
                setDays(e.target.value);
                setSaved(false);
                setPurged(null);
              }}
              placeholder={`例：${DEFAULT_TRASH_RETENTION_DAYS}`}
            />
            <Button variant="primary" onClick={() => void save()} disabled={saving}>
              {saving ? "保存中…" : "保存"}
            </Button>
            <Button
              onClick={() => {
                setDays(String(DEFAULT_TRASH_RETENTION_DAYS));
                setSaved(false);
                setPurged(null);
              }}
            >
              恢复默认
            </Button>
          </div>
          <p className="mt-2.5 text-[13px] text-[var(--muted)]">
            默认保留 {DEFAULT_TRASH_RETENTION_DAYS} 天，可设置 1-365 天。缩短保留期会立即清理已经超期的条目。
          </p>
          {saved && <p className="mt-2.5 text-[13px] text-[#7ad9a2]">已保存 ✓</p>}

          <div className="mt-6 flex flex-wrap items-center gap-3 border-t border-[var(--border)] pt-5">
            <Button onClick={() => void purge()} disabled={purging}>
              {purging ? "清理中…" : "立即清理超期条目"}
            </Button>
            {purged != null && (
              <span className="text-[13px] text-[var(--muted)]">
                {purged > 0 ? `已清理 ${purged} 项` : "没有超期条目"}
              </span>
            )}
          </div>
          {error && <p className="mt-2.5 text-[13px] text-[var(--danger)]">{error}</p>}
        </div>
      )}
    </section>
  );
}

function ProxySection() {
  const [proxy, setProxy] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      setProxy(await getScrapeProxy());
      setError(null);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const save = async () => {
    const value = proxy.trim();
    setError(null);
    setSaved(false);
    setSaving(true);
    try {
      await setScrapeProxy(value);
      // Show the canonical form the backend stored (e.g. scheme added).
      setProxy(await getScrapeProxy());
      setSaved(true);
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  };

  const clear = async () => {
    setError(null);
    setSaved(false);
    setSaving(true);
    try {
      await setScrapeProxy("");
      setProxy("");
      setSaved(true);
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  };

  return (
    <section data-tour="settings-panel-proxy" className="max-w-[640px]">
      <h2 className="mb-1.5 text-base font-semibold">网络代理</h2>
      <p className="mb-5 max-w-[560px] leading-relaxed text-[var(--muted)]">
        刮削请求（JavBus、FC2CMADB 及封面下载）默认直连。Muse 不会读取系统代理设置，
        需要在这里单独填写代理地址。保存后立即生效，无需重启应用。
      </p>

      {loading ? (
        <div className="flex min-h-[320px] h-full items-center justify-center text-[var(--muted)]">加载中…</div>
      ) : (
        <div className="max-w-[560px]">
          <label className="mb-2 block font-medium" htmlFor="scrape-proxy">
            代理地址
          </label>
          <Input
            id="scrape-proxy"
            className="w-full"
            value={proxy}
            onChange={(e) => {
              setProxy(e.target.value);
              setSaved(false);
              setError(null);
            }}
            placeholder="例：http://127.0.0.1:7890"
            autoComplete="off"
            spellCheck={false}
            aria-describedby="scrape-proxy-help"
          />
          <p id="scrape-proxy-help" className="mt-2.5 text-[13px] leading-relaxed text-[var(--muted)]">
            支持 <code>http</code>、<code>https</code>、<code>socks5</code>，端口必填；地址中可包含
            <code>用户名:密码@</code>。留空并保存即恢复直连（此时仍读取
            <code>HTTP_PROXY</code>/<code>HTTPS_PROXY</code> 环境变量）。
          </p>
          {proxy.trim() ? (
            <p className="mt-2.5 text-[13px] text-[var(--muted)]">
              当前为代理模式：若代理不可用，刮削任务会以 network 错误结束。
            </p>
          ) : (
            <p className="mt-2.5 text-[13px] text-[var(--muted)]">当前为直连模式。</p>
          )}

          <div className="mt-4 flex flex-wrap gap-2.5">
            <Button variant="primary" onClick={() => void save()} disabled={saving}>
              {saving ? "保存中…" : "保存"}
            </Button>
            <Button onClick={() => void clear()} disabled={saving || !proxy.trim()}>
              恢复直连
            </Button>
          </div>
          {saved && !error && <p className="mt-2.5 text-[13px] text-[#7ad9a2]">已保存 ✓</p>}
          {error && <p className="mt-2.5 text-[13px] text-[var(--danger)]">{error}</p>}
        </div>
      )}
    </section>
  );
}

function JavBusSection() {
  const [cookie, setCookie] = useState("");
  const [configured, setConfigured] = useState(false);
  const [showCookie, setShowCookie] = useState(false);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [clearing, setClearing] = useState(false);
  const [batchMode, setBatchMode] = useState<"pending" | "force" | null>(null);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const status = await getJavBusCookieStatus();
      setConfigured(status.configured);
      setError(null);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const save = async () => {
    const value = cookie.trim();
    if (!value) {
      setError("请输入完整的 Cookie 请求头内容");
      return;
    }

    setError(null);
    setSaved(false);
    setSaving(true);
    try {
      await setJavBusCookie(value);
      setConfigured(true);
      setCookie("");
      setShowCookie(false);
      setSaved(true);
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  };

  const clear = async () => {
    setError(null);
    setSaved(false);
    setClearing(true);
    try {
      await clearJavBusCookie();
      setConfigured(false);
      setCookie("");
      setShowCookie(false);
      setSaved(true);
    } catch (e) {
      setError(String(e));
    } finally {
      setClearing(false);
    }
  };

  const scrapeLibrary = async (force: boolean) => {
    setError(null);
    setSaved(false);
    setBatchMode(force ? "force" : "pending");
    try {
      const queued = await scrapeVideosJav(force);
      setSaved(true);
      setError(queued > 0 ? `已将 ${queued} 个视频加入刮削队列，任务将在后台处理。` : "没有需要加入队列的视频。 ");
    } catch (e) {
      setError(String(e));
    } finally {
      setBatchMode(null);
    }
  };

  return (
    <section className="max-w-[640px]">
      <h2 className="mb-1.5 text-base font-semibold">JavBus</h2>
      <p className="mb-5 max-w-[560px] leading-relaxed text-[var(--muted)]">
        配置 JavBus 刮削使用的 Cookie。请粘贴浏览器请求中的完整 Cookie 头内容，Cookie 只用于刮削请求，页面不会回填或显示已保存的真实值，也不会写入日志。
      </p>

      {loading ? (
        <div className="flex min-h-[320px] h-full items-center justify-center text-[var(--muted)]">加载中…</div>
      ) : (
        <div className="max-w-[560px]">
          <div className="mb-3 flex items-center gap-2 text-[13px]">
            <span className="text-[var(--muted)]">当前状态：</span>
            <span className={configured ? "text-[#7ad9a2]" : "text-[var(--muted)]"}>
              {configured ? "已配置" : "未配置"}
            </span>
          </div>

          <label className="mb-2 block font-medium" htmlFor="javbus-cookie">
            Cookie
          </label>
          <div className="flex items-center gap-2.5">
            <Input
              id="javbus-cookie"
              className="min-w-0 flex-1"
              type={showCookie ? "text" : "password"}
              value={cookie}
              onChange={(e) => {
                setCookie(e.target.value);
                setSaved(false);
                setError(null);
              }}
              placeholder="粘贴完整 Cookie 头内容"
              autoComplete="off"
              spellCheck={false}
              aria-describedby="javbus-cookie-help"
            />
            <Button
              onClick={() => setShowCookie((visible) => !visible)}
              disabled={!cookie}
              aria-label={showCookie ? "隐藏 Cookie" : "显示 Cookie"}
              aria-pressed={showCookie}
            >
              {showCookie ? "隐藏" : "显示"}
            </Button>
          </div>
          <p id="javbus-cookie-help" className="mt-2.5 text-[13px] leading-relaxed text-[var(--muted)]">
            只粘贴请求头中 <code>Cookie:</code> 后面的完整内容，例如 <code>PHPSESSID=…; existmag=…</code>。保存后输入框会清空，仅保留“已配置”状态。
          </p>

          <div className="mt-4 flex flex-wrap gap-2.5">
            <Button variant="primary" onClick={() => void save()} disabled={saving || clearing || batchMode !== null || !cookie.trim()}>
              {saving ? "保存中…" : "保存 Cookie"}
            </Button>
            <Button onClick={() => void clear()} disabled={saving || clearing || batchMode !== null || !configured}>
              {clearing ? "清除中…" : "清除 Cookie"}
            </Button>
          </div>

          <div className="mt-6 border-t border-[var(--border)] pt-5">
            <h3 className="text-sm font-semibold">批量刮削</h3>
            <p className="mt-1.5 text-[13px] leading-relaxed text-[var(--muted)]">
              任务会异步加入后台队列。未刮削模式跳过已成功和已设置为跳过的视频，其他失败状态会重新尝试；强制模式会重新处理整个影片库。
            </p>
            <div className="mt-3 flex flex-wrap gap-2.5">
              <Button onClick={() => void scrapeLibrary(false)} disabled={saving || clearing || batchMode !== null}>
                {batchMode === "pending" ? "入队中…" : "刮削未处理视频"}
              </Button>
              <Button variant="primary" onClick={() => void scrapeLibrary(true)} disabled={saving || clearing || batchMode !== null}>
                {batchMode === "force" ? "入队中…" : "全库强制重刮削"}
              </Button>
            </div>
          </div>

          {saved && !error && <p className="mt-2.5 text-[13px] text-[#7ad9a2]">操作成功 ✓</p>}
          {error && <p className={cn("mt-2.5 text-[13px]", error.includes("没有需要") ? "text-[var(--muted)]" : "text-[var(--danger)]")}>{error}</p>}
        </div>
      )}
    </section>
  );
}

function FC2Section() {
  const [cookie, setCookie] = useState("");
  const [configured, setConfigured] = useState(false);
  const [showCookie, setShowCookie] = useState(false);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [clearing, setClearing] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const status = await getFC2CookieStatus();
      setConfigured(status.configured);
      setError(null);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const save = async () => {
    const value = cookie.trim();
    if (!value) {
      setError("请输入完整的 Cookie 请求头内容");
      return;
    }
    setError(null);
    setSaved(false);
    setSaving(true);
    try {
      await setFC2Cookie(value);
      setConfigured(true);
      setCookie("");
      setShowCookie(false);
      setSaved(true);
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  };

  const clear = async () => {
    setError(null);
    setSaved(false);
    setClearing(true);
    try {
      await clearFC2Cookie();
      setConfigured(false);
      setCookie("");
      setShowCookie(false);
      setSaved(true);
    } catch (e) {
      setError(String(e));
    } finally {
      setClearing(false);
    }
  };

  return (
    <section className="max-w-[640px]">
      <h2 className="mb-1.5 text-base font-semibold">FC2CMADB</h2>
      <p className="mb-5 max-w-[560px] leading-relaxed text-[var(--muted)]">
        FC2 文件名中的数字会自动查询 FC2CMADB 文章页。请粘贴已登录且完成年龄确认的浏览器请求中的完整 Cookie；Cookie 只在 Windows DPAPI 保护后保存，不会回显或写入普通日志。
      </p>
      {loading ? (
        <div className="flex min-h-[320px] h-full items-center justify-center text-[var(--muted)]">加载中…</div>
      ) : (
        <div className="max-w-[560px]">
          <div className="mb-3 flex items-center gap-2 text-[13px]">
            <span className="text-[var(--muted)]">当前状态：</span>
            <span className={configured ? "text-[#7ad9a2]" : "text-[var(--muted)]"}>
              {configured ? "已配置" : "未配置"}
            </span>
          </div>
          <label className="mb-2 block font-medium" htmlFor="fc2-cookie">Cookie</label>
          <div className="flex items-center gap-2.5">
            <Input
              id="fc2-cookie"
              className="min-w-0 flex-1"
              type={showCookie ? "text" : "password"}
              value={cookie}
              onChange={(e) => {
                setCookie(e.target.value);
                setSaved(false);
                setError(null);
              }}
              placeholder="粘贴完整 Cookie 头内容"
              autoComplete="off"
              spellCheck={false}
            />
            <Button onClick={() => setShowCookie((visible) => !visible)} disabled={!cookie} aria-label={showCookie ? "隐藏 Cookie" : "显示 Cookie"} aria-pressed={showCookie}>
              {showCookie ? "隐藏" : "显示"}
            </Button>
          </div>
          <p className="mt-2.5 text-[13px] leading-relaxed text-[var(--muted)]">
            至少包含浏览器中 fc2cmadb.com 请求使用的会话 Cookie 和 <code>ageVerified=true</code>。Cloudflare 会话过期后请重新复制 Cookie。
          </p>
          <div className="mt-4 flex flex-wrap gap-2.5">
            <Button variant="primary" onClick={() => void save()} disabled={saving || clearing || !cookie.trim()}>
              {saving ? "保存中…" : "保存 Cookie"}
            </Button>
            <Button onClick={() => void clear()} disabled={saving || clearing || !configured}>
              {clearing ? "清除中…" : "清除 Cookie"}
            </Button>
          </div>
          {saved && !error && <p className="mt-2.5 text-[13px] text-[#7ad9a2]">操作成功 ✓</p>}
          {error && <p className="mt-2.5 text-[13px] text-[var(--danger)]">{error}</p>}
        </div>
      )}
    </section>
  );
}

function WatchSection() {
  const [dirs, setDirs] = useState<WatchedDir[]>([]);
  const [loading, setLoading] = useState(true);
  const [adding, setAdding] = useState(false);
  const [scanning, setScanning] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [removeTarget, setRemoveTarget] = useState<WatchedDir | null>(null);

  const load = useCallback(async () => {
    try {
      setDirs(await listWatchDirs());
      setError(null);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const add = async () => {
    setError(null);
    setAdding(true);
    try {
      const dir = await pickWatchDir();
      // The user cancelled the picker — nothing to do.
      if (dir) await addWatchDir(dir);
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setAdding(false);
    }
  };

  const rescan = async () => {
    setError(null);
    setScanning(true);
    try {
      // The scan itself is asynchronous in the backend; newly imported videos
      // surface in the library via the watch:scanned event.
      await rescanWatchDirs();
    } catch (e) {
      setError(String(e));
    } finally {
      setScanning(false);
    }
  };

  const remove = async () => {
    if (!removeTarget) return;
    try {
      await removeWatchDir(removeTarget.id);
      setRemoveTarget(null);
      await load();
    } catch (e) {
      setError(String(e));
      setRemoveTarget(null);
    }
  };

  return (
    <section data-tour="settings-panel-watch" className="max-w-[640px]">
      <h2 className="mb-1.5 text-base font-semibold">目录监控</h2>
      <p className="mb-5 max-w-[560px] leading-relaxed text-[var(--muted)]">
        添加自动监控的目录后，应用会递归扫描其中的视频文件并加入收藏列表；之后该目录下新增的视频文件也会被自动收录。
        移除目录不会删除已收藏的视频。
      </p>

      {loading ? (
        <div className="flex min-h-[320px] h-full items-center justify-center text-[var(--muted)]">加载中…</div>
      ) : (
        <>
          <div className="mt-3.5 flex gap-2.5">
            <Button variant="primary" onClick={() => void add()} disabled={adding}>
              {adding ? "添加中…" : "+ 添加目录"}
            </Button>
            <Button
              onClick={() => void rescan()}
              disabled={scanning || dirs.length === 0}
              title="手动重新扫描所有监控目录"
            >
              {scanning ? "扫描中…" : "立即扫描"}
            </Button>
          </div>
          {dirs.length === 0 ? (
            <p className="mt-2.5 text-[13px] text-[var(--muted)]">还没有监控目录。</p>
          ) : (
            <ul className="mt-3.5 flex list-none flex-col gap-2 p-0">
              {dirs.map((d) => (
                <li key={d.id} className="flex items-center gap-3 rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg)] p-2.5 px-3">
                  <div className="flex min-w-0 flex-1 flex-col gap-0.5">
                    <span className="overflow-hidden text-ellipsis whitespace-nowrap text-[13px]" title={d.path}>
                      {d.path}
                    </span>
                    <span className="text-xs text-[var(--muted)]">
                      {d.lastScanAt > 0
                        ? `上次扫描：${new Date(d.lastScanAt).toLocaleString()}`
                        : "尚未扫描"}
                    </span>
                  </div>
                  <Button
                    size="sm"
                    className="shrink-0"
                    onClick={() => setRemoveTarget(d)}
                    aria-label={`移除监控目录 ${d.path}`}
                  >
                    移除
                  </Button>
                </li>
              ))}
            </ul>
          )}
          {error && <p className="mt-2.5 text-[13px] text-[var(--danger)]">{error}</p>}
        </>
      )}

      {removeTarget && (
        <ConfirmDialog
          title="移除监控目录"
          message={`不再监控"${removeTarget.path}"？已收藏的视频会保留。`}
          confirmLabel="移除"
          cancelLabel="取消"
          danger
          onConfirm={() => void remove()}
          onCancel={() => setRemoveTarget(null)}
        />
      )}
    </section>
  );
}

// ---- WebDAV 网盘 ---------------------------------------------------------

type ConnectionDraft = {
  id: number;
  name: string;
  endpoint: string;
  webUrl: string;
  username: string;
  password: string;
  clearPassword: boolean;
  rootPath: string;
  verifyTls: boolean;
};

function blankDraft(): ConnectionDraft {
  return {
    id: 0,
    name: "",
    endpoint: "",
    webUrl: "",
    username: "",
    password: "",
    clearPassword: false,
    rootPath: "/",
    verifyTls: true,
  };
}

function draftFrom(c: CloudConnection): ConnectionDraft {
  return {
    id: c.id,
    name: c.name,
    endpoint: c.endpoint,
    webUrl: c.webUrl,
    username: c.username,
    password: "",
    clearPassword: false,
    rootPath: c.rootPath || "/",
    verifyTls: c.verifyTls,
  };
}

/**
 * 网盘（可选）: 只读 WebDAV 连接管理。Local-only users never need this page;
 * BitQiu users connect through OpenList's WebDAV endpoint here. Passwords are
 * write-only from the UI's point of view: the backend returns HasPassword, never
 * the secret.
 */
function CloudSection() {
  const [connections, setConnections] = useState<CloudConnection[]>([]);
  const [storageAvailable, setStorageAvailable] = useState(true);
  const [loading, setLoading] = useState(true);
  const [draft, setDraft] = useState<ConnectionDraft | null>(null);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [draftResult, setDraftResult] = useState<CloudTestResult | null>(null);
  const [rowTesting, setRowTesting] = useState<number | null>(null);
  const [rowResults, setRowResults] = useState<Record<number, CloudTestResult>>({});
  const [deleteTarget, setDeleteTarget] = useState<CloudConnection | null>(null);
  const [archiveTarget, setArchiveTarget] = useState<CloudConnection | null>(null);
  const [archiving, setArchiving] = useState(false);
  const [actionMessage, setActionMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const [list, storage] = await Promise.all([listCloudConnections(), cloudStorageAvailable()]);
      setConnections(list);
      setStorageAvailable(storage);
      setError(null);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const updateDraft = (patch: Partial<ConnectionDraft>) => {
    setDraft((current) => (current ? { ...current, ...patch } : current));
    setDraftResult(null);
  };

  const toInput = (value: ConnectionDraft): CloudConnectionInput => ({
    id: value.id,
    name: value.name,
    kind: "webdav",
    endpoint: value.endpoint,
    webUrl: value.webUrl,
    username: value.username,
    password: value.password,
    clearPassword: value.clearPassword,
    rootPath: value.rootPath,
    verifyTls: value.verifyTls,
  });

  const testDraft = async () => {
    if (!draft || testing || saving) return;
    setTesting(true);
    setDraftResult(null);
    try {
      setDraftResult(await testCloudConnection(toInput(draft)));
    } catch (e) {
      setDraftResult({ ok: false, latencyMs: 0, error: String(e) });
    } finally {
      setTesting(false);
    }
  };

  const saveDraft = async () => {
    if (!draft || saving || testing) return;
    setSaving(true);
    setError(null);
    try {
      await saveCloudConnection(toInput(draft));
      setDraft(null);
      setDraftResult(null);
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  };

  const testExisting = async (connection: CloudConnection) => {
    if (rowTesting != null) return;
    setRowTesting(connection.id);
    try {
      const result = await testCloudConnection({
        id: connection.id,
        name: connection.name,
        kind: "webdav",
        endpoint: connection.endpoint,
        webUrl: connection.webUrl,
        username: connection.username,
        password: "",
        clearPassword: false,
        rootPath: connection.rootPath,
        verifyTls: connection.verifyTls,
      });
      setRowResults((prev) => ({ ...prev, [connection.id]: result }));
      await load();
    } catch (e) {
      setRowResults((prev) => ({
        ...prev,
        [connection.id]: { ok: false, latencyMs: 0, error: String(e) },
      }));
    } finally {
      setRowTesting(null);
    }
  };

  const archive = async () => {
    if (!archiveTarget) return;
    const target = archiveTarget;
    setArchiveTarget(null);
    setArchiving(true);
    try {
      const count = await archiveCloudConnection(target.id);
      invalidate();
      await load();
      setError(null);
      setActionMessage(
        count > 0 ? `已停用「${target.name}」，归档 ${count} 个收藏` : `已停用「${target.name}」`,
      );
    } catch (e) {
      setError(String(e));
    } finally {
      setArchiving(false);
    }
  };

  const enable = async (connection: CloudConnection) => {
    try {
      await enableCloudConnection(connection.id);
      invalidate();
      await load();
      setActionMessage("已启用；归档不会自动恢复到片库");
    } catch (e) {
      setError(String(e));
    }
  };

  const remove = async () => {
    if (!deleteTarget) return;
    try {
      await deleteCloudConnection(deleteTarget.id);
      setDeleteTarget(null);
      await load();
    } catch (e) {
      setError(String(e));
      setDeleteTarget(null);
    }
  };

  return (
    <section className="max-w-[720px]">
      <h2 className="mb-1.5 text-base font-semibold">网盘（可选）</h2>
      <p className="mb-4 max-w-[620px] leading-relaxed text-[var(--muted)]">
        Muse 只读浏览标准 WebDAV，不改名、不移动、不删除网盘文件。比特球没有公开
        WebDAV，需要先由 OpenList 挂载，再填写 OpenList 的 WebDAV 地址（通常以
        <code className="mx-1">/dav</code> 结尾）。不用网盘的用户可以完全忽略这一页。
      </p>

      {loading ? (
        <div className="flex min-h-[260px] items-center justify-center text-[var(--muted)]">加载中…</div>
      ) : draft ? (
        <div className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg)] p-4">
          <div className="mb-3 flex items-center justify-between gap-3">
            <h3 className="m-0 text-sm font-semibold">{draft.id > 0 ? "编辑网盘连接" : "添加网盘连接"}</h3>
            <Button size="sm" variant="ghost" onClick={() => setDraft(null)} disabled={saving || testing}>
              取消
            </Button>
          </div>

          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="名称" htmlFor="cloud-name">
              <Input
                id="cloud-name"
                value={draft.name}
                onChange={(e) => updateDraft({ name: e.target.value })}
                placeholder="例如：比特球"
                autoFocus
              />
            </Field>
            <Field label="根目录" htmlFor="cloud-root">
              <Input
                id="cloud-root"
                value={draft.rootPath}
                onChange={(e) => updateDraft({ rootPath: e.target.value })}
                placeholder="/"
                spellCheck={false}
              />
            </Field>
          </div>

          <div className="mt-3">
            <Field label="WebDAV 地址" htmlFor="cloud-endpoint">
              <Input
                id="cloud-endpoint"
                value={draft.endpoint}
                onChange={(e) => updateDraft({ endpoint: e.target.value })}
                placeholder="http://127.0.0.1:5244/dav"
                spellCheck={false}
                autoComplete="url"
              />
            </Field>
          </div>

          <div className="mt-3">
            <Field label="OpenList 网页地址（可选）" htmlFor="cloud-web">
              <Input
                id="cloud-web"
                value={draft.webUrl}
                onChange={(e) => updateDraft({ webUrl: e.target.value })}
                placeholder="http://127.0.0.1:5244"
                spellCheck={false}
                autoComplete="url"
              />
            </Field>
            <p className="mt-1 text-[11px] leading-relaxed text-[var(--muted)]">
              留空时从 WebDAV 地址推导；用于「在 OpenList 中打开」。
            </p>
          </div>

          <div className="mt-3 grid gap-3 sm:grid-cols-2">
            <Field label="用户名（可选）" htmlFor="cloud-user">
              <Input
                id="cloud-user"
                value={draft.username}
                onChange={(e) => updateDraft({ username: e.target.value })}
                autoComplete="username"
                spellCheck={false}
              />
            </Field>
            <Field label="密码（可选）" htmlFor="cloud-password">
              <Input
                id="cloud-password"
                type="password"
                value={draft.password}
                onChange={(e) => updateDraft({ password: e.target.value })}
                placeholder={
                  !storageAvailable
                    ? "当前平台不支持安全保存密码"
                    : draft.id > 0
                      ? "留空保持原密码"
                      : "留空表示匿名访问"
                }
                disabled={!storageAvailable}
                autoComplete="new-password"
              />
            </Field>
          </div>

          <label className="mt-3 flex items-start gap-2 text-[13px] leading-relaxed">
            <input
              type="checkbox"
              className="mt-0.5 h-3.5 w-3.5 shrink-0 accent-[var(--accent)]"
              checked={draft.verifyTls}
              onChange={(e) => updateDraft({ verifyTls: e.target.checked })}
            />
            <span>
              校验 HTTPS 证书
              <span className="ml-1 text-[var(--muted)]">（默认开启；仅自签名 OpenList 才需要关闭）</span>
            </span>
          </label>

          {draft.id > 0 && connections.find((c) => c.id === draft.id)?.hasPassword && (
            <label className="mt-2 flex items-start gap-2 text-[13px] leading-relaxed">
              <input
                type="checkbox"
                className="mt-0.5 h-3.5 w-3.5 shrink-0 accent-[var(--accent)]"
                checked={draft.clearPassword}
                onChange={(e) => updateDraft({ clearPassword: e.target.checked, password: "" })}
              />
              <span>清除已保存的密码（改为匿名访问）</span>
            </label>
          )}

          {!storageAvailable && (
            <p className="mt-2 rounded-[var(--radius)] border border-[#e0b64f]/40 bg-[#e0b64f]/10 px-3 py-2 text-xs leading-relaxed text-[#e0b64f]">
              当前平台不能安全保存密码，只能使用匿名 WebDAV。Windows 版支持 DPAPI 加密保存。
            </p>
          )}

          <div className="mt-4 flex flex-wrap items-center gap-2.5">
            <Button variant="primary" onClick={() => void saveDraft()} disabled={saving || testing}>
              {saving ? "保存中…" : "保存连接"}
            </Button>
            <Button onClick={() => void testDraft()} disabled={saving || testing || !draft.endpoint.trim()}>
              {testing ? "测试中…" : "测试连接"}
            </Button>
            {draftResult && <TestBadge result={draftResult} />}
          </div>
        </div>
      ) : (
        <>
          <div className="flex flex-wrap gap-2.5">
            <Button variant="primary" onClick={() => { setDraft(blankDraft()); setDraftResult(null); }}>
              + 添加网盘连接
            </Button>
            <Button onClick={() => void load()}>刷新状态</Button>
          </div>

          {connections.length === 0 ? (
            <div className="mt-4 rounded-[var(--radius)] border border-dashed border-[var(--border)] px-4 py-8 text-center">
              <Cloud className="mx-auto mb-2 h-6 w-6 text-[var(--muted)]" aria-hidden />
              <p className="m-0 text-sm">还没有网盘连接</p>
              <p className="m-0 mt-1 text-xs text-[var(--muted)]">
                只用本地硬盘的话不需要配置；需要比特球时先启动 OpenList。
              </p>
            </div>
          ) : (
            <ul className="mt-4 flex list-none flex-col gap-2 p-0">
              {connections.map((connection) => {
                const result = rowResults[connection.id];
                const webURL = managementURL(connection);
                return (
                  <li
                    key={connection.id}
                    className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg)] px-3.5 py-3"
                  >
                    <div className="flex flex-wrap items-center gap-2">
                      <span
                        className={cn(
                          "h-2 w-2 shrink-0 rounded-full",
                          !connection.enabled
                            ? "bg-[var(--muted)]"
                            : connection.online
                              ? "bg-[#7ad9a2]"
                              : "bg-[#e0b64f]",
                        )}
                        aria-hidden
                      />
                      <span className="text-sm font-medium">{connection.name}</span>
                      <span
                        className={cn(
                          "text-[11px]",
                          !connection.enabled
                            ? "text-[var(--muted)]"
                            : connection.online
                              ? "text-[#7ad9a2]"
                              : "text-[#e0b64f]",
                        )}
                      >
                        {!connection.enabled ? "已停用 · 已归档" : connection.online ? "在线" : "离线"}
                      </span>
                      <span className="text-[11px] text-[var(--muted)]">
                        {connection.enabled ? `${connection.videoCount} 个收藏` : "归档保留中"}
                      </span>
                      <div className="ml-auto flex flex-wrap items-center gap-1.5">
                        {webURL && (
                          <Button size="sm" variant="ghost" onClick={() => BrowserOpenURL(webURL)}>
                            打开管理页
                          </Button>
                        )}
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => void testExisting(connection)}
                          disabled={rowTesting != null || !connection.enabled}
                        >
                          {rowTesting === connection.id ? "测试中…" : "测试"}
                        </Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => {
                            if (connection.enabled) setArchiveTarget(connection);
                            else void enable(connection);
                          }}
                          disabled={archiving}
                        >
                          {connection.enabled ? "停用并归档" : "启用"}
                        </Button>
                        <Button
                          size="sm"
                          onClick={() => {
                            setDraft(draftFrom(connection));
                            setDraftResult(null);
                          }}
                        >
                          编辑
                        </Button>
                        <Button size="sm" variant="ghost" onClick={() => setDeleteTarget(connection)}>
                          删除
                        </Button>
                      </div>
                    </div>
                    <div className="mt-1 break-all text-xs text-[var(--muted)]" title={connection.endpoint}>
                      {connection.endpoint}
                      {connection.rootPath && connection.rootPath !== "/" && (
                        <span className="ml-2 text-[var(--muted)]">根目录 {connection.rootPath}</span>
                      )}
                    </div>
                    {(result || connection.lastError) && (
                      <div className="mt-1.5 text-[11px] leading-relaxed">
                        {result ? (
                          <TestBadge result={result} />
                        ) : (
                          <span className="text-[#e0b64f]">{connection.lastError}</span>
                        )}
                      </div>
                    )}
                  </li>
                );
              })}
            </ul>
          )}

          <p className="mt-4 max-w-[620px] text-xs leading-relaxed text-[var(--muted)]">
            OpenList 是可选组件，不随 Muse 安装包分发。Muse 只连接它提供的 WebDAV 地址；挂载、改名、移动等管理操作请在 OpenList 网页完成。
          </p>

        </>
      )}

      {actionMessage && <p className="mt-3 text-[13px] text-[#7ad9a2]">{actionMessage}</p>}
      {error && <p className="mt-3 text-[13px] text-[var(--danger)]">{error}</p>}

      {archiveTarget && (
        <ConfirmDialog
          title="停用网盘并归档"
          message={`停用「${archiveTarget.name}」后，其 ${archiveTarget.videoCount} 个活动收藏会移入回收站的「归档」区：不再联网、不能播放，长期保留记录和封面。\n\n归档不会自动恢复到片库；需要时可在回收站手动恢复。继续？`}
          confirmLabel="停用并归档"
          cancelLabel="取消"
          danger
          onConfirm={() => void archive()}
          onCancel={() => setArchiveTarget(null)}
        />
      )}

      {deleteTarget && (
        <ConfirmDialog
          title="删除网盘连接"
          message={
            deleteTarget.videoCount > 0
              ? `「${deleteTarget.name}」下还有 ${deleteTarget.videoCount} 个活动收藏，请先停用归档或移除这些收藏。`
              : deleteTarget.enabled
                ? `删除「${deleteTarget.name}」？只会删除连接配置，不会删除网盘上的任何文件。`
                : `删除「${deleteTarget.name}」？连接会被移除，但已归档的收藏会继续保留在「归档与回收站」；网盘文件不会被删除。`
          }
          confirmLabel="删除"
          cancelLabel="取消"
          danger
          onConfirm={() => void remove()}
          onCancel={() => setDeleteTarget(null)}
        />
      )}
    </section>
  );
}

function Field({
  label,
  htmlFor,
  children,
}: {
  label: string;
  htmlFor: string;
  children: React.ReactNode;
}) {
  return (
    <div>
      <label className="mb-1.5 block text-[13px] text-[var(--muted)]" htmlFor={htmlFor}>
        {label}
      </label>
      {children}
    </div>
  );
}

function TestBadge({ result }: { result: CloudTestResult }) {
  if (result.ok) {
    return (
      <span className="text-[#7ad9a2]">
        连接成功 ✓ {result.latencyMs > 0 ? `${result.latencyMs} ms` : ""}
      </span>
    );
  }
  return <span className="text-[var(--danger)]">{result.error || "连接失败"}</span>;
}

/** Derive the OpenList management page from the WebDAV endpoint when web_url
 *  is empty, mirroring the backend rule (strip a trailing /dav). */
function managementURL(connection: CloudConnection): string {
  const explicit = connection.webUrl.trim();
  if (explicit) return explicit;
  try {
    const url = new URL(connection.endpoint);
    url.pathname = url.pathname.replace(/\/dav\/?$/i, "") || "/";
    url.search = "";
    url.hash = "";
    return url.toString().replace(/\/$/, "");
  } catch {
    return "";
  }
}
