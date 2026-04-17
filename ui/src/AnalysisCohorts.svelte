<script>
  import { onMount } from "svelte";
  import { t } from "./i18n.js";

  let { apiBase = "/api/v1" } = $props();

  let loading = $state(false);
  let loadError = $state("");
  let cohorts = $state([]);
  let noticeMessage = $state("");
  let noticeTone = $state("");
  let busyCohortId = $state(null);
  let creating = $state(false);
  let draft = $state(emptyDraft());

  function emptyDraft() {
    return {
      source_tag: "",
      label: "",
      description: "",
      analysis_enabled: true,
      public_enabled: false,
      is_default: false,
      sort_order: 0,
    };
  }

  const apiFetch = async (path, options = {}) => {
    const url = path?.startsWith("/") ? `${apiBase}${path}` : `${apiBase}/${path}`;
    const headers = { ...(options.headers || {}) };
    if (options.body && !headers["Content-Type"]) {
      headers["Content-Type"] = "application/json";
    }
    const response = await fetch(url, { ...options, headers });
    const contentType = response.headers.get("content-type") || "";
    const payload = contentType.includes("application/json")
      ? await response.json()
      : await response.text();
    if (!response.ok) {
      const message = payload?.error?.message || payload?.message || response.statusText;
      throw new Error(message);
    }
    return payload;
  };

  const setNotice = (message, tone = "") => {
    noticeMessage = message;
    noticeTone = tone;
  };

  const clearNotice = () => {
    noticeMessage = "";
    noticeTone = "";
  };

  async function loadCohorts({ preserveNotice = false } = {}) {
    loading = true;
    loadError = "";
    try {
      const result = await apiFetch("/analysis/cohorts");
      cohorts = Array.isArray(result) ? result : [];
      if (!preserveNotice) clearNotice();
    } catch (error) {
      loadError = error.message || $t("analysis_cohorts_load_error_generic");
    } finally {
      loading = false;
    }
  }

  async function createCohort() {
    const tag = String(draft.source_tag || "").trim();
    if (!tag) {
      setNotice($t("analysis_cohorts_validation_source_tag"), "warn");
      return;
    }
    if (draft.public_enabled && !draft.analysis_enabled) {
      setNotice($t("analysis_cohorts_validation_public_requires_analysis"), "warn");
      return;
    }
    if (draft.is_default && (!draft.public_enabled || !draft.analysis_enabled)) {
      setNotice($t("analysis_cohorts_validation_default_requires_public"), "warn");
      return;
    }
    creating = true;
    try {
      await apiFetch("/analysis/cohorts", {
        method: "POST",
        body: JSON.stringify({
          source_type: "tag",
          source_tag: tag,
          label: String(draft.label || "").trim(),
          description: String(draft.description || "").trim(),
          analysis_enabled: !!draft.analysis_enabled,
          public_enabled: !!draft.public_enabled,
          is_default: !!draft.is_default,
          sort_order: Number(draft.sort_order) || 0,
        }),
      });
      draft = emptyDraft();
      await loadCohorts({ preserveNotice: true });
      setNotice($t("analysis_cohorts_created", { tag }), "ok");
    } catch (error) {
      setNotice($t("analysis_cohorts_create_error", { error: error.message || "" }), "warn");
    } finally {
      creating = false;
    }
  }

  async function patchCohort(cohort, patch, successKey) {
    busyCohortId = cohort.id;
    try {
      await apiFetch(`/analysis/cohorts/${cohort.id}`, {
        method: "PATCH",
        body: JSON.stringify(patch),
      });
      await loadCohorts({ preserveNotice: true });
      if (successKey) {
        setNotice($t(successKey, { tag: cohort.source_tag }), "ok");
      }
    } catch (error) {
      setNotice($t("analysis_cohorts_update_error", { error: error.message || "" }), "warn");
    } finally {
      busyCohortId = null;
    }
  }

  const toggleAnalysis = (cohort) => {
    const next = !cohort.analysis_enabled;
    const patch = { analysis_enabled: next };
    if (!next) {
      patch.public_enabled = false;
      patch.is_default = false;
    }
    return patchCohort(cohort, patch,
      next ? "analysis_cohorts_enabled" : "analysis_cohorts_disabled");
  };

  const togglePublic = (cohort) => {
    const next = !cohort.public_enabled;
    if (next && !cohort.analysis_enabled) {
      setNotice($t("analysis_cohorts_validation_public_requires_analysis"), "warn");
      return;
    }
    const patch = { public_enabled: next };
    if (!next) patch.is_default = false;
    return patchCohort(cohort, patch,
      next ? "analysis_cohorts_public_on" : "analysis_cohorts_public_off");
  };

  const setDefault = (cohort) => {
    if (!cohort.analysis_enabled || !cohort.public_enabled) {
      setNotice($t("analysis_cohorts_validation_default_requires_public"), "warn");
      return;
    }
    return patchCohort(cohort, { is_default: true }, "analysis_cohorts_default_set");
  };

  async function triggerAction(cohort, action, successKey) {
    busyCohortId = cohort.id;
    try {
      await apiFetch(`/analysis/cohorts/${cohort.id}/${action}`, { method: "POST", body: "{}" });
      await loadCohorts({ preserveNotice: true });
      setNotice($t(successKey, { tag: cohort.source_tag }), "ok");
    } catch (error) {
      setNotice($t("analysis_cohorts_action_error", { action, error: error.message || "" }), "warn");
    } finally {
      busyCohortId = null;
    }
  }

  const rebuildCohort = (cohort) => triggerAction(cohort, "rebuild", "analysis_cohorts_rebuild_ok");
  const clearCohort = (cohort) => triggerAction(cohort, "clear", "analysis_cohorts_clear_ok");

  function statusBadgeTone(status) {
    switch (status) {
      case "ready":
        return "ok";
      case "failed":
        return "warn";
      case "pending":
      default:
        return "neutral";
    }
  }

  function formatTimestamp(value) {
    if (!value) return "";
    const parsed = new Date(value);
    if (Number.isNaN(parsed.getTime())) return "";
    return parsed.toLocaleString();
  }

  onMount(() => {
    loadCohorts();
  });
