import { describe, it, expect } from "vitest";
import {
  formatPercent,
  formatInteger,
  formatCompactInteger,
  formatDurationMs,
  formatRate,
  formatUptime,
  parseTimestamp,
  formatTimestampLocal,
  formatBatchTotalRuntime,
  prettyProfileJSON,
  formatSnapshotSlugPreview,
} from "./format.js";

describe("formatPercent", () => {
  it("scales 0..1 to xx.x%", () => {
    expect(formatPercent(0.5)).toBe("50.0%");
    expect(formatPercent(0.123)).toBe("12.3%");
    expect(formatPercent(1)).toBe("100.0%");
  });

  it("treats null/undefined as 0", () => {
    expect(formatPercent(null)).toBe("0.0%");
    expect(formatPercent(undefined)).toBe("0.0%");
  });
});

describe("formatInteger", () => {
  it("rounds and locale-formats finite numbers", () => {
    expect(formatInteger(1234)).toBe((1234).toLocaleString());
    expect(formatInteger(1234.7)).toBe((1235).toLocaleString());
  });

  it("returns '0' for non-finite values", () => {
    expect(formatInteger(NaN)).toBe("0");
    expect(formatInteger("foo")).toBe("0");
    expect(formatInteger(undefined)).toBe("0");
  });
});

describe("formatCompactInteger", () => {
  it("returns the locale integer for values under 1000", () => {
    expect(formatCompactInteger(0)).toBe("0");
    expect(formatCompactInteger(999)).toBe((999).toLocaleString());
  });

  it("formats thousands as K, millions as M, etc., with a comma decimal", () => {
    expect(formatCompactInteger(1500)).toBe("1,5K");
    expect(formatCompactInteger(2_000_000)).toBe("2M");
    expect(formatCompactInteger(1_500_000_000)).toBe("1,5B");
    expect(formatCompactInteger(1_500_000_000_000)).toBe("1,5T");
  });

  it("preserves sign for negatives", () => {
    expect(formatCompactInteger(-1500)).toBe("-1,5K");
  });

  it("returns '0' for non-finite values", () => {
    expect(formatCompactInteger(NaN)).toBe("0");
  });
});

describe("formatDurationMs", () => {
  it("appends ' ms' to a formatted integer", () => {
    expect(formatDurationMs(500)).toBe(`${(500).toLocaleString()} ms`);
  });
});

describe("formatRate", () => {
  it("returns '0/s' for non-positive or non-finite", () => {
    expect(formatRate(0)).toBe("0/s");
    expect(formatRate(-5)).toBe("0/s");
    expect(formatRate(NaN)).toBe("0/s");
  });

  it("formats positive rates with one decimal using comma", () => {
    expect(formatRate(2.5)).toBe("2,5/s");
    expect(formatRate(2)).toBe("2/s");
  });
});

describe("formatUptime", () => {
  it("returns 'unknown' for negative or non-finite seconds", () => {
    expect(formatUptime(-1)).toBe("unknown");
    expect(formatUptime(NaN)).toBe("unknown");
  });

  it("uses seconds, minutes, hours, days as the value grows", () => {
    expect(formatUptime(45)).toBe("45s");
    expect(formatUptime(125)).toBe("2m 5s");
    expect(formatUptime(3700)).toBe("1h 1m");
    expect(formatUptime(90061)).toBe("1d 1h");
  });
});

describe("parseTimestamp", () => {
  it("returns null for falsy or invalid input", () => {
    expect(parseTimestamp(null)).toBeNull();
    expect(parseTimestamp("")).toBeNull();
    expect(parseTimestamp("not-a-date")).toBeNull();
  });

  it("parses an ISO timestamp", () => {
    const d = parseTimestamp("2026-01-02T03:04:05Z");
    expect(d).not.toBeNull();
    expect(d.toISOString()).toBe("2026-01-02T03:04:05.000Z");
  });
});

describe("formatTimestampLocal", () => {
  it("returns 'unknown' for invalid input", () => {
    expect(formatTimestampLocal("")).toBe("unknown");
    expect(formatTimestampLocal("garbage")).toBe("unknown");
  });

  it("returns a Swedish-locale date string for valid input", () => {
    const out = formatTimestampLocal("2026-01-02T03:04:05Z");
    expect(typeof out).toBe("string");
    expect(out).not.toBe("unknown");
  });
});

describe("formatBatchTotalRuntime", () => {
  it("returns 'unknown' when created_at is missing", () => {
    expect(formatBatchTotalRuntime({})).toBe("unknown");
    expect(formatBatchTotalRuntime({ created_at: null })).toBe("unknown");
  });

  it("includes ' (running)' when finished_at is missing", () => {
    const created = new Date(Date.now() - 65_000).toISOString();
    const out = formatBatchTotalRuntime({ created_at: created });
    expect(out).toMatch(/ \(running\)$/);
  });

  it("omits the running suffix when finished_at is present", () => {
    const out = formatBatchTotalRuntime({
      created_at: "2026-01-01T00:00:00Z",
      finished_at: "2026-01-01T00:01:30Z",
    });
    expect(out).toBe("1m 30s");
  });
});

describe("prettyProfileJSON", () => {
  it("returns empty string for falsy input", () => {
    expect(prettyProfileJSON(null)).toBe("");
    expect(prettyProfileJSON(undefined)).toBe("");
    expect(prettyProfileJSON("")).toBe("");
  });

  it("pretty-prints an object", () => {
    expect(prettyProfileJSON({ a: 1 })).toBe('{\n  "a": 1\n}');
  });

  it("pretty-prints valid JSON strings", () => {
    expect(prettyProfileJSON('{"a":1}')).toBe('{\n  "a": 1\n}');
  });

  it("returns the original string when JSON parsing fails", () => {
    expect(prettyProfileJSON("not json")).toBe("not json");
  });
});

describe("formatSnapshotSlugPreview", () => {
  it("returns YYYY-MM-DD-<batch-hash>", () => {
    const today = new Date().toISOString().slice(0, 10);
    expect(formatSnapshotSlugPreview()).toBe(`${today}-<batch-hash>`);
  });
});
