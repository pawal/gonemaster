<script lang="ts">
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import ASNChip from "$lib/chips/ASNChip.svelte";
  import EndpointChip from "$lib/chips/EndpointChip.svelte";
  import NameserverChip from "$lib/chips/NameserverChip.svelte";
  import PrefixChip from "$lib/chips/PrefixChip.svelte";
  import TagChip from "$lib/chips/TagChip.svelte";
  import TestcaseChip from "$lib/chips/TestcaseChip.svelte";
  import { formatCount, formatTimestamp, gradeTone, levelTone } from "$lib/format";
  import type { DomainDetailPageData } from "./+page";

  let { data }: { data: DomainDetailPageData } = $props();

  const query = $derived(page.url.search);
</script>

<nav class="breadcrumbs" aria-label="Breadcrumb">
  <a href={`${base}/domains${query}`}>← Domains</a>
</nav>

{#if !data.datasetTag}
  <section class="card">
    <h2>{data.domain}</h2>
    <p class="status-banner warn">No cohort resolved. Configure a public cohort to view domain details.</p>
  </section>
{:else if data.error}
  <section class="card">
    <h2>{data.domain}</h2>
    <p class="status-banner error">Failed to load domain: {data.error}</p>
  </section>
{:else if !data.detail}
  <section class="card">
    <h2>{data.domain}</h2>
    <p class="status-banner">Domain not materialized for this cohort.</p>
  </section>
{:else}
  {@const d = data.detail}
  <section class="card">
    <div class="detail-header">
      <div>
        <h2>{d.domain}</h2>
        <p class="hint">
          Last run:
          {formatTimestamp(d.finished_at) || "—"}
        </p>
      </div>
      <div class="detail-scorecard">
        {#if d.score !== undefined && d.score !== null}
          <span class="score">{d.score}</span>
        {/if}
        {#if d.grade}
          <span class={`grade grade-${gradeTone(d.grade)}`}>{d.grade}</span>
        {/if}
        {#if d.worst_level}
          <span class={`level level-${levelTone(d.worst_level)}`}>{d.worst_level}</span>
        {/if}
      </div>
    </div>

    <dl class="detail-counts">
      <div><dt>Nameservers</dt><dd>{formatCount(d.nameserver_count)}</dd></div>
      <div><dt>Endpoints</dt><dd>{formatCount(d.endpoint_count)}</dd></div>
      <div><dt>ASNs</dt><dd>{formatCount(d.asn_count)}</dd></div>
      <div><dt>Prefixes</dt><dd>{formatCount(d.prefix_count)}</dd></div>
    </dl>
  </section>

  <section class="card">
    <h3>Nameservers</h3>
    {#if d.nameservers.length === 0}
      <p class="hint">No authoritative nameservers materialized.</p>
    {:else}
      <ul class="chip-list" role="list">
        {#each d.nameservers as ns (ns.nameserver)}
          <li>
            <NameserverChip nameserver={ns.nameserver} />
            <span class="hint">{ns.ipv4_count} IPv4 · {ns.ipv6_count} IPv6</span>
          </li>
        {/each}
      </ul>
    {/if}
  </section>

  <section class="card">
    <h3>Addresses</h3>
    {#if d.addresses.length === 0}
      <p class="hint">No addresses materialized.</p>
    {:else}
      <ul class="chip-list" role="list">
        {#each d.addresses as addr (addr.address)}
          <li>
            <EndpointChip address={addr.address} />
            <span class="hint">{addr.family}</span>
            {#if addr.asn !== undefined && addr.asn !== null}
              <ASNChip asn={addr.asn} />
            {/if}
            {#if addr.prefix}
              <PrefixChip prefix={addr.prefix} />
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </section>

  <section class="card">
    <h3>Tags</h3>
    {#if d.tags.length === 0}
      <p class="hint">No finding tags observed in the latest run.</p>
    {:else}
      <ul class="chip-list" role="list">
        {#each d.tags as tag (tag.tag)}
          <li>
            <TagChip tag={tag.tag} />
            {#if tag.level}
              <span class={`level level-${levelTone(tag.level)}`}>{tag.level}</span>
            {/if}
            {#if tag.module && tag.testcase}
              <TestcaseChip module={tag.module} testcase={tag.testcase} />
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </section>
{/if}

<style>
  .breadcrumbs {
    font-size: var(--text-sm);
  }
  .breadcrumbs a {
    color: var(--ink-2);
    text-decoration: none;
  }
  .breadcrumbs a:hover {
    color: var(--ink);
    text-decoration: underline;
  }

  .detail-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-4);
    flex-wrap: wrap;
  }
  .detail-header h2 {
    margin: 0;
    font-family: var(--mono);
  }
  .detail-scorecard {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
  }
  .score {
    font-size: var(--text-2xl);
    font-weight: 700;
    font-family: var(--mono);
  }
  .grade, .level {
    display: inline-block;
    padding: 2px 8px;
    border-radius: 6px;
    font-size: var(--text-xs);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .grade-aplus { background: #d1fae5; color: #065f46; }
  .grade-a { background: #dcfce7; color: #166534; }
  .grade-b { background: #fef9c3; color: #854d0e; }
  .grade-c { background: #fef3c7; color: #92400e; }
  .grade-d { background: #ffedd5; color: #9a3412; }
  .grade-f { background: #fee2e2; color: #991b1b; }
  .grade-neutral { background: var(--surface-2); color: var(--on-surface-2); }

  .level-critical { background: #fee2e2; color: #991b1b; }
  .level-error { background: #ffedd5; color: #9a3412; }
  .level-warning { background: #fef3c7; color: #92400e; }
  .level-notice { background: #e0f2fe; color: #075985; }
  .level-neutral { background: var(--surface-2); color: var(--on-surface-2); }

  .detail-counts {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-6);
    margin: 0;
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
    flex-direction: column;
    gap: 8px;
  }
  .chip-list li {
    display: inline-flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
  }
</style>
