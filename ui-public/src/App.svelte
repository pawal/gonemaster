<script>
  import { onMount, onDestroy } from "svelte";
  import { t, locale, loadCatalog } from "./i18n.js";
  import { parseHash, hashFor } from "./router.js";
  import { getLocales } from "./api.js";
  import TestForm from "./lib/TestForm.svelte";
  import Progress from "./lib/Progress.svelte";
  import Results from "./lib/Results.svelte";
  import ShareButton from "./lib/ShareButton.svelte";
  import ExpiredResult from "./lib/ExpiredResult.svelte";

  // ── Routing ────────────────────────────────────────────────────────────────
  let route = parseHash(window.location.hash);

  function onHashChange() {
    const next = parseHash(window.location.hash);
    if (next.view === "result" && next.publicID !== route.publicID) {
      resetResultState();
    }
    if (next.view === "home") resetResultState();
    route = next;
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
  $: themeIcon = themeIndex === 2 ? "☀" : "🌙";

  // ── Locale ─────────────────────────────────────────────────────────────────
  const localeDisplayNames = {
    en: "English", sv: "Svenska", da: "Dansk", fi: "Suomi",
    fr: "Français", es: "Español", nb: "Norsk", sl: "Slovenščina", ja: "日本語",
  };
  const localeLabel = (code) => localeDisplayNames[code] || code;

  let availableLocales = ["en"];
  let resultLocale = "en";

  async function fetchLocales() {
    try {
      const res = await getLocales();
      if (res.ok) {
        const data = await res.json();
        if (Array.isArray(data?.locales) && data.locales.length > 0) {
          availableLocales = data.locales;
        }
      }
    } catch (_) {}
  }

  async function onLocaleChange(e) {
    const code = e.target.value;
    await loadCatalog(code);
    locale.set(code);
    resultLocale = code;
  }

  // ── Result sub-state ────────────────────────────────────────────────────────
  let jobDone = false;
  let jobStatus = "";
  let jobDomain = "";

  function resetResultState() {
    jobDone = false;
    jobStatus = "";
    jobDomain = "";
  }

  function onJobCreated(e) {
    resetResultState();
    goResult(e.detail.publicID);
  }

  function onJobDone(e) {
    jobStatus = e.detail.status;
    jobDomain = e.detail.domain ?? "";
    jobDone = true;
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
      {#if availableLocales.length > 1}
        <select
          class="locale-select"
          aria-label={$t("pub.locale_select_aria")}
          title={$t("pub.locale_select_title")}
          bind:value={resultLocale}
          on:change={onLocaleChange}
        >
          {#each availableLocales as code}
            <option value={code}>{localeLabel(code)}</option>
          {/each}
        </select>
      {/if}
      <button
        class="theme-toggle"
        title={$t("pub.theme_cycle_title", { theme: themeLabel })}
        aria-label={$t("pub.theme_cycle_title", { theme: themeLabel })}
        on:click={cycleTheme}
      >{themeIcon}</button>
    </div>
  </header>

  {#if route.view === "result"}
    <div data-view="result" data-public-id={route.publicID}>
      {#if !jobDone}
        <Progress
          publicID={route.publicID}
          on:jobdone={onJobDone}
        />
      {:else if jobStatus === "succeeded"}
        <Results
          publicID={route.publicID}
          domain={jobDomain}
          locale={resultLocale}
        />
        <div class="row result-actions">
          <ShareButton publicID={route.publicID} />
          <button class="ghost" on:click={goHome} data-testid="new-test-link">
            {$t("pub.result_new_test")}
          </button>
        </div>
      {:else}
        <ExpiredResult on:newtest={goHome} />
      {/if}
    </div>
  {:else}
    <div data-view="home">
      <TestForm on:jobcreated={onJobCreated} />
    </div>
  {/if}
</main>
