import { describe, it, expect } from "vitest";
import {
  activeJobStatuses,
  resultReadyStatuses,
  LEVEL_ORDER,
  normalizeStatus,
  isActiveJobStatus,
  isResultReadyStatus,
  progressPercent,
  hasActiveBatchJobs,
  hasRunningOrQueuedJobs,
  normalizeLevel,
  severityRank,
} from "./jobUtils.js";

describe("constants", () => {
  it("exposes the active and result-ready status sets", () => {
    expect(activeJobStatuses).toEqual(["queued", "running"]);
    expect(resultReadyStatuses).toEqual(["succeeded", "failed", "canceled"]);
  });

  it("orders log levels from least to most severe", () => {
    expect(LEVEL_ORDER).toEqual(["DEBUG", "INFO", "NOTICE", "WARNING", "ERROR", "CRITICAL"]);
  });
});

describe("normalizeStatus", () => {
  it("lowercases and coerces null/undefined to empty string", () => {
    expect(normalizeStatus("Running")).toBe("running");
    expect(normalizeStatus(null)).toBe("");
    expect(normalizeStatus(undefined)).toBe("");
  });
});

describe("isActiveJobStatus", () => {
  it("returns true for queued and running, regardless of case", () => {
    expect(isActiveJobStatus("queued")).toBe(true);
    expect(isActiveJobStatus("RUNNING")).toBe(true);
  });

  it("returns false for terminal statuses or unknown values", () => {
    expect(isActiveJobStatus("succeeded")).toBe(false);
    expect(isActiveJobStatus("foo")).toBe(false);
    expect(isActiveJobStatus(null)).toBe(false);
  });
});

describe("isResultReadyStatus", () => {
  it("returns true for succeeded, failed, canceled", () => {
    expect(isResultReadyStatus("succeeded")).toBe(true);
    expect(isResultReadyStatus("FAILED")).toBe(true);
    expect(isResultReadyStatus("canceled")).toBe(true);
  });

  it("returns false for active or unknown statuses", () => {
    expect(isResultReadyStatus("queued")).toBe(false);
    expect(isResultReadyStatus("running")).toBe(false);
    expect(isResultReadyStatus("")).toBe(false);
  });
});

describe("progressPercent", () => {
  it("returns 0 for non-finite or missing progress", () => {
    expect(progressPercent(null)).toBe(0);
    expect(progressPercent({})).toBe(0);
    expect(progressPercent({ progress: NaN })).toBe(0);
  });

  it("clamps to [0, 100]", () => {
    expect(progressPercent({ progress: -5 })).toBe(0);
    expect(progressPercent({ progress: 50 })).toBe(50);
    expect(progressPercent({ progress: 150 })).toBe(100);
  });
});

describe("hasActiveBatchJobs", () => {
  it("returns true if queued or running counts are positive", () => {
    expect(hasActiveBatchJobs({ status_counts: { queued: 0, running: 1 } })).toBe(true);
    expect(hasActiveBatchJobs({ status_counts: { queued: 3 } })).toBe(true);
  });

  it("returns false when no active counts are positive", () => {
    expect(hasActiveBatchJobs({ status_counts: { succeeded: 5 } })).toBe(false);
    expect(hasActiveBatchJobs({})).toBe(false);
    expect(hasActiveBatchJobs(null)).toBe(false);
  });
});

describe("hasRunningOrQueuedJobs", () => {
  it("returns true if any item is queued or running", () => {
    expect(hasRunningOrQueuedJobs([{ status: "succeeded" }, { status: "queued" }])).toBe(true);
    expect(hasRunningOrQueuedJobs([{ status: "RUNNING" }])).toBe(true);
  });

  it("returns false for empty arrays or all-terminal items", () => {
    expect(hasRunningOrQueuedJobs([])).toBe(false);
    expect(hasRunningOrQueuedJobs([{ status: "succeeded" }, { status: "failed" }])).toBe(false);
  });

  it("treats undefined input as empty", () => {
    expect(hasRunningOrQueuedJobs(undefined)).toBe(false);
  });
});

describe("normalizeLevel", () => {
  it("uppercases the level and defaults to INFO", () => {
    expect(normalizeLevel("warning")).toBe("WARNING");
    expect(normalizeLevel(null)).toBe("INFO");
    expect(normalizeLevel("")).toBe("INFO");
  });
});

describe("severityRank", () => {
  it("returns -1 for falsy or unknown levels", () => {
    expect(severityRank(null)).toBe(-1);
    expect(severityRank("")).toBe(-1);
    expect(severityRank("UNKNOWN")).toBe(-1);
  });

  it("ranks NOTICE < WARNING < ERROR < CRITICAL", () => {
    expect(severityRank("NOTICE")).toBeLessThan(severityRank("WARNING"));
    expect(severityRank("WARNING")).toBeLessThan(severityRank("ERROR"));
    expect(severityRank("ERROR")).toBeLessThan(severityRank("CRITICAL"));
  });
});
