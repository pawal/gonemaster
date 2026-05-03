<script lang="ts">
  import { goto } from "$app/navigation";
  import { page } from "$app/state";
  import type { Cohort, SnapshotListEntry } from "$lib/api";
  import { applyFilterToParams, searchToString, type FilterKey } from "$lib/filters";
  import { snapshotOptionLabel } from "$lib/format";

  type Props = {
    cohorts?: Cohort[];
    selectorEnabled?: boolean;
    showSearch?: boolean;
    snapshots?: SnapshotListEntry[];
    defaultSnapshotSlug?: string;
  };

  let {
    cohorts = [],
    selectorEnabled = false,
    showSearch = true,
    snapshots = [],
    defaultSnapshotSlug = ""
  }: Props = $props();

  const params = $derived(page.url.searchParams);
  const datasetTag = $derived(params.get("dataset_tag") ?? "");
  const snapshot = $derived(params.get("snapshot") ?? "");
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

  // Changing the cohort drops any snapshot pin - slugs are scoped to one
  // cohort, and carrying the old slug across cohort changes would either
  // 404 or silently fall back to the new cohort's auto-latest. Neither
  // is useful, so clear the pin deliberately.
  function onCohortChange(value: string) {
    patch({ dataset_tag: value, snapshot: "" });
  }

  const showCohort = $derived(selectorEnabled && cohorts.length > 1);
  // Only render the snapshot selector when the cohort has more than one
  // captured snapshot to pick from. A single-snapshot cohort already has
  // an obvious "current" view; adding a selector with one option just
  // clutters the filter bar.
  const showSnapshot = $derived(snapshots.length > 1);
  const visible = $derived(showCohort || showSnapshot || showSearch);
  const defaultSnapshot = $derived(snapshots.find((snap) => snap.slug === defaultSnapshotSlug));
  const latestOptionLabel = $derived(
    defaultSnapshot ? `Latest (${snapshotOptionLabel(defaultSnapshot)})` : "Latest"
  );
</script>

{#if visible}
  <section class="filter-bar" aria-label="Cohort, snapshot, and search">
    {#if showCohort}
      <label class="filter-field">
        <span>Cohort</span>
        <select
          value={datasetTag}
          onchange={(e) => onCohortChange(e.currentTarget.value)}
        >
          <option value="">Default</option>
          {#each cohorts as cohort (cohort.dataset_tag)}
            <option value={cohort.dataset_tag}>{cohort.label}</option>
          {/each}
        </select>
      </label>
    {/if}

    {#if showSnapshot}
      <label class="filter-field">
        <span>Snapshot</span>
        <select
          value={snapshot}
          onchange={(e) => onChange("snapshot", e.currentTarget.value)}
        >
          <option value="">{latestOptionLabel}</option>
          {#each snapshots as snap (snap.slug)}
            <option value={snap.slug}>{snapshotOptionLabel(snap)}</option>
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
</style>
