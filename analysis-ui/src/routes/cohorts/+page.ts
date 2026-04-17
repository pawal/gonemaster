import { listCohorts, type Cohort } from "$lib/api";

export type CohortsPageData = {
  cohorts: Cohort[];
  error: string | null;
};

export async function load({ fetch }): Promise<CohortsPageData> {
  try {
    const cohorts = await listCohorts(fetch);
    return { cohorts: cohorts ?? [], error: null };
  } catch (error) {
    return { cohorts: [], error: error instanceof Error ? error.message : String(error) };
  }
}
