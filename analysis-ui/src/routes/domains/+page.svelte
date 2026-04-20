<script lang="ts">
  import { goto } from "$app/navigation";
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import FilterBar from "$lib/FilterBar.svelte";
  import SortHeader from "$lib/SortHeader.svelte";
  import { asnHref, domainHref } from "$lib/entityLinks";
  import { formatCount, formatTimestamp, gradeTone, levelTone } from "$lib/format";
  import { searchToString } from "$lib/filters";
  import { downloadCSV, downloadJSON, type ExportColumn } from "$lib/exporters";
  import type { DomainView } from "$lib/api";
  import type { LayoutData } from "../+layout";
  import type { DomainsPageData } from "./+page";

  let { data }: { data: DomainsPageData } = $props();

  const layoutData = $derived(page.data as LayoutData);

  const sortSpecs = {
    domain: { asc: "domain_asc", desc: "domain_desc" },
    score: { asc: "score_asc", desc: "score_desc" },
    worst: { desc: "worst_level_desc" }
  } as const;

  const currentSort = $derived(page.url.searchParams.get("sort") ?? "");
  const currentLimit = $derived(data.limit);
  const currentOffset = $derived(data.offset);
  const total = $derived(data.list?.total ?? 0);
  const pageStart = $derived(total === 0 ? 0 : currentOffset + 1);
  const pageEnd = $derived(Math.min(currentOffset + (data.list?.items.length ?? 0), total));

  function updateParam(key: string, value: string) {
    const params = new URLSearchParams(page.url.searchParams);
    if (value) params.set(key, value);
    else params.delete(key);
    goto(`${page.url.pathname}${searchToString(params)}`, {
      replaceState: false,
      noScroll: false,
      keepFocus: true
    });
  }

  function gotoPage(nextOffset: number) {
    const params = new URLSearchParams(page.url.searchParams);
    if (nextOffset > 0) params.set("offset", String(nextOffset));
    else params.delete("offset");
    goto(`${page.url.pathname}${searchToString(params)}`);
  }

  const hasPrev = $derived(currentOffset > 0);
  const hasNext = $derived(currentOffset + currentLimit < total);

  const rows = $derived((data.list?.items ?? []) as DomainView[]);

  const exportColumns: ExportColumn<DomainView>[] = [
    { key: "domain", label: "Domain", value: (r) => r.domain },
    { key: "score", label: "Score", value: (r) => r.score ?? "" },
    { key: "grade", label: "Grade", value: (r) => r.grade ?? "" },
    { key: "worst_level", label: "Worst level", value: (r) => r.worst_level ?? "" },
    { key: "operator", label: "Operator", value: (r) => r.operator ?? "" },
    { key: "operator_asn", label: "Operator ASN", value: (r) => r.operator_asn ?? "" },
    { key: "nameserver_count", label: "Nameservers", value: (r) => r.nameserver_count },
    { key: "endpoint_count", label: "Endpoints", value: (r) => r.endpoint_count },
    { key: "asn_count", label: "ASNs", value: (r) => r.asn_count },
    { key: "prefix_count", label: "Prefixes", value: (r) => r.prefix_count },
    { key: "finished_at", label: "Last run", value: (r) => r.finished_at ?? "" }
  ];

  function filenamePrefix(): string {
    const tag = data.datasetTag ?? "cohort";
    return `${tag}-domains`;
  }

  function exportCSV() {
    if (!data.list) return;
    downloadCSV(`${filenamePrefix()}.csv`, data.list.items, exportColumns);
  }

  function exportJSONFile() {
    if (!data.list) return;
    downloadJSON(`${filenamePrefix()}.json`, data.list.items, exportColumns);
  }

  const search = $derived(page.url.search);

  function rowClick(event: MouseEvent, href: string) {
    const target = event.target as HTMLElement | null;
    if (target?.closest("a")) return;
    goto(href);
  }
</script>

<FilterBar
  cohorts={layoutData.catalog?.cohorts ?? []}
  selectorEnabled={layoutData.catalog?.selector_enabled ?? false}
/>

