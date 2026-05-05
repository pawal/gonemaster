<script>
  import { t } from "../i18n.js";
  import { themeStore, toggleTheme } from "../lib/theme.svelte.js";

  const titleLabel = $derived(
    $t("theme_toggle_title", {
      theme: themeStore.value === "dark" ? $t("theme_dark") : $t("theme_light"),
    }),
  );
</script>

<button
  class="theme-toggle"
  type="button"
  onclick={toggleTheme}
  title={titleLabel}
  aria-label={titleLabel}
>
  {themeStore.isDark ? "☾" : "☀"}
</button>
