<script>
  import { onMount } from "svelte";
  import { t } from "../i18n.js";
  import { fetchMetricsSnapshot, metricsWindowOptions } from "../metrics.js";
  import {
    formatInteger,
    formatCompactInteger,
    formatPercent,
    formatDurationMs,
    formatRate,
    formatUptime,
  } from "../lib/format.js";
  import {
    sortItems,
    nextTableSort,
    tableSortIndicator,
    tableSortAria,
    compareNumber,
    compareText,
  } from "../lib/sort.js";
  import { moduleLevels } from "../lib/result.js";
  import { normalizePageSize } from "../lib/persistence.js";

  let {
    apiFetch,
    setStatus = () => {},
    onNavigateDomain = () => {},
    autoRefreshMetrics = $bindable(true),
    metricsWindow = $bindable("1h"),
    metricsDomainLimit = $bindable(10),
    metricsBatchLimit = $bindable(10),
  } = $props();

  const metricsLimitOptions = [5, 10, 20, 50, 100];
  const normalizeMetricsLimit = (value) => normalizePageSize(value, metricsLimitOptions, 10);

  const metricsCardHelp = {
    queue_depth: "help_queue_depth",
    in_flight_jobs: "help_in_flight_jobs",
    dns_queries_ipv4_total: "help_ipv4_queries",
    dns_queries_ipv6_total: "help_ipv6_queries",
    success_rate: "help_success_rate",
    failed_rate: "help_failed_rate",
    api_p90: "help_api_p90",
    avg_job_duration: "help_avg_duration",
    completed_total: "help_completed",
    failed_total: "help_failed",
    dns_cache_hits: "help_cache_hits",
    dns_cache_misses: "help_cache_misses",
    dns_cache_hit_rate: "help_cache_hit_rate",
    dns_lookups_total: "help_dns_lookups",
    jobs_per_minute: "help_jobs_per_minute",
  };

  let metricsSnapshot = $state(null);
  let metricsLoading = $state(false);
  let metricsError = $state("");
  let metricsLoadedAt = $state("");
  let metricsDomainsSortState = $state({ key: "", direction: "asc" });
  let metricsBatchesSortState = $state({ key: "failed_expired", direction: "desc" });

  const metricsResolutionSeconds = (snapshot, window) => {
    const windows = snapshot?.trends?.windows || {};
    return windows[window]?.resolution_seconds || (windows[Object.keys(windows)[0]]?.resolution_seconds) || 60;
  };
  const metricsSeriesPoints = (snapshot, window) => {
    const windows = snapshot?.trends?.windows || {};
    if (windows[window]?.points) return windows[window].points;
    const firstKey = Object.keys(windows)[0];
    return firstKey ? windows[firstKey].points || [] : [];
  };
  const metricsSeriesValues = (snapshot, window, field) =>
    metricsSeriesPoints(snapshot, window).map((point) => Number(point?.[field] || 0));
  const metricsJobsPerMinute = (snapshot, window) => {
    const points = metricsSeriesPoints(snapshot, window);
    if (!points.length) return 0;
    const last = Number(points[points.length - 1]?.throughput || 0);
    const resSec = metricsResolutionSeconds(snapshot, window);
    return last * (60 / resSec);
  };
  const metricsCacheHitRate = (snapshot) => {
    const hits = snapshot?.health?.dns_cache_hits || 0;
    const misses = snapshot?.health?.dns_cache_misses || 0;
    const total = hits + misses;
    if (total === 0) return 0;
    return hits / total;
  };
  const metricsFailedJobs = (snapshot) => {
    const currentFailed = Number(snapshot?.jobs?.status_counts?.failed);
    if (Number.isFinite(currentFailed)) return currentFailed;
    return Number(snapshot?.quality?.outcomes?.failed_total || 0);
  };
  const lastLoadedLabel = (value) => {
    if (!value) return $t("time_never");
    const parsed = new Date(value);
    if (Number.isNaN(parsed.getTime())) return $t("time_never");
    return [parsed.getHours(), parsed.getMinutes(), parsed.getSeconds()]
      .map((n) => String(n).padStart(2, "0"))
      .join(":");
  };
  const sparklineBounds = (...seriesList) => {
    const flattened = seriesList.flatMap((series) =>
      Array.isArray(series)
        ? series.map((value) => Number(value)).filter((value) => Number.isFinite(value))
        : []
    );
    if (flattened.length === 0) return { min: 0, max: 1 };
    const min = Math.min(...flattened);
    const max = Math.max(...flattened);
    return { min, max: max === min ? min + 1 : max };
  };
  const sparklinePoints = (values, width = 260, height = 66, padding = 6, bounds = null) => {
    if (!Array.isArray(values) || values.length === 0) return "";
    const usableValues = values.map((value) => {
      const numeric = Number(value);
      return Number.isFinite(numeric) ? numeric : 0;
    });
    const max = Number.isFinite(bounds?.max) ? Number(bounds.max) : Math.max(...usableValues);
    const min = Number.isFinite(bounds?.min) ? Number(bounds.min) : Math.min(...usableValues);
    const spread = max - min || 1;
    const spanX = Math.max(width - padding * 2, 1);
    const spanY = Math.max(height - padding * 2, 1);
    const denominator = usableValues.length > 1 ? usableValues.length - 1 : 1;
    return usableValues
      .map((value, index) => {
        const x = padding + (spanX * index) / denominator;
        const y = height - padding - ((value - min) / spread) * spanY;
        return `${x.toFixed(2)},${y.toFixed(2)}`;
      })
      .join(" ");
  };
  const metricsDomainRows = (snapshot) => snapshot?.insights?.domains?.items || [];
  const batchErrorScore = (batch) =>
    Number(batch?.outcomes?.failed || 0) + Number(batch?.outcomes?.expired || 0);
  const metricsBatchRows = (snapshot) => snapshot?.insights?.batches?.items || [];
  const metricsTopAPIP90 = (snapshot) =>
    (snapshot?.api?.routes || []).reduce((highest, route) => {
      const value = Number(route?.latency_ms?.p90 || 0);
      return value > highest ? value : highest;
    }, 0);
  const hasMetricsData = (snapshot) => Boolean(snapshot && snapshot.generated_at);
  const seriesLast = (values = []) =>
    values.length ? Number(values[values.length - 1] || 0) : 0;

  const sortedMetricsDomainRows = $derived(
    sortItems(metricsDomainRows(metricsSnapshot), metricsDomainsSortState, {
      domain: (left, right) => compareText(left?.domain, right?.domain),
      runs_total: (left, right) => compareNumber(left?.runs_total, right?.runs_total),
      last_status: (left, right) => compareText(left?.last_status, right?.last_status),
      avg_duration_ms: (left, right) => compareNumber(left?.avg_duration_ms, right?.avg_duration_ms),
      error_critical: (left, right) =>
        compareNumber(
          Number(left?.severity_totals?.ERROR || 0) + Number(left?.severity_totals?.CRITICAL || 0),
          Number(right?.severity_totals?.ERROR || 0) + Number(right?.severity_totals?.CRITICAL || 0)
        ),
    }, (left, right) => compareText(left?.domain, right?.domain))
  );

  const sortedMetricsBatchRows = $derived(
    sortItems(metricsBatchRows(metricsSnapshot), metricsBatchesSortState, {
      batch_id: (left, right) => compareText(left?.batch_id, right?.batch_id),
      processed_total: (left, right) => compareNumber(left?.processed_total, right?.processed_total),
      failed_expired: (left, right) => compareNumber(batchErrorScore(left), batchErrorScore(right)),
      canceled: (left, right) => compareNumber(left?.outcomes?.canceled, right?.outcomes?.canceled),
      error_critical: (left, right) =>
        compareNumber(
          Number(left?.severity_totals?.ERROR || 0) + Number(left?.severity_totals?.CRITICAL || 0),
          Number(right?.severity_totals?.ERROR || 0) + Number(right?.severity_totals?.CRITICAL || 0)
        ),
    })
  );

  async function loadMetrics(options = {}) {
    const { silent = false } = options;
    if (!silent) metricsLoading = true;
    metricsError = "";
    try {
      metricsSnapshot = await fetchMetricsSnapshot(apiFetch, {
        window: metricsWindow,
        include: ["health", "jobs", "api", "quality", "insights", "trends"],
        limitDomains: normalizeMetricsLimit(metricsDomainLimit),
        limitBatches: normalizeMetricsLimit(metricsBatchLimit),
      });
      metricsLoadedAt = new Date().toISOString();
    } catch (error) {
      metricsError = error.message || $t("error_unknown");
      if (!metricsSnapshot) {
        setStatus($t("metrics_load_error", { error: metricsError }), "warn");
      }
    } finally {
      if (!silent) metricsLoading = false;
    }
  }

  onMount(() => {
    loadMetrics();
  });

  $effect(() => {
    if (!autoRefreshMetrics) return;
    // Re-arm the timer when the window or limits change.
    metricsWindow;
    metricsDomainLimit;
    metricsBatchLimit;
    const handle = setInterval(() => loadMetrics({ silent: true }), 10000);
    return () => clearInterval(handle);
  });
