<script lang="ts">
  import { onMount } from "svelte";

  type Cohort = {
    dataset_tag: string;
    label: string;
    description?: string;
    is_default: boolean;
  };

  type CatalogResponse = {
    default_tag?: string;
    cohorts: Cohort[];
    selector_enabled: boolean;
  };

  let catalog = $state<CatalogResponse | null>(null);
  let loadError = $state<string>("");
  let loading = $state<boolean>(true);

  onMount(async () => {
    try {
      const response = await fetch("/pub/api/v1/analysis/catalog");
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
      }
      catalog = (await response.json()) as CatalogResponse;
    } catch (error) {
      loadError = error instanceof Error ? error.message : String(error);
    } finally {
      loading = false;
    }
  });
</script>

<section class="scope-bar" aria-label="Analysis scope">
  <span class="scope-label">Cohort</span>
  {#if loading}
    <span class="scope-value">…</span>
  {:else if catalog?.default_tag}
    <span class="scope-value">{catalog.default_tag}</span>
  {:else}
    <span class="scope-value">(not configured)</span>
  {/if}
  <span class="scope-label">Scope</span>
  <span class="scope-value">latest_global</span>
  <span class="placeholder-note">· batch / time-window filters land in the next phase-4 task.</span>
</section>

<section class="card">
  <h2>Overview</h2>
  <p class="hint">
    Overview metrics (domain totals, top findings, infrastructure concentration) arrive in phase 5.
    This scaffold confirms the public catalog endpoint is reachable from the embedded dashboard.
  </p>

  {#if loading}
    <p class="status-banner">Loading catalog…</p>
  {:else if loadError}
    <p class="status-banner error">Failed to load catalog: {loadError}</p>
  {:else if !catalog || catalog.cohorts.length === 0}
    <p class="status-banner warn">No public analysis cohorts are configured yet.</p>
  {:else}
    <ul class="cohort-list">
      {#each catalog.cohorts as cohort (cohort.dataset_tag)}
        <li>
          <strong>{cohort.label}</strong>
          <code>{cohort.dataset_tag}</code>
          {#if cohort.is_default}
            <span class="pill default">default</span>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</section>
