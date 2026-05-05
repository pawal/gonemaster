import { parseTimestamp } from "./format.js";
import { severityRank } from "./jobUtils.js";

const textCollator = new Intl.Collator(undefined, { numeric: true, sensitivity: "base" });

export const compareText = (left, right) => textCollator.compare(String(left ?? ""), String(right ?? ""));

export const compareNumber = (left, right) => Number(left || 0) - Number(right || 0);

export const timestampValue = (value) => parseTimestamp(value)?.getTime() ?? -1;

export const compareTimestamp = (left, right) => compareNumber(timestampValue(left), timestampValue(right));

export const compareSeverity = (left, right) => compareNumber(severityRank(left), severityRank(right));

export const nextTableSort = (state, key, defaultDirection = "asc") =>
  state.key === key
    ? { key, direction: state.direction === "asc" ? "desc" : "asc" }
    : { key, direction: defaultDirection };

export const tableSortIndicator = (state, key) => {
  if (state.key !== key) return "";
  return state.direction === "asc" ? "▲" : "▼";
};

export const tableSortAria = (state, key) => {
  if (state.key !== key) return "none";
  return state.direction === "asc" ? "ascending" : "descending";
};

export const sortItems = (items, state, comparators, tieBreaker = null) => {
  const list = Array.isArray(items) ? [...items] : [];
  const comparator = comparators[state?.key];
  if (!comparator) return list;
  list.sort((left, right) => {
    const primary = comparator(left, right);
    if (primary !== 0) {
      return state.direction === "asc" ? primary : -primary;
    }
    return tieBreaker ? tieBreaker(left, right) : 0;
  });
  return list;
};
