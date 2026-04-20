<script lang="ts">
  import EntityChip from "./EntityChip.svelte";

  type Props = { testcase: string; module: string };
  let { testcase, module }: Props = $props();

  // Link to the upstream specification markdown on codeberg. Paths are
  // lowercase: docs/specifications/tests/<module>/<testcase>.md.
  const href = $derived(
    `https://codeberg.org/pawal/gonemaster/src/branch/main/docs/specifications/tests/${module.toLowerCase()}/${testcase.toLowerCase()}.md`
  );
  const display = $derived(`${module}/${testcase}`);
</script>

<EntityChip {href} variant="testcase" title={`${display} specification`} external>
  {display}
</EntityChip>
