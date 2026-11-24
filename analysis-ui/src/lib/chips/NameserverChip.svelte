<script lang="ts">
    import { page } from "$app/state";
  import { nameserverHref } from "#lib/entityLinks.js";
  import { idnTooltip } from "#lib/idn.js";
  import EntityChip from "./EntityChip.svelte";

  type Props = { nameserver: string; preserveQuery?: boolean };
  let { nameserver, preserveQuery = true }: Props = $props();

  const href = $derived(nameserverHref(nameserver, preserveQuery ? page.url.search : ""));
</script>

<EntityChip {href} variant="nameserver" title={idnTooltip(nameserver)}>
  {nameserver}
</EntityChip>
