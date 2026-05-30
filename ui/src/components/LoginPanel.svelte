<script>
  import { t } from "../i18n.js";
  import { login } from "../lib/auth.svelte.js";

  let { apiFetch } = $props();
  let token = $state("");
  let error = $state(false);
  let submitting = $state(false);

  async function submit(event) {
    event.preventDefault();
    if (!token.trim() || submitting) return;
    submitting = true;
    error = false;
    try {
      await login(apiFetch, token.trim());
      window.location.reload();
    } catch (_) {
      error = true;
      submitting = false;
    }
  }
</script>

<div class="login-gate">
  <form class="login-card" onsubmit={submit}>
    <h1>{$t("auth_required_title")}</h1>
    <label for="admin-token">{$t("auth_token_label")}</label>
    <input
      id="admin-token"
      type="password"
      autocomplete="off"
      bind:value={token}
      placeholder={$t("auth_token_placeholder")}
    />
    {#if error}
      <p class="login-error" role="alert">{$t("auth_token_invalid")}</p>
    {/if}
    <button type="submit" disabled={submitting}>{$t("auth_token_submit")}</button>
  </form>
</div>

<style>
  .login-gate {
    min-height: 100vh;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: var(--space-6);
    font-family: var(--sans);
  }
  .login-card {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    width: 100%;
    max-width: 22rem;
    padding: var(--space-8);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--card);
    color: var(--ink);
  }
  .login-card h1 {
    margin: 0 0 var(--space-2);
    font-size: var(--text-xl);
  }
  .login-card label {
    font-size: var(--text-sm);
    color: var(--ink-2);
  }
  .login-card input {
    padding: var(--space-2);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    font: inherit;
    background: var(--surface);
    color: var(--ink);
  }
  .login-card button {
    margin-top: var(--space-2);
    padding: var(--space-2) var(--space-4);
    border: none;
    border-radius: var(--radius);
    background: var(--accent);
    color: var(--btn-fg);
    font: inherit;
    cursor: pointer;
  }
  .login-card button:disabled {
    opacity: 0.6;
    cursor: default;
  }
  .login-error {
    margin: 0;
    color: var(--grade-f);
    font-size: var(--text-sm);
  }
</style>
