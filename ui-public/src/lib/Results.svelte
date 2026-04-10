<script>
  import { t } from "../i18n.js";
  import { getResult } from "../api.js";
  import { LEVELS, levelClass, bannerClass, worstLevel } from "../severity.js";
  import ShareButton from "./ShareButton.svelte";

  function levelCounts(arr) {
    const counts = {};
    for (const e of arr) {
      const l = e.level?.toUpperCase();
      if (l) counts[l] = (counts[l] ?? 0) + 1;
    }
    return LEVELS.filter((l) => counts[l]).map((l) => ({ level: l, count: counts[l] }));
  }

  export let publicID;
  export let domain = "";
  export let locale = "en";
  export let finishedAt = null;

  $: finishedStr = (() => {
    if (!finishedAt) return "";
    const d = new Date(finishedAt);
    return d.getFullYear() > 2000 ? d.toISOString().slice(0, 16) : "";
  })();

  let entries = [];
  let tcDescs = {};
  let score = null;
  let loading = true;
  let errorKey = "";
  const NOTICE_IDX = LEVELS.indexOf("NOTICE");
  function isNoticeOrAbove(level) {
    return LEVELS.indexOf(level?.toUpperCase()) >= NOTICE_IDX;
  }

  let openModules = new Set();
  let openTestcases = new Set();

  async function fetchResult(pid, loc) {
    loading = true;
    errorKey = "";
    score = null;
    try {
      const res = await getResult(pid, loc);
      if (!res.ok) {
        errorKey = res.status === 404 ? "pub.expired_heading" : "pub.error_unknown";
        loading = false;
        return;
      }
      const data = await res.json();
      entries = data.raw?.entries ?? [];
      tcDescs = data.testcase_descriptions ?? {};
      score = data.score ?? null;
      loading = false;
    } catch (_) {
      errorKey = "pub.error_network";
      loading = false;
    }
  }

  $: fetchResult(publicID, locale);

  // Group entries by module → testcase, preserving insertion order.
  $: modules = entries.reduce((acc, e) => {
    if (!acc[e.module]) acc[e.module] = {};
    const tc = e.testcase || "Unspecified";
    if (!acc[e.module][tc]) acc[e.module][tc] = [];
    acc[e.module][tc].push(e);
    return acc;
  }, {});

  function allModuleEntries(mod) {
    return Object.values(mod).flat();
  }

  $: moduleNames = Object.keys(modules).sort((a, b) => {
    if (a === "System") return -1;
    if (b === "System") return 1;
    return 0;
  });
  $: overallLevel = worstLevel(entries);
  $: bannerCls = bannerClass(overallLevel);
  $: statusKey = `pub.result_status_${bannerCls}`;

  // ── Scoring helpers ──────────────────────────────────────────────────────────

  // Fixed display order for scoring categories.
  const CAT_ORDER = ["dnssec", "nameserver_health", "connectivity", "zone_consistency"];

  const CAT_LABELS = {
    dnssec:             "DNSSEC",
    nameserver_health:  "Nameserver",
    connectivity:       "Connectivity",
    zone_consistency:   "Zone",
  };

  function catLabel(cat) {
    return CAT_LABELS[cat] ?? cat.replace(/_/g, " ");
  }

  // Returns a color for a category sub-score bar (0–100).
  function categoryColor(s) {
    if (s >= 90) return "#22c55e";
    if (s >= 75) return "#84cc16";
    if (s >= 60) return "#ca8a04";
    if (s >= 40) return "#ea580c";
    return "#dc2626";
  }

  // Sorted category entries in display order.
  $: sortedCats = score?.categories
    ? CAT_ORDER.filter(c => c in score.categories).map(c => [c, score.categories[c]])
    : [];

  // Bonus criterion display labels.
  const BONUS_LABELS = {
    no_warnings_or_errors:  "No warnings or errors",
    dnssec_enabled:         "DNSSEC enabled",
    strong_algorithm:       "Strong algorithm (ECDSA / Ed25519)",
    nsec3_non_optout:       "NSEC3 without opt-out",
    cds_cdnskey_published:  "CDS / CDNSKEY published",
    ipv6_all_nameservers:   "IPv6 on all nameservers",
    as_diversity:           "Nameserver AS diversity",
  };

  // Number of unmet bonus criteria (null = not applicable, counts as met).
  $: bonusMissing = score?.bonus?.criteria
    ? Object.values(score.bonus.criteria).filter(v => v === false).length
    : 0;
</script>

