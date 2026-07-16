import {
  getDomainDetail,
  getEntityHistory,
  type AnalysisFilter,
  type DomainDetail,
  type HistoryPoint
} from "$lib/api";

export type DomainDetailPageData = {
  domain: string;
  datasetTag: string | null;
  detail: DomainDetail | null;
  history: HistoryPoint[];
  error: string | null;
};

export async function load({ parent, fetch, params }): Promise<DomainDetailPageData> {
  const layout = await parent();
  const datasetTag = layout.resolvedCohort ?? null;
  const domain = params.domain ?? "";

  if (!datasetTag || !domain) {
    return { domain, datasetTag, detail: null, history: [], error: null };
  }

  const snapshot = layout.effectiveSnapshotSlug ?? "";

  try {
    const filter: AnalysisFilter = { dataset_tag: datasetTag };
    if (snapshot) filter.snapshot = snapshot;
    const [detail, history] = await Promise.all([
      getDomainDetail(domain, filter, fetch),
      getEntityHistory(datasetTag, "domain", domain, fetch).then((h) => h.points).catch(() => [])
    ]);
    return { domain, datasetTag, detail, history, error: null };
  } catch (error) {
    return {
      domain,
      datasetTag,
      detail: null,
      history: [],
      error: error instanceof Error ? error.message : String(error)
    };
  }
}
