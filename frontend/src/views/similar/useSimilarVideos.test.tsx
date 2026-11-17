import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";

import type { SimilarVideo, Video } from "../../types";
import { configureClient, type MuseClient } from "../../services/client";
import { useSimilarVideos } from "./useSimilarVideos";

/**
 * 「类似影片」的视图模型契约。
 *
 * 这一栏的价值全在"出错时安静地消失"：详情页的其他内容不能因为推荐读不到而
 * 出问题。所以这里钉三件事——首屏与失败都归一成空数组（不是 null，数组才可直接
 * 渲染）、读到的结果原样交给展示层、条数进查询键（换条数就是换一个问题）。
 */

let latest: ReturnType<typeof useSimilarVideos> | null = null;
let root: Root | null = null;
let host: HTMLElement | null = null;

function state(): ReturnType<typeof useSimilarVideos> {
  if (!latest) throw new Error("the row has not rendered yet");
  return latest;
}

function Probe({ videoId, limit }: { videoId: number; limit?: number }) {
  latest = useSimilarVideos(videoId, limit);
  return null;
}

/** Binds a client whose only implemented read is the one under test. */
function stubClient(similarVideos: MuseClient["similarVideos"]): void {
  configureClient({ similarVideos } as unknown as MuseClient);
}

async function mount(videoId: number, limit?: number): Promise<void> {
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
  await act(async () => {
    root?.render(<Probe videoId={videoId} limit={limit} />);
  });
}

function similarItem(videoId: number, reasons: string[]): SimilarVideo {
  return {
    video: { id: videoId } as Video,
    score: 0.5,
    reasons,
  } as SimilarVideo;
}

beforeAll(() => {
  (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
});

afterAll(() => {
  delete (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT;
});

afterEach(async () => {
  latest = null;
  await act(async () => {
    root?.unmount();
  });
  root = null;
  host?.remove();
  host = null;
});

describe("useSimilarVideos", () => {
  // 读还没回来时展示层拿到的是空数组：它可以先不渲染这一段，而不是等一个 null。
  it("首屏给空数组而不是 null", async () => {
    stubClient(() => new Promise<SimilarVideo[]>(() => undefined));
    await mount(1);

    expect(state().loading).toBe(true);
    expect(state().items).toEqual([]);
  });

  // 推荐是附属信息：读失败不该让详情页显示错误，也不该让这一栏崩掉。
  it("读失败后仍是空数组", async () => {
    stubClient(() => Promise.reject(new Error("服务端不可用")));
    await mount(1);

    expect(state().loading).toBe(false);
    expect(state().items).toEqual([]);
  });

  it("把读到的推荐原样交给展示层", async () => {
    stubClient(async () => [similarItem(7, ["同演员", "同系列"])]);
    await mount(1);

    expect(state().items).toHaveLength(1);
    expect(state().items[0].video.id).toBe(7);
    expect(state().items[0].reasons).toEqual(["同演员", "同系列"]);
  });

  // 条数是两头各自的展示选择（手机一排更少），换条数必须重新提问。
  it("条数进查询键：改条数会重新取值", async () => {
    const calls: [number, number | undefined][] = [];
    stubClient(async (videoId, limit) => {
      calls.push([videoId, limit]);
      return [];
    });

    await mount(1, 6);
    await act(async () => {
      root?.render(<Probe videoId={1} limit={8} />);
    });

    expect(calls).toEqual([
      [1, 6],
      [1, 8],
    ]);
  });
});
