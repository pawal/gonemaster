import {
  ANALYSIS_STATUS_NO_SNAPSHOT,
  NoSnapshotError,
  getDiff,
  getOverview,
  getReport,
  getTrends,
  listASNs,
  listEndpoints,
  listNameservers,
  type ASNView,
  type AnalysisFilter,
  type DiffResponse,
  type EndpointView,
  type FactDistribution,
  type ListResponse,
  type NameserverView,
  type OverviewResponse,
  type OverviewTotals,
  type ReportResponse,
  type SnapshotView,
  type TopASNEntry,
  type TopNameserverEntry,
  type TopTagEntry,
  type TrendKeyMeta,
  type TrendPoint
} from "$lib/api";
import { previousSlug } from "$lib/diff";

// Front-page latency rankings: fastest/slowest N per entity, min-sample gated.
const RANK_LIMIT = 10;
const RANK_MIN_SAMPLES = 5;

export type LatencyRankings = {
  nameservers: { fastest: NameserverView[]; slowest: NameserverView[] };
  endpoints: { fastest: EndpointView[]; slowest: EndpointView[] };
  asns: { fastest: ASNView[]; slowest: ASNView[] };
};

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
  // Fastest/slowest entities by median latency; empty when the snapshot has none.
  latency: LatencyRankings;
  // Trend series that back the hero stat tiles (deltas + sparklines). Empty
  // when a cohort has a single snapshot or the fetch failed - both non-fatal.
  severityTrend: TrendBundle;
  gradeTrend: TrendBundle;
  dnssecTrend: TrendBundle;
  // Diff into the viewed snapshot for the "since last snapshot" card.
  diff: DiffResponse | null;
  // Classified report for the same pair, so each mover carries its cause.
  report: ReportResponse | null;
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
  const rank = <T>(
    list: (filter: AnalysisFilter, fetchFn: typeof fetch) => Promise<ListResponse<T>>,
    sort: string
  ): Promise<T[]> =>
    list({ ...filter, sort, limit: RANK_LIMIT, min_latency_samples: RANK_MIN_SAMPLES }, fetch)
      .then((r) => r.items ?? [])
      .catch(() => []);
  const [
    severityTrend,
    gradeTrend,
    dnssecTrend,
    diff,
    report,
    nsFast,
    nsSlow,
    epFast,
    epSlow,
    asnFast,
    asnSlow
  ] = await Promise.all([
    trend("severity"),
    trend("grade"),
    trend("dnssec_posture"),
    prevSlug && snapshotSlug
      ? getDiff(datasetTag, prevSlug, snapshotSlug, fetch).catch(() => null)
      : Promise.resolve(null),
    prevSlug && snapshotSlug
      ? getReport(datasetTag, prevSlug, snapshotSlug, fetch).catch(() => null)
      : Promise.resolve(null),
    rank<NameserverView>(listNameservers, "latency_p50_asc"),
    rank<NameserverView>(listNameservers, "latency_p50_desc"),
    rank<EndpointView>(listEndpoints, "latency_p50_asc"),
    rank<EndpointView>(listEndpoints, "latency_p50_desc"),
    rank<ASNView>(listASNs, "latency_p50_asc"),
    rank<ASNView>(listASNs, "latency_p50_desc")
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
    latency: {
      nameservers: { fastest: nsFast, slowest: nsSlow },
      endpoints: { fastest: epFast, slowest: epSlow },
      asns: { fastest: asnFast, slowest: asnSlow }
    },
    severityTrend,
    gradeTrend,
    dnssecTrend,
    diff,
    report,
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
    latency: {
      nameservers: { fastest: [], slowest: [] },
      endpoints: { fastest: [], slowest: [] },
      asns: { fastest: [], slowest: [] }
    },
    severityTrend: EMPTY_TREND,
    gradeTrend: EMPTY_TREND,
    dnssecTrend: EMPTY_TREND,
    diff: null,
    report: null,
    diffFrom: "",
    diffTo: "",
    snapshot: null,
    noSnapshot: false,
    loadError: null
  };
}
