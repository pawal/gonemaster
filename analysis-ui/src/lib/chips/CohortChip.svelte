<script lang="ts">
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import { cohortHref } from "$lib/entityLinks";
  import EntityChip from "./EntityChip.svelte";

  type Props = { datasetTag: string; label?: string; preserveQuery?: boolean };
  let { datasetTag, label, preserveQuery = true }: Props = $props();

  const href = $derived(cohortHref(base, datasetTag, preserveQuery ? page.url.search : ""));
</script>

<EntityChip {href} variant="cohort" title={label ?? datasetTag}>
  {label ?? datasetTag}
</EntityChip>
