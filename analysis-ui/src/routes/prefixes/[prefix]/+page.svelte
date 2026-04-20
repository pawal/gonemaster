<script lang="ts">
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import ASNChip from "$lib/chips/ASNChip.svelte";
  import DomainChip from "$lib/chips/DomainChip.svelte";
  import EndpointChip from "$lib/chips/EndpointChip.svelte";
  import { formatCount } from "$lib/format";
  import type { PrefixDetailPageData } from "./+page";

  let { data }: { data: PrefixDetailPageData } = $props();

  const query = $derived(page.url.search);
</script>

<nav class="breadcrumbs" aria-label="Breadcrumb">
  <a href={`${base}/prefixes${query}`}>← Prefixes</a>
</nav>

{#if !data.datasetTag}
  <section class="card">
    <h2>{data.prefix}</h2>
    <p class="status-banner warn">No cohort resolved. Configure a public cohort to view prefix details.</p>
  </section>
{:else if data.error}
  <section class="card">
    <h2>{data.prefix}</h2>
    <p class="status-banner error">Failed to load prefix: {data.error}</p>
  </section>
{:else if !data.detail}
  <section class="card">
    <h2>{data.prefix}</h2>
    <p class="status-banner">Prefix not materialized for this cohort.</p>
  </section>
{:else}
  {@const d = data.detail}
  <section class="card">
    <div class="detail-header">
      <h2>{d.prefix}</h2>
      <span class="family-pill">{d.family}</span>
    </div>

    <dl class="detail-counts">
      <div><dt>Domains</dt><dd>{formatCount(d.domain_count)}</dd></div>
      <div><dt>Addresses</dt><dd>{formatCount(d.address_count)}</dd></div>
    </dl>
  </section>

  <section class="card">
    <h3>ASNs</h3>
    {#if d.asns.length === 0}
      <p class="hint">No ASNs associated with this prefix.</p>
    {:else}
      <ul class="chip-list" role="list">
        {#each d.asns as asn (asn)}
          <li><ASNChip {asn} /></li>
        {/each}
      </ul>
    {/if}
  </section>

  <section class="card">
    <h3>Domains using this prefix</h3>
    {#if d.domains.length === 0}
      <p class="hint">No domains in this cohort use addresses inside this prefix.</p>
    {:else}
      <ul class="chip-list" role="list">
        {#each d.domains as domain (domain)}
          <li><DomainChip {domain} /></li>
        {/each}
      </ul>
    {/if}
  </section>

  <section class="card">
    <h3>Addresses in this prefix</h3>
    {#if d.addresses.length === 0}
      <p class="hint">No addresses materialized inside this prefix.</p>
    {:else}
      <ul class="chip-list" role="list">
        {#each d.addresses as address (address)}
          <li><EndpointChip {address} nameserver={null} /></li>
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
    gap: var(--space-3);
    flex-wrap: wrap;
  }
  .detail-header h2 { margin: 0; font-family: var(--mono); }
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
