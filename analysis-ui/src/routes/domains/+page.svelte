<script lang="ts">
  import { goto } from "$app/navigation";
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import FilterBar from "$lib/FilterBar.svelte";
  import Pagination from "$lib/Pagination.svelte";
  import SortHeader from "$lib/SortHeader.svelte";
  import { asnHref, domainHref } from "$lib/entityLinks";
  import { formatCount, formatTimestamp, gradeTone, levelTone } from "$lib/format";
  import { idnTooltip } from "$lib/idn";
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
  import { listDomains, type DomainView, type AnalysisFilter } from "$lib/api";
  import type { LayoutData } from "../+layout";
  import type { DomainsPageData } from "./+page";

  let { data }: { data: DomainsPageData } = $props();

  const layoutData = $derived(page.data as LayoutData);

  const sortSpecs = {
    domain: { asc: "domain_asc", desc: "domain_desc" },
    score: { asc: "score_asc", desc: "score_desc" },
    worst: { desc: "worst_level_desc" },
    nameservers: { asc: "nameserver_count_asc", desc: "nameserver_count_desc" },
    endpoints: { asc: "endpoint_count_asc", desc: "endpoint_count_desc" },
    asns: { asc: "asn_count_asc", desc: "asn_count_desc" },
    prefixes: { asc: "prefix_count_asc", desc: "prefix_count_desc" }
  } as const;

  const currentSort = $derived(page.url.searchParams.get("sort") ?? "");
  const currentLimit = $derived(data.limit);
  const currentOffset = $derived(data.offset);
  const total = $derived(data.list?.total ?? 0);

  function updateParam(key: string, value: string) {
    updateURLParam(page.url, key, value);
  }

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

  function fetchExportRows(): Promise<DomainView[]> {
    return collectExportRows(data.list?.items ?? [], currentOffset, total, (limit) =>
      listDomains({ ...exportFilter(), limit, offset: 0 }).then((r) => r.items)
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
        <div class="export-buttons">
          <button type="button" class="ghost" onclick={exportCSV} disabled={!data.list?.items.length}>CSV</button>
          <button type="button" class="ghost" onclick={exportJSONFile} disabled={!data.list?.items.length}>JSON</button>
        </div>
        <p class="export-note">{exportNote}</p>
      </div>
    </div>
  </div>

  {#if !data.datasetTag}
    <p class="status-banner warn">No cohort resolved. Configure a public cohort to see domains.</p>
  {:else if data.error}
    <p class="status-banner error">Failed to load domains: {data.error}</p>
  {:else if !data.list || data.list.items.length === 0}
    <div class="empty-state">
      <p class="hint">No domains materialized for this cohort yet.</p>
    </div>
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
            <th scope="col" class="col-num">
              <SortHeader label="Nameservers" spec={sortSpecs.nameservers} align="right" {currentSort} onsort={(v: string) => updateParam("sort", v)} />
            </th>
            <th scope="col" class="col-num">
              <SortHeader label="Endpoints" spec={sortSpecs.endpoints} align="right" {currentSort} onsort={(v: string) => updateParam("sort", v)} />
            </th>
            <th scope="col" class="col-num">
              <SortHeader label="ASNs" spec={sortSpecs.asns} align="right" {currentSort} onsort={(v: string) => updateParam("sort", v)} />
            </th>
            <th scope="col" class="col-num">
              <SortHeader label="Prefixes" spec={sortSpecs.prefixes} align="right" {currentSort} onsort={(v: string) => updateParam("sort", v)} />
            </th>
            <th scope="col">Last analyzed</th>
          </tr>
        </thead>
        <tbody>
          {#each rows as row (row.domain)}
            <tr class="row-clickable" onclick={(e: MouseEvent) => rowClick(e, domainHref(base, row.domain, search))}>
              <th scope="row" class="row-ident">
                <a class="cell-link" href={domainHref(base, row.domain, search)} title={idnTooltip(row.domain)}>{row.domain}</a>
              </th>
              <td class="col-num">{row.score ?? "-"}</td>
              <td>
                {#if row.grade}
                  <span class={`grade grade-${gradeTone(row.grade)}`}>{row.grade}</span>
                {:else}-{/if}
              </td>
              <td>
                {#if row.worst_level}
                  <span class={`level level-${levelTone(row.worst_level)}`}>{row.worst_level}</span>
                {:else}-{/if}
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
                {:else}-{/if}
              </td>
              <td class="col-num">{formatCount(row.nameserver_count)}</td>
              <td class="col-num">{formatCount(row.endpoint_count)}</td>
              <td class="col-num">{formatCount(row.asn_count)}</td>
              <td class="col-num">{formatCount(row.prefix_count)}</td>
              <td>{formatTimestamp(row.finished_at) || "-"}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    <Pagination {total} offset={currentOffset} limit={currentLimit} itemCount={data.list?.items.length ?? 0} />
  {/if}
</section>

<style>
  .operator-label { font-family: var(--sans); font-weight: 500; }
  .operator-asn { margin-left: 6px; color: var(--ink-2); font-size: var(--text-xs); }
  .operator-multi { color: var(--ink-2); font-family: var(--sans); font-style: italic; }
</style>
