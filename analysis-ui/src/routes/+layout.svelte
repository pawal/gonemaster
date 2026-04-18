<script lang="ts">
  import "../app.css";
  import { onMount } from "svelte";
  import { page } from "$app/state";
  import { base } from "$app/paths";
  import { navItems, isActive } from "$lib/nav";
  import { applyTheme, initialTheme, persistTheme, type Theme } from "$lib/theme";

  let { children } = $props();

  let theme = $state<Theme>("light");

  onMount(() => {
    theme = initialTheme();
    applyTheme(theme);
  });

  function toggleTheme() {
    theme = theme === "dark" ? "light" : "dark";
    applyTheme(theme);
    persistTheme(theme);
  }

  // Strip the base prefix so the isActive() helper can compare against
  // relative hrefs declared in nav.ts.
  const currentPath = $derived.by(() => {
    const url = page.url.pathname;
    if (base && url.startsWith(base)) {
      return url.slice(base.length) || "/";
    }
    return url || "/";
  });
</script>

<div class="app-shell">
  <header class="app-header">
    <a class="brand" href="{base}/">
      <span class="brand-kicker">Gonemaster</span>
      <span>Analysis</span>
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
      {#each navItems as item (item.href)}
        <a
          class="nav-link"
          class:active={isActive(currentPath, item)}
          href={`${base}${item.href === "/" ? "" : item.href}`}
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
</div>

<style>
  .backend-warning {
    border-color: var(--severity-warning, #b45309);
    background: color-mix(in srgb, var(--severity-warning, #b45309) 8%, var(--card));
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
