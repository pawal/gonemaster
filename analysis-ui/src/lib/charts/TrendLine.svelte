<script lang="ts">
  import { layoutLine, pickLabelIndices } from "./trendLayout";
  import { formatCount } from "$lib/format";

  // Full trend chart for the trends focus view: one numeric series across
  // snapshots, with y-axis ticks, dots, and a visually-hidden data table so
  // the values reach screen readers and touch users (the SVG itself is
  // decorative). Colour is set through a tone class, keeping stroke/fill
  // CSP-safe and theme-aware.
  type Tone = "ok" | "notice" | "warning" | "error" | "critical" | "neutral";
  export type Marker = { index: number; label: string };
  type Props = {
    values: number[];
    labels: string[];
    tone?: Tone;
    // Column headers for the accessible table.
    seriesLabel?: string;
    // "%" for share mode; appended to axis ticks and table values.
    valueSuffix?: string;
    yDomain?: [number, number];
    minSpan?: number;
    // Epoch ms per value for a time axis.
    xValues?: number[];
    markers?: Marker[];
    // Controlled hover; omit to let the chart track its own.
    hoverIndex?: number | null;
    onhover?: (index: number | null) => void;
    compact?: boolean;
    table?: boolean;
    caption: string;
  };
  let {
    values,
    labels,
    tone = "neutral",
    seriesLabel = "Count",
    valueSuffix = "",
    yDomain,
    minSpan,
    xValues,
    markers = [],
    hoverIndex,
    onhover,
    compact = false,
    table = true,
    caption
  }: Props = $props();

  const width = $derived(compact ? 280 : 640);
  const height = $derived(compact ? 120 : 220);
  const pad = $derived(
    compact
      ? { top: 8, right: 8, bottom: 20, left: 36 }
      : { top: 12, right: 12, bottom: 24, left: 40 }
  );

  const layout = $derived(
    layoutLine(values, { width, height, padding: pad, yDomain, minSpan, xValues, tickCount: compact ? 2 : 4 })
  );

  // The last label is end-anchored.
  const xLabelIndices = $derived(pickLabelIndices(layout.points.map((p) => p.x), 64, 96));
  const lastIndex = $derived(layout.points.length - 1);

  // Hit bands run between the midpoints of neighbouring points.
  const bands = $derived(
    layout.points.map((p, i, all) => {
      const left = i === 0 ? pad.left : (all[i - 1].x + p.x) / 2;
      const right = i === all.length - 1 ? width - pad.right : (p.x + all[i + 1].x) / 2;
      return { index: p.index, x: left, width: Math.max(0, right - left) };
    })
  );

  let localHover = $state<number | null>(null);
  const active = $derived(hoverIndex !== undefined ? hoverIndex : localHover);
  const activePoint = $derived(active === null ? null : (layout.points[active] ?? null));
  const activeMarker = $derived(markers.find((m) => m.index === active));

  function setHover(index: number | null) {
    if (index === active) return;
    localHover = index;
    onhover?.(index);
  }

  // Delegated from the hit bands, which carry their point index.
  function onmove(event: PointerEvent) {
    const raw = (event.target as Element | null)?.getAttribute?.("data-index");
    if (raw != null) setHover(Number(raw));
  }

  function fmt(value: number): string {
    return `${formatCount(value)}${valueSuffix}`;
  }
</script>

{#if layout.points.length === 0}
  <p class="hint">No data points to plot.</p>
{:else}
  <figure class="trend-line tone-{tone}" class:compact>
    <svg viewBox="0 0 {width} {height}" role="img" aria-label={caption} onpointermove={onmove} onpointerleave={() => setHover(null)}>
      {#each layout.yTicks as tick (tick.value)}
        <line class="grid" x1={pad.left} x2={width - pad.right} y1={tick.y} y2={tick.y} />
        <text class="tick-label" x={pad.left - 6} y={tick.y + 4} text-anchor="end">{fmt(tick.value)}</text>
      {/each}
      {#each markers as m (m.index)}
        {@const p = layout.points[m.index]}
        {#if p}
          <line class="marker" x1={p.x} x2={p.x} y1={pad.top} y2={layout.baselineY}>
            <title>{m.label}</title>
          </line>
        {/if}
      {/each}
      {#if layout.areaPath}
        <path class="area" d={layout.areaPath} />
      {/if}
      <path class="line" d={layout.linePath} />
      {#if activePoint}
        <line class="crosshair" x1={activePoint.x} x2={activePoint.x} y1={pad.top} y2={layout.baselineY} />
      {/if}
      {#each layout.points as p (p.index)}
        <circle class="dot" class:active={p.index === active} cx={p.x} cy={p.y} r={p.index === active ? 4 : 2.5}>
          <title>{labels[p.index]}: {fmt(p.value)}</title>
        </circle>
        {#if xLabelIndices.has(p.index)}
          <text
            class="x-label"
            x={p.x}
            y={height - 6}
            text-anchor={p.index === lastIndex && lastIndex > 0 ? "end" : "middle"}
          >{labels[p.index]}</text>
        {/if}
      {/each}
      {#each bands as b (b.index)}
        <rect
          class="hit"
          data-index={b.index}
          x={b.x}
          y={pad.top}
          width={b.width}
          height={layout.baselineY - pad.top}
        />
      {/each}
    </svg>
    {#if !compact && activePoint}
      <p class="hover-note">
        {labels[activePoint.index]}: {fmt(activePoint.value)}{activeMarker ? `. ${activeMarker.label}` : ""}
      </p>
    {/if}
    {#if table}
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
    {/if}
    {#if !compact}
      <figcaption class="hint">{caption}</figcaption>
    {/if}
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
  .dot.active {
    stroke-width: 2;
  }
  .grid {
    stroke: var(--border);
    stroke-width: 1;
  }
  .marker {
    stroke: var(--muted);
    stroke-width: 1;
    stroke-dasharray: 3 3;
  }
  .crosshair {
    stroke: var(--ink-2);
    stroke-width: 1;
  }
  .hit {
    fill: transparent;
  }
  .tick-label,
  .x-label {
    fill: var(--ink-2);
    font-family: var(--mono);
    font-size: 10px;
  }
  .hover-note {
    margin: 0;
    font-family: var(--mono);
    font-size: var(--text-xs);
    color: var(--ink-2);
    text-align: center;
  }
  figcaption {
    text-align: center;
  }
</style>
