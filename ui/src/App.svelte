<script>
  import { onMount, onDestroy } from "svelte";

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

  const apiPrefix = "/api/v1";

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
  const summaryRows = (summary) => {
    const levels = summary?.levels || {};
    return summaryLevels
      .map((level) => ({
        level,
        count: Number(levels[level] || 0)
      }))
      .filter((entry) => entry.count > 0);
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
      window.history.replaceState(null, "", nextHash);
    }
  };

  const updateTabFromHash = () => {
    const hash = window.location.hash || "";
    const value = hash.replace(/^#\/?/, "");
    const next = normalizeTab(value) || "recent";
    activeTab = next;
    if (!hash) {
      window.history.replaceState(null, "", `#/${next}`);
    }
  };

  const loadJobs = async () => {
    jobsLoading = true;
    try {
      const list = await apiFetch("/jobs?limit=20");
      jobs = list.items || [];
    } catch (error) {
      setStatus(`Failed to load jobs: ${error.message}`, "warn");
    } finally {
      jobsLoading = false;
    }
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
      await loadBatch(response.batch_id);
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

  const loadBatch = async (batchId = selectedBatchId) => {
    if (!batchId) return;
    batchLoading = true;
    try {
      selectedBatch = await apiFetch(`/batches/${batchId}`);
    } catch (error) {
      setStatus(`Failed to load batch: ${error.message}`, "warn");
      selectedBatch = null;
    } finally {
      batchLoading = false;
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

  onMount(() => {
    updateTabFromHash();
    window.addEventListener("hashchange", updateTabFromHash);
    loadJobs();
  });

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

  <nav class="tabs" role="tablist" aria-label="Job views">
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
  </nav>

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
    <section class="grid" id="panel-recent" role="tabpanel" aria-labelledby="tab-recent" style="margin-top: 22px;">
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
    </section>

    <section class="card reveal" style="--d: 0.34s; margin-top: 22px;">
      <h2>Recent Jobs</h2>
      <div class="row">
        <button class="ghost" type="button" on:click={loadJobs} disabled={jobsLoading}>
          {jobsLoading ? "Refreshing..." : "Refresh list"}
        </button>
      </div>
      <div class="list">
        {#if jobs.length === 0}
          <div class="small">No jobs yet. Run a single or batch job above.</div>
        {:else}
          {#each jobs as job}
            <div class="list-item">
              <div>
                <div class="mono">{job.id}</div>
                <div class="small">{job.domain} - {job.status}</div>
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
    <section class="grid" id="panel-batches" role="tabpanel" aria-labelledby="tab-batches" style="margin-top: 22px;">
      <div class="card reveal" style="--d: 0.3s">
        <h2>Batch Inspector</h2>
        <div class="stack">
          <label for="batch-id">Batch ID</label>
          <input id="batch-id" type="text" placeholder="batch_123" bind:value={selectedBatchId} on:change={() => loadBatch()} />
        </div>
        <div class="row">
          <button on:click={() => loadBatch()} disabled={batchLoading}>
            {batchLoading ? "Loading..." : "Refresh"}
          </button>
          <button class="ghost" type="button" on:click={() => (autoRefreshBatch = !autoRefreshBatch)}>
            {autoRefreshBatch ? "Auto refresh: on" : "Auto refresh: off"}
          </button>
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
            <div class="list">
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
            </div>
          </div>
        {/if}
      </div>
    </section>
  {/if}
</main>
