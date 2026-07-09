import { describe, it, expect } from "vitest";
import {
  moduleLevels,
  CAT_ORDER,
  CAT_LABELS,
  BONUS_HIDDEN,
  worstLevel,
  bannerClass,
  isNoticeOrAbove,
  hasScore,
  chipGrade,
  chipScore,
  resultScore,
  formatSeconds,
  formatTimingMs,
  nsRowStatus,
  nsTimingCell,
  nsSamplesCell,
  nsStatusLabelKey,
  entryMessage,
  moduleId,
  summaryRows,
  okTestcaseCount,
  groupRawEntries,
} from "./result.js";

describe("constants", () => {
  it("module levels run from NOTICE to CRITICAL", () => {
    expect(moduleLevels).toEqual(["NOTICE", "WARNING", "ERROR", "CRITICAL"]);
  });

  it("scoring categories follow the canonical order", () => {
    expect(CAT_ORDER).toEqual(["dnssec", "nameserver_health", "connectivity", "zone_consistency"]);
    expect(CAT_LABELS.dnssec).toBe("DNSSEC");
  });

  it("bonus criteria has the no_warnings_or_errors entry hidden", () => {
    expect(BONUS_HIDDEN.has("no_warnings_or_errors")).toBe(true);
  });
});

describe("worstLevel", () => {
  it("returns INFO for empty input", () => {
    expect(worstLevel([])).toBe("INFO");
    expect(worstLevel(null)).toBe("INFO");
  });

  it("returns the most severe level present", () => {
    expect(worstLevel([{ level: "NOTICE" }, { level: "ERROR" }, { level: "WARNING" }])).toBe("ERROR");
    expect(worstLevel([{ level: "CRITICAL" }, { level: "INFO" }])).toBe("CRITICAL");
  });
});

describe("bannerClass", () => {
  it("maps levels to css class names", () => {
    expect(bannerClass("CRITICAL")).toBe("critical");
    expect(bannerClass("ERROR")).toBe("error");
    expect(bannerClass("WARNING")).toBe("warning");
    expect(bannerClass("NOTICE")).toBe("ok");
    expect(bannerClass(null)).toBe("ok");
  });
});

describe("isNoticeOrAbove", () => {
  it("returns true for NOTICE and above", () => {
    expect(isNoticeOrAbove("NOTICE")).toBe(true);
    expect(isNoticeOrAbove("ERROR")).toBe(true);
    expect(isNoticeOrAbove("CRITICAL")).toBe(true);
  });

  it("returns false for INFO and DEBUG", () => {
    expect(isNoticeOrAbove("INFO")).toBe(false);
    expect(isNoticeOrAbove("DEBUG")).toBe(false);
  });
});

describe("hasScore / chipGrade / chipScore / resultScore", () => {
  it("hasScore requires both score and grade", () => {
    expect(hasScore({ score: 90, grade: "A" })).toBe(true);
    expect(hasScore({ score: 90 })).toBe(false);
    expect(hasScore({ grade: "A" })).toBe(false);
    expect(hasScore(null)).toBe(false);
  });

  it("chip helpers return null for missing fields", () => {
    expect(chipGrade(null)).toBeNull();
    expect(chipScore(null)).toBeNull();
    expect(chipGrade({ grade: "B" })).toBe("B");
    expect(chipScore({ score: 80 })).toBe(80);
  });

  it("resultScore returns the score sub-object or null", () => {
    expect(resultScore(null)).toBeNull();
    expect(resultScore({ score: { grade: "A" } })).toEqual({ grade: "A" });
  });
});

describe("formatSeconds / formatTimingMs", () => {
  it("formatSeconds returns 2 decimals or '0.00'", () => {
    expect(formatSeconds(1.234)).toBe("1.23");
    expect(formatSeconds("garbage")).toBe("0.00");
  });

  it("formatTimingMs rounds to integer string or '0'", () => {
    expect(formatTimingMs(12.7)).toBe("13");
    expect(formatTimingMs(NaN)).toBe("0");
  });
});

