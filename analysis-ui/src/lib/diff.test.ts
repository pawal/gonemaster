import { describe, expect, it } from "vitest";
import {
  buildGradeMatrix,
  gradeDirection,
  gradeMovement,
  gradeRank,
  levelDirection,
  netDirection,
  sortByMovement,
  summarizeDiff
} from "./diff";
import type { DiffResponse } from "./api";

// The diff API hands back only changed/added/removed domains, so these tests
// pin the direction and magnitude semantics the view sorts and colours by. A
// larger grade/level rank is worse, so a positive movement is a regression.

describe("gradeRank", () => {
  it("ranks A+ best and F worst", () => {
    expect(gradeRank("A+")).toBe(0);
    expect(gradeRank("F")).toBe(5);
  });

  it("is case- and whitespace-insensitive", () => {
    expect(gradeRank("  b ")).toBe(2);
  });

  it("returns null for grades outside the known set", () => {
    // Custom scoring can emit grades we don't rank; those must not be forced
    // into a direction.
    expect(gradeRank("Z")).toBeNull();
    expect(gradeRank(null)).toBeNull();
  });
});

describe("gradeMovement and gradeDirection", () => {
  it("treats a move toward F as a regression", () => {
    const entry = { domain: "a.se", from_grade: "B", to_grade: "D" };
    expect(gradeMovement(entry)).toBe(2);
    expect(gradeDirection(entry)).toBe("regressed");
  });

  it("treats a move toward A+ as an improvement", () => {
    const entry = { domain: "a.se", from_grade: "D", to_grade: "A" };
    expect(gradeMovement(entry)).toBe(-3);
    expect(gradeDirection(entry)).toBe("improved");
  });

  it("is neutral when a grade is unrankable", () => {
    expect(gradeDirection({ domain: "a.se", from_grade: "Z", to_grade: "A" })).toBe("neutral");
  });
});

describe("levelDirection", () => {
  it("treats rising severity as a regression", () => {
    expect(levelDirection({ domain: "a.se", from_level: "WARNING", to_level: "CRITICAL" })).toBe(
      "regressed"
    );
  });

  it("treats falling severity as an improvement", () => {
    expect(levelDirection({ domain: "a.se", from_level: "ERROR", to_level: "NOTICE" })).toBe(
      "improved"
    );
  });
});

describe("netDirection", () => {
  it("prefers grade movement over level movement", () => {
    // Grade improved but severity rose: net follows the grade.
    const entry = {
      domain: "a.se",
      from_grade: "D",
      to_grade: "A",
      from_level: "NOTICE",
      to_level: "CRITICAL"
    };
    expect(netDirection(entry)).toBe("improved");
  });

  it("falls back to level movement when the grade did not move", () => {
    const entry = {
      domain: "a.se",
      from_grade: "B",
      to_grade: "B",
      from_level: "NOTICE",
      to_level: "ERROR"
    };
    expect(netDirection(entry)).toBe("regressed");
  });
});

describe("summarizeDiff", () => {
  const diff: DiffResponse = {
    dataset_tag: "tld",
    from_slug: "s1",
    to_slug: "s2",
    added: [{ domain: "new1.se" }, { domain: "new2.se" }],
    removed: [{ domain: "gone.se" }],
    grade_changed: [
      { domain: "reg.se", from_grade: "B", to_grade: "F" },
      { domain: "imp.se", from_grade: "D", to_grade: "A" }
    ],
    level_changed: [
      // Same domain as a grade change: must not double-count.
      { domain: "reg.se", from_level: "WARNING", to_level: "CRITICAL" },
      // Level-only regression for a domain not in grade_changed.
      { domain: "lvl.se", from_level: "NOTICE", to_level: "ERROR" }
    ]
  };

  it("counts added and removed straight from the lists", () => {
    const s = summarizeDiff(diff);
    expect(s.added).toBe(2);
    expect(s.removed).toBe(1);
  });

  it("counts each changed domain once under its net direction", () => {
    const s = summarizeDiff(diff);
    // reg.se (grade regressed) + lvl.se (level regressed) = 2 regressed;
    // imp.se improved. reg.se must not be counted twice despite appearing in
    // both changed lists.
    expect(s.regressed).toBe(2);
    expect(s.improved).toBe(1);
  });

  it("returns zeroes for a null diff", () => {
    expect(summarizeDiff(null)).toEqual({ added: 0, removed: 0, regressed: 0, improved: 0 });
  });
});

describe("sortByMovement", () => {
  it("orders the worst regression first, then alphabetically", () => {
    const entries = [
      { domain: "b.se", from_grade: "B", to_grade: "C" }, // +1
      { domain: "a.se", from_grade: "A", to_grade: "F" }, // +4
      { domain: "c.se", from_grade: "C", to_grade: "A" } // -2 (improved)
    ];
    const sorted = sortByMovement(entries, "grade").map((e) => e.domain);
    expect(sorted).toEqual(["a.se", "b.se", "c.se"]);
  });

  it("does not mutate the input array", () => {
    const entries = [
      { domain: "b.se", from_grade: "B", to_grade: "C" },
      { domain: "a.se", from_grade: "A", to_grade: "F" }
    ];
    const before = entries.map((e) => e.domain);
    sortByMovement(entries, "grade");
    expect(entries.map((e) => e.domain)).toEqual(before);
  });
});

describe("buildGradeMatrix", () => {
  it("tallies transitions into from x to cells", () => {
    const matrix = buildGradeMatrix([
      { domain: "a.se", from_grade: "B", to_grade: "D" },
      { domain: "b.se", from_grade: "B", to_grade: "D" },
      { domain: "c.se", from_grade: "A", to_grade: "F" }
    ]);
    // B is index 2, D is index 4; two domains took that path.
    expect(matrix.cells[2][4]).toBe(2);
    expect(matrix.cells[1][5]).toBe(1); // A -> F
    expect(matrix.max).toBe(2);
    expect(matrix.total).toBe(3);
  });

  it("skips entries with an unrankable grade", () => {
    const matrix = buildGradeMatrix([{ domain: "x.se", from_grade: "Z", to_grade: "A" }]);
    expect(matrix.total).toBe(0);
    expect(matrix.max).toBe(0);
  });
});
