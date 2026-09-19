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
import type { ReportResponse, ReportTagEntry } from "./api";

const sampleReport: ReportResponse = {
  dataset_tag: "tld",
  from_slug: "s1",
  to_slug: "s2",
  min_cluster: 3,
  max_spread: 3,
  header: {
    from: { slug: "s1", captured_at: "2026-06-03T00:00:00Z", engine_version: "1.2.0", domain_count: 290 },
    to: { slug: "s2", captured_at: "2026-09-18T00:00:00Z", engine_version: "1.3.0", domain_count: 290 },
    engine: {
      from_engine_version: "1.2.0",
      to_engine_version: "1.3.0",
      crossed_engine_versions: true
    },
    vocabulary: {
      from_available: true,
      to_available: true,
      from_tag_count: 614,
      to_tag_count: 686,
      added: [{ tag: "Z15_NO_CAA", module: "ZONE", level: "NOTICE" }],
      removed: [],
      level_changed: []
    },
    scoring_config_changed: "false",
    tag_floor: "NOTICE"
  },
  totals: {
    from_domain_count: 290,
    to_domain_count: 290,
    both_domain_count: 290,
    added: 0,
    removed: 0,
    identical_score: 202,
    improved: 14,
    regressed: 18,
    from_mean_score: 91.07,
    to_mean_score: 91.21,
    domain_categories: { real: 10, measurement: 8, mixed: 4, unknown: 0 }
  },
  tags: {
    appeared: [
      {
        tag: "Z15_NO_CAA",
        module: "ZONE",
        to_level: "NOTICE",
        from_domain_count: 0,
        to_domain_count: 120,
        domain_delta: 120,
        classification: "new_in_engine"
      },
      {
        tag: "Z09_NO_RESPONSE_MX_QUERY",
        module: "ZONE",
        to_level: "WARNING",
        from_domain_count: 3,
        to_domain_count: 19,
        domain_delta: 16,
        classification: "cohort_change"
      }
    ],
    cleared: [],
    level_changed: []
  },
  domains: [
    {
      domain: "osteraker.se",
      from_score: 85,
      to_score: 65,
      score_delta: -20,
      from_grade: "B",
      to_grade: "D",
      grade_changed: true,
      category: "real",
      explained_delta: -20,
      unexplained_delta: 0,
      appeared: [
        {
          tag: "DS08_DNSKEY_RRSIG_EXPIRED",
          module: "DNSSEC",
          to_level: "ERROR",
          classification: "cohort_change"
        }
      ],
      cleared: [],
      level_changed: []
    },
    {
      domain: "salem.se",
      from_score: 100,
      to_score: 99,
      score_delta: -1,
      from_grade: "A",
      to_grade: "A",
      grade_changed: false,
      category: "measurement",
      explained_delta: -1,
      unexplained_delta: 0,
      appeared: [
        { tag: "Z15_NO_CAA", module: "ZONE", to_level: "NOTICE", classification: "new_in_engine" }
      ],
      cleared: [],
      level_changed: []
    }
  ],
  clusters: [
    {
      dimensions: [
        { dimension: "nameserver", value: "ns1.example", total_domains: 41 },
        { dimension: "software_version", value: "PowerDNS 5.0.7", total_domains: 9 }
      ],
      domains: ["a.se", "b.se", "c.se"],
      size: 3,
      min_delta: 8,
      max_delta: 9,
      direction: "improved"
    }
  ],
  domain_total: 2
};

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
