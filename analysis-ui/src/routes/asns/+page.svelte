<script lang="ts">
  import { goto } from "$app/navigation";
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import FilterBar from "$lib/FilterBar.svelte";
  import SortHeader from "$lib/SortHeader.svelte";
  import ASNChip from "$lib/chips/ASNChip.svelte";
  import { asnHref } from "$lib/entityLinks";
  import { formatCount } from "$lib/format";
  import { searchToString, updateURLParam } from "$lib/filters";
  import { downloadCSV, downloadJSON, type ExportColumn } from "$lib/exporters";
  import type { ASNView } from "$lib/api";
  import type { LayoutData } from "../+layout";
  import type { ASNsPageData } from "./+page";

  let { data }: { data: ASNsPageData } = $props();

  const layoutData = $derived(page.data as LayoutData);

  const sortSpecs = {
    domainCount: { asc: "domain_count_asc", desc: "domain_count_desc" },
    addressCount: { asc: "address_count_asc", desc: "address_count_desc" },
    nameserverCount: { asc: "nameserver_count_asc", desc: "nameserver_count_desc" },
    prefixCount: { asc: "prefix_count_asc", desc: "prefix_count_desc" }
  } as const;

  const currentSort = $derived(page.url.searchParams.get("sort") ?? "");
  const currentLimit = $derived(data.limit);
  const currentOffset = $derived(data.offset);
  const total = $derived(data.list?.total ?? 0);
  const pageStart = $derived(total === 0 ? 0 : currentOffset + 1);
  const pageEnd = $derived(Math.min(currentOffset + (data.list?.items.length ?? 0), total));

  function updateParam(key: string, value: string) {
    updateURLParam(page.url, key, value);
  }

  function gotoPage(nextOffset: number) {
    const params = new URLSearchParams(page.url.searchParams);
    if (nextOffset > 0) params.set("offset", String(nextOffset));
    else params.delete("offset");
    goto(`${page.url.pathname}${searchToString(params)}`);
  }

  const hasPrev = $derived(currentOffset > 0);
  const hasNext = $derived(currentOffset + currentLimit < total);

  const rows = $derived((data.list?.items ?? []) as ASNView[]);
  const search = $derived(page.url.search);

  function rowClick(event: MouseEvent, href: string) {
    const target = event.target as HTMLElement | null;
    if (target?.closest("a")) return;
    goto(href);
  }

  const exportColumns: ExportColumn<ASNView>[] = [
    { key: "asn", label: "ASN", value: (r) => r.asn },
    { key: "label", label: "Label", value: (r) => r.label ?? "" },
    { key: "domain_count", label: "Domains", value: (r) => r.domain_count },
    { key: "address_count", label: "Addresses", value: (r) => r.address_count },
    { key: "nameserver_count", label: "Nameservers", value: (r) => r.nameserver_count },
    { key: "prefix_count", label: "Prefixes", value: (r) => r.prefix_count },
    { key: "ipv4_count", label: "IPv4", value: (r) => r.ipv4_count },
    { key: "ipv6_count", label: "IPv6", value: (r) => r.ipv6_count }
  ];

  function filenamePrefix(): string {
    const tag = data.datasetTag ?? "cohort";
    return `${tag}-asns`;
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
    <h2>ASNs</h2>
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
    <p class="status-banner warn">No cohort resolved. Configure a public cohort to see ASNs.</p>
  {:else if data.error}
    <p class="status-banner error">Failed to load ASNs: {data.error}</p>
  {:else if !data.list || data.list.items.length === 0}
    <p class="status-banner">No ASNs materialized for this cohort yet.</p>
  {:else}
    <div class="table-wrap">
      <table class="data-table">
        <thead>
          <tr>
            <th scope="col">Operator</th>
            <th scope="col" class="col-num">
              <SortHeader label="Domains" spec={sortSpecs.domainCount} align="right" {currentSort} onsort={(v: string) => updateParam("sort", v)} />
            </th>
            <th scope="col" class="col-num">
              <SortHeader label="Addresses" spec={sortSpecs.addressCount} align="right" {currentSort} onsort={(v: string) => updateParam("sort", v)} />
            </th>
            <th scope="col" class="col-num">
              <SortHeader label="Nameservers" spec={sortSpecs.nameserverCount} align="right" {currentSort} onsort={(v: string) => updateParam("sort", v)} />
            </th>
            <th scope="col" class="col-num">
              <SortHeader label="Prefixes" spec={sortSpecs.prefixCount} align="right" {currentSort} onsort={(v: string) => updateParam("sort", v)} />
            </th>
          </tr>
        </thead>
        <tbody>
          {#each rows as row (row.asn)}
            <tr class="row-clickable" onclick={(e: MouseEvent) => rowClick(e, asnHref(base, row.asn, search))}>
              <th scope="row" class="row-ident">
                <ASNChip asn={row.asn} label={row.label ?? undefined} />
              </th>
              <td class="col-num">{formatCount(row.domain_count)}</td>
              <td class="col-num">
                {formatCount(row.address_count)}
                {#if row.ipv4_count > 0 || row.ipv6_count > 0}
                  <span class="family-mix">{row.ipv4_count}v4 · {row.ipv6_count}v6</span>
                {/if}
              </td>
              <td class="col-num">{formatCount(row.nameserver_count)}</td>
              <td class="col-num">{formatCount(row.prefix_count)}</td>
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

  .export-group { display: flex; gap: var(--space-2); }

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
  /* Long operator labels would otherwise force horizontal scroll. Let the
     ASN chip wrap; break anywhere so labels without spaces still fit. */
  .data-table :global(.entity-chip-asn) {
    white-space: normal;
    overflow-wrap: anywhere;
  }
  .data-table tbody tr:last-child td { border-bottom: none; }
  .data-table tbody tr:hover { background: rgba(3, 105, 161, 0.04); }
  .row-clickable { cursor: pointer; }
  .row-ident {
    font-family: var(--mono);
    font-weight: 500;
    color: var(--ink);
    text-transform: none;
    letter-spacing: normal;
    font-size: var(--text-sm);
  }
  .col-num { text-align: right; font-variant-numeric: tabular-nums; }
  .family-mix { display: block; color: var(--ink-2); font-size: var(--text-xs); font-family: var(--sans); }

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
