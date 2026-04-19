<script lang="ts">
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import DomainChip from "$lib/chips/DomainChip.svelte";
  import TagChip from "$lib/chips/TagChip.svelte";
  import { formatCount, levelTone } from "$lib/format";
  import type { TestcaseDetailPageData } from "./+page";

  let { data }: { data: TestcaseDetailPageData } = $props();

  const query = $derived(page.url.search);
  const title = $derived(data.module ? `${data.module}/${data.testcase}` : data.testcase);
</script>

<nav class="breadcrumbs" aria-label="Breadcrumb">
  <a href={`${base}/testcases${query}`}>← Testcases</a>
</nav>

{#if !data.datasetTag}
  <section class="card">
    <h2>{title}</h2>
    <p class="status-banner warn">No cohort resolved. Configure a public cohort to view testcase details.</p>
  </section>
{:else if !data.module}
  <section class="card">
    <h2>{title}</h2>
    <p class="status-banner warn">The <code>module</code> query parameter is required. Navigate here from a Testcase chip or the testcases list.</p>
  </section>
{:else if data.error}
  <section class="card">
    <h2>{title}</h2>
    <p class="status-banner error">Failed to load testcase: {data.error}</p>
  </section>
{:else if !data.detail}
  <section class="card">
    <h2>{title}</h2>
    <p class="status-banner">Testcase not materialized for this cohort.</p>
  </section>
{:else}
  {@const d = data.detail}
  <section class="card">
    <div class="detail-header">
      <h2>{d.module}/{d.testcase}</h2>
      {#if d.worst_level}
        <span class={`level level-${levelTone(d.worst_level)}`}>{d.worst_level}</span>
      {/if}
    </div>

    <dl class="detail-counts">
      <div><dt>Domains</dt><dd>{formatCount(d.domain_count)}</dd></div>
      <div><dt>Entries</dt><dd>{formatCount(d.entry_count)}</dd></div>
    </dl>
  </section>

  <section class="card">
    <h3>Tags from this testcase</h3>
    {#if d.tags.length === 0}
      <p class="hint">No tags observed for this testcase in the cohort.</p>
    {:else}
      <ul class="chip-list" role="list">
        {#each d.tags as tag (tag)}
          <li><TagChip {tag} /></li>
        {/each}
      </ul>
    {/if}
  </section>

  <section class="card">
    <h3>Domains affected</h3>
    {#if d.domains.length === 0}
      <p class="hint">No domains in this cohort triggered this testcase.</p>
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

  code {
    font-family: var(--mono);
    font-size: var(--text-sm);
    background: var(--surface-2);
    color: var(--on-surface-2);
    padding: 1px 6px;
    border-radius: 4px;
  }
</style>
