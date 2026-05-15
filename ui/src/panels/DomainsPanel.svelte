<script>
  import { onMount } from "svelte";
  import { t } from "../i18n.js";
  import { formatTimestampLocal } from "../lib/format.js";
  import {
    sortItems,
    nextTableSort,
    tableSortIndicator,
    tableSortAria,
    compareNumber,
    compareText,
    compareTimestamp,
    compareSeverity,
  } from "../lib/sort.js";
  import {
    CAT_ORDER,
    CAT_LABELS,
    BONUS_HIDDEN,
    hasScore,
    chipGrade,
    chipScore,
  } from "../lib/result.js";
  import RunResultBody from "../components/RunResultBody.svelte";

  let {
    apiFetch,
    setStatus = () => {},
    selectedDomain = $bindable(null),
    availableTags = [],
    scoringEnabled = false,
    nameserverTimingsEnabled = false,
    resultLocale = "",
    onNavigateJob = () => {},
  } = $props();

  let domains = $state([]);
  let domainsLoading = $state(false);
  let domainsTotal = $state(0);
  let domainsOffset = $state(0);
  const domainsLimit = 50;
  let domainNameFilter = $state("");
  let domainTagFilter = $state("");
  let domainLevelFilter = $state("");
  let domainsSortState = $state({ key: "", direction: "asc" });

  let domainRuns = $state([]);
  let domainRunsLoading = $state(false);
  let domainRunsTotal = $state(0);
  let domainRunsOffset = $state(0);
  const domainRunsLimit = 20;
  let domainRunsSortState = $state({ key: "", direction: "asc" });

  let domainResubmitting = $state(false);
  let selectedDomainRunId = $state(null);
  let selectedDomainRunResult = $state(null);
  let domainRunResultLoading = $state(false);

  // Track the last domain whose runs we loaded so we don't reload when
  // selectedDomain is mutated unrelated to detail-view navigation.
  let lastLoadedDomainId = null;

  const domainLevel = (d) => d?.latest_level || (d?.latest_run_at ? "INFO" : "");

  const GRADE_COLORS = { "A+": "var(--grade-aplus)", "A": "var(--grade-a)", "B": "var(--grade-b)", "C": "var(--grade-c)", "D": "var(--grade-d)", "F": "var(--grade-f)" };
  const gradeBarColor = (grade) => GRADE_COLORS[grade] ?? "var(--grade-a)";
  const applyBarStyle = (node, params) => {
    const apply = ({ pct, color, delay }) => {
      node.style.setProperty("--bar-pct", `${pct}%`);
      node.style.setProperty("--bar-color", color);
      node.style.animationDelay = `${delay}ms`;
    };
    apply(params);
    return { update(p) { apply(p); } };
  };

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

  const sortedDomains = $derived(
    sortItems(domains, domainsSortState, {
      name: (left, right) => compareText(left?.name, right?.name),
      tags: (left, right) => compareText(left?.tags?.join(", "), right?.tags?.join(", ")),
      latest_level: (left, right) => compareSeverity(domainLevel(left), domainLevel(right)),
      latest_score: (left, right) => compareNumber(left?.latest_score, right?.latest_score),
      latest_run_at: (left, right) => compareTimestamp(left?.latest_run_at, right?.latest_run_at),
      run_count: (left, right) => compareNumber(left?.run_count, right?.run_count),
    }, (left, right) => compareText(left?.name, right?.name))
  );

  const sortedDomainRuns = $derived(
    sortItems(domainRuns, domainRunsSortState, {
      id: (left, right) => compareText(left?.id, right?.id),
      finished_at: (left, right) => compareTimestamp(left?.finished_at, right?.finished_at),
      worst_level: (left, right) => compareSeverity(left?.worst_level, right?.worst_level),
      score: (left, right) => compareNumber(chipScore(left), chipScore(right)),
      duration_ms: (left, right) => compareNumber(left?.duration_ms, right?.duration_ms),
      entry_count: (left, right) => compareNumber(left?.entry_count, right?.entry_count),
    }, (left, right) => compareText(left?.id, right?.id))
  );

  async function loadDomains(options = {}) {
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
  }

  async function loadDomainRuns(options = {}) {
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
  }

  async function loadDomainRunResult(runId) {
    selectedDomainRunId = runId;
    selectedDomainRunResult = null;
    domainRunResultLoading = true;
    try {
      const locale = resultLocale ? `?locale=${encodeURIComponent(resultLocale)}` : "";
      selectedDomainRunResult = await apiFetch(`/jobs/${runId}/result${locale}`);
    } catch (_) {
      // result not available for this run
    } finally {
      domainRunResultLoading = false;
    }
  }

  async function retestDomain() {
    if (!selectedDomain) return;
    domainResubmitting = true;
    try {
      const job = await apiFetch("/jobs", {
        method: "POST",
        body: JSON.stringify({ domain: selectedDomain.name }),
      });
      setStatus($t("job_created", { id: job.id }), "ok");
      onNavigateJob(job.id);
    } catch (error) {
      setStatus($t("error_create_job", { error: error.message }), "warn");
    } finally {
      domainResubmitting = false;
    }
  }

  const sortDomains = (key, defaultDir = "asc") => {
    domainsSortState = nextTableSort(domainsSortState, key, defaultDir);
    loadDomains({ reset: true });
  };

  const sortDomainRuns = (key, defaultDir = "asc") => {
    domainRunsSortState = nextTableSort(domainRunsSortState, key, defaultDir);
    loadDomainRuns({ reset: true });
  };

  const navigateToDomainDetail = (d) => {
    selectedDomain = d;
  };

  // React to selectedDomain changes from outside (e.g. App-level navigateToDomainByName,
  // popstate restoring an in-history detail view, or our back button).
  $effect(() => {
    const id = selectedDomain?.id ?? null;
    if (id === lastLoadedDomainId) return;
    lastLoadedDomainId = id;
    if (id == null) {
      domainRuns = [];
      domainRunsOffset = 0;
      selectedDomainRunResult = null;
      selectedDomainRunId = null;
    } else {
      domainRunsOffset = 0;
      loadDomainRuns();
    }
  });

  // Reload the open run result when locale changes.
  let lastLocale;
  $effect(() => {
    const currentLocale = resultLocale;
    if (currentLocale === lastLocale) return;
    const wasInitialized = lastLocale !== undefined;
    lastLocale = currentLocale;
    if (wasInitialized && selectedDomainRunResult && selectedDomainRunId) {
      loadDomainRunResult(selectedDomainRunId);
    }
  });

  onMount(() => {
    if (!selectedDomain) {
      loadDomains();
    }
  });
