import {
  getEntityHistory,
  getTagDetail,
  type AnalysisFilter,
  type HistoryPoint,
  type TagDetail
} from "$lib/api";

export type TagDetailPageData = {
  tag: string;
  datasetTag: string | null;
  detail: TagDetail | null;
  history: HistoryPoint[];
  error: string | null;
};

export async function load({ parent, fetch, params }): Promise<TagDetailPageData> {
  const layout = await parent();
  const datasetTag = layout.resolvedCohort ?? null;
  const tag = params.tag ?? "";

  if (!datasetTag || !tag) {
    return { tag, datasetTag, detail: null, history: [], error: null };
  }

  const snapshot = layout.effectiveSnapshotSlug ?? "";

  try {
    const filter: AnalysisFilter = { dataset_tag: datasetTag };
    if (snapshot) filter.snapshot = snapshot;
    const [detail, history] = await Promise.all([
      getTagDetail(tag, filter, fetch),
      getEntityHistory(datasetTag, "tag", tag, fetch).then((h) => h.points).catch(() => [])
    ]);
    return { tag, datasetTag, detail, history, error: null };
  } catch (error) {
    return {
      tag,
      datasetTag,
      detail: null,
      history: [],
      error: error instanceof Error ? error.message : String(error)
    };
  }
}
