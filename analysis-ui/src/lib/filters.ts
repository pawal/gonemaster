// URL-sync helpers for the shared filter bar. The source of truth for the
// active filter is the current URL's search params; mutating the filter goes
// through `applyFilterToParams` so the URL stays canonical and the back
// button works as expected.
//
// Only the filter keys that actually reach the server are kept here:
//   - dataset_tag: pins the cohort across tab changes.
//   - search:      substring match on list endpoints.
//
// Other knobs (scope_mode, batch_id, from/to, family, level) were scaffolded
// in Phase 4 anticipating server-side filtering that never landed. They were
// removed to avoid affordances that do nothing. Re-introduce them alongside
// the matching server support.

import type { AnalysisFilter } from "$lib/api";

const FILTER_KEYS = [
  "dataset_tag",
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
  return next;
}

export function searchToString(params: URLSearchParams): string {
  const s = params.toString();
  return s ? `?${s}` : "";
}
