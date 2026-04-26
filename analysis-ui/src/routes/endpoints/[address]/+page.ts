import { getEndpointDetail, type EndpointDetail } from "$lib/api";

export type EndpointDetailPageData = {
  address: string;
  nameserver: string;
  datasetTag: string | null;
  detail: EndpointDetail | null;
  error: string | null;
};

export async function load({ parent, fetch, params, url }): Promise<EndpointDetailPageData> {
  const layout = await parent();
  const datasetTag = layout.resolvedCohort ?? null;
  const address = params.address ?? "";
  const nameserver = url.searchParams.get("nameserver") ?? "";

  if (!datasetTag || !address) {
    return { address, nameserver, datasetTag, detail: null, error: null };
  }

  const snapshot = layout.effectiveSnapshotSlug ?? "";

  try {
    const filter: Record<string, string> = { dataset_tag: datasetTag };
    if (snapshot) filter.snapshot = snapshot;
    if (nameserver) filter.nameserver = nameserver;
    const detail = await getEndpointDetail(address, filter, fetch);
    return { address, nameserver, datasetTag, detail, error: null };
  } catch (error) {
    return {
      address,
      nameserver,
      datasetTag,
      detail: null,
      error: error instanceof Error ? error.message : String(error)
    };
  }
}
