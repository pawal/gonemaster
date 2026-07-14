import { describe, expect, it } from "vitest";
import {
  percentOf,
  seriesShareByKeys,
  seriesShareByTone,
  seriesShareExcludingKeys,
  seriesTotals,
  summarizeMetric
} from "./overview";
import type { TrendPoint } from "./api";

function point(slug: string, payload: unknown): TrendPoint {
  return { slug, captured_at: `${slug}T00:00:00Z`, payload };
}

describe("seriesTotals", () => {
  it("sums each snapshot's bucket counts", () => {
    const points = [point("s1", { ok: 8, critical: 2 }), point("s2", { ok: 5, critical: 5 })];
    expect(seriesTotals(points)).toEqual([10, 10]);
  });

  it("treats a non-object payload as zero", () => {
    expect(seriesTotals([point("s1", [])])).toEqual([0]);
  });
});

describe("seriesShareByTone", () => {
  const keyMeta = {
    ok: { label: "OK", tone: "ok", order: 0 },
    warning: { label: "Warn", tone: "warning", order: 1 },
    critical: { label: "Crit", tone: "critical", order: 2 }
  };

  it("computes the share of domains whose bucket tone matches", () => {
    const points = [point("s1", { ok: 90, warning: 5, critical: 5 })];
    // Healthy = tone ok = 90/100 = 90%.
    expect(seriesShareByTone(points, keyMeta, ["ok"])).toEqual([90]);
    // Issues = warning + critical = 10%.
    expect(seriesShareByTone(points, keyMeta, ["warning", "critical"])).toEqual([10]);
  });

  it("returns zero for an empty snapshot", () => {
    expect(seriesShareByTone([point("s1", {})], keyMeta, ["ok"])).toEqual([0]);
  });
});

describe("seriesShareByKeys", () => {
  it("computes the share falling in specific bucket keys", () => {
    const points = [point("s1", { "A+": 20, A: 30, B: 50 })];
    // Top grades = A+ and A = 50/100 = 50%.
    expect(seriesShareByKeys(points, ["A+", "A"])).toEqual([50]);
  });
});

describe("seriesShareExcludingKeys", () => {
  it("computes the share of everything except the excluded keys", () => {
    const points = [point("s1", { signed: 30, nsec3: 40, unsigned: 30 })];
    // Signed = all but unsigned = 70/100 = 70%.
    expect(seriesShareExcludingKeys(points, ["unsigned"])).toEqual([70]);
  });

  it("returns zero for an empty snapshot rather than 100", () => {
    expect(seriesShareExcludingKeys([point("s1", {})], ["unsigned"])).toEqual([0]);
  });
});

describe("percentOf", () => {
  it("rounds a part over a whole to one decimal", () => {
    expect(percentOf(42, 120)).toBe(35);
    expect(percentOf(1, 3)).toBe(33.3);
  });

  it("returns zero when the whole is zero or negative", () => {
    expect(percentOf(5, 0)).toBe(0);
    expect(percentOf(5, -1)).toBe(0);
  });
});

describe("summarizeMetric", () => {
  it("reports the latest value and its movement vs the previous snapshot", () => {
    const s = summarizeMetric([40, 45, 50]);
    expect(s.latest).toBe(50);
    expect(s.previous).toBe(45);
    expect(s.delta).toBe(5);
  });

  it("has no delta for a single snapshot", () => {
    const s = summarizeMetric([42]);
    expect(s.latest).toBe(42);
    expect(s.previous).toBeNull();
    expect(s.delta).toBeNull();
  });

  it("has null everything for an empty series", () => {
    expect(summarizeMetric([])).toEqual({ values: [], latest: null, previous: null, delta: null });
  });
});
