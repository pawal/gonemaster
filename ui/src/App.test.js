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
    localStorage.clear();
    global.fetch = vi.fn();
  });

  afterEach(() => {
    cleanup();
  });

  it("renders the main sections", async () => {
    global.fetch.mockImplementation(() => jsonResponse({ items: [] }));

    const { unmount } = render(App);

    expect(await screen.findByText("Gonemaster")).toBeInTheDocument();
    expect(screen.getByText("Single Job")).toBeInTheDocument();
    expect(screen.getByText("Batch Jobs")).toBeInTheDocument();
    expect(screen.getByText("Job Inspector")).toBeInTheDocument();
    expect(screen.getByText("Batch Inspector")).toBeInTheDocument();
    expect(screen.getByText("Recent Jobs")).toBeInTheDocument();

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

  it("submits a single job and displays the created id", async () => {
    const job = {
      id: "job_1",
      domain: "example.com",
      status: "queued",
      created_at: "2026-02-03T00:00:00Z",
      progress: 0
    };

    global.fetch.mockImplementation((url, options = {}) => {
      if (url === "/jobs" && options.method === "POST") {
        return jsonResponse(job);
      }
      if (url === `/jobs/${job.id}`) {
        return jsonResponse(job);
      }
      if (typeof url === "string" && url.startsWith("/jobs?")) {
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
      expect(screen.getByText("job_1")).toBeInTheDocument();
    });

    expect(global.fetch).toHaveBeenCalledWith(
      "/jobs",
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
      if (url === "/jobs" && options.method === "POST") {
        return jsonResponse(job);
      }
      if (url === `/jobs/${job.id}`) {
        return jsonResponse(job);
      }
      if (typeof url === "string" && url.startsWith("/jobs?")) {
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
        ([url, options]) => url === "/jobs" && options?.method === "POST"
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
      if (url === "/jobs/batch" && options.method === "POST") {
        return jsonResponse(batch);
      }
      if (url === "/batches/batch_1") {
        return jsonResponse(summary);
      }
      if (typeof url === "string" && url.startsWith("/jobs?")) {
        return jsonResponse({ items: [] });
      }
      return jsonResponse({});
    });

    const { unmount } = render(App);

    const textarea = await screen.findByLabelText("Domains (one per line)");
    await fireEvent.input(textarea, { target: { value: "example.com\nexample.org" } });

    const button = screen.getByText("Run Batch");
    await fireEvent.click(button);

    await waitFor(() => {
      expect(screen.getByText(/Created batch:/)).toBeInTheDocument();
      expect(screen.getByText("batch_1")).toBeInTheDocument();
    });

    expect(global.fetch).toHaveBeenCalledWith(
      "/jobs/batch",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ domains: ["example.com", "example.org"] })
      })
    );

    unmount();
  });

  it("renders progress bars for job inspector and recent jobs", async () => {
    const job = {
      id: "job_a",
      domain: "example.com",
      status: "running",
      created_at: "2026-02-03T00:00:00Z",
      progress: 42
    };

    global.fetch.mockImplementation((url) => {
      if (typeof url === "string" && url.startsWith("/jobs?")) {
        return jsonResponse({ items: [job] });
      }
      if (url === `/jobs/${job.id}`) {
        return jsonResponse(job);
      }
      return jsonResponse({ items: [] });
    });

    const { unmount } = render(App);

    const refresh = await screen.findByRole("button", { name: /refresh list/i });
    if (refresh.disabled) {
      await waitFor(() => expect(refresh).not.toBeDisabled());
    }
    await fireEvent.click(refresh);
    await screen.findByText("job_a");

    const recentCard = screen.getByText("Recent Jobs").closest(".card");
    expect(recentCard).not.toBeNull();
    const recentBars = within(recentCard).getAllByRole("progressbar");
    const hasRecentProgress = recentBars.some((bar) => bar.getAttribute("aria-valuenow") === "42");
    expect(hasRecentProgress).toBe(true);

    const jobInput = screen.getByLabelText("Job ID");
    await fireEvent.input(jobInput, { target: { value: job.id } });
    await fireEvent.change(jobInput);

    const inspectorCard = screen.getByText("Job Inspector").closest(".card");
    expect(inspectorCard).not.toBeNull();
    await waitFor(() => {
      const inspectorBar = within(inspectorCard).getByRole("progressbar");
      expect(inspectorBar).toHaveAttribute("aria-valuenow", "42");
    });

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
      if (typeof url === "string" && url.startsWith("/jobs?")) {
        return jsonResponse({ items: [] });
      }
      if (url === `/jobs/${job.id}`) {
        return jsonResponse(job);
      }
      if (typeof url === "string" && url.startsWith(`/jobs/${job.id}/result`)) {
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
      if (typeof url === "string" && url.startsWith("/jobs?")) {
        return jsonResponse({ items: [] });
      }
      if (url === `/jobs/${job.id}`) {
        return jsonResponse(job);
      }
      if (typeof url === "string" && url.startsWith(`/jobs/${job.id}/result`)) {
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
