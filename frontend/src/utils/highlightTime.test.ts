import { describe, expect, it } from "vitest";

import { makeHighlight, makeHighlightPreview } from "../test/fixtures";
import {
  canUseHighlightPreset,
  formatHighlightTime,
  highlightLabel,
  isHighlightPending,
  isHighlightRange,
  isHighlightReady,
  mobileVisibleHighlights,
  msToTimeParts,
  previewMatchesRequest,
  timePartsToMs,
  validateHighlightParts,
  validateHighlightTimes,
} from "./highlightTime";

/**
 * 精彩时刻的时间规则。
 *
 * 这里钉的是"用户看到的那句话对不对"：同一个越界输入，前端红字与后端拒绝必须说
 * 同一件事，否则用户会在两个地方读到两种解释。时间换算错一位不会报错，只会安静地
 * 截到相邻的一帧，所以每条边界都单独钉一遍。
 */

const empty = { h: "", m: "", s: "" };

describe("timePartsToMs", () => {
  it("把时、分、秒换算成毫秒", () => {
    expect(timePartsToMs({ h: "1", m: "2", s: "3" })).toBe(3_723_000);
  });

  it("空着的子字段按 0 解释", () => {
    expect(timePartsToMs({ h: "", m: "1", s: "" })).toBe(60_000);
  });

  it("三个框全空表示没有这个时间，而不是 0:00", () => {
    expect(timePartsToMs(empty)).toBeNull();
  });

  it("非数字按无效处理，不悄悄当成 0", () => {
    expect(timePartsToMs({ h: "一", m: "", s: "" })).toBeNaN();
  });
});

describe("validateHighlightParts", () => {
  it("开始时间为空时要求填写", () => {
    expect(validateHighlightParts(empty, empty)).toBe("请填写开始时间");
  });

  it("分钟超过 59 时提示分/秒范围", () => {
    expect(validateHighlightParts({ h: "", m: "60", s: "0" }, empty)).toBe("分/秒需在 0–59");
  });

  it("秒超过 59 时同样提示分/秒范围", () => {
    expect(validateHighlightParts({ h: "", m: "0", s: "60" }, empty)).toBe("分/秒需在 0–59");
  });

  it("结束时间的分/秒越界也要提示", () => {
    expect(validateHighlightParts({ h: "", m: "1", s: "30" }, { h: "", m: "0", s: "75" })).toBe(
      "分/秒需在 0–59",
    );
  });

  it("结束时间全空是合法输入（时刻截图）", () => {
    expect(validateHighlightParts({ h: "", m: "1", s: "30" }, empty)).toBeNull();
  });

  it("结束行填了 0:00:00 时判定为不晚于开始时间", () => {
    expect(
      validateHighlightParts({ h: "", m: "1", s: "30" }, { h: "0", m: "0", s: "0" }),
    ).toBe("结束时间必须晚于开始时间");
  });

  it("两行都在范围内时不报错", () => {
    expect(
      validateHighlightParts({ h: "", m: "1", s: "30" }, { h: "", m: "1", s: "35" }),
    ).toBeNull();
  });
});

describe("validateHighlightTimes", () => {
  it("结束时间早于开始时间时要求晚于开始", () => {
    expect(validateHighlightTimes(90_000, 60_000, 0)).toBe("结束时间必须晚于开始时间");
  });

  it("结束时间等于开始时间同样不成立", () => {
    expect(validateHighlightTimes(90_000, 90_000, 0)).toBe("结束时间必须晚于开始时间");
  });

  it("不足 1 秒的时间段被拒绝", () => {
    expect(validateHighlightTimes(90_000, 90_500, 0)).toBe("时间段至少 1 秒");
  });

  it("超过 20 秒时报出当前秒数", () => {
    expect(validateHighlightTimes(90_000, 112_000, 0)).toBe("时间段最长 20 秒（当前 22 秒）");
  });

  it("恰好 20 秒可以通过", () => {
    expect(validateHighlightTimes(90_000, 110_000, 0)).toBeNull();
  });

  it("开始时间到达视频末尾时提示超出时长", () => {
    expect(validateHighlightTimes(120_000, 0, 120_000)).toBe("开始时间超出视频时长");
  });

  it("结束时间超出视频时长时单独提示", () => {
    expect(validateHighlightTimes(115_000, 121_000, 120_000)).toBe("结束时间超出视频时长");
  });

  it("时长未知时不判断越界", () => {
    expect(validateHighlightTimes(999_000_000, 0, 0)).toBeNull();
  });

  it("时刻（结束为 0）跳过时间段规则", () => {
    expect(validateHighlightTimes(90_000, 0, 0)).toBeNull();
  });
});

