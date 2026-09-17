<script>
  import { t } from "../i18n.js";
  import "./chain.css";
  import ChainMark from "./ChainMark.svelte";

  let { hasRevoked = false, hasNSNames = false, hasSevered = false } = $props();

  // Each line item is drawn by the rule that draws the edge it stands for.
  const LINES = [
    { cls: "edge-ok", key: "pub.dnssec_chain_legend_line_ok" },
    { cls: "edge-warn", key: "pub.dnssec_chain_legend_line_warn" },
    { cls: "edge-bad", key: "pub.dnssec_chain_legend_line_bad" },
  ];

  const MARKS = [
    { tone: "bad", key: "pub.dnssec_chain_legend_mark_bad" },
    { tone: "warn", key: "pub.dnssec_chain_legend_mark_warn" },
    { tone: "ghost", key: "pub.dnssec_chain_legend_mark_ghost" },
  ];
</script>

<div class="chain-legend" data-testid="chain-legend">
  <span class="chain-legend-item"><span class="chain-swatch swatch-ksk"></span>{$t("pub.dnssec_chain_legend_ksk")}</span>
  <span class="chain-legend-item"><span class="chain-swatch swatch-zsk"></span>{$t("pub.dnssec_chain_legend_zsk")}</span>
  <span class="chain-legend-item"><span class="chain-swatch swatch-ds"></span>{$t("pub.dnssec_chain_legend_ds")}</span>
  {#if hasRevoked}
    <span class="chain-legend-item" data-testid="chain-legend-revoked"
      ><span class="chain-swatch swatch-revoked"></span>{$t("pub.dnssec_chain_legend_revoked")}</span
    >
  {/if}
  {#if hasNSNames}
    <span class="chain-legend-item" data-testid="chain-legend-nsname"
      ><span class="chain-swatch swatch-nsname"></span>{$t("pub.dnssec_chain_legend_nsname")}</span
    >
  {/if}

  {#each LINES as line (line.cls)}
    <span class="chain-legend-item" data-testid="chain-legend-{line.cls}">
      <svg class="chain-legend-swatch" viewBox="0 0 30 12" aria-hidden="true">
        <line class="chain-edge {line.cls}" x1="1" y1="6" x2="29" y2="6"></line>
      </svg>{$t(line.key)}
    </span>
  {/each}

  {#if hasSevered}
    <span class="chain-legend-item" data-testid="chain-legend-severed">
      <svg class="chain-legend-swatch" viewBox="0 0 30 12" aria-hidden="true">
        <path class="chain-edge chain-stub edge-bad" d="M 1 6 L 11 6 M 19 6 L 29 6"></path>
        <path class="chain-edge chain-break-tick edge-bad" d="M 13 10 L 17 2"></path>
      </svg>{$t("pub.dnssec_chain_legend_line_severed")}
    </span>
  {/if}

  {#each MARKS as mark (mark.tone)}
    <span class="chain-legend-item" data-testid="chain-legend-mark-{mark.tone}">
      <svg class="chain-legend-swatch" viewBox="0 0 30 12" aria-hidden="true">
        <ChainMark tone={mark.tone} x={15} y={6} size={4} />
      </svg>{$t(mark.key)}
    </span>
  {/each}
</div>

<style>
  .chain-legend {
    display: flex;
    flex-wrap: wrap;
    gap: 0.75rem 1.25rem;
    font-size: 0.82rem;
    color: var(--ink-2);
  }
  .chain-legend-item {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
  }
  .chain-legend-swatch {
    flex: none;
    width: 30px;
    height: 12px;
    overflow: visible;
  }
  .chain-swatch {
    width: 14px;
    height: 14px;
    border-radius: 3px;
    border: 2px solid var(--border);
  }
  .swatch-ksk {
    border-color: var(--accent-2);
    border-width: 3px;
  }
  .swatch-zsk {
    border-color: var(--accent-2);
  }
  .swatch-ds {
    border-color: var(--accent);
  }
  .swatch-revoked {
    border-color: var(--grade-f);
    border-style: dashed;
  }
  .swatch-nsname {
    border-color: var(--border);
  }
</style>
