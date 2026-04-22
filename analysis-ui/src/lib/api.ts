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
  backend_supported: boolean;
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
};

export type DomainDetailAddress = {
  address: string;
  family: string;
  asn?: number;
  asn_label?: string;
  prefix?: string;
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
  entries: DomainDetailEntry[];
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
  search?: string;
  limit?: number;
  offset?: number;
  sort?: string;
  min_level?: string;
  worst_level?: string;
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
