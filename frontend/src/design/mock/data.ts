// Fictional library used only by the design preview (src/design/preview.tsx).
//
// Every value is invented: the preview must never look like a screenshot of a
// real person's collection, and codes that resemble real studio catalogues
// would make the mock point at actual works. The shapes, on the other hand, are
// exactly the shapes the Go models return, so the views render for real.
import type {
  ActorProfile,
  ActorSummary,
  BackgroundTasks,
  ContentDuplicateGroup,
  ContentScanStatus,
  Highlight,
  JavDetail,
  JavDuplicateGroup,
  JavTaskOverview,
  SimilarVideo,
  TrashSnapshot,
  Video,
} from "../../types";

interface Seed {
  code: string;
  title: string;
  titleZh: string;
  actors: string[];
  tags: string[];
  minutes: number;
  sizeGb: number;
  hoursAgo: number;
  liked?: number;
  status?: "available" | "missing";
  volumeOffline?: boolean;
  remote?: boolean;
  notes?: string;
  customTitle?: string;
}

const SEEDS: Seed[] = [
  {
    code: "MUSE-101",
    title: "真夏の海辺で、君と",
    titleZh: "盛夏海边，与你",
    actors: ["水野あかり", "白石みなみ"],
    tags: ["单体作品", "写真", "4K"],
    minutes: 148,
    sizeGb: 6.4,
    hoursAgo: 2,
    liked: 12,
    notes: "第 3 段的海景镜头值得做封面",
  },
  {
    code: "AURA-208",
    title: "残業のあとで",
    titleZh: "加班之后",
    actors: ["白石みなみ"],
    tags: ["职场", "剧情", "人妻"],
    minutes: 126,
    sizeGb: 4.2,
    hoursAgo: 6,
    liked: 4,
  },
  {
    code: "NOCT-044",
    title: "静かな夜の教室",
    titleZh: "安静的夜晚教室",
    actors: ["綾瀬ゆい", "水野あかり", "小倉さくら"],
    tags: ["剧情", "单体作品"],
    minutes: 172,
    sizeGb: 7.8,
    hoursAgo: 20,
    liked: 31,
  },
  {
    code: "LUMI-317",
    title: "旅館の女将",
    titleZh: "旅馆的女将",
    actors: ["小倉さくら"],
    tags: ["熟女", "剧情", "人妻"],
    minutes: 134,
    sizeGb: 5.1,
    hoursAgo: 28,
  },
  {
    code: "MUSE-118",
    title: "雨の日のオフィス",
    titleZh: "雨天的办公室",
    actors: ["綾瀬ゆい"],
    tags: ["职场", "单体作品"],
    minutes: 119,
    sizeGb: 3.6,
    hoursAgo: 44,
    liked: 2,
    customTitle: "雨天办公室（收藏版）",
  },
  {
    code: "AURA-233",
    title: "はじめての夏",
    titleZh: "最初的夏天",
    actors: ["水野あかり"],
    tags: ["单体作品", "素人"],
    minutes: 105,
    sizeGb: 2.9,
    hoursAgo: 52,
    liked: 7,
  },
  {
    code: "NOCT-061",
    title: "隣の部屋の灯り",
    titleZh: "隔壁房间的灯",
    actors: ["白石みなみ", "小倉さくら"],
    tags: ["剧情", "人妻"],
    minutes: 158,
    sizeGb: 6.0,
    hoursAgo: 70,
  },
  {
    code: "LUMI-329",
    title: "海辺の写真集",
    titleZh: "海边写真集",
    actors: ["綾瀬ゆい"],
    tags: ["写真", "单体作品", "4K"],
    minutes: 92,
    sizeGb: 5.7,
    hoursAgo: 80,
    status: "missing",
  },
  {
    code: "MUSE-124",
    title: "午後のカフェテラス",
    titleZh: "午后的咖啡露台",
    actors: ["小倉さくら"],
    tags: ["写真", "素人"],
    minutes: 88,
    sizeGb: 2.2,
    hoursAgo: 96,
    liked: 1,
  },
  {
    code: "AURA-241",
    title: "冬の約束",
    titleZh: "冬天的约定",
    actors: ["水野あかり", "綾瀬ゆい"],
    tags: ["剧情", "单体作品"],
    minutes: 141,
    sizeGb: 4.9,
    hoursAgo: 120,
  },
  {
    code: "NOCT-073",
    title: "終電のあと",
    titleZh: "末班车之后",
    actors: ["白石みなみ"],
    tags: ["职场", "剧情"],
    minutes: 133,
    sizeGb: 4.4,
    hoursAgo: 150,
    volumeOffline: true,
  },
  {
    code: "LUMI-340",
    title: "白いワンピース",
    titleZh: "白色连衣裙",
    actors: ["小倉さくら", "水野あかり"],
    tags: ["写真", "单体作品", "4K"],
    minutes: 97,
    sizeGb: 6.8,
    hoursAgo: 168,
    liked: 18,
  },
  {
    code: "MUSE-137",
    title: "夏祭りの夜",
    titleZh: "夏日祭的夜晚",
    actors: ["綾瀬ゆい"],
    tags: ["剧情", "素人"],
    minutes: 112,
    sizeGb: 3.3,
    hoursAgo: 190,
  },
  {
    code: "AURA-256",
    title: "遠い街のホテル",
    titleZh: "远方城市的酒店",
    actors: ["白石みなみ", "小倉さくら"],
    tags: ["剧情", "熟女"],
    minutes: 165,
    sizeGb: 7.1,
    hoursAgo: 210,
    remote: true,
  },
  {
    code: "NOCT-088",
    title: "週末のプール",
    titleZh: "周末的泳池",
    actors: ["水野あかり"],
    tags: ["写真", "单体作品"],
    minutes: 84,
    sizeGb: 2.6,
    hoursAgo: 240,
    liked: 3,
  },
  {
    code: "LUMI-351",
    title: "夜明けまで",
    titleZh: "直到黎明",
    actors: ["綾瀬ゆい", "白石みなみ"],
    tags: ["剧情", "4K"],
    minutes: 156,
    sizeGb: 8.2,
    hoursAgo: 280,
  },
  {
    code: "MUSE-149",
    title: "春の坂道",
    titleZh: "春天的坡道",
    actors: ["小倉さくら"],
    tags: ["写真", "单体作品"],
    minutes: 101,
    sizeGb: 3.1,
    hoursAgo: 320,
  },
  {
    code: "AURA-268",
    title: "最後の夏休み",
    titleZh: "最后的暑假",
    actors: ["水野あかり", "小倉さくら"],
    tags: ["剧情", "单体作品", "4K"],
    minutes: 144,
    sizeGb: 6.2,
    hoursAgo: 360,
  },
];

