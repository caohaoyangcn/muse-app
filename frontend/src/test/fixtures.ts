import type { Highlight, Video } from "../types";

/**
 * Test fixtures for the view-model layer.
 *
 * `makeVideo` starts from a complete, unremarkable favorite and every test
 * overrides only the fields it is about. A partial default would make each test
 * restate the same eight unrelated fields, and a shape change in `video.Video`
 * would break dozens of assertions instead of one constructor.
 */
export function makeVideo(overrides: Partial<Video> = {}): Video {
  return {
    id: 1,
    path: "D:\\library\\sample.mp4",
    fileName: "sample.mp4",
    fileSize: 1024 * 1024 * 1024,
    durationMs: 7_423_420,
    width: 1920,
    height: 1080,
    frameRate: 23.976,
    bitrate: 8_000_000,
    videoCodec: "h264",
    audioCodec: "aac",
    thumbnailPath: "",
    createdAt: 1_700_000_000_000,
    updatedAt: 1_700_000_000_000,
    lastOpenedAt: 0,
    status: "available",
    volumeOffline: false,
    volumeRoot: "D:\\",
    title: "",
    notes: "",
    javCode: "",
    javTitle: "",
    javTitleZh: "",
    actors: [],
    tags: [],
    mosaicRestored: false,
    customCoverAt: 0,
    likeCount: 0,
    coverSource: "thumbnail",
    scrapedCoverPath: "",
    allowScrapedCover: false,
    contentHash: "",
    hashVersion: 0,
    sourceId: 0,
    remotePath: "",
    sourceName: "",
    sourceOffline: false,
    sourceEnabled: false,
    backedUp: false,
    cloudUrl: "",
    deletedAt: 0,
    deleteReason: "",
    archiveSourceName: "",
    ...overrides,
  };
}

/**
 * One 精彩时刻 row. Defaults to a finished 1:30 screenshot: the commonest row on
 * screen, so a test about the range rules or the failure states only states the
 * fields it is actually about.
 */
export function makeHighlight(overrides: Partial<Highlight> = {}): Highlight {
  return {
    id: 1,
    videoId: 1,
    kind: "moment",
    startMs: 90_000,
    endMs: 0,
    mediaPath: "highlights/h-1-1700000000000000000-1.jpg",
    status: "ready",
    error: "",
    createdAt: 1_700_000_000_000,
    updatedAt: 1_700_000_000_000,
    finishedAt: 1_700_000_000_000,
    ...overrides,
  };
}
