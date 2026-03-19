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

const jobResp = (status, domain = "example.com", progress = 0) => ({
  ok: true,
  status: 200,
  json: async () => ({ public_id: "abc12345", domain, status, progress }),
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
    // Default: locales ok, jobs return queued (keeps Progress in-flight)
    fetchRouter([
      ["/locales", localesResp],
      ["/jobs/", jobResp("queued", "example.com", 0)],
    ]);
  });

  afterEach(() => {
    cleanup();
    document.documentElement.removeAttribute("data-theme");
  });

  // ── Routing ────────────────────────────────────────────────────────────────

  it("shows home view by default", () => {
    render(App);
    expect(document.querySelector("[data-view='home']")).not.toBeNull();
    expect(document.querySelector("[data-view='result']")).toBeNull();
  });

  it("shows result view when hash is #/result/:id", () => {
    window.location.hash = "#/result/abc12345";
    render(App);
    expect(document.querySelector("[data-view='result']")).not.toBeNull();
    expect(document.querySelector("[data-view='home']")).toBeNull();
  });

  it("result view carries the publicID as data attribute", () => {
    window.location.hash = "#/result/myid0001";
    fetchRouter([
      ["/locales", localesResp],
      ["/jobs/", jobResp("queued", "example.com", 0)],
    ]);
    render(App);
    const el = document.querySelector("[data-view='result']");
    expect(el?.dataset.publicId).toBe("myid0001");
  });

  it("switches to home view when hashchange fires with #/", async () => {
    window.location.hash = "#/result/abc12345";
    render(App);
    expect(document.querySelector("[data-view='result']")).not.toBeNull();

    window.location.hash = "#/";
    window.dispatchEvent(new Event("hashchange"));
    await new Promise((r) => setTimeout(r, 0));
    expect(document.querySelector("[data-view='home']")).not.toBeNull();
  });

  // ── Header ─────────────────────────────────────────────────────────────────

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

  // ── Component wiring ───────────────────────────────────────────────────────

  it("home view renders TestForm", () => {
    render(App);
    expect(document.querySelector("[data-testid='test-form']")).not.toBeNull();
  });

  it("result view shows Progress initially", () => {
    window.location.hash = "#/result/abc12345";
    render(App);
    expect(document.querySelector("[data-testid='progress-view']")).not.toBeNull();
  });

  it("shows Results and New test button after job succeeds", async () => {
    window.location.hash = "#/result/abc12345";
    fetchRouter([
      ["/locales", localesResp],
      ["jobs/abc12345/result", resultResp()],
      ["/jobs/", jobResp("succeeded", "example.com", 100)],
    ]);
    render(App);
    await waitFor(() => screen.getByTestId("results-view"));
    expect(screen.getByTestId("new-test-link")).toBeTruthy();
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

  it("New test button navigates home after job succeeds", async () => {
    window.location.hash = "#/result/abc12345";
    fetchRouter([
      ["/locales", localesResp],
      ["jobs/abc12345/result", resultResp()],
      ["/jobs/", jobResp("succeeded", "example.com", 100)],
    ]);
    render(App);
    await waitFor(() => screen.getByTestId("new-test-link"));
    await fireEvent.click(screen.getByTestId("new-test-link"));
    await new Promise((r) => setTimeout(r, 0));
    expect(document.querySelector("[data-view='home']")).not.toBeNull();
  });
});
