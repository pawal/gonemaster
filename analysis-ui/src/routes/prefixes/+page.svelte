<script lang="ts">
  import { goto } from "$app/navigation";
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import FilterBar from "$lib/FilterBar.svelte";
  import Pagination from "$lib/Pagination.svelte";
  import SortHeader from "$lib/SortHeader.svelte";
  import PrefixChip from "$lib/chips/PrefixChip.svelte";
  import ASNChip from "$lib/chips/ASNChip.svelte";
  import { prefixHref } from "$lib/entityLinks";
  import { formatCount } from "$lib/format";
  import { updateURLParam } from "$lib/filters";
  import { downloadCSV, downloadJSON, type ExportColumn } from "$lib/exporters";
  import type { PrefixView } from "$lib/api";
  import type { LayoutData } from "../+layout";
  import type { PrefixesPageData } from "./+page";

  let { data }: { data: PrefixesPageData } = $props();

  const layoutData = $derived(page.data as LayoutData);

  // Only domain_count is server-sortable; prefix ascending is the default.
  const sortSpecs = {
    domainCount: { asc: "domain_count_asc", desc: "domain_count_desc" }
  } as const;

  const currentSort = $derived(page.url.searchParams.get("sort") ?? "");
  const currentLimit = $derived(data.limit);
  const currentOffset = $derived(data.offset);
  const total = $derived(data.list?.total ?? 0);

  function updateParam(key: string, value: string) {
    updateURLParam(page.url, key, value);
  }

  const rows = $derived((data.list?.items ?? []) as PrefixView[]);
  const search = $derived(page.url.search);

  function rowClick(event: MouseEvent, href: string) {
    const target = event.target as HTMLElement | null;
    if (target?.closest("a")) return;
    goto(href);
  }

  function familyLabel(family: string): string {
    if (family === "ipv4") return "IPv4";
    if (family === "ipv6") return "IPv6";
    return family;
  }

  const exportColumns: ExportColumn<PrefixView>[] = [
    { key: "prefix", label: "Prefix", value: (r) => r.prefix },
    { key: "family", label: "Family", value: (r) => r.family },
    { key: "domain_count", label: "Domains", value: (r) => r.domain_count },
    { key: "address_count", label: "Addresses", value: (r) => r.address_count },
    { key: "asn", label: "ASN", value: (r) => r.asn ?? "" },
    { key: "asn_label", label: "Operator", value: (r) => r.asn_label ?? "" }
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

  <p class="list-hint">
    Operator is blank for prefixes announced by more than one ASN; the ASN and
    prefix-detail views cover those.
  </p>

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
            <th scope="col">Family</th>
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
                <PrefixChip prefix={row.prefix} />
              </th>
              <td>{familyLabel(row.family)}</td>
              <td class="col-num">{formatCount(row.domain_count)}</td>
              <td class="col-num">{formatCount(row.address_count)}</td>
              <td>
                {#if row.asn}
                  <ASNChip asn={row.asn} label={row.asn_label ?? undefined} />
                {:else}
                  <span class="muted">-</span>
                {/if}
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
  /* Long operator labels would otherwise force horizontal scroll. */
  .data-table :global(.entity-chip-asn) {
    white-space: normal;
    overflow-wrap: anywhere;
  }
  .list-hint {
    margin: 0 0 var(--space-3);
    color: var(--ink-2);
    font-size: var(--text-xs);
  }
  .muted { color: var(--ink-2); }
</style>
