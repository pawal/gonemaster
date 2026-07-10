import { describe, it, expect } from "vitest";
import { layoutChain, truncateName } from "./dnssecChainLayout.js";

// secureChain builds a minimal but complete "secure" summary: one DS matching a
// KSK, plus a ZSK, with a valid DNSKEY signature.
function secureChain(overrides = {}) {
  return {
    version: 1,
    zone: "example.com",
    parent_zone: "com",
    delegation: "normal",
    status: "secure",
    parent: {
      ds_source: "parent",
      ds: [{ key_tag: 1000, algorithm: 13, digest_type: 2, digest: "ab", servers: ["192.0.2.1"] }],
    },
    child: {
      dnskeys: [
        { key_tag: 1000, algorithm: 13, flags: 257, sep: true, servers: ["203.0.113.1"] },
        { key_tag: 2000, algorithm: 13, flags: 256, sep: false, servers: ["203.0.113.1"] },
      ],
      dnskey_rrsig: [{ key_tag: 1000, algorithm: 13, state: "valid", servers: ["203.0.113.1"] }],
      signed: [],
    },
    links: [{ ds_key_tag: 1000, dnskey_key_tag: 1000, status: "match", servers: ["203.0.113.1"] }],
    ...overrides,
  };
}

describe("truncateName", () => {
  it("leaves short names untouched", () => {
    expect(truncateName("example.com")).toBe("example.com");
  });

  it("middle-ellipsizes long names", () => {
    const out = truncateName("verylongsubdomain.example.com", 20);
    expect(out.length).toBe(20);
    expect(out).toContain("…");
    expect(out.startsWith("very")).toBe(true);
    expect(out.endsWith(".com")).toBe(true);
  });

  it("handles null and undefined", () => {
    expect(truncateName(null)).toBe("");
    expect(truncateName(undefined)).toBe("");
  });
});

