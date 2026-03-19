import { render, screen, fireEvent, cleanup } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App.svelte";

const mockFetch = (data = [], ok = true) => {
  global.fetch = vi.fn().mockResolvedValue({
    ok,
    json: async () => data,
  });
};

describe("App", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    window.location.hash = "";
    mockFetch(["en"]);
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

  it("shows result view when hash is #/result/:id", async () => {
    window.location.hash = "#/result/abc12345";
    render(App);
    expect(document.querySelector("[data-view='result']")).not.toBeNull();
    expect(document.querySelector("[data-view='home']")).toBeNull();
  });

  it("result view carries the publicID as data attribute", () => {
    window.location.hash = "#/result/myid0001";
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
    // Allow Svelte to flush
    await new Promise((r) => setTimeout(r, 0));
    expect(document.querySelector("[data-view='home']")).not.toBeNull();
  });

  // ── Header ─────────────────────────────────────────────────────────────────

  it("renders the app title", () => {
    render(App);
    expect(screen.getByText("Gonemaster")).toBeTruthy();
  });

  it("renders the locale selector", () => {
    render(App);
    expect(screen.getByRole("combobox")).toBeTruthy();
  });

  it("renders the theme toggle button", () => {
    render(App);
    expect(screen.getByRole("button", { name: /theme/i })).toBeTruthy();
  });

  // ── Theme cycling ──────────────────────────────────────────────────────────

  it("starts with no data-theme attribute (system default)", () => {
    render(App);
    expect(document.documentElement.getAttribute("data-theme")).toBeNull();
  });

  it("sets data-theme=light after one click", async () => {
    render(App);
    await fireEvent.click(screen.getByRole("button", { name: /theme/i }));
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
  });

  it("sets data-theme=dark after two clicks", async () => {
    render(App);
    const btn = screen.getByRole("button", { name: /theme/i });
    await fireEvent.click(btn);
    await fireEvent.click(btn);
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  });

  it("removes data-theme after three clicks (back to system)", async () => {
    render(App);
    const btn = screen.getByRole("button", { name: /theme/i });
    await fireEvent.click(btn);
    await fireEvent.click(btn);
    await fireEvent.click(btn);
    expect(document.documentElement.getAttribute("data-theme")).toBeNull();
  });

  // ── Result view "New test" button ──────────────────────────────────────────

  it("result view has a 'New test' button that navigates home", async () => {
    window.location.hash = "#/result/abc12345";
    render(App);
    const btn = screen.getByRole("button", { name: /new test/i });
    await fireEvent.click(btn);
    await new Promise((r) => setTimeout(r, 0));
    expect(document.querySelector("[data-view='home']")).not.toBeNull();
  });
});