describe("nameserver timing row status", () => {
  it("treats unset/ok status as ok", () => {
    expect(nsRowStatus({})).toBe("ok");
    expect(nsRowStatus({ status: "ok" })).toBe("ok");
    expect(nsRowStatus(null)).toBe("ok");
  });

  it("passes through unreachable and unresolved", () => {
    expect(nsRowStatus({ status: "unreachable" })).toBe("unreachable");
    expect(nsRowStatus({ status: "unresolved" })).toBe("unresolved");
  });

  it("renders timing cells: ms for ok, infinity for unreachable, dash for unresolved", () => {
    expect(nsTimingCell({ status: "ok" }, 12.7)).toBe("13");
    expect(nsTimingCell({ status: "unreachable" }, 5000)).toBe("∞");
    expect(nsTimingCell({ status: "unresolved" }, 0)).toBe("-");
  });

  it("renders the samples cell: count for ok, 0 for unreachable, dash for unresolved", () => {
    expect(nsSamplesCell({ status: "ok", count: 4 })).toBe("4");
    expect(nsSamplesCell({ status: "unreachable" })).toBe("0");
    expect(nsSamplesCell({ status: "unresolved" })).toBe("-");
  });

  it("maps status to a badge i18n key, empty for ok", () => {
    expect(nsStatusLabelKey({ status: "unreachable" })).toBe("ns_timing_status_unreachable");
    expect(nsStatusLabelKey({ status: "unresolved" })).toBe("ns_timing_status_unresolved");
    expect(nsStatusLabelKey({ status: "ok" })).toBe("");
  });
});

describe("entryMessage", () => {
  it("prefers entry.message", () => {
    expect(entryMessage({ message: "hi" })).toBe("hi");
  });

  it("falls back to entry.raw", () => {
    expect(entryMessage({ raw: "raw text" })).toBe("raw text");
  });

  it("composes from module/testcase/tag when neither is set", () => {
    expect(entryMessage({ module: "DNSSEC", testcase: "dnssec01", tag: "X" })).toBe(
      "DNSSEC:dnssec01:X",
    );
  });

  it("returns empty for null entry", () => {
    expect(entryMessage(null)).toBe("");
  });
});

describe("moduleId", () => {
  it("creates a stable slug from arbitrary keys", () => {
    expect(moduleId("DNSSEC Module")).toBe("module-dnssec-module");
    expect(moduleId("zone_consistency!")).toBe("module-zone-consistency-");
  });
});

describe("summaryRows", () => {
  it("returns levels with non-zero counts only", () => {
    expect(
      summaryRows({ levels: { NOTICE: 0, WARNING: 2, ERROR: 1, CRITICAL: 0 } }),
    ).toEqual([
      { level: "WARNING", count: 2 },
      { level: "ERROR", count: 1 },
    ]);
  });

  it("returns empty array for missing summary", () => {
    expect(summaryRows(null)).toEqual([]);
    expect(summaryRows({})).toEqual([]);
  });
});

describe("okTestcaseCount", () => {
  it("counts test cases whose level is below NOTICE", () => {
    const group = {
      testcasesArr: [
        { level: "INFO" },
        { level: "DEBUG" },
        { level: "WARNING" },
        { level: "ERROR" },
      ],
    };
    expect(okTestcaseCount(group)).toBe(2);
  });
});

describe("groupRawEntries", () => {
  it("returns an empty array for missing raw", () => {
    expect(groupRawEntries(null)).toEqual([]);
    expect(groupRawEntries({})).toEqual([]);
  });

  it("groups by module name and counts levels", () => {
    const raw = {
      entries: [
        { module: "DNSSEC", level: "WARNING", testcase: "dnssec01" },
        { module: "DNSSEC", level: "ERROR", testcase: "dnssec01" },
        { module: "DNSSEC", level: "INFO" },
        { module: "Connectivity", level: "INFO", testcase: "connectivity01" },
      ],
    };
    const groups = groupRawEntries(raw);
    expect(groups).toHaveLength(2);
    const dnssec = groups.find((g) => g.key === "DNSSEC");
    expect(dnssec.counts).toEqual({ WARNING: 1, ERROR: 1, INFO: 1 });
    expect(dnssec.testcasesArr).toHaveLength(1);
    expect(dnssec.testcasesArr[0].level).toBe("ERROR");
    expect(dnssec.ungrouped).toHaveLength(1);
  });

  it("places SYSTEM module first regardless of insertion order", () => {
    const raw = {
      entries: [
        { module: "DNSSEC", level: "INFO" },
        { module: "System", level: "INFO" },
      ],
    };
    expect(groupRawEntries(raw)[0].key).toBe("SYSTEM");
  });

  it("treats 'Unspecified' testcase as ungrouped", () => {
    const raw = {
      entries: [
        { module: "Mod", level: "INFO", testcase: "Unspecified" },
        { module: "Mod", level: "INFO" },
      ],
    };
    const [g] = groupRawEntries(raw);
    expect(g.testcasesArr).toHaveLength(0);
    expect(g.ungrouped).toHaveLength(2);
  });
});
