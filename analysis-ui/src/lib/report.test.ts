import { reportFixture } from "../test/helpers";
import { describe, expect, it } from "vitest";
import {
  CATEGORY_LABELS,
  categoryTone,
  clusterDimensionSummary,
  filterDomainsByCategory,
  isEngineChange,
  reportFilename,
  reportToMarkdown,
  signedNumber,
  splitTagsByClassification
} from "./report";
import type { ReportTagEntry } from "./api";

const sampleReport = reportFixture();

describe("signedNumber", () => {
  it.each([
    [5, "+5"],
    [-5, "-5"],
    [0, "0"],
    [undefined, ""]
  ])("renders %s as %s", (input, want) => {
    expect(signedNumber(input as number | undefined)).toBe(want);
  });
});

describe("isEngineChange", () => {
  it.each([
    ["new_in_engine", true],
    ["removed_from_engine", true],
    ["level_reclassified", true],
    ["cohort_change", false],
    ["unknown", false]
  ] as const)("%s is engine-driven: %s", (classification, want) => {
    expect(isEngineChange(classification)).toBe(want);
  });
});

describe("categoryTone", () => {
  it.each([
    ["real", "error"],
    ["mixed", "warning"],
    ["measurement", "notice"],
    ["unknown", "neutral"]
  ] as const)("%s uses the %s tone", (category, want) => {
    expect(categoryTone(category)).toBe(want);
  });
});

describe("splitTagsByClassification", () => {
  it("leads with cohort changes and separates engine and unknown rows", () => {
    const entries = sampleReport.tags.appeared.concat([
      {
        tag: "MYSTERY",
        from_domain_count: 1,
        to_domain_count: 2,
        domain_delta: 1,
        classification: "unknown"
      } as ReportTagEntry
    ]);
    const split = splitTagsByClassification(entries);
    expect(split.cohort.map((e) => e.tag)).toEqual(["Z09_NO_RESPONSE_MX_QUERY"]);
    expect(split.engine.map((e) => e.tag)).toEqual(["Z15_NO_CAA"]);
    expect(split.unknown.map((e) => e.tag)).toEqual(["MYSTERY"]);
  });

  it("returns empty buckets for no entries", () => {
    const split = splitTagsByClassification(undefined);
    expect(split.cohort.length).toBe(0);
    expect(split.engine.length).toBe(0);
    expect(split.unknown.length).toBe(0);
  });
});

describe("filterDomainsByCategory", () => {
  it("returns every mover when no category is selected", () => {
    expect(filterDomainsByCategory(sampleReport.domains, "").length).toBe(2);
  });

  it("keeps only the requested category", () => {
    const rows = filterDomainsByCategory(sampleReport.domains, "measurement");
    expect(rows.map((r) => r.domain)).toEqual(["salem.se"]);
  });
});

describe("clusterDimensionSummary", () => {
  it("names every dimension with its coverage", () => {
    expect(clusterDimensionSummary(sampleReport.clusters[0])).toBe(
      "nameserver ns1.example (3 of 41), software_version PowerDNS 5.0.7 (3 of 9)"
    );
  });
});

describe("reportToMarkdown", () => {
  const md = reportToMarkdown(sampleReport);

  it("heads the document with the pair", () => {
    expect(md.startsWith("# tld: s1 to s2\n")).toBe(true);
  });

  it("states the vocabulary move and the scoring state", () => {
    expect(md).toContain("Tag vocabulary 614 to 686 tags: 1 added, 0 removed, 0 reclassified.");
    expect(md).toContain("Scoring configuration unchanged.");
  });

  it("agrees with the response totals", () => {
    expect(md).toContain(
      "290 domains on both sides: 202 scored identically, 14 improved, 18 regressed."
    );
    expect(md).toContain("| Mean score | 91.07 | 91.21 |");
    expect(md).toContain(`| ${CATEGORY_LABELS.real} | 10 |`);
  });

  it("lists the clusters with their members", () => {
    expect(md).toContain("## Clusters");
    expect(md).toContain("+8 to +9");
    expect(md).toContain("a.se, b.se, c.se");
  });

  it("classifies every tag row", () => {
    expect(md).toContain("| Z15_NO_CAA | NOTICE | +120 | New in engine |");
    expect(md).toContain("| Z09_NO_RESPONSE_MX_QUERY | WARNING | +16 | Cohort change |");
  });

  it("lists the movers with delta, explained and cause", () => {
    expect(md).toContain("| osteraker.se | 85 to 65 | -20 | -20 | B to D | Real |");
    expect(md).toContain("| salem.se | 100 to 99 | -1 | -1 | A | Measurement |");
  });

  it("says so when the vocabulary is unknown on one side", () => {
    const blind = reportToMarkdown({
      ...sampleReport,
      header: {
        ...sampleReport.header,
        vocabulary: { ...sampleReport.header.vocabulary, from_available: false },
        scoring_config_changed: "unknown"
      }
    });
    expect(blind).toContain("Tag vocabulary unknown on at least one side");
    expect(blind).toContain("Scoring configuration provenance unknown");
  });

  it("marks a partial movers page and stays silent on a whole one", () => {
    expect(md).not.toContain("Showing");
    const paged = reportToMarkdown({ ...sampleReport, domain_total: 290, domain_offset: 0 });
    expect(paged).toContain("Showing 2 of 290 movers, from offset 0.");
  });
});

describe("reportFilename", () => {
  it("names the file after the cohort and the pair", () => {
    expect(reportFilename(sampleReport)).toBe("tld-report-s1-s2.md");
  });
});
