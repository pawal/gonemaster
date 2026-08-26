<script>
  import { onMount, onDestroy } from "svelte";
  import { t, locale, loadCatalog } from "./i18n.js";
  import { parsePath, pathFor, navigate, upgradeLegacyHash, isPlainClick, readLang, writeLang } from "./router.js";
  import { getLocales, getJob, getVersion, getInfo } from "./api.js";
  import TestForm from "./lib/TestForm.svelte";
  import Progress from "./lib/Progress.svelte";
  import Results from "./lib/Results.svelte";
  import ExpiredResult from "./lib/ExpiredResult.svelte";
  import RecentTests from "./lib/RecentTests.svelte";
  import { loadEntries, addEntry, setGrade, removeEntry, clearEntries } from "./lib/history.js";

  const logoSrc = `${import.meta.env.BASE_URL}gonemaster.svg`;

  // Before any routing: old shared links are hash-form.
  upgradeLegacyHash();

  // Bumped whenever the user should be sent back to the domain input: initial
  // home load and every transition back to idle ("new test"). Starts at 0 for
  // share-link visits to a result so we don't steal focus from the result.
  const initialView = parsePath(window.location.pathname).view;
  let focusSignal = $state(initialView === "home" ? 1 : 0);

  // Bumped when a result callout asks to test the parent zone: prefills and
  // flashes the domain input so the user can confirm with Test.
  let prefillDomain = $state("");
  let prefillSignal = $state(0);
  function onTestParent(parent) {
    prefillDomain = parent;
    prefillSignal += 1;
  }

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

  // ── Recent tests history ────────────────────────────────────────────────────
  let historyEntries = $state(loadEntries());
  // True only for runs created in this tab; share-link visits stay unrecorded.
  let startedHere = false;
  // Grade of the result on screen, for the document title.
  let resultGrade = $state("");

  function onScore(detail) {
    historyEntries = setGrade(detail.publicID, detail.grade);
    resultGrade = detail.grade;
  }

  function onClearHistory() {
    historyEntries = clearEntries();
  }

  // Names the result so tabs, bookmarks, history and text browsers are readable.
  $effect(() => {
    if (phase === "running") {
      document.title = `${jobProgress}% Gonemaster`;
    } else if (phase === "done" && jobStatus === "succeeded" && jobDomain) {
      document.title = resultGrade
        ? $t("pub.doc_title_grade", { domain: jobDomain, grade: resultGrade })
        : $t("pub.doc_title_result", { domain: jobDomain });
    } else {
      document.title = "Gonemaster";
    }
  });

  // Keeps <html lang> on the active catalog so screen readers pick the right voice.
  $effect(() => {
    document.documentElement.lang = $locale;
  });

  const TERMINAL = new Set(["succeeded", "failed", "canceled", "expired"]);

  // ── Theme ───────────────────────────────────────────────────────────────────
  const themeKey = "gonemaster.public.theme.v1";

  // Stored choice → system prefers-color-scheme → light.
  function pickInitialTheme() {
    if (typeof window !== "undefined") {
      try {
        const stored = window.localStorage.getItem(themeKey);
        if (stored === "dark" || stored === "light") return stored === "dark";
      } catch (_) {}
    }
    return window.matchMedia?.("(prefers-color-scheme: dark)").matches ?? false;
  }

  let isDark = $state(pickInitialTheme());

  function applyTheme() {
    document.documentElement.setAttribute("data-theme", isDark ? "dark" : "light");
  }

  function toggleTheme() {
    isDark = !isDark;
    applyTheme();
    try { window.localStorage.setItem(themeKey, isDark ? "dark" : "light"); } catch (_) {}
  }

  let themeLabel = $derived(isDark ? $t("pub.theme_dark") : $t("pub.theme_light"));
  let themeIcon = $derived(isDark ? "☀" : "☽");

  // ── Locale ──────────────────────────────────────────────────────────────────
  const localeKey = "gonemaster.public.locale.v1";
  const localeDisplayNames = {
    en: "English", sv: "Svenska", da: "Dansk", de: "Deutsch", cs: "Čeština", fi: "Suomi",
    fr: "Français", es: "Español", nb: "Norsk", nl: "Nederlands", sl: "Slovenščina", ja: "日本語",
  };
  const localeLabel = (code) => localeDisplayNames[code] || code;

  // ?lang → stored choice → first browser-preferred catalog we ship → "en".
  // ?lang comes first so a shared link renders the language it names, matching
  // what the server rendered for it.
  function pickInitialLocale() {
    if (typeof window === "undefined") return "en";
    const asked = readLang().split("-")[0].toLowerCase();
    if (localeDisplayNames[asked]) return asked;
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
  // Static links injected into the translatable about credit line.
  const authorLink = '<a href="https://codeberg.org/pawal" target="_blank" rel="noopener">Patrik Wallström</a>';
  const repoLink = '<a href="https://codeberg.org/pawal/gonemaster" target="_blank" rel="noopener">Codeberg</a>';
  // Fail-safe default: hide scoring until server confirms it is enabled.
  let scoringEnabled = $state(false);
  let nameserverTimingsEnabled = $state(false);
  let dnssecChainEnabled = $state(false);

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
    writeLang(code);
    try { window.localStorage.setItem(localeKey, code); } catch (_) {}
  }

  // ── Shared-link init ────────────────────────────────────────────────────────
  async function applyPath() {
    const { view, publicID: id } = parsePath(window.location.pathname);
    if (view !== "result" || !id) return;
    publicID = id;
    startedHere = false;
    resultGrade = "";
    try {
      const res = await getJob(id);
      if (!res.ok) {
        if (res.status === 404) historyEntries = removeEntry(id);
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

  function onPopState() {
    const { view, publicID: id } = parsePath(window.location.pathname);
    if (view === "home") resetToIdle();
    else if (view === "result" && id && id !== publicID) applyPath();
  }

  // Link clicks the SPA handles itself.
  function onHomeClick(e) {
    if (!isPlainClick(e)) return;
    e.preventDefault();
    goHome();
  }

  function onSelectResult(id) {
    navigate("result", id);
    applyPath();
  }

  // ── Job handlers ────────────────────────────────────────────────────────────
  function onJobCreated(detail) {
    publicID = detail.publicID;
    startedHere = true;
    jobStatus = "";
    jobDomain = "";
    jobFinishedAt = null;
    jobProgress = 0;
    resultGrade = "";
    phase = "running";
    navigate("result", publicID);
  }

  function onJobDone(detail) {
    jobStatus = detail.status;
    jobDomain = detail.domain ?? "";
    jobFinishedAt = detail.finishedAt ?? null;
    phase = "done";
    if (detail.status === "succeeded" && startedHere) {
      historyEntries = addEntry({ id: detail.publicID, domain: jobDomain, finishedAt: jobFinishedAt });
    }
  }

  function resetToIdle() {
    phase = "idle";
    publicID = null;
    jobStatus = "";
    jobDomain = "";
    jobFinishedAt = null;
    resultGrade = "";
    focusSignal += 1;
  }

  // Going home from a link or "new test" moves the URL too; popstate must not
  // push a second entry, so it calls resetToIdle directly.
  function goHome() {
    navigate("home");
    resetToIdle();
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
        if (typeof data?.show_dnssec_chain_public === "boolean") {
          dnssecChainEnabled = data.show_dnssec_chain_public;
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
    window.addEventListener("popstate", onPopState);
    applyTheme();
    const initial = pickInitialLocale();
    resultLocale = initial;
    locale.set(initial);
    loadCatalog(initial);
    fetchLocales();
    fetchVersion();
    fetchInfo();
    applyPath();
  });

  onDestroy(() => {
    window.removeEventListener("popstate", onPopState);
  });
</script>

<main>
  <header>
    <div class="header-text">
      <h1 class="brand-mark">
        <a class="brand-link" href={pathFor("home")} onclick={onHomeClick}>
          <img class="brand-logo" src={logoSrc} alt="gonemaster" />
        </a>
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

  <TestForm disabled={phase === "running"} {focusSignal} {prefillDomain} {prefillSignal} onjobcreated={onJobCreated} />

  {#if phase === "idle" && historyEntries.length > 0}
    <RecentTests entries={historyEntries} locale={resultLocale} onclear={onClearHistory} onselect={onSelectResult} />
  {/if}

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
        {dnssecChainEnabled}
        ontestparent={onTestParent}
        onscore={onScore}
      />
    {:else}
      <ExpiredResult onnewtest={goHome} />
    {/if}
  {/if}

  <footer class="version-footer">
    <details class="about-details">
      <summary class="about-summary">{$t("pub.about_summary")}</summary>
      <div class="about-body">
        <p>{$t("pub.about_intro")}</p>
        <p>{@html $t("pub.about_credit", { author: authorLink, repo: repoLink })}</p>
      </div>
    </details>
    {#if versionGonemaster}
      <div class="version-box">
        <span class="version-row"><span class="version-name">gonemaster</span>{versionGonemaster}</span>
        {#if versionDNS}
          <span class="version-row"><span class="version-name">miekg/dns</span>{versionDNS}</span>
        {/if}
      </div>
    {/if}
  </footer>

  <a class="fork-ribbon right-bottom fixed" href="https://codeberg.org/pawal/gonemaster" data-ribbon="Fork me on Codeberg" title="Fork me on Codeberg">Fork me on Codeberg</a>
</main>
