export const formatPercent = (value) => `${(Number(value || 0) * 100).toFixed(1)}%`;

export const formatInteger = (value) => {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return "0";
  return Math.round(numeric).toLocaleString();
};

export const formatCompactInteger = (value) => {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return "0";
  const sign = numeric < 0 ? "-" : "";
  const absolute = Math.abs(numeric);
  if (absolute < 1000) return `${sign}${formatInteger(absolute)}`;

  const units = [
    { divisor: 1e12, suffix: "T" },
    { divisor: 1e9, suffix: "B" },
    { divisor: 1e6, suffix: "M" },
    { divisor: 1e3, suffix: "K" }
  ];
  for (const unit of units) {
    if (absolute < unit.divisor) continue;
    const scaled = absolute / unit.divisor;
    const rounded = Math.round(scaled * 10) / 10;
    const raw = Number.isInteger(rounded) ? rounded.toFixed(0) : rounded.toFixed(1);
    return `${sign}${raw.replace(".", ",")}${unit.suffix}`;
  }
  return `${sign}${formatInteger(absolute)}`;
};

export const formatDurationMs = (value) => `${formatInteger(value)} ms`;

export const formatRate = (value) => {
  const numeric = Number(value);
  if (!Number.isFinite(numeric) || numeric <= 0) return "0/s";
  const rounded = Math.round(numeric * 10) / 10;
  const raw = Number.isInteger(rounded) ? rounded.toFixed(0) : rounded.toFixed(1);
  return `${raw.replace(".", ",")}/s`;
};

export const formatUptime = (value) => {
  const seconds = Number(value);
  if (!Number.isFinite(seconds) || seconds < 0) return "unknown";
  const total = Math.floor(seconds);
  if (total < 60) return `${total}s`;
  if (total < 3600) {
    const minutes = Math.floor(total / 60);
    const rem = total % 60;
    return `${minutes}m ${rem}s`;
  }
  if (total < 86400) {
    const hours = Math.floor(total / 3600);
    const minutes = Math.floor((total % 3600) / 60);
    return `${hours}h ${minutes}m`;
  }
  const days = Math.floor(total / 86400);
  const hours = Math.floor((total % 86400) / 3600);
  return `${days}d ${hours}h`;
};

export const parseTimestamp = (value) => {
  if (!value) return null;
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return null;
  return parsed;
};

export const formatTimestampLocal = (value) => {
  const parsed = parseTimestamp(value);
  if (!parsed) return "unknown";
  return parsed.toLocaleString("sv-SE");
};

export const formatBatchTotalRuntime = (batch) => {
  const created = parseTimestamp(batch?.created_at);
  if (!created) return "unknown";
  const finished = parseTimestamp(batch?.finished_at);
  const end = finished || new Date();
  const elapsedSeconds = Math.max(0, Math.floor((end.getTime() - created.getTime()) / 1000));
  return `${formatUptime(elapsedSeconds)}${finished ? "" : " (running)"}`;
};

export const prettyProfileJSON = (value) => {
  if (!value) return "";
  if (typeof value !== "string") {
    return JSON.stringify(value, null, 2);
  }
  try {
    return JSON.stringify(JSON.parse(value), null, 2);
  } catch (_) {
    return value;
  }
};

// Mirrors server defaultSnapshotSlug shape; the batch-id hash is not known
// until the server accepts the batch.
export const formatSnapshotSlugPreview = () => {
  const today = new Date().toISOString().slice(0, 10);
  return `${today}-<batch-hash>`;
};
