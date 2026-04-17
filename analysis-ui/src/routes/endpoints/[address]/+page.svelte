<script lang="ts">
  import { page } from "$app/state";
  import StubPage from "$lib/StubPage.svelte";

  const address = $derived(page.params.address ?? "");
</script>

<StubPage
  heading={`Endpoint · ${address}`}
  note="Endpoint detail. Phase 5 wires this to /pub/api/v1/analysis/endpoints/{address}."
/>
