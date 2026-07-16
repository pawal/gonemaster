<script lang="ts">
  import { layoutSparkline } from "./trendLayout";

  // Compact trend line for stat tiles. Colour comes from a tone class that
  // sets CSS custom properties, so stroke/fill stay CSP-safe (no inline
  // styles) and follow the light/dark theme.
  type Tone = "ok" | "notice" | "warning" | "error" | "critical" | "neutral";
  type Props = {
    values: number[];
    tone?: Tone;
    yDomain?: [number, number];
    width?: number;
    height?: number;
    ariaLabel: string;
  };
  let {
    values,
    tone = "neutral",
    yDomain,
    width = 96,
    height = 24,
    ariaLabel
  }: Props = $props();

  const layout = $derived(layoutSparkline(values, { width, height, yDomain }));
</script>

{#if layout.points.length > 0}
  <span class="sparkline tone-{tone}" role="img" aria-label={ariaLabel}>
    <svg viewBox="0 0 {width} {height}" preserveAspectRatio="none" aria-hidden="true">
      {#if layout.areaPath}
        <path class="spark-area" d={layout.areaPath} />
      {/if}
      {#if layout.linePath}
        <path class="spark-line" d={layout.linePath} />
      {/if}
      {#if layout.last}
        <circle class="spark-dot" cx={layout.last.x} cy={layout.last.y} r="2" />
      {/if}
    </svg>
  </span>
{/if}

<style>
  .sparkline {
    display: inline-block;
    line-height: 0;
  }
  .sparkline svg {
    display: block;
    width: 100%;
    height: auto;
    overflow: visible;
  }
  .tone-ok       { --line: var(--bar-ok);       --fill: var(--tone-ok-bg); }
  .tone-notice   { --line: var(--bar-notice);   --fill: var(--tone-notice-bg); }
  .tone-warning  { --line: var(--bar-warning);  --fill: var(--tone-warning-bg); }
  .tone-error    { --line: var(--bar-error);    --fill: var(--tone-error-bg); }
  .tone-critical { --line: var(--bar-critical); --fill: var(--tone-critical-bg); }
  .tone-neutral  { --line: var(--bar-neutral);  --fill: var(--surface-2); }
  .spark-line {
    fill: none;
    stroke: var(--line);
    stroke-width: 1.5;
    stroke-linejoin: round;
    stroke-linecap: round;
    vector-effect: non-scaling-stroke;
  }
  .spark-area {
    fill: var(--fill);
    opacity: 0.5;
    stroke: none;
  }
  .spark-dot {
    fill: var(--line);
    stroke: none;
  }
</style>
