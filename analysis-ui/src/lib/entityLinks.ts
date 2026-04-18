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
  let url = `${base}/endpoints/${encodeURIComponent(address)}`;
  // When multiple nameservers share an address, the public API requires a
  // `nameserver` disambiguator. Encode it into the query while preserving
  // the caller's existing query string.
  if (nameserver) {
    const separator = query ? "&" : "?";
    url += `${query}${separator}nameserver=${encodeURIComponent(nameserver)}`;
  } else if (query) {
    url += query;
  }
  return url;
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

export function testcaseHref(
  base: string,
  testcase: string,
  module: string | null = null,
  query = ""
): string {
  let url = `${base}/testcases/${encodeURIComponent(testcase)}`;
  if (module) {
    const separator = query ? "&" : "?";
    url += `${query}${separator}module=${encodeURIComponent(module)}`;
  } else if (query) {
    url += query;
  }
  return url;
}
