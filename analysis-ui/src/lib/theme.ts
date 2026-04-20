// Theme helper mirroring ui-public/'s pattern: persist the user's explicit
// choice in localStorage under "gonemaster.analysis.theme.v1", fall back to
// the system preference otherwise.

const STORAGE_KEY = "gonemaster.analysis.theme.v1";

export type Theme = "light" | "dark";

export function readStoredTheme(): Theme | null {
  if (typeof localStorage === "undefined") return null;
  const raw = localStorage.getItem(STORAGE_KEY);
  return raw === "light" || raw === "dark" ? raw : null;
}

export function systemPrefersDark(): boolean {
  if (typeof window === "undefined" || !window.matchMedia) return false;
  return window.matchMedia("(prefers-color-scheme: dark)").matches;
}

export function applyTheme(theme: Theme): void {
  if (typeof document === "undefined") return;
  document.documentElement.setAttribute("data-theme", theme);
}

export function persistTheme(theme: Theme): void {
  if (typeof localStorage === "undefined") return;
  localStorage.setItem(STORAGE_KEY, theme);
}

export function initialTheme(): Theme {
  return readStoredTheme() ?? (systemPrefersDark() ? "dark" : "light");
}
