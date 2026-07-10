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
      soa_rrsig: [],
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

  it("builds nodes for a secure chain with SEP key first", () => {
    const g = layoutChain(secureChain());
    expect(g).not.toBeNull();

    const ds = g.nodes.filter((n) => n.kind === "ds");
    expect(ds).toHaveLength(1);
    expect(ds[0].keyTag).toBe(1000);

    const keys = g.nodes.filter((n) => n.kind === "ksk" || n.kind === "zsk");
    expect(keys).toHaveLength(2);
    expect(keys[0].kind).toBe("ksk"); // SEP first

    const rrset = g.nodes.filter((n) => n.kind === "rrset");
    expect(rrset).toHaveLength(1); // DNSKEY only, no SOA sig
    expect(rrset[0].label).toBe("DNSKEY");
  });

  it("produces a matching DS edge and a valid signature edge", () => {
    const g = layoutChain(secureChain());
    const dsEdge = g.edges.find((e) => e.kind === "ds");
    expect(dsEdge.status).toBe("match");

    const sigEdge = g.edges.find((e) => e.kind === "sig");
    expect(sigEdge.status).toBe("valid");
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

  it("adds a SOA node and edge when a SOA signature exists", () => {
    const chain = secureChain();
    chain.child.soa_rrsig = [{ key_tag: 2000, algorithm: 13, state: "valid", servers: ["203.0.113.1"] }];
    const g = layoutChain(chain);
    const soa = g.nodes.find((n) => n.id === "rrset-soa");
    expect(soa).toBeTruthy();
    expect(g.edges.some((e) => e.id === "sig-soa-2000")).toBe(true);
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
      child: { dnskeys: [], dnskey_rrsig: [], soa_rrsig: [] },
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
