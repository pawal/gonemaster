<script lang="ts">
  import { goto } from "$app/navigation";
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import FilterBar from "$lib/FilterBar.svelte";
  import GradeChip from "$lib/GradeChip.svelte";
  import TagChip from "$lib/chips/TagChip.svelte";
  import { domainHref } from "$lib/entityLinks";
  import { snapshotOptionLabel, levelTone, formatCount } from "$lib/format";
  import { downloadCSV, type ExportColumn } from "$lib/exporters";
  import {
    buildGradeMatrix,
    gradeDirection,
    levelDirection,
    sortByMovement,
    summarizeDiff
  } from "$lib/diff";
  import type { DiffEntry, TagDiffEntry } from "$lib/api";
  import type { LayoutData } from "../+layout";
  import type { DiffPageData } from "./+page";

  let { data }: { data: DiffPageData } = $props();

  const layoutData = $derived(page.data as LayoutData);

  type TabKey = "added" | "removed" | "grade_changed" | "level_changed";
  const TABS: { key: TabKey; label: string }[] = [
    { key: "added", label: "Added" },
    { key: "removed", label: "Removed" },
    { key: "grade_changed", label: "Grade changed" },
    { key: "level_changed", label: "Worst-level changed" }
  ];
  const TAB_KEYS = TABS.map((t) => t.key);

  // Tab lives in the URL so a shared link lands on the same list. Read it from
  // page state (not the loader) so switching tabs never refetches the diff.
  const activeTab = $derived.by<TabKey>(() => {
    const raw = page.url.searchParams.get("tab") ?? "";
    return (TAB_KEYS as string[]).includes(raw) ? (raw as TabKey) : "added";
  });

  function setTab(key: TabKey) {
    const params = new URLSearchParams(page.url.searchParams);
    params.set("tab", key);
    goto(`${page.url.pathname}?${params.toString()}`, { replaceState: true, noScroll: true });
  }

  function setSide(side: "from" | "to", slug: string) {
    const params = new URLSearchParams(page.url.searchParams);
    if (slug) params.set(side, slug);
    else params.delete(side);
    goto(`${page.url.pathname}?${params.toString()}`, { replaceState: true, noScroll: true });
  }

  // Swap the two sides so the reader can flip the direction of the comparison
  // without re-picking both dropdowns.
  function swapSides() {
    const params = new URLSearchParams(page.url.searchParams);
    const from = data.fromSlug;
    const to = data.toSlug;
    if (from) params.set("to", from);
    else params.delete("to");
    if (to) params.set("from", to);
    else params.delete("from");
    goto(`${page.url.pathname}?${params.toString()}`, { replaceState: true, noScroll: true });
  }

  const summary = $derived(summarizeDiff(data.diff));
  const matrix = $derived(buildGradeMatrix(data.diff?.grade_changed ?? []));

  // Total tag movements, for the "no changes" empty state.
  const tagChangeTotal = $derived(
    data.tagDiff
      ? data.tagDiff.appeared.length + data.tagDiff.cleared.length + data.tagDiff.level_changed.length
      : 0
  );

  function signedCount(n: number): string {
    if (n > 0) return `+${formatCount(n)}`;
    if (n < 0) return `-${formatCount(-n)}`;
    return "0";
  }

  const entries = $derived.by<DiffEntry[]>(() => {
    if (!data.diff) return [];
    switch (activeTab) {
      case "added":
        return data.diff.added ?? [];
      case "removed":
        return data.diff.removed ?? [];
      case "grade_changed":
        return sortByMovement(data.diff.grade_changed ?? [], "grade");
      case "level_changed":
        return sortByMovement(data.diff.level_changed ?? [], "level");
    }
  });

  function countFor(key: TabKey): number {
    if (!data.diff) return 0;
    return (data.diff[key] ?? []).length;
  }

  // Open the domain detail in the "to" snapshot's context, keeping the cohort
  // scope so the page resolves the right materialized rows.
  function domainLink(domain: string): string {
    const params = new URLSearchParams();
    if (data.datasetTag) params.set("dataset_tag", data.datasetTag);
    if (data.toSlug) params.set("snapshot", data.toSlug);
    const q = params.toString();
    return domainHref(base, domain, q ? `?${q}` : "");
  }

  // Discrete intensity + direction class for a transition cell, avoiding a
  // dynamic inline style. Cells above the diagonal are regressions, below are
  // improvements.
  function cellClass(fromIdx: number, toIdx: number, count: number): string {
    if (count <= 0) return "";
    const dir = toIdx > fromIdx ? "reg" : "imp";
    const level = matrix.max > 0 ? Math.min(3, Math.ceil((count / matrix.max) * 3)) : 1;
    return `${dir}-${level}`;
  }

  const exportColumns = $derived.by<ExportColumn<DiffEntry>[]>(() => {
    const domain: ExportColumn<DiffEntry> = { key: "domain", label: "Domain", value: (r) => r.domain };
    if (activeTab === "grade_changed") {
      return [
        domain,
        { key: "from_grade", label: "From grade", value: (r) => r.from_grade ?? "" },
        { key: "to_grade", label: "To grade", value: (r) => r.to_grade ?? "" },
        { key: "from_level", label: "From level", value: (r) => r.from_level ?? "" },
        { key: "to_level", label: "To level", value: (r) => r.to_level ?? "" }
      ];
    }
    if (activeTab === "level_changed") {
      return [
        domain,
        { key: "from_level", label: "From level", value: (r) => r.from_level ?? "" },
        { key: "to_level", label: "To level", value: (r) => r.to_level ?? "" },
        { key: "from_grade", label: "From grade", value: (r) => r.from_grade ?? "" },
        { key: "to_grade", label: "To grade", value: (r) => r.to_grade ?? "" }
      ];
    }
    // added / removed
    return [
      domain,
      { key: "grade", label: "Grade", value: (r) => r.to_grade ?? r.from_grade ?? "" },
      { key: "worst_level", label: "Worst level", value: (r) => r.worst_level ?? r.to_level ?? r.from_level ?? "" }
    ];
  });

  function exportCSV() {
    if (entries.length === 0) return;
    const tag = data.datasetTag ?? "cohort";
    const name = `${tag}-diff-${data.fromSlug}-${data.toSlug}-${activeTab}.csv`;
    downloadCSV(name, entries, exportColumns);
  }

  const activeTabLabel = $derived(TABS.find((t) => t.key === activeTab)?.label ?? "");
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
    Compare two snapshots of the same cohort. The summary counts how many
    domains regressed or improved; "Added" / "Removed" list membership
    changes.
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
    <button
      type="button"
      class="ghost swap-btn"
      onclick={swapSides}
      disabled={!data.fromSlug || !data.toSlug}
      aria-label="Swap the two snapshots"
      title="Swap From and To"
    >
      ⇄
    </button>
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
  {#if data.fromDefaulted && data.fromSlug}
    <p class="hint">
      Comparing against the previous snapshot (<code>{data.fromSlug}</code>).
      Pick a <strong>From</strong> snapshot to change it.
    </p>
  {/if}
</section>

{#if data.error}
  <section class="card">
    <p class="status-banner error">Failed to load diff: {data.error}</p>
  </section>
{:else if !data.datasetTag}
  <section class="card empty-state">
    <p class="hint">No public cohort is configured - pick one from the filter bar to diff its snapshots.</p>
  </section>
{:else if !data.toSlug}
  <section class="card empty-state">
    <p class="hint">Pick a <strong>To</strong> snapshot above; the previous one is used as <strong>From</strong> automatically.</p>
  </section>
{:else if !data.fromSlug}
  <section class="card empty-state">
    <p class="hint">No earlier snapshot to compare against. Pick an explicit <strong>From</strong> snapshot.</p>
  </section>
{:else if !data.diff}
  <section class="card empty-state">
    <p class="hint">No diff available for the selected snapshots.</p>
  </section>
{:else}
  <section class="card diff-summary" aria-label="Diff summary">
    <ul class="summary-stats">
      <li class="stat tone-error"><span class="stat-count">{summary.regressed}</span><span class="stat-label">Regressed</span></li>
      <li class="stat tone-ok"><span class="stat-count">{summary.improved}</span><span class="stat-label">Improved</span></li>
      <li class="stat tone-notice"><span class="stat-count">{summary.added}</span><span class="stat-label">Added</span></li>
      <li class="stat tone-neutral"><span class="stat-count">{summary.removed}</span><span class="stat-label">Removed</span></li>
    </ul>
  </section>

  {#if matrix.total > 0}
    <section class="card">
      <h3>Grade transitions</h3>
      <p class="hint">
        Where the {matrix.total} grade-changed domains moved: rows are the
        From grade, columns the To grade. Redder cells are regressions,
        greener cells improvements. Unchanged domains are not shown.
      </p>
      <div class="matrix-wrap">
        <table class="matrix">
          <thead>
            <tr>
              <th class="corner"><span class="sr-only">From grade down, To grade across</span></th>
              {#each matrix.grades as g (g)}
                <th scope="col">{g}</th>
              {/each}
            </tr>
          </thead>
          <tbody>
            {#each matrix.grades as fromGrade, fi (fromGrade)}
              <tr>
                <th scope="row">{fromGrade}</th>
                {#each matrix.grades as toGrade, ti (toGrade)}
                  {@const count = matrix.cells[fi][ti]}
                  <td class="mx-cell {cellClass(fi, ti, count)}" class:diagonal={fi === ti}>
                    {#if count > 0}{count}{/if}
                  </td>
                {/each}
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </section>
  {/if}

  <nav class="diff-tabs" aria-label="Diff category">
    {#each TABS as tab (tab.key)}
      <button
        type="button"
        class="diff-tab"
        class:active={activeTab === tab.key}
        onclick={() => setTab(tab.key)}
      >
        {tab.label}
        <span class="diff-tab-count">{countFor(tab.key)}</span>
      </button>
    {/each}
  </nav>

  <section class="card">
    <div class="list-head">
      <h3>{activeTabLabel}</h3>
      <div class="export-group">
        <button type="button" class="ghost" onclick={exportCSV} disabled={entries.length === 0}>
          Export CSV
        </button>
      </div>
    </div>
    {#if entries.length === 0}
      <p class="hint">No entries in this category.</p>
    {:else}
      <div class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th>Domain</th>
              {#if activeTab === "grade_changed"}
                <th>Grade change</th>
                <th>Worst level</th>
              {:else if activeTab === "level_changed"}
                <th>Worst-level change</th>
                <th>Grade</th>
              {:else}
                <th>Grade</th>
                <th>Worst level</th>
              {/if}
            </tr>
          </thead>
          <tbody>
            {#each entries as entry (entry.domain)}
              <tr>
                <td class="row-ident">
                  <a class="cell-link" href={domainLink(entry.domain)}>{entry.domain}</a>
                </td>
                {#if activeTab === "grade_changed"}
                  <td>
                    <span class="pair dir-{gradeDirection(entry)}">
                      <GradeChip grade={entry.from_grade} />
                      <span class="arrow" aria-hidden="true">→</span>
                      <span class="sr-only">to</span>
                      <GradeChip grade={entry.to_grade} />
                    </span>
                  </td>
                  <td>
                    {#if entry.from_level || entry.to_level}
                      <span class="level level-{levelTone(entry.to_level ?? entry.from_level)}">
                        {entry.to_level ?? entry.from_level}
                      </span>
                    {/if}
                  </td>
                {:else if activeTab === "level_changed"}
                  <td>
                    <span class="pair dir-{levelDirection(entry)}">
                      <span class="level level-{levelTone(entry.from_level)}">{entry.from_level ?? "-"}</span>
                      <span class="arrow" aria-hidden="true">→</span>
                      <span class="sr-only">to</span>
                      <span class="level level-{levelTone(entry.to_level)}">{entry.to_level ?? "-"}</span>
                    </span>
                  </td>
                  <td><GradeChip grade={entry.to_grade ?? entry.from_grade} /></td>
                {:else}
                  <td><GradeChip grade={entry.to_grade ?? entry.from_grade} /></td>
                  <td>
                    {#if entry.worst_level ?? entry.to_level ?? entry.from_level}
                      {@const lvl = entry.worst_level ?? entry.to_level ?? entry.from_level}
                      <span class="level level-{levelTone(lvl)}">{lvl}</span>
                    {/if}
                  </td>
                {/if}
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </section>

  {#snippet movedTags(title: string, entries: TagDiffEntry[], showFrom: boolean)}
    <h4>{title}</h4>
    <div class="table-wrap">
      <table class="data-table">
        <thead>
          <tr><th>Tag</th><th>Level</th><th class="col-num">Domains</th></tr>
        </thead>
        <tbody>
          {#each entries as e (e.tag)}
            {@const lvl = showFrom ? e.from_level : e.to_level}
            <tr>
              <td class="row-ident"><TagChip tag={e.tag} /></td>
              <td>{#if lvl}<span class="level level-{levelTone(lvl)}">{lvl}</span>{/if}</td>
              <td class="col-num">{signedCount(e.domain_delta)}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/snippet}

  <section class="card tag-diff" aria-label="Tag changes">
    <h3>Tag changes</h3>
    {#if !data.tagDiff}
      <p class="hint">Tag-level changes are not available for these snapshots yet.</p>
    {:else if tagChangeTotal === 0}
      <p class="hint">No finding tags appeared, cleared, or changed severity between these snapshots.</p>
    {:else}
      <p class="hint">
        Which finding tags moved cohort-wide - this explains the domain
        movement above: a tag that appeared on many domains is a likely
        regression driver.
      </p>
      {#if data.tagDiff.appeared.length > 0}
        {@render movedTags("Appeared", data.tagDiff.appeared, false)}
      {/if}
      {#if data.tagDiff.level_changed.length > 0}
        <h4>Severity changed</h4>
        <div class="table-wrap">
          <table class="data-table">
            <thead>
              <tr><th>Tag</th><th>Severity change</th><th class="col-num">Domains</th></tr>
            </thead>
            <tbody>
              {#each data.tagDiff.level_changed as e (e.tag)}
                <tr>
                  <td class="row-ident"><TagChip tag={e.tag} /></td>
                  <td>
                    <span class="pair">
                      <span class="level level-{levelTone(e.from_level)}">{e.from_level ?? "-"}</span>
                      <span class="arrow" aria-hidden="true">→</span>
                      <span class="sr-only">to</span>
                      <span class="level level-{levelTone(e.to_level)}">{e.to_level ?? "-"}</span>
                    </span>
                  </td>
                  <td class="col-num">{signedCount(e.domain_delta)}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
      {#if data.tagDiff.cleared.length > 0}
        {@render movedTags("Cleared", data.tagDiff.cleared, true)}
      {/if}
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
    align-items: flex-end;
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
  .swap-btn {
    padding: 6px 12px;
    font-size: var(--text-base);
    line-height: 1;
  }

  .summary-stats {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
    gap: var(--space-3);
  }
  .stat {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: var(--space-3) var(--space-4);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
  }
  .stat-count {
    font-family: var(--mono);
    font-size: var(--text-2xl);
    font-weight: 700;
    line-height: 1;
  }
  .stat-label {
    font-size: var(--text-xs);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--ink-2);
  }
  .stat.tone-error .stat-count    { color: var(--bar-error); }
  .stat.tone-ok .stat-count       { color: var(--bar-ok); }
  .stat.tone-notice .stat-count   { color: var(--bar-notice); }
  .stat.tone-neutral .stat-count  { color: var(--ink-2); }

  .matrix-wrap {
    overflow-x: auto;
  }
  .matrix {
    border-collapse: collapse;
    font-size: var(--text-sm);
  }
  .matrix th,
  .matrix td {
    border: 1px solid var(--border);
    text-align: center;
    padding: 6px 10px;
    min-width: 2.5rem;
    font-family: var(--mono);
  }
  .matrix thead th,
  .matrix tbody th {
    color: var(--ink-2);
    font-weight: 600;
    background: var(--surface-2);
  }
  .matrix .corner {
    background: var(--surface);
  }
  .mx-cell {
    color: var(--ink);
  }
  .mx-cell.diagonal {
    background: var(--surface);
  }
  .mx-cell.reg-1 { background: color-mix(in srgb, var(--bar-critical) 15%, transparent); }
  .mx-cell.reg-2 { background: color-mix(in srgb, var(--bar-critical) 35%, transparent); }
  .mx-cell.reg-3 { background: color-mix(in srgb, var(--bar-critical) 55%, transparent); }
  .mx-cell.imp-1 { background: color-mix(in srgb, var(--bar-ok) 15%, transparent); }
  .mx-cell.imp-2 { background: color-mix(in srgb, var(--bar-ok) 35%, transparent); }
  .mx-cell.imp-3 { background: color-mix(in srgb, var(--bar-ok) 55%, transparent); }

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

  .pair {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }
  .arrow {
    color: var(--ink-2);
    font-family: var(--mono);
  }
  .pair.dir-regressed .arrow { color: var(--bar-error); }
  .pair.dir-improved .arrow { color: var(--bar-ok); }

  .tag-diff h4 {
    margin: var(--space-3) 0 var(--space-2);
    font-size: var(--text-sm);
  }
</style>
