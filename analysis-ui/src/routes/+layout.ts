import type { CatalogResponse } from "$lib/api";
import { getCatalog } from "$lib/api";

// The analysis UI is a pure client-side SPA embedded in the Go server.
export const ssr = false;
export const prerender = false;
export const trailingSlash = "never";

export type LayoutData = {
  catalog: CatalogResponse | null;
  catalogError: string | null;
  resolvedCohort: string | null;
};

// Bootstrap: resolve the cohort catalog before any page renders. Pages
// receive `catalog`, `catalogError`, and `resolvedCohort` through
// `page.data` and can fall back to sensible messaging when the catalog is
// empty or the server couldn't be reached.
export async function load({ fetch, url }): Promise<LayoutData> {
  try {
    const catalog = await getCatalog(fetch);
    const requested = url.searchParams.get("dataset_tag") || "";
    const resolvedCohort =
      requested ||
      catalog.default_tag ||
      catalog.cohorts[0]?.dataset_tag ||
      null;
    return { catalog, catalogError: null, resolvedCohort };
  } catch (error) {
    return {
      catalog: null,
      catalogError: error instanceof Error ? error.message : String(error),
      resolvedCohort: null
    };
  }
}
