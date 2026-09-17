// Pure layout for the DNSSEC chain graph: chain summary in, SVG geometry out.
// A key signing the DNSKEY RRset self-loops and vouches for the keys below it.

const NODE_W = 132;
const NODE_H = 72;
const RRSET_W = 104; // a record box holds one type name, so it is narrower
const LEAF_H = 52; // record, name and signer boxes carry at most two lines
const H_GAP = 20;
const LINE_GAP = 14; // between the lines a wrapped row spills into
const ROW_GAP = 52; // between a row's last line and the row below it
const MAX_ROW_W = 600; // a wider row wraps, so the card holds the graph unscaled
const MIN_ROW_W = 320; // narrowest column a row centres in, so a row label fits
const PAD_X = 24;
const PAD_BOTTOM = 16;
const FRAME_TOP = 4; // room above the first frame
const FRAME_PAD_X = 18; // frame border to the nearest box
const FRAME_HEAD_H = 28; // header strip holding the role word and the zone name
const FRAME_PAD_BOTTOM = 14;
const FRAME_GAP = 26; // between the two frames
const FRAME_TEXT_PAD = 12;
const FRAME_TEXT_GAP = 10;
const FRAME_ROLE_FONT = 11;
const FRAME_NAME_FONT = 12;
const FRAME_CHIP_FONT = 10;
const CHIP_PAD_X = 8;
const CHIP_H = 16;
const FRAME_PAD_TOP = 16; // header strip to a first row that carries no label
const ROW_LABEL_GAP = 34; // header strip to a first row that carries one
const LABEL_BASE = 22; // row label baseline above its boxes
const LOOP_PAD = 32; // how far a key self-loop reaches past the node
const REF_BOW = 46; // sideways bow of a CDS/CDNSKEY reference edge

// Mnemonics by IANA algorithm number. algoProperties in the engine is the
// source of truth; an entry missing here renders as a bare number.
const ALGO = {
  1: "RSAMD5", 3: "DSA", 5: "RSASHA1", 6: "DSA-NSEC3-SHA1", 7: "RSASHA1-NSEC3-SHA1",
  8: "RSASHA256", 10: "RSASHA512", 12: "ECC-GOST", 13: "ECDSAP256SHA256",
  14: "ECDSAP384SHA384", 15: "ED25519", 16: "ED448", 17: "SM2SM3", 18: "MLDSA44",
  23: "ECC-GOST12",
};

function algoLabel(algo) {
  const m = ALGO[algo];
  return m ? `${m} (alg ${algo})` : `alg ${algo}`;
}

const DIGEST = { 1: "SHA-1", 2: "SHA-256", 3: "GOST R 34.11-94", 4: "SHA-384", 5: "GOST R 34.11-2012", 6: "SM3" };

function digestLabel(dt) {
  const m = DIGEST[dt];
  return m ? `${m} (${dt})` : `digest type ${dt}`;
}

// algoMnemonic / digestMnemonic return the IANA identifier, falling back to the
// raw number when unknown.
export function algoMnemonic(algo) {
  return ALGO[algo] ?? String(algo);
}

export function digestMnemonic(dt) {
  return DIGEST[dt] ?? String(dt);
}

// Widest mnemonic a NODE_W box fits at the 10px face size. A longer entry in
// ALGO would overflow the node, so the table is pinned to this budget.
export const ALGO_FACE_MAX = 18;

// FACE_PAD is the room a face line leaves inside its box on both sides.
const FACE_PAD = 12;

// isFullWidth reports whether a character takes a full em, as CJK does.
function isFullWidth(ch) {
  const c = ch.codePointAt(0);
  return (
    (c >= 0x1100 && c <= 0x115f) ||
    (c >= 0x2e80 && c <= 0xa4cf) ||
    (c >= 0xac00 && c <= 0xd7a3) ||
    (c >= 0xf900 && c <= 0xfaff) ||
    (c >= 0xfe30 && c <= 0xfe6f) ||
    (c >= 0xff00 && c <= 0xff60) ||
    (c >= 0xffe0 && c <= 0xffe6)
  );
}

// faceWidth estimates what a face line renders to at the given type size.
// Latin runs a little over half an em per character, full-width scripts an em.
export function faceWidth(text, px) {
  let w = 0;
  for (const ch of String(text ?? "")) w += isFullWidth(ch) ? px : px * 0.58;
  return w;
}

// wrapFace breaks a face line at a space so a translated word such as the
// German "Vertrauenskette unterbrochen" reads inside the box instead of
// spilling out of it. A single word wider than the box stays whole: the tip
// repeats the text, so nothing is lost, and dropped characters would be.
export function wrapFace(text, boxW, px, max = 2) {
  const s = String(text ?? "");
  const budget = boxW - FACE_PAD;
  if (max <= 1 || faceWidth(s, px) <= budget) return [s];
  const words = s.split(" ");
  if (words.length < 2) return [s];
  // Break where the two halves come out most even.
  let best = null;
  for (let i = 1; i < words.length; i++) {
    const head = words.slice(0, i).join(" ");
    const tail = words.slice(i).join(" ");
    const worst = Math.max(faceWidth(head, px), faceWidth(tail, px));
    if (best === null || worst < best.worst) best = { head, tail, worst };
  }
  return [best.head, ...wrapFace(best.tail, boxW, px, max - 1)];
}

// clipToWidth elides the middle, so both ends of a name survive.
export function clipToWidth(text, width, px) {
  const s = String(text ?? "");
  if (faceWidth(s, px) <= width) return s;
  const chars = [...s];
  const fit = (keep) =>
    chars.slice(0, Math.ceil(keep / 2)).join("") + "…" + chars.slice(chars.length - Math.floor(keep / 2)).join("");
  let keep = chars.length - 1;
  while (keep > 0 && faceWidth(fit(keep), px) > width) keep--;
  return keep > 0 ? fit(keep) : "";
}

