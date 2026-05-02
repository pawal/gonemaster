import { getTrends, type TrendPoint } from "$lib/api";

// Trend categories; keys match the fact-store category constants on the
// server (server/analysis_fact_categories.go).
export const TREND_CATEGORIES = [
  { key: "severity", label: "Domain health" },
  { key: "grade", label: "Grade distribution" },
  { key: "dnssec_posture", label: "DNSSEC posture" },
  { key: "dnskey_algo", label: "DNSKEY algorithms" }
] as const;

export type TrendCategoryKey = (typeof TREND_CATEGORIES)[number]["key"];

export type TrendsPageData = {
  datasetTag: string | null;
  category: TrendCategoryKey;
  points: TrendPoint[];
  error: string | null;
};

export async function load({ parent, fetch, url }): Promise<TrendsPageData> {
  const layout = await parent();
  const datasetTag = layout.resolvedCohort ?? null;
  const rawCategory = url.searchParams.get("category") ?? "";
  const category: TrendCategoryKey =
    TREND_CATEGORIES.find((c) => c.key === rawCategory)?.key ?? "severity";
  if (!datasetTag) {
    return { datasetTag: null, category, points: [], error: null };
  }
  try {
    const trend = await getTrends(datasetTag, { category }, fetch);
    return { datasetTag, category, points: trend.points ?? [], error: null };
  } catch (error) {
    return {
      datasetTag,
      category,
      points: [],
      error: error instanceof Error ? error.message : String(error)
    };
  }
}
