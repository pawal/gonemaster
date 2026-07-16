import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/svelte";
import type { HistoryPoint } from "$lib/api";
import EntityHistorySparkline from "./EntityHistorySparkline.svelte";

function pt(over: Partial<HistoryPoint>): HistoryPoint {
  return { slug: "s", captured_at: "2026-01-01T00:00:00Z", present: true, domain_count: 0, ...over };
}

describe("EntityHistorySparkline", () => {
  it("renders the labelled sparkline for a domain_count series", () => {
    render(EntityHistorySparkline, {
      points: [pt({ domain_count: 3 }), pt({ domain_count: 5 }), pt({ domain_count: 4 })],
      metric: "domain_count",
      label: "Domains over snapshots"
    });
    expect(screen.getByText("Domains over snapshots")).toBeInTheDocument();
    expect(screen.getByText("3 snapshots")).toBeInTheDocument();
    expect(screen.getByRole("img", { name: /across 3 snapshots/ })).toBeInTheDocument();
  });

  it("counts only points carrying the chosen metric", () => {
    // Two points have a score; one absent point and one present-without-score
    // must both drop out, leaving two values (still enough to draw).
    render(EntityHistorySparkline, {
      points: [
        pt({ score: 90 }),
        pt({ present: false, score: 10 }),
        pt({}), // present but no score
        pt({ score: 70 })
      ],
      metric: "score",
      label: "Score"
    });
    expect(screen.getByText("2 snapshots")).toBeInTheDocument();
  });

  it("renders nothing with fewer than two usable points", () => {
    const { container } = render(EntityHistorySparkline, {
      points: [pt({ domain_count: 1 })],
      metric: "domain_count",
      label: "Domains"
    });
    expect(container.querySelector(".history-spark")).toBeNull();
  });
});
