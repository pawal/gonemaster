<script lang="ts">
  import { goto } from "$app/navigation";
  import { page } from "$app/state";
  import type { Cohort } from "$lib/api";
  import {
    applyFilterToParams,
    FAMILIES,
    LEVELS,
    SCOPE_MODES,
    searchToString,
    type FilterKey
  } from "$lib/filters";

  type Props = {
    cohorts?: Cohort[];
    selectorEnabled?: boolean;
    showSearch?: boolean;
  };

  let { cohorts = [], selectorEnabled = false, showSearch = true }: Props = $props();

  const params = $derived(page.url.searchParams);
  const scopeMode = $derived(params.get("scope_mode") ?? "latest_global");
  const datasetTag = $derived(params.get("dataset_tag") ?? "");
  const batchID = $derived(params.get("batch_id") ?? "");
  const from = $derived(params.get("from") ?? "");
  const to = $derived(params.get("to") ?? "");
  const family = $derived(params.get("family") ?? "");
  const level = $derived(params.get("level") ?? "");
  const search = $derived(params.get("search") ?? "");

  const needsBatch = $derived(scopeMode === "latest_in_batch" || scopeMode === "batch");
  const needsWindow = $derived(scopeMode === "time_window");

  function patch(updates: Partial<Record<FilterKey, string>>) {
    const next = applyFilterToParams(params, updates);
    goto(`${page.url.pathname}${searchToString(next)}`, {
      replaceState: true,
      noScroll: true,
      keepFocus: true
    });
  }

  function onScopeChange(event: Event) {
    patch({ scope_mode: (event.currentTarget as HTMLSelectElement).value });
  }

  function onChange(key: FilterKey, value: string) {
    patch({ [key]: value });
  }

  function clearAll() {
    const next = new URLSearchParams();
    goto(`${page.url.pathname}${searchToString(next)}`, {
      replaceState: true,
      noScroll: true,
      keepFocus: true
    });
  }

  const hasActiveFilters = $derived(params.toString().length > 0);
</script>

<section class="filter-bar" aria-label="Scope and filters">
  {#if selectorEnabled && cohorts.length > 1}
    <label class="filter-field">
      <span>Cohort</span>
      <select
        value={datasetTag}
        onchange={(e) => onChange("dataset_tag", e.currentTarget.value)}
      >
        <option value="">Default</option>
        {#each cohorts as cohort (cohort.dataset_tag)}
          <option value={cohort.dataset_tag}>{cohort.label}</option>
        {/each}
      </select>
    </label>
  {/if}

  <label class="filter-field">
    <span>Scope</span>
    <select value={scopeMode} onchange={onScopeChange}>
      {#each SCOPE_MODES as mode}
        <option value={mode}>{mode}</option>
      {/each}
    </select>
  </label>

  {#if needsBatch}
    <label class="filter-field">
      <span>Batch ID</span>
      <input
        type="text"
        value={batchID}
        placeholder="batch-…"
        onchange={(e) => onChange("batch_id", e.currentTarget.value.trim())}
      />
    </label>
  {/if}

  {#if needsWindow}
    <label class="filter-field">
      <span>From</span>
      <input
        type="datetime-local"
        value={from}
        onchange={(e) => onChange("from", e.currentTarget.value)}
      />
    </label>
    <label class="filter-field">
      <span>To</span>
      <input
        type="datetime-local"
        value={to}
        onchange={(e) => onChange("to", e.currentTarget.value)}
      />
    </label>
  {/if}

  <label class="filter-field">
    <span>Family</span>
    <select value={family} onchange={(e) => onChange("family", e.currentTarget.value)}>
      {#each FAMILIES as f}
        <option value={f}>{f || "any"}</option>
      {/each}
    </select>
  </label>

  <label class="filter-field">
    <span>Severity</span>
    <select value={level} onchange={(e) => onChange("level", e.currentTarget.value)}>
      {#each LEVELS as l}
        <option value={l}>{l || "any"}</option>
      {/each}
    </select>
  </label>

  {#if showSearch}
    <label class="filter-field filter-field-grow">
      <span>Search</span>
      <input
        type="search"
        value={search}
        placeholder="substring match"
        onchange={(e) => onChange("search", e.currentTarget.value.trim())}
      />
    </label>
  {/if}

  {#if hasActiveFilters}
    <button type="button" class="ghost filter-clear" onclick={clearAll}>Clear</button>
  {/if}
</section>

<style>
  .filter-bar {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    align-items: flex-end;
    padding: var(--space-3) var(--space-4);
    background: var(--card);
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }

  .filter-field {
    display: flex;
    flex-direction: column;
    gap: 2px;
    font-size: var(--text-xs);
    color: var(--ink-2);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .filter-field-grow {
    flex: 1 1 180px;
    min-width: 160px;
  }

  .filter-field span {
    font-weight: 600;
  }

  .filter-field select,
  .filter-field input {
    min-width: 140px;
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

  .filter-clear {
    margin-left: auto;
  }
</style>