describe("formatHighlightTime", () => {
  it("0 秒显示为 0:00", () => {
    expect(formatHighlightTime(0)).toBe("0:00");
  });

  it("1 分 30 秒显示为 1:30", () => {
    expect(formatHighlightTime(90_000)).toBe("1:30");
  });

  it("超过一小时才出现小时位，分也补零", () => {
    expect(formatHighlightTime(3_723_000)).toBe("1:02:03");
  });
});

describe("highlightLabel", () => {
  it("时刻标签是 时刻 + 时间", () => {
    expect(highlightLabel(makeHighlight({ kind: "moment", startMs: 90_000, endMs: 0 }))).toBe(
      "时刻 1:30",
    );
  });

  it("时间段标签带起止与秒数", () => {
    expect(
      highlightLabel(makeHighlight({ kind: "range", startMs: 90_000, endMs: 95_000 })),
    ).toBe("时间段 1:30–1:35 · 5 秒");
  });
});

describe("isHighlightPending", () => {
  it("排队中算进行中", () => {
    expect(isHighlightPending(makeHighlight({ status: "queued" }))).toBe(true);
  });

  it("生成中也算进行中", () => {
    expect(isHighlightPending(makeHighlight({ status: "running" }))).toBe(true);
  });

  it("已完成不算进行中", () => {
    expect(isHighlightPending(makeHighlight({ status: "ready" }))).toBe(false);
  });
});

describe("isHighlightReady", () => {
  it("完成后才可显示图片", () => {
    expect(isHighlightReady(makeHighlight({ status: "ready" }))).toBe(true);
  });

  it("排队中还不能显示图片", () => {
    expect(isHighlightReady(makeHighlight({ status: "queued" }))).toBe(false);
  });
});

describe("mobileVisibleHighlights", () => {
  it("隐藏失败条目，其余照常展示", () => {
    const visible = mobileVisibleHighlights([
      makeHighlight({ id: 1, status: "ready" }),
      makeHighlight({ id: 2, status: "failed" }),
      makeHighlight({ id: 3, status: "queued" }),
    ]);

    expect(visible.map((h) => h.id)).toEqual([1, 3]);
  });
});

describe("previewMatchesRequest", () => {
  it("时间没变时预览仍然有效", () => {
    expect(previewMatchesRequest(makeHighlightPreview(), 90_000, 95_000)).toBe(true);
  });

  it("改了结束时间后预览作废", () => {
    expect(previewMatchesRequest(makeHighlightPreview(), 90_000, 96_000)).toBe(false);
  });

  it("改了开始时间后预览作废", () => {
    expect(previewMatchesRequest(makeHighlightPreview(), 91_000, 95_000)).toBe(false);
  });

  it("时刻（结束为 0）按同样的规则比对", () => {
    const moment = makeHighlightPreview({ kind: "moment", startMs: 90_000, endMs: 0 });
    expect(previewMatchesRequest(moment, 90_000, 0)).toBe(true);
    expect(previewMatchesRequest(moment, 90_000, 5_000)).toBe(false);
  });
});

describe("msToTimeParts", () => {
  it("1 分 35 秒拆成 1 和 35，小时留空", () => {
    expect(msToTimeParts(95_000)).toEqual({ h: "", m: "1", s: "35" });
  });

  it("不足一分钟时把 0 分写出来，读起来才是 5 秒", () => {
    expect(msToTimeParts(5_000)).toEqual({ h: "", m: "0", s: "5" });
  });

  it("超过一小时才出现小时位", () => {
    expect(msToTimeParts(3_723_000)).toEqual({ h: "1", m: "2", s: "3" });
  });

  it("与 timePartsToMs 互为逆运算", () => {
    expect(timePartsToMs(msToTimeParts(95_000))).toBe(95_000);
  });
});

describe("canUseHighlightPreset", () => {
  it("时长在 20 秒以内且没超出视频时可用", () => {
    expect(canUseHighlightPreset(30_000, 20, 120_000)).toBe(true);
  });

  it("会超出视频结尾时不可用", () => {
    expect(canUseHighlightPreset(110_000, 20, 120_000)).toBe(false);
  });

  it("恰好到视频结尾仍然可用", () => {
    expect(canUseHighlightPreset(100_000, 20, 120_000)).toBe(true);
  });

  it("视频时长未知时不判断越界", () => {
    expect(canUseHighlightPreset(0, 20, 0)).toBe(true);
  });

  it("超过 20 秒上限的时长不可用", () => {
    expect(canUseHighlightPreset(0, 25, 120_000)).toBe(false);
  });
});

describe("isHighlightRange", () => {
  it("时间段需要播放器才能动", () => {
    expect(isHighlightRange(makeHighlight({ kind: "range", startMs: 0, endMs: 5_000 }))).toBe(true);
  });

  it("时刻是一张静态图", () => {
    expect(isHighlightRange(makeHighlight({ kind: "moment", endMs: 0 }))).toBe(false);
  });
});
