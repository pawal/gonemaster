import { listPrefixes, type PrefixView, type ListResponse } from "$lib/api";
import { filterFromURL } from "$lib/filters";

const DEFAULT_LIMIT = 50;

export type PrefixesPageData = {
  datasetTag: string | null;
  list: ListResponse<PrefixView> | null;
  error: string | null;
  limit: number;
  offset: number;
};

export async function load({ parent, fetch, url }): Promise<PrefixesPageData> {
  const layout = await parent();
  const datasetTag = layout.resolvedCohort ?? null;

  const filter = filterFromURL(url);
  const limit = clampInt(url.searchParams.get("limit"), DEFAULT_LIMIT, 1, 500);
  const offset = clampInt(url.searchParams.get("offset"), 0, 0, Number.MAX_SAFE_INTEGER);
  const sort = url.searchParams.get("sort") ?? "";

  if (!datasetTag) {
    return { datasetTag: null, list: null, error: null, limit, offset };
  }

  const snapshot = layout.effectiveSnapshotSlug ?? undefined;

  try {
    const list = await listPrefixes(
      { ...filter, dataset_tag: datasetTag, snapshot, limit, offset, sort: sort || undefined },
      fetch
    );
    return { datasetTag, list, error: null, limit, offset };
  } catch (error) {
    return {
      datasetTag,
      list: null,
      error: error instanceof Error ? error.message : String(error),
      limit,
      offset
    };
  }
}

function clampInt(raw: string | null, fallback: number, min: number, max: number): number {
  if (raw == null || raw === "") return fallback;
  const parsed = Number.parseInt(raw, 10);
  if (!Number.isFinite(parsed)) return fallback;
  if (parsed < min) return min;
  if (parsed > max) return max;
  return parsed;
}
