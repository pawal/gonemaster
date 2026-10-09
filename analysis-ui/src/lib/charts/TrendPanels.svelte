<script lang="ts">
  import TrendLine from "./TrendLine.svelte";
  import { formatCount } from "$lib/format";

  type Tone = "ok" | "notice" | "warning" | "error" | "critical" | "neutral";
  export type PanelPoint = {
    // X axis label, a date.
    label: string;
    title: string;
    // Epoch ms; NaN when unknown.
    time: number;
    engineVersion: string;
    crossesEngineBoundary: boolean;
  };
  export type Panel = { key: string; label: string; tone: Tone; values: number[] };
  type Props = {
    points: PanelPoint[];
    panels: Panel[];
    seriesLabel?: string;
    valueSuffix?: string;
    minSpan?: number;
    caption: string;
    onselect: (key: string) => void;
  };
  let {
    points,
    panels,
    seriesLabel = "domains",
    valueSuffix = "",
    minSpan,
    caption,
    onselect
  }: Props = $props();

  let hover = $state<number | null>(null);

  const labels = $derived(points.map((p) => p.label));
  const times = $derived(points.map((p) => p.time));
  const markers = $derived(
    points.flatMap((p, index) =>
      p.crossesEngineBoundary ? [{ index, label: `Engine changed to ${p.engineVersion}` }] : []
    )
  );
  const hovered = $derived(hover === null ? null : (points[hover] ?? null));

  function fmt(value: number | undefined): string {
    return value === undefined ? "-" : `${formatCount(value)}${valueSuffix}`;
  }

  function signed(n: number): string {
    const rounded = Math.round(n * 10) / 10;
    if (rounded > 0) return `+${fmt(rounded)}`;
    if (rounded < 0) return `-${fmt(-rounded)}`;
    return `±0${valueSuffix}`;
  }
</script>

<div class="panels">
  {#each panels as panel (panel.key)}
    {@const last = panel.values.length - 1}
    <section class="panel" aria-label={panel.label}>
      <div class="panel-head">
        <button type="button" class="panel-title" title="Focus this bucket" onclick={() => onselect(panel.key)}>
          {panel.label}
        </button>
        <span class="panel-value">{fmt(panel.values[hover ?? last])}</span>
        {#if hover === null && last > 0}
          <span class="panel-delta">{signed(panel.values[last] - panel.values[0])}</span>
        {/if}
      </div>
      <TrendLine
        values={panel.values}
        {labels}
        xValues={times}
        {markers}
        tone={panel.tone}
        {valueSuffix}
        {minSpan}
        hoverIndex={hover}
        onhover={(i) => (hover = i)}
        compact
        table={false}
        caption={`${panel.label}, ${seriesLabel} per snapshot`}
      />
    </section>
  {/each}
</div>
<p class="hint panels-note">
  {#if hovered}
    {hovered.title}{hovered.title !== hovered.label ? ` (${hovered.label})` : ""},
    engine {hovered.engineVersion || "unknown"}{hovered.crossesEngineBoundary ? ". Engine changed here." : "."}
  {:else}
    Each panel has its own scale. Dashed lines mark an engine change; the delta runs from the first snapshot to the last.
  {/if}
</p>
<table class="sr-only">
  <caption>{caption}</caption>
  <thead>
    <tr>
      <th>Snapshot</th>
      <th>Engine</th>
      {#each panels as panel (panel.key)}<th>{panel.label}</th>{/each}
    </tr>
  </thead>
  <tbody>
    {#each points as point, i (i)}
      <tr>
        <td>{point.title}</td>
        <td>{point.engineVersion || "unknown"}</td>
        {#each panels as panel (panel.key)}<td>{fmt(panel.values[i])}</td>{/each}
      </tr>
    {/each}
  </tbody>
</table>

<style>
  .panels {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
    gap: var(--space-3);
    margin-top: var(--space-2);
  }
  .panel {
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
  }
  .panel-head {
    display: flex;
    align-items: baseline;
    gap: var(--space-2);
  }
  .panel-title {
    margin-right: auto;
    padding: 0;
    border: none;
    background: none;
    color: var(--ink);
    font: inherit;
    font-size: var(--text-sm);
    font-weight: 600;
    cursor: pointer;
  }
  .panel-title:hover {
    color: var(--accent-2);
    text-decoration: underline;
  }
  .panel-value,
  .panel-delta {
    font-family: var(--mono);
    font-size: var(--text-sm);
    font-variant-numeric: tabular-nums;
  }
  .panel-delta {
    color: var(--ink-2);
  }
  .panels-note {
    margin: var(--space-2) 0 0;
    min-height: 1.5em;
  }
</style>
