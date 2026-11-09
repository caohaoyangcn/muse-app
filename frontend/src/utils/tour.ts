import type { CSSProperties } from "react";

// The first-run guided tour. The steps are data, not markup: each one names the
// UI it talks about, so the copy and the spotlight stay in one place and the
// renderer never needs to know which screen a step belongs to.
//
// `target` is the value of a `data-tour` attribute. A step without a target is
// shown as a centered card (the welcome and the finish), and a target that is
// not on screen right now turns into a centered card too — never a card
// pointing at nothing.

export type TourView = "library" | "settings";

/** Settings submenu key the tour can force open. Kept as a string so this module
 *  stays free of the Settings page's own type. */
export type TourSection = string;

export interface TourStep {
  /** Stable id, used in the DOM and for measurement. */
  id: string;
  title: string;
  body: string;
  /** One line of practical guidance, rendered with a lamp icon. Optional. */
  hint?: string;
  /** Which screen the step describes; the tour switches there first. */
  view: TourView;
  /** Settings submenu to open before measuring. */
  section?: TourSection;
  /** `data-tour` value of the spotlit element. */
  target?: string;
  /** The welcome step: a single primary action instead of 上一步 / 下一步. */
  intro?: boolean;
  /** The finishing step: no back / next, only 开始使用. */
  outro?: boolean;
}

/**
 * The tour in reading order. It follows the path a new user actually takes:
 * understand the idea, find your way around, add files, get information in,
 * then the two settings screens that trip people up (目录与代理).
 */
export const TOUR_STEPS: TourStep[] = [
  {
    id: "welcome",
    view: "library",
    intro: true,
    title: "欢迎使用 Muse Video",
    body:
      "把一个本地视频文件变成一张带封面、番号和演员的卡片，都收在你自己的库里。\n" +
      "先记住一件事：Muse 不复制、不移动、也不删除你的视频文件，它只记住文件放在哪里。",
    hint: "看到最后一页大概一两分钟，随时可以按 Esc 退出。",
  },
  {
    id: "sidebar",
    view: "library",
    target: "sidebar",
    title: "左边是分组",
    body:
      "左侧集中了所有入口：「全部收藏」看整个库，「最近播放」按 1 / 3 / 7 / 14 天回看播放记录，下面是你自己建的分组，再往下是演员、刮削任务和查重。\n" +
      "用分组把片子按题材、系列或硬盘分门别类，拖一张卡片到分组上就能直接收进去。",
    hint: "点分组上方的 + 新建分组；右键分组可以重命名、删除或调整层级。",
  },
  {
    id: "add",
    view: "library",
    target: "add-video",
    title: "添加视频",
    body:
      "点「+ 添加视频」先选择来源：本地文件，或扫描文件夹把整个目录收进来；也可以把文件直接拖进窗口。\n" +
      "扫描文件夹会同时加入目录监控；以后这个目录里新增的视频会自动收录。",
    hint: "文件很多时，扫描文件夹通常比一个个选文件省事得多。",
  },
  {
    id: "toolbar",
    view: "library",
    target: "library-toolbar",
    title: "找到你想看的",
    body:
      "这一行控制收藏怎么显示：切换网格 / 列表、调整卡片大小，按添加时间、最近播放、文件大小、文件名或点赞数排序。\n" +
      "多块硬盘都在用的话，硬盘筛选会出现在这一行下面，只看某块盘里的片子。",
    hint: "右上角的搜索框可以搜文件名或路径，按 Ctrl+F 直接聚焦。",
  },
  {
    id: "scrape",
    view: "library",
    target: "scrape-tasks",
    title: "刮削：补齐资料和封面",
    body:
      "「刮削」就是让 Muse 按番号去 JavBus 或 FC2CMADB 取回封面、标题、演员这些资料。\n" +
      "单部：点开卡片，在详情页点「重新刮削」；批量：设置 → JavBus → 批量刮削。任务在后台跑，这里能看到进度。",
    hint: "识别不到番号或网络不通都会失败，失败的任务会自动重试，不影响播放。",
  },
  {
    id: "server",
    view: "library",
    target: "server",
    title: "手机也能看",
    body:
      "「局域网访问」启动后，顶部会显示一个手机可访问的地址。手机浏览器打开它，就能用专门的移动端界面浏览收藏。\n" +
      "影片播放交给手机上的 Infuse 或 VLC 解码，Muse 只负责把文件送过去。",
    hint: "局域网接口没有密码，同一网络下的设备都能打开；网络不可信时用完记得点「停止」。",
  },
  {
    id: "settings-nav",
    view: "settings",
    target: "settings-nav",
    title: "设置里有什么",
    body:
      "「服务器」改端口（默认 8899，不用动）；「目录监控」让整个文件夹自动入库；「网络代理」给刮削配代理；最后两项填刮削站点的 Cookie。\n" +
      "接下来两页，就是新手最常卡住的两件事。",
    hint: "后续任何时候都能在这里重新看一遍引导。",
  },
  {
    id: "watch",
    view: "settings",
    section: "watch",
    target: "settings-panel-watch",
    title: "添加目录：一次收一整个文件夹",
    body:
      "点「+ 添加目录」选一个文件夹，Muse 会递归扫描里面所有视频文件并加入收藏；之后往这个文件夹里放新片子，也会自动出现。\n" +
      "移除目录只是停止监控，已经收藏的影片会保留，源文件不受影响。",
    hint: "第一次扫描大目录需要一点时间，扫完会自动出现在库里。",
  },
  {
    id: "proxy",
    view: "settings",
    section: "proxy",
    target: "settings-panel-proxy",
    title: "代理：刮削连不上时用",
    body:
      "如果刮削一直失败，多半是连不上 JavBus / FC2CMADB。在「网络代理」里填上你的代理地址，保存后立刻生效。\n" +
      "支持 http、https、socks5，例如 http://127.0.0.1:7890；留空保存就是直连。",
    hint: "Muse 不会自动使用系统代理，需要在这里单独填一次。",
  },
  {
    id: "done",
    view: "library",
    outro: true,
    title: "可以开始了",
    body:
      "先把片子加进库，再花几分钟刮削，之后就能搜索、分组、用手机躺着看了。\n" +
      "收藏库永远只是记录：换硬盘、改文件名、挪文件夹，都可以用文件检测和重新关联找回来。",
    hint: "重新看这次引导：设置 → 新手引导。",
  },
];

