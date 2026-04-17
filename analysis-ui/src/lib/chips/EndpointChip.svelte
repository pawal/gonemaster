<script lang="ts">
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import { endpointHref } from "$lib/entityLinks";
  import EntityChip from "./EntityChip.svelte";

  type Props = {
    address: string;
    nameserver?: string | null;
    preserveQuery?: boolean;
  };

  let { address, nameserver = null, preserveQuery = true }: Props = $props();

  const href = $derived(endpointHref(base, address, nameserver, preserveQuery ? page.url.search : ""));
</script>

<EntityChip {href} variant="endpoint" title={nameserver ? `${nameserver} @ ${address}` : address}>
  {address}
</EntityChip>
