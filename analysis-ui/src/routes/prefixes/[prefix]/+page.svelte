<script lang="ts">
  import { page } from "$app/state";
  import StubPage from "$lib/StubPage.svelte";

  const prefix = $derived(page.params.prefix ?? "");
</script>

<StubPage
  heading={`Prefix · ${prefix}`}
  note="Prefix detail. Phase 5 will call /pub/api/v1/analysis/prefix?prefix={prefix} since the prefix contains a slash."
/>