// ---------------------------------------------------------------------------
// Geometry
// ---------------------------------------------------------------------------

export interface Rect {
  left: number;
  top: number;
  right: number;
  bottom: number;
  /** Present on DOMRect; the geometry helpers below only need the edges. */
  width?: number;
  height?: number;
}

/** Breathing room between the spotlit element and the dimmed page. */
export const SPOTLIGHT_PAD = 6;
/** Gap between the spotlight and the card. */
export const TOOLTIP_GAP = 14;
export const TOOLTIP_WIDTH = 364;
const VIEWPORT_MARGIN = 12;
/** Half of the card's comfortable height, used to centre side placements. */
const CARD_MID = 220;

export const spotlightRect = (rect: Rect): Rect => ({
  left: rect.left - SPOTLIGHT_PAD,
  top: rect.top - SPOTLIGHT_PAD,
  right: rect.right + SPOTLIGHT_PAD,
  bottom: rect.bottom + SPOTLIGHT_PAD,
});

/** The rendered size of a rect. */
export const rectOf = (r: Rect): { width: number; height: number } => ({
  width: r.right - r.left,
  height: r.bottom - r.top,
});

/**
 * Keeps the spotlight ring inside the window: the header sits flush with the
 * top edge, and a ring drawn at -6px there would be clipped by the viewport
 * rather than surrounded by dimming.
 */
export function clampToViewport(rect: Rect, pad: number): Rect {
  return {
    left: Math.max(pad, rect.left),
    top: Math.max(pad, rect.top),
    right: Math.min(window.innerWidth - pad, rect.right),
    bottom: Math.min(window.innerHeight - pad, rect.bottom),
  };
}

export type Placement = "top" | "bottom" | "right" | "left" | "center";

/**
 * Picks the side of the spotlight the card goes on.
 *
 * One rule decides this: the card must not cover the thing it is describing.
 * A narrow target (a button, a menu) has room above or below and gets a card
 * that points at it. A target that spans the window (the header, a toolbar, a
 * settings section) would be covered by a card placed above or below, so it
 * moves to whichever side has room. The selected side only needs to fit the
 * card, not its whole content: the card caps and scrolls its own height.
 */
