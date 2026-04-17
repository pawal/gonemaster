<script lang="ts">
  import { page } from "$app/state";
  import StubPage from "$lib/StubPage.svelte";

  const name = $derived(page.params.name ?? "");
</script>

<StubPage
  heading={`Nameserver · ${name}`}
  note="Nameserver detail (domains served, addresses, ASNs). Phase 5 wires this to /pub/api/v1/analysis/nameservers/{name}."
/>
