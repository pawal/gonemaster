import { describe, expect, it } from "vitest";
import {
  asnHref,
  cohortHref,
  domainHref,
  domainsGradeHref,
  domainsPostureHref,
  domainsSeverityHref,
  endpointHref,
  nameserverHref,
  pinnedQuery,
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
    ).toBe("/analysis/endpoints/192.0.2.10?nameserver=ns3.example");
  });

  it("domainsPostureHref filters the domains list on one posture bucket", () => {
    expect(domainsPostureHref(BASE, "nsec3")).toBe("/analysis/domains?dnssec_posture=nsec3");
    expect(domainsPostureHref(BASE, "nsec3", QUERY)).toBe(
      "/analysis/domains?dataset_tag=tld&dnssec_posture=nsec3"
    );
    // A stale bucket in the caller's query is replaced, not appended.
    expect(domainsPostureHref(BASE, "mixed", "?dnssec_posture=nsec")).toBe(
      "/analysis/domains?dnssec_posture=mixed"
    );
  });

  // Every param a page may carry, pinned and unpinned, in mixed order.
  const NOISY = "?grade=A&nameserver=c.nic.example&dataset_tag=tld&sort=grade&snapshot=2026-07-14-51ef2053c397&offset=50";
  const PINNED = "dataset_tag=tld&snapshot=2026-07-14-51ef2053c397";

  it.each([
    ["domainHref", () => domainHref(BASE, "alpha.example", NOISY), `/analysis/domains/alpha.example?${PINNED}`],
    ["nameserverHref", () => nameserverHref(BASE, "a.dns.example", NOISY), `/analysis/nameservers/a.dns.example?${PINNED}`],
    ["endpointHref", () => endpointHref(BASE, "192.0.2.10", null, NOISY), `/analysis/endpoints/192.0.2.10?${PINNED}`],
    [
      "endpointHref with a nameserver",
      () => endpointHref(BASE, "192.0.2.10", "ns.example", NOISY),
      `/analysis/endpoints/192.0.2.10?${PINNED}&nameserver=ns.example`
    ],
    ["prefixHref", () => prefixHref(BASE, "192.0.2.0/24", NOISY), `/analysis/prefixes/192.0.2.0%2F24?${PINNED}`],
    ["asnHref", () => asnHref(BASE, 64500, NOISY), `/analysis/asns/64500?${PINNED}`],
    ["tagHref", () => tagHref(BASE, "Z01_MNAME_NOT_MASTER", NOISY), `/analysis/tags/Z01_MNAME_NOT_MASTER?${PINNED}`],
    [
      "cohortHref",
      () => cohortHref(BASE, "gtld", NOISY),
      "/analysis/?dataset_tag=gtld&snapshot=2026-07-14-51ef2053c397"
    ],
    ["domainsGradeHref", () => domainsGradeHref(BASE, "B", NOISY), `/analysis/domains?${PINNED}&grade=B`],
    [
      "domainsSeverityHref",
      () => domainsSeverityHref(BASE, "ERROR", NOISY),
      `/analysis/domains?${PINNED}&worst_level=ERROR`
    ],
    [
      "domainsPostureHref",
      () => domainsPostureHref(BASE, "nsec3", NOISY),
      `/analysis/domains?${PINNED}&dnssec_posture=nsec3`
    ]
  ])("%s carries only dataset_tag and snapshot from the caller's query", (_name, build, want) => {
    expect(build()).toBe(want);
  });
});

describe("pinnedQuery", () => {
  it.each([
    ["", ""],
    ["?sort=grade&offset=50", ""],
    ["?dataset_tag=tld", "?dataset_tag=tld"],
    ["dataset_tag=tld", "?dataset_tag=tld"],
    ["?snapshot=s1&dataset_tag=tld", "?dataset_tag=tld&snapshot=s1"],
    ["?dataset_tag=&snapshot=s1&nameserver=ns.example", "?snapshot=s1"]
  ])("pins %j to %j", (query, want) => {
    expect(pinnedQuery(query)).toBe(want);
  });
});
