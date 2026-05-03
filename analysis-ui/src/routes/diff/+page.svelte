<script lang="ts">
  import { goto } from "$app/navigation";
  import { page } from "$app/state";
  import FilterBar from "$lib/FilterBar.svelte";
  import type { DiffEntry } from "$lib/api";
  import { snapshotOptionLabel } from "$lib/format";
  import type { LayoutData } from "../+layout";
  import type { DiffPageData } from "./+page";

  let { data }: { data: DiffPageData } = $props();

  const layoutData = $derived(page.data as LayoutData);

  // Tabs mirror the four lists the API returns. The count in each label
  // comes from the diff response so the reader knows at a glance where
  // the change is concentrated.
  type TabKey = "added" | "removed" | "grade_changed" | "level_changed";
  const TABS: { key: TabKey; label: string }[] = [
    { key: "added", label: "Added" },
    { key: "removed", label: "Removed" },
    { key: "grade_changed", label: "Grade changed" },
    { key: "level_changed", label: "Worst-level changed" }
  ];
  let activeTab = $state<TabKey>("added");

  function entriesFor(key: TabKey): DiffEntry[] {
    if (!data.diff) return [];
    switch (key) {
      case "added":
        return data.diff.added ?? [];
      case "removed":
        return data.diff.removed ?? [];
      case "grade_changed":
        return data.diff.grade_changed ?? [];
      case "level_changed":
        return data.diff.level_changed ?? [];
    }
  }

  function countFor(key: TabKey): number {
    return entriesFor(key).length;
  }

  // Snapshot-slug dropdowns let the reader pick from or to without
  // typing the slug. When either side changes the URL updates so the
  // back button does the right thing.
  function setSide(side: "from" | "to", slug: string) {
    const params = new URLSearchParams(page.url.searchParams);
    if (slug) params.set(side, slug);
    else params.delete(side);
    goto(`${page.url.pathname}?${params.toString()}`, {
      replaceState: true,
      noScroll: true
    });
  }

  const entries = $derived(entriesFor(activeTab));

  const gradeColumnNeeded = $derived(
    activeTab === "added" || activeTab === "removed" || activeTab === "grade_changed"
  );
  const levelColumnNeeded = $derived(activeTab === "level_changed");
</script>

<FilterBar
  cohorts={layoutData.catalog?.cohorts ?? []}
  selectorEnabled={layoutData.catalog?.selector_enabled ?? false}
  snapshots={layoutData.snapshots ?? []}
  defaultSnapshotSlug={layoutData.defaultSnapshotSlug ?? ""}
  showSearch={false}
/>

<section class="card diff-header">
  <h2>Snapshot diff</h2>
  <p class="hint">
    Compare two snapshots of the same cohort. "Added" / "Removed" lists
    domain membership changes; "Grade changed" and "Worst-level changed"
    list domains that stayed in the cohort but moved between snapshots.
  </p>
  <div class="diff-side-select">
    <label>
      <span>From</span>
      <select value={data.fromSlug} onchange={(e) => setSide("from", e.currentTarget.value)}>
        <option value="">Pick snapshot…</option>
        {#each layoutData.snapshots ?? [] as snap (snap.slug)}
          <option value={snap.slug}>{snapshotOptionLabel(snap)}</option>
        {/each}
      </select>
    </label>
    <label>
      <span>To</span>
      <select value={data.toSlug} onchange={(e) => setSide("to", e.currentTarget.value)}>
        <option value="">Pick snapshot…</option>
        {#each layoutData.snapshots ?? [] as snap (snap.slug)}
          <option value={snap.slug}>{snapshotOptionLabel(snap)}</option>
        {/each}
      </select>
    </label>
  </div>
</section>

{#if data.error}
  <section class="card">
    <p class="status-banner error">Failed to load diff: {data.error}</p>
  </section>
{:else if !data.datasetTag}
  <section class="card empty-state">
    <p class="hint">No public cohort is configured - pick one from the filter bar to diff its snapshots.</p>
  </section>
{:else if !data.fromSlug || !data.toSlug}
  <section class="card empty-state">
    <p class="hint">Pick a <strong>From</strong> and <strong>To</strong> snapshot above to compute the diff.</p>
  </section>
{:else if !data.diff}
  <section class="card empty-state">
    <p class="hint">No diff available for the selected snapshots.</p>
  </section>
{:else}
  <nav class="diff-tabs" aria-label="Diff category">
    {#each TABS as tab (tab.key)}
      <button
        type="button"
        class="diff-tab"
        class:active={activeTab === tab.key}
        onclick={() => (activeTab = tab.key)}
      >
        {tab.label}
        <span class="diff-tab-count">{countFor(tab.key)}</span>
      </button>
    {/each}
  </nav>

  <section class="card">
    {#if entries.length === 0}
      <p class="hint">No entries in this category.</p>
    {:else}
      <table class="diff-table">
        <thead>
          <tr>
            <th>Domain</th>
            {#if gradeColumnNeeded}
              <th>From grade</th>
              <th>To grade</th>
            {/if}
            {#if levelColumnNeeded}
              <th>From level</th>
              <th>To level</th>
            {/if}
          </tr>
        </thead>
        <tbody>
          {#each entries as entry (entry.domain)}
            <tr>
              <td class="domain-cell">{entry.domain}</td>
              {#if gradeColumnNeeded}
                <td>{entry.from_grade ?? ""}</td>
                <td>{entry.to_grade ?? ""}</td>
              {/if}
              {#if levelColumnNeeded}
                <td>{entry.from_level ?? ""}</td>
                <td>{entry.to_level ?? ""}</td>
              {/if}
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </section>
{/if}

<style>
  .diff-header {
    gap: var(--space-2);
  }
  .diff-header h2 {
    margin: 0;
  }
  .diff-side-select {
    display: flex;
    gap: var(--space-3);
    flex-wrap: wrap;
  }
  .diff-side-select label {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: var(--text-xs);
    color: var(--ink-2);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .diff-side-select select {
    min-width: 200px;
    padding: 6px 10px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--surface);
    color: var(--ink);
    font: inherit;
    font-size: var(--text-sm);
    letter-spacing: normal;
    text-transform: none;
  }
  .diff-tabs {
    display: flex;
    gap: var(--space-2);
    flex-wrap: wrap;
  }
  .diff-tab {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    padding: 6px var(--space-3);
    border: 1px solid var(--border);
    border-radius: 999px;
    background: var(--surface);
    color: var(--ink);
    font: inherit;
    cursor: pointer;
  }
  .diff-tab.active {
    border-color: var(--accent-2);
    background: var(--surface-2);
  }
  .diff-tab-count {
    font-family: var(--mono);
    font-size: var(--text-xs);
    color: var(--ink-2);
  }
  .diff-table {
    width: 100%;
    border-collapse: collapse;
    font-size: var(--text-sm);
  }
  .diff-table th,
  .diff-table td {
    text-align: left;
    padding: 6px var(--space-3);
    border-bottom: 1px solid var(--border);
  }
  .diff-table th {
    color: var(--ink-2);
    font-weight: 600;
    text-transform: uppercase;
    font-size: var(--text-xs);
    letter-spacing: 0.04em;
  }
  .domain-cell {
    font-family: var(--mono);
  }
</style>