</script>

<div class="card reveal delay-38 panel-mt" id="panel-metrics" role="tabpanel" aria-labelledby="tab-metrics">
  <h2>{$t("metrics_heading")}</h2>
  <div class="row">
    <button class="ghost" type="button" onclick={() => loadMetrics()} disabled={metricsLoading}>
      {metricsLoading ? $t("refreshing") : $t("refresh_metrics")}
    </button>
    <button class="ghost" type="button" onclick={() => (autoRefreshMetrics = !autoRefreshMetrics)}>
      {autoRefreshMetrics ? $t("auto_refresh_on") : $t("auto_refresh_off")}
    </button>
    <div class="sort-control">
      <label for="metrics-window">{$t("trend_window_label")}</label>
      <select id="metrics-window" bind:value={metricsWindow} onchange={() => loadMetrics()}>
        {#each metricsWindowOptions as option}
          <option value={option.id}>{$t(option.labelKey)}</option>
        {/each}
      </select>
    </div>
    <div class="sort-control">
      <label for="metrics-domain-limit">{$t("top_domains_label")}</label>
      <select id="metrics-domain-limit" bind:value={metricsDomainLimit} onchange={() => loadMetrics()}>
        {#each metricsLimitOptions as value}
          <option value={value}>{value}</option>
        {/each}
      </select>
    </div>
    <div class="sort-control">
      <label for="metrics-batch-limit">{$t("top_batches_label")}</label>
      <select id="metrics-batch-limit" bind:value={metricsBatchLimit} onchange={() => loadMetrics()}>
        {#each metricsLimitOptions as value}
          <option value={value}>{value}</option>
        {/each}
      </select>
    </div>
  </div>
  <div class="small">
    {$t("metrics_status_line", { time: lastLoadedLabel(metricsLoadedAt), uptime: formatUptime(metricsSnapshot?.health?.uptime_seconds), version: metricsSnapshot?.server_version || $t("value_unknown") })}
  </div>

  {#if metricsLoading && !hasMetricsData(metricsSnapshot)}
    <div class="summary-empty">{$t("loading_metrics")}</div>
  {:else if metricsError && !hasMetricsData(metricsSnapshot)}
    <div class="notice">{$t("metrics_load_error", { error: metricsError })}</div>
    <button type="button" onclick={() => loadMetrics()}>{$t("retry")}</button>
  {:else if hasMetricsData(metricsSnapshot)}
    {@const throughputSeries = metricsSeriesValues(metricsSnapshot, metricsWindow, "throughput")}
    {@const failedSeries = metricsSeriesValues(metricsSnapshot, metricsWindow, "failed")}
    {@const querySeriesIPv4 = metricsSeriesValues(metricsSnapshot, metricsWindow, "dns_queries_ipv4_per_second")}
    {@const querySeriesIPv6 = metricsSeriesValues(metricsSnapshot, metricsWindow, "dns_queries_ipv6_per_second")}
    {@const cacheHitRateSeries = metricsSeriesValues(metricsSnapshot, metricsWindow, "dns_cache_hit_rate")}
    {@const queryBounds = sparklineBounds(querySeriesIPv4, querySeriesIPv6)}
    {@const severityTotals = metricsSnapshot?.quality?.severity?.totals || {}}

    {#if metricsError}
      <div class="notice">{$t("metrics_stale_error", { error: metricsError })}</div>
    {/if}

    <div class="metrics-card-group">
      <h4 class="metrics-group-heading">{$t("metrics_group_queue")}</h4>
      <div class="summary-grid metrics-summary-grid">
        <div class="summary-item" title={$t(metricsCardHelp.queue_depth)}>
          <span class="summary-label">{$t("metric_queue_depth")}</span>
          <span class="summary-count">{formatInteger(metricsSnapshot?.health?.queue_depth)}</span>
          <span class="sr-only">{$t(metricsCardHelp.queue_depth)}</span>
        </div>
        <div class="summary-item" title={$t(metricsCardHelp.in_flight_jobs)}>
          <span class="summary-label">{$t("metric_in_flight")}</span>
          <span class="summary-count">{formatInteger(metricsSnapshot?.health?.in_flight_jobs)}</span>
          <span class="sr-only">{$t(metricsCardHelp.in_flight_jobs)}</span>
        </div>
        <div class="summary-item" title={$t(metricsCardHelp.jobs_per_minute)}>
          <span class="summary-label">{$t("metric_jobs_per_minute")}</span>
          <span class="summary-count">{formatInteger(Math.round(metricsJobsPerMinute(metricsSnapshot, metricsWindow)))}</span>
          <span class="sr-only">{$t(metricsCardHelp.jobs_per_minute)}</span>
        </div>
      </div>
    </div>

    <div class="metrics-card-group">
      <h4 class="metrics-group-heading">{$t("metrics_group_dns")}</h4>
      <div class="summary-grid metrics-summary-grid">
        <div class="summary-item" title={$t(metricsCardHelp.dns_lookups_total)}>
          <span class="summary-label">{$t("metric_dns_lookups")}</span>
          <span class="summary-count">{formatCompactInteger((metricsSnapshot?.health?.dns_cache_hits || 0) + (metricsSnapshot?.health?.dns_cache_misses || 0))}</span>
          <span class="sr-only">{$t(metricsCardHelp.dns_lookups_total)}</span>
        </div>
        <div class="summary-item" title={$t(metricsCardHelp.dns_cache_hits)}>
          <span class="summary-label">{$t("metric_cache_hits")}</span>
          <span class="summary-count">{formatCompactInteger(metricsSnapshot?.health?.dns_cache_hits)}</span>
          <span class="sr-only">{$t(metricsCardHelp.dns_cache_hits)}</span>
        </div>
        <div class="summary-item" title={$t(metricsCardHelp.dns_cache_misses)}>
          <span class="summary-label">{$t("metric_cache_misses")}</span>
          <span class="summary-count">{formatCompactInteger(metricsSnapshot?.health?.dns_cache_misses)}</span>
          <span class="sr-only">{$t(metricsCardHelp.dns_cache_misses)}</span>
        </div>
        <div class="summary-item" title={$t(metricsCardHelp.dns_cache_hit_rate)}>
          <span class="summary-label">{$t("metric_cache_hit_rate")}</span>
          <span class="summary-count">{formatPercent(metricsCacheHitRate(metricsSnapshot))}</span>
          <span class="sr-only">{$t(metricsCardHelp.dns_cache_hit_rate)}</span>
        </div>
        <div class="summary-item" title={$t(metricsCardHelp.dns_queries_ipv4_total)}>
          <span class="summary-label">{$t("metric_ipv4_queries")}</span>
          <span class="summary-count">{formatCompactInteger(metricsSnapshot?.health?.dns_queries_ipv4_total)}</span>
          <span class="sr-only">{$t(metricsCardHelp.dns_queries_ipv4_total)}</span>
        </div>
        <div class="summary-item" title={$t(metricsCardHelp.dns_queries_ipv6_total)}>
          <span class="summary-label">{$t("metric_ipv6_queries")}</span>
          <span class="summary-count">{formatCompactInteger(metricsSnapshot?.health?.dns_queries_ipv6_total)}</span>
          <span class="sr-only">{$t(metricsCardHelp.dns_queries_ipv6_total)}</span>
        </div>
      </div>
    </div>

    <div class="metrics-card-group">
      <h4 class="metrics-group-heading">{$t("metrics_group_jobs")}</h4>
      <div class="summary-grid metrics-summary-grid">
        <div class="summary-item jobs-finished" title={$t(metricsCardHelp.completed_total)}>
          <span class="summary-label">{$t("metric_completed")}</span>
          <span class="summary-count">{formatInteger(metricsSnapshot?.jobs?.completed_total)}</span>
          <span class="sr-only">{$t(metricsCardHelp.completed_total)}</span>
        </div>
        <div class="summary-item failed-jobs" title={$t(metricsCardHelp.failed_total)}>
          <span class="summary-label">{$t("metric_failed")}</span>
          <span class="summary-count">{formatInteger(metricsFailedJobs(metricsSnapshot))}</span>
          <span class="sr-only">{$t(metricsCardHelp.failed_total)}</span>
        </div>
        <div class="summary-item" title={$t(metricsCardHelp.success_rate)}>
          <span class="summary-label">{$t("metric_success_rate")}</span>
          <span class="summary-count">{formatPercent(metricsSnapshot?.quality?.outcomes?.success_rate)}</span>
          <span class="sr-only">{$t(metricsCardHelp.success_rate)}</span>
        </div>
        <div class="summary-item" title={$t(metricsCardHelp.failed_rate)}>
          <span class="summary-label">{$t("metric_failure_rate")}</span>
          <span class="summary-count">{formatPercent(metricsSnapshot?.quality?.outcomes?.failed_rate)}</span>
          <span class="sr-only">{$t(metricsCardHelp.failed_rate)}</span>
        </div>
      </div>
    </div>

    <div class="metrics-card-group">
      <h4 class="metrics-group-heading">{$t("metrics_group_performance")}</h4>
      <div class="summary-grid metrics-summary-grid">
        <div class="summary-item" title={$t(metricsCardHelp.avg_job_duration)}>
          <span class="summary-label">{$t("metric_avg_duration")}</span>
          <span class="summary-count">{formatDurationMs(metricsSnapshot?.quality?.job_duration_ms?.avg)}</span>
          <span class="sr-only">{$t(metricsCardHelp.avg_job_duration)}</span>
        </div>
        <div class="summary-item" title={$t(metricsCardHelp.api_p90)}>
          <span class="summary-label">{$t("metric_api_p90")}</span>
          <span class="summary-count">{formatDurationMs(metricsTopAPIP90(metricsSnapshot))}</span>
          <span class="sr-only">{$t(metricsCardHelp.api_p90)}</span>
        </div>
      </div>
    </div>

    <div class="metrics-card-group">
      <h4 class="metrics-group-heading">{$t("metrics_group_severity")}</h4>
      <div class="summary-grid metrics-summary-grid">
        {#each moduleLevels as level}
          <div class={`summary-item severity-${level.toLowerCase()}`} title={$t("help_severity_card", { level })}>
            <span class="summary-label">{level}</span>
            <span class="summary-count">{formatInteger(severityTotals[level])}</span>
            <span class="sr-only">{$t("help_severity_card", { level })}</span>
          </div>
        {/each}
      </div>
    </div>

    <div class="metrics-trend-grid">
      <div class="metrics-trend-card">
        <div class="metrics-trend-head">
          <strong>{$t("metric_throughput")}</strong>
          <span>{formatInteger(seriesLast(throughputSeries))}{$t("per_bucket")}</span>
        </div>
        <svg class="sparkline" viewBox="0 0 260 66" role="img" aria-label={$t("aria_throughput_trend")}>
          <polyline points={sparklinePoints(throughputSeries)} />
        </svg>
      </div>
      <div class="metrics-trend-card">
        <div class="metrics-trend-head">
          <strong>{$t("metric_failures")}</strong>
          <span>{formatInteger(seriesLast(failedSeries))}{$t("per_bucket")}</span>
        </div>
        <svg class="sparkline sparkline-warn" viewBox="0 0 260 66" role="img" aria-label={$t("aria_failure_trend")}>
          <polyline points={sparklinePoints(failedSeries)} />
        </svg>
      </div>
      <div class="metrics-trend-card">
        <div class="metrics-trend-head">
          <strong>{$t("metric_dns_rate")}</strong>
          <span>{formatRate(seriesLast(querySeriesIPv4) + seriesLast(querySeriesIPv6))}</span>
        </div>
        <svg class="sparkline sparkline-queries" viewBox="0 0 260 66" role="img" aria-label={$t("aria_dns_trend")}>
          <polyline class="sparkline-ipv4" points={sparklinePoints(querySeriesIPv4, 260, 66, 6, queryBounds)} />
          <polyline class="sparkline-ipv6" points={sparklinePoints(querySeriesIPv6, 260, 66, 6, queryBounds)} />
        </svg>
        <div class="sparkline-legend">
          <span class="sparkline-legend-item">
            <span class="sparkline-legend-dot sparkline-legend-dot-ipv4" aria-hidden="true"></span>
            {$t("ipv4_label")} {formatRate(seriesLast(querySeriesIPv4))}
          </span>
          <span class="sparkline-legend-item">
            <span class="sparkline-legend-dot sparkline-legend-dot-ipv6" aria-hidden="true"></span>
            {$t("ipv6_label")} {formatRate(seriesLast(querySeriesIPv6))}
          </span>
        </div>
      </div>
      <div class="metrics-trend-card">
        <div class="metrics-trend-head">
          <strong>{$t("metric_cache_hit_rate")}</strong>
          <span>{formatPercent(seriesLast(cacheHitRateSeries))}</span>
        </div>
        <svg class="sparkline sparkline-cache" viewBox="0 0 260 66" role="img" aria-label={$t("aria_cache_hit_rate_trend")}>
          <polyline points={sparklinePoints(cacheHitRateSeries)} />
        </svg>
      </div>
    </div>

    <div class="metrics-tables">
      <div class="metrics-table-wrap">
        <h3>{$t("top_domains_heading")}</h3>
        {#if metricsDomainRows(metricsSnapshot).length === 0}
          <div class="summary-empty">{$t("no_domain_data")}</div>
        {:else}
          <table class="metrics-table">
            <thead>
              <tr>
                <th class="sortable-column" aria-sort={tableSortAria(metricsDomainsSortState, "domain")}><button class="table-sort-button" type="button" onclick={() => { metricsDomainsSortState = nextTableSort(metricsDomainsSortState, "domain"); }}><span>{$t("domain_col")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(metricsDomainsSortState, "domain")}</span></button></th>
                <th class="sortable-column" aria-sort={tableSortAria(metricsDomainsSortState, "runs_total")}><button class="table-sort-button" type="button" onclick={() => { metricsDomainsSortState = nextTableSort(metricsDomainsSortState, "runs_total", "desc"); }}><span>{$t("runs_col")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(metricsDomainsSortState, "runs_total")}</span></button></th>
                <th class="sortable-column" aria-sort={tableSortAria(metricsDomainsSortState, "last_status")}><button class="table-sort-button" type="button" onclick={() => { metricsDomainsSortState = nextTableSort(metricsDomainsSortState, "last_status"); }}><span>{$t("last_status_col")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(metricsDomainsSortState, "last_status")}</span></button></th>
                <th class="sortable-column" aria-sort={tableSortAria(metricsDomainsSortState, "avg_duration_ms")}><button class="table-sort-button" type="button" onclick={() => { metricsDomainsSortState = nextTableSort(metricsDomainsSortState, "avg_duration_ms", "desc"); }}><span>{$t("avg_duration_col")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(metricsDomainsSortState, "avg_duration_ms")}</span></button></th>
                <th class="sortable-column" aria-sort={tableSortAria(metricsDomainsSortState, "error_critical")}><button class="table-sort-button" type="button" onclick={() => { metricsDomainsSortState = nextTableSort(metricsDomainsSortState, "error_critical", "desc"); }}><span>{$t("error_critical_col")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(metricsDomainsSortState, "error_critical")}</span></button></th>
              </tr>
            </thead>
            <tbody>
              {#each sortedMetricsDomainRows as row}
                <tr>
                  <td class="mono"><button class="ghost btn-text-mono" type="button" onclick={() => onNavigateDomain(row.domain)}>{row.domain}</button></td>
                  <td>{formatInteger(row.runs_total)}</td>
                  <td>{row.last_status || "-"}</td>
                  <td>{formatDurationMs(row.avg_duration_ms)}</td>
                  <td>{formatInteger(Number(row?.severity_totals?.ERROR || 0) + Number(row?.severity_totals?.CRITICAL || 0))}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        {/if}
      </div>

      <div class="metrics-table-wrap">
        <h3>{$t("error_heavy_batches_heading")}</h3>
        {#if metricsBatchRows(metricsSnapshot).length === 0}
          <div class="summary-empty">{$t("no_batch_data")}</div>
        {:else}
          <table class="metrics-table metrics-table-batches">
            <colgroup>
              <col class="metrics-col-batch-id" />
              <col />
              <col />
              <col />
              <col />
            </colgroup>
            <thead>
              <tr>
                <th class="sortable-column" aria-sort={tableSortAria(metricsBatchesSortState, "batch_id")}><button class="table-sort-button" type="button" onclick={() => { metricsBatchesSortState = nextTableSort(metricsBatchesSortState, "batch_id"); }}><span>{$t("batch_id_col")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(metricsBatchesSortState, "batch_id")}</span></button></th>
                <th class="sortable-column" aria-sort={tableSortAria(metricsBatchesSortState, "processed_total")}><button class="table-sort-button" type="button" onclick={() => { metricsBatchesSortState = nextTableSort(metricsBatchesSortState, "processed_total", "desc"); }}><span>{$t("processed_col")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(metricsBatchesSortState, "processed_total")}</span></button></th>
                <th class="sortable-column" aria-sort={tableSortAria(metricsBatchesSortState, "failed_expired")}><button class="table-sort-button" type="button" onclick={() => { metricsBatchesSortState = nextTableSort(metricsBatchesSortState, "failed_expired", "desc"); }}><span>{$t("failed_expired_col")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(metricsBatchesSortState, "failed_expired")}</span></button></th>
                <th class="sortable-column" aria-sort={tableSortAria(metricsBatchesSortState, "canceled")}><button class="table-sort-button" type="button" onclick={() => { metricsBatchesSortState = nextTableSort(metricsBatchesSortState, "canceled", "desc"); }}><span>{$t("canceled_col")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(metricsBatchesSortState, "canceled")}</span></button></th>
                <th class="sortable-column" aria-sort={tableSortAria(metricsBatchesSortState, "error_critical")}><button class="table-sort-button" type="button" onclick={() => { metricsBatchesSortState = nextTableSort(metricsBatchesSortState, "error_critical", "desc"); }}><span>{$t("error_critical_col")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(metricsBatchesSortState, "error_critical")}</span></button></th>
              </tr>
            </thead>
            <tbody>
              {#each sortedMetricsBatchRows as row}
                <tr>
                  <td class="mono metrics-batch-id" title={row.batch_id}>{row.batch_id}</td>
                  <td>{formatInteger(row.processed_total)}</td>
                  <td>{formatInteger(batchErrorScore(row))}</td>
                  <td>{formatInteger(row?.outcomes?.canceled || 0)}</td>
                  <td>{formatInteger(Number(row?.severity_totals?.ERROR || 0) + Number(row?.severity_totals?.CRITICAL || 0))}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        {/if}
      </div>
    </div>
  {:else}
    <div class="summary-empty">{$t("no_metrics_data")}</div>
  {/if}
</div>
