<script>
  import { onMount, onDestroy } from "svelte";

  const levels = ["", "DEBUG", "INFO", "WARN", "ERROR"]; // empty means server default
  let apiBase = localStorage.getItem("gm_api_base") ?? "";
  let statusMessage = "";
  let statusTone = "";

  let singleDomain = "";
  let singleTests = "";
  let singleMinLevel = "";
  let singleOverrides = "";
  let singleSubmitting = false;
  let createdJobId = "";

  let batchDomains = "";
  let batchTests = "";
  let batchMinLevel = "";
  let batchOverrides = "";
  let batchSubmitting = false;
  let createdBatchId = "";

  let jobs = [];
  let jobsLoading = false;

  let selectedJobId = "";
  let selectedJob = null;
  let selectedJobResult = null;
  let jobLoading = false;
  let autoRefreshJob = true;
  let jobPoller = null;

  let selectedBatchId = "";
  let selectedBatch = null;
  let batchLoading = false;
  let autoRefreshBatch = false;
  let batchPoller = null;

  const saveApiBase = (value) => {
    apiBase = value;
    localStorage.setItem("gm_api_base", apiBase);
  };

  const normalizeBase = () => (apiBase || "").trim().replace(/\/$/, "");
  const buildUrl = (path) => {
    const base = normalizeBase();
    if (!base) return path;
    return `${base}${path}`;
  };

  const setStatus = (message, tone = "") => {
    statusMessage = message;
    statusTone = tone;
  };

  const parseTests = (value) =>
    value
      .split(/[,\n]/)
      .map((entry) => entry.trim())
      .filter(Boolean);

  const parseOverrides = (value) => {
    if (!value.trim()) return undefined;
    return JSON.parse(value);
  };

  const apiFetch = async (path, options = {}) => {
    const url = buildUrl(path);
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
    if (!singleDomain.trim()) {
      setStatus("Domain is required.", "warn");
      return;
    }
    singleSubmitting = true;
    createdJobId = "";
    try {
      const payload = {
        domain: singleDomain.trim()
      };
      const tests = parseTests(singleTests);
      if (tests.length) payload.tests = tests;
      if (singleMinLevel) payload.min_level = singleMinLevel;
      const overrides = parseOverrides(singleOverrides);
      if (overrides) payload.profile_overrides = overrides;

      const job = await apiFetch("/jobs", {
        method: "POST",
        body: JSON.stringify(payload)
      });
      createdJobId = job.id;
      selectedJobId = job.id;
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
      .map((entry) => entry.trim())
      .filter(Boolean);
    if (!domains.length) {
      setStatus("Provide at least one domain for the batch.", "warn");
      return;
    }
    batchSubmitting = true;
    createdBatchId = "";
    try {
      const payload = { domains };
      const tests = parseTests(batchTests);
      if (tests.length) payload.tests = tests;
      if (batchMinLevel) payload.min_level = batchMinLevel;
      const overrides = parseOverrides(batchOverrides);
      if (overrides) payload.profile_overrides = overrides;

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

  const loadJob = async (jobId = selectedJobId) => {
    if (!jobId) return;
    jobLoading = true;
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
      jobLoading = false;
    }
  };

  const loadJobResult = async (jobId = selectedJobId) => {
    if (!jobId) return;
    try {
      selectedJobResult = await apiFetch(`/jobs/${jobId}/result`);
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
    jobPoller = setInterval(() => loadJob(), 5000);
  };

  const startBatchPolling = () => {
    if (batchPoller) clearInterval(batchPoller);
    if (!autoRefreshBatch || !selectedBatchId) return;
    batchPoller = setInterval(() => loadBatch(), 7000);
  };

  $: {
    autoRefreshJob;
    selectedJobId;
    jobLoading;
    startJobPolling();
  }

  $: {
    autoRefreshBatch;
    selectedBatchId;
    batchLoading;
    startBatchPolling();
  }

  onMount(() => {
    loadJobs();
  });

  onDestroy(() => {
    if (jobPoller) clearInterval(jobPoller);
    if (batchPoller) clearInterval(batchPoller);
  });
</script>

<main>
  <header class="reveal" style="--d: 0.05s">
    <h1>Gonemaster Control Room</h1>
    <p class="subtitle">
      Launch single or batch domain jobs, watch progress, and inspect results from the embedded server UI.
    </p>
    <div class="row">
      <span class="badge">Embedded UI</span>
      <span class="badge">Single &amp; Batch</span>
      <span class="badge">Auto refresh</span>
    </div>
  </header>

  <section class="card reveal" style="--d: 0.12s">
    <h2>API Connection</h2>
    <p>Leave blank to use the current server host.</p>
    <div class="row">
      <input
        type="text"
        placeholder="https://gonemaster.example.com"
        bind:value={apiBase}
        on:change={(event) => saveApiBase(event.target.value)}
      />
      <button class="ghost" type="button" on:click={() => saveApiBase("")}>Use Local</button>
    </div>
    {#if statusMessage}
      <div class="notice">
        <strong>{statusTone === "ok" ? "OK" : "Heads up"}:</strong> {statusMessage}
      </div>
    {/if}
  </section>

  <section class="grid" style="margin-top: 22px;">
    <div class="card reveal" style="--d: 0.18s">
      <h2>Single Job</h2>
      <div class="stack">
        <label>Domain</label>
        <input type="text" placeholder="example.com" bind:value={singleDomain} />
      </div>
      <div class="stack">
        <label>Tests (comma or newline)</label>
        <input type="text" placeholder="dns, http, tls" bind:value={singleTests} />
      </div>
      <div class="stack">
        <label>Min Level</label>
        <select bind:value={singleMinLevel}>
          {#each levels as level}
            <option value={level}>{level || "Server default"}</option>
          {/each}
        </select>
      </div>
      <div class="stack">
        <label>Profile overrides (JSON)</label>
        <textarea placeholder={'{"timeout": 5}'} bind:value={singleOverrides}></textarea>
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
        <label>Domains (one per line)</label>
        <textarea placeholder="example.com\nexample.org" bind:value={batchDomains}></textarea>
      </div>
      <div class="stack">
        <label>Tests (comma or newline)</label>
        <input type="text" placeholder="dns, http, tls" bind:value={batchTests} />
      </div>
      <div class="stack">
        <label>Min Level</label>
        <select bind:value={batchMinLevel}>
          {#each levels as level}
            <option value={level}>{level || "Server default"}</option>
          {/each}
        </select>
      </div>
      <div class="stack">
        <label>Profile overrides (JSON)</label>
        <textarea placeholder={'{"timeout": 5}'} bind:value={batchOverrides}></textarea>
      </div>
      <button class="secondary" on:click={submitBatch} disabled={batchSubmitting}>
        {batchSubmitting ? "Submitting..." : "Run Batch"}
      </button>
      {#if createdBatchId}
        <div class="small">Created batch: <span class="mono">{createdBatchId}</span></div>
      {/if}
    </div>
  </section>

  <section class="grid" style="margin-top: 22px;">
    <div class="card reveal" style="--d: 0.26s">
      <h2>Job Inspector</h2>
      <div class="stack">
        <label>Job ID</label>
        <input type="text" placeholder="job_123" bind:value={selectedJobId} on:change={() => loadJob()} />
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
          <strong>{selectedJob.progress}%</strong>
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
          <label>Result summary</label>
          <pre class="mono">{JSON.stringify(selectedJobResult.summary || {}, null, 2)}</pre>
          <label>Raw payload</label>
          <pre class="mono">{JSON.stringify(selectedJobResult.raw || {}, null, 2)}</pre>
        </div>
      {/if}
      {#if selectedJob && !selectedJobResult && ["succeeded", "failed", "canceled"].includes(selectedJob.status)}
        <button class="ghost" type="button" on:click={() => loadJobResult()}>
          Load result payload
        </button>
      {/if}
    </div>

    <div class="card reveal" style="--d: 0.3s">
      <h2>Batch Inspector</h2>
      <div class="stack">
        <label>Batch ID</label>
        <input type="text" placeholder="batch_123" bind:value={selectedBatchId} on:change={() => loadBatch()} />
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
          <label>Jobs</label>
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
                }}>Inspect</button>
              </div>
            {/each}
          </div>
        </div>
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
              <div class="small">{job.domain} - {job.status} - {job.progress}%</div>
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
</main>
