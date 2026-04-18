<script lang="ts">
  import { goto } from "$app/navigation";
  import { page } from "$app/state";
  import type { Cohort } from "$lib/api";
  import { applyFilterToParams, searchToString, type FilterKey } from "$lib/filters";

  type Props = {
    cohorts?: Cohort[];
    selectorEnabled?: boolean;
    showSearch?: boolean;
  };

  let { cohorts = [], selectorEnabled = false, showSearch = true }: Props = $props();

  const params = $derived(page.url.searchParams);
  const datasetTag = $derived(params.get("dataset_tag") ?? "");
  const search = $derived(params.get("search") ?? "");

  function patch(updates: Partial<Record<FilterKey, string>>) {
    const next = applyFilterToParams(params, updates);
    goto(`${page.url.pathname}${searchToString(next)}`, {
      replaceState: true,
      noScroll: true,
      keepFocus: true
    });
  }

  function onChange(key: FilterKey, value: string) {
    patch({ [key]: value });
  }

  function clearAll() {
    goto(`${page.url.pathname}`, {
      replaceState: true,
      noScroll: true,
      keepFocus: true
    });
  }

  const hasActiveFilters = $derived(params.toString().length > 0);
  const showCohort = $derived(selectorEnabled && cohorts.length > 1);
  const visible = $derived(showCohort || showSearch);
</script>

{#if visible}
  <section class="filter-bar" aria-label="Cohort and search">
    {#if showCohort}
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
{/if}

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
