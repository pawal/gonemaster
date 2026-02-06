import { render, screen, fireEvent, waitFor, within, cleanup } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App.svelte";

const jsonResponse = (data, ok = true) => ({
  ok,
  statusText: ok ? "OK" : "Bad Request",
  headers: {
    get: () => "application/json"
  },
  json: async () => data,
  text: async () => JSON.stringify(data)
});

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

  it("renders the main sections", async () => {
    global.fetch.mockImplementation(() => jsonResponse({ items: [] }));

    const { container, unmount } = render(App);

    expect(await screen.findByText("Gonemaster")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Single Job" })).toBeInTheDocument();
    expect(screen.getByRole("tablist", { name: "Job views" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Single Job" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Recent Tests" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Batch Jobs" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Metrics" })).toBeInTheDocument();
    expect(screen.getByText("Job Inspector")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Recent Tests" })).toBeNull();
    expect(screen.queryByText("Batch Inspector")).not.toBeInTheDocument();
    expect(container.querySelector("nav[role='tablist']")).toBeNull();
    expect(container.querySelector("section[role='tabpanel']")).toBeNull();

    await openBatchTab();
    expect(screen.getByText("Batch Inspector")).toBeInTheDocument();

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
      expect(screen.getByText("example.com - succeeded")).toBeInTheDocument();
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

    const button = screen.getByText("Run Single Job");
    await fireEvent.click(button);

    await waitFor(() => {
      expect(screen.getByText(/Created job:/)).toBeInTheDocument();
      expect(screen.getAllByText("job_1").length).toBeGreaterThan(0);
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
      created_at: "2026-02-03T00:00:00Z"
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
      expect(screen.queryByText("job_clean")).toBeNull();
      expect(screen.getByText("job_warn")).toBeInTheDocument();
      expect(screen.getByText("job_err")).toBeInTheDocument();
    });

    await fireEvent.click(screen.getByRole("button", { name: "Errors only" }));
    await waitFor(() => {
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
    await fireEvent.click(within(row).getByRole("button", { name: "Inspect" }));

    await waitFor(() => {
      expect(calls.some((value) => value === `/api/v1/jobs/${job.id}`)).toBe(true);
      expect(screen.getByRole("tab", { name: "Single Job" })).toHaveAttribute("aria-selected", "true");
      expect(screen.getByLabelText("Job ID")).toHaveValue(job.id);
    });

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
});
