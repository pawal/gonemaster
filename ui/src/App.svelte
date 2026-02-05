<script>
  import { onDestroy } from "svelte";

  let statusMessage = "";
  let statusTone = "";

  let singleDomain = "";
  let singleSubmitting = false;
  let createdJobId = "";

  let batchDomains = "";
  let batchSubmitting = false;
  let createdBatchId = "";

  let jobs = [];
  let jobsLoading = false;
  let filteredJobs = [];
  let severityFilter = "all";
  let jobSort = "started_at_desc";
  let jobBatchFilter = "";

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
  let persistenceReady = false;
  let persistenceSignature = "";
  let initialized = false;

  const apiPrefix = "/api/v1";
  const persistedStateKey = "gonemaster.ui.state.v1";
  const persistedQueryKeys = [
    "r_sort",
    "r_sev",
    "r_batch",
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
  let activeTab = "recent";
  const tabs = [
    { id: "recent", label: "Recent Jobs" },
    { id: "batches", label: "Batch Jobs" }
  ];

  const setStatus = (message, tone = "") => {
    statusMessage = message;
    statusTone = tone;
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
  const batchPageSizes = [10, 20, 50, 100];

  const isKnownSort = (value, options) => options.some((option) => option.id === value);
  const isKnownSeverityFilter = (value) => severityFilters.some((option) => option.id === value);
  const isKnownBatchStatus = (value) => batchStatuses.includes(value);
  const normalizeBatchPageSize = (value) => {
    const parsed = Number(value);
    if (Number.isFinite(parsed) && batchPageSizes.includes(parsed)) {
      return parsed;
    }
    return 20;
  };
  const normalizeCursor = (value) => {
    const parsed = Number(value);
    if (Number.isFinite(parsed) && parsed >= 0) {
      return Math.floor(parsed);
    }
    return 0;
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
    const normalizedBatchID = selectedBatchId.trim();
    if (normalizedBatchID) {
      params.set("b_id", normalizedBatchID);
    }
    if (batchSort !== "started_at_desc") {
      params.set("b_sort", batchSort);
    }
    if (normalizeBatchPageSize(batchPageSize) !== 20) {
      params.set("b_limit", String(normalizeBatchPageSize(batchPageSize)));
    }
    if (normalizeCursor(batchCursor) > 0) {
      params.set("b_cursor", String(normalizeCursor(batchCursor)));
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
        selectedBatchId: normalizedBatchID,
        batchSort,
        batchPageSize: normalizeBatchPageSize(batchPageSize),
        batchCursor: normalizeCursor(batchCursor),
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
    if (tab === "recent" || tab === "jobs") return "recent";
    if (tab === "batches" || tab === "batch") return "batches";
    return "";
  };

  const setTab = (tab) => {
    const next = normalizeTab(tab) || "recent";
    activeTab = next;
    const nextHash = `#/${next}`;
    if (window.location.hash !== nextHash) {
      window.history.replaceState(
        null,
        "",
        `${window.location.pathname}${window.location.search}${nextHash}`
      );
    }
  };

  const updateTabFromHash = () => {
    const hash = window.location.hash || "";
    const value = hash.replace(/^#\/?/, "");
    const next = normalizeTab(value) || "recent";
    activeTab = next;
    if (!hash) {
      window.history.replaceState(
        null,
        "",
        `${window.location.pathname}${window.location.search}#/${next}`
      );
    }
  };

  const loadJobs = async () => {
    jobsLoading = true;
    try {
      const params = new URLSearchParams({
        limit: "20",
        sort: jobSort
      });
      const normalizedBatchID = jobBatchFilter.trim();
      if (normalizedBatchID) {
        params.set("batch_id", normalizedBatchID);
      }
      const list = await apiFetch(`/jobs?${params.toString()}`);
      jobs = list.items || [];
    } catch (error) {
      setStatus(`Failed to load jobs: ${error.message}`, "warn");
    } finally {
      jobsLoading = false;
    }
  };

  const clearRecentBatchFilter = async () => {
    jobBatchFilter = "";
    await loadJobs();
  };

  const submitSingle = async () => {
    const normalizedDomain = normalizeDomainInput(singleDomain);
    if (!normalizedDomain) {
      setStatus("Domain is required.", "warn");
      return;
    }
    singleSubmitting = true;
    createdJobId = "";
    try {
      const payload = {
        domain: normalizedDomain
      };

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
      await loadJobs();
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
      await loadJobs();
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
      if (["succeeded", "failed", "canceled"].includes(job.status)) {
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

  const loadBatch = async (batchId = selectedBatchId, options = {}) => {
    if (!batchId) return;
    const { resetCursor = false } = options;
    if (resetCursor) {
      batchCursor = 0;
    }
    batchLoading = true;
    try {
      const params = batchQueryParams();
      selectedBatch = await apiFetch(`/batches/${batchId}?${params.toString()}`);
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

  $: {
    autoRefreshJob;
    selectedJobId;
    startJobPolling();
  }

  $: {
    autoRefreshBatch;
    selectedBatchId;
    batchLoading;
    startBatchPolling();
  }

  $: if (
    autoRefreshJob &&
    selectedJob &&
    selectedJob.id === selectedJobId &&
    (selectedJob.progress === 100 || ["succeeded", "failed", "canceled"].includes(selectedJob.status))
  ) {
    autoRefreshJob = false;
    jobInspectorHighlight = false;
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
    persistenceReady = true;
    window.addEventListener("hashchange", updateTabFromHash);
    loadJobs();
    if (activeTab === "batches" && selectedBatchId) {
      loadBatch(selectedBatchId);
    }
  };

  initializeApp();

  onDestroy(() => {
    if (jobPoller) clearInterval(jobPoller);
    if (batchPoller) clearInterval(batchPoller);
    if (jobInspectorHighlightTimer) clearTimeout(jobInspectorHighlightTimer);
    window.removeEventListener("hashchange", updateTabFromHash);
  });
</script>

<main>
  <header class="reveal" style="--d: 0.05s">
    <h1>Gonemaster</h1>
    <p class="subtitle">
      Launch single or batch domain jobs, watch progress, and inspect results from the embedded server UI.
    </p>
  </header>

  {#if statusMessage}
    <div class="notice reveal" style="--d: 0.12s; margin-bottom: 22px;">
      <strong>{statusTone === "ok" ? "OK" : "Heads up"}:</strong> {statusMessage}
    </div>
  {/if}

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

  <section class="grid" style="margin-top: 22px;">
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
      <button on:click={submitSingle} disabled={singleSubmitting}>
        {singleSubmitting ? "Submitting..." : "Run Single Job"}
      </button>
      {#if createdJobId}
        <div class="small">Created job: <span class="mono">{createdJobId}</span></div>
      {/if}
    </div>

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
  </section>

  {#if activeTab === "recent"}
    <div class="grid" id="panel-recent" role="tabpanel" aria-labelledby="tab-recent" style="margin-top: 22px;">
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
            <strong>{selectedJob.status}</strong>
            <span>Progress</span>
            <div class="progress" role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow={selectedJob.progress || 0}>
              <div class="progress-bar" style={`width: ${selectedJob.progress || 0}%`}></div>
              <span class="progress-value">{selectedJob.progress || 0}%</span>
            </div>
            <span>Domain</span>
            <strong class="mono">{selectedJob.domain}</strong>
            <span>Created</span>
            <strong>{new Date(selectedJob.created_at).toLocaleString()}</strong>
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
                {#each summaryRows(selectedJobResult.summary) as row}
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
                {#each moduleGroups as group}
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
        {#if selectedJob && !selectedJobResult && ["succeeded", "failed", "canceled"].includes(selectedJob.status)}
          <button class="ghost" type="button" on:click={() => loadJobResult()}>
            Load result payload
          </button>
        {/if}
      </div>
    </div>

    <section class="card reveal" style="--d: 0.34s; margin-top: 22px;">
      <h2>Recent Jobs</h2>
      <div class="row">
        <button class="ghost" type="button" on:click={loadJobs} disabled={jobsLoading}>
          {jobsLoading ? "Refreshing..." : "Refresh list"}
        </button>
        <div class="sort-control">
          <label for="recent-sort">Sort</label>
          <select id="recent-sort" bind:value={jobSort} on:change={loadJobs}>
            {#each jobSortOptions as option}
              <option value={option.id}>{option.label}</option>
            {/each}
          </select>
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
                loadJobs();
              }
            }}
          />
        </div>
        <div class="row">
          <button class="ghost" type="button" on:click={loadJobs} disabled={jobsLoading}>Apply filters</button>
          <button class="ghost" type="button" on:click={clearRecentBatchFilter} disabled={jobsLoading}>Clear</button>
        </div>
      </div>
      <div class="severity-filter-bar" role="group" aria-label="Severity filters">
        {#each severityFilters as filter}
          <button
            type="button"
            class={`severity-filter ${severityFilter === filter.id ? "active" : ""}`}
            on:click={() => (severityFilter = filter.id)}
          >
            {filter.label}
          </button>
        {/each}
      </div>
      <div class="list">
        {#if jobs.length === 0}
          <div class="small">No jobs yet. Run a single or batch job above.</div>
        {:else if filteredJobs.length === 0}
          <div class="small">No jobs match the selected severity filter.</div>
        {:else}
          {#each filteredJobs as job}
            <div class="list-item">
              <div>
                <div class="mono">{job.id}</div>
                <div class="small">{job.domain} - {job.status}</div>
                {#if job.batch_id}
                  <div class="small mono">Batch: {job.batch_id}</div>
                {/if}
                <div class="job-severity-tags">
                  {#if jobSeverityRows(job).length}
                    {#each jobSeverityRows(job) as entry}
                      <span class={`level-pill severity-${entry.level.toLowerCase()}`}>{entry.level} {entry.count}</span>
                    {/each}
                  {:else}
                    <span class="small">No severity entries.</span>
                  {/if}
                </div>
                <div class="progress compact" role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow={job.progress || 0}>
                  <div class="progress-bar" style={`width: ${job.progress || 0}%`}></div>
                  <span class="progress-value">{job.progress || 0}%</span>
                </div>
              </div>
              <button class="ghost" type="button" on:click={() => {
                selectedJobId = job.id;
                loadJob(job.id);
              }}>Inspect</button>
            </div>
          {/each}
        {/if}
      </div>
    </section>
  {:else if activeTab === "batches"}
    <div class="grid" id="panel-batches" role="tabpanel" aria-labelledby="tab-batches" style="margin-top: 22px;">
      <div class="card reveal" style="--d: 0.3s">
        <h2>Batch Inspector</h2>
        <div class="stack">
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
          <button on:click={() => loadBatch()} disabled={batchLoading}>
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
              <option value={10}>10</option>
              <option value={20}>20</option>
              <option value={50}>50</option>
              <option value={100}>100</option>
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
            <strong>{new Date(selectedBatch.created_at).toLocaleString()}</strong>
            <span>Status counts</span>
            <strong class="mono">{JSON.stringify(selectedBatch.status_counts)}</strong>
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
                {#each selectedBatch.items as item}
                  <div class="list-item">
                    <div>
                      <div class="mono">{item.id}</div>
                      <div class="small">{item.domain} - {item.status}</div>
                    </div>
                    <button class="ghost" type="button" on:click={() => {
                      selectedJobId = item.id;
                      loadJob(item.id);
                      setTab("recent");
                    }}>Inspect</button>
                  </div>
                {/each}
              {/if}
            </div>
          </div>
        {/if}
      </div>
    </div>
  {/if}
</main>