// NS_TONE grades a nameserver name's status: bad where validators reject the
// name, warn where they accept it unsigned, ok where it validates.
const NS_TONE = {
  validates: "ok",
  insecure: "warn",
  unsigned: "bad",
  orphan: "bad",
  chain_broken: "bad",
  rrsig_expired: "bad",
  rrsig_invalid: "bad",
  indeterminate: "",
};

export const nsTone = (status) => NS_TONE[status] ?? "";

// statusTone grades a roll-up status for the heading badge, the frame border
// and the frame's chip.
export function statusTone(status) {
  if (status === "secure") return "ok";
  if (status === "broken" || status === "undelegated") return "bad";
  if (status === "partial") return "warn";
  return "neutral";
}

// nodeTone grades a node's own flags, so every tint is also a shape and a
// word: bad for a fault, warn for a caution, ghost for a record the zone does
// not publish, ok for a name that validates.
function nodeTone(n) {
  if (n.unmatched || n.revoked || n.deadAnchor || n.dsSigTone === "bad") return "bad";
  if (n.kind === "cut-broken" || n.kind === "orphan") return "bad";
  if (n.kind === "ds-ghost" || n.kind === "key-ghost" || n.kind === "key-phantom") return "ghost";
  if (n.dsSigTone === "warn" || n.rollover || n.incoming) return "warn";
  return nsTone(n.nsStatus);
}

// algoFace / bitsFace render the node-face lines. Unlike algoMnemonic, an
// unknown algorithm keeps the "alg" prefix so a bare number cannot be read as
// a key tag.
export function algoFace(algo) {
  return ALGO[algo] ?? `alg ${algo}`;
}

export function bitsFace(size) {
  return size ? `${size} bit` : null;
}

function flagWords(k) {
  const w = [];
  if (k.zone_key) w.push("ZONE");
  if (k.sep) w.push("SEP");
  if (k.revoked) w.push("REVOKE");
  return w.join(", ");
}

function shortHex(h) {
  const s = String(h ?? "");
  return s.length > 24 ? `${s.slice(0, 24)}…` : s;
}

// ttlLine returns a TTL tip line, or null when absent. A TTL of 0 is valid
// (NSEC3PARAM commonly uses it), so only a missing field is dropped.
function ttlLine(ttl) {
  return ttl == null ? null : { k: "pub.dnssec_chain_tip_ttl", p: { ttl } };
}

// serversTip returns a tip line listing up to four server addresses, or null.
// Addresses are protocol tokens, so only the label is localized.
function serversTip(servers) {
  if (!Array.isArray(servers) || servers.length === 0) return null;
  const shown = servers.slice(0, 4).join(", ");
  const list = servers.length > 4 ? `${shown}, +${servers.length - 4}` : shown;
  return { k: "pub.dnssec_chain_tip_servers", p: { servers: list } };
}

// fmtDate renders a unix-second timestamp as an ISO date.
export function fmtDate(sec) {
  if (!sec) return "";
  return new Date(sec * 1000).toISOString().slice(0, 10);
}

// worstSigTone reduces a set of signatures to the most severe tone: bad for
// expired/bogus/no-key, warn for not-yet-valid/unsupported (algorithm or key),
// else empty. An unsupported_key signature is unproven, not invalid, so it is a
// caution (warn), never a failure (bad).
export function worstSigTone(sigs) {
  let tone = "";
  for (const s of Array.isArray(sigs) ? sigs : []) {
    if (s.state === "expired" || s.state === "bogus" || s.state === "no_key") return "bad";
    if (s.state === "not_yet_valid" || s.state === "unsupported_algorithm" || s.state === "unsupported_key") tone = "warn";
  }
  return tone;
}

// SIG_SEVERITY ranks signature states so a set of signatures over one RRset can
// collapse to its worst member: a fresh signature must never mask an expired
// one drawn along the same path.
const SIG_SEVERITY = {
  expired: 3, bogus: 3, no_key: 3,
  not_yet_valid: 2, unsupported_algorithm: 2, unsupported_key: 2,
  unverified: 1,
  valid: 0,
};

// worstSigState returns the most severe state in sigs.
export function worstSigState(sigs) {
  let state = "valid";
  let rank = -1;
  for (const s of Array.isArray(sigs) ? sigs : []) {
    const r = SIG_SEVERITY[s.state] ?? 1;
    if (r > rank) {
      rank = r;
      state = s.state;
    }
  }
  return state;
}

// groupByKeyTag groups signatures by the key that made them, preserving order.
function groupByKeyTag(sigs) {
  const out = new Map();
  for (const s of Array.isArray(sigs) ? sigs : []) {
    const group = out.get(s.key_tag);
    if (group) group.push(s);
    else out.set(s.key_tag, [s]);
  }
  return out;
}

// sigDetail carries a signature's state and window for the component to
// localize; the component turns it into "state, from to to".
function sigDetail(sig) {
  return { state: sig.state, from: fmtDate(sig.inception), to: fmtDate(sig.expiration) };
}

// sigLine is a tip line whose label takes a localized signature detail.
function sigLine(k, sig, extra = {}) {
  return { k, p: extra, sig: sigDetail(sig) };
}

