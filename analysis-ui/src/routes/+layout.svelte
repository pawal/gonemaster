<script lang="ts">
  import "../app.css";
  import { onMount } from "svelte";
  import { page, navigating } from "$app/state";
  import { goto } from "$app/navigation";
  import { base } from "$app/paths";
  import { getVersion } from "$lib/api";
  import NavProgress from "$lib/NavProgress.svelte";
  import ShortcutsHelp from "$lib/ShortcutsHelp.svelte";
  import { navItems, isActive, visibleNavItems } from "$lib/nav";
  import { resolveShortcut } from "$lib/shortcuts";
  import type { LayoutData } from "./+layout";
  import { applyTheme, initialTheme, persistTheme, type Theme } from "$lib/theme";
  import { applyHead } from "$lib/head";

  let { children } = $props();

  // Keep the document head in sync with the active route so titles,
  // descriptions, and the canonical URL reflect the current view.
  $effect(() => {
    applyHead(page.route.id, page.params, page.url);
  });

  let theme = $state<Theme>("light");
  let versionGonemaster = $state("");
  let versionDNS = $state("");

  onMount(() => {
    theme = initialTheme();
    applyTheme(theme);
    getVersion()
      .then((v) => {
        versionGonemaster = v.gonemaster ?? "";
        versionDNS = v.dns ?? "";
      })
      .catch(() => {});
  });

  function toggleTheme() {
    theme = theme === "dark" ? "light" : "dark";
    applyTheme(theme);
    persistTheme(theme);
  }

  // Hide the "Cohorts" tab when there's only one cohort to pick from: the
  // FilterBar chooser disappears under the same rule, and the overview
  // page already surfaces the single cohort's summary. Falls back to the
  // full nav set when the catalog couldn't be loaded.
  const visibleItems = $derived.by(() => {
    const cohorts = (page.data as LayoutData | undefined)?.catalog?.cohorts;
    if (!cohorts) return navItems;
    return visibleNavItems(navItems, cohorts.length);
  });

  // Strip the base prefix so the isActive() helper can compare against
  // relative hrefs declared in nav.ts.
  const currentPath = $derived.by(() => {
    const url = page.url.pathname;
    if (base && url.startsWith(base)) {
      return url.slice(base.length) || "/";
    }
    return url || "/";
  });

  // Forward the active cohort and snapshot pin across nav-bar clicks so
  // navigating between tabs doesn't drop the user back to auto-latest of
  // the default cohort. Page-specific params like `sort`, `limit`,
  // `offset`, `search` are intentionally not propagated - they only make
  // sense within one list page.
  const FORWARDED_PARAMS = ["dataset_tag", "snapshot"];

  const navQuery = $derived.by(() => {
    const source = page.url.searchParams;
    const params = new URLSearchParams();
    for (const key of FORWARDED_PARAMS) {
      const value = source.get(key);
      if (value) params.set(key, value);
    }
    const str = params.toString();
    return str ? `?${str}` : "";
  });

  function navHref(href: string): string {
    const path = `${base}${href === "/" ? "" : href}`;
    return `${path}${navQuery}`;
  }

  // Keyboard shortcuts. helpOpen is reactive; pendingG/gTimer are not, since
  // the g-prefix is transient chord state the UI never renders.
  let helpOpen = $state(false);
  let pendingG = false;
  let gTimer: ReturnType<typeof setTimeout> | null = null;

  function clearG() {
    pendingG = false;
    if (gTimer) {
      clearTimeout(gTimer);
      gTimer = null;
    }
  }

  function armG() {
    pendingG = true;
    if (gTimer) clearTimeout(gTimer);
    gTimer = setTimeout(() => {
      pendingG = false;
      gTimer = null;
    }, 1200);
  }

  function handleShortcut(event: KeyboardEvent) {
    // A non-help modal owns the keyboard; it closes itself on Esc.
    if (!helpOpen && document.querySelector('[aria-modal="true"]')) return;
    const action = resolveShortcut(event, { pendingG, overlayOpen: helpOpen });
    switch (action.type) {
      case "help-toggle":
        event.preventDefault();
        helpOpen = !helpOpen;
        break;
      case "close":
        event.preventDefault();
        helpOpen = false;
        break;
      case "set-g":
        event.preventDefault();
        armG();
        break;
      case "clear-g":
        clearG();
        break;
      case "navigate":
        event.preventDefault();
        clearG();
        // Route through navHref so shortcuts preserve cohort/snapshot scope.
        goto(navHref(action.href));
        break;
      case "focus-filter":
        event.preventDefault();
        (document.querySelector("[data-shortcut-filter]") as HTMLElement | null)?.focus();
        break;
    }
  }
</script>

<svelte:window onkeydown={handleShortcut} />

<NavProgress active={!!navigating.to} />

<div class="app-shell">
  <header class="app-header">
    <a class="brand" href="{base}/">
      <img class="brand-logo" src={`${base}/gonemaster.svg`} alt="Gonemaster" />
      <span class="brand-kicker">Analysis</span>
    </a>
    <div class="header-controls">
      <button
        type="button"
        class="theme-toggle"
        aria-label="Toggle color theme"
        title={theme === "dark" ? "Switch to light theme" : "Switch to dark theme"}
        onclick={toggleTheme}
      >
        {theme === "dark" ? "☀" : "☽"}
      </button>
    </div>
  </header>

  <nav class="app-nav" aria-label="Dashboard sections">
    <div class="app-nav-inner">
      {#each visibleItems as item (item.href)}
        <a
          class="nav-link"
          class:active={isActive(currentPath, item)}
          href={navHref(item.href)}
        >
          {item.label}
        </a>
      {/each}
    </div>
  </nav>

  <main class="app-main">
    {#if page.data && (page.data as any).backendSupported === false}
      <section class="card backend-warning" role="alert">
        <h2>Analysis backend not configured</h2>
        <p class="hint">
          The server is running without a SQL storage backend, so analysis
          facts cannot be materialized or queried. Restart the server with
          <code>--db-driver sqlite --db-dsn &lt;path&gt;</code> (or another
          supported SQL backend) and re-create your cohort to enable this
          dashboard.
        </p>
      </section>
    {/if}
    {@render children?.()}
  </main>

  <footer class="app-footer">
    {#if versionGonemaster}
      <span class="version-row"><span class="version-name">gonemaster</span>{versionGonemaster}</span>
    {/if}
    {#if versionDNS}
      <span class="version-row"><span class="version-name">miekg/dns</span>{versionDNS}</span>
    {/if}
    <a class="footer-link" href="https://codeberg.org/pawal/gonemaster">Source on Codeberg</a>
  </footer>
</div>

<ShortcutsHelp open={helpOpen} onClose={() => (helpOpen = false)} />

<style>
  .backend-warning {
    border-color: var(--sev-warning-fg);
    background: color-mix(in srgb, var(--sev-warning-fg) 8%, var(--card));
  }
  .backend-warning code {
    font-family: var(--mono);
    font-size: var(--text-sm);
    background: var(--surface-2);
    color: var(--on-surface-2);
    padding: 1px 6px;
    border-radius: 4px;
  }
</style>
