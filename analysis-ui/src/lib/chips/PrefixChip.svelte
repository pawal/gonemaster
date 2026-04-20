<script lang="ts">
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import { prefixHref } from "$lib/entityLinks";
  import EntityChip from "./EntityChip.svelte";

  type Props = { prefix: string; preserveQuery?: boolean };
  let { prefix, preserveQuery = true }: Props = $props();

  const href = $derived(prefixHref(base, prefix, preserveQuery ? page.url.search : ""));
</script>

<EntityChip {href} variant="prefix" title={prefix}>
  {prefix}
</EntityChip>
