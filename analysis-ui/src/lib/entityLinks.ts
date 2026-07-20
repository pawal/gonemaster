// Path builders for entity-link chips. All chips go through these helpers
// so that route changes in the SvelteKit tree update link behavior in one
// place. Query preservation keeps the active scope/filter across clicks.

export type EntityType =
  | "cohort"
  | "domain"
  | "nameserver"
  | "endpoint"
  | "asn"
  | "prefix"
  | "tag"
  | "testcase";

export function cohortHref(base: string, datasetTag: string, query = ""): string {
  // The analysis dashboard has no standalone per-cohort page; clicking a
  // cohort chip rescopes the overview via the dataset_tag query parameter.
  // Any existing query is replaced so the chip doesn't accidentally carry
  // over an unrelated dataset_tag from the current URL.
  const params = new URLSearchParams(query);
  params.set("dataset_tag", datasetTag);
  return `${base}/?${params.toString()}`;
}

export function domainHref(base: string, domain: string, query = ""): string {
  return `${base}/domains/${encodeURIComponent(domain)}${query}`;
}

export function nameserverHref(base: string, name: string, query = ""): string {
  return `${base}/nameservers/${encodeURIComponent(name)}${query}`;
}

export function endpointHref(
  base: string,
  address: string,
  nameserver: string | null,
  query = ""
): string {
  const url = `${base}/endpoints/${encodeURIComponent(address)}`;
  // When multiple nameservers share an address, the public API requires a
  // `nameserver` disambiguator. Use set() so that any stale nameserver=
  // already in the caller's query (from earlier chip clicks) is replaced;
  // otherwise the detail page reads the first match and 404s when it
  // doesn't belong to the row the user clicked.
  const params = new URLSearchParams(query.startsWith("?") ? query.slice(1) : query);
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
  return `${base}/prefixes/${encodeURIComponent(prefix)}${query}`;
}

export function asnHref(base: string, asn: number | string, query = ""): string {
  return `${base}/asns/${encodeURIComponent(String(asn))}${query}`;
}

export function tagHref(base: string, tag: string, query = ""): string {
  return `${base}/tags/${encodeURIComponent(tag)}${query}`;
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
  const params = new URLSearchParams(query);
  params.set("worst_level", bucket);
  return `${base}/domains?${params.toString()}`;
}

// domainsGradeHref links to the domains list filtered by an exact grade
// label. Grade values are pass-through strings because scoring is
// configurable - we don't normalize or validate on the client.
export function domainsGradeHref(base: string, grade: string, query = ""): string {
  const params = new URLSearchParams(query);
  params.set("grade", grade);
  return `${base}/domains?${params.toString()}`;
}
