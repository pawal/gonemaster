// Pure helpers for reading a snapshot diff: which way a domain moved, how far,
// and how the grade transitions distribute. DOM-free so the semantics are unit
// tested without rendering. The diff API returns only changed/added/removed
// domains, so the transition matrix covers off-diagonal moves only - unchanged
// domains are not in the payload.

import type { DiffEntry, DiffResponse, ReportTransition } from "./api";
import { ADDED_KEY, REMOVED_KEY, type FlowLink } from "./charts/flowLayout";

// Grades best-to-worst; index is the rank, so a larger index is worse.
export const GRADE_ORDER = ["A+", "A", "B", "C", "D", "F"];

// Severity levels least-to-most severe; a larger index is worse.
export const LEVEL_ORDER = ["NOTICE", "WARNING", "ERROR", "CRITICAL"];

export type Direction = "regressed" | "improved" | "neutral";

// The snapshot preceding `toSlug` in a newest-first list, or "" when `to` is
// unknown or already the oldest. Lets the diff loader default the other side.
export function previousSlug(snapshots: { slug: string }[], toSlug: string): string {
  if (!toSlug) return "";
  const idx = snapshots.findIndex((s) => s.slug === toSlug);
  if (idx < 0) return "";
  return snapshots[idx + 1]?.slug ?? "";
}

function rankIn(order: string[], value: string | null | undefined): number | null {
  const idx = order.indexOf(String(value ?? "").trim().toUpperCase());
  return idx < 0 ? null : idx;
}

export function gradeRank(grade: string | null | undefined): number | null {
  return rankIn(GRADE_ORDER, grade);
}

export function levelRank(level: string | null | undefined): number | null {
  return rankIn(LEVEL_ORDER, level);
}

// Signed movement between two ranks: positive means it got worse (regressed),
// negative means it improved. Null when either side is unrankable.
function movement(
  rank: (v: string | null | undefined) => number | null,
  from: string | null | undefined,
  to: string | null | undefined
): number | null {
  const a = rank(from);
  const b = rank(to);
  if (a === null || b === null) return null;
  return b - a;
}

function directionOf(move: number | null): Direction {
  if (move === null || move === 0) return "neutral";
  return move > 0 ? "regressed" : "improved";
}

export function gradeMovement(entry: DiffEntry): number | null {
  return movement(gradeRank, entry.from_grade, entry.to_grade);
}

export function levelMovement(entry: DiffEntry): number | null {
  return movement(levelRank, entry.from_level, entry.to_level);
}

export function gradeDirection(entry: DiffEntry): Direction {
  return directionOf(gradeMovement(entry));
}

export function levelDirection(entry: DiffEntry): Direction {
  return directionOf(levelMovement(entry));
}

// Net direction for a changed domain: grade movement wins when known,
// otherwise fall back to the worst-level movement.
export function netDirection(entry: DiffEntry): Direction {
  const g = gradeMovement(entry);
  if (g !== null && g !== 0) return g > 0 ? "regressed" : "improved";
  return directionOf(levelMovement(entry));
}

export type DiffSummary = {
  added: number;
  removed: number;
  regressed: number;
  improved: number;
};

// Roll the four diff lists into headline counts. A domain that changed both
// grade and worst-level is counted once, under its net direction.
export function summarizeDiff(diff: DiffResponse | null | undefined): DiffSummary {
  const summary: DiffSummary = {
    added: diff?.added?.length ?? 0,
    removed: diff?.removed?.length ?? 0,
    regressed: 0,
    improved: 0
  };
  if (!diff) return summary;

  const byDomain = new Map<string, DiffEntry>();
  for (const e of diff.grade_changed ?? []) byDomain.set(e.domain, e);
  // Merge level-only changes without clobbering a grade change already seen.
  for (const e of diff.level_changed ?? []) {
    const existing = byDomain.get(e.domain);
    if (existing) {
      byDomain.set(e.domain, { ...existing, from_level: e.from_level, to_level: e.to_level });
    } else {
      byDomain.set(e.domain, e);
    }
  }
  for (const entry of byDomain.values()) {
    const dir = netDirection(entry);
    if (dir === "regressed") summary.regressed++;
    else if (dir === "improved") summary.improved++;
  }
  return summary;
}