/** The studio follows the code prefix, so the detail page and the card agree. */
const STUDIOS: Record<string, string> = {
  MUSE: "MUSE Works",
  AURA: "AURA Films",
  NOCT: "NOCT Studio",
  LUMI: "LUMI Pictures",
};

const HOUR = 3_600_000;
const NOW = 1_760_000_000_000;

function thumbFor(id: number): string {
  return `/thumbnails/${id}.jpg`;
}

function makeVideo(seed: Seed, index: number): Video {
  const id = index + 1;
  const createdAt = NOW - seed.hoursAgo * HOUR;
  const opened = index % 4 === 0 ? NOW - (index + 2) * HOUR : 0;
  const remote = seed.remote === true;
  const path = remote
    ? `/volume1/media/${seed.code}.mkv`
    : `D:\\Library\\MUSE\\${seed.code}\\${seed.code}.mp4`;
  return {
    id,
    path,
    fileName: `${seed.code}${remote ? ".mkv" : ".mp4"}`,
    fileSize: Math.round(seed.sizeGb * 1024 * 1024 * 1024),
    durationMs: seed.minutes * 60_000,
    width: 1920,
    height: 1080,
    frameRate: 29.97,
    bitrate: 6_000_000,
    videoCodec: "h264",
    audioCodec: "aac",
    thumbnailPath: thumbFor(id),
    createdAt,
    updatedAt: createdAt + 30_000,
    lastOpenedAt: opened,
    status: seed.status === "missing" ? "missing" : "available",
    volumeOffline: seed.volumeOffline === true,
    volumeRoot: remote ? "" : "D:\\",
    title: seed.customTitle ?? "",
    notes: seed.notes ?? "",
    javCode: seed.code,
    javTitle: seed.title,
    javTitleZh: seed.titleZh,
    actors: seed.actors,
    tags: seed.tags,
    mosaicRestored: false,
    customCoverAt: 0,
    likeCount: seed.liked ?? 0,
    coverSource: "thumbnail",
    scrapedCoverPath: "",
    allowScrapedCover: false,
    contentHash: `sha256:${(id * 7919).toString(16).padStart(12, "0")}`,
    hashVersion: 1,
    sourceId: remote ? 2 : 0,
    remotePath: remote ? `/volume1/media/${seed.code}.mkv` : "",
    sourceName: remote ? "家庭 NAS" : "",
    sourceOffline: false,
    sourceEnabled: remote,
    backedUp: false,
    cloudUrl: "",
    deletedAt: 0,
    deleteReason: "",
    archiveSourceName: "",
  };
}

