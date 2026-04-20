<script lang="ts">
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import { domainHref } from "$lib/entityLinks";
  import EntityChip from "./EntityChip.svelte";

  type Props = { domain: string; preserveQuery?: boolean };
  let { domain, preserveQuery = true }: Props = $props();

  const href = $derived(domainHref(base, domain, preserveQuery ? page.url.search : ""));
</script>

<EntityChip {href} variant="domain" title={domain}>
  {domain}
</EntityChip>
