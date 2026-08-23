import { render, screen, fireEvent, waitFor } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App.svelte";
import { errorResponse, fetchRouter, jsonResponse } from "./test/helpers.js";

const localesResp = jsonResponse({ locales: ["en"] });
const multiLocalesResp = jsonResponse({ locales: ["en", "sv", "da"] });

const jobResp = (status, domain = "example.com", progress = 0, finished_at = null) =>
  jsonResponse({ public_id: "abc12345", domain, status, progress, finished_at });

const resultResp = (entries = []) =>
  jsonResponse({ job_id: "x", status: "succeeded", raw: { locale: "en", entries } });

describe("App", () => {
  beforeEach(() => {
    window.location.hash = "";
    fetchRouter([
      ["/locales", localesResp],
      ["/jobs/", jobResp("queued", "example.com", 0)],
    ]);
  });

  afterEach(() => {
    document.documentElement.removeAttribute("data-theme");
    window.localStorage.clear();
  });

  // Layout

  it("always renders the test form", () => {
    render(App);
    expect(screen.getByTestId("test-form")).toBeTruthy();
  });

  it("shows no Progress or Results on initial load", () => {
    render(App);
    expect(document.querySelector("[data-testid='progress-view']")).toBeNull();
    expect(document.querySelector("[data-testid='results-view']")).toBeNull();
  });

  // Shared-link init

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
      ["/jobs/", errorResponse(404)],
    ]);
    render(App);
    await waitFor(() =>
      expect(screen.getByTestId("expired-view")).toBeTruthy()
    );
  });

  // Form disabled state

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

  // Post-job flow

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

  it("hides nameserver timings when public info disables them", async () => {
    window.location.hash = "#/result/abc12345";
    fetchRouter([
      ["/locales", localesResp],
      ["/info", jsonResponse({ show_score_public: false, show_nameserver_timings_public: false })],
      ["jobs/abc12345/result", jsonResponse({
        job_id: "x",
        status: "succeeded",
        nameserver_timings: [
          { nameserver: "ns1.example.com", address: "192.0.2.10", avg_ms: 24, min_ms: 20, max_ms: 30, count: 3 },
        ],
        raw: { locale: "en", entries: [] },
      })],
      ["/jobs/", jobResp("succeeded", "example.com", 100)],
    ]);
    render(App);
    await waitFor(() => screen.getByTestId("results-view"));
    expect(screen.queryByTestId("nameserver-timings")).toBeNull();
  });

  it("shows the DNSSEC chain section when public info enables it and the marker is set", async () => {
    window.location.hash = "#/result/abc12345";
    fetchRouter([
      ["/locales", localesResp],
      ["/info", jsonResponse({ show_dnssec_chain_public: true })],
      ["jobs/abc12345/result", jsonResponse({ job_id: "x", status: "succeeded", raw: { locale: "en", entries: [] }, has_dnssec_chain: true })],
      ["/jobs/", jobResp("succeeded", "example.com", 100)],
    ]);
    render(App);
    await waitFor(() => screen.getByTestId("dnssec-chain"));
  });

  it("hides the DNSSEC chain section by default (fail-safe, no info)", async () => {
    window.location.hash = "#/result/abc12345";
    fetchRouter([
      ["/locales", localesResp],
      ["jobs/abc12345/result", jsonResponse({ job_id: "x", status: "succeeded", raw: { locale: "en", entries: [] }, has_dnssec_chain: true })],
      ["/jobs/", jobResp("succeeded", "example.com", 100)],
    ]);
    render(App);
    await waitFor(() => screen.getByTestId("results-view"));
    expect(screen.queryByTestId("dnssec-chain")).toBeNull();
  });

  it("shows ExpiredResult when job is expired", async () => {
    window.location.hash = "#/result/abc12345";
    fetchRouter([
      ["/locales", localesResp],
      ["/jobs/", errorResponse(404)],
    ]);
    render(App);
    await waitFor(() => screen.getByTestId("expired-view"));
  });

  // Header

  it("renders the app logo", () => {
    render(App);
    expect(screen.getByAltText("gonemaster")).toBeTruthy();
  });

  it("wraps the logo in a link back to the home view", () => {
    render(App);
    const link = screen.getByAltText("gonemaster").closest("a");
    expect(link).not.toBeNull();
    expect(link.getAttribute("href")).toBe("#/");
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

  // Locale init / persistence

  describe("locale", () => {
    const LOCALE_KEY = "gonemaster.public.locale.v1";
    const setLanguages = (langs) => {
      Object.defineProperty(navigator, "languages", { value: langs, configurable: true });
      Object.defineProperty(navigator, "language", { value: langs[0] ?? "", configurable: true });
    };

    beforeEach(() => {
      window.localStorage.removeItem(LOCALE_KEY);
    });

    afterEach(() => {
      window.localStorage.removeItem(LOCALE_KEY);
      setLanguages(["en-US"]);
    });

    it("uses stored locale on load when set", async () => {
      window.localStorage.setItem(LOCALE_KEY, "sv");
      fetchRouter([
        ["/locales", multiLocalesResp],
        ["/jobs/", jobResp("queued", "example.com", 0)],
      ]);
      render(App);
      const sel = await screen.findByRole("combobox", { name: /language/i });
      expect(sel.value).toBe("sv");
    });

    it("auto-detects locale from navigator.languages on first visit", async () => {
      setLanguages(["da-DK", "en-US"]);
      fetchRouter([
        ["/locales", multiLocalesResp],
        ["/jobs/", jobResp("queued", "example.com", 0)],
      ]);
      render(App);
      const sel = await screen.findByRole("combobox", { name: /language/i });
      expect(sel.value).toBe("da");
    });

    it("falls back to en when no browser language matches a shipped catalog", async () => {
      setLanguages(["zz-ZZ"]);
      fetchRouter([
        ["/locales", multiLocalesResp],
        ["/jobs/", jobResp("queued", "example.com", 0)],
      ]);
      render(App);
      const sel = await screen.findByRole("combobox", { name: /language/i });
      expect(sel.value).toBe("en");
    });

    it("persists locale choice to localStorage on manual change", async () => {
      fetchRouter([
        ["/locales", multiLocalesResp],
        ["/jobs/", jobResp("queued", "example.com", 0)],
      ]);
      render(App);
      const sel = await screen.findByRole("combobox", { name: /language/i });
      await fireEvent.change(sel, { target: { value: "sv" } });
      await waitFor(() => expect(window.localStorage.getItem(LOCALE_KEY)).toBe("sv"));
    });

    it("snaps stored locale back to en when the server does not advertise it", async () => {
      window.localStorage.setItem(LOCALE_KEY, "fr");
      fetchRouter([
        ["/locales", multiLocalesResp],
        ["/jobs/", jobResp("queued", "example.com", 0)],
      ]);
      render(App);
      const sel = await screen.findByRole("combobox", { name: /language/i });
      await waitFor(() => expect(sel.value).toBe("en"));
    });
  });

  it("renders the theme toggle button", () => {
    render(App);
    expect(screen.getByRole("button", { name: /theme/i })).toBeTruthy();
  });

  // Theme init / persistence

  describe("theme", () => {
    const THEME_KEY = "gonemaster.public.theme.v1";

    beforeEach(() => {
      window.localStorage.removeItem(THEME_KEY);
    });

    afterEach(() => {
      window.localStorage.removeItem(THEME_KEY);
    });

    it("uses stored theme on load when set", () => {
      window.localStorage.setItem(THEME_KEY, "dark");
      render(App);
      expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
    });

    it("persists theme choice to localStorage on toggle", async () => {
      render(App);
      const btn = screen.getByRole("button", { name: /theme/i });
      const before = document.documentElement.getAttribute("data-theme");
      await fireEvent.click(btn);
      const after = document.documentElement.getAttribute("data-theme");
      expect(after).not.toBe(before);
      expect(window.localStorage.getItem(THEME_KEY)).toBe(after);
    });
  });

  // Document title

  it("sets title to percentage while running", async () => {
    window.location.hash = "#/result/abc12345";
    fetchRouter([
      ["/locales", localesResp],
      ["/jobs/abc12345", jobResp("running", "example.com", 42)],
    ]);
    render(App);
    await waitFor(() =>
      expect(document.title).toBe("42% Gonemaster")
    );
  });

  it("resets title to Gonemaster when job finishes", async () => {
    window.location.hash = "#/result/abc12345";
    fetchRouter([
      ["/locales", localesResp],
      ["jobs/abc12345/result", resultResp()],
      ["/jobs/", jobResp("succeeded", "example.com", 100)],
    ]);
    render(App);
    await waitFor(() => screen.getByTestId("results-view"));
    expect(document.title).toBe("Gonemaster");
  });

  it("title is Gonemaster on initial idle state", () => {
    render(App);
    expect(document.title).toBe("Gonemaster");
  });

  // Theme toggle
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

  // Recent tests history

  describe("recent tests history", () => {
    const HISTORY_KEY = "gonemaster.public.history.v1";
    const readHistory = () => JSON.parse(window.localStorage.getItem(HISTORY_KEY) ?? "[]");
    const seedHistory = (entries) =>
      window.localStorage.setItem(HISTORY_KEY, JSON.stringify(entries));

    // Drives the real form: type a domain and click Test, so the run goes
    // through onJobCreated and counts as started in this tab.
    const startTest = async (domain) => {
      await fireEvent.input(screen.getByLabelText("Domain"), { target: { value: domain } });
      await fireEvent.click(screen.getByRole("button", { name: "Test" }));
    };

    it("records a run started in this tab when it succeeds", async () => {
      fetchRouter([
        ["/locales", localesResp],
        ["jobs/abc12345/result", resultResp()],
        ["/jobs/abc12345", jobResp("succeeded", "example.com", 100, "2026-08-07T09:30:00Z")],
        ["/jobs", jsonResponse({ public_id: "abc12345" })],
      ]);
      render(App);
      await startTest("example.com");
      await waitFor(() => screen.getByTestId("results-view"));
      expect(readHistory()).toEqual([
        { id: "abc12345", domain: "example.com", finishedAt: "2026-08-07T09:30:00Z" },
      ]);
    });

    it("does not record a failed run", async () => {
      fetchRouter([
        ["/locales", localesResp],
        ["/jobs/abc12345", jobResp("failed", "example.com", 100)],
        ["/jobs", jsonResponse({ public_id: "abc12345" })],
      ]);
      render(App);
      await startTest("example.com");
      await waitFor(() => screen.getByTestId("expired-view"));
      expect(readHistory()).toEqual([]);
    });

    it("does not record a share-link visit, even one watched to completion", async () => {
      // The shared job is still running on arrival (applyHash sees "running"),
      // then Progress's first poll sees it finish. Despite the succeeded
      // onJobDone, nothing may be recorded: the run was not started here.
      window.location.hash = "#/result/abc12345";
      let jobCalls = 0;
      global.fetch = vi.fn().mockImplementation(async (url) => {
        if (url.includes("/locales")) return localesResp;
        if (url.includes("jobs/abc12345/result")) return resultResp();
        if (url.includes("/jobs/abc12345")) {
          jobCalls += 1;
          return jobResp(jobCalls === 1 ? "running" : "succeeded", "example.com", 100, "2026-08-07T09:30:00Z");
        }
        return jsonResponse([]);
      });
      render(App);
      await waitFor(() => screen.getByTestId("results-view"));
      expect(readHistory()).toEqual([]);
    });

    it("prunes a stored entry when its result has expired (404)", async () => {
      seedHistory([
        { id: "abc12345", domain: "example.com", finishedAt: null },
        { id: "keep1", domain: "example.org", finishedAt: null },
      ]);
      window.location.hash = "#/result/abc12345";
      fetchRouter([
        ["/locales", localesResp],
        ["/jobs/", errorResponse(404)],
      ]);
      render(App);
      await waitFor(() => screen.getByTestId("expired-view"));
      expect(readHistory().map((e) => e.id)).toEqual(["keep1"]);
    });

    it("shows the list on the idle view when entries exist", () => {
      seedHistory([{ id: "id1", domain: "example.com", finishedAt: null }]);
      render(App);
      expect(screen.getByTestId("recent-tests")).toBeTruthy();
      expect(screen.getByText("example.com")).toBeTruthy();
    });

    it("hides the list when there is no history", () => {
      render(App);
      expect(screen.queryByTestId("recent-tests")).toBeNull();
    });

    it("hides the list while a test is running", async () => {
      seedHistory([{ id: "id1", domain: "example.com", finishedAt: null }]);
      window.location.hash = "#/result/abc12345";
      fetchRouter([
        ["/locales", localesResp],
        ["/jobs/abc12345", jobResp("running", "example.com", 30)],
      ]);
      render(App);
      await waitFor(() => screen.getByTestId("progress-view"));
      expect(screen.queryByTestId("recent-tests")).toBeNull();
    });

    it("hides the list on the result view", async () => {
      seedHistory([{ id: "id1", domain: "example.com", finishedAt: null }]);
      window.location.hash = "#/result/abc12345";
      fetchRouter([
        ["/locales", localesResp],
        ["jobs/abc12345/result", resultResp()],
        ["/jobs/", jobResp("succeeded", "example.com", 100)],
      ]);
      render(App);
      await waitFor(() => screen.getByTestId("results-view"));
      expect(screen.queryByTestId("recent-tests")).toBeNull();
    });

    it("shows the list after navigating back to the home view", async () => {
      // The full loop the feature exists for: run a test, land on the result
      // view (list hidden there), go home the way the logo link does (hash
      // navigation), and find the finished run in the recent tests list.
      fetchRouter([
        ["/locales", localesResp],
        ["jobs/abc12345/result", resultResp()],
        ["/jobs/abc12345", jobResp("succeeded", "example.com", 100, "2026-08-07T09:30:00Z")],
        ["/jobs", jsonResponse({ public_id: "abc12345" })],
      ]);
      render(App);
      await startTest("example.com");
      await waitFor(() => screen.getByTestId("results-view"));
      expect(screen.queryByTestId("recent-tests")).toBeNull();
      window.location.hash = "#/";
      await waitFor(() => screen.getByTestId("recent-tests"));
      expect(screen.getByText("example.com")).toBeTruthy();
    });

    it("updates the stored grade from a viewed result", async () => {
      seedHistory([{ id: "abc12345", domain: "example.com", finishedAt: null }]);
      window.location.hash = "#/result/abc12345";
      fetchRouter([
        ["/locales", localesResp],
        ["jobs/abc12345/result", jsonResponse({
          job_id: "x",
          status: "succeeded",
          raw: { locale: "en", entries: [] },
          score: { grade: "B" },
        })],
        ["/jobs/", jobResp("succeeded", "example.com", 100)],
      ]);
      render(App);
      await waitFor(() => screen.getByTestId("results-view"));
      await waitFor(() => expect(readHistory()[0].grade).toBe("B"));
    });
  });
});
