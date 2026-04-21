<script>
  import { onMount, onDestroy } from "svelte";
  import { t } from "../i18n.js";
  import { getResult } from "../api.js";
  import { LEVELS, levelClass, bannerClass, worstLevel } from "../severity.js";
  import ShareButton from "./ShareButton.svelte";

  // Force all <details> open before printing, restore after.
  let closedBeforePrint = [];
  function onBeforePrint() {
    closedBeforePrint = [...document.querySelectorAll("details:not([open])")];
    closedBeforePrint.forEach(d => d.setAttribute("open", ""));
  }
  function onAfterPrint() {
    closedBeforePrint.forEach(d => d.removeAttribute("open"));
    closedBeforePrint = [];
  }
  onMount(() => {
    window.addEventListener("beforeprint", onBeforePrint);
    window.addEventListener("afterprint", onAfterPrint);
  });
  onDestroy(() => {
    window.removeEventListener("beforeprint", onBeforePrint);
    window.removeEventListener("afterprint", onAfterPrint);
  });

  function levelCounts(arr) {
    const counts = {};
    for (const e of arr) {
      const l = e.level?.toUpperCase();
      if (l) counts[l] = (counts[l] ?? 0) + 1;
    }
    return LEVELS.filter((l) => counts[l]).map((l) => ({ level: l, count: counts[l] }));
  }

  let { publicID, domain = "", locale = "en", finishedAt = null, scoringEnabled = false, nameserverTimingsEnabled = true } = $props();

  let finishedStr = $derived((() => {
    if (!finishedAt) return "";
    const d = new Date(finishedAt);
    return d.getFullYear() > 2000 ? d.toISOString().slice(0, 16) : "";
  })());

  let entries = $state([]);
  let nameserverTimings = $state([]);
  let score = $state(null);
  let loading = $state(true);
  let errorKey = $state("");
  const NOTICE_IDX = LEVELS.indexOf("NOTICE");
  function isNoticeOrAbove(level) {
    return LEVELS.indexOf(level?.toUpperCase()) >= NOTICE_IDX;
  }

  let openModules = $state(new Set());
  let openTestcases = $state(new Set());

  async function fetchResult(pid, loc) {
    loading = true;
    errorKey = "";
    score = null;
    try {
      const res = await getResult(pid, loc);
      if (!res.ok) {
        errorKey = res.status === 404 ? "pub.expired_heading" : "pub.error_unknown";
        nameserverTimings = [];
        loading = false;
        return;
      }
      const data = await res.json();
      entries = data.raw?.entries ?? [];
      nameserverTimings = data.nameserver_timings ?? [];
      score = data.score ?? null;
      loading = false;
    } catch (_) {
      errorKey = "pub.error_network";
      nameserverTimings = [];
      loading = false;
    }
  }

  $effect(() => {
    fetchResult(publicID, locale);
  });

  // Group entries by module -> testcase, preserving insertion order.
  let modules = $derived(entries.reduce((acc, e) => {
    if (!acc[e.module]) acc[e.module] = {};
    const tc = e.testcase || "Unspecified";
    if (!acc[e.module][tc]) acc[e.module][tc] = [];
    acc[e.module][tc].push(e);
    return acc;
  }, {}));

  function allModuleEntries(mod) {
    return Object.values(mod).flat();
  }

  let moduleNames = $derived(Object.keys(modules).sort((a, b) => {
    if (a === "System") return -1;
    if (b === "System") return 1;
    return 0;
  }));
  let overallLevel = $derived(worstLevel(entries));
  let bannerCls = $derived(bannerClass(overallLevel));
  let statusKey = $derived(`pub.result_status_${bannerCls}`);

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

  // Maps a grade letter to a CSS variable name for consistent grade coloring.
  const GRADE_COLORS = {
    "A+": "var(--grade-aplus)", "A": "var(--grade-a)", "B": "var(--grade-b)",
    "C": "var(--grade-c)", "D": "var(--grade-d)", "F": "var(--grade-f)",
  };

  // Single grade color for all category bars (cohesive, not per-bar).
  let gradeColor = $derived(GRADE_COLORS[score?.grade] ?? "var(--grade-a)");

  // Sorted category entries in display order.
  let sortedCats = $derived(score?.categories
    ? CAT_ORDER.filter(c => c in score.categories).map(c => [c, score.categories[c]])
    : []);

  // Translate a bonus criterion key via i18n, falling back to humanised key.
  // Reactive so the template re-evaluates when $t changes (e.g. on locale switch).
  let bonusLabel = $derived((key) => {
    const k = `pub.score_bonus_${key}`;
    const s = $t(k);
    return s !== k ? s : key.replace(/_/g, " ");
  });

  // Number of unmet bonus criteria (null = not applicable, counts as met).
  let bonusMissing = $derived(score?.bonus?.criteria
    ? Object.entries(score.bonus.criteria)
        .filter(([k, v]) => k !== "no_warnings_or_errors" && v === false).length
    : 0);

  function formatTimingMs(value) {
    return `${Math.round(value)}`;
  }
