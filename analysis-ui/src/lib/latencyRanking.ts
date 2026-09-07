import type { ASNView, EndpointView, LatencyFields, NameserverView } from "$lib/api";

// Latency is measured from wherever this instance runs, so every surface
// that shows it repeats the caveat.
export const LATENCY_VANTAGE_NOTE =
  "Measured from this instance's network location; a single vantage point.";

// One normalized row in a front-page latency ranking; href is built by the caller.
export type LatencyRankRow = {
  key: string;
  label: string;
  sublabel: string;
  latencyP50: number;
  latencyP95: number | null;
  samples: number;
};

// Drop rows without a median (server already filters), then normalize.
function toLatencyRows<T extends LatencyFields>(
  items: T[],
  identify: (item: T) => { key: string; label: string; sublabel?: string }
): LatencyRankRow[] {
  const rows: LatencyRankRow[] = [];
  for (const item of items) {
    if (item.latency_p50_ms == null) continue;
    const id = identify(item);
    rows.push({
      key: id.key,
      label: id.label,
      sublabel: id.sublabel ?? "",
      latencyP50: item.latency_p50_ms,
      latencyP95: item.latency_p95_ms ?? null,
      samples: item.latency_samples ?? 0
    });
  }
  return rows;
}

export function nameserverLatencyRows(items: NameserverView[]): LatencyRankRow[] {
  return toLatencyRows(items, (n) => ({ key: n.nameserver, label: n.nameserver }));
}

export function endpointLatencyRows(items: EndpointView[]): LatencyRankRow[] {
  return toLatencyRows(items, (e) => ({
    key: `${e.address}|${e.nameserver}`,
    label: e.address,
    sublabel: e.nameserver
  }));
}

export function asnLatencyRows(items: ASNView[]): LatencyRankRow[] {
  return toLatencyRows(items, (a) => ({
    key: String(a.asn),
    label: `AS${a.asn}`,
    sublabel: a.label ?? ""
  }));
}

// True when any list has rows; lets the front page hide the whole section.
export function hasAnyLatencyRanking(lists: LatencyRankRow[][]): boolean {
  return lists.some((l) => l.length > 0);
}