// sigTitle builds the tip lines for a signature edge. Several signatures share
// one edge when they cover the same RRset, so every window is listed.
function sigTitle(headline, sigs) {
  const lines = [headline];
  for (const sig of Array.isArray(sigs) ? sigs : [sigs]) {
    lines.push({ k: "pub.dnssec_chain_tip_signing_key", p: { tag: sig.key_tag } });
    lines.push({ k: "pub.dnssec_chain_tip_algorithm", p: { algo: algoLabel(sig.algorithm) } });
    if (sig.inception && sig.expiration) {
      lines.push({ k: "pub.dnssec_chain_tip_valid", p: { from: fmtDate(sig.inception), to: fmtDate(sig.expiration) } });
    }
    lines.push({ k: "pub.dnssec_chain_tip_status", statusState: sig.state });
    lines.push(serversTip(sig.servers));
  }
  return lines.filter(Boolean);
}

// truncateName shortens a long name with a middle ellipsis. The full name is
// meant to go into a title/tooltip via titleText.
export function truncateName(name, max = 28) {
  const s = String(name ?? "");
  if (s.length <= max) return s;
  const keep = max - 1;
  const head = Math.ceil(keep / 2);
  const tail = Math.floor(keep / 2);
  return s.slice(0, head) + "…" + s.slice(s.length - tail);
}

// relativeName drops the zone suffix so a name node reads "a.ns" inside the
// zone's own cluster. A name outside the zone keeps its full form.
export function relativeName(name, zone) {
  const n = String(name ?? "").replace(/\.$/, "");
  const z = String(zone ?? "").replace(/\.$/, "");
  if (!z || n.toLowerCase() === z.toLowerCase()) return n;
  const suffix = `.${z}`;
  if (n.toLowerCase().endsWith(suffix.toLowerCase())) return n.slice(0, n.length - suffix.length);
  return n;
}

function rowWidth(count, w) {
  if (count <= 0) return w;
  return count * w + (count - 1) * H_GAP;
}

// nodesPerLine is how many boxes of width w the row budget holds.
function nodesPerLine(w) {
  return Math.max(1, Math.floor((MAX_ROW_W + H_GAP) / (w + H_GAP)));
}

// lineCounts splits a row into balanced lines, none wider than the budget:
// five keys read as 3/2, not a full line and a lone box.
function lineCounts(count, w) {
  const lines = Math.max(1, Math.ceil(count / nodesPerLine(w)));
  const base = Math.floor(count / lines);
  const extra = count % lines;
  return Array.from({ length: lines }, (_, i) => base + (i < extra ? 1 : 0));
}

// packLines fills lines up to the budget, keeping a group on one line whenever
// it fits, so the names one zone signs stay together under their signer.
function packLines(groups, w) {
  const perLine = nodesPerLine(w);
  const out = [];
  let cur = 0;
  for (const size of groups) {
    if (cur > 0 && cur + size > perLine) {
      out.push(cur);
      cur = 0;
    }
    let rest = size;
    while (cur + rest > perLine) {
      const take = perLine - cur;
      out.push(cur + take);
      rest -= take;
      cur = 0;
    }
    cur += rest;
  }
  if (cur > 0) out.push(cur);
  return out.length > 0 ? out : [0];
}

// groupSizes counts the runs of names that share a signer.
function groupSizes(nodes) {
  const out = [];
  let prev;
  for (const n of nodes) {
    const key = n.signerId ?? "";
    if (out.length === 0 || key !== prev) out.push(1);
    else out[out.length - 1] += 1;
    prev = key;
  }
  return out;
}

// rowHeight is how tall a row stands once its lines are known.
function rowHeight(row) {
  return row.lines.length * row.h + (row.lines.length - 1) * LINE_GAP;
}

// place lays a row out line by line, each line centred in totalW.
function place(row, y, totalW, rowIndex) {
  let i = 0;
  let lineY = y;
  row.lines.forEach((count, line) => {
    const startX = PAD_X + (totalW - rowWidth(count, row.w)) / 2;
    for (let j = 0; j < count; j++, i++) {
      const n = row.nodes[i];
      n.x = startX + j * (row.w + H_GAP);
      n.y = lineY;
      n.w = row.w;
      n.h = row.h;
      n.rowIndex = rowIndex;
      n.lineIndex = line;
    }
    lineY += row.h + LINE_GAP;
  });
}