</script>

<h2>{$t("analysis_cohorts_heading")}</h2>
<div class="small" style="margin-bottom: 12px;">{$t("analysis_cohorts_subtitle")}</div>

{#if noticeMessage}
  <div class={`notice notice-${noticeTone === "ok" ? "ok" : "warn"}`} role="status" aria-live="polite">
    {noticeMessage}
  </div>
{/if}

{#if loading}
  <p>{$t("analysis_cohorts_loading")}</p>
{:else if loadError}
  <p class="error">{$t("analysis_cohorts_load_error", { error: loadError })}</p>
{:else}
  <h3>{$t("analysis_cohorts_existing_heading")}</h3>
  {#if cohorts.length === 0}
    <p class="small">{$t("analysis_cohorts_empty")}</p>
  {:else}
    <div class="table-scroll">
      <table class="analysis-cohorts-table">
        <thead>
          <tr>
            <th scope="col">{$t("analysis_cohorts_col_tag")}</th>
            <th scope="col">{$t("analysis_cohorts_col_label")}</th>
            <th scope="col">{$t("analysis_cohorts_col_analysis")}</th>
            <th scope="col">{$t("analysis_cohorts_col_public")}</th>
            <th scope="col">{$t("analysis_cohorts_col_default")}</th>
            <th scope="col">{$t("analysis_cohorts_col_status")}</th>
            <th scope="col">{$t("analysis_cohorts_col_last_materialized")}</th>
            <th scope="col">{$t("analysis_cohorts_col_last_error")}</th>
            <th scope="col">{$t("analysis_cohorts_col_actions")}</th>
          </tr>
        </thead>
        <tbody>
          {#each cohorts as cohort (cohort.id)}
            {@const busy = busyCohortId === cohort.id}
            {@const tone = statusBadgeTone(cohort.materialization_status)}
            <tr>
              <th scope="row">{cohort.source_tag}</th>
              <td>{cohort.label || cohort.source_tag}</td>
              <td>
                <button
                  type="button"
                  class="toggle-pill"
                  class:toggle-on={cohort.analysis_enabled}
                  disabled={busy}
                  onclick={() => toggleAnalysis(cohort)}
                >
                  {cohort.analysis_enabled ? $t("analysis_cohorts_toggle_on") : $t("analysis_cohorts_toggle_off")}
                </button>
              </td>
              <td>
                <button
                  type="button"
                  class="toggle-pill"
                  class:toggle-on={cohort.public_enabled}
                  disabled={busy || !cohort.analysis_enabled}
                  onclick={() => togglePublic(cohort)}
                >
                  {cohort.public_enabled ? $t("analysis_cohorts_toggle_on") : $t("analysis_cohorts_toggle_off")}
                </button>
              </td>
              <td>
                {#if cohort.is_default}
                  <span class="default-marker">{$t("analysis_cohorts_default_active")}</span>
                {:else}
                  <button
                    type="button"
                    class="link-button"
                    disabled={busy || !cohort.analysis_enabled || !cohort.public_enabled}
                    onclick={() => setDefault(cohort)}
                  >
                    {$t("analysis_cohorts_default_set_action")}
                  </button>
                {/if}
              </td>
              <td>
                <span class={`status-badge status-${tone}`}>{cohort.materialization_status || "pending"}</span>
              </td>
              <td>
                {#if cohort.last_materialized_at}
                  <time datetime={cohort.last_materialized_at}>{formatTimestamp(cohort.last_materialized_at)}</time>
                {:else}
                  <span class="small muted">—</span>
                {/if}
              </td>
              <td>
                {#if cohort.last_materialization_error}
                  <span class="error-detail" title={cohort.last_materialization_error}>
                    {cohort.last_materialization_error}
                  </span>
                {:else}
                  <span class="small muted">—</span>
                {/if}
              </td>
              <td>
                <div class="row-actions">
                  <button type="button" disabled={busy || !cohort.analysis_enabled} onclick={() => rebuildCohort(cohort)}>
                    {$t("analysis_cohorts_rebuild")}
                  </button>
                  <button type="button" disabled={busy} onclick={() => clearCohort(cohort)}>
                    {$t("analysis_cohorts_clear")}
                  </button>
                </div>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}

  <h3 style="margin-top: 24px;">{$t("analysis_cohorts_create_heading")}</h3>
  <p class="small" style="margin-bottom: 12px;">{$t("analysis_cohorts_create_hint")}</p>
  <div class="create-grid">
    <label>
      <span>{$t("analysis_cohorts_field_source_tag")}</span>
      <input type="text" bind:value={draft.source_tag} placeholder="tld" />
    </label>
    <label>
      <span>{$t("analysis_cohorts_field_label")}</span>
      <input type="text" bind:value={draft.label} placeholder="TLD" />
    </label>
    <label class="full-width">
      <span>{$t("analysis_cohorts_field_description")}</span>
      <input type="text" bind:value={draft.description} />
    </label>
    <label>
      <span>{$t("analysis_cohorts_field_sort_order")}</span>
      <input type="number" bind:value={draft.sort_order} min="0" />
    </label>
    <label class="check-field">
      <input type="checkbox" bind:checked={draft.analysis_enabled} />
      <span>{$t("analysis_cohorts_field_analysis_enabled")}</span>
    </label>
    <label class="check-field">
      <input type="checkbox" bind:checked={draft.public_enabled} />
      <span>{$t("analysis_cohorts_field_public_enabled")}</span>
    </label>
    <label class="check-field">
      <input type="checkbox" bind:checked={draft.is_default} />
      <span>{$t("analysis_cohorts_field_is_default")}</span>
    </label>
  </div>
  <div style="margin-top: 12px;">
    <button type="button" disabled={creating} onclick={createCohort}>
      {creating ? $t("analysis_cohorts_creating") : $t("analysis_cohorts_create_button")}
    </button>
  </div>
{/if}

<style>
  .table-scroll {
    overflow-x: auto;
  }

  .analysis-cohorts-table {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.94rem;
  }

  .analysis-cohorts-table th,
  .analysis-cohorts-table td {
    padding: 8px 10px;
    text-align: left;
    border-bottom: 1px solid var(--border-color, #ddd);
    vertical-align: top;
  }

  .analysis-cohorts-table thead th {
    font-weight: 600;
    background: var(--surface-muted, transparent);
  }

  .toggle-pill {
    border: 1px solid var(--border-color, #ccc);
    background: transparent;
    padding: 2px 10px;
    border-radius: 999px;
    font-size: 0.85rem;
    cursor: pointer;
    min-width: 48px;
  }

  .toggle-pill[disabled] {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .toggle-pill.toggle-on {
    background: var(--accent, #2a6);
    color: #fff;
    border-color: var(--accent, #2a6);
  }

  .default-marker {
    display: inline-block;
    padding: 2px 8px;
    border-radius: 4px;
    background: var(--accent, #2a6);
    color: #fff;
    font-size: 0.8rem;
  }

  .link-button {
    background: transparent;
    border: none;
    color: var(--link, #06c);
    cursor: pointer;
    padding: 0;
    text-decoration: underline;
    font: inherit;
  }

  .link-button[disabled] {
    opacity: 0.5;
    cursor: not-allowed;
    text-decoration: none;
  }

  .status-badge {
    display: inline-block;
    padding: 2px 8px;
    border-radius: 4px;
    font-size: 0.8rem;
    text-transform: lowercase;
  }

  .status-ok {
    background: #d6f1d0;
    color: #1e5b1e;
  }

  .status-warn {
    background: #fbe3c8;
    color: #7a3f05;
  }

  .status-neutral {
    background: var(--surface-muted, #eee);
    color: var(--text, #333);
  }

  .error-detail {
    color: var(--error, #b00020);
    font-size: 0.85rem;
    max-width: 18rem;
    display: inline-block;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .row-actions {
    display: flex;
    gap: 6px;
  }

  .create-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
    gap: 12px;
  }

  .create-grid label {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 0.9rem;
  }

  .create-grid .full-width {
    grid-column: 1 / -1;
  }

  .create-grid .check-field {
    flex-direction: row;
    align-items: center;
    gap: 8px;
  }

  .muted {
    color: var(--text-muted, #888);
  }
</style>
