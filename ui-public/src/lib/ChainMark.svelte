<script>
  // The tone a second time, as a shape: a colourblind reader and a monochrome
  // print keep the verdict. A settled box draws nothing.
  const SHAPES = { bad: "triangle", warn: "square", ghost: "cross" };

  let { tone, x, y, size = 4 } = $props();

  const shape = $derived(SHAPES[tone] ?? "");
</script>

{#if shape === "triangle"}
  <path class="chain-mark mark-bad" d="M {x} {y - size} L {x + size} {y + size} L {x - size} {y + size} Z"></path>
{:else if shape === "square"}
  <rect class="chain-mark chain-mark-hollow mark-warn" x={x - size} y={y - size} width={size * 2} height={size * 2} />
{:else if shape === "cross"}
  <path
    class="chain-mark chain-mark-hollow mark-ghost"
    d="M {x - size} {y - size} L {x + size} {y + size} M {x + size} {y - size} L {x - size} {y + size}"
  ></path>
{/if}

<style>
  .chain-mark {
    fill: var(--ink-2);
    stroke: none;
  }
  .chain-mark-hollow {
    fill: none;
    stroke: var(--ink-2);
    stroke-width: 1.5;
  }
  .mark-bad {
    fill: var(--grade-f);
  }
  .mark-warn {
    stroke: var(--grade-c);
  }
  .mark-ghost {
    stroke: var(--muted);
  }
</style>
