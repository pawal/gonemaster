import { describe, expect, it } from "vitest";
import {
  addressLinks,
  asnLinks,
  domainLinks,
  isTLD,
  nameserverLinks,
  prefixLinks
} from "./externalLinks";

function hrefFor(links: { label: string; href: string }[], label: string): string {
  return links.find((l) => l.label === label)?.href ?? "";
}

describe("isTLD", () => {
  it.each([
    ["se", true],
    ["se.", true],
    ["SE", true],
    ["xn--mgbaam7a8h", true],
    ["example.se", false],
    ["a.b.example.se", false],
    [".", false],
    ["", false],
    ["   ", false]
  ])("isTLD(%j) === %s", (input, want) => {
    expect(isTLD(input)).toBe(want);
  });
});

describe("domainLinks", () => {
  it("links a TLD to IANA, ICANNWiki and the IANA RDAP service", () => {
    const links = domainLinks("se");
    expect(links.map((l) => l.label)).toEqual(["IANA root zone", "ICANNWiki", "RDAP"]);
    expect(hrefFor(links, "IANA root zone")).toBe("https://www.iana.org/domains/root/db/se.html");
    expect(hrefFor(links, "ICANNWiki")).toBe("https://icannwiki.org/.se");
    expect(hrefFor(links, "RDAP")).toBe("https://rdap.iana.org/domain/se");
  });

  it("normalizes case and a trailing root dot before building hrefs", () => {
    expect(hrefFor(domainLinks("SE."), "IANA root zone")).toBe(
      "https://www.iana.org/domains/root/db/se.html"
    );
  });

  it("uses the U-label for ICANNWiki and the A-label everywhere else", () => {
    // ICANNWiki keys its pages on the Unicode form of the TLD.
    const links = domainLinks("xn--mgbaam7a8h");
    expect(hrefFor(links, "ICANNWiki")).toBe(
      `https://icannwiki.org/.${encodeURIComponent("امارات")}`
    );
    expect(hrefFor(links, "IANA root zone")).toBe(
      "https://www.iana.org/domains/root/db/xn--mgbaam7a8h.html"
    );
    expect(hrefFor(links, "RDAP")).toBe("https://rdap.iana.org/domain/xn--mgbaam7a8h");
  });

  it("links a second-level domain only to the RDAP web client", () => {
    const links = domainLinks("example.se");
    expect(links.map((l) => l.label)).toEqual(["RDAP"]);
    expect(hrefFor(links, "RDAP")).toBe("https://client.rdap.org/?type=domain&object=example.se");
  });

  it("prefers a server-supplied direct RDAP url over the web client", () => {
    const links = domainLinks("example.se", {
      rdapUrl: "https://rdap.example-registry.se/domain/example.se"
    });
    expect(hrefFor(links, "RDAP")).toBe("https://rdap.example-registry.se/domain/example.se");
  });

  it("falls back to the web client when the supplied url is empty or absent", () => {
    expect(hrefFor(domainLinks("example.se", { rdapUrl: "" }), "RDAP")).toContain(
      "client.rdap.org"
    );
    expect(hrefFor(domainLinks("example.se", { rdapUrl: null }), "RDAP")).toContain(
      "client.rdap.org"
    );
  });

  it("prefers a direct RDAP url for a TLD while keeping the reference pages", () => {
    const links = domainLinks("se", { rdapUrl: "https://rdap.iana.org/domain/se?direct" });
    expect(links.map((l) => l.label)).toEqual(["IANA root zone", "ICANNWiki", "RDAP"]);
    expect(hrefFor(links, "RDAP")).toBe("https://rdap.iana.org/domain/se?direct");
  });

  it("percent-encodes a hostile name instead of letting it shape the url", () => {
    const href = hrefFor(domainLinks("evil.example/../../etc?x=1#f"), "RDAP");
    expect(href).toBe(
      "https://client.rdap.org/?type=domain&object=evil.example%2F..%2F..%2Fetc%3Fx%3D1%23f"
    );
    // The encoded name stays inside the object parameter.
    expect(new URL(href).pathname).toBe("/");
  });

  it("returns nothing for an empty name", () => {
    expect(domainLinks("")).toEqual([]);
    expect(domainLinks(".")).toEqual([]);
  });
});

describe("asnLinks", () => {
  it("links an ASN to RIPEstat, accepting both number and AS-prefixed forms", () => {
    expect(hrefFor(asnLinks(64500), "RIPEstat")).toBe("https://stat.ripe.net/AS64500");
    expect(hrefFor(asnLinks("64500"), "RIPEstat")).toBe("https://stat.ripe.net/AS64500");
    expect(hrefFor(asnLinks("AS64500"), "RIPEstat")).toBe("https://stat.ripe.net/AS64500");
  });

  it("returns nothing for a non-numeric ASN", () => {
    expect(asnLinks("not-an-asn")).toEqual([]);
    expect(asnLinks("")).toEqual([]);
  });
});

describe("prefixLinks and addressLinks", () => {
  it("link the resource to RIPEstat with reserved characters encoded", () => {
    expect(hrefFor(prefixLinks("192.0.2.0/24"), "RIPEstat")).toBe(
      "https://stat.ripe.net/192.0.2.0%2F24"
    );
    expect(hrefFor(addressLinks("2001:db8::1"), "RIPEstat")).toBe(
      "https://stat.ripe.net/2001%3Adb8%3A%3A1"
    );
  });

  it("return nothing for an empty resource", () => {
    expect(prefixLinks("")).toEqual([]);
    expect(addressLinks("  ")).toEqual([]);
  });
});

describe("nameserverLinks", () => {
  it("has no institutional reference to link yet", () => {
    expect(nameserverLinks("ns1.example")).toEqual([]);
  });
});

describe("link metadata", () => {
  it("gives every link a label, an https href and a title", () => {
    const all = [
      ...domainLinks("se"),
      ...domainLinks("example.se"),
      ...asnLinks(64500),
      ...prefixLinks("192.0.2.0/24"),
      ...addressLinks("192.0.2.1")
    ];
    expect(all.length).toBeGreaterThan(0);
    for (const link of all) {
      expect(link.label).not.toBe("");
      expect(link.title).not.toBe("");
      expect(link.href.startsWith("https://")).toBe(true);
    }
  });
});
