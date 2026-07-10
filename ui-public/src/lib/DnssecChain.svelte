<script>
  import { t } from "../i18n.js";
  import { getDnssecChain } from "../api.js";
  import { layoutChain } from "./dnssecChainLayout.js";

  let { publicID, domain = "" } = $props();

  // phase: idle | loading | loaded | empty | error
  let phase = $state("idle");
  let chain = $state.raw(null);
  // loaded gates re-fetching on success/empty; error intentionally does not,
  // so the retry button can re-run load().
  let loaded = false;

  async function load() {
    if (phase === "loading") return;
    phase = "loading";
    try {
      const res = await getDnssecChain(publicID);
      if (res.status === 404) {
        phase = "empty";
        loaded = true;
        return;
      }
      if (!res.ok) {
        phase = "error";
        return;
      }
      chain = await res.json();
      phase = "loaded";
      loaded = true;
    } catch (_) {
      phase = "error";
    }
  }

  function onToggle(e) {
    if (e.target.open && !loaded && phase !== "loading") load();
  }

  let graph = $derived(phase === "loaded" && chain ? layoutChain(chain) : null);
  // Reference edges (CDS/CDNSKEY -> DNSKEY) draw after the nodes so they are
  // not hidden behind the key boxes they span.
  let mainEdges = $derived(graph ? graph.edges.filter((e) => e.kind !== "ref") : []);
  let refEdges = $derived(graph ? graph.edges.filter((e) => e.kind === "ref") : []);

  let dsSummary = $derived(
    (chain?.parent?.ds ?? [])
      .map((d) => `DS ${d.key_tag} (${d.algorithm}/${d.digest_type})`)
      .join(", ") || "-"
  );
  let keySummary = $derived(
    (chain?.child?.dnskeys ?? [])
      .map((k) => `${k.sep ? "KSK" : "ZSK"} ${k.key_tag} (${k.algorithm})`)
      .join(", ") || "-"
  );

  let disagree = $derived(
    (chain?.parent?.servers_disagreeing?.length ?? 0) > 0 ||
      (chain?.child?.servers_disagreeing?.length ?? 0) > 0
  );
  let providedDS = $derived(chain?.parent?.ds_source === "input");
  let unsigned = $derived(chain?.status === "unsigned");
  let noDS = $derived(chain?.parent?.ds_source === "none" && (chain?.child?.dnskeys?.length ?? 0) > 0);
  let noDNSKEY = $derived(
    (chain?.parent?.ds?.length ?? 0) > 0 && (chain?.child?.dnskeys?.length ?? 0) === 0
  );

  function fmtDate(sec) {
    if (!sec) return "";
    return new Date(sec * 1000).toISOString().slice(0, 10);
  }

  let sigWindows = $derived([
    ...(chain?.child?.dnskey_rrsig ?? [])
      .filter((s) => s.state === "valid" && s.inception && s.expiration)
      .map((s) => ({ rrset: "DNSKEY", s })),
    ...(chain?.child?.signed ?? []).flatMap((entry) =>
      (entry.rrsig ?? [])
        .filter((s) => s.state === "valid" && s.inception && s.expiration)
        .map((s) => ({ rrset: entry.type, s }))
    ),
  ]);

  function edgeClass(edge) {
    if (edge.kind === "ref") {
      return "edge-ref";
    }
    if (edge.kind === "ds") {
      return edge.status === "match" ? "edge-ok" : "edge-bad";
    }
    switch (edge.status) {
      case "valid":
        return "edge-ok";
      case "expired":
      case "bogus":
      case "no_key":
        return "edge-bad";
      case "not_yet_valid":
      case "unsupported_algorithm":
        return "edge-warn";
      default:
        return "edge-neutral";
    }
  }

  function edgeTitle(edge) {
    if (edge.kind === "ref") {
      return `${edge.rrset} -> DNSKEY ${edge.targetTag}`;
    }
    if (edge.kind === "ds") {
      return `DS ${edge.dsKeyTag} -> DNSKEY ${edge.dnskeyKeyTag ?? "?"}: ${edge.status}`;
    }
    const detail =
      edge.status === "valid" && edge.inception && edge.expiration
        ? $t("pub.dnssec_chain_sig_window", { from: fmtDate(edge.inception), to: fmtDate(edge.expiration) })
        : edge.status;
    if (edge.kind === "selfsig") return `DNSKEY ${edge.keyTag} -> DNSKEY RRset: ${detail}`;
    if (edge.kind === "keysig") return `DNSKEY ${edge.keyTag} -> DNSKEY ${edge.targetTag}: ${detail}`;
    if (edge.kind === "sig") return `DNSKEY ${edge.keyTag} -> ${edge.rrset} RRset: ${detail}`;
    return "";
  }

  function nodeLines(node) {
    switch (node.kind) {
      case "ds":
      case "ds-input":
        return ["DS", `tag ${node.keyTag}`];
      case "ds-ghost":
        return ["DS", ""];
      case "ksk":
        return ["KSK", `tag ${node.keyTag}`];
      case "zsk":
        return ["ZSK", `tag ${node.keyTag}`];
      case "key-ghost":
        return ["DNSKEY", ""];
      case "rrset":
        return [node.label, ""];
      default:
        return ["", ""];
    }
  }
