<script lang="ts">
  import { goto } from "$app/navigation";
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import FilterBar from "$lib/FilterBar.svelte";
  import GradeChip from "$lib/GradeChip.svelte";
  import TagChip from "$lib/chips/TagChip.svelte";
  import { domainHref, scopedQuery } from "$lib/entityLinks";
  import { snapshotOptionLabel, levelTone, formatCount } from "$lib/format";
  import { downloadBlob, downloadCSV, type ExportColumn } from "$lib/exporters";
  import {
    buildGradeMatrix,
    gradeDirection,
    levelDirection,
    sortByMovement,
    summarizeDiff
  } from "$lib/diff";
  import {
    CATEGORY_LABELS,
    CATEGORY_ORDER,
    CLASSIFICATION_LABELS,
    categoryTone,
    clusterDimensionSummary,
    filterDomainsByCategory,
    reportFilename,
    reportToMarkdown,
    signedNumber,
    splitTagsByClassification
  } from "$lib/report";
  import type { DiffEntry, DomainCategory, ReportTagEntry, TagDiffEntry } from "$lib/api";
  import type { LayoutData } from "../+layout";
  import type { DiffPageData } from "./+page";

  let { data }: { data: DiffPageData } = $props();

  const layoutData = $derived(page.data as LayoutData);

  type TabKey = "movers" | "added" | "removed" | "grade_changed" | "level_changed";
  const TABS: { key: TabKey; label: string }[] = [
    { key: "movers", label: "Movers" },
    { key: "added", label: "Added" },
    { key: "removed", label: "Removed" },
    { key: "grade_changed", label: "Grade changed" },
    { key: "level_changed", label: "Worst-level changed" }
  ];
  // The movers tab needs the report; without it the page keeps the four
  // membership and movement tabs it always had.
  const tabs = $derived(TABS.filter((t) => t.key !== "movers" || !!data.report));

  // Tab lives in the URL so a shared link lands on the same list. Read it from
  // page state (not the loader) so switching tabs never refetches the diff.
  const activeTab = $derived.by<TabKey>(() => {
    const raw = page.url.searchParams.get("tab") ?? "";
    const known = tabs.map((t) => t.key as string);
    if (known.includes(raw)) return raw as TabKey;
    return data.report ? "movers" : "added";
  });

  // Category filter for the movers tab, also in the URL.
  const categoryFilter = $derived.by<DomainCategory | "">(() => {
    const raw = page.url.searchParams.get("cause") ?? "";
    return (CATEGORY_ORDER as string[]).includes(raw) ? (raw as DomainCategory) : "";
  });

  function setParam(name: string, value: string) {
    const params = new URLSearchParams(page.url.searchParams);
    if (value) params.set(name, value);
    else params.delete(name);
    goto(`${page.url.pathname}?${params.toString()}`, { replaceState: true, noScroll: true });
  }

  function setTab(key: TabKey) {
    setParam("tab", key);
  }

  function setSide(side: "from" | "to", slug: string) {
    setParam(side, slug);
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

  // Either granularity carries the same provenance header; the domain diff
  // is the one that is always loaded, so prefer it.
  const engine = $derived(data.diff?.engine ?? data.tagDiff?.engine);
  const header = $derived(data.report?.header);
  const vocabulary = $derived(header?.vocabulary);
  const vocabularyKnown = $derived(
    !!vocabulary && vocabulary.from_available && vocabulary.to_available
  );

  const summary = $derived(summarizeDiff(data.diff));
  const matrix = $derived(buildGradeMatrix(data.diff?.grade_changed ?? []));

  // Tag rows come from the report when it loaded, so each row carries a
  // classification; otherwise from the plain tag diff.
  const tagSource = $derived(data.report?.tags ?? data.tagDiff);
  const tagChangeTotal = $derived(
    tagSource
      ? tagSource.appeared.length + tagSource.cleared.length + tagSource.level_changed.length
      : 0
  );

  function classified(entries: (TagDiffEntry | ReportTagEntry)[] | undefined) {
    return splitTagsByClassification(entries as ReportTagEntry[]);
  }
  const appearedTags = $derived(classified(tagSource?.appeared));
  const clearedTags = $derived(classified(tagSource?.cleared));
  const levelChangedTags = $derived(classified(tagSource?.level_changed));

  const categoryCounts = $derived(data.report?.totals.domain_categories ?? {});
  const movers = $derived(filterDomainsByCategory(data.report?.domains, categoryFilter));
  // The server pages the movers; the other sections still cover them all.
  const partialMovers = $derived(
    !!data.report && data.report.domain_total > data.report.domains.length
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
      default:
        return [];
    }
  });

  function countFor(key: TabKey): number {
    if (key === "movers") return data.report?.domain_total ?? 0;
    if (!data.diff) return 0;
    return (data.diff[key] ?? []).length;
  }

  // Open the domain detail in the "to" snapshot's context, keeping the cohort
  // scope so the page resolves the right materialized rows.
  function domainLink(domain: string): string {
    return domainHref(base, domain, scopedQuery(data.datasetTag, data.toSlug));
  }

  // Tag detail is snapshot-scoped: a cleared tag exists only in "from",
  // appeared/changed tags in "to". Pin the side where its view row exists.
  function tagLinkQuery(side: "from" | "to"): string {
    return scopedQuery(data.datasetTag, side === "from" ? data.fromSlug : data.toSlug);
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

  function exportMarkdown() {
    if (!data.report) return;
    downloadBlob(reportFilename(data.report), "text/markdown", reportToMarkdown(data.report));
  }

  const activeTabLabel = $derived(tabs.find((t) => t.key === activeTab)?.label ?? "");
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
  {#if header}
    <div class="provenance" aria-label="Measurement provenance">
      <p>
        Engine <code>{header.from.engine_version || "unknown"}</code> →
        <code>{header.to.engine_version || "unknown"}</code>,
        profile <code>{header.from.profile_name || "default"}</code> →
        <code>{header.to.profile_name || "default"}</code>.
      </p>
      {#if vocabularyKnown && vocabulary}
        <p>
          Tag vocabulary {vocabulary.from_tag_count} → {vocabulary.to_tag_count}:
          {vocabulary.added.length} added, {vocabulary.removed.length} removed,
          {vocabulary.level_changed.length} reclassified. Findings the engine
          gained or dropped are marked below, so a wider vocabulary does not
          read as a wave of regressions.
        </p>
      {:else}
        <p class="warn">
          The tag vocabulary is unknown on at least one side, so no change can
          be attributed to the engine or to the cohort.
        </p>
      {/if}
      <p class:warn={header.scoring_config_changed !== "false"}>
        {#if header.scoring_config_changed === "true"}
          The scoring configuration changed between these snapshots, so a
          score move need not come from the findings.
        {:else if header.scoring_config_changed === "false"}
          Scoring configuration unchanged.
        {:else}
          Scoring configuration provenance is unknown, so a score move cannot
          be fully attributed.
        {/if}
      </p>
      {#if header.tag_floor}
        <p class="hint">
          Findings below {header.tag_floor} are not stored per domain, so the
          per-domain lists do not cover them.
        </p>
      {/if}
    </div>
  {:else if engine?.crossed_engine_versions}
    <p class="status-banner warn engine-banner">
      These snapshots ran different engine versions
      (<code>{engine.from_engine_version}</code> → <code>{engine.to_engine_version}</code>).
      Findings that appear or clear across this step may be new engine
      capability rather than a change in the cohort.
    </p>
  {:else if engine?.engine_version_unknown}
    <p class="status-banner engine-banner">
      Engine provenance is unknown for at least one of these snapshots, so
      changes here cannot be attributed to the cohort or to the engine.
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

  {#if data.report}
    {@const totals = data.report.totals}
    <section class="card report-card" aria-label="Report">
      <div class="list-head">
        <h3>Report</h3>
        <button type="button" class="ghost" onclick={exportMarkdown}>Export Markdown</button>
      </div>
      <p class="hint">
        {totals.both_domain_count} domains on both sides:
        {totals.identical_score} scored identically, {totals.improved} improved,
        {totals.regressed} regressed. Mean score
        {totals.from_mean_score ?? "-"} → {totals.to_mean_score ?? "-"}.
      </p>
      <ul class="summary-stats cause-stats">
        {#each CATEGORY_ORDER as cause (cause)}
          <li class="stat tone-{categoryTone(cause)}">
            <span class="stat-count">{categoryCounts[cause] ?? 0}</span>
            <span class="stat-label">{CATEGORY_LABELS[cause]}</span>
          </li>
        {/each}
      </ul>
      <p class="hint">
        "Measurement" means every finding that moved on the domain is one the
        engine gained, dropped or reclassified. "Real" means the cohort moved.
      </p>

      <h4>Clusters</h4>
      {#if data.report.clusters.length === 0}
        <p class="hint">
          No group of {data.report.min_cluster} or more domains moved together
          within {data.report.max_spread} points.
        </p>
      {:else}
        <p class="hint">
          Domains that moved together and share a nameserver, an AS, a prefix
          or a software version. The grouping is the fact; the cause is not.
        </p>
        <div class="table-wrap">
          <table class="data-table">
            <thead>
              <tr><th>Shared</th><th class="col-num">Domains</th><th>Score move</th><th>Members</th></tr>
            </thead>
            <tbody>
              {#each data.report.clusters as cluster (cluster.domains.join(","))}
                <tr>
                  <td>
                    <ul class="dim-list">
                      {#each cluster.dimensions as dim (dim.dimension + dim.value)}
                        <li>
                          <span class="dim-name">{dim.dimension}</span>
                          <span class="dim-value">{dim.label || dim.value}</span>
                          <span class="dim-total">{cluster.size} of {dim.total_domains}</span>
                        </li>
                      {/each}
                    </ul>
                  </td>
                  <td class="col-num">{cluster.size}</td>
                  <td class="dir-{cluster.direction}">
                    {signedNumber(cluster.min_delta)} to {signedNumber(cluster.max_delta)}
                  </td>
                  <td>
                    <ul class="member-list">
                      {#each cluster.domains as domain (domain)}
                        <li><a class="cell-link" href={domainLink(domain)}>{domain}</a></li>
                      {/each}
                    </ul>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </section>
  {/if}

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
    {#each tabs as tab (tab.key)}
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

  {#if activeTab === "movers" && data.report}
    <section class="card" aria-label="Movers">
      <div class="list-head">
        <h3>Movers</h3>
        <label class="cause-filter">
          <span>Cause</span>
          <select value={categoryFilter} onchange={(e) => setParam("cause", e.currentTarget.value)}>
            <option value="">All</option>
            {#each CATEGORY_ORDER as cause (cause)}
              <option value={cause}>{CATEGORY_LABELS[cause]}</option>
            {/each}
          </select>
        </label>
      </div>
      {#if movers.length === 0}
        <p class="hint">No domain moved in this category.</p>
      {:else}
        <div class="table-wrap">
          <table class="data-table">
            <thead>
              <tr>
                <th>Domain</th>
                <th class="col-num">Score</th>
                <th class="col-num">Delta</th>
                <th class="col-num">Explained</th>
                <th>Grade</th>
                <th>Cause</th>
                <th>Findings that moved</th>
              </tr>
            </thead>
            <tbody>
              {#each movers as row (row.domain)}
                <tr>
                  <td class="row-ident">
                    <a class="cell-link" href={domainLink(row.domain)}>{row.domain}</a>
                  </td>
                  <td class="col-num">{row.from_score ?? "-"} → {row.to_score ?? "-"}</td>
                  <td class="col-num delta-{row.score_delta && row.score_delta > 0 ? 'up' : 'down'}">
                    {signedNumber(row.score_delta)}
                  </td>
                  <td class="col-num">{signedNumber(row.explained_delta)}</td>
                  <td>
                    {#if row.grade_changed}
                      <span class="pair">
                        <GradeChip grade={row.from_grade} />
                        <span class="arrow" aria-hidden="true">→</span>
                        <span class="sr-only">to</span>
                        <GradeChip grade={row.to_grade} />
                      </span>
                    {:else}
                      <GradeChip grade={row.to_grade} />
                    {/if}
                  </td>
                  <td><span class="cause cause-{categoryTone(row.category)}">{CATEGORY_LABELS[row.category]}</span></td>
                  <td>
                    <ul class="move-list">
                      {#each row.appeared as m (m.tag)}
                        <li>
                          <span class="move-sign">+</span>
                          <TagChip tag={m.tag} query={tagLinkQuery("to")} />
                          <span class="class-note">{CLASSIFICATION_LABELS[m.classification]}</span>
                        </li>
                      {/each}
                      {#each row.cleared as m (m.tag)}
                        <li>
                          <span class="move-sign">-</span>
                          <TagChip tag={m.tag} query={tagLinkQuery("from")} />
                          <span class="class-note">{CLASSIFICATION_LABELS[m.classification]}</span>
                        </li>
                      {/each}
                      {#each row.level_changed as m (m.tag)}
                        <li>
                          <span class="move-sign">±</span>
                          <TagChip tag={m.tag} query={tagLinkQuery("to")} />
                          <span class="class-note">
                            {m.from_level} → {m.to_level}, {CLASSIFICATION_LABELS[m.classification]}
                          </span>
                        </li>
                      {/each}
                    </ul>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
        {#if partialMovers}
          <p class="hint">
            Showing {data.report.domains.length} of {data.report.domain_total} movers. The counts,
            tag tables and clusters cover every one.
          </p>
        {/if}
      {/if}
    </section>
  {:else}
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
  {/if}

  {#snippet tagRows(rows: ReportTagEntry[], showFrom: boolean, withClass: boolean)}
    <div class="table-wrap">
      <table class="data-table">
        <thead>
          <tr>
            <th>Tag</th>
            <th>Level</th>
            <th class="col-num">Domains</th>
            {#if withClass}<th>Classification</th>{/if}
          </tr>
        </thead>
        <tbody>
          {#each rows as e (e.tag)}
            {@const lvl = showFrom ? e.from_level : e.to_level}
            <tr>
              <td class="row-ident"><TagChip tag={e.tag} query={tagLinkQuery(showFrom ? "from" : "to")} /></td>
              <td>{#if lvl}<span class="level level-{levelTone(lvl)}">{lvl}</span>{/if}</td>
              <td class="col-num">{signedCount(e.domain_delta)}</td>
              {#if withClass}
                <td><span class="class-note">{CLASSIFICATION_LABELS[e.classification]}</span></td>
              {/if}
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/snippet}

  {#snippet movedTags(title: string, split: ReturnType<typeof splitTagsByClassification>, showFrom: boolean)}
    {#if split.cohort.length + split.engine.length + split.unknown.length > 0}
      <h4>{title}</h4>
      {#if data.report}
        {#if split.cohort.length > 0}
          {@render tagRows(split.cohort, showFrom, false)}
        {:else}
          <p class="hint">Nothing here is a cohort change.</p>
        {/if}
        {#if split.engine.length > 0}
          <details class="engine-rows">
            <summary>{split.engine.length} explained by the engine's tag vocabulary</summary>
            {@render tagRows(split.engine, showFrom, true)}
          </details>
        {/if}
        {#if split.unknown.length > 0}
          <details class="engine-rows">
            <summary>{split.unknown.length} unattributable</summary>
            {@render tagRows(split.unknown, showFrom, true)}
          </details>
        {/if}
      {:else}
        {@render tagRows([...split.cohort, ...split.engine, ...split.unknown], showFrom, false)}
      {/if}
    {/if}
  {/snippet}

  <section class="card tag-diff" aria-label="Tag changes">
    <h3>Tag changes</h3>
    {#if !tagSource}
      <p class="hint">Tag-level changes are not available for these snapshots yet.</p>
    {:else if tagChangeTotal === 0}
      <p class="hint">No finding tags appeared, cleared, or changed severity between these snapshots.</p>
    {:else}
      <p class="hint">
        Which finding tags moved cohort-wide - this explains the domain
        movement above: a tag that appeared on many domains is a likely
        regression driver.
      </p>
      {@render movedTags("Appeared", appearedTags, false)}
      {@render movedTags("Severity changed", levelChangedTags, false)}
      {@render movedTags("Cleared", clearedTags, true)}
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
  .diff-side-select label,
  .cause-filter {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: var(--text-xs);
    color: var(--ink-2);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .diff-side-select select,
  .cause-filter select {
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
  .cause-filter select {
    min-width: 140px;
  }
  .swap-btn {
    padding: 6px 12px;
    font-size: var(--text-base);
    line-height: 1;
  }

  .provenance {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-3) var(--space-4);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface-2);
    font-size: var(--text-sm);
  }
  .provenance p {
    margin: 0;
  }
  .provenance .warn {
    color: var(--bar-warning);
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
  .stat.tone-warning .stat-count  { color: var(--bar-warning); }
  .stat.tone-neutral .stat-count  { color: var(--ink-2); }

  .report-card h4 {
    margin: var(--space-3) 0 var(--space-2);
    font-size: var(--text-sm);
  }
  .dim-list,
  .member-list,
  .move-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
    font-size: var(--text-sm);
  }
  .member-list {
    flex-direction: row;
    flex-wrap: wrap;
    gap: 2px var(--space-2);
  }
  .dim-name {
    font-family: var(--mono);
    font-size: var(--text-xs);
    color: var(--ink-2);
  }
  .dim-total {
    color: var(--ink-2);
    font-size: var(--text-xs);
  }
  .move-list li {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .move-sign {
    font-family: var(--mono);
    color: var(--ink-2);
  }
  .class-note {
    font-size: var(--text-xs);
    color: var(--ink-2);
  }
  .cause {
    display: inline-block;
    padding: 1px 8px;
    border-radius: 999px;
    font-size: var(--text-xs);
    border: 1px solid var(--border);
  }
  .cause-error   { color: var(--bar-error); }
  .cause-warning { color: var(--bar-warning); }
  .cause-notice  { color: var(--bar-notice); }
  .cause-neutral { color: var(--ink-2); }
  .delta-up   { color: var(--bar-ok); }
  .delta-down { color: var(--bar-error); }
  .dir-improved  { color: var(--bar-ok); }
  .dir-regressed { color: var(--bar-error); }
  .engine-rows {
    margin: var(--space-2) 0;
  }
  .engine-rows summary {
    cursor: pointer;
    font-size: var(--text-sm);
    color: var(--ink-2);
  }

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

  /* Printing the page yields the report: controls go, disclosures open. */
  @media print {
    .diff-side-select,
    .diff-tabs,
    .cause-filter,
    .swap-btn {
      display: none;
    }
    .engine-rows {
      display: block;
    }
    .table-wrap {
      overflow: visible;
    }
  }
</style>
