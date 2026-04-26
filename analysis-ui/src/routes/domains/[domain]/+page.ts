import { getDomainDetail, type AnalysisFilter, type DomainDetail } from "$lib/api";

export type DomainDetailPageData = {
  domain: string;
  datasetTag: string | null;
  detail: DomainDetail | null;
  error: string | null;
};

export async function load({ parent, fetch, params, url }): Promise<DomainDetailPageData> {
  const layout = await parent();
  const datasetTag = layout.resolvedCohort ?? null;
  const domain = params.domain ?? "";
  const snapshot = url.searchParams.get("snapshot") ?? "";

  if (!datasetTag || !domain) {
    return { domain, datasetTag, detail: null, error: null };
  }

  try {
    const filter: AnalysisFilter = { dataset_tag: datasetTag };
    if (snapshot) filter.snapshot = snapshot;
    const detail = await getDomainDetail(domain, filter, fetch);
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
