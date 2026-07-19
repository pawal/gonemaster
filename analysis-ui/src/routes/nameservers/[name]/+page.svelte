<script lang="ts">
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import ASNChip from "$lib/chips/ASNChip.svelte";
  import DomainChip from "$lib/chips/DomainChip.svelte";
  import EndpointChip from "$lib/chips/EndpointChip.svelte";
  import EntityHistorySparkline from "$lib/EntityHistorySparkline.svelte";
  import { formatCount, formatMs } from "$lib/format";
  import { idnToUnicode } from "$lib/idn";
  import type { NameserverDetailPageData } from "./+page";

  let { data }: { data: NameserverDetailPageData } = $props();

  const query = $derived(page.url.search);

  function unicodeName(name: string): string | null {
    const decoded = idnToUnicode(name);
    return decoded !== name ? decoded : null;
  }
</script>

<nav class="breadcrumbs" aria-label="Breadcrumb">
  <a href={`${base}/nameservers${query}`}>← Nameservers</a>
</nav>

{#if !data.datasetTag}
  <section class="card">
    <h2>{data.nameserver}{#if unicodeName(data.nameserver)} <span class="idn-unicode">({unicodeName(data.nameserver)})</span>{/if}</h2>
    <p class="status-banner warn">No cohort resolved. Configure a public cohort to view nameserver details.</p>
  </section>
{:else if data.error}
  <section class="card">
    <h2>{data.nameserver}{#if unicodeName(data.nameserver)} <span class="idn-unicode">({unicodeName(data.nameserver)})</span>{/if}</h2>
    <p class="status-banner error">Failed to load nameserver: {data.error}</p>
  </section>
{:else if !data.detail}
  <section class="card">
    <h2>{data.nameserver}{#if unicodeName(data.nameserver)} <span class="idn-unicode">({unicodeName(data.nameserver)})</span>{/if}</h2>
    <p class="status-banner">Nameserver not materialized for this cohort.</p>
  </section>
{:else}
  {@const d = data.detail}
  <section class="card">
    <div class="detail-header">
      <h2>{d.nameserver}{#if unicodeName(d.nameserver)} <span class="idn-unicode">({unicodeName(d.nameserver)})</span>{/if}</h2>
    </div>

    <dl class="detail-counts">
      <div><dt>Domains</dt><dd>{formatCount(d.domain_count)}</dd></div>
      <div><dt>Endpoints</dt><dd>{formatCount(d.endpoint_count)}</dd></div>
      <div><dt>IPv4</dt><dd>{formatCount(d.ipv4_count)}</dd></div>
      <div><dt>IPv6</dt><dd>{formatCount(d.ipv6_count)}</dd></div>
      {#if d.latency_p50_ms != null}
        <div><dt>Median latency</dt><dd title={`${d.latency_samples ?? 0} observations`}>{formatMs(d.latency_p50_ms)}</dd></div>
        {#if d.latency_p95_ms != null}
          <div><dt>p95 latency</dt><dd>{formatMs(d.latency_p95_ms)}</dd></div>
        {/if}
      {/if}
    </dl>
    <div class="history-sparks">
      <EntityHistorySparkline points={data.history} metric="domain_count" label="Domains over snapshots" />
      <EntityHistorySparkline points={data.history} metric="latency_p50_ms" label="Median latency over snapshots" />
    </div>
  </section>

  <section class="card">
    <h3>Addresses</h3>
    {#if d.addresses.length === 0}
      <p class="hint">No addresses materialized.</p>
    {:else}
      <ul class="chip-list" role="list">
        {#each d.addresses as address (address)}
          <li><EndpointChip {address} nameserver={d.nameserver} /></li>
        {/each}
      </ul>
    {/if}
  </section>

  <section class="card">
    <h3>Domains served</h3>
    {#if d.domains.length === 0}
      <p class="hint">No domains in this cohort use this nameserver.</p>
    {:else}
      <ul class="chip-list" role="list">
        {#each d.domains as domain (domain)}
          <li><DomainChip {domain} /></li>
        {/each}
      </ul>
    {/if}
  </section>

  <section class="card">
    <h3>ASNs</h3>
    {#if d.asns.length === 0}
      <p class="hint">No ASN data available for this nameserver.</p>
    {:else}
      <ul class="chip-list" role="list">
        {#each d.asns as asn (asn)}
          <li><ASNChip {asn} /></li>
        {/each}
      </ul>
    {/if}
  </section>
{/if}

<style>
  .breadcrumbs { font-size: var(--text-sm); }
  .breadcrumbs a { color: var(--ink-2); text-decoration: none; }
  .breadcrumbs a:hover { color: var(--accent-2); }

  .detail-header h2 {
    margin: 0;
    font-family: var(--mono);
  }
  .idn-unicode {
    font-family: inherit;
    color: var(--ink-2);
    font-weight: 400;
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

  .history-sparks {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
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