<div class="card stack" data-testid="results-view">
  {#if errorKey}
    <p class="error-box" role="alert">{$t(errorKey)}</p>
  {:else if loading}
    <p data-testid="results-loading">{$t("pub.progress_queued")}</p>
  {:else}
    {#if domain}
      <div class="result-heading-row">
        <div>
          <h2 class="result-heading">{$t("pub.result_heading", { domain })}</h2>
          {#if finishedStr}<p class="result-date small">{finishedStr}</p>{/if}
        </div>
        <ShareButton {publicID} />
      </div>
    {/if}

    {#if score}
      <div class="score-card" data-testid="score-card">
        <div class="score-left">
          <div class="grade-badge" data-grade={score.grade}>
            <span class="grade-letter">{score.grade}</span>
          </div>
          <div class="score-meta">
            <div class="score-number">{score.score}<span class="score-denom">/100</span></div>
            <div class="score-label">{$t("pub.score_label")}</div>
          </div>
        </div>
        {#if sortedCats.length > 0}
          <div class="score-cats">
            {#each sortedCats as [cat, res], i}
              <div class="score-cat-row">
                <span class="score-cat-name">{catLabel(cat)}</span>
                <div class="score-cat-bar-track">
                  <div
                    class="score-cat-bar"
                    style="--bar-pct:{res.score}%; --bar-color:{categoryColor(res.score)}; animation-delay:{i * 60}ms"
                  ></div>
                </div>
                <span class="score-cat-num">{res.score}</span>
              </div>
            {/each}
          </div>
        {/if}
      </div>
      {#if score.disabled_stacks?.length > 0}
        <p class="score-partial-notice">{$t("pub.score_partial", { stacks: score.disabled_stacks.join(", ") })}</p>
      {/if}
      {#if score.bonus?.criteria && Object.keys(score.bonus.criteria).length > 0}
        <details class="score-bonus">
          <summary class="score-bonus-summary">
            <span class="score-bonus-chevron"></span>
            <span class="score-bonus-title">{$t("pub.score_aplus_criteria")}</span>
            <span class="score-bonus-status" data-met={score.bonus.eligible ? "yes" : "no"}>
              {score.bonus.eligible ? $t("pub.score_aplus_achieved") : $t("pub.score_aplus_missing", { n: bonusMissing })}
            </span>
          </summary>
          <div class="score-bonus-list">
            {#each Object.entries(score.bonus.criteria) as [key, val]}
              <div class="score-bonus-item" data-met={val === null ? "na" : val ? "yes" : "no"}>
                <span class="score-bonus-icon">{val === null ? "–" : val ? "✓" : "✗"}</span>
                <span>{BONUS_LABELS[key] ?? key.replace(/_/g, " ")}</span>
              </div>
            {/each}
          </div>
        </details>
      {/if}
    {/if}

    <div class="status-banner {bannerCls}" data-testid="result-banner" role="status">
      {$t(statusKey)}
    </div>
    {#each moduleNames as moduleName}
      {@const mod = modules[moduleName]}
      {@const modEntries = allModuleEntries(mod)}
      {@const testcases = Object.keys(mod)}
      <details
        class="module-card"
        data-testid="module-group"
        open={openModules.has(moduleName)}
        on:toggle={(e) => { if (e.target.open) openModules.add(moduleName); else openModules.delete(moduleName); openModules = openModules; }}
      >
        <summary class="module-summary">
          <span class="module-chevron"></span>
          <span class="module-name">{moduleName}</span>
          <span class="module-badges">
            {#each levelCounts(modEntries) as { level, count }}
              <span class="level-pill {levelClass(level)}" data-testid="module-badge">{level} {count}</span>
            {/each}
          </span>
        </summary>
        <div class="module-entries">
          {#each testcases as tc}
            {@const tcEntries = mod[tc]}
            {@const tcLevel = worstLevel(tcEntries)}
            {@const tcKey = `pub.tc.${tc.toLowerCase()}`}
            {@const tcDesc = $t(tcKey)}
            {#if tc !== "Unspecified"}
              <details
                class="testcase-group"
                data-testid="testcase-group"
                open={isNoticeOrAbove(tcLevel) || openTestcases.has(tc)}
                on:toggle={(e) => { if (e.target.open) openTestcases.add(tc); else openTestcases.delete(tc); openTestcases = openTestcases; }}
              >
                <summary class="testcase-summary">
                  <span class="testcase-chevron"></span>
                  <span class="testcase-desc">{tcDesc !== tcKey ? tcDesc : tc}</span>
                  <span class="testcase-badge">
                    <span class="level-pill {levelClass(tcLevel)}">{tcLevel}</span>
                  </span>
                </summary>
                <div class="testcase-entries">
                  {#each tcEntries.filter(e => e.message && e.message !== e.raw) as entry}
                    <div class="result-row" data-testid="result-row">
                      <span class="level-pill {levelClass(entry.level)}">{entry.level}</span>
                      <span class="result-message">{entry.message}</span>
                    </div>
                  {/each}
                </div>
              </details>
            {:else}
              {#each tcEntries.filter(e => e.message && e.message !== e.raw) as entry}
                <div class="result-row" data-testid="result-row">
                  <span class="level-pill {levelClass(entry.level)}">{entry.level}</span>
                  <span class="result-message">{entry.message}</span>
                </div>
              {/each}
            {/if}
          {/each}
        </div>
      </details>
    {/each}
  {/if}
</div>
