import { describe, expect, it } from "vitest";
import { formatCount, formatTimestamp, gradeTone, levelTone } from "./format";

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

  it("formatCount renders thousands separators and handles null/undefined", () => {
    expect(formatCount(null)).toBe("—");
    expect(formatCount(undefined)).toBe("—");
    expect(formatCount(NaN)).toBe("—");
    expect(formatCount(0)).toBe("0");
    const big = formatCount(1234567);
    expect(big.replace(/[,.\u202f\s]/g, "")).toBe("1234567");
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
