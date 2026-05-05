<script>
  import { t } from "../i18n.js";
  import { formatTimestampLocal, prettyProfileJSON, formatJobTotalRuntime as formatJobTotalRuntimeRaw } from "../lib/format.js";
  import { progressPercent, isResultReadyStatus, isActiveJobStatus, normalizeLevel } from "../lib/jobUtils.js";
  import {
    moduleLevels,
    CAT_ORDER,
    CAT_LABELS,
    BONUS_HIDDEN,
    worstLevel,
    bannerClass,
    isNoticeOrAbove,
    hasScore,
    chipGrade,
    chipScore,
    resultScore,
    formatSeconds,
    formatTimingMs,
    entryMessage,
    moduleId,
    summaryRows,
    okTestcaseCount,
    groupRawEntries,
  } from "../lib/result.js";

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
    onLoadResult = () => {},
    onNavigateDomain = () => {},
  } = $props();

  let moduleOpen = $state({});
  let lastResultJobId = "";

  const moduleGroups = $derived(groupRawEntries(selectedJobResult?.raw));

  $effect(() => {
    const id = selectedJobResult?.job_id || "";
    if (id !== lastResultJobId) {
      lastResultJobId = id;
      moduleOpen = {};
    }
  });

  const toggleModule = (key) => {
    moduleOpen = { ...moduleOpen, [key]: !moduleOpen[key] };
  };

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
    <input id="job-id" type="text" placeholder="job_123" bind:value={selectedJobId} onchange={onRefresh} />
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
        <strong>{selectedRun.duration_ms != null ? selectedRun.duration_ms + " ms" : "-"}</strong>
        <span>{$t("col_entries")}</span>
        <strong>{selectedRun.entry_count ?? 0}</strong>
        <span>{$t("col_worst_level")}</span>
        <strong><span class="badge level-{(selectedRun.worst_level || '').toLowerCase()}">{selectedRun.worst_level || "-"}</span></strong>
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
      {#if nameserverTimingsEnabled && nsTimings.length}
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
    <button class="ghost" type="button" onclick={onLoadResult}>
      {$t("load_result")}
    </button>
  {/if}
</div>
