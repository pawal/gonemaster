import { render, screen, fireEvent } from "@testing-library/svelte";
import { describe, expect, it, vi } from "vitest";
import RecentTests from "./RecentTests.svelte";
import { clickLink, MODIFIED_CLICKS } from "../test/helpers.js";

// Entries are given newest first, the way the history module stores them.
const ENTRIES = [
  { id: "id2", domain: "example.org", finishedAt: "2026-08-07T10:12:00Z", grade: "F" },
  { id: "id1", domain: "example.com", finishedAt: "2026-08-03T22:40:00Z" },
];

describe("RecentTests", () => {
  it("renders one row per entry, in the given order, with result links", () => {
    render(RecentTests, { entries: ENTRIES, locale: "en" });
    const links = screen.getAllByRole("link");
    expect(links).toHaveLength(2);
    expect(links[0].textContent).toBe("example.org");
    expect(links[0].getAttribute("href")).toBe("/public/result/id2");
    expect(links[1].textContent).toBe("example.com");
    expect(links[1].getAttribute("href")).toBe("/public/result/id1");
  });

  // A real href for new-tab opening, but a plain click stays in the SPA.
  it("fires onselect for a plain left click and suppresses the navigation", async () => {
    const onselect = vi.fn();
    render(RecentTests, { entries: ENTRIES, locale: "en", onselect });
    expect(await clickLink(screen.getAllByRole("link")[0])).toBe(true);
    expect(onselect).toHaveBeenCalledWith("id2");
  });

  it.each(MODIFIED_CLICKS)("leaves %s to the browser so it opens a new tab", async (_name, init) => {
    const onselect = vi.fn();
    render(RecentTests, { entries: ENTRIES, locale: "en", onselect });
    expect(await clickLink(screen.getAllByRole("link")[0], init)).toBe(false);
    expect(onselect).not.toHaveBeenCalled();
  });

  it("does not throw when no onselect handler is given", async () => {
    render(RecentTests, { entries: ENTRIES, locale: "en" });
    expect(await clickLink(screen.getAllByRole("link")[0])).toBe(true);
  });

  it("renders the grade chip only for entries that have a grade", () => {
    render(RecentTests, { entries: ENTRIES, locale: "en" });
    const chips = document.querySelectorAll(".grade-chip-letter");
    expect(chips).toHaveLength(1);
    expect(chips[0].textContent).toBe("F");
    expect(chips[0].getAttribute("data-grade")).toBe("F");
  });

  it("fires onclear when the clear button is clicked", async () => {
    const onclear = vi.fn();
    render(RecentTests, { entries: ENTRIES, locale: "en", onclear });
    await fireEvent.click(screen.getByRole("button", { name: "Clear" }));
    expect(onclear).toHaveBeenCalledTimes(1);
  });

  it("formats dates with the given locale", () => {
    // Compute the expectation with the same Intl call the component uses, so
    // the assertion is independent of the test environment's timezone.
    const expected = new Intl.DateTimeFormat("sv", {
      dateStyle: "medium",
      timeStyle: "short",
    }).format(new Date("2026-08-07T10:12:00Z"));
    render(RecentTests, { entries: [ENTRIES[0]], locale: "sv" });
    expect(screen.getByText(expected)).toBeTruthy();
  });

  it("renders an empty date for a null or unparseable finishedAt", () => {
    render(RecentTests, {
      entries: [
        { id: "a", domain: "a.example", finishedAt: null },
        { id: "b", domain: "b.example", finishedAt: "not a date" },
      ],
      locale: "en",
    });
    const dates = document.querySelectorAll(".recent-date");
    expect(dates).toHaveLength(2);
    expect(dates[0].textContent).toBe("");
    expect(dates[1].textContent).toBe("");
  });

  it("uses the localized heading", () => {
    render(RecentTests, { entries: ENTRIES, locale: "en" });
    expect(screen.getByRole("heading", { name: "Recent tests" })).toBeTruthy();
  });
});