export const VIDEOS: Video[] = SEEDS.map(makeVideo);

/** Three favorites that were yet to be reviewed sit in the recycle bin. */
export const TRASH_VIDEOS: Video[] = [3, 10, 16].map((id) => {
  const video = VIDEOS[id - 1];
  return {
    ...video,
    deletedAt: NOW - (id + 1) * HOUR,
    deleteReason: id === 3 ? "重复收藏" : "清理旧收藏",
  };
});

export const TAG_NAMES = [
  "单体作品",
  "剧情",
  "写真",
  "人妻",
  "职场",
  "4K",
  "素人",
  "熟女",
];

export const ACTOR_NAMES = [
  "水野あかり",
  "白石みなみ",
  "綾瀬ゆい",
  "小倉さくら",
];

function profile(name: string, index: number): ActorProfile {
  return {
    name,
    provider: "mock",
    starId: `star-${index + 1}`,
    avatarUrl: `/avatars/${encodeURIComponent(name)}.jpg`,
    birthday: ["1996-03-14", "1994-11-02", "1998-07-21", "1992-05-09"][index],
    heightCm: [162, 158, 165, 155][index],
    bustCm: [88, 84, 90, 82][index],
    cup: ["D", "C", "E", "B"][index],
    waistCm: [58, 56, 60, 54][index],
    hipCm: [88, 85, 91, 84][index],
    bloodType: ["A", "O", "B", "A"][index],
    hometown: ["東京", "大阪", "福岡", "札幌"][index],
    hobby: ["映画鑑賞", "料理", "読書", "旅行"][index],
    movieCount: 40 + index * 17,
    lastReleaseUnix: 1_755_000_000 - index * 86_400,
    aliases: index === 0 ? ["みずの あかり"] : [],
    fetchedAt: NOW - HOUR,
  };
}

export const ACTORS = ACTOR_NAMES.map((name, index) => ({
  name,
  count: VIDEOS.filter((video) => video.actors.includes(name)).length,
  aliases: index === 0 ? ["みずの あかり"] : [],
  profile: profile(name, index),
})) as unknown as ActorSummary[];

export function javDetailFor(video: Video): JavDetail {
  const seed = SEEDS[video.id - 1];
  return {
    videoId: video.id,
    code: seed.code,
    title: seed.title,
    titleZh: seed.titleZh,
    titleZhStatus: "done",
    titleZhError: "",
    studio: STUDIOS[seed.code.split("-")[0]] ?? "MUSE Works",
    series: `${seed.code.split("-")[0]} 系列`,
    releaseUnix: 1_740_000_000 - video.id * 86_400 * 9,
    durationMin: seed.minutes,
    isUncensored: false,
    actors: seed.actors,
    tags: seed.tags,
    coverUrl: `/covers/${video.id}.jpg`,
    coverStatus: "ready",
    provider: "mock",
    codeSource: "filename",
    overrideMode: "",
    scrapeStatus: "done",
    attempts: 1,
    lastError: "",
    fetchedAt: NOW - video.id * HOUR,
    errorCode: "",
  } as JavDetail;
}

export const SIMILAR = [7, 12, 18, 6].map((id, index) => ({
  video: VIDEOS[id - 1],
  score: 92 - index * 7,
  reasons: ["同系列", "同演员", "同标签"],
})) as unknown as SimilarVideo[];

export const JAV_DUPLICATES: JavDuplicateGroup[] = [
  {
    key: "MUSE-101",
    code: "MUSE-101",
    part: "",
    tier: "same-quality",
    ignored: false,
    videos: [VIDEOS[0], { ...VIDEOS[1], javCode: "MUSE-101", fileName: "MUSE-101-copy.mp4" }],
  },
  {
    key: "AURA-208",
    code: "AURA-208",
    part: "",
    tier: "different-quality",
    ignored: false,
    videos: [VIDEOS[1], { ...VIDEOS[4], javCode: "AURA-208", fileName: "AURA-208-4k.mp4" }],
  },
] as JavDuplicateGroup[];

