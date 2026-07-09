import { describe, expect, it } from "vitest";
import { canonicalUrl, routeHead } from "./head";

describe("routeHead", () => {
  it("gives the overview a site-branded title and the default description", () => {
    const head = routeHead("/", {});
    expect(head.title).toBe("Gonemaster Analysis - DNS Health");
    expect(head.description).toContain("DNS");
  });

  it("titles list sections with the section name and site suffix", () => {
    expect(routeHead("/domains", {}).title).toBe("Domains - Gonemaster Analysis");
    expect(routeHead("/nameservers", {}).title).toBe("Nameservers - Gonemaster Analysis");
    expect(routeHead("/tags", {}).title).toBe("Tags - Gonemaster Analysis");
  });

  it("uses the route parameter as the title for entity detail pages", () => {
    // Detail pages have their entity as the first path segment so browser
    // tabs, bookmarks, and JS-rendering crawlers all identify the subject.
    expect(routeHead("/domains/[domain]", { domain: "example.com" }).title).toBe(
      "example.com - Gonemaster Analysis"
    );
    expect(routeHead("/nameservers/[name]", { name: "ns1.example.net" }).title).toBe(
      "ns1.example.net - Gonemaster Analysis"
    );
    expect(routeHead("/tags/[tag]", { tag: "NS_ERROR" }).title).toBe(
      "NS_ERROR - Gonemaster Analysis"
    );
  });

  it("prefixes ASN detail titles with AS to match the on-page heading", () => {
    const head = routeHead("/asns/[asn]", { asn: "64500" });
    expect(head.title).toBe("AS64500 - Gonemaster Analysis");
    expect(head.description).toContain("AS64500");
  });

  it("embeds the entity in detail-page descriptions", () => {
    expect(routeHead("/domains/[domain]", { domain: "example.com" }).description).toContain(
      "example.com"
    );
    expect(routeHead("/prefixes/[prefix]", { prefix: "192.0.2.0/24" }).description).toContain(
      "192.0.2.0/24"
    );
  });

  it("falls back to the bare site name for unknown routes", () => {
    expect(routeHead(null, {}).title).toBe("Gonemaster Analysis");
    expect(routeHead("/does-not-exist", {}).title).toBe("Gonemaster Analysis");
  });
});

describe("canonicalUrl", () => {
  it("drops the query string when there are no view-selecting params", () => {
    const url = new URL("https://example.com/analysis/domains?sort=grade&offset=40&search=foo");
    expect(canonicalUrl(url)).toBe("https://example.com/analysis/domains");
  });

  it("keeps only dataset_tag and snapshot, dropping volatile UI state", () => {
    // dataset_tag and snapshot pick which data view is shown, so they define
    // distinct canonical pages; sort/limit/offset/search are pagination noise
    // that would otherwise multiply into duplicate URLs (a crawler trap).
    const url = new URL(
      "https://example.com/analysis/tags?dataset_tag=tld&snapshot=2026-07-01&sort=count&limit=50&offset=100"
    );
    expect(canonicalUrl(url)).toBe(
      "https://example.com/analysis/tags?dataset_tag=tld&snapshot=2026-07-01"
    );
  });

  it("preserves the path for entity detail pages", () => {
    const url = new URL("https://example.com/analysis/domains/example.com?dataset_tag=tld&x=1");
    expect(canonicalUrl(url)).toBe(
      "https://example.com/analysis/domains/example.com?dataset_tag=tld"
    );
  });
});
