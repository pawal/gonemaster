<script>
  import { t } from "../i18n.js";
  import { getDnssecChain } from "../api.js";
  import { layoutChain, algoMnemonic, digestMnemonic, fmtDate, nsTone, statusTone, wrapFace } from "./dnssecChainLayout.js";
  import { chainFileName, downloadSVG, serializeSVG } from "./chainExport.js";

  // saveSVG is injected so a test can read the file without a blob URL.
  let { publicID, domain = "", saveSVG = downloadSVG } = $props();

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

  // The layout measures the header words it draws, so the component owns the
  // translations and hands them in.
  let words = $derived({
    parent: $t("pub.dnssec_chain_parent_label"),
    zone: $t("pub.dnssec_chain_zone_label"),
    root: $t("pub.dnssec_chain_root"),
    status: (s) => (KNOWN_STATUS.includes(s) ? $t(`pub.dnssec_chain_status_${s}`) : ""),
  });
  let graph = $derived(phase === "loaded" && chain ? layoutChain(chain, { words }) : null);
  let svgEl = $state(null);

  // The saved file is the diagram at rest, in the theme the reader is viewing.
  function onSave() {
    if (!svgEl || !graph) return;
    saveSVG(serializeSVG(svgEl, { width: graph.width, height: graph.height }), chainFileName(domain));
  }

  // Reference edges (CDS/CDNSKEY -> DNSKEY) draw after the nodes so they are
  // not hidden behind the key boxes they span.
  let mainEdges = $derived(graph ? graph.edges.filter((e) => e.kind !== "ref" && e.kind !== "stub") : []);
  let refEdges = $derived(graph ? graph.edges.filter((e) => e.kind === "ref") : []);
  // Stubs carry no arrowhead: they say how a signer zone is reached, not what
  // points at what, and a broken one ends in a gap rather than a target.
  let stubEdges = $derived(graph ? graph.edges.filter((e) => e.kind === "stub") : []);

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
  let hasNSNames = $derived((chain?.ns_names ?? []).length > 0);
  let undelegated = $derived(chain?.status === "undelegated");
  // Nameserver names validators reject. These drive the red callout, which is
  // how the card reports every other fault; the graph alone is too quiet.
  let bogusNSNames = $derived((chain?.ns_names ?? []).filter((n) => nsTone(n.status) === "bad").map((n) => n.name));

  let disagreeingServers = $derived([
    ...(chain?.parent?.servers_disagreeing ?? []),
    ...(chain?.child?.servers_disagreeing ?? []),
  ]);
  let serversWithoutDS = $derived(chain?.parent?.servers_without_ds ?? []);
  let serversWithoutDNSKEY = $derived(chain?.child?.servers_without_dnskey ?? []);
  // Servers serving a signature outside its validity window: a lagging
  // secondary breaks validation for the resolvers that happen to reach it.
  let staleServers = $derived([
    ...new Set([...(chain?.parent?.servers_stale ?? []), ...(chain?.child?.servers_stale ?? [])]),
  ]);
  let disagree = $derived(disagreeingServers.length > 0);
  let providedDS = $derived(chain?.parent?.ds_source === "input");
  let unsigned = $derived(chain?.status === "unsigned");
  let indeterminate = $derived(chain?.status === "indeterminate");
  // DNSKEY key tags whose signature the local verifier cannot check (unsupported
  // RSA exponent): unproven, not invalid. Drives the amber "unverifiable" callout.
  let unverifiableKeys = $derived([
    ...new Set(
      (chain?.child?.dnskey_rrsig ?? []).filter((s) => s.state === "unsupported_key").map((s) => s.key_tag)
    ),
  ]);
  // A DS naming a key that signs nothing: resolvers picking that DS fail.
  let deadAnchorKeys = $derived([
    ...new Set((chain?.links ?? []).filter((l) => l.status === "key_not_signing").map((l) => l.ds_key_tag)),
  ]);
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
  // A status this build has no name for (a newer blob version) is left out
  // rather than shown as a raw token.
  const KNOWN_STATUS = ["secure", "partial", "broken", "island", "unsigned", "indeterminate", "undelegated"];
  let status = $derived(
    phase === "loaded" && KNOWN_STATUS.includes(chain?.status) ? chain.status : ""
  );
  let truncated = $derived(!!chain?.truncated);
  let parentZoneText = $derived(chain?.parent_zone === "." ? $t("pub.dnssec_chain_root") : chain?.parent_zone || "-");
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
  // Tip box size, measured once per text change. Measuring inside the pointer
  // handler would force a layout on every move and still read the previous
  // text's box, since the DOM updates only after the handler returns.
  let tipW = 0;
  let tipH = 0;
  let pointer = { x: 0, y: 0 };

  $effect(() => {
    tipText;
    if (!tipEl) return;
    const r = tipEl.getBoundingClientRect();
    tipW = r.width;
    tipH = r.height;
    positionTip();
  });

  function showTip(e, text) {
    if (!text) return;
    pointer = { x: e.clientX, y: e.clientY };
    tipText = text;
    tipShown = true;
    positionTip();
  }
  // Flips the box to the other side of the pointer when it would overflow.
  function positionTip() {
    if (!tipEl) return;
    const pad = 14;
    let x = pointer.x + pad;
    let y = pointer.y + pad;
    if (x + tipW > window.innerWidth) x = pointer.x - tipW - pad;
    if (y + tipH > window.innerHeight) y = pointer.y - tipH - pad;
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
    if (l.nsStatus) return $t(l.k, { status: $t(`pub.dnssec_chain_nsstatus_${l.nsStatus}`) });
    if (l.statusState) return $t(l.k, { status: $t(`pub.dnssec_chain_state_${l.statusState}`) });
    if (l.linkState) return $t(l.k, { status: $t(`pub.dnssec_chain_linkstatus_${l.linkState}`) });
    return $t(l.k, l.p);
  }

  // buildTip joins a node or edge's tip lines into the hover string.
  function buildTip(lines) {
    return (lines ?? []).map(tipLine).filter(Boolean).join("\n");
  }

  // edgeTone maps a node tone onto the edge classes.
  const edgeTone = (tone) => (tone ? `edge-${tone}` : "edge-neutral");

  function edgeClass(edge) {
    if (edge.kind === "stub") {
      return edge.bad ? "edge-bad" : "edge-ok";
    }
    if (edge.kind === "nssig") {
      return edgeTone(nsTone(edge.nsStatus));
    }
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
      case "unsupported_key":
        return "edge-warn";
      default:
        return "edge-neutral";
    }
  }

  function nodeHeading(node) {
    switch (node.kind) {
      case "ds":
      case "ds-input":
      case "ds-ghost":
        return "DS";
      case "ksk":
        return "KSK";
      case "zsk":
        return "ZSK";
      case "key-ghost":
      case "key-phantom":
      case "parent-key":
        return "DNSKEY";
      case "rrset":
        return node.label;
      case "nsname":
      case "cut":
      case "cut-broken":
      case "orphan":
        return node.nameText;
      default:
        return "";
    }
  }

  // Cap height of the 13px title, which is where the block's ink starts. The
  // face has no descenders, so its ink ends on the last baseline.
  const CAP_H = 9.5;

  // Baseline step from the previous line, and class, per face line.
  const LINE_STEP = [
    { gap: 0, cls: "chain-node-title" },
    { gap: 16, cls: "chain-node-sub" },
    { gap: 14, cls: "chain-node-algo" },
    { gap: 13, cls: "chain-node-bits" },
  ];

  // Type size per face class, which is what the CSS below sets.
  const FACE_PX = {
    "chain-node-title": 13,
    "chain-node-sub": 11,
    "chain-node-algo": 10,
    "chain-node-bits": 10,
  };

  // Baseline step for a line a long translation wrapped onto.
  const WRAP_STEP = 12;

  // nodeLines returns the face lines with their baselines. The block is centred
  // on its ink rather than its baselines, so the space above the title matches
  // the space below the last line whatever the line count.
  function nodeLines(node) {
    const texts = [{ text: nodeHeading(node) }];
    if (node.keyTag != null) texts.push({ text: `tag ${node.keyTag}` });
    if (node.nsStatus) {
      texts.push({ text: $t(`pub.dnssec_chain_nsstatus_${node.nsStatus}`), extra: `chain-node-ns-${node.tone || "neutral"}` });
    }
    if (node.signerWordKey) {
      texts.push({ text: $t(node.signerWordKey), extra: node.kind === "cut" ? "" : "chain-node-ns-bad" });
    }
    if (node.algoText) texts.push({ text: node.algoText });
    if (node.bitsText) texts.push({ text: node.bitsText });
    // A line too wide for the box wraps, so the face keeps its own lines.
    const lines = [];
    texts.forEach((line, i) => {
      const step = LINE_STEP[Math.min(i, LINE_STEP.length - 1)];
      const cls = line.extra ? `${step.cls} ${line.extra}` : step.cls;
      wrapFace(line.text, node.w, FACE_PX[step.cls]).forEach((text, part) => {
        lines.push({ text, cls, gap: part === 0 ? step.gap : WRAP_STEP });
      });
    });
    const inkH = CAP_H + lines.reduce((sum, l) => sum + l.gap, 0);
    let y = node.y + (node.h - inkH) / 2 + CAP_H;
    return lines.map((line) => {
      y += line.gap;
      return { text: line.text, cls: line.cls, y };
    });
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
        {#if unverifiableKeys.length}
          <p class="dnssec-chain-callout callout-warn" data-testid="chain-unsupported-key">{$t("pub.dnssec_chain_unsupported_key", { keys: unverifiableKeys.join(", ") })}</p>
        {/if}
        {#if deadAnchorKeys.length}
          <p class="dnssec-chain-callout callout-bad" data-testid="chain-dead-anchor">{$t("pub.dnssec_chain_dead_anchor", { keys: deadAnchorKeys.join(", ") })}</p>
        {/if}
        {#if indeterminate}
          <p class="dnssec-chain-callout callout-info" data-testid="chain-indeterminate">{$t("pub.dnssec_chain_indeterminate")}</p>
        {/if}
        {#if staleServers.length}
          <p class="dnssec-chain-callout callout-warn" data-testid="chain-stale">{$t("pub.dnssec_chain_stale_secondary")}</p>
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
        {#if undelegated}
          <p class="dnssec-chain-callout callout-bad" data-testid="chain-undelegated">{$t("pub.dnssec_chain_undelegated", { parent: chain.parent_zone })}</p>
        {/if}
        {#if bogusNSNames.length}
          <p class="dnssec-chain-callout callout-bad" data-testid="chain-ns-bogus">{$t("pub.dnssec_chain_ns_bogus", { names: bogusNSNames.join(", ") })}</p>
        {/if}
        {#if rolloverKeys.length}
          <p class="dnssec-chain-callout callout-warn" data-testid="chain-rollover">{$t("pub.dnssec_chain_rollover", { keys: rolloverKeys.join(", ") })}</p>
        {/if}

        <div class="chain-toolbar">
          <button type="button" class="chain-toolbar-button" data-testid="chain-export" onclick={onSave}>
            {$t("pub.dnssec_chain_export")}
          </button>
        </div>

        <div class="chain-scroll">
          <svg
            bind:this={svgEl}
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

            {#each graph.frames as frame (frame.id)}
              <g class="chain-frame frame-{frame.tone}" data-testid="chain-frame-{frame.id}">
                <rect class="chain-frame-box" x={frame.x} y={frame.y} width={frame.w} height={frame.h} rx="10" />
                <path class="chain-frame-head" d={frame.headPath}></path>
                <text class="chain-frame-role" x={frame.header.roleX} y={frame.header.roleY}>{frame.header.roleText}</text>
                <text class="chain-frame-name" x={frame.header.nameX} y={frame.header.nameY}>{frame.header.name}</text>
                {#if frame.header.chip}
                  <rect
                    class="chain-frame-chip"
                    x={frame.header.chip.x}
                    y={frame.header.chip.y}
                    width={frame.header.chip.w}
                    height={frame.header.chip.h}
                    rx="8"
                  />
                  <text class="chain-frame-chip-label" x={frame.header.chip.textX} y={frame.header.chip.textY} text-anchor="middle">
                    {frame.header.chip.text}
                  </text>
                {/if}
              </g>
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
              <g class="chain-node node-{node.kind} node-tone-{node.tone || 'none'}" class:node-unmatched={node.unmatched} class:node-revoked={node.revoked} class:node-sig-bad={node.dsSigTone === "bad"} class:node-sig-warn={node.dsSigTone === "warn"} class:node-rollover={node.rollover} class:node-incoming={node.incoming} class:node-ns-bad={node.nsStatus && node.tone === "bad"} class:node-ns-warn={node.nsStatus && node.tone === "warn"} class:node-ns-ok={node.nsStatus && node.tone === "ok"} data-tip={buildTip(node.tip)}>
                <rect x={node.x} y={node.y} width={node.w} height={node.h} rx="8" class="chain-node-box" />
                {#each lines as line, i (i)}
                  <text class="chain-node-label {line.cls}" x={node.x + node.w / 2} y={line.y} text-anchor="middle">{line.text}</text>
                {/each}
              </g>
            {/each}

            {#each stubEdges as edge (edge.id)}
              <path class="chain-edge chain-stub {edgeClass(edge)}" d={edge.d} data-tip={buildTip(edge.tip)}></path>
              {#each edge.ticks ?? [] as tick, i (i)}
                <path class="chain-edge chain-break-tick {edgeClass(edge)}" d={tick} data-tip={buildTip(edge.tip)}></path>
              {/each}
            {/each}

            {#each refEdges as edge (edge.id)}
              <path class="chain-edge {edgeClass(edge)}" d={edge.d} marker-end="url(#chain-arrow)" data-tip={buildTip(edge.tip)}></path>
            {/each}

            <!-- Row labels last: an edge that runs past one must not cut the text. -->
            {#each graph.clusters as cl (cl.id)}
              <text class="chain-cluster-label" x={cl.x} y={cl.y}>{$t(cl.labelKey)}{#if cl.name}<tspan class="chain-cluster-name"> · {cl.name}</tspan>{/if}</text>
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
          {#if hasNSNames}
            <span class="chain-legend-item" data-testid="chain-legend-nsname"><span class="chain-swatch swatch-nsname"></span>{$t("pub.dnssec_chain_legend_nsname")}</span>
          {/if}
        </div>
      {/if}

      <ul class="chain-facts" data-testid="chain-facts">
        {#if status}
          <li data-testid="chain-status-fact">{$t("pub.dnssec_chain_status_label")}: {$t(`pub.dnssec_chain_status_${status}`)}</li>
        {/if}
        <li>{$t("pub.dnssec_chain_parent_label")}: {parentZoneText}</li>
        <li>DS: {dsSummary}</li>
        <li>{$t("pub.dnssec_chain_keys_label")}: {keySummary}</li>
        {#if staleServers.length}
          <li data-testid="chain-stale-servers">{$t("pub.dnssec_chain_stale_servers", { servers: staleServers.join(", ") })}</li>
        {/if}
        {#if disagreeingServers.length}
          <li data-testid="chain-disagree-servers">{$t("pub.dnssec_chain_disagree_servers", { servers: disagreeingServers.join(", ") })}</li>
        {/if}
        {#if serversWithoutDS.length}
          <li data-testid="chain-servers-without-ds">{$t("pub.dnssec_chain_servers_without_ds", { servers: serversWithoutDS.join(", ") })}</li>
        {/if}
        {#if serversWithoutDNSKEY.length}
          <li data-testid="chain-servers-without-dnskey">{$t("pub.dnssec_chain_servers_without_dnskey", { servers: serversWithoutDNSKEY.join(", ") })}</li>
        {/if}
        {#each dsSigWindows as sw, i (i)}
          <li data-testid="chain-ds-sig-fact">
            RRSIG DS ({sw.keyTag}): {$t("pub.dnssec_chain_sig_window", { from: sw.from, to: sw.to })}{sw.state !== "valid" ? ` - ${sw.state}` : ""}
          </li>
        {/each}
        {#each sigWindows as sw, i (i)}
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
  .badge-warn {
    border-color: var(--grade-c);
    color: var(--grade-c);
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
    /* A flex item must be free to shrink or it pushes the diagram past the card. */
    min-width: 0;
  }
  /* A drawing wider than the card scrolls; scaling it down loses the face. */
  .chain-svg {
    display: block;
    margin: 0 auto;
    font-family: var(--sans);
  }
  .chain-toolbar {
    display: flex;
    justify-content: flex-end;
  }
  .chain-toolbar-button {
    padding: 0.3rem 0.8rem;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--surface-2);
    color: var(--ink);
    font-size: 0.82rem;
    cursor: pointer;
  }
  .chain-frame-box {
    fill: none;
    stroke: var(--border);
    stroke-width: 1.5;
  }
  .chain-frame-head {
    fill: var(--surface-2);
    stroke: none;
  }
  .chain-frame-role {
    font-size: 11px;
    font-weight: 700;
    fill: var(--ink-2);
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }
  .chain-frame-name {
    font-family: var(--mono);
    font-size: 12px;
    fill: var(--ink);
  }
  .chain-frame-chip {
    fill: none;
    stroke: var(--border);
  }
  .chain-frame-chip-label {
    font-size: 10px;
    font-weight: 700;
    fill: var(--ink-2);
  }
  .frame-ok .chain-frame-box,
  .frame-ok .chain-frame-chip {
    stroke: var(--grade-a);
  }
  .frame-ok .chain-frame-chip-label {
    fill: var(--grade-a);
  }
  .frame-warn .chain-frame-box,
  .frame-warn .chain-frame-chip {
    stroke: var(--grade-c);
  }
  .frame-warn .chain-frame-chip-label {
    fill: var(--grade-c);
  }
  .frame-bad .chain-frame-box,
  .frame-bad .chain-frame-chip {
    stroke: var(--grade-f);
  }
  .frame-bad .chain-frame-chip-label {
    fill: var(--grade-f);
  }
  .chain-cluster-label {
    fill: var(--ink-2);
    font-size: 12px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    /* Halo in the card colour, so an edge behind the text stays out of it. */
    stroke: var(--surface);
    stroke-width: 3px;
    paint-order: stroke;
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
  .chain-node-ns-bad {
    fill: var(--grade-f);
    font-weight: 700;
  }
  .chain-node-ns-warn {
    fill: var(--grade-c);
    font-weight: 700;
  }
  .chain-node-ns-ok {
    fill: var(--grade-a);
  }
  .chain-node-algo,
  .chain-node-bits {
    font-size: 10px;
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
  .node-revoked .chain-node-box {
    stroke-dasharray: 5 3;
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
    stroke-width: 2;
  }
  .node-incoming .chain-node-box {
    stroke-dasharray: 5 3;
  }
  .node-cut .chain-node-box {
    fill: var(--surface-2);
    stroke: var(--accent);
  }
  .node-cut-broken .chain-node-box {
    fill: var(--surface-2);
    stroke: var(--grade-f);
    stroke-width: 2;
  }
  .node-orphan .chain-node-box {
    fill: var(--surface-2);
    stroke: var(--grade-f);
    stroke-width: 2;
    stroke-dasharray: 5 3;
  }
  .node-nsname .chain-node-box {
    fill: var(--surface);
    stroke: var(--border);
  }
  .node-ns-bad .chain-node-box {
    stroke-width: 2;
  }
  /* Tone paints the border; a flag adds only the treatment that tells two faults apart. */
  .node-tone-bad .chain-node-box {
    stroke: var(--grade-f);
  }
  .node-tone-warn .chain-node-box {
    stroke: var(--grade-c);
  }
  .node-tone-ok .chain-node-box {
    stroke: var(--grade-a);
  }
  .chain-stub {
    stroke-width: 2.5;
  }
  .chain-break-tick {
    stroke-width: 2;
    stroke-linecap: round;
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
  .swatch-nsname {
    border-color: var(--border);
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
    max-width: min(64ch, 90vw);
    overflow-wrap: anywhere;
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
