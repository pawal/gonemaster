<script lang="ts">
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import FactDistributionBar from "$lib/FactDistributionBar.svelte";
  import FilterBar from "$lib/FilterBar.svelte";
  import GradeChip from "$lib/GradeChip.svelte";
  import Sparkline from "$lib/charts/Sparkline.svelte";
  import {
    asnHref,
    domainHref,
    domainsGradeHref,
    domainsSeverityHref,
    nameserverHref,
    tagHref
  } from "$lib/entityLinks";
  import {
    formatCount,
    formatTimestamp,
    levelTone,
    snapshotDisplayLabel,
    snapshotSourceDate
  } from "$lib/format";
  import { netDirection, sortByMovement, summarizeDiff } from "$lib/diff";
  import {
    percentOf,
    seriesShareByKeys,
    seriesShareByTone,
    seriesShareExcludingKeys,
    seriesTotals,
    summarizeMetric
  } from "$lib/overview";
  import { idnTooltip } from "$lib/idn";
  import type { LayoutData } from "./+layout";
  import type { OverviewPageData } from "./+page";

  let { data }: { data: OverviewPageData } = $props();

  const layoutData = $derived(page.data as LayoutData);

  // ── Hero metrics: latest value + movement + sparkline per tile. ──────────
  const domainsMetric = $derived(summarizeMetric(seriesTotals(data.severityTrend.points)));
  const healthyMetric = $derived(
    summarizeMetric(seriesShareByTone(data.severityTrend.points, data.severityTrend.keyMeta, ["ok"]))
  );
  const topGradeMetric = $derived(
    summarizeMetric(seriesShareByKeys(data.gradeTrend.points, ["A+", "A"]))
  );
  // Signed = every DNSSEC posture except the unsigned bucket.
  const signedMetric = $derived(
    summarizeMetric(seriesShareExcludingKeys(data.dnssecTrend.points, ["unsigned"]))
  );

  // Single-provider concentration: the leading nameserver / ASN's share of
  // domains. Exact (one entity's domain count over the total), unlike a
  // top-N sum which would double-count multi-homed domains.
  const nsLeaderShare = $derived(
    data.topNameservers.length && data.totals
      ? percentOf(data.topNameservers[0].domain_count, data.totals.domain_count)
      : 0
  );
  const asnLeaderShare = $derived(
    data.topASNs.length && data.totals
      ? percentOf(data.topASNs[0].domain_count, data.totals.domain_count)
      : 0
  );

  // Domain count prefers the live trend, falling back to the overview total
  // when a cohort has no trend series yet.
  const domainCount = $derived(domainsMetric.latest ?? data.totals?.domain_count ?? 0);

  type DeltaTone = "ok" | "error" | "neutral";
  function deltaTone(delta: number | null, higherIsBetter: boolean | null): DeltaTone {
    if (delta === null || delta === 0 || higherIsBetter === null) return "neutral";
    return delta > 0 === higherIsBetter ? "ok" : "error";
  }
  function deltaText(delta: number | null, suffix: string): string {
    if (delta === null || delta === 0) return "";
    return `${delta > 0 ? "▲ +" : "▼ "}${formatCount(delta)}${suffix}`;
  }

  // ── "Since last snapshot" movers, derived from the diff-vs-previous. ──────
  const diffSummary = $derived(summarizeDiff(data.diff));
  const gradeChanges = $derived(sortByMovement(data.diff?.grade_changed ?? [], "grade"));
  const regressions = $derived(
    gradeChanges.filter((e) => netDirection(e) === "regressed").slice(0, 5)
  );
  const improvements = $derived(
    gradeChanges.filter((e) => netDirection(e) === "improved").reverse().slice(0, 5)
  );
  const hasMovement = $derived(
    !!data.diff &&
      (diffSummary.regressed > 0 ||
        diffSummary.improved > 0 ||
        diffSummary.added > 0 ||
        diffSummary.removed > 0)
  );

  function moverDomainLink(domain: string): string {
    const params = new URLSearchParams();
    if (data.datasetTag) params.set("dataset_tag", data.datasetTag);
    if (data.diffTo) params.set("snapshot", data.diffTo);
    const q = params.toString();
    return domainHref(base, domain, q ? `?${q}` : "");
  }

  const fullDiffHref = $derived.by(() => {
    const params = new URLSearchParams();
    if (data.datasetTag) params.set("dataset_tag", data.datasetTag);
    if (data.diffFrom) params.set("from", data.diffFrom);
    if (data.diffTo) params.set("to", data.diffTo);
    params.set("tab", "grade_changed");
    return `${base}/diff?${params.toString()}`;
  });

  const summaryCards = $derived.by<{ label: string; value: number; href: string | null }[]>(() => {
    const t = data.totals;
    if (!t) return [];
    // Domains lead the hero tiles, so the entity row covers the rest.
    // Prefixes have no list view (folded into the Addresses tab), so the
    // count shows without a link rather than pointing at a dead route.
    return [
      { label: "Nameservers", value: t.nameserver_count, href: "/nameservers" },
      { label: "Endpoints", value: t.endpoint_count, href: "/endpoints" },
      { label: "ASNs", value: t.asn_count, href: "/asns" },
      { label: "Prefixes", value: t.prefix_count, href: null }
    ];
  });

  const factDistributions = $derived.by(() => {
    const map = data.factDistributions;
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

  // Categories whose segments deep-link into a filtered domains list. Other
  // categories render as informational bars (hrefForKey omitted entirely).
  function buildHrefForKey(category: string): ((key: string) => string) | undefined {
    if (category === "severity") return (k) => domainsSeverityHref(base, k, query);
    if (category === "grade") return (k) => domainsGradeHref(base, k, query);
    return undefined;
  }

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
  const isEmpty = $derived((data.totals?.domain_count ?? 0) === 0);
</script>

<FilterBar
  cohorts={layoutData.catalog?.cohorts ?? []}
  selectorEnabled={layoutData.catalog?.selector_enabled ?? false}
  snapshots={layoutData.snapshots ?? []}
  defaultSnapshotSlug={layoutData.defaultSnapshotSlug ?? ""}
  showSearch={false}
/>

{#if data.snapshot}
  <p class="snapshot-pill" aria-label="Active snapshot">
    Snapshot:
    <span class="snapshot-slug" title={`Snapshot ${data.snapshot.slug}`}>
      {snapshotDisplayLabel(data.snapshot)}
    </span>
    {#if data.snapshot.label && snapshotSourceDate(data.snapshot)}
      <span class="snapshot-label">({snapshotSourceDate(data.snapshot)})</span>
    {/if}
    {#if formatTimestamp(data.snapshot.captured_at)}
      <span class="snapshot-captured">Built {formatTimestamp(data.snapshot.captured_at)}</span>
    {/if}
  </p>
{/if}

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
      <code>public_enabled: false</code> - it will materialize data in the background but
      stays hidden here until an admin publishes it.
    </p>
    <ol class="hint next-steps">
      <li>Open the admin UI (Settings → Analysis).</li>
      <li>Toggle <strong>Public</strong> on for the cohort you want to expose here.</li>
      <li>Optionally click <strong>Make default</strong> so it becomes the default view.</li>
    </ol>
  </section>
{:else if data.noSnapshot}
  <section class="card empty-state">
    <h2>No snapshot has been captured yet</h2>
    <p class="hint">
      This cohort is published, but this dashboard reads captured snapshots
      and the cohort does not have one yet. In the admin UI, click
      <strong>Run new snapshot</strong> for the cohort and wait for the batch
      to finish. <strong>Rebuild</strong> updates materialized rows, but it
      does not create a public snapshot by itself.
    </p>
  </section>
{:else if data.loadError}
  <section class="card">
    <h2>Overview</h2>
    <p class="status-banner error">Failed to load cohort: {data.loadError}</p>
  </section>
{:else}
  <section class="card overview-header">
    <h2>{data.label}</h2>
    {#if data.description}
      <p class="hint">{data.description}</p>
    {/if}
    {#if formatTimestamp(data.lastMaterializedAt)}
      <p class="hint">Last analyzed: {formatTimestamp(data.lastMaterializedAt)}</p>
    {/if}
  </section>

  {#if isEmpty}
    <section class="card empty-state">
      <h3>No data has been materialized yet</h3>
      <p class="hint">
        This cohort is published but the projector hasn't seen any matching runs yet.
        Run a job (or batch) tagged with <code>{data.datasetTag}</code>, or trigger
        <strong>Rebuild</strong> from the admin UI to project any existing runs that
        already carry this tag.
      </p>
    </section>
  {:else}
    <section class="hero" aria-label="Cohort headline metrics">
      <a class="hero-tile" href={`${base}/domains${query}`}>
        <span class="hero-label">Domains</span>
        <span class="hero-value">{formatCount(domainCount)}</span>
        <span class="hero-foot">
          {#if domainsMetric.delta !== null && domainsMetric.delta !== 0}
            <span class="hero-delta tone-{deltaTone(domainsMetric.delta, null)}">{deltaText(domainsMetric.delta, "")}</span>
          {/if}
          {#if domainsMetric.values.length > 1}
            <Sparkline values={domainsMetric.values} tone="neutral" ariaLabel="Domain count trend" />
          {/if}
        </span>
      </a>
      {#if healthyMetric.latest !== null}
        <a class="hero-tile" href={`${base}/trends?category=severity${data.datasetTag ? `&dataset_tag=${data.datasetTag}` : ""}`}>
          <span class="hero-label">Healthy</span>
          <span class="hero-value">{healthyMetric.latest}%</span>
          <span class="hero-foot">
            {#if healthyMetric.delta !== null && healthyMetric.delta !== 0}
              <span class="hero-delta tone-{deltaTone(healthyMetric.delta, true)}">{deltaText(healthyMetric.delta, "%")}</span>
            {/if}
            {#if healthyMetric.values.length > 1}
              <Sparkline values={healthyMetric.values} tone="ok" yDomain={[0, 100]} ariaLabel="Healthy share trend" />
            {/if}
          </span>
        </a>
      {/if}
      {#if topGradeMetric.latest !== null}
        <a class="hero-tile" href={`${base}/trends?category=grade${data.datasetTag ? `&dataset_tag=${data.datasetTag}` : ""}`}>
          <span class="hero-label">Grade A / A+</span>
          <span class="hero-value">{topGradeMetric.latest}%</span>
          <span class="hero-foot">
            {#if topGradeMetric.delta !== null && topGradeMetric.delta !== 0}
              <span class="hero-delta tone-{deltaTone(topGradeMetric.delta, true)}">{deltaText(topGradeMetric.delta, "%")}</span>
            {/if}
            {#if topGradeMetric.values.length > 1}
              <Sparkline values={topGradeMetric.values} tone="ok" yDomain={[0, 100]} ariaLabel="Top-grade share trend" />
            {/if}
          </span>
        </a>
      {/if}
      {#if signedMetric.latest !== null}
        <a class="hero-tile" href={`${base}/trends?category=dnssec_posture${data.datasetTag ? `&dataset_tag=${data.datasetTag}` : ""}`}>
          <span class="hero-label">Signed</span>
          <span class="hero-value">{signedMetric.latest}%</span>
          <span class="hero-foot">
            {#if signedMetric.delta !== null && signedMetric.delta !== 0}
              <span class="hero-delta tone-{deltaTone(signedMetric.delta, true)}">{deltaText(signedMetric.delta, "%")}</span>
            {/if}
            {#if signedMetric.values.length > 1}
              <Sparkline values={signedMetric.values} tone="notice" yDomain={[0, 100]} ariaLabel="Signed share trend" />
            {/if}
          </span>
        </a>
      {/if}
    </section>

    <section class="summary-grid" aria-label="Cohort entity counts">
      {#each summaryCards as card (card.label)}
        <svelte:element
          this={card.href ? "a" : "div"}
          class="summary-card"
          class:static={!card.href}
          href={card.href ? `${base}${card.href}${query}` : undefined}
        >
          <span class="summary-count">{formatCount(card.value)}</span>
          <span class="summary-label">{card.label}</span>
        </svelte:element>
      {/each}
    </section>

    {#if hasMovement}
      <section class="card movers-card">
        <div class="list-head">
          <h3>Since the previous snapshot</h3>
          <a class="movers-difflink" href={fullDiffHref}>View full diff</a>
        </div>
        <ul class="mover-summary">
          <li class="tone-error"><strong>{diffSummary.regressed}</strong> regressed</li>
          <li class="tone-ok"><strong>{diffSummary.improved}</strong> improved</li>
          <li class="tone-notice"><strong>{diffSummary.added}</strong> added</li>
          <li class="tone-neutral"><strong>{diffSummary.removed}</strong> removed</li>
        </ul>
        <div class="mover-cols">
          <div class="mover-col">
            <h4>Top regressions</h4>
            {#if regressions.length > 0}
              <ul class="mover-domains">
                {#each regressions as e (e.domain)}
                  <li>
                    <a class="mover-domain" href={moverDomainLink(e.domain)}>{e.domain}</a>
                    <span class="pair">
                      <GradeChip grade={e.from_grade} />
                      <span class="arrow" aria-hidden="true">→</span>
                      <span class="sr-only">to</span>
                      <GradeChip grade={e.to_grade} />
                    </span>
                  </li>
                {/each}
              </ul>
            {:else}
              <p class="hint">No grade regressions.</p>
            {/if}
          </div>
          <div class="mover-col">
            <h4>Top improvements</h4>
            {#if improvements.length > 0}
              <ul class="mover-domains">
                {#each improvements as e (e.domain)}
                  <li>
                    <a class="mover-domain" href={moverDomainLink(e.domain)}>{e.domain}</a>
                    <span class="pair">
                      <GradeChip grade={e.from_grade} />
                      <span class="arrow" aria-hidden="true">→</span>
                      <span class="sr-only">to</span>
                      <GradeChip grade={e.to_grade} />
                    </span>
                  </li>
                {/each}
              </ul>
            {:else}
              <p class="hint">No grade improvements.</p>
            {/if}
          </div>
        </div>
      </section>
    {/if}

    {#each factDistributions as dist (dist.category)}
      <FactDistributionBar
        title={dist.label}
        description={dist.description}
        buckets={dist.buckets}
        multiPerDomain={multiBucketCategories.has(dist.category)}
        hrefForKey={buildHrefForKey(dist.category)}
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
    {/if}
    {#if topNameserverRows.length > 0 || topASNRows.length > 0}
      <div class="infra-grid">
        {#if topNameserverRows.length > 0}
          <section class="card infra-card">
            <h3>Top nameservers</h3>
            <p class="hint">
              Nameservers hosting the most domains in this cohort.
              Showing top {topNameserverRows.length}.
              {#if nsLeaderShare > 0}
                <span class="concentration">Leader hosts {nsLeaderShare}% of domains.</span>
              {/if}
            </p>
            <ol class="infra-list">
              {#each topNameserverRows as row (row.name)}
                <li>
                  <a class="infra-row" href={nameserverHref(base, row.name, query)}>
                    <span class="infra-name" title={idnTooltip(row.name)}>{row.name}</span>
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
              {topASNRows.length}.
              {#if asnLeaderShare > 0}
                <span class="concentration">Leader hosts {asnLeaderShare}% of domains.</span>
              {/if}
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
    {/if}
  {/if}
{/if}

<style>
  .overview-header {
    gap: var(--space-2);
  }
  .overview-header h2 {
    margin: 0;
  }
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
  .concentration {
    color: var(--ink);
    font-weight: 600;
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

  .hero {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: var(--space-3);
  }
  .hero-tile {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    padding: var(--space-4) var(--space-5);
    background: var(--card);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    text-decoration: none;
    color: var(--ink);
    transition: border-color 0.15s ease, transform 0.15s ease;
  }
  .hero-tile:hover {
    border-color: var(--accent-2);
    transform: translateY(-1px);
  }
  .hero-label {
    font-size: var(--text-sm);
    color: var(--ink-2);
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }
  .hero-value {
    font-size: var(--text-2xl);
    font-weight: 700;
    font-family: var(--mono);
    line-height: 1.1;
  }
  .hero-foot {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    min-height: 24px;
  }
  .hero-delta {
    font-family: var(--mono);
    font-size: var(--text-xs);
    font-weight: 600;
  }
  .hero-delta.tone-ok { color: var(--bar-ok); }
  .hero-delta.tone-error { color: var(--bar-error); }
  .hero-delta.tone-neutral { color: var(--ink-2); }

  .movers-card {
    gap: var(--space-3);
  }
  .movers-difflink {
    color: var(--accent-2);
    text-decoration: none;
    font-size: var(--text-sm);
  }
  .movers-difflink:hover {
    text-decoration: underline;
  }
  .mover-summary {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-4);
    font-size: var(--text-sm);
    color: var(--ink-2);
  }
  .mover-summary strong {
    font-family: var(--mono);
    font-size: var(--text-lg);
  }
  .mover-summary .tone-error strong { color: var(--bar-error); }
  .mover-summary .tone-ok strong { color: var(--bar-ok); }
  .mover-summary .tone-notice strong { color: var(--bar-notice); }
  .mover-summary .tone-neutral strong { color: var(--ink); }
  .mover-cols {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(16rem, 1fr));
    gap: var(--space-4);
  }
  .mover-col h4 {
    margin: 0 0 var(--space-2);
    font-size: var(--text-sm);
    color: var(--ink-2);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .mover-domains {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .mover-domains li {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
  }
  .mover-domain {
    font-family: var(--mono);
    font-size: var(--text-sm);
    color: var(--ink);
    text-decoration: none;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .mover-domain:hover {
    color: var(--accent-2);
    text-decoration: underline;
  }
  .pair {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    flex-shrink: 0;
  }
  .pair .arrow {
    color: var(--ink-2);
    font-family: var(--mono);
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
  .summary-card:hover:not(.static) {
    border-color: var(--accent-2);
    transform: translateY(-1px);
  }
  /* Non-linking count (e.g. Prefixes, which has no list view). */
  .summary-card.static {
    cursor: default;
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

  .snapshot-pill {
    margin: 0;
    padding: 6px var(--space-3);
    background: var(--surface-2);
    color: var(--on-surface-2);
    border-radius: 999px;
    font-size: var(--text-sm);
    display: inline-flex;
    align-items: baseline;
    gap: 8px;
    align-self: flex-start;
  }
  .snapshot-slug {
    font-weight: 600;
  }
  .snapshot-label {
    color: var(--ink-2);
  }
  .snapshot-captured {
    color: var(--ink-2);
    font-size: var(--text-xs);
    border-left: 1px solid var(--border);
    padding-left: 8px;
  }
</style>
