// Helpers that wire table-header buttons to the server's sort tokens.
// Each list page declares a SortColumnSpec per sortable column; the helpers
// turn a column spec plus the current URL sort token into (state, nextToken)
// used by SortHeader.svelte.

export type SortState = "off" | "asc" | "desc";

export type SortColumnSpec = {
  asc?: string;
  desc?: string;
};

export function currentSortState(currentSort: string, spec: SortColumnSpec): SortState {
  if (spec.asc && currentSort === spec.asc) return "asc";
  if (spec.desc && currentSort === spec.desc) return "desc";
  return "off";
}

// Cycle order: off → desc → asc → off. If one direction is unsupported the
// cycle skips it so every click lands on a supported state.
export function nextSortToken(state: SortState, spec: SortColumnSpec): string {
  switch (state) {
    case "off":
      if (spec.desc) return spec.desc;
      if (spec.asc) return spec.asc;
      return "";
    case "desc":
      if (spec.asc) return spec.asc;
      return "";
    case "asc":
      return "";
  }
}

export function sortIndicator(state: SortState): string {
  if (state === "asc") return "↑";
  if (state === "desc") return "↓";
  return "↕";
}
