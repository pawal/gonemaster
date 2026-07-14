// Pure helpers for reading a snapshot diff: which way a domain moved, how far,
// and how the grade transitions distribute. DOM-free so the semantics are unit
// tested without rendering. The diff API returns only changed/added/removed
// domains, so the transition matrix covers off-diagonal moves only - unchanged
// domains are not in the payload.

import type { DiffEntry, DiffResponse } from "./api";

// Grades best-to-worst; index is the rank, so a larger index is worse.
export const GRADE_ORDER = ["A+", "A", "B", "C", "D", "F"];

// Severity levels least-to-most severe; a larger index is worse.
export const LEVEL_ORDER = ["NOTICE", "WARNING", "ERROR", "CRITICAL"];

export type Direction = "regressed" | "improved" | "neutral";

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
  grades: string[];
  // cells[fromIdx][toIdx] = number of domains moving from->to.
  cells: number[][];
  max: number;
  total: number;
};

// Build a from-grade x to-grade count matrix from the grade-changed entries.
// Only off-diagonal (actually changed) cells are populated; entries with an
// unrankable grade on either side are ignored.
export function buildGradeMatrix(entries: DiffEntry[]): GradeMatrix {
  const n = GRADE_ORDER.length;
  const cells: number[][] = Array.from({ length: n }, () => Array(n).fill(0));
  let max = 0;
  let total = 0;
  for (const e of entries ?? []) {
    const from = gradeRank(e.from_grade);
    const to = gradeRank(e.to_grade);
    if (from === null || to === null) continue;
    cells[from][to]++;
    total++;
    if (cells[from][to] > max) max = cells[from][to];
  }
  return { grades: [...GRADE_ORDER], cells, max, total };
}
