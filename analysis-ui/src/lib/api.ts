// Thin client for the public analysis API. Every request goes through
// PUBLIC_BASE — the trusted `/api/v1/*` admin surface is intentionally NOT
// reachable from this UI (see api.test.ts for a machine-checked assertion).

export const PUBLIC_BASE = "/pub/api/v1/analysis";

export type Cohort = {
  dataset_tag: string;
  label: string;
  description?: string;
  is_default: boolean;
  sort_order?: number;
};

export type CatalogResponse = {
  default_tag?: string;
  cohorts: Cohort[];
  selector_enabled: boolean;
};

export type OverviewResponse = {
  dataset_tag: string;
  label: string;
  description?: string;
  materialization_status: string;
  last_materialized_at?: string;
  is_default: boolean;
};

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
  finished_at?: string;
};

export type NameserverView = {
  nameserver: string;
  domain_count: number;
  endpoint_count: number;
  ipv4_count: number;
  ipv6_count: number;
  asn_count: number;
  query_count?: number;
};

export type EndpointView = {
  nameserver: string;
  address: string;
  family: string;
  domain_count: number;
  asn?: number;
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
};

export type TagView = {
  tag: string;
  module?: string;
  level?: string;
  domain_count: number;
  occurrence_count: number;
};

export type TestcaseView = {
  module: string;
  testcase: string;
  domain_count: number;
  entry_count: number;
  worst_level?: string;
  unique_tags: number;
};

export type AnalysisFilter = {
  dataset_tag?: string;
  scope_mode?: string;
  batch_id?: string;
  from?: string;
  to?: string;
  family?: string;
  level?: string;
  search?: string;
  limit?: number;
  offset?: number;
  sort?: string;
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

// buildURL forbids crossing into /api/v1 (admin surface). Any caller that
// tries to pass an absolute path outside /pub/ is rejected. This is the
// single enforcement point for "public API only".
function buildURL(path: string, filter: AnalysisFilter = {}): string {
  if (!path.startsWith("/")) path = `/${path}`;
  const full = `${PUBLIC_BASE}${path}${buildQuery(filter)}`;
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

export const listTestcases = (filter: AnalysisFilter = {}, fetchFn: FetchLike = fetch) =>
  getJSON<ListResponse<TestcaseView>>("/testcases", filter, fetchFn);

// ── Detail endpoints ──────────────────────────────────────────────────────

export const getDomainDetail = (domain: string, filter: AnalysisFilter = {}, fetchFn: FetchLike = fetch) =>
  getJSON<unknown>(`/domains/${encodeURIComponent(domain)}`, filter, fetchFn);

export const getNameserverDetail = (name: string, filter: AnalysisFilter = {}, fetchFn: FetchLike = fetch) =>
  getJSON<unknown>(`/nameservers/${encodeURIComponent(name)}`, filter, fetchFn);

export const getEndpointDetail = (address: string, filter: AnalysisFilter = {}, fetchFn: FetchLike = fetch) =>
  getJSON<unknown>(`/endpoints/${encodeURIComponent(address)}`, filter, fetchFn);

export const getASNDetail = (asn: number | string, filter: AnalysisFilter = {}, fetchFn: FetchLike = fetch) =>
  getJSON<unknown>(`/asns/${encodeURIComponent(String(asn))}`, filter, fetchFn);

// Prefix and testcase detail use query parameters because the identifiers
// contain characters ("/" in prefixes) that fight with path routing.
export const getPrefixDetail = (prefix: string, filter: AnalysisFilter = {}, fetchFn: FetchLike = fetch) =>
  getJSON<unknown>("/prefix", { ...filter, prefix } as AnalysisFilter & { prefix: string }, fetchFn);

export const getTagDetail = (tag: string, filter: AnalysisFilter = {}, fetchFn: FetchLike = fetch) =>
  getJSON<unknown>(`/tags/${encodeURIComponent(tag)}`, filter, fetchFn);

export const getTestcaseDetail = (
  module: string,
  testcase: string,
  filter: AnalysisFilter = {},
  fetchFn: FetchLike = fetch
) =>
  getJSON<unknown>(
    "/testcase",
    { ...filter, module, testcase } as AnalysisFilter & { module: string; testcase: string },
    fetchFn
  );
