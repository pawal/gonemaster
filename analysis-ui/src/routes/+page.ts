import { getCohortDetail } from "$lib/api";

export type OverviewPageData = {
  datasetTag: string | null;
  detail: CohortDetail | null;
  detailError: string | null;
};

export type CohortDetail = {
  dataset_tag: string;
  label: string;
  description?: string;
  materialization_status: string;
  last_materialized_at?: string;
  is_default?: boolean;
  domain_count?: number;
  nameserver_count?: number;
  endpoint_count?: number;
  asn_count?: number;
  prefix_count?: number;
  severity_distribution?: Record<string, number>;
};

export async function load({ parent, fetch }): Promise<OverviewPageData> {
  const layout = await parent();
  const datasetTag = layout.resolvedCohort ?? null;
  if (!datasetTag) {
    return { datasetTag: null, detail: null, detailError: null };
  }
  try {
    const detail = (await getCohortDetail(datasetTag, fetch)) as CohortDetail;
    return { datasetTag, detail, detailError: null };
  } catch (error) {
    return {
      datasetTag,
      detail: null,
      detailError: error instanceof Error ? error.message : String(error)
    };
  }
}