export function pickPlacement(spot: Rect): Placement {
  const vw = window.innerWidth;
  const vh = window.innerHeight;
  const targetHeight = spot.bottom - spot.top;
  const targetWidth = spot.right - spot.left;
  const sideRoom = TOOLTIP_WIDTH + TOOLTIP_GAP + VIEWPORT_MARGIN;
  const roomRight = vw - spot.right;
  const roomLeft = spot.left;

  // Entries inside a narrow left rail (the sidebar): a card below would cover
  // the neighbouring entries, so it sits beside the rail instead.
  if (spot.right <= 280 && roomRight >= sideRoom) return "right";
  if (spot.left >= vw - 280 && roomLeft >= sideRoom) return "left";

  // Wide targets: go beside them (the header, the toolbar).
  if (targetWidth > vw * 0.55) {
    if (roomRight >= sideRoom) return "right";
    if (roomLeft >= sideRoom) return "left";
  }

  // Tall targets with a whole free side: a centred card reads better than one
  // jammed under a menu that fills the window.
  if (targetHeight > vh * 0.4) {
    if (roomRight >= sideRoom) return "right";
    if (roomLeft >= sideRoom) return "left";
  }

  // Narrow targets: above or below, whichever has more room.
  const roomTop = spot.top;
  const roomBottom = vh - spot.bottom;
  if (roomBottom >= 180 && roomBottom >= roomTop) return "bottom";
  if (roomTop >= 180) return "top";
  if (roomRight >= sideRoom) return "right";
  if (roomLeft >= sideRoom) return "left";

  // No room for the card anywhere around the target.
  return "center";
}

/**
 * The card's fixed position and its optional arrow. Coordinates are viewport
 * coordinates; the card is `position: fixed`. A step with no spotlight (the
 * welcome and the finish) is a plain centred card. When `placement` is
 * top/bottom the card is anchored past the spotlight edge; otherwise it is
 * centred on the given edge.
 */
export function cardPosition(
  placement: Placement,
  spot: Rect | null,
): { style: CSSProperties; arrow: CSSProperties | null } {
  const vw = window.innerWidth;
  const vh = window.innerHeight;
  const width = Math.min(TOOLTIP_WIDTH, vw - VIEWPORT_MARGIN * 2);

  if (spot == null || placement === "center") {
    return {
      style: {
        width,
        left: Math.round((vw - width) / 2),
        top: Math.round(vh * 0.5),
        transform: "translateY(-50%)",
      },
      arrow: null,
    };
  }

  const centreX = (spot.left + spot.right) / 2;
  const centreY = (spot.top + spot.bottom) / 2;

  if (placement === "top" || placement === "bottom") {
    const left = clamp(centreX - width / 2, VIEWPORT_MARGIN, vw - width - VIEWPORT_MARGIN);
    const arrowX = clamp(centreX - left - 7, 16, width - 30);
    if (placement === "bottom") {
      return {
        style: { width, left: Math.round(left), top: Math.round(spot.bottom + TOOLTIP_GAP) },
        arrow: { top: -7, left: arrowX, transform: "rotate(45deg)" },
      };
    }
    return {
      style: {
        width,
        left: Math.round(left),
        top: Math.round(spot.top - TOOLTIP_GAP),
        transform: "translateY(-100%)",
      },
      arrow: { bottom: -7, left: arrowX, transform: "rotate(45deg)" },
    };
  }

  if (placement === "right") {
    const top = clamp(centreY - CARD_MID, VIEWPORT_MARGIN, vh - CARD_MID * 2);
    return {
      style: { width, left: Math.round(spot.right + TOOLTIP_GAP), top: Math.round(top) },
      arrow: {
        top: clamp(centreY - top, 22, 220),
        left: -7,
        transform: "translateY(-50%) rotate(45deg)",
      },
    };
  }

  const top = clamp(centreY - CARD_MID, VIEWPORT_MARGIN, vh - CARD_MID * 2);
  return {
    style: { width, left: Math.round(spot.left - TOOLTIP_GAP - width), top: Math.round(top) },
    arrow: {
      top: clamp(centreY - top, 22, 220),
      right: -7,
      transform: "translateY(-50%) rotate(45deg)",
    },
  };
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max);
}
