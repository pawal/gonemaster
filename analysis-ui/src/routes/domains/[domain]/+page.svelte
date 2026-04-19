<script lang="ts">
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import EndpointChip from "$lib/chips/EndpointChip.svelte";
  import NameserverChip from "$lib/chips/NameserverChip.svelte";
  import { asnHref, prefixHref, tagHref, testcaseHref } from "$lib/entityLinks";
  import { formatCount, formatTimestamp, gradeTone, levelTone } from "$lib/format";
  import type { DomainDetailEntry } from "$lib/api";
  import type { DomainDetailPageData } from "./+page";

  let { data }: { data: DomainDetailPageData } = $props();

  const query = $derived(page.url.search);

  // Severity ordering, matching the server-side severityRank. Used for sort
  // and for the "is this worth auto-expanding" threshold.
  const SEVERITY_RANK: Record<string, number> = {
    CRITICAL: 5,
    ERROR: 4,
    WARNING: 3,
    NOTICE: 2,
    INFO: 1
  };
  function severityRank(level: string | null | undefined): number {
    if (!level) return 0;
    return SEVERITY_RANK[level.toUpperCase()] ?? 0;
  }
  function worstLevel(entries: DomainDetailEntry[]): string {
    let worst = "";
    for (const e of entries) {
      if (severityRank(e.level) > severityRank(worst)) worst = e.level ?? "";
    }
    return worst;
  }
  function levelCounts(entries: DomainDetailEntry[]) {
    const counts: Record<string, number> = {};
    for (const e of entries) {
      const l = (e.level ?? "").toUpperCase();
      if (!l) continue;
      counts[l] = (counts[l] ?? 0) + 1;
    }
    return Object.entries(counts)
      .sort((a, b) => severityRank(b[0]) - severityRank(a[0]))
      .map(([level, count]) => ({ level, count }));
  }
  function isNoticeOrAbove(level: string): boolean {
    return severityRank(level) >= severityRank("NOTICE");
  }
  // Mirror ui-public/Results.svelte: drop entries where translation
  // produced nothing useful (message is empty, or fell back to the raw
  // "MODULE:TESTCASE:TAG" string). Those rows add noise without conveying
  // anything a user can act on.
  function hasDistinctMessage(e: DomainDetailEntry): boolean {
    return !!e.message && e.message !== e.raw;
  }

  // Group entries by module → testcase, preserving backend insertion order
  // (timestamp-sorted from the Zonemaster run).
  const grouped = $derived.by(() => {
    const byModule: Record<string, Record<string, DomainDetailEntry[]>> = {};
    const entries = data.detail?.entries ?? [];
    for (const e of entries) {
      const mod = e.module || "Unspecified";
      const tc = e.testcase || "Unspecified";
      if (!byModule[mod]) byModule[mod] = {};
      if (!byModule[mod][tc]) byModule[mod][tc] = [];
      byModule[mod][tc].push(e);
    }
    return byModule;
  });

  // Sort module names with System first (matches ui-public convention);
  // remaining modules follow in insertion order.
  const moduleNames = $derived.by(() =>
    Object.keys(grouped).sort((a, b) => {
      if (a === "System") return -1;
      if (b === "System") return 1;
      return 0;
    })
  );

  function allModuleEntries(mod: Record<string, DomainDetailEntry[]>): DomainDetailEntry[] {
    return Object.values(mod).flat();
  }

  // Stop summary click-to-toggle when the user intends to navigate via an
  // anchor inside the summary. Without this, clicking the testcase link
  // both navigates and flips the disclosure state.
  function stopToggle(event: MouseEvent) {
    event.stopPropagation();
  }
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
    <h3>Authoritative servers</h3>
    {#if d.nameservers.length === 0}
      <p class="hint">No authoritative nameservers materialized.</p>
    {:else}
      <div class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th scope="col">Nameserver</th>
              <th scope="col">Address</th>
              <th scope="col">Operator</th>
              <th scope="col">Prefix</th>
            </tr>
          </thead>
          <tbody>
            {#each d.nameservers as ns (ns.nameserver)}
              {#each ns.addresses as addr, addrIdx (`${ns.nameserver}|${addr.address}`)}
                <tr class={addrIdx === 0 ? "ns-group-start" : "ns-group-cont"}>
                  <th scope="row" class="row-ident ns-cell">
                    {#if addrIdx === 0}
                      <NameserverChip nameserver={ns.nameserver} />
                    {/if}
                  </th>
                  <td class="row-ident">
                    <EndpointChip address={addr.address} nameserver={ns.nameserver} />
                    <span class={`family-badge family-${addr.family}`}>{addr.family === "ipv6" ? "v6" : "v4"}</span>
                  </td>
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
              {:else}
                <tr>
                  <th scope="row" class="row-ident ns-cell">
                    <NameserverChip nameserver={ns.nameserver} />
                  </th>
                  <td colspan="3" class="hint">No materialized addresses.</td>
                </tr>
              {/each}
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </section>

  <section class="card results-card">
    <h3>Findings</h3>
    {#if (d.entries?.length ?? 0) === 0}
      <p class="hint">No findings observed in the latest run.</p>
    {:else}
      {#each moduleNames as moduleName (moduleName)}
        {@const mod = grouped[moduleName]}
        {@const modEntries = allModuleEntries(mod)}
        {@const modLevel = worstLevel(modEntries)}
        {@const testcases = Object.keys(mod)}
        <details
          class="module-card"
          data-level={modLevel.toLowerCase()}
          open={isNoticeOrAbove(modLevel)}
        >
          <summary class="module-summary">
            <span class="module-chevron" aria-hidden="true"></span>
            <span class="module-name">{moduleName}</span>
            <span class="module-badges">
              {#each levelCounts(modEntries) as { level, count } (level)}
                <span class="level level-{levelTone(level)}">{level} {count}</span>
              {/each}
            </span>
          </summary>
          <div class="module-entries">
            {#each testcases as tc (tc)}
              {@const tcEntries = mod[tc]}
              {@const tcLevel = worstLevel(tcEntries)}
              {@const tcRows = tcEntries.filter(hasDistinctMessage)}
              {#if tc !== "Unspecified"}
                <details class="testcase-group" open={isNoticeOrAbove(tcLevel)}>
                  <summary class="testcase-summary">
                    <span class="testcase-chevron" aria-hidden="true"></span>
                    <a
                      class="testcase-link"
                      href={testcaseHref(base, tc, moduleName, query)}
                      onclick={stopToggle}
                    >{tc}</a>
                    <span class="testcase-badge">
                      <span class="level level-{levelTone(tcLevel)}">{tcLevel}</span>
                    </span>
                  </summary>
                  {#if tcRows.length === 0}
                    <p class="hint tc-empty">No translatable messages.</p>
                  {:else}
                    <ul class="result-rows" role="list">
                      {#each tcRows as entry, i (i)}
                        <li class="result-row">
                          <span class="level level-{levelTone(entry.level)}">{entry.level}</span>
                          <a class="entry-tag" href={tagHref(base, entry.tag, query)}>{entry.tag}</a>
                          <span class="result-message">{entry.message}</span>
                        </li>
                      {/each}
                    </ul>
                  {/if}
                </details>
              {:else}
                <ul class="result-rows" role="list">
                  {#each tcRows as entry, i (i)}
                    <li class="result-row">
                      <span class="level level-{levelTone(entry.level)}">{entry.level}</span>
                      <a class="entry-tag" href={tagHref(base, entry.tag, query)}>{entry.tag}</a>
                      <span class="result-message">{entry.message}</span>
                    </li>
                  {/each}
                </ul>
              {/if}
            {/each}
          </div>
        </details>
      {/each}
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
    color: var(--accent-2);
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

  .results-card {
    gap: var(--space-3);
  }

  .module-card {
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
    overflow: hidden;
  }
  .module-card[open] {
    background: var(--card);
  }
  .module-summary {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: 10px var(--space-4);
    cursor: pointer;
    list-style: none;
  }
  .module-summary::-webkit-details-marker { display: none; }
  .module-chevron {
    width: 0;
    height: 0;
    border-left: 5px solid transparent;
    border-right: 5px solid transparent;
    border-top: 6px solid var(--ink-2);
    transform: rotate(-90deg);
    transition: transform 0.15s ease;
    flex-shrink: 0;
  }
  .module-card[open] > .module-summary .module-chevron {
    transform: rotate(0deg);
  }
  .module-name {
    font-weight: 600;
    font-size: var(--text-base);
  }
  .module-badges {
    display: inline-flex;
    gap: 6px;
    margin-left: auto;
    flex-wrap: wrap;
  }

  .module-entries {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-4) var(--space-3);
    border-top: 1px solid var(--border);
  }

  .testcase-group {
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg);
  }
  .testcase-summary {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: 6px var(--space-3);
    cursor: pointer;
    list-style: none;
  }
  .testcase-summary::-webkit-details-marker { display: none; }
  .testcase-chevron {
    width: 0;
    height: 0;
    border-left: 4px solid transparent;
    border-right: 4px solid transparent;
    border-top: 5px solid var(--ink-2);
    transform: rotate(-90deg);
    transition: transform 0.15s ease;
    flex-shrink: 0;
  }
  .testcase-group[open] > .testcase-summary .testcase-chevron {
    transform: rotate(0deg);
  }
  .testcase-link {
    font-family: var(--mono);
    font-size: var(--text-sm);
    color: var(--ink);
    text-decoration: none;
  }
  .testcase-link:hover {
    color: var(--accent-2);
  }
  .testcase-badge {
    margin-left: auto;
  }

  /* Single grid at the list level so all rows in a testcase share the
     same three columns; otherwise each row sizes independently and the
     message column starts at different x positions, which also lets long
     messages push the tag column off the right edge. */
  .result-rows {
    list-style: none;
    margin: 0;
    padding: 4px var(--space-3) var(--space-2);
    display: grid;
    grid-template-columns:
      minmax(4.5rem, auto)
      minmax(10rem, 14rem)
      1fr;
    column-gap: var(--space-2);
    row-gap: 6px;
  }
  .result-row {
    display: grid;
    grid-template-columns: subgrid;
    grid-column: 1 / -1;
    align-items: baseline;
  }
  .entry-tag {
    font-family: var(--mono);
    font-size: var(--text-xs);
    padding: 1px 6px;
    background: var(--surface-2);
    color: var(--ink);
    border-radius: 4px;
    text-decoration: none;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    justify-self: start;
  }
  .entry-tag:hover {
    background: var(--accent-2);
    color: var(--btn-fg);
  }
  .result-message {
    font-size: var(--text-sm);
    color: var(--ink);
    line-height: 1.35;
  }
  .tc-empty {
    margin: 0;
    padding: 4px var(--space-3) var(--space-2);
    font-style: italic;
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
  .data-table tbody tr.ns-group-cont .ns-cell { border-top: none; }
  .data-table tbody tr.ns-group-cont td { border-top: 1px dashed transparent; }
  .data-table tbody tr.ns-group-start:not(:first-child) th,
  .data-table tbody tr.ns-group-start:not(:first-child) td {
    border-top: 1px solid var(--border);
  }
  .data-table tbody tr.ns-group-cont th,
  .data-table tbody tr.ns-group-cont td { border-top: none; border-bottom: 1px dashed var(--border); }
  .data-table tbody tr.ns-group-cont:last-child td { border-bottom: none; }
  .row-ident {
    font-family: var(--mono);
    font-weight: 500;
    color: var(--ink);
    text-transform: none;
    letter-spacing: normal;
    font-size: var(--text-sm);
  }
  .cell-link { color: inherit; text-decoration: none; }
  .cell-link:hover { color: var(--accent-2); }
  .operator-label { font-family: var(--sans); font-weight: 500; }
  .operator-asn { margin-left: 6px; color: var(--ink-2); font-size: var(--text-xs); }
</style>
