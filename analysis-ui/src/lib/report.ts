// Pure helpers for reading a cohort report: how to label a classification,
// how to group the tag tables, and how to render the whole response as
// Markdown. DOM-free so the export is unit tested without rendering.

import type {
  ChangeClassification,
  DomainCategory,
  ReportCluster,
  ReportDomain,
  ReportResponse,
  ReportTagEntry
} from "./api";

export const CLASSIFICATION_LABELS: Record<ChangeClassification, string> = {
  cohort_change: "Cohort change",
  new_in_engine: "New in engine",
  removed_from_engine: "Removed from engine",
  level_reclassified: "Severity reclassified",
  unknown: "Unknown"
};

export const CATEGORY_LABELS: Record<DomainCategory, string> = {
  real: "Real",
  measurement: "Measurement",
  mixed: "Mixed",
  unknown: "Unknown"
};

export const CATEGORY_ORDER: DomainCategory[] = ["real", "mixed", "measurement", "unknown"];

// Tone token for a category chip, reusing the bar palette.
export function categoryTone(category: DomainCategory): string {
  switch (category) {
    case "real":
      return "error";
    case "mixed":
      return "warning";
    case "measurement":
      return "notice";
    default:
      return "neutral";
  }
}

// A change the engine drove rather than the cohort.
export function isEngineChange(classification: ChangeClassification): boolean {
  return (
    classification === "new_in_engine" ||
    classification === "removed_from_engine" ||
    classification === "level_reclassified"
  );
}

export type TagSplit = {
  cohort: ReportTagEntry[];
  engine: ReportTagEntry[];
  unknown: ReportTagEntry[];
};

// Split one tag list so cohort changes lead and engine rows can collapse
// behind a disclosure.
export function splitTagsByClassification(entries: ReportTagEntry[] | undefined): TagSplit {
  const split: TagSplit = { cohort: [], engine: [], unknown: [] };
  for (const entry of entries ?? []) {
    if (entry.classification === "cohort_change") split.cohort.push(entry);
    else if (entry.classification === "unknown") split.unknown.push(entry);
    else split.engine.push(entry);
  }
  return split;
}

// Movers in the requested category, or all of them when none is selected.
export function filterDomainsByCategory(
  domains: ReportDomain[] | undefined,
  category: DomainCategory | ""
): ReportDomain[] {
  if (!category) return domains ?? [];
  return (domains ?? []).filter((d) => d.category === category);
}

export function signedNumber(value: number | null | undefined): string {
  if (value === null || value === undefined) return "";
  if (value > 0) return `+${value}`;
  return String(value);
}

// One-line description of the dimensions a cluster was detected over.
export function clusterDimensionSummary(cluster: ReportCluster): string {
  return cluster.dimensions
    .map((d) => `${d.dimension} ${d.label || d.value} (${cluster.size} of ${d.total_domains})`)
    .join(", ");
}

function mdCell(value: string | number | null | undefined): string {
  return String(value ?? "").replace(/\|/g, "\\|");
}

function mdTable(headers: string[], rows: (string | number | null | undefined)[][]): string[] {
  if (rows.length === 0) return [];
  return [
    `| ${headers.join(" | ")} |`,
    `| ${headers.map(() => "---").join(" | ")} |`,
    ...rows.map((row) => `| ${row.map(mdCell).join(" | ")} |`),
    ""
  ];
}

function scoringLine(state: "true" | "false" | "unknown"): string {
  if (state === "true") return "Scoring configuration changed between the two snapshots.";
  if (state === "false") return "Scoring configuration unchanged.";
  return "Scoring configuration provenance unknown; a score move cannot be fully attributed.";
}

function vocabularyLine(report: ReportResponse): string {
  const v = report.header.vocabulary;
  if (!v.from_available || !v.to_available) {
    return "Tag vocabulary unknown on at least one side; no change can be attributed to the engine.";
  }
  return (
    `Tag vocabulary ${v.from_tag_count} to ${v.to_tag_count} tags: ` +
    `${v.added.length} added, ${v.removed.length} removed, ${v.level_changed.length} reclassified.`
  );
}

