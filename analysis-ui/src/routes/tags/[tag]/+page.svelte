<script lang="ts">
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import DomainChip from "$lib/chips/DomainChip.svelte";
  import TestcaseChip from "$lib/chips/TestcaseChip.svelte";
  import { formatCount, levelTone } from "$lib/format";
  import { testcaseTitle } from "$lib/testcaseTitles";
  import type { TagDetailPageData } from "./+page";

  let { data }: { data: TagDetailPageData } = $props();

  const query = $derived(page.url.search);
  const tcTitle = $derived(testcaseTitle(data.detail?.testcase));
</script>

<nav class="breadcrumbs" aria-label="Breadcrumb">
  <a href={`${base}/tags${query}`}>← Tags</a>
</nav>

{#if !data.datasetTag}
  <section class="card">
    <h2>{data.tag}</h2>
    <p class="status-banner warn">No cohort resolved. Configure a public cohort to view tag details.</p>
  </section>
{:else if data.error}
  <section class="card">
    <h2>{data.tag}</h2>
    <p class="status-banner error">Failed to load tag: {data.error}</p>
  </section>
{:else if !data.detail}
  <section class="card">
    <h2>{data.tag}</h2>
    <p class="status-banner">Tag not materialized for this cohort.</p>
  </section>
{:else}
  {@const d = data.detail}
  <section class="card">
    <div class="detail-header">
      <div class="detail-title">
        <h2>{d.tag}</h2>
        {#if d.module && d.testcase}
          <div class="detail-subtitle">
            <TestcaseChip module={d.module} testcase={d.testcase} />
            {#if tcTitle}<span class="detail-subtitle-text">{tcTitle}</span>{/if}
          </div>
        {:else if tcTitle}
          <p class="detail-subtitle detail-subtitle-text">{tcTitle}</p>
        {/if}
      </div>
      <div class="detail-scorecard">
        {#if d.level}
          <span class={`level level-${levelTone(d.level)}`}>{d.level}</span>
        {/if}
      </div>
    </div>

    <dl class="detail-counts">
      <div><dt>Domains</dt><dd>{formatCount(d.domain_count)}</dd></div>
      <div><dt>Occurrences</dt><dd>{formatCount(d.occurrence_count)}</dd></div>
    </dl>
  </section>

  <section class="card">
    <h3>Domains with this tag</h3>
    {#if d.domains.length === 0}
      <p class="hint">No domains in this cohort carry this tag.</p>
    {:else}
      <ul class="chip-list" role="list">
        {#each d.domains as domain (domain)}
          <li><DomainChip {domain} /></li>
        {/each}
      </ul>
    {/if}
  </section>
{/if}

<style>
  .breadcrumbs { font-size: var(--text-sm); }
  .breadcrumbs a { color: var(--ink-2); text-decoration: none; }
  .breadcrumbs a:hover { color: var(--accent-2); }

  .detail-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    flex-wrap: wrap;
  }
  .detail-header h2 { margin: 0; font-family: var(--mono); }
  .detail-title {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }
  .detail-subtitle {
    margin: 0;
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-2);
    color: var(--ink-2);
    font-size: var(--text-sm);
  }
  .detail-subtitle-text { min-width: 0; }
  .detail-scorecard {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
  }

  .level {
    display: inline-block;
    padding: 2px 8px;
    border-radius: 6px;
    font-size: var(--text-xs);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .level-critical { background: #fee2e2; color: #991b1b; }
  .level-error { background: #ffedd5; color: #9a3412; }
  .level-warning { background: #fef3c7; color: #92400e; }
  .level-notice { background: #e0f2fe; color: #075985; }
  .level-neutral { background: var(--surface-2); color: var(--on-surface-2); }

  .detail-counts {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-6);
    margin: var(--space-3) 0 0;
  }
  .detail-counts > div { display: flex; flex-direction: column; gap: 2px; }
  .detail-counts dt {
    font-size: var(--text-xs);
    color: var(--ink-2);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .detail-counts dd {
    margin: 0;
    font-family: var(--mono);
    font-size: var(--text-lg);
    font-weight: 600;
  }

  .chip-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }
</style>
