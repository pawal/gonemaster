<script lang="ts">
  import { ADDED_KEY, REMOVED_KEY, layoutFlow, type FlowLink } from "./flowLayout";
  import { CATEGORY_LABELS, CATEGORY_ORDER } from "$lib/report";
  import { formatCount } from "$lib/format";
  import type { DomainCategory } from "$lib/api";

  type Props = {
    order: string[];
    links: FlowLink[];
    fromLabel: string;
    toLabel: string;
    // Link for a moved pair; null leaves the ribbon unlinked.
    hrefFor: (from: string, to: string) => string | null;
    // Noun for the dropped-domain note, e.g. "grade".
    valueNoun: string;
    caption: string;
  };
  let { order, links, fromLabel, toLabel, hrefFor, valueNoun, caption }: Props = $props();

  const uid = $props.id();
  const hatchId = `flow-hatch-${uid}`;
  const layout = $derived(layoutFlow(order, links, CATEGORY_ORDER, { width: 960, height: 360, labelWidth: 116 }));
  const held = $derived(layout.ribbons.filter((r) => r.held));
  const moved = $derived(layout.ribbons.filter((r) => !r.held));

  function nodeName(key: string): string {
    if (key === ADDED_KEY) return "New";
    if (key === REMOVED_KEY) return "Removed";
    return key;
  }

  function plural(n: number): string {
    return `${formatCount(n)} ${n === 1 ? "domain" : "domains"}`;
  }

  function categoryName(category: string): string {
    return CATEGORY_LABELS[category as DomainCategory]?.toLowerCase() ?? category;
  }

  function ribbonLabel(from: string, to: string, category: string, count: number): string {
    if (from === to) return `${nodeName(from)} held: ${plural(count)}`;
    const cat = category ? `, ${categoryName(category)}` : "";
    return `${nodeName(from)} to ${nodeName(to)}${cat}: ${plural(count)}`;
  }

  // Every category of one pair, for the note under the chart.
  function pairNote(pair: string): string {
    const [from, to] = pair.split("|");
    const rows = layout.ribbons.filter((r) => r.from === from && r.to === to);
    const sum = rows.reduce((s, r) => s + r.count, 0);
    if (from === to) return `${nodeName(from)} held: ${plural(sum)}.`;
    const split = rows
      .filter((r) => r.category)
      .sort((a, b) => b.count - a.count)
      .map((r) => `${formatCount(r.count)} ${categoryName(r.category)}`)
      .join(", ");
    return `${nodeName(from)} → ${nodeName(to)}: ${plural(sum)}${split ? `; ${split}` : ""}.`;
  }

  let activePair = $state<string | null>(null);

  // Delegated from ribbons and their links, which carry data-pair.
  function track(event: Event) {
    const el = (event.target as Element | null)?.closest?.("[data-pair]");
    activePair = el?.getAttribute("data-pair") ?? null;
  }
</script>

