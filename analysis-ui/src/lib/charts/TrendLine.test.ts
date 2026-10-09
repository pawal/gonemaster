import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/svelte";
import TrendLine from "./TrendLine.svelte";
import Sparkline from "./Sparkline.svelte";

describe("TrendLine", () => {
  const labels = ["2026-01", "2026-02", "2026-03"];
  const values = [10, 25, 40];

  it("renders an accessible data table that mirrors the plotted series", () => {
    render(TrendLine, { values, labels, caption: "Algorithm 13 over time" });
    // Every snapshot label and its value must be present as table text so
    // screen readers and touch users get the numbers the SVG only hints at.
    for (const label of labels) {
      expect(screen.getByRole("cell", { name: label })).toBeInTheDocument();
    }
    expect(screen.getByRole("cell", { name: "40" })).toBeInTheDocument();
  });

  it("appends the value suffix to table values in share mode", () => {
    render(TrendLine, {
      values: [12, 48],
      labels: ["a", "b"],
      valueSuffix: "%",
      caption: "Share"
    });
    expect(screen.getByRole("cell", { name: "48%" })).toBeInTheDocument();
  });

  it("labels the chart via aria-label for screen readers", () => {
    render(TrendLine, { values, labels, caption: "NSEC3 adoption" });
    expect(screen.getByRole("img", { name: "NSEC3 adoption" })).toBeInTheDocument();
  });

  it("draws one dot per data point", () => {
    const { container } = render(TrendLine, { values, labels, caption: "c" });
    expect(container.querySelectorAll("circle.dot")).toHaveLength(3);
  });

  it("shows an empty note instead of an empty chart when there are no points", () => {
    render(TrendLine, { values: [], labels: [], caption: "c" });
    expect(screen.getByText(/no data points/i)).toBeInTheDocument();
  });

  it("draws a dashed marker titled with its label", () => {
    const { container } = render(TrendLine, {
      values,
      labels,
      markers: [{ index: 2, label: "Engine changed to v1.7.14" }],
      caption: "c"
    });
    const markers = container.querySelectorAll("line.marker");
    expect(markers).toHaveLength(1);
    expect(markers[0].querySelector("title")?.textContent).toBe("Engine changed to v1.7.14");
  });

  it("spaces dots by time when xValues are given", () => {
    const { container } = render(TrendLine, { values, labels, xValues: [0, 1, 4], caption: "c" });
    const xs = Array.from(container.querySelectorAll("circle.dot")).map((c) => c.getAttribute("cx"));
    expect(xs).toEqual(["40", "187", "628"]);
  });

  it("follows the pointer and names the hovered snapshot and marker", async () => {
    const { container } = render(TrendLine, {
      values,
      labels,
      markers: [{ index: 1, label: "Engine changed to v2" }],
      caption: "c"
    });
    await fireEvent.pointerMove(container.querySelector('rect.hit[data-index="1"]')!);
    expect(container.querySelector("circle.dot.active")?.getAttribute("cx")).toBe("334");
    expect(container.querySelector("line.crosshair")).not.toBeNull();
    expect(screen.getByText("2026-02: 25. Engine changed to v2")).toBeInTheDocument();
    await fireEvent.pointerLeave(container.querySelector("svg")!);
    expect(container.querySelector("circle.dot.active")).toBeNull();
    expect(container.querySelector("line.crosshair")).toBeNull();
  });

  it("reports hover to the parent and draws the controlled index", async () => {
    const onhover = vi.fn();
    const { container } = render(TrendLine, { values, labels, hoverIndex: 0, onhover, caption: "c" });
    expect(container.querySelector("circle.dot.active")?.getAttribute("cx")).toBe("40");
    await fireEvent.pointerMove(container.querySelector('rect.hit[data-index="2"]')!);
    expect(onhover).toHaveBeenCalledWith(2);
    expect(container.querySelector("circle.dot.active")?.getAttribute("cx")).toBe("40");
  });

  it("drops the caption, the hover note and, on request, the table when compact", async () => {
    const { container } = render(TrendLine, { values, labels, compact: true, table: false, caption: "c" });
    expect(container.querySelector("svg")?.getAttribute("viewBox")).toBe("0 0 280 120");
    expect(container.querySelector("figcaption")).toBeNull();
    expect(container.querySelector("table")).toBeNull();
    await fireEvent.pointerMove(container.querySelector('rect.hit[data-index="0"]')!);
    expect(container.querySelector(".hover-note")).toBeNull();
  });
});

describe("Sparkline", () => {
  it("exposes its meaning through an aria-label image role", () => {
    render(Sparkline, { values: [1, 2, 3], ariaLabel: "Domains trending up" });
    expect(screen.getByRole("img", { name: "Domains trending up" })).toBeInTheDocument();
  });

  it("draws a dot at the final data point", () => {
    const { container } = render(Sparkline, { values: [1, 2, 3], ariaLabel: "x" });
    expect(container.querySelector("circle.spark-dot")).not.toBeNull();
  });

  it("renders nothing for an empty series", () => {
    const { container } = render(Sparkline, { values: [], ariaLabel: "x" });
    expect(container.querySelector(".sparkline")).toBeNull();
  });
});
