import { describe, it, expect } from "vitest";
import { secureChain } from "../test/helpers.js";
import { layoutChain, relativeName, truncateName, worstSigTone, worstSigState, algoMnemonic, algoFace, bitsFace, ALGO_FACE_MAX } from "./dnssecChainLayout.js";

// tipParams returns the params of the tip line with the given i18n key.
function tipParams(el, k) {
  const line = (el.tip ?? []).find((l) => l.k === k);
  return line ? line.p : undefined;
}

// hasTip reports whether an element carries a tip line with the given key.
function hasTip(el, k) {
  return (el.tip ?? []).some((l) => l.k === k);
}

describe("algoMnemonic", () => {
  // The diagram keeps its own algorithm table, so a new algorithm reads as a
  // bare number until it is added here. ML-DSA-44 shipped verifying but shown
  // as "alg 18", which looks unsupported.
  it("names ML-DSA-44 instead of showing a bare algorithm number", () => {
    expect(algoMnemonic(18)).toBe("MLDSA44");

    const chain = secureChain();
    chain.parent.ds[0].algorithm = 18;
    chain.child.dnskeys[0].algorithm = 18;
    const g = layoutChain(chain);

    const ds = g.nodes.find((n) => n.kind === "ds");
    expect(tipParams(ds, "pub.dnssec_chain_tip_algorithm").algo).toBe("MLDSA44 (alg 18)");
  });

  // Unassigned numbers have no mnemonic, so the raw value is the right
  // fallback, not a missing entry.
  it("falls back to the raw number for an unassigned algorithm", () => {
    expect(algoMnemonic(19)).toBe("19");
  });
});

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
    // An unsupported RSA exponent is unproven, not invalid: a caution, not bad.
    expect(worstSigTone([{ state: "unsupported_key" }])).toBe("warn");
    expect(worstSigTone([{ state: "valid" }, { state: "unsupported_key" }])).toBe("warn");
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

  it("draws the parent key above the DS with a signing edge, when present", () => {
    const chain = secureChain();
    chain.parent.dnskeys = [{ key_tag: 5000, algorithm: 13, flags: 256, key_size: 1024, ttl: 3600, servers: ["192.0.2.1"] }];
    chain.parent.ds_rrsig = [{ key_tag: 5000, algorithm: 13, state: "valid", inception: 100, expiration: 200, servers: ["192.0.2.1"] }];
    const g = layoutChain(chain);
    const pk = g.nodes.find((n) => n.id === "pkey-5000");
    expect(pk.kind).toBe("parent-key");
    const ds = g.nodes.find((n) => n.kind === "ds");
    // Parent key sits above the DS.
    expect(pk.y).toBeLessThan(ds.y);
    // A signing edge runs from the parent key down to the DS.
    const edge = g.edges.find((e) => e.kind === "keysig" && e.keyTag === 5000);
    expect(edge).toBeTruthy();
    expect(edge.status).toBe("valid");
    expect(edge.from.y).toBeLessThan(edge.to.y);
    // The parent cluster label is on the parent-key row.
    const parent = g.clusters.find((c) => c.id === "parent");
    expect(parent.y).toBeLessThan(ds.y);
  });

  it("keeps the DS as the parent row when the parent key is unknown", () => {
    const g = layoutChain(secureChain());
    expect(g.nodes.some((n) => n.kind === "parent-key")).toBe(false);
    const parent = g.clusters.find((c) => c.id === "parent");
    const ds = g.nodes.find((n) => n.kind === "ds");
    // The parent label sits on the DS row (no separate parent-key row).
    expect(parent.y).toBeLessThan(ds.y);
    expect(parent.y).toBeGreaterThan(ds.y - 40);
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

  it("carries an algorithm_mismatch DS link through to the edge and tip", () => {
    // A DS whose algorithm field disagrees with the DNSKEY it points at is
    // unusable for validators (RFC 4034 section 5.2). The layout must pass
    // the status through untouched: DnssecChain.svelte colors any non-match
    // DS edge as bad, and the tip localizes the status via
    // pub.dnssec_chain_linkstatus_algorithm_mismatch.
    const g = layoutChain(secureChain({
      status: "broken",
      parent: {
        ds_source: "parent",
        ds: [{ key_tag: 1000, algorithm: 253, digest_type: 2, digest: "ab", servers: ["192.0.2.1"] }],
      },
      links: [{ ds_key_tag: 1000, dnskey_key_tag: 1000, status: "algorithm_mismatch", servers: ["203.0.113.1"] }],
    }));
    const dsEdge = g.edges.find((e) => e.kind === "ds");
    expect(dsEdge.status).toBe("algorithm_mismatch");
    const statusLine = dsEdge.tip.find((l) => l.k === "pub.dnssec_chain_tip_status");
    expect(statusLine.linkState).toBe("algorithm_mismatch");
  });

  it("prefers the matching link when a mismatched sibling DS shares the key tag", () => {
    // Two DS records for the same key tag, one usable and one with a wrong
    // algorithm field: the single collapsed edge must stay a match, since a
    // validator needs only one usable DS.
    const g = layoutChain(secureChain({
      links: [
        { ds_key_tag: 1000, dnskey_key_tag: 1000, status: "algorithm_mismatch", servers: ["203.0.113.1"] },
        { ds_key_tag: 1000, dnskey_key_tag: 1000, status: "match", servers: ["203.0.113.1"] },
      ],
    }));
    const dsEdges = g.edges.filter((e) => e.kind === "ds");
    expect(dsEdges.length).toBe(1);
    expect(dsEdges[0].status).toBe("match");
  });

  it("ships a localized label for the algorithm_mismatch link status in every locale", async () => {
    const locales = ["cs", "da", "de", "en", "es", "fi", "fr", "ja", "nb", "nl", "sl", "sv"];
    for (const loc of locales) {
      const catalog = (await import(`../i18n/${loc}.json`)).default;
      const val = catalog["pub.dnssec_chain_linkstatus_algorithm_mismatch"];
      expect(typeof val, loc).toBe("string");
      expect(val.length > 0, loc).toBe(true);
    }
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
    // ZSK signs SOA, KSK signs CDS. One edge per signer and RRset: every
    // signature that key made over the RRset shares the same path.
    expect(g.edges.some((e) => e.id === "sig-SOA-2000")).toBe(true);
    expect(g.edges.some((e) => e.id === "sig-CDS-1000")).toBe(true);
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

  it("adds a TTL tip line to DS, DNSKEY, and signed-RRset nodes when present", () => {
    const chain = secureChain();
    chain.parent.ds[0].ttl = 86400;
    chain.child.dnskeys[0].ttl = 3600;
    chain.child.signed = [{ type: "SOA", ttl: 900, rrsig: [{ key_tag: 2000, state: "valid" }] }];
    const g = layoutChain(chain);
    expect(tipParams(g.nodes.find((n) => n.kind === "ds"), "pub.dnssec_chain_tip_ttl").ttl).toBe(86400);
    expect(tipParams(g.nodes.find((n) => n.id === "key-1000"), "pub.dnssec_chain_tip_ttl").ttl).toBe(3600);
    expect(tipParams(g.nodes.find((n) => n.id === "rrset-SOA"), "pub.dnssec_chain_tip_ttl").ttl).toBe(900);
  });

  it("omits the TTL tip line when the blob has no ttl (older data)", () => {
    const g = layoutChain(secureChain());
    expect(hasTip(g.nodes.find((n) => n.kind === "ds"), "pub.dnssec_chain_tip_ttl")).toBe(false);
  });

  it("shows a TTL of 0 (NSEC3PARAM commonly uses it)", () => {
    const chain = secureChain();
    chain.child.signed = [{ type: "NSEC3PARAM", ttl: 0, rrsig: [{ key_tag: 2000, state: "valid" }] }];
    const g = layoutChain(chain);
    const node = g.nodes.find((n) => n.id === "rrset-NSEC3PARAM");
    expect(tipParams(node, "pub.dnssec_chain_tip_ttl").ttl).toBe(0);
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

  it("draws a sibling signing edge from a signing KSK to a non-signing KSK", () => {
    // Two anchored KSKs, but only 10075 signs the DNSKEY RRset; its signature
    // covers the whole RRset, so it vouches for the non-signing KSK 37745.
    const chain = secureChain();
    chain.child.dnskeys = [
      { key_tag: 10075, algorithm: 8, flags: 257, sep: true, anchored: true, servers: ["203.0.113.1"] },
      { key_tag: 37745, algorithm: 8, flags: 257, sep: true, anchored: true, servers: ["203.0.113.1"] },
      { key_tag: 23415, algorithm: 8, flags: 256, sep: false, servers: ["203.0.113.1"] },
    ];
    chain.child.dnskey_rrsig = [{ key_tag: 10075, algorithm: 8, state: "valid" }];
    const g = layoutChain(chain);
    const sibling = g.edges.find((e) => e.kind === "keysig" && e.keyTag === 10075 && e.targetTag === 37745);
    expect(sibling).toBeTruthy();
    // Same-row sibling edges render as a path, not a straight line.
    expect(typeof sibling.d).toBe("string");
    // 10075 still vouches downward for the ZSK too.
    expect(g.edges.some((e) => e.kind === "keysig" && e.keyTag === 10075 && e.targetTag === 23415)).toBe(true);
  });

  it("does not draw sibling edges when both KSKs sign the DNSKEY RRset", () => {
    const chain = secureChain();
    chain.child.dnskeys = [
      { key_tag: 872, algorithm: 8, flags: 257, sep: true, anchored: true, servers: ["203.0.113.1"] },
      { key_tag: 62294, algorithm: 8, flags: 257, sep: true, anchored: true, servers: ["203.0.113.1"] },
    ];
    chain.child.dnskey_rrsig = [
      { key_tag: 872, algorithm: 8, state: "valid" },
      { key_tag: 62294, algorithm: 8, state: "valid" },
    ];
    const g = layoutChain(chain);
    // Both self-sign, so no KSK -> KSK vouching edge is needed.
    const ksks = new Set([872, 62294]);
    expect(g.edges.some((e) => e.kind === "keysig" && ksks.has(e.targetTag))).toBe(false);
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

  it("groups dual-digest DS records for one key tag into a single node and edge", () => {
    // A parent commonly publishes SHA-256 and SHA-384 DS records for the same
    // KSK. They collapse to one DS node (both digests listed in its tooltip)
    // and one DS -> DNSKEY edge, rather than two boxes with the same tag.
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
    expect(dsNodes).toHaveLength(1);
    expect(dsNodes[0].id).toBe("ds-1000");
    // Both digest types appear in the one node's tooltip.
    const digestTypes = dsNodes[0].tip.filter((l) => l.k === "pub.dnssec_chain_tip_digest_type").map((l) => l.p.dt);
    expect(digestTypes).toEqual(["SHA-256 (2)", "SHA-384 (4)"]);
    const dsEdges = g.edges.filter((e) => e.kind === "ds");
    expect(dsEdges).toHaveLength(1);
    expect(dsEdges[0].status).toBe("match");
    const nodeIds = new Set(g.nodes.map((n) => n.id));
    const edgeIds = new Set(g.edges.map((e) => e.id));
    expect(nodeIds.size).toBe(g.nodes.length);
    expect(edgeIds.size).toBe(g.edges.length);
  });

  it("marks a grouped DS as matched when only one digest matches the key", () => {
    // If SHA-256 matches but SHA-1 does not, the tag is still anchored: a
    // validator accepts the delegation, so the single edge reads as a match.
    const chain = secureChain();
    chain.parent.ds = [
      { key_tag: 1000, algorithm: 13, digest_type: 1, digest: "ab", servers: ["192.0.2.1"] },
      { key_tag: 1000, algorithm: 13, digest_type: 2, digest: "cd", servers: ["192.0.2.1"] },
    ];
    chain.links = [
      { ds_key_tag: 1000, ds_digest_type: 1, dnskey_key_tag: 1000, status: "digest_mismatch", servers: ["203.0.113.1"] },
      { ds_key_tag: 1000, ds_digest_type: 2, dnskey_key_tag: 1000, status: "match", servers: ["203.0.113.1"] },
    ];
    const g = layoutChain(chain);
    const dsEdges = g.edges.filter((e) => e.kind === "ds");
    expect(dsEdges).toHaveLength(1);
    expect(dsEdges[0].status).toBe("match");
  });

  it("merges overlapping signatures by the same key into one identified edge", () => {
    // During re-signing a zone serves two RRSIGs by the same key with
    // different validity windows. They share one path, so they share one edge
    // rather than being drawn on top of each other, and ids stay unique.
    const chain = secureChain();
    chain.child.dnskey_rrsig = [
      { key_tag: 1000, algorithm: 13, state: "valid", inception: 100, expiration: 200, servers: ["203.0.113.1"] },
      { key_tag: 1000, algorithm: 13, state: "expired", inception: 1, expiration: 99, servers: ["203.0.113.1"] },
    ];
    const g = layoutChain(chain);
    const selfLoops = g.edges.filter((e) => e.kind === "selfsig");
    expect(selfLoops).toHaveLength(1);
    expect(selfLoops[0].status).toBe("expired");
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

  it("orders the key row by key tag so DS edges do not cross the phantom", () => {
    // A DS for the absent key 11155 (phantom) and a DS for the present KSK
    // 34586. The key row must be [11155, 34586] to match the DS row order, so
    // the two DS edges stay parallel.
    const chain = secureChain();
    chain.parent.ds = [
      { key_tag: 11155, algorithm: 8, digest_type: 2, digest: "ab", servers: ["192.0.2.1"] },
      { key_tag: 34586, algorithm: 8, digest_type: 2, digest: "cd", servers: ["192.0.2.1"] },
    ];
    chain.child.dnskeys = [
      { key_tag: 34586, algorithm: 8, flags: 257, sep: true, anchored: true, servers: ["203.0.113.1"] },
      { key_tag: 10194, algorithm: 8, flags: 256, sep: false, servers: ["203.0.113.1"] },
    ];
    chain.links = [
      { ds_key_tag: 11155, ds_digest_type: 2, status: "no_dnskey", servers: ["192.0.2.1"] },
      { ds_key_tag: 34586, ds_digest_type: 2, dnskey_key_tag: 34586, status: "match", servers: ["203.0.113.1"] },
    ];
    const g = layoutChain(chain);
    const phantom = g.nodes.find((n) => n.id === "key-11155");
    const realKsk = g.nodes.find((n) => n.id === "key-34586");
    // Lower key tag sits to the left, matching the DS row order.
    expect(phantom.x).toBeLessThan(realKsk.x);
    const dsLow = g.nodes.find((n) => n.id === "ds-11155");
    const dsHigh = g.nodes.find((n) => n.id === "ds-34586");
    expect(dsLow.x).toBeLessThan(dsHigh.x);
    // Both DS edges run left-to-left and right-to-right (no crossing).
    const edges = g.edges.filter((e) => e.kind === "ds");
    const eLow = edges.find((e) => Math.round(e.from.x) === Math.round(dsLow.x + dsLow.w / 2));
    const eHigh = edges.find((e) => Math.round(e.from.x) === Math.round(dsHigh.x + dsHigh.w / 2));
    expect(eLow.to.x).toBeLessThan(eHigh.to.x);
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

describe("worstSigState", () => {
  it("ranks expired above valid so a fresh signature cannot mask it", () => {
    expect(worstSigState([{ state: "valid" }, { state: "expired" }])).toBe("expired");
    expect(worstSigState([{ state: "expired" }, { state: "valid" }])).toBe("expired");
  });

  it("prefers a window failure over an unverifiable one", () => {
    expect(worstSigState([{ state: "unsupported_key" }, { state: "expired" }])).toBe("expired");
    expect(worstSigState([{ state: "valid" }, { state: "not_yet_valid" }])).toBe("not_yet_valid");
  });

  it("returns valid for an all-valid or empty set", () => {
    expect(worstSigState([{ state: "valid" }])).toBe("valid");
    expect(worstSigState([])).toBe("valid");
    expect(worstSigState(undefined)).toBe("valid");
  });
});

describe("layoutChain signature overlap", () => {
  // A lagging secondary serves an expired signature over the same RRset as the
  // fresh servers. Both edges share a path, so the fresh one used to be drawn
  // on top of the expired one and hide it.
  it("collapses same-path DNSKEY signatures into one worst-state edge", () => {
    const chain = secureChain();
    chain.child.dnskey_rrsig = [
      { key_tag: 1000, algorithm: 13, state: "valid", inception: 1700000000, expiration: 1800000000, servers: ["203.0.113.1"] },
      { key_tag: 1000, algorithm: 13, state: "expired", inception: 1600000000, expiration: 1650000000, servers: ["203.0.113.9"] },
    ];
    const g = layoutChain(chain);

    const loops = g.edges.filter((e) => e.kind === "selfsig");
    expect(loops).toHaveLength(1);
    expect(loops[0].id).toBe("self-1000");
    expect(loops[0].status).toBe("expired");
    // Both windows stay in the tip, so hovering explains the two states.
    const windows = loops[0].tip.filter((l) => l.k === "pub.dnssec_chain_tip_valid");
    expect(windows).toHaveLength(2);
    const states = loops[0].tip.filter((l) => l.statusState).map((l) => l.statusState);
    expect(states).toEqual(["valid", "expired"]);

    // The edge vouching for the ZSK collapses the same way.
    const vouch = g.edges.filter((e) => e.kind === "keysig" && e.targetTag === 2000);
    expect(vouch).toHaveLength(1);
    expect(vouch[0].status).toBe("expired");
  });

  it("collapses mixed-state signatures over a signed RRset to the worst state", () => {
    const chain = secureChain();
    chain.child.signed = [
      {
        type: "SOA",
        rrsig: [
          { key_tag: 2000, algorithm: 13, state: "valid", inception: 1700000000, expiration: 1800000000, servers: ["203.0.113.1"] },
          { key_tag: 2000, algorithm: 13, state: "expired", inception: 1600000000, expiration: 1650000000, servers: ["203.0.113.9"] },
        ],
      },
    ];
    const g = layoutChain(chain);
    const soaEdges = g.edges.filter((e) => e.kind === "sig");
    expect(soaEdges).toHaveLength(1);
    expect(soaEdges[0].id).toBe("sig-SOA-2000");
    expect(soaEdges[0].status).toBe("expired");
  });

  it("keeps signatures by different keys on their own edges", () => {
    const chain = secureChain();
    chain.child.dnskey_rrsig = [
      { key_tag: 1000, algorithm: 13, state: "valid", servers: ["203.0.113.1"] },
      { key_tag: 2000, algorithm: 13, state: "expired", servers: ["203.0.113.1"] },
    ];
    const g = layoutChain(chain);
    const loops = g.edges.filter((e) => e.kind === "selfsig");
    expect(loops.map((e) => e.id).sort()).toEqual(["self-1000", "self-2000"]);
  });
});

describe("layoutChain authenticated denial", () => {
  it("lays out NSEC and NSEC3 as signed rows", () => {
    const chain = secureChain();
    chain.child.signed = [
      { type: "NSEC", rrsig: [{ key_tag: 2000, algorithm: 13, state: "expired", servers: ["203.0.113.9"] }] },
      { type: "NSEC3", rrsig: [{ key_tag: 2000, algorithm: 13, state: "valid", servers: ["203.0.113.1"] }] },
    ];
    const g = layoutChain(chain);

    const nsec = g.nodes.find((n) => n.id === "rrset-NSEC");
    const nsec3 = g.nodes.find((n) => n.id === "rrset-NSEC3");
    expect(nsec.label).toBe("NSEC");
    expect(nsec3.label).toBe("NSEC3");
    // Both sit in the same (bottom) row, inside the viewBox.
    expect(nsec.rowIndex).toBe(nsec3.rowIndex);
    for (const n of [nsec, nsec3]) {
      expect(n.x + n.w).toBeLessThanOrEqual(g.width);
      expect(n.y + n.h).toBeLessThanOrEqual(g.height);
    }
    // The expired denial signature colors its own edge.
    const nsecEdge = g.edges.find((e) => e.id === "sig-NSEC-2000");
    expect(nsecEdge.status).toBe("expired");
  });
});

describe("stale-signature strings", () => {
  it("are translated in every locale", async () => {
    const locales = ["cs", "da", "de", "en", "es", "fi", "fr", "ja", "nb", "nl", "sl", "sv"];
    for (const loc of locales) {
      const catalog = (await import(`../i18n/${loc}.json`)).default;
      for (const key of ["pub.dnssec_chain_stale_secondary", "pub.dnssec_chain_stale_servers"]) {
        const val = catalog[key];
        expect(typeof val, `${loc} ${key}`).toBe("string");
        expect(val.length > 0, `${loc} ${key}`).toBe(true);
      }
      expect(catalog["pub.dnssec_chain_stale_servers"].includes("{servers}"), loc).toBe(true);
    }
  });
});

describe("node face labels", () => {
  it("puts the algorithm and key size on the key nodes", () => {
    const chain = secureChain();
    chain.child.dnskeys[0].key_size = 256;
    chain.child.dnskeys[1].algorithm = 8;
    chain.child.dnskeys[1].key_size = 2048;
    const g = layoutChain(chain);

    const ksk = g.nodes.find((n) => n.kind === "ksk");
    expect(ksk.algoText).toBe("ECDSAP256SHA256");
    expect(ksk.bitsText).toBe("256 bit");

    const zsk = g.nodes.find((n) => n.kind === "zsk");
    expect(zsk.algoText).toBe("RSASHA256");
    expect(zsk.bitsText).toBe("2048 bit");
  });

  // The DS face carries the key algorithm, not the digest: that is the field
  // that has to equal the DNSKEY's, and one DS node groups every digest type
  // published for the tag, so a single digest line would be lossy.
  it("puts the key algorithm on a DS node and keeps every digest in its tip", () => {
    const chain = secureChain();
    chain.parent.ds = [
      { key_tag: 1000, algorithm: 13, digest_type: 1, digest: "aa", servers: ["192.0.2.1"] },
      { key_tag: 1000, algorithm: 13, digest_type: 2, digest: "bb", servers: ["192.0.2.1"] },
    ];
    const g = layoutChain(chain);

    const dsNodes = g.nodes.filter((n) => n.kind === "ds");
    expect(dsNodes.length).toBe(1);
    expect(dsNodes[0].algoText).toBe("ECDSAP256SHA256");
    const digests = dsNodes[0].tip.filter((l) => l.k === "pub.dnssec_chain_tip_digest_type");
    expect(digests.map((l) => l.p.dt)).toEqual(["SHA-1 (1)", "SHA-256 (2)"]);
  });

  it("keeps the alg prefix for an algorithm with no mnemonic", () => {
    // A bare "99" on a node face reads as a key tag; "alg 99" does not.
    expect(algoFace(99)).toBe("alg 99");

    const chain = secureChain();
    chain.child.dnskeys[0].algorithm = 99;
    const g = layoutChain(chain);
    expect(g.nodes.find((n) => n.kind === "ksk").algoText).toBe("alg 99");
  });

  it("omits the size line when the key size is unknown", () => {
    expect(bitsFace(0)).toBe(null);
    expect(bitsFace(undefined)).toBe(null);

    // The default fixture carries no key_size.
    const g = layoutChain(secureChain());
    expect(g.nodes.find((n) => n.kind === "ksk").bitsText).toBe(null);
  });

  // Placeholder nodes stand in for a record nobody published, so there is no
  // algorithm to show and the face must stay at two lines.
  it("gives ghost and phantom nodes no algorithm or size", () => {
    const noKeys = secureChain();
    noKeys.child.dnskeys = [];
    const ghost = layoutChain(noKeys).nodes.find((n) => n.kind === "key-ghost");
    expect(ghost).toBeTruthy();
    expect(ghost.algoText).toBe(undefined);

    const noDS = secureChain();
    noDS.parent.ds = [];
    noDS.links = [];
    const dsGhost = layoutChain(noDS).nodes.find((n) => n.kind === "ds-ghost");
    expect(dsGhost).toBeTruthy();
    expect(dsGhost.algoText).toBe(undefined);

    // A DS naming a key tag the zone does not publish gets a phantom target.
    const stale = secureChain();
    stale.parent.ds.push({ key_tag: 5000, algorithm: 13, digest_type: 2, digest: "cc", servers: ["192.0.2.1"] });
    stale.links.push({ ds_key_tag: 5000, ds_digest_type: 2, status: "no_dnskey", servers: ["192.0.2.1"] });
    const phantom = layoutChain(stale).nodes.find((n) => n.kind === "key-phantom");
    expect(phantom).toBeTruthy();
    expect(phantom.algoText).toBe(undefined);
    expect(phantom.bitsText).toBe(undefined);
  });

  // The face is 10px type in a fixed-width box. A longer mnemonic would spill
  // out of the node, so adding an algorithm has to respect the budget.
  it("keeps every mnemonic inside the width the node box allows", () => {
    for (let n = 0; n < 256; n++) {
      const m = algoMnemonic(n);
      if (m === String(n)) continue; // unassigned: rendered as "alg N"
      expect(m.length).toBeLessThanOrEqual(ALGO_FACE_MAX);
    }
  });

  it("leaves room between the rows for the signature edges", () => {
    const g = layoutChain(secureChain());
    const rows = [...new Set(g.nodes.map((n) => n.y))].sort((a, b) => a - b);
    expect(rows.length).toBeGreaterThan(1);
    const h = g.nodes[0].h;
    for (let i = 1; i < rows.length; i++) {
      expect(rows[i] - rows[i - 1] - h).toBeGreaterThanOrEqual(40);
    }
  });
});

describe("relativeName", () => {
  it("drops the zone suffix so a name reads inside its own cluster", () => {
    expect(relativeName("a.ns.example.com", "example.com")).toBe("a.ns");
  });

  it("ignores a trailing dot on either side", () => {
    expect(relativeName("a.ns.example.com.", "example.com")).toBe("a.ns");
    expect(relativeName("a.ns.example.com", "example.com.")).toBe("a.ns");
  });

  it("keeps a name that is not inside the zone", () => {
    expect(relativeName("ns.example.net", "example.com")).toBe("ns.example.net");
  });

  it("keeps the apex itself whole", () => {
    expect(relativeName("example.com", "example.com")).toBe("example.com");
  });
});

describe("in-domain nameserver names", () => {
  // The chain of the zone is intact and the branch below it is not: the graph
  // must show the fault the run reports, not a clean chain.
  const orphanChain = () =>
    secureChain({
      version: 3,
      ns_names: [
        { name: "a.ns.example.com", status: "orphan", signer: "a.ns.example.com", servers: ["192.0.2.1"] },
        { name: "b.ns.example.com", status: "validates", signer: "ns.example.com", servers: ["192.0.2.1"] },
        { name: "c.ns.example.com", status: "validates", signer: "example.com", servers: ["192.0.2.1"] }
      ]
    });

  it("draws one node per name, labelled relative to the zone", () => {
    const g = layoutChain(orphanChain());
    const names = g.nodes.filter((n) => n.kind === "nsname");
    expect(names.map((n) => n.nameText)).toEqual(["a.ns", "b.ns", "c.ns"]);
    expect(names.map((n) => n.nsStatus)).toEqual(["orphan", "validates", "validates"]);
  });

  it("gives the orphan apex its own node with a severed stub", () => {
    const g = layoutChain(orphanChain());
    const orphan = g.nodes.find((n) => n.kind === "orphan");
    expect(orphan.nameText).toBe("a.ns");
    const stub = g.edges.find((e) => e.kind === "stub" && e.id === `stub-${orphan.id}`);
    expect(stub.broken).toBe(true);
    expect(stub.ticks.length).toBe(2);
    // The severed stub reaches up into the gap above and stops there.
    expect(stub.d).toContain("M");
  });

  it("draws a secure cut as an unbroken stub", () => {
    const g = layoutChain(orphanChain());
    const cut = g.nodes.find((n) => n.kind === "cut");
    expect(cut.nameText).toBe("ns");
    const stub = g.edges.find((e) => e.kind === "stub" && e.id === `stub-${cut.id}`);
    expect(stub.broken).toBe(false);
    expect(stub.ticks).toBe(null);
  });

  it("points each name at the zone that signs it, and leaves apex-signed names free", () => {
    const g = layoutChain(orphanChain());
    const edges = g.edges.filter((e) => e.kind === "nssig");
    // c.ns is signed by the apex, whose chain the graph above already draws.
    expect(edges.length).toBe(2);
    expect(edges.map((e) => e.nsStatus).sort()).toEqual(["orphan", "validates"]);
  });

  it("labels the two new clusters with the zone", () => {
    const g = layoutChain(orphanChain());
    const ids = g.clusters.map((c) => c.id);
    expect(ids).toContain("nsnames");
    expect(ids).toContain("signers");
    const nsCluster = g.clusters.find((c) => c.id === "nsnames");
    expect(nsCluster.name).toBe("example.com");
    expect(nsCluster.labelKey).toBe("pub.dnssec_chain_nsnames_label");
  });

  it("omits the signer row when every name is signed by the apex", () => {
    const g = layoutChain(
      secureChain({
        version: 3,
        ns_names: [{ name: "ns1.example.com", status: "validates", signer: "example.com", servers: ["192.0.2.1"] }]
      })
    );
    expect(g.nodes.some((n) => n.kind === "cut" || n.kind === "orphan")).toBe(false);
    expect(g.edges.some((e) => e.kind === "nssig")).toBe(false);
    expect(g.nodes.filter((n) => n.kind === "nsname").length).toBe(1);
  });

  // A document stored before the section existed must render exactly as before.
  it("adds nothing for a blob without the section", () => {
    const before = layoutChain(secureChain());
    const after = layoutChain(secureChain({ ns_names: [] }));
    expect(after).toEqual(before);
    expect(before.nodes.some((n) => n.kind === "nsname")).toBe(false);
  });
});

describe("signer nodes", () => {
  // A delegation that exists with a broken chain is a different fault from one
  // nothing delegates, and must not be drawn as a healthy cut.
  it("marks a broken cut bad without severing its stub", () => {
    const g = layoutChain(
      secureChain({
        version: 3,
        ns_names: [
          { name: "a.ns.example.com", status: "chain_broken", signer: "ns.example.com", servers: ["192.0.2.1"] }
        ]
      })
    );
    const signer = g.nodes.find((n) => n.kind === "cut-broken");
    expect(signer.nameText).toBe("ns");
    const stub = g.edges.find((e) => e.kind === "stub");
    expect(stub.broken).toBe(false);
    expect(stub.bad).toBe(true);
    expect(signer.tip[0].k).toBe("pub.dnssec_chain_tip_cut_broken");
  });

  it("keeps a healthy cut's stub unbroken and not bad", () => {
    const g = layoutChain(
      secureChain({
        version: 3,
        ns_names: [{ name: "a.ns.example.com", status: "validates", signer: "ns.example.com", servers: ["192.0.2.1"] }]
      })
    );
    const stub = g.edges.find((e) => e.kind === "stub");
    expect(stub.broken).toBe(false);
    expect(stub.bad).toBe(false);
  });

  // Names sharing a signer collapse onto one node, which must carry the worst
  // verdict rather than whichever name was listed first.
  it("takes the worst verdict when names share a signer", () => {
    const g = layoutChain(
      secureChain({
        version: 3,
        ns_names: [
          { name: "a.ns.example.com", status: "validates", signer: "ns.example.com", servers: ["192.0.2.1"] },
          { name: "b.ns.example.com", status: "chain_broken", signer: "ns.example.com", servers: ["192.0.2.1"] }
        ]
      })
    );
    const signers = g.nodes.filter((n) => n.kind === "cut" || n.kind === "cut-broken" || n.kind === "orphan");
    expect(signers.length).toBe(1);
    expect(signers[0].kind).toBe("cut-broken");
  });
});
