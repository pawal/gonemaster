import { appPaths } from "../test/helpers";
import { describe, expect, it, vi } from "vitest";
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

vi.mock("$app/paths", () => appPaths());

const QUERY = "?dataset_tag=tld";

describe("entity link builders", () => {
  it("builds paths under the SvelteKit base", () => {
    expect(domainHref("alpha.example")).toBe("/analysis/domains/alpha.example");
    expect(nameserverHref("ns.example")).toBe("/analysis/nameservers/ns.example");
    expect(asnHref(64500)).toBe("/analysis/asns/64500");
    expect(tagHref("DS07_NOT_SIGNED")).toBe("/analysis/tags/DS07_NOT_SIGNED");
    expect(cohortHref("tld")).toBe("/analysis/?dataset_tag=tld");
  });

  it("preserves the caller's query string", () => {
    expect(domainHref("alpha.example", QUERY)).toBe("/analysis/domains/alpha.example?dataset_tag=tld");
    expect(asnHref(64500, QUERY)).toBe("/analysis/asns/64500?dataset_tag=tld");
  });

  it("encodes path segments that contain reserved characters", () => {
    expect(prefixHref("192.0.2.0/24")).toBe("/analysis/prefixes/192.0.2.0%2F24");
    expect(domainHref("xn--bcher-kva.example")).toBe("/analysis/domains/xn--bcher-kva.example");
  });

  it("endpointHref appends nameserver disambiguator, merging with existing query", () => {
    expect(endpointHref("192.0.2.10", "ns.example")).toBe(
      "/analysis/endpoints/192.0.2.10?nameserver=ns.example"
    );
    expect(endpointHref("192.0.2.10", "ns.example", QUERY)).toBe(
      "/analysis/endpoints/192.0.2.10?dataset_tag=tld&nameserver=ns.example"
    );
    expect(endpointHref("192.0.2.10", null, QUERY)).toBe(
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
    expect(tagHref("DS10_NSEC_QUERY_RESPONSE_ERR", scopedQuery("TLDs", from))).toBe(
      "/analysis/tags/DS10_NSEC_QUERY_RESPONSE_ERR?dataset_tag=TLDs&snapshot=2026-07-14-51ef2053c397"
    );
    expect(tagHref("DS07_NOT_SIGNED", scopedQuery("TLDs", to))).toBe(
      "/analysis/tags/DS07_NOT_SIGNED?dataset_tag=TLDs&snapshot=2026-07-20-cdbee862ffe3"
    );
  });

  it("endpointHref replaces stale nameserver= already in the query", () => {
    // Without replacement the detail page reads the first nameserver= and
    // 404s because it doesn't belong to the row the user clicked.
    expect(
      endpointHref("192.0.2.10", "ns2.example", "?nameserver=ns1.example")
    ).toBe("/analysis/endpoints/192.0.2.10?nameserver=ns2.example");
    expect(
      endpointHref(
        "192.0.2.10",
        "ns3.example",
        "?offset=50&nameserver=ns1.example&nameserver=ns2.example"
      )
    ).toBe("/analysis/endpoints/192.0.2.10?nameserver=ns3.example");
  });

  it("domainsPostureHref filters the domains list on one posture bucket", () => {
    expect(domainsPostureHref("nsec3")).toBe("/analysis/domains?dnssec_posture=nsec3");
    expect(domainsPostureHref("nsec3", QUERY)).toBe(
      "/analysis/domains?dataset_tag=tld&dnssec_posture=nsec3"
    );
    // A stale bucket in the caller's query is replaced, not appended.
    expect(domainsPostureHref("mixed", "?dnssec_posture=nsec")).toBe(
      "/analysis/domains?dnssec_posture=mixed"
    );
  });

  // Every param a page may carry, pinned and unpinned, in mixed order.
  const NOISY = "?grade=A&nameserver=c.nic.example&dataset_tag=tld&sort=grade&snapshot=2026-07-14-51ef2053c397&offset=50";
  const PINNED = "dataset_tag=tld&snapshot=2026-07-14-51ef2053c397";

  it.each([
    ["domainHref", () => domainHref("alpha.example", NOISY), `/analysis/domains/alpha.example?${PINNED}`],
    ["nameserverHref", () => nameserverHref("a.dns.example", NOISY), `/analysis/nameservers/a.dns.example?${PINNED}`],
    ["endpointHref", () => endpointHref("192.0.2.10", null, NOISY), `/analysis/endpoints/192.0.2.10?${PINNED}`],
    [
      "endpointHref with a nameserver",
      () => endpointHref("192.0.2.10", "ns.example", NOISY),
      `/analysis/endpoints/192.0.2.10?${PINNED}&nameserver=ns.example`
    ],
    ["prefixHref", () => prefixHref("192.0.2.0/24", NOISY), `/analysis/prefixes/192.0.2.0%2F24?${PINNED}`],
    ["asnHref", () => asnHref(64500, NOISY), `/analysis/asns/64500?${PINNED}`],
    ["tagHref", () => tagHref("Z01_MNAME_NOT_MASTER", NOISY), `/analysis/tags/Z01_MNAME_NOT_MASTER?${PINNED}`],
    [
      "cohortHref",
      () => cohortHref("gtld", NOISY),
      "/analysis/?dataset_tag=gtld&snapshot=2026-07-14-51ef2053c397"
    ],
    ["domainsGradeHref", () => domainsGradeHref("B", NOISY), `/analysis/domains?${PINNED}&grade=B`],
    [
      "domainsSeverityHref",
      () => domainsSeverityHref("ERROR", NOISY),
      `/analysis/domains?${PINNED}&worst_level=ERROR`
    ],
    [
      "domainsPostureHref",
      () => domainsPostureHref("nsec3", NOISY),
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