</script>

<details class="score-bonus dnssec-chain-card" data-testid="dnssec-chain" ontoggle={onToggle}>
  <summary class="score-bonus-summary">
    <span class="score-bonus-chevron"></span>
    <span class="score-bonus-title">{$t("pub.dnssec_chain_heading")}</span>
    <span class="dnssec-chain-subtitle">{$t("pub.dnssec_chain_subtitle")}</span>
  </summary>

  <div class="score-bonus-list dnssec-chain-content">
    {#if phase === "loading"}
      <p class="dnssec-chain-note" data-testid="chain-loading">{$t("pub.dnssec_chain_loading")}</p>
    {:else if phase === "empty"}
      <p class="dnssec-chain-note" data-testid="chain-empty">{$t("pub.dnssec_chain_unavailable")}</p>
    {:else if phase === "error"}
      <p class="dnssec-chain-note" data-testid="chain-error">{$t("pub.error_network")}</p>
      <button type="button" class="dnssec-chain-retry" data-testid="chain-retry" onclick={load}>
        {$t("pub.dnssec_chain_retry")}
      </button>
    {:else if phase === "loaded"}
      {#if unsigned}
        <p class="dnssec-chain-note" data-testid="chain-unsigned">{$t("pub.dnssec_chain_unsigned")}</p>
      {:else if graph}
        {#if noDS}
          <p class="dnssec-chain-callout callout-info" data-testid="chain-no-ds">{$t("pub.dnssec_chain_no_ds")}</p>
        {/if}
        {#if noDNSKEY}
          <p class="dnssec-chain-callout callout-bad" data-testid="chain-no-dnskey">{$t("pub.dnssec_chain_no_dnskey")}</p>
        {/if}
        {#if disagree}
          <p class="dnssec-chain-callout callout-warn" data-testid="chain-disagree">{$t("pub.dnssec_chain_disagree")}</p>
        {/if}
        {#if providedDS}
          <p class="dnssec-chain-callout callout-info" data-testid="chain-provided-ds">{$t("pub.dnssec_chain_provided_ds")}</p>
        {/if}

        <div class="chain-scroll">
          <svg
            class="chain-svg"
            width={graph.width}
            height={graph.height}
            viewBox="0 0 {graph.width} {graph.height}"
            role="img"
            aria-label={$t("pub.dnssec_chain_aria", { domain })}
            data-testid="chain-svg"
          >
            <defs>
              <marker id="chain-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
                <path d="M0,0 L10,5 L0,10 z" fill="context-stroke" />
              </marker>
            </defs>

            {#each graph.clusters as cl (cl.id)}
              <text class="chain-cluster-label" x={cl.x} y={cl.y}>{$t(cl.labelKey)}{#if cl.name}<tspan class="chain-cluster-name"> · {cl.name}</tspan>{/if}</text>
            {/each}

            {#each mainEdges as edge (edge.id)}
              {#if edge.kind === "selfsig"}
                <path class="chain-edge {edgeClass(edge)}" d={edge.d} marker-end="url(#chain-arrow)">
                  <title>{edgeTitle(edge)}</title>
                </path>
              {:else}
                <line
                  class="chain-edge {edgeClass(edge)}"
                  x1={edge.from.x}
                  y1={edge.from.y}
                  x2={edge.to.x}
                  y2={edge.to.y}
                  marker-end="url(#chain-arrow)"
                >
                  <title>{edgeTitle(edge)}</title>
                </line>
              {/if}
            {/each}

            {#each graph.nodes as node (node.id)}
              {@const lines = nodeLines(node)}
              <g class="chain-node node-{node.kind}">
                <title>{node.titleText}</title>
                <rect x={node.x} y={node.y} width={node.w} height={node.h} rx="8" class="chain-node-box" />
                <text class="chain-node-label chain-node-title" x={node.x + node.w / 2} y={node.y + 21} text-anchor="middle">{lines[0]}</text>
                {#if lines[1]}
                  <text class="chain-node-label chain-node-sub" x={node.x + node.w / 2} y={node.y + 38} text-anchor="middle">{lines[1]}</text>
                {/if}
              </g>
            {/each}

            {#each refEdges as edge (edge.id)}
              <line
                class="chain-edge {edgeClass(edge)}"
                x1={edge.from.x}
                y1={edge.from.y}
                x2={edge.to.x}
                y2={edge.to.y}
                marker-end="url(#chain-arrow)"
              >
                <title>{edgeTitle(edge)}</title>
              </line>
            {/each}
          </svg>
        </div>

        <div class="chain-legend" data-testid="chain-legend">
          <span class="chain-legend-item"><span class="chain-swatch swatch-ksk"></span>{$t("pub.dnssec_chain_legend_ksk")}</span>
          <span class="chain-legend-item"><span class="chain-swatch swatch-zsk"></span>{$t("pub.dnssec_chain_legend_zsk")}</span>
          <span class="chain-legend-item"><span class="chain-swatch swatch-ds"></span>{$t("pub.dnssec_chain_legend_ds")}</span>
          <span class="chain-legend-item"><span class="chain-swatch swatch-sig"></span>{$t("pub.dnssec_chain_legend_sig")}</span>
        </div>
      {/if}

      <ul class="chain-facts" data-testid="chain-facts">
        <li>{$t("pub.dnssec_chain_parent_label")}: {chain?.parent_zone || "-"}</li>
        <li>DS: {dsSummary}</li>
        <li>{$t("pub.dnssec_chain_keys_label")}: {keySummary}</li>
        {#each sigWindows as item, i (i)}
          <li>{item.rrset} tag {item.s.key_tag}: {$t("pub.dnssec_chain_sig_window", { from: fmtDate(item.s.inception), to: fmtDate(item.s.expiration) })}</li>
        {/each}
      </ul>
    {/if}
  </div>
</details>

<style>
  .dnssec-chain-subtitle {
    color: var(--ink-2);
    font-size: 0.85rem;
    margin-left: 0.5rem;
  }
  .dnssec-chain-content {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }
  .dnssec-chain-note {
    color: var(--ink-2);
    margin: 0;
  }
  .dnssec-chain-callout {
    margin: 0;
    padding: 0.5rem 0.75rem;
    border-radius: 6px;
    border: 1px solid var(--border);
    font-size: 0.9rem;
  }
  .callout-info {
    color: var(--ink-2);
  }
  .callout-warn {
    border-color: var(--grade-c);
    color: var(--grade-c);
  }
  .callout-bad {
    border-color: var(--grade-f);
    color: var(--grade-f);
  }
  .dnssec-chain-retry {
    align-self: flex-start;
    padding: 0.35rem 0.9rem;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--surface-2);
    color: var(--ink);
    cursor: pointer;
  }
  .chain-scroll {
    overflow-x: auto;
    max-width: 100%;
  }
  .chain-svg {
    display: block;
  }
  .chain-cluster-label {
    fill: var(--ink-2);
    font-size: 12px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .chain-cluster-name {
    fill: var(--ink);
    font-weight: 400;
    text-transform: none;
    letter-spacing: 0;
  }
  .chain-node-box {
    fill: var(--surface);
    stroke: var(--border);
    stroke-width: 1.5;
  }
  .chain-node-label {
    font-size: 13px;
    fill: var(--ink);
  }
  .chain-node-title {
    font-weight: 700;
  }
  .chain-node-sub {
    font-size: 11px;
    fill: var(--ink-2);
  }
  .node-ksk .chain-node-box {
    stroke: var(--accent-2);
    stroke-width: 3;
  }
  .node-zsk .chain-node-box {
    stroke: var(--accent-2);
  }
  .node-ds .chain-node-box {
    stroke: var(--accent);
  }
  .node-ds-input .chain-node-box {
    stroke: var(--ink-2);
    stroke-dasharray: 4 3;
  }
  .node-ds-ghost .chain-node-box,
  .node-key-ghost .chain-node-box {
    fill: transparent;
    stroke: var(--border);
    stroke-dasharray: 5 4;
  }
  .node-rrset .chain-node-box {
    fill: var(--surface-2);
    stroke: var(--border);
  }
  .chain-edge {
    stroke-width: 2;
    fill: none;
  }
  .edge-ok {
    stroke: var(--grade-a);
  }
  .edge-bad {
    stroke: var(--grade-f);
  }
  .edge-warn {
    stroke: var(--grade-c);
  }
  .edge-neutral {
    stroke: var(--ink-2);
    stroke-dasharray: 5 4;
  }
  .edge-ref {
    stroke: var(--ink-2);
    stroke-width: 1.75;
    stroke-dasharray: 4 3;
    opacity: 0.85;
  }
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
  .swatch-sig {
    border: none;
    background: linear-gradient(90deg, var(--grade-a), var(--grade-c), var(--grade-f));
  }
  .chain-facts {
    margin: 0;
    padding-left: 1.1rem;
    color: var(--ink-2);
    font-size: 0.85rem;
    line-height: 1.5;
  }
</style>
