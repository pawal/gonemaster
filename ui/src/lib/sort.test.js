import { describe, it, expect } from "vitest";
import {
  compareText,
  compareNumber,
  timestampValue,
  compareTimestamp,
  compareSeverity,
  nextTableSort,
  tableSortIndicator,
  tableSortAria,
  sortItems,
} from "./sort.js";

describe("compareText", () => {
  it("compares strings using a numeric, base-sensitivity collator", () => {
    expect(Math.sign(compareText("abc", "abd"))).toBe(-1);
    expect(Math.sign(compareText("ABC", "abc"))).toBe(0);
    expect(Math.sign(compareText("file2", "file10"))).toBe(-1);
  });

  it("treats null and undefined as empty strings", () => {
    expect(compareText(null, "")).toBe(0);
    expect(compareText(undefined, "")).toBe(0);
  });
});

describe("compareNumber", () => {
  it("returns the numeric difference", () => {
    expect(compareNumber(3, 1)).toBe(2);
    expect(compareNumber(1, 3)).toBe(-2);
    expect(compareNumber(2, 2)).toBe(0);
  });

  it("coerces null/undefined to 0", () => {
    expect(compareNumber(null, 5)).toBe(-5);
    expect(compareNumber(undefined, 0)).toBe(0);
  });
});

describe("timestampValue", () => {
  it("returns -1 for missing or invalid input", () => {
    expect(timestampValue(null)).toBe(-1);
    expect(timestampValue("")).toBe(-1);
    expect(timestampValue("not-a-date")).toBe(-1);
  });

  it("returns the millisecond timestamp for valid input", () => {
    const expected = new Date("2026-01-02T03:04:05Z").getTime();
    expect(timestampValue("2026-01-02T03:04:05Z")).toBe(expected);
  });
});

describe("compareTimestamp", () => {
  it("orders earlier timestamps before later ones", () => {
    expect(Math.sign(compareTimestamp("2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"))).toBe(-1);
    expect(Math.sign(compareTimestamp("2026-01-02T00:00:00Z", "2026-01-01T00:00:00Z"))).toBe(1);
  });

  it("treats invalid timestamps as -1 (sorts before any valid date)", () => {
    expect(Math.sign(compareTimestamp("garbage", "2026-01-01T00:00:00Z"))).toBe(-1);
  });
});

describe("compareSeverity", () => {
  it("orders by log-level severity", () => {
    expect(Math.sign(compareSeverity("NOTICE", "ERROR"))).toBe(-1);
    expect(Math.sign(compareSeverity("CRITICAL", "WARNING"))).toBe(1);
    expect(compareSeverity("ERROR", "ERROR")).toBe(0);
  });
});

describe("nextTableSort", () => {
  it("toggles direction when the same key is re-clicked", () => {
    expect(nextTableSort({ key: "name", direction: "asc" }, "name")).toEqual({
      key: "name",
      direction: "desc",
    });
    expect(nextTableSort({ key: "name", direction: "desc" }, "name")).toEqual({
      key: "name",
      direction: "asc",
    });
  });

  it("starts a different key with the supplied default direction", () => {
    expect(nextTableSort({ key: "name", direction: "asc" }, "score")).toEqual({
      key: "score",
      direction: "asc",
    });
    expect(nextTableSort({ key: "name", direction: "asc" }, "score", "desc")).toEqual({
      key: "score",
      direction: "desc",
    });
  });
});

describe("tableSortIndicator", () => {
  it("returns empty when the column is not the active key", () => {
    expect(tableSortIndicator({ key: "name", direction: "asc" }, "score")).toBe("");
  });

  it("returns the up/down arrow for the active column", () => {
    expect(tableSortIndicator({ key: "name", direction: "asc" }, "name")).toBe("▲");
    expect(tableSortIndicator({ key: "name", direction: "desc" }, "name")).toBe("▼");
  });
});

describe("tableSortAria", () => {
  it("returns 'none' when the column is not the active key", () => {
    expect(tableSortAria({ key: "name", direction: "asc" }, "score")).toBe("none");
  });

  it("returns 'ascending' or 'descending' for the active column", () => {
    expect(tableSortAria({ key: "name", direction: "asc" }, "name")).toBe("ascending");
    expect(tableSortAria({ key: "name", direction: "desc" }, "name")).toBe("descending");
  });
});

describe("sortItems", () => {
  const comparators = {
    name: (a, b) => compareText(a.name, b.name),
    score: (a, b) => compareNumber(a.score, b.score),
  };

  it("returns a new sorted array without mutating the input", () => {
    const items = [
      { name: "b", score: 1 },
      { name: "a", score: 2 },
    ];
    const original = items.slice();
    const sorted = sortItems(items, { key: "name", direction: "asc" }, comparators);
    expect(sorted.map((x) => x.name)).toEqual(["a", "b"]);
    expect(items).toEqual(original);
  });

  it("returns an unsorted copy when the key has no comparator", () => {
    const items = [{ name: "b" }, { name: "a" }];
    const sorted = sortItems(items, { key: "missing", direction: "asc" }, comparators);
    expect(sorted.map((x) => x.name)).toEqual(["b", "a"]);
  });

  it("reverses order on direction='desc'", () => {
    const items = [
      { name: "a", score: 1 },
      { name: "b", score: 2 },
    ];
    const sorted = sortItems(items, { key: "score", direction: "desc" }, comparators);
    expect(sorted.map((x) => x.name)).toEqual(["b", "a"]);
  });

  it("applies the tie-breaker when the primary comparator returns 0", () => {
    const items = [
      { name: "b", score: 1 },
      { name: "a", score: 1 },
    ];
    const sorted = sortItems(
      items,
      { key: "score", direction: "asc" },
      comparators,
      (left, right) => compareText(left.name, right.name),
    );
    expect(sorted.map((x) => x.name)).toEqual(["a", "b"]);
  });

  it("handles non-array inputs gracefully", () => {
    expect(sortItems(null, { key: "name", direction: "asc" }, comparators)).toEqual([]);
    expect(sortItems(undefined, { key: "name", direction: "asc" }, comparators)).toEqual([]);
  });
});
