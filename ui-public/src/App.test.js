import { render, screen, fireEvent, waitFor } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App.svelte";
import { clickLink, errorResponse, fetchRouter, goBackTo, goTo, jsonResponse } from "./test/helpers.js";

const localesResp = jsonResponse({ locales: ["en"] });
const multiLocalesResp = jsonResponse({ locales: ["en", "sv", "da"] });

const jobResp = (status, domain = "example.com", progress = 0, finished_at = null) =>
  jsonResponse({ public_id: "abc12345", domain, status, progress, finished_at });

const resultResp = (entries = []) =>
  jsonResponse({ job_id: "x", status: "succeeded", raw: { locale: "en", entries } });

const brandLink = () => screen.getByAltText("gonemaster").closest("a");
const clickHome = () => clickLink(brandLink());

describe("App", () => {
  beforeEach(() => {
    goTo("/public/");
    fetchRouter([
      ["/locales", localesResp],
      ["/jobs/", jobResp("queued", "example.com", 0)],
    ]);
  });

  afterEach(() => {
    document.documentElement.removeAttribute("data-theme");
    document.documentElement.removeAttribute("lang");
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

  // Path routing

  describe("routing", () => {
    const succeededRoutes = [
      ["/locales", localesResp],
      ["jobs/abc12345/result", resultResp()],
      ["/jobs/", jobResp("succeeded", "example.com", 100)],
    ];

    // Links shared while results lived at #/result/:id must keep working.
    it("rewrites a legacy hash link to the path form and still renders it", async () => {
      goTo("/public/#/result/abc12345");
      fetchRouter(succeededRoutes);
      render(App);
      await waitFor(() => screen.getByTestId("results-view"));
      expect(window.location.pathname).toBe("/public/result/abc12345");
      expect(window.location.hash).toBe("");
    });

    it("puts the result path in the URL when a test is started", async () => {
      fetchRouter([
        ["/locales", localesResp],
        ["jobs/abc12345/result", resultResp()],
        ["/jobs/abc12345", jobResp("succeeded", "example.com", 100)],
        ["/jobs", jsonResponse({ public_id: "abc12345" })],
      ]);
      render(App);
      await fireEvent.input(screen.getByLabelText("Domain"), { target: { value: "example.com" } });
      await fireEvent.click(screen.getByRole("button", { name: "Test" }));
      await waitFor(() => expect(window.location.pathname).toBe("/public/result/abc12345"));
    });

    it("returns to the idle form on Back from a result", async () => {
      goTo("/public/result/abc12345");
      fetchRouter(succeededRoutes);
      render(App);
      await waitFor(() => screen.getByTestId("results-view"));
      await goBackTo("/public/");
      await waitFor(() => expect(screen.queryByTestId("results-view")).toBeNull());
    });

    // Reacting to Back by pushing home again would need a second Back press.
    it("adds no history entry when Back lands on home", async () => {
      goTo("/public/result/abc12345");
      fetchRouter(succeededRoutes);
      render(App);
      await waitFor(() => screen.getByTestId("results-view"));
      const before = window.history.length;
      await goBackTo("/public/");
      await waitFor(() => expect(screen.queryByTestId("results-view")).toBeNull());
      expect(window.history.length).toBe(before);
    });

    it("loads the result on Forward to a result path", async () => {
      fetchRouter(succeededRoutes);
      render(App);
      await goBackTo("/public/result/abc12345");
      await waitFor(() => screen.getByTestId("results-view"));
    });

    it("moves the URL home when the logo is clicked", async () => {
      goTo("/public/result/abc12345");
      fetchRouter(succeededRoutes);
      render(App);
      await waitFor(() => screen.getByTestId("results-view"));
      await clickHome();
      expect(window.location.pathname).toBe("/public/");
      await waitFor(() => expect(screen.queryByTestId("results-view")).toBeNull());
    });

    it("intercepts the logo click instead of reloading the page", async () => {
      render(App);
      expect(await clickLink(brandLink())).toBe(true);
    });

    it("opens a recent test from its row, moving the URL with it", async () => {
      window.localStorage.setItem(
        "gonemaster.public.history.v1",
        JSON.stringify([{ id: "abc12345", domain: "example.com", finishedAt: null }]),
      );
      fetchRouter(succeededRoutes);
      render(App);
      await fireEvent.click(screen.getByText("example.com"));
      expect(window.location.pathname).toBe("/public/result/abc12345");
      await waitFor(() => screen.getByTestId("results-view"));
    });
  });

  // Shared-link init

  it("shows Progress when the path is a running job on load", async () => {
    goTo("/public/result/abc12345");
    fetchRouter([
      ["/locales", localesResp],
      ["/jobs/abc12345", jobResp("running", "example.com", 30)],
    ]);
    render(App);
    await waitFor(() =>
      expect(document.querySelector("[data-testid='progress-view']")).not.toBeNull()
    );
  });

  it("shows Results when the path is a succeeded job on load", async () => {
    goTo("/public/result/abc12345");
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

  it("shows ExpiredResult when the job returns 404 on load", async () => {
    goTo("/public/result/abc12345");
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
    goTo("/public/result/abc12345");
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
    goTo("/public/result/abc12345");
    fetchRouter([
      ["/locales", localesResp],
      ["jobs/abc12345/result", resultResp()],
      ["/jobs/", jobResp("succeeded", "example.com", 100)],
    ]);
    render(App);
    await waitFor(() => screen.getByTestId("results-view"));
  });

  it("hides nameserver timings when public info disables them", async () => {
    goTo("/public/result/abc12345");
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
    goTo("/public/result/abc12345");
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
    goTo("/public/result/abc12345");
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
    goTo("/public/result/abc12345");
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
    expect(link.getAttribute("href")).toBe("/public/");
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
    // The locales fetch has to land, then Svelte needs a tick to re-render.
    await waitFor(() => expect(global.fetch).toHaveBeenCalled());
    await new Promise((r) => setTimeout(r, 0));
    expect(screen.queryByRole("combobox", { name: /language/i })).toBeNull();
  });

  // Locale init / persistence

  describe("locale", () => {
    const LOCALE_KEY = "gonemaster.public.locale.v1";
    // The selector's accessible name is itself localized, so a test that
    // starts in another language cannot find it by name.
    const localeSelect = async () =>
      await waitFor(() => {
        const el = document.querySelector(".locale-select");
        if (!el) throw new Error("locale select not rendered yet");
        return el;
      });

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
      expect((await localeSelect()).value).toBe("da");
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

    // A shared ?lang link must render the language it names, so it agrees with
    // what the server rendered into that same URL.
    it("lets ?lang win over a stored locale", async () => {
      window.localStorage.setItem(LOCALE_KEY, "da");
      goTo("/public/?lang=sv");
      fetchRouter([
        ["/locales", multiLocalesResp],
        ["/jobs/", jobResp("queued", "example.com", 0)],
      ]);
      render(App);
      expect((await localeSelect()).value).toBe("sv");
    });

    it("takes the base locale from a ?lang region tag", async () => {
      goTo("/public/?lang=sv-SE");
      fetchRouter([
        ["/locales", multiLocalesResp],
        ["/jobs/", jobResp("queued", "example.com", 0)],
      ]);
      render(App);
      expect((await localeSelect()).value).toBe("sv");
    });

    it("ignores a ?lang we do not ship", async () => {
      window.localStorage.setItem(LOCALE_KEY, "da");
      goTo("/public/?lang=zz");
      fetchRouter([
        ["/locales", multiLocalesResp],
        ["/jobs/", jobResp("queued", "example.com", 0)],
      ]);
      render(App);
      expect((await localeSelect()).value).toBe("da");
    });

    // The URL stays shareable, and a ?lang link renders the same server-side.
    it("puts the chosen locale in the URL without leaving the page", async () => {
      goTo("/public/result/abc12345");
      fetchRouter([
        ["/locales", multiLocalesResp],
        ["jobs/abc12345/result", resultResp()],
        ["/jobs/", jobResp("succeeded", "example.com", 100)],
      ]);
      render(App);
      const sel = await screen.findByRole("combobox", { name: /language/i });
      await fireEvent.change(sel, { target: { value: "sv" } });
      await waitFor(() =>
        expect(new URLSearchParams(window.location.search).get("lang")).toBe("sv")
      );
      expect(window.location.pathname).toBe("/public/result/abc12345");
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
    goTo("/public/result/abc12345");
    fetchRouter([
      ["/locales", localesResp],
      ["/jobs/abc12345", jobResp("running", "example.com", 42)],
    ]);
    render(App);
    await waitFor(() =>
      expect(document.title).toBe("42% Gonemaster")
    );
  });

  it("names the domain in the title of an unscored result", async () => {
    goTo("/public/result/abc12345");
    fetchRouter([
      ["/locales", localesResp],
      ["jobs/abc12345/result", resultResp()],
      ["/jobs/", jobResp("succeeded", "example.com", 100)],
    ]);
    render(App);
    await waitFor(() => screen.getByTestId("results-view"));
    expect(document.title).toBe("example.com - Gonemaster");
  });

  it("adds the grade to the title once the result reports one", async () => {
    goTo("/public/result/abc12345");
    fetchRouter([
      ["/locales", localesResp],
      ["jobs/abc12345/result", jsonResponse({
        job_id: "x",
        status: "succeeded",
        score: { grade: "A+", score: 100 },
        raw: { locale: "en", entries: [] },
      })],
      ["/jobs/", jobResp("succeeded", "example.com", 100)],
    ]);
    render(App);
    await waitFor(() =>
      expect(document.title).toBe("example.com - grade A+ - Gonemaster")
    );
  });

  it("drops the result from the title when the job did not succeed", async () => {
    goTo("/public/result/abc12345");
    fetchRouter([
      ["/locales", localesResp],
      ["/jobs/", jobResp("failed", "example.com", 100)],
    ]);
    render(App);
    await waitFor(() => screen.getByTestId("expired-view"));
    expect(document.title).toBe("Gonemaster");
  });

  it("title is Gonemaster on initial idle state", () => {
    render(App);
    expect(document.title).toBe("Gonemaster");
  });

  // Document language

  it("sets html lang to the initial locale", () => {
    render(App);
    expect(document.documentElement.lang).toBe("en");
  });

  it("follows the locale selector into html lang", async () => {
    fetchRouter([
      ["/locales", multiLocalesResp],
      ["/jobs/", jobResp("queued", "example.com", 0)],
    ]);
    render(App);
    const select = await waitFor(() =>
      screen.getByRole("combobox", { name: /language/i })
    );
    await fireEvent.change(select, { target: { value: "sv" } });
    await waitFor(() => expect(document.documentElement.lang).toBe("sv"));
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
      goTo("/public/result/abc12345");
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
      goTo("/public/result/abc12345");
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
      goTo("/public/result/abc12345");
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
      goTo("/public/result/abc12345");
      fetchRouter([
        ["/locales", localesResp],
        ["jobs/abc12345/result", resultResp()],
        ["/jobs/", jobResp("succeeded", "example.com", 100)],
      ]);
      render(App);
      await waitFor(() => screen.getByTestId("results-view"));
      expect(screen.queryByTestId("recent-tests")).toBeNull();
    });

    // The full loop the feature exists for: run a test, land on the result
    // view (list hidden there), click the logo home, find the run in the list.
    it("shows the list after navigating back to the home view", async () => {
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
      await clickHome();
      await waitFor(() => screen.getByTestId("recent-tests"));
      expect(screen.getByText("example.com")).toBeTruthy();
      expect(window.location.pathname).toBe("/public/");
    });

    it("updates the stored grade from a viewed result", async () => {
      seedHistory([{ id: "abc12345", domain: "example.com", finishedAt: null }]);
      goTo("/public/result/abc12345");
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