// layoutChain builds the graph. Returns null for an unsigned zone (rendered as
// a callout, not a graph) or when there is nothing to draw. words carries the
// localized frame header text, which the caller owns and the layout measures.
export function layoutChain(chain, options = {}) {
  if (!chain || chain.status === "unsigned") return null;
  const words = options.words ?? {};

  const dsList = Array.isArray(chain.parent?.ds) ? chain.parent.ds : [];
  const dsSource = chain.parent?.ds_source ?? "none";
  const dsRRSIG = Array.isArray(chain.parent?.ds_rrsig) ? chain.parent.ds_rrsig : [];
  const parentKeys = Array.isArray(chain.parent?.dnskeys) ? chain.parent.dnskeys : [];
  const keys = Array.isArray(chain.child?.dnskeys) ? chain.child.dnskeys : [];
  const dnskeySigs = Array.isArray(chain.child?.dnskey_rrsig) ? chain.child.dnskey_rrsig : [];
  const signed = Array.isArray(chain.child?.signed) ? chain.child.signed : [];
  const links = Array.isArray(chain.links) ? chain.links : [];

  // Parent DS nodes, or a dashed ghost when the zone is an island (keys, no DS).
  // DS records are grouped by key tag: a key commonly publishes the same tag
  // under several digest types (SHA-1 and SHA-256), drawn as one node with each
  // digest listed in its tooltip. Tags sort ascending to match the key row.
  const dsNodes = [];
  if (dsList.length > 0) {
    const dsSigLines = dsRRSIG.map((r) => sigLine("pub.dnssec_chain_tip_ds_sig", r, { tag: r.key_tag }));
    // The DS RRSIG covers the whole DS RRset, so its worst state tints every
    // DS node's border, making an expired DS signature visible at a glance.
    const dsSigTone = worstSigTone(dsRRSIG);
    const input = dsSource === "input";
    const byTag = new Map();
    for (const ds of dsList) {
      const group = byTag.get(ds.key_tag);
      if (group) group.push(ds);
      else byTag.set(ds.key_tag, [ds]);
    }
    for (const tag of [...byTag.keys()].sort((a, b) => a - b)) {
      const group = byTag.get(tag).slice().sort((a, b) => (a.digest_type ?? 0) - (b.digest_type ?? 0));
      const first = group[0];
      const digestLines = group.flatMap((ds) => [
        { k: "pub.dnssec_chain_tip_digest_type", p: { dt: digestLabel(ds.digest_type) } },
        ds.digest ? { k: "pub.dnssec_chain_tip_digest", p: { digest: shortHex(ds.digest) } } : null,
      ]);
      const servers = [...new Set(group.flatMap((ds) => ds.servers ?? []))];
      dsNodes.push({
        id: `ds-${tag}`,
        kind: input ? "ds-input" : "ds",
        keyTag: tag,
        dsSigTone,
        algoText: algoFace(first.algorithm),
        tip: [
          { k: input ? "pub.dnssec_chain_tip_ds_input" : "pub.dnssec_chain_tip_ds", p: { tag } },
          { k: "pub.dnssec_chain_tip_algorithm", p: { algo: algoLabel(first.algorithm) } },
          ...digestLines,
          ttlLine(first.ttl),
          ...dsSigLines,
          input ? null : serversTip(servers),
        ].filter(Boolean),
      });
    }
  } else {
    dsNodes.push({ id: "ds-ghost", kind: "ds-ghost", tip: [{ k: "pub.dnssec_chain_tip_no_ds" }] });
  }

  // Parent-zone key(s) that sign the DS RRset, drawn above the DS row.
  const parentKeyNodes = parentKeys.map((pk) => ({
    id: `pkey-${pk.key_tag}`,
    kind: "parent-key",
    keyTag: pk.key_tag,
    algoText: algoFace(pk.algorithm),
    bitsText: bitsFace(pk.key_size),
    tip: [
      { k: "pub.dnssec_chain_tip_parent_key", p: { tag: pk.key_tag } },
      { k: "pub.dnssec_chain_tip_algorithm", p: { algo: algoLabel(pk.algorithm) } },
      pk.key_size ? { k: "pub.dnssec_chain_tip_key_size", p: { bits: pk.key_size } } : null,
      ttlLine(pk.ttl),
      serversTip(pk.servers),
    ].filter(Boolean),
  }));

  // Split DNSKEYs into KSK (SEP) and ZSK rows so signing edges read downward.
  // An unanchored KSK is a rollover signal only when another KSK is anchored;
  // with no anchored key at all the zone is broken or an island, not rolling.
  const anyAnchored = keys.some((k) => k.anchored);
  const kskNodes = [];
  const zskNodes = [];
  // The DNSKEY RRset signature(s) cover every key in the set, so show them on
  // each key node with validity, like the DS and signed-RRset nodes do.
  const dnskeySigLines = dnskeySigs.map((s) => sigLine("pub.dnssec_chain_tip_dnskey_sig", s, { tag: s.key_tag }));
  for (const k of [...keys].sort((a, b) => a.key_tag - b.key_tag)) {
    const words = flagWords(k);
    const flagsText = words ? `${k.flags} (${words})` : `${k.flags}`;
    const incoming = !!k.sep && !k.anchored && anyAnchored;
    const node = {
      id: `key-${k.key_tag}`,
      keyTag: k.key_tag,
      algoText: algoFace(k.algorithm),
      bitsText: bitsFace(k.key_size),
      revoked: !!k.revoked,
      incoming,
      tip: [
        { k: "pub.dnssec_chain_tip_key", p: { role: k.sep ? "KSK" : "ZSK", tag: k.key_tag } },
        { k: "pub.dnssec_chain_tip_algorithm", p: { algo: algoLabel(k.algorithm) } },
        { k: "pub.dnssec_chain_tip_flags", p: { flags: flagsText } },
        k.key_size ? { k: "pub.dnssec_chain_tip_key_size", p: { bits: k.key_size } } : null,
        incoming ? { k: "pub.dnssec_chain_tip_unanchored" } : null,
        ttlLine(k.ttl),
        ...dnskeySigLines,
        serversTip(k.servers),
      ].filter(Boolean),
    };
    if (k.sep) {
      node.kind = "ksk";
      kskNodes.push(node);
    } else {
      node.kind = "zsk";
      zskNodes.push(node);
    }
  }

  const signedNodes = signed.map((s) => {
    const sigLines = (s.rrsig ?? []).map((r) => sigLine("pub.dnssec_chain_tip_rrset_sig", r, { tag: r.key_tag }));
    const rollover = s.ds_match === "rollover";
    const newKeys = Array.isArray(s.new_keys) ? s.new_keys : [];
    return {
      id: `rrset-${s.type}`,
      kind: "rrset",
      label: s.type,
      rollover,
      tip: [
        { k: "pub.dnssec_chain_tip_rrset", p: { type: s.type } },
        ttlLine(s.ttl),
        ...sigLines,
        s.refs?.length ? { k: "pub.dnssec_chain_tip_names_key", p: { tags: s.refs.join(", ") } } : null,
        rollover && newKeys.length ? { k: "pub.dnssec_chain_tip_rollover", p: { keys: newKeys.join(", ") } } : null,
      ].filter(Boolean),
    };
  });

  // Phantom keys: a DS or CDS/CDNSKEY names a key tag that is absent from the
  // DNSKEY RRset (an outgoing key already removed, or an incoming one not yet
  // published). Draw a grey ghost so those edges have a target.
  const keyTags = new Set(keys.map((k) => k.key_tag));
  const phantomTags = new Set();
  for (const l of links) {
    if (l.status === "no_dnskey" && !keyTags.has(l.ds_key_tag)) phantomTags.add(l.ds_key_tag);
  }
  for (const s of signed) {
    for (const t of s.refs ?? []) {
      if (!keyTags.has(t)) phantomTags.add(t);
    }
  }
  if (keys.length > 0) {
    for (const tag of phantomTags) {
      kskNodes.push({ id: `key-${tag}`, kind: "key-phantom", keyTag: tag, tip: [{ k: "pub.dnssec_chain_tip_phantom_key", p: { tag } }] });
    }
    // Order the KSK row by key tag so it matches the key-tag-ordered DS row and
    // the DS -> key edges stay parallel instead of crossing.
    kskNodes.sort((a, b) => a.keyTag - b.keyTag);
  }

  // In-domain nameserver names and the zones that sign their address records.
  // A signer that is the zone apex gets no node of its own: the chain to it is
  // already the graph above. Everything else is a zone below the apex, drawn as
  // a secure cut or, where nothing delegates it, as an orphan apex.
  const nsNames = Array.isArray(chain.ns_names) ? chain.ns_names : [];
  const signerNodes = [];
  const signerById = new Map();
  const zoneName = String(chain.zone ?? "").replace(/\.$/, "").toLowerCase();
  // A signer's own node says how the zone is reached, which is a different
  // fault from a signature that fails: orphan means nothing delegates it,
  // cut-broken means the delegation exists and its chain of trust does not.
  // Several names can share a signer, so the worst of their verdicts wins.
  const SIGNER_KIND = { orphan: "orphan", chain_broken: "cut-broken" };
  const SIGNER_RANK = { cut: 0, "cut-broken": 1, orphan: 2 };
  for (const entry of nsNames) {
    const signer = String(entry.signer ?? "").replace(/\.$/, "");
    if (!signer || signer.toLowerCase() === zoneName) continue;
    const id = `signer-${signer.toLowerCase()}`;
    const kind = SIGNER_KIND[entry.status] ?? "cut";
    const seen = signerById.get(id);
    if (seen) {
      if (SIGNER_RANK[kind] > SIGNER_RANK[seen.kind]) {
        seen.kind = kind;
        seen.signerWordKey = signerWordKey(kind);
        seen.tip = [{ k: signerTipKey(kind), p: { name: signer } }];
      }
      continue;
    }
    const node = {
      id,
      kind,
      nameText: truncateName(relativeName(signer, chain.zone), 17),
      signer,
      signerWordKey: signerWordKey(kind),
      tip: [{ k: signerTipKey(kind), p: { name: signer } }],
    };
    signerById.set(id, node);
    signerNodes.push(node);
  }
  const nsNameNodes = nsNames.map((entry) => {
    const name = String(entry.name ?? "").replace(/\.$/, "");
    const signer = String(entry.signer ?? "").replace(/\.$/, "");
    return {
      id: `nsname-${name.toLowerCase()}`,
      kind: "nsname",
      nameText: truncateName(relativeName(name, chain.zone), 17),
      nsStatus: entry.status,
      signerId: signer && signer.toLowerCase() !== zoneName ? `signer-${signer.toLowerCase()}` : null,
      tip: [
        { k: "pub.dnssec_chain_tip_nsname", p: { name } },
        signer ? { k: "pub.dnssec_chain_tip_nsname_signer", p: { signer } } : null,
        { k: "pub.dnssec_chain_tip_nsname_status", nsStatus: entry.status },
        serversTip(entry.servers),
      ].filter(Boolean),
    };
  });

  // Order names by the signer they hang from so each signer's edges run into
  // one contiguous block below it. A name the apex signs draws no edge, so
  // those trail the ones that do.
  const signerOrder = new Map(signerNodes.map((n, i) => [n.id, i]));
  const signerRank = (n) => (n.signerId ? signerOrder.get(n.signerId) ?? 0 : signerNodes.length);
  nsNameNodes.sort((a, b) => signerRank(a) - signerRank(b));

  // Assemble the visible rows top to bottom, tagging which carries a label.
  // When the parent keys are known, they sit above the DS in the parent zone.
  const row = (label, nodes, w = NODE_W, h = NODE_H, side = "zone") => ({ label, nodes, w, h, side });
  const rows = [];
  // The parent rows carry no label: their frame header says whose they are.
  if (parentKeyNodes.length > 0) {
    rows.push(row(null, parentKeyNodes, NODE_W, NODE_H, "parent"));
  }
  rows.push(row(null, dsNodes, NODE_W, NODE_H, "parent"));
  if (keys.length === 0) {
    rows.push(row("keys", [{ id: "key-ghost", kind: "key-ghost", tip: [{ k: "pub.dnssec_chain_tip_no_dnskey" }] }]));
  } else {
    let keyLabelUsed = false;
    if (kskNodes.length > 0) {
      rows.push(row("keys", kskNodes));
      keyLabelUsed = true;
    }
    if (zskNodes.length > 0) {
      rows.push(row(keyLabelUsed ? null : "keys", zskNodes));
    }
  }
  if (signedNodes.length > 0) {
    rows.push(row("signed", signedNodes, RRSET_W, LEAF_H));
  }
  if (signerNodes.length > 0) {
    rows.push(row("signers", signerNodes, NODE_W, LEAF_H));
  }
  if (nsNameNodes.length > 0) {
    const r = row("nsnames", nsNameNodes, NODE_W, LEAF_H);
    r.groups = groupSizes(nsNameNodes);
    rows.push(r);
  }

  // Wrap every row to the budget first: the widest line that survives sets the
  // column all rows are centred in.
  for (const r of rows) r.lines = r.groups ? packLines(r.groups, r.w) : lineCounts(r.nodes.length, r.w);
  const totalW = Math.max(MIN_ROW_W, ...rows.map((r) => rowWidth(Math.max(...r.lines), r.w)));

  // Each side of the delegation is framed, so its rows start below a header
  // strip and the frame closes under the last of them.
  const sides = ["parent", "zone"]
    .map((id) => ({ id, rows: rows.filter((r) => r.side === id) }))
    .filter((s) => s.rows.length > 0);
  let rowIndex = 0;
  let frameTop = FRAME_TOP;
  for (const side of sides) {
    let rowY = frameTop + FRAME_HEAD_H + (side.rows[0].label ? ROW_LABEL_GAP : FRAME_PAD_TOP);
    for (const r of side.rows) {
      place(r, rowY, totalW, rowIndex++);
      rowY += rowHeight(r) + ROW_GAP;
    }
    side.y = frameTop;
    side.h = round(rowY - ROW_GAP + FRAME_PAD_BOTTOM - frameTop);
    frameTop = round(side.y + side.h + FRAME_GAP);
  }
  const contentBottom = frameTop - FRAME_GAP;

  const nodes = rows.flatMap((r) => r.nodes);
  const byId = new Map(nodes.map((n) => [n.id, n]));

  const LABEL_KEY = {
    keys: "pub.dnssec_chain_keys_label",
    signed: "pub.dnssec_chain_signed_label",
    signers: "pub.dnssec_chain_signers_label",
    nsnames: "pub.dnssec_chain_nsnames_label",
  };
  const clusters = rows
    .filter((r) => r.label)
    .map((r) => ({ id: r.label, labelKey: LABEL_KEY[r.label], x: PAD_X, y: round(r.nodes[0].y - LABEL_BASE) }));

  const edges = [];
  // inkRight is the rightmost point anything is drawn at, so a self-loop or a
  // reference bow widens the canvas only when it actually reaches past a row.
  let inkRight = PAD_X + totalW;

  // Parent key(s) sign the DS RRset: an edge from the parent key to each DS.
  for (const [tag, group] of groupByKeyTag(dsRRSIG)) {
    const signer = byId.get(`pkey-${tag}`);
    if (!signer) continue;
    for (const ds of dsNodes) {
      if (ds.kind !== "ds" && ds.kind !== "ds-input") continue;
      edges.push({
        id: `dssig-${tag}-${ds.id}`,
        kind: "keysig",
        status: worstSigState(group),
        keyTag: tag,
        tip: sigTitle({ k: "pub.dnssec_chain_tip_rrsig_ds" }, group),
        from: edgePoint(signer, "bottom"),
        to: edgePoint(ds, "top"),
      });
    }
  }

  // DS -> DNSKEY edges from the computed links, one per DS key tag. The digest
  // types for a tag share one DS node, so their links collapse to a single
  // edge: the tag is anchored when any digest matches.
  const linksByTag = new Map();
  for (const link of links) {
    const group = linksByTag.get(link.ds_key_tag);
    if (group) group.push(link);
    else linksByTag.set(link.ds_key_tag, [link]);
  }
  for (const [dsTag, group] of linksByTag) {
    const link = group.find((l) => l.status === "match") ?? group[0];
    // Fall back to the DS node by key tag for older blobs without matching ids.
    const from = byId.get(`ds-${dsTag}`) ?? dsNodes.find((n) => n.keyTag === dsTag);
    if (!from) continue;
    let toId;
    if (link.status === "no_dnskey") {
      // Route to the phantom key the DS names, or the generic ghost when the
      // zone serves no DNSKEY at all.
      toId = byId.has(`key-${dsTag}`) ? `key-${dsTag}` : "key-ghost";
    } else {
      toId = `key-${link.dnskey_key_tag}`;
    }
    // A DS naming a key that signs nothing cannot be entered, so the record
    // itself is at fault and not only the edge leaving it.
    if (link.status === "key_not_signing") from.deadAnchor = true;
    const to = byId.get(toId);
    if (!to) {
      from.unmatched = true;
      from.tip.push({ k: "pub.dnssec_chain_tip_no_key_tag", p: { tag: dsTag } });
      continue;
    }
    const servers = [...new Set(group.flatMap((l) => l.servers ?? []))];
    edges.push({
      id: `link-${dsTag}-${link.dnskey_key_tag ?? "none"}`,
      kind: "ds",
      status: link.status,
      dnskeyKeyTag: link.dnskey_key_tag,
      tip: [
        { k: "pub.dnssec_chain_tip_link", p: { ds: dsTag, key: link.dnskey_key_tag ?? "?" } },
        { k: "pub.dnssec_chain_tip_status", linkState: link.status },
        serversTip(servers),
      ].filter(Boolean),
      from: edgePoint(from, "bottom"),
      to: edgePoint(to, "top"),
    });
  }

  // Keys that sign the DNSKEY RRset self-loop, vouch for lower-row keys, and
  // vouch for same-row KSKs that do not sign themselves (one signature covers
  // the whole RRset). One key can serve several signatures over that RRset -
  // re-signing, or a lagging secondary - and they share one path, so they
  // collapse into a single edge carrying the worst state and every window.
  const dnskeySigsByTag = groupByKeyTag(dnskeySigs);
  const signingTags = new Set(dnskeySigsByTag.keys());
  for (const [tag, group] of dnskeySigsByTag) {
    const signer = byId.get(`key-${tag}`);
    if (!signer) continue;
    const status = worstSigState(group);
    // An unanchored KSK's signatures are valid but off the chain of trust.
    const incoming = !!signer.incoming;
    inkRight = Math.max(inkRight, signer.x + signer.w + LOOP_PAD);
    edges.push({
      id: `self-${tag}`,
      kind: "selfsig",
      status,
      keyTag: tag,
      incoming,
      tip: sigTitle({ k: "pub.dnssec_chain_tip_rrsig_dnskey" }, group),
      d: selfLoopPath(signer),
    });
    for (const target of nodes) {
      if (target.kind !== "ksk" && target.kind !== "zsk") continue;
      if (target.id === signer.id) continue;
      const downward = target.rowIndex > signer.rowIndex;
      const sibling = target.rowIndex === signer.rowIndex && target.kind === "ksk" && !signingTags.has(target.keyTag);
      if (!downward && !sibling) continue;
      const edge = {
        id: `keysig-${tag}-${target.keyTag}`,
        kind: "keysig",
        status,
        keyTag: tag,
        targetTag: target.keyTag,
        incoming,
        tip: sigTitle({ k: "pub.dnssec_chain_tip_rrsig_dnskey_covers", p: { tag: target.keyTag } }, group),
      };
      if (downward) {
        edge.from = edgePoint(signer, "bottom");
        edge.to = edgePoint(target, "top");
      } else if (target.lineIndex === signer.lineIndex) {
        edge.d = siblingSigPath(signer, target);
      } else {
        // The row wrapped: the sibling sits on another line, so the edge runs
        // between the facing sides instead of bowing across one line.
        const down = target.lineIndex > signer.lineIndex;
        edge.from = edgePoint(signer, down ? "bottom" : "top");
        edge.to = edgePoint(target, down ? "top" : "bottom");
      }
      edges.push(edge);
    }
  }

  // Keys that sign zone data point at each signed RRset.
  for (const entry of signed) {
    const to = byId.get(`rrset-${entry.type}`);
    if (!to) continue;
    for (const [tag, group] of groupByKeyTag(entry.rrsig)) {
      const from = byId.get(`key-${tag}`);
      if (!from) continue;
      edges.push({
        id: `sig-${entry.type}-${tag}`,
        kind: "sig",
        status: worstSigState(group),
        keyTag: tag,
        tip: sigTitle({ k: "pub.dnssec_chain_tip_rrsig_over", p: { type: entry.type } }, group),
        from: edgePoint(from, "bottom"),
        to: edgePoint(to, "top"),
      });
    }
    // CDS/CDNSKEY name a DNSKEY by tag: draw a grey reference edge to that key.
    // It is bowed to the side so it does not sit on top of the signature edge
    // between the same two nodes.
    const newKeys = Array.isArray(entry.new_keys) ? entry.new_keys : [];
    for (const tag of entry.refs ?? []) {
      const key = byId.get(`key-${tag}`);
      if (!key) continue;
      const pending = newKeys.includes(tag);
      const a = edgePoint(to, "top");
      const b = edgePoint(key, "bottom");
      // A quadratic bows half way to its control point.
      inkRight = Math.max(inkRight, (a.x + b.x) / 2 + REF_BOW / 2 + 8);
      edges.push({
        id: `ref-${entry.type}-${tag}`,
        kind: "ref",
        rrset: entry.type,
        targetTag: tag,
        rollover: pending,
        tip: pending
          ? [{ k: "pub.dnssec_chain_tip_ref_pending", p: { type: entry.type, tag } }]
          : [{ k: "pub.dnssec_chain_tip_ref", p: { type: entry.type, tag } }],
        d: refPath(a, b),
      });
    }
  }

  // Each nameserver name points at the zone that signs its address records.
  // A name signed by the apex itself gets no edge: its signer is the key row
  // the whole graph above already reaches.
  for (const node of nsNameNodes) {
    if (!node.signerId) continue;
    const signer = byId.get(node.signerId);
    if (!signer) continue;
    edges.push({
      id: `nssig-${node.id}`,
      kind: "nssig",
      nsStatus: node.nsStatus,
      from: edgePoint(signer, "bottom"),
      to: edgePoint(node, "top"),
    });
  }

  // A stub above each signer node says how it is reached: solid into the zone
  // for a delegation the zone proves, broken for an apex nothing delegates.
  for (const node of signerNodes) {
    edges.push({
      id: `stub-${node.id}`,
      kind: "stub",
      broken: node.kind === "orphan",
      bad: node.kind !== "cut",
      d: stubPath(node),
      ticks: node.kind === "orphan" ? breakTicks(node) : null,
      tip: node.tip,
    });
  }

  const width = round(inkRight + PAD_X);
  const height = contentBottom + PAD_BOTTOM;
  // The frames span the drawing, so a self-loop or a reference bow stays inside.
  const frameX = PAD_X - FRAME_PAD_X;
  const frames = sides.map((side) => buildFrame(side, frameX, round(width - 2 * frameX), chain, words));
  for (const n of nodes) n.tone = nodeTone(n);
  return { width, height, frames, clusters, nodes, edges };
}

