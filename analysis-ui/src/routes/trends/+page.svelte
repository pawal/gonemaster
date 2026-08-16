<script lang="ts">
  import { goto } from "$app/navigation";
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import FilterBar from "$lib/FilterBar.svelte";
  import TrendLine from "$lib/charts/TrendLine.svelte";
  import { domainsGradeHref, domainsSeverityHref, tagHref } from "$lib/entityLinks";
  import { downloadCSV, type ExportColumn } from "$lib/exporters";
  import { formatCount, levelTone, snapshotDisplayLabel, snapshotSourceDate } from "$lib/format";
  import { computeTagMovers, pinnedSeries } from "$lib/trends";
  import type { LayoutData } from "../+layout";
  import { TREND_CATEGORIES, type TrendsPageData } from "./+page";

  let { data }: { data: TrendsPageData } = $props();

  const layoutData = $derived(page.data as LayoutData);

  // Tone keys the TrendLine understands; anything else falls back to neutral.
  type ChartTone = "ok" | "notice" | "warning" | "error" | "critical" | "neutral";
  const CHART_TONES = new Set<ChartTone>(["ok", "notice", "warning", "error", "critical", "neutral"]);
  function chartTone(tone: string | null): ChartTone {
    return tone && CHART_TONES.has(tone as ChartTone) ? (tone as ChartTone) : "neutral";
  }

  // Segments narrower than this render as a bare colour sliver: labels would
  // overflow. The value still reaches readers via the tooltip and the
  // accessible table below the chart.
  const TINY_PCT = 5;

  // Deep-link a severity/grade segment to the domains behind it, in that
  // snapshot. Other categories have no matching domains filter.
  function segmentHref(key: string, slug: string): string | null {
    const params = new URLSearchParams();
    if (data.datasetTag) params.set("dataset_tag", data.datasetTag);
    params.set("snapshot", slug);
    const q = `?${params.toString()}`;
    if (data.category === "severity") return domainsSeverityHref(base, key, q);
    if (data.category === "grade") return domainsGradeHref(base, key, q);
    return null;
  }

  // Link a snapshot row to the diff against its predecessor. Series is
  // oldest-first, so the previous snapshot is the row above; the first row
  // has none.
  function diffRowHref(index: number): string | null {
    if (index <= 0) return null;
    const from = series[index - 1]?.slug;
    const to = series[index]?.slug;
    if (!from || !to) return null;
    const params = new URLSearchParams();
    if (data.datasetTag) params.set("dataset_tag", data.datasetTag);
    params.set("from", from);
    params.set("to", to);
    params.set("tab", "grade_changed");
    return `${base}/diff?${params.toString()}`;
  }

  function onCategoryChange(value: string) {
    const params = new URLSearchParams(page.url.searchParams);
    params.set("category", value);
    // Buckets differ per category, so a pinned key from the old category is
    // meaningless under the new one.
    params.delete("key");
    goto(`${page.url.pathname}?${params.toString()}`, {
      replaceState: true,
      noScroll: true
    });
  }

  // Focus mode: ?key= pins one bucket to a line chart; ?scale= toggles the
  // y-axis between absolute counts (default) and percentage share.
  const pinnedKey = $derived(page.url.searchParams.get("key") ?? "");
  const scale = $derived(page.url.searchParams.get("scale") === "share" ? "share" : "count");

  function setKey(key: string) {
    const params = new URLSearchParams(page.url.searchParams);
    if (key) params.set("key", key);
    else params.delete("key");
    goto(`${page.url.pathname}?${params.toString()}`, { replaceState: true, noScroll: true });
  }

  function setScale(value: "count" | "share") {
    const params = new URLSearchParams(page.url.searchParams);
    if (value === "share") params.set("scale", "share");
    else params.delete("scale");
    goto(`${page.url.pathname}?${params.toString()}`, { replaceState: true, noScroll: true });
  }

  function onKeydown(event: KeyboardEvent) {
    if (event.key === "Escape" && pinnedKey) setKey("");
  }

  // Label, tone, and order for every observed key come from data.keyMeta,
  // built server-side from factCategoryDisplays in
  // server/analysis_fact_categories.go.
  function metaFor(key: string) {
    return data.keyMeta?.[key];
  }

  function toneForKey(key: string): string | null {
    return metaFor(key)?.tone ?? null;
  }

  function rankForKey(key: string): number | null {
    return metaFor(key)?.order ?? null;
  }

  function compareBuckets(a: string, b: string): number {
    const ra = rankForKey(a);
    const rb = rankForKey(b);
    if (ra !== null && rb !== null && ra !== rb) return ra - rb;
    if (ra !== null && rb === null) return -1;
    if (ra === null && rb !== null) return 1;
    return a.localeCompare(b);
  }

  type Series = {
    label: string;
    slug: string;
    sourceDate: string;
    engineVersion: string;
    mixedEngine: boolean;
    // True when this snapshot ran a different engine than the one before
    // it, so the step into this row is confounded by an engine change.
    crossesEngineBoundary: boolean;
    buckets: Array<{ key: string; count: number }>;
  };

  const series = $derived.by<Series[]>(() => {
    const snapshotBySlug = new Map((layoutData.snapshots ?? []).map((snap) => [snap.slug, snap]));
    return data.points.map((point, index) => {
      const meta = snapshotBySlug.get(point.slug);
      const snapshot = {
        slug: point.slug,
        label: point.label ?? meta?.label,
        first_run_at: point.first_run_at ?? meta?.first_run_at,
        last_run_at: point.last_run_at ?? meta?.last_run_at,
        captured_at: point.captured_at
      };
      const payload = point.payload as Record<string, number> | undefined;
      const buckets = payload
        ? Object.entries(payload).map(([key, count]) => ({ key, count: Number(count) || 0 }))
        : [];
      buckets.sort((a, b) => compareBuckets(a.key, b.key));
      const engineVersion = point.engine_version ?? meta?.engine_version ?? "";
      const prev = data.points[index - 1];
      const prevVersion = prev
        ? (prev.engine_version ?? snapshotBySlug.get(prev.slug)?.engine_version ?? "")
        : "";
      return {
        label: snapshotDisplayLabel(snapshot),
        slug: point.slug,
        sourceDate: snapshotSourceDate(snapshot),
        engineVersion,
        mixedEngine: Boolean(point.mixed_engine_version ?? meta?.mixed_engine_version),
        // Unknown on either side means we cannot claim a boundary.
        crossesEngineBoundary:
          Boolean(engineVersion) && Boolean(prevVersion) && engineVersion !== prevVersion,
        buckets
      };
    });
  });

  // Unique bucket keys, sorted by category-aware order so legend and
  // stacked columns line up the same way every render.
  const bucketKeys = $derived.by<string[]>(() => {
    const seen = new Set<string>();
    const out: string[] = [];
    for (const s of series) {
      for (const b of s.buckets) {
        if (!seen.has(b.key)) {
          seen.add(b.key);
          out.push(b.key);
        }
      }
    }
    out.sort((a, b) => compareBuckets(a, b));
    return out;
  });

  // Tone background and foreground reference the CSS variables defined in
  // app.css so light/dark theme changes propagate automatically.
  const TONE_BG: Record<string, string> = {
    ok:       "var(--tone-ok-bg)",
    notice:   "var(--tone-notice-bg)",
    warning:  "var(--tone-warning-bg)",
    error:    "var(--tone-error-bg)",
    critical: "var(--tone-critical-bg)",
    neutral:  "var(--surface-2)"
  };
  const TONE_FG: Record<string, string> = {
    ok:       "var(--tone-ok-fg)",
    notice:   "var(--tone-notice-fg)",
    warning:  "var(--tone-warning-fg)",
    error:    "var(--tone-error-fg)",
    critical: "var(--tone-critical-fg)",
    neutral:  "var(--on-surface-2)"
  };
  const FALLBACK_TONES = ["ok", "notice", "warning", "error", "critical", "neutral"];

  function resolvedTone(key: string, fallbackIndex: number): string {
    return toneForKey(key) ?? FALLBACK_TONES[fallbackIndex % FALLBACK_TONES.length];
  }

  function colorForBucket(key: string, fallbackIndex: number): string {
    return TONE_BG[resolvedTone(key, fallbackIndex)] ?? TONE_BG.neutral;
  }

  function colorFgForBucket(key: string, fallbackIndex: number): string {
    return TONE_FG[resolvedTone(key, fallbackIndex)] ?? TONE_FG.neutral;
  }

  function labelForKey(key: string): string {
    return metaFor(key)?.label ?? key;
  }

  function totalFor(s: Series): number {
    return s.buckets.reduce((sum, b) => sum + b.count, 0);
  }

  function pctFor(s: Series, key: string): number {
    const total = totalFor(s);
    if (total === 0) return 0;
    const bucket = s.buckets.find((b) => b.key === key);
    if (!bucket) return 0;
    return Math.round((bucket.count / total) * 1000) / 10;
  }

  // Unrounded share for the bar width, so a bucket with count > 0 keeps a
  // nonzero width instead of collapsing to 0% and vanishing.
  function rawPctFor(s: Series, key: string): number {
    const total = totalFor(s);
    if (total === 0) return 0;
    const bucket = s.buckets.find((b) => b.key === key);
    return bucket ? (bucket.count / total) * 100 : 0;
  }

  function countFor(s: Series, key: string): number {
    return s.buckets.find((b) => b.key === key)?.count ?? 0;
  }

  // Focus mode is active only when the pinned key is a real bucket in the
  // current series.
  const focusActive = $derived(!!pinnedKey && bucketKeys.includes(pinnedKey));
  const pinned = $derived.by(() => (focusActive ? pinnedSeries(series, pinnedKey) : []));
  const pinnedValues = $derived(
    scale === "share" ? pinned.map((p) => p.share) : pinned.map((p) => p.count)
  );
  // Prefer the short source date for the x-axis; series is aligned to pinned.
  const pinnedLabels = $derived(series.map((s) => s.sourceDate || s.label));

  // Biggest tag movers between the first and last snapshot, from the separate
  // top_tags series.
  const movers = $derived(computeTagMovers(data.topTagPoints ?? [], 10));
  const query = $derived(page.url.search);

  // Export the visible category series: one row per snapshot, one column per
  // bucket, plus the total.
  function exportSeries() {
    if (series.length === 0) return;
    const columns: ExportColumn<Series>[] = [
      { key: "snapshot", label: "Snapshot", value: (s) => s.label },
      { key: "date", label: "Source date", value: (s) => s.sourceDate },
      ...bucketKeys.map((key) => ({
        key,
        label: labelForKey(key),
        value: (s: Series) => countFor(s, key)
      })),
      { key: "total", label: "Total", value: (s) => totalFor(s) }
    ];
    const tag = data.datasetTag ?? "cohort";
    downloadCSV(`${tag}-trends-${data.category}.csv`, series, columns);
  }
