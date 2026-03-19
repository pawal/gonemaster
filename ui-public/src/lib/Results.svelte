<script>
  import { t } from "../i18n.js";
  import { getResult } from "../api.js";
  import { LEVELS, levelClass, bannerClass, worstLevel } from "../severity.js";

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
    return d.getFullYear() > 2000
      ? d.toLocaleString([], { dateStyle: "medium", timeStyle: "short" })
      : "";
  })();

  let entries = [];
  let loading = true;
  let errorKey = "";
  let openModules = new Set();

  async function fetchResult(pid, loc) {
    loading = true;
    errorKey = "";
    try {
      const res = await getResult(pid, loc);
      if (!res.ok) {
        errorKey = res.status === 404 ? "pub.expired_heading" : "pub.error_unknown";
        loading = false;
        return;
      }
      const data = await res.json();
      entries = data.raw?.entries ?? [];
      loading = false;
    } catch (_) {
      errorKey = "pub.error_network";
      loading = false;
    }
  }

  $: fetchResult(publicID, locale);

  // Group entries by module, preserving insertion order.
  $: modules = entries.reduce((acc, e) => {
    if (!acc[e.module]) acc[e.module] = [];
    acc[e.module].push(e);
    return acc;
  }, {});

  $: moduleNames = Object.keys(modules);
  $: overallLevel = worstLevel(entries);
  $: bannerCls = bannerClass(overallLevel);
  $: statusKey = `pub.result_status_${bannerCls}`;
</script>

<div class="card stack" data-testid="results-view">
  {#if errorKey}
    <p class="error-box" role="alert">{$t(errorKey)}</p>
  {:else if loading}
    <p data-testid="results-loading">{$t("pub.progress_queued")}</p>
  {:else}
    {#if domain}
      <h2 class="result-heading">{$t("pub.result_heading", { domain })}</h2>
      {#if finishedStr}<p class="result-date small">{finishedStr}</p>{/if}
    {/if}
    <div class="status-banner {bannerCls}" data-testid="result-banner" role="status">
      {$t(statusKey)}
    </div>
    {#each moduleNames as moduleName}
      {@const modEntries = modules[moduleName]}
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
          {#each modEntries as entry}
            <div class="result-row" data-testid="result-row">
              <span class="level-pill {levelClass(entry.level)}">{entry.level}</span>
              <span class="result-message">{entry.message ?? entry.raw ?? ""}</span>
            </div>
          {/each}
        </div>
      </details>
    {/each}
  {/if}
</div>
