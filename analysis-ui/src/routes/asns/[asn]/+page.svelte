<script lang="ts">
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import DomainChip from "$lib/chips/DomainChip.svelte";
  import NameserverChip from "$lib/chips/NameserverChip.svelte";
  import PrefixChip from "$lib/chips/PrefixChip.svelte";
  import EntityHistorySparkline from "$lib/EntityHistorySparkline.svelte";
  import { formatCount, formatMs } from "$lib/format";
  import type { ASNDetailPageData } from "./+page";

  let { data }: { data: ASNDetailPageData } = $props();

  const query = $derived(page.url.search);
</script>

<nav class="breadcrumbs" aria-label="Breadcrumb">
  <a href={`${base}/asns${query}`}>← ASNs</a>
</nav>

{#if !data.datasetTag}
  <section class="card">
    <h2>AS{data.asn}</h2>
    <p class="status-banner warn">No cohort resolved. Configure a public cohort to view ASN details.</p>
  </section>
{:else if data.error}
  <section class="card">
    <h2>AS{data.asn}</h2>
    <p class="status-banner error">Failed to load ASN: {data.error}</p>
  </section>
{:else if !data.detail}
  <section class="card">
    <h2>AS{data.asn}</h2>
    <p class="status-banner">ASN not materialized for this cohort.</p>
  </section>
{:else}
  {@const d = data.detail}
  <section class="card">
    <div class="detail-header">
      <h2>AS{d.asn}</h2>
      {#if d.label}
        <p class="hint">{d.label}</p>
      {/if}
    </div>

    <dl class="detail-counts">
      <div><dt>Domains</dt><dd>{formatCount(d.domain_count)}</dd></div>
      <div><dt>Addresses</dt><dd>{formatCount(d.address_count)}</dd></div>
      <div><dt>Nameservers</dt><dd>{formatCount(d.nameserver_count)}</dd></div>
      <div><dt>Prefixes</dt><dd>{formatCount(d.prefix_count)}</dd></div>
      {#if d.latency_p50_ms != null}
        <div><dt>Median latency</dt><dd title={`${d.latency_samples ?? 0} observations`}>{formatMs(d.latency_p50_ms)}</dd></div>
        {#if d.latency_p95_ms != null}
          <div><dt>p95 latency</dt><dd>{formatMs(d.latency_p95_ms)}</dd></div>
        {/if}
      {/if}
    </dl>
    <EntityHistorySparkline points={data.history} metric="domain_count" label="Domains over snapshots" />
  </section>

  <section class="card">
    <h3>Domains</h3>
    {#if d.domains.length === 0}
      <p class="hint">No domains use addresses in this ASN.</p>
    {:else}
      <ul class="chip-list" role="list">
        {#each d.domains as domain (domain)}
          <li><DomainChip {domain} /></li>
        {/each}
      </ul>
    {/if}
  </section>

  <section class="card">
    <h3>Nameservers</h3>
    {#if d.nameservers.length === 0}
      <p class="hint">No nameservers use addresses in this ASN.</p>
    {:else}
      <ul class="chip-list" role="list">
        {#each d.nameservers as ns (ns)}
          <li><NameserverChip nameserver={ns} /></li>
        {/each}
      </ul>
    {/if}
  </section>

  <section class="card">
    <h3>Prefixes</h3>
    {#if d.prefixes.length === 0}
      <p class="hint">No prefixes announced by this ASN in the cohort.</p>
    {:else}
      <ul class="chip-list" role="list">
        {#each d.prefixes as prefix (prefix)}
          <li><PrefixChip {prefix} /></li>
        {/each}
      </ul>
    {/if}
  </section>
{/if}

<style>
  .breadcrumbs { font-size: var(--text-sm); }
  .breadcrumbs a { color: var(--ink-2); text-decoration: none; }
  .breadcrumbs a:hover { color: var(--accent-2); }

  .detail-header h2 { margin: 0; font-family: var(--mono); }
  .detail-header .hint { margin: 4px 0 0; }

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
