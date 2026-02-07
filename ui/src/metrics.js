export const metricsWindowOptions = [
  { id: "1h", label: "Last 1h" },
  { id: "6h", label: "Last 6h" },
  { id: "24h", label: "Last 24h" },
  { id: "48h", label: "Last 48h" }
];

const metricsIncludeDefault = ["health", "jobs", "api", "quality", "insights", "trends"];

export const buildMetricsQuery = (options = {}) => {
  const params = new URLSearchParams();
  const window = String(options.window || "1h");
  params.set("window", window);

  const include = Array.isArray(options.include) && options.include.length
    ? options.include
    : metricsIncludeDefault;
  params.set("include", include.join(","));

  const limitDomains = Number(options.limitDomains);
  if (Number.isFinite(limitDomains) && limitDomains > 0) {
    params.set("limit_domains", String(limitDomains));
  }
  const limitBatches = Number(options.limitBatches);
  if (Number.isFinite(limitBatches) && limitBatches > 0) {
    params.set("limit_batches", String(limitBatches));
  }

  return params.toString();
};

export const fetchMetricsSnapshot = async (apiFetch, options = {}) => {
  if (typeof apiFetch !== "function") {
    throw new Error("apiFetch is required");
  }
  const query = buildMetricsQuery(options);
  return apiFetch(`/metrics?${query}`);
};
