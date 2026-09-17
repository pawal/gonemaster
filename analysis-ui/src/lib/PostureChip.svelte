<script lang="ts">
  type Props = {
    posture?: string | null;
    label?: string;
    tone?: string;
  };

  let { posture, label, tone }: Props = $props();

  const known = $derived(typeof posture === "string" && posture !== "");
  const text = $derived(label || posture || "");
</script>

{#if known}
  <span class={`posture-chip tone-${tone || "neutral"}`} title={`Denial-of-existence posture: ${text}`}>
    {text}
  </span>
{:else}
  <span class="posture-none">-</span>
{/if}

<style>
  .posture-chip {
    display: inline-block;
    padding: 2px 8px;
    border-radius: 6px;
    font-size: var(--text-xs);
    font-weight: 600;
    white-space: nowrap;
  }
  .posture-none {
    color: var(--ink-2);
  }

  .tone-ok       { background: var(--tone-ok-bg);       color: var(--tone-ok-fg); }
  .tone-notice   { background: var(--tone-notice-bg);   color: var(--tone-notice-fg); }
  .tone-warning  { background: var(--tone-warning-bg);  color: var(--tone-warning-fg); }
  .tone-error    { background: var(--tone-error-bg);    color: var(--tone-error-fg); }
  .tone-critical { background: var(--tone-critical-bg); color: var(--tone-critical-fg); }
  .tone-neutral  { background: var(--surface-2);        color: var(--on-surface-2); }
</style>
