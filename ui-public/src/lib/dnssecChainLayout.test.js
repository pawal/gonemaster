import { describe, it, expect } from "vitest";
import { layoutChain, truncateName, worstSigTone } from "./dnssecChainLayout.js";

// tipParams returns the params of the tip line with the given i18n key.
function tipParams(el, k) {
  const line = (el.tip ?? []).find((l) => l.k === k);
  return line ? line.p : undefined;
}

// hasTip reports whether an element carries a tip line with the given key.
function hasTip(el, k) {
  return (el.tip ?? []).some((l) => l.k === k);
}

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

describe("worstSigTone", () => {
  it("returns bad for expired, bogus, or missing-key states", () => {
    expect(worstSigTone([{ state: "valid" }, { state: "expired" }])).toBe("bad");
    expect(worstSigTone([{ state: "bogus" }])).toBe("bad");
    expect(worstSigTone([{ state: "no_key" }])).toBe("bad");
  });

  it("returns warn for not-yet-valid or unsupported without anything worse", () => {
    expect(worstSigTone([{ state: "valid" }, { state: "not_yet_valid" }])).toBe("warn");
    expect(worstSigTone([{ state: "unsupported_algorithm" }])).toBe("warn");
  });

  it("returns empty for all-valid or no signatures", () => {
    expect(worstSigTone([{ state: "valid" }])).toBe("");
    expect(worstSigTone([])).toBe("");
    expect(worstSigTone(undefined)).toBe("");
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

  it("emits structured, localizable tip lines for nodes and signature edges", () => {
    const chain = secureChain();
    chain.parent.ds[0].digest = "ab34cd";
    chain.child.dnskeys[0].key_size = 2048;
    chain.child.dnskeys[0].zone_key = true;
    chain.child.dnskey_rrsig[0].algorithm = 13;
    chain.child.dnskey_rrsig[0].inception = 1751500800;
    chain.child.dnskey_rrsig[0].expiration = 1752710400;
    const g = layoutChain(chain);

    // Tip lines carry i18n keys plus verbatim protocol tokens as params, so
    // the component can localize labels while key tags and dates stay literal.
    const ds = g.nodes.find((n) => n.kind === "ds");
    expect(tipParams(ds, "pub.dnssec_chain_tip_algorithm").algo).toBe("ECDSAP256SHA256 (alg 13)");
    expect(tipParams(ds, "pub.dnssec_chain_tip_digest_type").dt).toBe("SHA-256 (2)");
    expect(tipParams(ds, "pub.dnssec_chain_tip_digest").digest).toBe("ab34cd");

    const ksk = g.nodes.find((n) => n.kind === "ksk");
    expect(tipParams(ksk, "pub.dnssec_chain_tip_flags").flags).toBe("257 (ZONE, SEP)");
    expect(tipParams(ksk, "pub.dnssec_chain_tip_key_size").bits).toBe(2048);

    const self = g.edges.find((e) => e.kind === "selfsig");
    expect(tipParams(self, "pub.dnssec_chain_tip_signing_key").tag).toBe(1000);
    const valid = self.tip.find((l) => l.k === "pub.dnssec_chain_tip_valid");
    expect(valid.p).toEqual({ from: "2025-07-03", to: "2025-07-17" });
    const st = self.tip.find((l) => l.k === "pub.dnssec_chain_tip_status");
    expect(st.statusState).toBe("valid");
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
      { type: "CDS", rrsig: [{ key_tag: 1000, algorithm: 13, state: "valid", servers: ["203.0.113.1"] }], refs: [1000] },
    ];
    const g = layoutChain(chain);
    expect(g.nodes.some((n) => n.id === "rrset-SOA" && n.label === "SOA")).toBe(true);
    expect(g.nodes.some((n) => n.id === "rrset-CDS" && n.label === "CDS")).toBe(true);
    // ZSK signs SOA, KSK signs CDS. Edge ids end with the inception (0 when
    // the fixture sets none) so overlapping signatures stay distinct.
    expect(g.edges.some((e) => e.id === "sig-SOA-2000-0")).toBe(true);
    expect(g.edges.some((e) => e.id === "sig-CDS-1000-0")).toBe(true);
    // CDS names the KSK (key tag 1000): a grey reference edge points to it,
    // drawn as a bowed path so it clears the signature edge.
    const ref = g.edges.find((e) => e.kind === "ref" && e.id === "ref-CDS-1000");
    expect(ref).toBeTruthy();
    expect(ref.targetTag).toBe(1000);
    expect(typeof ref.d).toBe("string");
  });

  it("flags a CDS rollover node and tints the ref edge to the unanchored key", () => {
    const chain = secureChain();
    // CDS names the anchored KSK 1000 plus an incoming key 3000 with no DS.
    chain.child.dnskeys.push({ key_tag: 3000, algorithm: 13, flags: 257, sep: true, servers: ["203.0.113.1"] });
    chain.child.signed = [
      { type: "CDS", rrsig: [{ key_tag: 1000, state: "valid" }], refs: [1000, 3000], ds_match: "rollover", new_keys: [3000] },
    ];
    const g = layoutChain(chain);
    const cds = g.nodes.find((n) => n.id === "rrset-CDS");
    expect(cds.rollover).toBe(true);
    // Ref edge to the unanchored key 3000 is marked pending; the one to 1000 is not.
    const pending = g.edges.find((e) => e.kind === "ref" && e.targetTag === 3000);
    const anchored = g.edges.find((e) => e.kind === "ref" && e.targetTag === 1000);
    expect(pending.rollover).toBe(true);
    expect(anchored.rollover).toBe(false);
  });

  it("does not flag CDS as rollover when it matches the DS", () => {
    const chain = secureChain();
    chain.child.signed = [
      { type: "CDS", rrsig: [{ key_tag: 1000, state: "valid" }], refs: [1000], ds_match: "match" },
    ];
    const g = layoutChain(chain);
    const cds = g.nodes.find((n) => n.id === "rrset-CDS");
    expect(cds.rollover).toBe(false);
    const ref = g.edges.find((e) => e.kind === "ref");
    expect(ref.rollover).toBe(false);
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

  it("tints DS nodes by the worst covering-signature state", () => {
    const chain = secureChain();
    chain.parent.ds_rrsig = [
      { key_tag: 5000, state: "valid" },
      { key_tag: 5000, state: "expired" },
    ];
    const g = layoutChain(chain);
    const ds = g.nodes.find((n) => n.kind === "ds");
    expect(ds.dsSigTone).toBe("bad");
  });

  it("leaves DS nodes untinted when the covering signature is valid", () => {
    const chain = secureChain();
    chain.parent.ds_rrsig = [{ key_tag: 5000, state: "valid" }];
    const g = layoutChain(chain);
    const ds = g.nodes.find((n) => n.kind === "ds");
    expect(ds.dsSigTone).toBe("");
  });

  it("marks an unanchored KSK incoming and de-emphasizes its edges", () => {
    // Double-signature KSK rollover: KSK 1000 is anchored by a DS; KSK 3000
    // signs the DNSKEY RRset but has no DS yet, so it is an incoming key.
    const chain = secureChain();
    chain.child.dnskeys = [
      { key_tag: 1000, algorithm: 13, flags: 257, sep: true, anchored: true, servers: ["203.0.113.1"] },
      { key_tag: 3000, algorithm: 13, flags: 257, sep: true, servers: ["203.0.113.1"] },
      { key_tag: 2000, algorithm: 13, flags: 256, sep: false, servers: ["203.0.113.1"] },
    ];
    chain.child.dnskey_rrsig = [
      { key_tag: 1000, algorithm: 13, state: "valid" },
      { key_tag: 3000, algorithm: 13, state: "valid" },
    ];
    const g = layoutChain(chain);
    const anchored = g.nodes.find((n) => n.id === "key-1000");
    const incoming = g.nodes.find((n) => n.id === "key-3000");
    expect(anchored.incoming).toBe(false);
    expect(incoming.incoming).toBe(true);
    // The incoming KSK's self-loop and vouching edges are flagged incoming.
    const incomingEdges = g.edges.filter((e) => (e.kind === "selfsig" || e.kind === "keysig") && e.keyTag === 3000);
    expect(incomingEdges.length).toBeGreaterThan(0);
    expect(incomingEdges.every((e) => e.incoming === true)).toBe(true);
    // The anchored KSK's edges are not.
    const anchoredEdges = g.edges.filter((e) => (e.kind === "selfsig" || e.kind === "keysig") && e.keyTag === 1000);
    expect(anchoredEdges.every((e) => e.incoming === false)).toBe(true);
  });

  it("does not mark a KSK incoming when no key is anchored", () => {
    // With no anchored key at all the zone is broken/island, not rolling.
    const chain = secureChain();
    chain.child.dnskeys = [
      { key_tag: 1000, algorithm: 13, flags: 257, sep: true, servers: ["203.0.113.1"] },
      { key_tag: 2000, algorithm: 13, flags: 256, sep: false, servers: ["203.0.113.1"] },
    ];
    const g = layoutChain(chain);
    expect(g.nodes.every((n) => !n.incoming)).toBe(true);
  });

  it("flags revoked keys on their node", () => {
    const chain = secureChain();
    chain.child.dnskeys[0].revoked = true;
    const g = layoutChain(chain);
    const ksk = g.nodes.find((n) => n.kind === "ksk");
    const zsk = g.nodes.find((n) => n.kind === "zsk");
    expect(ksk.revoked).toBe(true);
    expect(zsk.revoked).toBe(false);
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
    const input = g.nodes.find((n) => n.kind === "ds-input");
    expect(input).toBeTruthy();
    // The "-" placeholder for input DS must not render as a servers line.
    expect(hasTip(input, "pub.dnssec_chain_tip_servers")).toBe(false);
    expect(hasTip(input, "pub.dnssec_chain_tip_ds_input")).toBe(true);
  });

  it("keeps dual-digest DS records for one key tag as distinct nodes and edges", () => {
    // A parent commonly publishes SHA-256 and SHA-384 DS records for the same
    // KSK. Both must get unique node and edge ids or Svelte's keyed each
    // blocks throw on duplicates and the graph fails to render.
    const chain = secureChain();
    chain.parent.ds = [
      { key_tag: 1000, algorithm: 13, digest_type: 2, digest: "ab", servers: ["192.0.2.1"] },
      { key_tag: 1000, algorithm: 13, digest_type: 4, digest: "cd", servers: ["192.0.2.1"] },
    ];
    chain.links = [
      { ds_key_tag: 1000, ds_digest_type: 2, dnskey_key_tag: 1000, status: "match", servers: ["203.0.113.1"] },
      { ds_key_tag: 1000, ds_digest_type: 4, dnskey_key_tag: 1000, status: "match", servers: ["203.0.113.1"] },
    ];
    const g = layoutChain(chain);
    const dsNodes = g.nodes.filter((n) => n.kind === "ds");
    expect(dsNodes).toHaveLength(2);
    const dsEdges = g.edges.filter((e) => e.kind === "ds");
    expect(dsEdges).toHaveLength(2);
    const nodeIds = new Set(g.nodes.map((n) => n.id));
    const edgeIds = new Set(g.edges.map((e) => e.id));
    expect(nodeIds.size).toBe(g.nodes.length);
    expect(edgeIds.size).toBe(g.edges.length);
    // Each edge starts at its own DS node, not both at the first one.
    expect(dsEdges[0].from.x).not.toBe(dsEdges[1].from.x);
  });

  it("keeps overlapping signatures by the same key as distinct edges", () => {
    // During re-signing a zone serves two RRSIGs by the same key with
    // different validity windows; their edges need unique ids.
    const chain = secureChain();
    chain.child.dnskey_rrsig = [
      { key_tag: 1000, algorithm: 13, state: "valid", inception: 100, expiration: 200, servers: ["203.0.113.1"] },
      { key_tag: 1000, algorithm: 13, state: "expired", inception: 1, expiration: 99, servers: ["203.0.113.1"] },
    ];
    const g = layoutChain(chain);
    const selfLoops = g.edges.filter((e) => e.kind === "selfsig");
    expect(selfLoops).toHaveLength(2);
    const edgeIds = new Set(g.edges.map((e) => e.id));
    expect(edgeIds.size).toBe(g.edges.length);
  });

  it("draws a phantom key node for a DS naming an absent key, with an edge to it", () => {
    // Stale DS after a rollover: keytag 1000 has a DS but is gone from the
    // DNSKEY RRset. A grey phantom key node stands in for it and the broken
    // DS edge points at it, rather than the DS floating with no target.
    const chain = secureChain();
    chain.links = [{ ds_key_tag: 1000, ds_digest_type: 2, status: "no_dnskey", servers: ["192.0.2.1"] }];
    chain.child.dnskeys = [{ key_tag: 2000, algorithm: 13, flags: 256, sep: false, servers: ["203.0.113.1"] }];
    const g = layoutChain(chain);
    const phantom = g.nodes.find((n) => n.id === "key-1000");
    expect(phantom.kind).toBe("key-phantom");
    expect(phantom.keyTag).toBe(1000);
    const edge = g.edges.find((e) => e.kind === "ds");
    expect(edge.status).toBe("no_dnskey");
  });

  it("draws a phantom key node for a CDS/CDNSKEY naming a not-yet-published key", () => {
    // CDS/CDNSKEY signal an incoming key 30169 that is not in the DNSKEY RRset;
    // it becomes a phantom node so the reference edge has a target.
    const chain = secureChain();
    chain.child.signed = [
      { type: "CDNSKEY", rrsig: [{ key_tag: 1000, state: "valid" }], refs: [30169], ds_match: "rollover", new_keys: [30169] },
    ];
    const g = layoutChain(chain);
    const phantom = g.nodes.find((n) => n.id === "key-30169");
    expect(phantom.kind).toBe("key-phantom");
    const ref = g.edges.find((e) => e.kind === "ref" && e.targetTag === 30169);
    expect(ref.rollover).toBe(true);
  });

  it("does not create phantom nodes for keys that are present", () => {
    const g = layoutChain(secureChain());
    expect(g.nodes.some((n) => n.kind === "key-phantom")).toBe(false);
  });

  it("resolves links without ds_digest_type to the DS node by key tag", () => {
    // Blobs stored before links carried ds_digest_type still draw their edge.
    const chain = secureChain();
    const g = layoutChain(chain);
    const dsEdge = g.edges.find((e) => e.kind === "ds");
    expect(dsEdge).toBeTruthy();
    expect(dsEdge.status).toBe("match");
  });
});
