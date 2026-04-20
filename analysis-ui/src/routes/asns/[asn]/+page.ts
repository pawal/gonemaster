import { getASNDetail, type ASNDetail } from "$lib/api";

export type ASNDetailPageData = {
  asn: string;
  datasetTag: string | null;
  detail: ASNDetail | null;
  error: string | null;
};

export async function load({ parent, fetch, params }): Promise<ASNDetailPageData> {
  const layout = await parent();
  const datasetTag = layout.resolvedCohort ?? null;
  const asn = params.asn ?? "";

  if (!datasetTag || !asn) {
    return { asn, datasetTag, detail: null, error: null };
  }

  try {
    const detail = await getASNDetail(asn, { dataset_tag: datasetTag }, fetch);
    return { asn, datasetTag, detail, error: null };
  } catch (error) {
    return {
      asn,
      datasetTag,
      detail: null,
      error: error instanceof Error ? error.message : String(error)
    };
  }
}
