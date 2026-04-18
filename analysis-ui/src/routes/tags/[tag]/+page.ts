import { getTagDetail, type TagDetail } from "$lib/api";

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

  try {
    const detail = await getTagDetail(tag, { dataset_tag: datasetTag }, fetch);
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
