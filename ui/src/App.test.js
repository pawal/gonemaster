import { render, screen, fireEvent, waitFor, within, cleanup } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App.svelte";
import { setCatalog, locale } from "./i18n.js";

const jsonResponse = (data, ok = true) => ({
  ok,
  statusText: ok ? "OK" : "Bad Request",
  headers: {
    get: () => "application/json"
  },
  json: async () => data,
  text: async () => JSON.stringify(data)
});

const emptyResponse = () => ({
  ok: true,
  statusText: "No Content",
  headers: {
    get: () => ""
  },
  json: async () => ({}),
  text: async () => ""
});

const sampleProfiles = () => ([
  {
    id: 11,
    name: "baseline",
    description: "Default baseline",
    config: { net: { ipv4: true, ipv6: true } },
    public: false,
    created_at: "2026-04-01T10:00:00Z",
    updated_at: "2026-04-02T10:00:00Z"
  },
  {
    id: 12,
    name: "strict",
    description: "Strict DNS profile",
    config: { resolver: { defaults: { timeout: 5 } } },
    public: true,
    created_at: "2026-04-01T11:00:00Z",
    updated_at: "2026-04-03T09:30:00Z"
  }
]);

describe("App", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    window.history.replaceState(null, "", "/");
    localStorage.clear();
    global.fetch = vi.fn();
  });

  afterEach(() => {
    cleanup();
  });

  const openRecentTab = async () => {
    await fireEvent.click(screen.getByRole("tab", { name: "Recent Tests" }));
  };

  const openBatchTab = async () => {
    await fireEvent.click(screen.getByRole("tab", { name: "Batch Jobs" }));
  };

  const openMetricsTab = async () => {
    await fireEvent.click(screen.getByRole("tab", { name: "Metrics" }));
  };

  const openSettingsTab = async () => {
    await fireEvent.click(screen.getByRole("tab", { name: "Settings" }));
  };

  const getMetricsPanel = () => screen.getByRole("tabpanel", { name: "Metrics" });

  it("renders the main sections", async () => {
    global.fetch.mockImplementation(() => jsonResponse({ items: [] }));

    const { container, unmount } = render(App);

    expect(screen.getByAltText("gonemaster")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Single Job" })).toBeInTheDocument();
    expect(screen.getByRole("tablist", { name: "Job views" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Single Job" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Recent Tests" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Batch Jobs" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Metrics" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Settings" })).toBeInTheDocument();
    expect(screen.getByText("Job Inspector")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Recent Tests" })).toBeNull();
    expect(screen.queryByText("Batch Inspector")).not.toBeInTheDocument();
    expect(container.querySelector("nav.sidebar")).toBeInTheDocument();
    expect(container.querySelector("section[role='tabpanel']")).toBeNull();

    await openBatchTab();
    expect(screen.getByText("Batch Inspector")).toBeInTheDocument();

    await openSettingsTab();
    expect(screen.getByText("Profile Library")).toBeInTheDocument();

    unmount();
  });

  it("refreshes recent tests list when clicking the recent tests tab", async () => {
    const calls = [];
    global.fetch.mockImplementation((url) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      calls.push(value);
      if (value.includes("/api/v1/jobs?")) {
        return jsonResponse({ items: [], total: 0 });
      }
      return jsonResponse({});
    });

    const { unmount } = render(App);

    await waitFor(() => {
      expect(calls.some((value) => value.includes("/api/v1/jobs?"))).toBe(true);
    });
    calls.length = 0;

    await openRecentTab();
    await waitFor(() => {
      expect(calls.some((value) => value.includes("/api/v1/jobs?"))).toBe(true);
    });

    unmount();
  });

  it("applies recent domain filter to jobs query", async () => {
    const calls = [];
    global.fetch.mockImplementation((url) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      calls.push(value);
      if (value.includes("/api/v1/jobs?")) {
        return jsonResponse({ items: [], total: 0 });
      }
      return jsonResponse({});
    });

    const { unmount } = render(App);
    await openRecentTab();

    calls.length = 0;
    const domainInput = screen.getByLabelText("Domain contains");
    await fireEvent.input(domainInput, { target: { value: "joburg" } });
    await fireEvent.keyDown(domainInput, { key: "Enter", code: "Enter" });

    await waitFor(() => {
      expect(calls.some((value) => value.includes("domain=joburg"))).toBe(true);
    });

    unmount();
  });

  it("auto-refreshes recent tests and stops when jobs are no longer queued or running", async () => {
    let jobsCallCount = 0;
    const runningJob = {
      id: "job_live",
      domain: "example.com",
      status: "running",
      created_at: "2026-02-03T00:00:00Z",
      progress: 65
    };
    const doneJob = {
      ...runningJob,
      status: "succeeded",
      progress: 100
    };

    global.fetch.mockImplementation((url) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      if (value.includes("/api/v1/jobs?")) {
        jobsCallCount += 1;
        if (jobsCallCount >= 3) {
          return jsonResponse({ items: [doneJob], total: 1 });
        }
        return jsonResponse({ items: [runningJob], total: 1 });
      }
      return jsonResponse({});
    });

    const { unmount } = render(App);

    await openRecentTab();
    expect(await screen.findByText("job_live")).toBeInTheDocument();

    const intervalCallbacks = [];
    vi.spyOn(global, "setInterval").mockImplementation((callback) => {
      intervalCallbacks.push(callback);
      return intervalCallbacks.length;
    });
    vi.spyOn(global, "clearInterval").mockImplementation(() => {});

    await fireEvent.click(screen.getByRole("button", { name: /auto refresh: off/i }));
    expect(screen.getByRole("button", { name: /auto refresh: on/i })).toBeInTheDocument();
    expect(intervalCallbacks.length).toBeGreaterThan(0);

    const poll = intervalCallbacks[intervalCallbacks.length - 1];
    expect(typeof poll).toBe("function");
    await poll();

    await waitFor(() => {
      expect(screen.getByRole("button", { name: /auto refresh: off/i })).toBeInTheDocument();
      expect(screen.getByText("job_live")).toBeInTheDocument();
      expect(screen.getByText("example.com \u2013 succeeded")).toBeInTheDocument();
    });
    expect(jobsCallCount).toBeGreaterThanOrEqual(3);

    unmount();
  });

  it("auto-refreshes batch inspector and stops when no batch jobs are queued or running", async () => {
    let batchCalls = 0;
    const runningBatch = {
      batch_id: "batch_live",
      total: 1,
      status_counts: { running: 1 },
      items: [
        {
          id: "job_batch_live",
          domain: "batch.example",
          status: "running",
          created_at: "2026-02-03T00:00:00Z",
          progress: 55
        }
      ],
      created_at: "2026-02-03T00:00:00Z"
    };
    const doneBatch = {
      ...runningBatch,
      status_counts: { succeeded: 1 },
      items: [
        {
          ...runningBatch.items[0],
          status: "succeeded",
          progress: 100
        }
      ]
    };

    global.fetch.mockImplementation((url) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      if (value.includes("/api/v1/jobs?")) {
        return jsonResponse({ items: [], total: 0 });
      }
      if (value.includes("/api/v1/batches/batch_live")) {
        batchCalls += 1;
        return jsonResponse(batchCalls >= 2 ? doneBatch : runningBatch);
      }
      return jsonResponse({});
    });

    const { unmount } = render(App);

    await openBatchTab();
    await fireEvent.input(screen.getByLabelText("Batch ID"), { target: { value: "batch_live" } });
    await fireEvent.change(screen.getByLabelText("Batch ID"));
    expect(await screen.findByText("job_batch_live")).toBeInTheDocument();

    const intervalCallbacks = [];
    vi.spyOn(global, "setInterval").mockImplementation((callback) => {
      intervalCallbacks.push(callback);
      return intervalCallbacks.length;
    });
    vi.spyOn(global, "clearInterval").mockImplementation(() => {});

    await fireEvent.click(screen.getByRole("button", { name: /auto refresh: off/i }));
    expect(screen.getByRole("button", { name: /auto refresh: on/i })).toBeInTheDocument();
    expect(intervalCallbacks.length).toBeGreaterThan(0);

    const poll = intervalCallbacks[intervalCallbacks.length - 1];
    expect(typeof poll).toBe("function");
    await poll();

    await waitFor(() => {
      expect(screen.getByRole("button", { name: /auto refresh: off/i })).toBeInTheDocument();
      expect(screen.getByText("batch.example - succeeded")).toBeInTheDocument();
    });
    expect(batchCalls).toBeGreaterThanOrEqual(2);

    unmount();
  });

  it("refreshes batch inspector when opening batch tab with selected batch id", async () => {
    let batchCalls = 0;
    const batch = {
      batch_id: "batch_refresh",
      total: 1,
      status_counts: { running: 1 },
      items: [
        {
          id: "job_batch_refresh",
          domain: "refresh.example",
          status: "running",
          created_at: "2026-02-03T00:00:00Z",
          progress: 40
        }
      ],
      created_at: "2026-02-03T00:00:00Z"
    };

    global.fetch.mockImplementation((url) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      if (value.includes("/api/v1/jobs?")) {
        return jsonResponse({ items: [], total: 0 });
      }
      if (value.includes("/api/v1/batches/batch_refresh")) {
        batchCalls += 1;
        return jsonResponse(batch);
      }
      return jsonResponse({});
    });

    const { unmount } = render(App);

    await openBatchTab();
    await fireEvent.input(screen.getByLabelText("Batch ID"), { target: { value: "batch_refresh" } });
    await fireEvent.change(screen.getByLabelText("Batch ID"));
    await screen.findByText("job_batch_refresh");

    const callsAfterInitialLoad = batchCalls;
    await openRecentTab();
    await openBatchTab();

    await waitFor(() => {
      expect(batchCalls).toBeGreaterThan(callsAfterInitialLoad);
    });

    expect(screen.getByRole("button", { name: /auto refresh: off/i })).toBeInTheDocument();
    unmount();
  });

  it("lists latest batches in dropdown and loads selected batch", async () => {
    const jobsPage = {
      items: [
        { id: "job_1", batch_id: "batch_new", created_at: "2026-02-04T10:00:00Z" },
        { id: "job_2", batch_id: "batch_mid", created_at: "2026-02-04T09:00:00Z" },
        { id: "job_3", batch_id: "batch_new", created_at: "2026-02-04T08:00:00Z" },
        { id: "job_4", batch_id: "batch_old", created_at: "2026-02-04T07:00:00Z" }
      ],
      total: 4
    };
    const batchMid = {
      batch_id: "batch_mid",
      total: 1,
      status_counts: { succeeded: 1 },
      items: [
        {
          id: "job_mid",
          domain: "mid.example",
          status: "succeeded",
          created_at: "2026-02-04T09:00:00Z",
          progress: 100
        }
      ],
      created_at: "2026-02-04T09:00:00Z"
    };

    global.fetch.mockImplementation((url) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      if (value.includes("/api/v1/jobs?")) {
        return jsonResponse(jobsPage);
      }
      if (value.includes("/api/v1/batches/batch_mid")) {
        return jsonResponse(batchMid);
      }
      return jsonResponse({});
    });

    const { unmount } = render(App);

    await openBatchTab();
    const recentBatchesSelect = screen.getByLabelText("Recent batches");
    await waitFor(() => {
      const options = within(recentBatchesSelect).getAllByRole("option");
      expect(options[1]).toHaveValue("batch_new");
      expect(options[2]).toHaveValue("batch_mid");
      expect(options[3]).toHaveValue("batch_old");
    });

    await fireEvent.change(recentBatchesSelect, { target: { value: "batch_mid" } });
    expect(await screen.findByText("job_mid")).toBeInTheDocument();

    unmount();
  });

  it("renders metrics cards, trends, and insight tables", async () => {
    const metricsPayload = {
      schema_version: "v1",
      server_version: "0.9.16",
      generated_at: "2026-02-10T12:00:00Z",
      health: {
        queue_depth: 3,
        in_flight_jobs: 2,
        dns_cache_hits: 30,
        dns_cache_misses: 17,
        dns_queries_ipv4_total: 11234,
        dns_queries_ipv6_total: 22000000
      },
      jobs: {
        completed_total: 7,
        status_counts: {
          failed: 4321,
          queued: 1,
          running: 1
        }
      },
      api: {
        routes: [
          { latency_ms: { p90: 320 } }
        ]
      },
      quality: {
        outcomes: {
          failed_total: 2,
          success_rate: 0.8,
          failed_rate: 0.15
        },
        job_duration_ms: {
          avg: 1450
        },
        severity: {
          totals: {
            NOTICE: 5,
            WARNING: 2,
            ERROR: 1,
            CRITICAL: 0
          }
        }
      },
      insights: {
        domains: {
          items: [
            {
              domain: "alpha.example",
              runs_total: 3,
              last_status: "failed",
              avg_duration_ms: 1777,
              severity_totals: {
                ERROR: 2,
                CRITICAL: 1
              }
            }
          ]
        },
        batches: {
          items: [
            {
              batch_id: "batch_a",
              processed_total: 4,
              outcomes: {
                failed: 2,
                expired: 1,
                canceled: 0
              },
              severity_totals: {
                ERROR: 2,
                CRITICAL: 1
              }
            }
          ]
        }
      },
      trends: {
        windows: {
          "1h": {
            points: [
              {
                throughput: 1,
                failed: 0,
                queue_depth: 2,
                dns_cache_hit_rate: 0.4,
                dns_queries_per_second: 11.2,
                dns_queries_ipv4_per_second: 7.1,
                dns_queries_ipv6_per_second: 4.1
              },
              {
                throughput: 3,
                failed: 1,
                queue_depth: 4,
                dns_cache_hit_rate: 0.75,
                dns_queries_per_second: 18.6,
                dns_queries_ipv4_per_second: 11.8,
                dns_queries_ipv6_per_second: 6.8
              }
            ]
          }
        }
      }
    };

    global.fetch.mockImplementation((url) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      if (value.includes("/api/v1/jobs?")) {
        return jsonResponse({ items: [], total: 0 });
      }
      if (value.startsWith("/api/v1/metrics?")) {
        return jsonResponse(metricsPayload);
      }
      return jsonResponse({});
    });

    const { unmount } = render(App);
    await openMetricsTab();
    const metricsPanel = getMetricsPanel();

    expect(await within(metricsPanel).findByLabelText("DNS query rates trend")).toBeInTheDocument();
    expect(within(metricsPanel).getByLabelText("Cache hit rate trend")).toBeInTheDocument();
    expect(within(metricsPanel).getByText("In-flight jobs")).toBeInTheDocument();
    expect(within(metricsPanel).getByText("External IPv4")).toBeInTheDocument();
    expect(within(metricsPanel).getByText("External IPv6")).toBeInTheDocument();
    expect(within(metricsPanel).getByText("Cache misses")).toBeInTheDocument();
    expect(within(metricsPanel).getByText("11,2K")).toBeInTheDocument();
    expect(within(metricsPanel).getByText("17")).toBeInTheDocument();
    expect(within(metricsPanel).getByText("22M")).toBeInTheDocument();
    expect(within(metricsPanel).getByText("4,321")).toBeInTheDocument();
    expect(within(metricsPanel).getByText("80.0%")).toBeInTheDocument();
    expect(within(metricsPanel).getByText("15.0%")).toBeInTheDocument();
    expect(within(metricsPanel).getAllByText("75.0%")).toHaveLength(1);
    expect(within(metricsPanel).getByText("320 ms")).toBeInTheDocument();
    expect(within(metricsPanel).getByText("Total jobs finished")).toBeInTheDocument();
    expect(within(metricsPanel).getByText("Failed jobs")).toBeInTheDocument();
    expect(within(metricsPanel).getByText(/Server uptime:/)).toBeInTheDocument();
    expect(within(metricsPanel).getByText(/Server version:/)).toBeInTheDocument();
    expect(within(metricsPanel).getByRole("heading", { name: "Top domains" })).toBeInTheDocument();
    expect(within(metricsPanel).getByText("alpha.example")).toBeInTheDocument();
    expect(within(metricsPanel).getByRole("heading", { name: "Error-heavy batches" })).toBeInTheDocument();
    expect(within(metricsPanel).getByText("batch_a")).toBeInTheDocument();

    unmount();
  });

  it("auto-refreshes metrics tab on polling interval", async () => {
    const metricsCalls = [];
    let metricsCount = 0;
    global.fetch.mockImplementation((url) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      if (value.includes("/api/v1/jobs?")) {
        return jsonResponse({ items: [], total: 0 });
      }
      if (value.startsWith("/api/v1/metrics?")) {
        metricsCalls.push(value);
        metricsCount += 1;
        return jsonResponse({
          schema_version: "v1",
          server_version: "0.9.16",
          generated_at: "2026-02-10T12:00:00Z",
          health: { queue_depth: metricsCount, in_flight_jobs: 0, dns_queries_ipv4_total: 0, dns_queries_ipv6_total: 0 },
          jobs: { status_counts: { queued: 0, running: 0 } },
          api: { routes: [] },
          quality: { outcomes: { success_rate: 1, failed_rate: 0 }, job_duration_ms: { avg: 0 }, severity: { totals: {} } },
          insights: { domains: { items: [] }, batches: { items: [] } },
          trends: { windows: { "1h": { points: [] } } }
        });
      }
      return jsonResponse({});
    });

    const intervalCallbacks = [];
    vi.spyOn(global, "setInterval").mockImplementation((callback) => {
      intervalCallbacks.push(callback);
      return intervalCallbacks.length;
    });
    vi.spyOn(global, "clearInterval").mockImplementation(() => {});

    const { unmount } = render(App);
    await openMetricsTab();
    const metricsPanel = getMetricsPanel();
    expect(await within(metricsPanel).findByLabelText("DNS query rates trend")).toBeInTheDocument();
    expect(intervalCallbacks.length).toBeGreaterThan(0);
    const metricsCallsBeforePoll = metricsCalls.length;
    for (const poll of intervalCallbacks) {
      if (typeof poll === "function") {
        await poll();
      }
    }

    await waitFor(() => {
      expect(metricsCalls.length).toBeGreaterThan(metricsCallsBeforePoll);
    });

    unmount();
  });

  it("shows metrics error state and supports retry", async () => {
    let metricsCalls = 0;
    global.fetch.mockImplementation((url) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      if (value.includes("/api/v1/jobs?")) {
        return jsonResponse({ items: [], total: 0 });
      }
      if (value.startsWith("/api/v1/metrics?")) {
        metricsCalls += 1;
        if (metricsCalls === 1) {
          return jsonResponse({ error: { message: "metrics unavailable" } }, false);
        }
        return jsonResponse({
          schema_version: "v1",
          server_version: "0.9.16",
          generated_at: "2026-02-10T12:00:00Z",
          health: { queue_depth: 0, in_flight_jobs: 0, dns_queries_ipv4_total: 0, dns_queries_ipv6_total: 0 },
          jobs: { status_counts: { queued: 0, running: 0 } },
          api: { routes: [] },
          quality: { outcomes: { success_rate: 0, failed_rate: 0 }, job_duration_ms: { avg: 0 }, severity: { totals: {} } },
          insights: { domains: { items: [] }, batches: { items: [] } },
          trends: { windows: { "1h": { points: [] } } }
        });
      }
      return jsonResponse({});
    });

    const { unmount } = render(App);
    await openMetricsTab();
    const metricsPanel = getMetricsPanel();

    expect(await within(metricsPanel).findByRole("button", { name: "Retry" })).toBeInTheDocument();
    await fireEvent.click(within(metricsPanel).getByRole("button", { name: "Retry" }));

    await waitFor(() => {
      expect(within(metricsPanel).getByLabelText("DNS query rates trend")).toBeInTheDocument();
    });

    unmount();
  });

  it("warns when submitting a single job without a domain", async () => {
    global.fetch.mockImplementation(() => jsonResponse({ items: [] }));

    const { unmount } = render(App);

    const button = await screen.findByText("Run Single Job");
    await fireEvent.click(button);

    await waitFor(() => {
      expect(screen.getByText("Domain is required.")).toBeInTheDocument();
    });

    unmount();
  });

  it("clears status notice when switching tabs", async () => {
    global.fetch.mockImplementation(() => jsonResponse({ items: [] }));

    const { unmount } = render(App);

    const button = await screen.findByText("Run Single Job");
    await fireEvent.click(button);
    expect(screen.getByText("Domain is required.")).toBeInTheDocument();

    await openRecentTab();
    await waitFor(() => {
      expect(screen.queryByText("Domain is required.")).toBeNull();
    });

    unmount();
  });

  it("submits a single job and displays the created id", async () => {
    const job = {
      id: "job_1",
      domain: "example.com",
      status: "succeeded",
      created_at: "2026-02-03T00:00:00Z",
      started_at: "2026-02-03T00:00:00Z",
      finished_at: "2026-02-03T00:10:00Z",
      progress: 100
    };

    global.fetch.mockImplementation((url, options = {}) => {
      if (url === "/api/v1/jobs" && options.method === "POST") {
        return jsonResponse(job);
      }
      if (url === `/api/v1/jobs/${job.id}`) {
        return jsonResponse(job);
      }
      if (typeof url === "string" && url.startsWith("/api/v1/jobs?")) {
        return jsonResponse({ items: [job] });
      }
      return jsonResponse({ items: [] });
    });

    const { unmount } = render(App);

    const input = await screen.findByPlaceholderText("example.com");
    await fireEvent.input(input, { target: { value: "example.com" } });

    const button = screen.getByText("Run Single Job");
    await fireEvent.click(button);

    await waitFor(() => {
      expect(screen.getByText(/Created job:/)).toBeInTheDocument();
      expect(screen.getAllByText("job_1").length).toBeGreaterThan(0);
      expect(screen.getByText("succeeded · 10m 0s")).toBeInTheDocument();
    });

    expect(global.fetch).toHaveBeenCalledWith(
      "/api/v1/jobs",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ domain: "example.com" })
      })
    );

    unmount();
  });

  it("shows profile names in the recent job list and job inspector", async () => {
    const job = {
      id: "job_profile_name",
      domain: "example.com",
      status: "running",
      created_at: "2026-02-03T00:00:00Z",
      progress: 48,
      profile_name: "strict job profile"
    };

    global.fetch.mockImplementation((url) => {
      if (typeof url === "string" && url.startsWith("/api/v1/jobs?")) {
        return jsonResponse({ items: [job], total: 1 });
      }
      if (url === `/api/v1/jobs/${job.id}`) {
        return jsonResponse(job);
      }
      if (url === "/api/v1/profiles") {
        return jsonResponse(sampleProfiles());
      }
      return jsonResponse({ items: [] });
    });

    const { unmount } = render(App);

    await openRecentTab();
    expect(await screen.findByText("strict job profile")).toBeInTheDocument();

    const row = (await screen.findByText(job.id)).closest(".list-item");
    await fireEvent.click(row);

    await waitFor(() => {
      expect(screen.getByRole("tab", { name: "Single Job" })).toHaveAttribute("aria-selected", "true");
      expect(screen.getByText("strict job profile")).toBeInTheDocument();
    });

    unmount();
  });

  it("includes tags in single job payload when tag field is filled", async () => {
    const job = { id: "job_tagged", domain: "example.com", status: "pending" };
    let capturedBody = null;
    global.fetch.mockImplementation((url, options = {}) => {
      if (url === "/api/v1/jobs" && options.method === "POST") {
        capturedBody = JSON.parse(options.body || "{}");
        return jsonResponse(job);
      }
      if (url === `/api/v1/jobs/${job.id}`) return jsonResponse(job);
      return jsonResponse({ items: [] });
    });

    const { unmount } = render(App);

    await fireEvent.input(await screen.findByPlaceholderText("example.com"), { target: { value: "example.com" } });
    await fireEvent.input(screen.getByLabelText("Tags"), { target: { value: "tld, ccTLD" } });
    await fireEvent.click(screen.getByText("Run Single Job"));

    await waitFor(() => {
      expect(capturedBody?.tags).toEqual(["tld", "ccTLD"]);
    });
    unmount();
  });

  it("omits tags from single job payload when tag field is empty", async () => {
    const job = { id: "job_notag", domain: "example.com", status: "pending" };
    let capturedBody = null;
    global.fetch.mockImplementation((url, options = {}) => {
      if (url === "/api/v1/jobs" && options.method === "POST") {
        capturedBody = JSON.parse(options.body || "{}");
        return jsonResponse(job);
      }
      if (url === `/api/v1/jobs/${job.id}`) return jsonResponse(job);
      return jsonResponse({ items: [] });
    });

    const { unmount } = render(App);

    await fireEvent.input(await screen.findByPlaceholderText("example.com"), { target: { value: "example.com" } });
    await fireEvent.click(screen.getByText("Run Single Job"));

    await waitFor(() => {
      expect(capturedBody?.tags).toBeUndefined();
    });
    unmount();
  });

  it("converts IDN domains to punycode before submit", async () => {
    const idn = "r\u00e4ksm\u00f6rg\u00e5s.se";
    const puny = "xn--rksmrgs-5wao1o.se";
    const job = {
      id: "job_idn",
      domain: puny,
      status: "queued",
      created_at: "2026-02-03T00:00:00Z",
      progress: 0
    };

    global.fetch.mockImplementation((url, options = {}) => {
      if (url === "/api/v1/jobs" && options.method === "POST") {
        return jsonResponse(job);
      }
      if (url === `/api/v1/jobs/${job.id}`) {
        return jsonResponse(job);
      }
      if (typeof url === "string" && url.startsWith("/api/v1/jobs?")) {
        return jsonResponse({ items: [job] });
      }
      return jsonResponse({ items: [] });
    });

    const { unmount } = render(App);

    const input = await screen.findByPlaceholderText("example.com");
    await fireEvent.input(input, { target: { value: idn } });

    const button = screen.getByText("Run Single Job");
    await fireEvent.click(button);

    await waitFor(() => {
      const postCall = global.fetch.mock.calls.find(
        ([url, options]) => url === "/api/v1/jobs" && options?.method === "POST"
      );
      expect(postCall).toBeTruthy();
      const body = JSON.parse(postCall[1].body);
      expect(body.domain).toBe(puny);
    });

    unmount();
  });

  it("submits single job with IPv6-only profile override when IPv4 is disabled", async () => {
    const job = {
      id: "job_v6_only",
      domain: "example.com",
      status: "queued",
      created_at: "2026-02-03T00:00:00Z",
      progress: 0
    };

    global.fetch.mockImplementation((url, options = {}) => {
      if (url === "/api/v1/jobs" && options.method === "POST") {
        return jsonResponse(job);
      }
      if (url === `/api/v1/jobs/${job.id}`) {
        return jsonResponse(job);
      }
      if (typeof url === "string" && url.startsWith("/api/v1/jobs?")) {
        return jsonResponse({ items: [job] });
      }
      return jsonResponse({ items: [] });
    });

    const { unmount } = render(App);

    const input = await screen.findByPlaceholderText("example.com");
    await fireEvent.input(input, { target: { value: "example.com" } });
    await fireEvent.click(screen.getByText("Advanced profile"));
    await fireEvent.change(screen.getByLabelText("IP transport"), { target: { value: "disable_ipv4" } });

    await fireEvent.click(screen.getByText("Run Single Job"));

    await waitFor(() => {
      const postCall = global.fetch.mock.calls.find(
        ([url, options]) => url === "/api/v1/jobs" && options?.method === "POST"
      );
      expect(postCall).toBeTruthy();
      const body = JSON.parse(postCall[1].body);
      expect(body.domain).toBe("example.com");
      expect(body.profile_overrides).toEqual({
        net: {
          ipv4: false,
          ipv6: true
        }
      });
    });

    unmount();
  });

  it("submits single job with IPv4-only profile override when IPv6 is disabled", async () => {
    const job = {
      id: "job_v4_only",
      domain: "example.com",
      status: "queued",
      created_at: "2026-02-03T00:00:00Z",
      progress: 0
    };

    global.fetch.mockImplementation((url, options = {}) => {
      if (url === "/api/v1/jobs" && options.method === "POST") {
        return jsonResponse(job);
      }
      if (url === `/api/v1/jobs/${job.id}`) {
        return jsonResponse(job);
      }
      if (typeof url === "string" && url.startsWith("/api/v1/jobs?")) {
        return jsonResponse({ items: [job] });
      }
      return jsonResponse({ items: [] });
    });

    const { unmount } = render(App);

    const input = await screen.findByPlaceholderText("example.com");
    await fireEvent.input(input, { target: { value: "example.com" } });
    await fireEvent.click(screen.getByText("Advanced profile"));
    await fireEvent.change(screen.getByLabelText("IP transport"), { target: { value: "disable_ipv6" } });

    await fireEvent.click(screen.getByText("Run Single Job"));

    await waitFor(() => {
      const postCall = global.fetch.mock.calls.find(
        ([url, options]) => url === "/api/v1/jobs" && options?.method === "POST"
      );
      expect(postCall).toBeTruthy();
      const body = JSON.parse(postCall[1].body);
      expect(body.domain).toBe("example.com");
      expect(body.profile_overrides).toEqual({
        net: {
          ipv4: true,
          ipv6: false
        }
      });
    });

    unmount();
  });

  it("submits a single job with a selected stored profile and inline override", async () => {
    const job = {
      id: "job_profiled_single",
      domain: "example.com",
      status: "queued",
      created_at: "2026-02-03T00:00:00Z",
      progress: 0
    };

    global.fetch.mockImplementation((url, options = {}) => {
      if (url === "/api/v1/profiles") {
        return jsonResponse(sampleProfiles());
      }
      if (url === "/api/v1/jobs" && options.method === "POST") {
        return jsonResponse(job);
      }
      if (url === `/api/v1/jobs/${job.id}`) {
        return jsonResponse(job);
      }
      if (typeof url === "string" && url.startsWith("/api/v1/jobs?")) {
        return jsonResponse({ items: [job] });
      }
      return jsonResponse({ items: [] });
    });

    const { unmount } = render(App);

    await fireEvent.input(await screen.findByPlaceholderText("example.com"), {
      target: { value: "example.com" }
    });
    await screen.findByRole("option", { name: "strict" });
    await fireEvent.change(screen.getByLabelText("Stored profile"), {
      target: { value: "12" }
    });
    await fireEvent.click(screen.getByText("Advanced profile"));
    await fireEvent.change(screen.getByLabelText("IP transport"), {
      target: { value: "disable_ipv4" }
    });

    await fireEvent.click(screen.getByText("Run Single Job"));

    await waitFor(() => {
      const postCall = global.fetch.mock.calls.find(
        ([url, options]) => url === "/api/v1/jobs" && options?.method === "POST"
      );
      expect(postCall).toBeTruthy();
      const body = JSON.parse(postCall[1].body);
      expect(body).toEqual({
        domain: "example.com",
        profile_id: 12,
        profile_overrides: {
          net: {
            ipv4: false,
            ipv6: true
          }
        }
      });
    });

    unmount();
  });

  it("includes undelegated nameservers and ds_info in single-job payload when configured", async () => {
    const job = {
      id: "job_undelegated",
      domain: "example.com",
      status: "queued",
      created_at: "2026-02-03T00:00:00Z",
      progress: 0
    };

    global.fetch.mockImplementation((url, options = {}) => {
      if (url === "/api/v1/jobs" && options.method === "POST") {
        return jsonResponse(job);
      }
      if (url === `/api/v1/jobs/${job.id}`) {
        return jsonResponse(job);
      }
      if (typeof url === "string" && url.startsWith("/api/v1/jobs?")) {
        return jsonResponse({ items: [job] });
      }
      return jsonResponse({ items: [] });
    });

    const { unmount } = render(App);

    await fireEvent.input(await screen.findByPlaceholderText("example.com"), {
      target: { value: "example.com" }
    });
    await fireEvent.click(screen.getByText("Undelegated / Pre-delegation"));
    await fireEvent.click(screen.getByRole("button", { name: "Add nameserver" }));
    await fireEvent.input(screen.getByLabelText("Undelegated NS 1"), {
      target: { value: "ns1.example.com" }
    });
    await fireEvent.input(screen.getByLabelText("Undelegated NS IP 1"), {
      target: { value: "2001:db8::10" }
    });
    await fireEvent.click(screen.getByRole("button", { name: "Add DS record" }));
    await fireEvent.input(screen.getByLabelText("Undelegated DS keytag 1"), {
      target: { value: "12345" }
    });
    await fireEvent.input(screen.getByLabelText("Undelegated DS algorithm 1"), {
      target: { value: "13" }
    });
    await fireEvent.input(screen.getByLabelText("Undelegated DS digest type 1"), {
      target: { value: "2" }
    });
    await fireEvent.input(screen.getByLabelText("Undelegated DS digest 1"), {
      target: { value: "aabbccdd" }
    });

    await fireEvent.click(screen.getByText("Run Single Job"));

    await waitFor(() => {
      const postCall = global.fetch.mock.calls.find(
        ([url, options]) => url === "/api/v1/jobs" && options?.method === "POST"
      );
      expect(postCall).toBeTruthy();
      const body = JSON.parse(postCall[1].body);
      expect(body).toEqual({
        domain: "example.com",
        nameservers: [{ ns: "ns1.example.com", ip: "2001:db8::10" }],
        ds_info: [{ keytag: 12345, algorithm: 13, digtype: 2, digest: "AABBCCDD" }]
      });
    });

    unmount();
  });

  it("blocks single-job submit when undelegated rows are invalid", async () => {
    global.fetch.mockImplementation((url) => {
      if (typeof url === "string" && url.startsWith("/api/v1/jobs?")) {
        return jsonResponse({ items: [] });
      }
      return jsonResponse({ items: [] });
    });

    const { unmount } = render(App);

    await fireEvent.input(await screen.findByPlaceholderText("example.com"), {
      target: { value: "example.com" }
    });
    await fireEvent.click(screen.getByText("Undelegated / Pre-delegation"));
    await fireEvent.click(screen.getByRole("button", { name: "Add nameserver" }));
    await fireEvent.input(screen.getByLabelText("Undelegated NS 1"), {
      target: { value: "ns1.example.com" }
    });
    await fireEvent.input(screen.getByLabelText("Undelegated NS IP 1"), {
      target: { value: "not-an-ip" }
    });

    await fireEvent.click(screen.getByText("Run Single Job"));

    await waitFor(() => {
      expect(
        screen.getByText("Undelegated nameserver row 1: IP must be a valid IPv4 or IPv6 address.")
      ).toBeInTheDocument();
    });

    const postCalls = global.fetch.mock.calls.filter(
      ([url, options]) => url === "/api/v1/jobs" && options?.method === "POST"
    );
    expect(postCalls).toHaveLength(0);

    unmount();
  });

  it("submits a batch job and displays the created batch id", async () => {
    const batch = {
      batch_id: "batch_1",
      job_ids: ["job_1"]
    };
    const summary = {
      batch_id: "batch_1",
      total: 1,
      status_counts: { queued: 1 },
      items: [],
      created_at: "2026-02-03T00:00:00Z",
      finished_at: "2026-02-03T00:10:00Z"
    };

    global.fetch.mockImplementation((url, options = {}) => {
      if (url === "/api/v1/jobs/batch" && options.method === "POST") {
        return jsonResponse(batch);
      }
      if (typeof url === "string" && url.startsWith("/api/v1/batches/batch_1")) {
        return jsonResponse(summary);
      }
      if (typeof url === "string" && url.startsWith("/api/v1/jobs?")) {
        return jsonResponse({ items: [] });
      }
      return jsonResponse({});
    });

    const { unmount } = render(App);

    await openBatchTab();
    const textarea = await screen.findByLabelText("Domains (one per line)");
    await fireEvent.input(textarea, { target: { value: "example.com\nexample.org" } });

    const button = screen.getByText("Run Batch");
    await fireEvent.click(button);

    await waitFor(() => {
      expect(screen.getByText(/Created batch:/)).toBeInTheDocument();
      expect(screen.getByText("batch_1")).toBeInTheDocument();
      expect(screen.getByText("Total runtime")).toBeInTheDocument();
      expect(screen.getByText("10m 0s")).toBeInTheDocument();
      expect(screen.getByText("queued 1")).toBeInTheDocument();
    });

    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalledWith(
        "/api/v1/batches/batch_1?limit=20&sort=started_at_desc",
        expect.objectContaining({
          headers: {}
        })
      );
    });

    expect(global.fetch).toHaveBeenCalledWith(
      "/api/v1/jobs/batch",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ domains: ["example.com", "example.org"] })
      })
    );

    unmount();
  });

  it("keeps batch payload domains-only even when undelegated single-job fields are filled", async () => {
    const batch = {
      batch_id: "batch_domains_only",
      job_ids: ["job_1"]
    };
    const summary = {
      batch_id: "batch_domains_only",
      total: 1,
      status_counts: { queued: 1 },
      items: [],
      created_at: "2026-02-03T00:00:00Z"
    };

    global.fetch.mockImplementation((url, options = {}) => {
      if (url === "/api/v1/jobs/batch" && options.method === "POST") {
        return jsonResponse(batch);
      }
      if (typeof url === "string" && url.startsWith("/api/v1/batches/batch_domains_only")) {
        return jsonResponse(summary);
      }
      if (typeof url === "string" && url.startsWith("/api/v1/jobs?")) {
        return jsonResponse({ items: [] });
      }
      return jsonResponse({});
    });

    const { unmount } = render(App);

    await fireEvent.click(screen.getByText("Undelegated / Pre-delegation"));
    await fireEvent.click(screen.getByRole("button", { name: "Add nameserver" }));
    await fireEvent.input(screen.getByLabelText("Undelegated NS 1"), {
      target: { value: "ns1.example.com" }
    });
    await fireEvent.input(screen.getByLabelText("Undelegated NS IP 1"), {
      target: { value: "192.0.2.10" }
    });

    await openBatchTab();
    await fireEvent.input(await screen.findByLabelText("Domains (one per line)"), {
      target: { value: "example.com\nexample.org" }
    });
    await fireEvent.click(screen.getByText("Run Batch"));

    await waitFor(() => {
      const postCall = global.fetch.mock.calls.find(
        ([url, options]) => url === "/api/v1/jobs/batch" && options?.method === "POST"
      );
      expect(postCall).toBeTruthy();
      const body = JSON.parse(postCall[1].body);
      expect(body).toEqual({ domains: ["example.com", "example.org"] });
      expect(body.nameservers).toBeUndefined();
      expect(body.ds_info).toBeUndefined();
    });

    unmount();
  });

  it("includes tags in batch payload when tag field is filled", async () => {
    let capturedBody = null;
    global.fetch.mockImplementation((url, options = {}) => {
      if (url === "/api/v1/profiles") return jsonResponse(sampleProfiles());
      if (url === "/api/v1/jobs/batch" && options.method === "POST") {
        capturedBody = JSON.parse(options.body || "{}");
        return jsonResponse({ batch_id: "batch_t" });
      }
      if (typeof url === "string" && url.startsWith("/api/v1/batches/")) return jsonResponse({ batch_id: "batch_t", total: 0, status_counts: {}, items: [], created_at: "2026-03-24T00:00:00Z" });
      if (typeof url === "string" && url.startsWith("/api/v1/jobs?")) return jsonResponse({ items: [] });
      if (url.includes("/api/v1/tags")) return jsonResponse([]);
      return jsonResponse({});
    });

    const { unmount } = render(App);
    await openBatchTab();

    await fireEvent.input(await screen.findByLabelText("Domains (one per line)"), { target: { value: "example.com" } });
    await fireEvent.input(screen.getByLabelText("Tags"), { target: { value: "tld, ccTLD" } });
    await fireEvent.click(screen.getByText("Run Batch"));

    await waitFor(() => {
      expect(capturedBody?.tags).toEqual(["tld", "ccTLD"]);
      expect(capturedBody?.domains).toEqual(["example.com"]);
    });
    unmount();
  });

  it("includes the selected stored profile in batch payload", async () => {
    let capturedBody = null;
    global.fetch.mockImplementation((url, options = {}) => {
      if (url === "/api/v1/profiles") return jsonResponse(sampleProfiles());
      if (url === "/api/v1/jobs/batch" && options.method === "POST") {
        capturedBody = JSON.parse(options.body || "{}");
        return jsonResponse({ batch_id: "batch_profiled" });
      }
      if (typeof url === "string" && url.startsWith("/api/v1/batches/batch_profiled")) {
        return jsonResponse({ batch_id: "batch_profiled", total: 0, status_counts: {}, items: [], created_at: "2026-03-24T00:00:00Z" });
      }
      if (typeof url === "string" && url.startsWith("/api/v1/jobs?")) return jsonResponse({ items: [] });
      if (url.includes("/api/v1/tags")) return jsonResponse([]);
      return jsonResponse({});
    });

    const { unmount } = render(App);
    await openBatchTab();

    await fireEvent.input(await screen.findByLabelText("Domains (one per line)"), {
      target: { value: "example.com" }
    });
    await fireEvent.change(screen.getByLabelText("Stored profile"), {
      target: { value: "11" }
    });
    await fireEvent.click(screen.getByText("Run Batch"));

    await waitFor(() => {
      expect(capturedBody).toEqual({
        domains: ["example.com"],
        profile_id: 11
      });
    });
    unmount();
  });

  it("refreshes admin profile selectors after creating a non-public profile in settings", async () => {
    let profiles = [
      {
        id: 12,
        name: "strict",
        description: "Strict DNS profile",
        config: { resolver: { defaults: { timeout: 5 } } },
        public: true,
        created_at: "2026-04-01T11:00:00Z",
        updated_at: "2026-04-03T09:30:00Z"
      }
    ];
    let tags = [{ name: "ops", description: "Operations", domain_count: 1, default_profile_id: null }];

    global.fetch.mockImplementation((url, options = {}) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      if (typeof value === "string" && value.startsWith("/api/v1/jobs?")) {
        return jsonResponse({ items: [], total: 0 });
      }
      if (value === "/api/v1/profiles/default") {
        return jsonResponse({
          id: 0,
          name: "default",
          description: "Server base profile",
          config: { net: { ipv4: true, ipv6: true } },
          public: false,
          created_at: "0001-01-01T00:00:00Z",
          updated_at: "0001-01-01T00:00:00Z"
        });
      }
      if (value === "/api/v1/profiles" && (!options.method || options.method === "GET")) {
        return jsonResponse(profiles);
      }
      if (value === "/api/v1/profiles" && options.method === "POST") {
        const body = JSON.parse(options.body || "{}");
        const created = {
          id: 13,
          ...body,
          created_at: "2026-04-05T10:00:00Z",
          updated_at: "2026-04-05T10:00:00Z"
        };
        profiles = [...profiles, created];
        return jsonResponse(created);
      }
      if (value.includes("/api/v1/tags/ops/summary")) {
        return jsonResponse({ tag: "ops", domain_count: 1, ok: 0, notice: 0, warning: 0, error: 0, critical: 0 });
      }
      if (value.includes("/api/v1/tags/ops/domains")) {
        return jsonResponse({ items: [], total: 0 });
      }
      if (value === "/api/v1/tags?limit=500" || value === "/api/v1/tags") {
        return jsonResponse(tags);
      }
      return jsonResponse({});
    });

    const { unmount } = render(App);

    await openSettingsTab();
    await fireEvent.click(await screen.findByRole("button", { name: "New profile" }));
    await fireEvent.input(screen.getByLabelText("Name"), { target: { value: "internal-only" } });
    await fireEvent.input(screen.getByLabelText("Description"), { target: { value: "Admin profile" } });
    await fireEvent.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByText("Profile created.")).toBeInTheDocument();

    await fireEvent.click(screen.getByRole("tab", { name: "Single Job" }));
    const singleSelect = await screen.findByLabelText("Stored profile");
    await waitFor(() => {
      expect(within(singleSelect).getByRole("option", { name: "internal-only" })).toBeInTheDocument();
    });

    await openBatchTab();
    const batchSelect = await screen.findByLabelText("Stored profile");
    await waitFor(() => {
      expect(within(batchSelect).getByRole("option", { name: "internal-only" })).toBeInTheDocument();
    });

    await fireEvent.click(screen.getByRole("tab", { name: "Tags" }));
    await fireEvent.click(await screen.findByText("ops"));
    const tagSelect = await screen.findByLabelText("Profile for this tag");
    await waitFor(() => {
      expect(within(tagSelect).getByRole("option", { name: "internal-only" })).toBeInTheDocument();
    });

    unmount();
  });

  it("from-tag mode sends from_tag in batch payload", async () => {
    let capturedBody = null;
    global.fetch.mockImplementation((url, options = {}) => {
      if (url === "/api/v1/profiles") return jsonResponse(sampleProfiles());
      if (url === "/api/v1/jobs/batch" && options.method === "POST") {
        capturedBody = JSON.parse(options.body || "{}");
        return jsonResponse({ batch_id: "batch_ft" });
      }
      if (typeof url === "string" && url.startsWith("/api/v1/batches/")) return jsonResponse({ batch_id: "batch_ft", total: 0, status_counts: {}, items: [], created_at: "2026-03-24T00:00:00Z" });
      if (typeof url === "string" && url.startsWith("/api/v1/jobs?")) return jsonResponse({ items: [] });
      if (url.includes("/api/v1/tags")) return jsonResponse([{ name: "tld", description: "", domain_count: 3 }]);
      return jsonResponse({});
    });

    const { unmount } = render(App);
    await openBatchTab();

    await fireEvent.click(await screen.findByText("From tag"));
    const tagSelect = await screen.findByLabelText("Run all domains in tag");
    await fireEvent.change(tagSelect, { target: { value: "tld" } });
    await fireEvent.click(screen.getByText("Run Batch"));

    await waitFor(() => {
      expect(capturedBody?.from_tag).toBe("tld");
      expect(capturedBody?.domains).toBeUndefined();
    });
    unmount();
  });

  it("batch inspector shows tag when batch has a tag", async () => {
    global.fetch.mockImplementation((url, options = {}) => {
      if (typeof url === "string" && url.startsWith("/api/v1/batches/batch_tagged")) {
        return jsonResponse({ id: "batch_tagged", tag: "tld", total: 1, status_counts: { succeeded: 1 }, items: [], created_at: "2026-03-24T00:00:00Z" });
      }
      if (typeof url === "string" && url.startsWith("/api/v1/jobs?")) return jsonResponse({ items: [{ id: "j1", batch_id: "batch_tagged", created_at: "2026-03-24T00:00:00Z" }] });
      if (url.includes("/api/v1/tags")) return jsonResponse([]);
      return jsonResponse({});
    });

    const { unmount } = render(App);
    await openBatchTab();

    await fireEvent.input(await screen.findByLabelText("Batch ID"), { target: { value: "batch_tagged" } });
    await fireEvent.change(screen.getByLabelText("Batch ID"), {});

    await waitFor(() => {
      expect(screen.getByText("Tag")).toBeInTheDocument();
      expect(screen.getByText("tld")).toBeInTheDocument();
    });
    unmount();
  });

  it("recent batch dropdown label includes tag when batch is loaded", async () => {
    global.fetch.mockImplementation((url, options = {}) => {
      if (typeof url === "string" && url.startsWith("/api/v1/batches/batch_wtag")) {
        return jsonResponse({ id: "batch_wtag", tag: "ccTLD", total: 1, status_counts: {}, items: [], created_at: "2026-03-24T00:00:00Z" });
      }
      if (typeof url === "string" && url.startsWith("/api/v1/jobs?")) {
        return jsonResponse({ items: [{ id: "j1", batch_id: "batch_wtag", created_at: "2026-03-24T00:00:00Z" }] });
      }
      if (url.includes("/api/v1/tags")) return jsonResponse([]);
      return jsonResponse({});
    });

    const { unmount } = render(App);
    await openBatchTab();

    // Load the batch to populate the tag
    const batchInput = await screen.findByLabelText("Batch ID");
    await fireEvent.input(batchInput, { target: { value: "batch_wtag" } });
    await fireEvent.change(batchInput, {});

    await waitFor(() => {
      const options = screen.getAllByRole("option");
      const batchOption = options.find((o) => o.value === "batch_wtag");
      expect(batchOption?.textContent).toContain("[ccTLD]");
    });
    unmount();
  });

  it("shows active batches card with empty state when no batches are active", async () => {
    global.fetch.mockImplementation((url) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      if (value.includes("/api/v1/jobs?")) return jsonResponse({ items: [] });
      if (value.includes("/api/v1/tags")) return jsonResponse([]);
      return jsonResponse({});
    });

    const { unmount } = render(App);
    await openBatchTab();

    await waitFor(() => {
      expect(screen.getByTestId("active-batches-card")).toBeInTheDocument();
      expect(screen.getByText("Active Batches")).toBeInTheDocument();
      expect(screen.getByText("No active batches.")).toBeInTheDocument();
    });
    unmount();
  });

  it("shows active batches with running jobs", async () => {
    const jobsPage = {
      items: [
        { id: "job_1", batch_id: "batch_active", created_at: "2026-03-01T10:00:00Z" },
        { id: "job_2", batch_id: "batch_done", created_at: "2026-03-01T09:00:00Z" }
      ],
      total: 2
    };
    const batchActive = {
      batch_id: "batch_active",
      tag: "se-domains",
      total: 5,
      status_counts: { running: 2, queued: 1, succeeded: 2 },
      items: [{ id: "job_1", domain: "example.se", status: "running", progress: 40 }],
      created_at: "2026-03-01T10:00:00Z"
    };
    const batchDone = {
      batch_id: "batch_done",
      total: 3,
      status_counts: { succeeded: 3 },
      items: [],
      created_at: "2026-03-01T09:00:00Z"
    };

    global.fetch.mockImplementation((url) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      if (value.includes("/api/v1/batches/batch_active")) return jsonResponse(batchActive);
      if (value.includes("/api/v1/batches/batch_done")) return jsonResponse(batchDone);
      if (value.includes("/api/v1/jobs?")) return jsonResponse(jobsPage);
      if (value.includes("/api/v1/tags")) return jsonResponse([]);
      return jsonResponse({});
    });

    const { unmount } = render(App);
    await openBatchTab();

    const card = await waitFor(() => screen.getByTestId("active-batches-card"));
    await waitFor(() => {
      expect(within(card).getByText(/batch_active/)).toBeInTheDocument();
      expect(within(card).getByText("(se-domains)")).toBeInTheDocument();
    });

    // batch_done should NOT appear in the active batches card since it has no active jobs
    expect(within(card).queryByText(/batch_done/)).not.toBeInTheDocument();

    unmount();
  });

  it("clicking an active batch selects it in the batch inspector", async () => {
    const jobsPage = {
      items: [
        { id: "job_1", batch_id: "batch_click", created_at: "2026-03-01T10:00:00Z" }
      ],
      total: 1
    };
    const batchClick = {
      batch_id: "batch_click",
      total: 2,
      status_counts: { running: 1, queued: 1 },
      items: [{ id: "job_1", domain: "click.example", status: "running", progress: 50 }],
      created_at: "2026-03-01T10:00:00Z"
    };

    global.fetch.mockImplementation((url) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      if (value.includes("/api/v1/batches/batch_click")) return jsonResponse(batchClick);
      if (value.includes("/api/v1/jobs?")) return jsonResponse(jobsPage);
      if (value.includes("/api/v1/tags")) return jsonResponse([]);
      return jsonResponse({});
    });

    const { unmount } = render(App);
    await openBatchTab();

    // Wait for the active batch to appear
    const batchRow = await waitFor(() => {
      const card = screen.getByTestId("active-batches-card");
      return within(card).getByText(/batch_click/);
    });

    // Click the row
    await fireEvent.click(batchRow.closest("[role='button']"));

    // The batch inspector should now show the batch job details
    await waitFor(() => {
      expect(screen.getByText("click.example - running")).toBeInTheDocument();
    });

    unmount();
  });

  it("shows pause queue button and sends POST to pause endpoint", async () => {
    const calls = [];
    global.fetch.mockImplementation((url, options = {}) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      calls.push({ url: value, method: options?.method || "GET" });
      if (value.includes("/api/v1/metrics")) {
        return jsonResponse({ health: { queue_paused: false } });
      }
      if (value.includes("/api/v1/queue/pause")) return jsonResponse({});
      if (value.includes("/api/v1/jobs?")) return jsonResponse({ items: [] });
      if (value.includes("/api/v1/tags")) return jsonResponse([]);
      return jsonResponse({});
    });

    const { unmount } = render(App);
    await openBatchTab();

    const pauseBtn = await waitFor(() => screen.getByRole("button", { name: /pause queue/i }));
    expect(pauseBtn).toBeInTheDocument();

    await fireEvent.click(pauseBtn);

    await waitFor(() => {
      expect(calls.some((c) => c.url.includes("/api/v1/queue/pause") && c.method === "POST")).toBe(true);
    });

    unmount();
  });

  it("shows resume button and paused banner when queue is paused", async () => {
    global.fetch.mockImplementation((url, options = {}) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      if (value.includes("/api/v1/metrics")) {
        return jsonResponse({ health: { queue_paused: true } });
      }
      if (value.includes("/api/v1/queue/resume")) return jsonResponse({});
      if (value.includes("/api/v1/jobs?")) return jsonResponse({ items: [] });
      if (value.includes("/api/v1/tags")) return jsonResponse([]);
      return jsonResponse({});
    });

    const { unmount } = render(App);
    await openBatchTab();

    await waitFor(() => {
      expect(screen.getByText(/queue is paused/i)).toBeInTheDocument();
      expect(screen.getByRole("button", { name: /resume queue/i })).toBeInTheDocument();
    });

    unmount();
  });

  it("clicking resume sends POST to resume endpoint and hides banner", async () => {
    const calls = [];
    global.fetch.mockImplementation((url, options = {}) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      calls.push({ url: value, method: options?.method || "GET" });
      if (value.includes("/api/v1/metrics")) {
        return jsonResponse({ health: { queue_paused: true } });
      }
      if (value.includes("/api/v1/queue/resume")) return jsonResponse({});
      if (value.includes("/api/v1/jobs?")) return jsonResponse({ items: [] });
      if (value.includes("/api/v1/tags")) return jsonResponse([]);
      return jsonResponse({});
    });

    const { unmount } = render(App);
    await openBatchTab();

    const resumeBtn = await waitFor(() => screen.getByRole("button", { name: /resume queue/i }));
    await fireEvent.click(resumeBtn);

    await waitFor(() => {
      expect(calls.some((c) => c.url.includes("/api/v1/queue/resume") && c.method === "POST")).toBe(true);
    });

    // After resume, banner should disappear
    await waitFor(() => {
      expect(screen.queryByText(/queue is paused/i)).not.toBeInTheDocument();
    });

    unmount();
  });

  it("pause button has tooltip explaining what it does", async () => {
    global.fetch.mockImplementation((url) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      if (value.includes("/api/v1/metrics")) return jsonResponse({ health: { queue_paused: false } });
      if (value.includes("/api/v1/jobs?")) return jsonResponse({ items: [] });
      if (value.includes("/api/v1/tags")) return jsonResponse([]);
      return jsonResponse({});
    });

    const { unmount } = render(App);
    await openBatchTab();

    const pauseBtn = await waitFor(() => screen.getByRole("button", { name: /pause queue/i }));
    expect(pauseBtn.getAttribute("title")).toMatch(/pausing the queue stops workers/i);
    unmount();
  });

  it("active batch rows have keyboard accessible attributes", async () => {
    const jobsPage = {
      items: [{ id: "job_1", batch_id: "batch_kb", created_at: "2026-03-01T10:00:00Z" }],
      total: 1
    };
    const batchKb = {
      batch_id: "batch_kb",
      total: 2,
      status_counts: { running: 1, queued: 1 },
      items: [{ id: "job_1", domain: "kb.example", status: "running", progress: 20 }],
      created_at: "2026-03-01T10:00:00Z"
    };

    global.fetch.mockImplementation((url) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      if (value.includes("/api/v1/batches/batch_kb")) return jsonResponse(batchKb);
      if (value.includes("/api/v1/jobs?")) return jsonResponse(jobsPage);
      if (value.includes("/api/v1/tags")) return jsonResponse([]);
      return jsonResponse({});
    });

    const { unmount } = render(App);
    await openBatchTab();

    const card = await waitFor(() => screen.getByTestId("active-batches-card"));
    const batchText = await waitFor(() => within(card).getByText(/batch_kb/));
    const row = batchText.closest("[role='button']");
    expect(row).not.toBeNull();
    expect(row.getAttribute("tabindex")).toBe("0");
    unmount();
  });

  it("shows error toast when pause toggle fails", async () => {
    global.fetch.mockImplementation((url, options = {}) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      if (value.includes("/api/v1/queue/pause")) {
        return jsonResponse({ error: { message: "server error" } }, false);
      }
      if (value.includes("/api/v1/metrics")) return jsonResponse({ health: { queue_paused: false } });
      if (value.includes("/api/v1/jobs?")) return jsonResponse({ items: [] });
      if (value.includes("/api/v1/tags")) return jsonResponse([]);
      return jsonResponse({});
    });

    const { unmount } = render(App);
    await openBatchTab();

    const pauseBtn = await waitFor(() => screen.getByRole("button", { name: /pause queue/i }));
    await fireEvent.click(pauseBtn);

    await waitFor(() => {
      expect(screen.getByText(/failed to toggle queue/i)).toBeInTheDocument();
    });

    unmount();
  });

  it("applies batch filters and pagination query params", async () => {
    const calls = [];
    const firstPage = {
      batch_id: "batch_1",
      total: 2,
      status_counts: { failed: 2 },
      items: [
        {
          id: "job_b1",
          domain: "beta.example",
          status: "failed",
          created_at: "2026-02-03T00:00:00Z",
          progress: 100
        }
      ],
      created_at: "2026-02-03T00:00:00Z",
      offset: 0,
      next_cursor: "1",
      prev_cursor: "",
      sort: "started_at_desc"
    };
    const secondPage = {
      ...firstPage,
      items: [
        {
          id: "job_b2",
          domain: "beta-2.example",
          status: "failed",
          created_at: "2026-02-03T00:00:01Z",
          progress: 100
        }
      ],
      offset: 1,
      next_cursor: "",
      prev_cursor: "0"
    };

    global.fetch.mockImplementation((url) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      calls.push(value);
      if (value.includes("/api/v1/jobs?")) {
        return jsonResponse({ items: [] });
      }
      if (value.includes("/api/v1/batches/batch_1")) {
        if (value.includes("cursor=1")) {
          return jsonResponse(secondPage);
        }
        return jsonResponse(firstPage);
      }
      return jsonResponse({});
    });

    const { unmount } = render(App);

    await openBatchTab();
    const batchId = screen.getByLabelText("Batch ID");
    await fireEvent.input(batchId, { target: { value: "batch_1" } });
    await fireEvent.change(batchId);

    await waitFor(() => {
      expect(calls.some((value) => value.includes("limit=20") && value.includes("sort=started_at_desc"))).toBe(true);
    });

    await fireEvent.change(screen.getByLabelText("Status"), { target: { value: "failed" } });
    await waitFor(() => {
      expect(calls.some((value) => value.includes("status=failed"))).toBe(true);
    });

    await fireEvent.input(screen.getByLabelText("Domain contains"), { target: { value: "beta" } });
    await fireEvent.click(screen.getByRole("button", { name: "Apply filters" }));
    await waitFor(() => {
      expect(calls.some((value) => value.includes("status=failed") && value.includes("domain=beta"))).toBe(true);
    });

    await fireEvent.click(screen.getByRole("button", { name: "Next" }));
    await waitFor(() => {
      expect(calls.some((value) => value.includes("cursor=1"))).toBe(true);
    });

    unmount();
  });

  it("persists view filters and sorts in URL query params", async () => {
    global.fetch.mockImplementation((url) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      if (value.includes("/api/v1/jobs?")) {
        return jsonResponse({ items: [], total: 0 });
      }
      if (value.includes("/api/v1/batches/")) {
        return jsonResponse({
          batch_id: "batch_1",
          total: 0,
          status_counts: {},
          items: [],
          created_at: "2026-02-03T00:00:00Z"
        });
      }
      return jsonResponse({});
    });

    const { unmount } = render(App);

    await openRecentTab();
    await fireEvent.change(screen.getByLabelText("Sort"), { target: { value: "domain_desc" } });
    await fireEvent.click(screen.getByRole("button", { name: "Warnings+" }));
    await fireEvent.input(screen.getByLabelText("Batch ID filter"), { target: { value: "batch_recent" } });
    await fireEvent.click(screen.getByRole("button", { name: "Apply filters" }));

    await openBatchTab();
    await fireEvent.input(screen.getByLabelText("Batch ID"), { target: { value: "batch_1" } });
    await fireEvent.change(screen.getByLabelText("Batch ID"));
    await fireEvent.change(screen.getByLabelText("Status"), { target: { value: "failed" } });
    await fireEvent.input(screen.getByLabelText("Domain contains"), { target: { value: "beta" } });
    await fireEvent.click(screen.getByRole("button", { name: "Apply filters" }));

    await waitFor(() => {
      const params = new URLSearchParams(window.location.search);
      expect(params.get("r_sort")).toBe("domain_desc");
      expect(params.get("r_sev")).toBe("warnings_plus");
      expect(params.get("r_batch")).toBe("batch_recent");
      expect(params.get("b_id")).toBe("batch_1");
      expect(params.get("b_status")).toBe("failed");
      expect(params.get("b_domain")).toBe("beta");
      expect(window.location.hash).toBe("#/batches");
    });

    unmount();
  });

  it("restores view state from URL params with precedence over localStorage", async () => {
    localStorage.setItem(
      "gonemaster.ui.state.v1",
      JSON.stringify({
        jobSort: "domain_asc",
        severityFilter: "all",
        jobBatchFilter: "batch_local",
        recentPageSize: 20,
        recentCursor: 0,
        selectedBatchId: "batch_local_1",
        batchSort: "started_at_desc",
        batchPageSize: 20,
        batchCursor: 0,
        batchStatusFilter: "",
        batchDomainFilter: ""
      })
    );
    window.history.replaceState(
      null,
      "",
      "/?r_sort=domain_desc&r_sev=errors_only&r_batch=batch_url&r_limit=50&r_cursor=2&b_id=batch_url_1&b_sort=created_at_asc&b_limit=50&b_cursor=2&b_status=failed&b_domain=beta#/batches"
    );

    const calls = [];
    global.fetch.mockImplementation((url) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      calls.push(value);
      if (value.includes("/api/v1/jobs?")) {
        return jsonResponse({ items: [], total: 0 });
      }
      if (value.includes("/api/v1/batches/batch_url_1")) {
        return jsonResponse({
          batch_id: "batch_url_1",
          total: 0,
          status_counts: {},
          items: [],
          created_at: "2026-02-03T00:00:00Z"
        });
      }
      return jsonResponse({});
    });

    const { unmount } = render(App);

    await waitFor(() => {
      expect(calls.some((value) => value.includes("/api/v1/jobs?") && value.includes("sort=domain_desc") && value.includes("batch_id=batch_url"))).toBe(true);
      expect(calls.some((value) => value.includes("/api/v1/jobs?") && value.includes("limit=50") && value.includes("cursor=2"))).toBe(true);
      expect(
        calls.some(
          (value) =>
            value.includes("/api/v1/batches/batch_url_1?") &&
            value.includes("sort=created_at_asc") &&
            value.includes("limit=50") &&
            value.includes("cursor=2") &&
            value.includes("status=failed") &&
            value.includes("domain=beta")
        )
      ).toBe(true);
    });

    expect(screen.getByLabelText("Batch ID")).toHaveValue("batch_url_1");
    expect(screen.getByLabelText("Sort")).toHaveValue("created_at_asc");
    expect(screen.getByLabelText("Page size")).toHaveValue("50");
    expect(screen.getByLabelText("Status")).toHaveValue("failed");
    expect(screen.getByLabelText("Domain contains")).toHaveValue("beta");

    await openRecentTab();
    expect(screen.getByLabelText("Sort")).toHaveValue("domain_desc");
    expect(screen.getByLabelText("Page size")).toHaveValue("50");
    expect(screen.getByLabelText("Batch ID filter")).toHaveValue("batch_url");

    unmount();
  });

  it("uses localStorage view state when URL has no persisted params", async () => {
    localStorage.setItem(
      "gonemaster.ui.state.v1",
      JSON.stringify({
        jobSort: "domain_asc",
        severityFilter: "errors_only",
        jobBatchFilter: "batch_storage",
        recentPageSize: 50,
        recentCursor: 2
      })
    );
    const calls = [];

    global.fetch.mockImplementation((url) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      calls.push(value);
      if (value.includes("/api/v1/jobs?")) {
        return jsonResponse({ items: [], total: 0 });
      }
      return jsonResponse({});
    });

    const { unmount } = render(App);

    await waitFor(() => {
      expect(
        calls.some(
          (value) =>
            value.includes("sort=domain_asc") &&
            value.includes("batch_id=batch_storage") &&
            value.includes("limit=50") &&
            value.includes("cursor=2")
        )
      ).toBe(true);
    });

    unmount();
  });

  it("renders progress bars for job inspector, recent jobs, and batch jobs", async () => {
    const job = {
      id: "job_a",
      domain: "example.com",
      status: "running",
      created_at: "2026-02-03T00:00:00Z",
      progress: 42,
      severity_totals: {
        NOTICE: 2,
        WARNING: 1,
        ERROR: 3,
        CRITICAL: 0
      }
    };
    const batch = {
      batch_id: "batch_progress",
      total: 1,
      status_counts: { running: 1 },
      items: [
        {
          id: "job_batch_progress",
          domain: "batch.example",
          status: "running",
          created_at: "2026-02-03T00:00:00Z",
          progress: 73
        }
      ],
      created_at: "2026-02-03T00:00:00Z"
    };

    global.fetch.mockImplementation((url) => {
      if (typeof url === "string" && url.startsWith("/api/v1/jobs?")) {
        return jsonResponse({ items: [job] });
      }
      if (url === `/api/v1/jobs/${job.id}`) {
        return jsonResponse(job);
      }
      if (typeof url === "string" && url.startsWith("/api/v1/batches/batch_progress")) {
        return jsonResponse(batch);
      }
      return jsonResponse({ items: [] });
    });

    const { unmount } = render(App);

    await openRecentTab();
    const refresh = await screen.findByRole("button", { name: /refresh list/i });
    if (refresh.disabled) {
      await waitFor(() => expect(refresh).not.toBeDisabled());
    }
    await fireEvent.click(refresh);
    await screen.findByText("job_a");

    const recentHeading = screen.getByRole("heading", { name: "Recent Tests" });
    const recentCard = recentHeading.closest(".card");
    expect(recentCard).not.toBeNull();
    const recentBars = within(recentCard).getAllByRole("progressbar");
    const hasRecentProgress = recentBars.some((bar) => bar.getAttribute("aria-valuenow") === "42");
    expect(hasRecentProgress).toBe(true);
    const noticePill = within(recentCard).getByText("NOTICE 2");
    expect(noticePill).toHaveClass("level-pill", "severity-notice");
    expect(within(recentCard).getByText("WARNING 1")).toHaveClass("level-pill", "severity-warning");
    expect(within(recentCard).getByText("ERROR 3")).toHaveClass("level-pill", "severity-error");
    expect(within(recentCard).queryByText("CRITICAL 0")).toBeNull();
    expect(within(recentCard).queryByText("NOTICE 2 · WARNING 1 · ERROR 3 · CRITICAL 0")).toBeNull();

    await fireEvent.click(screen.getByRole("tab", { name: "Single Job" }));
    const jobInput = screen.getByLabelText("Job ID");
    await fireEvent.input(jobInput, { target: { value: job.id } });
    await fireEvent.change(jobInput);

    const inspectorCard = screen.getByText("Job Inspector").closest(".card");
    expect(inspectorCard).not.toBeNull();
    await waitFor(() => {
      const inspectorBar = within(inspectorCard).getByRole("progressbar");
      expect(inspectorBar).toHaveAttribute("aria-valuenow", "42");
    });

    await openBatchTab();
    await fireEvent.input(screen.getByLabelText("Batch ID"), { target: { value: "batch_progress" } });
    await fireEvent.change(screen.getByLabelText("Batch ID"));

    const batchCard = screen.getByText("Batch Inspector").closest(".card");
    expect(batchCard).not.toBeNull();
    await waitFor(() => {
      const batchBars = within(batchCard).getAllByRole("progressbar");
      const hasBatchProgress = batchBars.some((bar) => bar.getAttribute("aria-valuenow") === "73");
      expect(hasBatchProgress).toBe(true);
    });

    unmount();
  });

  it("filters recent jobs by severity totals", async () => {
    const calls = [];
    const jobs = [
      {
        id: "job_clean",
        domain: "clean.example",
        status: "succeeded",
        created_at: "2026-02-03T00:00:00Z",
        progress: 100,
        severity_totals: { NOTICE: 0, WARNING: 0, ERROR: 0, CRITICAL: 0 }
      },
      {
        id: "job_warn",
        domain: "warn.example",
        status: "failed",
        created_at: "2026-02-03T00:00:01Z",
        progress: 100,
        severity_totals: { NOTICE: 0, WARNING: 2, ERROR: 0, CRITICAL: 0 }
      },
      {
        id: "job_err",
        domain: "error.example",
        status: "failed",
        created_at: "2026-02-03T00:00:02Z",
        progress: 100,
        severity_totals: { NOTICE: 0, WARNING: 0, ERROR: 1, CRITICAL: 0 }
      }
    ];

    global.fetch.mockImplementation((url) => {
      const value = typeof url === "string" ? url : String(url?.url || "");
      calls.push(value);
      if (value.includes("/api/v1/jobs?")) {
        return jsonResponse({ items: jobs, total: jobs.length });
      }
      return jsonResponse({});
    });

    const { unmount } = render(App);

    await openRecentTab();
    const refresh = await screen.findByRole("button", { name: /refresh list/i });
    if (refresh.disabled) {
      await waitFor(() => expect(refresh).not.toBeDisabled());
    }
    await fireEvent.click(refresh);

    expect(await screen.findByText("job_clean")).toBeInTheDocument();
    expect(screen.getByText("job_warn")).toBeInTheDocument();
    expect(screen.getByText("job_err")).toBeInTheDocument();

    await fireEvent.click(screen.getByRole("button", { name: "Warnings+" }));
    await waitFor(() => {
      expect(calls.some((value) => value.includes("/api/v1/jobs?") && value.includes("severity=warnings_plus"))).toBe(true);
      expect(screen.queryByText("job_clean")).toBeNull();
      expect(screen.getByText("job_warn")).toBeInTheDocument();
      expect(screen.getByText("job_err")).toBeInTheDocument();
    });

    await fireEvent.click(screen.getByRole("button", { name: "Errors only" }));
    await waitFor(() => {
      expect(calls.some((value) => value.includes("/api/v1/jobs?") && value.includes("severity=errors_only"))).toBe(true);
      expect(screen.queryByText("job_clean")).toBeNull();
      expect(screen.queryByText("job_warn")).toBeNull();
      expect(screen.getByText("job_err")).toBeInTheDocument();
    });

    await fireEvent.click(screen.getByRole("button", { name: "All severities" }));
    await waitFor(() => {
      expect(screen.getByText("job_clean")).toBeInTheDocument();
      expect(screen.getByText("job_warn")).toBeInTheDocument();
      expect(screen.getByText("job_err")).toBeInTheDocument();
    });

    unmount();
  });

  it("shows an empty state when severity filters exclude all jobs", async () => {
    const jobs = [
      {
        id: "job_clean_only",
        domain: "clean-only.example",
        status: "succeeded",
        created_at: "2026-02-03T00:00:00Z",
        progress: 100,
        severity_totals: { NOTICE: 0, WARNING: 0, ERROR: 0, CRITICAL: 0 }
      }
    ];

    global.fetch.mockImplementation((url) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      if (value.includes("/api/v1/jobs?")) {
        return jsonResponse({ items: jobs, total: jobs.length });
      }
      return jsonResponse({});
    });

    const { unmount } = render(App);

    await openRecentTab();
    expect(await screen.findByText("job_clean_only")).toBeInTheDocument();

    await fireEvent.click(screen.getByRole("button", { name: "Warnings+" }));
    await waitFor(() => {
      expect(screen.queryByText("job_clean_only")).toBeNull();
      expect(screen.getByText("No jobs match the selected severity filter.")).toBeInTheDocument();
    });

    unmount();
  });

  it("requests recent jobs with selected sort mode", async () => {
    const calls = [];
    const jobs = [
      {
        id: "job_a",
        domain: "a.example",
        status: "queued",
        created_at: "2026-02-03T00:00:00Z",
        progress: 0,
        severity_totals: { NOTICE: 0, WARNING: 0, ERROR: 0, CRITICAL: 0 }
      }
    ];

    global.fetch.mockImplementation((url) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      calls.push(value);
      if (value.includes("/api/v1/jobs?")) {
        return jsonResponse({ items: jobs, total: jobs.length });
      }
      return jsonResponse({});
    });

    const { unmount } = render(App);

    await openRecentTab();
    const refresh = await screen.findByRole("button", { name: /refresh list/i });
    if (refresh.disabled) {
      await waitFor(() => expect(refresh).not.toBeDisabled());
    }
    await fireEvent.click(refresh);

    await waitFor(() => {
      expect(calls.some((value) => value.includes("sort=started_at_desc"))).toBe(true);
    });

    const select = screen.getByLabelText("Sort");
    await fireEvent.change(select, { target: { value: "domain_asc" } });

    await waitFor(() => {
      expect(calls.some((value) => value.includes("sort=domain_asc"))).toBe(true);
    });

    unmount();
  });

  it("applies recent tests pagination query params", async () => {
    const calls = [];
    const firstPage = {
      items: [
        {
          id: "job_recent_1",
          domain: "one.example",
          status: "queued",
          created_at: "2026-02-03T00:00:00Z",
          progress: 10,
          severity_totals: { NOTICE: 0, WARNING: 0, ERROR: 0, CRITICAL: 0 }
        }
      ],
      total: 2,
      limit: 20,
      offset: 0,
      next_cursor: "1",
      prev_cursor: ""
    };
    const secondPage = {
      ...firstPage,
      items: [
        {
          id: "job_recent_2",
          domain: "two.example",
          status: "queued",
          created_at: "2026-02-03T00:00:01Z",
          progress: 20,
          severity_totals: { NOTICE: 0, WARNING: 0, ERROR: 0, CRITICAL: 0 }
        }
      ],
      offset: 1,
      next_cursor: "",
      prev_cursor: "0"
    };

    global.fetch.mockImplementation((url) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      calls.push(value);
      if (value.includes("/api/v1/jobs?")) {
        if (value.includes("cursor=1")) {
          return jsonResponse(secondPage);
        }
        return jsonResponse(firstPage);
      }
      return jsonResponse({});
    });

    const { unmount } = render(App);

    await openRecentTab();
    expect(await screen.findByText("job_recent_1")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Previous" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Next" })).not.toBeDisabled();

    await fireEvent.change(screen.getByLabelText("Page size"), { target: { value: "50" } });
    await waitFor(() => {
      expect(calls.some((value) => value.includes("/api/v1/jobs?") && value.includes("limit=50"))).toBe(true);
    });

    await fireEvent.click(screen.getByRole("button", { name: "Next" }));
    await waitFor(() => {
      expect(calls.some((value) => value.includes("/api/v1/jobs?") && value.includes("cursor=1"))).toBe(true);
      expect(screen.getByText("job_recent_2")).toBeInTheDocument();
    });

    expect(screen.getByRole("button", { name: "Next" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Previous" })).not.toBeDisabled();

    unmount();
  });

  it("filters recent jobs by batch id", async () => {
    const calls = [];
    const allJobs = [
      {
        id: "job_batch_a",
        batch_id: "batch_a",
        domain: "a.example",
        status: "queued",
        created_at: "2026-02-03T00:00:00Z",
        progress: 0,
        severity_totals: { NOTICE: 0, WARNING: 0, ERROR: 0, CRITICAL: 0 }
      },
      {
        id: "job_batch_b",
        batch_id: "batch_b",
        domain: "b.example",
        status: "queued",
        created_at: "2026-02-03T00:00:01Z",
        progress: 0,
        severity_totals: { NOTICE: 0, WARNING: 0, ERROR: 0, CRITICAL: 0 }
      }
    ];
    const filtered = [allJobs[0]];

    global.fetch.mockImplementation((url) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      calls.push(value);
      if (value.includes("/api/v1/jobs?")) {
        if (value.includes("batch_id=batch_a")) {
          return jsonResponse({ items: filtered, total: filtered.length });
        }
        return jsonResponse({ items: allJobs, total: allJobs.length });
      }
      return jsonResponse({});
    });

    const { unmount } = render(App);

    await openRecentTab();
    const refresh = await screen.findByRole("button", { name: /refresh list/i });
    if (refresh.disabled) {
      await waitFor(() => expect(refresh).not.toBeDisabled());
    }
    await fireEvent.click(refresh);

    expect(await screen.findByText("job_batch_a")).toBeInTheDocument();
    expect(screen.getByText("job_batch_b")).toBeInTheDocument();

    await fireEvent.input(screen.getByLabelText("Batch ID filter"), { target: { value: "batch_a" } });
    await fireEvent.click(screen.getByRole("button", { name: "Apply filters" }));

    await waitFor(() => {
      expect(calls.some((value) => value.includes("batch_id=batch_a"))).toBe(true);
      expect(screen.getByText("job_batch_a")).toBeInTheDocument();
      expect(screen.queryByText("job_batch_b")).toBeNull();
    });

    unmount();
  });

  it("opens single job inspector when inspecting from recent tests", async () => {
    const calls = [];
    const job = {
      id: "job_recent_inspect",
      domain: "inspect.example",
      status: "running",
      created_at: "2026-02-03T00:00:00Z",
      progress: 25,
      severity_totals: { NOTICE: 0, WARNING: 0, ERROR: 0, CRITICAL: 0 }
    };

    global.fetch.mockImplementation((url) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      calls.push(value);
      if (value.includes("/api/v1/jobs?")) {
        return jsonResponse({ items: [job], total: 1 });
      }
      if (value === `/api/v1/jobs/${job.id}`) {
        return jsonResponse(job);
      }
      return jsonResponse({});
    });

    const { unmount } = render(App);

    await openRecentTab();
    const refresh = await screen.findByRole("button", { name: /refresh list/i });
    if (refresh.disabled) {
      await waitFor(() => expect(refresh).not.toBeDisabled());
    }
    await fireEvent.click(refresh);

    const row = (await screen.findByText(job.id)).closest(".list-item");
    expect(row).not.toBeNull();
    expect(row.getAttribute("role")).toBe("button");
    await fireEvent.click(row);

    await waitFor(() => {
      expect(calls.some((value) => value === `/api/v1/jobs/${job.id}`)).toBe(true);
      expect(screen.getByRole("tab", { name: "Single Job" })).toHaveAttribute("aria-selected", "true");
      expect(screen.getByLabelText("Job ID")).toHaveValue(job.id);
    });

    unmount();
  });

  it("renders recent job rows as clickable with job id, domain, and severity on one line", async () => {
    const job = {
      id: "job_inline",
      domain: "inline.example",
      status: "succeeded",
      created_at: "2026-02-03T00:00:00Z",
      progress: 100,
      severity_totals: { NOTICE: 0, WARNING: 2, ERROR: 1, CRITICAL: 0 }
    };

    global.fetch.mockImplementation((url) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      if (value.includes("/api/v1/jobs?")) {
        return jsonResponse({ items: [job], total: 1 });
      }
      return jsonResponse({});
    });

    const { unmount } = render(App);
    await openRecentTab();
    const refresh = await screen.findByRole("button", { name: /refresh list/i });
    if (refresh.disabled) {
      await waitFor(() => expect(refresh).not.toBeDisabled());
    }
    await fireEvent.click(refresh);

    const row = (await screen.findByText(job.id)).closest(".list-item");
    expect(row).not.toBeNull();

    // Row is clickable (role=button), no separate Inspect button
    expect(row.getAttribute("role")).toBe("button");
    expect(within(row).queryByRole("button", { name: "Inspect" })).toBeNull();

    // Job ID, domain-status, and severity pills share the same .job-headline container
    const headline = row.querySelector(".job-headline");
    expect(headline).not.toBeNull();
    expect(within(headline).getByText(job.id)).toBeInTheDocument();
    expect(within(headline).getByText(/inline\.example/)).toBeInTheDocument();
    expect(within(headline).getByText(/WARNING 2/)).toBeInTheDocument();
    expect(within(headline).getByText(/ERROR 1/)).toBeInTheDocument();

    unmount();
  });

  it("shows INFO pill for completed job with all-zero severity", async () => {
    const cleanJob = {
      id: "job_clean",
      domain: "clean.example",
      status: "succeeded",
      created_at: "2026-02-03T00:00:00Z",
      progress: 100,
      severity_totals: { NOTICE: 0, WARNING: 0, ERROR: 0, CRITICAL: 0 }
    };

    global.fetch.mockImplementation((url) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      if (value.includes("/api/v1/jobs?")) {
        return jsonResponse({ items: [cleanJob], total: 1 });
      }
      return jsonResponse({});
    });

    const { unmount } = render(App);
    await openRecentTab();
    const refresh = await screen.findByRole("button", { name: /refresh list/i });
    if (refresh.disabled) {
      await waitFor(() => expect(refresh).not.toBeDisabled());
    }
    await fireEvent.click(refresh);

    const row = (await screen.findByText(cleanJob.id)).closest(".list-item");
    const headline = row.querySelector(".job-headline");
    expect(within(headline).getByText("INFO")).toBeInTheDocument();
    expect(within(headline).getByText("INFO").className).toContain("severity-info");

    unmount();
  });

  it("handles batch pagination edges on first and last pages", async () => {
    const firstPage = {
      batch_id: "batch_edge",
      total: 2,
      status_counts: { failed: 2 },
      items: [
        {
          id: "job_edge_1",
          domain: "edge-1.example",
          status: "failed",
          created_at: "2026-02-03T00:00:00Z",
          progress: 100
        }
      ],
      created_at: "2026-02-03T00:00:00Z",
      offset: 0,
      next_cursor: "1",
      prev_cursor: "",
      sort: "started_at_desc"
    };
    const lastPage = {
      ...firstPage,
      items: [
        {
          id: "job_edge_2",
          domain: "edge-2.example",
          status: "failed",
          created_at: "2026-02-03T00:00:01Z",
          progress: 100
        }
      ],
      offset: 1,
      next_cursor: "",
      prev_cursor: "0"
    };

    global.fetch.mockImplementation((url) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      if (value.includes("/api/v1/jobs?")) {
        return jsonResponse({ items: [] });
      }
      if (value.includes("/api/v1/batches/batch_edge")) {
        if (value.includes("cursor=1")) {
          return jsonResponse(lastPage);
        }
        return jsonResponse(firstPage);
      }
      return jsonResponse({});
    });

    const { unmount } = render(App);

    await openBatchTab();
    await fireEvent.input(screen.getByLabelText("Batch ID"), { target: { value: "batch_edge" } });
    await fireEvent.change(screen.getByLabelText("Batch ID"));

    expect(await screen.findByText("job_edge_1")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Previous" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Next" })).not.toBeDisabled();

    await fireEvent.click(screen.getByRole("button", { name: "Next" }));
    expect(await screen.findByText("job_edge_2")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Next" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Previous" })).not.toBeDisabled();

    await fireEvent.click(screen.getByRole("button", { name: "Previous" }));
    expect(await screen.findByText("job_edge_1")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Previous" })).toBeDisabled();

    unmount();
  });

  it("normalizes invalid persisted URL filters and pagination values", async () => {
    window.history.replaceState(
      null,
      "",
      "/?r_sort=bad_sort&r_sev=bad_filter&r_batch=batch_from_url&r_limit=13&r_cursor=-4&b_id=batch_invalid&b_sort=bad_batch_sort&b_limit=13&b_cursor=-4&b_status=wat&b_domain=edge#/batches"
    );

    const calls = [];
    global.fetch.mockImplementation((url) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      calls.push(value);
      if (value.includes("/api/v1/jobs?")) {
        return jsonResponse({ items: [], total: 0 });
      }
      if (value.includes("/api/v1/batches/batch_invalid")) {
        return jsonResponse({
          batch_id: "batch_invalid",
          total: 0,
          status_counts: {},
          items: [],
          created_at: "2026-02-03T00:00:00Z"
        });
      }
      return jsonResponse({});
    });

    const { unmount } = render(App);

    await waitFor(() => {
      expect(
        calls.some(
          (value) =>
            value.includes("/api/v1/jobs?") &&
            value.includes("sort=started_at_desc") &&
            value.includes("batch_id=batch_from_url") &&
            value.includes("limit=20") &&
            !value.includes("cursor=")
        )
      ).toBe(true);
      expect(
        calls.some(
          (value) =>
            value.includes("/api/v1/batches/batch_invalid?") &&
            value.includes("sort=started_at_desc") &&
            value.includes("limit=20") &&
            value.includes("domain=edge") &&
            !value.includes("status=wat") &&
            !value.includes("cursor=")
        )
      ).toBe(true);
    });

    expect(screen.getByLabelText("Sort")).toHaveValue("started_at_desc");
    expect(screen.getByLabelText("Page size")).toHaveValue("20");
    expect(screen.getByLabelText("Status")).toHaveValue("");

    await openRecentTab();
    expect(screen.getByLabelText("Sort")).toHaveValue("started_at_desc");
    expect(screen.getByLabelText("Page size")).toHaveValue("20");
    expect(screen.getByLabelText("Batch ID filter")).toHaveValue("batch_from_url");

    unmount();
  });

  it("summarizes only notice and above levels with non-zero counts", async () => {
    const job = {
      id: "job_summary",
      domain: "example.com",
      status: "succeeded",
      created_at: "2026-02-03T00:00:00Z",
      progress: 100
    };
    const result = {
      summary: {
        levels: {
          NOTICE: 2,
          WARNING: 0,
          ERROR: 1,
          CRITICAL: 0,
          INFO: 5
        }
      },
      raw: {}
    };

    global.fetch.mockImplementation((url) => {
      if (typeof url === "string" && url.startsWith("/api/v1/jobs?")) {
        return jsonResponse({ items: [] });
      }
      if (url === `/api/v1/jobs/${job.id}`) {
        return jsonResponse(job);
      }
      if (typeof url === "string" && url.startsWith(`/api/v1/jobs/${job.id}/result`)) {
        return jsonResponse(result);
      }
      return jsonResponse({});
    });

    const { unmount } = render(App);

    const input = await screen.findByLabelText("Job ID");
    await fireEvent.input(input, { target: { value: job.id } });
    await fireEvent.change(input);

    await waitFor(() => {
      expect(screen.getByText("NOTICE")).toBeInTheDocument();
      expect(screen.getByText("ERROR")).toBeInTheDocument();
    });

    const summaryGrid = screen.getByText("NOTICE").closest(".summary-grid");
    expect(summaryGrid).not.toBeNull();
    const summaryScope = within(summaryGrid);
    expect(summaryScope.getByText("2")).toBeInTheDocument();
    expect(summaryScope.getByText("1")).toBeInTheDocument();

    expect(screen.queryByText("WARNING")).toBeNull();
    expect(screen.queryByText("CRITICAL")).toBeNull();

    unmount();
  });

  it("groups raw results by module and toggles entries", async () => {
    const job = {
      id: "job_raw",
      domain: "example.com",
      status: "succeeded",
      created_at: "2026-02-03T00:00:00Z",
      progress: 100
    };
    const result = {
      summary: { levels: {} },
      raw: {
        locale: "en",
        entries: [
          {
            timestamp: 0.12,
            module: "BASIC",
            testcase: "basic01",
            tag: "BASIC01",
            level: "NOTICE",
            message: "All checks passed.",
            raw: "BASIC:basic01:BASIC01"
          },
          {
            timestamp: 0.33,
            module: "DNSSEC",
            testcase: "dnssec01",
            tag: "DNSSEC01",
            level: "ERROR",
            message: "DNSSEC validation failed.",
            raw: "DNSSEC:dnssec01:DNSSEC01"
          }
        ]
      }
    };

    global.fetch.mockImplementation((url) => {
      if (typeof url === "string" && url.startsWith("/api/v1/jobs?")) {
        return jsonResponse({ items: [] });
      }
      if (url === `/api/v1/jobs/${job.id}`) {
        return jsonResponse(job);
      }
      if (typeof url === "string" && url.startsWith(`/api/v1/jobs/${job.id}/result`)) {
        return jsonResponse(result);
      }
      return jsonResponse({});
    });

    const { unmount } = render(App);

    const input = await screen.findByLabelText("Job ID");
    await fireEvent.input(input, { target: { value: job.id } });
    await fireEvent.change(input);

    const moduleButton = await screen.findByRole("button", { name: /basic/i });
    expect(screen.queryByText("All checks passed.")).toBeNull();

    await fireEvent.click(moduleButton);
    expect(await screen.findByText("All checks passed.")).toBeInTheDocument();

    await fireEvent.click(moduleButton);
    await waitFor(() => {
      expect(screen.queryByText("All checks passed.")).toBeNull();
    });

    unmount();
  });

  describe("locale selector", () => {
    const mockFetchWithLocales = (locales) => {
      global.fetch.mockImplementation((url) => {
        const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
        if (value.includes("/api/v1/locales")) {
          return jsonResponse({ locales });
        }
        if (value.includes("/api/v1/jobs?")) {
          return jsonResponse({ items: [], total: 0 });
        }
        return jsonResponse({});
      });
    };

    it("shows language selector when server returns multiple locales", async () => {
      mockFetchWithLocales(["da", "en", "fr", "sv"]);

      const { unmount } = render(App);

      await waitFor(() => {
        expect(screen.getByRole("combobox", { name: "Result language" })).toBeInTheDocument();
      });
      const select = screen.getByRole("combobox", { name: "Result language" });
      expect(within(select).getByRole("option", { name: "English" })).toBeInTheDocument();
      expect(within(select).getByRole("option", { name: "Dansk" })).toBeInTheDocument();
      expect(within(select).getByRole("option", { name: "Français" })).toBeInTheDocument();
      expect(within(select).getByRole("option", { name: "Svenska" })).toBeInTheDocument();

      unmount();
    });

    it("hides language selector when server returns only one locale", async () => {
      mockFetchWithLocales(["en"]);

      const { unmount } = render(App);

      // Give the locales fetch time to resolve.
      await waitFor(() => {
        expect(global.fetch).toHaveBeenCalled();
      });
      // Wait a tick for Svelte to re-render.
      await new Promise((r) => setTimeout(r, 0));

      expect(screen.queryByRole("combobox", { name: "Result language" })).toBeNull();

      unmount();
    });

    it("persists chosen locale to localStorage when changed", async () => {
      mockFetchWithLocales(["en", "sv", "da"]);

      const { unmount } = render(App);

      const select = await screen.findByRole("combobox", { name: "Result language" });
      await fireEvent.change(select, { target: { value: "sv" } });

      expect(localStorage.getItem("gonemaster.ui.locale.v1")).toBe("sv");

      unmount();
    });

    it("restores locale from localStorage on load", async () => {
      localStorage.setItem("gonemaster.ui.locale.v1", "fr");
      mockFetchWithLocales(["en", "fr", "sv"]);

      const { unmount } = render(App);

      const select = await screen.findByRole("combobox", { name: "Result language" });
      expect(select.value).toBe("fr");

      unmount();
    });

    it("falls back to English when stored locale is not in server list", async () => {
      localStorage.setItem("gonemaster.ui.locale.v1", "ja");
      mockFetchWithLocales(["en", "da", "sv"]);

      const { unmount } = render(App);

      const select = await screen.findByRole("combobox", { name: "Result language" });
      await waitFor(() => {
        expect(select.value).toBe("en");
      });
      expect(localStorage.getItem("gonemaster.ui.locale.v1")).toBe("en");

      unmount();
    });

    it("switching locale selector updates UI chrome strings reactively", async () => {
      // Inject a minimal Swedish catalog so we can observe a chrome string change.
      setCatalog("sv", { run_single_job: "Kör enkeljobb" });

      global.fetch.mockImplementation((url) => {
        const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
        if (value.includes("/api/v1/locales")) {
          return jsonResponse({ locales: ["en", "sv"] });
        }
        if (value.includes("/api/v1/jobs?")) {
          return jsonResponse({ items: [], total: 0 });
        }
        return jsonResponse({});
      });

      const { unmount } = render(App);

      // Wait for the locale selector to appear (loadLocales must resolve first).
      const select = await screen.findByRole("combobox", { name: "Result language" });

      // Initially English.
      expect(screen.getByRole("button", { name: "Run Single Job" })).toBeInTheDocument();

      // Switch to Swedish.
      await fireEvent.change(select, { target: { value: "sv" } });

      // The chrome string should update reactively to the Swedish translation.
      await waitFor(() => {
        expect(screen.getByRole("button", { name: "Kör enkeljobb" })).toBeInTheDocument();
      });

      unmount();
      // Reset locale store so subsequent tests start in English.
      locale.set("en");
    });
  });

  describe("Browser notifications", () => {
    const makeNotificationMock = (permission) => {
      const mock = vi.fn();
      mock.permission = permission;
      mock.requestPermission = vi.fn().mockResolvedValue(permission);
      return mock;
    };

    const submitAndWaitForQueued = async (domain) => {
      const input = await screen.findByPlaceholderText("example.com");
      await fireEvent.input(input, { target: { value: domain } });
      await fireEvent.click(screen.getByText("Run Single Job"));
      await waitFor(() => {
        expect(screen.getByText(/queued/)).toBeInTheDocument();
      });
    };

    it("sends a browser notification when a watched single job completes", async () => {
      const NotificationMock = makeNotificationMock("granted");
      vi.stubGlobal("Notification", NotificationMock);

      const intervalCallbacks = [];
      vi.spyOn(global, "setInterval").mockImplementation((fn) => {
        intervalCallbacks.push(fn);
        return intervalCallbacks.length;
      });
      vi.spyOn(global, "clearInterval").mockImplementation(() => {});

      let jobCallCount = 0;
      const queuedJob = {
        id: "job_notify",
        domain: "notify.example",
        status: "queued",
        progress: 0,
        created_at: "2026-02-03T00:00:00Z"
      };
      const doneJob = { ...queuedJob, status: "succeeded", progress: 100 };

      global.fetch.mockImplementation((url, options = {}) => {
        const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
        if (url === "/api/v1/jobs" && options?.method === "POST") return jsonResponse(queuedJob);
        if (value.includes(`/api/v1/jobs/${queuedJob.id}`)) {
          jobCallCount++;
          return jsonResponse(jobCallCount >= 2 ? doneJob : queuedJob);
        }
        if (value.includes("/api/v1/jobs?")) return jsonResponse({ items: [queuedJob], total: 1 });
        return jsonResponse({});
      });

      const { unmount } = render(App);
      await submitAndWaitForQueued("notify.example");

      expect(intervalCallbacks.length).toBeGreaterThan(0);
      const poller = intervalCallbacks[intervalCallbacks.length - 1];
      await poller();

      await waitFor(() => {
        expect(NotificationMock).toHaveBeenCalledWith(
          "Scan complete",
          expect.objectContaining({ body: expect.stringContaining("notify.example") })
        );
      });

      unmount();
    });

    it("requests notification permission when submitting a single job", async () => {
      const NotificationMock = makeNotificationMock("default");
      vi.stubGlobal("Notification", NotificationMock);

      const queuedJob = {
        id: "job_perm",
        domain: "perm.example",
        status: "queued",
        progress: 0,
        created_at: "2026-02-03T00:00:00Z"
      };

      global.fetch.mockImplementation((url, options = {}) => {
        const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
        if (url === "/api/v1/jobs" && options?.method === "POST") return jsonResponse(queuedJob);
        if (value.includes(`/api/v1/jobs/${queuedJob.id}`)) return jsonResponse(queuedJob);
        if (value.includes("/api/v1/jobs?")) return jsonResponse({ items: [], total: 0 });
        return jsonResponse({});
      });

      const { unmount } = render(App);
      await submitAndWaitForQueued("perm.example");

      await waitFor(() => {
        expect(NotificationMock.requestPermission).toHaveBeenCalled();
      });

      unmount();
    });

    it("does not send a notification when Notification permission is denied", async () => {
      const NotificationMock = makeNotificationMock("denied");
      vi.stubGlobal("Notification", NotificationMock);

      const intervalCallbacks = [];
      vi.spyOn(global, "setInterval").mockImplementation((fn) => {
        intervalCallbacks.push(fn);
        return intervalCallbacks.length;
      });
      vi.spyOn(global, "clearInterval").mockImplementation(() => {});

      let jobCallCount = 0;
      const queuedJob = {
        id: "job_denied",
        domain: "denied.example",
        status: "queued",
        progress: 0,
        created_at: "2026-02-03T00:00:00Z"
      };
      const doneJob = { ...queuedJob, status: "succeeded", progress: 100 };

      global.fetch.mockImplementation((url, options = {}) => {
        const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
        if (url === "/api/v1/jobs" && options?.method === "POST") return jsonResponse(queuedJob);
        if (value.includes(`/api/v1/jobs/${queuedJob.id}`)) {
          jobCallCount++;
          return jsonResponse(jobCallCount >= 2 ? doneJob : queuedJob);
        }
        if (value.includes("/api/v1/jobs?")) return jsonResponse({ items: [], total: 0 });
        return jsonResponse({});
      });

      const { unmount } = render(App);
      await submitAndWaitForQueued("denied.example");

      expect(intervalCallbacks.length).toBeGreaterThan(0);
      const poller = intervalCallbacks[intervalCallbacks.length - 1];
      await poller();

      await waitFor(() => {
        expect(screen.getByText(/succeeded/)).toBeInTheDocument();
      });

      // Notification constructor must not have been called.
      expect(NotificationMock).not.toHaveBeenCalled();

      unmount();
    });

    it("sends a browser notification when a batch finishes", async () => {
      const NotificationMock = makeNotificationMock("granted");
      vi.stubGlobal("Notification", NotificationMock);

      const intervalCallbacks = [];
      vi.spyOn(global, "setInterval").mockImplementation((fn) => {
        intervalCallbacks.push(fn);
        return intervalCallbacks.length;
      });
      vi.spyOn(global, "clearInterval").mockImplementation(() => {});

      let batchCalls = 0;
      const runningBatch = {
        batch_id: "batch_notify",
        total: 1,
        status_counts: { running: 1 },
        items: [
          {
            id: "job_bn",
            domain: "bn.example",
            status: "running",
            created_at: "2026-02-03T00:00:00Z",
            progress: 50
          }
        ],
        created_at: "2026-02-03T00:00:00Z"
      };
      const doneBatch = {
        ...runningBatch,
        status_counts: { succeeded: 1 },
        items: [{ ...runningBatch.items[0], status: "succeeded", progress: 100 }]
      };

      global.fetch.mockImplementation((url, options = {}) => {
        const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
        if (url === "/api/v1/jobs/batch" && options?.method === "POST") {
          return jsonResponse({ batch_id: "batch_notify", job_ids: ["job_bn"] });
        }
        if (value.includes("/api/v1/batches/batch_notify")) {
          batchCalls++;
          return jsonResponse(batchCalls >= 2 ? doneBatch : runningBatch);
        }
        if (value.includes("/api/v1/jobs?")) return jsonResponse({ items: [], total: 0 });
        return jsonResponse({});
      });

      const { unmount } = render(App);

      await fireEvent.click(screen.getByRole("tab", { name: "Batch Jobs" }));
      await fireEvent.input(await screen.findByLabelText("Domains (one per line)"), {
        target: { value: "bn.example" }
      });
      await fireEvent.click(screen.getByText("Run Batch"));

      await waitFor(() => {
        expect(screen.getByText("job_bn")).toBeInTheDocument();
      });

      // Fire the batch poller
      expect(intervalCallbacks.length).toBeGreaterThan(0);
      const poller = intervalCallbacks[intervalCallbacks.length - 1];
      await poller();

      // Flush microtasks so the async sendBatchNotification completes
      await new Promise((r) => setTimeout(r, 0));

      await waitFor(() => {
        expect(NotificationMock).toHaveBeenCalledWith(
          "Batch complete",
          expect.objectContaining({ body: expect.stringContaining("batch_notify") })
        );
      });

      unmount();
    });
  });

  describe("Domains tab", () => {
    const openDomainsTab = async () => {
      await fireEvent.click(screen.getByRole("tab", { name: "Domains" }));
    };

    it("renders Domains tab button", async () => {
      global.fetch.mockImplementation(() => jsonResponse({ items: [], total: 0 }));
      const { unmount } = render(App);
      expect(screen.getByRole("tab", { name: "Domains" })).toBeInTheDocument();
      unmount();
    });

    it("shows domain list when tab opened", async () => {
      const calls = [];
      global.fetch.mockImplementation((url) => {
        const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
        calls.push(value);
        if (value.includes("/api/v1/domains")) {
          return jsonResponse({
            items: [{ id: 1, name: "example.com", tags: ["tld"], latest_level: "WARNING", latest_run_at: "2026-03-15T10:00:00Z", run_count: 3 }],
            total: 1
          });
        }
        if (value.includes("/api/v1/tags")) {
          return jsonResponse([{ name: "tld", description: "TLD", domain_count: 1 }]);
        }
        return jsonResponse({ items: [], total: 0 });
      });

      const { unmount } = render(App);
      await openDomainsTab();

      await waitFor(() => {
        expect(calls.some((v) => v.includes("/api/v1/domains"))).toBe(true);
      });
      expect(await screen.findByText("example.com")).toBeInTheDocument();
      expect(screen.getByText("WARNING")).toBeInTheDocument();

      unmount();
    });

    it("shows placeholder when no domains", async () => {
      global.fetch.mockImplementation((url) => {
        const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
        if (value.includes("/api/v1/domains")) return jsonResponse({ items: [], total: 0 });
        if (value.includes("/api/v1/tags")) return jsonResponse([]);
        return jsonResponse({ items: [], total: 0 });
      });

      const { unmount } = render(App);
      await openDomainsTab();

      expect(await screen.findByText("No domains found.")).toBeInTheDocument();
      unmount();
    });

    it("tag filter dropdown is populated from /api/v1/tags", async () => {
      global.fetch.mockImplementation((url) => {
        const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
        if (value.includes("/api/v1/domains")) return jsonResponse({ items: [], total: 0 });
        if (value.includes("/api/v1/tags")) return jsonResponse([{ name: "ccTLD", description: "", domain_count: 5 }]);
        return jsonResponse({ items: [], total: 0 });
      });

      const { unmount } = render(App);
      await openDomainsTab();

      await waitFor(() => {
        expect(screen.getByRole("option", { name: "ccTLD" })).toBeInTheDocument();
      });
      unmount();
    });

    it("name filter triggers new domains request", async () => {
      const calls = [];
      global.fetch.mockImplementation((url) => {
        const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
        calls.push(value);
        if (value.includes("/api/v1/domains")) return jsonResponse({ items: [], total: 0 });
        if (value.includes("/api/v1/tags")) return jsonResponse([]);
        return jsonResponse({ items: [], total: 0 });
      });

      const { unmount } = render(App);
      await openDomainsTab();
      await waitFor(() => calls.some((v) => v.includes("/api/v1/domains")));
      calls.length = 0;

      const searchInput = await screen.findByPlaceholderText("Search by name…");
      await fireEvent.input(searchInput, { target: { value: "example" } });

      await waitFor(() => {
        expect(calls.some((v) => v.includes("/api/v1/domains") && v.includes("name=example"))).toBe(true);
      });
      unmount();
    });

    it("level filter sends min_level param", async () => {
      const calls = [];
      global.fetch.mockImplementation((url) => {
        const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
        calls.push(value);
        if (value.includes("/api/v1/domains")) return jsonResponse({ items: [], total: 0 });
        if (value.includes("/api/v1/tags")) return jsonResponse([]);
        return jsonResponse({ items: [], total: 0 });
      });

      const { unmount } = render(App);
      await openDomainsTab();
      await waitFor(() => calls.some((v) => v.includes("/api/v1/domains")));
      calls.length = 0;

      const levelSelect = await screen.findByRole("combobox", { name: "Filter by level" });
      await fireEvent.change(levelSelect, { target: { value: "WARNING" } });

      await waitFor(() => {
        expect(calls.some((v) => v.includes("/api/v1/domains") && v.includes("min_level=WARNING"))).toBe(true);
      });
      unmount();
    });

    it("clicking a domain row shows detail view and loads runs", async () => {
      const calls = [];
      global.fetch.mockImplementation((url) => {
        const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
        calls.push(value);
        if (value.includes("/runs")) return jsonResponse({ items: [], total: 0 });
        if (value.includes("/api/v1/domains")) {
          return jsonResponse({
            items: [{ id: 7, name: "example.com", tags: ["tld"], latest_level: "WARNING", latest_run_at: "2026-03-15T10:00:00Z", run_count: 1 }],
            total: 1
          });
        }
        if (value.includes("/api/v1/tags")) return jsonResponse([]);
        return jsonResponse({ items: [], total: 0 });
      });

      const { unmount } = render(App);
      await openDomainsTab();

      const row = await screen.findByText("example.com");
      await fireEvent.click(row);

      await waitFor(() => {
        expect(screen.getByText("← Back to domains")).toBeInTheDocument();
        expect(calls.some((v) => v.includes("/api/v1/domains/7/runs"))).toBe(true);
      });
      unmount();
    });

    it("back button returns to domain list", async () => {
      global.fetch.mockImplementation((url) => {
        const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
        if (value.includes("/api/v1/domains")) {
          return jsonResponse({
            items: [{ id: 1, name: "example.com", tags: [], latest_level: "OK", latest_run_at: null, run_count: 0 }],
            total: 1
          });
        }
        if (value.includes("/api/v1/tags")) return jsonResponse([]);
        return jsonResponse({ items: [], total: 0 });
      });

      const { unmount } = render(App);
      await openDomainsTab();

      await fireEvent.click(await screen.findByText("example.com"));
      await waitFor(() => screen.getByText("← Back to domains"));

      await fireEvent.click(screen.getByText("← Back to domains"));
      await waitFor(() => {
        expect(screen.queryByText("← Back to domains")).not.toBeInTheDocument();
        expect(screen.getByRole("heading", { name: "Domains" })).toBeInTheDocument();
      });
      unmount();
    });

    it("detail view shows tags, level, run count, and run history table", async () => {
      global.fetch.mockImplementation((url) => {
        const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
        if (value.includes("/runs")) {
          return jsonResponse({
            items: [{ id: "run-abc", finished_at: "2026-03-15T10:00:00Z", worst_level: "WARNING", duration_ms: 1200, entry_count: 5 }],
            total: 1
          });
        }
        if (value.includes("/api/v1/domains")) {
          return jsonResponse({
            items: [{ id: 3, name: "example.com", tags: ["tld", "ccTLD"], latest_level: "WARNING", latest_run_at: "2026-03-15T10:00:00Z", run_count: 4 }],
            total: 1
          });
        }
        if (value.includes("/api/v1/tags")) return jsonResponse([]);
        return jsonResponse({ items: [], total: 0 });
      });

      const { unmount } = render(App);
      await openDomainsTab();
      await fireEvent.click(await screen.findByText("example.com"));

      await waitFor(() => {
        expect(screen.getByText("tld")).toBeInTheDocument();
        expect(screen.getByText("ccTLD")).toBeInTheDocument();
        expect(screen.getByRole("heading", { name: "Run History" })).toBeInTheDocument();
        expect(screen.getByText("run-abc")).toBeInTheDocument();
        expect(screen.getByText("1200ms")).toBeInTheDocument();
      });
      unmount();
    });

    it("clicking a run row navigates to job inspector", async () => {
      global.fetch.mockImplementation((url) => {
        const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
        if (value.includes("/runs")) {
          return jsonResponse({
            items: [{ id: "run-xyz", finished_at: "2026-03-15T10:00:00Z", worst_level: "ERROR", duration_ms: 800, entry_count: 3 }],
            total: 1
          });
        }
        if (value.includes("/api/v1/domains")) {
          return jsonResponse({ items: [{ id: 2, name: "test.com", tags: [], latest_level: "ERROR", run_count: 1 }], total: 1 });
        }
        if (value.includes("/api/v1/jobs/run-xyz")) return jsonResponse({ id: "run-xyz", status: "succeeded", domain: "test.com" });
        if (value.includes("/api/v1/tags")) return jsonResponse([]);
        return jsonResponse({ items: [], total: 0 });
      });

      const { unmount } = render(App);
      await openDomainsTab();
      await fireEvent.click(await screen.findByText("test.com"));
      const runRow = await screen.findByText("run-xyz");
      await fireEvent.click(runRow);

      await waitFor(() => {
        expect(screen.getByRole("tab", { name: "Single Job" })).toHaveAttribute("aria-selected", "true");
      });
      unmount();
    });

    it("re-test button creates a new job and navigates to inspector", async () => {
      const calls = [];
      global.fetch.mockImplementation((url, opts) => {
        const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
        calls.push({ url: value, method: opts?.method });
        if (value.includes("/runs")) return jsonResponse({ items: [], total: 0 });
        if (value.includes("/api/v1/domains")) {
          return jsonResponse({ items: [{ id: 5, name: "retest.com", tags: [], latest_level: "OK", run_count: 1 }], total: 1 });
        }
        if (value.includes("/api/v1/jobs") && opts?.method === "POST") {
          return jsonResponse({ id: "new-job-99", domain: "retest.com", status: "pending" });
        }
        if (value.includes("/api/v1/jobs/new-job-99")) {
          return jsonResponse({ id: "new-job-99", status: "pending", domain: "retest.com" });
        }
        if (value.includes("/api/v1/tags")) return jsonResponse([]);
        return jsonResponse({ items: [], total: 0 });
      });

      const { unmount } = render(App);
      await openDomainsTab();
      await fireEvent.click(await screen.findByText("retest.com"));
      await waitFor(() => screen.getByText("Re-test"));
      await fireEvent.click(screen.getByText("Re-test"));

      await waitFor(() => {
        expect(calls.some((c) => c.url.includes("/api/v1/jobs") && c.method === "POST")).toBe(true);
        expect(screen.getByRole("tab", { name: "Single Job" })).toHaveAttribute("aria-selected", "true");
      });
      unmount();
    });

    it("pagination next/prev buttons appear when total exceeds limit", async () => {
      global.fetch.mockImplementation((url) => {
        const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
        if (value.includes("/api/v1/domains")) {
          return jsonResponse({
            items: Array.from({ length: 50 }, (_, i) => ({
              id: i + 1, name: `domain${i}.example`, tags: [], run_count: 0
            })),
            total: 120
          });
        }
        if (value.includes("/api/v1/tags")) return jsonResponse([]);
        return jsonResponse({ items: [], total: 0 });
      });

      const { unmount } = render(App);
      await openDomainsTab();

      await waitFor(() => {
        expect(screen.getByText("Next →")).toBeInTheDocument();
      });
      expect(screen.getByText("1–50 / 120")).toBeInTheDocument();
      unmount();
    });
  });

  describe("Tags tab", () => {
    const openTagsTab = async () => {
      await fireEvent.click(screen.getByRole("tab", { name: "Tags" }));
    };

    const mockTagFetch = (tags = [], summary = null, domains = []) => {
      global.fetch.mockImplementation((url, opts) => {
        const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
        if (value.includes("/summary")) return jsonResponse(summary ?? { tag: "t", domain_count: 0, ok: 0, notice: 0, warning: 0, error: 0, critical: 0 });
        if (value.includes("/domains") && value.includes("/tags/")) return jsonResponse({ items: domains, total: domains.length });
        if (value.includes("/api/v1/tags") && opts?.method === "POST") return jsonResponse({ name: "new-tag", description: "", domain_count: 0 }, true);
        if (value.includes("/api/v1/tags") && opts?.method === "DELETE") return { ok: true, status: 204, headers: { get: () => null }, json: async () => ({}), text: async () => "" };
        if (value.includes("/api/v1/tags")) return jsonResponse(tags);
        return jsonResponse({ items: [], total: 0 });
      });
    };

    it("renders Tags tab button", async () => {
      mockTagFetch();
      const { unmount } = render(App);
      expect(screen.getByRole("tab", { name: "Tags" })).toBeInTheDocument();
      unmount();
    });

    it("shows tag list when tab opened", async () => {
      mockTagFetch([{ name: "tld", description: "Top-level", domain_count: 5 }]);
      const { unmount } = render(App);
      await openTagsTab();
      expect(await screen.findByText("tld")).toBeInTheDocument();
      expect(screen.getByText("Top-level")).toBeInTheDocument();
      unmount();
    });

    it("shows placeholder when no tags", async () => {
      mockTagFetch([]);
      const { unmount } = render(App);
      await openTagsTab();
      expect(await screen.findByText("No tags found.")).toBeInTheDocument();
      unmount();
    });

    it("create tag form calls POST /api/v1/tags", async () => {
      const calls = [];
      global.fetch.mockImplementation((url, opts) => {
        const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
        calls.push({ url: value, method: opts?.method });
        if (value.includes("/api/v1/tags") && opts?.method === "POST") return jsonResponse({ name: "newtag", description: "", domain_count: 0 });
        if (value.includes("/api/v1/tags")) return jsonResponse([]);
        return jsonResponse({ items: [], total: 0 });
      });

      const { unmount } = render(App);
      await openTagsTab();

      await fireEvent.input(screen.getByLabelText("Name"), { target: { value: "newtag" } });
      await fireEvent.click(screen.getByText("Create"));

      await waitFor(() => {
        expect(calls.some((c) => c.url.includes("/api/v1/tags") && c.method === "POST")).toBe(true);
      });
      unmount();
    });

    it("pressing Enter in description field submits create tag form", async () => {
      const calls = [];
      global.fetch.mockImplementation((url, opts) => {
        const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
        calls.push({ url: value, method: opts?.method });
        if (value.includes("/api/v1/tags") && opts?.method === "POST") return jsonResponse({ name: "newtag", description: "a desc", domain_count: 0 });
        if (value.includes("/api/v1/tags")) return jsonResponse([]);
        return jsonResponse({ items: [], total: 0 });
      });

      const { unmount } = render(App);
      await openTagsTab();

      await fireEvent.input(screen.getByLabelText("Name"), { target: { value: "newtag" } });
      await fireEvent.input(screen.getByLabelText("Description"), { target: { value: "a desc" } });
      await fireEvent.keyDown(screen.getByLabelText("Description"), { key: "Enter" });

      await waitFor(() => {
        expect(calls.some((c) => c.url.includes("/api/v1/tags") && c.method === "POST")).toBe(true);
      });
      unmount();
    });

    it("clicking a tag row opens detail view with summary and domains", async () => {
      mockTagFetch(
        [{ name: "ccTLD", description: "ccTLDs", domain_count: 2 }],
        { tag: "ccTLD", domain_count: 2, ok: 0, notice: 0, warning: 1, error: 1, critical: 0 },
        [{ id: 1, name: "example.com", latest_level: "WARNING", run_count: 1 }]
      );
      const { unmount } = render(App);
      await openTagsTab();
      await fireEvent.click(await screen.findByText("ccTLD"));

      await waitFor(() => {
        expect(screen.getByText("← Back to tags")).toBeInTheDocument();
        expect(screen.getByRole("heading", { name: "Tag: ccTLD" })).toBeInTheDocument();
        expect(screen.getByRole("heading", { name: "Domains in this tag" })).toBeInTheDocument();
        expect(screen.getByText("example.com")).toBeInTheDocument();
      });
      unmount();
    });

    it("renders tag severity counts inside level pills with Info styling", async () => {
      mockTagFetch(
        [{ name: "ccTLD", description: "ccTLDs", domain_count: 2 }],
        { tag: "ccTLD", domain_count: 2, ok: 3, notice: 2, warning: 1, error: 4, critical: 0 },
        [{ id: 1, name: "example.com", latest_level: "WARNING", run_count: 1 }]
      );
      const { unmount } = render(App);
      await openTagsTab();
      await fireEvent.click(await screen.findByText("ccTLD"));

      await waitFor(() => {
        expect(screen.getByText("Info 3")).toHaveClass("level-pill", "severity-info");
        expect(screen.getByText("Notice 2")).toHaveClass("level-pill", "severity-notice");
        expect(screen.getByText("Warning 1")).toHaveClass("level-pill", "severity-warning");
        expect(screen.getByText("Error 4")).toHaveClass("level-pill", "severity-error");
        expect(screen.getByText("Critical 0")).toHaveClass("level-pill", "severity-critical");
      });
      expect(screen.queryByText("OK: 3")).toBeNull();

      unmount();
    });

    it("back button returns to tag list", async () => {
      mockTagFetch([{ name: "tld", description: "", domain_count: 0 }]);
      const { unmount } = render(App);
      await openTagsTab();
      await fireEvent.click(await screen.findByText("tld"));
      await waitFor(() => screen.getByText("← Back to tags"));
      await fireEvent.click(screen.getByText("← Back to tags"));
      await waitFor(() => {
        expect(screen.queryByText("← Back to tags")).not.toBeInTheDocument();
        expect(screen.getByRole("heading", { name: "Tags" })).toBeInTheDocument();
      });
      unmount();
    });

    it("lets the user save and clear a tag default profile", async () => {
      const calls = [];
      let tags = [{ name: "ops", description: "Operations", domain_count: 2, default_profile_id: null }];

      global.fetch.mockImplementation((url, opts = {}) => {
        const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
        calls.push({ url: value, method: opts?.method, body: opts?.body });
        if (value === "/api/v1/profiles") return jsonResponse(sampleProfiles());
        if (value.includes("/summary")) return jsonResponse({ tag: "ops", domain_count: 2, ok: 0, notice: 0, warning: 0, error: 0, critical: 0 });
        if (value.includes("/domains") && value.includes("/tags/")) return jsonResponse({ items: [], total: 0 });
        if (value.includes("/api/v1/tags/ops/profile") && opts?.method === "PUT") {
          tags = [{ ...tags[0], default_profile_id: 12 }];
          return emptyResponse();
        }
        if (value.includes("/api/v1/tags/ops/profile") && opts?.method === "DELETE") {
          tags = [{ ...tags[0], default_profile_id: null }];
          return emptyResponse();
        }
        if (value.includes("/api/v1/tags")) return jsonResponse(tags);
        return jsonResponse({ items: [], total: 0 });
      });

      const { unmount } = render(App);
      await openTagsTab();
      await fireEvent.click(await screen.findByText("ops"));

      const profileSelect = await screen.findByLabelText("Profile for this tag");
      await fireEvent.change(profileSelect, { target: { value: "12" } });
      await fireEvent.click(screen.getByRole("button", { name: "Save default" }));

      await waitFor(() => {
        const putCall = calls.find((call) => call.url.includes("/api/v1/tags/ops/profile") && call.method === "PUT");
        expect(putCall).toBeTruthy();
        expect(JSON.parse(putCall.body)).toEqual({ profile_id: 12 });
        expect(screen.getByText("Current default: strict")).toBeInTheDocument();
      });

      await fireEvent.click(screen.getByRole("button", { name: "Clear default" }));

      await waitFor(() => {
        expect(calls.some((call) => call.url.includes("/api/v1/tags/ops/profile") && call.method === "DELETE")).toBe(true);
        expect(screen.getByText("Current default: none")).toBeInTheDocument();
      });

      unmount();
    });

    it("Run all button POSTs batch with from_tag", async () => {
      const calls = [];
      global.fetch.mockImplementation((url, opts) => {
        const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
        calls.push({ url: value, method: opts?.method, body: opts?.body });
        if (value.includes("/summary")) return jsonResponse({ tag: "t", domain_count: 0, ok: 0, notice: 0, warning: 0, error: 0, critical: 0 });
        if (value.includes("/domains") && value.includes("/tags/")) return jsonResponse({ items: [], total: 0 });
        if (value.includes("/api/v1/jobs/batch") && opts?.method === "POST") return jsonResponse({ batch_id: "batch-1" });
        if (value.includes("/api/v1/tags")) return jsonResponse([{ name: "tld", description: "", domain_count: 3 }]);
        if (value.includes("/api/v1/jobs")) return jsonResponse({ items: [], total: 0 });
        return jsonResponse({ items: [], total: 0 });
      });

      const { unmount } = render(App);
      await openTagsTab();
      await fireEvent.click(await screen.findByText("tld"));
      await waitFor(() => screen.getByText("Run all domains"));
      await fireEvent.click(screen.getByText("Run all domains"));

      await waitFor(() => {
        const batchCall = calls.find((c) => c.url.includes("/api/v1/jobs/batch") && c.method === "POST");
        expect(batchCall).toBeTruthy();
        expect(batchCall.body).toContain("from_tag");
      });
      unmount();
    });

    it("delete tag button shows confirmation then calls DELETE", async () => {
      const calls = [];
      global.fetch.mockImplementation((url, opts) => {
        const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
        calls.push({ url: value, method: opts?.method });
        if (value.includes("/summary")) return jsonResponse({ tag: "t", domain_count: 0, ok: 0, notice: 0, warning: 0, error: 0, critical: 0 });
        if (value.includes("/domains") && value.includes("/tags/")) return jsonResponse({ items: [], total: 0 });
        if (opts?.method === "DELETE" && value.includes("/api/v1/tags/")) return { ok: true, status: 204, headers: { get: () => null }, json: async () => ({}), text: async () => "" };
        if (value.includes("/api/v1/tags")) return jsonResponse([{ name: "del-me", description: "", domain_count: 0 }]);
        return jsonResponse({ items: [], total: 0 });
      });

      const { unmount } = render(App);
      await openTagsTab();
      await fireEvent.click(await screen.findByText("del-me"));
      await waitFor(() => screen.getByText("Delete tag"));

      await fireEvent.click(screen.getByText("Delete tag"));
      await waitFor(() => screen.getByText("Confirm delete"));

      await fireEvent.click(screen.getByText("Confirm delete"));
      await waitFor(() => {
        expect(calls.some((c) => c.method === "DELETE" && c.url.includes("/api/v1/tags/del-me"))).toBe(true);
      });
      unmount();
    });

    it("add domains calls POST tags/{name}/domains", async () => {
      const calls = [];
      global.fetch.mockImplementation((url, opts) => {
        const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
        calls.push({ url: value, method: opts?.method });
        if (value.includes("/summary")) return jsonResponse({ tag: "t", domain_count: 0, ok: 0, notice: 0, warning: 0, error: 0, critical: 0 });
        if (value.includes("/domains") && value.includes("/tags/") && opts?.method === "POST") return { ok: true, status: 204, headers: { get: () => null }, json: async () => ({}), text: async () => "" };
        if (value.includes("/domains") && value.includes("/tags/")) return jsonResponse({ items: [], total: 0 });
        if (value.includes("/api/v1/tags")) return jsonResponse([{ name: "mytag", description: "", domain_count: 0 }]);
        return jsonResponse({ items: [], total: 0 });
      });

      const { unmount } = render(App);
      await openTagsTab();
      await fireEvent.click(await screen.findByText("mytag"));
      await waitFor(() => screen.getByText("Add Domains"));

      const textareas = screen.getAllByRole("textbox");
      const addArea = textareas.find((el) => el.placeholder?.includes("example.com"));
      await fireEvent.input(addArea, { target: { value: "new.example" } });
      await fireEvent.click(screen.getByText("Add"));

      await waitFor(() => {
        expect(calls.some((c) => c.url.includes("/tags/mytag/domains") && c.method === "POST")).toBe(true);
      });
      unmount();
    });
  });

  describe("Run Inspector", () => {
    it("shows Run Inspector heading when viewing a completed job", async () => {
      const job = { id: "run-done", domain: "example.com", status: "succeeded", created_at: "2026-01-01T00:00:00Z", progress: 100 };
      global.fetch.mockImplementation((url) => {
        const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
        if (value.includes(`/api/v1/jobs/${job.id}/result`)) return jsonResponse({ job_id: job.id, status: "succeeded", summary: {}, raw: { entries: [] } });
        if (value.includes(`/api/v1/jobs/${job.id}`)) return jsonResponse(job);
        if (value.includes(`/api/v1/runs/${job.id}`)) return jsonResponse({ id: job.id, domain: "example.com", status: "succeeded", duration_ms: 3200, entry_count: 12, worst_level: "WARNING" });
        return jsonResponse({ items: [], total: 0 });
      });

      const { unmount } = render(App);

      const jobIdInput = await screen.findByPlaceholderText("job_123");
      await fireEvent.input(jobIdInput, { target: { value: job.id } });
      await fireEvent.change(jobIdInput);

      await waitFor(() => {
        expect(screen.getByRole("heading", { name: "Run Inspector" })).toBeInTheDocument();
      });
      unmount();
    });

    it("shows run metadata (duration, entry count, worst level) when run is loaded", async () => {
      const job = { id: "run-meta", domain: "meta.example.com", status: "succeeded", created_at: "2026-01-01T00:00:00Z", progress: 100 };
      global.fetch.mockImplementation((url) => {
        const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
        if (value.includes(`/api/v1/jobs/${job.id}/result`)) return jsonResponse({ job_id: job.id, status: "succeeded", summary: {}, raw: { entries: [] } });
        if (value.includes(`/api/v1/jobs/${job.id}`)) return jsonResponse(job);
        if (value.includes(`/api/v1/runs/${job.id}`)) return jsonResponse({ id: job.id, domain: "meta.example.com", status: "succeeded", duration_ms: 4500, entry_count: 27, worst_level: "ERROR" });
        return jsonResponse({ items: [], total: 0 });
      });

      const { unmount } = render(App);

      const jobIdInput = await screen.findByPlaceholderText("job_123");
      await fireEvent.input(jobIdInput, { target: { value: job.id } });
      await fireEvent.change(jobIdInput);

      await waitFor(() => {
        expect(screen.getByText("4500 ms")).toBeInTheDocument();
        expect(screen.getByText("27")).toBeInTheDocument();
        expect(screen.getByText("ERROR")).toBeInTheDocument();
      });
      unmount();
    });

    it("shows the effective profile as a collapsible JSON block for completed runs", async () => {
      const job = { id: "run-effective-profile", domain: "effective.example", status: "succeeded", created_at: "2026-01-01T00:00:00Z", progress: 100 };
      global.fetch.mockImplementation((url) => {
        const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
        if (value.includes(`/api/v1/jobs/${job.id}/result`)) {
          return jsonResponse({ job_id: job.id, status: "succeeded", summary: {}, raw: { entries: [] } });
        }
        if (value.includes(`/api/v1/jobs/${job.id}`)) {
          return jsonResponse(job);
        }
        if (value.includes(`/api/v1/runs/${job.id}`)) {
          return jsonResponse({
            id: job.id,
            domain: "effective.example",
            status: "succeeded",
            duration_ms: 2100,
            entry_count: 6,
            worst_level: "WARNING",
            effective_profile: JSON.stringify({
              net: { ipv6: false },
              resolver: { defaults: { timeout: 5 } }
            })
          });
        }
        return jsonResponse({ items: [], total: 0 });
      });

      const { unmount } = render(App);

      const jobIdInput = await screen.findByPlaceholderText("job_123");
      await fireEvent.input(jobIdInput, { target: { value: job.id } });
      await fireEvent.change(jobIdInput);

      const summary = await screen.findByText("Effective profile");
      await fireEvent.click(summary);

      await waitFor(() => {
        expect(screen.getByText(/"ipv6": false/)).toBeInTheDocument();
        expect(screen.getByText(/"timeout": 5/)).toBeInTheDocument();
      });
      unmount();
    });

    it("domain link in inspector navigates to domains tab on click", async () => {
      const job = { id: "run-domlink", domain: "nav.example.com", status: "succeeded", created_at: "2026-01-01T00:00:00Z", progress: 100 };
      global.fetch.mockImplementation((url) => {
        const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
        if (value.includes(`/api/v1/jobs/${job.id}/result`)) return jsonResponse({ job_id: job.id, status: "succeeded", summary: {}, raw: { entries: [] } });
        if (value.includes(`/api/v1/jobs/${job.id}`)) return jsonResponse(job);
        if (value.includes(`/api/v1/runs/${job.id}`)) return jsonResponse({ id: job.id, domain: "nav.example.com", status: "succeeded", duration_ms: 100, entry_count: 1, worst_level: "" });
        if (value.includes("/api/v1/domains") && value.includes("nav.example.com")) return jsonResponse({ items: [{ id: 99, name: "nav.example.com", tags: [], latest_level: "", run_count: 1 }], total: 1 });
        if (value.includes(`/api/v1/domains/99/runs`)) return jsonResponse({ items: [], total: 0 });
        return jsonResponse({ items: [], total: 0 });
      });

      const { unmount } = render(App);

      const jobIdInput = await screen.findByPlaceholderText("job_123");
      await fireEvent.input(jobIdInput, { target: { value: job.id } });
      await fireEvent.change(jobIdInput);

      await waitFor(() => screen.getByText("nav.example.com"));

      const domainBtn = screen.getAllByText("nav.example.com").find((el) => el.tagName === "BUTTON");
      expect(domainBtn).toBeTruthy();
      await fireEvent.click(domainBtn);

      await waitFor(() => {
        expect(screen.getByRole("tab", { name: "Domains" })).toHaveAttribute("aria-selected", "true");
      });
      unmount();
    });
  });

  describe("Server Settings", () => {
    const sampleSettings = () => ({
      listen_addr: { value: "127.0.0.1:8080", source: "default", readonly: true },
      db_driver: { value: "", source: "default", readonly: true },
      db_dsn: { value: "", source: "default", readonly: true },
      profile_path: { value: "", source: "default", readonly: true },
      worker_count: { value: 4, source: "default" },
      max_concurrent_jobs: { value: 0, source: "default" },
      min_level: { value: "INFO", source: "config_file" },
      retention_days: { value: 0, source: "default" },
      public_url: { value: "", source: "default" },
      rate_limit_enabled: { value: false, source: "default" },
      rate_limit_max: { value: 10, source: "default" },
      rate_limit_window: { value: "10m0s", source: "default" },
      show_score_admin: { value: true, source: "default" },
    });

    const settingsMock = (url, options = {}) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      if (value.includes("/api/v1/settings") && (!options.method || options.method === "GET")) {
        return jsonResponse(sampleSettings());
      }
      if (value.includes("/api/v1/settings") && options.method === "PUT") {
        return jsonResponse({ status: "ok" });
      }
      if (value.includes("/api/v1/profiles/default")) {
        return jsonResponse({ id: 0, name: "default", config: {}, public: false, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" });
      }
      if (value.includes("/api/v1/profiles")) {
        return jsonResponse([]);
      }
      if (value.includes("/api/v1/tags")) {
        return jsonResponse([]);
      }
      return jsonResponse({});
    };

    it("renders server settings with labels and values after loading", async () => {
      global.fetch.mockImplementation(settingsMock);
      const { unmount } = render(App);
      await openSettingsTab();

      await waitFor(() => {
        expect(screen.getByText("Server Settings")).toBeInTheDocument();
      });
      await waitFor(() => {
        expect(screen.getByLabelText(/Worker count/)).toBeInTheDocument();
      });

      const workerInput = screen.getByLabelText(/Worker count/);
      expect(workerInput.value).toBe("4");
      expect(workerInput.disabled).toBe(false);

      const listenInput = screen.getByLabelText(/Listen address/);
      expect(listenInput.value).toBe("127.0.0.1:8080");
      expect(listenInput.disabled).toBe(true);

      unmount();
    });

    it("shows source labels for settings", async () => {
      global.fetch.mockImplementation(settingsMock);
      const { unmount } = render(App);
      await openSettingsTab();

      await waitFor(() => {
        expect(screen.getByLabelText(/Worker count/)).toBeInTheDocument();
      });

      expect(screen.getAllByText("(default)", { exact: false }).length).toBeGreaterThan(0);
      expect(screen.getAllByText("(config file)", { exact: false }).length).toBeGreaterThan(0);

      unmount();
    });

    it("disables save button when no changes are made", async () => {
      global.fetch.mockImplementation(settingsMock);
      const { unmount } = render(App);
      await openSettingsTab();

      await waitFor(() => {
        expect(screen.getByLabelText(/Worker count/)).toBeInTheDocument();
      });

      const saveButton = screen.getByRole("button", { name: "Save changes" });
      expect(saveButton.disabled).toBe(true);

      unmount();
    });

    it("enables save button after editing a mutable setting", async () => {
      global.fetch.mockImplementation(settingsMock);
      const { unmount } = render(App);
      await openSettingsTab();

      await waitFor(() => {
        expect(screen.getByLabelText(/Worker count/)).toBeInTheDocument();
      });

      const workerInput = screen.getByLabelText(/Worker count/);
      await fireEvent.input(workerInput, { target: { value: "8" } });

      const saveButton = screen.getByRole("button", { name: "Save changes" });
      expect(saveButton.disabled).toBe(false);

      unmount();
    });

    it("sends PUT request with changed values on save", async () => {
      const calls = [];
      global.fetch.mockImplementation((url, options = {}) => {
        const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
        if (options.method === "PUT") calls.push({ url: value, body: JSON.parse(options.body) });
        return settingsMock(url, options);
      });

      const { unmount } = render(App);
      await openSettingsTab();

      await waitFor(() => {
        expect(screen.getByLabelText(/Worker count/)).toBeInTheDocument();
      });

      const workerInput = screen.getByLabelText(/Worker count/);
      await fireEvent.input(workerInput, { target: { value: "8" } });
      await fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

      await waitFor(() => {
        expect(calls.length).toBe(1);
      });
      expect(calls[0].body.worker_count).toBe(8);

      unmount();
    });

    it("shows success toast after saving settings", async () => {
      global.fetch.mockImplementation(settingsMock);
      const { unmount } = render(App);
      await openSettingsTab();

      await waitFor(() => {
        expect(screen.getByLabelText(/Worker count/)).toBeInTheDocument();
      });

      await fireEvent.input(screen.getByLabelText(/Worker count/), { target: { value: "8" } });
      await fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

      await waitFor(() => {
        expect(screen.getByText("Settings saved.")).toBeInTheDocument();
      });

      unmount();
    });

    it("shows error when settings fail to load", async () => {
      global.fetch.mockImplementation((url, options = {}) => {
        const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
        if (value.includes("/api/v1/settings")) {
          return jsonResponse({ error: { message: "db connection lost" } }, false);
        }
        return settingsMock(url, options);
      });

      const { unmount } = render(App);
      await openSettingsTab();

      await waitFor(() => {
        expect(screen.getByText(/Failed to load settings/)).toBeInTheDocument();
      });

      unmount();
    });

    it("renders readonly settings as disabled inputs", async () => {
      global.fetch.mockImplementation(settingsMock);
      const { unmount } = render(App);
      await openSettingsTab();

      await waitFor(() => {
        expect(screen.getByLabelText(/Database driver/)).toBeInTheDocument();
      });

      expect(screen.getByLabelText(/Database driver/).disabled).toBe(true);
      expect(screen.getByLabelText(/Database DSN/).disabled).toBe(true);
      expect(screen.getByLabelText(/Profile path/).disabled).toBe(true);

      unmount();
    });

    it("renders toggle inputs for boolean settings", async () => {
      global.fetch.mockImplementation(settingsMock);
      const { unmount } = render(App);
      await openSettingsTab();

      await waitFor(() => {
        expect(screen.getByLabelText(/Rate limiting/)).toBeInTheDocument();
      });

      const toggle = screen.getByLabelText(/Rate limiting/);
      expect(toggle.type).toBe("checkbox");
      expect(toggle.checked).toBe(false);

      unmount();
    });
  });
});
