<script>
  import { t } from "../i18n.js";
  import { apiCall } from "../lib/api.js";

  let {
    open = false,
    batchId = "",
    apiPrefix = "/api/v1",
    onClose = () => {},
    onDeleted = () => {},
    setStatus = () => {},
  } = $props();

  let loading = $state(false);
  let submitting = $state(false);
  let error = $state("");
  let preview = $state(null);
  let typed = $state("");

  $effect(() => {
    if (open && batchId) {
      loadPreview();
    } else if (!open) {
      preview = null;
      typed = "";
      error = "";
    }
  });

  const apiFetch = async (path, options) => apiCall(apiPrefix, path, options);

  async function loadPreview() {
    loading = true;
    error = "";
    preview = null;
    try {
      preview = await apiFetch(`/batches/${encodeURIComponent(batchId)}/delete-preview`);
    } catch (err) {
      error = err.message || String(err);
    } finally {
      loading = false;
    }
  }

  async function confirmDelete() {
    if (typed.trim() !== batchId) return;
    submitting = true;
    error = "";
    try {
      await apiFetch(`/batches/${encodeURIComponent(batchId)}`, { method: "DELETE" });
      setStatus($t("batch_delete_success", { id: batchId }), "ok");
      onDeleted(batchId);
      onClose();
    } catch (err) {
      error = err.message || String(err);
      setStatus($t("batch_delete_error", { error: error }), "warn");
    } finally {
      submitting = false;
    }
  }

  const confirmEnabled = $derived(
    !submitting && preview && preview.exists && typed.trim() === batchId
  );

  const formatDate = (s) => {
    if (!s) return "";
    try {
      return new Date(s).toISOString().slice(0, 19).replace("T", " ");
    } catch {
      return s;
    }
  };
</script>

