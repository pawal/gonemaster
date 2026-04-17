// URL-sync helpers for the shared filter bar. The source of truth for the
// active filter is the current URL's search params; mutating the filter goes
// through `navigateWithFilter` so the URL stays canonical and the back
// button works as expected.

import type { AnalysisFilter } from "$lib/api";

export const SCOPE_MODES = ["latest_global", "latest_in_batch", "batch", "time_window"] as const;
export type ScopeMode = (typeof SCOPE_MODES)[number];

export const FAMILIES = ["", "ipv4", "ipv6"] as const;
export type Family = (typeof FAMILIES)[number];

export const LEVELS = ["", "NOTICE", "WARNING", "ERROR", "CRITICAL"] as const;
export type Level = (typeof LEVELS)[number];

const FILTER_KEYS = [
  "dataset_tag",
  "scope_mode",
  "batch_id",
  "from",
  "to",
  "family",
  "level",
  "search"
] as const satisfies readonly (keyof AnalysisFilter)[];

export type FilterKey = (typeof FILTER_KEYS)[number];

export function filterFromURL(url: URL): AnalysisFilter {
  const filter: AnalysisFilter = {};
  for (const key of FILTER_KEYS) {
    const value = url.searchParams.get(key);
    if (value != null && value !== "") {
      (filter as Record<string, string>)[key] = value;
    }
  }
  return filter;
}

export function applyFilterToParams(
  current: URLSearchParams,
  patch: Partial<Record<FilterKey, string>>
): URLSearchParams {
  const next = new URLSearchParams(current);
  for (const [key, value] of Object.entries(patch) as [FilterKey, string][]) {
    if (value === "" || value === undefined || value === null) {
      next.delete(key);
    } else {
      next.set(key, value);
    }
  }
  // Dependent-field cleanup so the URL can't hold contradictory combinations.
  const scope = next.get("scope_mode") ?? "latest_global";
  if (scope === "latest_global") {
    next.delete("batch_id");
    next.delete("from");
    next.delete("to");
  } else if (scope === "latest_in_batch" || scope === "batch") {
    next.delete("from");
    next.delete("to");
  } else if (scope === "time_window") {
    next.delete("batch_id");
  }
  return next;
}

export function searchToString(params: URLSearchParams): string {
  const s = params.toString();
  return s ? `?${s}` : "";
}
