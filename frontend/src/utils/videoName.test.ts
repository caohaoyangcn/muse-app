import { describe, expect, it } from "vitest";

import { makeVideo } from "../test/fixtures";
import { videoDisplayName } from "./videoName";

/**
 * How a favorite is named on both clients.
 *
 * The rule is a small one with several edges that were each a deliberate
 * decision — a user title always wins, a scraped work shows as 【番号】译名 while
 * the translation queue catches up, an echoed 番号 is not repeated, and an
 * unscraped favorite keeps its file name instead of pretending to be a work.
 */
describe("videoDisplayName", () => {
  it("prefers the user's own title over everything scraped", () => {
    const name = videoDisplayName(
      makeVideo({ title: "我的命名", javCode: "IPX-633", javTitle: "元タイトル", javTitleZh: "译名" }),
    );
    expect(name).toBe("我的命名");
  });

  it("shows the scraped work as 【番号】译名", () => {
    expect(
      videoDisplayName(makeVideo({ javCode: "IPX-633", javTitle: "元タイトル", javTitleZh: "中文译名" })),
    ).toBe("【IPX-633】中文译名");
  });

  it("falls back to the scraped original title until a translation exists", () => {
    expect(videoDisplayName(makeVideo({ javCode: "IPX-633", javTitle: "元タイトル" }))).toBe(
      "【IPX-633】元タイトル",
    );
  });

  it("drops a 番号 echoed at the start of the title", () => {
    expect(
      videoDisplayName(
        makeVideo({ javCode: "IPX-633", javTitleZh: "IPX-633 中文译名" }),
      ),
    ).toBe("【IPX-633】中文译名");
    expect(
      videoDisplayName(makeVideo({ javCode: "IPX-633", javTitleZh: "【IPX-633】中文译名" })),
    ).toBe("【IPX-633】中文译名");
  });

  it("does not let a short 番号 eat the start of a longer one", () => {
    expect(
      videoDisplayName(makeVideo({ javCode: "AB-1", javTitleZh: "AB-123 タイトル" })),
    ).toBe("【AB-1】AB-123 タイトル");
  });

  it("keeps the bracketed 番号 when the title is only the 番号", () => {
    expect(videoDisplayName(makeVideo({ javCode: "IPX-633", javTitleZh: "IPX-633" }))).toBe("【IPX-633】");
  });

  it("keeps the file name when nothing was scraped", () => {
    expect(videoDisplayName(makeVideo({ fileName: "sample.mp4" }))).toBe("sample.mp4");
    // A manually entered 番号 stores no title, so the card stays on the file.
    expect(videoDisplayName(makeVideo({ javCode: "IPX-633", fileName: "sample.mp4" }))).toBe("sample.mp4");
  });
});
