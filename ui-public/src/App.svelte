<script>
  import { onMount, onDestroy } from "svelte";
  import { t, locale, loadCatalog } from "./i18n.js";
  import { parseHash, hashFor } from "./router.js";
  import { getLocales } from "./api.js";

  // ── Routing ────────────────────────────────────────────────────────────────
  let route = parseHash(window.location.hash);

  function onHashChange() {
    route = parseHash(window.location.hash);
  }

  // ── Theme ──────────────────────────────────────────────────────────────────
  const THEMES = ["system", "light", "dark"];
  let themeIndex = 0;

  function applyTheme(idx) {
    const theme = THEMES[idx];
    if (theme === "system") {
      document.documentElement.removeAttribute("data-theme");
    } else {
      document.documentElement.setAttribute("data-theme", theme);
    }
  }

  function cycleTheme() {
    themeIndex = (themeIndex + 1) % THEMES.length;
    applyTheme(themeIndex);
  }

  $: themeLabel = [$t("pub.theme_system"), $t("pub.theme_light"), $t("pub.theme_dark")][themeIndex];

  // ── Locale ─────────────────────────────────────────────────────────────────
  let availableLocales = ["en"];
  let resultLocale = "en";

  async function fetchLocales() {
    try {
      const res = await getLocales();
      if (res.ok) {
        const data = await res.json();
        if (Array.isArray(data)) availableLocales = data;
      }
    } catch (_) {}
  }

  async function onLocaleChange(e) {
    const code = e.target.value;
    await loadCatalog(code);
    locale.set(code);
    resultLocale = code;
  }

  // ── Navigation helpers ─────────────────────────────────────────────────────
  function goHome() {
    window.location.hash = hashFor("home").slice(1);
  }

  function goResult(publicID) {
    window.location.hash = hashFor("result", publicID).slice(1);
  }

  // ── Lifecycle ──────────────────────────────────────────────────────────────
  onMount(() => {
    window.addEventListener("hashchange", onHashChange);
    applyTheme(themeIndex);
    fetchLocales();
  });

  onDestroy(() => {
    window.removeEventListener("hashchange", onHashChange);
  });
</script>

<main>
  <header>
    <div class="header-text">
      <h1>{$t("pub.app_title")}</h1>
      <p class="subtitle">{$t("pub.app_subtitle")}</p>
    </div>
    <div class="header-controls">
      <select
        aria-label={$t("pub.locale_select_aria")}
        title={$t("pub.locale_select_title")}
        bind:value={resultLocale}
        on:change={onLocaleChange}
      >
        {#each availableLocales as code}
          <option value={code}>{code}</option>
        {/each}
      </select>
      <button
        class="theme-toggle"
        title={$t("pub.theme_cycle_title", { theme: themeLabel })}
        aria-label={$t("pub.theme_cycle_title", { theme: themeLabel })}
        on:click={cycleTheme}
      >☀</button>
    </div>
  </header>

  {#if route.view === "result"}
    <div data-view="result" data-public-id={route.publicID}>
      <!-- Results view — populated in C.6/C.7/C.8/C.9 -->
      <button class="ghost" on:click={goHome}>{$t("pub.result_new_test")}</button>
    </div>
  {:else}
    <div data-view="home">
      <!-- Home / test form — populated in C.5 -->
    </div>
  {/if}
</main>
