<script lang="ts">
  import { goto } from "$app/navigation";
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import FilterBar from "$lib/FilterBar.svelte";
  import Pagination from "$lib/Pagination.svelte";
  import SortHeader from "$lib/SortHeader.svelte";
  import ASNChip from "$lib/chips/ASNChip.svelte";
  import { asnHref } from "$lib/entityLinks";
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
  import { listASNs, type ASNView, type AnalysisFilter } from "$lib/api";
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

  function updateParam(key: string, value: string) {
    updateURLParam(page.url, key, value);
  }

  const rows = $derived((data.list?.items ?? []) as ASNView[]);
  const search = $derived(page.url.search);
  // Only show the latency column when the snapshot actually has data for it.
  const hasLatency = $derived(rows.some((r) => r.latency_p50_ms != null));

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
    { key: "ipv6_count", label: "IPv6", value: (r) => r.ipv6_count },
    { key: "latency_p50_ms", label: "Latency p50 (ms)", value: (r) => r.latency_p50_ms ?? "" },
    { key: "latency_p95_ms", label: "Latency p95 (ms)", value: (r) => r.latency_p95_ms ?? "" }
  ];

  function filenamePrefix(): string {
    const tag = data.datasetTag ?? "cohort";
    return `${tag}-asns`;
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

  function fetchExportRows(): Promise<ASNView[]> {
    return collectExportRows(data.list?.items ?? [], currentOffset, total, (limit) =>
      listASNs({ ...exportFilter(), limit, offset: 0 }).then((r) => r.items)
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
        <div class="export-buttons">
          <button type="button" class="ghost" onclick={exportCSV} disabled={!data.list?.items.length}>CSV</button>
          <button type="button" class="ghost" onclick={exportJSONFile} disabled={!data.list?.items.length}>JSON</button>
        </div>
        <p class="export-note">{exportNote}</p>
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
            {#if hasLatency}
              <th scope="col" class="col-num">Latency</th>
            {/if}
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
              {#if hasLatency}
                <td class="col-num">
                  {#if row.latency_p50_ms != null}
                    {formatMs(row.latency_p50_ms)}
                    {#if row.latency_p95_ms != null}
                      <span class="family-mix">p95 {formatMs(row.latency_p95_ms)}</span>
                    {/if}
                  {:else}
                    <span class="family-mix">-</span>
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
  .family-mix { display: block; color: var(--ink-2); font-size: var(--text-xs); font-family: var(--sans); }
</style>
