// Pure layout for the two-column transition flow: links in, SVG geometry out.

export const ADDED_KEY = "new";
export const REMOVED_KEY = "removed";

// One band of domains; from equal to to is a held band, category "" uncategorised.
export type FlowLink = { from: string; to: string; category: string; count: number };

export type FlowNodeRect = {
  side: "from" | "to";
  key: string;
  x: number;
  y: number;
  width: number;
  height: number;
  count: number;
  // Label baseline, pushed apart so small nodes do not collide.
  labelY: number;
};

export type FlowRibbon = FlowLink & { path: string; held: boolean };

export type FlowLayout = {
  width: number;
  height: number;
  nodes: FlowNodeRect[];
  ribbons: FlowRibbon[];
  // Domains on links whose ends are not in the order.
  dropped: number;
};

export type FlowOptions = {
  width?: number;
  height?: number;
  nodeWidth?: number;
  // Room for the node labels outside each column.
  labelWidth?: number;
  gap?: number;
  minNodeHeight?: number;
  minRibbon?: number;
  padY?: number;
  labelGap?: number;
};

const DEFAULTS = {
  width: 640,
  height: 320,
  nodeWidth: 12,
  labelWidth: 72,
  gap: 2,
  minNodeHeight: 2,
  minRibbon: 1,
  padY: 8,
  labelGap: 13
};

function round(n: number): number {
  return Math.round(n * 100) / 100;
}

function rankOf(order: string[]): (key: string) => number {
  const index = new Map(order.map((k, i) => [k, i]));
  return (key) => index.get(key) ?? order.length;
}

// Lay out links between an ordered From column and an ordered To column.
export function layoutFlow(
  order: string[],
  links: FlowLink[],
  categoryOrder: string[],
  options: FlowOptions = {}
): FlowLayout {
  const o = { ...DEFAULTS, ...options };
  const leftOrder = [...order, ADDED_KEY];
  const rightOrder = [...order, REMOVED_KEY];
  const leftRank = rankOf(leftOrder);
  const rightRank = rankOf(rightOrder);
  const categoryRank = rankOf(["", ...categoryOrder]);

  const kept: FlowLink[] = [];
  let dropped = 0;
  for (const link of links) {
    if (link.count <= 0) continue;
    if (leftRank(link.from) >= leftOrder.length || rightRank(link.to) >= rightOrder.length) {
      dropped += link.count;
      continue;
    }
    kept.push(link);
  }

  const total = (side: "from" | "to", key: string) =>
    kept.reduce((sum, l) => sum + ((side === "from" ? l.from : l.to) === key ? l.count : 0), 0);
  const leftKeys = leftOrder.filter((k) => total("from", k) > 0);
  const rightKeys = rightOrder.filter((k) => total("to", k) > 0);
  const maxCount = Math.max(
    0,
    leftKeys.reduce((s, k) => s + total("from", k), 0),
    rightKeys.reduce((s, k) => s + total("to", k), 0)
  );
  const maxNodes = Math.max(leftKeys.length, rightKeys.length);
  const scale = maxCount > 0 ? (o.height - 2 * o.padY - o.gap * Math.max(0, maxNodes - 1)) / maxCount : 0;
  const thickness = (count: number) => Math.max(o.minRibbon, count * scale);

  const leftX = o.labelWidth;
  const rightX = o.width - o.labelWidth - o.nodeWidth;

  // Slots stack in the other end's order, then by category.
  function column(side: "from" | "to", keys: string[], x: number) {
    const nodes: FlowNodeRect[] = [];
    const slotTop = new Map<FlowLink, number>();
    let cursor = o.padY;
    for (const key of keys) {
      const own = kept
        .filter((l) => (side === "from" ? l.from : l.to) === key)
        .sort((a, b) => {
          const other = side === "from" ? rightRank(a.to) - rightRank(b.to) : leftRank(a.from) - leftRank(b.from);
          return other || categoryRank(a.category) - categoryRank(b.category);
        });
      let slot = cursor;
      for (const link of own) {
        slotTop.set(link, slot);
        slot += thickness(link.count);
      }
      const height = Math.max(o.minNodeHeight, slot - cursor);
      nodes.push({
        side,
        key,
        x,
        y: round(cursor),
        width: o.nodeWidth,
        height: round(height),
        count: own.reduce((s, l) => s + l.count, 0),
        labelY: 0
      });
      cursor += height + o.gap;
    }
    let prev = -Infinity;
    for (const node of nodes) {
      node.labelY = round(Math.max(node.y + node.height / 2 + 4, prev + o.labelGap));
      prev = node.labelY;
    }
    return { nodes, slotTop, bottom: cursor - o.gap };
  }

  const left = column("from", leftKeys, leftX);
  const right = column("to", rightKeys, rightX);

  const x0 = leftX + o.nodeWidth;
  const x1 = rightX;
  const xm = round((x0 + x1) / 2);
  const ordered = [...kept].sort(
    (a, b) =>
      leftRank(a.from) - leftRank(b.from) ||
      rightRank(a.to) - rightRank(b.to) ||
      categoryRank(a.category) - categoryRank(b.category)
  );
  const ribbons: FlowRibbon[] = ordered.map((link) => {
    const t = thickness(link.count);
    const ys = round(left.slotTop.get(link) ?? 0);
    const yt = round(right.slotTop.get(link) ?? 0);
    const ys2 = round(ys + t);
    const yt2 = round(yt + t);
    const path =
      `M ${x0} ${ys} C ${xm} ${ys} ${xm} ${yt} ${x1} ${yt} ` +
      `L ${x1} ${yt2} C ${xm} ${yt2} ${xm} ${ys2} ${x0} ${ys2} Z`;
    return { ...link, path, held: link.from === link.to };
  });

  const lastLabel = Math.max(0, ...[...left.nodes, ...right.nodes].map((n) => n.labelY));
  const height = round(Math.max(left.bottom, right.bottom, lastLabel) + o.padY);
  return { width: o.width, height: maxCount > 0 ? height : 0, nodes: [...left.nodes, ...right.nodes], ribbons, dropped };
}
