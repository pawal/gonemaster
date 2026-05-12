// Thin client for the public analysis API. Every request goes through
// PUBLIC_BASE - the trusted `/api/v1/*` admin surface is intentionally NOT
// reachable from this UI (see api.test.ts for a machine-checked assertion).

export const PUBLIC_BASE = "/pub/api/v1/analysis";

export type VersionResponse = {
  gonemaster?: string;
  dns?: string;
};

export type SnapshotView = {
  slug: string;
  label?: string;
  captured_at?: string;
  first_run_at?: string;
  last_run_at?: string;
  run_count: number;
  domain_count: number;
  profile_name?: string;
  tag_view_min_level?: string;
};

export type Cohort = {
  dataset_tag: string;
  label: string;
  description?: string;
  is_default: boolean;
  sort_order?: number;
  // default_snapshot is the slug auto-latest resolution picks when
  // ?snapshot= is omitted; undefined for cohorts with no captured
  // public snapshot yet (UI renders the no_snapshot empty state).
  default_snapshot?: SnapshotView;
  // Total number of captured public snapshots in the cohort. Drives
  // whether the snapshot selector chip is worth rendering.
  snapshot_count?: number;
};

export type CatalogResponse = {
  default_tag?: string;
  cohorts: Cohort[];
  selector_enabled: boolean;
  backend_supported: boolean;
};

export type OverviewTotals = {
  domain_count: number;
  nameserver_count: number;
  endpoint_count: number;
  asn_count: number;
  prefix_count: number;
};

export type TopTagEntry = {
  tag: string;
  level?: string;
  domain_count: number;
};

export type TopNameserverEntry = {
  nameserver: string;
  domain_count: number;
};

