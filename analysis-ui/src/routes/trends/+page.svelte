<script lang="ts">
  import { goto } from "$app/navigation";
  import { page } from "$app/state";
  import FilterBar from "$lib/FilterBar.svelte";
  import { formatCount, snapshotDisplayLabel, snapshotSourceDate } from "$lib/format";
  import type { LayoutData } from "../+layout";
  import { TREND_CATEGORIES, type TrendsPageData } from "./+page";

  let { data }: { data: TrendsPageData } = $props();

  const layoutData = $derived(page.data as LayoutData);

  function onCategoryChange(value: string) {
    const params = new URLSearchParams(page.url.searchParams);
    params.set("category", value);
    goto(`${page.url.pathname}?${params.toString()}`, {
      replaceState: true,
      noScroll: true
    });
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
    buckets: Array<{ key: string; count: number }>;
  };

  const series = $derived.by<Series[]>(() => {
    const snapshotBySlug = new Map((layoutData.snapshots ?? []).map((snap) => [snap.slug, snap]));
    return data.points.map((point) => {
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
      return {
        label: snapshotDisplayLabel(snapshot),
        slug: point.slug,
        sourceDate: snapshotSourceDate(snapshot),
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

  function countFor(s: Series, key: string): number {
    return s.buckets.find((b) => b.key === key)?.count ?? 0;
  }
</script>

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
    <h3>{TREND_CATEGORIES.find((c) => c.key === data.category)?.label}</h3>
    <ol class="trend-list" aria-label="Stacked distribution per snapshot">
      {#each series as s (s.slug)}
        <li class="trend-row">
          <div class="trend-meta">
            <span class="trend-slug" title={`Snapshot ${s.slug}`}>{s.label}</span>
            {#if s.sourceDate && s.sourceDate !== s.label}
              <span class="trend-captured">{s.sourceDate}</span>
            {/if}
          </div>
          <div class="trend-bar" aria-hidden="true">
            {#each bucketKeys as key, i (key)}
              {@const pct = pctFor(s, key)}
              {@const count = countFor(s, key)}
              {#if pct > 0}
                <span
                  class="trend-segment"
                  style:width="{pct}%"
                  style:background={colorForBucket(key, i)}
                  style:color={colorFgForBucket(key, i)}
                  title="{labelForKey(key)}: {formatCount(count)} ({pct}%)"
                >
                  <span class="trend-segment-label">{labelForKey(key)}</span>
                  <span class="trend-segment-count">{formatCount(count)}</span>
                </span>
              {/if}
            {/each}
          </div>
          <span class="trend-total">{totalFor(s)}</span>
        </li>
      {/each}
    </ol>
    <ul class="trend-legend" aria-label="Buckets">
      {#each bucketKeys as key, i (key)}
        <li class="trend-legend-item">
          <span class="legend-swatch" style:background={colorForBucket(key, i)}></span>
          <span class="legend-label">{labelForKey(key)}</span>
        </li>
      {/each}
    </ul>
  </section>
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
  .legend-swatch {
    display: inline-block;
    width: 12px;
    height: 12px;
    border-radius: 3px;
  }
  .legend-label {
    font-family: var(--mono);
  }
</style>
