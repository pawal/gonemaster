<script>
  import { t } from "../i18n.js";
  import { status, clearStatus } from "../lib/status.svelte.js";
</script>

{#if status.message}
  <div
    class={`status-toast status-${status.tone === "ok" ? "ok" : "warn"} reveal delay-12`}
    role="status"
    aria-live="polite"
  >
    <div>
      <strong>{status.tone === "ok" ? $t("toast_ok") : $t("toast_warn")}:</strong>
      {status.message}
    </div>
    <button
      class="status-toast-close"
      type="button"
      aria-label={$t("dismiss_notification_aria")}
      onclick={clearStatus}
    >
      {$t("dismiss")}
    </button>
  </div>
{/if}
