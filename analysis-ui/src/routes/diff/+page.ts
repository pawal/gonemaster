import { getDiff, type DiffResponse } from "$lib/api";
import { previousSlug } from "$lib/diff";

export type DiffPageData = {
  datasetTag: string | null;
  fromSlug: string;
  toSlug: string;
  // True when fromSlug was auto-filled with the snapshot preceding `to`
  // because the URL carried only `to`.
  fromDefaulted: boolean;
  diff: DiffResponse | null;
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
    return { datasetTag, fromSlug, toSlug, fromDefaulted, diff: null, error: null };
  }
  try {
    const diff = await getDiff(datasetTag, fromSlug, toSlug, fetch);
    return { datasetTag, fromSlug, toSlug, fromDefaulted, diff, error: null };
  } catch (error) {
    return {
      datasetTag,
      fromSlug,
      toSlug,
      fromDefaulted,
      diff: null,
      error: error instanceof Error ? error.message : String(error)
    };
  }
}
