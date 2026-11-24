<script lang="ts">
    import { page } from "$app/state";
  import { cohortHref } from "#lib/entityLinks.js";
  import EntityChip from "./EntityChip.svelte";

  type Props = { datasetTag: string; label?: string; preserveQuery?: boolean };
  let { datasetTag, label, preserveQuery = true }: Props = $props();

  const href = $derived(cohortHref(datasetTag, preserveQuery ? page.url.search : ""));
</script>

<EntityChip {href} variant="cohort" title={label ?? datasetTag}>
  {label ?? datasetTag}
</EntityChip>