export type TopASNEntry = {
  asn: number;
  label?: string;
  domain_count: number;
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

export type SnapshotOverviewV2 = {
  totals: OverviewTotals;
  top_tags: TopTagEntry[];
  top_nameservers: TopNameserverEntry[];
  top_asns: TopASNEntry[];
  fact_distributions?: Record<string, FactDistribution>;
};

export type OverviewResponse = {
  dataset_tag: string;
  label: string;
  description?: string;
  materialization_status: string;
  last_materialized_at?: string;
  is_default: boolean;
  // Snapshot resolution tokens: snapshot carries the current pin's
  // metadata; status = "no_snapshot" when the cohort has no captured
  // public snapshot so the UI renders an empty-state panel.
  snapshot?: SnapshotView;
  status?: string;
  // Consolidated overview payload bundled at capture time. Drives the
  // overview tab without fanning out to /tags, /nameservers, /asns.
  overview?: SnapshotOverviewV2;
};

// Status token emitted by the public analysis API when a cohort has no
// captured public snapshot yet. Centralised here so UI code keys on a
// constant instead of a loose string comparison.
export const ANALYSIS_STATUS_NO_SNAPSHOT = "no_snapshot";

export type ListResponse<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

export type DomainView = {
  domain: string;
  score?: number;
  grade?: string;
  worst_level?: string;
  nameserver_count: number;
  endpoint_count: number;
  asn_count: number;
  prefix_count: number;
  operator?: string;
  operator_asn?: number;
  finished_at?: string;
};

export type NameserverView = {
  nameserver: string;
  domain_count: number;
  endpoint_count: number;
  ipv4_count: number;
  ipv6_count: number;
  asn_count: number;
  operator?: string;
  operator_asn?: number;
  query_count?: number;
};

export type EndpointView = {
  nameserver: string;
  address: string;
  family: string;
  domain_count: number;
  asn?: number;
  asn_label?: string;
  prefix?: string;
};

export type ASNView = {
  asn: number;
  label?: string;
  domain_count: number;
  address_count: number;
  nameserver_count: number;
  prefix_count: number;
  ipv4_count: number;
  ipv6_count: number;
};

export type PrefixView = {
  prefix: string;
  family: string;
  domain_count: number;
  address_count: number;
  asn?: number;
  asn_label?: string;
};

export type TagView = {
  tag: string;
  module?: string;
  level?: string;
  domain_count: number;
  occurrence_count: number;
};

// ── Detail response shapes ─────────────────────────────────────────────────

export type DomainDetailNameserver = {
  nameserver: string;
  ipv4_count: number;
  ipv6_count: number;
  addresses: DomainDetailAddress[];
  // "unresolved" when no real address is materialized for this NS.
  status?: string;
};

export type DomainDetailAddress = {
  address: string;
  family: string;
  asn?: number;
  asn_label?: string;
  prefix?: string;
  // "unreachable" when the engine got no samples for this endpoint.
  status?: string;
};

export type DomainDetailTag = {
  tag: string;
  module?: string;
  testcase?: string;
  level?: string;
};

export type DomainDetailEntry = {
  timestamp: number;
  module?: string;
  testcase?: string;
  tag: string;
  level?: string;
  message?: string;
  raw?: string;
};

export type DomainDetail = {
  domain: string;
  score?: number;
  grade?: string;
  worst_level?: string;
  finished_at?: string;
  nameserver_count: number;
  endpoint_count: number;
  asn_count: number;
  prefix_count: number;
  nameservers: DomainDetailNameserver[];
  addresses: DomainDetailAddress[];
  tags?: DomainDetailTag[];
  // Localized log entries from the originating run; absent when the
  // run has been purged and the UI falls back to tags.
  entries?: DomainDetailEntry[];
};

export type EndpointDetail = {
  nameserver: string;
  address: string;
  family: string;
  asn?: number;
  asn_label?: string;
  prefix?: string;
  domain_count: number;
  domains: string[];
};

export type PrefixDetail = {
  prefix: string;
  family: string;
  domain_count: number;
  address_count: number;
  asns: number[];
  domains: string[];
  addresses: string[];
};

export type NameserverDetail = {
  nameserver: string;
  domain_count: number;
  endpoint_count: number;
  ipv4_count: number;
  ipv6_count: number;
  addresses: string[];
  domains: string[];
  asns: number[];
};

export type ASNDetail = {
  asn: number;
  label?: string;
  domain_count: number;
  address_count: number;
  nameserver_count: number;
  prefix_count: number;
  domains: string[];
  nameservers: string[];
  prefixes: string[];
};

export type TagDetail = {
  tag: string;
  module?: string;
  testcase?: string;
  level?: string;
  domain_count: number;
  occurrence_count: number;
  domains: string[];
};

export type AnalysisFilter = {
  dataset_tag?: string;
  snapshot?: string;
  search?: string;
  limit?: number;
  offset?: number;
  sort?: string;
  min_level?: string;
  worst_level?: string;
  grade?: string;
};

// Snapshot-specific response shapes consumed by the snapshot selector,
// trends line charts, and diff tables.
export type SnapshotListEntry = {
  slug: string;
  label?: string;
  description?: string;
  captured_at: string;
  first_run_at?: string;
  last_run_at?: string;
  run_count: number;
  domain_count: number;
  profile_name?: string;
  is_default?: boolean;
  tag_view_min_level?: string;
};

export type SnapshotListResponse = {
  dataset_tag: string;
  label: string;
  snapshots: SnapshotListEntry[];
};

export type SnapshotDetail = {
  dataset_tag: string;
  slug: string;
  label?: string;
  description?: string;
  captured_at: string;
  first_run_at?: string;
  last_run_at?: string;
  run_count: number;
  domain_count: number;
  profile_name?: string;
  is_default: boolean;
  aggregates?: Record<string, unknown>;
};

export type TrendPoint = {
  slug: string;
  label?: string;
  captured_at: string;
  first_run_at?: string;
  last_run_at?: string;
  payload: unknown;
};

export type TrendKeyMeta = {
  label: string;
  tone: string;
  order: number;
};

export type TrendResponse = {
  dataset_tag: string;
  category: string;
  points: TrendPoint[];
  key_meta?: Record<string, TrendKeyMeta>;
};

export type DiffEntry = {
  domain: string;
  from_grade?: string;
  to_grade?: string;
  from_level?: string;
  to_level?: string;
  worst_level?: string;
};

export type DiffResponse = {
  dataset_tag: string;
  from_slug: string;
  to_slug: string;
  added: DiffEntry[];
  removed: DiffEntry[];
  grade_changed: DiffEntry[];
  level_changed: DiffEntry[];
};

export type FetchLike = typeof fetch;

// buildQuery drops empty/null fields and returns a string starting with "?"
// or an empty string when nothing is set.
export function buildQuery(filter: AnalysisFilter = {}): string {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(filter)) {
    if (value === undefined || value === null) continue;
    const str = String(value);
    if (str === "") continue;
    params.set(key, str);
  }
  const serialized = params.toString();
  return serialized ? `?${serialized}` : "";
}

