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
      if (url === `/jobs/${job.id}/result`) {
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
});
