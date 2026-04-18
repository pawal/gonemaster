<script lang="ts">
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import EndpointChip from "$lib/chips/EndpointChip.svelte";
  import NameserverChip from "$lib/chips/NameserverChip.svelte";
  import TagChip from "$lib/chips/TagChip.svelte";
  import TestcaseChip from "$lib/chips/TestcaseChip.svelte";
  import { asnHref, prefixHref } from "$lib/entityLinks";
  import { formatCount, formatTimestamp, gradeTone, levelTone } from "$lib/format";
  import type { DomainDetailPageData } from "./+page";

  let { data }: { data: DomainDetailPageData } = $props();

  const query = $derived(page.url.search);

  // Severity ordering so the most critical tags surface at the top.
  const SEVERITY_RANK: Record<string, number> = {
    CRITICAL: 5,
    ERROR: 4,
    WARNING: 3,
    NOTICE: 2,
    INFO: 1
  };
  function severityRank(level: string | undefined): number {
    if (!level) return 0;
    return SEVERITY_RANK[level.toUpperCase()] ?? 0;
  }

  const sortedTags = $derived.by(() => {
    if (!data.detail) return [];
    return [...data.detail.tags].sort((a, b) => {
      const rd = severityRank(b.level) - severityRank(a.level);
      if (rd !== 0) return rd;
      return a.tag.localeCompare(b.tag);
    });
  });
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
          Last analyzed:
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
            {#if ns.ipv4_count > 0}<span class="family-badge family-ipv4">{ns.ipv4_count} v4</span>{/if}
            {#if ns.ipv6_count > 0}<span class="family-badge family-ipv6">{ns.ipv6_count} v6</span>{/if}
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
      <div class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th scope="col">Address</th>
              <th scope="col">Operator</th>
              <th scope="col">Prefix</th>
            </tr>
          </thead>
          <tbody>
            {#each d.addresses as addr (addr.address)}
              <tr>
                <th scope="row" class="row-ident">
                  <EndpointChip address={addr.address} />
                  <span class={`family-badge family-${addr.family}`}>{addr.family === "ipv6" ? "v6" : "v4"}</span>
                </th>
                <td class="row-ident">
                  {#if addr.asn !== undefined && addr.asn !== null}
                    <a class="cell-link" href={asnHref(base, addr.asn, query)} title={addr.asn_label ? `AS${addr.asn} · ${addr.asn_label}` : `AS${addr.asn}`}>
                      {#if addr.asn_label}
                        <span class="operator-label">{addr.asn_label}</span>
                        <span class="operator-asn">AS{addr.asn}</span>
                      {:else}
                        AS{addr.asn}
                      {/if}
                    </a>
                  {:else}—{/if}
                </td>
                <td class="row-ident">
                  {#if addr.prefix}
                    <a class="cell-link" href={prefixHref(base, addr.prefix, query)}>{addr.prefix}</a>
                  {:else}—{/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </section>

  <section class="card">
    <h3>Tags</h3>
    {#if sortedTags.length === 0}
      <p class="hint">No finding tags observed in the latest run.</p>
    {:else}
      <ul class="chip-list" role="list">
        {#each sortedTags as tag (tag.tag)}
          <li>
            {#if tag.level}
              <span class={`level level-${levelTone(tag.level)}`}>{tag.level}</span>
            {/if}
            <TagChip tag={tag.tag} />
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

  .family-badge {
    display: inline-block;
    padding: 1px 6px;
    border-radius: 4px;
    font-size: var(--text-xs);
    font-weight: 600;
    font-family: var(--sans);
    letter-spacing: 0.04em;
  }
  .family-ipv4 { background: #e0f2fe; color: #075985; }
  .family-ipv6 { background: #ede9fe; color: #5b21b6; }

  .table-wrap {
    overflow-x: auto;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
  }
  .data-table {
    width: 100%;
    border-collapse: collapse;
    font-size: var(--text-sm);
  }
  .data-table th {
    text-align: left;
    padding: 8px 10px;
    background: var(--surface-2);
    color: var(--ink-2);
    font-size: var(--text-xs);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    border-bottom: 1px solid var(--border);
    white-space: nowrap;
  }
  .data-table td {
    padding: 8px 10px;
    border-bottom: 1px solid var(--border);
    vertical-align: middle;
  }
  .data-table tbody tr:last-child td { border-bottom: none; }
  .row-ident {
    font-family: var(--mono);
    font-weight: 500;
    color: var(--ink);
    text-transform: none;
    letter-spacing: normal;
    font-size: var(--text-sm);
  }
  .cell-link { color: inherit; text-decoration: none; }
  .cell-link:hover { text-decoration: underline; }
  .operator-label { font-family: var(--sans); font-weight: 500; }
  .operator-asn { margin-left: 6px; color: var(--ink-2); font-size: var(--text-xs); }
</style>
