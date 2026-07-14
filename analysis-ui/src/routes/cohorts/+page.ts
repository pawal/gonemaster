import type { Cohort } from "$lib/api";

export type CohortsPageData = {
  cohorts: Cohort[];
  error: string | null;
};

// The layout already loaded the catalog (cohorts with per-cohort snapshot
// metadata), so reuse it instead of refetching the cohort list here.
export async function load({ parent }): Promise<CohortsPageData> {
  const layout = await parent();
  if (layout.catalogError) {
    return { cohorts: [], error: layout.catalogError };
  }
  return { cohorts: layout.catalog?.cohorts ?? [], error: null };
}
