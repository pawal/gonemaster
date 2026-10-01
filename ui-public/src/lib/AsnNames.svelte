<script>
  import { t } from "../i18n.js";

  // asns: the finding's AS numbers; names: Map of AS number to holder.
  let { asns = [], names = new Map() } = $props();

  let shown = $derived(asns.map((asn) => names.get(asn)).filter((n) => n?.name));
</script>

{#if shown.length > 0}
  <ul class="asn-names" aria-label={$t("pub.asn_holders")} data-testid="asn-names">
    {#each shown as holder (holder.asn)}
      <li title={holder.label}>AS{holder.asn}: {holder.name}{#if holder.country}{" "}({holder.country}){/if}</li>
    {/each}
  </ul>
{/if}

<style>
  .asn-names {
    list-style: none;
    margin: 0;
    padding: 0 0 0 100px; /* under the message column, as .result-explanation */
    font-size: var(--text-sm);
    color: var(--muted);
    overflow-wrap: break-word;
  }

  @media (max-width: 600px) {
    .asn-names {
      padding-left: 0;
    }
  }
</style>
