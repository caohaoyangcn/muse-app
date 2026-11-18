import type { Video } from "../types";

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
