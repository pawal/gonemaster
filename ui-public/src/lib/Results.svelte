<script>
  import { onMount } from "svelte";
  import { t } from "../i18n.js";
  import { getResult } from "../api.js";
  import { levelClass, bannerClass, worstLevel } from "../severity.js";

  export let publicID;
  export let domain = "";
  export let locale = "en";

  let entries = [];
  let loading = true;
  let errorKey = "";

  onMount(async () => {
    try {
      const res = await getResult(publicID, locale);
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
  });

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
    {/if}
    <div class="status-banner {bannerCls}" data-testid="result-banner" role="status">
      {$t(statusKey)}
    </div>
    {#each moduleNames as moduleName}
      {@const modEntries = modules[moduleName]}
      {@const modLevel = worstLevel(modEntries)}
      <details class="module-card" data-testid="module-group">
        <summary class="module-summary">
          <span class="module-name">{moduleName}</span>
          <span class="level-pill {levelClass(modLevel)}" data-testid="module-badge">{modLevel}</span>
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
