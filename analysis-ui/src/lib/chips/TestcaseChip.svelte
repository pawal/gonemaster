<script lang="ts">
  import { base } from "$app/paths";
  import { page } from "$app/state";
  import { testcaseHref } from "$lib/entityLinks";
  import EntityChip from "./EntityChip.svelte";

  type Props = { testcase: string; module?: string; preserveQuery?: boolean };
  let { testcase, module, preserveQuery = true }: Props = $props();

  const href = $derived(testcaseHref(base, testcase, preserveQuery ? page.url.search : ""));
  const display = $derived(module ? `${module}/${testcase}` : testcase);
</script>

<EntityChip {href} variant="testcase" title={display}>
  {display}
</EntityChip>
