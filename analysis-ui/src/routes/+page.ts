import {
  ANALYSIS_STATUS_NO_SNAPSHOT,
  NoSnapshotError,
  getDiff,
  getOverview,
  getTrends,
  type DiffResponse,
  type FactDistribution,
  type OverviewResponse,
  type OverviewTotals,
  type SnapshotView,
  type TopASNEntry,
  type TopNameserverEntry,
  type TopTagEntry,
  type TrendKeyMeta,
  type TrendPoint
} from "$lib/api";
import { previousSlug } from "$lib/diff";

export type TrendBundle = { points: TrendPoint[]; keyMeta: Record<string, TrendKeyMeta> };

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
  // Trend series that back the hero stat tiles (deltas + sparklines). Empty
  // when a cohort has a single snapshot or the fetch failed - both non-fatal.
  severityTrend: TrendBundle;
  gradeTrend: TrendBundle;
  dnssecTrend: TrendBundle;
  // Diff into the viewed snapshot for the "since last snapshot" card.
  diff: DiffResponse | null;
  diffFrom: string;
  diffTo: string;
  // Snapshot anchor the overview is pinned to. When snapshot is null and
  // noSnapshot is true, the cohort has no captured public snapshot yet
  // and the overview renders the no_snapshot empty state.
  snapshot: SnapshotView | null;
  noSnapshot: boolean;
  loadError: string | null;
};

const EMPTY_TREND: TrendBundle = { points: [], keyMeta: {} };

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

  // Hero deltas/sparklines and the movers card are enrichments: fetch them in
  // parallel and never let a failure blank the overview.
  const prevSlug = previousSlug(layout.snapshots ?? [], snapshotSlug);
  const trend = (category: string) =>
    getTrends(datasetTag, { category }, fetch)
      .then((t): TrendBundle => ({ points: t.points ?? [], keyMeta: t.key_meta ?? {} }))
      .catch(() => EMPTY_TREND);
  const [severityTrend, gradeTrend, dnssecTrend, diff] = await Promise.all([
    trend("severity"),
    trend("grade"),
    trend("dnssec_posture"),
    prevSlug && snapshotSlug
      ? getDiff(datasetTag, prevSlug, snapshotSlug, fetch).catch(() => null)
      : Promise.resolve(null)
  ]);

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
    severityTrend,
    gradeTrend,
    dnssecTrend,
    diff,
    diffFrom: prevSlug,
    diffTo: snapshotSlug,
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
    severityTrend: EMPTY_TREND,
    gradeTrend: EMPTY_TREND,
    dnssecTrend: EMPTY_TREND,
    diff: null,
    diffFrom: "",
    diffTo: "",
    snapshot: null,
    noSnapshot: false,
    loadError: null
  };
}
