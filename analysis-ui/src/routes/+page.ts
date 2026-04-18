import { getCohortDetail, listTags, type TagView } from "$lib/api";

export type OverviewPageData = {
  datasetTag: string | null;
  detail: CohortDetail | null;
  detailError: string | null;
  topTags: TagView[];
  topTagsError: string | null;
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
    return {
      datasetTag: null,
      detail: null,
      detailError: null,
      topTags: [],
      topTagsError: null
    };
  }
  const [detailResult, tagsResult] = await Promise.allSettled([
    getCohortDetail(datasetTag, fetch),
    listTags({ dataset_tag: datasetTag, limit: 10, min_level: "WARNING" }, fetch)
  ]);
  const errorMessage = (r: PromiseRejectedResult): string =>
    r.reason instanceof Error ? r.reason.message : String(r.reason);
  return {
    datasetTag,
    detail:
      detailResult.status === "fulfilled" ? (detailResult.value as CohortDetail) : null,
    detailError: detailResult.status === "rejected" ? errorMessage(detailResult) : null,
    topTags: tagsResult.status === "fulfilled" ? tagsResult.value.items : [],
    topTagsError: tagsResult.status === "rejected" ? errorMessage(tagsResult) : null
  };
}
