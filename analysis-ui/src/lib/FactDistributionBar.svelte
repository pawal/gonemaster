<script lang="ts">
  import { formatCount } from "$lib/format";
  import type { FactBucket } from "$lib/api";

  type Props = {
    title: string;
    description?: string;
    buckets: FactBucket[];
    // Caption shown below bars when a domain can legitimately land in
    // multiple buckets (e.g. zones publishing two DNSKEY algorithms).
    multiPerDomain?: boolean;
    // Optional href builder. When provided, each segment becomes a link
    // into a filtered view (e.g. /domains?grade=B). Omit for bars that
    // are purely informational.
    hrefForKey?: (key: string) => string;
  };

  let {
    title,
    description,
    buckets,
    multiPerDomain = false,
    hrefForKey
  }: Props = $props();

  const totalCount = $derived(buckets.reduce((sum, b) => sum + b.count, 0));
  // flex-grow uses the raw count so a 50-domain segment is 10x as wide
  // as a 5-domain segment, matching the existing health-bar visual.
  const visible = $derived(buckets.filter((b) => b.count > 0));
</script>

{#if visible.length > 0}
  <section class="card fact-dist-section">
    <h3>{title}</h3>
    {#if description}
      <p class="hint">{description}</p>
    {/if}
    {#if hrefForKey}
      <ul class="fact-dist-bar" aria-label={title}>
        {#each visible as b (b.key)}
          <li class="fact-dist-item" style:flex-grow={b.count}>
            <a
              class="fact-dist-segment fact-dist-link tone-{b.tone}"
              href={hrefForKey(b.key)}
              title="{b.label}: {formatCount(b.count)}{multiPerDomain ? ' domains' : ''}. Click to filter the domains list."
            >
              <span class="fact-dist-label">{b.label}</span>
              <span class="fact-dist-count">{formatCount(b.count)}</span>
            </a>
          </li>
        {/each}
      </ul>
    {:else}
      <div class="fact-dist-bar" role="list" aria-label={title}>
        {#each visible as b (b.key)}
          <div
            class="fact-dist-segment tone-{b.tone}"
            role="listitem"
            style:flex-grow={b.count}
            title="{b.label}: {formatCount(b.count)}{multiPerDomain ? ' domains' : ''}"
          >
            <span class="fact-dist-label">{b.label}</span>
            <span class="fact-dist-count">{formatCount(b.count)}</span>
          </div>
        {/each}
      </div>
    {/if}
    {#if multiPerDomain && totalCount > 0}
      <p class="hint fact-dist-caption">
        Domains can publish multiple algorithms, so segments sum to more than the
        cohort's domain count.
      </p>
    {/if}
  </section>
{/if}

<style>
  .fact-dist-section {
    gap: var(--space-2);
  }
  .fact-dist-section h3 {
    margin: 0;
  }
  .fact-dist-bar {
    display: flex;
    width: 100%;
    min-height: 36px;
    border-radius: var(--radius);
    overflow: hidden;
    border: 1px solid var(--border);
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .fact-dist-item {
    display: flex;
    min-width: 3rem;
  }
  .fact-dist-segment {
    display: flex;
    flex: 1 1 auto;
    align-items: center;
    justify-content: center;
    gap: 8px;
    padding: 0 var(--space-3);
    min-width: 3rem;
    font-size: var(--text-xs);
    white-space: nowrap;
    overflow: hidden;
  }
  .fact-dist-link {
    text-decoration: none;
    color: inherit;
    transition: filter 0.15s ease;
  }
  .fact-dist-link:hover,
  .fact-dist-link:focus-visible {
    filter: brightness(0.95);
  }
  .fact-dist-label {
    font-weight: 600;
  }
  .fact-dist-count {
    font-family: var(--mono);
  }
  .tone-ok { background: #dcfce7; color: #166534; }
  .tone-notice { background: #e0f2fe; color: #075985; }
  .tone-warning { background: #fef3c7; color: #92400e; }
  .tone-error { background: #ffedd5; color: #9a3412; }
  .tone-critical { background: #fee2e2; color: #991b1b; }
  .tone-neutral { background: var(--surface-2); color: var(--on-surface-2); }
  .fact-dist-caption {
    margin: 0;
  }

  /* On narrow viewports, the proportional segmented bar can't fit labels
     like "WARNING" or "ECDSAP256SHA256" inside small flex-grow segments.
     Stack as full-width rows: column flex with no extra space means
     flex-grow no longer affects sizing, so each segment renders at a
     readable min-height with label and count on opposite ends. */
  @media (max-width: 600px) {
    .fact-dist-bar {
      flex-direction: column;
    }
    .fact-dist-item {
      min-width: 0;
    }
    .fact-dist-segment {
      min-width: 0;
      min-height: 36px;
      padding: 6px var(--space-3);
      white-space: normal;
      justify-content: space-between;
    }
  }
</style>
