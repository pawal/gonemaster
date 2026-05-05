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
  import BatchesPanel from "./panels/BatchesPanel.svelte";
  import SingleTestPanel from "./panels/SingleTestPanel.svelte";
  import SettingsPanel from "./panels/SettingsPanel.svelte";
  import { status, setStatus, clearStatus } from "./lib/status.svelte.js";
  import { initThemeFromStorage } from "./lib/theme.svelte.js";

  const logoSrc = `${import.meta.env.BASE_URL}gonemaster.svg`;

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

  // Batches tab state at App level: persisted filter state plus the
  // selected-batch pointer (used by URL/storage routing).
  let selectedBatchId = "";
  let batchSort = "started_at_desc";
  let batchPageSize = 20;
  let batchCursor = 0;
  let batchStatusFilter = "";
  let batchDomainFilter = "";
  let batchDeleteModalOpen = false;
  let batchDeleteModalId = "";
  let batchDeletedCounter = 0;
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
  let notifyOnJobComplete = false;
  let pendingPermission = null;

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
      if (!tagsLoaded) loadDomainTags();
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

  const handleSingleJobCreated = (jobId) => {
    selectedJobId = jobId;
    autoRefreshJob = true;
    notifyOnJobComplete = true;
    ensureNotificationPermission();
    jobInspectorHighlight = true;
    if (jobInspectorHighlightTimer) clearTimeout(jobInspectorHighlightTimer);
    jobInspectorHighlightTimer = setTimeout(() => {
      jobInspectorHighlight = false;
      jobInspectorHighlightTimer = null;
    }, 6000);
    setStatus($t("job_created", { id: jobId }), "ok");
    recentCursor = 0;
    loadJob(jobId);
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
    }
    batchDeletedCounter += 1;
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

  const startJobPolling = () => {
    if (jobPoller) clearInterval(jobPoller);
    if (!autoRefreshJob || !selectedJobId) return;
    jobPoller = setInterval(() => loadJob(selectedJobId, { silent: true }), 5000);
  };

  $: {
    autoRefreshJob;
    selectedJobId;
    startJobPolling();
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
      <SingleTestPanel
        {apiFetch}
        {setStatus}
        {availableProfiles}
        {profilesLoading}
        onJobCreated={handleSingleJobCreated}
      />
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
    <BatchesPanel
      {apiFetch}
      {setStatus}
      {ensureNotificationPermission}
      bind:selectedBatchId
      bind:batchSort
      bind:batchPageSize
      bind:batchStatusFilter
      bind:batchDomainFilter
      bind:batchCursor
      {availableTags}
      {availableProfiles}
      {profilesLoading}
      {tagCohortByName}
      {batchSortOptions}
      {batchStatuses}
      {listPageSizes}
      {batchDeletedCounter}
      onNavigateJob={navigateToJob}
      onOpenBatchDelete={openBatchDelete}
    />
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
    <SettingsPanel
      {settingsSubTab}
      {settingsSubTabs}
      onSetSubTab={setSettingsSubTab}
      onProfilesChanged={handleProfilesChanged}
    />
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

