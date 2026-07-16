import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/svelte";
import FactDistributionBar from "./FactDistributionBar.svelte";
import type { FactBucket } from "$lib/api";

const bucket = (over: Partial<FactBucket> = {}): FactBucket => ({
  key: "ok",
  label: "OK",
  tone: "ok",
  count: 0,
  order: 0,
  ...over
});

describe("FactDistributionBar", () => {
  it("renders a segment per non-empty bucket with label and count", () => {
    const { container } = render(FactDistributionBar, {
      title: "Severity",
      buckets: [bucket({ key: "ok", label: "OK", count: 12 }), bucket({ key: "warn", label: "WARNING", tone: "warning", count: 3 })]
    });
    expect(screen.getByRole("heading", { name: "Severity" })).toBeInTheDocument();
    expect(container.querySelector(".fact-dist-bar")).not.toBeNull();
    expect(screen.getByText("OK")).toBeInTheDocument();
    expect(screen.getByText("12")).toBeInTheDocument();
    expect(screen.getByText("WARNING")).toBeInTheDocument();
    expect(container.querySelector(".empty-state")).toBeNull();
  });

  it("omits buckets whose count is zero", () => {
    const { container } = render(FactDistributionBar, {
      title: "Severity",
      buckets: [bucket({ key: "ok", label: "OK", count: 5 }), bucket({ key: "warn", label: "WARNING", count: 0 })]
    });
    expect(screen.getByText("OK")).toBeInTheDocument();
    expect(screen.queryByText("WARNING")).toBeNull();
    expect(container.querySelector(".empty-state")).toBeNull();
  });

  it("renders the title and an empty-state message instead of vanishing when every bucket is zero", () => {
    const { container } = render(FactDistributionBar, {
      title: "DNSSEC posture",
      description: "How zones are signed.",
      buckets: [bucket({ count: 0 }), bucket({ key: "warn", label: "WARNING", count: 0 })]
    });
    // The section still renders, keeping the metric visible.
    expect(screen.getByRole("heading", { name: "DNSSEC posture" })).toBeInTheDocument();
    expect(screen.getByText("How zones are signed.")).toBeInTheDocument();
    // But it shows the shared empty-state pattern, not a bar.
    expect(container.querySelector(".empty-state")).not.toBeNull();
    expect(container.querySelector(".fact-dist-bar")).toBeNull();
    expect(screen.getByText(/No data for this metric/i)).toBeInTheDocument();
  });

  it("makes segments links when hrefForKey is provided", () => {
    render(FactDistributionBar, {
      title: "Grade",
      buckets: [bucket({ key: "A", label: "A", count: 4 })],
      hrefForKey: (key: string) => `/domains?grade=${key}`
    });
    expect(screen.getByRole("link", { name: /A/ })).toHaveAttribute("href", "/domains?grade=A");
  });
});
