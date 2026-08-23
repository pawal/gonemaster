import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  applyTheme,
  initialTheme,
  persistTheme,
  readStoredTheme,
  systemPrefersDark
} from "./theme";

describe("theme helpers", () => {
  let originalMatchMedia: typeof window.matchMedia;

  beforeEach(() => {
    originalMatchMedia = window.matchMedia;
  });

  afterEach(() => {
    window.matchMedia = originalMatchMedia;
  });

  it("persists and reads stored theme", () => {
    expect(readStoredTheme()).toBeNull();
    persistTheme("dark");
    expect(readStoredTheme()).toBe("dark");
    persistTheme("light");
    expect(readStoredTheme()).toBe("light");
  });

  it("falls back to system preference when nothing is stored", () => {
    window.matchMedia = vi.fn().mockReturnValue({ matches: true }) as unknown as typeof window.matchMedia;
    expect(systemPrefersDark()).toBe(true);
    expect(initialTheme()).toBe("dark");

    window.matchMedia = vi.fn().mockReturnValue({ matches: false }) as unknown as typeof window.matchMedia;
    expect(initialTheme()).toBe("light");
  });

  it("prefers the stored value over system preference", () => {
    persistTheme("light");
    window.matchMedia = vi.fn().mockReturnValue({ matches: true }) as unknown as typeof window.matchMedia;
    expect(initialTheme()).toBe("light");
  });

  it("sets data-theme on the document root", () => {
    applyTheme("dark");
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
    applyTheme("light");
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
  });
});