// The role word is drawn uppercase with tracking, so it runs wider than the estimate.
const roleWidth = (text, px) => faceWidth(text, px) * 1.25;

// parentName reads the root as the word the caller hands in, never a lone dot
// beside a full name.
function parentName(chain, words) {
  const raw = String(chain.parent_zone ?? "");
  if (raw === ".") return words.root ?? ".";
  return raw.replace(/\.$/, "");
}

// buildFrame wraps one side of the delegation: a header strip naming the zone,
// a border tinted by the roll-up, and on the tested side a status chip, which
// is what carries the verdict into an exported file.
function buildFrame(side, x, w, chain, words) {
  const parent = side.id === "parent";
  const roleText = (parent ? words.parent : words.zone) ?? "";
  const rawName = parent ? parentName(chain, words) : String(chain.zone ?? "").replace(/\.$/, "");
  const chipText = parent ? "" : words.status?.(chain.status) ?? "";
  const baseline = round(side.y + FRAME_HEAD_H / 2 + 4);
  const roleX = round(x + FRAME_TEXT_PAD);
  const nameX = round(roleX + (roleText ? roleWidth(roleText, FRAME_ROLE_FONT) + FRAME_TEXT_GAP : 0));
  let chip = null;
  if (chipText) {
    const cw = round(faceWidth(chipText, FRAME_CHIP_FONT) + 2 * CHIP_PAD_X);
    const cx = round(x + w - FRAME_TEXT_PAD - cw);
    const cy = round(side.y + (FRAME_HEAD_H - CHIP_H) / 2);
    chip = { text: chipText, x: cx, y: cy, w: cw, h: CHIP_H, textX: round(cx + cw / 2), textY: round(cy + CHIP_H / 2 + 3.5) };
  }
  // The name takes what the header has left, then elides, so it never widens
  // the drawing.
  const room = (chip ? chip.x - FRAME_TEXT_GAP : x + w - FRAME_TEXT_PAD) - nameX;
  return {
    id: side.id,
    x,
    y: side.y,
    w,
    h: side.h,
    tone: parent ? "neutral" : statusTone(chain.status),
    headPath: headPath(x, side.y, w),
    header: { roleText, roleX, roleY: baseline, name: clipToWidth(rawName, room, FRAME_NAME_FONT), nameX, nameY: baseline, chip },
  };
}

