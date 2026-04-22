<script lang="ts">
  import { formatCount } from "$lib/format";
  import type { FactBucket } from "../routes/+page";

  type Props = {
    title: string;
    description?: string;
    buckets: FactBucket[];
    // Caption shown below bars when a domain can legitimately land in
    // multiple buckets (e.g. zones publishing two DNSKEY algorithms).
    multiPerDomain?: boolean;
  };

  let { title, description, buckets, multiPerDomain = false }: Props = $props();

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
  }
  .fact-dist-segment {
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
</style>
