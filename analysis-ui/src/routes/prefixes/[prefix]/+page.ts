import { getPrefixDetail, type PrefixDetail } from "$lib/api";

export type PrefixDetailPageData = {
  prefix: string;
  datasetTag: string | null;
  detail: PrefixDetail | null;
  error: string | null;
};

export async function load({ parent, fetch, params }): Promise<PrefixDetailPageData> {
  const layout = await parent();
  const datasetTag = layout.resolvedCohort ?? null;
  const prefix = params.prefix ?? "";

  if (!datasetTag || !prefix) {
    return { prefix, datasetTag, detail: null, error: null };
  }

  try {
    const detail = await getPrefixDetail(prefix, { dataset_tag: datasetTag }, fetch);
    return { prefix, datasetTag, detail, error: null };
  } catch (error) {
    return {
      prefix,
      datasetTag,
      detail: null,
      error: error instanceof Error ? error.message : String(error)
    };
  }
}
