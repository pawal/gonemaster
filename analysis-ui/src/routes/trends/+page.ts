import { getTrends, type TrendKeyMeta, type TrendPoint, type TrendResponse } from "$lib/api";

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
  keyMeta: Record<string, TrendKeyMeta>;
  // Top-tags time series that drives the "Top movers" panel. Independent of
  // the selected category and non-fatal: empty when unavailable.
  topTagPoints: TrendPoint[];
  error: string | null;
};

export async function load({ parent, fetch, url }): Promise<TrendsPageData> {
  const layout = await parent();
  const datasetTag = layout.resolvedCohort ?? null;
  const rawCategory = url.searchParams.get("category") ?? "";
  const category: TrendCategoryKey =
    TREND_CATEGORIES.find((c) => c.key === rawCategory)?.key ?? "severity";
  if (!datasetTag) {
    return { datasetTag: null, category, points: [], keyMeta: {}, topTagPoints: [], error: null };
  }
  try {
    // The movers series is a separate category the server already serves; keep
    // it non-fatal so a movers failure never blanks the main chart.
    const [trend, movers] = await Promise.all([
      getTrends(datasetTag, { category }, fetch),
      getTrends(datasetTag, { category: "top_tags" }, fetch).catch(
        (): TrendResponse => ({ dataset_tag: datasetTag, category: "top_tags", points: [] })
      )
    ]);
    return {
      datasetTag,
      category,
      points: trend.points ?? [],
      keyMeta: trend.key_meta ?? {},
      topTagPoints: movers.points ?? [],
      error: null
    };
  } catch (error) {
    return {
      datasetTag,
      category,
      points: [],
      keyMeta: {},
      topTagPoints: [],
      error: error instanceof Error ? error.message : String(error)
    };
  }
}
