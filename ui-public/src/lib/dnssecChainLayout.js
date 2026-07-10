// Pure layout for the DNSSEC chain-of-trust graph. Given a chain summary it
// returns geometry ({width, height, clusters, nodes, edges}) with no DOM
// dependency, so it is fully unit-testable. The component renders the geometry
// as SVG; all coordinates are plain numbers used as SVG attributes.
//
// The model follows the DNSViz convention: a key that signs the DNSKEY RRset
// self-signs (a loop) and vouches for the other keys in the set, so KSKs sit in
// a row above the ZSKs with downward "signs" edges; ZSKs then sign zone data.

const NODE_W = 132;
const NODE_H = 52;
const H_GAP = 20;
const V_GAP = 104;
const PAD_X = 24;
const PAD_TOP = 44;
const PAD_BOTTOM = 16;
const LOOP_PAD = 36; // right margin so a key self-loop is not clipped

const ALGO = {
  1: "RSAMD5", 3: "DSA", 5: "RSASHA1", 6: "DSA-NSEC3-SHA1", 7: "RSASHA1-NSEC3-SHA1",
  8: "RSASHA256", 10: "RSASHA512", 12: "ECC-GOST", 13: "ECDSAP256SHA256",
  14: "ECDSAP384SHA384", 15: "ED25519", 16: "ED448",
};

function algoLabel(algo) {
  const m = ALGO[algo];
  return m ? `${m} (alg ${algo})` : `alg ${algo}`;
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
    for (const ds of dsList) {
      dsNodes.push({
        id: `ds-${ds.key_tag}`,
        kind: dsSource === "input" ? "ds-input" : "ds",
        keyTag: ds.key_tag,
        titleText: `DS ${ds.key_tag}, ${algoLabel(ds.algorithm)}, digest type ${ds.digest_type}`,
      });
    }
  } else {
    dsNodes.push({ id: "ds-ghost", kind: "ds-ghost", titleText: "No DS at the parent" });
  }

  // Split DNSKEYs into KSK (SEP) and ZSK rows so signing edges read downward.
  const kskNodes = [];
  const zskNodes = [];
  for (const k of [...keys].sort((a, b) => a.key_tag - b.key_tag)) {
    const bits = k.key_size ? `, ${k.key_size} bits` : "";
    const node = {
      id: `key-${k.key_tag}`,
      keyTag: k.key_tag,
      titleText: `${k.sep ? "KSK" : "ZSK"} ${k.key_tag}, ${algoLabel(k.algorithm)}${bits}`,
    };
    if (k.sep) {
      node.kind = "ksk";
      kskNodes.push(node);
    } else {
      node.kind = "zsk";
      zskNodes.push(node);
    }
  }

  const signedNodes = signed.map((s) => ({
    id: `rrset-${s.type}`,
    kind: "rrset",
    label: s.type,
    titleText: `${s.type} RRset`,
  }));

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

  // DS -> DNSKEY edges from the computed links.
  for (const link of links) {
    const from = byId.get(`ds-${link.ds_key_tag}`);
    if (!from) continue;
    const toId = link.status === "no_dnskey" && byId.has("key-ghost") ? "key-ghost" : `key-${link.dnskey_key_tag}`;
    const to = byId.get(toId);
    if (!to) continue;
    edges.push({
      id: `link-${link.ds_key_tag}-${link.dnskey_key_tag ?? "none"}`,
      kind: "ds",
      status: link.status,
      dsKeyTag: link.ds_key_tag,
      dnskeyKeyTag: link.dnskey_key_tag,
      from: edgePoint(from, "bottom"),
      to: edgePoint(to, "top"),
    });
  }

  // Keys that sign the DNSKEY RRset self-loop and vouch for lower-row keys.
  for (const sig of dnskeySigs) {
    const signer = byId.get(`key-${sig.key_tag}`);
    if (!signer) continue;
    edges.push({
      id: `self-${sig.key_tag}`,
      kind: "selfsig",
      status: sig.state,
      keyTag: sig.key_tag,
      inception: sig.inception,
      expiration: sig.expiration,
      d: selfLoopPath(signer),
    });
    for (const target of nodes) {
      if ((target.kind !== "ksk" && target.kind !== "zsk") || target.id === signer.id) continue;
      if (target.rowIndex <= signer.rowIndex) continue;
      edges.push({
        id: `keysig-${sig.key_tag}-${target.keyTag}`,
        kind: "keysig",
        status: sig.state,
        keyTag: sig.key_tag,
        targetTag: target.keyTag,
        inception: sig.inception,
        expiration: sig.expiration,
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
        id: `sig-${entry.type}-${sig.key_tag}`,
        kind: "sig",
        status: sig.state,
        keyTag: sig.key_tag,
        rrset: entry.type,
        inception: sig.inception,
        expiration: sig.expiration,
        from: edgePoint(from, "bottom"),
        to: edgePoint(to, "top"),
      });
    }
    // CDS/CDNSKEY name a DNSKEY by tag: draw a grey reference edge to that key.
    for (const tag of entry.refs ?? []) {
      const key = byId.get(`key-${tag}`);
      if (!key) continue;
      edges.push({
        id: `ref-${entry.type}-${tag}`,
        kind: "ref",
        rrset: entry.type,
        targetTag: tag,
        from: edgePoint(to, "top"),
        to: edgePoint(key, "bottom"),
      });
    }
  }

  const hasLoop = edges.some((e) => e.kind === "selfsig");
  const width = totalW + 2 * PAD_X + (hasLoop ? LOOP_PAD : 0);
  const height = PAD_TOP + (rows.length - 1) * V_GAP + NODE_H + PAD_BOTTOM;
  return { width, height, clusters, nodes, edges };
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