<section class="card">
  <div class="list-head">
    <h2>Domains</h2>
    <div class="list-toolbar">
      <label class="inline-field">
        <span>Page size</span>
        <select value={String(currentLimit)} onchange={(e) => updateParam("limit", e.currentTarget.value)}>
          {#each [25, 50, 100, 250, 500] as n}
            <option value={String(n)}>{n}</option>
          {/each}
        </select>
      </label>
      <div class="export-group">
        <button type="button" class="ghost" onclick={exportCSV} disabled={!data.list?.items.length}>CSV</button>
        <button type="button" class="ghost" onclick={exportJSONFile} disabled={!data.list?.items.length}>JSON</button>
      </div>
    </div>
  </div>

  {#if !data.datasetTag}
    <p class="status-banner warn">No cohort resolved. Configure a public cohort to see domains.</p>
  {:else if data.error}
    <p class="status-banner error">Failed to load domains: {data.error}</p>
  {:else if !data.list || data.list.items.length === 0}
    <p class="status-banner">No domains materialized for this cohort yet.</p>
  {:else}
    <div class="table-wrap">
      <table class="data-table">
        <thead>
          <tr>
            <th scope="col">
              <SortHeader label="Domain" spec={sortSpecs.domain} {currentSort} onsort={(v: string) => updateParam("sort", v)} />
            </th>
            <th scope="col" class="col-num">
              <SortHeader label="Score" spec={sortSpecs.score} align="right" {currentSort} onsort={(v: string) => updateParam("sort", v)} />
            </th>
            <th scope="col">Grade</th>
            <th scope="col">
              <SortHeader label="Worst" spec={sortSpecs.worst} {currentSort} onsort={(v: string) => updateParam("sort", v)} />
            </th>
            <th scope="col">Operator</th>
            <th scope="col" class="col-num">Nameservers</th>
            <th scope="col" class="col-num">Endpoints</th>
            <th scope="col" class="col-num">ASNs</th>
            <th scope="col" class="col-num">Prefixes</th>
            <th scope="col">Last analyzed</th>
          </tr>
        </thead>
        <tbody>
          {#each rows as row (row.domain)}
            <tr class="row-clickable" onclick={(e: MouseEvent) => rowClick(e, domainHref(base, row.domain, search))}>
              <th scope="row" class="row-ident">
                <a class="cell-link" href={domainHref(base, row.domain, search)}>{row.domain}</a>
              </th>
              <td class="col-num">{row.score ?? "—"}</td>
              <td>
                {#if row.grade}
                  <span class={`grade grade-${gradeTone(row.grade)}`}>{row.grade}</span>
                {:else}—{/if}
              </td>
              <td>
                {#if row.worst_level}
                  <span class={`level level-${levelTone(row.worst_level)}`}>{row.worst_level}</span>
                {:else}—{/if}
              </td>
              <td class="row-ident">
                {#if row.operator === "Multiple"}
                  <span class="operator-multi">Multiple ({row.asn_count})</span>
                {:else if row.operator_asn !== undefined && row.operator_asn !== null}
                  <a class="cell-link" href={asnHref(base, row.operator_asn, search)} title={row.operator ? `AS${row.operator_asn} · ${row.operator}` : `AS${row.operator_asn}`}>
                    {#if row.operator}
                      <span class="operator-label">{row.operator}</span>
                      <span class="operator-asn">AS{row.operator_asn}</span>
                    {:else}
                      AS{row.operator_asn}
                    {/if}
                  </a>
                {:else}—{/if}
              </td>
              <td class="col-num">{formatCount(row.nameserver_count)}</td>
              <td class="col-num">{formatCount(row.endpoint_count)}</td>
              <td class="col-num">{formatCount(row.asn_count)}</td>
              <td class="col-num">{formatCount(row.prefix_count)}</td>
              <td>{formatTimestamp(row.finished_at) || "—"}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    <div class="pagination">
      <span class="hint">
        Showing {pageStart}–{pageEnd} of {formatCount(total)}
      </span>
      <div class="pagination-controls">
        <button type="button" class="ghost" disabled={!hasPrev} onclick={() => gotoPage(Math.max(0, currentOffset - currentLimit))}>
          ← Previous
        </button>
        <button type="button" class="ghost" disabled={!hasNext} onclick={() => gotoPage(currentOffset + currentLimit)}>
          Next →
        </button>
      </div>
    </div>
  {/if}
</section>

<style>
  .list-head {
    display: flex;
    justify-content: space-between;
    align-items: flex-end;
    gap: var(--space-3);
    flex-wrap: wrap;
  }
  .list-head h2 { margin: 0; }

  .list-toolbar {
    display: flex;
    gap: var(--space-3);
    flex-wrap: wrap;
    align-items: flex-end;
  }

  .export-group { display: flex; gap: var(--space-2); }

  .inline-field {
    display: flex;
    flex-direction: column;
    gap: 2px;
    font-size: var(--text-xs);
    color: var(--ink-2);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .inline-field span { font-weight: 600; }
  .inline-field select {
    padding: 6px 10px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--surface);
    color: var(--ink);
    font: inherit;
    font-size: var(--text-sm);
    text-transform: none;
    letter-spacing: normal;
  }

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
  .data-table tbody tr:hover { background: rgba(3, 105, 161, 0.04); }
  .row-clickable { cursor: pointer; }
  .operator-label { font-family: var(--sans); font-weight: 500; }
  .operator-asn { margin-left: 6px; color: var(--ink-2); font-size: var(--text-xs); }
  .operator-multi { color: var(--ink-2); font-family: var(--sans); font-style: italic; }
  .cell-link { color: inherit; text-decoration: none; }
  .cell-link:hover { color: var(--accent-2); }
  .cell-link:focus-visible {
    outline: 2px solid var(--accent-2);
    outline-offset: 2px;
    border-radius: 2px;
  }
  .row-ident {
    font-family: var(--mono);
    font-weight: 500;
    color: var(--ink);
    text-transform: none;
    letter-spacing: normal;
    font-size: var(--text-sm);
  }
  .col-num { text-align: right; font-variant-numeric: tabular-nums; }

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

  .pagination {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: var(--space-3);
    flex-wrap: wrap;
    margin-top: var(--space-3);
  }
  .pagination-controls { display: flex; gap: var(--space-2); }
</style>
