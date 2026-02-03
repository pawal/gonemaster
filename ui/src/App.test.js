import { render, screen, fireEvent, waitFor } from "@testing-library/svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
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
});
