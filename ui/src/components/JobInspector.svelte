<script>
  import { t } from "../i18n.js";
  import { formatTimestampLocal, prettyProfileJSON, formatDurationMs, formatJobTotalRuntime as formatJobTotalRuntimeRaw } from "../lib/format.js";
  import { progressPercent, isResultReadyStatus, isActiveJobStatus } from "../lib/jobUtils.js";
  import RunResultView from "./RunResultView.svelte";

  let {
    selectedJobId = $bindable(""),
    selectedJob = null,
    selectedJobResult = null,
    selectedRun = null,
    jobLoading = false,
    autoRefreshJob = $bindable(true),
    highlight = false,
    availableProfiles = [],
    scoringEnabled = false,
    nameserverTimingsEnabled = false,
    onRefresh = () => {},
    onSubmitJobId = () => {},
    onLoadResult = () => {},
    onNavigateDomain = () => {},
  } = $props();

  const formatJobTotalRuntime = (job) => formatJobTotalRuntimeRaw(job, isActiveJobStatus);

  const profileName = $derived(() => {
    const direct = String(selectedJob?.profile_name || selectedRun?.profile_name || "").trim();
    if (direct) return direct;
    const id = Number(selectedJob?.profile_id || selectedRun?.profile_id);
    if (!Number.isFinite(id) || id <= 0) return "";
    const match = availableProfiles.find((p) => p.id === id);
    return match?.name || `#${id}`;
  });

  const applyWidth = (node, value) => {
    node.style.width = value;
    return { update(v) { node.style.width = v; } };
  };
</script>

<div class="card reveal delay-26" class:highlight>
  <h2>{selectedJob && isResultReadyStatus(selectedJob.status) ? $t("run_inspector_heading") : $t("job_inspector_heading")}</h2>
  <div class="stack">
    <label for="job-id">{$t("job_id_label")}</label>
    <input id="job-id" type="text" placeholder="job_123" bind:value={selectedJobId} onchange={() => onSubmitJobId(selectedJobId)} />
  </div>
  <div class="row">
    <button onclick={onRefresh} disabled={jobLoading}>{jobLoading ? $t("loading") : $t("refresh")}</button>
    <button class="ghost" type="button" onclick={() => (autoRefreshJob = !autoRefreshJob)}>
      {autoRefreshJob ? $t("auto_refresh_on") : $t("auto_refresh_off")}
    </button>
  </div>
  {#if selectedJob}
    {@const resolvedProfileName = profileName()}
    <div class="kv">
      <span>{$t("status_label")}</span>
      <strong>{selectedJob.status} · {formatJobTotalRuntime(selectedJob)}</strong>
      <span>{$t("progress_label")}</span>
      <div class="progress" role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow={progressPercent(selectedJob)}>
        <div class="progress-bar" use:applyWidth={`${progressPercent(selectedJob)}%`}></div>
        <span class="progress-value">{progressPercent(selectedJob)}%</span>
      </div>
      <span>{$t("domain_label")}</span>
      <button class="ghost btn-text-mono" type="button" onclick={() => onNavigateDomain(selectedJob.domain)}>{selectedJob.domain}</button>
      {#if resolvedProfileName}
        <span>{$t("job_profile_label")}</span>
        <strong class="mono">{resolvedProfileName}</strong>
      {/if}
      <span>{$t("created_label")}</span>
      <strong>{formatTimestampLocal(selectedJob.created_at)}</strong>
      {#if selectedRun}
        <span>{$t("col_duration")}</span>
        <strong>{selectedRun.duration_ms != null ? formatDurationMs(selectedRun.duration_ms) : "-"}</strong>
        <span>{$t("col_entries")}</span>
        <strong>{selectedRun.entry_count ?? 0}</strong>
        <span>{$t("col_worst_level")}</span>
        <strong>{#if selectedRun.worst_level}<span class="badge level-{selectedRun.worst_level.toLowerCase()}">{selectedRun.worst_level}</span>{:else}-{/if}</strong>
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
    <div class="stack">
      <RunResultView result={selectedJobResult} {scoringEnabled} {nameserverTimingsEnabled} />
    </div>
  {/if}
  {#if selectedJob && !selectedJobResult && isResultReadyStatus(selectedJob.status)}
    <button class="ghost" type="button" onclick={onLoadResult}>
      {$t("load_result")}
    </button>
  {/if}
</div>
