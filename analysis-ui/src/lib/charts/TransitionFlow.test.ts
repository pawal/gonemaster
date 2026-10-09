import { describe, expect, it } from "vitest";
import { fireEvent, render, screen, within } from "@testing-library/svelte";
import TransitionFlow from "./TransitionFlow.svelte";
import type { FlowLink } from "./flowLayout";

const links: FlowLink[] = [
  { from: "A", to: "A", category: "", count: 905 },
  { from: "A", to: "D", category: "measurement", count: 171 },
  { from: "A", to: "D", category: "real", count: 9 },
  { from: "B", to: "A", category: "mixed", count: 4 },
  { from: "B", to: "B", category: "", count: 20 }
];

function renderFlow(overrides: Partial<{ links: FlowLink[] }> = {}) {
  return render(TransitionFlow, {
    order: ["A+", "A", "B", "C", "D", "F"],
    links,
    fromLabel: "2026-09-08",
    toLabel: "2026-10-02",
    hrefFor: (from: string, to: string) => `/analysis/diff?tab=grade_changed&from_grade=${from}&to_grade=${to}`,
    valueNoun: "grade",
    caption: "Grade transitions",
    ...overrides
  });
}

describe("TransitionFlow", () => {
  it("links every moved ribbon to its pair and leaves held bands unlinked", () => {
    const { container } = renderFlow();
    const group = screen.getByRole("group", { name: "Grade transitions" });
    const ribbons = within(group).getAllByRole("link");
    expect(ribbons.map((a) => a.getAttribute("aria-label"))).toEqual([
      "A to D, real: 9 domains",
      "A to D, measurement: 171 domains",
      "B to A, mixed: 4 domains"
    ]);
    expect(ribbons[1].getAttribute("href")).toBe("/analysis/diff?tab=grade_changed&from_grade=A&to_grade=D");
    expect(container.querySelectorAll("path.ribbon.held")).toHaveLength(2);
  });

  it("hatches a mixed ribbon", () => {
    const { container } = renderFlow();
    const fill = container.querySelector("path.cat-mixed")?.getAttribute("fill") ?? "";
    expect(fill.startsWith("url(#flow-hatch-")).toBe(true);
    expect(container.querySelector(`pattern${fill.slice(4, -1)}`)).not.toBeNull();
  });

  it("names the hovered pair with its category split", async () => {
    const { container } = renderFlow();
    await fireEvent.pointerMove(container.querySelector("path.cat-measurement")!);
    expect(screen.getByText("A → D: 180 domains; 171 measurement, 9 real.")).toBeInTheDocument();
    await fireEvent.pointerMove(container.querySelector("path.held")!);
    expect(screen.getByText("A held: 905 domains.")).toBeInTheDocument();
  });

  it("names a focused ribbon for keyboard readers", async () => {
    renderFlow();
    await fireEvent.focusIn(screen.getByRole("link", { name: "B to A, mixed: 4 domains" }));
    expect(screen.getByText("B → A: 4 domains; 4 mixed.")).toBeInTheDocument();
  });

  it("labels each node with its count", () => {
    const { container } = renderFlow();
    const labels = Array.from(container.querySelectorAll("text.node-label")).map((t) => t.textContent?.trim());
    expect(labels).toEqual(["A 1,085", "B 24", "A 909", "B 20", "D 180"]);
  });

  it("says how many domains it could not draw", () => {
    renderFlow({ links: [...links, { from: "", to: "A", category: "unknown", count: 3 }] });
    expect(screen.getByText("3 domains without a grade on one side are not drawn.")).toBeInTheDocument();
  });

  it("names every cause in the legend", () => {
    renderFlow();
    const legend = screen.getByRole("list", { name: "Cause" });
    expect(within(legend).getAllByRole("listitem").map((li) => li.textContent)).toEqual([
      "Real",
      "Mixed",
      "Measurement",
      "Unknown",
      "Held"
    ]);
  });

  it("renders nothing without links", () => {
    const { container } = renderFlow({ links: [] });
    expect(container.querySelector("figure")).toBeNull();
  });
});
