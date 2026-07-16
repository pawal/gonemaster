import {
  getEntityHistory,
  getNameserverDetail,
  type AnalysisFilter,
  type HistoryPoint,
  type NameserverDetail
} from "$lib/api";

export type NameserverDetailPageData = {
  nameserver: string;
  datasetTag: string | null;
  detail: NameserverDetail | null;
  history: HistoryPoint[];
  error: string | null;
};

export async function load({ parent, fetch, params }): Promise<NameserverDetailPageData> {
  const layout = await parent();
  const datasetTag = layout.resolvedCohort ?? null;
  const nameserver = params.name ?? "";

  if (!datasetTag || !nameserver) {
    return { nameserver, datasetTag, detail: null, history: [], error: null };
  }

  const snapshot = layout.effectiveSnapshotSlug ?? "";

  try {
    const filter: AnalysisFilter = { dataset_tag: datasetTag };
    if (snapshot) filter.snapshot = snapshot;
    // History degrades independently of the detail body.
    const [detail, history] = await Promise.all([
      getNameserverDetail(nameserver, filter, fetch),
      getEntityHistory(datasetTag, "nameserver", nameserver, fetch).then((h) => h.points).catch(() => [])
    ]);
    return { nameserver, datasetTag, detail, history, error: null };
  } catch (error) {
    return {
      nameserver,
      datasetTag,
      detail: null,
      history: [],
      error: error instanceof Error ? error.message : String(error)
    };
  }
}
