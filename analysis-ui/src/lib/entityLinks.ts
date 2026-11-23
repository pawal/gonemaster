// Path builders for entity-link chips. All chips go through these helpers
// so that route changes in the SvelteKit tree update link behavior in one
// place. Links carry only the pinned cohort and snapshot from the caller's query.

export type EntityType =
  | "cohort"
  | "domain"
  | "nameserver"
  | "endpoint"
  | "asn"
  | "prefix"
  | "tag"
  | "testcase";

// Params that pin the cohort and snapshot; links and the canonical URL keep only these.
const PINNED_PARAMS = ["dataset_tag", "snapshot"] as const;

// Query string with only the non-empty pinned params of query.
export function pinnedQuery(query: string): string {
  const source = new URLSearchParams(query);
  const params = new URLSearchParams();
  for (const key of PINNED_PARAMS) {
    const value = source.get(key);
    if (value) params.set(key, value);
  }
  const q = params.toString();
  return q ? `?${q}` : "";
}

export function cohortHref(base: string, datasetTag: string, query = ""): string {
  // The analysis dashboard has no standalone per-cohort page; clicking a
  // cohort chip rescopes the overview via the dataset_tag query parameter.
  // Any existing query is replaced so the chip doesn't accidentally carry
  // over an unrelated dataset_tag from the current URL.
  const params = new URLSearchParams(pinnedQuery(query));
  params.set("dataset_tag", datasetTag);
  return `${base}/?${params.toString()}`;
}

export function domainHref(base: string, domain: string, query = ""): string {
  return `${base}/domains/${encodeURIComponent(domain)}${pinnedQuery(query)}`;
}

export function nameserverHref(base: string, name: string, query = ""): string {
  return `${base}/nameservers/${encodeURIComponent(name)}${pinnedQuery(query)}`;
}

export function endpointHref(
  base: string,
  address: string,
  nameserver: string | null,
  query = ""
): string {
  const url = `${base}/endpoints/${encodeURIComponent(address)}`;
  // The public API needs the nameserver when several share an address.
  const params = new URLSearchParams(pinnedQuery(query));
  if (nameserver) {
    params.set("nameserver", nameserver);
  }
  const serialized = params.toString();
  return serialized ? `${url}?${serialized}` : url;
}

export function prefixHref(base: string, prefix: string, query = ""): string {
  // Prefixes contain "/" (CIDR) so we route through a param-based segment
  // rather than an in-path encoding. The SvelteKit route is /prefixes/[prefix]
  // and accepts the already-encoded value.
  return `${base}/prefixes/${encodeURIComponent(prefix)}${pinnedQuery(query)}`;
}

export function asnHref(base: string, asn: number | string, query = ""): string {
  return `${base}/asns/${encodeURIComponent(String(asn))}${pinnedQuery(query)}`;
}

export function tagHref(base: string, tag: string, query = ""): string {
  return `${base}/tags/${encodeURIComponent(tag)}${pinnedQuery(query)}`;
}

// Query that pins a snapshot-scoped detail page to one cohort snapshot.
export function scopedQuery(datasetTag: string | null, snapshot: string): string {
  const params = new URLSearchParams();
  if (datasetTag) params.set("dataset_tag", datasetTag);
  if (snapshot) params.set("snapshot", snapshot);
  const q = params.toString();
  return q ? `?${q}` : "";
}

// domainsSeverityHref links to the domains list filtered by an exact
// worst_level bucket. Used by the overview's health bar so every segment
// deep-links into the matching subset without losing the cohort scope.
// Exact match (not threshold) - clicking "ERROR" shows only ERROR
// domains, not "ERROR and worse".
export function domainsSeverityHref(base: string, bucket: string, query = ""): string {
  const params = new URLSearchParams(pinnedQuery(query));
  params.set("worst_level", bucket);
  return `${base}/domains?${params.toString()}`;
}

// domainsGradeHref links to the domains list filtered by an exact grade
// label. Grade values are pass-through strings because scoring is
// configurable - we don't normalize or validate on the client.
export function domainsGradeHref(base: string, grade: string, query = ""): string {
  const params = new URLSearchParams(pinnedQuery(query));
  params.set("grade", grade);
  return `${base}/domains?${params.toString()}`;
}

// domainsPostureHref filters the domains list by one DNSSEC posture bucket.
export function domainsPostureHref(base: string, posture: string, query = ""): string {
  const params = new URLSearchParams(pinnedQuery(query));
  params.set("dnssec_posture", posture);
  return `${base}/domains?${params.toString()}`;
}
