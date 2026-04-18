<script lang="ts">
  import { goto } from "$app/navigation";
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import FilterBar from "$lib/FilterBar.svelte";
  import SortHeader from "$lib/SortHeader.svelte";
  import { asnHref, prefixHref } from "$lib/entityLinks";
  import { formatCount } from "$lib/format";
  import { searchToString } from "$lib/filters";
  import { downloadCSV, downloadJSON, type ExportColumn } from "$lib/exporters";
  import type { PrefixView } from "$lib/api";
  import type { LayoutData } from "../+layout";
  import type { PrefixesPageData } from "./+page";

  let { data }: { data: PrefixesPageData } = $props();

  const layoutData = $derived(page.data as LayoutData);

  const sortSpecs = {
    domainCount: { asc: "domain_count_asc", desc: "domain_count_desc" }
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

  const rows = $derived((data.list?.items ?? []) as PrefixView[]);
  const search = $derived(page.url.search);

  function rowClick(event: MouseEvent, href: string) {
    const target = event.target as HTMLElement | null;
    if (target?.closest("a")) return;
    goto(href);
  }

  const exportColumns: ExportColumn<PrefixView>[] = [
    { key: "prefix", label: "Prefix", value: (r) => r.prefix },
    { key: "family", label: "Family", value: (r) => r.family },
    { key: "domain_count", label: "Domains", value: (r) => r.domain_count },
    { key: "address_count", label: "Addresses", value: (r) => r.address_count },
    { key: "asn", label: "ASN", value: (r) => r.asn ?? "" }
  ];

  function filenamePrefix(): string {
    const tag = data.datasetTag ?? "cohort";
    return `${tag}-prefixes`;
  }

  function exportCSV() {
    if (!data.list) return;
    downloadCSV(`${filenamePrefix()}.csv`, data.list.items, exportColumns);
  }

  function exportJSONFile() {
    if (!data.list) return;
    downloadJSON(`${filenamePrefix()}.json`, data.list.items, exportColumns);
  }

</script>

<FilterBar
  cohorts={layoutData.catalog?.cohorts ?? []}
  selectorEnabled={layoutData.catalog?.selector_enabled ?? false}
/>

<section class="card">
  <div class="list-head">
    <h2>Prefixes</h2>
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
    <p class="status-banner warn">No cohort resolved. Configure a public cohort to see prefixes.</p>
  {:else if data.error}
    <p class="status-banner error">Failed to load prefixes: {data.error}</p>
  {:else if !data.list || data.list.items.length === 0}
    <p class="status-banner">No prefixes materialized for this cohort yet.</p>
  {:else}
    <div class="table-wrap">
      <table class="data-table">
        <thead>
          <tr>
            <th scope="col">Prefix</th>
            <th scope="col" class="col-num">
              <SortHeader label="Domains" spec={sortSpecs.domainCount} align="right" {currentSort} onsort={(v: string) => updateParam("sort", v)} />
            </th>
            <th scope="col" class="col-num">Addresses</th>
            <th scope="col">Operator</th>
          </tr>
        </thead>
        <tbody>
          {#each rows as row (row.prefix)}
            <tr class="row-clickable" onclick={(e: MouseEvent) => rowClick(e, prefixHref(base, row.prefix, search))}>
              <th scope="row" class="row-ident">
                <a class="cell-link" href={prefixHref(base, row.prefix, search)}>{row.prefix}</a>
                <span class={`family-badge family-${row.family}`}>{row.family === "ipv6" ? "v6" : "v4"}</span>
              </th>
              <td class="col-num">{formatCount(row.domain_count)}</td>
              <td class="col-num">{formatCount(row.address_count)}</td>
              <td class="row-ident">
                {#if row.asn !== undefined && row.asn !== null}
                  <a class="cell-link" href={asnHref(base, row.asn, search)} title={row.asn_label ? `AS${row.asn} · ${row.asn_label}` : `AS${row.asn}`}>
                    {#if row.asn_label}
                      <span class="operator-label">{row.asn_label}</span>
                      <span class="operator-asn">AS{row.asn}</span>
                    {:else}
                      AS{row.asn}
                    {/if}
                  </a>
                {:else}—{/if}
              </td>
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
  .list-head { display: flex; justify-content: space-between; align-items: flex-end; gap: var(--space-3); flex-wrap: wrap; }
  .list-head h2 { margin: 0; }
  .list-toolbar { display: flex; gap: var(--space-3); flex-wrap: wrap; align-items: flex-end; }
  .inline-field { display: flex; flex-direction: column; gap: 2px; font-size: var(--text-xs); color: var(--ink-2); text-transform: uppercase; letter-spacing: 0.04em; }
  .inline-field span { font-weight: 600; }
  .inline-field select { padding: 6px 10px; border: 1px solid var(--border); border-radius: 6px; background: var(--surface); color: var(--ink); font: inherit; font-size: var(--text-sm); text-transform: none; letter-spacing: normal; }
  .export-group { display: flex; gap: var(--space-2); }
  .table-wrap { overflow-x: auto; border: 1px solid var(--border); border-radius: var(--radius); background: var(--surface); }
  .data-table { width: 100%; border-collapse: collapse; font-size: var(--text-sm); }
  .data-table th { text-align: left; padding: 8px 10px; background: var(--surface-2); color: var(--ink-2); font-size: var(--text-xs); text-transform: uppercase; letter-spacing: 0.04em; border-bottom: 1px solid var(--border); white-space: nowrap; }
  .data-table td { padding: 8px 10px; border-bottom: 1px solid var(--border); vertical-align: middle; }
  .data-table tbody tr:last-child td { border-bottom: none; }
  .data-table tbody tr:hover { background: rgba(3, 105, 161, 0.04); }
  .row-clickable { cursor: pointer; }
  .family-badge {
    display: inline-block;
    margin-left: 6px;
    padding: 1px 6px;
    border-radius: 4px;
    font-size: var(--text-xs);
    font-weight: 600;
    font-family: var(--sans);
    letter-spacing: 0.04em;
  }
  .family-ipv4 { background: #e0f2fe; color: #075985; }
  .family-ipv6 { background: #ede9fe; color: #5b21b6; }
  .operator-label { font-family: var(--sans); font-weight: 500; }
  .operator-asn { margin-left: 6px; color: var(--ink-2); font-size: var(--text-xs); }
  .cell-link { color: inherit; text-decoration: none; }
  .cell-link:hover { text-decoration: underline; }
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
  .pagination { display: flex; justify-content: space-between; align-items: center; gap: var(--space-3); flex-wrap: wrap; margin-top: var(--space-3); }
  .pagination-controls { display: flex; gap: var(--space-2); }
</style>
