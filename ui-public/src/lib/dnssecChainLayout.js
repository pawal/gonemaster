// Pure layout for the DNSSEC chain graph: chain summary in, SVG geometry out.
// A key signing the DNSKEY RRset self-loops and vouches for the keys below it.

const NODE_W = 132;
const NODE_H = 52;
const H_GAP = 20;
const V_GAP = 104;
const PAD_X = 24;
const PAD_TOP = 44;
const PAD_BOTTOM = 16;
const LOOP_PAD = 36; // right margin so a key self-loop is not clipped
const REF_BOW = 46; // sideways bow of a CDS/CDNSKEY reference edge

const ALGO = {
  1: "RSAMD5", 3: "DSA", 5: "RSASHA1", 6: "DSA-NSEC3-SHA1", 7: "RSASHA1-NSEC3-SHA1",
  8: "RSASHA256", 10: "RSASHA512", 12: "ECC-GOST", 13: "ECDSAP256SHA256",
  14: "ECDSAP384SHA384", 15: "ED25519", 16: "ED448", 17: "SM2SM3", 23: "ECC-GOST12",
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

function serversLine(servers) {
  if (!Array.isArray(servers) || servers.length === 0) return null;
  const shown = servers.slice(0, 4).join(", ");
  return servers.length > 4 ? `Servers: ${shown}, +${servers.length - 4}` : `Servers: ${shown}`;
}

// fmtDate renders a unix-second timestamp as an ISO date.
export function fmtDate(sec) {
  if (!sec) return "";
  return new Date(sec * 1000).toISOString().slice(0, 10);
}

function joinLines(lines) {
  return lines.filter(Boolean).join("\n");
}

// sigInline renders a signature's state and validity window on one line.
function sigInline(sig) {
  let s = sig.state;
  if (sig.inception && sig.expiration) {
    s += `, ${fmtDate(sig.inception)} to ${fmtDate(sig.expiration)}`;
  }
  return s;
}

// sigTitle builds the multi-line hover text for one RRSIG.
function sigTitle(headline, sig) {
  const lines = [headline, `Signing key: ${sig.key_tag}`, `Algorithm: ${algoLabel(sig.algorithm)}`];
  if (sig.inception && sig.expiration) {
    lines.push(`Valid: ${fmtDate(sig.inception)} to ${fmtDate(sig.expiration)}`);
  }
  lines.push(`Status: ${sig.state}`);
  lines.push(serversLine(sig.servers));
  return joinLines(lines);
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

function rowWidth(count) {
  if (count <= 0) return NODE_W;
  return count * NODE_W + (count - 1) * H_GAP;
}

function place(nodes, y, totalW, rowIndex) {
  const w = rowWidth(nodes.length);
  const startX = PAD_X + (totalW - w) / 2;
  nodes.forEach((n, i) => {
    n.x = startX + i * (NODE_W + H_GAP);
    n.y = y;
    n.w = NODE_W;
    n.h = NODE_H;
    n.rowIndex = rowIndex;
  });
}

// layoutChain builds the graph. Returns null for an unsigned zone (rendered as
// a callout, not a graph) or when there is nothing to draw.
export function layoutChain(chain) {
  if (!chain || chain.status === "unsigned") return null;

  const dsList = Array.isArray(chain.parent?.ds) ? chain.parent.ds : [];
  const dsSource = chain.parent?.ds_source ?? "none";
  const keys = Array.isArray(chain.child?.dnskeys) ? chain.child.dnskeys : [];
  const dnskeySigs = Array.isArray(chain.child?.dnskey_rrsig) ? chain.child.dnskey_rrsig : [];
  const signed = Array.isArray(chain.child?.signed) ? chain.child.signed : [];
  const links = Array.isArray(chain.links) ? chain.links : [];

  // Parent DS nodes, or a dashed ghost when the zone is an island (keys, no DS).
  const dsNodes = [];
  if (dsList.length > 0) {
    const dsRRSIG = Array.isArray(chain.parent?.ds_rrsig) ? chain.parent.ds_rrsig : [];
    const dsSigLines = dsRRSIG.map((r) => `DS RRset signature (key ${r.key_tag}): ${sigInline(r)}`);
    for (const ds of dsList) {
      const input = dsSource === "input";
      dsNodes.push({
        id: `ds-${ds.key_tag}-${ds.digest_type ?? 0}`,
        kind: input ? "ds-input" : "ds",
        keyTag: ds.key_tag,
        digestType: ds.digest_type ?? 0,
        titleText: joinLines([
          `DS · key tag ${ds.key_tag}${input ? " (test input)" : ""}`,
          `Algorithm: ${algoLabel(ds.algorithm)}`,
          `Digest type: ${digestLabel(ds.digest_type)}`,
          ds.digest ? `Digest: ${shortHex(ds.digest)}` : null,
          ...dsSigLines,
          input ? null : serversLine(ds.servers),
        ]),
      });
    }
  } else {
    dsNodes.push({ id: "ds-ghost", kind: "ds-ghost", titleText: "No DS at the parent" });
  }

  // Split DNSKEYs into KSK (SEP) and ZSK rows so signing edges read downward.
  const kskNodes = [];
  const zskNodes = [];
  for (const k of [...keys].sort((a, b) => a.key_tag - b.key_tag)) {
    const words = flagWords(k);
    const signsSet = dnskeySigs
      .filter((s) => s.key_tag === k.key_tag)
      .map((s) => `Signs DNSKEY RRset: ${sigInline(s)}`);
    const node = {
      id: `key-${k.key_tag}`,
      keyTag: k.key_tag,
      revoked: !!k.revoked,
      titleText: joinLines([
        `${k.sep ? "KSK" : "ZSK"} · key tag ${k.key_tag}`,
        `Algorithm: ${algoLabel(k.algorithm)}`,
        `Flags: ${k.flags}${words ? ` (${words})` : ""}`,
        k.key_size ? `Key size: ${k.key_size} bits` : null,
        ...signsSet,
        serversLine(k.servers),
      ]),
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
    const sigLines = (s.rrsig ?? []).map((r) => `Signature by key ${r.key_tag}: ${sigInline(r)}`);
    return {
      id: `rrset-${s.type}`,
      kind: "rrset",
      label: s.type,
      titleText: joinLines([
        `${s.type} RRset`,
        ...sigLines,
        s.refs?.length ? `Names key: ${s.refs.join(", ")}` : null,
      ]),
    };
  });

  // Assemble the visible rows top to bottom, tagging which carries a label.
  const rows = [{ label: "parent", nodes: dsNodes }];
  if (keys.length === 0) {
    rows.push({ label: "keys", nodes: [{ id: "key-ghost", kind: "key-ghost", titleText: "No DNSKEY at the zone" }] });
  } else {
    let keyLabelUsed = false;
    if (kskNodes.length > 0) {
      rows.push({ label: "keys", nodes: kskNodes });
      keyLabelUsed = true;
    }
    if (zskNodes.length > 0) {
      rows.push({ label: keyLabelUsed ? null : "keys", nodes: zskNodes });
    }
  }
  if (signedNodes.length > 0) {
    rows.push({ label: "signed", nodes: signedNodes });
  }

  const totalW = Math.max(...rows.map((r) => rowWidth(r.nodes.length)));
  rows.forEach((r, i) => place(r.nodes, PAD_TOP + i * V_GAP, totalW, i));

  const nodes = rows.flatMap((r) => r.nodes);
  const byId = new Map(nodes.map((n) => [n.id, n]));

  const nameFor = (label) => {
    if (label === "parent") return truncateName(chain.parent_zone ?? "");
    if (label === "keys") return truncateName(chain.zone ?? "");
    return "";
  };
  const labelKeyFor = (label) => `pub.dnssec_chain_${label === "parent" ? "parent_label" : label === "keys" ? "keys_label" : "signed_label"}`;
  const clusters = rows
    .filter((r) => r.label)
    .map((r) => ({ id: r.label, labelKey: labelKeyFor(r.label), name: nameFor(r.label), x: PAD_X, y: r.nodes[0].y - 22 }));

  const edges = [];

  // DS -> DNSKEY edges from the computed links. Older blobs lack
  // ds_digest_type; fall back to the first DS node with the key tag.
  for (const link of links) {
    const from =
      byId.get(`ds-${link.ds_key_tag}-${link.ds_digest_type ?? 0}`) ??
      dsNodes.find((n) => n.keyTag === link.ds_key_tag);
    if (!from) continue;
    const toId = link.status === "no_dnskey" && byId.has("key-ghost") ? "key-ghost" : `key-${link.dnskey_key_tag}`;
    const to = byId.get(toId);
    if (!to) {
      // A stale DS names a retired key while other keys exist: no target node
      // to draw to, so mark the DS node itself as broken.
      if (link.status === "no_dnskey") {
        from.unmatched = true;
        from.titleText += `\nNo DNSKEY with tag ${link.ds_key_tag}`;
      }
      continue;
    }
    edges.push({
      id: `link-${link.ds_key_tag}-${link.ds_digest_type ?? 0}-${link.dnskey_key_tag ?? "none"}`,
      kind: "ds",
      status: link.status,
      dnskeyKeyTag: link.dnskey_key_tag,
      title: joinLines([
        `DS ${link.ds_key_tag} -> DNSKEY ${link.dnskey_key_tag ?? "?"}`,
        `Status: ${link.status}`,
        serversLine(link.servers),
      ]),
      from: edgePoint(from, "bottom"),
      to: edgePoint(to, "top"),
    });
  }

  // Keys that sign the DNSKEY RRset self-loop and vouch for lower-row keys.
  // Edge ids carry the inception: one key can serve overlapping signatures.
  for (const sig of dnskeySigs) {
    const signer = byId.get(`key-${sig.key_tag}`);
    if (!signer) continue;
    edges.push({
      id: `self-${sig.key_tag}-${sig.inception ?? 0}`,
      kind: "selfsig",
      status: sig.state,
      keyTag: sig.key_tag,
      title: sigTitle("RRSIG over DNSKEY RRset", sig),
      d: selfLoopPath(signer),
    });
    for (const target of nodes) {
      if ((target.kind !== "ksk" && target.kind !== "zsk") || target.id === signer.id) continue;
      if (target.rowIndex <= signer.rowIndex) continue;
      edges.push({
        id: `keysig-${sig.key_tag}-${sig.inception ?? 0}-${target.keyTag}`,
        kind: "keysig",
        status: sig.state,
        keyTag: sig.key_tag,
        targetTag: target.keyTag,
        title: sigTitle(`RRSIG over DNSKEY RRset (covers key ${target.keyTag})`, sig),
        from: edgePoint(signer, "bottom"),
        to: edgePoint(target, "top"),
      });
    }
  }

  // Keys that sign zone data point at each signed RRset.
  for (const entry of signed) {
    const to = byId.get(`rrset-${entry.type}`);
    if (!to) continue;
    for (const sig of entry.rrsig ?? []) {
      const from = byId.get(`key-${sig.key_tag}`);
      if (!from) continue;
      edges.push({
        id: `sig-${entry.type}-${sig.key_tag}-${sig.inception ?? 0}`,
        kind: "sig",
        status: sig.state,
        keyTag: sig.key_tag,
        title: sigTitle(`RRSIG over ${entry.type} RRset`, sig),
        from: edgePoint(from, "bottom"),
        to: edgePoint(to, "top"),
      });
    }
    // CDS/CDNSKEY name a DNSKEY by tag: draw a grey reference edge to that key.
    // It is bowed to the side so it does not sit on top of the signature edge
    // between the same two nodes.
    for (const tag of entry.refs ?? []) {
      const key = byId.get(`key-${tag}`);
      if (!key) continue;
      edges.push({
        id: `ref-${entry.type}-${tag}`,
        kind: "ref",
        rrset: entry.type,
        targetTag: tag,
        title: `${entry.type} names DNSKEY ${tag}`,
        d: refPath(edgePoint(to, "top"), edgePoint(key, "bottom")),
      });
    }
  }

  const hasLoop = edges.some((e) => e.kind === "selfsig");
  const hasRef = edges.some((e) => e.kind === "ref");
  const rightPad = Math.max(hasLoop ? LOOP_PAD : 0, hasRef ? REF_BOW + 12 : 0);
  const width = totalW + 2 * PAD_X + rightPad;
  const height = PAD_TOP + (rows.length - 1) * V_GAP + NODE_H + PAD_BOTTOM;
  return { width, height, clusters, nodes, edges };
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
