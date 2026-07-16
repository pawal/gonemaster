// Pure helpers for the trends view: pull one bucket into a single series for
// the focus line, and rank the biggest tag movers across a top_tags time
// series. DOM-free so both are unit tested without rendering.

import type { TrendPoint, TopTagEntry } from "./api";

export type SeriesLike = {
  slug: string;
  label: string;
  buckets: { key: string; count: number }[];
};

export type PinnedPoint = { slug: string; label: string; count: number; share: number };

// Extract one bucket's count and share across snapshots, aligned to the input
// order. Missing bucket or empty snapshot yields zero.
export function pinnedSeries(series: SeriesLike[], key: string): PinnedPoint[] {
  return series.map((s) => {
    const total = s.buckets.reduce((sum, b) => sum + b.count, 0);
    const count = s.buckets.find((b) => b.key === key)?.count ?? 0;
    const share = total > 0 ? Math.round((count / total) * 1000) / 10 : 0;
    return { slug: s.slug, label: s.label, count, share };
  });
}

export type TagMover = {
  tag: string;
  level: string;
  first: number;
  last: number;
  delta: number;
};

function asTopTags(payload: unknown): TopTagEntry[] {
  return Array.isArray(payload) ? (payload as TopTagEntry[]) : [];
}

// Rank tags by how much their domain_count moved between the first and last
// point of a top_tags time series. A tag absent from one end counts as 0
// there, so newly-appearing or fully-cleared tags surface. Sorted by absolute
// movement, largest first; ties broken by tag name.
export function computeTagMovers(points: TrendPoint[], limit = 10): TagMover[] {
  if (!points || points.length < 2) return [];
  const first = new Map<string, TopTagEntry>();
  const last = new Map<string, TopTagEntry>();
  for (const t of asTopTags(points[0].payload)) first.set(t.tag, t);
  for (const t of asTopTags(points[points.length - 1].payload)) last.set(t.tag, t);

  const tags = new Set<string>([...first.keys(), ...last.keys()]);
  const movers: TagMover[] = [];
  for (const tag of tags) {
    const f = first.get(tag)?.domain_count ?? 0;
    const l = last.get(tag)?.domain_count ?? 0;
    const delta = l - f;
    if (delta === 0) continue;
    movers.push({
      tag,
      level: last.get(tag)?.level ?? first.get(tag)?.level ?? "",
      first: f,
      last: l,
      delta
    });
  }
  movers.sort((a, b) => {
    const byMag = Math.abs(b.delta) - Math.abs(a.delta);
    if (byMag !== 0) return byMag;
    return a.tag.localeCompare(b.tag);
  });
  return movers.slice(0, limit);
}