</script>

<div class="card stack" data-testid="results-view">
  {#if errorKey}
    <p class="error-box" role="alert">{$t(errorKey)}</p>
  {:else if loading}
    <p data-testid="results-loading">{$t("pub.progress_queued")}</p>
  {:else}
    {#if scoringEnabled && score && domain}
      <div class="score-card" data-testid="score-card">
        <div class="score-left">
          <div class="grade-badge" data-grade={score.grade}>
            <span class="grade-letter">{score.grade}</span>
          </div>
          <div class="score-meta">
            <div class="score-domain">{domain}</div>
            <div class="score-number">{score.score}<span class="score-denom">/100</span></div>
            <div class="score-meta-row">
              {#if finishedStr}<span class="result-date">{finishedStr}</span>{/if}
              <ShareButton {publicID} {domain} score={scoringEnabled ? score : null} />
              <button type="button" class="print-button" onclick={() => window.print()}>{$t("pub.result_print")}</button>
            </div>
          </div>
        </div>
        {#if sortedCats.length > 0}
          <div class="score-cats">
            {#each sortedCats as [cat, res], i}
              <div class="score-cat-row" data-untested={res.tested === false ? "" : undefined}>
                <span class="score-cat-name">{catLabel(cat)}</span>
                <div class="score-cat-bar-track">
                  <div
                    class="score-cat-bar"
                    style="--bar-pct:{res.tested === false ? 0 : res.score}%; --bar-color:{gradeColor}; animation-delay:{i * 60}ms"
                  ></div>
                </div>
                <span class="score-cat-num">{res.tested === false ? "-" : res.score}</span>
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
            {#each Object.entries(score.bonus.criteria).filter(([k]) => k !== "no_warnings_or_errors") as [key, val]}
              <div class="score-bonus-item" data-met={val === null ? "na" : val ? "yes" : "no"}>
                <span class="score-bonus-icon">{val === null ? "–" : val ? "✓" : "✗"}</span>
                <span>{bonusLabel(key)}</span>
              </div>
            {/each}
          </div>
        </details>
      {/if}
    {:else if domain}
      <div class="result-heading-row">
        <div>
          <h2 class="result-heading">{$t("pub.result_heading", { domain })}</h2>
          {#if finishedStr}<p class="result-date small">{finishedStr}</p>{/if}
        </div>
        <ShareButton {publicID} {domain} />
        <button type="button" class="print-button" onclick={() => window.print()}>{$t("pub.result_print")}</button>
      </div>
    {/if}

    <div class="status-banner {bannerCls}" data-testid="result-banner" role="status">
      {$t(statusKey)}
    </div>
    {#each moduleNames as moduleName}
      {@const mod = modules[moduleName]}
      {@const modEntries = allModuleEntries(mod)}
      {@const modLevel = worstLevel(modEntries)}
      {@const testcases = Object.keys(mod)}
      <details
        class="module-card"
        data-testid="module-group"
        data-level={modLevel.toLowerCase()}
        open={openModules.has(moduleName)}
        ontoggle={(e) => { if (e.target.open) openModules.add(moduleName); else openModules.delete(moduleName); openModules = new Set(openModules); }}
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
            {@const tcLongKey = `pub.tc_desc.${tc.toLowerCase()}`}
            {@const tcLong = $t(tcLongKey)}
            {@const tcTitle = tcDesc !== tcKey ? tcDesc : tc}
            {@const tcLongText = tcLong !== tcLongKey ? tcLong : null}
            {#if tc !== "Unspecified"}
              <details
                class="testcase-group"
                data-testid="testcase-group"
                open={isNoticeOrAbove(tcLevel) || openTestcases.has(tc)}
                ontoggle={(e) => { if (e.target.open) openTestcases.add(tc); else openTestcases.delete(tc); openTestcases = new Set(openTestcases); }}
              >
                <summary class="testcase-summary">
                  <span class="testcase-chevron"></span>
                  <span class="testcase-desc">{tcTitle}</span>
                  <span class="testcase-badge">
                    <span class="level-pill {levelClass(tcLevel)}">{tcLevel}</span>
                  </span>
                </summary>
                <div class="testcase-entries">
                  {#each tcEntries.filter(e => e.message && e.message !== e.raw) as entry}
                    {@const modKey = (entry.module ?? "").toLowerCase()}
                    {@const headerKey = entry.tag && modKey ? `pub.tag.${modKey}.${entry.tag}.header` : null}
                    {@const descKey = entry.tag && modKey ? `pub.tag.${modKey}.${entry.tag}.desc` : null}
                    {@const headerText = headerKey ? $t(headerKey) : null}
                    {@const descText = descKey ? $t(descKey) : null}
                    {@const tagHeader = headerText && headerText !== headerKey ? headerText : null}
                    {@const tagDesc = descText && descText !== descKey ? descText : null}
                    {@const hasExplanation = tcLongText || tagDesc}
                    <div class="result-row" data-testid="result-row">
                      <span class="result-row-caption" data-testid="result-row-caption">{tcTitle}</span>
                      <div class="result-row-main">
                        <span class="level-pill {levelClass(entry.level)}">{entry.level}</span>
                        <span class="result-message">
                          {#if tagHeader}<strong class="result-tag-header" data-testid="result-tag-header">{tagHeader}</strong>{" — "}{/if}{entry.message}
                        </span>
                      </div>
                      {#if hasExplanation}
                        <details class="result-explanation" data-testid="result-explanation">
                          <summary class="result-explanation-summary">
                            <span class="result-explanation-chevron"></span>
                            <span>{$t("pub.about_finding")}</span>
                          </summary>
                          <div class="result-explanation-body">
                            {#if tcLongText}
                              <p class="result-explanation-para result-explanation-para-test" data-testid="result-explanation-test">{tcLongText}</p>
                            {/if}
                            {#if tagDesc}
                              <p class="result-explanation-para result-explanation-para-tag" data-testid="result-explanation-tag">{tagDesc}</p>
                            {/if}
                          </div>
                        </details>
                      {/if}
                    </div>
                  {/each}
                </div>
              </details>
            {:else}
              {#each tcEntries.filter(e => e.message && e.message !== e.raw) as entry}
                {@const modKey = (entry.module ?? "").toLowerCase()}
                {@const headerKey = entry.tag && modKey ? `pub.tag.${modKey}.${entry.tag}.header` : null}
                {@const descKey = entry.tag && modKey ? `pub.tag.${modKey}.${entry.tag}.desc` : null}
                {@const headerText = headerKey ? $t(headerKey) : null}
                {@const descText = descKey ? $t(descKey) : null}
                {@const tagHeader = headerText && headerText !== headerKey ? headerText : null}
                {@const tagDesc = descText && descText !== descKey ? descText : null}
                {@const hasExplanation = tcLongText || tagDesc}
                <div class="result-row" data-testid="result-row">
                  <div class="result-row-main">
                    <span class="level-pill {levelClass(entry.level)}">{entry.level}</span>
                    <span class="result-message">
                      {#if tagHeader}<strong class="result-tag-header" data-testid="result-tag-header">{tagHeader}</strong>{" — "}{/if}{entry.message}
                    </span>
                  </div>
                  {#if hasExplanation}
                    <details class="result-explanation" data-testid="result-explanation">
                      <summary class="result-explanation-summary">
                        <span class="result-explanation-chevron"></span>
                        <span>{$t("pub.about_finding")}</span>
                      </summary>
                      <div class="result-explanation-body">
                        {#if tcLongText}
                          <p class="result-explanation-para result-explanation-para-test" data-testid="result-explanation-test">{tcLongText}</p>
                        {/if}
                        {#if tagDesc}
                          <p class="result-explanation-para result-explanation-para-tag" data-testid="result-explanation-tag">{tagDesc}</p>
                        {/if}
                      </div>
                    </details>
                  {/if}
                </div>
              {/each}
            {/if}
          {/each}
        </div>
      </details>
    {/each}
    {#if nameserverTimingsEnabled && nameserverTimings.length > 0}
      <details class="score-bonus ns-timings-card" data-testid="nameserver-timings">
        <summary class="score-bonus-summary ns-timings-summary">
          <span class="score-bonus-chevron"></span>
          <span class="score-bonus-title">{$t("pub.ns_timing_heading")}</span>
          <span class="ns-timings-summary-text">{$t("pub.ns_timing_subtitle")}</span>
        </summary>
        <div class="score-bonus-list ns-timings-content">
          <table class="ns-timings-table">
            <thead>
              <tr>
                <th scope="col">{$t("pub.ns_timing_nameserver")}</th>
                <th scope="col">{$t("pub.ns_timing_ip")}</th>
                <th scope="col" class="ns-timings-num">{$t("pub.ns_timing_avg_ms")}</th>
                <th scope="col" class="ns-timings-num">{$t("pub.ns_timing_min_ms")}</th>
                <th scope="col" class="ns-timings-num">{$t("pub.ns_timing_max_ms")}</th>
                <th scope="col" class="ns-timings-num">{$t("pub.ns_timing_samples")}</th>
              </tr>
            </thead>
            <tbody>
              {#each nameserverTimings as item}
                <tr data-testid="nameserver-timing-row">
                  <td class="ns-timings-name">{item.nameserver}</td>
                  <td class="ns-timings-ip">{item.address}</td>
                  <td class="ns-timings-num ns-timings-avg">{formatTimingMs(item.avg_ms)}</td>
                  <td class="ns-timings-num">{formatTimingMs(item.min_ms)}</td>
                  <td class="ns-timings-num">{formatTimingMs(item.max_ms)}</td>
                  <td class="ns-timings-num">{item.count}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </details>
    {/if}
  {/if}
</div>