</script>

<svelte:window onkeydown={onKeydown} />

<FilterBar
  cohorts={layoutData.catalog?.cohorts ?? []}
  selectorEnabled={layoutData.catalog?.selector_enabled ?? false}
  snapshots={[]}
  defaultSnapshotSlug=""
  showSearch={false}
/>

<section class="card trends-header">
  <h2>Trends</h2>
  <p class="hint">
    How each aggregate has evolved across this cohort's captured snapshots.
    Pick a category to switch the chart; the URL updates so shared links
    land on the same view.
  </p>
  <label class="category-select">
    <span>Category</span>
    <select
      value={data.category}
      onchange={(e) => onCategoryChange(e.currentTarget.value)}
    >
      {#each TREND_CATEGORIES as c (c.key)}
        <option value={c.key}>{c.label}</option>
      {/each}
    </select>
  </label>
</section>

{#if data.error}
  <section class="card">
    <p class="status-banner error">Failed to load trends: {data.error}</p>
  </section>
{:else if !data.datasetTag}
  <section class="card empty-state">
    <p class="hint">No public cohort is configured - pick one from the
    filter bar to see its trends.</p>
  </section>
{:else if series.length === 0}
  <section class="card empty-state">
    <p class="hint">
      No captured snapshots yet. Trends populate after the projector
      captures the cohort's first snapshot-intent batch.
    </p>
  </section>
{:else}
  <section class="card">
    <div class="list-head">
      <h3>{TREND_CATEGORIES.find((c) => c.key === data.category)?.label}</h3>
      <div class="export-group">
        <button type="button" class="ghost" onclick={exportSeries} disabled={series.length === 0}>
          Export CSV
        </button>
      </div>
    </div>
    {#if focusActive}
      <div class="focus-head">
        <div>
          <h4 class="focus-title">Focus: {labelForKey(pinnedKey)}</h4>
          <p class="hint">This bucket's {scale === "share" ? "share" : "domain count"} across snapshots.</p>
        </div>
        <div class="focus-controls">
          <div class="scale-toggle" role="group" aria-label="Value scale">
            <button type="button" class="scale-btn" class:active={scale === "count"} onclick={() => setScale("count")}>Count</button>
            <button type="button" class="scale-btn" class:active={scale === "share"} onclick={() => setScale("share")}>Share</button>
          </div>
          <button type="button" class="ghost" onclick={() => setKey("")}>Show all buckets</button>
        </div>
      </div>
      <TrendLine
        values={pinnedValues}
        labels={pinnedLabels}
        tone={chartTone(toneForKey(pinnedKey))}
        seriesLabel={scale === "share" ? "Share" : "Domains"}
        valueSuffix={scale === "share" ? "%" : ""}
        yDomain={scale === "share" ? [0, 100] : undefined}
        caption={`${labelForKey(pinnedKey)} across snapshots`}
      />
    {:else}
    <ol class="trend-list" aria-label="Stacked distribution per snapshot">
      {#each series as s, i (s.slug)}
        {@const diffHref = diffRowHref(i)}
        <li class="trend-row" class:engine-boundary={s.crossesEngineBoundary}>
          {#if s.crossesEngineBoundary}
            <p class="engine-boundary-note">
              Engine changed to {s.engineVersion} here. Differences from the previous
              row may be new engine capability rather than cohort change.
            </p>
          {/if}
          <div class="trend-meta">
            <span class="trend-slug" title={`Snapshot ${s.slug}`}>{s.label}</span>
            {#if s.sourceDate && s.sourceDate !== s.label}
              <span class="trend-captured">{s.sourceDate}</span>
            {/if}
            {#if s.engineVersion}
              <span class="trend-engine" title={`Engine ${s.engineVersion}`}>{s.engineVersion}</span>
            {:else}
              <span class="trend-engine unknown" title="Engine version unknown for this snapshot">
                engine ?
              </span>
            {/if}
            {#if s.mixedEngine}
              <span class="trend-engine mixed" title="This batch spanned an engine upgrade">mixed</span>
            {/if}
            {#if diffHref}
              <a class="trend-diff-link" href={diffHref}>Diff vs previous</a>
            {/if}
          </div>
          <div class="trend-bar" role="group" aria-label="{s.label} distribution">
            {#each bucketKeys as key, i (key)}
              {@const pct = pctFor(s, key)}
              {@const rawPct = rawPctFor(s, key)}
              {@const count = countFor(s, key)}
              {#if count > 0}
                {@const href = segmentHref(key, s.slug)}
                {@const tiny = rawPct < TINY_PCT}
                {@const label = `${labelForKey(key)}: ${formatCount(count)} domains, ${pct}%${href ? " - view domains" : ""}`}
                <svelte:element
                  this={href ? "a" : "span"}
                  {href}
                  role={href ? undefined : "img"}
                  class="trend-segment"
                  class:tiny
                  class:linked={!!href}
                  style:width="{rawPct}%"
                  style:background={colorForBucket(key, i)}
                  style:color={colorFgForBucket(key, i)}
                  aria-label={label}
                  title="{labelForKey(key)}: {formatCount(count)} ({pct}%)"
                >
                  {#if !tiny}
                    <span class="trend-segment-label" aria-hidden="true">{labelForKey(key)}</span>
                    <span class="trend-segment-count" aria-hidden="true">{formatCount(count)}</span>
                  {/if}
                </svelte:element>
              {/if}
            {/each}
          </div>
          <span class="trend-total">{formatCount(totalFor(s))}</span>
        </li>
      {/each}
    </ol>
    {/if}
    <ul class="trend-legend" aria-label="Buckets - select one to focus its trend">
      {#each bucketKeys as key, i (key)}
        <li class="trend-legend-item">
          <button
            type="button"
            class="legend-btn"
            class:active={pinnedKey === key}
            aria-pressed={pinnedKey === key}
            onclick={() => setKey(pinnedKey === key ? "" : key)}
          >
            <span class="legend-swatch" style:background={colorForBucket(key, i)}></span>
            <span class="legend-label">{labelForKey(key)}</span>
          </button>
        </li>
      {/each}
    </ul>
  </section>

  {#if movers.length > 0}
    <section class="card">
      <h3>Top movers</h3>
      <p class="hint">
        Finding tags whose domain count changed most between the first and
        last snapshot shown. Rising counts are regressions.
      </p>
      <ol class="mover-list">
        {#each movers as m (m.tag)}
          <li class="mover-row">
            <a class="mover-tag" href={tagHref(base, m.tag, query)}>{m.tag}</a>
            {#if m.level}
              <span class="level level-{levelTone(m.level)}">{m.level}</span>
            {:else}
              <span class="level level-neutral">-</span>
            {/if}
            <span class="mover-counts">{formatCount(m.first)} → {formatCount(m.last)}</span>
            <span class="mover-delta" class:up={m.delta > 0} class:down={m.delta < 0}>
              {m.delta > 0 ? "+" : ""}{formatCount(m.delta)}
            </span>
          </li>
        {/each}
      </ol>
    </section>
  {/if}
{/if}

<style>
  .trends-header {
    gap: var(--space-2);
  }
  .trends-header h2 {
    margin: 0;
  }
  .category-select {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: var(--text-xs);
    color: var(--ink-2);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    align-self: flex-start;
  }
  .category-select select {
    min-width: 200px;
    padding: 6px 10px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--surface);
    color: var(--ink);
    font: inherit;
    font-size: var(--text-sm);
    letter-spacing: normal;
    text-transform: none;
  }
  .trend-list {
    list-style: none;
    margin: var(--space-2) 0 0;
    padding: 0;
    display: grid;
    grid-template-columns: minmax(12rem, 18rem) 1fr minmax(3.5rem, auto);
    column-gap: var(--space-3);
    row-gap: 8px;
    align-items: center;
  }
  .trend-row {
    display: contents;
  }
  .trend-meta {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .trend-slug {
    font-weight: 600;
  }
  .trend-captured {
    color: var(--ink-2);
    font-size: var(--text-xs);
  }
  .trend-engine {
    color: var(--ink-2);
    font-size: var(--text-xs);
    font-variant-numeric: tabular-nums;
  }
  .trend-engine.unknown {
    font-style: italic;
  }
  .trend-engine.mixed {
    color: var(--sev-warning-fg);
    font-weight: 600;
  }
  /* Spans every grid column so the boundary reads as a band across the
     whole series, not a note attached to one cell. */
  .engine-boundary-note {
    grid-column: 1 / -1;
    margin: var(--space-2) 0 0;
    padding: 4px var(--space-2);
    border-top: 1px dashed var(--border);
    color: var(--ink-2);
    font-size: var(--text-xs);
  }
  .trend-diff-link {
    font-size: var(--text-xs);
    color: var(--accent-2);
    text-decoration: none;
    width: fit-content;
  }
  .trend-diff-link:hover {
    text-decoration: underline;
  }
  .trend-bar {
    display: flex;
    height: 36px;
    width: 100%;
    background: var(--surface-2);
    border-radius: 4px;
    overflow: hidden;
  }
  .trend-segment {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
    height: 100%;
    overflow: hidden;
    padding: 0 var(--space-2);
    text-decoration: none;
  }
  /* Tiny segments drop their padding and take a small floor width so a
     nonzero count is always visible without over-representing it. */
  .trend-segment.tiny {
    padding: 0;
    min-width: 3px;
  }
  .trend-segment.linked {
    cursor: pointer;
  }
  .trend-segment.linked:hover {
    outline: 2px solid var(--ink);
    outline-offset: -2px;
  }
  .trend-segment-label {
    font-size: var(--text-xs);
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: clip;
  }
  .trend-segment-count {
    font-family: var(--mono);
    font-size: var(--text-xs);
    white-space: nowrap;
  }
  .trend-total {
    font-family: var(--mono);
    font-size: var(--text-sm);
    text-align: right;
  }
  .trend-legend {
    list-style: none;
    margin: var(--space-3) 0 0;
    padding: 0;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    font-size: var(--text-xs);
    color: var(--ink-2);
  }
  .trend-legend-item {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .legend-btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 3px 8px;
    background: transparent;
    border: 1px solid transparent;
    border-radius: 999px;
    color: var(--ink-2);
    font: inherit;
    font-size: var(--text-xs);
    cursor: pointer;
  }
  .legend-btn:hover {
    border-color: var(--border);
    color: var(--ink);
  }
  .legend-btn.active {
    border-color: var(--accent-2);
    background: var(--surface-2);
    color: var(--ink);
  }
  .legend-swatch {
    display: inline-block;
    width: 12px;
    height: 12px;
    border-radius: 3px;
  }
  .legend-label {
    font-family: var(--mono);
  }

  .focus-head {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: var(--space-3);
    flex-wrap: wrap;
  }
  .focus-title {
    margin: 0;
    font-size: var(--text-base);
    font-weight: 600;
  }
  .focus-controls {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-wrap: wrap;
  }
  .scale-toggle {
    display: inline-flex;
    border: 1px solid var(--border);
    border-radius: 999px;
    overflow: hidden;
  }
  .scale-btn {
    padding: 5px var(--space-3);
    background: var(--surface);
    color: var(--ink-2);
    border: none;
    border-radius: 0;
    font: inherit;
    font-size: var(--text-sm);
    cursor: pointer;
  }
  .scale-btn.active {
    background: var(--surface-2);
    color: var(--ink);
    font-weight: 600;
  }

  .mover-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    grid-template-columns: minmax(10rem, 1fr) minmax(4rem, auto) minmax(6rem, auto) minmax(4rem, auto);
    column-gap: var(--space-3);
    row-gap: 6px;
    align-items: center;
  }
  .mover-row {
    display: contents;
  }
  .mover-tag {
    font-family: var(--mono);
    font-size: var(--text-sm);
    color: var(--ink);
    text-decoration: none;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .mover-tag:hover {
    color: var(--accent-2);
    text-decoration: underline;
  }
  .mover-counts {
    font-family: var(--mono);
    font-size: var(--text-sm);
    color: var(--ink-2);
    text-align: right;
  }
  .mover-delta {
    font-family: var(--mono);
    font-size: var(--text-sm);
    font-weight: 600;
    text-align: right;
    color: var(--ink-2);
  }
  .mover-delta.up {
    color: var(--bar-error);
  }
  .mover-delta.down {
    color: var(--bar-ok);
  }
</style>
