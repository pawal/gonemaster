import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/svelte";
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
