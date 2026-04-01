import { describe, expect, it } from "vitest";
import { levelClass, bannerClass, worstLevel, LEVELS } from "./severity.js";

describe("LEVELS", () => {
  it("contains all six levels in ascending order", () => {
    expect(LEVELS).toEqual(["DEBUG", "INFO", "NOTICE", "WARNING", "ERROR", "CRITICAL"]);
  });
});

describe("levelClass", () => {
  it.each([
    ["DEBUG",    "severity-debug"],
    ["INFO",     "severity-info"],
    ["NOTICE",   "severity-notice"],
    ["WARNING",  "severity-warning"],
    ["ERROR",    "severity-error"],
    ["CRITICAL", "severity-critical"],
  ])("maps %s to %s", (level, expected) => {
    expect(levelClass(level)).toBe(expected);
  });

  it("is case-insensitive", () => {
    expect(levelClass("warning")).toBe("severity-warning");
    expect(levelClass("Error")).toBe("severity-error");
  });

  it("returns empty string for unknown level", () => {
    expect(levelClass("UNKNOWN")).toBe("");
  });

  it("returns empty string for null/undefined", () => {
    expect(levelClass(null)).toBe("");
    expect(levelClass(undefined)).toBe("");
  });
});

describe("bannerClass", () => {
  it.each([
    ["DEBUG",    "ok"],
    ["INFO",     "ok"],
    ["NOTICE",   "ok"],
    ["WARNING",  "warning"],
    ["ERROR",    "error"],
    ["CRITICAL", "critical"],
  ])("maps %s to %s", (level, expected) => {
    expect(bannerClass(level)).toBe(expected);
  });

  it("is case-insensitive", () => {
    expect(bannerClass("critical")).toBe("critical");
  });

  it("defaults to ok for unknown level", () => {
    expect(bannerClass("UNKNOWN")).toBe("ok");
    expect(bannerClass(null)).toBe("ok");
  });
});

describe("worstLevel", () => {
  it("returns INFO for empty input", () => {
    expect(worstLevel([])).toBe("INFO");
    expect(worstLevel(null)).toBe("INFO");
    expect(worstLevel(undefined)).toBe("INFO");
  });

  it("returns the single level when there is one entry", () => {
    expect(worstLevel([{ level: "WARNING" }])).toBe("WARNING");
  });

  it("returns the highest severity from multiple entries", () => {
    const entries = [
      { level: "INFO" },
      { level: "NOTICE" },
      { level: "CRITICAL" },
      { level: "WARNING" },
    ];
    expect(worstLevel(entries)).toBe("CRITICAL");
  });

  it("is case-insensitive on entry levels", () => {
    expect(worstLevel([{ level: "error" }, { level: "notice" }])).toBe("ERROR");
  });

  it("ignores entries with missing level", () => {
    expect(worstLevel([{ level: "WARNING" }, { level: undefined }])).toBe("WARNING");
  });
});
