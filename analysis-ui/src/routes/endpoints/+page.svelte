<script lang="ts">
  import { goto } from "$app/navigation";
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import FilterBar from "$lib/FilterBar.svelte";
  import Pagination from "$lib/Pagination.svelte";
  import SortHeader from "$lib/SortHeader.svelte";
  import ASNChip from "$lib/chips/ASNChip.svelte";
  import EndpointChip from "$lib/chips/EndpointChip.svelte";
  import NameserverChip from "$lib/chips/NameserverChip.svelte";
  import PrefixChip from "$lib/chips/PrefixChip.svelte";
  import { endpointHref } from "$lib/entityLinks";
  import { formatCount } from "$lib/format";
  import { updateURLParam } from "$lib/filters";
  import { downloadCSV, downloadJSON, type ExportColumn } from "$lib/exporters";
  import type { EndpointView } from "$lib/api";
  import type { LayoutData } from "../+layout";
  import type { EndpointsPageData } from "./+page";

  let { data }: { data: EndpointsPageData } = $props();

  const layoutData = $derived(page.data as LayoutData);

  const currentLimit = $derived(data.limit);
  const currentOffset = $derived(data.offset);
  const total = $derived(data.list?.total ?? 0);

  function updateParam(key: string, value: string) {
    updateURLParam(page.url, key, value);
  }

  const rows = $derived((data.list?.items ?? []) as EndpointView[]);
  const currentSort = $derived(page.url.searchParams.get("sort") ?? "");

  const sortSpecs = {
    nameserver: { desc: "nameserver_desc" },
    address: { asc: "address_asc", desc: "address_desc" },
    domainCount: { asc: "domain_count_asc", desc: "domain_count_desc" },
    operator: { asc: "operator_asc", desc: "operator_desc" },
    prefix: { asc: "prefix_asc", desc: "prefix_desc" }
  } as const;

  const exportColumns: ExportColumn<EndpointView>[] = [
    { key: "nameserver", label: "Nameserver", value: (r) => r.nameserver },
    { key: "address", label: "Address", value: (r) => r.address },
    { key: "family", label: "Family", value: (r) => r.family },
    { key: "domain_count", label: "Domains", value: (r) => r.domain_count },
    { key: "asn", label: "ASN", value: (r) => r.asn ?? "" },
    { key: "prefix", label: "Prefix", value: (r) => r.prefix ?? "" }
  ];

  function filenamePrefix(): string {
    const tag = data.datasetTag ?? "cohort";
    return `${tag}-endpoints`;
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
    <h2>Addresses</h2>
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
    <p class="status-banner warn">No cohort resolved. Configure a public cohort to see endpoints.</p>
  {:else if data.error}
    <p class="status-banner error">Failed to load endpoints: {data.error}</p>
  {:else if !data.list || data.list.items.length === 0}
    <p class="status-banner">No endpoints materialized for this cohort yet.</p>
  {:else}
    <div class="table-wrap">
      <table class="data-table">
        <thead>
          <tr>
            <th scope="col">
              <SortHeader label="Nameserver" spec={sortSpecs.nameserver} {currentSort} onsort={(v: string) => updateParam("sort", v)} />
            </th>
            <th scope="col">
              <SortHeader label="Address" spec={sortSpecs.address} {currentSort} onsort={(v: string) => updateParam("sort", v)} />
            </th>
            <th scope="col" class="col-num">
              <SortHeader label="Domains" spec={sortSpecs.domainCount} align="right" {currentSort} onsort={(v: string) => updateParam("sort", v)} />
            </th>
            <th scope="col">
              <SortHeader label="Operator" spec={sortSpecs.operator} {currentSort} onsort={(v: string) => updateParam("sort", v)} />
            </th>
            <th scope="col">
              <SortHeader label="Prefix" spec={sortSpecs.prefix} {currentSort} onsort={(v: string) => updateParam("sort", v)} />
            </th>
          </tr>
        </thead>
        <tbody>
          {#each rows as row (`${row.nameserver}|${row.address}`)}
            <tr class="row-clickable" onclick={(e: MouseEvent) => rowClick(e, endpointHref(base, row.address, row.nameserver, search))}>
              <th scope="row" class="row-ident">
                <NameserverChip nameserver={row.nameserver} />
              </th>
              <td class="row-ident">
                <EndpointChip address={row.address} nameserver={row.nameserver} />
              </td>
              <td class="col-num">{formatCount(row.domain_count)}</td>
              <td class="row-ident">
                {#if row.asn !== undefined && row.asn !== null}
                  <ASNChip asn={row.asn} label={row.asn_label ?? undefined} />
                {:else}—{/if}
              </td>
              <td class="row-ident">
                {#if row.prefix}
                  <PrefixChip prefix={row.prefix} />
                {:else}—{/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    <Pagination {total} offset={currentOffset} limit={currentLimit} itemCount={data.list?.items.length ?? 0} />
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
  /* Long operator labels would otherwise force horizontal scroll. Let the
     ASN chip wrap; break anywhere so labels without spaces still fit. */
  .data-table :global(.entity-chip-asn) {
    white-space: normal;
    overflow-wrap: anywhere;
  }
  .data-table tbody tr:last-child td { border-bottom: none; }
  .data-table tbody tr:hover { background: rgba(3, 105, 161, 0.04); }
  .row-ident {
    font-family: var(--mono);
    font-weight: 500;
    color: var(--ink);
    text-transform: none;
    letter-spacing: normal;
    font-size: var(--text-sm);
  }
  .row-clickable { cursor: pointer; }
  .col-num { text-align: right; font-variant-numeric: tabular-nums; }
</style>
