<script>
  let { grade = null, score = null, showDenominator = false } = $props();
</script>

{#if grade != null && score != null}
  <span class="grade-chip">
    <span class="grade-chip-letter" data-grade={grade}>{grade}</span>
    <span class="grade-chip-score">{score}{showDenominator ? "/100" : ""}</span>
  </span>
{/if}
