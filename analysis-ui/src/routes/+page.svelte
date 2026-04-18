<script lang="ts">
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import FilterBar from "$lib/FilterBar.svelte";
  import { formatCount, formatTimestamp } from "$lib/format";
  import type { LayoutData } from "./+layout";
  import type { OverviewPageData } from "./+page";

  let { data }: { data: OverviewPageData } = $props();

  const layoutData = $derived(page.data as LayoutData);

  const summaryCards = $derived.by(() => {
    const d = data.detail;
    if (!d) return [];
    return [
      { label: "Domains", value: d.domain_count, href: "/domains" },
      { label: "Nameservers", value: d.nameserver_count, href: "/nameservers" },
      { label: "Endpoints", value: d.endpoint_count, href: "/endpoints" },
      { label: "ASNs", value: d.asn_count, href: "/asns" },
      { label: "Prefixes", value: d.prefix_count, href: "/prefixes" }
    ];
  });

  const query = $derived(page.url.search);
</script>

<FilterBar
  cohorts={layoutData.catalog?.cohorts ?? []}
  selectorEnabled={layoutData.catalog?.selector_enabled ?? false}
  showSearch={false}
/>

{#if layoutData.catalogError}
  <section class="card">
    <h2>Overview</h2>
    <p class="status-banner error">Failed to load catalog: {layoutData.catalogError}</p>
  </section>
{:else if !data.datasetTag}
  <section class="card empty-state">
    <h2>No public cohort is published yet</h2>
    <p class="hint">
      The analysis dashboard only shows cohorts that have been explicitly marked as public.
      A freshly-created cohort defaults to <code>analysis_enabled: true</code>,
      <code>public_enabled: false</code> — it will materialize data in the background but
      stays hidden here until an admin publishes it.
    </p>
    <ol class="hint next-steps">
      <li>Open the admin UI (Settings → Analysis).</li>
      <li>Toggle <strong>Public</strong> on for the cohort you want to expose here.</li>
      <li>Optionally click <strong>Make default</strong> so it becomes the default view.</li>
    </ol>
  </section>
{:else if data.detailError}
  <section class="card">
    <h2>Overview</h2>
    <p class="status-banner error">Failed to load cohort: {data.detailError}</p>
  </section>
{:else if data.detail}
  {@const d = data.detail}
  {@const isEmpty = (d.domain_count ?? 0) === 0}
  <section class="card overview-header">
    <h2>{d.label}</h2>
    {#if d.description}
      <p class="hint">{d.description}</p>
    {/if}
    {#if formatTimestamp(d.last_materialized_at)}
      <dl class="overview-meta">
        <div>
          <dt>Last analyzed</dt>
          <dd>{formatTimestamp(d.last_materialized_at)}</dd>
        </div>
      </dl>
    {/if}
  </section>

  {#if isEmpty}
    <section class="card empty-state">
      <h3>No data has been materialized yet</h3>
      <p class="hint">
        This cohort is published but the projector hasn't seen any matching runs yet.
        Run a job (or batch) tagged with <code>{d.dataset_tag}</code>, or trigger
        <strong>Rebuild</strong> from the admin UI to project any existing runs that
        already carry this tag.
      </p>
    </section>
  {:else}
    <section class="summary-grid" aria-label="Cohort summary counts">
      {#each summaryCards as card (card.label)}
        <a class="summary-card" href={`${base}${card.href}${query}`}>
          <span class="summary-count">{formatCount(card.value)}</span>
          <span class="summary-label">{card.label}</span>
        </a>
      {/each}
    </section>
  {/if}
{/if}

<style>
  .overview-header {
    gap: var(--space-2);
  }
  .overview-header h2 {
    margin: 0;
  }
  .overview-meta {
    display: flex;
    gap: var(--space-6);
    flex-wrap: wrap;
    margin: 0;
  }
  .overview-meta > div {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .overview-meta dt {
    font-size: var(--text-xs);
    color: var(--ink-2);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .overview-meta dd {
    margin: 0;
    font-size: var(--text-sm);
  }

  .summary-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
    gap: var(--space-3);
  }

  .summary-card {
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding: var(--space-4) var(--space-5);
    background: var(--card);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    text-decoration: none;
    color: var(--ink);
    transition: border-color 0.15s ease, transform 0.15s ease;
  }
  .summary-card:hover {
    border-color: var(--accent-2);
    transform: translateY(-1px);
  }
  .summary-count {
    font-size: var(--text-2xl);
    font-weight: 700;
    font-family: var(--mono);
    color: var(--ink);
    line-height: 1.1;
  }
  .summary-label {
    font-size: var(--text-sm);
    color: var(--ink-2);
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }

  .empty-state h2,
  .empty-state h3 {
    margin: 0;
    color: var(--ink);
  }

  .empty-state code {
    font-family: var(--mono);
    font-size: var(--text-sm);
    background: var(--surface-2);
    color: var(--on-surface-2);
    padding: 1px 6px;
    border-radius: 4px;
  }

  .next-steps {
    padding-left: var(--space-5);
    margin: var(--space-2) 0 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
</style>
