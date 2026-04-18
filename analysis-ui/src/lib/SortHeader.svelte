<script lang="ts">
  import { currentSortState, nextSortToken, sortIndicator, type SortColumnSpec } from "$lib/sort";

  type Props = {
    label: string;
    spec: SortColumnSpec;
    currentSort: string;
    align?: "left" | "right";
    onsort: (nextToken: string) => void;
  };

  let { label, spec, currentSort, align = "left", onsort }: Props = $props();

  const state = $derived(currentSortState(currentSort, spec));
  const disabled = $derived(!spec.asc && !spec.desc);

  function handleClick() {
    onsort(nextSortToken(state, spec));
  }
</script>

<button
  type="button"
  class="sort-header"
  class:active={state !== "off"}
  class:align-right={align === "right"}
  {disabled}
  aria-label={`Sort by ${label}${state === "asc" ? " (ascending)" : state === "desc" ? " (descending)" : ""}`}
  onclick={handleClick}
>
  <span class="sort-label">{label}</span>
  {#if !disabled}
    <span class="sort-arrow" aria-hidden="true">{sortIndicator(state)}</span>
  {/if}
</button>

<style>
  .sort-header {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    background: transparent;
    border: none;
    padding: 0;
    margin: 0;
    font: inherit;
    color: inherit;
    text-transform: inherit;
    letter-spacing: inherit;
    cursor: pointer;
  }
  .sort-header:disabled { cursor: default; }
  .sort-header:not(:disabled):hover .sort-label { text-decoration: underline; }
  .sort-header.align-right { justify-content: flex-end; width: 100%; }
  .sort-header.active { color: var(--ink); }
  .sort-arrow {
    font-size: 0.85em;
    color: var(--ink-2);
  }
  .sort-header.active .sort-arrow { color: var(--accent-2); }
</style>
