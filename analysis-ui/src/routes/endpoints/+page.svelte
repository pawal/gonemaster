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
  import { formatCount, formatMs } from "$lib/format";
  import { updateURLParam, filterFromURL } from "$lib/filters";
  import {
    downloadCSV,
    downloadJSON,
    collectExportRows,
    exportScope,
    exportCaption,
    exportScopeSuffix,
    type ExportColumn
  } from "$lib/exporters";
  import { listEndpoints, type EndpointView, type AnalysisFilter } from "$lib/api";
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
  // Only show the latency column when the snapshot actually has data for it.
  const hasLatency = $derived(rows.some((r) => r.latency_p50_ms != null));

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
    { key: "prefix", label: "Prefix", value: (r) => r.prefix ?? "" },
    { key: "latency_p50_ms", label: "Latency p50 (ms)", value: (r) => r.latency_p50_ms ?? "" },
    { key: "latency_p95_ms", label: "Latency p95 (ms)", value: (r) => r.latency_p95_ms ?? "" }
  ];

  function filenamePrefix(): string {
    const tag = data.datasetTag ?? "cohort";
    return `${tag}-endpoints`;
  }

  const exportScopeInfo = $derived(exportScope(total));
  const exportNote = $derived(exportCaption(exportScopeInfo));

  // Reproduce the loader's list params so the export honours the active
  // filter, snapshot and sort, then widen pagination to the row cap.
  function exportFilter(): AnalysisFilter {
    return {
      ...filterFromURL(page.url),
      dataset_tag: data.datasetTag ?? undefined,
      snapshot: layoutData.effectiveSnapshotSlug ?? undefined,
      sort: currentSort || undefined
    };
  }

  function fetchExportRows(): Promise<EndpointView[]> {
    return collectExportRows(data.list?.items ?? [], currentOffset, total, (limit) =>
      listEndpoints({ ...exportFilter(), limit, offset: 0 }).then((r) => r.items)
    );
  }

  async function exportCSV() {
    const rows = await fetchExportRows();
    if (!rows.length) return;
    downloadCSV(`${filenamePrefix()}${exportScopeSuffix(exportScopeInfo)}.csv`, rows, exportColumns);
  }

  async function exportJSONFile() {
    const rows = await fetchExportRows();
    if (!rows.length) return;
    downloadJSON(`${filenamePrefix()}${exportScopeSuffix(exportScopeInfo)}.json`, rows, exportColumns);
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
        <div class="export-buttons">
          <button type="button" class="ghost" onclick={exportCSV} disabled={!data.list?.items.length}>CSV</button>
          <button type="button" class="ghost" onclick={exportJSONFile} disabled={!data.list?.items.length}>JSON</button>
        </div>
        <p class="export-note">{exportNote}</p>
      </div>
    </div>
  </div>

  {#if !data.datasetTag}
    <p class="status-banner warn">No cohort resolved. Configure a public cohort to see endpoints.</p>
  {:else if data.error}
    <p class="status-banner error">Failed to load endpoints: {data.error}</p>
  {:else if !data.list || data.list.items.length === 0}
    <div class="empty-state">
      <p class="hint">No endpoints materialized for this cohort yet.</p>
    </div>
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
            {#if hasLatency}
              <th scope="col" class="col-num">Latency</th>
            {/if}
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
                {:else}-{/if}
              </td>
              <td class="row-ident">
                {#if row.prefix}
                  <PrefixChip prefix={row.prefix} />
                {:else}-{/if}
              </td>
              {#if hasLatency}
                <td class="col-num">
                  {#if row.latency_p50_ms != null}
                    {formatMs(row.latency_p50_ms)}
                    {#if row.latency_p95_ms != null}
                      <span class="latency-sub">p95 {formatMs(row.latency_p95_ms)}</span>
                    {/if}
                  {:else}
                    <span class="latency-sub">-</span>
                  {/if}
                </td>
              {/if}
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    <Pagination {total} offset={currentOffset} limit={currentLimit} itemCount={data.list?.items.length ?? 0} />
  {/if}
</section>

<style>
  /* Long operator labels would otherwise force horizontal scroll. Let the
     ASN chip wrap; break anywhere so labels without spaces still fit. */
  .data-table :global(.entity-chip-asn) {
    white-space: normal;
    overflow-wrap: anywhere;
  }
  .latency-sub { display: block; color: var(--ink-2); font-size: var(--text-xs); font-family: var(--sans); }
</style>
