import type { CatalogResponse, Cohort, SnapshotListEntry } from "$lib/api";
import { getCatalog, listSnapshots } from "$lib/api";

// The analysis UI is a pure client-side SPA embedded in the Go server.
export const ssr = false;
export const prerender = false;
export const trailingSlash = "never";

export type LayoutData = {
  catalog: CatalogResponse | null;
  catalogError: string | null;
  resolvedCohort: string | null;
  backendSupported: boolean;
  // Snapshots for the resolved cohort so the FilterBar can render the
  // snapshot selector without each page refetching them. Empty when no
  // captured public snapshot exists or the list couldn't be loaded.
  snapshots: SnapshotListEntry[];
  defaultSnapshotSlug: string | null;
};

function resolveDefaultSnapshotSlug(
  catalog: CatalogResponse | null,
  datasetTag: string | null
): string | null {
  if (!catalog || !datasetTag) return null;
  const cohort = catalog.cohorts.find((c: Cohort) => c.dataset_tag === datasetTag);
  return cohort?.default_snapshot?.slug ?? null;
}

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

    let snapshots: SnapshotListEntry[] = [];
    if (resolvedCohort) {
      try {
        const list = await listSnapshots(resolvedCohort, fetch);
        snapshots = list.snapshots ?? [];
      } catch {
        // Snapshot list failure is non-fatal — the selector just hides
        // and the rest of the UI continues to work against auto-latest.
        snapshots = [];
      }
    }

    return {
      catalog,
      catalogError: null,
      resolvedCohort,
      backendSupported: catalog.backend_supported !== false,
      snapshots,
      defaultSnapshotSlug: resolveDefaultSnapshotSlug(catalog, resolvedCohort)
    };
  } catch (error) {
    return {
      catalog: null,
      catalogError: error instanceof Error ? error.message : String(error),
      resolvedCohort: null,
      backendSupported: true,
      snapshots: [],
      defaultSnapshotSlug: null
    };
  }
}