{#if layout.ribbons.length > 0}
  <figure class="flow">
    <div class="flow-heads" aria-hidden="true">
      <span>From {fromLabel}</span>
      <span>To {toLabel}</span>
    </div>
    <svg
      viewBox="0 0 {layout.width} {layout.height}"
      role="group"
      aria-label={caption}
      onpointermove={track}
      onfocusin={track}
      onpointerleave={() => (activePair = null)}
      onfocusout={() => (activePair = null)}
    >
      <defs>
        <pattern id={hatchId} width="6" height="6" patternUnits="userSpaceOnUse" patternTransform="rotate(45)">
          <rect class="hatch-bg" width="6" height="6" />
          <line class="hatch-line" x1="0" y1="0" x2="0" y2="6" />
        </pattern>
      </defs>
      {#each held as r (r.from)}
        <path class="ribbon held" d={r.path} data-pair="{r.from}|{r.to}" />
      {/each}
      {#each moved as r (`${r.from}|${r.to}|${r.category}`)}
        {@const href = r.from === ADDED_KEY || r.to === REMOVED_KEY ? null : hrefFor(r.from, r.to)}
        {@const fill = r.category === "mixed" ? `url(#${hatchId})` : undefined}
        {#if href}
          <a {href} data-pair="{r.from}|{r.to}" aria-label={ribbonLabel(r.from, r.to, r.category, r.count)}>
            <path class="ribbon cat-{r.category || 'none'}" d={r.path} {fill} />
          </a>
        {:else}
          <path class="ribbon cat-{r.category || 'none'}" d={r.path} {fill} data-pair="{r.from}|{r.to}" />
        {/if}
      {/each}
      {#each layout.nodes as n (`${n.side}|${n.key}`)}
        <rect class="node" x={n.x} y={n.y} width={n.width} height={n.height} />
        <text
          class="node-label"
          x={n.side === "from" ? n.x - 6 : n.x + n.width + 6}
          y={n.labelY}
          text-anchor={n.side === "from" ? "end" : "start"}
        >
          <tspan class="node-key">{nodeName(n.key)}</tspan>
          <tspan class="node-count" dx="4">{formatCount(n.count)}</tspan>
        </text>
      {/each}
    </svg>
    <p class="hint flow-note">
      {#if activePair}
        {pairNote(activePair)}
      {:else}
        Ribbons run from the {valueNoun} in {fromLabel} to the {valueNoun} in {toLabel}. Select one to list its domains.
      {/if}
    </p>
    {#if layout.dropped > 0}
      <p class="hint">{plural(layout.dropped)} without a {valueNoun} on one side are not drawn.</p>
    {/if}
    <ul class="flow-legend" aria-label="Cause">
      {#each CATEGORY_ORDER as category (category)}
        <li><span class="swatch cat-{category}" aria-hidden="true"></span>{CATEGORY_LABELS[category]}</li>
      {/each}
      <li><span class="swatch held" aria-hidden="true"></span>Held</li>
    </ul>
  </figure>
{/if}

<style>
  .flow {
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .flow-heads {
    display: flex;
    justify-content: space-between;
    font-size: var(--text-xs);
    color: var(--ink-2);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .flow svg {
    display: block;
    width: 100%;
    max-width: 960px;
    height: auto;
  }
  .flow-heads {
    max-width: 960px;
  }
  .ribbon {
    opacity: 0.8;
  }
  .ribbon.held,
  .ribbon.cat-none {
    fill: var(--flow-held);
    opacity: 0.7;
  }
  .ribbon.cat-real {
    fill: var(--flow-real);
  }
  .ribbon.cat-mixed {
    stroke: var(--flow-real);
    stroke-width: 0.5;
  }
  .ribbon.cat-measurement {
    fill: var(--flow-measurement);
  }
  .ribbon.cat-unknown {
    fill: transparent;
    stroke: var(--flow-unknown);
    stroke-width: 1;
    stroke-dasharray: 3 2;
  }
  a:hover .ribbon,
  a:focus-visible .ribbon {
    opacity: 1;
    stroke: var(--ink);
    stroke-width: 1;
  }
  a:focus-visible {
    outline: none;
  }
  .hatch-bg {
    fill: var(--flow-mixed-bg);
  }
  .hatch-line {
    stroke: var(--flow-real);
    stroke-width: 2;
  }
  .node {
    fill: var(--ink-2);
  }
  .node-label {
    font-size: 11px;
    fill: var(--ink);
  }
  .node-key {
    font-weight: 600;
  }
  .node-count {
    font-family: var(--mono);
    fill: var(--ink-2);
  }
  .flow-note {
    margin: 0;
    min-height: 1.5em;
  }
  .flow-legend {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    font-size: var(--text-xs);
    color: var(--ink-2);
  }
  .flow-legend li {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }
  .swatch {
    display: inline-block;
    width: 14px;
    height: 10px;
    border-radius: 2px;
  }
  .swatch.cat-real {
    background: var(--flow-real);
  }
  .swatch.cat-mixed {
    background: repeating-linear-gradient(45deg, var(--flow-real) 0 2px, var(--flow-mixed-bg) 2px 6px);
  }
  .swatch.cat-measurement {
    background: var(--flow-measurement);
  }
  .swatch.cat-unknown {
    border: 1px dashed var(--flow-unknown);
  }
  .swatch.held {
    background: var(--flow-held);
  }
</style>
