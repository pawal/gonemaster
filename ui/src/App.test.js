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

  const openSettingsTab = async (subTab = null) => {
    await fireEvent.click(screen.getByRole("tab", { name: "Settings" }));
    if (subTab) {
      await fireEvent.click(await screen.findByRole("tab", { name: subTab }));
    }
  };

  const getMetricsPanel = () => screen.getByRole("tabpanel", { name: "Metrics" });
  const tableColumnValues = (table, columnIndex = 0) =>
    Array.from(table.querySelectorAll("tbody tr")).map((row) =>
      row.querySelectorAll("td")[columnIndex].textContent.replace(/\s+/g, " ").trim()
    );

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

    await openSettingsTab("Profiles");
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
    await openRecentTab();
    await waitFor(() => {
      expect(calls.some((value) => value.includes("/api/v1/jobs?"))).toBe(true);
    });

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

  it("stops auto-refresh once a watched single job completes", async () => {
    const intervalCallbacks = [];
    vi.spyOn(global, "setInterval").mockImplementation((fn) => {
      intervalCallbacks.push(fn);
      return intervalCallbacks.length;
    });
    vi.spyOn(global, "clearInterval").mockImplementation(() => {});

    let jobCallCount = 0;
    const queuedJob = {
      id: "job_autostop",
      domain: "autostop.example",
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

    const input = await screen.findByPlaceholderText("example.com");
    await fireEvent.input(input, { target: { value: "autostop.example" } });
    await fireEvent.click(screen.getByText("Run Single Job"));

    // Wait for the job to be created and watched, with auto-refresh on and a
    // poller scheduled.
    await waitFor(() => {
      expect(screen.getByText(/Created job:/)).toBeInTheDocument();
    });
    expect(screen.getByText("Auto refresh: on")).toBeInTheDocument();
    expect(intervalCallbacks.length).toBeGreaterThan(0);

    // The poller fetches the job again; it is now complete.
    const poller = intervalCallbacks[intervalCallbacks.length - 1];
    await poller();

    // The auto-stop effect must switch auto-refresh off once the job is done.
    await waitFor(() => {
      expect(screen.getByText("Auto refresh: off")).toBeInTheDocument();
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

    await openSettingsTab("Profiles");
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
    await waitFor(() => {
      expect(calls.some((value) => value.includes("/api/v1/jobs?") && value.includes("sort=domain_desc") && value.includes("batch_id=batch_url"))).toBe(true);
      expect(calls.some((value) => value.includes("/api/v1/jobs?") && value.includes("limit=50") && value.includes("cursor=2"))).toBe(true);
    });
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
    await openRecentTab();

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
    });
    expect(screen.getByLabelText("Sort")).toHaveValue("started_at_desc");
    expect(screen.getByLabelText("Page size")).toHaveValue("20");
    expect(screen.getByLabelText("Batch ID filter")).toHaveValue("batch_from_url");

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

    it("clicking a run row loads the result inline and stays in domain view", async () => {
      global.fetch.mockImplementation((url) => {
        const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
        if (value.includes("/runs")) {
          return jsonResponse({
            items: [
              { id: "run-xyz", finished_at: "2026-03-15T10:00:00Z", worst_level: "ERROR", duration_ms: 800, entry_count: 3 },
              { id: "run-old", finished_at: "2026-03-14T09:00:00Z", worst_level: "WARNING", duration_ms: 600, entry_count: 2 }
            ],
            total: 2
          });
        }
        if (value.includes("/api/v1/domains")) {
          return jsonResponse({ items: [{ id: 2, name: "test.com", tags: [], latest_level: "ERROR", run_count: 2 }], total: 1 });
        }
        if (value.includes("/api/v1/jobs/run-")) return jsonResponse({ id: "run-old", status: "succeeded", domain: "test.com" });
        if (value.includes("/api/v1/tags")) return jsonResponse([]);
        return jsonResponse({ items: [], total: 0 });
      });

      const { unmount } = render(App);
      await openDomainsTab();
      await fireEvent.click(await screen.findByText("test.com"));
      const olderRow = await screen.findByText("run-old");
      await fireEvent.click(olderRow);

      await waitFor(() => {
        expect(screen.getByRole("tab", { name: "Domains" })).toHaveAttribute("aria-selected", "true");
        expect(screen.getByRole("tab", { name: "Single Job" })).toHaveAttribute("aria-selected", "false");
        expect(olderRow.closest("tr")).toHaveClass("run-row-selected");
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
        if (value.match(/\/tags\/[^/]+\/batches/)) return jsonResponse({ items: [], total: 0 });
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

    describe("Earlier batches panel", () => {
      const mockTagBatchesFetch = (batches = [], extra = {}) => {
        const calls = [];
        global.fetch.mockImplementation((url, opts) => {
          const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
          const method = opts?.method || "GET";
          calls.push({ url: value, method });
          if (value.includes("/summary")) {
            return jsonResponse({ tag: "tld", domain_count: 0, ok: 0, notice: 0, warning: 0, error: 0, critical: 0 });
          }
          if (value.match(/\/tags\/[^/]+\/batches/)) {
            return jsonResponse({ items: batches, total: batches.length });
          }
          if (value.includes("/domains") && value.includes("/tags/")) {
            return jsonResponse({ items: [], total: 0 });
          }
          if (value.match(/\/batches\/[^/]+\/delete-preview/)) {
            return jsonResponse(extra.preview || {
              batch_id: "batch_a",
              tag: "tld",
              exists: true,
              queued_jobs: 0,
              running_jobs: 0,
              completed_runs: 3,
              entries: 50,
              fact_rows: 20,
              snapshots: [],
            });
          }
          if (value.match(/\/batches\/[^/]+$/) && method === "GET") {
            return jsonResponse({
              batch_id: "batch_a",
              total: 1,
              status_counts: { succeeded: 1 },
              items: [{ id: "job_a", domain: "a.example", status: "succeeded", created_at: "2026-04-10T12:00:00Z", progress: 100 }],
              created_at: "2026-04-10T12:00:00Z",
            });
          }
          if (value.includes("/api/v1/tags") && opts?.method !== "POST") {
            return jsonResponse([{ name: "tld", description: "", domain_count: 2 }]);
          }
          return jsonResponse({ items: [], total: 0 });
        });
        return calls;
      };

      it("row click navigates to the batches tab with the batch pre-loaded", async () => {
        mockTagBatchesFetch([
          { id: "batch_a", tag: "tld", created_at: "2026-04-10T12:00:00Z", domain_count: 3, snapshot_intent: false },
        ]);

        const { unmount } = render(App);
        await openTagsTab();
        await fireEvent.click(await screen.findByText("tld"));

        const row = (await screen.findByText("batch_a")).closest("tr");
        await fireEvent.click(row);

        await waitFor(() => {
          expect(screen.getByRole("tab", { name: "Batch Jobs", selected: true })).toBeInTheDocument();
        });
        const input = await screen.findByLabelText("Batch ID");
        expect(input.value).toBe("batch_a");
        unmount();
      });
    });
  });


  describe("Settings sub-tabs", () => {
    const subtabMock = (url, options = {}) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      if (value.includes("/api/v1/settings")) return jsonResponse({});
      if (value.includes("/api/v1/scoring-config/defaults")) return jsonResponse({});
      if (value.includes("/api/v1/scoring-config")) return jsonResponse({
        config: {
          severity_penalties: { NOTICE: 1, WARNING: 5, ERROR: 20, CRITICAL: 0 },
          category_weights: { dnssec: 1.5 },
          module_categories: {},
          tag_penalties: {},
          grade_bands: [{ grade: "A", min_score: 90 }],
          bonus_criteria: { no_warnings_or_errors: true, dnssec_enabled: true, strong_algorithm: true, nsec3_non_optout: true, cds_cdnskey_published: true, ipv6_all_nameservers: true, as_diversity: true },
        },
        source: "default",
        readonly: false,
      });
      if (value.includes("/api/v1/profiles/default")) {
        return jsonResponse({
          id: 0, name: "default", config: {}, public: false,
          created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
        });
      }
      if (value.includes("/api/v1/profiles")) return jsonResponse([]);
      if (value.includes("/api/v1/tags")) return jsonResponse([]);
      if (value.includes("/api/v1/analysis/cohorts")) return jsonResponse([]);
      return jsonResponse({});
    };

    it("defaults to System sub-tab and renders ServerSettings", async () => {
      global.fetch.mockImplementation(subtabMock);
      const { unmount } = render(App);
      await openSettingsTab();

      const systemTab = screen.getByRole("tab", { name: "System" });
      expect(systemTab.getAttribute("aria-selected")).toBe("true");
      await waitFor(() => {
        expect(screen.getByRole("heading", { name: "Server Settings" })).toBeInTheDocument();
      });
      expect(screen.queryByRole("heading", { name: "Profile Library" })).toBeNull();
      expect(screen.queryByRole("heading", { name: "Analysis Cohorts" })).toBeNull();
      unmount();
    });

    it("switches to Profiles sub-tab and renders ProfileSettings", async () => {
      global.fetch.mockImplementation(subtabMock);
      const { unmount } = render(App);
      await openSettingsTab("Profiles");

      await waitFor(() => {
        expect(screen.getByText("Profile Library")).toBeInTheDocument();
      });
      expect(screen.queryByRole("heading", { name: "Server Settings" })).toBeNull();
      unmount();
    });

    it("switches to Scoring sub-tab and renders ScoringSettings", async () => {
      global.fetch.mockImplementation(subtabMock);
      const { unmount } = render(App);
      await openSettingsTab("Scoring");

      await waitFor(() => {
        expect(screen.getByRole("heading", { name: "Scoring" })).toBeInTheDocument();
      });
      await waitFor(() => {
        expect(screen.getByText("Severity Penalties")).toBeInTheDocument();
      });
      unmount();
    });

    it("pushes sub-tab changes to history and restores them on popstate", async () => {
      global.fetch.mockImplementation(subtabMock);
      const { unmount } = render(App);

      await openSettingsTab();
      expect(window.location.hash).toBe("#/settings");

      await fireEvent.click(await screen.findByRole("tab", { name: "Profiles" }));
      await waitFor(() => expect(window.location.hash).toBe("#/settings/profiles"));

      await fireEvent.click(await screen.findByRole("tab", { name: "Scoring" }));
      await waitFor(() => expect(window.location.hash).toBe("#/settings/scoring"));

      window.history.back();
      await waitFor(() => {
        expect(screen.getByRole("tab", { name: "Profiles" }).getAttribute("aria-selected")).toBe("true");
      });
      expect(window.location.hash).toBe("#/settings/profiles");

      window.history.back();
      await waitFor(() => {
        expect(screen.getByRole("tab", { name: "System" }).getAttribute("aria-selected")).toBe("true");
      });
      expect(window.location.hash).toBe("#/settings");

      unmount();
    });
  });

  // Regression tests: both entry points into per-tab data loading
  // (setTab on click, initializeApp on mount) must go through the shared
  // loadDataForTab helper so they can't drift. We previously had a bug
  // where /analysis/cohorts loaded on tab-click but not on page reload.
  describe("per-tab data loading parity", () => {
    const setupTagsFetchTracking = () => {
      const calls = [];
      global.fetch.mockImplementation((url) => {
        const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
        calls.push(value);
        if (value === "/api/v1/analysis/cohorts") {
          return jsonResponse([{ id: 1, source_tag: "tld", label: "TLD", analysis_enabled: true, public_enabled: true }]);
        }
        if (value.includes("/api/v1/tags")) {
          return jsonResponse([{ name: "tld", description: "TLDs cohort source", domain_count: 3, default_profile_id: null }]);
        }
        return jsonResponse({});
      });
      return calls;
    };

    it("fetches /analysis/cohorts on page reload with #/tags", async () => {
      window.history.replaceState(null, "", "/#/tags");
      const calls = setupTagsFetchTracking();

      const { unmount } = render(App);

      await waitFor(() => {
        expect(calls).toContain("/api/v1/analysis/cohorts");
      });
      unmount();
    });

    it("fetches /analysis/cohorts when clicking the Tags tab", async () => {
      const calls = setupTagsFetchTracking();

      const { unmount } = render(App);
      await fireEvent.click(screen.getByRole("tab", { name: "Tags" }));

      await waitFor(() => {
        expect(calls).toContain("/api/v1/analysis/cohorts");
      });
      unmount();
    });

    it("shows the cohort label in the tags list for tags used as cohort source", async () => {
      setupTagsFetchTracking();

      const { container, unmount } = render(App);
      await fireEvent.click(screen.getByRole("tab", { name: "Tags" }));

      await waitFor(() => {
        const chip = container.querySelector(".tag-cohort-chip");
        expect(chip).not.toBeNull();
        expect(chip.textContent.trim()).toBe("TLD");
      });
      unmount();
    });
  });

  // Routing: in-panel drill-downs now push a hash route, so the browser Back
  // button returns from a detail view to its list, and switching tabs resets
  // the drill-down instead of silently re-entering it.
  describe("hash routing and back-button navigation", () => {
    const domainsMock = (url) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      if (value.includes("/runs")) return jsonResponse({ items: [], total: 0 });
      if (value.includes("/api/v1/domains")) {
        return jsonResponse({
          items: [{ id: 7, name: "example.com", tags: [], latest_level: "WARNING", latest_run_at: "2026-03-15T10:00:00Z", run_count: 1 }],
          total: 1,
        });
      }
      if (value.includes("/api/v1/tags")) return jsonResponse([]);
      return jsonResponse({ items: [], total: 0 });
    };

    it("clicking a domain row pushes a hash route and Back returns to the list", async () => {
      global.fetch.mockImplementation(domainsMock);
      const { unmount } = render(App);

      await fireEvent.click(screen.getByRole("tab", { name: "Domains" }));
      await fireEvent.click(await screen.findByText("example.com"));

      await waitFor(() => {
        expect(window.location.hash).toBe("#/domains/example.com");
        expect(screen.getByText("← Back to domains")).toBeInTheDocument();
      });

      window.history.back();

      await waitFor(() => {
        expect(window.location.hash).toBe("#/domains");
        expect(screen.queryByText("← Back to domains")).not.toBeInTheDocument();
        expect(screen.getByRole("heading", { name: "Domains" })).toBeInTheDocument();
      });
      unmount();
    });

    it("switching to another tab clears an open domain detail", async () => {
      global.fetch.mockImplementation(domainsMock);
      const { unmount } = render(App);

      await fireEvent.click(screen.getByRole("tab", { name: "Domains" }));
      await fireEvent.click(await screen.findByText("example.com"));
      await waitFor(() => expect(screen.getByText("← Back to domains")).toBeInTheDocument());

      await fireEvent.click(screen.getByRole("tab", { name: "Recent Tests" }));
      await fireEvent.click(screen.getByRole("tab", { name: "Domains" }));

      await waitFor(() => {
        expect(window.location.hash).toBe("#/domains");
        expect(screen.queryByText("← Back to domains")).not.toBeInTheDocument();
        expect(screen.getByRole("heading", { name: "Domains" })).toBeInTheDocument();
      });
      unmount();
    });

    it("clicking a tag row pushes a hash route and Back returns to the list", async () => {
      global.fetch.mockImplementation((url) => {
        const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
        if (value.includes("/summary")) return jsonResponse({ ok: 0, notice: 0, warning: 0, error: 0, critical: 0 });
        if (value.match(/\/tags\/[^/]+\/domains/)) return jsonResponse({ items: [], total: 0 });
        if (value.match(/\/tags\/[^/]+\/batches/)) return jsonResponse({ items: [], total: 0 });
        if (value.includes("/api/v1/analysis/cohorts")) return jsonResponse([]);
        if (value.includes("/api/v1/tags")) return jsonResponse([{ name: "tld", description: "TLD", domain_count: 2, default_profile_id: null }]);
        return jsonResponse({ items: [], total: 0 });
      });
      const { unmount } = render(App);

      await fireEvent.click(screen.getByRole("tab", { name: "Tags" }));
      await fireEvent.click(await screen.findByText("tld"));

      await waitFor(() => {
        expect(window.location.hash).toBe("#/tags/tld");
        expect(screen.getByText("← Back to tags")).toBeInTheDocument();
      });

      window.history.back();

      await waitFor(() => {
        expect(window.location.hash).toBe("#/tags");
        expect(screen.queryByText("← Back to tags")).not.toBeInTheDocument();
        expect(screen.getByRole("heading", { name: "Tags" })).toBeInTheDocument();
      });
      unmount();
    });

    it("redirects the legacy #/settings/analysis bookmark to the Cohorts tab", async () => {
      window.history.replaceState(null, "", "/#/settings/analysis");
      global.fetch.mockImplementation(() => jsonResponse({ items: [] }));
      const { unmount } = render(App);

      await waitFor(() => {
        expect(screen.getByRole("tab", { name: "Cohorts" })).toHaveAttribute("aria-selected", "true");
        expect(window.location.hash).toBe("#/cohorts");
      });
      unmount();
    });
  });
});
