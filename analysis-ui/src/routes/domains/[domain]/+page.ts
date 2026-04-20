import { getDomainDetail, type DomainDetail } from "$lib/api";

export type DomainDetailPageData = {
  domain: string;
  datasetTag: string | null;
  detail: DomainDetail | null;
  error: string | null;
};

export async function load({ parent, fetch, params }): Promise<DomainDetailPageData> {
  const layout = await parent();
  const datasetTag = layout.resolvedCohort ?? null;
  const domain = params.domain ?? "";

  if (!datasetTag || !domain) {
    return { domain, datasetTag, detail: null, error: null };
  }

  try {
    const detail = await getDomainDetail(domain, { dataset_tag: datasetTag }, fetch);
    return { domain, datasetTag, detail, error: null };
  } catch (error) {
    return {
      domain,
      datasetTag,
      detail: null,
      error: error instanceof Error ? error.message : String(error)
    };
  }
}
