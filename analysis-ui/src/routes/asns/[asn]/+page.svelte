<script lang="ts">
  import { page } from "$app/state";
  import StubPage from "$lib/StubPage.svelte";

  const asn = $derived(page.params.asn ?? "");
</script>

<StubPage
  heading={`ASN · AS${asn}`}
  note="ASN detail. Phase 5 wires this to /pub/api/v1/analysis/asns/{asn}."
/>
