import {
  ANALYSIS_STATUS_NO_SNAPSHOT,
  NoSnapshotError,
  getOverview,
  type FactDistribution,
  type OverviewResponse,
  type OverviewTotals,
  type SnapshotView,
  type TopASNEntry,
  type TopNameserverEntry,
  type TopTagEntry
} from "$lib/api";

export type OverviewPageData = {
  datasetTag: string | null;
  label: string;
  description: string;
  lastMaterializedAt: string | null;
  totals: OverviewTotals | null;
  factDistributions: Record<string, FactDistribution> | null;
  topTags: TopTagEntry[];
  topNameservers: TopNameserverEntry[];
  topASNs: TopASNEntry[];
  // Snapshot anchor the overview is pinned to. When snapshot is null and
  // noSnapshot is true, the cohort has no captured public snapshot yet
  // and the overview renders the no_snapshot empty state.
  snapshot: SnapshotView | null;
  noSnapshot: boolean;
  loadError: string | null;
};

export async function load({ parent, fetch }): Promise<OverviewPageData> {
  const layout = await parent();
  const datasetTag = layout.resolvedCohort ?? null;
  if (!datasetTag) {
    return emptyPageData(null);
  }

  const snapshotSlug = layout.effectiveSnapshotSlug ?? "";
  const filter: Record<string, string> = { dataset_tag: datasetTag };
  if (snapshotSlug) filter.snapshot = snapshotSlug;

  let overview: OverviewResponse;
  try {
    overview = await getOverview(filter, fetch);
  } catch (err) {
    if (err instanceof NoSnapshotError) {
      return { ...emptyPageData(datasetTag), noSnapshot: true };
    }
    return {
      ...emptyPageData(datasetTag),
      loadError: err instanceof Error ? err.message : String(err)
    };
  }

  if (overview.status === ANALYSIS_STATUS_NO_SNAPSHOT) {
    return { ...emptyPageData(datasetTag), label: overview.label, description: overview.description ?? "", noSnapshot: true };
  }

  const payload = overview.overview ?? null;
  return {
    datasetTag,
    label: overview.label,
    description: overview.description ?? "",
    lastMaterializedAt: overview.last_materialized_at ?? null,
    totals: payload?.totals ?? null,
    factDistributions: payload?.fact_distributions ?? null,
    topTags: payload?.top_tags ?? [],
    topNameservers: payload?.top_nameservers ?? [],
    topASNs: payload?.top_asns ?? [],
    snapshot: overview.snapshot ?? null,
    noSnapshot: false,
    loadError: null
  };
}

function emptyPageData(datasetTag: string | null): OverviewPageData {
  return {
    datasetTag,
    label: "",
    description: "",
    lastMaterializedAt: null,
    totals: null,
    factDistributions: null,
    topTags: [],
    topNameservers: [],
    topASNs: [],
    snapshot: null,
    noSnapshot: false,
    loadError: null
  };
}
