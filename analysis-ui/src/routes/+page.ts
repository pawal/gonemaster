import {
  ANALYSIS_STATUS_NO_SNAPSHOT,
  getCohortDetail,
  getOverview,
  listASNs,
  listNameservers,
  listTags,
  type ASNView,
  type NameserverView,
  type OverviewResponse,
  type SnapshotView,
  type TagView
} from "$lib/api";

export type OverviewPageData = {
  datasetTag: string | null;
  detail: CohortDetail | null;
  detailError: string | null;
  topTags: TagView[];
  topTagsError: string | null;
  topNameservers: NameserverView[];
  topNameserversTotal: number;
  topNameserversError: string | null;
  topASNs: ASNView[];
  topASNsTotal: number;
  topASNsError: string | null;
  // Snapshot anchor the overview is pinned to. When snapshot is null and
  // noSnapshot is true, the cohort has no captured public snapshot yet
  // and the overview renders the no_snapshot empty state instead of
  // trying to fetch cohort detail.
  snapshot: SnapshotView | null;
  noSnapshot: boolean;
};

export type FactBucket = {
  key: string;
  label: string;
  tone: string;
  count: number;
  order: number;
};

export type FactDistribution = {
  category: string;
  label: string;
  description?: string;
  order: number;
  buckets: FactBucket[];
};

export type CohortDetail = {
  dataset_tag: string;
  label: string;
  description?: string;
  materialization_status: string;
  last_materialized_at?: string;
  is_default?: boolean;
  domain_count?: number;
  nameserver_count?: number;
  endpoint_count?: number;
  asn_count?: number;
  prefix_count?: number;
  severity_distribution?: Record<string, number>;
  fact_distributions?: Record<string, FactDistribution>;
  snapshot?: SnapshotView;
  status?: string;
};

export async function load({ parent, fetch, url }): Promise<OverviewPageData> {
  const layout = await parent();
  const datasetTag = layout.resolvedCohort ?? null;
  if (!datasetTag) {
    return {
      datasetTag: null,
      detail: null,
      detailError: null,
      topTags: [],
      topTagsError: null,
      topNameservers: [],
      topNameserversTotal: 0,
      topNameserversError: null,
      topASNs: [],
      topASNsTotal: 0,
      topASNsError: null,
      snapshot: null,
      noSnapshot: false
    };
  }

  // Resolve the overview's snapshot anchor up front so the other loaders
  // can skip entirely when the cohort has no captured public snapshot.
  // This avoids surfacing a generic "Failed to load cohort" error when
  // the real state is "cohort is empty".
  const snapshotSlug = url.searchParams.get("snapshot") || "";
  const overviewFilter: Record<string, string> = { dataset_tag: datasetTag };
  if (snapshotSlug) overviewFilter.snapshot = snapshotSlug;
  let snapshot: SnapshotView | null = null;
  let noSnapshot = false;
  try {
    const overview: OverviewResponse = await getOverview(overviewFilter, fetch);
    snapshot = overview.snapshot ?? null;
    noSnapshot = overview.status === ANALYSIS_STATUS_NO_SNAPSHOT;
  } catch {
    // Ignore — the detail loaders below surface a descriptive error.
  }

  if (noSnapshot) {
    return {
      datasetTag,
      detail: null,
      detailError: null,
      topTags: [],
      topTagsError: null,
      topNameservers: [],
      topNameserversTotal: 0,
      topNameserversError: null,
      topASNs: [],
      topASNsTotal: 0,
      topASNsError: null,
      snapshot: null,
      noSnapshot: true
    };
  }

  const scopedFilter: Record<string, string | number> = { dataset_tag: datasetTag };
  if (snapshotSlug) scopedFilter.snapshot = snapshotSlug;
  const infraFilter = { ...scopedFilter, limit: 10, sort: "domain_count_desc" };
  const [detailResult, tagsResult, nsResult, asnResult] = await Promise.allSettled([
    snapshotSlug
      ? fetch(
          `/pub/api/v1/analysis/cohorts/${encodeURIComponent(datasetTag)}?snapshot=${encodeURIComponent(
            snapshotSlug
          )}`
        ).then((r) => (r.ok ? (r.json() as Promise<CohortDetail>) : Promise.reject(new Error(`HTTP ${r.status}`))))
      : getCohortDetail(datasetTag, fetch),
    listTags({ ...scopedFilter, limit: 10, min_level: "WARNING" }, fetch),
    listNameservers(infraFilter, fetch),
    listASNs(infraFilter, fetch)
  ]);
  const errorMessage = (r: PromiseRejectedResult): string =>
    r.reason instanceof Error ? r.reason.message : String(r.reason);
  return {
    datasetTag,
    detail:
      detailResult.status === "fulfilled" ? (detailResult.value as CohortDetail) : null,
    detailError: detailResult.status === "rejected" ? errorMessage(detailResult) : null,
    topTags: tagsResult.status === "fulfilled" ? tagsResult.value.items : [],
    topTagsError: tagsResult.status === "rejected" ? errorMessage(tagsResult) : null,
    topNameservers: nsResult.status === "fulfilled" ? nsResult.value.items : [],
    topNameserversTotal: nsResult.status === "fulfilled" ? nsResult.value.total : 0,
    topNameserversError: nsResult.status === "rejected" ? errorMessage(nsResult) : null,
    topASNs: asnResult.status === "fulfilled" ? asnResult.value.items : [],
    topASNsTotal: asnResult.status === "fulfilled" ? asnResult.value.total : 0,
    topASNsError: asnResult.status === "rejected" ? errorMessage(asnResult) : null,
    snapshot,
    noSnapshot: false
  };
}
