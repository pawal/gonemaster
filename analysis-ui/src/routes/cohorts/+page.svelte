<script lang="ts">
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import CohortChip from "$lib/chips/CohortChip.svelte";
  import type { CohortsPageData } from "./+page";

  let { data }: { data: CohortsPageData } = $props();

  // When a cohort is clicked we navigate to the overview with the
  // corresponding ?dataset_tag= so the rest of the dashboard rescopes.
  const overviewHref = (tag: string) => `${base}/?dataset_tag=${encodeURIComponent(tag)}`;

  const activeTag = $derived(page.url.searchParams.get("dataset_tag") ?? "");
</script>

<section class="card">
  <h2>Cohorts</h2>
  <p class="hint">Public analysis cohorts exposed by the server. Pick one to rescope the dashboard.</p>

  {#if data.error}
    <p class="status-banner error">Failed to load cohorts: {data.error}</p>
  {:else if data.cohorts.length === 0}
    <p class="status-banner warn">No public cohorts are configured yet.</p>
  {:else}
    <ul class="cohort-select-list" role="list">
      {#each data.cohorts as cohort (cohort.dataset_tag)}
        {@const isActive = cohort.dataset_tag === activeTag}
        <li class:active={isActive}>
          <a class="cohort-select" href={overviewHref(cohort.dataset_tag)}>
            <div class="cohort-select-head">
              <CohortChip datasetTag={cohort.dataset_tag} label={cohort.dataset_tag} preserveQuery={false} />
              <strong>{cohort.label}</strong>
              {#if cohort.is_default}<span class="pill default">default</span>{/if}
              {#if isActive}<span class="pill">active</span>{/if}
            </div>
            {#if cohort.description}
              <p class="hint">{cohort.description}</p>
            {/if}
          </a>
        </li>
      {/each}
    </ul>
  {/if}
</section>

<style>
  .cohort-select-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: grid;
    gap: var(--space-2);
  }

  .cohort-select {
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding: var(--space-3) var(--space-4);
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    color: var(--ink);
    text-decoration: none;
    transition: border-color 0.15s ease, transform 0.15s ease;
  }
  .cohort-select:hover {
    border-color: var(--accent-2);
    transform: translateY(-1px);
  }

  .cohort-select-head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .active .cohort-select {
    border-color: var(--accent-2);
    background: rgba(3, 105, 161, 0.04);
  }
</style>
