<script>
  import { onMount } from "svelte";
  import { fetchMetricsSnapshot, metricsWindowOptions } from "./metrics.js";
  import { t, locale, loadCatalog } from "./i18n.js";
  import ProfileSettings from "./ProfileSettings.svelte";
  import ServerSettings from "./ServerSettings.svelte";

  const logoSrc = `${import.meta.env.BASE_URL}gonemaster.svg`;

  let statusMessage = "";
  let statusTone = "";
  let statusDismissTimer = null;

  let singleDomain = "";
  let singleTags = "";
  let singleSubmitting = false;
  let createdJobId = "";
  let singleIPMode = "default";
  let undelegatedNameservers = [];
  let undelegatedDSInfo = [];

  let batchDomains = "";
  let batchTags = "";
  let batchFromTagMode = false;
  let batchFromTag = "";
  let batchSubmitting = false;
  let createdBatchId = "";
  let batchProfileId = "";

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
  let selectedRun = null;
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
  let activeBatches = [];
  let activeBatchesLoading = false;
  let activeBatchesPoller = null;
  let queuePaused = false;
  let queuePauseToggling = false;
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
  // Server-controlled feature flag: whether scoring UI is shown. Defaults to
  // true so scoring is visible before the features response arrives.
  let scoringEnabled = true;
  let undelegatedRowCounter = 0;
  let notifyOnJobComplete = false;
  let notifyOnBatchComplete = false;
  let pendingPermission = null;

  // Theme management: "system" follows OS preference via CSS media query;
  // "light" and "dark" set data-theme on <html> explicitly.
  // "system" is the default for first-time visitors and is never stored.
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

  const osDark = () => typeof window !== "undefined" && window.matchMedia?.("(prefers-color-scheme: dark)").matches;

  const toggleTheme = () => {
    const effectiveDark = theme === "dark" || (theme === "system" && osDark());
    theme = effectiveDark ? "light" : "dark";
    localStorage.setItem(themeKey, theme);
    applyTheme(theme);
  };

  $: themeIcon = theme === "dark" || (theme === "system" && osDark()) ? "☾" : "☀";
  $: themeTitle = $t("theme_toggle_title", { theme: theme === "dark" ? $t("theme_dark") : $t("theme_light") });

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
    { id: "single", labelKey: "tab_single" },
    { id: "recent", labelKey: "tab_recent" },
    { id: "domains", labelKey: "tab_domains" },
    { id: "tags", labelKey: "tab_tags" },
    { id: "batches", labelKey: "tab_batches" },
    { id: "metrics", labelKey: "tab_metrics" },
    { id: "settings", labelKey: "tab_settings" }
  ];

  // Domains tab state.
  let domains = [];
  let domainsLoading = false;
  let domainsTotal = 0;
  let domainsOffset = 0;
  let domainsLimit = 50;
  let domainsNextCursor = "";
  let domainsPrevCursor = "";
  let domainNameFilter = "";
  let domainTagFilter = "";
  let domainLevelFilter = "";
  let selectedDomain = null;
  let domainRuns = [];
  let domainRunsTotal = 0;
  let domainRunsOffset = 0;
  let domainRunsLimit = 20;
  let domainRunsLoading = false;
  let domainRunsSortState = { key: "", direction: "asc" };
  let sortedDomainRuns = [];
  let domainResubmitting = false;
  let selectedDomainRunId = null;
  let selectedDomainRunResult = null;
  let domainRunResultLoading = false;
  let domainModuleOpen = {};
  let domainModuleGroups = [];
  let availableTags = [];
  let tagsLoaded = false;
  let availableProfiles = [];
  let profilesLoaded = false;
  let profilesLoading = false;
  let singleProfileId = "";

  // Tags tab state.
  let tagsList = [];
  let tagsListLoading = false;
  let tagCreateName = "";
  let tagCreateDescription = "";
  let tagCreating = false;
  let selectedTag = null;
  let tagSummary = null;
  let tagSummaryLoading = false;
  let tagDomains = [];
  let tagDomainsTotal = 0;
  let tagDomainsOffset = 0;
  let tagDomainsLimit = 50;
  let tagDomainsLoading = false;
  let tagDomainsSortState = { key: "", direction: "asc" };
  let sortedTagDomains = [];
  let tagDomainLevelFilter = "";
  let tagAddDomainsInput = "";
  let tagAddingDomains = false;
  let tagRemoveDomainsInput = "";
  let tagRemovingDomains = false;
  let tagRunAllSubmitting = false;
  let tagDeleteConfirm = false;
  let tagDeleting = false;
  let tagProfileDraftId = "";
  let tagProfileCurrentID = null;
  let tagProfileSelectedID = null;
  let tagProfileDirty = false;
  let tagProfileUpdating = false;
  let tagProfileClearing = false;
  let domainsSortState = { key: "", direction: "asc" };
  let sortedDomains = [];
  let tagsListSortState = { key: "", direction: "asc" };
  let sortedTagsList = [];
  let metricsDomainsSortState = { key: "", direction: "asc" };
  let sortedMetricsDomainRows = [];
  let metricsBatchesSortState = { key: "failed_expired", direction: "desc" };
  let sortedMetricsBatchRows = [];

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
    { id: "all", labelKey: "sev_all" },
    { id: "warnings_plus", labelKey: "sev_warnings_plus" },
    { id: "errors_only", labelKey: "sev_errors_only" }
  ];
  const jobSortOptions = [
    { id: "started_at_desc", labelKey: "sort_started_at_desc" },
    { id: "started_at_asc", labelKey: "sort_started_at_asc" },
    { id: "batch_id_asc", labelKey: "sort_batch_id_asc" },
    { id: "batch_id_desc", labelKey: "sort_batch_id_desc" },
    { id: "error_desc", labelKey: "sort_error_desc" },
    { id: "critical_desc", labelKey: "sort_critical_desc" },
    { id: "domain_asc", labelKey: "sort_domain_asc" },
    { id: "domain_desc", labelKey: "sort_domain_desc" }
  ];
  const batchSortOptions = [
    { id: "started_at_desc", labelKey: "sort_started_at_desc" },
    { id: "started_at_asc", labelKey: "sort_started_at_asc" },
    { id: "error_desc", labelKey: "sort_error_desc" },
    { id: "critical_desc", labelKey: "sort_critical_desc" },
    { id: "domain_asc", labelKey: "sort_domain_asc" },
    { id: "domain_desc", labelKey: "sort_domain_desc" },
    { id: "created_at_desc", labelKey: "sort_created_at_desc" },
    { id: "created_at_asc", labelKey: "sort_created_at_asc" }
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
    const parts = [option.id];
    if (option.createdAt) {
      const parsed = new Date(option.createdAt);
      if (!Number.isNaN(parsed.getTime())) parts.push(parsed.toLocaleString());
    }
    if (option.tag) parts.push(`[${option.tag}]`);
    return parts.join(" - ");
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
    window.history.replaceState(window.history.state, "", `${window.location.pathname}${search ? `?${search}` : ""}${hash}`);

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
  const normalizeOptionalProfileID = (value) => {
    const parsed = Number(value);
    if (!Number.isFinite(parsed) || parsed <= 0) return null;
    return parsed;
  };
  const profileNameByID = (profileID) => {
    const normalized = normalizeOptionalProfileID(profileID);
    if (!normalized) return "";
    const match = availableProfiles.find((profile) => profile.id === normalized);
    return match?.name || `#${normalized}`;
  };
  const jobProfileName = (job, run = null) => {
    const direct = String(job?.profile_name || run?.profile_name || "").trim();
    if (direct) return direct;
    return profileNameByID(job?.profile_id || run?.profile_id);
  };
  const prettyProfileJSON = (value) => {
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
    queue_depth: "help_queue_depth",
    in_flight_jobs: "help_in_flight_jobs",
    dns_queries_ipv4_total: "help_ipv4_queries",
    dns_queries_ipv6_total: "help_ipv6_queries",
    success_rate: "help_success_rate",
    failed_rate: "help_failed_rate",
    api_p90: "help_api_p90",
    avg_job_duration: "help_avg_duration",
    completed_total: "help_completed",
    failed_total: "help_failed",
    dns_cache_hits: "help_cache_hits",
    dns_cache_misses: "help_cache_misses",
    dns_cache_hit_rate: "help_cache_hit_rate",
    dns_lookups_total: "help_dns_lookups",
    jobs_per_minute: "help_jobs_per_minute"
  };
  const metricsResolutionSeconds = (snapshot, window) => {
    const windows = snapshot?.trends?.windows || {};
    return windows[window]?.resolution_seconds || (windows[Object.keys(windows)[0]]?.resolution_seconds) || 60;
  };
  const metricsJobsPerMinute = (snapshot, window) => {
    const points = metricsSeriesPoints(snapshot, window);
    if (!points.length) return 0;
    const last = Number(points[points.length - 1]?.throughput || 0);
    const resSec = metricsResolutionSeconds(snapshot, window);
    return last * (60 / resSec);
  };
  const metricsCacheHitRate = (snapshot) => {
    const hits = snapshot?.health?.dns_cache_hits || 0;
    const misses = snapshot?.health?.dns_cache_misses || 0;
    const total = hits + misses;
    if (total === 0) return 0;
    return hits / total;
  };
  const metricsFailedJobs = (snapshot) => {
    const currentFailed = Number(snapshot?.jobs?.status_counts?.failed);
    if (Number.isFinite(currentFailed)) return currentFailed;
    return Number(snapshot?.quality?.outcomes?.failed_total || 0);
  };
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
  const metricsBatchRows = (snapshot) => snapshot?.insights?.batches?.items || [];
  const metricsTopAPIP90 = (snapshot) =>
    (snapshot?.api?.routes || []).reduce((highest, route) => {
      const value = Number(route?.latency_ms?.p90 || 0);
      return value > highest ? value : highest;
    }, 0);
  const hasMetricsData = (snapshot) => Boolean(snapshot && snapshot.generated_at);
  const seriesLast = (values = []) => (values.length ? Number(values[values.length - 1] || 0) : 0);
  const textCollator = new Intl.Collator(undefined, { numeric: true, sensitivity: "base" });
  const compareText = (left, right) => textCollator.compare(String(left ?? ""), String(right ?? ""));
  const compareNumber = (left, right) => Number(left || 0) - Number(right || 0);
  const timestampValue = (value) => parseTimestamp(value)?.getTime() ?? -1;
  const compareTimestamp = (left, right) => compareNumber(timestampValue(left), timestampValue(right));
  const severityRank = (value) => {
    if (!value) return -1;
    const index = LEVEL_ORDER.indexOf(normalizeLevel(value));
    return index >= 0 ? index : -1;
  };
  const compareSeverity = (left, right) => compareNumber(severityRank(left), severityRank(right));
  const nextTableSort = (state, key, defaultDirection = "asc") =>
    state.key === key
      ? { key, direction: state.direction === "asc" ? "desc" : "asc" }
      : { key, direction: defaultDirection };
  const sortDomains = (key, defaultDir = "asc") => {
    domainsSortState = nextTableSort(domainsSortState, key, defaultDir);
    loadDomains({ reset: true });
  };
  const sortDomainRuns = (key, defaultDir = "asc") => {
    domainRunsSortState = nextTableSort(domainRunsSortState, key, defaultDir);
    loadDomainRuns({ reset: true });
  };
  const sortTagDomains = (key, defaultDir = "asc") => {
    tagDomainsSortState = nextTableSort(tagDomainsSortState, key, defaultDir);
    loadTagDomains({ reset: true });
  };

  const tableSortIndicator = (state, key) => {
    if (state.key !== key) return "";
    return state.direction === "asc" ? "▲" : "▼";
  };
  const tableSortAria = (state, key) => {
    if (state.key !== key) return "none";
    return state.direction === "asc" ? "ascending" : "descending";
  };
  const sortItems = (items, state, comparators, tieBreaker = null) => {
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
  // Maps UI sort state {key, direction} to an API sort parameter string.
  // Returns "" when no server-side sort is applicable (client-only columns).
  const domainSortParam = (state) => {
    const map = { name: "name", latest_level: "latest_level", latest_score: "latest_score", latest_run_at: "latest_run_at", run_count: "run_count" };
    const col = map[state?.key];
    return col ? `${col}_${state.direction}` : "";
  };
  const runSortParam = (state) => {
    const map = { finished_at: "finished_at", worst_level: "worst_level", score: "score", duration_ms: "duration", entry_count: "entry_count" };
    const col = map[state?.key];
    return col ? `${col}_${state.direction}` : "";
  };

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
        return { error: $t("error_ns_row_ns_required", { row: i + 1 }) };
      }
      if (/\s/.test(row.ns)) {
        return { error: $t("error_ns_row_ns_whitespace", { row: i + 1 }) };
      }
      if (row.ip && !isIPAddress(row.ip)) {
        return { error: $t("error_ns_row_ip_invalid", { row: i + 1 }) };
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
        return { error: $t("error_ds_row_all_required", { row: i + 1 }) };
      }
      if (!isUIntInRange(row.keytag, 0, 65535)) {
        return { error: $t("error_ds_row_keytag_range", { row: i + 1, min: 0, max: 65535 }) };
      }
      if (!isUIntInRange(row.algorithm, 0, 255)) {
        return { error: $t("error_ds_row_algorithm_range", { row: i + 1, min: 0, max: 255 }) };
      }
      if (!isUIntInRange(row.digtype, 0, 255)) {
        return { error: $t("error_ds_row_digtype_range", { row: i + 1, min: 0, max: 255 }) };
      }
      if (!isHexDigest(row.digest)) {
        return { error: $t("error_ds_row_digest_hex", { row: i + 1 }) };
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
  const LEVEL_ORDER = ["DEBUG", "INFO", "NOTICE", "WARNING", "ERROR", "CRITICAL"];
  const normalizeLevel = (value) => (value || "INFO").toUpperCase();
  // Returns the effective display level for a domain. The server only tracks
  const domainLevel = (d) => d?.latest_level || (d?.latest_run_at ? "INFO" : "");
  const worstLevel = (entries) => {
    if (!entries?.length) return "INFO";
    let worst = 0;
    for (const e of entries) {
      const idx = LEVEL_ORDER.indexOf(normalizeLevel(e.level));
      if (idx > worst) worst = idx;
    }
    return LEVEL_ORDER[worst] ?? "INFO";
  };
  const bannerClass = (level) => {
    const l = normalizeLevel(level);
    if (l === "CRITICAL") return "critical";
    if (l === "ERROR") return "error";
    if (l === "WARNING") return "warning";
    return "ok";
  };
  const isNoticeOrAbove = (level) => {
    return LEVEL_ORDER.indexOf(normalizeLevel(level)) >= LEVEL_ORDER.indexOf("NOTICE");
  };

  // ── Grade chip helpers ───────────────────────────────────────────────────
  const CAT_ORDER = ["dnssec", "nameserver_health", "connectivity", "zone_consistency"];
  const CAT_LABELS = { dnssec: "DNSSEC", nameserver_health: "Nameserver", connectivity: "Connectivity", zone_consistency: "Zone" };
  const BONUS_HIDDEN = new Set(["no_warnings_or_errors"]);
  // Returns true when a run/job has a score to display.
  const hasScore = (item) => item?.score != null && item?.grade != null;
  const GRADE_COLORS = { "A+": "var(--grade-aplus)", "A": "var(--grade-a)", "B": "var(--grade-b)", "C": "var(--grade-c)", "D": "var(--grade-d)", "F": "var(--grade-f)" };
  const gradeBarColor = (grade) => GRADE_COLORS[grade] ?? "var(--grade-a)";
  // Returns the full scoring result from either a run (score object) or a
  // job-list item where score is just an int and grade a string.
  const chipGrade  = (item) => item?.grade ?? null;
  const chipScore  = (item) => item?.score ?? null;
  // Full scoring result — only present on JobResult / selectedJobResult.
  const resultScore = (result) => result?.score ?? null;
  const formatSeconds = (value) => {
    const numeric = Number(value);
    if (!Number.isFinite(numeric)) return "0.00";
    return numeric.toFixed(2);
  };
  const formatTimingMs = (value) => {
    const numeric = Number(value);
    if (!Number.isFinite(numeric)) return "0";
    return Math.round(numeric).toString();
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
        modules.set(key, { key, name, entries: [], testcases: new Map(), ungrouped: [], counts: {} });
      }
      const mod = modules.get(key);
      mod.entries.push(entry);
      const level = normalizeLevel(entry.level);
      mod.counts[level] = (mod.counts[level] || 0) + 1;
      const tc = (entry.testcase && entry.testcase !== "Unspecified") ? entry.testcase : "";
      if (tc) {
        if (!mod.testcases.has(tc)) mod.testcases.set(tc, { tc, entries: [] });
        mod.testcases.get(tc).entries.push(entry);
      } else {
        mod.ungrouped.push(entry);
      }
    });
    for (const mod of modules.values()) {
      for (const tcg of mod.testcases.values()) tcg.level = worstLevel(tcg.entries);
      mod.testcasesArr = Array.from(mod.testcases.values());
    }
    const arr = Array.from(modules.values());
    arr.sort((a, b) => {
      if (a.key === "SYSTEM") return -1;
      if (b.key === "SYSTEM") return 1;
      return 0;
    });
    return arr;
  };
  const toggleModule = (key) => {
    moduleOpen = { ...moduleOpen, [key]: !moduleOpen[key] };
  };

  const toggleDomainModule = (key) => {
    domainModuleOpen = { ...domainModuleOpen, [key]: !domainModuleOpen[key] };
  };

  const okTestcaseCount = (group) =>
    group.testcasesArr.filter(tcg => !moduleLevels.includes(normalizeLevel(tcg.level))).length;

  const normalizeTab = (value) => {
    const tab = String(value || "").replace(/^\/+/, "").toLowerCase();
    if (tab === "single" || tab === "job" || tab === "jobs" || tab === "home") return "single";
    if (tab === "recent" || tab === "tests") return "recent";
    if (tab === "domains" || tab === "domain") return "domains";
    if (tab === "tags" || tab === "tag") return "tags";
    if (tab === "batches" || tab === "batch") return "batches";
    if (tab === "metrics" || tab === "metric") return "metrics";
    if (tab === "settings" || tab === "setting") return "settings";
    return "";
  };

  const setTab = (tab, { replace = false } = {}) => {
    const next = normalizeTab(tab) || "single";
    const changed = activeTab !== next;
    activeTab = next;
    const nextHash = `#/${next}`;
    const url = `${window.location.pathname}${window.location.search}${nextHash}`;
    const state = { tab: next, domain: null, tag: null, jobId: null };
    if (!changed || replace) {
      if (window.location.hash !== nextHash) window.history.replaceState(state, "", url);
    } else {
      window.history.pushState(state, "", url);
    }
    if (changed && statusMessage) {
      clearStatus();
    }
    if (next === "single" || next === "tags" || next === "batches") {
      loadProfiles();
    }
    if (next === "recent") {
      loadJobs();
    } else if (next === "domains") {
      loadDomains();
      if (!tagsLoaded) loadDomainTags();
    } else if (next === "tags") {
      loadTagsList();
      if (selectedTag) {
        loadTagDomains({ reset: true });
        loadTagSummary();
      }
    } else if (next === "batches") {
      loadRecentBatchOptions();
      loadActiveBatches();
      fetchQueueStatus();
      if (!tagsLoaded) loadDomainTags();
      if (selectedBatchId) {
        loadBatch(selectedBatchId);
      }
    } else if (next === "metrics") {
      loadMetrics();
    }
  };

  const navigateToJob = (jobId) => {
    selectedJobId = jobId;
    activeTab = "single";
    const hash = `#/single/${encodeURIComponent(jobId)}`;
    window.history.pushState({ tab: "single", domain: null, tag: null, jobId }, "", `${window.location.pathname}${window.location.search}${hash}`);
    if (statusMessage) clearStatus();
    loadJob(jobId);
  };

  const navigateToDomainDetail = (d) => {
    activeTab = "domains";
    selectedDomain = d;
    domainRuns = [];
    domainRunsOffset = 0;
    selectedDomainRunResult = null;
    selectedDomainRunId = null;
    const hash = `#/domains/${encodeURIComponent(d.name)}`;
    window.history.pushState({ tab: "domains", domain: d, tag: null, jobId: null }, "", `${window.location.pathname}${window.location.search}${hash}`);
    if (statusMessage) clearStatus();
    loadDomainRuns();
  };

  const navigateToTagDetail = (tag) => {
    activeTab = "tags";
    selectedTag = tag;
    tagProfileDraftId = tag?.default_profile_id ? String(tag.default_profile_id) : "";
    tagSummary = null;
    tagDomains = [];
    tagDomainsOffset = 0;
    tagDomainLevelFilter = "";
    tagDeleteConfirm = false;
    const hash = `#/tags/${encodeURIComponent(tag.name)}`;
    window.history.pushState({ tab: "tags", domain: null, tag, jobId: null }, "", `${window.location.pathname}${window.location.search}${hash}`);
    if (statusMessage) clearStatus();
    loadTagSummary();
    loadTagDomains();
  };

  // Guard flag: when popstate fires, a hashchange event also fires for the
  // same navigation.  updateTabFromHash must skip that duplicate because
  // onPopState already restored the full state (domain, tag, jobId) from
  // history — updateTabFromHash would clobber it with nulls.
  let popStateHandled = false;

  const onPopState = (e) => {
    popStateHandled = true;
    const state = e.state;
    if (!state) { updateTabFromHash(); return; }
    activeTab = state.tab || "single";
    selectedDomain = state.domain ?? null;
    selectedTag = state.tag ?? null;
    tagProfileDraftId = selectedTag?.default_profile_id ? String(selectedTag.default_profile_id) : "";
    if (state.jobId) {
      selectedJobId = state.jobId;
      loadJob(state.jobId);
    }
    if (!selectedDomain) {
      domainRuns = [];
      selectedDomainRunResult = null;
      selectedDomainRunId = null;
    } else {
      loadDomainRuns();
    }
    if (!selectedTag) {
      tagDeleteConfirm = false;
    } else {
      loadTagSummary();
      loadTagDomains({ reset: true });
    }
  };

  const updateTabFromHash = () => {
    if (popStateHandled) {
      popStateHandled = false;
      return;
    }
    const hash = window.location.hash || "";
    const parts = hash.replace(/^#\/?/, "").split("/");
    const segment = parts[0];
    const next = normalizeTab(segment) || "single";
    activeTab = next;
    const jobId = (next === "single" && parts[1]) ? decodeURIComponent(parts[1]) : null;
    if (jobId) {
      selectedJobId = jobId;
      loadJob(jobId);
    }
    window.history.replaceState(
      { tab: next, domain: null, tag: null, jobId: jobId || undefined },
      "",
      `${window.location.pathname}${window.location.search}${hash || `#/${next}`}`
    );
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
      setStatus($t("error_load_jobs", { error: error.message }), "warn");
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
      setStatus($t("error_domain_required"), "warn");
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
      const parsedTags = singleTags.split(/[,\s]+/).map((t) => t.trim()).filter(Boolean);
      const selectedProfileID = normalizeOptionalProfileID(singleProfileId);
      const payload = {
        domain: normalizedDomain,
        ...(selectedProfileID && { profile_id: selectedProfileID }),
        ...(parsedTags.length > 0 && { tags: parsedTags })
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
      notifyOnJobComplete = true;
      ensureNotificationPermission();
      jobInspectorHighlight = true;
      if (jobInspectorHighlightTimer) {
        clearTimeout(jobInspectorHighlightTimer);
      }
      jobInspectorHighlightTimer = setTimeout(() => {
        jobInspectorHighlight = false;
        jobInspectorHighlightTimer = null;
      }, 6000);
      setStatus($t("job_created", { id: job.id }), "ok");
      recentCursor = 0;
      await loadJobs({ resetCursor: true });
      await loadJob(job.id);
    } catch (error) {
      setStatus($t("error_create_job", { error: error.message }), "warn");
    } finally {
      singleSubmitting = false;
    }
  };

  const submitBatch = async () => {
    let payload;
    if (batchFromTagMode) {
      if (!batchFromTag) {
        setStatus($t("error_batch_from_tag_required"), "warn");
        return;
      }
      payload = { from_tag: batchFromTag };
    } else {
      const domains = batchDomains
        .split(/\n/)
        .map((entry) => normalizeDomainInput(entry))
        .filter(Boolean);
      if (!domains.length) {
        setStatus($t("error_batch_empty"), "warn");
        return;
      }
      payload = { domains };
    }
    const parsedTags = batchTags.split(/[,\s]+/).map((t) => t.trim()).filter(Boolean);
    const selectedProfileID = normalizeOptionalProfileID(batchProfileId);
    if (parsedTags.length > 0) payload.tags = parsedTags;
    if (selectedProfileID) payload.profile_id = selectedProfileID;
    batchSubmitting = true;
    createdBatchId = "";
    try {
      const response = await apiFetch("/jobs/batch", {
        method: "POST",
        body: JSON.stringify(payload)
      });
      createdBatchId = response.batch_id;
      selectedBatchId = response.batch_id;
      autoRefreshBatch = true;
      notifyOnBatchComplete = true;
      ensureNotificationPermission();
      setStatus($t("batch_accepted", { id: response.batch_id }), "ok");
      recentCursor = 0;
      await loadJobs({ resetCursor: true });
      await loadRecentBatchOptions();
      await loadBatch(response.batch_id, { resetCursor: true });
    } catch (error) {
      setStatus($t("error_create_batch", { error: error.message }), "warn");
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
      selectedRun = null;
      if (notifyOnJobComplete && isResultReadyStatus(job.status)) {
        notifyOnJobComplete = false;
        sendJobNotification(job);
      }
      if (isResultReadyStatus(job.status)) {
        await loadJobResult(jobId);
        await loadRun(jobId);
      }
    } catch (error) {
      setStatus($t("error_load_job", { error: error.message }), "warn");
      selectedJob = null;
      selectedJobResult = null;
      selectedRun = null;
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
      setStatus($t("error_load_result", { error: error.message }), "warn");
    }
  };

  const loadRun = async (jobId = selectedJobId) => {
    if (!jobId) return;
    try {
      selectedRun = await apiFetch(`/runs/${jobId}`);
    } catch (_) {
      selectedRun = null;
    }
  };

  const navigateToDomainByName = async (name) => {
    try {
      const data = await apiFetch(`/domains?name=${encodeURIComponent(name)}&limit=20`);
      const match = (data?.items ?? []).find((d) => d.name === name);
      if (!match) return;
      navigateToDomainDetail(match);
    } catch (_) {}
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
      setStatus($t("error_load_batches", { error: error.message }), "warn");
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
      // Propagate tag into recentBatchOptions so the dropdown can show it.
      if (batch?.tag) {
        const idx = recentBatchOptions.findIndex((o) => o.id === batchId);
        if (idx >= 0 && !recentBatchOptions[idx].tag) {
          recentBatchOptions = recentBatchOptions.map((o, i) => i === idx ? { ...o, tag: batch.tag } : o);
        }
      }
      if (notifyOnBatchComplete && !hasActiveBatchJobs(batch)) {
        notifyOnBatchComplete = false;
        sendBatchNotification(batch);
      }
      if (autoRefreshBatch && !hasActiveBatchJobs(batch)) {
        autoRefreshBatch = false;
      }
    } catch (error) {
      setStatus($t("error_load_batch", { error: error.message }), "warn");
      selectedBatch = null;
    } finally {
      batchLoading = false;
    }
  };

  const loadActiveBatches = async () => {
    activeBatchesLoading = true;
    try {
      // Discover recent batch IDs from the job list (same approach as loadRecentBatchOptions).
      const batchIds = [];
      const seen = new Set();
      let cursor = 0;
      let pages = 0;
      while (batchIds.length < 10 && pages < 3) {
        const params = new URLSearchParams({ limit: "100", sort: "created_at_desc" });
        if (cursor > 0) params.set("cursor", String(cursor));
        const list = await apiFetch(`/jobs?${params.toString()}`);
        const items = list?.items || [];
        for (const item of items) {
          const bid = String(item?.batch_id || "").trim();
          if (!bid || seen.has(bid)) continue;
          seen.add(bid);
          batchIds.push(bid);
          if (batchIds.length >= 10) break;
        }
        if (!list?.next_cursor) break;
        const next = normalizeCursor(list.next_cursor);
        if (next <= cursor) break;
        cursor = next;
        pages++;
      }

      // Fetch summary for each batch and keep only active ones.
      const summaries = await Promise.all(
        batchIds.map(async (id) => {
          try {
            return await apiFetch(`/batches/${id}?limit=1&sort=started_at_desc`);
          } catch (_) {
            return null;
          }
        })
      );
      activeBatches = summaries.filter((b) => b && hasActiveBatchJobs(b));
    } catch (_) {
      // Silently ignore — active batches is supplementary.
    } finally {
      activeBatchesLoading = false;
    }
  };

  const fetchQueueStatus = async () => {
    try {
      const snapshot = await apiFetch("/metrics?window=1h&include=health");
      queuePaused = !!snapshot?.health?.queue_paused;
    } catch (_) {
      // Non-critical — silently ignore.
    }
  };

  const toggleQueuePause = async () => {
    if (queuePauseToggling) return;
    queuePauseToggling = true;
    try {
      const endpoint = queuePaused ? "/queue/resume" : "/queue/pause";
      await apiFetch(endpoint, { method: "POST" });
      queuePaused = !queuePaused;
      setStatus($t(queuePaused ? "queue_paused_status" : "queue_resumed_status"), "ok");
    } catch (error) {
      setStatus($t("error_queue_toggle", { error: error.message }), "warn");
    } finally {
      queuePauseToggling = false;
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
        setStatus($t("metrics_load_error", { error: metricsError }), "warn");
      }
    } finally {
      if (!silent) {
        metricsLoading = false;
      }
    }
  };

  const loadDomains = async (options = {}) => {
    const { reset = false } = options;
    if (reset) domainsOffset = 0;
    domainsLoading = true;
    try {
      const params = new URLSearchParams({ limit: String(domainsLimit), offset: String(domainsOffset) });
      if (domainNameFilter) params.set("name", domainNameFilter);
      if (domainTagFilter) params.set("tag", domainTagFilter);
      if (domainLevelFilter) params.set("min_level", domainLevelFilter);
      const sortVal = domainSortParam(domainsSortState);
      if (sortVal) params.set("sort", sortVal);
      const data = await apiFetch(`/domains?${params}`);
      domains = data?.items ?? [];
      domainsTotal = data?.total ?? 0;
    } catch (error) {
      setStatus($t("domains_load_error", { error: error.message || "unknown error" }), "warn");
    } finally {
      domainsLoading = false;
    }
  };

  const loadDomainTags = async () => {
    try {
      const data = await apiFetch("/tags");
      availableTags = Array.isArray(data) ? data : [];
      tagsLoaded = true;
    } catch (_) {
      availableTags = [];
      tagsLoaded = true;
    }
  };

  const loadProfiles = async () => {
    profilesLoading = true;
    try {
      const data = await apiFetch("/profiles");
      availableProfiles = Array.isArray(data) ? data : [];
      profilesLoaded = true;
    } catch (error) {
      availableProfiles = [];
      profilesLoaded = false;
      setStatus($t("profile_load_error", { error: error.message || "unknown error" }), "warn");
    } finally {
      profilesLoading = false;
    }
  };

  const handleProfilesChanged = async (detail) => {
    const nextProfiles = detail?.profiles;
    if (Array.isArray(nextProfiles)) {
      availableProfiles = nextProfiles;
      profilesLoaded = true;
      return;
    }
    await loadProfiles();
  };

  const loadDomainRuns = async (options = {}) => {
    if (!selectedDomain) return;
    const { reset = false } = options;
    if (reset) domainRunsOffset = 0;
    domainRunsLoading = true;
    try {
      const params = new URLSearchParams({ limit: String(domainRunsLimit), offset: String(domainRunsOffset) });
      const sortVal = runSortParam(domainRunsSortState);
      if (sortVal) params.set("sort", sortVal);
      const data = await apiFetch(`/domains/${selectedDomain.id}/runs?${params}`);
      domainRuns = data?.items ?? [];
      domainRunsTotal = data?.total ?? 0;
      if (domainRuns.length > 0 && domainRunsOffset === 0) {
        loadDomainRunResult(domainRuns[0].id);
      }
    } catch (error) {
      setStatus($t("domain_runs_load_error", { error: error.message || "unknown error" }), "warn");
    } finally {
      domainRunsLoading = false;
    }
  };

  const loadDomainRunResult = async (runId) => {
    selectedDomainRunId = runId;
    selectedDomainRunResult = null;
    domainModuleOpen = {};
    domainRunResultLoading = true;
    try {
      const locale = resultLocale ? `?locale=${encodeURIComponent(resultLocale)}` : "";
      selectedDomainRunResult = await apiFetch(`/jobs/${runId}/result${locale}`);
    } catch (_) {
      // result not available for this run
    } finally {
      domainRunResultLoading = false;
    }
  };

  const retestDomain = async () => {
    if (!selectedDomain) return;
    domainResubmitting = true;
    try {
      const job = await apiFetch("/jobs", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ domain: selectedDomain.name })
      });
      createdJobId = job.id;
      setStatus($t("job_created", { id: job.id }), "ok");
      navigateToJob(job.id);
    } catch (error) {
      setStatus($t("error_create_job", { error: error.message }), "warn");
    } finally {
      domainResubmitting = false;
    }
  };

  const loadTagsList = async () => {
    tagsListLoading = true;
    try {
      const data = await apiFetch("/tags");
      tagsList = Array.isArray(data) ? data : [];
    } catch (error) {
      setStatus($t("tags_load_error", { error: error.message || "unknown error" }), "warn");
    } finally {
      tagsListLoading = false;
    }
  };

  const syncTagProfileState = (tagName, profileID) => {
    const nextValue = normalizeOptionalProfileID(profileID);
    if (selectedTag?.name === tagName) {
      selectedTag = { ...selectedTag, default_profile_id: nextValue };
    }
    tagsList = tagsList.map((tag) =>
      tag.name === tagName ? { ...tag, default_profile_id: nextValue } : tag
    );
    availableTags = availableTags.map((tag) =>
      tag.name === tagName ? { ...tag, default_profile_id: nextValue } : tag
    );
    tagProfileDraftId = nextValue ? String(nextValue) : "";
  };

  const createTag = async () => {
    const name = tagCreateName.trim();
    if (!name) return;
    tagCreating = true;
    try {
      await apiFetch("/tags", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name, description: tagCreateDescription.trim() })
      });
      tagCreateName = "";
      tagCreateDescription = "";
      setStatus($t("tag_created"), "ok");
      await loadTagsList();
      await loadDomainTags();
    } catch (error) {
      setStatus($t("tag_create_error", { error: error.message || "unknown error" }), "warn");
    } finally {
      tagCreating = false;
    }
  };

  const loadTagSummary = async () => {
    if (!selectedTag) return;
    tagSummaryLoading = true;
    try {
      tagSummary = await apiFetch(`/tags/${encodeURIComponent(selectedTag.name)}/summary`);
    } catch (_) {
      tagSummary = null;
    } finally {
      tagSummaryLoading = false;
    }
  };

  const loadTagDomains = async (options = {}) => {
    if (!selectedTag) return;
    const { reset = false } = options;
    if (reset) tagDomainsOffset = 0;
    tagDomainsLoading = true;
    try {
      const params = new URLSearchParams({ limit: String(tagDomainsLimit), offset: String(tagDomainsOffset) });
      if (tagDomainLevelFilter) params.set("min_level", tagDomainLevelFilter);
      const sortVal = domainSortParam(tagDomainsSortState);
      if (sortVal) params.set("sort", sortVal);
      const data = await apiFetch(`/tags/${encodeURIComponent(selectedTag.name)}/domains?${params}`);
      tagDomains = data?.items ?? [];
      tagDomainsTotal = data?.total ?? 0;
    } catch (error) {
      setStatus($t("tag_domains_load_error", { error: error.message || "unknown error" }), "warn");
    } finally {
      tagDomainsLoading = false;
    }
  };

  const runAllFromTag = async () => {
    if (!selectedTag) return;
    tagRunAllSubmitting = true;
    try {
      const response = await apiFetch("/jobs/batch", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ from_tag: selectedTag.name })
      });
      createdBatchId = response.batch_id || "";
      selectedBatchId = response.batch_id || "";
      setStatus($t("batch_accepted", { id: response.batch_id }), "ok");
      await loadRecentBatchOptions();
      await loadBatch(response.batch_id, { resetCursor: true });
      setTab("batches");
    } catch (error) {
      setStatus($t("tag_run_all_error", { error: error.message || "unknown error" }), "warn");
    } finally {
      tagRunAllSubmitting = false;
    }
  };

  const saveTagProfile = async () => {
    if (!selectedTag || !tagProfileSelectedID || !tagProfileDirty) return;
    tagProfileUpdating = true;
    try {
      await apiFetch(`/tags/${encodeURIComponent(selectedTag.name)}/profile`, {
        method: "PUT",
        body: JSON.stringify({ profile_id: tagProfileSelectedID })
      });
      syncTagProfileState(selectedTag.name, tagProfileSelectedID);
      setStatus($t("tag_profile_saved", { name: profileNameByID(tagProfileSelectedID) }), "ok");
      await loadTagsList();
      await loadDomainTags();
    } catch (error) {
      setStatus($t("tag_profile_save_error", { error: error.message || "unknown error" }), "warn");
    } finally {
      tagProfileUpdating = false;
    }
  };

  const clearTagProfile = async () => {
    if (!selectedTag || !tagProfileCurrentID) return;
    tagProfileClearing = true;
    try {
      await apiFetch(`/tags/${encodeURIComponent(selectedTag.name)}/profile`, {
        method: "DELETE"
      });
      syncTagProfileState(selectedTag.name, null);
      setStatus($t("tag_profile_cleared"), "ok");
      await loadTagsList();
      await loadDomainTags();
    } catch (error) {
      setStatus($t("tag_profile_clear_error", { error: error.message || "unknown error" }), "warn");
    } finally {
      tagProfileClearing = false;
    }
  };

  const addTagDomains = async () => {
    if (!selectedTag || !tagAddDomainsInput.trim()) return;
    const domains = tagAddDomainsInput.split(/[\n,]+/).map((s) => s.trim()).filter(Boolean);
    if (domains.length === 0) return;
    tagAddingDomains = true;
    try {
      await apiFetch(`/tags/${encodeURIComponent(selectedTag.name)}/domains`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ domains })
      });
      tagAddDomainsInput = "";
      setStatus($t("tag_domains_added"), "ok");
      await loadTagDomains({ reset: true });
      await loadTagSummary();
    } catch (error) {
      setStatus($t("tag_domains_add_error", { error: error.message || "unknown error" }), "warn");
    } finally {
      tagAddingDomains = false;
    }
  };

  const removeTagDomains = async () => {
    if (!selectedTag || !tagRemoveDomainsInput.trim()) return;
    const domains = tagRemoveDomainsInput.split(/[\n,]+/).map((s) => s.trim()).filter(Boolean);
    if (domains.length === 0) return;
    tagRemovingDomains = true;
    try {
      await apiFetch(`/tags/${encodeURIComponent(selectedTag.name)}/domains`, {
        method: "DELETE",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ domains })
      });
      tagRemoveDomainsInput = "";
      setStatus($t("tag_domains_removed"), "ok");
      await loadTagDomains({ reset: true });
      await loadTagSummary();
    } catch (error) {
      setStatus($t("tag_domains_remove_error", { error: error.message || "unknown error" }), "warn");
    } finally {
      tagRemovingDomains = false;
    }
  };

  const deleteTag = async () => {
    if (!selectedTag) return;
    tagDeleting = true;
    try {
      await apiFetch(`/tags/${encodeURIComponent(selectedTag.name)}`, { method: "DELETE" });
      setStatus($t("tag_deleted"), "ok");
      selectedTag = null;
      tagProfileDraftId = "";
      tagDeleteConfirm = false;
      await loadTagsList();
      await loadDomainTags();
    } catch (error) {
      setStatus($t("tag_delete_error", { error: error.message || "unknown error" }), "warn");
    } finally {
      tagDeleting = false;
    }
  };

  const loadFeatures = async () => {
    try {
      const data = await apiFetch("/features");
      if (data && typeof data.show_score_admin === "boolean") {
        scoringEnabled = data.show_score_admin;
      }
    } catch (_) {
      // Keep scoringEnabled = true on failure (fail-open for admin UI).
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
        locale.set(resultLocale);
        loadCatalog(resultLocale);
      }
    } catch (_) {
      // Keep availableLocales as ["en"] default; locale select stays hidden.
    }
  };

  const onLocaleChange = () => {
    try { localStorage.setItem(localeKey, resultLocale); } catch (_) {}
    locale.set(resultLocale);
    loadCatalog(resultLocale);
    if (selectedJobResult) {
      loadJobResult(selectedJobId);
    }
    if (selectedDomainRunResult) {
      loadDomainRunResult(selectedDomainRunId);
    }
  };

  const ensureNotificationPermission = () => {
    if (typeof Notification === "undefined") return Promise.resolve("denied");
    const perm = Notification.permission;
    if (perm === "granted" || perm === "denied") return Promise.resolve(perm);
    if (!pendingPermission) {
      pendingPermission = Notification.requestPermission().then((result) => {
        pendingPermission = null;
        return result;
      });
    }
    return pendingPermission;
  };

  const sendJobNotification = async (job) => {
    const permission = await ensureNotificationPermission();
    console.debug("[notify] job done – permission=%s domain=%s status=%s", permission, job.domain, job.status);
    if (permission !== "granted") return;
    const title = $t("notify_job_done_title");
    const body = $t("notify_job_done_body", { domain: job.domain, status: job.status });
    try {
      new Notification(title, { body });
    } catch (err) {
      console.warn("[notify] Notification constructor failed:", err);
    }
  };

  const sendBatchNotification = async (batch) => {
    const permission = await ensureNotificationPermission();
    console.debug("[notify] batch done – permission=%s batch_id=%s", permission, batch.batch_id);
    if (permission !== "granted") return;
    const title = $t("notify_batch_done_title");
    const body = $t("notify_batch_done_body", { id: batch.batch_id });
    try {
      new Notification(title, { body });
    } catch (err) {
      console.warn("[notify] Notification constructor failed:", err);
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

  const startActiveBatchesPolling = () => {
    if (activeBatchesPoller) clearInterval(activeBatchesPoller);
    if (activeTab !== "batches") return;
    activeBatchesPoller = setInterval(() => { loadActiveBatches(); fetchQueueStatus(); }, 7000);
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
    activeTab;
    startActiveBatchesPolling();
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

  $: {
    domainModuleGroups = groupRawEntries(selectedDomainRunResult?.raw);
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

  $: sortedDomainRuns = sortItems(domainRuns, domainRunsSortState, {
    id: (left, right) => compareText(left?.id, right?.id),
    finished_at: (left, right) => compareTimestamp(left?.finished_at, right?.finished_at),
    worst_level: (left, right) => compareSeverity(left?.worst_level, right?.worst_level),
    score: (left, right) => compareNumber(chipScore(left), chipScore(right)),
    duration_ms: (left, right) => compareNumber(left?.duration_ms, right?.duration_ms),
    entry_count: (left, right) => compareNumber(left?.entry_count, right?.entry_count)
  }, (left, right) => compareText(left?.id, right?.id));

  $: sortedDomains = sortItems(domains, domainsSortState, {
    name: (left, right) => compareText(left?.name, right?.name),
    tags: (left, right) => compareText(left?.tags?.join(", "), right?.tags?.join(", ")),
    latest_level: (left, right) => compareSeverity(domainLevel(left), domainLevel(right)),
    latest_score: (left, right) => compareNumber(left?.latest_score, right?.latest_score),
    latest_run_at: (left, right) => compareTimestamp(left?.latest_run_at, right?.latest_run_at),
    run_count: (left, right) => compareNumber(left?.run_count, right?.run_count)
  }, (left, right) => compareText(left?.name, right?.name));

  $: sortedTagDomains = sortItems(tagDomains, tagDomainsSortState, {
    name: (left, right) => compareText(left?.name, right?.name),
    latest_level: (left, right) => compareSeverity(domainLevel(left), domainLevel(right)),
    latest_score: (left, right) => compareNumber(left?.latest_score, right?.latest_score),
    latest_run_at: (left, right) => compareTimestamp(left?.latest_run_at, right?.latest_run_at)
  }, (left, right) => compareText(left?.name, right?.name));

  $: sortedTagsList = sortItems(tagsList, tagsListSortState, {
    name: (left, right) => compareText(left?.name, right?.name),
    description: (left, right) => compareText(left?.description, right?.description),
    domain_count: (left, right) => compareNumber(left?.domain_count, right?.domain_count)
  }, (left, right) => compareText(left?.name, right?.name));

  $: sortedMetricsDomainRows = sortItems(metricsDomainRows(metricsSnapshot), metricsDomainsSortState, {
    domain: (left, right) => compareText(left?.domain, right?.domain),
    runs_total: (left, right) => compareNumber(left?.runs_total, right?.runs_total),
    last_status: (left, right) => compareText(left?.last_status, right?.last_status),
    avg_duration_ms: (left, right) => compareNumber(left?.avg_duration_ms, right?.avg_duration_ms),
    error_critical: (left, right) =>
      compareNumber(
        Number(left?.severity_totals?.ERROR || 0) + Number(left?.severity_totals?.CRITICAL || 0),
        Number(right?.severity_totals?.ERROR || 0) + Number(right?.severity_totals?.CRITICAL || 0)
      )
  }, (left, right) => compareText(left?.domain, right?.domain));

  $: sortedMetricsBatchRows = sortItems(metricsBatchRows(metricsSnapshot), metricsBatchesSortState, {
    batch_id: (left, right) => compareText(left?.batch_id, right?.batch_id),
    processed_total: (left, right) => compareNumber(left?.processed_total, right?.processed_total),
    failed_expired: (left, right) => compareNumber(batchErrorScore(left), batchErrorScore(right)),
    canceled: (left, right) => compareNumber(left?.outcomes?.canceled, right?.outcomes?.canceled),
    error_critical: (left, right) =>
      compareNumber(
        Number(left?.severity_totals?.ERROR || 0) + Number(left?.severity_totals?.CRITICAL || 0),
        Number(right?.severity_totals?.ERROR || 0) + Number(right?.severity_totals?.CRITICAL || 0)
      )
  }, (left, right) => {
    const processedDelta = compareNumber(left?.processed_total, right?.processed_total);
    if (processedDelta !== 0) return -processedDelta;
    return compareText(left?.batch_id, right?.batch_id);
  });

  $: tagProfileCurrentID = normalizeOptionalProfileID(selectedTag?.default_profile_id);
  $: tagProfileSelectedID = normalizeOptionalProfileID(tagProfileDraftId);
  $: tagProfileDirty = tagProfileCurrentID !== tagProfileSelectedID;

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
    if (storedTheme === "light" || storedTheme === "dark") {
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
    locale.set(resultLocale);
    loadCatalog(resultLocale);
    loadLocales();
    loadProfiles();
    loadFeatures();

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
    window.addEventListener("popstate", onPopState);
    loadJobs();
    if (activeTab === "batches") {
      loadRecentBatchOptions();
      loadActiveBatches();
      fetchQueueStatus();
      if (!tagsLoaded) loadDomainTags();
      if (selectedBatchId) {
        loadBatch(selectedBatchId);
      }
    }
    if (activeTab === "tags") {
      loadTagsList();
    }
    if (activeTab === "domains") {
      loadDomains();
      if (!tagsLoaded) loadDomainTags();
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
      if (activeBatchesPoller) clearInterval(activeBatchesPoller);
      if (recentPoller) clearInterval(recentPoller);
      if (metricsPoller) clearInterval(metricsPoller);
      if (jobInspectorHighlightTimer) clearTimeout(jobInspectorHighlightTimer);
      if (statusDismissTimer) clearTimeout(statusDismissTimer);
      window.removeEventListener("hashchange", updateTabFromHash);
      window.removeEventListener("popstate", onPopState);
    };
  });
</script>

<div class="app-header">
  <header class="reveal" style="--d: 0.05s">
    <div class="header-text">
      <h1 class="brand-mark">
        <img class="brand-logo" src={logoSrc} alt="gonemaster" />
      </h1>
    </div>
    <div class="header-controls">
      {#if availableLocales.length > 1}
        <select
          bind:value={resultLocale}
          onchange={onLocaleChange}
          id="locale-select"
          class="locale-select"
          title={$t("locale_select_title")}
          aria-label={$t("locale_select_aria")}
        >
          {#each availableLocales as code}
            <option value={code}>{localeLabel(code)}</option>
          {/each}
        </select>
      {/if}
      <button class="theme-toggle" type="button" onclick={toggleTheme} title={themeTitle} aria-label={themeTitle}>
        {themeIcon}
      </button>
    </div>
  </header>
</div>

<div class="mobile-nav" role="tablist" aria-label={$t("tabs_aria_label")}>
  {#each tabs as tab}
    <button
      class={`nav-item ${activeTab === tab.id ? "active" : ""}`}
      type="button"
      role="tab"
      id={`tab-${tab.id}`}
      aria-selected={activeTab === tab.id}
      aria-controls={`panel-${tab.id}`}
      onclick={() => setTab(tab.id)}
    >
      {$t(tab.labelKey)}
    </button>
  {/each}
</div>

<div class="app-layout">
  <nav class="sidebar" aria-label={$t("tabs_aria_label")}>
    <div class="nav-items">
      {#each tabs.filter(t => t.id !== "settings") as tab}
        <button
          class={`nav-item ${activeTab === tab.id ? "active" : ""}`}
          type="button"
          aria-current={activeTab === tab.id ? "page" : undefined}
          onclick={() => setTab(tab.id)}
        >
          {$t(tab.labelKey)}
        </button>
      {/each}
    </div>
    <div class="nav-bottom">
      <hr class="nav-divider" />
      <button
        class={`nav-item ${activeTab === "settings" ? "active" : ""}`}
        type="button"
        aria-current={activeTab === "settings" ? "page" : undefined}
        onclick={() => setTab("settings")}
      >
        {$t("tab_settings")}
      </button>
    </div>
  </nav>

  <main>
  {#if activeTab === "single"}
    <div class="grid" id="panel-single" role="tabpanel" aria-labelledby="tab-single" style="margin-top: 22px;">
      <div class="card reveal" style="--d: 0.18s">
        <h2>{$t("single_job_heading")}</h2>
        <div class="stack">
          <label for="single-domain">{$t("single_domain_label")}</label>
          <input
            id="single-domain"
            type="text"
            placeholder="example.com"
            bind:value={singleDomain}
            onkeydown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault();
                submitSingle();
              }
            }}
          />
        </div>
        <div class="stack">
          <label for="single-tags">{$t("single_tags_label")}</label>
          <input
            id="single-tags"
            type="text"
            placeholder={$t("single_tags_placeholder")}
            bind:value={singleTags}
          />
          <div class="small">{$t("single_tags_hint")}</div>
        </div>
        <div class="stack">
          <label for="single-profile">{$t("stored_profile_label")}</label>
          <select id="single-profile" bind:value={singleProfileId} disabled={profilesLoading && availableProfiles.length === 0}>
            <option value="">{$t("stored_profile_auto_option")}</option>
            {#each availableProfiles as profile}
              <option value={profile.id}>{profile.name}</option>
            {/each}
          </select>
          <div class="small">{$t("stored_profile_hint")}</div>
        </div>
        <details class="advanced-options">
          <summary>{$t("advanced_profile_summary")}</summary>
          <div class="stack advanced-stack">
            <label for="single-ip-mode">{$t("ip_transport_label")}</label>
            <select id="single-ip-mode" bind:value={singleIPMode}>
              <option value="default">{$t("ip_mode_default")}</option>
              <option value="disable_ipv4">{$t("ip_mode_disable_ipv4")}</option>
              <option value="disable_ipv6">{$t("ip_mode_disable_ipv6")}</option>
            </select>
            <div class="small">{$t("ip_mode_hint")}</div>
          </div>
        </details>
        <details class="advanced-options">
          <summary>{$t("undelegated_summary")}</summary>
          <div class="stack advanced-stack">
            <div class="field-label">{$t("ns_field_label")}</div>
            {#if undelegatedNameservers.length === 0}
              <div class="small">{$t("ns_none")}</div>
            {:else}
              <div class="undelegated-list">
                {#each undelegatedNameservers as row, index (row.id)}
                  <div class="undelegated-row">
                    <input
                      type="text"
                      aria-label={$t("ns_aria_label", { n: index + 1 })}
                      placeholder="ns1.example.com"
                      bind:value={row.ns}
                    />
                    <input
                      type="text"
                      aria-label={$t("ns_ip_aria_label", { n: index + 1 })}
                      placeholder="192.0.2.10 or 2001:db8::10"
                      bind:value={row.ip}
                    />
                    <button class="ghost mini-button" type="button" onclick={() => removeUndelegatedNameserverRow(row.id)}>
                      {$t("remove")}
                    </button>
                  </div>
                {/each}
              </div>
            {/if}
            <button class="ghost" type="button" onclick={addUndelegatedNameserverRow}>
              {$t("add_nameserver")}
            </button>

            <div class="field-label">{$t("ds_field_label")}</div>
            {#if undelegatedDSInfo.length === 0}
              <div class="small">{$t("ds_none")}</div>
            {:else}
              <div class="undelegated-list">
                {#each undelegatedDSInfo as row, index (row.id)}
                  <div class="undelegated-ds-row">
                    <input type="text" aria-label={$t("ds_keytag_aria_label", { n: index + 1 })} placeholder="12345" bind:value={row.keytag} />
                    <input type="text" aria-label={$t("ds_algorithm_aria_label", { n: index + 1 })} placeholder="13" bind:value={row.algorithm} />
                    <input type="text" aria-label={$t("ds_digtype_aria_label", { n: index + 1 })} placeholder="2" bind:value={row.digtype} />
                    <input
                      type="text"
                      aria-label={$t("ds_digest_aria_label", { n: index + 1 })}
                      placeholder="ABCD..."
                      bind:value={row.digest}
                    />
                    <button class="ghost mini-button" type="button" onclick={() => removeUndelegatedDSRow(row.id)}>
                      {$t("remove")}
                    </button>
                  </div>
                {/each}
              </div>
            {/if}
            <button class="ghost" type="button" onclick={addUndelegatedDSRow}>
              {$t("add_ds_record")}
            </button>
            <div class="small">{$t("undelegated_validation_hint")}</div>
          </div>
        </details>
        <button onclick={submitSingle} disabled={singleSubmitting}>
          {singleSubmitting ? $t("submitting") : $t("run_single_job")}
        </button>
        {#if createdJobId}
          <div class="small">{$t("created_job_prefix")} <span class="mono">{createdJobId}</span></div>
        {/if}
      </div>

      <div class="card reveal" style="--d: 0.26s" class:highlight={jobInspectorHighlight}>
        <h2>{selectedJob && isResultReadyStatus(selectedJob.status) ? $t("run_inspector_heading") : $t("job_inspector_heading")}</h2>
        <div class="stack">
          <label for="job-id">{$t("job_id_label")}</label>
          <input id="job-id" type="text" placeholder="job_123" bind:value={selectedJobId} onchange={() => loadJob()} />
        </div>
        <div class="row">
          <button onclick={() => loadJob()} disabled={jobLoading}>{jobLoading ? $t("loading") : $t("refresh")}</button>
          <button class="ghost" type="button" onclick={() => (autoRefreshJob = !autoRefreshJob)}>
            {autoRefreshJob ? $t("auto_refresh_on") : $t("auto_refresh_off")}
          </button>
        </div>
        {#if selectedJob}
          {@const selectedJobProfileName = jobProfileName(selectedJob, selectedRun)}
          <div class="kv">
            <span>{$t("status_label")}</span>
            <strong>{selectedJob.status} · {formatJobTotalRuntime(selectedJob)}</strong>
            <span>{$t("progress_label")}</span>
            <div class="progress" role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow={progressPercent(selectedJob)}>
              <div class="progress-bar" style={`width: ${progressPercent(selectedJob)}%`}></div>
              <span class="progress-value">{progressPercent(selectedJob)}%</span>
            </div>
            <span>{$t("domain_label")}</span>
            <button class="ghost" type="button" style="padding: 0; font-family: monospace; text-align: left; border: none;" onclick={() => navigateToDomainByName(selectedJob.domain)}>{selectedJob.domain}</button>
            {#if selectedJobProfileName}
              <span>{$t("job_profile_label")}</span>
              <strong class="mono">{selectedJobProfileName}</strong>
            {/if}
            <span>{$t("created_label")}</span>
            <strong>{formatTimestampLocal(selectedJob.created_at)}</strong>
            {#if selectedRun}
              <span>{$t("col_duration")}</span>
              <strong>{selectedRun.duration_ms != null ? selectedRun.duration_ms + " ms" : "—"}</strong>
              <span>{$t("col_entries")}</span>
              <strong>{selectedRun.entry_count ?? 0}</strong>
              <span>{$t("col_worst_level")}</span>
              <strong><span class="badge level-{(selectedRun.worst_level || '').toLowerCase()}">{selectedRun.worst_level || "—"}</span></strong>
              {#if scoringEnabled && hasScore(selectedRun)}
                {@const rs = resultScore(selectedJobResult)}
                <span>{$t("col_score")}</span>
                <strong>
                  <span class="grade-chip-wrap">
                    <span class="grade-chip">
                      <span class="grade-chip-letter" data-grade={chipGrade(selectedRun)}>{chipGrade(selectedRun)}</span>
                      <span class="grade-chip-score">{chipScore(selectedRun)}/100</span>
                    </span>
                    {#if rs}
                      <span class="grade-chip-tooltip">
                        {#each CAT_ORDER.filter(c => c in (rs.categories ?? {})) as cat}
                          <div class="grade-tip-row">
                            <span class="grade-tip-cat">{CAT_LABELS[cat]}</span>
                            <span class="grade-tip-score">{rs.categories[cat].tested === false ? "-" : rs.categories[cat].score}</span>
                          </div>
                        {/each}
                        {#if rs.bonus?.criteria}
                          {@const bonusCriteria = Object.entries(rs.bonus.criteria).filter(([k]) => !BONUS_HIDDEN.has(k))}
                          {#if bonusCriteria.length}
                            <hr class="grade-tip-divider">
                            <div class="grade-tip-bonus">
                              {#each bonusCriteria as [key, val]}
                                <div class="grade-tip-criterion">
                                  <span class="grade-tip-icon {val === true ? 'met' : val === false ? 'unmet' : ''}">{val === true ? '✓' : val === false ? '✗' : '–'}</span>
                                  <span>{$t(`pub.score_bonus_${key}`)}</span>
                                </div>
                              {/each}
                            </div>
                          {/if}
                        {/if}
                      </span>
                    {/if}
                  </span>
                </strong>
              {/if}
            {/if}
          </div>
          {#if selectedJob.error}
            <div class="notice">{$t("error_prefix")} {selectedJob.error}</div>
          {/if}
          {#if selectedRun?.effective_profile}
            <details class="advanced-options">
              <summary>{$t("run_effective_profile_summary")}</summary>
              <pre>{prettyProfileJSON(selectedRun.effective_profile)}</pre>
            </details>
          {/if}
        {/if}
        {#if selectedJobResult}
          {@const allEntries = selectedJobResult?.raw?.entries ?? []}
          {@const nsTimings = selectedJobResult?.nameserver_timings ?? []}
          {@const bannerCls = bannerClass(worstLevel(allEntries))}
          <div class="stack">
            {#if allEntries.length}
              <div class="status-banner {bannerCls}">{$t(`result_status_${bannerCls}`)}</div>
            {/if}
            <div class="field-label">{$t("result_summary_label")}</div>
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
              <div class="summary-empty">{$t("no_result_entries")}</div>
            {/if}
              <div class="field-label">{$t("result_details_label")}</div>
              {#if moduleGroups.length === 0}
                <div class="summary-empty">{$t("no_raw_entries")}</div>
            {:else}
              <div class="small">{$t("module_expand_hint")}</div>
              <div class="module-list">
                {#each moduleGroups as group (group.key)}
                  {@const groupLevel = worstLevel(group.entries)}
                  <div class="module-card" data-level={groupLevel.toLowerCase()}>
                    <button
                      class="module-toggle"
                      type="button"
                      aria-expanded={!!moduleOpen[group.key]}
                      aria-controls={moduleId(group.key)}
                      onclick={() => toggleModule(group.key)}
                    >
                      <div class="module-title">{group.name}</div>
                      <div class="module-meta">{group.testcasesArr.length > 0 ? `${group.testcasesArr.length} tests · ` : ""}{$t("entries_count", { count: group.entries.length })}</div>
                      <div class="module-badges">
                        {#if okTestcaseCount(group) > 0}
                          <span class="level-pill severity-info">INFO {okTestcaseCount(group)}</span>
                        {/if}
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
                        <!-- Test case sub-groups -->
                        {#each group.testcasesArr as tcg (tcg.tc)}
                          {@const tcKey = `tc.${tcg.tc.toLowerCase()}`}
                          {@const tcDesc = $t(tcKey)}
                          <details class="testcase-group" open={isNoticeOrAbove(tcg.level)}>
                            <summary class="testcase-summary">
                              <span class="testcase-chevron"></span>
                              <span class="testcase-desc">{tcDesc !== tcKey ? tcDesc : tcg.tc}</span>
                              <span class="testcase-badge">
                                <span class={`level-pill severity-${normalizeLevel(tcg.level).toLowerCase()}`}>{normalizeLevel(tcg.level)}</span>
                              </span>
                            </summary>
                            <div class="testcase-entries">
                              {#each tcg.entries as entry}
                                {@const level = normalizeLevel(entry.level)}
                                <div class="result-row tc-row">
                                  <span class={`entry-level severity-${level.toLowerCase()}`}>{level}</span>
                                  <span class="entry-message">{entryMessage(entry)}</span>
                                </div>
                              {/each}
                            </div>
                          </details>
                        {/each}
                        <!-- Ungrouped entries (no testcase, e.g. START_TIME, DEPENDENCY_VERSION) -->
                        {#if group.ungrouped.length}
                          {#if group.testcasesArr.length === 0}
                            <!-- Module has only ungrouped entries (e.g. System): flat level+message rows -->
                            {#each group.ungrouped as entry}
                              {@const level = normalizeLevel(entry.level)}
                              <div class="result-row tc-row">
                                <span class={`entry-level severity-${level.toLowerCase()}`}>{level}</span>
                                <span class="entry-message">{entryMessage(entry)}</span>
                              </div>
                            {/each}
                          {:else}
                            <div class="result-header ungrouped-header">
                              <span>{$t("result_col_seconds")}</span>
                              <span>{$t("result_col_level")}</span>
                              <span>{$t("result_col_message")}</span>
                            </div>
                            {#each group.ungrouped as entry}
                              {@const level = normalizeLevel(entry.level)}
                              <div class="result-row">
                                <span class="entry-time">{formatSeconds(entry.timestamp)}</span>
                                <span class={`entry-level severity-${level.toLowerCase()}`}>{level}</span>
                                <span class="entry-message">{entryMessage(entry)}</span>
                              </div>
                            {/each}
                          {/if}
                        {/if}
                      </div>
                    {/if}
                  </div>
                {/each}
              </div>
            {/if}
            {#if nsTimings.length}
              <details class="score-bonus ns-timings-card" data-testid="admin-nameserver-timings">
                <summary class="score-bonus-summary ns-timings-summary">
                  <span class="score-bonus-chevron"></span>
                  <span class="score-bonus-title">{$t("ns_timing_heading")}</span>
                  <span class="ns-timings-summary-text">{$t("ns_timing_subtitle")}</span>
                </summary>
                <div class="score-bonus-list ns-timings-content">
                  <table class="ns-timings-table">
                    <thead>
                      <tr>
                        <th scope="col">{$t("ns_timing_nameserver")}</th>
                        <th scope="col">{$t("ns_timing_ip")}</th>
                        <th scope="col" class="ns-timings-num">{$t("ns_timing_avg_ms")}</th>
                        <th scope="col" class="ns-timings-num">{$t("ns_timing_min_ms")}</th>
                        <th scope="col" class="ns-timings-num">{$t("ns_timing_max_ms")}</th>
                        <th scope="col" class="ns-timings-num">{$t("ns_timing_samples")}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {#each nsTimings as item}
                        <tr data-testid="admin-nameserver-timing-row">
                          <td class="ns-timings-name">{item.nameserver}</td>
                          <td class="ns-timings-ip">{item.address}</td>
                          <td class="ns-timings-num ns-timings-avg">{formatTimingMs(item.avg_ms)}</td>
                          <td class="ns-timings-num">{formatTimingMs(item.min_ms)}</td>
                          <td class="ns-timings-num">{formatTimingMs(item.max_ms)}</td>
                          <td class="ns-timings-num">{item.count}</td>
                        </tr>
                      {/each}
                    </tbody>
                  </table>
                </div>
              </details>
            {/if}
          </div>
        {/if}
        {#if selectedJob && !selectedJobResult && isResultReadyStatus(selectedJob.status)}
          <button class="ghost" type="button" onclick={() => loadJobResult()}>
            {$t("load_result")}
          </button>
        {/if}
      </div>
    </div>
  {:else if activeTab === "recent"}
    <div class="card reveal" id="panel-recent" role="tabpanel" aria-labelledby="tab-recent" style="--d: 0.34s; margin-top: 22px;">
      <h2>{$t("recent_tests_heading")}</h2>
      <div class="row">
        <button class="ghost" type="button" onclick={loadJobs} disabled={jobsLoading}>
          {jobsLoading ? $t("refreshing") : $t("refresh_list")}
        </button>
        <button class="ghost" type="button" onclick={() => (autoRefreshRecent = !autoRefreshRecent)}>
          {autoRefreshRecent ? $t("auto_refresh_on") : $t("auto_refresh_off")}
        </button>
        <div class="sort-control">
          <label for="recent-sort">{$t("sort_label")}</label>
          <select id="recent-sort" bind:value={jobSort} onchange={applyRecentFilters}>
            {#each jobSortOptions as option}
              <option value={option.id}>{$t(option.labelKey)}</option>
            {/each}
          </select>
        </div>
        <div class="sort-control">
          <label for="recent-page-size">{$t("page_size_label")}</label>
          <select id="recent-page-size" bind:value={recentPageSize} onchange={applyRecentFilters}>
            {#each recentPageSizes as pageSize}
              <option value={pageSize}>{pageSize}</option>
            {/each}
          </select>
        </div>
        <div class="sort-control grow">
          <label for="recent-domain-filter">{$t("domain_contains_label")}</label>
          <input
            id="recent-domain-filter"
            type="text"
            placeholder="example.com"
            bind:value={recentDomainFilter}
            onkeydown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault();
                applyRecentFilters();
              }
            }}
          />
        </div>
        <div class="sort-control grow">
          <label for="recent-batch-filter">{$t("batch_id_filter_label")}</label>
          <input
            id="recent-batch-filter"
            type="text"
            placeholder="batch_123"
            bind:value={jobBatchFilter}
            onkeydown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault();
                applyRecentFilters();
              }
            }}
          />
        </div>
        <div class="row">
          <button class="ghost" type="button" onclick={applyRecentFilters} disabled={jobsLoading}>{$t("apply_filters")}</button>
          <button class="ghost" type="button" onclick={clearRecentFilters} disabled={jobsLoading}>{$t("clear")}</button>
        </div>
      </div>
      <div class="severity-filter-bar" role="group" aria-label={$t("severity_filters_aria")}>
        {#each severityFilters as filter}
          <button
            type="button"
            class={`severity-filter ${severityFilter === filter.id ? "active" : ""}`}
            onclick={async () => {
              severityFilter = filter.id;
              await applyRecentFilters();
            }}
          >
            {$t(filter.labelKey)}
          </button>
        {/each}
      </div>
      <div class="row batch-pagination">
        <button
          class="ghost"
          type="button"
          onclick={() => goToRecentCursor(recentPrevCursor)}
          disabled={!recentPrevCursor || jobsLoading}
        >
          {$t("previous")}
        </button>
        <button
          class="ghost"
          type="button"
          onclick={() => goToRecentCursor(recentNextCursor)}
          disabled={!recentNextCursor || jobsLoading}
        >
          {$t("next")}
        </button>
        <span class="small">
          {$t("showing_jobs", { shown: jobs.length, total: recentTotal, offset: recentOffset || 0 })}
        </span>
      </div>
      <div class="list">
        {#if jobs.length === 0}
          <div class="small">{$t("no_jobs")}</div>
        {:else if filteredJobs.length === 0}
          <div class="small">{$t("no_jobs_severity")}</div>
        {:else}
          {#each filteredJobs as job (job.id)}
            <div class="list-item clickable" onclick={() => {
              navigateToJob(job.id);
            }} onkeydown={(e) => {
              if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                navigateToJob(job.id);
              }
            }} role="button" tabindex="0">
              <div class="list-item-main">
                <div class="job-headline">
                  <span class="mono job-id-link">{job.id}</span>
                  <span class="small">{job.domain} – {job.status}</span>
                  {#if jobSeverityRows(job).length}
                    {#each jobSeverityRows(job) as entry (entry.level)}
                      <span class={`level-pill severity-${entry.level.toLowerCase()}`}>{entry.level} {entry.count}</span>
                    {/each}
                  {:else if job.severity_totals !== undefined}
                    <span class="level-pill severity-info">INFO</span>
                  {/if}
                  {#if scoringEnabled && hasScore(job)}
                    <span class="grade-chip">
                      <span class="grade-chip-letter" data-grade={chipGrade(job)}>{chipGrade(job)}</span>
                      <span class="grade-chip-score">{chipScore(job)}</span>
                    </span>
                  {/if}
                </div>
                {#if job.batch_id}
                  <div class="small mono">{$t("batch_prefix")} {job.batch_id}</div>
                {/if}
                {#if jobProfileName(job)}
                  <div class="small">{$t("job_profile_label")}: <span class="mono">{jobProfileName(job)}</span></div>
                {/if}
                <div class="progress compact list-progress" role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow={progressPercent(job)}>
                  <div class="progress-bar" style={`width: ${progressPercent(job)}%`}></div>
                  <span class="progress-value">{progressPercent(job)}%</span>
                </div>
              </div>
            </div>
          {/each}
        {/if}
      </div>
    </div>
  {:else if activeTab === "domains"}
    <div class="card reveal" id="panel-domains" role="tabpanel" aria-labelledby="tab-domains" style="--d: 0.34s; margin-top: 22px;">
      {#if selectedDomain}
        <div>
          <button class="secondary small" onclick={() => { selectedDomain = null; setTab("domains"); }}>{$t("back_to_domains")}</button>
          <div style="display:flex; align-items: baseline; gap: 0.6rem; flex-wrap: wrap; margin-top: 0.5rem; margin-bottom: 0.75rem;">
            <h2 class="mono" style="margin: 0;">{selectedDomain.name}</h2>
            {#if selectedDomain.tags && selectedDomain.tags.length > 0}
              {#each selectedDomain.tags as tag}
                <span class="badge">{tag}</span>
              {/each}
            {/if}
          </div>
          <div class="kv" style="margin-bottom: 1rem;">
            <span>{$t("col_latest_level")}</span>
            <span>{#if domainLevel(selectedDomain)}<span class="badge level-{domainLevel(selectedDomain).toLowerCase()}">{domainLevel(selectedDomain)}</span>{:else}-{/if}</span>
            <span>{$t("col_latest_run_at")}</span>
            <strong>{selectedDomain.latest_run_at ? formatTimestampLocal(selectedDomain.latest_run_at) : "—"}</strong>
            <span>{$t("col_run_count")}</span>
            <strong>{selectedDomain.run_count ?? 0}</strong>
          </div>
          <button class="secondary" onclick={retestDomain} disabled={domainResubmitting}>
            {domainResubmitting ? $t("submitting") : $t("retest_domain")}
          </button>

          <!-- Latest / selected run result -->
          {#if domainRunResultLoading}
            <p class="muted" style="margin-top: 1rem;">{$t("loading")}</p>
          {:else if selectedDomainRunResult}
            {@const allEntries = selectedDomainRunResult?.raw?.entries ?? []}
            {@const nsTimings = selectedDomainRunResult?.nameserver_timings ?? []}
            {@const bannerCls = bannerClass(worstLevel(allEntries))}
            {@const sc = selectedDomainRunResult?.score}
            <div class="stack" style="margin-top: 1.25rem;">
              {#if scoringEnabled && sc}
                {@const sortedCats = CAT_ORDER.filter(c => c in (sc.categories ?? {})).map(c => [c, sc.categories[c]])}
                <div class="score-card">
                  <div class="score-left">
                    <div class="grade-badge" data-grade={sc.grade}>
                      <span class="grade-letter">{sc.grade}</span>
                    </div>
                    <div class="score-meta">
                      <div class="score-number">{sc.score}<span class="score-denom">/100</span></div>
                      <div class="score-label">DNS Quality Score</div>
                    </div>
                  </div>
                  {#if sortedCats.length}
                    <div class="score-cats">
                      {#each sortedCats as [cat, res], i}
                        <div class="score-cat-row" data-untested={res.tested === false ? "" : undefined}>
                          <span class="score-cat-name">{CAT_LABELS[cat] ?? cat}</span>
                          <div class="score-cat-bar-track">
                            <div class="score-cat-bar" style="--bar-pct:{res.tested === false ? 0 : res.score}%; --bar-color:{gradeBarColor(sc.grade)}; animation-delay:{i * 60}ms"></div>
                          </div>
                          <span class="score-cat-num">{res.tested === false ? "-" : res.score}</span>
                        </div>
                      {/each}
                    </div>
                  {/if}
                </div>
                {#if sc.bonus?.criteria}
                  {@const bonusCriteria = Object.entries(sc.bonus.criteria).filter(([k]) => !BONUS_HIDDEN.has(k))}
                  {@const bonusMissing = bonusCriteria.filter(([, v]) => v === false).length}
                  {#if bonusCriteria.length}
                    <details class="score-bonus">
                      <summary class="score-bonus-summary">
                        <span class="score-bonus-chevron"></span>
                        <span class="score-bonus-title">{$t("pub.score_aplus_criteria")}</span>
                        <span class="score-bonus-status" data-met={sc.bonus.eligible ? "yes" : "no"}>
                          {sc.bonus.eligible ? $t("pub.score_aplus_achieved") : $t("pub.score_aplus_missing", { n: bonusMissing })}
                        </span>
                      </summary>
                      <div class="score-bonus-list">
                        {#each bonusCriteria as [key, val]}
                          <div class="score-bonus-item" data-met={val === null ? "na" : val ? "yes" : "no"}>
                            <span class="score-bonus-icon">{val === null ? "–" : val ? "✓" : "✗"}</span>
                            <span>{$t(`pub.score_bonus_${key}`)}</span>
                          </div>
                        {/each}
                      </div>
                    </details>
                  {/if}
                {/if}
              {/if}
              {#if allEntries.length}
                <div class="status-banner {bannerCls}">{$t(`result_status_${bannerCls}`)}</div>
              {/if}
              <div class="field-label">{$t("result_summary_label")}</div>
              {#if summaryRows(selectedDomainRunResult.summary).length}
                <div class="summary-grid">
                  {#each summaryRows(selectedDomainRunResult.summary) as row (row.level)}
                    <div class={`summary-item severity-${row.level.toLowerCase()}`}>
                      <span class="summary-label">{row.level}</span>
                      <span class="summary-count">{row.count}</span>
                    </div>
                  {/each}
                </div>
              {:else}
                <div class="summary-empty">{$t("no_result_entries")}</div>
              {/if}
              <div class="field-label">{$t("result_details_label")}</div>
              {#if domainModuleGroups.length === 0}
                <div class="summary-empty">{$t("no_raw_entries")}</div>
              {:else}
                <div class="small">{$t("module_expand_hint")}</div>
                <div class="module-list">
                  {#each domainModuleGroups as group (group.key)}
                    {@const groupLevel = worstLevel(group.entries)}
                    <div class="module-card" data-level={groupLevel.toLowerCase()}>
                      <button
                        class="module-toggle"
                        type="button"
                        aria-expanded={!!domainModuleOpen[group.key]}
                        aria-controls={"dm-" + moduleId(group.key)}
                        onclick={() => toggleDomainModule(group.key)}
                      >
                        <div class="module-title">{group.name}</div>
                        <div class="module-meta">{group.testcasesArr.length > 0 ? `${group.testcasesArr.length} tests · ` : ""}{$t("entries_count", { count: group.entries.length })}</div>
                        <div class="module-badges">
                          {#if okTestcaseCount(group) > 0}
                            <span class="level-pill severity-info">INFO {okTestcaseCount(group)}</span>
                          {/if}
                          {#each moduleLevels as level}
                            {#if group.counts[level]}
                              <span class={`level-pill severity-${level.toLowerCase()}`}>{level} {group.counts[level]}</span>
                            {/if}
                          {/each}
                        </div>
                        <span class={`module-chevron ${domainModuleOpen[group.key] ? "open" : ""}`}></span>
                      </button>
                      {#if domainModuleOpen[group.key]}
                        <div class="module-body" id={"dm-" + moduleId(group.key)}>
                          {#each group.testcasesArr as tcg (tcg.tc)}
                            {@const tcKey = `tc.${tcg.tc.toLowerCase()}`}
                            {@const tcDesc = $t(tcKey)}
                            <details class="testcase-group" open={isNoticeOrAbove(tcg.level)}>
                              <summary class="testcase-summary">
                                <span class="testcase-chevron"></span>
                                <span class="testcase-desc">{tcDesc !== tcKey ? tcDesc : tcg.tc}</span>
                                <span class="testcase-badge">
                                  <span class={`level-pill severity-${normalizeLevel(tcg.level).toLowerCase()}`}>{normalizeLevel(tcg.level)}</span>
                                </span>
                              </summary>
                              <div class="testcase-entries">
                                {#each tcg.entries as entry}
                                  {@const level = normalizeLevel(entry.level)}
                                  <div class="result-row tc-row">
                                    <span class={`entry-level severity-${level.toLowerCase()}`}>{level}</span>
                                    <span class="entry-message">{entryMessage(entry)}</span>
                                  </div>
                                {/each}
                              </div>
                            </details>
                          {/each}
                          {#if group.ungrouped.length}
                            {#if group.testcasesArr.length === 0}
                              {#each group.ungrouped as entry}
                                {@const level = normalizeLevel(entry.level)}
                                <div class="result-row tc-row">
                                  <span class={`entry-level severity-${level.toLowerCase()}`}>{level}</span>
                                  <span class="entry-message">{entryMessage(entry)}</span>
                                </div>
                              {/each}
                            {:else}
                              <div class="result-header ungrouped-header">
                                <span>{$t("result_col_seconds")}</span>
                                <span>{$t("result_col_level")}</span>
                                <span>{$t("result_col_message")}</span>
                              </div>
                              {#each group.ungrouped as entry}
                                {@const level = normalizeLevel(entry.level)}
                                <div class="result-row">
                                  <span class="entry-time">{formatSeconds(entry.timestamp)}</span>
                                  <span class={`entry-level severity-${level.toLowerCase()}`}>{level}</span>
                                  <span class="entry-message">{entryMessage(entry)}</span>
                                </div>
                              {/each}
                            {/if}
                          {/if}
                        </div>
                      {/if}
                    </div>
                  {/each}
                </div>
              {/if}
              {#if nsTimings.length}
                <details class="score-bonus ns-timings-card" data-testid="admin-nameserver-timings">
                  <summary class="score-bonus-summary ns-timings-summary">
                    <span class="score-bonus-chevron"></span>
                    <span class="score-bonus-title">{$t("ns_timing_heading")}</span>
                    <span class="ns-timings-summary-text">{$t("ns_timing_subtitle")}</span>
                  </summary>
                  <div class="score-bonus-list ns-timings-content">
                    <table class="ns-timings-table">
                      <thead>
                        <tr>
                          <th scope="col">{$t("ns_timing_nameserver")}</th>
                          <th scope="col">{$t("ns_timing_ip")}</th>
                          <th scope="col" class="ns-timings-num">{$t("ns_timing_avg_ms")}</th>
                          <th scope="col" class="ns-timings-num">{$t("ns_timing_min_ms")}</th>
                          <th scope="col" class="ns-timings-num">{$t("ns_timing_max_ms")}</th>
                          <th scope="col" class="ns-timings-num">{$t("ns_timing_samples")}</th>
                        </tr>
                      </thead>
                      <tbody>
                        {#each nsTimings as item}
                          <tr data-testid="admin-nameserver-timing-row">
                            <td class="ns-timings-name">{item.nameserver}</td>
                            <td class="ns-timings-ip">{item.address}</td>
                            <td class="ns-timings-num ns-timings-avg">{formatTimingMs(item.avg_ms)}</td>
                            <td class="ns-timings-num">{formatTimingMs(item.min_ms)}</td>
                            <td class="ns-timings-num">{formatTimingMs(item.max_ms)}</td>
                            <td class="ns-timings-num">{item.count}</td>
                          </tr>
                        {/each}
                      </tbody>
                    </table>
                  </div>
                </details>
              {/if}
            </div>
          {/if}

          <!-- Run history -->
          <h3 style="margin-top: 1.5rem;">{$t("domain_run_history_heading")}</h3>
          {#if domainRunsLoading}
            <p class="muted">{$t("loading")}</p>
          {:else if domainRuns.length === 0}
            <p class="muted">{$t("no_runs")}</p>
          {:else}
            <table class="data-table">
              <thead>
                <tr>
                  <th class="sortable-column" aria-sort={tableSortAria(domainRunsSortState, "id")}><button class="table-sort-button" type="button" onclick={() => { sortDomainRuns("id"); }}><span>{$t("col_run_id")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(domainRunsSortState, "id")}</span></button></th>
                  <th class="sortable-column" aria-sort={tableSortAria(domainRunsSortState, "finished_at")}><button class="table-sort-button" type="button" onclick={() => { sortDomainRuns("finished_at", "desc"); }}><span>{$t("col_finished_at")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(domainRunsSortState, "finished_at")}</span></button></th>
                  <th class="sortable-column" aria-sort={tableSortAria(domainRunsSortState, "worst_level")}><button class="table-sort-button" type="button" onclick={() => { sortDomainRuns("worst_level", "desc"); }}><span>{$t("col_worst_level")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(domainRunsSortState, "worst_level")}</span></button></th>
                  {#if scoringEnabled}<th class="sortable-column" aria-sort={tableSortAria(domainRunsSortState, "score")}><button class="table-sort-button" type="button" onclick={() => { sortDomainRuns("score", "desc"); }}><span>{$t("col_score")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(domainRunsSortState, "score")}</span></button></th>{/if}
                  <th class="sortable-column" aria-sort={tableSortAria(domainRunsSortState, "duration_ms")}><button class="table-sort-button" type="button" onclick={() => { sortDomainRuns("duration_ms", "desc"); }}><span>{$t("col_duration")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(domainRunsSortState, "duration_ms")}</span></button></th>
                  <th class="sortable-column" aria-sort={tableSortAria(domainRunsSortState, "entry_count")}><button class="table-sort-button" type="button" onclick={() => { sortDomainRuns("entry_count", "desc"); }}><span>{$t("col_entries")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(domainRunsSortState, "entry_count")}</span></button></th>
                </tr>
              </thead>
              <tbody>
                {#each sortedDomainRuns as run}
                  <tr
                    style="cursor: pointer;"
                    class={selectedDomainRunId === run.id ? "run-row-selected" : ""}
                    onclick={() => { navigateToJob(run.id); }}
                    role="button"
                    tabindex="0"
                    onkeydown={(e) => { if (e.key === "Enter" || e.key === " ") { navigateToJob(run.id); } }}
                  >
                    <td class="run-id-cell" title={run.id}>{run.id}</td>
                    <td>{run.finished_at ? run.finished_at.slice(0, 16).replace("T", " ") : "—"}</td>
                    <td><span class="badge level-{(run.worst_level || 'info').toLowerCase()}">{run.worst_level || "INFO"}</span></td>
                    {#if scoringEnabled}<td>{#if hasScore(run)}<span class="grade-chip"><span class="grade-chip-letter" data-grade={chipGrade(run)}>{chipGrade(run)}</span><span class="grade-chip-score">{chipScore(run)}</span></span>{:else}—{/if}</td>{/if}
                    <td>{run.duration_ms != null ? run.duration_ms + "ms" : "—"}</td>
                    <td>{run.entry_count ?? 0}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
            <div class="pagination" style="margin-top: 0.5rem; display:flex; gap: 0.5rem; align-items: center;">
              <button
                class="secondary small"
                disabled={domainRunsOffset === 0}
                onclick={() => { domainRunsOffset = Math.max(0, domainRunsOffset - domainRunsLimit); loadDomainRuns(); }}
              >{$t("prev_page")}</button>
              <span class="muted small">{domainRunsOffset + 1}–{Math.min(domainRunsOffset + domainRunsLimit, domainRunsTotal)} / {domainRunsTotal}</span>
              <button
                class="secondary small"
                disabled={domainRunsOffset + domainRunsLimit >= domainRunsTotal}
                onclick={() => { domainRunsOffset += domainRunsLimit; loadDomainRuns(); }}
              >{$t("next_page")}</button>
            </div>
          {/if}
        </div>
      {:else}
        <h2>{$t("domains_tab_heading")}</h2>
        <div class="toolbar" style="display:flex; gap: 0.5rem; flex-wrap: wrap; margin-bottom: 0.75rem;">
          <input
            type="search"
            placeholder={$t("domains_search_placeholder")}
            bind:value={domainNameFilter}
            oninput={() => loadDomains({ reset: true })}
            style="flex: 1 1 180px;"
          />
          <select
            bind:value={domainTagFilter}
            onchange={() => loadDomains({ reset: true })}
            style="flex: 0 1 180px;"
            aria-label={$t("tag_filter_label")}
          >
            <option value="">{$t("tag_filter_all")}</option>
            {#each availableTags as tag}
              <option value={tag.name}>{tag.name}</option>
            {/each}
          </select>
          <select
            bind:value={domainLevelFilter}
            onchange={() => loadDomains({ reset: true })}
            style="flex: 0 1 160px;"
            aria-label={$t("level_filter_label")}
          >
            <option value="">{$t("level_filter_all")}</option>
            <option value="WARNING">{$t("level_filter_warning_plus")}</option>
            <option value="ERROR">{$t("level_filter_error_plus")}</option>
          </select>
        </div>
        {#if domainsLoading}
          <p class="muted">{$t("loading")}</p>
        {:else if domains.length === 0}
          <p class="muted">{$t("no_domains")}</p>
        {:else}
          <table class="data-table">
            <thead>
              <tr>
                <th class="sortable-column" aria-sort={tableSortAria(domainsSortState, "name")}><button class="table-sort-button" type="button" onclick={() => { sortDomains("name"); }}><span>{$t("col_domain_name")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(domainsSortState, "name")}</span></button></th>
                <th class="sortable-column" aria-sort={tableSortAria(domainsSortState, "tags")}><button class="table-sort-button" type="button" onclick={() => { sortDomains("tags"); }}><span>{$t("col_tags")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(domainsSortState, "tags")}</span></button></th>
                <th class="sortable-column" aria-sort={tableSortAria(domainsSortState, "latest_level")}><button class="table-sort-button" type="button" onclick={() => { sortDomains("latest_level", "desc"); }}><span>{$t("col_latest_level")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(domainsSortState, "latest_level")}</span></button></th>
                {#if scoringEnabled}<th class="sortable-column" aria-sort={tableSortAria(domainsSortState, "latest_score")}><button class="table-sort-button" type="button" onclick={() => { sortDomains("latest_score", "desc"); }}><span>{$t("col_score")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(domainsSortState, "latest_score")}</span></button></th>{/if}
                <th class="sortable-column" aria-sort={tableSortAria(domainsSortState, "latest_run_at")}><button class="table-sort-button" type="button" onclick={() => { sortDomains("latest_run_at", "desc"); }}><span>{$t("col_latest_run_at")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(domainsSortState, "latest_run_at")}</span></button></th>
                <th class="sortable-column" aria-sort={tableSortAria(domainsSortState, "run_count")}><button class="table-sort-button" type="button" onclick={() => { sortDomains("run_count", "desc"); }}><span>{$t("col_run_count")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(domainsSortState, "run_count")}</span></button></th>
              </tr>
            </thead>
            <tbody>
              {#each sortedDomains as d}
                <tr
                  style="cursor: pointer;"
                  onclick={() => navigateToDomainDetail(d)}
                  role="button"
                  tabindex="0"
                  onkeydown={(e) => { if (e.key === "Enter" || e.key === " ") navigateToDomainDetail(d); }}
                >
                  <td class="mono">{d.name}</td>
                  <td>{d.tags ? d.tags.join(", ") : ""}</td>
                  <td>{#if domainLevel(d)}<span class="badge level-{domainLevel(d).toLowerCase()}">{domainLevel(d)}</span>{:else}-{/if}</td>
                  {#if scoringEnabled}<td>{#if d.latest_grade != null && d.latest_score != null}<span class="grade-chip"><span class="grade-chip-letter" data-grade={d.latest_grade}>{d.latest_grade}</span><span class="grade-chip-score">{d.latest_score}</span></span>{:else}—{/if}</td>{/if}
                  <td>{d.latest_run_at ? d.latest_run_at.slice(0, 10) : "—"}</td>
                  <td>{d.run_count ?? 0}</td>
                </tr>
              {/each}
            </tbody>
          </table>
          <div class="pagination" style="margin-top: 0.5rem; display:flex; gap: 0.5rem; align-items: center;">
            <button
              class="secondary small"
              disabled={domainsOffset === 0}
              onclick={() => { domainsOffset = Math.max(0, domainsOffset - domainsLimit); loadDomains(); }}
            >{$t("prev_page")}</button>
            <span class="muted small">{domainsOffset + 1}–{Math.min(domainsOffset + domainsLimit, domainsTotal)} / {domainsTotal}</span>
            <button
              class="secondary small"
              disabled={domainsOffset + domainsLimit >= domainsTotal}
              onclick={() => { domainsOffset += domainsLimit; loadDomains(); }}
            >{$t("next_page")}</button>
          </div>
        {/if}
      {/if}
    </div>
  {:else if activeTab === "tags"}
    <div class="card reveal" id="panel-tags" role="tabpanel" aria-labelledby="tab-tags" style="--d: 0.34s; margin-top: 22px;">
      {#if selectedTag}
        <button class="secondary small" onclick={() => { selectedTag = null; tagProfileDraftId = ""; setTab("tags"); }}>{$t("back_to_tags")}</button>
        <h2 style="margin-top: 0.5rem;">{$t("batch_tag_label")}: {selectedTag.name}</h2>

        <!-- Severity summary -->
        {#if tagSummaryLoading}
          <p class="muted">{$t("loading")}</p>
        {:else if tagSummary}
          <div style="display:flex; gap: 1rem; flex-wrap: wrap; margin-bottom: 1rem;">
            <span class="level-pill severity-info">{$t("sev_ok")} {tagSummary.ok}</span>
            <span class="level-pill severity-notice">{$t("sev_notice")} {tagSummary.notice}</span>
            <span class="level-pill severity-warning">{$t("sev_warning")} {tagSummary.warning}</span>
            <span class="level-pill severity-error">{$t("sev_error")} {tagSummary.error}</span>
            <span class="level-pill severity-critical">{$t("sev_critical")} {tagSummary.critical}</span>
          </div>
        {/if}

        <!-- Run all + delete -->
        <div style="display:flex; gap: 0.5rem; flex-wrap: wrap; margin-bottom: 1rem;">
          <button class="secondary" onclick={runAllFromTag} disabled={tagRunAllSubmitting}>
            {tagRunAllSubmitting ? $t("submitting") : $t("tag_run_all_button")}
          </button>
          {#if tagDeleteConfirm}
            <button class="warn" onclick={deleteTag} disabled={tagDeleting}>{tagDeleting ? $t("submitting") : $t("tag_delete_confirm_button")}</button>
            <button class="ghost" onclick={() => { tagDeleteConfirm = false; }}>{$t("tag_delete_cancel_button")}</button>
          {:else}
            <button class="ghost" onclick={() => { tagDeleteConfirm = true; }}>{$t("tag_delete_button")}</button>
          {/if}
        </div>

        <h3>{$t("tag_default_profile_heading")}</h3>
        <div class="stack" style="max-width: 440px; margin-bottom: 1rem;">
          <label for="tag-default-profile">{$t("tag_default_profile_label")}</label>
          <select id="tag-default-profile" bind:value={tagProfileDraftId} disabled={profilesLoading && availableProfiles.length === 0}>
            <option value="">{$t("tag_default_profile_none_option")}</option>
            {#each availableProfiles as profile}
              <option value={profile.id}>{profile.name}</option>
            {/each}
          </select>
          <div class="small">{$t("tag_default_profile_hint")}</div>
          <div class="small">
            {#if tagProfileCurrentID}
              {$t("tag_default_profile_current", { name: profileNameByID(tagProfileCurrentID) })}
            {:else}
              {$t("tag_default_profile_current_none")}
            {/if}
          </div>
          <div class="row">
            <button
              class="secondary small"
              onclick={saveTagProfile}
              disabled={!tagProfileDirty || !tagProfileSelectedID || tagProfileUpdating || tagProfileClearing}
            >
              {tagProfileUpdating ? $t("submitting") : $t("tag_default_profile_save_button")}
            </button>
            <button
              class="ghost small"
              onclick={clearTagProfile}
              disabled={!tagProfileCurrentID || tagProfileUpdating || tagProfileClearing}
            >
              {tagProfileClearing ? $t("submitting") : $t("tag_default_profile_clear_button")}
            </button>
          </div>
        </div>

        <!-- Domain list with level filter -->
        <h3>{$t("tag_domains_heading")}</h3>
        <div style="display:flex; gap: 0.5rem; flex-wrap: wrap; margin-bottom: 0.5rem;">
          <select
            bind:value={tagDomainLevelFilter}
            onchange={() => loadTagDomains({ reset: true })}
            aria-label={$t("level_filter_label")}
            style="flex: 0 1 180px;"
          >
            <option value="">{$t("level_filter_all")}</option>
            <option value="WARNING">{$t("level_filter_warning_plus")}</option>
            <option value="ERROR">{$t("level_filter_error_plus")}</option>
          </select>
        </div>
        {#if tagDomainsLoading}
          <p class="muted">{$t("loading")}</p>
        {:else if tagDomains.length === 0}
          <p class="muted">{$t("no_domains")}</p>
        {:else}
          <table class="data-table">
            <thead><tr>
              <th class="sortable-column" aria-sort={tableSortAria(tagDomainsSortState, "name")}><button class="table-sort-button" type="button" onclick={() => { sortTagDomains("name"); }}><span>{$t("col_domain_name")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(tagDomainsSortState, "name")}</span></button></th>
              <th class="sortable-column" aria-sort={tableSortAria(tagDomainsSortState, "latest_level")}><button class="table-sort-button" type="button" onclick={() => { sortTagDomains("latest_level", "desc"); }}><span>{$t("col_latest_level")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(tagDomainsSortState, "latest_level")}</span></button></th>
              {#if scoringEnabled}<th class="sortable-column" aria-sort={tableSortAria(tagDomainsSortState, "latest_score")}><button class="table-sort-button" type="button" onclick={() => { sortTagDomains("latest_score", "desc"); }}><span>{$t("col_score")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(tagDomainsSortState, "latest_score")}</span></button></th>{/if}
              <th class="sortable-column" aria-sort={tableSortAria(tagDomainsSortState, "latest_run_at")}><button class="table-sort-button" type="button" onclick={() => { sortTagDomains("latest_run_at", "desc"); }}><span>{$t("col_latest_run_at")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(tagDomainsSortState, "latest_run_at")}</span></button></th>
            </tr></thead>
            <tbody>
              {#each sortedTagDomains as d}
                <tr
                  style="cursor: pointer;"
                  onclick={() => navigateToDomainDetail(d)}
                  role="button"
                  tabindex="0"
                  onkeydown={(e) => { if (e.key === "Enter" || e.key === " ") navigateToDomainDetail(d); }}
                >
                  <td class="mono">{d.name}</td>
                  <td>{#if domainLevel(d)}<span class="badge level-{domainLevel(d).toLowerCase()}">{domainLevel(d)}</span>{:else}-{/if}</td>
                  {#if scoringEnabled}<td>{#if d.latest_grade != null && d.latest_score != null}<span class="grade-chip"><span class="grade-chip-letter" data-grade={d.latest_grade}>{d.latest_grade}</span><span class="grade-chip-score">{d.latest_score}</span></span>{:else}—{/if}</td>{/if}
                  <td>{d.latest_run_at ? d.latest_run_at.slice(0, 10) : "—"}</td>
                </tr>
              {/each}
            </tbody>
          </table>
          <div class="pagination" style="margin-top: 0.5rem; display:flex; gap: 0.5rem; align-items: center;">
            <button class="secondary small" disabled={tagDomainsOffset === 0}
              onclick={() => { tagDomainsOffset = Math.max(0, tagDomainsOffset - tagDomainsLimit); loadTagDomains(); }}
            >{$t("prev_page")}</button>
            <span class="muted small">{tagDomainsOffset + 1}–{Math.min(tagDomainsOffset + tagDomainsLimit, tagDomainsTotal)} / {tagDomainsTotal}</span>
            <button class="secondary small" disabled={tagDomainsOffset + tagDomainsLimit >= tagDomainsTotal}
              onclick={() => { tagDomainsOffset += tagDomainsLimit; loadTagDomains(); }}
            >{$t("next_page")}</button>
          </div>
        {/if}

        <!-- Add domains -->
        <h3 style="margin-top: 1.25rem;">{$t("tag_add_domains_heading")}</h3>
        <textarea
          bind:value={tagAddDomainsInput}
          placeholder={$t("tag_domains_placeholder")}
          rows="3"
          style="width: 100%; box-sizing: border-box;"
        ></textarea>
        <button class="secondary" onclick={addTagDomains} disabled={tagAddingDomains}>
          {tagAddingDomains ? $t("submitting") : $t("tag_add_domains_button")}
        </button>

        <!-- Remove domains -->
        <h3 style="margin-top: 1.25rem;">{$t("tag_remove_domains_heading")}</h3>
        <textarea
          bind:value={tagRemoveDomainsInput}
          placeholder={$t("tag_domains_placeholder")}
          rows="3"
          style="width: 100%; box-sizing: border-box;"
        ></textarea>
        <button class="ghost" onclick={removeTagDomains} disabled={tagRemovingDomains}>
          {tagRemovingDomains ? $t("submitting") : $t("tag_remove_domains_button")}
        </button>

      {:else}
        <h2>{$t("tags_tab_heading")}</h2>

        <!-- Create tag form -->
        <div style="display:flex; gap: 0.5rem; flex-wrap: wrap; margin-bottom: 1rem; align-items: flex-end;">
          <div class="stack" style="flex: 1 1 160px;">
            <label for="tag-create-name">{$t("tag_name_label")}</label>
            <input id="tag-create-name" type="text" bind:value={tagCreateName} placeholder="my-tag" />
          </div>
          <div class="stack" style="flex: 2 1 240px;">
            <label for="tag-create-desc">{$t("tag_description_label")}</label>
            <input id="tag-create-desc" type="text" bind:value={tagCreateDescription} placeholder={$t("tag_description_placeholder")} onkeydown={(e) => { if (e.key === 'Enter') createTag(); }} />
          </div>
          <button class="secondary" onclick={createTag} disabled={tagCreating || !tagCreateName.trim()}>
            {tagCreating ? $t("submitting") : $t("tag_create_button")}
          </button>
        </div>

        <!-- Tag list -->
        {#if tagsListLoading}
          <p class="muted">{$t("loading")}</p>
        {:else if tagsList.length === 0}
          <p class="muted">{$t("no_tags")}</p>
        {:else}
          <table class="data-table">
            <thead><tr>
              <th class="sortable-column" aria-sort={tableSortAria(tagsListSortState, "name")}><button class="table-sort-button" type="button" onclick={() => { tagsListSortState = nextTableSort(tagsListSortState, "name"); }}><span>{$t("tag_name_label")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(tagsListSortState, "name")}</span></button></th>
              <th class="sortable-column" aria-sort={tableSortAria(tagsListSortState, "description")}><button class="table-sort-button" type="button" onclick={() => { tagsListSortState = nextTableSort(tagsListSortState, "description"); }}><span>{$t("tag_description_label")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(tagsListSortState, "description")}</span></button></th>
              <th class="sortable-column" aria-sort={tableSortAria(tagsListSortState, "domain_count")}><button class="table-sort-button" type="button" onclick={() => { tagsListSortState = nextTableSort(tagsListSortState, "domain_count", "desc"); }}><span>{$t("col_domain_count")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(tagsListSortState, "domain_count")}</span></button></th>
            </tr></thead>
            <tbody>
              {#each sortedTagsList as tag}
                <tr
                  style="cursor: pointer;"
                  onclick={() => navigateToTagDetail(tag)}
                  role="button"
                  tabindex="0"
                  onkeydown={(e) => { if (e.key === "Enter" || e.key === " ") navigateToTagDetail(tag); }}
                >
                  <td class="mono">{tag.name}</td>
                  <td>{tag.description || "—"}</td>
                  <td>{tag.domain_count ?? 0}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        {/if}
      {/if}
    </div>
  {:else if activeTab === "batches"}
    <div class="grid" id="panel-batches" role="tabpanel" aria-labelledby="tab-batches" style="margin-top: 22px;">
      <div class="card reveal" style="--d: 0.22s">
        <h2>{$t("batch_jobs_heading")}</h2>
        <div style="display:flex; gap: 0.5rem; margin-bottom: 0.5rem;">
          <button
            class={batchFromTagMode ? "ghost" : "secondary small"}
            type="button"
            onclick={() => { batchFromTagMode = false; }}
          >{$t("batch_domains_mode_label")}</button>
          <button
            class={batchFromTagMode ? "secondary small" : "ghost"}
            type="button"
            onclick={() => { batchFromTagMode = true; }}
          >{$t("batch_from_tag_mode_label")}</button>
        </div>
        {#if batchFromTagMode}
          <div class="stack">
            <label for="batch-from-tag">{$t("batch_from_tag_label")}</label>
            <select id="batch-from-tag" bind:value={batchFromTag}>
              <option value="">{$t("tag_filter_all")}</option>
              {#each availableTags as tag}
                <option value={tag.name}>{tag.name}{tag.domain_count ? ` (${tag.domain_count})` : ""}</option>
              {/each}
            </select>
          </div>
        {:else}
          <div class="stack">
            <label for="batch-domains">{$t("domains_label")}</label>
            <textarea
              id="batch-domains"
              placeholder={`example.com
example.org`}
              bind:value={batchDomains}
            ></textarea>
          </div>
        {/if}
        <div class="stack">
          <label for="batch-tags">{$t("batch_tags_label")}</label>
          <input
            id="batch-tags"
            type="text"
            placeholder={$t("batch_tags_placeholder")}
            bind:value={batchTags}
          />
          <div class="small">{$t("batch_tags_hint")}</div>
        </div>
        <div class="stack">
          <label for="batch-profile">{$t("stored_profile_label")}</label>
          <select id="batch-profile" bind:value={batchProfileId} disabled={profilesLoading && availableProfiles.length === 0}>
            <option value="">{$t("stored_profile_auto_option")}</option>
            {#each availableProfiles as profile}
              <option value={profile.id}>{profile.name}</option>
            {/each}
          </select>
          <div class="small">{$t("stored_profile_hint")}</div>
        </div>
        <button class="secondary" onclick={submitBatch} disabled={batchSubmitting}>
          {batchSubmitting ? $t("submitting") : $t("run_batch")}
        </button>
        {#if createdBatchId}
          <div class="small">{$t("created_batch_prefix")} <span class="mono">{createdBatchId}</span></div>
        {/if}
      </div>

      <div class="card reveal" style="--d: 0.26s" data-testid="active-batches-card">
        <div class="row" style="justify-content: space-between; align-items: center;">
          <h2 style="margin: 0;">{$t("active_batches_heading")}</h2>
          <button
            class={queuePaused ? "secondary small" : "ghost small"}
            type="button"
            onclick={toggleQueuePause}
            disabled={queuePauseToggling}
            aria-busy={queuePauseToggling}
            title={$t("queue_pause_tooltip")}
          >
            {queuePauseToggling ? $t("loading") : queuePaused ? $t("queue_resume_button") : $t("queue_pause_button")}
          </button>
        </div>
        {#if queuePaused}
          <div class="status-banner warn" role="status" aria-live="polite">{$t("queue_paused_banner")}</div>
        {/if}
        {#if activeBatchesLoading && activeBatches.length === 0}
          <div class="small">{$t("loading")}</div>
        {:else if activeBatches.length === 0}
          <div class="small">{$t("no_active_batches")}</div>
        {:else}
          <div class="list">
            {#each activeBatches as batch (batch.batch_id)}
              <div class="list-item clickable" class:disabled={batchLoading} onclick={() => {
                if (batchLoading) return;
                selectedBatchId = batch.batch_id;
                loadBatch(batch.batch_id, { resetCursor: true });
              }} onkeydown={(e) => {
                if (batchLoading) return;
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  selectedBatchId = batch.batch_id;
                  loadBatch(batch.batch_id, { resetCursor: true });
                }
              }} role="button" tabindex="0" aria-disabled={batchLoading}>
                <div class="list-item-main">
                  <div class="mono">
                    {batch.batch_id}{#if batch.tag} <span class="small">({batch.tag})</span>{/if}
                  </div>
                  <div class="small">
                    {$t("total_label")}: {batch.total} · {formatBatchStatusCounts(batch.status_counts)} · {formatBatchTotalRuntime(batch)}
                  </div>
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </div>

      <div class="card reveal" style="--d: 0.3s">
        <h2>{$t("batch_inspector_heading")}</h2>
        <div class="stack">
          <label for="batch-recent">{$t("recent_batches_label")}</label>
          <select
            id="batch-recent"
            bind:value={selectedRecentBatch}
            disabled={recentBatchLoading}
            onchange={async () => {
              const nextBatchID = selectedRecentBatch.trim();
              if (!nextBatchID) return;
              selectedBatchId = nextBatchID;
              await loadBatch(nextBatchID, { resetCursor: true });
            }}
          >
            <option value="">{recentBatchLoading ? $t("loading_batches") : $t("select_recent_batch")}</option>
            {#each recentBatchOptions as option}
              <option value={option.id}>{formatRecentBatchOption(option)}</option>
            {/each}
          </select>
          <div class="small">{$t("latest_batches_hint")}</div>
          <label for="batch-id">{$t("batch_id_label")}</label>
          <input
            id="batch-id"
            type="text"
            placeholder="batch_123"
            bind:value={selectedBatchId}
            onchange={() => loadBatch(selectedBatchId, { resetCursor: true })}
          />
        </div>
        <div class="row">
          <button
            onclick={async () => {
              await loadRecentBatchOptions();
              await loadBatch();
            }}
            disabled={batchLoading}
          >
            {batchLoading ? $t("loading") : $t("refresh")}
          </button>
          <button class="ghost" type="button" onclick={() => (autoRefreshBatch = !autoRefreshBatch)}>
            {autoRefreshBatch ? $t("auto_refresh_on") : $t("auto_refresh_off")}
          </button>
        </div>
        <div class="batch-controls">
          <div class="sort-control">
            <label for="batch-sort">{$t("sort_label")}</label>
            <select id="batch-sort" bind:value={batchSort} onchange={applyBatchFilters}>
              {#each batchSortOptions as option}
                <option value={option.id}>{$t(option.labelKey)}</option>
              {/each}
            </select>
          </div>
          <div class="sort-control">
            <label for="batch-page-size">{$t("page_size_label")}</label>
            <select id="batch-page-size" bind:value={batchPageSize} onchange={applyBatchFilters}>
              {#each batchPageSizes as pageSize}
                <option value={pageSize}>{pageSize}</option>
              {/each}
            </select>
          </div>
          <div class="sort-control">
            <label for="batch-status">{$t("status_label")}</label>
            <select id="batch-status" bind:value={batchStatusFilter} onchange={applyBatchFilters}>
              {#each batchStatuses as status}
                <option value={status}>{status || $t("batch_status_all")}</option>
              {/each}
            </select>
          </div>
          <div class="sort-control grow">
            <label for="batch-domain-filter">{$t("domain_contains_label")}</label>
            <input
              id="batch-domain-filter"
              type="text"
              placeholder="example"
              bind:value={batchDomainFilter}
              onkeydown={(event) => {
                if (event.key === "Enter") {
                  event.preventDefault();
                  applyBatchFilters();
                }
              }}
            />
          </div>
          <div class="row">
            <button class="ghost" type="button" onclick={applyBatchFilters} disabled={batchLoading}>{$t("apply_filters")}</button>
            <button class="ghost" type="button" onclick={clearBatchFilters} disabled={batchLoading}>{$t("clear")}</button>
          </div>
        </div>
        {#if selectedBatch}
          <div class="kv">
            {#if selectedBatch.tag}
              <span>{$t("batch_tag_label")}</span>
              <strong class="mono">{selectedBatch.tag}</strong>
            {/if}
            <span>{$t("total_label")}</span>
            <strong>{selectedBatch.total}</strong>
            <span>{$t("created_label")}</span>
            <strong>{formatTimestampLocal(selectedBatch.created_at)}</strong>
            <span>{$t("total_runtime_label")}</span>
            <strong>{formatBatchTotalRuntime(selectedBatch)}</strong>
            <span>{$t("status_counts_label")}</span>
            <strong>{formatBatchStatusCounts(selectedBatch.status_counts)}</strong>
          </div>
          <div class="stack">
            <div class="field-label">{$t("jobs_label")}</div>
            <div class="row batch-pagination">
              <button
                class="ghost"
                type="button"
                onclick={() => goToBatchCursor(selectedBatch.prev_cursor)}
                disabled={!selectedBatch.prev_cursor || batchLoading}
              >
                {$t("previous")}
              </button>
              <button
                class="ghost"
                type="button"
                onclick={() => goToBatchCursor(selectedBatch.next_cursor)}
                disabled={!selectedBatch.next_cursor || batchLoading}
              >
                {$t("next")}
              </button>
              <span class="small">
                {$t("showing_jobs", { shown: selectedBatch.items.length, total: selectedBatch.total, offset: selectedBatch.offset || 0 })}
              </span>
            </div>
            <div class="list">
              {#if selectedBatch.items.length === 0}
                <div class="small">{$t("no_batch_jobs")}</div>
              {:else}
                {#each selectedBatch.items as item (item.id)}
                  <div class="list-item">
                    <div class="list-item-main">
                      <div class="mono">{item.id}</div>
                      <div class="small">{item.domain} - {item.status}</div>
                      {#if jobProfileName(item)}
                        <div class="small">{$t("job_profile_label")}: <span class="mono">{jobProfileName(item)}</span></div>
                      {/if}
                      <div class="progress compact list-progress" role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow={progressPercent(item)}>
                        <div class="progress-bar" style={`width: ${progressPercent(item)}%`}></div>
                        <span class="progress-value">{progressPercent(item)}%</span>
                      </div>
                    </div>
                    <button class="ghost" type="button" onclick={() => {
                      navigateToJob(item.id);
                    }}>{$t("inspect")}</button>
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
      <h2>{$t("metrics_heading")}</h2>
      <div class="row">
        <button class="ghost" type="button" onclick={() => loadMetrics()} disabled={metricsLoading}>
          {metricsLoading ? $t("refreshing") : $t("refresh_metrics")}
        </button>
        <button class="ghost" type="button" onclick={() => (autoRefreshMetrics = !autoRefreshMetrics)}>
          {autoRefreshMetrics ? $t("auto_refresh_on") : $t("auto_refresh_off")}
        </button>
        <div class="sort-control">
          <label for="metrics-window">{$t("trend_window_label")}</label>
          <select id="metrics-window" bind:value={metricsWindow} onchange={() => loadMetrics()}>
            {#each metricsWindowOptions as option}
              <option value={option.id}>{$t(option.labelKey)}</option>
            {/each}
          </select>
        </div>
        <div class="sort-control">
          <label for="metrics-domain-limit">{$t("top_domains_label")}</label>
          <select id="metrics-domain-limit" bind:value={metricsDomainLimit} onchange={() => loadMetrics()}>
            {#each metricsLimitOptions as value}
              <option value={value}>{value}</option>
            {/each}
          </select>
        </div>
        <div class="sort-control">
          <label for="metrics-batch-limit">{$t("top_batches_label")}</label>
          <select id="metrics-batch-limit" bind:value={metricsBatchLimit} onchange={() => loadMetrics()}>
            {#each metricsLimitOptions as value}
              <option value={value}>{value}</option>
            {/each}
          </select>
        </div>
      </div>
      <div class="small">
        {$t("metrics_status_line", { time: lastLoadedLabel(metricsLoadedAt), uptime: formatUptime(metricsSnapshot?.health?.uptime_seconds), version: metricsSnapshot?.server_version || "unknown" })}
      </div>

      {#if metricsLoading && !hasMetricsData(metricsSnapshot)}
        <div class="summary-empty">{$t("loading_metrics")}</div>
      {:else if metricsError && !hasMetricsData(metricsSnapshot)}
        <div class="notice">{$t("metrics_load_error", { error: metricsError })}</div>
        <button type="button" onclick={() => loadMetrics()}>{$t("retry")}</button>
      {:else if hasMetricsData(metricsSnapshot)}
        {@const throughputSeries = metricsSeriesValues(metricsSnapshot, metricsWindow, "throughput")}
        {@const failedSeries = metricsSeriesValues(metricsSnapshot, metricsWindow, "failed")}
        {@const querySeriesIPv4 = metricsSeriesValues(metricsSnapshot, metricsWindow, "dns_queries_ipv4_per_second")}
        {@const querySeriesIPv6 = metricsSeriesValues(metricsSnapshot, metricsWindow, "dns_queries_ipv6_per_second")}
        {@const cacheHitRateSeries = metricsSeriesValues(metricsSnapshot, metricsWindow, "dns_cache_hit_rate")}
        {@const queryBounds = sparklineBounds(querySeriesIPv4, querySeriesIPv6)}
        {@const severityTotals = metricsSnapshot?.quality?.severity?.totals || {}}

        {#if metricsError}
          <div class="notice">{$t("metrics_stale_error", { error: metricsError })}</div>
        {/if}

        <div class="metrics-card-group">
          <h4 class="metrics-group-heading">{$t("metrics_group_queue")}</h4>
          <div class="summary-grid metrics-summary-grid">
            <div class="summary-item" title={$t(metricsCardHelp.queue_depth)}>
              <span class="summary-label">{$t("metric_queue_depth")}</span>
              <span class="summary-count">{formatInteger(metricsSnapshot?.health?.queue_depth)}</span>
            </div>
            <div class="summary-item" title={$t(metricsCardHelp.in_flight_jobs)}>
              <span class="summary-label">{$t("metric_in_flight")}</span>
              <span class="summary-count">{formatInteger(metricsSnapshot?.health?.in_flight_jobs)}</span>
            </div>
            <div class="summary-item" title={$t(metricsCardHelp.jobs_per_minute)}>
              <span class="summary-label">{$t("metric_jobs_per_minute")}</span>
              <span class="summary-count">{formatInteger(Math.round(metricsJobsPerMinute(metricsSnapshot, metricsWindow)))}</span>
            </div>
          </div>
        </div>

        <div class="metrics-card-group">
          <h4 class="metrics-group-heading">{$t("metrics_group_dns")}</h4>
          <div class="summary-grid metrics-summary-grid">
            <div class="summary-item" title={$t(metricsCardHelp.dns_lookups_total)}>
              <span class="summary-label">{$t("metric_dns_lookups")}</span>
              <span class="summary-count">{formatCompactInteger((metricsSnapshot?.health?.dns_cache_hits || 0) + (metricsSnapshot?.health?.dns_cache_misses || 0))}</span>
            </div>
            <div class="summary-item" title={$t(metricsCardHelp.dns_cache_hits)}>
              <span class="summary-label">{$t("metric_cache_hits")}</span>
              <span class="summary-count">{formatCompactInteger(metricsSnapshot?.health?.dns_cache_hits)}</span>
            </div>
            <div class="summary-item" title={$t(metricsCardHelp.dns_cache_misses)}>
              <span class="summary-label">{$t("metric_cache_misses")}</span>
              <span class="summary-count">{formatCompactInteger(metricsSnapshot?.health?.dns_cache_misses)}</span>
            </div>
            <div class="summary-item" title={$t(metricsCardHelp.dns_cache_hit_rate)}>
              <span class="summary-label">{$t("metric_cache_hit_rate")}</span>
              <span class="summary-count">{formatPercent(metricsCacheHitRate(metricsSnapshot))}</span>
            </div>
            <div class="summary-item" title={$t(metricsCardHelp.dns_queries_ipv4_total)}>
              <span class="summary-label">{$t("metric_ipv4_queries")}</span>
              <span class="summary-count">{formatCompactInteger(metricsSnapshot?.health?.dns_queries_ipv4_total)}</span>
            </div>
            <div class="summary-item" title={$t(metricsCardHelp.dns_queries_ipv6_total)}>
              <span class="summary-label">{$t("metric_ipv6_queries")}</span>
              <span class="summary-count">{formatCompactInteger(metricsSnapshot?.health?.dns_queries_ipv6_total)}</span>
            </div>
          </div>
        </div>

        <div class="metrics-card-group">
          <h4 class="metrics-group-heading">{$t("metrics_group_jobs")}</h4>
          <div class="summary-grid metrics-summary-grid">
            <div class="summary-item jobs-finished" title={$t(metricsCardHelp.completed_total)}>
              <span class="summary-label">{$t("metric_completed")}</span>
              <span class="summary-count">{formatInteger(metricsSnapshot?.jobs?.completed_total)}</span>
            </div>
            <div class="summary-item failed-jobs" title={$t(metricsCardHelp.failed_total)}>
              <span class="summary-label">{$t("metric_failed")}</span>
              <span class="summary-count">{formatInteger(metricsFailedJobs(metricsSnapshot))}</span>
            </div>
            <div class="summary-item" title={$t(metricsCardHelp.success_rate)}>
              <span class="summary-label">{$t("metric_success_rate")}</span>
              <span class="summary-count">{formatPercent(metricsSnapshot?.quality?.outcomes?.success_rate)}</span>
            </div>
            <div class="summary-item" title={$t(metricsCardHelp.failed_rate)}>
              <span class="summary-label">{$t("metric_failure_rate")}</span>
              <span class="summary-count">{formatPercent(metricsSnapshot?.quality?.outcomes?.failed_rate)}</span>
            </div>
          </div>
        </div>

        <div class="metrics-card-group">
          <h4 class="metrics-group-heading">{$t("metrics_group_performance")}</h4>
          <div class="summary-grid metrics-summary-grid">
            <div class="summary-item" title={$t(metricsCardHelp.avg_job_duration)}>
              <span class="summary-label">{$t("metric_avg_duration")}</span>
              <span class="summary-count">{formatDurationMs(metricsSnapshot?.quality?.job_duration_ms?.avg)}</span>
            </div>
            <div class="summary-item" title={$t(metricsCardHelp.api_p90)}>
              <span class="summary-label">{$t("metric_api_p90")}</span>
              <span class="summary-count">{formatDurationMs(metricsTopAPIP90(metricsSnapshot))}</span>
            </div>
          </div>
        </div>

        <div class="metrics-card-group">
          <h4 class="metrics-group-heading">{$t("metrics_group_severity")}</h4>
          <div class="summary-grid metrics-summary-grid">
            {#each summaryLevels as level}
              <div class={`summary-item severity-${level.toLowerCase()}`} title={$t("help_severity_card", { level })}>
                <span class="summary-label">{level}</span>
                <span class="summary-count">{formatInteger(severityTotals[level])}</span>
              </div>
            {/each}
          </div>
        </div>

        <div class="metrics-trend-grid">
          <div class="metrics-trend-card">
            <div class="metrics-trend-head">
              <strong>{$t("metric_throughput")}</strong>
              <span>{formatInteger(seriesLast(throughputSeries))}{$t("per_bucket")}</span>
            </div>
            <svg class="sparkline" viewBox="0 0 260 66" role="img" aria-label={$t("aria_throughput_trend")}>
              <polyline points={sparklinePoints(throughputSeries)} />
            </svg>
          </div>
          <div class="metrics-trend-card">
            <div class="metrics-trend-head">
              <strong>{$t("metric_failures")}</strong>
              <span>{formatInteger(seriesLast(failedSeries))}{$t("per_bucket")}</span>
            </div>
            <svg class="sparkline sparkline-warn" viewBox="0 0 260 66" role="img" aria-label={$t("aria_failure_trend")}>
              <polyline points={sparklinePoints(failedSeries)} />
            </svg>
          </div>
          <div class="metrics-trend-card">
            <div class="metrics-trend-head">
              <strong>{$t("metric_dns_rate")}</strong>
              <span>{formatRate(seriesLast(querySeriesIPv4) + seriesLast(querySeriesIPv6))}</span>
            </div>
            <svg class="sparkline sparkline-queries" viewBox="0 0 260 66" role="img" aria-label={$t("aria_dns_trend")}>
              <polyline class="sparkline-ipv4" points={sparklinePoints(querySeriesIPv4, 260, 66, 6, queryBounds)} />
              <polyline class="sparkline-ipv6" points={sparklinePoints(querySeriesIPv6, 260, 66, 6, queryBounds)} />
            </svg>
            <div class="sparkline-legend">
              <span class="sparkline-legend-item">
                <span class="sparkline-legend-dot sparkline-legend-dot-ipv4" aria-hidden="true"></span>
                {$t("ipv4_label")} {formatRate(seriesLast(querySeriesIPv4))}
              </span>
              <span class="sparkline-legend-item">
                <span class="sparkline-legend-dot sparkline-legend-dot-ipv6" aria-hidden="true"></span>
                {$t("ipv6_label")} {formatRate(seriesLast(querySeriesIPv6))}
              </span>
            </div>
          </div>
          <div class="metrics-trend-card">
            <div class="metrics-trend-head">
              <strong>{$t("metric_cache_hit_rate")}</strong>
              <span>{formatPercent(seriesLast(cacheHitRateSeries))}</span>
            </div>
            <svg class="sparkline sparkline-cache" viewBox="0 0 260 66" role="img" aria-label={$t("aria_cache_hit_rate_trend")}>
              <polyline points={sparklinePoints(cacheHitRateSeries)} />
            </svg>
          </div>
        </div>

        <div class="metrics-tables">
          <div class="metrics-table-wrap">
            <h3>{$t("top_domains_heading")}</h3>
            {#if metricsDomainRows(metricsSnapshot).length === 0}
              <div class="summary-empty">{$t("no_domain_data")}</div>
            {:else}
              <table class="metrics-table">
                <thead>
                  <tr>
                    <th class="sortable-column" aria-sort={tableSortAria(metricsDomainsSortState, "domain")}><button class="table-sort-button" type="button" onclick={() => { metricsDomainsSortState = nextTableSort(metricsDomainsSortState, "domain"); }}><span>{$t("domain_col")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(metricsDomainsSortState, "domain")}</span></button></th>
                    <th class="sortable-column" aria-sort={tableSortAria(metricsDomainsSortState, "runs_total")}><button class="table-sort-button" type="button" onclick={() => { metricsDomainsSortState = nextTableSort(metricsDomainsSortState, "runs_total", "desc"); }}><span>{$t("runs_col")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(metricsDomainsSortState, "runs_total")}</span></button></th>
                    <th class="sortable-column" aria-sort={tableSortAria(metricsDomainsSortState, "last_status")}><button class="table-sort-button" type="button" onclick={() => { metricsDomainsSortState = nextTableSort(metricsDomainsSortState, "last_status"); }}><span>{$t("last_status_col")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(metricsDomainsSortState, "last_status")}</span></button></th>
                    <th class="sortable-column" aria-sort={tableSortAria(metricsDomainsSortState, "avg_duration_ms")}><button class="table-sort-button" type="button" onclick={() => { metricsDomainsSortState = nextTableSort(metricsDomainsSortState, "avg_duration_ms", "desc"); }}><span>{$t("avg_duration_col")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(metricsDomainsSortState, "avg_duration_ms")}</span></button></th>
                    <th class="sortable-column" aria-sort={tableSortAria(metricsDomainsSortState, "error_critical")}><button class="table-sort-button" type="button" onclick={() => { metricsDomainsSortState = nextTableSort(metricsDomainsSortState, "error_critical", "desc"); }}><span>{$t("error_critical_col")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(metricsDomainsSortState, "error_critical")}</span></button></th>
                  </tr>
                </thead>
                <tbody>
                  {#each sortedMetricsDomainRows as row}
                    <tr>
                      <td class="mono"><button class="ghost" type="button" style="padding: 0; font-family: monospace; text-align: left; border: none;" onclick={() => navigateToDomainByName(row.domain)}>{row.domain}</button></td>
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
            <h3>{$t("error_heavy_batches_heading")}</h3>
            {#if metricsBatchRows(metricsSnapshot).length === 0}
              <div class="summary-empty">{$t("no_batch_data")}</div>
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
                    <th class="sortable-column" aria-sort={tableSortAria(metricsBatchesSortState, "batch_id")}><button class="table-sort-button" type="button" onclick={() => { metricsBatchesSortState = nextTableSort(metricsBatchesSortState, "batch_id"); }}><span>{$t("batch_id_col")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(metricsBatchesSortState, "batch_id")}</span></button></th>
                    <th class="sortable-column" aria-sort={tableSortAria(metricsBatchesSortState, "processed_total")}><button class="table-sort-button" type="button" onclick={() => { metricsBatchesSortState = nextTableSort(metricsBatchesSortState, "processed_total", "desc"); }}><span>{$t("processed_col")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(metricsBatchesSortState, "processed_total")}</span></button></th>
                    <th class="sortable-column" aria-sort={tableSortAria(metricsBatchesSortState, "failed_expired")}><button class="table-sort-button" type="button" onclick={() => { metricsBatchesSortState = nextTableSort(metricsBatchesSortState, "failed_expired", "desc"); }}><span>{$t("failed_expired_col")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(metricsBatchesSortState, "failed_expired")}</span></button></th>
                    <th class="sortable-column" aria-sort={tableSortAria(metricsBatchesSortState, "canceled")}><button class="table-sort-button" type="button" onclick={() => { metricsBatchesSortState = nextTableSort(metricsBatchesSortState, "canceled", "desc"); }}><span>{$t("canceled_col")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(metricsBatchesSortState, "canceled")}</span></button></th>
                    <th class="sortable-column" aria-sort={tableSortAria(metricsBatchesSortState, "error_critical")}><button class="table-sort-button" type="button" onclick={() => { metricsBatchesSortState = nextTableSort(metricsBatchesSortState, "error_critical", "desc"); }}><span>{$t("error_critical_col")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(metricsBatchesSortState, "error_critical")}</span></button></th>
                  </tr>
                </thead>
                <tbody>
                  {#each sortedMetricsBatchRows as row}
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
        <div class="summary-empty">{$t("no_metrics_data")}</div>
      {/if}
    </div>
  {:else if activeTab === "settings"}
    <div class="grid" id="panel-settings" role="tabpanel" aria-labelledby="tab-settings" style="margin-top: 22px;">
      <div class="card reveal" style="--d: 0.34s; grid-column: 1 / -1;">
        <ProfileSettings onprofileschanged={handleProfilesChanged} />
      </div>
      <div class="card reveal" style="--d: 0.4s; grid-column: 1 / -1;">
        <ServerSettings />
      </div>
    </div>
  {/if}

  {#if statusMessage}
    <div class={`status-toast status-${statusTone === "ok" ? "ok" : "warn"} reveal`} role="status" aria-live="polite" style="--d: 0.12s;">
      <div><strong>{statusTone === "ok" ? $t("toast_ok") : $t("toast_warn")}:</strong> {statusMessage}</div>
      <button class="status-toast-close" type="button" aria-label={$t("dismiss_notification_aria")} onclick={clearStatus}>{$t("dismiss")}</button>
    </div>
  {/if}
  </main>
</div>