// Read endpoints that lift dataset_tag + snapshot into a path segment.
const snapshotScopedPaths = new Set([
  "/overview",
  "/domains",
  "/nameservers",
  "/endpoints",
  "/asns",
  "/prefixes",
  "/prefix",
  "/tags",
  "/testcases",
  "/testcase"
]);

// Thrown when a snapshot-scoped read is attempted for a cohort that has
// no captured public snapshot. Loaders catch this and render an empty
// state instead of letting the request fall through to a 404.
export class NoSnapshotError extends Error {
  constructor(message = "No content available yet for this cohort.") {
    super(message);
    this.name = "NoSnapshotError";
  }
}

// liftToSnapshotPath rewrites `/path` into
// `/cohorts/{tag}/snapshots/{slug}/path` when both are set.
function liftToSnapshotPath(path: string, filter: AnalysisFilter): { path: string; filter: AnalysisFilter } {
  const tag = filter.dataset_tag;
  const slug = filter.snapshot;
  if (!tag) return { path, filter };
  const slash = path.indexOf("/", 1);
  const head = slash === -1 ? path : path.slice(0, slash);
  if (!snapshotScopedPaths.has(head)) return { path, filter };
  if (!slug) throw new NoSnapshotError();
  const next: AnalysisFilter = { ...filter };
  delete next.dataset_tag;
  delete next.snapshot;
  return {
    path: `/cohorts/${encodeURIComponent(tag)}/snapshots/${encodeURIComponent(slug)}${path}`,
    filter: next
  };
}

// buildURL forbids crossing into /api/v1 (admin surface).
function buildURL(path: string, filter: AnalysisFilter = {}): string {
  if (!path.startsWith("/")) path = `/${path}`;
  const lifted = liftToSnapshotPath(path, filter);
  const full = `${PUBLIC_BASE}${lifted.path}${buildQuery(lifted.filter)}`;
  if (!full.startsWith(PUBLIC_BASE)) {
    throw new Error(`analysis API client refused non-public URL: ${full}`);
  }
  return full;
}

async function getJSON<T>(path: string, filter: AnalysisFilter, fetchFn: FetchLike): Promise<T> {
  const url = buildURL(path, filter);
  const response = await fetchFn(url);
  if (!response.ok) {
    const message = await response.text().catch(() => "");
    throw new Error(`${path} → HTTP ${response.status}${message ? `: ${message.slice(0, 120)}` : ""}`);
  }
  return (await response.json()) as T;
}

// ── Catalog / overview ─────────────────────────────────────────────────────

export const getCatalog = (fetchFn: FetchLike = fetch) =>
  getJSON<CatalogResponse>("/catalog", {}, fetchFn);

export const getOverview = (filter: AnalysisFilter = {}, fetchFn: FetchLike = fetch) =>
  getJSON<OverviewResponse>("/overview", filter, fetchFn);

export const getCohortDetail = (datasetTag: string, fetchFn: FetchLike = fetch) =>
  getJSON<OverviewResponse>(`/cohorts/${encodeURIComponent(datasetTag)}`, {}, fetchFn);

// ── List endpoints ────────────────────────────────────────────────────────

export const listCohorts = (fetchFn: FetchLike = fetch) =>
  getJSON<Cohort[]>("/cohorts", {}, fetchFn);

export const listDomains = (filter: AnalysisFilter = {}, fetchFn: FetchLike = fetch) =>
  getJSON<ListResponse<DomainView>>("/domains", filter, fetchFn);

export const listNameservers = (filter: AnalysisFilter = {}, fetchFn: FetchLike = fetch) =>
  getJSON<ListResponse<NameserverView>>("/nameservers", filter, fetchFn);

