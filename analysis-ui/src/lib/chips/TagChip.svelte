<script lang="ts">
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import { tagHref } from "$lib/entityLinks";
  import EntityChip from "./EntityChip.svelte";

  type Props = { tag: string; preserveQuery?: boolean };
  let { tag, preserveQuery = true }: Props = $props();

  const href = $derived(tagHref(base, tag, preserveQuery ? page.url.search : ""));
</script>

<EntityChip {href} variant="tag" title={tag}>
  {tag}
</EntityChip>
