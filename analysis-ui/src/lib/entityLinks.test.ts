import { describe, expect, it } from "vitest";
import {
  asnHref,
  cohortHref,
  domainHref,
  endpointHref,
  nameserverHref,
  prefixHref,
  tagHref,
  testcaseHref
} from "./entityLinks";

const BASE = "/analysis";
const QUERY = "?dataset_tag=tld";

describe("entity link builders", () => {
  it("builds paths under the SvelteKit base", () => {
    expect(domainHref(BASE, "alpha.example")).toBe("/analysis/domains/alpha.example");
    expect(nameserverHref(BASE, "ns.example")).toBe("/analysis/nameservers/ns.example");
    expect(asnHref(BASE, 64500)).toBe("/analysis/asns/64500");
    expect(tagHref(BASE, "DS07_NOT_SIGNED")).toBe("/analysis/tags/DS07_NOT_SIGNED");
    expect(testcaseHref(BASE, "dnssec07")).toBe("/analysis/testcases/dnssec07");
    expect(cohortHref(BASE, "tld")).toBe("/analysis/cohorts/tld");
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
});
