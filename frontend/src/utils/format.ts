import type { ActorProfile, Highlight, HighlightPreview, Video } from "../types";

/** 7423420 -> "2:03:43"; returns "" when duration is unknown. */
export function formatDuration(ms: number): string {
  if (!ms || ms <= 0) return "";
  const total = Math.round(ms / 1000);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  const ss = String(s).padStart(2, "0");
  if (h > 0) return `${h}:${String(m).padStart(2, "0")}:${ss}`;
  return `${m}:${ss}`;
}

export function formatFileSize(bytes: number): string {
  if (!bytes || bytes <= 0) return "";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = bytes;
  let i = 0;
  while (value >= 1024 && i < units.length - 1) {
    value /= 1024;
    i++;
  }
  return `${value >= 100 ? Math.round(value) : value.toFixed(1)} ${units[i]}`;
}

export function baseName(p: string): string {
  const norm = p.replace(/[\\/]+$/, "");
  const idx = Math.max(norm.lastIndexOf("\\"), norm.lastIndexOf("/"));
  return idx >= 0 ? norm.slice(idx + 1) : norm;
}

export function videoResolution(v: Video): string {
  return v.width > 0 && v.height > 0 ? `${v.width}×${v.height}` : "";
}

export function videoCoverURL(v: Video): string {
  const version = v.customCoverAt || v.updatedAt || "";
  if (v.coverSource === "scraped" && v.scrapedCoverPath) {
    return `/${v.scrapedCoverPath}?v=${version}`;
  }
  return `/thumbnails/${v.id}.jpg?v=${version}`;
}

/**
 * 一条精彩时刻生成图片的地址。
 *
 * 生成文件落在 `<dataDir>/highlights/` 下，桌面端与局域网服务都以 `/highlights/`
 * 暴露同一个相对路径，所以两端共用这一种拼法。`updatedAt` 进查询串是因为 8 fps
 * 重试会覆盖同一个文件名，没有它 WebView 会继续显示旧图。只在 `ready` 时调用。
 */
export function highlightURL(h: Highlight): string {
  return `/${h.mediaPath}?v=${h.updatedAt || h.createdAt}`;
}

/**
 * 一张待确认预览的地址。
 *
 * 预览文件名本身每次生成都不同（带编号），所以不需要像正式图片那样再挂一个
 * `?v=` 反缓存参数：换一张预览就是换一个 URL。
 */
export function highlightPreviewURL(preview: HighlightPreview): string {
  return `/${preview.path}`;
}

/** 23.976543 -> "23.98 fps"; returns "" when the frame rate is unknown. */
export function formatFrameRate(fps: number): string {
  if (!fps || fps <= 0) return "";
  const trimmed = fps >= 100 ? Math.round(fps) : Math.round(fps * 100) / 100;
  return `${trimmed} fps`;
}

/** Bit rate (bits per second) -> "8.00 Mbps"; returns "" when unknown. */
export function formatBitrate(bitrate: number): string {
  if (!bitrate || bitrate <= 0) return "";
  const mbps = bitrate / 1_000_000;
  return mbps >= 100
    ? `${Math.round(mbps)} Mbps`
    : `${Math.round(mbps * 100) / 100} Mbps`;
}

/**
 * 演员头像的地址。优先用后端缓存到本地的文件（离线也能显示，且不受图床防盗链
 * 影响）；没有缓存时才回退到刮削时记下的远程地址。
 */
export function actorAvatarURL(profile?: ActorProfile | null): string {
  if (!profile) return "";
  if (profile.avatarPath) {
    return `/${profile.avatarPath}?v=${profile.fetchedAt ?? ""}`;
  }
  return profile.avatarUrl ?? "";
}

/** 生日 · 身高 · 三围 · 出演数 · 出身，只保留数据源确实给出的项。 */
export function actorProfileMeta(profile: ActorProfile): string {
  const parts: string[] = [];
  if (profile.birthday) parts.push(profile.birthday);
  if (profile.heightCm) parts.push(`${profile.heightCm}cm`);
  const size = [
    profile.bustCm ? `B${profile.bustCm}` : "",
    profile.cup ? `(${profile.cup})` : "",
    profile.waistCm ? `W${profile.waistCm}` : "",
    profile.hipCm ? `H${profile.hipCm}` : "",
  ]
    .filter(Boolean)
    .join(" ");
  if (size) parts.push(size);
  if (profile.movieCount) parts.push(`出演 ${profile.movieCount} 部`);
  if (profile.hometown) parts.push(profile.hometown);
  return parts.join(" · ");
}
