const themeKey = "gonemaster.ui.theme.v1";

let theme = $state("system");

const applyToDocument = (value) => {
  if (typeof document === "undefined") return;
  const root = document.documentElement;
  if (value === "light" || value === "dark") {
    root.setAttribute("data-theme", value);
  } else {
    root.removeAttribute("data-theme");
  }
};

export const osDark = () =>
  typeof window !== "undefined" && !!window.matchMedia?.("(prefers-color-scheme: dark)")?.matches;

export const themeStore = {
  get value() { return theme; },
  get isDark() { return theme === "dark" || (theme === "system" && osDark()); },
};

export function initThemeFromStorage() {
  if (typeof window === "undefined") return;
  try {
    const stored = localStorage.getItem(themeKey);
    if (stored === "light" || stored === "dark") theme = stored;
  } catch (_) {}
  applyToDocument(theme);
}

export function toggleTheme() {
  const effectiveDark = theme === "dark" || (theme === "system" && osDark());
  theme = effectiveDark ? "light" : "dark";
  try {
    localStorage.setItem(themeKey, theme);
  } catch (_) {}
  applyToDocument(theme);
}

// Test-only: reset module-level state so each test starts from "system".
export function _resetForTests() {
  theme = "system";
  applyToDocument(theme);
}