// headPath rounds the strip where it meets the frame's top corners and leaves
// it square where the rows begin.
function headPath(x, y, w, r = 10) {
  const right = round(x + w);
  return `M ${x} ${round(y + r)} A ${r} ${r} 0 0 1 ${round(x + r)} ${y} H ${round(right - r)} A ${r} ${r} 0 0 1 ${right} ${round(y + r)} V ${round(y + FRAME_HEAD_H)} H ${x} Z`;
}

// signerWordKey names the face line that says what a signer node is, so an
// orphan apex and a healthy cut differ in words and not only in colour.
function signerWordKey(kind) {
  if (kind === "orphan") return "pub.dnssec_chain_signer_orphan";
  if (kind === "cut-broken") return "pub.dnssec_chain_signer_cut_broken";
  return "pub.dnssec_chain_signer_cut";
}

// signerTipKey names the line that explains how a signer zone is reached.
function signerTipKey(kind) {
  if (kind === "orphan") return "pub.dnssec_chain_tip_orphan";
  if (kind === "cut-broken") return "pub.dnssec_chain_tip_cut_broken";
  return "pub.dnssec_chain_tip_cut";
}

// STUB_H is how far a signer node's stub reaches above it, short enough to stay
// inside the row gap and never cross the row above.
const STUB_H = 34;

