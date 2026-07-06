<script>
  import ScoreCard from "./ScoreCard.svelte";
  import RunResultBody from "./RunResultBody.svelte";

  let {
    result = null,
    scoringEnabled = false,
    nameserverTimingsEnabled = false,
    idPrefix = "",
  } = $props();
</script>

{#if result}
  {#if scoringEnabled}
    <ScoreCard score={result.score} />
  {/if}
  <RunResultBody {result} {nameserverTimingsEnabled} {idPrefix} />
{/if}
