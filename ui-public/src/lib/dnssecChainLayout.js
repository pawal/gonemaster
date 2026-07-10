// Pure layout for the DNSSEC chain-of-trust graph. Given a chain summary it
// returns geometry ({width, height, clusters, nodes, edges}) with no DOM
// dependency, so it is fully unit-testable. The component renders the geometry
// as SVG; all coordinates are plain numbers used as SVG attributes.

const NODE_W = 132;
const NODE_H = 52;
const H_GAP = 20;
const V_GAP = 84;
const PAD_X = 24;
const PAD_TOP = 44; // room for the cluster label above the first row
const PAD_BOTTOM = 16;

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

// place assigns centered x positions to a row of nodes within totalW.
function place(nodes, y, totalW) {
  const w = rowWidth(nodes.length);
  const startX = PAD_X + (totalW - w) / 2;
  nodes.forEach((n, i) => {
    n.x = startX + i * (NODE_W + H_GAP);
    n.y = y;
    n.w = NODE_W;
    n.h = NODE_H;
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
  const soaSigs = Array.isArray(chain.child?.soa_rrsig) ? chain.child.soa_rrsig : [];
  const links = Array.isArray(chain.links) ? chain.links : [];

  // Row 0: DS nodes, or a dashed ghost when the zone is an island (keys, no DS).
  const dsNodes = [];
  if (dsList.length > 0) {
    for (const ds of dsList) {
      dsNodes.push({
        id: `ds-${ds.key_tag}`,
        kind: dsSource === "input" ? "ds-input" : "ds",
        keyTag: ds.key_tag,
        algo: ds.algorithm,
        digestType: ds.digest_type,
        titleText: `DS key tag ${ds.key_tag}, algorithm ${ds.algorithm}, digest ${ds.digest_type}`,
      });
    }
  } else {
    dsNodes.push({ id: "ds-ghost", kind: "ds-ghost", titleText: "No DS at the parent" });
  }

  // Row 1: DNSKEY nodes, SEP (KSK) first; a dashed ghost when DS exists but no key.
  const keyNodes = [];
  if (keys.length > 0) {
    const sorted = [...keys].sort((a, b) => (b.sep ? 1 : 0) - (a.sep ? 1 : 0) || a.key_tag - b.key_tag);
    for (const k of sorted) {
      keyNodes.push({
        id: `key-${k.key_tag}`,
        kind: k.sep ? "ksk" : "zsk",
        keyTag: k.key_tag,
        algo: k.algorithm,
        titleText: `${k.sep ? "KSK" : "ZSK"} key tag ${k.key_tag}, algorithm ${k.algorithm}`,
      });
    }
  } else {
    keyNodes.push({ id: "key-ghost", kind: "key-ghost", titleText: "No DNSKEY at the zone" });
  }

  // Row 2: signed RRsets. DNSKEY is always shown; SOA only when a signature exists.
  const rrsetNodes = [{ id: "rrset-dnskey", kind: "rrset", label: "DNSKEY", titleText: "DNSKEY RRset" }];
  if (soaSigs.length > 0) {
    rrsetNodes.push({ id: "rrset-soa", kind: "rrset", label: "SOA", titleText: "SOA RRset" });
  }

  const rows = [dsNodes, keyNodes, rrsetNodes];
  const totalW = Math.max(...rows.map((r) => rowWidth(r.length)));

  rows.forEach((row, i) => place(row, PAD_TOP + i * V_GAP, totalW));

  const nodes = [...dsNodes, ...keyNodes, ...rrsetNodes];
  const byId = new Map(nodes.map((n) => [n.id, n]));

  const clusters = [
    { id: "parent", labelKey: "pub.dnssec_chain_parent_label", x: PAD_X, y: PAD_TOP - 22 },
    { id: "keys", labelKey: "pub.dnssec_chain_keys_label", x: PAD_X, y: PAD_TOP + V_GAP - 22 },
    { id: "signed", labelKey: "pub.dnssec_chain_signed_label", x: PAD_X, y: PAD_TOP + 2 * V_GAP - 22 },
  ];

  const edges = [];

  // DS -> DNSKEY edges from the computed links.
  for (const link of links) {
    const from = byId.get(`ds-${link.ds_key_tag}`);
    if (!from) continue;
    let toId;
    if (link.status === "no_dnskey") {
      toId = byId.has("key-ghost") ? "key-ghost" : `key-${link.dnskey_key_tag}`;
    } else {
      toId = `key-${link.dnskey_key_tag}`;
    }
    const to = byId.get(toId);
    if (!to) continue;
    edges.push({
      id: `link-${link.ds_key_tag}-${link.dnskey_key_tag ?? "none"}`,
      kind: "ds",
      status: link.status,
      from: edgePoint(from, "bottom"),
      to: edgePoint(to, "top"),
    });
  }

  // Key -> signed-RRset edges, colored by signature state.
  for (const sig of dnskeySigs) {
    const from = byId.get(`key-${sig.key_tag}`);
    const to = byId.get("rrset-dnskey");
    if (!from || !to) continue;
    edges.push({
      id: `sig-dnskey-${sig.key_tag}`,
      kind: "sig",
      status: sig.state,
      from: edgePoint(from, "bottom"),
      to: edgePoint(to, "top"),
    });
  }
  for (const sig of soaSigs) {
    const from = byId.get(`key-${sig.key_tag}`);
    const to = byId.get("rrset-soa");
    if (!from || !to) continue;
    edges.push({
      id: `sig-soa-${sig.key_tag}`,
      kind: "sig",
      status: sig.state,
      from: edgePoint(from, "bottom"),
      to: edgePoint(to, "top"),
    });
  }

  const width = totalW + 2 * PAD_X;
  const height = PAD_TOP + 2 * V_GAP + NODE_H + PAD_BOTTOM;
  return { width, height, clusters, nodes, edges };
}

function edgePoint(node, side) {
  const cx = node.x + node.w / 2;
  return { x: cx, y: side === "top" ? node.y : node.y + node.h };
}
