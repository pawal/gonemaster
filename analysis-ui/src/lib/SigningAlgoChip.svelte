<script lang="ts">
  type Props = {
    algo?: number | null;
    label?: string;
    tone?: string;
  };

  let { algo, label, tone }: Props = $props();

  const signed = $derived(algo !== undefined && algo !== null);
  const text = $derived(label || (signed ? `ALGO ${algo}` : ""));
</script>

{#if signed}
  <span class={`algo-chip tone-${tone || "neutral"}`} title={`DNSKEY algorithm ${algo}`}>
    {text}
  </span>
{:else}
  <span class="algo-none">-</span>
{/if}

<style>
  .algo-chip {
    display: inline-block;
    padding: 2px 8px;
    border-radius: 6px;
    font-size: var(--text-xs);
    font-weight: 600;
    white-space: nowrap;
  }
  .algo-none {
    color: var(--ink-2);
  }

  .tone-ok       { background: var(--tone-ok-bg);       color: var(--tone-ok-fg); }
  .tone-notice   { background: var(--tone-notice-bg);   color: var(--tone-notice-fg); }
  .tone-warning  { background: var(--tone-warning-bg);  color: var(--tone-warning-fg); }
  .tone-error    { background: var(--tone-error-bg);    color: var(--tone-error-fg); }
  .tone-critical { background: var(--tone-critical-bg); color: var(--tone-critical-fg); }
  .tone-neutral  { background: var(--surface-2);        color: var(--on-surface-2); }
</style>
