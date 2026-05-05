export const activeJobStatuses = ["queued", "running"];
export const resultReadyStatuses = ["succeeded", "failed", "canceled"];

export const LEVEL_ORDER = ["DEBUG", "INFO", "NOTICE", "WARNING", "ERROR", "CRITICAL"];

export const normalizeStatus = (value) => String(value || "").toLowerCase();

export const isActiveJobStatus = (status) => activeJobStatuses.includes(normalizeStatus(status));

export const isResultReadyStatus = (status) => resultReadyStatuses.includes(normalizeStatus(status));

export const progressPercent = (job) => {
  const value = Number(job?.progress);
  if (!Number.isFinite(value)) return 0;
  return Math.max(0, Math.min(100, value));
};

export const hasActiveBatchJobs = (batch) =>
  activeJobStatuses.some((status) => Number(batch?.status_counts?.[status] || 0) > 0);

export const hasRunningOrQueuedJobs = (items = []) => items.some((job) => isActiveJobStatus(job?.status));

export const normalizeLevel = (value) => (value || "INFO").toUpperCase();

export const severityRank = (value) => {
  if (!value) return -1;
  const index = LEVEL_ORDER.indexOf(normalizeLevel(value));
  return index >= 0 ? index : -1;
};
