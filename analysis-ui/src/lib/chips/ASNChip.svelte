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
  const display = $derived(`AS${asn}`);
</script>

<EntityChip {href} variant="asn" title={label ?? display}>
  {display}
</EntityChip>