export const listEndpoints = (filter: AnalysisFilter = {}, fetchFn: FetchLike = fetch) =>
  getJSON<ListResponse<EndpointView>>("/endpoints", filter, fetchFn);

export const listASNs = (filter: AnalysisFilter = {}, fetchFn: FetchLike = fetch) =>
  getJSON<ListResponse<ASNView>>("/asns", filter, fetchFn);

export const listPrefixes = (filter: AnalysisFilter = {}, fetchFn: FetchLike = fetch) =>
  getJSON<ListResponse<PrefixView>>("/prefixes", filter, fetchFn);

export const listTags = (filter: AnalysisFilter = {}, fetchFn: FetchLike = fetch) =>
  getJSON<ListResponse<TagView>>("/tags", filter, fetchFn);

// ── Detail endpoints ──────────────────────────────────────────────────────

export const getDomainDetail = (domain: string, filter: AnalysisFilter = {}, fetchFn: FetchLike = fetch) =>
  getJSON<DomainDetail>(`/domains/${encodeURIComponent(domain)}`, filter, fetchFn);

export const getNameserverDetail = (name: string, filter: AnalysisFilter = {}, fetchFn: FetchLike = fetch) =>
  getJSON<NameserverDetail>(`/nameservers/${encodeURIComponent(name)}`, filter, fetchFn);

export const getEndpointDetail = (address: string, filter: AnalysisFilter = {}, fetchFn: FetchLike = fetch) =>
  getJSON<EndpointDetail>(`/endpoints/${encodeURIComponent(address)}`, filter, fetchFn);

export const getASNDetail = (asn: number | string, filter: AnalysisFilter = {}, fetchFn: FetchLike = fetch) =>
  getJSON<ASNDetail>(`/asns/${encodeURIComponent(String(asn))}`, filter, fetchFn);

// Prefix detail uses a query parameter because the identifier contains
// characters ("/" in CIDRs) that fight with path routing.
export const getPrefixDetail = (prefix: string, filter: AnalysisFilter = {}, fetchFn: FetchLike = fetch) =>
  getJSON<PrefixDetail>("/prefix", { ...filter, prefix } as AnalysisFilter & { prefix: string }, fetchFn);

export const getTagDetail = (tag: string, filter: AnalysisFilter = {}, fetchFn: FetchLike = fetch) =>
  getJSON<TagDetail>(`/tags/${encodeURIComponent(tag)}`, filter, fetchFn);

// ── Snapshot surface ──────────────────────────────────────────────────────

export const listSnapshots = (datasetTag: string, fetchFn: FetchLike = fetch) =>
  getJSON<SnapshotListResponse>(`/cohorts/${encodeURIComponent(datasetTag)}/snapshots`, {}, fetchFn);

export const getSnapshotDetail = (datasetTag: string, slug: string, fetchFn: FetchLike = fetch) =>
  getJSON<SnapshotDetail>(
    `/cohorts/${encodeURIComponent(datasetTag)}/snapshots/${encodeURIComponent(slug)}`,
    {},
    fetchFn
  );

// getTrends fetches one time series of aggregate payloads across the
// cohort's captured public snapshots. Category defaults to severity
// server-side; passing an explicit category lets the UI switch charts
// (grade, dnssec_posture, dnskey_algo, …).
export const getTrends = (
  datasetTag: string,
  filter: { category?: string; from?: string; to?: string } = {},
  fetchFn: FetchLike = fetch
) =>
  getJSON<TrendResponse>(
    `/cohorts/${encodeURIComponent(datasetTag)}/trends`,
    filter as AnalysisFilter,
    fetchFn
  );

export const getDiff = (
  datasetTag: string,
  from: string,
  to: string,
  fetchFn: FetchLike = fetch
) =>
  getJSON<DiffResponse>(
    `/cohorts/${encodeURIComponent(datasetTag)}/diff`,
    { from, to } as AnalysisFilter & { from: string; to: string },
    fetchFn
  );

export const getVersion = (fetchFn: FetchLike = fetch) =>
  getJSON<VersionResponse>("/version", {}, fetchFn);
