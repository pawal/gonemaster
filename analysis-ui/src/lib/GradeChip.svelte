<script lang="ts">
  // Grade letter tile, optionally paired with a numeric score. Colour is
  // keyed by the data-grade attribute so CSS owns the palette.
  type Props = {
    grade?: string | null;
    score?: number | null;
    showDenominator?: boolean;
  };
  let { grade = null, score = null, showDenominator = false }: Props = $props();

  const normalized = $derived(String(grade ?? "").trim().toUpperCase());
</script>

{#if normalized}
  <span class="grade-chip">
    <span class="grade-chip-letter" data-grade={normalized}>{normalized}</span>
    {#if score != null}
      <span class="grade-chip-score">{score}{showDenominator ? "/100" : ""}</span>
    {/if}
  </span>
{/if}

<style>
  .grade-chip {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    font-size: var(--text-xs);
    font-weight: 600;
    white-space: nowrap;
  }
  .grade-chip-letter {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 22px;
    height: 20px;
    padding: 0 4px;
    border-radius: 4px;
    color: #fff;
    font-size: var(--text-xs);
    font-weight: 700;
    font-family: var(--mono);
    background: var(--bar-neutral);
    flex-shrink: 0;
  }
  .grade-chip-letter[data-grade="A+"] { background: var(--grade-aplus); }
  .grade-chip-letter[data-grade="A"]  { background: var(--grade-a); }
  .grade-chip-letter[data-grade="B"]  { background: var(--grade-b); color: #1a1a1a; }
  .grade-chip-letter[data-grade="C"]  { background: var(--grade-c); color: #1a1a1a; }
  .grade-chip-letter[data-grade="D"]  { background: var(--grade-d); }
  .grade-chip-letter[data-grade="F"]  { background: var(--grade-f); }
  .grade-chip-score {
    color: var(--ink);
    font-family: var(--mono);
    font-size: var(--text-xs);
    font-weight: 600;
  }
</style>
