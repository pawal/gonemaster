import { render, screen, waitFor, cleanup } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import Progress from "./Progress.svelte";

const jobResp = (status, progress = 0, domain = "example.com") => ({
  ok: true,
  status: 200,
  json: async () => ({ public_id: "abc12345", domain, status, progress }),
});

const errResp = (status) => ({
  ok: false,
  status,
  json: async () => ({}),
});

describe("Progress", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    global.fetch = vi.fn();
  });

  afterEach(() => cleanup());

  it("shows progressbar on mount", async () => {
    global.fetch.mockResolvedValue(jobResp("queued", 0));
    render(Progress, { props: { publicID: "abc12345" } });
    await waitFor(() => expect(screen.getByRole("progressbar")).toBeTruthy());
  });

  it("shows domain in progress text", async () => {
    global.fetch.mockResolvedValue(jobResp("running", 30, "dns.example"));
    render(Progress, { props: { publicID: "abc12345" } });
    await waitFor(() =>
      expect(screen.getByText(/dns\.example/)).toBeTruthy()
    );
  });

  it("reflects progress in progressbar aria-valuenow", async () => {
    global.fetch.mockResolvedValue(jobResp("running", 72));
    render(Progress, { props: { publicID: "abc12345" } });
    await waitFor(() =>
      expect(screen.getByRole("progressbar").getAttribute("aria-valuenow")).toBe("72")
    );
  });

  it("dispatches jobdone with status=succeeded", async () => {
    global.fetch.mockResolvedValue(jobResp("succeeded", 100));
    const events = [];
    render(Progress, {
      props: { publicID: "abc12345" },
      events: { jobdone: (e) => events.push(e.detail) },
    });
    await waitFor(() => expect(events.length).toBe(1));
    expect(events[0]).toMatchObject({ publicID: "abc12345", status: "succeeded", domain: "example.com" });
  });

  it("dispatches jobdone with status=failed", async () => {
    global.fetch.mockResolvedValue(jobResp("failed", 0));
    const events = [];
    render(Progress, {
      props: { publicID: "abc12345" },
      events: { jobdone: (e) => events.push(e.detail) },
    });
    await waitFor(() => expect(events.length).toBe(1));
    expect(events[0].status).toBe("failed");
  });

  it("dispatches jobdone with status=expired on 404", async () => {
    global.fetch.mockResolvedValue(errResp(404));
    const events = [];
    render(Progress, {
      props: { publicID: "abc12345" },
      events: { jobdone: (e) => events.push(e.detail) },
    });
    await waitFor(() => expect(events.length).toBe(1));
    expect(events[0].status).toBe("expired");
  });

  it("shows error on non-404 server error", async () => {
    global.fetch.mockResolvedValue(errResp(500));
    render(Progress, { props: { publicID: "abc12345" } });
    await waitFor(() => expect(screen.getByRole("alert")).toBeTruthy());
  });

  it("shows error on network failure", async () => {
    global.fetch.mockRejectedValue(new Error("network down"));
    render(Progress, { props: { publicID: "abc12345" } });
    await waitFor(() => expect(screen.getByRole("alert")).toBeTruthy());
  });

  it("polls again after 2s interval", async () => {
    vi.useFakeTimers();
    try {
      global.fetch
        .mockResolvedValueOnce(jobResp("queued", 0))
        .mockResolvedValueOnce(jobResp("running", 50));
      render(Progress, { props: { publicID: "abc12345" } });
      await vi.advanceTimersByTimeAsync(2000);
      expect(fetch).toHaveBeenCalledTimes(2);
    } finally {
      vi.useRealTimers();
    }
  });
});
