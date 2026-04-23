import { getDiff, type DiffResponse } from "$lib/api";

export type DiffPageData = {
  datasetTag: string | null;
  fromSlug: string;
  toSlug: string;
  diff: DiffResponse | null;
  error: string | null;
};

export async function load({ parent, fetch, url }): Promise<DiffPageData> {
  const layout = await parent();
  const datasetTag = layout.resolvedCohort ?? null;
  const fromSlug = url.searchParams.get("from") ?? "";
  const toSlug = url.searchParams.get("to") ?? "";
  if (!datasetTag || !fromSlug || !toSlug) {
    return { datasetTag, fromSlug, toSlug, diff: null, error: null };
  }
  try {
    const diff = await getDiff(datasetTag, fromSlug, toSlug, fetch);
    return { datasetTag, fromSlug, toSlug, diff, error: null };
  } catch (error) {
    return {
      datasetTag,
      fromSlug,
      toSlug,
      diff: null,
      error: error instanceof Error ? error.message : String(error)
    };
  }
}
