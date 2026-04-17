<script lang="ts">
  import { page } from "$app/state";
  import StubPage from "$lib/StubPage.svelte";

  const tag = $derived(page.params.tag ?? "");
</script>

<StubPage
  heading={`Tag · ${tag}`}
  note="Tag detail. Phase 5 wires this to /pub/api/v1/analysis/tags/{tag}."
/>
