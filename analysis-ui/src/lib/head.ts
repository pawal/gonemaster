// Per-route document metadata for the client-rendered analysis SPA.
// routeHead/canonicalUrl are pure; applyHead is the DOM side-effect.

const SITE = "Gonemaster Analysis";

const DEFAULT_DESCRIPTION =
  "Analyse DNS, delegation, DNSSEC, and nameserver health across many domains.";

export type HeadMeta = { title: string; description: string };

// Params that select the data view and belong in a canonical URL. Volatile
// state (sort, limit, offset, search, filters) is dropped.
const CANONICAL_PARAMS = ["dataset_tag", "snapshot"];

function withSite(label: string): string {
  return `${label} - ${SITE}`;
}

export function routeHead(routeId: string | null, params: Record<string, string>): HeadMeta {
  switch (routeId) {
    case "/":
      return { title: `${SITE} - DNS Health`, description: DEFAULT_DESCRIPTION };
    case "/cohorts":
      return {
        title: withSite("Cohorts"),
        description: "Compare analysed domain sets by their DNS and delegation health grades."
      };
    case "/domains":
      return {
        title: withSite("Domains"),
        description: "DNS, delegation, and DNSSEC health grades for each analysed domain."
      };
    case "/domains/[domain]":
      return {
        title: withSite(params.domain || "Domain"),
        description: `DNS, delegation, DNSSEC, and zone findings for ${params.domain || "this domain"}.`
      };
    case "/nameservers":
      return {
        title: withSite("Nameservers"),
        description: "Nameservers ranked by the DNS findings they contribute."
      };
    case "/nameservers/[name]":
      return {
        title: withSite(params.name || "Nameserver"),
        description: `Domains served by ${params.name || "this nameserver"} and its associated DNS findings.`
      };
    case "/asns":
      return {
        title: withSite("ASNs"),
        description: "Autonomous systems hosting nameservers, ranked by DNS findings."
      };
    case "/asns/[asn]": {
      const as = params.asn ? `AS${params.asn}` : "this AS";
      return { title: withSite(params.asn ? `AS${params.asn}` : "ASN"), description: `Nameservers and domains hosted in ${as}.` };
    }
    case "/endpoints":
      return {
        title: withSite("Endpoints"),
        description: "Nameserver IP addresses and the DNS findings linked to them."
      };
    case "/endpoints/[address]":
      return {
        title: withSite(params.address || "Endpoint"),
        description: `DNS findings for nameserver endpoint ${params.address || "this address"}.`
      };
    case "/prefixes/[prefix]":
      return {
        title: withSite(params.prefix || "Prefix"),
        description: `Nameserver endpoints and findings within network prefix ${params.prefix || "this range"}.`
      };
    case "/tags":
      return {
        title: withSite("Tags"),
        description: "Finding types driving DNS and delegation health issues."
      };
    case "/tags/[tag]":
      return {
        title: withSite(params.tag || "Tag"),
        description: `Domains affected by the ${params.tag || "selected"} finding and its severity.`
      };
    case "/trends":
      return {
        title: withSite("Trends"),
        description: "How DNS health changes over time across snapshots."
      };
    case "/diff":
      return {
        title: withSite("Diff"),
        description: "Compare two snapshots to see which DNS findings appeared, cleared, or changed."
      };
    default:
      return { title: SITE, description: DEFAULT_DESCRIPTION };
  }
}

export function canonicalUrl(url: URL): string {
  const canon = new URL(url.origin + url.pathname);
  for (const key of CANONICAL_PARAMS) {
    const value = url.searchParams.get(key);
    if (value) canon.searchParams.set(key, value);
  }
  return canon.toString();
}

function setMeta(selector: string, attr: "name" | "property", key: string, content: string): void {
  let el = document.head.querySelector<HTMLMetaElement>(selector);
  if (!el) {
    el = document.createElement("meta");
    el.setAttribute(attr, key);
    document.head.appendChild(el);
  }
  el.setAttribute("content", content);
}

function setCanonical(href: string): void {
  let el = document.head.querySelector<HTMLLinkElement>('link[rel="canonical"]');
  if (!el) {
    el = document.createElement("link");
    el.setAttribute("rel", "canonical");
    document.head.appendChild(el);
  }
  el.setAttribute("href", href);
}

// Update the live head in place, reusing the server-injected static tags so
// crawlers that run JS see per-route values and others keep the defaults.
export function applyHead(routeId: string | null, params: Record<string, string>, url: URL): void {
  if (typeof document === "undefined") return;
  const { title, description } = routeHead(routeId, params);
  const canonical = canonicalUrl(url);
  document.title = title;
  setMeta('meta[name="description"]', "name", "description", description);
  setCanonical(canonical);
  setMeta('meta[property="og:title"]', "property", "og:title", title);
  setMeta('meta[property="og:description"]', "property", "og:description", description);
  setMeta('meta[property="og:url"]', "property", "og:url", canonical);
  setMeta('meta[name="twitter:title"]', "name", "twitter:title", title);
  setMeta('meta[name="twitter:description"]', "name", "twitter:description", description);
}
