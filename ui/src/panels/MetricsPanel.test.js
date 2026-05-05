import { render, screen, fireEvent, waitFor, within, cleanup } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import MetricsPanel from "./MetricsPanel.svelte";

const sampleSnapshot = () => ({
  schema_version: "v1",
  server_version: "0.9.16",
  generated_at: "2026-02-10T12:00:00Z",
  health: {
    queue_depth: 4,
    in_flight_jobs: 2,
    dns_cache_hits: 800,
    dns_cache_misses: 200,
    dns_queries_ipv4_total: 600,
    dns_queries_ipv6_total: 400,
    uptime_seconds: 3600,
  },
  jobs: { status_counts: { failed: 5 }, completed_total: 95 },
  api: { routes: [{ latency_ms: { p90: 120 } }] },
  quality: {
    outcomes: { success_rate: 0.95, failed_rate: 0.05, failed_total: 5 },
    job_duration_ms: { avg: 350 },
    severity: { totals: { NOTICE: 1, WARNING: 2, ERROR: 3, CRITICAL: 0 } },
  },
  insights: {
    domains: {
      items: [
        { domain: "example.com", runs_total: 10, last_status: "succeeded", avg_duration_ms: 200, severity_totals: { ERROR: 1 } },
        { domain: "alpha.test", runs_total: 5, last_status: "failed", avg_duration_ms: 500, severity_totals: { CRITICAL: 1 } },
      ],
    },
    batches: {
      items: [
        { batch_id: "batch_1", processed_total: 100, outcomes: { failed: 2, expired: 0, canceled: 0 }, severity_totals: {} },
      ],
    },
  },
  trends: {
    windows: {
      "1h": {
        resolution_seconds: 60,
        points: [
          { throughput: 5, failed: 0, dns_queries_ipv4_per_second: 10, dns_queries_ipv6_per_second: 4, dns_cache_hit_rate: 0.8 },
          { throughput: 8, failed: 1, dns_queries_ipv4_per_second: 12, dns_queries_ipv6_per_second: 5, dns_cache_hit_rate: 0.85 },
        ],
      },
    },
  },
});

describe("MetricsPanel", () => {
  let apiFetch;

  beforeEach(() => {
    apiFetch = vi.fn().mockResolvedValue(sampleSnapshot());
  });

  afterEach(() => cleanup());

  it("loads metrics on mount and renders the queue, DNS, and severity groups", async () => {
    render(MetricsPanel, { props: { apiFetch } });

    await waitFor(() => {
      expect(apiFetch).toHaveBeenCalled();
    });
    expect(await screen.findByLabelText("Throughput trend")).toBeInTheDocument();
    expect(screen.getByText("example.com")).toBeInTheDocument();
    expect(screen.getByText("batch_1")).toBeInTheDocument();
  });

  it("shows the loading state while the first request is in flight", () => {
    let resolveFn;
    const slowFetch = vi.fn(() => new Promise((resolve) => { resolveFn = resolve; }));
    render(MetricsPanel, { props: { apiFetch: slowFetch } });
    expect(screen.getByText(/Loading metrics/i)).toBeInTheDocument();
    resolveFn(sampleSnapshot());
  });

  it("shows the error state with a Retry button when the first load fails", async () => {
    const failing = vi.fn().mockRejectedValue(new Error("metrics unavailable"));
    render(MetricsPanel, { props: { apiFetch: failing } });
    expect(await screen.findByRole("button", { name: /Retry/i })).toBeInTheDocument();
  });

  it("retries after an error and shows the snapshot once it succeeds", async () => {
    const flaky = vi
      .fn()
      .mockRejectedValueOnce(new Error("first call failed"))
      .mockResolvedValue(sampleSnapshot());
    render(MetricsPanel, { props: { apiFetch: flaky } });
    const retry = await screen.findByRole("button", { name: /Retry/i });
    await fireEvent.click(retry);
    expect(await screen.findByLabelText("Throughput trend")).toBeInTheDocument();
  });

  it("calls onNavigateDomain when a top-domain row link is clicked", async () => {
    const onNavigateDomain = vi.fn();
    render(MetricsPanel, { props: { apiFetch, onNavigateDomain } });
    const link = await screen.findByRole("button", { name: "example.com" });
    await fireEvent.click(link);
    expect(onNavigateDomain).toHaveBeenCalledWith("example.com");
  });

  it("toggles a column sort when the header button is clicked", async () => {
    render(MetricsPanel, { props: { apiFetch } });
    await screen.findByText("example.com");
    const runsHeader = screen.getByRole("button", { name: /Runs/i });
    await fireEvent.click(runsHeader);
    const rows = within(document.querySelector(".metrics-table")).getAllByRole("row");
    // Header row is rows[0]; first data row is rows[1]. After clicking Runs (default desc), example.com (10 runs) should come first.
    expect(within(rows[1]).getByText("example.com")).toBeInTheDocument();
  });
});
