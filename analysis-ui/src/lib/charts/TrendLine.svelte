<script lang="ts">
  import { layoutLine } from "./trendLayout";
  import { formatCount } from "$lib/format";

  // Full trend chart for the trends focus view: one numeric series across
  // snapshots, with y-axis ticks, dots, and a visually-hidden data table so
  // the values reach screen readers and touch users (the SVG itself is
  // decorative). Colour is set through a tone class, keeping stroke/fill
  // CSP-safe and theme-aware.
  type Tone = "ok" | "notice" | "warning" | "error" | "critical" | "neutral";
  type Props = {
    values: number[];
    labels: string[];
    tone?: Tone;
    // Column headers for the accessible table.
    seriesLabel?: string;
    // "%" for share mode; appended to axis ticks and table values.
    valueSuffix?: string;
    yDomain?: [number, number];
    width?: number;
    height?: number;
    caption: string;
  };
  let {
    values,
    labels,
    tone = "neutral",
    seriesLabel = "Count",
    valueSuffix = "",
    yDomain,
    width = 640,
    height = 220,
    caption
  }: Props = $props();

  const layout = $derived(layoutLine(values, { width, height, yDomain }));

  // Label only a handful of x positions so the axis never crowds; always
  // include the first and last snapshot.
  const xLabelIndices = $derived.by<Set<number>>(() => {
    const n = labels.length;
    if (n === 0) return new Set();
    const max = 6;
    if (n <= max) return new Set(labels.map((_, i) => i));
    const step = (n - 1) / (max - 1);
    const idx = new Set<number>();
    for (let i = 0; i < max; i++) idx.add(Math.round(i * step));
    return idx;
  });

  function fmt(value: number): string {
    return `${formatCount(value)}${valueSuffix}`;
  }
</script>

{#if layout.points.length === 0}
  <p class="hint">No data points to plot.</p>
{:else}
  <figure class="trend-line tone-{tone}">
    <svg viewBox="0 0 {width} {height}" role="img" aria-label={caption}>
      <!-- Y gridlines + tick labels -->
      {#each layout.yTicks as tick (tick.value)}
        <line class="grid" x1="40" x2={width - 12} y1={tick.y} y2={tick.y} />
        <text class="tick-label" x="34" y={tick.y + 4} text-anchor="end">{fmt(tick.value)}</text>
      {/each}
      {#if layout.areaPath}
        <path class="area" d={layout.areaPath} />
      {/if}
      <path class="line" d={layout.linePath} />
      {#each layout.points as p (p.index)}
        <circle class="dot" cx={p.x} cy={p.y} r="2.5">
          <title>{labels[p.index]}: {fmt(p.value)}</title>
        </circle>
        {#if xLabelIndices.has(p.index)}
          <text class="x-label" x={p.x} y={height - 8} text-anchor="middle">{labels[p.index]}</text>
        {/if}
      {/each}
    </svg>
    <table class="sr-only">
      <caption>{caption}</caption>
      <thead>
        <tr><th>Snapshot</th><th>{seriesLabel}</th></tr>
      </thead>
      <tbody>
        {#each layout.points as p (p.index)}
          <tr><td>{labels[p.index]}</td><td>{fmt(p.value)}</td></tr>
        {/each}
      </tbody>
    </table>
    <figcaption class="hint">{caption}</figcaption>
  </figure>
{/if}

<style>
  .trend-line {
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .trend-line svg {
    display: block;
    width: 100%;
    height: auto;
  }
  .tone-ok       { --line: var(--bar-ok);       --fill: var(--tone-ok-bg); }
  .tone-notice   { --line: var(--bar-notice);   --fill: var(--tone-notice-bg); }
  .tone-warning  { --line: var(--bar-warning);  --fill: var(--tone-warning-bg); }
  .tone-error    { --line: var(--bar-error);    --fill: var(--tone-error-bg); }
  .tone-critical { --line: var(--bar-critical); --fill: var(--tone-critical-bg); }
  .tone-neutral  { --line: var(--bar-neutral);  --fill: var(--surface-2); }
  .line {
    fill: none;
    stroke: var(--line);
    stroke-width: 2;
    stroke-linejoin: round;
    stroke-linecap: round;
  }
  .area {
    fill: var(--fill);
    opacity: 0.4;
    stroke: none;
  }
  .dot {
    fill: var(--line);
    stroke: var(--card);
    stroke-width: 1;
  }
  .grid {
    stroke: var(--border);
    stroke-width: 1;
  }
  .tick-label,
  .x-label {
    fill: var(--ink-2);
    font-family: var(--mono);
    font-size: 10px;
  }
  figcaption {
    text-align: center;
  }
</style>
