<script lang="ts">
  import { page } from "$app/state";
  import StubPage from "$lib/StubPage.svelte";

  const domain = $derived(page.params.domain ?? "");
</script>

<StubPage
  heading={`Domain · ${domain}`}
  note="Domain detail (nameservers, addresses, tags). Phase 5 wires this to /pub/api/v1/analysis/domains/{domain}."
/>
