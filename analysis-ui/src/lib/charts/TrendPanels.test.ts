import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, within } from "@testing-library/svelte";
import TrendPanels, { type Panel, type PanelPoint } from "./TrendPanels.svelte";

const points: PanelPoint[] = [
  { label: "2026-08-15", title: "2026-08-15", time: Date.UTC(2026, 7, 15), engineVersion: "v1.6.6", crossesEngineBoundary: false },
  { label: "2026-09-02", title: "September", time: Date.UTC(2026, 8, 2), engineVersion: "v1.7.4", crossesEngineBoundary: true },
  { label: "2026-10-02", title: "2026-10-02", time: Date.UTC(2026, 9, 2), engineVersion: "v1.7.4", crossesEngineBoundary: false }
];
const panels: Panel[] = [
  { key: "A", label: "A", tone: "ok", values: [1310, 1121, 905] },
  { key: "D", label: "D", tone: "error", values: [4, 45, 189] }
];

function renderPanels(onselect = vi.fn()) {
  const view = render(TrendPanels, { points, panels, caption: "Grades per snapshot", onselect });
  return { ...view, onselect };
}

describe("TrendPanels", () => {
  it("draws one labelled chart per bucket", () => {
    renderPanels();
    expect(screen.getByRole("img", { name: "A, domains per snapshot" })).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "D, domains per snapshot" })).toBeInTheDocument();
  });

  it("heads each panel with the last value and the delta from the first", () => {
    renderPanels();
    const d = screen.getByRole("region", { name: "D" });
    expect(within(d).getByText("189")).toBeInTheDocument();
    expect(within(d).getByText("+185")).toBeInTheDocument();
    const a = screen.getByRole("region", { name: "A" });
    expect(within(a).getByText("-405")).toBeInTheDocument();
  });

  it("marks the engine change in every panel", () => {
    const { container } = renderPanels();
    expect(container.querySelectorAll("line.marker")).toHaveLength(2);
  });

  it("shares the hovered snapshot across panels and names it below", async () => {
    const { container } = renderPanels();
    const d = screen.getByRole("region", { name: "D" });
    await fireEvent.pointerMove(d.querySelector('rect.hit[data-index="1"]')!);
    expect(container.querySelectorAll("circle.dot.active")).toHaveLength(2);
    expect(within(screen.getByRole("region", { name: "A" })).getByText("1,121")).toBeInTheDocument();
    expect(within(d).getByText("45")).toBeInTheDocument();
    expect(within(d).queryByText("+185")).toBeNull();
    expect(screen.getByText(/September \(2026-09-02\),\s+engine v1\.7\.4\. Engine changed here\./)).toBeInTheDocument();
  });

  it("mirrors every value in one hidden table", () => {
    renderPanels();
    const table = screen.getByRole("table", { name: "Grades per snapshot" });
    const rows = within(table).getAllByRole("row");
    expect(rows).toHaveLength(4);
    expect(within(rows[2]).getAllByRole("cell").map((c) => c.textContent)).toEqual([
      "September",
      "v1.7.4",
      "1,121",
      "45"
    ]);
  });

  it("selects a bucket from its title", async () => {
    const { onselect } = renderPanels();
    await fireEvent.click(screen.getByRole("button", { name: "D" }));
    expect(onselect).toHaveBeenCalledWith("D");
  });
});
