<script>
  import { onMount, onDestroy } from "svelte";
  import { t, locale, loadCatalog } from "./i18n.js";
  import { parseHash, hashFor } from "./router.js";
  import { getLocales, getJob, getVersion, getInfo } from "./api.js";
  import TestForm from "./lib/TestForm.svelte";
  import Progress from "./lib/Progress.svelte";
  import Results from "./lib/Results.svelte";
  import ExpiredResult from "./lib/ExpiredResult.svelte";

  const logoSrc = `${import.meta.env.BASE_URL}gonemaster.svg`;

  // ── Phase ───────────────────────────────────────────────────────────────────
  // "idle"    - form shown, no results
  // "running" - form disabled, Progress shown below
  // "done"    - form enabled, Results/ExpiredResult shown below
  let phase = $state("idle");
  let publicID = $state(null);
  let jobStatus = $state("");
  let jobDomain = $state("");
  let jobFinishedAt = $state(null);
  let jobProgress = $state(0);

  $effect(() => {
    document.title = phase === "running" ? `${jobProgress}% Gonemaster` : "Gonemaster";
  });

  const TERMINAL = new Set(["succeeded", "failed", "canceled", "expired"]);

  // ── Theme ───────────────────────────────────────────────────────────────────
  let isDark = $state(window.matchMedia?.("(prefers-color-scheme: dark)").matches ?? false);

  function applyTheme() {
    document.documentElement.setAttribute("data-theme", isDark ? "dark" : "light");
  }

  function toggleTheme() {
    isDark = !isDark;
    applyTheme();
  }

  let themeLabel = $derived(isDark ? $t("pub.theme_dark") : $t("pub.theme_light"));
  let themeIcon = $derived(isDark ? "☀" : "☽");

  // ── Locale ──────────────────────────────────────────────────────────────────
  const localeKey = "gonemaster.public.locale.v1";
  const localeDisplayNames = {
    en: "English", sv: "Svenska", da: "Dansk", fi: "Suomi",
    fr: "Français", es: "Español", nb: "Norsk", sl: "Slovenščina", ja: "日本語",
  };
  const localeLabel = (code) => localeDisplayNames[code] || code;

  // Stored choice → first browser-preferred catalog we ship → "en".
  function pickInitialLocale() {
    if (typeof window === "undefined") return "en";
    try {
      const stored = window.localStorage.getItem(localeKey);
      if (stored && localeDisplayNames[stored]) return stored;
    } catch (_) {}
    const prefs = (typeof navigator !== "undefined" && Array.isArray(navigator.languages) && navigator.languages.length > 0)
      ? navigator.languages
      : (typeof navigator !== "undefined" && navigator.language ? [navigator.language] : []);
    for (const tag of prefs) {
      const base = String(tag).split("-")[0].toLowerCase();
      if (localeDisplayNames[base]) return base;
    }
    return "en";
  }

  let availableLocales = $state(["en"]);
  let resultLocale = $state("en");
  let versionGonemaster = $state("");
  let versionDNS = $state("");
  // Fail-safe default: hide scoring until server confirms it is enabled.
  let scoringEnabled = $state(false);
  let nameserverTimingsEnabled = $state(false);

  async function fetchLocales() {
    try {
      const res = await getLocales();
      if (res.ok) {
        const data = await res.json();
        if (Array.isArray(data?.locales) && data.locales.length > 0) {
          availableLocales = data.locales;
          if (!availableLocales.includes(resultLocale)) {
            resultLocale = "en";
            locale.set(resultLocale);
          }
        }
      }
    } catch (_) {}
  }

  async function onLocaleChange(e) {
    const code = e.target.value;
    await loadCatalog(code);
    locale.set(code);
    resultLocale = code;
    try { window.localStorage.setItem(localeKey, code); } catch (_) {}
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
  function onJobCreated(detail) {
    publicID = detail.publicID;
    jobStatus = "";
    jobDomain = "";
    jobFinishedAt = null;
    jobProgress = 0;
    phase = "running";
    window.location.hash = hashFor("result", publicID).slice(1);
  }

  function onJobDone(detail) {
    jobStatus = detail.status;
    jobDomain = detail.domain ?? "";
    jobFinishedAt = detail.finishedAt ?? null;
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
  async function fetchInfo() {
    try {
      const res = await getInfo();
      if (res.ok) {
        const data = await res.json();
        if (typeof data?.show_score_public === "boolean") {
          scoringEnabled = data.show_score_public;
        }
        if (typeof data?.show_nameserver_timings_public === "boolean") {
          nameserverTimingsEnabled = data.show_nameserver_timings_public;
        }
      }
    } catch (_) { /* keep false - fail-safe */ }
  }

  async function fetchVersion() {
    try {
      const res = await getVersion();
      if (res.ok) {
        const data = await res.json();
        if (data?.gonemaster) versionGonemaster = data.gonemaster;
        if (data?.dns) versionDNS = data.dns;
      }
    } catch (_) {}
  }

  onMount(() => {
    window.addEventListener("hashchange", onHashChange);
    applyTheme();
    const initial = pickInitialLocale();
    resultLocale = initial;
    locale.set(initial);
    loadCatalog(initial);
    fetchLocales();
    fetchVersion();
    fetchInfo();
    applyHash();
  });

  onDestroy(() => {
    window.removeEventListener("hashchange", onHashChange);
  });
</script>

<main>
  <header>
    <div class="header-text">
      <h1 class="brand-mark">
        <img class="brand-logo" src={logoSrc} alt="gonemaster" />
      </h1>
    </div>
    <div class="header-controls">
      {#if availableLocales.length > 1}
        <select
          class="locale-select"
          aria-label={$t("pub.locale_select_aria")}
          title={$t("pub.locale_select_title")}
          bind:value={resultLocale}
          onchange={onLocaleChange}
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
        onclick={toggleTheme}
      >{themeIcon}</button>
    </div>
  </header>

  <TestForm disabled={phase === "running"} onjobcreated={onJobCreated} />

  {#if phase === "running"}
    <Progress
      publicID={publicID}
      bind:progress={jobProgress}
      onjobdone={onJobDone}
    />
  {:else if phase === "done"}
    {#if jobStatus === "succeeded"}
      <Results
        publicID={publicID}
        domain={jobDomain}
        locale={resultLocale}
        finishedAt={jobFinishedAt}
        {scoringEnabled}
        {nameserverTimingsEnabled}
      />
    {:else}
      <ExpiredResult onnewtest={resetToIdle} />
    {/if}
  {/if}

  {#if versionGonemaster}
    <footer class="version-footer">
      <div class="version-box">
        <span class="version-row"><span class="version-name">gonemaster</span>{versionGonemaster}</span>
        {#if versionDNS}
          <span class="version-row"><span class="version-name">miekg/dns</span>{versionDNS}</span>
        {/if}
      </div>
    </footer>
  {/if}

  <a class="fork-ribbon right-bottom fixed" href="https://codeberg.org/pawal/gonemaster" data-ribbon="Fork me on Codeberg" title="Fork me on Codeberg">Fork me on Codeberg</a>
</main>
