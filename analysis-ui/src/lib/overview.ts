// Pure helpers for the front-page hero tiles: turn a trend series into a
// per-snapshot metric and summarize its latest value and movement. DOM-free
// so the arithmetic is unit tested without rendering.

import type { TrendPoint, TrendKeyMeta } from "./api";

function asCounts(payload: unknown): Record<string, number> {
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) return {};
  return payload as Record<string, number>;
}

function total(counts: Record<string, number>): number {
  let sum = 0;
  for (const v of Object.values(counts)) sum += Number(v) || 0;
  return sum;
}

// Total domains per snapshot: the sum of any distribution's buckets.
export function seriesTotals(points: TrendPoint[]): number[] {
  return (points ?? []).map((p) => total(asCounts(p.payload)));
}

// Percentage of each snapshot's domains whose bucket tone is in `tones`.
export function seriesShareByTone(
  points: TrendPoint[],
  keyMeta: Record<string, TrendKeyMeta>,
  tones: string[]
): number[] {
  const want = new Set(tones);
  return (points ?? []).map((p) => {
    const counts = asCounts(p.payload);
    const t = total(counts);
    if (t === 0) return 0;
    let hit = 0;
    for (const [key, count] of Object.entries(counts)) {
      if (want.has(keyMeta?.[key]?.tone ?? "")) hit += Number(count) || 0;
    }
    return Math.round((hit / t) * 1000) / 10;
  });
}

// Percentage of each snapshot's domains falling in an explicit set of bucket
// keys (e.g. grades A+/A).
export function seriesShareByKeys(points: TrendPoint[], keys: string[]): number[] {
  const want = new Set(keys);
  return (points ?? []).map((p) => {
    const counts = asCounts(p.payload);
    const t = total(counts);
    if (t === 0) return 0;
    let hit = 0;
    for (const [key, count] of Object.entries(counts)) {
      if (want.has(key)) hit += Number(count) || 0;
    }
    return Math.round((hit / t) * 1000) / 10;
  });
}

export type MetricSummary = {
  values: number[];
  latest: number | null;
  previous: number | null;
  delta: number | null;
};

// Reduce a value series to its latest value and movement vs the prior point.
export function summarizeMetric(values: number[]): MetricSummary {
  if (!values || values.length === 0) {
    return { values: [], latest: null, previous: null, delta: null };
  }
  const latest = values[values.length - 1];
  const previous = values.length >= 2 ? values[values.length - 2] : null;
  const delta = previous === null ? null : Math.round((latest - previous) * 10) / 10;
  return { values, latest, previous, delta };
}
