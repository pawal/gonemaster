import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  applyTheme,
  initialTheme,
  persistTheme,
  readStoredTheme,
  systemPrefersDark
} from "./theme";

// SvelteKit's test shim clears the jsdom localStorage, so provide a minimal
// in-memory implementation per test to exercise the theme helpers.
function installLocalStorageMock() {
  let store: Record<string, string> = {};
  const api = {
    get length() {
      return Object.keys(store).length;
    },
    clear: () => {
      store = {};
    },
    getItem: (key: string) => (key in store ? store[key] : null),
    key: (index: number) => Object.keys(store)[index] ?? null,
    removeItem: (key: string) => {
      delete store[key];
    },
    setItem: (key: string, value: string) => {
      store[key] = String(value);
    }
  };
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    value: api
  });
  return api;
}

describe("theme helpers", () => {
  let originalMatchMedia: typeof window.matchMedia;

  beforeEach(() => {
    installLocalStorageMock();
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