// stubPath draws the stub above a signer node, with a gap in the middle when it
// is broken so the two halves read as severed.
function stubPath(node) {
  const x = round(node.x + node.w / 2);
  const top = round(node.y - STUB_H);
  const bottom = round(node.y);
  const mid = round(node.y - STUB_H / 2);
  return `M ${x} ${top} L ${x} ${round(mid - 6)} M ${x} ${round(mid + 6)} L ${x} ${bottom}`;
}

// breakTicks draws the two diagonals that mark a severed stub.
function breakTicks(node) {
  const x = round(node.x + node.w / 2);
  const mid = round(node.y - STUB_H / 2);
  const w = 9;
  const h = 7;
  return [
    `M ${round(x - w)} ${round(mid - 1)} L ${round(x + w)} ${round(mid - 1 - h)}`,
    `M ${round(x - w)} ${round(mid + 6)} L ${round(x + w)} ${round(mid + 6 - h)}`,
  ];
}

// refPath draws a reference edge as a quadratic curve bowed to the right so it
// stays clear of the straight signature edge between the same two nodes.
function refPath(a, b) {
  const mx = (a.x + b.x) / 2 + REF_BOW;
  const my = (a.y + b.y) / 2;
  return `M ${round(a.x)} ${round(a.y)} Q ${round(mx)} ${round(my)} ${round(b.x)} ${round(b.y)}`;
}