// Sort a copy of the entries by absolute movement, worst regression first,
// then alphabetically. `by` selects grade or level movement.
export function sortByMovement(entries: DiffEntry[], by: "grade" | "level"): DiffEntry[] {
  const move = by === "grade" ? gradeMovement : levelMovement;
  return [...entries].sort((a, b) => {
    const ma = move(a) ?? 0;
    const mb = move(b) ?? 0;
    if (mb !== ma) return mb - ma; // larger (more regressed) first
    return a.domain.localeCompare(b.domain);
  });
}

export type GradeMatrix = {
  keys: string[];
  // cells[fromIdx][toIdx] = number of domains moving from->to.
  cells: number[][];
  max: number;
  total: number;
};

export type Dimension = "grade" | "level";

// Worst-level buckets best to worst, as the cohort report counts them.
export const SEVERITY_BUCKETS = ["OK", "NOTICE", "WARNING", "ERROR", "CRITICAL"];

// A worst level as the report buckets it; anything below NOTICE is OK.
export function severityBucket(level: string | null | undefined): string {
  const upper = String(level ?? "").trim().toUpperCase();
  return SEVERITY_BUCKETS.includes(upper) ? upper : "OK";
}

export function dimensionOrder(dim: Dimension): string[] {
  return dim === "grade" ? GRADE_ORDER : SEVERITY_BUCKETS;
}

// One side of an entry in a dimension; "" for a missing grade.
export function sideValue(entry: DiffEntry, dim: Dimension, side: "from" | "to"): string {
  if (dim === "level") return severityBucket(side === "from" ? entry.from_level : entry.to_level);
  return String((side === "from" ? entry.from_grade : entry.to_grade) ?? "").trim().toUpperCase();
}

// Build a from x to count matrix over changed entries; unknown values are ignored.
export function buildMatrix(entries: DiffEntry[], dim: Dimension): GradeMatrix {
  const keys = dimensionOrder(dim);
  const cells: number[][] = Array.from({ length: keys.length }, () => Array(keys.length).fill(0));
  let max = 0;
  let total = 0;
  for (const e of entries ?? []) {
    const from = keys.indexOf(sideValue(e, dim, "from"));
    const to = keys.indexOf(sideValue(e, dim, "to"));
    if (from < 0 || to < 0 || from === to) continue;
    cells[from][to]++;
    total++;
    if (cells[from][to] > max) max = cells[from][to];
  }
  return { keys: [...keys], cells, max, total };
}

export function buildGradeMatrix(entries: DiffEntry[]): GradeMatrix {
  return buildMatrix(entries, "grade");
}

// Entries moving from one value to another; an empty side matches any.
export function filterByPair(entries: DiffEntry[], dim: Dimension, from: string, to: string): DiffEntry[] {
  if (!from && !to) return entries;
  return entries.filter(
    (e) => (!from || sideValue(e, dim, "from") === from) && (!to || sideValue(e, dim, "to") === to)
  );
}

// Category counts per moved pair, largest first, keyed "from|to".
export function pairCategories(rows: ReportTransition[]): Map<string, { category: string; count: number }[]> {
  const out = new Map<string, { category: string; count: number }[]>();
  for (const r of rows) {
    if (r.from === r.to) continue;
    const key = `${r.from}|${r.to}`;
    out.set(key, [...(out.get(key) ?? []), { category: r.category ?? "unknown", count: r.count }]);
  }
  for (const list of out.values()) list.sort((a, b) => b.count - a.count);
  return out;
}

// Report transitions plus the domains the diff added or removed, as flow links.
export function flowLinks(rows: ReportTransition[], diff: DiffResponse | null | undefined, dim: Dimension): FlowLink[] {
  const links: FlowLink[] = rows.map((r) => ({ from: r.from, to: r.to, category: r.category ?? "", count: r.count }));
  const tally = (entries: DiffEntry[] | undefined, side: "from" | "to") => {
    const counts = new Map<string, number>();
    for (const e of entries ?? []) {
      const value = sideValue(e, dim, side);
      counts.set(value, (counts.get(value) ?? 0) + 1);
    }
    return counts;
  };
  for (const [value, count] of tally(diff?.added, "to")) {
    links.push({ from: ADDED_KEY, to: value, category: "", count });
  }
  for (const [value, count] of tally(diff?.removed, "from")) {
    links.push({ from: value, to: REMOVED_KEY, category: "", count });
  }
  return links;
}
