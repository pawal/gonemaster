<script lang="ts">
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import FactDistributionBar from "$lib/FactDistributionBar.svelte";
  import FilterBar from "$lib/FilterBar.svelte";
  import { asnHref, nameserverHref, tagHref } from "$lib/entityLinks";
  import { formatCount, formatTimestamp, levelTone } from "$lib/format";
  import type { LayoutData } from "./+layout";
  import type { FactDistribution, OverviewPageData } from "./+page";

  let { data }: { data: OverviewPageData } = $props();

  const layoutData = $derived(page.data as LayoutData);

  const summaryCards = $derived.by(() => {
    const d = data.detail;
    if (!d) return [];
    return [
      { label: "Domains", value: d.domain_count, href: "/domains" },
      { label: "Nameservers", value: d.nameserver_count, href: "/nameservers" },
      { label: "Endpoints", value: d.endpoint_count, href: "/endpoints" },
      { label: "ASNs", value: d.asn_count, href: "/asns" },
      { label: "Prefixes", value: d.prefix_count, href: "/prefixes" }
    ];
  });

  // Ordered lowest → highest severity so the bar reads left to right as
  // "how much is fine" → "how much is broken".
  const SEVERITY_BUCKETS = [
    { key: "OK", label: "OK", tone: "ok" },
    { key: "NOTICE", label: "Notice", tone: "notice" },
    { key: "WARNING", label: "Warning", tone: "warning" },
    { key: "ERROR", label: "Error", tone: "error" },
    { key: "CRITICAL", label: "Critical", tone: "critical" }
  ] as const;

  const factDistributions = $derived.by<FactDistribution[]>(() => {
    const map = data.detail?.fact_distributions;
    if (!map) return [];
    return Object.values(map).sort((a, b) => {
      if (a.order !== b.order) return a.order - b.order;
      return a.category.localeCompare(b.category);
    });
  });

  // DNSKEY algorithm bar is the one category where a domain can legitimately
  // contribute to multiple buckets (dual-algo rollovers, emergency keys).
  // Rendering needs a caption so readers don't interpret the bar as a
  // partition of the cohort.
  const multiBucketCategories = new Set(["dnskey_algo"]);

  const healthSegments = $derived.by(() => {
    const dist = data.detail?.severity_distribution;
    if (!dist) return [];
    const total = Object.values(dist).reduce((sum, n) => sum + (n ?? 0), 0);
    if (total === 0) return [];
    return SEVERITY_BUCKETS.map((b) => {
      const count = dist[b.key] ?? 0;
      return {
        ...b,
        count,
        pct: Math.round((count / total) * 100)
      };
    }).filter((b) => b.count > 0);
  });

  // Scale each top-tag bar against the widest bar, so the leader is 100%
  // wide and the rest are proportional within the top-N.
  const topTagRows = $derived.by(() => {
    const tags = data.topTags ?? [];
    if (tags.length === 0) return [];
    const max = tags.reduce((m, t) => (t.domain_count > m ? t.domain_count : m), 0);
    return tags.map((t) => ({
      tag: t.tag,
      level: t.level ?? "",
      tone: levelTone(t.level),
      count: t.domain_count,
      widthPct: max > 0 ? Math.max(4, Math.round((t.domain_count / max) * 100)) : 0
    }));
  });

  // Same scaling for the infrastructure bars. No severity here, so all
  // bars share one neutral fill; concentration is read from the length.
  const topNameserverRows = $derived.by(() => {
    const items = data.topNameservers ?? [];
    if (items.length === 0) return [];
    const max = items.reduce((m, n) => (n.domain_count > m ? n.domain_count : m), 0);
    return items.map((n) => ({
      name: n.nameserver,
      count: n.domain_count,
      widthPct: max > 0 ? Math.max(4, Math.round((n.domain_count / max) * 100)) : 0
    }));
  });

  const topASNRows = $derived.by(() => {
    const items = data.topASNs ?? [];
    if (items.length === 0) return [];
    const max = items.reduce((m, a) => (a.domain_count > m ? a.domain_count : m), 0);
    return items.map((a) => ({
      asn: a.asn,
      label: a.label ?? "",
      count: a.domain_count,
      widthPct: max > 0 ? Math.max(4, Math.round((a.domain_count / max) * 100)) : 0
    }));
  });

  const query = $derived(page.url.search);
</script>

<FilterBar
  cohorts={layoutData.catalog?.cohorts ?? []}
  selectorEnabled={layoutData.catalog?.selector_enabled ?? false}
  showSearch={false}
