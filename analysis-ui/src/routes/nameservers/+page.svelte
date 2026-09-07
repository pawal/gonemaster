<script lang="ts">
  import { goto } from "$app/navigation";
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import FilterBar from "$lib/FilterBar.svelte";
  import Pagination from "$lib/Pagination.svelte";
  import SortHeader from "$lib/SortHeader.svelte";
  import ASNChip from "$lib/chips/ASNChip.svelte";
  import NameserverChip from "$lib/chips/NameserverChip.svelte";
  import { nameserverHref } from "$lib/entityLinks";
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
  import { listNameservers, type NameserverView, type AnalysisFilter } from "$lib/api";
  import { latencyVantageNote } from "$lib/latencyRanking";
  import type { LayoutData } from "../+layout";
  import type { NameserversPageData } from "./+page";

  let { data }: { data: NameserversPageData } = $props();

  const layoutData = $derived(page.data as LayoutData);
  const vantageNote = $derived(latencyVantageNote(layoutData.vantageLabel));

  const sortSpecs = {
    name: { desc: "name_desc" },
    domainCount: { asc: "domain_count_asc", desc: "domain_count_desc" },
    endpointCount: { asc: "endpoint_count_asc", desc: "endpoint_count_desc" },
    latency: { asc: "latency_p50_asc", desc: "latency_p50_desc" }
  } as const;

  const currentSort = $derived(page.url.searchParams.get("sort") ?? "");
  const currentLimit = $derived(data.limit);
  const currentOffset = $derived(data.offset);
  const total = $derived(data.list?.total ?? 0);

  function updateParam(key: string, value: string) {
    updateURLParam(page.url, key, value);
  }

  const rows = $derived((data.list?.items ?? []) as NameserverView[]);
  const search = $derived(page.url.search);
  // Only show the latency column when the snapshot actually has data for it.
  const hasLatency = $derived(rows.some((r) => r.latency_p50_ms != null));

  function rowClick(event: MouseEvent, href: string) {
    const target = event.target as HTMLElement | null;
    if (target?.closest("a")) return;
    goto(href);
  }

  const exportColumns: ExportColumn<NameserverView>[] = [
    { key: "nameserver", label: "Nameserver", value: (r) => r.nameserver },
    { key: "operator", label: "Operator", value: (r) => r.operator ?? "" },
    { key: "operator_asn", label: "Operator ASN", value: (r) => r.operator_asn ?? "" },
    { key: "domain_count", label: "Domains", value: (r) => r.domain_count },
    { key: "endpoint_count", label: "Endpoints", value: (r) => r.endpoint_count },
    { key: "ipv4_count", label: "IPv4", value: (r) => r.ipv4_count },
    { key: "ipv6_count", label: "IPv6", value: (r) => r.ipv6_count },
    { key: "asn_count", label: "ASNs", value: (r) => r.asn_count },
    { key: "latency_p50_ms", label: "Latency p50 (ms)", value: (r) => r.latency_p50_ms ?? "" },
    { key: "latency_p95_ms", label: "Latency p95 (ms)", value: (r) => r.latency_p95_ms ?? "" }
  ];

  function filenamePrefix(): string {
    const tag = data.datasetTag ?? "cohort";
    return `${tag}-nameservers`;
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

  function fetchExportRows(): Promise<NameserverView[]> {
    return collectExportRows(data.list?.items ?? [], currentOffset, total, (limit) =>
      listNameservers({ ...exportFilter(), limit, offset: 0 }).then((r) => r.items)
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
    <h2>Nameservers</h2>
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
    <p class="status-banner warn">No cohort resolved. Configure a public cohort to see nameservers.</p>
  {:else if data.error}
    <p class="status-banner error">Failed to load nameservers: {data.error}</p>
  {:else if !data.list || data.list.items.length === 0}
    <div class="empty-state">
      <p class="hint">No nameservers materialized for this cohort yet.</p>
    </div>
  {:else}
    <div class="table-wrap">
      <table class="data-table">
        <thead>
          <tr>
            <th scope="col">
              <SortHeader label="Nameserver" spec={sortSpecs.name} {currentSort} onsort={(v: string) => updateParam("sort", v)} />
            </th>
            <th scope="col">Operator</th>
            <th scope="col" class="col-num">
              <SortHeader label="Domains" spec={sortSpecs.domainCount} align="right" {currentSort} onsort={(v: string) => updateParam("sort", v)} />
            </th>
            <th scope="col" class="col-num">
              <SortHeader label="Endpoints" spec={sortSpecs.endpointCount} align="right" {currentSort} onsort={(v: string) => updateParam("sort", v)} />
            </th>
            <th scope="col" class="col-num">IPv4</th>
            <th scope="col" class="col-num">IPv6</th>
            {#if hasLatency}
              <th scope="col" class="col-num">
                <SortHeader label="Latency" spec={sortSpecs.latency} align="right" title={vantageNote} {currentSort} onsort={(v: string) => updateParam("sort", v)} />
              </th>
            {/if}
          </tr>
        </thead>
        <tbody>
          {#each rows as row (row.nameserver)}
            <tr class="row-clickable" onclick={(e: MouseEvent) => rowClick(e, nameserverHref(base, row.nameserver, search))}>
              <th scope="row" class="row-ident">
                <NameserverChip nameserver={row.nameserver} />
              </th>
              <td class="row-ident">
                {#if row.operator === "Multiple"}
                  <span class="operator-multi">Multiple ({row.asn_count})</span>
                {:else if row.operator_asn !== undefined && row.operator_asn !== null}
                  <ASNChip asn={row.operator_asn} label={row.operator ?? undefined} />
                {:else}-{/if}
              </td>
              <td class="col-num">{formatCount(row.domain_count)}</td>
              <td class="col-num">{formatCount(row.endpoint_count)}</td>
              <td class="col-num">{formatCount(row.ipv4_count)}</td>
              <td class="col-num">{formatCount(row.ipv6_count)}</td>
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

    {#if hasLatency}
      <p class="hint">{vantageNote}</p>
    {/if}

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
  .operator-multi { color: var(--ink-2); font-family: var(--sans); font-style: italic; }
  .latency-sub { display: block; color: var(--ink-2); font-size: var(--text-xs); font-family: var(--sans); }
</style>
