import { describe, expect, it } from "vitest";
import {
  asnHref,
  cohortHref,
  domainHref,
  endpointHref,
  nameserverHref,
  prefixHref,
  scopedQuery,
  tagHref
} from "./entityLinks";

const BASE = "/analysis";
const QUERY = "?dataset_tag=tld";

describe("entity link builders", () => {
  it("builds paths under the SvelteKit base", () => {
    expect(domainHref(BASE, "alpha.example")).toBe("/analysis/domains/alpha.example");
    expect(nameserverHref(BASE, "ns.example")).toBe("/analysis/nameservers/ns.example");
    expect(asnHref(BASE, 64500)).toBe("/analysis/asns/64500");
    expect(tagHref(BASE, "DS07_NOT_SIGNED")).toBe("/analysis/tags/DS07_NOT_SIGNED");
    expect(cohortHref(BASE, "tld")).toBe("/analysis/?dataset_tag=tld");
  });

  it("preserves the caller's query string", () => {
    expect(domainHref(BASE, "alpha.example", QUERY)).toBe("/analysis/domains/alpha.example?dataset_tag=tld");
    expect(asnHref(BASE, 64500, QUERY)).toBe("/analysis/asns/64500?dataset_tag=tld");
  });

  it("encodes path segments that contain reserved characters", () => {
    expect(prefixHref(BASE, "192.0.2.0/24")).toBe("/analysis/prefixes/192.0.2.0%2F24");
    expect(domainHref(BASE, "xn--bcher-kva.example")).toBe("/analysis/domains/xn--bcher-kva.example");
  });

  it("endpointHref appends nameserver disambiguator, merging with existing query", () => {
    expect(endpointHref(BASE, "192.0.2.10", "ns.example")).toBe(
      "/analysis/endpoints/192.0.2.10?nameserver=ns.example"
    );
    expect(endpointHref(BASE, "192.0.2.10", "ns.example", QUERY)).toBe(
      "/analysis/endpoints/192.0.2.10?dataset_tag=tld&nameserver=ns.example"
    );
    expect(endpointHref(BASE, "192.0.2.10", null, QUERY)).toBe(
      "/analysis/endpoints/192.0.2.10?dataset_tag=tld"
    );
  });

  it("scopedQuery pins the cohort and snapshot, omitting empty parts", () => {
    expect(scopedQuery("TLDs", "2026-07-14-51ef2053c397")).toBe(
      "?dataset_tag=TLDs&snapshot=2026-07-14-51ef2053c397"
    );
    // A missing cohort or snapshot must drop out rather than emit an empty
    // param that would rescope the detail page to the wrong thing.
    expect(scopedQuery(null, "2026-07-14-51ef2053c397")).toBe(
      "?snapshot=2026-07-14-51ef2053c397"
    );
    expect(scopedQuery("TLDs", "")).toBe("?dataset_tag=TLDs");
    expect(scopedQuery(null, "")).toBe("");
  });

  it("pins a cleared tag to the from snapshot and an appeared tag to the to snapshot", () => {
    // Regression guard for the diff-page 404: a cleared tag only has a view
    // row in the "from" snapshot, so linking it to the "to" (or auto-latest)
    // snapshot returns HTTP 404 "tag not found in cohort". Appeared and
    // severity-changed tags live in "to", so they pin the "to" snapshot.
    const from = "2026-07-14-51ef2053c397";
    const to = "2026-07-20-cdbee862ffe3";
    expect(tagHref(BASE, "DS10_NSEC_QUERY_RESPONSE_ERR", scopedQuery("TLDs", from))).toBe(
      "/analysis/tags/DS10_NSEC_QUERY_RESPONSE_ERR?dataset_tag=TLDs&snapshot=2026-07-14-51ef2053c397"
    );
    expect(tagHref(BASE, "DS07_NOT_SIGNED", scopedQuery("TLDs", to))).toBe(
      "/analysis/tags/DS07_NOT_SIGNED?dataset_tag=TLDs&snapshot=2026-07-20-cdbee862ffe3"
    );
  });

  it("endpointHref replaces stale nameserver= already in the query", () => {
    // Without replacement the detail page reads the first nameserver= and
    // 404s because it doesn't belong to the row the user clicked.
    expect(
      endpointHref(BASE, "192.0.2.10", "ns2.example", "?nameserver=ns1.example")
    ).toBe("/analysis/endpoints/192.0.2.10?nameserver=ns2.example");
    expect(
      endpointHref(
        BASE,
        "192.0.2.10",
        "ns3.example",
        "?offset=50&nameserver=ns1.example&nameserver=ns2.example"
      )
    ).toBe("/analysis/endpoints/192.0.2.10?offset=50&nameserver=ns3.example");
  });
});
