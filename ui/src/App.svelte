<script>
  import { onMount } from "svelte";
  import { fetchMetricsSnapshot, metricsWindowOptions } from "./metrics.js";

  let statusMessage = "";
  let statusTone = "";
  let statusDismissTimer = null;

  let singleDomain = "";
  let singleSubmitting = false;
  let createdJobId = "";
  let singleIPMode = "default";
  let undelegatedNameservers = [];
  let undelegatedDSInfo = [];

  let batchDomains = "";
  let batchSubmitting = false;
  let createdBatchId = "";

  let jobs = [];
  let jobsLoading = false;
  let filteredJobs = [];
  let autoRefreshRecent = false;
  let recentPoller = null;
  let severityFilter = "all";
  let jobSort = "started_at_desc";
  let jobBatchFilter = "";
  let recentDomainFilter = "";
  let recentPageSize = 20;
  let recentCursor = 0;
  let recentTotal = 0;
  let recentOffset = 0;
  let recentNextCursor = "";
  let recentPrevCursor = "";

  let selectedJobId = "";
  let selectedJob = null;
  let selectedJobResult = null;
  let resultLocale = "en";
  let jobLoading = false;
  let autoRefreshJob = true;
  let jobPoller = null;
  let jobInspectorHighlight = false;
  let jobInspectorHighlightTimer = null;

  let selectedBatchId = "";
  let selectedBatch = null;
  let batchLoading = false;
  let autoRefreshBatch = false;
  let batchPoller = null;
  let batchSort = "started_at_desc";
  let batchPageSize = 20;
  let batchCursor = 0;
  let batchStatusFilter = "";
  let batchDomainFilter = "";
  let recentBatchOptions = [];
  let recentBatchLoading = false;
  let selectedRecentBatch = "";
  let metricsSnapshot = null;
  let metricsLoading = false;
  let metricsError = "";
  let autoRefreshMetrics = true;
  let metricsPoller = null;
  let metricsWindow = "1h";
  let metricsDomainLimit = 10;
  let metricsBatchLimit = 10;
  let metricsLoadedAt = "";
  let persistenceReady = false;
  let persistenceSignature = "";
  let initialized = false;
  let undelegatedRowCounter = 0;

  // Theme management: "system" follows OS preference via CSS media query;
  // "light" and "dark" set data-theme on <html> explicitly.
  const themeKey = "gonemaster.ui.theme.v1";
  let theme = "system";

  const applyTheme = (t) => {
    if (typeof document === "undefined") return;
    const root = document.documentElement;
    if (t === "light" || t === "dark") {
      root.setAttribute("data-theme", t);
    } else {
      root.removeAttribute("data-theme");
    }
  };

  const cycleTheme = () => {
    const order = ["system", "light", "dark"];
    theme = order[(order.indexOf(theme) + 1) % order.length];
    localStorage.setItem(themeKey, theme);
    applyTheme(theme);
  };

  $: themeIcon = theme === "light" ? "☀" : theme === "dark" ? "☾" : "⊙";
  $: themeTitle =
    "Color theme: " + (theme === "light" ? "Light" : theme === "dark" ? "Dark" : "System") + " — click to cycle";

  // Locale management: fetch available locales from the server, persist choice
  // in localStorage, and auto-detect from the browser language on first visit.
  const localeKey = "gonemaster.ui.locale.v1";
  const localeDisplayNames = {
    da: "Dansk",
    en: "English",
    es: "Español",
    fi: "Suomi",
    fr: "Français",
    ja: "日本語",
    nb: "Norsk bokmål",
    sl: "Slovenščina",
    sv: "Svenska"
  };
  let availableLocales = ["en"];
  const localeLabel = (code) => localeDisplayNames[code] || code;

  const apiPrefix = "/api/v1";
  const persistedStateKey = "gonemaster.ui.state.v1";
  const persistedQueryKeys = [
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
    "b_domain"
  ];

  let moduleGroups = [];
  let moduleOpen = {};
  let lastResultJobId = "";
  let activeTab = "single";
  const tabs = [
    { id: "single", label: "Single Job" },
    { id: "recent", label: "Recent Tests" },
    { id: "batches", label: "Batch Jobs" },
    { id: "metrics", label: "Metrics" }
  ];

  const clearStatus = () => {
    statusMessage = "";
    statusTone = "";
    if (statusDismissTimer) {
      clearTimeout(statusDismissTimer);
      statusDismissTimer = null;
    }
  };

  const setStatus = (message, tone = "") => {
    statusMessage = message;
    statusTone = tone;
    if (statusDismissTimer) {
      clearTimeout(statusDismissTimer);
      statusDismissTimer = null;
    }
    if (!message) return;
    const dismissDelayMs = tone === "ok" ? 5000 : 8000;
    statusDismissTimer = setTimeout(() => {
      statusMessage = "";
      statusTone = "";
      statusDismissTimer = null;
    }, dismissDelayMs);
  };

  const apiFetch = async (path, options = {}) => {
    const url = path?.startsWith("/") ? `${apiPrefix}${path}` : `${apiPrefix}/${path}`;
    const headers = { ...(options.headers || {}) };
    if (options.body && !headers["Content-Type"]) {
      headers["Content-Type"] = "application/json";
    }
    const response = await fetch(url, { ...options, headers });
    const contentType = response.headers.get("content-type") || "";
    const payload = contentType.includes("application/json")
      ? await response.json()
      : await response.text();
    if (!response.ok) {
      const message = payload?.error?.message || payload?.message || response.statusText;
      throw new Error(message);
    }
    return payload;
  };

  const summaryLevels = ["NOTICE", "WARNING", "ERROR", "CRITICAL"];
  const severityFilters = [
    { id: "all", label: "All severities" },
    { id: "warnings_plus", label: "Warnings+" },
    { id: "errors_only", label: "Errors only" }
  ];
  const jobSortOptions = [
    { id: "started_at_desc", label: "Start time (newest)" },
    { id: "started_at_asc", label: "Start time (oldest)" },
    { id: "batch_id_asc", label: "Batch ID (A-Z)" },
    { id: "batch_id_desc", label: "Batch ID (Z-A)" },
    { id: "error_desc", label: "Errors + critical (high-low)" },
    { id: "critical_desc", label: "Critical (high-low)" },
    { id: "domain_asc", label: "Domain (A-Z)" },
    { id: "domain_desc", label: "Domain (Z-A)" }
  ];
  const batchSortOptions = [
    { id: "started_at_desc", label: "Start time (newest)" },
    { id: "started_at_asc", label: "Start time (oldest)" },
    { id: "error_desc", label: "Errors + critical (high-low)" },
    { id: "critical_desc", label: "Critical (high-low)" },
    { id: "domain_asc", label: "Domain (A-Z)" },
    { id: "domain_desc", label: "Domain (Z-A)" },
    { id: "created_at_desc", label: "Created (newest)" },
    { id: "created_at_asc", label: "Created (oldest)" }
  ];
  const batchStatuses = ["", "queued", "running", "succeeded", "failed", "canceled", "expired", "paused"];
  const listPageSizes = [10, 20, 50, 100];
  const batchPageSizes = listPageSizes;
  const recentPageSizes = listPageSizes;
  const metricsLimitOptions = [5, 10, 20, 50, 100];
  const activeJobStatuses = ["queued", "running"];
  const resultReadyStatuses = ["succeeded", "failed", "canceled"];

  const isKnownSort = (value, options) => options.some((option) => option.id === value);
  const isKnownSeverityFilter = (value) => severityFilters.some((option) => option.id === value);
  const isKnownBatchStatus = (value) => batchStatuses.includes(value);
  const normalizeStatus = (value) => String(value || "").toLowerCase();
  const isActiveJobStatus = (status) => activeJobStatuses.includes(normalizeStatus(status));
  const isResultReadyStatus = (status) => resultReadyStatuses.includes(normalizeStatus(status));
  const progressPercent = (job) => {
    const value = Number(job?.progress);
    if (!Number.isFinite(value)) return 0;
    return Math.max(0, Math.min(100, value));
  };
  const hasActiveBatchJobs = (batch) =>
    activeJobStatuses.some((status) => Number(batch?.status_counts?.[status] || 0) > 0);
  const normalizePageSize = (value) => {
    const parsed = Number(value);
    if (Number.isFinite(parsed) && listPageSizes.includes(parsed)) {
      return parsed;
    }
    return 20;
  };
  const normalizeBatchPageSize = (value) => normalizePageSize(value);
  const normalizeRecentPageSize = (value) => normalizePageSize(value);
  const normalizeMetricsLimit = (value) => {
    const parsed = Number(value);
    if (Number.isFinite(parsed) && metricsLimitOptions.includes(parsed)) {
      return parsed;
    }
    return 10;
  };
  const normalizeCursor = (value) => {
    const parsed = Number(value);
    if (Number.isFinite(parsed) && parsed >= 0) {
      return Math.floor(parsed);
    }
    return 0;
  };
  const syncSelectedRecentBatch = () => {
    const normalized = selectedBatchId.trim();
    if (!normalized) {
      selectedRecentBatch = "";
      return;
    }
    selectedRecentBatch = recentBatchOptions.some((option) => option.id === normalized) ? normalized : "";
  };
  const formatRecentBatchOption = (option) => {
    if (!option || !option.id) return "";
    if (!option.createdAt) return option.id;
    const parsed = new Date(option.createdAt);
    if (Number.isNaN(parsed.getTime())) return option.id;
    return `${option.id} - ${parsed.toLocaleString()}`;
  };

  const hasPersistedURLState = (params) => persistedQueryKeys.some((key) => params.has(key));

  const readStateFromURL = () => {
    const params = new URLSearchParams(window.location.search);
    if (!hasPersistedURLState(params)) return null;

    const next = {};
    const recentSort = params.get("r_sort");
    if (recentSort && isKnownSort(recentSort, jobSortOptions)) {
      next.jobSort = recentSort;
    }
    const recentSeverity = params.get("r_sev");
    if (recentSeverity && isKnownSeverityFilter(recentSeverity)) {
      next.severityFilter = recentSeverity;
    }
    if (params.has("r_batch")) {
      next.jobBatchFilter = (params.get("r_batch") || "").trim();
    }
    if (params.has("r_domain")) {
      next.recentDomainFilter = (params.get("r_domain") || "").trim();
    }
    if (params.has("r_limit")) {
      next.recentPageSize = normalizeRecentPageSize(params.get("r_limit"));
    }
    if (params.has("r_cursor")) {
      next.recentCursor = normalizeCursor(params.get("r_cursor"));
    }
    if (params.has("b_id")) {
      next.selectedBatchId = (params.get("b_id") || "").trim();
    }
    const batchSortValue = params.get("b_sort");
    if (batchSortValue && isKnownSort(batchSortValue, batchSortOptions)) {
      next.batchSort = batchSortValue;
    }
    if (params.has("b_limit")) {
      next.batchPageSize = normalizeBatchPageSize(params.get("b_limit"));
    }
    if (params.has("b_cursor")) {
      next.batchCursor = normalizeCursor(params.get("b_cursor"));
    }
    const batchStatusValue = params.get("b_status");
    if (batchStatusValue !== null && isKnownBatchStatus(batchStatusValue)) {
      next.batchStatusFilter = batchStatusValue;
    }
    if (params.has("b_domain")) {
      next.batchDomainFilter = (params.get("b_domain") || "").trim();
    }
    return next;
  };

  const readStateFromStorage = () => {
    try {
      const storage = typeof window === "undefined" ? null : window.localStorage;
      if (!storage) return null;
      const raw = storage.getItem(persistedStateKey);
      if (!raw) return null;
      const parsed = JSON.parse(raw);
      if (!parsed || typeof parsed !== "object") return null;

      const next = {};
      if (typeof parsed.jobSort === "string" && isKnownSort(parsed.jobSort, jobSortOptions)) {
        next.jobSort = parsed.jobSort;
      }
      if (typeof parsed.severityFilter === "string" && isKnownSeverityFilter(parsed.severityFilter)) {
        next.severityFilter = parsed.severityFilter;
      }
      if (typeof parsed.jobBatchFilter === "string") {
        next.jobBatchFilter = parsed.jobBatchFilter.trim();
      }
      if (typeof parsed.recentDomainFilter === "string") {
        next.recentDomainFilter = parsed.recentDomainFilter.trim();
      }
      next.recentPageSize = normalizeRecentPageSize(parsed.recentPageSize);
      next.recentCursor = normalizeCursor(parsed.recentCursor);
      if (typeof parsed.selectedBatchId === "string") {
        next.selectedBatchId = parsed.selectedBatchId.trim();
      }
      if (typeof parsed.batchSort === "string" && isKnownSort(parsed.batchSort, batchSortOptions)) {
        next.batchSort = parsed.batchSort;
      }
      next.batchPageSize = normalizeBatchPageSize(parsed.batchPageSize);
      next.batchCursor = normalizeCursor(parsed.batchCursor);
      if (typeof parsed.batchStatusFilter === "string" && isKnownBatchStatus(parsed.batchStatusFilter)) {
        next.batchStatusFilter = parsed.batchStatusFilter;
      }
      if (typeof parsed.batchDomainFilter === "string") {
        next.batchDomainFilter = parsed.batchDomainFilter.trim();
      }
      return next;
    } catch (error) {
      return null;
    }
  };

  const applyPersistedState = (state) => {
    if (!state) return;
    if (state.jobSort) jobSort = state.jobSort;
    if (state.severityFilter) severityFilter = state.severityFilter;
    if (typeof state.jobBatchFilter === "string") jobBatchFilter = state.jobBatchFilter;
    if (typeof state.recentDomainFilter === "string") recentDomainFilter = state.recentDomainFilter;
    if (state.recentPageSize !== undefined) recentPageSize = normalizeRecentPageSize(state.recentPageSize);
    if (state.recentCursor !== undefined) recentCursor = normalizeCursor(state.recentCursor);
    if (typeof state.selectedBatchId === "string") selectedBatchId = state.selectedBatchId;
    if (state.batchSort) batchSort = state.batchSort;
    if (state.batchPageSize !== undefined) batchPageSize = normalizeBatchPageSize(state.batchPageSize);
    if (state.batchCursor !== undefined) batchCursor = normalizeCursor(state.batchCursor);
    if (state.batchStatusFilter !== undefined && isKnownBatchStatus(state.batchStatusFilter)) {
      batchStatusFilter = state.batchStatusFilter;
    }
    if (typeof state.batchDomainFilter === "string") batchDomainFilter = state.batchDomainFilter;
  };

  const persistState = () => {
    const params = new URLSearchParams(window.location.search);
    persistedQueryKeys.forEach((key) => params.delete(key));
    const normalizedRecentPage = normalizeRecentPageSize(recentPageSize);
    const normalizedRecentCursor = normalizeCursor(recentCursor);
    const normalizedBatchPage = normalizeBatchPageSize(batchPageSize);
    const normalizedBatchCursor = normalizeCursor(batchCursor);

    if (jobSort !== "started_at_desc") {
      params.set("r_sort", jobSort);
    }
    if (severityFilter !== "all") {
      params.set("r_sev", severityFilter);
    }
    const normalizedJobBatch = jobBatchFilter.trim();
    if (normalizedJobBatch) {
      params.set("r_batch", normalizedJobBatch);
    }
    const normalizedRecentDomain = recentDomainFilter.trim();
    if (normalizedRecentDomain) {
      params.set("r_domain", normalizedRecentDomain);
    }
    if (normalizedRecentPage !== 20) {
      params.set("r_limit", String(normalizedRecentPage));
    }
    if (normalizedRecentCursor > 0) {
      params.set("r_cursor", String(normalizedRecentCursor));
    }
    const normalizedBatchID = selectedBatchId.trim();
    if (normalizedBatchID) {
      params.set("b_id", normalizedBatchID);
    }
    if (batchSort !== "started_at_desc") {
      params.set("b_sort", batchSort);
    }
    if (normalizedBatchPage !== 20) {
      params.set("b_limit", String(normalizedBatchPage));
    }
    if (normalizedBatchCursor > 0) {
      params.set("b_cursor", String(normalizedBatchCursor));
    }
    if (batchStatusFilter) {
      params.set("b_status", batchStatusFilter);
    }
    const normalizedBatchDomain = batchDomainFilter.trim();
    if (normalizedBatchDomain) {
      params.set("b_domain", normalizedBatchDomain);
    }

    const hash = window.location.hash || `#/${activeTab}`;
    const search = params.toString();
    window.history.replaceState(null, "", `${window.location.pathname}${search ? `?${search}` : ""}${hash}`);

    try {
      const storage = typeof window === "undefined" ? null : window.localStorage;
      if (!storage) return;
      const persisted = {
        jobSort,
        severityFilter,
        jobBatchFilter: normalizedJobBatch,
        recentDomainFilter: normalizedRecentDomain,
        recentPageSize: normalizedRecentPage,
        recentCursor: normalizedRecentCursor,
        selectedBatchId: normalizedBatchID,
        batchSort,
        batchPageSize: normalizedBatchPage,
        batchCursor: normalizedBatchCursor,
        batchStatusFilter,
        batchDomainFilter: normalizedBatchDomain
      };
      storage.setItem(persistedStateKey, JSON.stringify(persisted));
    } catch (error) {
      // Ignore storage issues in restricted browser contexts.
    }
  };
  const summaryRows = (summary) => {
    const levels = summary?.levels || {};
    return summaryLevels
      .map((level) => ({
        level,
        count: Number(levels[level] || 0)
      }))
      .filter((entry) => entry.count > 0);
  };
  const jobSeverityRows = (job) =>
    summaryLevels
      .map((level) => ({
        level,
        count: Number(job?.severity_totals?.[level] || 0)
      }))
      .filter((entry) => entry.count > 0);
  const jobSeverityTotal = (job, level) => Number(job?.severity_totals?.[level] || 0);
  const hasRunningOrQueuedJobs = (items = []) => items.some((job) => isActiveJobStatus(job?.status));
  const formatPercent = (value) => `${(Number(value || 0) * 100).toFixed(1)}%`;
  const formatInteger = (value) => {
    const numeric = Number(value);
    if (!Number.isFinite(numeric)) return "0";
    return Math.round(numeric).toLocaleString();
  };
  const formatCompactInteger = (value) => {
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
  const formatDurationMs = (value) => `${formatInteger(value)} ms`;
  const formatRate = (value) => {
    const numeric = Number(value);
    if (!Number.isFinite(numeric) || numeric <= 0) return "0/s";
    const rounded = Math.round(numeric * 10) / 10;
    const raw = Number.isInteger(rounded) ? rounded.toFixed(0) : rounded.toFixed(1);
    return `${raw.replace(".", ",")}/s`;
  };
  const formatUptime = (value) => {
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
  const parseTimestamp = (value) => {
    if (!value) return null;
    const parsed = new Date(value);
    if (Number.isNaN(parsed.getTime())) return null;
    return parsed;
  };
  const formatTimestampLocal = (value) => {
    const parsed = parseTimestamp(value);
    if (!parsed) return "unknown";
    return parsed.toLocaleString();
  };
  const formatBatchTotalRuntime = (batch) => {
    const created = parseTimestamp(batch?.created_at);
    if (!created) return "unknown";
    const finished = parseTimestamp(batch?.finished_at);
    const end = finished || new Date();
    const elapsedSeconds = Math.max(0, Math.floor((end.getTime() - created.getTime()) / 1000));
    return `${formatUptime(elapsedSeconds)}${finished ? "" : " (running)"}`;
  };
  const formatJobTotalRuntime = (job) => {
    const started = parseTimestamp(job?.started_at);
    if (!started) return "not started";
    const finished = parseTimestamp(job?.finished_at);
    const end = finished || new Date();
    const elapsedSeconds = Math.max(0, Math.floor((end.getTime() - started.getTime()) / 1000));
    return `${formatUptime(elapsedSeconds)}${!finished && isActiveJobStatus(job?.status) ? " (running)" : ""}`;
  };
  const formatBatchStatusCounts = (statusCounts) => {
    if (!statusCounts || typeof statusCounts !== "object") return "none";
    const knownOrder = ["queued", "running", "succeeded", "failed", "canceled", "expired", "paused"];
    const counts = new Map();
    for (const [status, rawCount] of Object.entries(statusCounts)) {
      const normalized = normalizeStatus(status);
      if (!normalized) continue;
      const numeric = Number(rawCount);
      counts.set(normalized, Number.isFinite(numeric) ? numeric : 0);
    }
    if (counts.size === 0) return "none";

    const orderedStatuses = [
      ...knownOrder.filter((status) => counts.has(status)),
      ...Array.from(counts.keys())
        .filter((status) => !knownOrder.includes(status))
        .sort()
    ];
    const orderedEntries = orderedStatuses.map((status) => [status, Number(counts.get(status) || 0)]);
    const nonZeroEntries = orderedEntries.filter(([, count]) => count > 0);
    const displayEntries = nonZeroEntries.length > 0 ? nonZeroEntries : orderedEntries;
    return displayEntries.map(([status, count]) => `${status} ${formatInteger(count)}`).join(" · ");
  };
  const metricsCardHelp = {
    queue_depth: "Current number of jobs waiting in the queue.",
    in_flight_jobs: "Jobs currently being processed by workers.",
    dns_queries_ipv4_total: "Total DNS queries sent over IPv4 across all processed jobs.",
    dns_queries_ipv6_total: "Total DNS queries sent over IPv6 across all processed jobs.",
    success_rate: "Share of completed jobs that succeeded.",
    failed_rate: "Share of completed jobs that failed.",
    api_p90: "Worst route-level 90th percentile API latency.",
    avg_job_duration: "Average runtime of completed jobs.",
    completed_total: "Total number of jobs that reached a terminal state.",
    failed_total: "Total number of completed jobs with failed status."
  };
  const severityCardHelp = (level) =>
    `Total ${String(level || "").toUpperCase()} log entries aggregated across completed jobs.`;
  const lastLoadedLabel = (value) => {
    if (!value) return "never";
    const parsed = new Date(value);
    if (Number.isNaN(parsed.getTime())) return "never";
    return parsed.toLocaleTimeString();
  };
  const metricsSeriesPoints = (snapshot, window) => {
    const windows = snapshot?.trends?.windows || {};
    if (windows[window]?.points) {
      return windows[window].points;
    }
    const firstKey = Object.keys(windows)[0];
    return firstKey ? windows[firstKey].points || [] : [];
  };
  const metricsSeriesValues = (snapshot, window, field) =>
    metricsSeriesPoints(snapshot, window).map((point) => Number(point?.[field] || 0));
  const sparklineBounds = (...seriesList) => {
    const flattened = seriesList.flatMap((series) =>
      Array.isArray(series)
        ? series
            .map((value) => Number(value))
            .filter((value) => Number.isFinite(value))
        : []
    );
    if (flattened.length === 0) {
      return { min: 0, max: 1 };
    }
    const min = Math.min(...flattened);
    const max = Math.max(...flattened);
    return { min, max: max === min ? min + 1 : max };
  };
  const sparklinePoints = (values, width = 260, height = 66, padding = 6, bounds = null) => {
    if (!Array.isArray(values) || values.length === 0) return "";
    const usableValues = values.map((value) => {
      const numeric = Number(value);
      return Number.isFinite(numeric) ? numeric : 0;
    });
    const max = Number.isFinite(bounds?.max) ? Number(bounds.max) : Math.max(...usableValues);
    const min = Number.isFinite(bounds?.min) ? Number(bounds.min) : Math.min(...usableValues);
    const spread = max - min || 1;
    const spanX = Math.max(width - padding * 2, 1);
    const spanY = Math.max(height - padding * 2, 1);
    const denominator = usableValues.length > 1 ? usableValues.length - 1 : 1;
    return usableValues
      .map((value, index) => {
        const x = padding + (spanX * index) / denominator;
        const y = height - padding - ((value - min) / spread) * spanY;
        return `${x.toFixed(2)},${y.toFixed(2)}`;
      })
      .join(" ");
  };
  const metricsDomainRows = (snapshot) => snapshot?.insights?.domains?.items || [];
  const batchErrorScore = (batch) => Number(batch?.outcomes?.failed || 0) + Number(batch?.outcomes?.expired || 0);
  const metricsBatchRows = (snapshot) => {
    const items = [...(snapshot?.insights?.batches?.items || [])];
    return items.sort((left, right) => {
      const scoreDelta = batchErrorScore(right) - batchErrorScore(left);
      if (scoreDelta !== 0) return scoreDelta;
      const processedDelta = Number(right?.processed_total || 0) - Number(left?.processed_total || 0);
      if (processedDelta !== 0) return processedDelta;
      return String(left?.batch_id || "").localeCompare(String(right?.batch_id || ""));
    });
  };
  const metricsTopAPIP90 = (snapshot) =>
    (snapshot?.api?.routes || []).reduce((highest, route) => {
      const value = Number(route?.latency_ms?.p90 || 0);
      return value > highest ? value : highest;
    }, 0);
  const hasMetricsData = (snapshot) => Boolean(snapshot && snapshot.generated_at);
  const seriesLast = (values = []) => (values.length ? Number(values[values.length - 1] || 0) : 0);
  const matchesSeverityFilter = (job) => {
    if (severityFilter === "warnings_plus") {
      return (
        jobSeverityTotal(job, "WARNING") > 0 ||
        jobSeverityTotal(job, "ERROR") > 0 ||
        jobSeverityTotal(job, "CRITICAL") > 0
      );
    }
    if (severityFilter === "errors_only") {
      return jobSeverityTotal(job, "ERROR") > 0 || jobSeverityTotal(job, "CRITICAL") > 0;
    }
    return true;
  };

  const normalizeDomainInput = (value) => {
    const trimmed = (value || "").trim();
    if (!trimmed) return "";
    try {
      const hasScheme = /^[a-z][a-z0-9+.-]*:\/\//i.test(trimmed);
      const url = new URL(hasScheme ? trimmed : `http://${trimmed}`);
      return url.hostname;
    } catch (error) {
      return trimmed;
    }
  };

  const nextUndelegatedRowID = (prefix) => `${prefix}-${++undelegatedRowCounter}`;
  const emptyUndelegatedNameserverRow = () => ({ id: nextUndelegatedRowID("ns"), ns: "", ip: "" });
  const emptyUndelegatedDSRow = () => ({
    id: nextUndelegatedRowID("ds"),
    keytag: "",
    algorithm: "",
    digtype: "",
    digest: ""
  });
  const trimUndelegatedNameserverRow = (row = {}) => ({
    ns: String(row?.ns || "").trim(),
    ip: String(row?.ip || "").trim()
  });
  const trimUndelegatedDSRow = (row = {}) => ({
    keytag: String(row?.keytag || "").trim(),
    algorithm: String(row?.algorithm || "").trim(),
    digtype: String(row?.digtype || "").trim(),
    digest: String(row?.digest || "").trim()
  });
  const isIPv4Address = (value) => {
    const text = String(value || "").trim();
    const parts = text.split(".");
    if (parts.length !== 4) return false;
    return parts.every((part) => /^\d{1,3}$/.test(part) && Number(part) >= 0 && Number(part) <= 255);
  };
  const isIPv6Address = (value) => {
    const text = String(value || "").trim();
    if (!text.includes(":")) return false;
    try {
      const parsed = new URL(`http://[${text}]`).hostname;
      return parsed.startsWith("[") && parsed.endsWith("]");
    } catch (error) {
      return false;
    }
  };
  const isIPAddress = (value) => isIPv4Address(value) || isIPv6Address(value);
  const isUIntInRange = (value, min, max) => {
    if (!/^\d+$/.test(value)) return false;
    const numeric = Number(value);
    return Number.isFinite(numeric) && numeric >= min && numeric <= max;
  };
  const isHexDigest = (value) => /^[0-9a-fA-F]+$/.test(value);

  const addUndelegatedNameserverRow = () => {
    undelegatedNameservers = [...undelegatedNameservers, emptyUndelegatedNameserverRow()];
  };

  const removeUndelegatedNameserverRow = (rowID) => {
    undelegatedNameservers = undelegatedNameservers.filter((row) => row?.id !== rowID);
  };

  const addUndelegatedDSRow = () => {
    undelegatedDSInfo = [...undelegatedDSInfo, emptyUndelegatedDSRow()];
  };

  const removeUndelegatedDSRow = (rowID) => {
    undelegatedDSInfo = undelegatedDSInfo.filter((row) => row?.id !== rowID);
  };

  const buildUndelegatedPayload = () => {
    const nameservers = [];
    for (let i = 0; i < undelegatedNameservers.length; i += 1) {
      const row = trimUndelegatedNameserverRow(undelegatedNameservers[i]);
      if (!row.ns && !row.ip) continue;
      if (!row.ns) {
        return { error: `Undelegated nameserver row ${i + 1}: NS is required when IP is provided.` };
      }
      if (/\s/.test(row.ns)) {
        return { error: `Undelegated nameserver row ${i + 1}: NS must not contain whitespace.` };
      }
      if (row.ip && !isIPAddress(row.ip)) {
        return { error: `Undelegated nameserver row ${i + 1}: IP must be a valid IPv4 or IPv6 address.` };
      }
      const payloadRow = { ns: row.ns };
      if (row.ip) payloadRow.ip = row.ip;
      nameservers.push(payloadRow);
    }

    const dsInfo = [];
    for (let i = 0; i < undelegatedDSInfo.length; i += 1) {
      const row = trimUndelegatedDSRow(undelegatedDSInfo[i]);
      const hasAny = row.keytag || row.algorithm || row.digtype || row.digest;
      if (!hasAny) continue;
      const hasAll = row.keytag && row.algorithm && row.digtype && row.digest;
      if (!hasAll) {
        return {
          error: `Undelegated DS row ${i + 1}: keytag, algorithm, digest type, and digest are all required.`
        };
      }
      if (!isUIntInRange(row.keytag, 0, 65535)) {
        return { error: `Undelegated DS row ${i + 1}: keytag must be in range 0-65535.` };
      }
      if (!isUIntInRange(row.algorithm, 0, 255)) {
        return { error: `Undelegated DS row ${i + 1}: algorithm must be in range 0-255.` };
      }
      if (!isUIntInRange(row.digtype, 0, 255)) {
        return { error: `Undelegated DS row ${i + 1}: digest type must be in range 0-255.` };
      }
      if (!isHexDigest(row.digest)) {
        return { error: `Undelegated DS row ${i + 1}: digest must be hex encoded.` };
      }
      dsInfo.push({
        keytag: Number(row.keytag),
        algorithm: Number(row.algorithm),
        digtype: Number(row.digtype),
        digest: row.digest.toUpperCase()
      });
    }

    return { nameservers, dsInfo };
  };

  const moduleLevels = ["NOTICE", "WARNING", "ERROR", "CRITICAL"];
  const normalizeLevel = (value) => (value || "INFO").toUpperCase();
  const formatSeconds = (value) => {
    const numeric = Number(value);
    if (!Number.isFinite(numeric)) return "0.00";
    return numeric.toFixed(2);
  };
  const entryMessage = (entry) => {
    if (!entry) return "";
    if (entry.message) return entry.message;
    if (entry.raw) return entry.raw;
    return [entry.module, entry.testcase, entry.tag].filter(Boolean).join(":");
  };
  const entryMeta = (entry) => [entry?.testcase, entry?.tag].filter(Boolean).join(" · ");
  const moduleId = (key) => `module-${String(key).toLowerCase().replace(/[^a-z0-9]+/g, "-")}`;
  const groupRawEntries = (raw) => {
    const entries = raw?.entries || [];
    const modules = new Map();
    entries.forEach((entry) => {
      const name = entry.module || "Unspecified";
      const key = name.toUpperCase();
      if (!modules.has(key)) {
        modules.set(key, {
          key,
          name,
          entries: [],
          counts: {}
        });
      }
      const current = modules.get(key);
      current.entries.push(entry);
      const level = normalizeLevel(entry.level);
      current.counts[level] = (current.counts[level] || 0) + 1;
    });
    return Array.from(modules.values());
  };
  const toggleModule = (key) => {
    moduleOpen = { ...moduleOpen, [key]: !moduleOpen[key] };
  };

  const normalizeTab = (value) => {
    const tab = String(value || "").replace(/^\/+/, "").toLowerCase();
    if (tab === "single" || tab === "job" || tab === "jobs" || tab === "home") return "single";
    if (tab === "recent" || tab === "tests") return "recent";
    if (tab === "batches" || tab === "batch") return "batches";
    if (tab === "metrics" || tab === "metric") return "metrics";
    return "";
  };

  const setTab = (tab) => {
    const next = normalizeTab(tab) || "single";
    const changed = activeTab !== next;
    activeTab = next;
    const nextHash = `#/${next}`;
    if (window.location.hash !== nextHash) {
      window.history.replaceState(
        null,
        "",
        `${window.location.pathname}${window.location.search}${nextHash}`
      );
    }
    if (changed && statusMessage) {
      clearStatus();
    }
    if (next === "recent") {
      loadJobs();
    } else if (next === "batches") {
      loadRecentBatchOptions();
      if (selectedBatchId) {
        loadBatch(selectedBatchId);
      }
    } else if (next === "metrics") {
      loadMetrics();
    }
  };

  const updateTabFromHash = () => {
    const hash = window.location.hash || "";
    const value = hash.replace(/^#\/?/, "");
    const next = normalizeTab(value) || "single";
    activeTab = next;
    if (!hash) {
      window.history.replaceState(
        null,
        "",
        `${window.location.pathname}${window.location.search}#/${next}`
      );
    }
  };

  const loadJobs = async (options = {}) => {
    const { resetCursor = false } = options;
    if (resetCursor) {
      recentCursor = 0;
    }
    jobsLoading = true;
    try {
      const params = new URLSearchParams({
        limit: String(normalizeRecentPageSize(recentPageSize)),
        sort: jobSort
      });
      const cursor = normalizeCursor(recentCursor);
      if (cursor > 0) {
        params.set("cursor", String(cursor));
      }
      const normalizedBatchID = jobBatchFilter.trim();
      if (normalizedBatchID) {
        params.set("batch_id", normalizedBatchID);
      }
      const normalizedDomain = recentDomainFilter.trim();
      if (normalizedDomain) {
        params.set("domain", normalizedDomain);
      }
      if (severityFilter !== "all") {
        params.set("severity", severityFilter);
      }
      const list = await apiFetch(`/jobs?${params.toString()}`);
      jobs = list.items || [];
      recentTotal = Number.isFinite(Number(list.total)) ? Number(list.total) : jobs.length;
      recentOffset = normalizeCursor(list.offset);
      recentNextCursor = String(list.next_cursor || "");
      recentPrevCursor = String(list.prev_cursor || "");
      if (autoRefreshRecent && !hasRunningOrQueuedJobs(jobs)) {
        autoRefreshRecent = false;
      }
    } catch (error) {
      setStatus(`Failed to load jobs: ${error.message}`, "warn");
    } finally {
      jobsLoading = false;
    }
  };

  const applyRecentFilters = async () => {
    recentCursor = 0;
    await loadJobs({ resetCursor: true });
  };

  const clearRecentFilters = async () => {
    jobBatchFilter = "";
    recentDomainFilter = "";
    await loadJobs({ resetCursor: true });
  };

  const goToRecentCursor = async (cursor) => {
    const parsed = Number(cursor);
    recentCursor = Number.isFinite(parsed) && parsed > 0 ? parsed : 0;
    await loadJobs();
  };

  const submitSingle = async () => {
    const normalizedDomain = normalizeDomainInput(singleDomain);
    if (!normalizedDomain) {
      setStatus("Domain is required.", "warn");
      return;
    }
    const undelegatedPayload = buildUndelegatedPayload();
    if (undelegatedPayload.error) {
      setStatus(undelegatedPayload.error, "warn");
      return;
    }
    singleSubmitting = true;
    createdJobId = "";
    try {
      const payload = {
        domain: normalizedDomain
      };
      if (singleIPMode === "disable_ipv4") {
        payload.profile_overrides = {
          net: {
            ipv4: false,
            ipv6: true
          }
        };
      } else if (singleIPMode === "disable_ipv6") {
        payload.profile_overrides = {
          net: {
            ipv4: true,
            ipv6: false
          }
        };
      }
      if (undelegatedPayload.nameservers.length > 0) {
        payload.nameservers = undelegatedPayload.nameservers;
      }
      if (undelegatedPayload.dsInfo.length > 0) {
        payload.ds_info = undelegatedPayload.dsInfo;
      }

      const job = await apiFetch("/jobs", {
        method: "POST",
        body: JSON.stringify(payload)
      });
      createdJobId = job.id;
      selectedJobId = job.id;
      autoRefreshJob = true;
      jobInspectorHighlight = true;
      if (jobInspectorHighlightTimer) {
        clearTimeout(jobInspectorHighlightTimer);
      }
      jobInspectorHighlightTimer = setTimeout(() => {
        jobInspectorHighlight = false;
        jobInspectorHighlightTimer = null;
      }, 6000);
      setStatus(`Job ${job.id} created.`, "ok");
      recentCursor = 0;
      await loadJobs({ resetCursor: true });
      await loadJob(job.id);
    } catch (error) {
      setStatus(`Failed to create job: ${error.message}`, "warn");
    } finally {
      singleSubmitting = false;
    }
  };

  const submitBatch = async () => {
    const domains = batchDomains
      .split(/\n/)
      .map((entry) => normalizeDomainInput(entry))
      .filter(Boolean);
    if (!domains.length) {
      setStatus("Provide at least one domain for the batch.", "warn");
      return;
    }
    batchSubmitting = true;
    createdBatchId = "";
    try {
      const payload = { domains };

      const response = await apiFetch("/jobs/batch", {
        method: "POST",
        body: JSON.stringify(payload)
      });
      createdBatchId = response.batch_id;
      selectedBatchId = response.batch_id;
      setStatus(`Batch ${response.batch_id} accepted.`, "ok");
      recentCursor = 0;
      await loadJobs({ resetCursor: true });
      await loadRecentBatchOptions();
      await loadBatch(response.batch_id, { resetCursor: true });
    } catch (error) {
      setStatus(`Failed to create batch: ${error.message}`, "warn");
    } finally {
      batchSubmitting = false;
    }
  };

  const loadJob = async (jobId = selectedJobId, options = {}) => {
    if (!jobId) return;
    const { silent = false } = options;
    if (!silent) {
      jobLoading = true;
    }
    try {
      const job = await apiFetch(`/jobs/${jobId}`);
      selectedJob = job;
      selectedJobResult = null;
      if (isResultReadyStatus(job.status)) {
        await loadJobResult(jobId);
      }
    } catch (error) {
      setStatus(`Failed to load job: ${error.message}`, "warn");
      selectedJob = null;
      selectedJobResult = null;
    } finally {
      if (!silent) {
        jobLoading = false;
      }
    }
  };

  const loadJobResult = async (jobId = selectedJobId) => {
    if (!jobId) return;
    try {
      const locale = resultLocale ? `?locale=${encodeURIComponent(resultLocale)}` : "";
      selectedJobResult = await apiFetch(`/jobs/${jobId}/result${locale}`);
    } catch (error) {
      setStatus(`Failed to load job result: ${error.message}`, "warn");
    }
  };

  const batchQueryParams = () => {
    const params = new URLSearchParams({
      limit: String(normalizeBatchPageSize(batchPageSize)),
      sort: batchSort
    });
    const cursor = normalizeCursor(batchCursor);
    if (cursor > 0) {
      params.set("cursor", String(cursor));
    }
    if (batchStatusFilter) {
      params.set("status", batchStatusFilter);
    }
    const normalizedDomain = batchDomainFilter.trim();
    if (normalizedDomain) {
      params.set("domain", normalizedDomain);
    }
    return params;
  };

  const loadRecentBatchOptions = async () => {
    recentBatchLoading = true;
    try {
      const collected = [];
      const seen = new Set();
      let cursor = 0;
      let pages = 0;
      const maxItems = 20;
      const maxPages = 5;

      while (collected.length < maxItems && pages < maxPages) {
        const params = new URLSearchParams({
          limit: "100",
          sort: "created_at_desc"
        });
        if (cursor > 0) {
          params.set("cursor", String(cursor));
        }
        const list = await apiFetch(`/jobs?${params.toString()}`);
        const items = list?.items || [];
        for (const item of items) {
          const batchID = String(item?.batch_id || "").trim();
          if (!batchID || seen.has(batchID)) {
            continue;
          }
          seen.add(batchID);
          collected.push({
            id: batchID,
            createdAt: item?.created_at || ""
          });
          if (collected.length >= maxItems) {
            break;
          }
        }

        if (!list?.next_cursor) {
          break;
        }
        const nextCursor = normalizeCursor(list.next_cursor);
        if (nextCursor <= cursor) {
          break;
        }
        cursor = nextCursor;
        pages++;
      }
      recentBatchOptions = collected;
      syncSelectedRecentBatch();
    } catch (error) {
      setStatus(`Failed to load recent batches: ${error.message}`, "warn");
    } finally {
      recentBatchLoading = false;
    }
  };

  const loadBatch = async (batchId = selectedBatchId, options = {}) => {
    if (!batchId) return;
    const { resetCursor = false } = options;
    if (resetCursor) {
      batchCursor = 0;
    }
    batchLoading = true;
    try {
      const params = batchQueryParams();
      const batch = await apiFetch(`/batches/${batchId}?${params.toString()}`);
      selectedBatch = batch;
      if (autoRefreshBatch && !hasActiveBatchJobs(batch)) {
        autoRefreshBatch = false;
      }
    } catch (error) {
      setStatus(`Failed to load batch: ${error.message}`, "warn");
      selectedBatch = null;
    } finally {
      batchLoading = false;
    }
  };

  const applyBatchFilters = async () => {
    batchCursor = 0;
    await loadBatch(selectedBatchId, { resetCursor: true });
  };

  const clearBatchFilters = async () => {
    batchSort = "started_at_desc";
    batchPageSize = 20;
    batchStatusFilter = "";
    batchDomainFilter = "";
    batchCursor = 0;
    await loadBatch(selectedBatchId, { resetCursor: true });
  };

  const goToBatchCursor = async (cursor) => {
    const parsed = Number(cursor);
    batchCursor = Number.isFinite(parsed) && parsed > 0 ? parsed : 0;
    await loadBatch(selectedBatchId);
  };

  const loadMetrics = async (options = {}) => {
    const { silent = false } = options;
    if (!silent) {
      metricsLoading = true;
    }
    metricsError = "";
    try {
      metricsSnapshot = await fetchMetricsSnapshot(apiFetch, {
        window: metricsWindow,
        include: ["health", "jobs", "api", "quality", "insights", "trends"],
        limitDomains: normalizeMetricsLimit(metricsDomainLimit),
        limitBatches: normalizeMetricsLimit(metricsBatchLimit)
      });
      metricsLoadedAt = new Date().toISOString();
    } catch (error) {
      metricsError = error.message || "unknown error";
      if (!metricsSnapshot) {
        setStatus(`Failed to load metrics: ${metricsError}`, "warn");
      }
    } finally {
      if (!silent) {
        metricsLoading = false;
      }
    }
  };

  const loadLocales = async () => {
    try {
      const data = await apiFetch("/locales");
      if (Array.isArray(data?.locales) && data.locales.length > 0) {
        availableLocales = data.locales;
        // Re-validate current locale against what the server actually supports.
        if (!availableLocales.includes(resultLocale)) {
          resultLocale = "en";
          try { localStorage.setItem(localeKey, resultLocale); } catch (_) {}
        }
      }
    } catch (_) {
      // Keep availableLocales as ["en"] default; locale select stays hidden.
    }
  };

  const onLocaleChange = () => {
    try { localStorage.setItem(localeKey, resultLocale); } catch (_) {}
    if (selectedJobResult) {
      loadJobResult(selectedJobId);
    }
  };

  const startJobPolling = () => {
    if (jobPoller) clearInterval(jobPoller);
    if (!autoRefreshJob || !selectedJobId) return;
    jobPoller = setInterval(() => loadJob(selectedJobId, { silent: true }), 5000);
  };

  const startBatchPolling = () => {
    if (batchPoller) clearInterval(batchPoller);
    if (!autoRefreshBatch || !selectedBatchId) return;
    batchPoller = setInterval(() => loadBatch(), 7000);
  };

  const startRecentPolling = () => {
    if (recentPoller) clearInterval(recentPoller);
    if (!autoRefreshRecent || activeTab !== "recent") return;
    recentPoller = setInterval(() => loadJobs(), 7000);
  };

  const startMetricsPolling = () => {
    if (metricsPoller) clearInterval(metricsPoller);
    if (!autoRefreshMetrics || activeTab !== "metrics") return;
    metricsPoller = setInterval(() => loadMetrics({ silent: true }), 10000);
  };

  $: {
    autoRefreshJob;
    selectedJobId;
    startJobPolling();
  }

  $: {
    autoRefreshBatch;
    selectedBatchId;
    startBatchPolling();
  }

  $: {
    selectedBatchId;
    recentBatchOptions;
    syncSelectedRecentBatch();
  }

  $: {
    autoRefreshRecent;
    activeTab;
    startRecentPolling();
  }

  $: {
    autoRefreshMetrics;
    activeTab;
    metricsWindow;
    metricsDomainLimit;
    metricsBatchLimit;
    startMetricsPolling();
  }

  $: if (
    autoRefreshJob &&
    selectedJob &&
    selectedJob.id === selectedJobId &&
    (progressPercent(selectedJob) === 100 || isResultReadyStatus(selectedJob.status))
  ) {
    autoRefreshJob = false;
    jobInspectorHighlight = false;
  }

  $: if (autoRefreshBatch && selectedBatch && !hasActiveBatchJobs(selectedBatch)) {
    autoRefreshBatch = false;
  }

  $: {
    moduleGroups = groupRawEntries(selectedJobResult?.raw);
  }

  $: if (selectedJobResult?.job_id !== lastResultJobId) {
    lastResultJobId = selectedJobResult?.job_id || "";
    moduleOpen = {};
  }

  $: {
    jobs;
    severityFilter;
    filteredJobs = jobs.filter((job) => matchesSeverityFilter(job));
  }

  $: persistenceSignature = [
    activeTab,
    jobSort,
    severityFilter,
    jobBatchFilter,
    recentDomainFilter,
    String(recentPageSize),
    String(recentCursor),
    selectedBatchId,
    batchSort,
    String(batchPageSize),
    String(batchCursor),
    batchStatusFilter,
    batchDomainFilter
  ].join("|");

  $: if (persistenceReady && persistenceSignature) {
    persistState();
  }

  const initializeApp = () => {
    if (initialized || typeof window === "undefined") return;
    initialized = true;
    const storedTheme = localStorage.getItem(themeKey);
    if (storedTheme === "light" || storedTheme === "dark" || storedTheme === "system") {
      theme = storedTheme;
    }
    applyTheme(theme);

    // Locale: restore from localStorage, or auto-detect from browser language.
    const storedLocale = (() => { try { return localStorage.getItem(localeKey); } catch (_) { return null; } })();
    if (storedLocale) {
      resultLocale = storedLocale;
    } else {
      const browserLang = (typeof navigator !== "undefined" ? navigator.language || "" : "")
        .split("-")[0]
        .toLowerCase();
      if (browserLang) {
        resultLocale = browserLang; // validated against available list after loadLocales()
      }
    }
    loadLocales();

    updateTabFromHash();
    const urlState = readStateFromURL();
    if (urlState) {
      applyPersistedState(urlState);
    } else {
      const storageState = readStateFromStorage();
      applyPersistedState(storageState);
    }
    batchPageSize = normalizeBatchPageSize(batchPageSize);
    batchCursor = normalizeCursor(batchCursor);
    recentPageSize = normalizeRecentPageSize(recentPageSize);
    recentCursor = normalizeCursor(recentCursor);
    persistenceReady = true;
    window.addEventListener("hashchange", updateTabFromHash);
    loadJobs();
    if (activeTab === "batches") {
      loadRecentBatchOptions();
      if (selectedBatchId) {
        loadBatch(selectedBatchId);
      }
    }
    if (activeTab === "metrics") {
      loadMetrics();
    }
  };

  onMount(() => {
    initializeApp();
    return () => {
      if (jobPoller) clearInterval(jobPoller);
      if (batchPoller) clearInterval(batchPoller);
      if (recentPoller) clearInterval(recentPoller);
      if (metricsPoller) clearInterval(metricsPoller);
      if (jobInspectorHighlightTimer) clearTimeout(jobInspectorHighlightTimer);
      if (statusDismissTimer) clearTimeout(statusDismissTimer);
      window.removeEventListener("hashchange", updateTabFromHash);
    };
  });
</script>

<main>
  <header class="reveal" style="--d: 0.05s">
    <div class="header-text">
      <h1>Gonemaster</h1>
      <p class="subtitle">
        Launch single or batch domain jobs, watch progress, and inspect results from the embedded server UI.
      </p>
    </div>
    <div class="header-controls">
      {#if availableLocales.length > 1}
        <select
          bind:value={resultLocale}
          on:change={onLocaleChange}
          class="locale-select"
          title="Result message language"
          aria-label="Result language"
        >
          {#each availableLocales as code}
            <option value={code}>{localeLabel(code)}</option>
          {/each}
        </select>
      {/if}
      <button class="theme-toggle" type="button" on:click={cycleTheme} title={themeTitle} aria-label={themeTitle}>
        {themeIcon}
      </button>
    </div>
  </header>

  <div class="tabs" role="tablist" aria-label="Job views">
    {#each tabs as tab}
      <button
        class={`tab ${activeTab === tab.id ? "active" : ""}`}
        type="button"
        role="tab"
        id={`tab-${tab.id}`}
        aria-selected={activeTab === tab.id}
        aria-controls={`panel-${tab.id}`}
        on:click={() => setTab(tab.id)}
      >
        {tab.label}
      </button>
    {/each}
  </div>

  {#if activeTab === "single"}
    <div class="grid" id="panel-single" role="tabpanel" aria-labelledby="tab-single" style="margin-top: 22px;">
      <div class="card reveal" style="--d: 0.18s">
        <h2>Single Job</h2>
        <div class="stack">
          <label for="single-domain">Domain</label>
          <input
            id="single-domain"
            type="text"
            placeholder="example.com"
            bind:value={singleDomain}
            on:keydown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault();
                submitSingle();
              }
            }}
          />
        </div>
        <details class="advanced-options">
          <summary>Advanced profile</summary>
          <div class="stack advanced-stack">
            <label for="single-ip-mode">IP transport</label>
            <select id="single-ip-mode" bind:value={singleIPMode}>
              <option value="default">Profile default (IPv4 + IPv6)</option>
              <option value="disable_ipv4">Disable IPv4 (IPv6 only)</option>
              <option value="disable_ipv6">Disable IPv6 (IPv4 only)</option>
            </select>
            <div class="small">Choose at most one protocol to disable.</div>
          </div>
        </details>
        <details class="advanced-options">
          <summary>Undelegated / Pre-delegation</summary>
          <div class="stack advanced-stack">
            <div class="field-label">Nameservers (NS + optional IP)</div>
            {#if undelegatedNameservers.length === 0}
              <div class="small">No undelegated nameservers configured.</div>
            {:else}
              <div class="undelegated-list">
                {#each undelegatedNameservers as row, index (row.id)}
                  <div class="undelegated-row">
                    <input
                      type="text"
                      aria-label={`Undelegated NS ${index + 1}`}
                      placeholder="ns1.example.com"
                      bind:value={row.ns}
                    />
                    <input
                      type="text"
                      aria-label={`Undelegated NS IP ${index + 1}`}
                      placeholder="192.0.2.10 or 2001:db8::10"
                      bind:value={row.ip}
                    />
                    <button class="ghost mini-button" type="button" on:click={() => removeUndelegatedNameserverRow(row.id)}>
                      Remove
                    </button>
                  </div>
                {/each}
              </div>
            {/if}
            <button class="ghost" type="button" on:click={addUndelegatedNameserverRow}>
              Add nameserver
            </button>

            <div class="field-label">DS records</div>
            {#if undelegatedDSInfo.length === 0}
              <div class="small">No undelegated DS records configured.</div>
            {:else}
              <div class="undelegated-list">
                {#each undelegatedDSInfo as row, index (row.id)}
                  <div class="undelegated-ds-row">
                    <input type="text" aria-label={`Undelegated DS keytag ${index + 1}`} placeholder="12345" bind:value={row.keytag} />
                    <input type="text" aria-label={`Undelegated DS algorithm ${index + 1}`} placeholder="13" bind:value={row.algorithm} />
                    <input type="text" aria-label={`Undelegated DS digest type ${index + 1}`} placeholder="2" bind:value={row.digtype} />
                    <input
                      type="text"
                      aria-label={`Undelegated DS digest ${index + 1}`}
                      placeholder="ABCD..."
                      bind:value={row.digest}
                    />
                    <button class="ghost mini-button" type="button" on:click={() => removeUndelegatedDSRow(row.id)}>
                      Remove
                    </button>
                  </div>
                {/each}
              </div>
            {/if}
            <button class="ghost" type="button" on:click={addUndelegatedDSRow}>
              Add DS record
            </button>
            <div class="small">Validation: NS is required for nameserver rows; DS values must be numeric + hex digest.</div>
          </div>
        </details>
        <button on:click={submitSingle} disabled={singleSubmitting}>
          {singleSubmitting ? "Submitting..." : "Run Single Job"}
        </button>
        {#if createdJobId}
          <div class="small">Created job: <span class="mono">{createdJobId}</span></div>
        {/if}
      </div>

      <div class="card reveal" style="--d: 0.26s" class:highlight={jobInspectorHighlight}>
        <h2>Job Inspector</h2>
        <div class="stack">
          <label for="job-id">Job ID</label>
          <input id="job-id" type="text" placeholder="job_123" bind:value={selectedJobId} on:change={() => loadJob()} />
        </div>
        <div class="row">
          <button on:click={() => loadJob()} disabled={jobLoading}>{jobLoading ? "Loading..." : "Refresh"}</button>
          <button class="ghost" type="button" on:click={() => (autoRefreshJob = !autoRefreshJob)}>
            {autoRefreshJob ? "Auto refresh: on" : "Auto refresh: off"}
          </button>
        </div>
        {#if selectedJob}
          <div class="kv">
            <span>Status</span>
            <strong>{selectedJob.status} · {formatJobTotalRuntime(selectedJob)}</strong>
            <span>Progress</span>
            <div class="progress" role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow={progressPercent(selectedJob)}>
              <div class="progress-bar" style={`width: ${progressPercent(selectedJob)}%`}></div>
              <span class="progress-value">{progressPercent(selectedJob)}%</span>
            </div>
            <span>Domain</span>
            <strong class="mono">{selectedJob.domain}</strong>
            <span>Created</span>
            <strong>{formatTimestampLocal(selectedJob.created_at)}</strong>
          </div>
          {#if selectedJob.error}
            <div class="notice">Error: {selectedJob.error}</div>
          {/if}
        {/if}
        {#if selectedJobResult}
          <div class="stack">
            <div class="field-label">Result summary</div>
            {#if summaryRows(selectedJobResult.summary).length}
              <div class="summary-grid">
                {#each summaryRows(selectedJobResult.summary) as row (row.level)}
                  <div class={`summary-item severity-${row.level.toLowerCase()}`}>
                    <span class="summary-label">{row.level}</span>
                    <span class="summary-count">{row.count}</span>
                  </div>
                {/each}
              </div>
            {:else}
              <div class="summary-empty">No NOTICE/WARNING/ERROR entries.</div>
            {/if}
            <div class="field-label">Result details</div>
            {#if moduleGroups.length === 0}
              <div class="summary-empty">No raw entries available.</div>
            {:else}
              <div class="small">Grouped by module. Click a module to expand.</div>
              <div class="module-list">
                {#each moduleGroups as group (group.key)}
                  <div class="module-card">
                    <button
                      class="module-toggle"
                      type="button"
                      aria-expanded={!!moduleOpen[group.key]}
                      aria-controls={moduleId(group.key)}
                      on:click={() => toggleModule(group.key)}
                    >
                      <div class="module-title">{group.name}</div>
                      <div class="module-meta">{group.entries.length} entries</div>
                      <div class="module-badges">
                        {#each moduleLevels as level}
                          {#if group.counts[level]}
                            <span class={`level-pill severity-${level.toLowerCase()}`}>{level} {group.counts[level]}</span>
                          {/if}
                        {/each}
                      </div>
                      <span class={`module-chevron ${moduleOpen[group.key] ? "open" : ""}`}></span>
                    </button>
                    {#if moduleOpen[group.key]}
                      <div class="module-body" id={moduleId(group.key)}>
                        <div class="result-header">
                          <span>Seconds</span>
                          <span>Level</span>
                          <span>Message</span>
                        </div>
                        {#each group.entries as entry}
                          {@const level = normalizeLevel(entry.level)}
                          {@const meta = entryMeta(entry)}
                          <div class="result-row">
                            <span class="entry-time">{formatSeconds(entry.timestamp)}</span>
                          <span class={`entry-level severity-${level.toLowerCase()}`}>{level}</span>
                            <span class="entry-message">{entryMessage(entry)}</span>
                          </div>
                          {#if meta}
                            <div class="entry-meta">{meta}</div>
                          {/if}
                        {/each}
                      </div>
                    {/if}
                  </div>
                {/each}
              </div>
            {/if}
          </div>
        {/if}
        {#if selectedJob && !selectedJobResult && isResultReadyStatus(selectedJob.status)}
          <button class="ghost" type="button" on:click={() => loadJobResult()}>
            Load result payload
          </button>
        {/if}
      </div>
    </div>
  {:else if activeTab === "recent"}
    <div class="card reveal" id="panel-recent" role="tabpanel" aria-labelledby="tab-recent" style="--d: 0.34s; margin-top: 22px;">
      <h2>Recent Tests</h2>
      <div class="row">
        <button class="ghost" type="button" on:click={loadJobs} disabled={jobsLoading}>
          {jobsLoading ? "Refreshing..." : "Refresh list"}
        </button>
        <button class="ghost" type="button" on:click={() => (autoRefreshRecent = !autoRefreshRecent)}>
          {autoRefreshRecent ? "Auto refresh: on" : "Auto refresh: off"}
        </button>
        <div class="sort-control">
          <label for="recent-sort">Sort</label>
          <select id="recent-sort" bind:value={jobSort} on:change={applyRecentFilters}>
            {#each jobSortOptions as option}
              <option value={option.id}>{option.label}</option>
            {/each}
          </select>
        </div>
        <div class="sort-control">
          <label for="recent-page-size">Page size</label>
          <select id="recent-page-size" bind:value={recentPageSize} on:change={applyRecentFilters}>
            {#each recentPageSizes as pageSize}
              <option value={pageSize}>{pageSize}</option>
            {/each}
          </select>
        </div>
        <div class="sort-control grow">
          <label for="recent-domain-filter">Domain contains</label>
          <input
            id="recent-domain-filter"
            type="text"
            placeholder="example.com"
            bind:value={recentDomainFilter}
            on:keydown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault();
                applyRecentFilters();
              }
            }}
          />
        </div>
        <div class="sort-control grow">
          <label for="recent-batch-filter">Batch ID filter</label>
          <input
            id="recent-batch-filter"
            type="text"
            placeholder="batch_123"
            bind:value={jobBatchFilter}
            on:keydown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault();
                applyRecentFilters();
              }
            }}
          />
        </div>
        <div class="row">
          <button class="ghost" type="button" on:click={applyRecentFilters} disabled={jobsLoading}>Apply filters</button>
          <button class="ghost" type="button" on:click={clearRecentFilters} disabled={jobsLoading}>Clear</button>
        </div>
      </div>
      <div class="severity-filter-bar" role="group" aria-label="Severity filters">
        {#each severityFilters as filter}
          <button
            type="button"
            class={`severity-filter ${severityFilter === filter.id ? "active" : ""}`}
            on:click={async () => {
              severityFilter = filter.id;
              await applyRecentFilters();
            }}
          >
            {filter.label}
          </button>
        {/each}
      </div>
      <div class="row batch-pagination">
        <button
          class="ghost"
          type="button"
          on:click={() => goToRecentCursor(recentPrevCursor)}
          disabled={!recentPrevCursor || jobsLoading}
        >
          Previous
        </button>
        <button
          class="ghost"
          type="button"
          on:click={() => goToRecentCursor(recentNextCursor)}
          disabled={!recentNextCursor || jobsLoading}
        >
          Next
        </button>
        <span class="small">
          Showing {jobs.length} of {recentTotal} matching jobs (offset {recentOffset || 0})
        </span>
      </div>
      <div class="list">
        {#if jobs.length === 0}
          <div class="small">No jobs yet. Run a single or batch job from the tabs above.</div>
        {:else if filteredJobs.length === 0}
          <div class="small">No jobs match the selected severity filter.</div>
        {:else}
          {#each filteredJobs as job (job.id)}
            <div class="list-item">
              <div class="list-item-main">
                <div class="mono">{job.id}</div>
                <div class="small">{job.domain} - {job.status}</div>
                {#if job.batch_id}
                  <div class="small mono">Batch: {job.batch_id}</div>
                {/if}
                <div class="job-severity-tags">
                  {#if jobSeverityRows(job).length}
                    {#each jobSeverityRows(job) as entry (entry.level)}
                      <span class={`level-pill severity-${entry.level.toLowerCase()}`}>{entry.level} {entry.count}</span>
                    {/each}
                  {:else}
                    <span class="small">No severity entries.</span>
                  {/if}
                </div>
                <div class="progress compact list-progress" role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow={progressPercent(job)}>
                  <div class="progress-bar" style={`width: ${progressPercent(job)}%`}></div>
                  <span class="progress-value">{progressPercent(job)}%</span>
                </div>
              </div>
              <button class="ghost" type="button" on:click={() => {
                selectedJobId = job.id;
                loadJob(job.id);
                setTab("single");
              }}>Inspect</button>
            </div>
          {/each}
        {/if}
      </div>
    </div>
  {:else if activeTab === "batches"}
    <div class="grid" id="panel-batches" role="tabpanel" aria-labelledby="tab-batches" style="margin-top: 22px;">
      <div class="card reveal" style="--d: 0.22s">
        <h2>Batch Jobs</h2>
        <div class="stack">
          <label for="batch-domains">Domains (one per line)</label>
          <textarea
            id="batch-domains"
            placeholder={`example.com
example.org`}
            bind:value={batchDomains}
          ></textarea>
        </div>
        <button class="secondary" on:click={submitBatch} disabled={batchSubmitting}>
          {batchSubmitting ? "Submitting..." : "Run Batch"}
        </button>
        {#if createdBatchId}
          <div class="small">Created batch: <span class="mono">{createdBatchId}</span></div>
        {/if}
      </div>

      <div class="card reveal" style="--d: 0.3s">
        <h2>Batch Inspector</h2>
        <div class="stack">
          <label for="batch-recent">Recent batches</label>
          <select
            id="batch-recent"
            bind:value={selectedRecentBatch}
            disabled={recentBatchLoading}
            on:change={async () => {
              const nextBatchID = selectedRecentBatch.trim();
              if (!nextBatchID) return;
              selectedBatchId = nextBatchID;
              await loadBatch(nextBatchID, { resetCursor: true });
            }}
          >
            <option value="">{recentBatchLoading ? "Loading latest batches..." : "Select one of the latest 20 batches"}</option>
            {#each recentBatchOptions as option}
              <option value={option.id}>{formatRecentBatchOption(option)}</option>
            {/each}
          </select>
          <div class="small">Latest 20 unique batch IDs, newest first.</div>
          <label for="batch-id">Batch ID</label>
          <input
            id="batch-id"
            type="text"
            placeholder="batch_123"
            bind:value={selectedBatchId}
            on:change={() => loadBatch(selectedBatchId, { resetCursor: true })}
          />
        </div>
        <div class="row">
          <button
            on:click={async () => {
              await loadRecentBatchOptions();
              await loadBatch();
            }}
            disabled={batchLoading}
          >
            {batchLoading ? "Loading..." : "Refresh"}
          </button>
          <button class="ghost" type="button" on:click={() => (autoRefreshBatch = !autoRefreshBatch)}>
            {autoRefreshBatch ? "Auto refresh: on" : "Auto refresh: off"}
          </button>
        </div>
        <div class="batch-controls">
          <div class="sort-control">
            <label for="batch-sort">Sort</label>
            <select id="batch-sort" bind:value={batchSort} on:change={applyBatchFilters}>
              {#each batchSortOptions as option}
                <option value={option.id}>{option.label}</option>
              {/each}
            </select>
          </div>
          <div class="sort-control">
            <label for="batch-page-size">Page size</label>
            <select id="batch-page-size" bind:value={batchPageSize} on:change={applyBatchFilters}>
              {#each batchPageSizes as pageSize}
                <option value={pageSize}>{pageSize}</option>
              {/each}
            </select>
          </div>
          <div class="sort-control">
            <label for="batch-status">Status</label>
            <select id="batch-status" bind:value={batchStatusFilter} on:change={applyBatchFilters}>
              {#each batchStatuses as status}
                <option value={status}>{status || "all"}</option>
              {/each}
            </select>
          </div>
          <div class="sort-control grow">
            <label for="batch-domain-filter">Domain contains</label>
            <input
              id="batch-domain-filter"
              type="text"
              placeholder="example"
              bind:value={batchDomainFilter}
              on:keydown={(event) => {
                if (event.key === "Enter") {
                  event.preventDefault();
                  applyBatchFilters();
                }
              }}
            />
          </div>
          <div class="row">
            <button class="ghost" type="button" on:click={applyBatchFilters} disabled={batchLoading}>Apply filters</button>
            <button class="ghost" type="button" on:click={clearBatchFilters} disabled={batchLoading}>Clear</button>
          </div>
        </div>
        {#if selectedBatch}
          <div class="kv">
            <span>Total</span>
            <strong>{selectedBatch.total}</strong>
            <span>Created</span>
            <strong>{formatTimestampLocal(selectedBatch.created_at)}</strong>
            <span>Total runtime</span>
            <strong>{formatBatchTotalRuntime(selectedBatch)}</strong>
            <span>Status counts</span>
            <strong>{formatBatchStatusCounts(selectedBatch.status_counts)}</strong>
          </div>
          <div class="stack">
            <div class="field-label">Jobs</div>
            <div class="row batch-pagination">
              <button
                class="ghost"
                type="button"
                on:click={() => goToBatchCursor(selectedBatch.prev_cursor)}
                disabled={!selectedBatch.prev_cursor || batchLoading}
              >
                Previous
              </button>
              <button
                class="ghost"
                type="button"
                on:click={() => goToBatchCursor(selectedBatch.next_cursor)}
                disabled={!selectedBatch.next_cursor || batchLoading}
              >
                Next
              </button>
              <span class="small">
                Showing {selectedBatch.items.length} of {selectedBatch.total} matching jobs (offset {selectedBatch.offset || 0})
              </span>
            </div>
            <div class="list">
              {#if selectedBatch.items.length === 0}
                <div class="small">No batch jobs match the current filters.</div>
              {:else}
                {#each selectedBatch.items as item (item.id)}
                  <div class="list-item">
                    <div class="list-item-main">
                      <div class="mono">{item.id}</div>
                      <div class="small">{item.domain} - {item.status}</div>
                      <div class="progress compact list-progress" role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow={progressPercent(item)}>
                        <div class="progress-bar" style={`width: ${progressPercent(item)}%`}></div>
                        <span class="progress-value">{progressPercent(item)}%</span>
                      </div>
                    </div>
                    <button class="ghost" type="button" on:click={() => {
                      selectedJobId = item.id;
                      loadJob(item.id);
                      setTab("single");
                    }}>Inspect</button>
                  </div>
                {/each}
              {/if}
            </div>
          </div>
        {/if}
      </div>
    </div>
  {:else if activeTab === "metrics"}
    <div class="card reveal" id="panel-metrics" role="tabpanel" aria-labelledby="tab-metrics" style="--d: 0.38s; margin-top: 22px;">
      <h2>Metrics</h2>
      <div class="row">
        <button class="ghost" type="button" on:click={() => loadMetrics()} disabled={metricsLoading}>
          {metricsLoading ? "Refreshing..." : "Refresh metrics"}
        </button>
        <button class="ghost" type="button" on:click={() => (autoRefreshMetrics = !autoRefreshMetrics)}>
          {autoRefreshMetrics ? "Auto refresh: on" : "Auto refresh: off"}
        </button>
        <div class="sort-control">
          <label for="metrics-window">Trend window</label>
          <select id="metrics-window" bind:value={metricsWindow} on:change={() => loadMetrics()}>
            {#each metricsWindowOptions as option}
              <option value={option.id}>{option.label}</option>
            {/each}
          </select>
        </div>
        <div class="sort-control">
          <label for="metrics-domain-limit">Top domains</label>
          <select id="metrics-domain-limit" bind:value={metricsDomainLimit} on:change={() => loadMetrics()}>
            {#each metricsLimitOptions as value}
              <option value={value}>{value}</option>
            {/each}
          </select>
        </div>
        <div class="sort-control">
          <label for="metrics-batch-limit">Top batches</label>
          <select id="metrics-batch-limit" bind:value={metricsBatchLimit} on:change={() => loadMetrics()}>
            {#each metricsLimitOptions as value}
              <option value={value}>{value}</option>
            {/each}
          </select>
        </div>
      </div>
      <div class="small">
        Last loaded: {lastLoadedLabel(metricsLoadedAt)} | Server uptime: {formatUptime(metricsSnapshot?.health?.uptime_seconds)} | Server version: {metricsSnapshot?.server_version || "unknown"}
      </div>

      {#if metricsLoading && !hasMetricsData(metricsSnapshot)}
        <div class="summary-empty">Loading metrics snapshot...</div>
      {:else if metricsError && !hasMetricsData(metricsSnapshot)}
        <div class="notice">Failed to load metrics: {metricsError}</div>
        <button type="button" on:click={() => loadMetrics()}>Retry</button>
      {:else if hasMetricsData(metricsSnapshot)}
        {@const throughputSeries = metricsSeriesValues(metricsSnapshot, metricsWindow, "throughput")}
        {@const failedSeries = metricsSeriesValues(metricsSnapshot, metricsWindow, "failed")}
        {@const querySeriesIPv4 = metricsSeriesValues(metricsSnapshot, metricsWindow, "dns_queries_ipv4_per_second")}
        {@const querySeriesIPv6 = metricsSeriesValues(metricsSnapshot, metricsWindow, "dns_queries_ipv6_per_second")}
        {@const queryBounds = sparklineBounds(querySeriesIPv4, querySeriesIPv6)}
        {@const severityTotals = metricsSnapshot?.quality?.severity?.totals || {}}

        {#if metricsError}
          <div class="notice">Showing last snapshot. Latest refresh failed: {metricsError}</div>
        {/if}

        <div class="summary-grid metrics-summary-grid">
          <div class="summary-item" title={metricsCardHelp.queue_depth}>
            <span class="summary-label">Queue depth</span>
            <span class="summary-count">{formatInteger(metricsSnapshot?.health?.queue_depth)}</span>
          </div>
          <div class="summary-item" title={metricsCardHelp.in_flight_jobs}>
            <span class="summary-label">In-flight jobs</span>
            <span class="summary-count">{formatInteger(metricsSnapshot?.health?.in_flight_jobs)}</span>
          </div>
          <div class="summary-item" title={metricsCardHelp.dns_queries_ipv4_total}>
            <span class="summary-label">Total IPv4 queries</span>
            <span class="summary-count">{formatCompactInteger(metricsSnapshot?.health?.dns_queries_ipv4_total)}</span>
          </div>
          <div class="summary-item" title={metricsCardHelp.dns_queries_ipv6_total}>
            <span class="summary-label">Total IPv6 queries</span>
            <span class="summary-count">{formatCompactInteger(metricsSnapshot?.health?.dns_queries_ipv6_total)}</span>
          </div>
          <div class="summary-item" title={metricsCardHelp.success_rate}>
            <span class="summary-label">Success rate</span>
            <span class="summary-count">{formatPercent(metricsSnapshot?.quality?.outcomes?.success_rate)}</span>
          </div>
          <div class="summary-item" title={metricsCardHelp.failed_rate}>
            <span class="summary-label">Failure rate</span>
            <span class="summary-count">{formatPercent(metricsSnapshot?.quality?.outcomes?.failed_rate)}</span>
          </div>
          <div class="summary-item" title={metricsCardHelp.api_p90}>
            <span class="summary-label">API p90</span>
            <span class="summary-count">{formatDurationMs(metricsTopAPIP90(metricsSnapshot))}</span>
          </div>
          <div class="summary-item" title={metricsCardHelp.avg_job_duration}>
            <span class="summary-label">Avg job duration</span>
            <span class="summary-count">{formatDurationMs(metricsSnapshot?.quality?.job_duration_ms?.avg)}</span>
          </div>
          <div class="summary-item jobs-finished" title={metricsCardHelp.completed_total}>
            <span class="summary-label">Total jobs finished</span>
            <span class="summary-count">{formatInteger(metricsSnapshot?.jobs?.completed_total)}</span>
          </div>
          <div class="summary-item failed-jobs" title={metricsCardHelp.failed_total}>
            <span class="summary-label">Failed jobs</span>
            <span class="summary-count">{formatInteger(metricsSnapshot?.quality?.outcomes?.failed_total)}</span>
          </div>
          {#each summaryLevels as level}
            <div class={`summary-item severity-${level.toLowerCase()}`} title={severityCardHelp(level)}>
              <span class="summary-label">{level}</span>
              <span class="summary-count">{formatInteger(severityTotals[level])}</span>
            </div>
          {/each}
        </div>

        <div class="metrics-trend-grid">
          <div class="metrics-trend-card">
            <div class="metrics-trend-head">
              <strong>Throughput</strong>
              <span>{formatInteger(seriesLast(throughputSeries))}/bucket</span>
            </div>
            <svg class="sparkline" viewBox="0 0 260 66" role="img" aria-label="Throughput trend">
              <polyline points={sparklinePoints(throughputSeries)} />
            </svg>
          </div>
          <div class="metrics-trend-card">
            <div class="metrics-trend-head">
              <strong>Failures</strong>
              <span>{formatInteger(seriesLast(failedSeries))}/bucket</span>
            </div>
            <svg class="sparkline sparkline-warn" viewBox="0 0 260 66" role="img" aria-label="Failure trend">
              <polyline points={sparklinePoints(failedSeries)} />
            </svg>
          </div>
          <div class="metrics-trend-card">
            <div class="metrics-trend-head">
              <strong>DNS queries/s</strong>
              <span>{formatRate(seriesLast(querySeriesIPv4) + seriesLast(querySeriesIPv6))}</span>
            </div>
            <svg class="sparkline sparkline-queries" viewBox="0 0 260 66" role="img" aria-label="DNS query rates trend">
              <polyline class="sparkline-ipv4" points={sparklinePoints(querySeriesIPv4, 260, 66, 6, queryBounds)} />
              <polyline class="sparkline-ipv6" points={sparklinePoints(querySeriesIPv6, 260, 66, 6, queryBounds)} />
            </svg>
            <div class="sparkline-legend">
              <span class="sparkline-legend-item">
                <span class="sparkline-legend-dot sparkline-legend-dot-ipv4" aria-hidden="true"></span>
                IPv4 {formatRate(seriesLast(querySeriesIPv4))}
              </span>
              <span class="sparkline-legend-item">
                <span class="sparkline-legend-dot sparkline-legend-dot-ipv6" aria-hidden="true"></span>
                IPv6 {formatRate(seriesLast(querySeriesIPv6))}
              </span>
            </div>
          </div>
        </div>

        <div class="metrics-tables">
          <div class="metrics-table-wrap">
            <h3>Top domains</h3>
            {#if metricsDomainRows(metricsSnapshot).length === 0}
              <div class="summary-empty">No domain insight data yet.</div>
            {:else}
              <table class="metrics-table">
                <thead>
                  <tr>
                    <th>Domain</th>
                    <th>Runs</th>
                    <th>Last status</th>
                    <th>Avg duration</th>
                    <th>Error+critical</th>
                  </tr>
                </thead>
                <tbody>
                  {#each metricsDomainRows(metricsSnapshot) as row}
                    <tr>
                      <td class="mono">{row.domain}</td>
                      <td>{formatInteger(row.runs_total)}</td>
                      <td>{row.last_status || "-"}</td>
                      <td>{formatDurationMs(row.avg_duration_ms)}</td>
                      <td>{formatInteger(Number(row?.severity_totals?.ERROR || 0) + Number(row?.severity_totals?.CRITICAL || 0))}</td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            {/if}
          </div>

          <div class="metrics-table-wrap">
            <h3>Error-heavy batches</h3>
            {#if metricsBatchRows(metricsSnapshot).length === 0}
              <div class="summary-empty">No batch insight data yet.</div>
            {:else}
              <table class="metrics-table metrics-table-batches">
                <colgroup>
                  <col class="metrics-col-batch-id" />
                  <col />
                  <col />
                  <col />
                  <col />
                </colgroup>
                <thead>
                  <tr>
                    <th>Batch ID</th>
                    <th>Processed</th>
                    <th>Failed+expired</th>
                    <th>Canceled</th>
                    <th>Error+critical</th>
                  </tr>
                </thead>
                <tbody>
                  {#each metricsBatchRows(metricsSnapshot) as row}
                    <tr>
                      <td class="mono metrics-batch-id" title={row.batch_id}>{row.batch_id}</td>
                      <td>{formatInteger(row.processed_total)}</td>
                      <td>{formatInteger(batchErrorScore(row))}</td>
                      <td>{formatInteger(row?.outcomes?.canceled || 0)}</td>
                      <td>{formatInteger(Number(row?.severity_totals?.ERROR || 0) + Number(row?.severity_totals?.CRITICAL || 0))}</td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            {/if}
          </div>
        </div>
      {:else}
        <div class="summary-empty">No metrics data available yet.</div>
      {/if}
    </div>
  {/if}

  {#if statusMessage}
    <div class={`status-toast status-${statusTone === "ok" ? "ok" : "warn"} reveal`} role="status" aria-live="polite" style="--d: 0.12s;">
      <div><strong>{statusTone === "ok" ? "OK" : "Heads up"}:</strong> {statusMessage}</div>
      <button class="status-toast-close" type="button" aria-label="Dismiss notification" on:click={clearStatus}>Dismiss</button>
    </div>
  {/if}
</main>