/>

{#if layoutData.catalogError}
  <section class="card">
    <h2>Overview</h2>
    <p class="status-banner error">Failed to load catalog: {layoutData.catalogError}</p>
  </section>
{:else if !data.datasetTag}
  <section class="card empty-state">
    <h2>No public cohort is published yet</h2>
    <p class="hint">
      The analysis dashboard only shows cohorts that have been explicitly marked as public.
      A freshly-created cohort defaults to <code>analysis_enabled: true</code>,
      <code>public_enabled: false</code> — it will materialize data in the background but
      stays hidden here until an admin publishes it.
    </p>
    <ol class="hint next-steps">
      <li>Open the admin UI (Settings → Analysis).</li>
      <li>Toggle <strong>Public</strong> on for the cohort you want to expose here.</li>
      <li>Optionally click <strong>Make default</strong> so it becomes the default view.</li>
    </ol>
  </section>
{:else if data.detailError}
  <section class="card">
    <h2>Overview</h2>
    <p class="status-banner error">Failed to load cohort: {data.detailError}</p>
  </section>
{:else if data.detail}
  {@const d = data.detail}
  {@const isEmpty = (d.domain_count ?? 0) === 0}
  <section class="card overview-header">
    <h2>{d.label}</h2>
    {#if d.description}
      <p class="hint">{d.description}</p>
    {/if}
    {#if formatTimestamp(d.last_materialized_at)}
      <p class="hint">Last analyzed: {formatTimestamp(d.last_materialized_at)}</p>
    {/if}
  </section>

  {#if isEmpty}
    <section class="card empty-state">
      <h3>No data has been materialized yet</h3>
      <p class="hint">
        This cohort is published but the projector hasn't seen any matching runs yet.
        Run a job (or batch) tagged with <code>{d.dataset_tag}</code>, or trigger
        <strong>Rebuild</strong> from the admin UI to project any existing runs that
        already carry this tag.
      </p>
    </section>
  {:else}
    {#if healthSegments.length > 0}
      <section class="card health-bar-section">
        <h3>Domain health</h3>
        <p class="hint">
          Each domain counted by the worst severity in its latest run.
        </p>
        <div class="health-bar" role="list" aria-label="Severity distribution">
          {#each healthSegments as seg (seg.key)}
            <div
              class="health-bar-segment tone-{seg.tone}"
              role="listitem"
              style:flex-grow={seg.count}
              title="{seg.label}: {formatCount(seg.count)} domains ({seg.pct}%)"
            >
              <span class="health-bar-label">{seg.label}</span>
              <span class="health-bar-count">{formatCount(seg.count)}</span>
            </div>
          {/each}
        </div>
      </section>
    {/if}
    {#each factDistributions as dist (dist.category)}
      <FactDistributionBar
        title={dist.label}
        description={dist.description}
        buckets={dist.buckets}
        multiPerDomain={multiBucketCategories.has(dist.category)}
      />
    {/each}
    {#if topTagRows.length > 0}
      <section class="card top-tags-section">
        <h3>Top issues</h3>
        <p class="hint">
          Most common finding tags at WARNING level or worse, ranked by
          how many domains they affect. NOTICE-level tags are excluded so
          universal "zone exists" chatter doesn't crowd out the
          actionable findings.
        </p>
        <ol class="top-tags-list">
          {#each topTagRows as row (row.tag)}
            <li>
              <a class="top-tag-row" href={tagHref(base, row.tag, query)}>
                <span class="top-tag-name">{row.tag}</span>
                <span class="top-tag-bar-track" aria-hidden="true">
                  <span
                    class="top-tag-bar tone-{row.tone}"
                    style:width="{row.widthPct}%"
                  ></span>
                </span>
                <span class="top-tag-count">{formatCount(row.count)}</span>
                {#if row.level}
                  <span class="level level-{row.tone}">{row.level}</span>
                {:else}
                  <span class="level level-neutral">-</span>
                {/if}
              </a>
            </li>
          {/each}
        </ol>
      </section>
    {:else if data.topTagsError}
      <section class="card">
        <p class="status-banner error">Failed to load top tags: {data.topTagsError}</p>
      </section>
    {/if}
    {#if topNameserverRows.length > 0 || topASNRows.length > 0}
      <div class="infra-grid">
        {#if topNameserverRows.length > 0}
          <section class="card infra-card">
            <h3>Top nameservers</h3>
            <p class="hint">
              Nameservers hosting the most domains in this cohort.
              Showing top {topNameserverRows.length} of {formatCount(data.topNameserversTotal)}.
            </p>
            <ol class="infra-list">
              {#each topNameserverRows as row (row.name)}
                <li>
                  <a class="infra-row" href={nameserverHref(base, row.name, query)}>
                    <span class="infra-name">{row.name}</span>
                    <span class="infra-bar-track" aria-hidden="true">
                      <span class="infra-bar" style:width="{row.widthPct}%"></span>
                    </span>
                    <span class="infra-count">{formatCount(row.count)}</span>
                  </a>
                </li>
              {/each}
            </ol>
          </section>
        {/if}
        {#if topASNRows.length > 0}
          <section class="card infra-card">
            <h3>Top ASNs</h3>
            <p class="hint">
              ASNs hosting the most domains in this cohort. Showing top
              {topASNRows.length} of {formatCount(data.topASNsTotal)}.
            </p>
            <ol class="infra-list">
              {#each topASNRows as row (row.asn)}
                <li>
                  <a class="infra-row" href={asnHref(base, row.asn, query)}>
                    <span class="infra-name">
                      <span class="infra-asn">AS{row.asn}</span>
                      {#if row.label}
                        <span class="infra-asn-label">{row.label}</span>
                      {/if}
                    </span>
                    <span class="infra-bar-track" aria-hidden="true">
                      <span class="infra-bar" style:width="{row.widthPct}%"></span>
                    </span>
                    <span class="infra-count">{formatCount(row.count)}</span>
                  </a>
                </li>
              {/each}
            </ol>
          </section>
        {/if}
      </div>
    {:else if data.topNameserversError || data.topASNsError}
      <section class="card">
        {#if data.topNameserversError}
          <p class="status-banner error">Failed to load top nameservers: {data.topNameserversError}</p>
        {/if}
        {#if data.topASNsError}
          <p class="status-banner error">Failed to load top ASNs: {data.topASNsError}</p>
        {/if}
      </section>
    {/if}
    <section class="summary-grid" aria-label="Cohort summary counts">
      {#each summaryCards as card (card.label)}
        <a class="summary-card" href={`${base}${card.href}${query}`}>
          <span class="summary-count">{formatCount(card.value)}</span>
          <span class="summary-label">{card.label}</span>
        </a>
      {/each}
    </section>
  {/if}
{/if}

<style>
  .overview-header {
    gap: var(--space-2);
  }
  .overview-header h2 {
    margin: 0;
  }
  .health-bar-section {
    gap: var(--space-2);
  }
  .health-bar-section h3 {
    margin: 0;
  }
  .health-bar {
    display: flex;
    width: 100%;
    min-height: 36px;
    border-radius: var(--radius);
    overflow: hidden;
    border: 1px solid var(--border);
  }
  .health-bar-segment {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
    padding: 0 var(--space-3);
    min-width: 3rem;
    font-size: var(--text-xs);
    white-space: nowrap;
    overflow: hidden;
  }
  .health-bar-label {
    text-transform: uppercase;
    letter-spacing: 0.04em;
    font-weight: 600;
  }
  .health-bar-count {
    font-family: var(--mono);
  }
  .tone-ok { background: #dcfce7; color: #166534; }
  .tone-notice { background: #e0f2fe; color: #075985; }
  .tone-warning { background: #fef3c7; color: #92400e; }
  .tone-error { background: #ffedd5; color: #9a3412; }
  .tone-critical { background: #fee2e2; color: #991b1b; }
  .tone-neutral { background: var(--surface-2); color: var(--on-surface-2); }

  .top-tags-section {
    gap: var(--space-2);
  }
  .top-tags-section h3 {
    margin: 0;
  }
  /* Single grid at the list level so all rows share the same column widths;
     otherwise each row's tag-name column sizes to its own content and the
     bar tracks end up at slightly different lengths. display:contents on
     the <li> keeps the list semantics while letting <a> participate
     directly in the parent grid via subgrid. */
  .top-tags-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    grid-template-columns:
      minmax(10rem, 18rem)
      1fr
      minmax(4rem, auto)
      minmax(4.5rem, auto);
    column-gap: var(--space-3);
    row-gap: 6px;
  }
  .top-tags-list > li {
    display: contents;
  }
  .top-tag-row {
    display: grid;
    grid-template-columns: subgrid;
    grid-column: 1 / -1;
    align-items: center;
    padding: 6px var(--space-3);
    border-radius: var(--radius);
    text-decoration: none;
    color: var(--ink);
    border: 1px solid transparent;
  }
  .top-tag-row:hover {
    border-color: var(--accent-2);
    background: var(--surface-2);
  }
  .top-tag-name {
    font-family: var(--mono);
    font-size: var(--text-sm);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .top-tag-bar-track {
    display: block;
    width: 100%;
    height: 10px;
    background: var(--surface-2);
    border-radius: 5px;
    overflow: hidden;
  }
  .top-tag-bar {
    display: block;
    height: 100%;
    border-radius: 5px;
  }
  /* Solid bars need their own fills; the pastel .tone-* used by the health
     bar's labelled segments doesn't provide enough contrast against the
     white card in light mode. --bar-* is defined per theme in app.css. */
  .top-tag-bar.tone-ok { background: var(--bar-ok); }
  .top-tag-bar.tone-notice { background: var(--bar-notice); }
  .top-tag-bar.tone-warning { background: var(--bar-warning); }
  .top-tag-bar.tone-error { background: var(--bar-error); }
  .top-tag-bar.tone-critical { background: var(--bar-critical); }
  .top-tag-bar.tone-neutral { background: var(--bar-neutral); }
  .top-tag-count {
    font-family: var(--mono);
    font-size: var(--text-sm);
    text-align: right;
  }
  .level {
    display: inline-block;
    padding: 1px 8px;
    border-radius: 999px;
    font-size: var(--text-xs);
    font-weight: 600;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    text-align: center;
  }
  .level-critical { background: #fee2e2; color: #991b1b; }
  .level-error { background: #ffedd5; color: #9a3412; }
  .level-warning { background: #fef3c7; color: #92400e; }
  .level-notice { background: #e0f2fe; color: #075985; }
  .level-neutral { background: var(--surface-2); color: var(--on-surface-2); }

  .infra-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(24rem, 1fr));
    gap: var(--space-3);
  }
  .infra-card {
    gap: var(--space-2);
  }
  .infra-card h3 {
    margin: 0;
  }
  .infra-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    grid-template-columns:
      minmax(8rem, 14rem)
      1fr
      minmax(4rem, auto);
    column-gap: var(--space-3);
    row-gap: 6px;
  }
  .infra-list > li {
    display: contents;
  }
  .infra-row {
    display: grid;
    grid-template-columns: subgrid;
    grid-column: 1 / -1;
    align-items: center;
    padding: 6px var(--space-3);
    border-radius: var(--radius);
    text-decoration: none;
    color: var(--ink);
    border: 1px solid transparent;
  }
  .infra-row:hover {
    border-color: var(--accent-2);
    background: var(--surface-2);
  }
  .infra-name {
    font-family: var(--mono);
    font-size: var(--text-sm);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    display: flex;
    gap: 6px;
    align-items: baseline;
  }
  .infra-asn {
    font-weight: 600;
  }
  .infra-asn-label {
    color: var(--ink-2);
    font-family: var(--sans);
    font-size: var(--text-xs);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .infra-bar-track {
    display: block;
    width: 100%;
    height: 10px;
    background: var(--surface-2);
    border-radius: 5px;
    overflow: hidden;
  }
  .infra-bar {
    display: block;
    height: 100%;
    border-radius: 5px;
    background: var(--accent-2);
  }
  .infra-count {
    font-family: var(--mono);
    font-size: var(--text-sm);
    text-align: right;
  }

  .summary-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
    gap: var(--space-3);
  }

  .summary-card {
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding: var(--space-4) var(--space-5);
    background: var(--card);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    text-decoration: none;
    color: var(--ink);
    transition: border-color 0.15s ease, transform 0.15s ease;
  }
  .summary-card:hover {
    border-color: var(--accent-2);
    transform: translateY(-1px);
  }
  .summary-count {
    font-size: var(--text-2xl);
    font-weight: 700;
    font-family: var(--mono);
    color: var(--ink);
    line-height: 1.1;
  }
  .summary-label {
    font-size: var(--text-sm);
    color: var(--ink-2);
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }

  .empty-state h2,
  .empty-state h3 {
    margin: 0;
    color: var(--ink);
  }

  .empty-state code {
    font-family: var(--mono);
    font-size: var(--text-sm);
    background: var(--surface-2);
    color: var(--on-surface-2);
    padding: 1px 6px;
    border-radius: 4px;
  }

  .next-steps {
    padding-left: var(--space-5);
    margin: var(--space-2) 0 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
</style>
