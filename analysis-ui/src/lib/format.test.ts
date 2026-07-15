import { describe, expect, it } from "vitest";
import {
  formatCount,
  formatDate,
  formatMs,
  formatTimestamp,
  gradeTone,
  levelTone,
  snapshotDisplayLabel,
  snapshotOptionLabel,
  snapshotSlugDate,
  snapshotSourceDate
} from "./format";

describe("format helpers", () => {
  it("formatTimestamp handles empty, invalid, and Go-zero values", () => {
    expect(formatTimestamp("")).toBe("");
    expect(formatTimestamp(null)).toBe("");
    expect(formatTimestamp(undefined)).toBe("");
    expect(formatTimestamp("not a date")).toBe("");
    expect(formatTimestamp("0001-01-01T00:00:00Z")).toBe("");
  });

  it("formatTimestamp returns a locale string for real dates", () => {
    const result = formatTimestamp("2026-04-17T12:00:00Z");
    expect(result).not.toBe("");
    expect(result).toMatch(/2026/);
  });

  it("formatDate returns only the calendar date", () => {
    expect(formatDate("2026-04-17T12:00:00Z")).toBe("2026-04-17");
    expect(formatDate("0001-01-01T00:00:00Z")).toBe("");
    expect(formatDate("not a date")).toBe("");
  });

  it("snapshot label helpers prefer explicit labels, then source dates, then slugs", () => {
    expect(snapshotSlugDate("2026-04-20-b96e69e375d5")).toBe("2026-04-20");
    expect(snapshotSourceDate({
      slug: "2026-04-20-b96e69e375d5",
      first_run_at: "2026-04-19T23:00:00Z",
      last_run_at: "2026-04-21T01:00:00Z"
    })).toBe("2026-04-21");
    expect(snapshotDisplayLabel({
      slug: "2026-04-20-b96e69e375d5",
      last_run_at: "2026-04-21T01:00:00Z"
    })).toBe("2026-04-21");
    expect(snapshotDisplayLabel({
      slug: "2026-04-20-b96e69e375d5",
      label: "Original",
      last_run_at: "2026-04-21T01:00:00Z"
    })).toBe("Original");
    expect(snapshotOptionLabel({
      slug: "2026-04-20-b96e69e375d5",
      label: "Original",
      last_run_at: "2026-04-21T01:00:00Z"
    })).toBe("Original (2026-04-21)");
    expect(snapshotDisplayLabel({ slug: "custom-snapshot" })).toBe("custom-snapshot");
  });

  it("formatCount renders thousands separators and handles null/undefined", () => {
    expect(formatCount(null)).toBe("-");
    expect(formatCount(undefined)).toBe("-");
    expect(formatCount(NaN)).toBe("-");
    expect(formatCount(0)).toBe("0");
    const big = formatCount(1234567);
    expect(big.replace(/[,.\u202f\s]/g, "")).toBe("1234567");
  });

  it("formatMs rounds by magnitude and blanks missing values", () => {
    // Missing/invalid latency renders as a dash so the UI degrades gracefully.
    expect(formatMs(null)).toBe("-");
    expect(formatMs(undefined)).toBe("-");
    expect(formatMs(NaN)).toBe("-");
    // Sub-10ms keeps one decimal; larger values round to whole ms.
    expect(formatMs(0.4)).toBe("0.4 ms");
    expect(formatMs(9.87)).toBe("9.9 ms");
    expect(formatMs(15.4)).toBe("15 ms");
    expect(formatMs(123.6)).toBe("124 ms");
  });

  it("levelTone returns stable class suffixes", () => {
    expect(levelTone("CRITICAL")).toBe("critical");
    expect(levelTone("error")).toBe("error");
    expect(levelTone("WARNING")).toBe("warning");
    expect(levelTone("NOTICE")).toBe("notice");
    expect(levelTone("")).toBe("neutral");
    expect(levelTone(undefined)).toBe("neutral");
    expect(levelTone("unknown")).toBe("neutral");
  });

  it("gradeTone maps letter grades (case-insensitive)", () => {
    expect(gradeTone("A+")).toBe("aplus");
    expect(gradeTone("a")).toBe("a");
    expect(gradeTone("B")).toBe("b");
    expect(gradeTone("F")).toBe("f");
    expect(gradeTone("")).toBe("neutral");
    expect(gradeTone(null)).toBe("neutral");
  });
});
