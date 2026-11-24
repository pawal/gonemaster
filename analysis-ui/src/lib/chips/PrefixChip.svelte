<script lang="ts">
    import { page } from "$app/state";
  import { prefixHref } from "#lib/entityLinks.js";
  import EntityChip from "./EntityChip.svelte";

  type Props = { prefix: string; preserveQuery?: boolean };
  let { prefix, preserveQuery = true }: Props = $props();

  const href = $derived(prefixHref(prefix, preserveQuery ? page.url.search : ""));
</script>

<EntityChip {href} variant="prefix" title={prefix}>
  {prefix}
</EntityChip>
