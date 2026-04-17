<script lang="ts">
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import FilterBar from "$lib/FilterBar.svelte";
  import CohortChip from "$lib/chips/CohortChip.svelte";
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
  <section class="card">
    <h2>Overview</h2>
    <p class="status-banner warn">No public analysis cohorts are configured yet.</p>
  </section>
{:else if data.detailError}
  <section class="card">
    <h2>Overview</h2>
    <p class="status-banner error">Failed to load cohort: {data.detailError}</p>
  </section>
{:else if data.detail}
  {@const d = data.detail}
  <section class="card overview-header">
    <div class="overview-title-row">
      <h2>{d.label}</h2>
      <CohortChip datasetTag={d.dataset_tag} label={d.dataset_tag} />
      {#if d.is_default}<span class="pill default">default</span>{/if}
    </div>
    {#if d.description}
      <p class="hint">{d.description}</p>
    {/if}
    <dl class="overview-meta">
      <div>
        <dt>Materialization</dt>
        <dd><span class={`pill pill-status-${d.materialization_status}`}>{d.materialization_status}</span></dd>
      </div>
      <div>
        <dt>Last materialized</dt>
        <dd>{formatTimestamp(d.last_materialized_at) || "—"}</dd>
      </div>
    </dl>
  </section>

  <section class="summary-grid" aria-label="Cohort summary counts">
    {#each summaryCards as card (card.label)}
      <a class="summary-card" href={`${base}${card.href}${query}`}>
        <span class="summary-count">{formatCount(card.value)}</span>
        <span class="summary-label">{card.label}</span>
      </a>
    {/each}
  </section>
{/if}

<style>
  .overview-header {
    gap: var(--space-2);
  }
  .overview-title-row {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    flex-wrap: wrap;
  }
  .overview-title-row h2 {
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

  .pill-status-ready  { background: #d6f1d0; color: #1e5b1e; }
  .pill-status-failed { background: #fee2e2; color: #991b1b; }
  .pill-status-pending { background: var(--surface-2); color: var(--on-surface-2); }
</style>
