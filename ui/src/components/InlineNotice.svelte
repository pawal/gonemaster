<script>
  let { message = "", tone = "warn", role = "status" } = $props();
</script>

{#if message}
  <div class={`inline-notice inline-notice-${tone === "ok" ? "ok" : "warn"}`} {role} aria-live="polite">{message}</div>
{/if}
