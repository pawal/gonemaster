import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { themeStore, osDark, toggleTheme, initThemeFromStorage } from "./theme.svelte.js";
import { resetTheme, stubMatchMedia } from "../test/helpers.js";

describe("theme store", () => {
  beforeEach(() => {
    resetTheme();
  });

  afterEach(() => {
    resetTheme();
  });

  it("starts in 'system' mode with no data-theme attribute", () => {
    expect(themeStore.value).toBe("system");
    expect(document.documentElement.getAttribute("data-theme")).toBeNull();
  });

  it("isDark reflects OS preference when value is 'system'", () => {
    stubMatchMedia(true);
    expect(themeStore.isDark).toBe(true);
    stubMatchMedia(false);
    expect(themeStore.isDark).toBe(false);
  });

  it("toggleTheme from 'system' flips to the opposite of the OS preference", () => {
    stubMatchMedia(true);
    toggleTheme();
    expect(themeStore.value).toBe("light");
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
  });

  it("toggleTheme from 'system' (light OS) sets dark", () => {
    stubMatchMedia(false);
    toggleTheme();
    expect(themeStore.value).toBe("dark");
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  });

  it("toggleTheme persists to localStorage", () => {
    stubMatchMedia(false);
    toggleTheme();
    expect(localStorage.getItem("gonemaster.ui.theme.v1")).toBe("dark");
  });

  it("initThemeFromStorage restores a stored 'dark' value", () => {
    localStorage.setItem("gonemaster.ui.theme.v1", "dark");
    initThemeFromStorage();
    expect(themeStore.value).toBe("dark");
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  });

  it("initThemeFromStorage restores a stored 'light' value", () => {
    localStorage.setItem("gonemaster.ui.theme.v1", "light");
    initThemeFromStorage();
    expect(themeStore.value).toBe("light");
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
  });

  it("initThemeFromStorage ignores unrecognized values and stays in 'system'", () => {
    localStorage.setItem("gonemaster.ui.theme.v1", "neon");
    initThemeFromStorage();
    expect(themeStore.value).toBe("system");
    expect(document.documentElement.getAttribute("data-theme")).toBeNull();
  });

  it("toggleTheme toggles between explicit light and dark", () => {
    stubMatchMedia(false);
    toggleTheme();
    expect(themeStore.value).toBe("dark");
    toggleTheme();
    expect(themeStore.value).toBe("light");
    toggleTheme();
    expect(themeStore.value).toBe("dark");
  });

  it("osDark returns false when matchMedia is unavailable", () => {
    const original = window.matchMedia;
    delete window.matchMedia;
    expect(osDark()).toBe(false);
    window.matchMedia = original;
  });
});
