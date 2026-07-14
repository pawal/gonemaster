import { describe, expect, it } from "vitest";
import { layoutLine, layoutSparkline, resolveDomain } from "./trendLayout";

// These tests pin the geometry contract the SVG renderers rely on: an empty
// series must produce no path (so the component can show an empty state), a
// single sample must still expose a point (so a lone dot can render), higher
// values must map to smaller y (SVG y grows downward), and the percentage
// domain override must not be widened by "nice" rounding.

describe("resolveDomain", () => {
  it("starts counts at zero and rounds the max to a nice number", () => {
    // 47 -> nice max 50, not 47, so ticks land on readable values.
    expect(resolveDomain([12, 47, 30])).toEqual([0, 50]);
  });

  it("passes an explicit domain through untouched", () => {
    expect(resolveDomain([1, 2, 3], [0, 100])).toEqual([0, 100]);
  });

  it("falls back to [0, 1] when every value is zero", () => {
    // Avoids a divide-by-zero when scaling an all-zero series.
    expect(resolveDomain([0, 0, 0])).toEqual([0, 1]);
  });

  it("ignores non-finite values when finding the max", () => {
    expect(resolveDomain([NaN, 8, Infinity])).toEqual([0, 10]);
  });
});

describe("layoutLine", () => {
  const opts = { width: 100, height: 100, padding: { top: 0, right: 0, bottom: 0, left: 0 } };

  it("returns no path and no points for an empty series", () => {
    const layout = layoutLine([], opts);
    expect(layout.points).toHaveLength(0);
    expect(layout.linePath).toBe("");
    expect(layout.areaPath).toBe("");
  });

  it("centers a single sample and emits a move-only path so a dot can render", () => {
    const layout = layoutLine([5], opts);
    expect(layout.points).toHaveLength(1);
    expect(layout.points[0].x).toBe(50); // centered horizontally
    expect(layout.linePath).toBe("M 50 0"); // value 5 = domain max -> top (y 0)
    expect(layout.areaPath).toBe(""); // no area for a lone point
  });

  it("maps the highest value to the smallest y (top of the plot)", () => {
    const layout = layoutLine([0, 10], { ...opts, yDomain: [0, 10] });
    // First point at min -> baseline (bottom, y=100); second at max -> top (y=0).
    expect(layout.points[0].y).toBe(100);
    expect(layout.points[1].y).toBe(0);
    expect(layout.points[0].x).toBe(0);
    expect(layout.points[1].x).toBe(100);
  });

  it("spaces points evenly across the plot width", () => {
    const layout = layoutLine([1, 2, 3], { ...opts, yDomain: [0, 3] });
    expect(layout.points.map((p) => p.x)).toEqual([0, 50, 100]);
  });

  it("closes the area path back to the baseline", () => {
    const layout = layoutLine([0, 10], { ...opts, yDomain: [0, 10] });
    // Ends by dropping to the baseline under the last and first points, then Z.
    expect(layout.areaPath).toContain("L 100 100");
    expect(layout.areaPath).toContain("L 0 100");
    expect(layout.areaPath.endsWith("Z")).toBe(true);
  });

  it("keeps the percentage domain exactly [0,100] without nice-rounding", () => {
    const layout = layoutLine([12.5, 47.5], { ...opts, yDomain: [0, 100] });
    expect(layout.yMin).toBe(0);
    expect(layout.yMax).toBe(100);
  });

  it("generates readable y ticks within the domain", () => {
    const layout = layoutLine([0, 100], { ...opts, yDomain: [0, 100], tickCount: 4 });
    const values = layout.yTicks.map((t) => t.value);
    expect(values).toContain(0);
    expect(values).toContain(100);
    // Every tick sits inside the domain.
    expect(values.every((v) => v >= 0 && v <= 100)).toBe(true);
  });

  it("places a flat line at the baseline when all values are zero", () => {
    const layout = layoutLine([0, 0, 0], opts);
    expect(layout.points.every((p) => p.y === layout.baselineY)).toBe(true);
  });
});

describe("layoutSparkline", () => {
  it("reports a rising direction when the last value exceeds the first", () => {
    expect(layoutSparkline([1, 2, 5]).direction).toBe(1);
  });

  it("reports a falling direction when the last value is below the first", () => {
    expect(layoutSparkline([5, 2, 1]).direction).toBe(-1);
  });

  it("reports flat when first and last are equal", () => {
    expect(layoutSparkline([3, 9, 3]).direction).toBe(0);
  });

  it("reports flat direction and a null last point for an empty series", () => {
    const layout = layoutSparkline([]);
    expect(layout.direction).toBe(0);
    expect(layout.last).toBeNull();
  });

  it("exposes the final point so the renderer can mark the current value", () => {
    const layout = layoutSparkline([1, 2, 3]);
    expect(layout.last).not.toBeNull();
    expect(layout.last?.index).toBe(2);
    expect(layout.last?.value).toBe(3);
  });
});