export const CONTENT_DUPLICATES: ContentDuplicateGroup[] = [
  {
    hash: "sha256:ab12cd",
    members: [
      { video: VIDEOS[2], missing: false, volumeOffline: false, needsReverify: false },
      { video: { ...VIDEOS[6], contentHash: "sha256:ab12cd" }, missing: false, volumeOffline: false, needsReverify: false },
    ],
    totalBytes: 16 * 1024 ** 3,
    reclaimableBytes: 8 * 1024 ** 3,
    ignored: false,
    confirmed: true,
    refuted: false,
    confirming: false,
    needsReverify: false,
    missing: 0,
  },
] as ContentDuplicateGroup[];

export const JAV_TASK_OVERVIEWS: JavTaskOverview[] = [
  { status: "running" },
  { status: "queued" },
  { status: "failed" },
  { status: "queued" },
].map((state, index) => {
  const video = VIDEOS[index + 10];
  return {
    ...state,
    id: index + 1,
    videoId: video.id,
    force: false,
    attempts: state.status === "failed" ? 5 : 1,
    maxAttempts: 5,
    errorCode: state.status === "failed" ? "network" : "",
    lastError: state.status === "failed" ? "连接超时：javdb 无响应" : "",
    createdAt: NOW - (index + 1) * 5 * 60_000,
    updatedAt: NOW - index * 60_000,
    startedAt: state.status === "running" ? NOW - 40_000 : undefined,
    finishedAt: undefined,
    category: "jav",
    video,
  } as JavTaskOverview;
});

export const TRASH_SNAPSHOT = {
  videos: TRASH_VIDEOS,
  retentionDays: 14,
} as unknown as TrashSnapshot;

export const CONTENT_SCAN_STATUS: ContentScanStatus = {
  running: false,
  phase: "idle",
  examined: 42,
  examineTotal: 42,
  total: 42,
  done: 42,
  hashed: 42,
  skipped: 6,
  failed: 0,
  startedAt: NOW - 20 * HOUR,
  finishedAt: NOW - 19 * HOUR,
} as ContentScanStatus;

export const BACKGROUND: BackgroundTasks = {
  tasks: [
    {
      id: "scrape-queue",
      title: "刮削队列",
      kind: "scrape",
      state: "running",
      done: 12,
      total: 18,
      failed: 1,
      unit: "部",
      current: "AURA-256 遠い街のホテル",
      message: "正在抓取详情",
      startedAt: NOW - 6 * 60_000,
      updatedAt: NOW - 1_200,
    },
    {
      id: "translate-titles",
      title: "片名翻译",
      kind: "translate",
      state: "running",
      done: 34,
      total: 51,
      failed: 0,
      unit: "条",
      current: "NOCT-073 終電のあと",
      startedAt: NOW - 22 * 60_000,
      updatedAt: NOW - 4_000,
    },
    {
      id: "content-scan",
      title: "内容指纹扫描",
      kind: "hash",
      state: "done",
      done: 42,
      total: 42,
      failed: 0,
      unit: "个文件",
      startedAt: NOW - 20 * HOUR,
      updatedAt: NOW - 19 * HOUR,
    },
  ],
} as BackgroundTasks;

/**
 * 详情页「精彩时刻」。预览里给两部片各配两行已完成的结果，这样截图/循环视频
 * 两种卡片、时间戳和「重新生成」都能渲染出来；媒体路径走预览服务器的
 * /covers/* 兜底图，不依赖任何真实文件。
 */
export const HIGHLIGHTS: Record<number, Highlight[]> = {
  1: [
    {
      id: 9101,
      videoId: 1,
      kind: "moment",
      startMs: 512_000,
      endMs: 0,
      mediaPath: "covers/h-muse-101-1.jpg",
      status: "ready",
      error: "",
      interruptions: 0,
      createdAt: NOW - 3 * HOUR,
      updatedAt: NOW - 3 * HOUR + 4_000,
      finishedAt: NOW - 3 * HOUR + 4_000,
    },
    {
      id: 9102,
      videoId: 1,
      kind: "moment",
      startMs: 1_830_000,
      endMs: 0,
      mediaPath: "covers/h-muse-101-2.jpg",
      status: "ready",
      error: "",
      interruptions: 0,
      createdAt: NOW - 2 * HOUR,
      updatedAt: NOW - 2 * HOUR + 9_000,
      finishedAt: NOW - 2 * HOUR + 9_000,
    },
  ],
  2: [
    {
      id: 9103,
      videoId: 2,
      kind: "moment",
      startMs: 402_000,
      endMs: 0,
      mediaPath: "covers/h-aura-208-1.jpg",
      status: "ready",
      error: "",
      interruptions: 0,
      createdAt: NOW - 5 * HOUR,
      updatedAt: NOW - 5 * HOUR + 3_000,
      finishedAt: NOW - 5 * HOUR + 3_000,
    },
  ],
};
