import { getDiff, getTagDiff, type DiffResponse, type TagDiffResponse } from "$lib/api";
import { previousSlug } from "$lib/diff";

export type DiffPageData = {
  datasetTag: string | null;
  fromSlug: string;
  toSlug: string;
  // True when fromSlug was auto-filled with the snapshot preceding `to`
  // because the URL carried only `to`.
  fromDefaulted: boolean;
  diff: DiffResponse | null;
  // Tag-level diff degrades independently: null when it could not be
  // fetched, so the domain diff still renders.
  tagDiff: TagDiffResponse | null;
  error: string | null;
};

export async function load({ parent, fetch, url }): Promise<DiffPageData> {
  const layout = await parent();
  const datasetTag = layout.resolvedCohort ?? null;
  const toSlug = url.searchParams.get("to") ?? "";
  const urlFrom = url.searchParams.get("from") ?? "";
  // When only `to` is chosen, default the other side to the snapshot right
  // before it so a single pick already produces a useful comparison.
  const fromDefaulted = !urlFrom && !!toSlug;
  const fromSlug = urlFrom || (fromDefaulted ? previousSlug(layout.snapshots ?? [], toSlug) : "");

  if (!datasetTag || !fromSlug || !toSlug) {
    return { datasetTag, fromSlug, toSlug, fromDefaulted, diff: null, tagDiff: null, error: null };
  }
  try {
    const [diff, tagDiff] = await Promise.all([
      getDiff(datasetTag, fromSlug, toSlug, fetch),
      getTagDiff(datasetTag, fromSlug, toSlug, fetch).catch(() => null)
    ]);
    return { datasetTag, fromSlug, toSlug, fromDefaulted, diff, tagDiff, error: null };
  } catch (error) {
    return {
      datasetTag,
      fromSlug,
      toSlug,
      fromDefaulted,
      diff: null,
      tagDiff: null,
      error: error instanceof Error ? error.message : String(error)
    };
  }
}
