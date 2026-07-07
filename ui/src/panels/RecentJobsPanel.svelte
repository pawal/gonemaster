<script>
  import { onMount } from "svelte";
  import { t } from "../i18n.js";
  import { progressPercent, hasRunningOrQueuedJobs } from "../lib/jobUtils.js";
  import { hasScore, chipGrade, chipScore, moduleLevels } from "../lib/result.js";
  import { normalizePageSize, normalizeCursor } from "../lib/persistence.js";
  import GradeChip from "../components/GradeChip.svelte";
  import { href } from "../lib/router.svelte.js";

  let {
    apiFetch,
    setStatus = () => {},
    severityFilters = [],
    jobSortOptions = [],
    listPageSizes = [10, 20, 50, 100],
    availableProfiles = [],
    scoringEnabled = false,
    jobSort = $bindable("started_at_desc"),
    severityFilter = $bindable("all"),
    jobBatchFilter = $bindable(""),
    recentDomainFilter = $bindable(""),
    recentPageSize = $bindable(20),
    recentCursor = $bindable(0),
    onNavigateJob = () => {},
    onNavigateBatch = () => {},
  } = $props();

  let jobs = $state([]);
  let jobsLoading = $state(false);
  let recentTotal = $state(0);
  let recentOffset = $state(0);
  let recentNextCursor = $state("");
  let recentPrevCursor = $state("");
  let autoRefreshRecent = $state(false);

  const normalizeRecentPageSize = (value) => normalizePageSize(value, listPageSizes, 20);

  const jobSeverityRows = (job) =>
    moduleLevels
      .map((level) => ({ level, count: Number(job?.severity_totals?.[level] || 0) }))
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

  const filteredJobs = $derived(jobs.filter((job) => matchesSeverityFilter(job)));

  const normalizeOptionalProfileID = (value) => {
    const parsed = Number(value);
    if (!Number.isFinite(parsed) || parsed <= 0) return null;
    return parsed;
  };
  const jobProfileName = (job, run = null) => {
    const direct = String(job?.profile_name || run?.profile_name || "").trim();
    if (direct) return direct;
    const id = normalizeOptionalProfileID(job?.profile_id || run?.profile_id);
    if (!id) return "";
    const match = availableProfiles.find((profile) => profile.id === id);
    return match?.name || `#${id}`;
  };

  const applyWidth = (node, value) => {
    node.style.width = value;
    return { update(v) { node.style.width = v; } };
  };

  async function loadJobs(options = {}) {
    const { resetCursor = false } = options;
    if (resetCursor) recentCursor = 0;
    jobsLoading = true;
    try {
      const params = new URLSearchParams({
        limit: String(normalizeRecentPageSize(recentPageSize)),
        sort: jobSort,
      });
      const cursor = normalizeCursor(recentCursor);
      if (cursor > 0) params.set("cursor", String(cursor));
      const normalizedBatchID = jobBatchFilter.trim();
      if (normalizedBatchID) params.set("batch_id", normalizedBatchID);
      const normalizedDomain = recentDomainFilter.trim();
      if (normalizedDomain) params.set("domain", normalizedDomain);
      if (severityFilter !== "all") params.set("severity", severityFilter);
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
  }

  async function applyRecentFilters() {
    recentCursor = 0;
    await loadJobs({ resetCursor: true });
  }

  async function clearRecentFilters() {
    jobBatchFilter = "";
    recentDomainFilter = "";
    await loadJobs({ resetCursor: true });
  }

  async function goToRecentCursor(cursor) {
    const parsed = Number(cursor);
    recentCursor = Number.isFinite(parsed) && parsed > 0 ? parsed : 0;
    await loadJobs();
  }

  onMount(() => {
    loadJobs();
  });

  $effect(() => {
    if (!autoRefreshRecent) return;
    const handle = setInterval(() => loadJobs(), 7000);
    return () => clearInterval(handle);
  });
</script>

<div class="card reveal delay-34 panel-mt" id="panel-recent" role="tabpanel" aria-labelledby="tab-recent">
  <h2>{$t("recent_tests_heading")}</h2>
  <div class="row">
    <button class="ghost" type="button" onclick={() => loadJobs()} disabled={jobsLoading}>
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
        {#each listPageSizes as pageSize}
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
      {$t("showing_jobs", { shown: filteredJobs.length, total: recentTotal, offset: recentOffset || 0 })}
    </span>
  </div>
  <div class="list">
    {#if jobs.length === 0}
      <div class="small">{$t("no_jobs")}</div>
    {:else if filteredJobs.length === 0}
      <div class="small">{$t("no_jobs_severity")}</div>
    {:else}
      {#each filteredJobs as job (job.id)}
        <div class="list-item clickable" onclick={() => onNavigateJob(job.id)} onkeydown={(e) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            onNavigateJob(job.id);
          }
        }} role="button" tabindex="0">
          <div class="list-item-main">
            <div class="job-headline">
              <a
                class="mono job-id-link"
                href={href("single", { jobId: job.id })}
                onclick={(e) => { e.preventDefault(); e.stopPropagation(); onNavigateJob(job.id); }}
              >{job.id}</a>
              <span class="small">{job.domain} - {job.status}</span>
              {#if jobSeverityRows(job).length}
                {#each jobSeverityRows(job) as entry (entry.level)}
                  <span class={`level-pill severity-${entry.level.toLowerCase()}`}>{entry.level} {entry.count}</span>
                {/each}
              {:else if job.severity_totals !== undefined}
                <span class="level-pill severity-info">INFO</span>
              {/if}
              {#if scoringEnabled && hasScore(job)}
                <GradeChip grade={chipGrade(job)} score={chipScore(job)} />
              {/if}
            </div>
            {#if job.batch_id}
              <div class="small mono">{$t("batch_prefix")} <a
                href={href("batches", { batchId: job.batch_id })}
                onclick={(e) => { e.preventDefault(); e.stopPropagation(); onNavigateBatch(job.batch_id); }}
              >{job.batch_id}</a></div>
            {/if}
            {#if jobProfileName(job)}
              <div class="small">{$t("job_profile_label")}: <span class="mono">{jobProfileName(job)}</span></div>
            {/if}
            <div class="progress compact list-progress" role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow={progressPercent(job)}>
              <div class="progress-bar" use:applyWidth={`${progressPercent(job)}%`}></div>
              <span class="progress-value">{progressPercent(job)}%</span>
            </div>
          </div>
        </div>
      {/each}
    {/if}
  </div>
</div>
