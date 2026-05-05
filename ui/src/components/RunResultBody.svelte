<script>
  import { t } from "../i18n.js";
  import { normalizeLevel } from "../lib/jobUtils.js";
  import {
    moduleLevels,
    worstLevel,
    bannerClass,
    isNoticeOrAbove,
    formatSeconds,
    formatTimingMs,
    entryMessage,
    moduleId,
    summaryRows,
    okTestcaseCount,
    groupRawEntries,
  } from "../lib/result.js";

  let {
    result = null,
    nameserverTimingsEnabled = false,
    idPrefix = "",
  } = $props();

  let moduleOpen = $state({});
  let lastResultJobId = "";

  const moduleGroups = $derived(groupRawEntries(result?.raw));

  $effect(() => {
    const id = result?.job_id || "";
    if (id !== lastResultJobId) {
      lastResultJobId = id;
      moduleOpen = {};
    }
  });

  const toggleModule = (key) => {
    moduleOpen = { ...moduleOpen, [key]: !moduleOpen[key] };
  };

  const fullId = (key) => `${idPrefix}${moduleId(key)}`;
</script>

{#if result}
  {@const allEntries = result?.raw?.entries ?? []}
  {@const nsTimings = result?.nameserver_timings ?? []}
  {@const bannerCls = bannerClass(worstLevel(allEntries))}
  {#if allEntries.length}
    <div class="status-banner {bannerCls}">{$t(`result_status_${bannerCls}`)}</div>
  {/if}
  <div class="field-label">{$t("result_summary_label")}</div>
  {#if summaryRows(result.summary).length}
    <div class="summary-grid">
      {#each summaryRows(result.summary) as row (row.level)}
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
            aria-controls={fullId(group.key)}
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
            <div class="module-body" id={fullId(group.key)}>
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
{/if}
