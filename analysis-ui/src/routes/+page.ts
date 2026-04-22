import {
  getCohortDetail,
  listASNs,
  listNameservers,
  listTags,
  type ASNView,
  type NameserverView,
  type TagView
} from "$lib/api";

export type OverviewPageData = {
  datasetTag: string | null;
  detail: CohortDetail | null;
  detailError: string | null;
  topTags: TagView[];
  topTagsError: string | null;
  topNameservers: NameserverView[];
  topNameserversTotal: number;
  topNameserversError: string | null;
  topASNs: ASNView[];
  topASNsTotal: number;
  topASNsError: string | null;
};

export type FactBucket = {
  key: string;
  label: string;
  tone: string;
  count: number;
  order: number;
};

export type FactDistribution = {
  category: string;
  label: string;
  description?: string;
  order: number;
  buckets: FactBucket[];
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
  fact_distributions?: Record<string, FactDistribution>;
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
      topTagsError: null,
      topNameservers: [],
      topNameserversTotal: 0,
      topNameserversError: null,
      topASNs: [],
      topASNsTotal: 0,
      topASNsError: null
    };
  }
  const infraFilter = { dataset_tag: datasetTag, limit: 10, sort: "domain_count_desc" };
  const [detailResult, tagsResult, nsResult, asnResult] = await Promise.allSettled([
    getCohortDetail(datasetTag, fetch),
    listTags({ dataset_tag: datasetTag, limit: 10, min_level: "WARNING" }, fetch),
    listNameservers(infraFilter, fetch),
    listASNs(infraFilter, fetch)
  ]);
  const errorMessage = (r: PromiseRejectedResult): string =>
    r.reason instanceof Error ? r.reason.message : String(r.reason);
  return {
    datasetTag,
    detail:
      detailResult.status === "fulfilled" ? (detailResult.value as CohortDetail) : null,
    detailError: detailResult.status === "rejected" ? errorMessage(detailResult) : null,
    topTags: tagsResult.status === "fulfilled" ? tagsResult.value.items : [],
    topTagsError: tagsResult.status === "rejected" ? errorMessage(tagsResult) : null,
    topNameservers: nsResult.status === "fulfilled" ? nsResult.value.items : [],
    topNameserversTotal: nsResult.status === "fulfilled" ? nsResult.value.total : 0,
    topNameserversError: nsResult.status === "rejected" ? errorMessage(nsResult) : null,
    topASNs: asnResult.status === "fulfilled" ? asnResult.value.items : [],
    topASNsTotal: asnResult.status === "fulfilled" ? asnResult.value.total : 0,
    topASNsError: asnResult.status === "rejected" ? errorMessage(asnResult) : null
  };
}
