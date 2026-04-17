<script lang="ts">
  import { page } from "$app/state";
  import FilterBar from "$lib/FilterBar.svelte";
  import CohortChip from "$lib/chips/CohortChip.svelte";
  import type { LayoutData } from "./+layout";

  const data = $derived(page.data as LayoutData);
</script>

<FilterBar
  cohorts={data.catalog?.cohorts ?? []}
  selectorEnabled={data.catalog?.selector_enabled ?? false}
  showSearch={false}
/>

<section class="card">
  <h2>Overview</h2>
  <p class="hint">
    Overview metrics (domain totals, top findings, infrastructure concentration) arrive in phase 5.
    This card confirms that the catalog bootstrap, shared filter bar, and entity chips are wired
    through the public analysis API.
  </p>

  {#if data.catalogError}
    <p class="status-banner error">Failed to load catalog: {data.catalogError}</p>
  {:else if !data.catalog || data.catalog.cohorts.length === 0}
    <p class="status-banner warn">No public analysis cohorts are configured yet.</p>
  {:else}
    <p class="hint">
      Resolved cohort:
      {#if data.resolvedCohort}
        <CohortChip datasetTag={data.resolvedCohort} />
      {:else}
        <em>(none)</em>
      {/if}
    </p>
    <ul class="cohort-list">
      {#each data.catalog.cohorts as cohort (cohort.dataset_tag)}
        <li>
          <CohortChip datasetTag={cohort.dataset_tag} label={cohort.label} />
          {#if cohort.description}
            <span class="hint">{cohort.description}</span>
          {/if}
          {#if cohort.is_default}
            <span class="pill default">default</span>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</section>
