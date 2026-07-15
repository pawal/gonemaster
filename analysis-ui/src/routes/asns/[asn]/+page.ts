import {
  getASNDetail,
  getEntityHistory,
  type AnalysisFilter,
  type ASNDetail,
  type HistoryPoint
} from "$lib/api";

export type ASNDetailPageData = {
  asn: string;
  datasetTag: string | null;
  detail: ASNDetail | null;
  history: HistoryPoint[];
  error: string | null;
};

export async function load({ parent, fetch, params }): Promise<ASNDetailPageData> {
  const layout = await parent();
  const datasetTag = layout.resolvedCohort ?? null;
  const asn = params.asn ?? "";

  if (!datasetTag || !asn) {
    return { asn, datasetTag, detail: null, history: [], error: null };
  }

  const snapshot = layout.effectiveSnapshotSlug ?? "";

  try {
    const filter: AnalysisFilter = { dataset_tag: datasetTag };
    if (snapshot) filter.snapshot = snapshot;
    const [detail, history] = await Promise.all([
      getASNDetail(asn, filter, fetch),
      getEntityHistory(datasetTag, "asn", asn, fetch).then((h) => h.points).catch(() => [])
    ]);
    return { asn, datasetTag, detail, history, error: null };
  } catch (error) {
    return {
      asn,
      datasetTag,
      detail: null,
      history: [],
      error: error instanceof Error ? error.message : String(error)
    };
  }
}
