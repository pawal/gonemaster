import { render, screen, fireEvent, waitFor, cleanup } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App.svelte";

/** A fetch mock that dispatches different responses by URL pattern. */
const fetchRouter = (routes) => {
  global.fetch = vi.fn().mockImplementation(async (url) => {
    for (const [pat, resp] of routes) {
      if (url.includes(pat)) return resp;
    }
    return { ok: true, json: async () => [] };
  });
};

const localesResp = { ok: true, json: async () => ({ locales: ["en"] }) };
const multiLocalesResp = { ok: true, json: async () => ({ locales: ["en", "sv", "da"] }) };

const jobResp = (status, domain = "example.com", progress = 0, finished_at = null) => ({
  ok: true,
  status: 200,
  json: async () => ({ public_id: "abc12345", domain, status, progress, finished_at }),
});

const resultResp = (entries = []) => ({
  ok: true,
  status: 200,
  json: async () => ({ job_id: "x", status: "succeeded", raw: { locale: "en", entries } }),
});

describe("App", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    window.location.hash = "";
    fetchRouter([
      ["/locales", localesResp],
      ["/jobs/", jobResp("queued", "example.com", 0)],
    ]);
  });

  afterEach(() => {
    cleanup();
    document.documentElement.removeAttribute("data-theme");
  });

  // ── Layout ──────────────────────────────────────────────────────────────────

  it("always renders the test form", () => {
    render(App);
    expect(screen.getByTestId("test-form")).toBeTruthy();
  });

  it("shows no Progress or Results on initial load", () => {
    render(App);
    expect(document.querySelector("[data-testid='progress-view']")).toBeNull();
    expect(document.querySelector("[data-testid='results-view']")).toBeNull();
  });

  // ── Shared-link init ────────────────────────────────────────────────────────

  it("shows Progress when hash is a running job on load", async () => {
    window.location.hash = "#/result/abc12345";
    fetchRouter([
      ["/locales", localesResp],
      ["/jobs/abc12345", jobResp("running", "example.com", 30)],
    ]);
    render(App);
    await waitFor(() =>
      expect(document.querySelector("[data-testid='progress-view']")).not.toBeNull()
    );
  });

  it("shows Results when hash is a succeeded job on load", async () => {
    window.location.hash = "#/result/abc12345";
    fetchRouter([
      ["/locales", localesResp],
      ["jobs/abc12345/result", resultResp()],
      ["/jobs/", jobResp("succeeded", "example.com", 100)],
    ]);
    render(App);
    await waitFor(() =>
      expect(screen.getByTestId("results-view")).toBeTruthy()
    );
  });

  it("shows ExpiredResult when hash job returns 404 on load", async () => {
    window.location.hash = "#/result/abc12345";
    fetchRouter([
      ["/locales", localesResp],
      ["/jobs/", { ok: false, status: 404, json: async () => ({}) }],
    ]);
    render(App);
    await waitFor(() =>
      expect(screen.getByTestId("expired-view")).toBeTruthy()
    );
  });

  // ── Form disabled state ─────────────────────────────────────────────────────

  it("form is enabled while idle", () => {
    render(App);
    expect(screen.getByLabelText("Domain").disabled).toBe(false);
  });

  it("form is disabled while test is running", async () => {
    window.location.hash = "#/result/abc12345";
    fetchRouter([
      ["/locales", localesResp],
      ["/jobs/abc12345", jobResp("running", "example.com", 30)],
    ]);
    render(App);
    await waitFor(() =>
      expect(screen.getByLabelText("Domain").disabled).toBe(true)
    );
  });

  // ── Post-job flow ───────────────────────────────────────────────────────────

  it("shows Results after job succeeds", async () => {
    window.location.hash = "#/result/abc12345";
    fetchRouter([
      ["/locales", localesResp],
      ["jobs/abc12345/result", resultResp()],
      ["/jobs/", jobResp("succeeded", "example.com", 100)],
    ]);
    render(App);
    await waitFor(() => screen.getByTestId("results-view"));
  });

  it("shows ExpiredResult when job is expired", async () => {
    window.location.hash = "#/result/abc12345";
    fetchRouter([
      ["/locales", localesResp],
      ["/jobs/", { ok: false, status: 404, json: async () => ({}) }],
    ]);
    render(App);
    await waitFor(() => screen.getByTestId("expired-view"));
  });

  // ── Header ──────────────────────────────────────────────────────────────────

  it("renders the app title", () => {
    render(App);
    expect(screen.getByText("Gonemaster")).toBeTruthy();
  });

  it("renders the locale selector when multiple locales are available", async () => {
    fetchRouter([
      ["/locales", multiLocalesResp],
      ["/jobs/", jobResp("queued", "example.com", 0)],
    ]);
    render(App);
    await waitFor(() =>
      expect(screen.getByRole("combobox", { name: /language/i })).toBeTruthy()
    );
  });

  it("hides locale selector when only one locale is available", async () => {
    render(App);
    await new Promise((r) => setTimeout(r, 50));
    expect(screen.queryByRole("combobox", { name: /language/i })).toBeNull();
  });

  it("renders the theme toggle button", () => {
    render(App);
    expect(screen.getByRole("button", { name: /theme/i })).toBeTruthy();
  });

  // ── Theme toggle ───────────────────────────────────────────────────────────
  // jsdom has no matchMedia, so isDark initialises to false → data-theme="light"

  it("starts with data-theme=light in test env (no system dark preference)", () => {
    render(App);
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
  });

  it("sets data-theme=dark after one click", async () => {
    render(App);
    await fireEvent.click(screen.getByRole("button", { name: /theme/i }));
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  });

  it("sets data-theme=light after two clicks", async () => {
    render(App);
    const btn = screen.getByRole("button", { name: /theme/i });
    await fireEvent.click(btn);
    await fireEvent.click(btn);
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
  });
});
