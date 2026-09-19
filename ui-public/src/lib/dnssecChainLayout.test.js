import { describe, it, expect } from "vitest";
import { nsName, nsNamesChain, secureChain } from "../test/helpers.js";
import { layoutChain, relativeName, truncateName, worstSigTone, worstSigState, algoMnemonic, algoFace, bitsFace, clipToWidth, faceWidth, nsTone, statusTone, wrapFace, ALGO_FACE_MAX } from "./dnssecChainLayout.js";

// MAX_WIDTH is the widest graph the public card holds without scaling it down.
const MAX_WIDTH = 660;

// The public UI catalogs.
const LOCALES = ["cs", "da", "de", "en", "es", "fi", "fr", "ja", "nb", "nl", "sl", "sv"];

// The header words a caller hands in, which the layout measures and elides.
const WORDS = {
  parent: "Parent zone",
  zone: "Tested zone",
  root: "root (.)",
  status: (s) => ({ secure: "Secure", partial: "Partial", broken: "Broken", undelegated: "Not delegated" })[s] ?? "",
};

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
    // The parent frame holds both parent rows.
    const frame = g.frames.find((f) => f.id === "parent");
    expect(frame.y).toBeLessThan(pk.y);
    expect(frame.y + frame.h).toBeGreaterThan(ds.y + ds.h);
  });

  it("keeps the DS as the parent row when the parent key is unknown", () => {
    const g = layoutChain(secureChain());
    expect(g.nodes.some((n) => n.kind === "parent-key")).toBe(false);
    const ds = g.nodes.find((n) => n.kind === "ds");
    const frame = g.frames.find((f) => f.id === "parent");
    // The parent frame holds the DS row and nothing else.
    expect(frame.y).toBeLessThan(ds.y);
    expect(g.nodes.filter((n) => n.y >= frame.y && n.y + n.h <= frame.y + frame.h)).toEqual([ds]);
  });

  it("names the two zones on the frames, not on the row labels", () => {
    const g = layoutChain(secureChain(), { words: WORDS });
    expect(g.frames.map((f) => f.header.roleText)).toEqual(["Parent zone", "Tested zone"]);
    expect(g.frames.map((f) => f.header.name)).toEqual(["com", "example.com"]);
    // A row label names its row alone, and the parent rows need none.
    expect(g.clusters.every((c) => c.name === undefined)).toBe(true);
    expect(g.clusters.some((c) => c.id === "parent")).toBe(false);
    // No signed-records row without a zone-data signature.
    expect(g.clusters.some((c) => c.id === "signed")).toBe(false);
  });

  it("reads the root as the word the caller hands in, not a lone dot", () => {
    const g = layoutChain(secureChain({ parent_zone: "." }), { words: WORDS });
    expect(g.frames.find((f) => f.id === "parent").header.name).toBe("root (.)");
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

  it("carries a key_not_signing DS link through to the edge and tip", () => {
    // key_not_signing passes through to the DS edge and its tip.
    const g = layoutChain(secureChain({
      status: "partial",
      links: [{ ds_key_tag: 1000, dnskey_key_tag: 1000, status: "key_not_signing", servers: ["203.0.113.1"] }],
    }));
    const dsEdge = g.edges.find((e) => e.kind === "ds");
    expect(dsEdge.status).toBe("key_not_signing");
    const statusLine = dsEdge.tip.find((l) => l.k === "pub.dnssec_chain_tip_status");
    expect(statusLine.linkState).toBe("key_not_signing");
  });

  it("prefers the matching link when a mismatched sibling DS shares the key tag", () => {
    // Two DS for one key tag, one usable.
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
    for (const loc of LOCALES) {
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

  it("wraps a wide key row onto a second line instead of widening the graph", () => {
    const one = layoutChain(secureChain());
    const many = secureChain();
    for (let i = 0; i < 6; i++) {
      many.child.dnskeys.push({ key_tag: 3000 + i, algorithm: 13, flags: 256, sep: false, servers: ["203.0.113.1"] });
    }
    const g = layoutChain(many);
    // Seven ZSKs spill onto a second line: the graph grows down, not out.
    expect(g.width).toBeLessThanOrEqual(MAX_WIDTH);
    expect(g.height).toBeGreaterThan(one.height);
    const zsks = g.nodes.filter((n) => n.kind === "zsk");
    expect(new Set(zsks.map((n) => n.lineIndex)).size).toBe(2);
    // Every line of the row is centred on the same axis.
    const mid = (n) => n.x + n.w / 2;
    const line0 = zsks.filter((n) => n.lineIndex === 0);
    const line1 = zsks.filter((n) => n.lineIndex === 1);
    const centre = (line) => (mid(line[0]) + mid(line[line.length - 1])) / 2;
    expect(Math.abs(centre(line0) - centre(line1))).toBeLessThan(1);
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
    for (const loc of LOCALES) {
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

describe("wrapFace", () => {
  // A name box is 132 wide and renders its status line in 11px type.
  const BOX = 132;
  const SUB = 11;
  const BUDGET = 120;

  it("leaves a line that already fits alone", () => {
    expect(wrapFace("validates", BOX, SUB)).toEqual(["validates"]);
  });

  it("breaks a long translation at the most even space", () => {
    expect(wrapFace("Vertrauenskette unterbrochen", BOX, SUB)).toEqual(["Vertrauenskette", "unterbrochen"]);
  });

  it("keeps a single long word whole rather than losing characters", () => {
    expect(wrapFace("allekirjoittamaton", BOX, SUB)).toEqual(["allekirjoittamaton"]);
  });

  it("makes no more lines than the box has room for", () => {
    expect(wrapFace("one two three four five six seven eight", BOX, SUB).length).toBe(2);
  });

  it("counts a full-width script at an em per character", () => {
    expect(faceWidth("署名", 11)).toBe(22);
    expect(faceWidth("ab", 11)).toBeLessThan(22);
  });

  // Every status word a name or signer box shows reads inside the box.
  it.each(LOCALES)("fits the name box status in %s", async (loc) => {
    const catalog = (await import(`../i18n/${loc}.json`)).default;
    for (const [key, value] of Object.entries(catalog)) {
      if (!key.startsWith("pub.dnssec_chain_nsstatus_") && !key.startsWith("pub.dnssec_chain_signer_")) continue;
      for (const line of wrapFace(value, BOX, SUB)) {
        expect(faceWidth(line, SUB), key).toBeLessThanOrEqual(BUDGET);
      }
    }
  });
});

describe("relativeName", () => {
  it.each([
    ["a.ns.example.com", "example.com", "a.ns"],
    ["a.ns.example.com.", "example.com", "a.ns"],
    ["a.ns.example.com", "example.com.", "a.ns"],
    ["ns.example.net", "example.com", "ns.example.net"],
    ["example.com", "example.com", "example.com"]
  ])("reads %s inside %s as %s", (name, zone, want) => {
    expect(relativeName(name, zone)).toBe(want);
  });
});

describe("in-domain nameserver names", () => {
  // An intact zone chain with an orphan name, a name under a cut and an apex-signed name.
  const orphanChain = () =>
    nsNamesChain([
      nsName("a.ns.example.com", "orphan", "a.ns.example.com"),
      nsName("b.ns.example.com", "validates", "ns.example.com"),
      nsName("c.ns.example.com", "validates")
    ]);

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
    // The stub climbs the box's centre line from its top and stops below the row above.
    const points = stub.d.match(/-?\d+(?:\.\d+)?/g).map(Number);
    const xs = points.filter((_, i) => i % 2 === 0);
    const ys = points.filter((_, i) => i % 2 === 1);
    for (const x of xs) expect(x).toBeCloseTo(orphan.x + orphan.w / 2, 1);
    expect(Math.max(...ys)).toBeCloseTo(orphan.y, 1);
    const rowAbove = Math.max(...g.nodes.filter((n) => n.y + n.h <= orphan.y).map((n) => n.y + n.h));
    expect(Math.min(...ys)).toBeGreaterThan(rowAbove);
    expect(Math.min(...ys)).toBeLessThan(orphan.y);
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

  it("labels the two new rows without repeating the zone", () => {
    const g = layoutChain(orphanChain());
    const ids = g.clusters.map((c) => c.id);
    expect(ids).toContain("nsnames");
    expect(ids).toContain("signers");
    const nsCluster = g.clusters.find((c) => c.id === "nsnames");
    expect(nsCluster.name).toBeUndefined();
    expect(nsCluster.labelKey).toBe("pub.dnssec_chain_nsnames_label");
  });

  it("omits the signer row when every name is signed by the apex", () => {
    const g = layoutChain(nsNamesChain([nsName("ns1.example.com", "validates")]));
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

describe("wide rows", () => {
  it("wraps a long name row so the graph stays inside the card", () => {
    const g = layoutChain(nsNamesChain(Array.from({ length: 10 }, (_, i) => nsName(`ns${i}.example.com`, "validates"))));
    const names = g.nodes.filter((n) => n.kind === "nsname");
    expect(names.length).toBe(10);
    expect(g.width).toBeLessThanOrEqual(MAX_WIDTH);
    expect(new Set(names.map((n) => n.lineIndex)).size).toBe(3);
    // Each line clears the one above it, and nothing leaves the canvas.
    const top = (line) => names.find((n) => n.lineIndex === line).y;
    expect(top(1)).toBeGreaterThanOrEqual(top(0) + names[0].h);
    expect(top(2)).toBeGreaterThanOrEqual(top(1) + names[0].h);
    for (const n of g.nodes) {
      expect(n.x).toBeGreaterThanOrEqual(0);
      expect(n.x + n.w).toBeLessThanOrEqual(g.width);
      expect(n.y + n.h).toBeLessThanOrEqual(g.height);
    }
  });

  it("keeps the names one zone signs on a single line", () => {
    const g = layoutChain(
      nsNamesChain([
        nsName("ns1.first.example", "validates", "first.example"),
        nsName("ns1.second.example", "validates", "second.example"),
        nsName("ns2.first.example", "validates", "first.example"),
        nsName("ns2.second.example", "validates", "second.example"),
        nsName("ns.third.example", "validates", "third.example"),
        nsName("ns.example.com", "validates")
      ])
    );
    const lineOf = (name) => g.nodes.find((n) => n.id === `nsname-${name}`).lineIndex;
    // A name signed elsewhere sits next to its sibling, whatever order the
    // blob lists the names in.
    expect(lineOf("ns1.first.example")).toBe(lineOf("ns2.first.example"));
    expect(lineOf("ns1.second.example")).toBe(lineOf("ns2.second.example"));
    // The apex-signed name draws no edge, so it trails the ones that do.
    expect(lineOf("ns.example.com")).toBeGreaterThan(lineOf("ns1.first.example"));
  });

  it("holds the signed records of an NSEC3 zone on one line", () => {
    const chain = secureChain();
    chain.child.signed = ["SOA", "NSEC3", "NSEC3PARAM", "CDS", "CDNSKEY"].map((type) => ({
      type,
      rrsig: [{ key_tag: 2000, algorithm: 13, state: "valid", servers: ["203.0.113.1"] }],
    }));
    const g = layoutChain(chain);
    const rrsets = g.nodes.filter((n) => n.kind === "rrset");
    expect(rrsets.length).toBe(5);
    expect(new Set(rrsets.map((n) => n.lineIndex)).size).toBe(1);
    expect(g.width).toBeLessThanOrEqual(MAX_WIDTH);
  });

  it("ends the canvas at the ink rather than a fixed margin", () => {
    const g = layoutChain(nsNamesChain(Array.from({ length: 4 }, (_, i) => nsName(`ns${i}.example.com`, "validates"))));
    const right = Math.max(...g.nodes.map((n) => n.x + n.w));
    // The widest row ends one margin short of the edge; the self-loop off the
    // key sits well inside it.
    expect(g.width - right).toBeLessThan(40);
  });
});

describe("signer nodes", () => {
  // A delegated signer with a broken chain is neither a healthy cut nor an orphan.
  it("marks a broken cut bad without severing its stub", () => {
    const g = layoutChain(nsNamesChain([nsName("a.ns.example.com", "chain_broken", "ns.example.com")]));
    const signer = g.nodes.find((n) => n.kind === "cut-broken");
    expect(signer.nameText).toBe("ns");
    const stub = g.edges.find((e) => e.kind === "stub");
    expect(stub.broken).toBe(false);
    expect(stub.bad).toBe(true);
    expect(signer.tip[0].k).toBe("pub.dnssec_chain_tip_cut_broken");
  });

  it("keeps a healthy cut's stub unbroken and not bad", () => {
    const g = layoutChain(nsNamesChain([nsName("a.ns.example.com", "validates", "ns.example.com")]));
    const stub = g.edges.find((e) => e.kind === "stub");
    expect(stub.broken).toBe(false);
    expect(stub.bad).toBe(false);
  });

  // Names sharing a signer collapse onto one node carrying the worst verdict.
  it("takes the worst verdict when names share a signer", () => {
    const g = layoutChain(
      nsNamesChain([
        nsName("a.ns.example.com", "validates", "ns.example.com"),
        nsName("b.ns.example.com", "chain_broken", "ns.example.com")
      ])
    );
    const signers = g.nodes.filter((n) => n.kind === "cut" || n.kind === "cut-broken" || n.kind === "orphan");
    expect(signers.length).toBe(1);
    expect(signers[0].kind).toBe("cut-broken");
  });
});

describe("zone frames", () => {
  // A frame holds exactly the rows of its side of the delegation.
  it("frames the parent above the tested zone, each around its own rows", () => {
    const g = layoutChain(
      nsNamesChain([nsName("a.ns.example.com", "orphan", "ns.example.com")], {
        child: {
          dnskeys: [{ key_tag: 1000, algorithm: 13, flags: 257, sep: true, servers: ["203.0.113.1"] }],
          dnskey_rrsig: [{ key_tag: 1000, algorithm: 13, state: "valid", servers: ["203.0.113.1"] }],
          signed: [{ type: "SOA", rrsig: [{ key_tag: 1000, state: "valid" }] }],
        },
      }),
      { words: WORDS }
    );
    const [parent, zone] = g.frames;
    expect(parent.id).toBe("parent");
    expect(zone.id).toBe("zone");
    expect(parent.y + parent.h).toBeLessThan(zone.y);

    const held = (frame) =>
      g.nodes.filter((n) => n.y >= frame.y && n.y + n.h <= frame.y + frame.h).map((n) => n.kind);
    expect(held(parent)).toEqual(["ds"]);
    expect(held(zone)).toEqual(["ksk", "rrset", "orphan", "nsname"]);
    // Every node belongs to one frame or the other.
    expect(held(parent).length + held(zone).length).toBe(g.nodes.length);
  });

  it("keeps every frame inside the drawing", () => {
    const g = layoutChain(secureChain(), { words: WORDS });
    for (const f of g.frames) {
      expect(f.x).toBeGreaterThanOrEqual(0);
      expect(f.y).toBeGreaterThanOrEqual(0);
      expect(f.x + f.w).toBeLessThanOrEqual(g.width);
      expect(f.y + f.h).toBeLessThanOrEqual(g.height);
    }
    for (const n of g.nodes) {
      expect(n.y + n.h).toBeLessThanOrEqual(g.height);
    }
  });

  // The chip carries the verdict into a saved file.
  it("chips the tested frame with the roll-up word and tone", () => {
    const g = layoutChain(secureChain({ status: "partial" }), { words: WORDS });
    const [parent, zone] = g.frames;
    expect(zone.header.chip.text).toBe("Partial");
    expect(zone.tone).toBe("warn");
    expect(parent.header.chip).toBeNull();
    expect(parent.tone).toBe("neutral");
  });

  it("tones the frame from the roll-up, bad for a zone the parent proves undelegated", () => {
    const g = layoutChain(secureChain({ status: "undelegated" }), { words: WORDS });
    expect(g.frames.find((f) => f.id === "zone").tone).toBe("bad");
  });

  // A long name must not push the drawing wider than the card holds.
  it("elides a long zone name in the middle instead of widening the drawing", () => {
    const long = `${"a".repeat(60)}.example.com`;
    const plain = layoutChain(secureChain(), { words: WORDS });
    const g = layoutChain(secureChain({ zone: long }), { words: WORDS });
    expect(g.width).toBe(plain.width);
    const name = g.frames.find((f) => f.id === "zone").header.name;
    expect(name).not.toBe(long);
    expect(name).toContain("…");
    expect(name.startsWith("aaa")).toBe(true);
    expect(name.endsWith(".com")).toBe(true);
    // The name stops short of the chip beside it.
    const zone = g.frames.find((f) => f.id === "zone");
    expect(faceWidth(name, 12)).toBeLessThanOrEqual(zone.header.chip.x - zone.header.nameX);
  });

  it.each(LOCALES)("ships the frame header words in %s", async (loc) => {
    const catalog = (await import(`../i18n/${loc}.json`)).default;
    for (const key of ["pub.dnssec_chain_root", "pub.dnssec_chain_zone_label", "pub.dnssec_chain_export"]) {
      expect(typeof catalog[key], key).toBe("string");
      expect(catalog[key], key).toMatch(/\S/);
    }
  });

  // Without words the layout still lays out; only the header reads empty.
  it("lays out with no words at all", () => {
    const g = layoutChain(secureChain());
    expect(g.frames).toHaveLength(2);
    expect(g.frames[0].header.roleText).toBe("");
    expect(g.frames[1].header.chip).toBeNull();
  });
});

describe("node tone", () => {
  const toneOf = (chain, pick) => layoutChain(chain).nodes.find(pick)?.tone;
  const ds = (over = {}) => ({ key_tag: 1000, algorithm: 13, digest_type: 2, digest: "ab", servers: ["192.0.2.1"], ...over });

  it("grades a DS whose key tag names no published key as bad", () => {
    const chain = secureChain({ links: [{ ds_key_tag: 1000, dnskey_key_tag: 9999, status: "match", servers: ["192.0.2.1"] }] });
    expect(toneOf(chain, (n) => n.kind === "ds")).toBe("bad");
  });

  // The dead anchor was drawn on the edge alone, leaving the record plain.
  it("grades a DS naming a key that signs nothing as bad", () => {
    const chain = secureChain({
      status: "partial",
      links: [{ ds_key_tag: 1000, dnskey_key_tag: 1000, status: "key_not_signing", servers: ["192.0.2.1"] }],
    });
    expect(toneOf(chain, (n) => n.kind === "ds")).toBe("bad");
  });

  it("grades a revoked key and an expired DS signature as bad", () => {
    const revoked = secureChain();
    revoked.child.dnskeys[0].revoked = true;
    expect(toneOf(revoked, (n) => n.kind === "ksk")).toBe("bad");

    const expired = secureChain({ parent: { ds_source: "parent", ds: [ds()], ds_rrsig: [{ key_tag: 5, state: "expired" }] } });
    expect(toneOf(expired, (n) => n.kind === "ds")).toBe("bad");
  });

  it("grades a not-yet-valid DS signature and an unanchored key as warn", () => {
    const early = secureChain({ parent: { ds_source: "parent", ds: [ds()], ds_rrsig: [{ key_tag: 5, state: "not_yet_valid" }] } });
    expect(toneOf(early, (n) => n.kind === "ds")).toBe("warn");

    const rolling = secureChain();
    rolling.child.dnskeys[0].anchored = true;
    rolling.child.dnskeys.push({ key_tag: 3000, algorithm: 13, flags: 257, sep: true, servers: ["203.0.113.1"] });
    expect(toneOf(rolling, (n) => n.keyTag === 3000)).toBe("warn");
  });

  it("grades a record the zone does not publish as ghost", () => {
    const island = secureChain({ parent: { ds_source: "none", ds: [] }, links: [] });
    expect(toneOf(island, (n) => n.kind === "ds-ghost")).toBe("ghost");

    const phantom = secureChain({ links: [{ ds_key_tag: 4000, status: "no_dnskey", servers: ["192.0.2.1"] }] });
    expect(toneOf(phantom, (n) => n.kind === "key-phantom")).toBe("ghost");
  });

  it.each([
    ["validates", "ok"],
    ["insecure", "warn"],
    ["rrsig_expired", "bad"],
    ["indeterminate", ""]
  ])("grades a nameserver name with status %s as %j", (status, want) => {
    expect(toneOf(nsNamesChain([nsName("a.ns.example.com", status)]), (n) => n.kind === "nsname")).toBe(want);
  });

  it("grades a signer the zone does not delegate as bad and a settled key as plain", () => {
    const orphan = nsNamesChain([nsName("a.ns.example.com", "orphan", "ns.example.com")]);
    expect(toneOf(orphan, (n) => n.kind === "orphan")).toBe("bad");
    expect(toneOf(secureChain(), (n) => n.kind === "ksk")).toBe("");
  });

  it.each([
    ["nsTone", nsTone, "chain_broken", "bad"],
    ["nsTone", nsTone, "nonsense", ""],
    ["statusTone", statusTone, "secure", "ok"],
    ["statusTone", statusTone, "broken", "bad"],
    ["statusTone", statusTone, "island", "neutral"]
  ])("%s grades %s as %j", (_name, grade, status, want) => {
    expect(grade(status)).toBe(want);
  });
});

describe("clipToWidth", () => {
  it("leaves a string that fits untouched", () => {
    expect(clipToWidth("example.com", 400, 12)).toBe("example.com");
  });

  it("keeps both ends of a name it has to shorten", () => {
    const out = clipToWidth("verylongsubdomain.example.com", 80, 12);
    expect(out).toContain("…");
    expect(out.startsWith("very")).toBe(true);
    expect(out.endsWith(".com")).toBe(true);
    expect(faceWidth(out, 12)).toBeLessThanOrEqual(80);
  });

  // Full-width scripts take an em each, so a Japanese header elides sooner.
  it("measures a full-width script at an em per character", () => {
    expect(clipToWidth("テスト対象ゾーン", 40, 10)).not.toBe("テスト対象ゾーン");
    expect(faceWidth(clipToWidth("テスト対象ゾーン", 40, 10), 10)).toBeLessThanOrEqual(40);
  });

  it("returns nothing when there is no room at all", () => {
    expect(clipToWidth("example.com", 2, 12)).toBe("");
    expect(clipToWidth(null, 100, 12)).toBe("");
  });
});

describe("legend strings", () => {
  const KEYS = [
    "pub.dnssec_chain_detail_heading",
    "pub.dnssec_chain_detail_close",
    "pub.dnssec_chain_legend_line_ok",
    "pub.dnssec_chain_legend_line_warn",
    "pub.dnssec_chain_legend_line_bad",
    "pub.dnssec_chain_legend_line_severed",
    "pub.dnssec_chain_legend_mark_bad",
    "pub.dnssec_chain_legend_mark_warn",
    "pub.dnssec_chain_legend_mark_ghost",
  ];

  it.each(LOCALES)("ships the panel and legend words in %s", async (loc) => {
    const catalog = (await import(`../i18n/${loc}.json`)).default;
    for (const key of KEYS) {
      expect(typeof catalog[key], key).toBe("string");
      expect(catalog[key], key).toMatch(/\S/);
    }
  });

  // The three line items replaced the gradient swatch.
  it.each(LOCALES)("drops the gradient swatch string from %s", async (loc) => {
    const catalog = (await import(`../i18n/${loc}.json`)).default;
    expect(catalog["pub.dnssec_chain_legend_sig"]).toBeUndefined();
  });
});
