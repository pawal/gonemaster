import { getTagDetail, type AnalysisFilter, type TagDetail } from "$lib/api";

export type TagDetailPageData = {
  tag: string;
  datasetTag: string | null;
  detail: TagDetail | null;
  error: string | null;
};

export async function load({ parent, fetch, params }): Promise<TagDetailPageData> {
  const layout = await parent();
  const datasetTag = layout.resolvedCohort ?? null;
  const tag = params.tag ?? "";

  if (!datasetTag || !tag) {
    return { tag, datasetTag, detail: null, error: null };
  }

  const snapshot = layout.effectiveSnapshotSlug ?? "";

  try {
    const filter: AnalysisFilter = { dataset_tag: datasetTag };
    if (snapshot) filter.snapshot = snapshot;
    const detail = await getTagDetail(tag, filter, fetch);
    return { tag, datasetTag, detail, error: null };
  } catch (error) {
    return {
      tag,
      datasetTag,
      detail: null,
      error: error instanceof Error ? error.message : String(error)
    };
  }
}
