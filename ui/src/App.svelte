<script>
  import { onMount } from "svelte";
  import { t, locale, loadCatalog } from "./i18n.js";
  import { apiCall } from "./lib/api.js";
  import {
    formatPercent,
    formatInteger,
    formatCompactInteger,
    formatDurationMs,
    formatRate,
    formatUptime,
    parseTimestamp,
    formatTimestampLocal,
    formatBatchTotalRuntime,
    prettyProfileJSON,
    formatSnapshotSlugPreview,
    formatJobTotalRuntime as formatJobTotalRuntimeRaw,
    formatBatchStatusCounts as formatBatchStatusCountsRaw,
  } from "./lib/format.js";
  import {
    activeJobStatuses,
    resultReadyStatuses,
    LEVEL_ORDER,
    normalizeStatus,
    isActiveJobStatus,
    isResultReadyStatus,
    progressPercent,
    hasActiveBatchJobs,
    hasRunningOrQueuedJobs,
    normalizeLevel,
    severityRank,
  } from "./lib/jobUtils.js";
  import {
    compareText,
    compareNumber,
    compareTimestamp,
    compareSeverity,
    nextTableSort,
    tableSortIndicator,
    tableSortAria,
    sortItems,
  } from "./lib/sort.js";
  import {
    persistedStateKey,
    persistedQueryKeys,
    normalizePageSize,
    normalizeCursor,
    decodeStateFromURL,
    decodeStateFromStorage,
    encodeStateToURLParams,
    serializeStateForStorage,
  } from "./lib/persistence.js";
  import {
    moduleLevels,
    CAT_ORDER,
    CAT_LABELS,
    BONUS_HIDDEN,
    hasScore,
    chipGrade,
    chipScore,
  } from "./lib/result.js";
  import ProfileSettings from "./ProfileSettings.svelte";
  import ServerSettings from "./ServerSettings.svelte";
  import ScoringSettings from "./ScoringSettings.svelte";
  import AnalysisCohorts from "./AnalysisCohorts.svelte";
  import BatchDeleteModal from "./BatchDeleteModal.svelte";
  import StatusBanner from "./components/StatusBanner.svelte";
  import ThemeToggle from "./components/ThemeToggle.svelte";
  import JobInspector from "./components/JobInspector.svelte";
  import RunResultBody from "./components/RunResultBody.svelte";
  import MetricsPanel from "./panels/MetricsPanel.svelte";
  import DomainsPanel from "./panels/DomainsPanel.svelte";
  import TagsPanel from "./panels/TagsPanel.svelte";
  import RecentJobsPanel from "./panels/RecentJobsPanel.svelte";
  import { status, setStatus, clearStatus } from "./lib/status.svelte.js";
  import { initThemeFromStorage } from "./lib/theme.svelte.js";

  const logoSrc = `${import.meta.env.BASE_URL}gonemaster.svg`;

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
  // Snapshot-intent batch flag; `touched` suppresses auto-defaulting
  // once the admin has chosen.
  let batchSnapshotIntent = false;
  let batchSnapshotIntentTouched = false;

  // Recent-jobs filter state owned at App level so it survives tab unmounts
  // and feeds the URL/storage persistence wiring.
  let severityFilter = "all";
  let jobSort = "started_at_desc";
  let jobBatchFilter = "";
  let recentDomainFilter = "";
  let recentPageSize = 20;
  let recentCursor = 0;

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
  let batchDeleteModalOpen = false;
  let batchDeleteModalId = "";
  let batchDeletedCounter = 0;
  let activeBatches = [];
  let activeBatchesLoading = false;
  let activeBatchesPoller = null;
  let queuePaused = false;
  let queuePauseToggling = false;
  let autoRefreshMetrics = true;
  let metricsWindow = "1h";
  let metricsDomainLimit = 10;
  let metricsBatchLimit = 10;
  let persistenceReady = false;
  let persistenceSignature = "";
  let initialized = false;
  // Server-controlled feature flag: whether scoring UI is shown. Defaults to
  // true so scoring is visible before the features response arrives.
  let scoringEnabled = true;
  let nameserverTimingsEnabled = true;
  let undelegatedRowCounter = 0;
  let notifyOnJobComplete = false;
  let notifyOnBatchComplete = false;
  let pendingPermission = null;

  // Snapshot checkbox visibility + auto-default.
  $: batchCohortForTag = batchFromTag ? tagCohortByName.get(batchFromTag) : null;
  $: snapshotCheckboxVisible = !!(batchCohortForTag && batchCohortForTag.analysis_enabled);
  $: if (!snapshotCheckboxVisible && batchSnapshotIntent) {
    batchSnapshotIntent = false;
    batchSnapshotIntentTouched = false;
  }
  $: if (snapshotCheckboxVisible && !batchSnapshotIntentTouched) {
    batchSnapshotIntent = batchFromTagMode;
  }
  $: batchSnapshotPartial = batchSnapshotIntent && !batchFromTagMode;
  $: batchSnapshotSlugPreview = snapshotCheckboxVisible && batchSnapshotIntent
    ? formatSnapshotSlugPreview()
    : "";

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

  let activeTab = "single";
  const tabs = [
    { id: "single", labelKey: "tab_single" },
    { id: "recent", labelKey: "tab_recent" },
    { id: "domains", labelKey: "tab_domains" },
    { id: "tags", labelKey: "tab_tags" },
    { id: "cohorts", labelKey: "tab_cohorts" },
    { id: "batches", labelKey: "tab_batches" },
    { id: "metrics", labelKey: "tab_metrics" },
    { id: "settings", labelKey: "tab_settings" }
  ];

  let settingsSubTab = "system";
  const settingsSubTabs = [
    { id: "system", labelKey: "settings_subtab_system" },
    { id: "profiles", labelKey: "settings_subtab_profiles" },
    { id: "scoring", labelKey: "settings_subtab_scoring" }
  ];

  let selectedDomain = null;
  let availableTags = [];
  let tagsLoaded = false;
  let availableProfiles = [];
  let profilesLoaded = false;
  let profilesLoading = false;
  let singleProfileId = "";

  // Tags tab state owned at the App level (cross-tab pointers + cohort map).
  let tagCohortByName = new Map();
  let selectedTag = null;
  const apiFetch = async (path, options) => apiCall(apiPrefix, path, options);

  const summaryLevels = moduleLevels;
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
  const isKnownSort = (value, options) => options.some((option) => option.id === value);
  const isKnownSeverityFilter = (value) => severityFilters.some((option) => option.id === value);
  const isKnownBatchStatus = (value) => batchStatuses.includes(value);
  const normalizeBatchPageSize = (value) => normalizePageSize(value, listPageSizes, 20);
  const normalizeRecentPageSize = (value) => normalizePageSize(value, listPageSizes, 20);
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
      if (!Number.isNaN(parsed.getTime())) parts.push(parsed.toLocaleString("sv-SE"));
    }
    if (option.tag) parts.push(`[${option.tag}]`);
    return parts.join(" - ");
  };

  const persistenceValidators = {
    isKnownJobSort: (v) => isKnownSort(v, jobSortOptions),
    isKnownBatchSort: (v) => isKnownSort(v, batchSortOptions),
    isKnownSeverityFilter,
    isKnownBatchStatus,
    normalizeRecentPageSize,
    normalizeBatchPageSize,
  };

  const readStateFromURL = () =>
    decodeStateFromURL(new URLSearchParams(window.location.search), persistenceValidators);

  const readStateFromStorage = () => {
    try {
      const storage = typeof window === "undefined" ? null : window.localStorage;
      if (!storage) return null;
      return decodeStateFromStorage(storage.getItem(persistedStateKey), persistenceValidators);
    } catch (_) {
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
    const state = {
      jobSort,
      severityFilter,
      jobBatchFilter: jobBatchFilter.trim(),
      recentDomainFilter: recentDomainFilter.trim(),
      recentPageSize: normalizeRecentPageSize(recentPageSize),
      recentCursor: normalizeCursor(recentCursor),
      selectedBatchId: selectedBatchId.trim(),
      batchSort,
      batchPageSize: normalizeBatchPageSize(batchPageSize),
      batchCursor: normalizeCursor(batchCursor),
      batchStatusFilter,
      batchDomainFilter: batchDomainFilter.trim(),
    };

    const params = encodeStateToURLParams(new URLSearchParams(window.location.search), state);
    const hash = window.location.hash || `#/${activeTab}`;
    const search = params.toString();
    window.history.replaceState(window.history.state, "", `${window.location.pathname}${search ? `?${search}` : ""}${hash}`);

    try {
      const storage = typeof window === "undefined" ? null : window.localStorage;
      if (!storage) return;
      storage.setItem(persistedStateKey, serializeStateForStorage(state));
    } catch (_) {
      // Ignore storage issues in restricted browser contexts.
    }
  };
  const formatJobTotalRuntime = (job) => formatJobTotalRuntimeRaw(job, isActiveJobStatus);
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
  const formatBatchStatusCounts = (statusCounts) => formatBatchStatusCountsRaw(statusCounts, normalizeStatus);

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

  const applyWidth = (node, value) => {
    node.style.width = value;
    return { update(v) { node.style.width = v; } };
  };
  const entryMeta = (entry) => [entry?.testcase, entry?.tag].filter(Boolean).join(" · ");

  const normalizeTab = (value) => {
    const tab = String(value || "").replace(/^\/+/, "").toLowerCase();
    if (tab === "single" || tab === "job" || tab === "jobs" || tab === "home") return "single";
    if (tab === "recent" || tab === "tests") return "recent";
    if (tab === "domains" || tab === "domain") return "domains";
    if (tab === "tags" || tab === "tag") return "tags";
    if (tab === "cohorts" || tab === "cohort" || tab === "analysis") return "cohorts";
    if (tab === "batches" || tab === "batch") return "batches";
    if (tab === "metrics" || tab === "metric") return "metrics";
    if (tab === "settings" || tab === "setting") return "settings";
    return "";
  };

  const normalizeSettingsSubTab = (value) => {
    const sub = String(value || "").toLowerCase();
    if (sub === "system" || sub === "profiles" || sub === "scoring") return sub;
    return "system";
  };

  const settingsHash = (subTab) => {
    const sub = normalizeSettingsSubTab(subTab);
    return sub === "system" ? "#/settings" : `#/settings/${sub}`;
  };

  const setTab = (tab, { replace = false } = {}) => {
    const next = normalizeTab(tab) || "single";
    const changed = activeTab !== next;
    activeTab = next;
    const nextHash = next === "settings" ? settingsHash(settingsSubTab) : `#/${next}`;
    const url = `${window.location.pathname}${window.location.search}${nextHash}`;
    const state = {
      tab: next,
      settingsSubTab: next === "settings" ? settingsSubTab : null,
      domain: null,
      tag: null,
      jobId: null,
    };
    if (!changed || replace) {
      if (window.location.hash !== nextHash) window.history.replaceState(state, "", url);
    } else {
      window.history.pushState(state, "", url);
    }
    if (changed && status.message) {
      clearStatus();
    }
    loadDataForTab(next);
  };

  // Single source of truth for per-tab data loading. Called from both setTab
  // (tab click) and initializeApp (first mount / page reload) so the two
  // entry points can't drift - all new per-tab loads go here, not at the
  // call sites.
  const loadDataForTab = (tab) => {
    if (tab === "single" || tab === "tags" || tab === "batches") {
      loadProfiles();
    }
    if (tab === "domains") {
      if (!tagsLoaded) loadDomainTags();
    } else if (tab === "tags") {
      loadTagCohortMap();
    } else if (tab === "batches") {
      loadRecentBatchOptions();
      loadActiveBatches();
      fetchQueueStatus();
      if (!tagsLoaded) loadDomainTags();
      if (selectedBatchId) {
        loadBatch(selectedBatchId);
      }
    }
  };

  const navigateToJob = (jobId) => {
    selectedJobId = jobId;
    activeTab = "single";
    const hash = `#/single/${encodeURIComponent(jobId)}`;
    window.history.pushState({ tab: "single", domain: null, tag: null, jobId }, "", `${window.location.pathname}${window.location.search}${hash}`);
    if (status.message) clearStatus();
    loadJob(jobId);
  };

  const navigateToDomainDetail = (d) => {
    activeTab = "domains";
    selectedDomain = d;
    const hash = `#/domains/${encodeURIComponent(d.name)}`;
    window.history.pushState({ tab: "domains", domain: d, tag: null, jobId: null }, "", `${window.location.pathname}${window.location.search}${hash}`);
    if (status.message) clearStatus();
  };

  const navigateToTagDetail = (tag) => {
    activeTab = "tags";
    selectedTag = tag;
    const hash = `#/tags/${encodeURIComponent(tag.name)}`;
    window.history.pushState({ tab: "tags", domain: null, tag, jobId: null }, "", `${window.location.pathname}${window.location.search}${hash}`);
    if (status.message) clearStatus();
  };

  // Guard flag: when popstate fires, a hashchange event also fires for the
  // same navigation.  updateTabFromHash must skip that duplicate because
  // onPopState already restored the full state (domain, tag, jobId) from
  // history - updateTabFromHash would clobber it with nulls.
  let popStateHandled = false;

  const setSettingsSubTab = (subTab) => {
    const next = normalizeSettingsSubTab(subTab);
    const changed = settingsSubTab !== next;
    settingsSubTab = next;
    if (activeTab !== "settings") return;
    const nextHash = settingsHash(next);
    const url = `${window.location.pathname}${window.location.search}${nextHash}`;
    const state = { tab: "settings", settingsSubTab: next, domain: null, tag: null, jobId: null };
    if (!changed) {
      if (window.location.hash !== nextHash) window.history.replaceState(state, "", url);
    } else {
      window.history.pushState(state, "", url);
    }
  };

  const onPopState = (e) => {
    popStateHandled = true;
    const state = e.state;
    if (!state) { updateTabFromHash(); return; }
    activeTab = state.tab || "single";
    if (activeTab === "settings") {
      settingsSubTab = normalizeSettingsSubTab(state.settingsSubTab);
    }
    selectedDomain = state.domain ?? null;
    selectedTag = state.tag ?? null;
    if (state.jobId) {
      selectedJobId = state.jobId;
      loadJob(state.jobId);
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
    // Legacy redirect: cohort management used to live under
    // #/settings/analysis. The sub-tab is now a top-level tab, so map
    // stale bookmarks forward.
    let next;
    if (segment === "settings" && (parts[1] || "").toLowerCase() === "analysis") {
      next = "cohorts";
    } else {
      next = normalizeTab(segment) || "single";
    }
    activeTab = next;
    const jobId = (next === "single" && parts[1]) ? decodeURIComponent(parts[1]) : null;
    if (jobId) {
      selectedJobId = jobId;
      loadJob(jobId);
    }
    let settingsSub = null;
    if (next === "settings") {
      settingsSub = normalizeSettingsSubTab(parts[1]);
      settingsSubTab = settingsSub;
    }
    window.history.replaceState(
      {
        tab: next,
        settingsSubTab: settingsSub,
        domain: null,
        tag: null,
        jobId: jobId || undefined,
      },
      "",
      `${window.location.pathname}${window.location.search}${hash || `#/${next}`}`
    );
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
    if (batchSnapshotIntent) payload.snapshot_intent = true;
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
      // Silently ignore - active batches is supplementary.
    } finally {
      activeBatchesLoading = false;
    }
  };

  const fetchQueueStatus = async () => {
    try {
      const snapshot = await apiFetch("/metrics?window=1h&include=health");
      queuePaused = !!snapshot?.health?.queue_paused;
    } catch (_) {
      // Non-critical - silently ignore.
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

  const handleTagProfileChanged = (tagName, profileID) => {
    availableTags = availableTags.map((tag) =>
      tag.name === tagName ? { ...tag, default_profile_id: profileID } : tag
    );
  };

  const loadTagCohortMap = async () => {
    try {
      const data = await apiFetch("/analysis/cohorts");
      const cohorts = Array.isArray(data) ? data : [];
      tagCohortByName = new Map(cohorts.map((c) => [c.source_tag, c]));
    } catch (_) {
      tagCohortByName = new Map();
    }
  };

  const openBatchFromTagRow = async (batchId) => {
    const id = (batchId || "").trim();
    if (!id) return;
    selectedBatchId = id;
    await setTab("batches");
    await loadBatch(id, { resetCursor: true });
  };

  const openBatchDelete = (batchId) => {
    const id = (batchId || "").trim();
    if (!id) return;
    batchDeleteModalId = id;
    batchDeleteModalOpen = true;
  };

  const closeBatchDelete = () => {
    batchDeleteModalOpen = false;
    batchDeleteModalId = "";
  };

  const handleBatchDeleted = async (deletedId) => {
    if (selectedBatchId === deletedId) {
      selectedBatchId = "";
      selectedBatch = null;
    }
    if (selectedRecentBatch === deletedId) {
      selectedRecentBatch = "";
    }
    batchDeletedCounter += 1;
    await loadRecentBatchOptions();
    await loadActiveBatches();
  };

  const loadFeatures = async () => {
    try {
      const data = await apiFetch("/features");
      if (data && typeof data.show_score_admin === "boolean") {
        scoringEnabled = data.show_score_admin;
      }
      if (data && typeof data.show_nameserver_timings_admin === "boolean") {
        nameserverTimingsEnabled = data.show_nameserver_timings_admin;
      }
    } catch (_) {
      // Keep feature flags enabled on failure (fail-open for admin UI).
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

  const startActiveBatchesPolling = () => {
    if (activeBatchesPoller) clearInterval(activeBatchesPoller);
    if (activeTab !== "batches") return;
    activeBatchesPoller = setInterval(() => { loadActiveBatches(); fetchQueueStatus(); }, 7000);
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
    activeTab;
    startActiveBatchesPolling();
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
    initThemeFromStorage();

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
    loadDataForTab(activeTab);
  };

  let initialAnimationDone = false;

  onMount(() => {
    initializeApp();
    // Reveal animations (CSS: .reveal @ 0.6s + staggered --d delays up to ~0.5s)
    // are intentional on first paint but feel slow on every subsequent tab switch.
    // Disable them once the initial intro has had a chance to play.
    const revealTimer = setTimeout(() => {
      initialAnimationDone = true;
    }, 1200);
    return () => {
      clearTimeout(revealTimer);
      if (jobPoller) clearInterval(jobPoller);
      if (batchPoller) clearInterval(batchPoller);
      if (activeBatchesPoller) clearInterval(activeBatchesPoller);
      if (jobInspectorHighlightTimer) clearTimeout(jobInspectorHighlightTimer);
      window.removeEventListener("hashchange", updateTabFromHash);
      window.removeEventListener("popstate", onPopState);
    };
  });
</script>

<div class="app-header">
  <header class="reveal delay-05">
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
      <ThemeToggle />
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

  <main class:no-reveal={initialAnimationDone}>
  {#if activeTab === "single"}
    <div class="grid panel-mt" id="panel-single" role="tabpanel" aria-labelledby="tab-single">
      <div class="card reveal delay-18">
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

      <JobInspector
        bind:selectedJobId
        {selectedJob}
        {selectedJobResult}
        {selectedRun}
        {jobLoading}
        bind:autoRefreshJob
        highlight={jobInspectorHighlight}
        {availableProfiles}
        {scoringEnabled}
        {nameserverTimingsEnabled}
        onRefresh={() => loadJob()}
        onLoadResult={() => loadJobResult()}
        onNavigateDomain={navigateToDomainByName}
      />
    </div>
  {:else if activeTab === "recent"}
    <RecentJobsPanel
      {apiFetch}
      {setStatus}
      {severityFilters}
      {jobSortOptions}
      {listPageSizes}
      {availableProfiles}
      {scoringEnabled}
      bind:jobSort
      bind:severityFilter
      bind:jobBatchFilter
      bind:recentDomainFilter
      bind:recentPageSize
      bind:recentCursor
      onNavigateJob={navigateToJob}
    />
  {:else if activeTab === "domains"}
    <DomainsPanel
      {apiFetch}
      {setStatus}
      bind:selectedDomain
      {availableTags}
      {scoringEnabled}
      {nameserverTimingsEnabled}
      {resultLocale}
      onNavigateJob={navigateToJob}
    />
  {:else if activeTab === "tags"}
    <TagsPanel
      {apiFetch}
      {setStatus}
      {clearStatus}
      bind:selectedTag
      {availableProfiles}
      {profilesLoading}
      {tagCohortByName}
      {scoringEnabled}
      onNavigateDomainDetail={navigateToDomainDetail}
      onSetTab={setTab}
      onOpenBatchDelete={openBatchDelete}
      onOpenBatchFromTagRow={openBatchFromTagRow}
      onTagProfileChanged={handleTagProfileChanged}
      onTagsListChanged={loadDomainTags}
    />
  {:else if activeTab === "cohorts"}
    <div class="grid panel-mt" id="panel-cohorts" role="tabpanel" aria-labelledby="tab-cohorts">
      <div class="card reveal delay-22 grid-span-full">
        <AnalysisCohorts onDeleteBatch={openBatchDelete} refreshSignal={batchDeletedCounter} />
      </div>
    </div>
  {:else if activeTab === "batches"}
    <div class="grid panel-mt" id="panel-batches" role="tabpanel" aria-labelledby="tab-batches">
      <div class="card reveal delay-22">
        <h2>{$t("batch_jobs_heading")}</h2>
        <div class="toolbar-row no-wrap mb-half">
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
        {#if snapshotCheckboxVisible}
          <div class="stack batch-snapshot-field">
            <label class="batch-snapshot-check">
              <input
                type="checkbox"
                bind:checked={batchSnapshotIntent}
                onchange={() => { batchSnapshotIntentTouched = true; }}
              />
              <span>{$t("batch_snapshot_label")}</span>
            </label>
            <div class="small">{$t("batch_snapshot_hint")}</div>
            {#if batchSnapshotIntent}
              <div class="small mono">
                {$t("batch_snapshot_slug_preview", { slug: batchSnapshotSlugPreview })}
              </div>
            {/if}
            {#if batchSnapshotPartial}
              <div class="notice notice-warn" role="status" aria-live="polite">
                {$t("batch_snapshot_partial_warning")}
              </div>
            {/if}
          </div>
        {/if}
        <button class="secondary" onclick={submitBatch} disabled={batchSubmitting}>
          {batchSubmitting ? $t("submitting") : $t("run_batch")}
        </button>
        {#if createdBatchId}
          <div class="small">{$t("created_batch_prefix")} <span class="mono">{createdBatchId}</span></div>
        {/if}
      </div>

      <div class="card reveal delay-26" data-testid="active-batches-card">
        <div class="row row-toolbar-end">
          <h2 class="m-zero">{$t("active_batches_heading")}</h2>
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

      <div class="card reveal delay-30">
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
          <button
            class="warn"
            type="button"
            onclick={() => openBatchDelete(selectedBatchId)}
            disabled={!selectedBatchId || batchLoading}
          >
            {$t("batch_delete_button")}
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
                        <div class="progress-bar" use:applyWidth={`${progressPercent(item)}%`}></div>
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
    <MetricsPanel
      {apiFetch}
      {setStatus}
      onNavigateDomain={navigateToDomainByName}
      bind:autoRefreshMetrics
      bind:metricsWindow
      bind:metricsDomainLimit
      bind:metricsBatchLimit
    />
  {:else if activeTab === "settings"}
    <div class="grid panel-settings" id="panel-settings" role="tabpanel" aria-labelledby="tab-settings">
      <div class="settings-subtabs" role="tablist" aria-label={$t("settings_subtabs_aria")}>
        {#each settingsSubTabs as subTab}
          <button
            class={`settings-subtab ${settingsSubTab === subTab.id ? "active" : ""}`}
            type="button"
            role="tab"
            id={`settings-subtab-${subTab.id}`}
            aria-selected={settingsSubTab === subTab.id}
            aria-controls={`settings-subpanel-${subTab.id}`}
            onclick={() => setSettingsSubTab(subTab.id)}
          >
            {$t(subTab.labelKey)}
          </button>
        {/each}
      </div>
      {#if settingsSubTab === "system"}
        <div class="card reveal delay-34 grid-span-full" id="settings-subpanel-system" role="tabpanel" aria-labelledby="settings-subtab-system">
          <ServerSettings />
        </div>
      {:else if settingsSubTab === "profiles"}
        <div class="card reveal delay-34 grid-span-full" id="settings-subpanel-profiles" role="tabpanel" aria-labelledby="settings-subtab-profiles">
          <ProfileSettings onprofileschanged={handleProfilesChanged} />
        </div>
      {:else if settingsSubTab === "scoring"}
        <div class="card reveal delay-34 grid-span-full" id="settings-subpanel-scoring" role="tabpanel" aria-labelledby="settings-subtab-scoring">
          <ScoringSettings />
        </div>
      {/if}
    </div>
  {/if}

  <StatusBanner />

  </main>
</div>

<BatchDeleteModal
  open={batchDeleteModalOpen}
  batchId={batchDeleteModalId}
  onClose={closeBatchDelete}
  onDeleted={handleBatchDeleted}
  setStatus={setStatus}
/>

<style>
  .panel-mt { margin-top: 22px; }
  .panel-settings { margin-top: 0; gap: 10px; }
  .mt-half { margin-top: 0.5rem; }
  .mt-one { margin-top: 1rem; }
  .mt-1-25 { margin-top: 1.25rem; }
  .mt-1-5 { margin-top: 1.5rem; }
  .mb-half { margin-bottom: 0.5rem; }
  .mb-3-4 { margin-bottom: 0.75rem; }
  .mb-one { margin-bottom: 1rem; }
  .mb-12 { margin-bottom: 12px; }
  .m-zero { margin: 0; }
  .ml-quarter { margin-left: 0.25rem; }
  .heading-tight { margin-top: -0.25rem; margin-bottom: 0.75rem; }
  .text-right { text-align: right; }
  .row-clickable { cursor: pointer; }

  .toolbar,
  .toolbar-row {
    display: flex;
    gap: 0.5rem;
    flex-wrap: wrap;
  }
  .toolbar-row.gap-1 { gap: 1rem; }
  .toolbar-row.no-wrap { flex-wrap: nowrap; }
  .toolbar-row.align-end { align-items: flex-end; }

  .row-toolbar-end {
    justify-content: space-between;
    align-items: center;
  }

  .pagination {
    margin-top: 0.5rem;
    display: flex;
    gap: 0.5rem;
    align-items: center;
  }

  .header-with-meta {
    display: flex;
    align-items: baseline;
    gap: 0.6rem;
    flex-wrap: wrap;
    margin-top: 0.5rem;
    margin-bottom: 0.75rem;
  }

  .fb-160-shrink { flex: 0 1 160px; }
  .fb-180-shrink { flex: 0 1 180px; }
  .fb-160-grow { flex: 1 1 160px; }
  .fb-180-grow { flex: 1 1 180px; }
  .fb-240-grow-2 { flex: 2 1 240px; }

  .btn-text-mono {
    padding: 0;
    font-family: monospace;
    text-align: left;
    border: none;
  }
  .input-fluid {
    width: 100%;
    box-sizing: border-box;
  }
  .config-form {
    max-width: 440px;
    margin-bottom: 1rem;
  }
</style>