describe("layoutChain", () => {
  it("returns null for an unsigned zone", () => {
    expect(layoutChain({ status: "unsigned" })).toBeNull();
  });

  it("returns null for nullish input", () => {
    expect(layoutChain(null)).toBeNull();
    expect(layoutChain(undefined)).toBeNull();
  });

  it("builds nodes with the KSK row above the ZSK row", () => {
    const g = layoutChain(secureChain());
    expect(g).not.toBeNull();

    const ds = g.nodes.filter((n) => n.kind === "ds");
    expect(ds).toHaveLength(1);
    expect(ds[0].keyTag).toBe(1000);

    const ksk = g.nodes.find((n) => n.kind === "ksk");
    const zsk = g.nodes.find((n) => n.kind === "zsk");
    expect(ksk).toBeTruthy();
    expect(zsk).toBeTruthy();
    // KSK sits in a row above the ZSK.
    expect(ksk.rowIndex).toBeLessThan(zsk.rowIndex);

    // No abstract DNSKEY-RRset node: signing is shown with edges, not a box.
    expect(g.nodes.some((n) => n.id === "rrset-dnskey")).toBe(false);
  });

  it("labels the parent and key clusters with their zone names", () => {
    const g = layoutChain(secureChain());
    const parent = g.clusters.find((c) => c.id === "parent");
    const keys = g.clusters.find((c) => c.id === "keys");
    expect(parent.name).toBe("com");
    expect(keys.name).toBe("example.com");
    // No signed-records row without a zone-data signature.
    expect(g.clusters.some((c) => c.id === "signed")).toBe(false);
  });

  it("keeps the root parent name as a dot, not an em dash", () => {
    const chain = secureChain({ parent_zone: "." });
    const g = layoutChain(chain);
    const parent = g.clusters.find((c) => c.id === "parent");
    expect(parent.name).toBe(".");
  });

  it("produces a matching DS edge and a self-signing KSK loop", () => {
    const g = layoutChain(secureChain());
    const dsEdge = g.edges.find((e) => e.kind === "ds");
    expect(dsEdge.status).toBe("match");
    expect(dsEdge.dnskeyKeyTag).toBe(1000);

    // The KSK signs the DNSKEY RRset: a self-loop plus a "signs" edge to the ZSK.
    const selfLoop = g.edges.find((e) => e.kind === "selfsig" && e.keyTag === 1000);
    expect(selfLoop).toBeTruthy();
    expect(selfLoop.status).toBe("valid");
    expect(typeof selfLoop.d).toBe("string");

    const keySig = g.edges.find((e) => e.kind === "keysig" && e.keyTag === 1000 && e.targetTag === 2000);
    expect(keySig).toBeTruthy();
  });

  it("keeps all node coordinates within the viewBox", () => {
    const g = layoutChain(secureChain());
    for (const n of g.nodes) {
      expect(n.x).toBeGreaterThanOrEqual(0);
      expect(n.y).toBeGreaterThanOrEqual(0);
      expect(n.x + n.w).toBeLessThanOrEqual(g.width);
      expect(n.y + n.h).toBeLessThanOrEqual(g.height);
    }
  });

  it("grows in height only vertically as key count rises", () => {
    const one = layoutChain(secureChain());
    const many = secureChain();
    for (let i = 0; i < 6; i++) {
      many.child.dnskeys.push({ key_tag: 3000 + i, algorithm: 13, flags: 256, sep: false, servers: ["203.0.113.1"] });
    }
    const g = layoutChain(many);
    // More keys widen the graph but do not change the number of rows (height).
    expect(g.width).toBeGreaterThan(one.width);
    expect(g.height).toBe(one.height);
  });

  it("adds a node and edge per signed RRset (SOA, CDS)", () => {
    const chain = secureChain();
    chain.child.signed = [
      { type: "SOA", rrsig: [{ key_tag: 2000, algorithm: 13, state: "valid", servers: ["203.0.113.1"] }] },
      { type: "CDS", rrsig: [{ key_tag: 1000, algorithm: 13, state: "valid", servers: ["203.0.113.1"] }] },
    ];
    const g = layoutChain(chain);
    expect(g.nodes.some((n) => n.id === "rrset-SOA" && n.label === "SOA")).toBe(true);
    expect(g.nodes.some((n) => n.id === "rrset-CDS" && n.label === "CDS")).toBe(true);
    // ZSK signs SOA, KSK signs CDS.
    expect(g.edges.some((e) => e.id === "sig-SOA-2000")).toBe(true);
    expect(g.edges.some((e) => e.id === "sig-CDS-1000")).toBe(true);
  });

  it("draws a ghost DS node for an island (keys, no DS)", () => {
    const chain = secureChain({
      status: "island",
      parent: { ds_source: "none", ds: [] },
      links: [],
    });
    const g = layoutChain(chain);
    expect(g.nodes.some((n) => n.kind === "ds-ghost")).toBe(true);
    expect(g.nodes.some((n) => n.kind === "ksk")).toBe(true);
  });

  it("draws a ghost DNSKEY node when DS exists but no key", () => {
    const chain = secureChain({
      status: "broken",
      child: { dnskeys: [], dnskey_rrsig: [], signed: [] },
      links: [{ ds_key_tag: 1000, status: "no_dnskey", servers: ["192.0.2.1"] }],
    });
    const g = layoutChain(chain);
    expect(g.nodes.some((n) => n.kind === "key-ghost")).toBe(true);
    const edge = g.edges.find((e) => e.kind === "ds");
    expect(edge.status).toBe("no_dnskey");
  });

  it("marks input-provided DS nodes distinctly", () => {
    const chain = secureChain({
      delegation: "undelegated",
      parent: {
        ds_source: "input",
        ds: [{ key_tag: 1000, algorithm: 13, digest_type: 2, digest: "ab", servers: ["-"] }],
      },
    });
    const g = layoutChain(chain);
    expect(g.nodes.some((n) => n.kind === "ds-input")).toBe(true);
  });
});
