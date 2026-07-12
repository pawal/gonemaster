<script>
  import { t } from "../i18n.js";
  import { getDnssecChain } from "../api.js";
  import { layoutChain, algoMnemonic, digestMnemonic, fmtDate } from "./dnssecChainLayout.js";

  let { publicID, domain = "" } = $props();

  // phase: idle | loading | loaded | empty | error
  let phase = $state("idle");
  let chain = $state.raw(null);

  async function load() {
    if (phase === "loading") return;
    phase = "loading";
    try {
      const res = await getDnssecChain(publicID);
      if (res.status === 404) {
        phase = "empty";
        return;
      }
      if (!res.ok) {
        phase = "error";
        return;
      }
      chain = await res.json();
      phase = "loaded";
    } catch (_) {
      phase = "error";
    }
  }

  function onToggle(e) {
    // Error re-fires on next open so the retry path is not one-shot.
    if (e.target.open && (phase === "idle" || phase === "error")) load();
  }

  let graph = $derived(phase === "loaded" && chain ? layoutChain(chain) : null);
  // Reference edges (CDS/CDNSKEY -> DNSKEY) draw after the nodes so they are
  // not hidden behind the key boxes they span.
  let mainEdges = $derived(graph ? graph.edges.filter((e) => e.kind !== "ref") : []);
  let refEdges = $derived(graph ? graph.edges.filter((e) => e.kind === "ref") : []);

  let dsSummary = $derived(
    (chain?.parent?.ds ?? [])
      .map((d) => `${d.key_tag} (${algoMnemonic(d.algorithm)}/${digestMnemonic(d.digest_type)})`)
      .join(", ") || "-"
  );
  let keySummary = $derived(
    (chain?.child?.dnskeys ?? [])
      .map((k) => {
        const size = k.key_size ? `, ${k.key_size} bit` : "";
        const revoked = k.revoked ? ` [${$t("pub.dnssec_chain_revoked")}]` : "";
        return `${k.sep ? "KSK" : "ZSK"} ${k.key_tag} (${algoMnemonic(k.algorithm)}${size})${revoked}`;
      })
      .join(", ") || "-"
  );
  let hasRevoked = $derived((chain?.child?.dnskeys ?? []).some((k) => k.revoked));

  let disagreeingServers = $derived([
    ...(chain?.parent?.servers_disagreeing ?? []),
    ...(chain?.child?.servers_disagreeing ?? []),
  ]);
  let serversWithoutDS = $derived(chain?.parent?.servers_without_ds ?? []);
  let serversWithoutDNSKEY = $derived(chain?.child?.servers_without_dnskey ?? []);
  let disagree = $derived(disagreeingServers.length > 0);
  let providedDS = $derived(chain?.parent?.ds_source === "input");
  let unsigned = $derived(chain?.status === "unsigned");
  let indeterminate = $derived(chain?.status === "indeterminate");
  let noDS = $derived(chain?.parent?.ds_source === "none" && (chain?.child?.dnskeys?.length ?? 0) > 0);
  // Claiming "serves no DNSKEY" needs a server that answered without keys.
  let noDNSKEY = $derived(
    (chain?.parent?.ds?.length ?? 0) > 0 &&
      (chain?.child?.dnskeys?.length ?? 0) === 0 &&
      (chain?.child?.servers_without_dnskey?.length ?? 0) > 0
  );
  let sigWindows = $derived(
    (chain?.child?.dnskey_rrsig ?? []).map((s) => ({
      keyTag: s.key_tag,
      from: fmtDate(s.inception),
      to: fmtDate(s.expiration),
      state: s.state,
    }))
  );
  let dsSigWindows = $derived(
    (chain?.parent?.ds_rrsig ?? []).map((s) => ({
      keyTag: s.key_tag,
      from: fmtDate(s.inception),
      to: fmtDate(s.expiration),
      state: s.state,
    }))
  );

  // status is the roll-up shown as a heading badge and the first facts line.
  let status = $derived(phase === "loaded" && chain?.status ? chain.status : "");
  let truncated = $derived(!!chain?.truncated);
  // rolloverKeys: keys not yet anchored by a DS - unanchored KSKs plus any
  // CDS/CDNSKEY signals a new key. A rollover is only meaningful when some key
  // is already anchored.
  let anyAnchored = $derived((chain?.child?.dnskeys ?? []).some((k) => k.anchored));
  let rolloverKeys = $derived([
    ...new Set([
      ...(anyAnchored ? (chain?.child?.dnskeys ?? []).filter((k) => k.sep && !k.anchored).map((k) => k.key_tag) : []),
      ...(chain?.child?.signed ?? []).flatMap((s) => (s.ds_match === "rollover" ? s.new_keys ?? [] : [])),
    ]),
  ]);

  // Custom hover tooltip: the native SVG <title> has a browser-controlled
  // delay; this one appears immediately and is positioned via the JS DOM API.
  let tipEl = $state(null);
  let tipText = $state("");
  let tipShown = $state(false);

  function showTip(e, text) {
    if (!text) return;
    tipText = text;
    tipShown = true;
    positionTip(e);
  }
  function positionTip(e) {
    if (!tipEl) return;
    const pad = 14;
    const r = tipEl.getBoundingClientRect();
    let x = e.clientX + pad;
    let y = e.clientY + pad;
    if (x + r.width > window.innerWidth) x = e.clientX - r.width - pad;
    if (y + r.height > window.innerHeight) y = e.clientY - r.height - pad;
    tipEl.style.left = `${Math.max(4, x)}px`;
    tipEl.style.top = `${Math.max(4, y)}px`;
  }
  function hideTip() {
    tipShown = false;
  }
  function onSvgMove(e) {
    const el = e.target.closest?.("[data-tip]");
    const text = el?.dataset?.tip;
    if (text) showTip(e, text);
    else hideTip();
  }

  // statusTone maps a roll-up status to a badge color: secure ok, broken bad,
  // everything else neutral.
  function statusTone(s) {
    if (s === "secure") return "ok";
    if (s === "broken") return "bad";
    return "neutral";
  }

  // sigDetail localizes a signature's state and appends its window.
  function sigDetail(sig) {
    const st = $t(`pub.dnssec_chain_state_${sig.state}`);
    return sig.from && sig.to
      ? $t("pub.dnssec_chain_tip_window", { state: st, from: sig.from, to: sig.to })
      : st;
  }

  // tipLine renders one structured tip line through i18n; sig/statusState/
  // linkState carry values that must themselves be localized before substitution.
  function tipLine(l) {
    if (l.sig) return $t(l.k, { ...l.p, detail: sigDetail(l.sig) });
    if (l.statusState) return $t(l.k, { status: $t(`pub.dnssec_chain_state_${l.statusState}`) });
    if (l.linkState) return $t(l.k, { status: $t(`pub.dnssec_chain_linkstatus_${l.linkState}`) });
    return $t(l.k, l.p);
  }

  // buildTip joins a node or edge's tip lines into the hover string.
  function buildTip(lines) {
    return (lines ?? []).map(tipLine).filter(Boolean).join("\n");
  }

  function edgeClass(edge) {
    if (edge.kind === "ref") {
      return edge.rollover ? "edge-ref-pending" : "edge-ref";
    }
    if (edge.incoming) {
      return "edge-incoming";
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
      case "key-phantom":
        return ["DNSKEY", `tag ${node.keyTag}`];
      case "parent-key":
        return ["DNSKEY", `tag ${node.keyTag}`];
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
    {#if status}
      <span class="dnssec-chain-badge badge-{statusTone(status)}" data-testid="chain-status-badge">{$t(`pub.dnssec_chain_status_${status}`)}</span>
    {/if}
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
        {#if indeterminate}
          <p class="dnssec-chain-callout callout-info" data-testid="chain-indeterminate">{$t("pub.dnssec_chain_indeterminate")}</p>
        {/if}
        {#if disagree}
          <p class="dnssec-chain-callout callout-warn" data-testid="chain-disagree">{$t("pub.dnssec_chain_disagree")}</p>
        {/if}
        {#if providedDS}
          <p class="dnssec-chain-callout callout-info" data-testid="chain-provided-ds">{$t("pub.dnssec_chain_provided_ds")}</p>
        {/if}
        {#if truncated}
          <p class="dnssec-chain-callout callout-info" data-testid="chain-truncated">{$t("pub.dnssec_chain_truncated")}</p>
        {/if}
        {#if rolloverKeys.length}
          <p class="dnssec-chain-callout callout-warn" data-testid="chain-rollover">{$t("pub.dnssec_chain_rollover", { keys: rolloverKeys.join(", ") })}</p>
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
            onmousemove={onSvgMove}
            onmouseleave={hideTip}
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
              {#if edge.d}
                <path class="chain-edge {edgeClass(edge)}" d={edge.d} marker-end="url(#chain-arrow)" data-tip={buildTip(edge.tip)}></path>
              {:else}
                <line
                  class="chain-edge {edgeClass(edge)}"
                  x1={edge.from.x}
                  y1={edge.from.y}
                  x2={edge.to.x}
                  y2={edge.to.y}
                  marker-end="url(#chain-arrow)"
                  data-tip={buildTip(edge.tip)}
                ></line>
              {/if}
            {/each}

            {#each graph.nodes as node (node.id)}
              {@const lines = nodeLines(node)}
              <g class="chain-node node-{node.kind}" class:node-unmatched={node.unmatched} class:node-revoked={node.revoked} class:node-sig-bad={node.dsSigTone === "bad"} class:node-sig-warn={node.dsSigTone === "warn"} class:node-rollover={node.rollover} class:node-incoming={node.incoming} data-tip={buildTip(node.tip)}>
                <rect x={node.x} y={node.y} width={node.w} height={node.h} rx="8" class="chain-node-box" />
                <text class="chain-node-label chain-node-title" x={node.x + node.w / 2} y={node.y + 21} text-anchor="middle">{lines[0]}</text>
                {#if lines[1]}
                  <text class="chain-node-label chain-node-sub" x={node.x + node.w / 2} y={node.y + 38} text-anchor="middle">{lines[1]}</text>
                {/if}
              </g>
            {/each}

            {#each refEdges as edge (edge.id)}
              <path class="chain-edge {edgeClass(edge)}" d={edge.d} marker-end="url(#chain-arrow)" data-tip={buildTip(edge.tip)}></path>
            {/each}
          </svg>
        </div>
        <div bind:this={tipEl} class="chain-tip" class:chain-tip-shown={tipShown} aria-hidden="true">{tipText}</div>

        <div class="chain-legend" data-testid="chain-legend">
          <span class="chain-legend-item"><span class="chain-swatch swatch-ksk"></span>{$t("pub.dnssec_chain_legend_ksk")}</span>
          <span class="chain-legend-item"><span class="chain-swatch swatch-zsk"></span>{$t("pub.dnssec_chain_legend_zsk")}</span>
          <span class="chain-legend-item"><span class="chain-swatch swatch-ds"></span>{$t("pub.dnssec_chain_legend_ds")}</span>
          <span class="chain-legend-item"><span class="chain-swatch swatch-sig"></span>{$t("pub.dnssec_chain_legend_sig")}</span>
          {#if hasRevoked}
            <span class="chain-legend-item" data-testid="chain-legend-revoked"><span class="chain-swatch swatch-revoked"></span>{$t("pub.dnssec_chain_legend_revoked")}</span>
          {/if}
        </div>
      {/if}

      <ul class="chain-facts" data-testid="chain-facts">
        {#if status}
          <li data-testid="chain-status-fact">{$t("pub.dnssec_chain_status_label")}: {$t(`pub.dnssec_chain_status_${status}`)}</li>
        {/if}
        <li>{$t("pub.dnssec_chain_parent_label")}: {chain?.parent_zone || "-"}</li>
        <li>DS: {dsSummary}</li>
        <li>{$t("pub.dnssec_chain_keys_label")}: {keySummary}</li>
        {#if disagreeingServers.length}
          <li data-testid="chain-disagree-servers">{$t("pub.dnssec_chain_disagree_servers", { servers: disagreeingServers.join(", ") })}</li>
        {/if}
        {#if serversWithoutDS.length}
          <li data-testid="chain-servers-without-ds">{$t("pub.dnssec_chain_servers_without_ds", { servers: serversWithoutDS.join(", ") })}</li>
        {/if}
        {#if serversWithoutDNSKEY.length}
          <li data-testid="chain-servers-without-dnskey">{$t("pub.dnssec_chain_servers_without_dnskey", { servers: serversWithoutDNSKEY.join(", ") })}</li>
        {/if}
        {#each dsSigWindows as sw (sw.keyTag + "-" + sw.from)}
          <li data-testid="chain-ds-sig-fact">
            RRSIG DS ({sw.keyTag}): {$t("pub.dnssec_chain_sig_window", { from: sw.from, to: sw.to })}{sw.state !== "valid" ? ` - ${sw.state}` : ""}
          </li>
        {/each}
        {#each sigWindows as sw (sw.keyTag + "-" + sw.from)}
          <li>
            RRSIG DNSKEY ({sw.keyTag}): {$t("pub.dnssec_chain_sig_window", { from: sw.from, to: sw.to })}{sw.state !== "valid" ? ` - ${sw.state}` : ""}
          </li>
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
  .dnssec-chain-badge {
    margin-left: 0.5rem;
    padding: 0.1rem 0.5rem;
    border-radius: 999px;
    border: 1px solid var(--border);
    font-size: 0.72rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.03em;
  }
  .badge-ok {
    border-color: var(--grade-a);
    color: var(--grade-a);
  }
  .badge-bad {
    border-color: var(--grade-f);
    color: var(--grade-f);
  }
  .badge-neutral {
    border-color: var(--border);
    color: var(--ink-2);
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
  .node-ds .chain-node-box,
  .node-parent-key .chain-node-box {
    stroke: var(--accent);
  }
  .node-ds-input .chain-node-box {
    stroke: var(--ink-2);
    stroke-dasharray: 4 3;
  }
  .node-unmatched .chain-node-box {
    stroke: var(--grade-f);
  }
  .node-revoked .chain-node-box {
    stroke: var(--grade-f);
    stroke-dasharray: 5 3;
  }
  .node-sig-bad .chain-node-box {
    stroke: var(--grade-f);
  }
  .node-sig-warn .chain-node-box {
    stroke: var(--grade-c);
  }
  .node-ds-ghost .chain-node-box,
  .node-key-ghost .chain-node-box,
  .node-key-phantom .chain-node-box {
    fill: transparent;
    stroke: var(--border);
    stroke-dasharray: 5 4;
  }
  .node-key-phantom .chain-node-label {
    fill: var(--ink-2);
  }
  .node-rrset .chain-node-box {
    fill: var(--surface-2);
    stroke: var(--border);
  }
  .node-rollover .chain-node-box {
    stroke: var(--grade-c);
    stroke-width: 2;
  }
  .node-incoming .chain-node-box {
    stroke: var(--grade-c);
    stroke-dasharray: 5 3;
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
  .edge-ref-pending {
    stroke: var(--grade-c);
    stroke-width: 2;
    stroke-dasharray: 4 3;
    fill: none;
  }
  .edge-incoming {
    stroke: var(--ink-2);
    stroke-width: 1.5;
    stroke-dasharray: 3 3;
    opacity: 0.45;
    fill: none;
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
  .swatch-revoked {
    border-color: var(--grade-f);
    border-style: dashed;
  }
  .chain-facts {
    margin: 0;
    padding-left: 1.1rem;
    color: var(--ink-2);
    font-size: 0.85rem;
    line-height: 1.5;
  }
  .chain-tip {
    position: fixed;
    left: 0;
    top: 0;
    z-index: 50;
    max-width: 340px;
    padding: 0.5rem 0.65rem;
    border-radius: 6px;
    background: var(--surface);
    color: var(--ink);
    border: 1px solid var(--border);
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.2);
    font-size: 0.78rem;
    line-height: 1.45;
    white-space: pre-line;
    pointer-events: none;
    visibility: hidden;
    opacity: 0;
  }
  .chain-tip-shown {
    visibility: visible;
    opacity: 1;
  }
</style>
