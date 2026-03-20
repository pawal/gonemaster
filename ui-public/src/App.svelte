<script>
  import { onMount, onDestroy } from "svelte";
  import { t, locale, loadCatalog } from "./i18n.js";
  import { parseHash, hashFor } from "./router.js";
  import { getLocales, getJob } from "./api.js";
  import TestForm from "./lib/TestForm.svelte";
  import Progress from "./lib/Progress.svelte";
  import Results from "./lib/Results.svelte";
  import ExpiredResult from "./lib/ExpiredResult.svelte";

  // ── Phase ───────────────────────────────────────────────────────────────────
  // "idle"    — form shown, no results
  // "running" — form disabled, Progress shown below
  // "done"    — form enabled, Results/ExpiredResult shown below
  let phase = "idle";
  let publicID = null;
  let jobStatus = "";
  let jobDomain = "";
  let jobFinishedAt = null;

  const TERMINAL = new Set(["succeeded", "failed", "canceled", "expired"]);

  // ── Theme ───────────────────────────────────────────────────────────────────
  let isDark = window.matchMedia?.("(prefers-color-scheme: dark)").matches ?? false;

  function applyTheme() {
    document.documentElement.setAttribute("data-theme", isDark ? "dark" : "light");
  }

  function toggleTheme() {
    isDark = !isDark;
    applyTheme();
  }

  $: themeLabel = isDark ? $t("pub.theme_dark") : $t("pub.theme_light");
  $: themeIcon = isDark ? "☀" : "☽";

  // ── Locale ──────────────────────────────────────────────────────────────────
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

  // ── Shared-link init ────────────────────────────────────────────────────────
  async function applyHash() {
    const { view, publicID: id } = parseHash(window.location.hash);
    if (view !== "result" || !id) return;
    publicID = id;
    try {
      const res = await getJob(id);
      if (!res.ok) {
        jobStatus = "expired";
        phase = "done";
        return;
      }
      const job = await res.json();
      jobDomain = job.domain ?? "";
      jobFinishedAt = job.finished_at ?? null;
      if (TERMINAL.has(job.status)) {
        jobStatus = job.status;
        phase = "done";
      } else {
        phase = "running";
      }
    } catch (_) {
      jobStatus = "expired";
      phase = "done";
    }
  }

  function onHashChange() {
    const { view, publicID: id } = parseHash(window.location.hash);
    if (view === "home") resetToIdle();
    else if (view === "result" && id && id !== publicID) applyHash();
  }

  // ── Job handlers ────────────────────────────────────────────────────────────
  function onJobCreated(e) {
    publicID = e.detail.publicID;
    jobStatus = "";
    jobDomain = "";
    jobFinishedAt = null;
    phase = "running";
    window.location.hash = hashFor("result", publicID).slice(1);
  }

  function onJobDone(e) {
    jobStatus = e.detail.status;
    jobDomain = e.detail.domain ?? "";
    jobFinishedAt = e.detail.finishedAt ?? null;
    phase = "done";
  }

  function resetToIdle() {
    phase = "idle";
    publicID = null;
    jobStatus = "";
    jobDomain = "";
    jobFinishedAt = null;
    window.location.hash = hashFor("home").slice(1);
  }

  // ── Lifecycle ───────────────────────────────────────────────────────────────
  onMount(() => {
    window.addEventListener("hashchange", onHashChange);
    applyTheme();
    fetchLocales();
    applyHash();
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
        on:click={toggleTheme}
      >{themeIcon}</button>
    </div>
  </header>

  <TestForm disabled={phase === "running"} on:jobcreated={onJobCreated} />

  {#if phase === "running"}
    <Progress
      publicID={publicID}
      on:jobdone={onJobDone}
    />
  {:else if phase === "done"}
    {#if jobStatus === "succeeded"}
      <Results
        publicID={publicID}
        domain={jobDomain}
        locale={resultLocale}
        finishedAt={jobFinishedAt}
      />
    {:else}
      <ExpiredResult on:newtest={resetToIdle} />
    {/if}
  {/if}
</main>
