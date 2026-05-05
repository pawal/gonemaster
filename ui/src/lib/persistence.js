export const persistedStateKey = "gonemaster.ui.state.v1";

export const persistedQueryKeys = [
  "r_sort",
  "r_sev",
  "r_batch",
  "r_domain",
  "r_limit",
  "r_cursor",
  "b_id",
  "b_sort",
  "b_limit",
  "b_cursor",
  "b_status",
  "b_domain",
];

export const normalizePageSize = (value, allowed, defaultSize = 20) => {
  const parsed = Number(value);
  if (Number.isFinite(parsed) && allowed.includes(parsed)) return parsed;
  return defaultSize;
};

export const normalizeCursor = (value) => {
  const parsed = Number(value);
  if (Number.isFinite(parsed) && parsed >= 0) return Math.floor(parsed);
  return 0;
};

export const hasPersistedURLState = (params) =>
  persistedQueryKeys.some((key) => params.has(key));

// Decode persisted UI state from URL search params. The validators object
// supplies the per-field acceptors that depend on App-level config.
export const decodeStateFromURL = (params, validators) => {
  if (!hasPersistedURLState(params)) return null;
  const next = {};

  const recentSort = params.get("r_sort");
  if (recentSort && validators.isKnownJobSort(recentSort)) next.jobSort = recentSort;

  const recentSeverity = params.get("r_sev");
  if (recentSeverity && validators.isKnownSeverityFilter(recentSeverity)) {
    next.severityFilter = recentSeverity;
  }

  if (params.has("r_batch")) next.jobBatchFilter = (params.get("r_batch") || "").trim();
  if (params.has("r_domain")) next.recentDomainFilter = (params.get("r_domain") || "").trim();
  if (params.has("r_limit")) next.recentPageSize = validators.normalizeRecentPageSize(params.get("r_limit"));
  if (params.has("r_cursor")) next.recentCursor = normalizeCursor(params.get("r_cursor"));

  if (params.has("b_id")) next.selectedBatchId = (params.get("b_id") || "").trim();

  const batchSortValue = params.get("b_sort");
  if (batchSortValue && validators.isKnownBatchSort(batchSortValue)) next.batchSort = batchSortValue;

  if (params.has("b_limit")) next.batchPageSize = validators.normalizeBatchPageSize(params.get("b_limit"));
  if (params.has("b_cursor")) next.batchCursor = normalizeCursor(params.get("b_cursor"));

  const batchStatusValue = params.get("b_status");
  if (batchStatusValue !== null && validators.isKnownBatchStatus(batchStatusValue)) {
    next.batchStatusFilter = batchStatusValue;
  }
  if (params.has("b_domain")) next.batchDomainFilter = (params.get("b_domain") || "").trim();

  return next;
};

export const decodeStateFromStorage = (raw, validators) => {
  if (!raw) return null;
  let parsed;
  try {
    parsed = JSON.parse(raw);
  } catch (_) {
    return null;
  }
  if (!parsed || typeof parsed !== "object") return null;

  const next = {};

  if (typeof parsed.jobSort === "string" && validators.isKnownJobSort(parsed.jobSort)) {
    next.jobSort = parsed.jobSort;
  }
  if (typeof parsed.severityFilter === "string" && validators.isKnownSeverityFilter(parsed.severityFilter)) {
    next.severityFilter = parsed.severityFilter;
  }
  if (typeof parsed.jobBatchFilter === "string") next.jobBatchFilter = parsed.jobBatchFilter.trim();
  if (typeof parsed.recentDomainFilter === "string") next.recentDomainFilter = parsed.recentDomainFilter.trim();

  next.recentPageSize = validators.normalizeRecentPageSize(parsed.recentPageSize);
  next.recentCursor = normalizeCursor(parsed.recentCursor);

  if (typeof parsed.selectedBatchId === "string") next.selectedBatchId = parsed.selectedBatchId.trim();
  if (typeof parsed.batchSort === "string" && validators.isKnownBatchSort(parsed.batchSort)) {
    next.batchSort = parsed.batchSort;
  }
  next.batchPageSize = validators.normalizeBatchPageSize(parsed.batchPageSize);
  next.batchCursor = normalizeCursor(parsed.batchCursor);

  if (typeof parsed.batchStatusFilter === "string" && validators.isKnownBatchStatus(parsed.batchStatusFilter)) {
    next.batchStatusFilter = parsed.batchStatusFilter;
  }
  if (typeof parsed.batchDomainFilter === "string") next.batchDomainFilter = parsed.batchDomainFilter.trim();

  return next;
};

// Apply the live UI state to a URLSearchParams instance: clears any persisted
// keys and re-sets only those that differ from defaults.
export const encodeStateToURLParams = (params, state) => {
  persistedQueryKeys.forEach((key) => params.delete(key));

  if (state.jobSort !== "started_at_desc") params.set("r_sort", state.jobSort);
  if (state.severityFilter !== "all") params.set("r_sev", state.severityFilter);
  if (state.jobBatchFilter) params.set("r_batch", state.jobBatchFilter);
  if (state.recentDomainFilter) params.set("r_domain", state.recentDomainFilter);
  if (state.recentPageSize !== 20) params.set("r_limit", String(state.recentPageSize));
  if (state.recentCursor > 0) params.set("r_cursor", String(state.recentCursor));
  if (state.selectedBatchId) params.set("b_id", state.selectedBatchId);
  if (state.batchSort !== "started_at_desc") params.set("b_sort", state.batchSort);
  if (state.batchPageSize !== 20) params.set("b_limit", String(state.batchPageSize));
  if (state.batchCursor > 0) params.set("b_cursor", String(state.batchCursor));
  if (state.batchStatusFilter) params.set("b_status", state.batchStatusFilter);
  if (state.batchDomainFilter) params.set("b_domain", state.batchDomainFilter);

  return params;
};

export const serializeStateForStorage = (state) => JSON.stringify(state);
