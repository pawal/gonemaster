<script lang="ts">
  import type { DomainRegistry } from "$lib/api";
  import { formatDate, formatTimestamp } from "$lib/format";

  type Props = {
    registry?: DomainRegistry;
  };

  let { registry }: Props = $props();

  const state = $derived(registry?.state ?? "");
  const hasData = $derived(state === "fresh" || state === "stale");

  type Row = { term: string; value: string };

  const rows = $derived.by<Row[]>(() => {
    const r = registry;
    if (!r || !hasData) return [];
    const out: Row[] = [];
    const add = (term: string, value: string | undefined) => {
      if (value) out.push({ term, value });
    };
    add("Registrar", r.registrar);
    add("Registry", r.registry_org);
    add("Handle", r.handle);
    add("Status", (r.status ?? []).join(", "));
    add("Registered", formatDate(r.registered_at));
    add("Expires", formatDate(r.expires_at));
    add("Changed", formatDate(r.changed_at));
    if (r.delegation_signed !== undefined) {
      add("Signed delegation", r.delegation_signed ? "Yes" : "No");
    }
    add("Nameservers", (r.nameservers ?? []).join(", "));
    return out;
  });

  const fetched = $derived(formatTimestamp(registry?.fetched_at));
</script>

{#if registry}
  <section class="card registry-card">
    <h3>Registry</h3>
    {#if state === "pending"}
      <p class="hint">Registry data is being fetched; reload in a moment.</p>
    {:else if !hasData}
      <p class="hint">No registry data is available for this domain.</p>
    {:else if rows.length === 0}
      <p class="hint">The registry published no details for this domain.</p>
    {:else}
      <dl class="registry-rows">
        {#each rows as row (row.term)}
          <div>
            <dt>{row.term}</dt>
            <dd>{row.value}</dd>
          </div>
        {/each}
      </dl>
      <p class="hint">
        {#if fetched}Retrieved {fetched} over RDAP{:else}Retrieved over RDAP{/if}
        {#if registry.source_url}
          from
          <a href={registry.source_url} target="_blank" rel="noopener noreferrer"
            >{registry.source_url}</a
          >{/if}. It reflects the registry, not this measurement.
      </p>
    {/if}
  </section>
{/if}

<style>
  .registry-rows {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
    gap: var(--space-3) var(--space-6);
    margin: 0;
  }
  .registry-rows > div {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }
  .registry-rows dt {
    font-size: var(--text-xs);
    color: var(--ink-2);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .registry-rows dd {
    margin: 0;
    font-size: var(--text-sm);
    overflow-wrap: anywhere;
  }
  .registry-card a {
    color: var(--accent-2);
  }
</style>
