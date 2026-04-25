<script lang="ts">
  import { goto } from "$app/navigation";
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import FilterBar from "$lib/FilterBar.svelte";
  import Pagination from "$lib/Pagination.svelte";
  import SortHeader from "$lib/SortHeader.svelte";
  import TagChip from "$lib/chips/TagChip.svelte";
  import { tagHref } from "$lib/entityLinks";
  import { formatCount, levelTone } from "$lib/format";
  import { updateURLParam } from "$lib/filters";
  import { downloadCSV, downloadJSON, type ExportColumn } from "$lib/exporters";
  import type { TagView } from "$lib/api";
  import type { LayoutData } from "../+layout";
  import type { TagsPageData } from "./+page";

  let { data }: { data: TagsPageData } = $props();

  const layoutData = $derived(page.data as LayoutData);

  const sortSpecs = {
    // server default when sort is empty is already domain_count_desc; the
    // Domains header still needs desc as the active token so the arrow
    // visibly matches the default ordering.
    domainCount: { asc: "domain_count_asc", desc: "domain_count_desc" },
    occurrenceCount: { asc: "occurrence_count_asc", desc: "occurrence_count_desc" },
    level: { asc: "level_asc", desc: "level_desc" }
  } as const;

  const currentSort = $derived(page.url.searchParams.get("sort") ?? "");
  const currentLimit = $derived(data.limit);
  const currentOffset = $derived(data.offset);
  const total = $derived(data.list?.total ?? 0);

  function updateParam(key: string, value: string) {
    updateURLParam(page.url, key, value);
  }

  const rows = $derived((data.list?.items ?? []) as TagView[]);
  const search = $derived(page.url.search);

  function rowClick(event: MouseEvent, href: string) {
    const target = event.target as HTMLElement | null;
    if (target?.closest("a")) return;
    goto(href);
  }

  const exportColumns: ExportColumn<TagView>[] = [
    { key: "tag", label: "Tag", value: (r) => r.tag },
    { key: "module", label: "Module", value: (r) => r.module ?? "" },
    { key: "level", label: "Level", value: (r) => r.level ?? "" },
    { key: "domain_count", label: "Domains", value: (r) => r.domain_count },
    { key: "occurrence_count", label: "Occurrences", value: (r) => r.occurrence_count }
  ];

  function filenamePrefix(): string {
    const tag = data.datasetTag ?? "cohort";
    return `${tag}-tags`;
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
    <h2>Tags</h2>
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
    <p class="status-banner warn">No cohort resolved. Configure a public cohort to see tags.</p>
  {:else if data.error}
    <p class="status-banner error">Failed to load tags: {data.error}</p>
  {:else if !data.list || data.list.items.length === 0}
    <p class="status-banner">No finding tags materialized for this cohort yet.</p>
  {:else}
    <div class="table-wrap">
      <table class="data-table">
        <thead>
          <tr>
            <th scope="col">Tag</th>
            <th scope="col">Module</th>
            <th scope="col">
              <SortHeader label="Level" spec={sortSpecs.level} {currentSort} onsort={(v: string) => updateParam("sort", v)} />
            </th>
            <th scope="col" class="col-num">
              <SortHeader label="Domains" spec={sortSpecs.domainCount} align="right" {currentSort} onsort={(v: string) => updateParam("sort", v)} />
            </th>
            <th scope="col" class="col-num">
              <SortHeader label="Occurrences" spec={sortSpecs.occurrenceCount} align="right" {currentSort} onsort={(v: string) => updateParam("sort", v)} />
            </th>
          </tr>
        </thead>
        <tbody>
          {#each rows as row (row.tag)}
            <tr class="row-clickable" onclick={(e: MouseEvent) => rowClick(e, tagHref(base, row.tag, search))}>
              <th scope="row" class="row-ident">
                <TagChip tag={row.tag} />
              </th>
              <td>{row.module ?? "—"}</td>
              <td>
                {#if row.level}
                  <span class={`level level-${levelTone(row.level)}`}>{row.level}</span>
                {:else}—{/if}
              </td>
              <td class="col-num">{formatCount(row.domain_count)}</td>
              <td class="col-num">{formatCount(row.occurrence_count)}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    <Pagination {total} offset={currentOffset} limit={currentLimit} itemCount={data.list?.items.length ?? 0} />
  {/if}
</section>
