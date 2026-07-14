import { describe, expect, it } from "vitest";
import { computeTagMovers, pinnedSeries } from "./trends";
import type { TrendPoint } from "./api";

describe("pinnedSeries", () => {
  const series = [
    { slug: "s1", label: "Jan", buckets: [{ key: "13", count: 20 }, { key: "8", count: 80 }] },
    { slug: "s2", label: "Feb", buckets: [{ key: "13", count: 50 }, { key: "8", count: 50 }] }
  ];

  it("pulls one bucket's count and share across snapshots in order", () => {
    const pinned = pinnedSeries(series, "13");
    expect(pinned.map((p) => p.count)).toEqual([20, 50]);
    // 20 of 100 = 20%, 50 of 100 = 50%.
    expect(pinned.map((p) => p.share)).toEqual([20, 50]);
    expect(pinned.map((p) => p.label)).toEqual(["Jan", "Feb"]);
  });

  it("returns zero for a snapshot missing the bucket", () => {
    const pinned = pinnedSeries(
      [{ slug: "s1", label: "Jan", buckets: [{ key: "8", count: 10 }] }],
      "13"
    );
    expect(pinned[0].count).toBe(0);
    expect(pinned[0].share).toBe(0);
  });

  it("reports zero share when a snapshot has no domains", () => {
    const pinned = pinnedSeries([{ slug: "s1", label: "Jan", buckets: [] }], "13");
    expect(pinned[0].share).toBe(0);
  });
});

describe("computeTagMovers", () => {
  function point(slug: string, tags: { tag: string; level?: string; domain_count: number }[]): TrendPoint {
    return { slug, captured_at: `${slug}T00:00:00Z`, payload: tags };
  }

  it("ranks tags by absolute change between first and last snapshot", () => {
    const points = [
      point("s1", [
        { tag: "DS02", level: "ERROR", domain_count: 100 },
        { tag: "NS01", level: "WARNING", domain_count: 10 }
      ]),
      point("s2", [
        { tag: "DS02", level: "ERROR", domain_count: 60 }, // -40
        { tag: "NS01", level: "WARNING", domain_count: 25 } // +15
      ])
    ];
    const movers = computeTagMovers(points);
    expect(movers.map((m) => m.tag)).toEqual(["DS02", "NS01"]);
    expect(movers[0].delta).toBe(-40);
    expect(movers[1].delta).toBe(15);
  });

  it("treats a newly-appearing tag as rising from zero", () => {
    const points = [
      point("s1", [{ tag: "DS02", domain_count: 5 }]),
      point("s2", [
        { tag: "DS02", domain_count: 5 }, // unchanged -> dropped
        { tag: "NEW", domain_count: 30 } // +30 from absent
      ])
    ];
    const movers = computeTagMovers(points);
    expect(movers).toHaveLength(1);
    expect(movers[0].tag).toBe("NEW");
    expect(movers[0].first).toBe(0);
    expect(movers[0].delta).toBe(30);
  });

  it("omits tags whose count did not change", () => {
    const points = [
      point("s1", [{ tag: "DS02", domain_count: 5 }]),
      point("s2", [{ tag: "DS02", domain_count: 5 }])
    ];
    expect(computeTagMovers(points)).toEqual([]);
  });

  it("returns nothing when there are fewer than two snapshots", () => {
    expect(computeTagMovers([point("s1", [{ tag: "DS02", domain_count: 5 }])])).toEqual([]);
  });

  it("honours the limit", () => {
    const many = Array.from({ length: 20 }, (_, i) => ({ tag: `T${i}`, domain_count: i + 1 }));
    const points = [
      point("s1", many.map((t) => ({ ...t, domain_count: 0 }))),
      point("s2", many)
    ];
    expect(computeTagMovers(points, 5)).toHaveLength(5);
  });
});
