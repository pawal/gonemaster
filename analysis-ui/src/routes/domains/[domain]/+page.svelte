<script lang="ts">
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import ASNChip from "$lib/chips/ASNChip.svelte";
  import EndpointChip from "$lib/chips/EndpointChip.svelte";
  import NameserverChip from "$lib/chips/NameserverChip.svelte";
  import PrefixChip from "$lib/chips/PrefixChip.svelte";
  import EntityHistorySparkline from "$lib/EntityHistorySparkline.svelte";
  import { tagHref } from "$lib/entityLinks";
  import { formatCount, formatTimestamp, gradeTone, levelTone } from "$lib/format";
  import { idnToUnicode } from "$lib/idn";
  import type { DomainDetailEntry, DomainDetailTag, NameserverTiming } from "$lib/api";
  import type { DomainDetailPageData } from "./+page";

  let { data }: { data: DomainDetailPageData } = $props();

  const query = $derived(page.url.search);
  const tagFloor = $derived(
    ((page.data as { effectiveSnapshotTagFloor?: string } | undefined)
      ?.effectiveSnapshotTagFloor ?? "") as string
  );

  function unicodeName(name: string): string | null {
    const decoded = idnToUnicode(name);
    return decoded !== name ? decoded : null;
  }

  // Response-time cells mirror the public result view: infinity for a
  // reachable-but-silent endpoint, dash for a name that never resolved.
  function timingStatus(t: NameserverTiming): "ok" | "unreachable" | "unresolved" {
    return t.status === "unreachable" || t.status === "unresolved" ? t.status : "ok";
  }
  function timingMs(t: NameserverTiming, value: number): string {
    const s = timingStatus(t);
    if (s === "unreachable") return "∞";
    if (s === "unresolved") return "-";
    return `${Math.round(value)}`;
  }
  function timingSamples(t: NameserverTiming): string {
    const s = timingStatus(t);
    if (s === "unreachable") return "0";
    if (s === "unresolved") return "-";
    return `${t.count}`;
  }

  // Unified row covering both detail sources: per-entry rows (run still
  // present) carry message+raw; tag-floor rows (run purged) don't.
  type Row = {
    level?: string;
    tag: string;
    module?: string;
    testcase?: string;
    message?: string;
    raw?: string;
  };

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
  function worstLevel(rows: Row[]): string {
    let worst = "";
    for (const r of rows) {
      if (severityRank(r.level) > severityRank(worst)) worst = r.level ?? "";
    }
    return worst;
  }
  function levelCounts(rows: Row[]) {
    const counts: Record<string, number> = {};
    for (const r of rows) {
      const l = (r.level ?? "").toUpperCase();
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
  function tagIsClickable(level: string | undefined): boolean {
    if (!tagFloor) return true;
    return severityRank(level) >= severityRank(tagFloor);
  }

  // Prefer the run's localized entries; fall back to the tag floor
  // when the run has been purged.
  const rows = $derived<Row[]>(
    (data.detail?.entries?.length ?? 0) > 0
      ? (data.detail?.entries as DomainDetailEntry[])
      : ((data.detail?.tags ?? []) as DomainDetailTag[])
  );

  const grouped = $derived.by(() => {
    const byModule: Record<string, Record<string, Row[]>> = {};
    for (const r of rows) {
      const mod = r.module || "Unspecified";
      const tc = r.testcase || "Unspecified";
      if (!byModule[mod]) byModule[mod] = {};
      if (!byModule[mod][tc]) byModule[mod][tc] = [];
      byModule[mod][tc].push(r);
    }
    return byModule;
  });

  const moduleNames = $derived.by(() =>
    Object.keys(grouped).sort((a, b) => {
      if (a === "System") return -1;
      if (b === "System") return 1;
      return 0;
    })
  );

  function allModuleRows(mod: Record<string, Row[]>): Row[] {
    return Object.values(mod).flat();
  }
</script>

<nav class="breadcrumbs" aria-label="Breadcrumb">
  <a href={`${base}/domains${query}`}>← Domains</a>
</nav>

{#if !data.datasetTag}
  <section class="card">
    <h2>{data.domain}{#if unicodeName(data.domain)} <span class="idn-unicode">({unicodeName(data.domain)})</span>{/if}</h2>
    <p class="status-banner warn">No cohort resolved. Configure a public cohort to view domain details.</p>
  </section>
{:else if data.error}
  <section class="card">
    <h2>{data.domain}{#if unicodeName(data.domain)} <span class="idn-unicode">({unicodeName(data.domain)})</span>{/if}</h2>
    <p class="status-banner error">Failed to load domain: {data.error}</p>
  </section>
{:else if !data.detail}
  <section class="card">
    <h2>{data.domain}{#if unicodeName(data.domain)} <span class="idn-unicode">({unicodeName(data.domain)})</span>{/if}</h2>
    <p class="status-banner">No results for this domain in the current cohort snapshot.</p>
    <p class="hint">
      A cohort snapshot is a point-in-time picture of analysis results. This page is
      empty when the snapshot does not include this domain - typically because the
      domain was not part of the cohort when the snapshot was captured, or because
      the snapshot has since been retired. Try a different snapshot from the
      selector, or pick a cohort this domain belongs to.
    </p>
  </section>
{:else}
  {@const d = data.detail}
  <section class="card">
    <div class="detail-header">
      <div>
        <h2>{d.domain}{#if unicodeName(d.domain)} <span class="idn-unicode">({unicodeName(d.domain)})</span>{/if}</h2>
        <p class="hint">
          Last analyzed:
          {formatTimestamp(d.finished_at) || "-"}
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
    <EntityHistorySparkline points={data.history} metric="score" label="Score over snapshots" />
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
                <tr
                  class={addrIdx === 0 ? "ns-group-start" : "ns-group-cont"}
                  class:ns-row-unreachable={addr.status === "unreachable"}
                >
                  <th scope="row" class="row-ident ns-cell">
                    {#if addrIdx === 0}
                      <NameserverChip nameserver={ns.nameserver} />
                    {/if}
                  </th>
                  <td class="row-ident">
                    <EndpointChip address={addr.address} nameserver={ns.nameserver} />
                    {#if addr.status === "unreachable"}
                      <span class="ns-status-badge">No response</span>
                    {/if}
                  </td>
                  <td class="row-ident">
                    {#if addr.asn !== undefined && addr.asn !== null}
                      <ASNChip asn={addr.asn} label={addr.asn_label ?? undefined} />
                    {:else}-{/if}
                  </td>
                  <td class="row-ident">
                    {#if addr.prefix}
                      <PrefixChip prefix={addr.prefix} />
                    {:else}-{/if}
                  </td>
                </tr>
              {:else}
                <tr class="ns-row-unresolved">
                  <th scope="row" class="row-ident ns-cell">
                    <NameserverChip nameserver={ns.nameserver} />
                  </th>
                  <td colspan="3">
                    <span class="ns-status-badge">Does not resolve</span>
                  </td>
                </tr>
              {/each}
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </section>

  {#if (d.nameserver_timings?.length ?? 0) > 0}
    <section class="card">
      <h3>Nameserver response times</h3>
      <p class="hint">Per-address query response time from the run that produced this snapshot.</p>
      <div class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th scope="col">Nameserver</th>
              <th scope="col">Address</th>
              <th scope="col" class="ts-num">Avg (ms)</th>
              <th scope="col" class="ts-num">Min (ms)</th>
              <th scope="col" class="ts-num">Max (ms)</th>
              <th scope="col" class="ts-num">Samples</th>
            </tr>
          </thead>
          <tbody>
            {#each d.nameserver_timings ?? [] as t (`${t.nameserver}|${t.address}`)}
              {@const st = timingStatus(t)}
              <tr class:ns-row-unreachable={st === "unreachable"} class:ns-row-unresolved={st === "unresolved"}>
                <th scope="row" class="row-ident">
                  <NameserverChip nameserver={t.nameserver} />
                </th>
                <td class="row-ident">
                  {#if t.address}
                    <EndpointChip address={t.address} nameserver={t.nameserver} />
                  {:else}-{/if}
                  {#if st === "unreachable"}
                    <span class="ns-status-badge">No response</span>
                  {:else if st === "unresolved"}
                    <span class="ns-status-badge">Does not resolve</span>
                  {/if}
                </td>
                <td class="ts-num">{timingMs(t, t.avg_ms)}</td>
                <td class="ts-num">{timingMs(t, t.min_ms)}</td>
                <td class="ts-num">{timingMs(t, t.max_ms)}</td>
                <td class="ts-num">{timingSamples(t)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </section>
  {/if}

  <section class="card results-card">
    <h3>Findings</h3>
    {#if rows.length === 0}
      <p class="hint">No findings observed in the latest run.</p>
    {:else}
      {#each moduleNames as moduleName (moduleName)}
        {@const mod = grouped[moduleName]}
        {@const modRows = allModuleRows(mod)}
        {@const modLevel = worstLevel(modRows)}
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
              {#each levelCounts(modRows) as { level, count } (level)}
                <span class="level level-{levelTone(level)}">{level} {count}</span>
              {/each}
            </span>
          </summary>
          <div class="module-entries">
            {#each testcases as tc (tc)}
              {@const tcRows = mod[tc]}
              {@const tcLevel = worstLevel(tcRows)}
              {#if tc !== "Unspecified"}
                <details class="testcase-group" open={isNoticeOrAbove(tcLevel)}>
                  <summary class="testcase-summary">
                    <span class="testcase-chevron" aria-hidden="true"></span>
                    <span class="testcase-name">{tc}</span>
                    <span class="testcase-badge">
                      <span class="level level-{levelTone(tcLevel)}">{tcLevel}</span>
                    </span>
                  </summary>
                  <ul class="result-rows" role="list">
                    {#each tcRows as r, i (i)}
                      <li class="result-row">
                        <span class="level level-{levelTone(r.level)}">{r.level}</span>
                        {#if tagIsClickable(r.level)}
                          <a class="entry-tag" href={tagHref(base, r.tag, query)} title={r.tag}>{r.tag}</a>
                        {:else}
                          <span class="entry-tag entry-tag-static" title={r.tag}>{r.tag}</span>
                        {/if}
                        {#if r.message && r.message !== r.raw}
                          <span class="result-message">{r.message}</span>
                        {/if}
                      </li>
                    {/each}
                  </ul>
                </details>
              {:else}
                <ul class="result-rows" role="list">
                  {#each tcRows as r, i (i)}
                    <li class="result-row">
                      <span class="level level-{levelTone(r.level)}">{r.level}</span>
                      <a class="entry-tag" href={tagHref(base, r.tag, query)} title={r.tag}>{r.tag}</a>
                      {#if r.message && r.message !== r.raw}
                        <span class="result-message">{r.message}</span>
                      {/if}
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
  .idn-unicode {
    font-family: inherit;
    color: var(--ink-2);
    font-weight: 400;
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
  .testcase-name {
    font-family: var(--mono);
    font-size: var(--text-sm);
    color: var(--ink);
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
    /* Without an explicit max-width the intrinsic (nowrap) width wins and
       the tag spills out of its grid column into the message cell. */
    max-width: 100%;
    display: inline-block;
  }
  .entry-tag:hover {
    background: var(--accent-2);
    color: var(--btn-fg);
  }
  .entry-tag-static {
    color: var(--ink-2);
    cursor: default;
  }
  .entry-tag-static:hover {
    background: var(--surface-2);
    color: var(--ink-2);
  }
  .result-message {
    font-size: var(--text-sm);
    color: var(--ink);
    line-height: 1.35;
    min-width: 0;
    overflow-wrap: anywhere;
  }
  .tc-empty {
    margin: 0;
    padding: 4px var(--space-3) var(--space-2);
    font-style: italic;
  }

  /* Long operator labels would otherwise force horizontal scroll. Let the
     ASN chip wrap; break anywhere so labels without spaces still fit. */
  .data-table :global(.entity-chip-asn) {
    white-space: normal;
    overflow-wrap: anywhere;
  }
  .data-table tbody tr.ns-group-cont .ns-cell { border-top: none; }
  .data-table tbody tr.ns-group-cont td { border-top: 1px dashed transparent; }
  .data-table tbody tr.ns-group-start:not(:first-child) th,
  .data-table tbody tr.ns-group-start:not(:first-child) td {
    border-top: 1px solid var(--border);
  }
  .data-table tbody tr.ns-group-cont th,
  .data-table tbody tr.ns-group-cont td { border-top: none; border-bottom: 1px dashed var(--border); }
  .data-table tbody tr.ns-group-cont:last-child td { border-bottom: none; }

  .ns-row-unreachable,
  .ns-row-unresolved {
    background: var(--sev-critical-bg);
  }
  .ns-row-unreachable td,
  .ns-row-unreachable th,
  .ns-row-unresolved td,
  .ns-row-unresolved th {
    color: var(--sev-critical-fg);
  }
  .ns-status-badge {
    display: inline-block;
    margin-left: 8px;
    padding: 1px 8px;
    border-radius: 999px;
    font-size: var(--text-xs);
    font-family: var(--sans);
    font-weight: 600;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    background: var(--sev-critical-bg);
    color: var(--sev-critical-fg);
  }

  .ts-num {
    text-align: right;
    font-family: var(--mono);
    white-space: nowrap;
  }
</style>
