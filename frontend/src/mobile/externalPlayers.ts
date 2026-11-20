import type { Video } from "../types";
import { currentAccessKey } from "../services/web";

export type ExternalPlayerID = "infuse" | "vlc";

export type ExternalPlayer = {
  id: ExternalPlayerID;
  name: string;
  description: string;
  appStoreURL: string;
  buildURL: (video: Video, streamURL: string) => string;
};

const PREFERRED_PLAYER_KEY = "muse.preferredExternalPlayer";

export const externalPlayers: ExternalPlayer[] = [
  {
    id: "infuse",
    name: "Infuse",
    description: "Apple 设备上的全能播放器",
    appStoreURL: "https://apps.apple.com/app/id1136220934",
    buildURL: (video, streamURL) =>
      `infuse://x-callback-url/play?url=${encodeURIComponent(streamURL)}&filename=${encodeURIComponent(video.fileName)}`,
  },
  {
    id: "vlc",
    name: "VLC",
    description: "免费、开源，支持大量视频格式",
    appStoreURL: "https://apps.apple.com/app/id650377962",
    buildURL: (video, streamURL) =>
      `vlc-x-callback://x-callback-url/stream?url=${encodeURIComponent(streamURL)}&filename=${encodeURIComponent(video.fileName)}`,
  },
];

/** True for iPhone, iPod, iPad, and iPadOS reporting a desktop Safari UA. */
export function isIOSDevice(): boolean {
  const ua = navigator.userAgent;
  return /iPad|iPhone|iPod/.test(ua) || (navigator.platform === "MacIntel" && navigator.maxTouchPoints > 1);
}

/**
 * Absolute URL fetched by the native player over the same LAN connection.
 *
 * A native player cannot send the session cookie, so the access key travels in
 * the query string. The server only accepts a query key on `/stream/`, which is
 * exactly this URL; rotating the key invalidates copied links.
 */
export function externalStreamURL(videoID: number): string {
  const url = new URL(`/stream/video/${videoID}`, window.location.origin);
  const key = currentAccessKey();
  if (key) url.searchParams.set("key", key);
  return url.toString();
}

export function playerByID(id: ExternalPlayerID): ExternalPlayer {
  return externalPlayers.find((player) => player.id === id) ?? externalPlayers[0];
}

export function loadPreferredPlayer(): ExternalPlayerID | null {
  try {
    const value = window.localStorage.getItem(PREFERRED_PLAYER_KEY);
    return value === "infuse" || value === "vlc" ? value : null;
  } catch {
    return null;
  }
}

export function savePreferredPlayer(id: ExternalPlayerID) {
  try {
    window.localStorage.setItem(PREFERRED_PLAYER_KEY, id);
  } catch {
    // Private browsing may disable localStorage; the launch itself still works.
  }
}

/** Clipboard API first, with a method that also works on plain-HTTP LAN pages. */
export async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    const input = document.createElement("textarea");
    input.value = text;
    input.setAttribute("readonly", "");
    input.style.position = "fixed";
    input.style.opacity = "0";
    document.body.appendChild(input);
    input.select();
    const copied = document.execCommand("copy");
    input.remove();
    return copied;
  }
}
