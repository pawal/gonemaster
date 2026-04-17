<script lang="ts">
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import { nameserverHref } from "$lib/entityLinks";
  import EntityChip from "./EntityChip.svelte";

  type Props = { nameserver: string; preserveQuery?: boolean };
  let { nameserver, preserveQuery = true }: Props = $props();

  const href = $derived(nameserverHref(base, nameserver, preserveQuery ? page.url.search : ""));
</script>

<EntityChip {href} variant="nameserver" title={nameserver}>
  {nameserver}
</EntityChip>
