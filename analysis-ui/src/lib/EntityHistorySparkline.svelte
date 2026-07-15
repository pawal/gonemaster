<script lang="ts">
  import Sparkline from "$lib/charts/Sparkline.svelte";
  import type { HistoryPoint } from "$lib/api";

  type Metric = "domain_count" | "latency_p50_ms" | "score";
  type Tone = "ok" | "notice" | "warning" | "error" | "critical" | "neutral";
  type Props = {
    points: HistoryPoint[];
    metric: Metric;
    label: string;
    tone?: Tone;
  };
  let { points, metric, label, tone = "neutral" }: Props = $props();

  // Present points only; a missing metric drops out so the line stays honest.
  const values = $derived(
    points
      .filter((p) => p.present)
      .map((p) =>
        metric === "domain_count"
          ? p.domain_count
          : metric === "latency_p50_ms"
            ? p.latency_p50_ms
            : p.score
      )
      .filter((v): v is number => typeof v === "number" && Number.isFinite(v))
  );
</script>

{#if values.length >= 2}
  <div class="history-spark">
    <span class="history-label">{label}</span>
    <Sparkline {values} {tone} ariaLabel={`${label} across ${values.length} snapshots`} />
    <span class="history-range">{values.length} snapshots</span>
  </div>
{/if}

<style>
  .history-spark {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    margin-top: var(--space-3);
  }
  .history-label,
  .history-range {
    font-size: var(--text-xs);
    color: var(--ink-2);
  }
</style>
