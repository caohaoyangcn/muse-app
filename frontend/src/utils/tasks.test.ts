import { describe, expect, it } from "vitest";

import type { BackgroundTask } from "../types";
import { taskEtaText, taskFinishedAtText, taskProgressText, taskRemaining } from "./tasks";

function makeTask(overrides: Partial<BackgroundTask> = {}): BackgroundTask {
  return {
    id: "title-translation",
    title: "片名翻译",
    kind: "batch",
    state: "running",
    done: 0,
    total: 0,
    failed: 0,
    unit: "条",
    ...overrides,
  } as BackgroundTask;
}

/**
 * The dashboard's phrasing rules. They are easy to get subtly wrong in a way
 * nothing else catches: a percentage without a denominator, a negative
 * remainder, or an ETA computed from one sample.
 */
describe("taskProgressText", () => {
  it("shows done over total when the total is known", () => {
    expect(taskProgressText(makeTask({ done: 200, total: 525 }))).toBe("200/525 条");
  });

  it("shows only the count when the total is unknown", () => {
    expect(taskProgressText(makeTask({ done: 12, total: 0, unit: "部" }))).toBe("已完成 12 部");
  });

  it("stays empty before the first result", () => {
    expect(taskProgressText(makeTask({ done: 0, total: 0 }))).toBe("");
  });
});

describe("taskRemaining", () => {
  it("counts finished and failed rows as processed", () => {
    expect(taskRemaining(makeTask({ done: 200, total: 525, failed: 3 }))).toBe(322);
  });

  it("returns null when the total is unknown", () => {
    expect(taskRemaining(makeTask({ done: 12, total: 0 }))).toBeNull();
  });

  it("never goes negative", () => {
    expect(taskRemaining(makeTask({ done: 10, total: 10, failed: 2 }))).toBe(0);
  });
});

describe("taskEtaText", () => {
  const now = 10_000_000;

  it("estimates from the average rate once there is enough evidence", () => {
    const task = makeTask({ done: 100, total: 200, startedAt: now - 60_000 });
    expect(taskEtaText(task, now)).toBe("约 1 分钟");
  });

  it("switches to hours for long passes", () => {
    const task = makeTask({ done: 1_800, total: 12_600, startedAt: now - 600_000 });
    expect(taskEtaText(task, now)).toBe("约 1 小时");
  });

  it("stays quiet with fewer than three results", () => {
    const task = makeTask({ done: 2, total: 200, startedAt: now - 60_000 });
    expect(taskEtaText(task, now)).toBeNull();
  });

  it("stays quiet while the first seconds have no rate yet", () => {
    const task = makeTask({ done: 3, total: 200, startedAt: now - 2_000 });
    expect(taskEtaText(task, now)).toBeNull();
  });

  it("stays quiet when the total is unknown", () => {
    expect(taskEtaText(makeTask({ done: 30, total: 0, startedAt: now - 60_000 }), now)).toBeNull();
  });

  it("stays quiet for a finished task", () => {
    const task = makeTask({ state: "done", done: 200, total: 200, startedAt: now - 60_000 });
    expect(taskEtaText(task, now)).toBeNull();
  });
});

describe("taskFinishedAtText", () => {
  const now = new Date(2026, 10, 18, 20, 0); // 2026-11-18 20:00 local

  it("shows only the time for a task finished today", () => {
    expect(taskFinishedAtText(new Date(2026, 10, 18, 19, 48).getTime(), now)).toBe("19:48");
  });

  it("adds the date for an earlier day", () => {
    expect(taskFinishedAtText(new Date(2026, 0, 15, 9, 5).getTime(), now)).toBe("01-15 09:05");
  });
});
