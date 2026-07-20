<script lang="ts">
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import { tagHref } from "$lib/entityLinks";
  import EntityChip from "./EntityChip.svelte";

  // `query` overrides the preserved URL query to pin a specific snapshot.
  type Props = { tag: string; preserveQuery?: boolean; query?: string };
  let { tag, preserveQuery = true, query }: Props = $props();

  const href = $derived(tagHref(base, tag, query ?? (preserveQuery ? page.url.search : "")));
</script>

<EntityChip {href} variant="tag" title={tag}>
  {tag}
</EntityChip>
