import { getNameserverDetail, type NameserverDetail } from "$lib/api";

export type NameserverDetailPageData = {
  nameserver: string;
  datasetTag: string | null;
  detail: NameserverDetail | null;
  error: string | null;
};

export async function load({ parent, fetch, params }): Promise<NameserverDetailPageData> {
  const layout = await parent();
  const datasetTag = layout.resolvedCohort ?? null;
  const nameserver = params.name ?? "";

  if (!datasetTag || !nameserver) {
    return { nameserver, datasetTag, detail: null, error: null };
  }

  try {
    const detail = await getNameserverDetail(nameserver, { dataset_tag: datasetTag }, fetch);
    return { nameserver, datasetTag, detail, error: null };
  } catch (error) {
    return {
      nameserver,
      datasetTag,
      detail: null,
      error: error instanceof Error ? error.message : String(error)
    };
  }
}
