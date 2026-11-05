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
  scrapeVideosJav,
} from "../services/api";
import type { WatchedDir } from "../types";
import ConfirmDialog from "../components/ConfirmDialog";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";
import { cn } from "../lib/utils";
import { ArrowRight, Keyboard } from "lucide-react";

type SectionKey = "guide" | "server" | "watch" | "proxy" | "javbus" | "fc2";

const DEFAULT_PORT = 8899;

// The submenu, in reading order. The first entry is where a new user should
// land: a one-page orientation plus a replay of the tour.
const NAV: { key: SectionKey; label: string }[] = [
  { key: "guide", label: "新手引导" },
  { key: "server", label: "服务器" },
  { key: "watch", label: "目录监控" },
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
    <div className="flex h-screen flex-col">
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
