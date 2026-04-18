<script lang="ts">
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import { asnHref } from "$lib/entityLinks";
  import EntityChip from "./EntityChip.svelte";

  type Props = {
    asn: number | string;
    label?: string;
    preserveQuery?: boolean;
  };

  let { asn, label, preserveQuery = true }: Props = $props();

  const href = $derived(asnHref(base, asn, preserveQuery ? page.url.search : ""));
  const tooltip = $derived(label ? `AS${asn} · ${label}` : `AS${asn}`);
</script>

<EntityChip {href} variant="asn" title={tooltip}>
  {#if label}
    <span class="asn-num">AS{asn}</span>
    <span class="asn-label">{label}</span>
  {:else}
    AS{asn}
  {/if}
</EntityChip>

<style>
  .asn-num {
    opacity: 0.7;
    margin-right: 4px;
  }
  .asn-label {
    font-family: var(--sans);
    font-weight: 500;
  }
</style>
