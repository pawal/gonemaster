import {
  ApiError,
  getEntityHistory,
  getTagDetail,
  type AnalysisFilter,
  type HistoryPoint,
  type TagDetail
} from "$lib/api";

export type TagDetailPageData = {
  tag: string;
  datasetTag: string | null;
  snapshot: string;
  detail: TagDetail | null;
  history: HistoryPoint[];
  // True when the tag is absent from the selected snapshot (server 404),
  // rendered as a friendly note rather than a raw error banner.
  notInSnapshot: boolean;
  error: string | null;
};

export async function load({ parent, fetch, params }): Promise<TagDetailPageData> {
  const layout = await parent();
  const datasetTag = layout.resolvedCohort ?? null;
  const tag = params.tag ?? "";

  if (!datasetTag || !tag) {
    return { tag, datasetTag, snapshot: "", detail: null, history: [], notInSnapshot: false, error: null };
  }

  const snapshot = layout.effectiveSnapshotSlug ?? "";

  // Cohort-wide history has its own catch, so it survives a detail 404 and
  // still shows when the tag was present in other snapshots.
  const historyPromise = getEntityHistory(datasetTag, "tag", tag, fetch)
    .then((h) => h.points)
    .catch(() => []);

  try {
    const filter: AnalysisFilter = { dataset_tag: datasetTag };
    if (snapshot) filter.snapshot = snapshot;
    const detail = await getTagDetail(tag, filter, fetch);
    const history = await historyPromise;
    return { tag, datasetTag, snapshot, detail, history, notInSnapshot: false, error: null };
  } catch (error) {
    const history = await historyPromise;
    if (error instanceof ApiError && error.status === 404) {
      return { tag, datasetTag, snapshot, detail: null, history, notInSnapshot: true, error: null };
    }
    return {
      tag,
      datasetTag,
      snapshot,
      detail: null,
      history,
      notInSnapshot: false,
      error: error instanceof Error ? error.message : String(error)
    };
  }
}