{#if open}
  <div
    class="modal-backdrop"
    role="button"
    tabindex="-1"
    aria-label={$t("batch_delete_modal_close_label")}
    onclick={() => { if (!submitting) onClose(); }}
    onkeydown={(e) => { if (e.key === "Escape" && !submitting) onClose(); }}
  >
    <div
      class="modal-card"
      role="dialog"
      tabindex="-1"
      aria-modal="true"
      aria-labelledby="batch-delete-title"
      onclick={(e) => e.stopPropagation()}
      onkeydown={(e) => e.stopPropagation()}
    >
      <h2 id="batch-delete-title">{$t("batch_delete_modal_title")}</h2>

      {#if loading}
        <p class="muted">{$t("loading")}</p>
      {:else if error && !preview}
        <p class="status-banner warn">{error}</p>
      {:else if preview}
        <div class="meta">
          <div><strong>{$t("batch_id_label")}:</strong> <code class="mono">{preview.batch_id}</code></div>
          {#if preview.tag}
            <div><strong>{$t("batch_tag_label")}:</strong> {preview.tag}</div>
          {/if}
          {#if preview.created_at}
            <div><strong>{$t("col_created_at")}:</strong> {formatDate(preview.created_at)}</div>
          {/if}
          {#if preview.snapshot_intent}
            <div><span class="pill snapshot-intent">{$t("batch_snapshot_intent_pill")}</span></div>
          {/if}
        </div>

        <h3>{$t("batch_delete_impact_preview")}</h3>
        <table class="impact-table">
          <tbody>
            <tr><td>{$t("batch_delete_impact_queued_jobs")}</td><td>{preview.queued_jobs}</td></tr>
            <tr><td>{$t("batch_delete_impact_running_jobs")}</td><td>{preview.running_jobs}</td></tr>
            <tr><td>{$t("batch_delete_impact_completed_runs")}</td><td>{preview.completed_runs}</td></tr>
            <tr><td>{$t("batch_delete_impact_entries")}</td><td>{preview.entries}</td></tr>
            <tr><td>{$t("batch_delete_impact_fact_rows")}</td><td>{preview.fact_rows}</td></tr>
          </tbody>
        </table>

        {#if (preview.queued_jobs + preview.running_jobs) > 0}
          <p class="status-banner warn">
            {$t("batch_delete_inflight_warning", {
              count: preview.queued_jobs + preview.running_jobs
            })}
          </p>
        {/if}

        {#if preview.snapshots && preview.snapshots.length > 0}
          <h3>{$t("batch_delete_cohort_warning_heading")}</h3>
          <ul class="snapshot-list">
            {#each preview.snapshots as snap}
              <li>
                <strong>{snap.cohort_label || snap.cohort_id}</strong>:
                <code class="mono">{snap.snapshot_slug}</code>
                {#if snap.snapshot_label} - {snap.snapshot_label}{/if}
                {#if snap.is_default}
                  <span class="pill default">{$t("batch_delete_default_pill")}</span>
                {/if}
              </li>
            {/each}
          </ul>
          {#if preview.snapshots.some((s) => s.is_default)}
            <p class="status-banner warn">{$t("batch_delete_default_warning")}</p>
          {/if}
        {/if}

        <p class="status-banner warn">{$t("batch_delete_scope_notice")}</p>

        <p class="irreversible">{$t("batch_delete_irreversible_notice")}</p>

        <label for="batch-delete-typed">
          {$t("batch_delete_typed_confirm_label", { id: batchId })}
        </label>
        <input
          id="batch-delete-typed"
          type="text"
          bind:value={typed}
          disabled={submitting}
          autocomplete="off"
          spellcheck="false"
        />

        {#if error}
          <p class="status-banner warn">{error}</p>
        {/if}
      {/if}

      <div class="actions">
        <button type="button" class="ghost" onclick={onClose} disabled={submitting}>
          {$t("cancel")}
        </button>
        <button
          type="button"
          class="warn"
          onclick={confirmDelete}
          disabled={!confirmEnabled}
        >
          {submitting ? $t("submitting") : $t("batch_delete_confirm_button")}
        </button>
      </div>
    </div>
  </div>
{/if}

<style>
  .modal-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.45);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
    padding: 1rem;
  }
  .modal-card {
    background: var(--bg, #fff);
    color: var(--fg, #111);
    border-radius: 8px;
    padding: 1.5rem;
    max-width: 640px;
    width: 100%;
    max-height: 90vh;
    overflow-y: auto;
    box-shadow: 0 10px 30px rgba(0, 0, 0, 0.3);
  }
  h2 {
    margin-top: 0;
  }
  h3 {
    margin-top: 1.25rem;
    margin-bottom: 0.5rem;
  }
  .meta {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    margin-bottom: 0.5rem;
  }
  .impact-table {
    width: 100%;
    border-collapse: collapse;
  }
  .impact-table td {
    padding: 0.25rem 0.5rem;
    border-bottom: 1px solid var(--border, #eee);
  }
  .impact-table td:last-child {
    text-align: right;
    font-variant-numeric: tabular-nums;
  }
  .snapshot-list {
    margin: 0.25rem 0 0.5rem 1rem;
    padding: 0;
  }
  .snapshot-list li {
    margin-bottom: 0.25rem;
  }
  .irreversible {
    margin-top: 1rem;
    font-weight: 600;
  }
  .actions {
    display: flex;
    gap: 0.5rem;
    justify-content: flex-end;
    margin-top: 1.25rem;
  }
  input[type="text"] {
    width: 100%;
    box-sizing: border-box;
    padding: 0.4rem 0.5rem;
    margin-top: 0.25rem;
  }
  .pill {
    display: inline-block;
    padding: 0.1rem 0.5rem;
    border-radius: 999px;
    font-size: 0.75rem;
    background: var(--pill-bg, #eee);
    margin-left: 0.25rem;
  }
  .pill.default {
    background: #fce8b2;
  }
  .pill.snapshot-intent {
    background: #cfe9ff;
  }
</style>
