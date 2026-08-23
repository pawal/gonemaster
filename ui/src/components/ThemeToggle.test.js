import { render, screen, fireEvent } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ThemeToggle from "./ThemeToggle.svelte";
import { themeStore, _resetForTests } from "../lib/theme.svelte.js";

const stubMatchMedia = (matchesDark) => {
  const mock = vi.fn().mockImplementation((q) => ({
    matches: q === "(prefers-color-scheme: dark)" ? matchesDark : false,
    media: q,
    addEventListener: () => {},
    removeEventListener: () => {},
  }));
  vi.stubGlobal("matchMedia", mock);
  window.matchMedia = mock;
};

describe("ThemeToggle", () => {
  beforeEach(() => {
    localStorage.clear();
    document.documentElement.removeAttribute("data-theme");
    _resetForTests();
  });

  afterEach(() => {
    localStorage.clear();
    document.documentElement.removeAttribute("data-theme");
    _resetForTests();
  });

  it("shows the sun icon when the OS is light and value is 'system'", () => {
    stubMatchMedia(false);
    render(ThemeToggle);
    expect(screen.getByRole("button").textContent.trim()).toBe("☀");
  });

  it("shows the moon icon when the OS is dark and value is 'system'", () => {
    stubMatchMedia(true);
    render(ThemeToggle);
    expect(screen.getByRole("button").textContent.trim()).toBe("☾");
  });

  it("clicking the toggle switches to dark when starting in light-OS system", async () => {
    stubMatchMedia(false);
    render(ThemeToggle);
    await fireEvent.click(screen.getByRole("button"));
    expect(themeStore.value).toBe("dark");
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  });

  it("clicking the toggle persists the new value to localStorage", async () => {
    stubMatchMedia(false);
    render(ThemeToggle);
    await fireEvent.click(screen.getByRole("button"));
    expect(localStorage.getItem("gonemaster.ui.theme.v1")).toBe("dark");
  });
});
