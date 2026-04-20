<script lang="ts">
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import ASNChip from "$lib/chips/ASNChip.svelte";
  import DomainChip from "$lib/chips/DomainChip.svelte";
  import NameserverChip from "$lib/chips/NameserverChip.svelte";
  import PrefixChip from "$lib/chips/PrefixChip.svelte";
  import { formatCount } from "$lib/format";
  import type { EndpointDetailPageData } from "./+page";

  let { data }: { data: EndpointDetailPageData } = $props();

  const query = $derived(page.url.search);
  const title = $derived(
    data.nameserver ? `${data.nameserver} · ${data.address}` : data.address
  );
</script>

<nav class="breadcrumbs" aria-label="Breadcrumb">
  <a href={`${base}/endpoints${query}`}>← Endpoints</a>
</nav>

{#if !data.datasetTag}
  <section class="card">
    <h2>{title}</h2>
    <p class="status-banner warn">No cohort resolved. Configure a public cohort to view endpoint details.</p>
  </section>
{:else if data.error}
  <section class="card">
    <h2>{title}</h2>
    <p class="status-banner error">Failed to load endpoint: {data.error}</p>
  </section>
{:else if !data.detail}
  <section class="card">
    <h2>{title}</h2>
    <p class="status-banner">Endpoint not materialized for this cohort.</p>
  </section>
{:else}
  {@const d = data.detail}
  <section class="card">
    <div class="detail-header">
      <h2>{d.address}</h2>
      <div class="detail-scorecard">
        <NameserverChip nameserver={d.nameserver} />
        <span class="family-pill">{d.family}</span>
        {#if d.asn !== undefined && d.asn !== null}
          <ASNChip asn={d.asn} label={d.asn_label} />
        {/if}
        {#if d.prefix}
          <PrefixChip prefix={d.prefix} />
        {/if}
      </div>
    </div>

    <dl class="detail-counts">
      <div><dt>Domains</dt><dd>{formatCount(d.domain_count)}</dd></div>
    </dl>
  </section>

  <section class="card">
    <h3>Domains served via this endpoint</h3>
    {#if d.domains.length === 0}
      <p class="hint">No domains in this cohort use this endpoint.</p>
    {:else}
      <ul class="chip-list" role="list">
        {#each d.domains as domain (domain)}
          <li><DomainChip {domain} /></li>
        {/each}
      </ul>
    {/if}
  </section>
{/if}

<style>
  .breadcrumbs { font-size: var(--text-sm); }
  .breadcrumbs a { color: var(--ink-2); text-decoration: none; }
  .breadcrumbs a:hover { color: var(--accent-2); }

  .detail-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    flex-wrap: wrap;
  }
  .detail-header h2 { margin: 0; font-family: var(--mono); }
  .detail-scorecard {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    flex-wrap: wrap;
  }
  .family-pill {
    display: inline-block;
    padding: 2px 8px;
    border-radius: 6px;
    font-size: var(--text-xs);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    background: var(--surface-2);
    color: var(--on-surface-2);
  }

  .detail-counts {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-6);
    margin: var(--space-3) 0 0;
  }
  .detail-counts > div { display: flex; flex-direction: column; gap: 2px; }
  .detail-counts dt {
    font-size: var(--text-xs);
    color: var(--ink-2);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .detail-counts dd {
    margin: 0;
    font-family: var(--mono);
    font-size: var(--text-lg);
    font-weight: 600;
  }

  .chip-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }
</style>
