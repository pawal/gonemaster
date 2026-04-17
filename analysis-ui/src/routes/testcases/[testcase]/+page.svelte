<script lang="ts">
  import { page } from "$app/state";
  import StubPage from "$lib/StubPage.svelte";

  const testcase = $derived(page.params.testcase ?? "");
</script>

<StubPage
  heading={`Testcase · ${testcase}`}
  note="Testcase detail. Phase 5 will call /pub/api/v1/analysis/testcase?module=…&testcase={testcase}."
/>