</script>

<div class="card reveal delay-34 panel-mt" id="panel-domains" role="tabpanel" aria-labelledby="tab-domains">
  {#if selectedDomain}
    <div>
      <button class="secondary small" onclick={() => { selectedDomain = null; }}>{$t("back_to_domains")}</button>
      <div class="header-with-meta">
        <h2 class="mono m-zero">{selectedDomain.name}</h2>
        {#if selectedDomain.tags && selectedDomain.tags.length > 0}
          {#each selectedDomain.tags as tag}
            <span class="badge">{tag}</span>
          {/each}
        {/if}
      </div>
      <div class="kv mb-one">
        <span>{$t("col_latest_level")}</span>
        <span>{#if domainLevel(selectedDomain)}<span class="badge level-{domainLevel(selectedDomain).toLowerCase()}">{domainLevel(selectedDomain)}</span>{:else}-{/if}</span>
        <span>{$t("col_latest_run_at")}</span>
        <strong>{selectedDomain.latest_run_at ? formatTimestampLocal(selectedDomain.latest_run_at) : "-"}</strong>
        <span>{$t("col_run_count")}</span>
        <strong>{selectedDomain.run_count ?? 0}</strong>
      </div>
      <button class="secondary" onclick={retestDomain} disabled={domainResubmitting}>
        {domainResubmitting ? $t("submitting") : $t("retest_domain")}
      </button>

      {#if domainRunResultLoading}
        <p class="muted mt-one">{$t("loading")}</p>
      {:else if selectedDomainRunResult}
        {@const sc = selectedDomainRunResult?.score}
        <div class="stack mt-1-25">
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
                        <div class="score-cat-bar" use:applyBarStyle={{ pct: res.tested === false ? 0 : res.score, color: gradeBarColor(sc.grade), delay: i * 60 }}></div>
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
          <RunResultBody result={selectedDomainRunResult} {nameserverTimingsEnabled} idPrefix="dm-" />
        </div>
      {/if}

      <h3 class="mt-1-5">{$t("domain_run_history_heading")}</h3>
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
                class={`row-clickable ${selectedDomainRunId === run.id ? "run-row-selected" : ""}`}
                onclick={() => loadDomainRunResult(run.id)}
                role="button"
                tabindex="0"
                onkeydown={(e) => { if (e.key === "Enter" || e.key === " ") loadDomainRunResult(run.id); }}
              >
                <td class="run-id-cell" title={run.id}>{run.id}</td>
                <td>{run.finished_at ? run.finished_at.slice(0, 16).replace("T", " ") : "-"}</td>
                <td><span class="badge level-{(run.worst_level || 'info').toLowerCase()}">{run.worst_level || "INFO"}</span></td>
                {#if scoringEnabled}<td>{#if hasScore(run)}<span class="grade-chip"><span class="grade-chip-letter" data-grade={chipGrade(run)}>{chipGrade(run)}</span><span class="grade-chip-score">{chipScore(run)}</span></span>{:else}-{/if}</td>{/if}
                <td>{run.duration_ms != null ? run.duration_ms + "ms" : "-"}</td>
                <td>{run.entry_count ?? 0}</td>
              </tr>
            {/each}
          </tbody>
        </table>
        <div class="pagination">
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
    <div class="toolbar mb-3-4">
      <input
        type="search"
        placeholder={$t("domains_search_placeholder")}
        bind:value={domainNameFilter}
        oninput={() => loadDomains({ reset: true })}
        class="fb-180-grow"
      />
      <div class="filter-group">
        <select
          bind:value={domainTagFilter}
          onchange={() => loadDomains({ reset: true })}
          class="filter-select"
          aria-label={$t("tag_filter_label")}
        >
          <option value="">{$t("tag_filter_all")}</option>
          <option value="__none__">{$t("tag_filter_none")}</option>
          {#each availableTags as tag}
            <option value={tag.name}>{tag.name}</option>
          {/each}
        </select>
        <select
          bind:value={domainLevelFilter}
          onchange={() => loadDomains({ reset: true })}
          class="filter-select"
          aria-label={$t("level_filter_label")}
        >
          <option value="">{$t("level_filter_all")}</option>
          <option value="WARNING">{$t("level_filter_warning_plus")}</option>
          <option value="ERROR">{$t("level_filter_error_plus")}</option>
        </select>
      </div>
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
              class="row-clickable"
              onclick={() => navigateToDomainDetail(d)}
              role="button"
              tabindex="0"
              onkeydown={(e) => { if (e.key === "Enter" || e.key === " ") navigateToDomainDetail(d); }}
            >
              <td class="mono">{d.name}</td>
              <td>{d.tags ? d.tags.join(", ") : ""}</td>
              <td>{#if domainLevel(d)}<span class="badge level-{domainLevel(d).toLowerCase()}">{domainLevel(d)}</span>{:else}-{/if}</td>
              {#if scoringEnabled}<td>{#if d.latest_grade != null && d.latest_score != null}<span class="grade-chip"><span class="grade-chip-letter" data-grade={d.latest_grade}>{d.latest_grade}</span><span class="grade-chip-score">{d.latest_score}</span></span>{:else}-{/if}</td>{/if}
              <td>{d.latest_run_at ? d.latest_run_at.slice(0, 10) : "-"}</td>
              <td>{d.run_count ?? 0}</td>
            </tr>
          {/each}
        </tbody>
      </table>
      <div class="pagination">
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