function tagRows(entries: ReportTagEntry[], showFrom: boolean) {
  return entries.map((e) => [
    e.tag,
    showFrom ? (e.from_level ?? "") : (e.to_level ?? ""),
    signedNumber(e.domain_delta),
    CLASSIFICATION_LABELS[e.classification]
  ]);
}

// Render the whole report as Markdown, in the order the page shows it.
export function reportToMarkdown(report: ReportResponse): string {
  const { header, totals } = report;
  const lines: string[] = [];

  lines.push(`# ${report.dataset_tag}: ${report.from_slug} to ${report.to_slug}`, "");
  lines.push(
    `From ${header.from.slug} (engine ${header.from.engine_version || "unknown"}, ` +
      `${header.from.domain_count} domains) to ${header.to.slug} ` +
      `(engine ${header.to.engine_version || "unknown"}, ${header.to.domain_count} domains).`,
    ""
  );
  lines.push(`- ${vocabularyLine(report)}`);
  lines.push(`- ${scoringLine(header.scoring_config_changed)}`);
  if (header.tag_floor) {
    lines.push(`- Findings below ${header.tag_floor} are not covered by the per-domain lists.`);
  }
  lines.push("");

  lines.push("## Totals", "");
  lines.push(
    ...mdTable(
      ["Metric", "From", "To"],
      [
        ["Domains", totals.from_domain_count, totals.to_domain_count],
        ["Mean score", totals.from_mean_score ?? "", totals.to_mean_score ?? ""]
      ]
    )
  );
  lines.push(
    `${totals.both_domain_count} domains on both sides: ${totals.identical_score} scored identically, ` +
      `${totals.improved} improved, ${totals.regressed} regressed. ` +
      `${totals.added} added, ${totals.removed} removed.`,
    ""
  );
  const categories = totals.domain_categories ?? {};
  const categoryRows = CATEGORY_ORDER.filter((c) => categories[c]).map((c) => [
    CATEGORY_LABELS[c],
    categories[c]
  ]);
  if (categoryRows.length > 0) {
    lines.push("## Movers by cause", "");
    lines.push(...mdTable(["Cause", "Domains"], categoryRows));
  }

  if (report.clusters.length > 0) {
    lines.push("## Clusters", "");
    lines.push(
      ...mdTable(
        ["Dimensions", "Domains", "Score move", "Members"],
        report.clusters.map((c) => [
          clusterDimensionSummary(c),
          c.size,
          `${signedNumber(c.min_delta)} to ${signedNumber(c.max_delta)}`,
          c.domains.join(", ")
        ])
      )
    );
  }

  const sections: [string, ReportTagEntry[], boolean][] = [
    ["Tags appeared", report.tags.appeared, false],
    ["Tags cleared", report.tags.cleared, true],
    ["Tag severity changed", report.tags.level_changed, false]
  ];
  for (const [title, entries, showFrom] of sections) {
    if (entries.length === 0) continue;
    lines.push(`## ${title}`, "");
    lines.push(...mdTable(["Tag", "Level", "Domains", "Classification"], tagRows(entries, showFrom)));
  }

  if (report.domains.length > 0) {
    lines.push("## Movers", "");
    lines.push(
      ...mdTable(
        ["Domain", "Score", "Delta", "Explained", "Grade", "Cause"],
        report.domains.map((d) => [
          d.domain,
          `${d.from_score ?? ""} to ${d.to_score ?? ""}`,
          signedNumber(d.score_delta),
          signedNumber(d.explained_delta),
          d.grade_changed ? `${d.from_grade ?? ""} to ${d.to_grade ?? ""}` : (d.to_grade ?? ""),
          CATEGORY_LABELS[d.category]
        ])
      )
    );
  }

  return lines.join("\n").replace(/\n{3,}/g, "\n\n").trimEnd() + "\n";
}

// Filename for the Markdown export of one pair.
export function reportFilename(report: ReportResponse): string {
  return `${report.dataset_tag}-report-${report.from_slug}-${report.to_slug}.md`;
}