function edgePoint(node, side) {
  const cx = node.x + node.w / 2;
  return { x: round(cx), y: side === "top" ? node.y : node.y + node.h };
}

// siblingSigPath draws a gently bowed edge between two same-row KSK boxes. It
// runs low on the boxes to clear the signer's self-loop and approaches the
// target horizontally so the arrowhead points clearly into it.
function siblingSigPath(from, to) {
  const y = round(from.y + from.h * 0.72);
  const leftToRight = from.x < to.x;
  const ax = round(leftToRight ? from.x + from.w : from.x);
  const bx = round(leftToRight ? to.x : to.x + to.w);
  const span = Math.abs(bx - ax);
  const bow = Math.max(10, Math.min(22, span * 0.4));
  const dir = leftToRight ? 1 : -1;
  const c1x = round(ax + dir * span * 0.35);
  const c2x = round(bx - dir * span * 0.5);
  return `M ${ax} ${y} C ${c1x} ${round(y + bow)}, ${c2x} ${y}, ${bx} ${y}`;
}

// selfLoopPath draws a small loop off the node's top-right corner, ending on the
// right edge with the arrowhead pointing back into the node.
function selfLoopPath(node) {
  const sx = node.x + node.w * 0.66;
  const sy = node.y;
  const ex = node.x + node.w;
  const ey = node.y + node.h * 0.3;
  return `M ${round(sx)} ${round(sy)} C ${round(sx + 22)} ${round(sy - 28)}, ${round(ex + 30)} ${round(ey - 24)}, ${round(ex)} ${round(ey)}`;
}

function round(n) {
  return Math.round(n * 10) / 10;
}
