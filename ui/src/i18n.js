import { writable, derived } from "svelte/store";
import en from "./i18n/en.json";

/**
 * Loaded translation catalogs. `en` is always present (bundled at build time
 * so it is available synchronously as the fallback for every lookup).
 */
const catalogs = { en };

/**
 * Internal version counter. Incremented whenever a new catalog is loaded so
 * that the derived `t` store re-runs and picks up the fresh translations.
 */
const _version = writable(0);

/** The active locale code (e.g. "en", "da", "sv"). */
export const locale = writable("en");

/**
 * Injects a catalog object directly under the given locale code and triggers
 * a reactive update. Useful in tests (avoids mocking dynamic imports) and for
 * any caller that already has the catalog data in memory.
 */
export const setCatalog = (code, catalog) => {
  catalogs[code] = catalog;
  _version.update((v) => v + 1);
};

/**
 * Asynchronously loads `ui/src/i18n/<code>.json` if it has not been loaded
 * yet. On success, calls `setCatalog` so reactive derivations update. On
 * failure (file missing or parse error), does nothing — the UI falls back to
 * English silently.
 */
export const loadCatalog = async (code) => {
  if (code === "en" || catalogs[code] !== undefined) return;
  try {
    const mod = await import(`./i18n/${code}.json`);
    setCatalog(code, mod.default ?? mod);
  } catch (_) {
    // Unknown or unavailable locale: stay on English.
  }
};

/**
 * Reactive translation function. Subscribe as `$t` in Svelte templates.
 *
 * Usage:
 *   {$t("key")}
 *   {$t("key_with_var", { count: n })}
 *
 * Resolution order for a given key:
 *   1. Active locale catalog
 *   2. English catalog
 *   3. The raw key string (never throws)
 *
 * Interpolation: `{var}` placeholders in the string are replaced with the
 * matching value from `vars`. Extra vars are ignored; missing vars leave the
 * placeholder literal in the output.
 */
export const t = derived([locale, _version], ([$locale]) => (key, vars = {}) => {
  const catalog = catalogs[$locale] ?? catalogs.en;
  let str = catalog?.[key] ?? catalogs.en?.[key] ?? key;
  for (const [k, v] of Object.entries(vars)) {
    str = str.replaceAll(`{${k}}`, String(v));
  }
  return str;
});
