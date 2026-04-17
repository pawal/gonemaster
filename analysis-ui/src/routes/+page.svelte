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

<section class="page">
  <h1>Cohort Analysis</h1>
  <p class="hint">
    This is the scaffold for the standalone analysis dashboard. Page structure, filter bar, and
    entity pages arrive in the next phase-4 tasks.
  </p>

  {#if loading}
    <p class="status">Loading catalog…</p>
  {:else if loadError}
    <p class="status error">Failed to load catalog: {loadError}</p>
  {:else if !catalog || catalog.cohorts.length === 0}
    <p class="status">No public analysis cohorts are configured yet.</p>
  {:else}
    <p class="status">
      Default cohort:
      <code>{catalog.default_tag ?? "(none)"}</code> · selector enabled:
      <code>{String(catalog.selector_enabled)}</code>
    </p>
    <ul class="cohort-list">
      {#each catalog.cohorts as cohort (cohort.dataset_tag)}
        <li>
          <strong>{cohort.label}</strong>
          <code>{cohort.dataset_tag}</code>
          {#if cohort.is_default}
            <span class="pill">default</span>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</section>
