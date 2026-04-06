<script>
  import { onMount } from "svelte";
  import { t } from "./i18n.js";

  export let apiBase = "/api/v1";

  let loading = false;
  let saving = false;
  let settings = {};
  let editedValues = {};
  let loadError = "";

  const settingGroups = [
    {
      key: "settings_group_general",
      settings: [
        { key: "listen_addr", type: "text", readonly: true },
        { key: "worker_count", type: "number" },
        { key: "max_concurrent_jobs", type: "number" },
        { key: "min_level", type: "select", options: ["INFO", "NOTICE", "WARNING", "ERROR", "CRITICAL"] },
        { key: "public_url", type: "text" },
      ],
    },
    {
      key: "settings_group_database",
      settings: [
        { key: "db_driver", type: "text", readonly: true },
        { key: "db_dsn", type: "text", readonly: true },
        { key: "profile_path", type: "text", readonly: true },
        { key: "retention_days", type: "number" },
      ],
    },
    {
      key: "settings_group_public_api",
      settings: [
        { key: "rate_limit_enabled", type: "toggle" },
        { key: "rate_limit_max", type: "number" },
        { key: "rate_limit_window", type: "text" },
      ],
    },
  ];

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

  let noticeMessage = "";
  let noticeTone = "";
  const setNotice = (message, tone = "") => {
    noticeMessage = message;
    noticeTone = tone;
  };

  async function loadSettings() {
    loading = true;
    loadError = "";
    try {
      settings = await apiFetch("/settings");
      editedValues = {};
    } catch (e) {
      loadError = e.message;
    } finally {
      loading = false;
    }
  }

  function currentValue(key) {
    if (key in editedValues) return editedValues[key];
    const entry = settings[key];
    if (!entry) return "";
    return entry.value;
  }

  function isEdited(key) {
    if (!(key in editedValues)) return false;
    const entry = settings[key];
    if (!entry) return true;
    return String(editedValues[key]) !== String(entry.value);
  }

  function handleInput(key, value) {
    editedValues[key] = value;
    editedValues = editedValues;
  }

  function handleToggle(key) {
    const current = currentValue(key);
    editedValues[key] = !current;
    editedValues = editedValues;
  }

  $: hasChanges = Object.keys(editedValues).some((k) => isEdited(k));

  function sourceLabel(source) {
    const map = {
      default: $t("settings_server_source_default"),
      config_file: $t("settings_server_source_config_file"),
      database: $t("settings_server_source_database"),
      cli_flag: $t("settings_server_source_cli_flag"),
    };
    return map[source] || source;
  }

  function isReadonly(key) {
    const entry = settings[key];
    return entry?.readonly || false;
  }

  async function saveSettings() {
    const changes = {};
    for (const [key, val] of Object.entries(editedValues)) {
      if (isEdited(key)) {
        changes[key] = val;
      }
    }
    if (Object.keys(changes).length === 0) {
      setNotice($t("settings_server_no_changes"), "ok");
      return;
    }
    saving = true;
    try {
      await apiFetch("/settings", {
        method: "PUT",
        body: JSON.stringify(changes),
      });
      setNotice($t("settings_server_saved"), "ok");
      await loadSettings();
    } catch (e) {
      setNotice($t("settings_server_save_error", { error: e.message }), "warn");
    } finally {
      saving = false;
    }
  }

  onMount(() => {
    loadSettings();
  });
</script>

<h2>{$t("settings_server_heading")}</h2>
<div class="small" style="margin-bottom: 12px;">{$t("settings_server_subtitle")}</div>

{#if noticeMessage}
  <div class={`notice notice-${noticeTone === "ok" ? "ok" : "warn"}`} role="status" aria-live="polite">
    {noticeMessage}
  </div>
{/if}

{#if loading}
  <p>{$t("settings_server_loading")}</p>
{:else if loadError}
  <p class="error">{$t("settings_server_load_error", { error: loadError })}</p>
{:else}
  {#each settingGroups as group}
    <h3>{$t(group.key)}</h3>
    <div class="settings-grid">
      {#each group.settings as s}
        {@const entry = settings[s.key]}
        {@const ro = s.readonly || isReadonly(s.key)}
        {@const val = currentValue(s.key)}
        {@const edited = isEdited(s.key)}
        <div class="setting-row" class:setting-edited={edited}>
          <label for="setting-{s.key}" class="setting-label">
            {$t(`settings_label_${s.key}`)}
            {#if entry}
              <span class="setting-source" title={sourceLabel(entry.source)}>({sourceLabel(entry.source)})</span>
            {/if}
          </label>
          <div class="setting-help">{$t(`settings_help_${s.key}`)}</div>
          <div class="setting-input">
            {#if s.type === "toggle"}
              <label class="toggle-label">
                <input
                  type="checkbox"
                  id="setting-{s.key}"
                  checked={!!val}
                  disabled={ro}
                  on:change={() => handleToggle(s.key)}
                />
                {val ? "Enabled" : "Disabled"}
              </label>
            {:else if s.type === "select"}
              <select
                id="setting-{s.key}"
                value={val ?? ""}
                disabled={ro}
                on:change={(e) => handleInput(s.key, e.target.value)}
              >
                {#each s.options as opt}
                  <option value={opt}>{opt}</option>
                {/each}
              </select>
            {:else if s.type === "number"}
              <input
                type="number"
                id="setting-{s.key}"
                value={val}
                disabled={ro}
                on:input={(e) => handleInput(s.key, Number(e.target.value))}
              />
            {:else}
              <input
                type="text"
                id="setting-{s.key}"
                value={val ?? ""}
                disabled={ro}
                on:input={(e) => handleInput(s.key, e.target.value)}
              />
            {/if}
          </div>
        </div>
      {/each}
    </div>
  {/each}

  <div class="settings-actions">
    <button
      type="button"
      disabled={!hasChanges || saving}
      on:click={saveSettings}
    >
      {saving ? $t("settings_server_saving") : $t("settings_server_save_button")}
    </button>
  </div>
{/if}

<style>
  h3 {
    margin: 16px 0 8px;
    font-size: 0.95em;
    font-weight: 600;
    border-bottom: 1px solid var(--border, #e0e0e0);
    padding-bottom: 4px;
  }
  .settings-grid {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .setting-row {
    display: grid;
    grid-template-columns: 200px 1fr;
    grid-template-rows: auto auto;
    gap: 2px 12px;
    align-items: center;
    padding: 6px 8px;
    border-radius: 4px;
  }
  .setting-row.setting-edited {
    background: var(--edited-bg, #fffde7);
  }
  .setting-label {
    font-weight: 500;
    font-size: 0.9em;
    grid-column: 1;
    grid-row: 1;
  }
  .setting-source {
    font-weight: 400;
    font-size: 0.8em;
    opacity: 0.65;
  }
  .setting-help {
    grid-column: 1 / -1;
    grid-row: 2;
    font-size: 0.8em;
    opacity: 0.6;
    margin-bottom: 2px;
  }
  .setting-input {
    grid-column: 2;
    grid-row: 1;
  }
  .setting-input input[type="text"],
  .setting-input input[type="number"] {
    width: 100%;
    max-width: 300px;
    padding: 4px 6px;
    font-family: inherit;
    font-size: 0.9em;
  }
  .setting-input input:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  .toggle-label {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: 0.9em;
    cursor: pointer;
  }
  .toggle-label input:disabled {
    cursor: not-allowed;
  }
  .settings-actions {
    margin-top: 16px;
    display: flex;
    gap: 8px;
  }
  .notice {
    padding: 8px 12px;
    border-radius: 4px;
    margin-bottom: 12px;
    font-size: 0.9em;
  }
  .notice-ok {
    background: var(--notice-ok-bg, #e8f5e9);
    color: var(--notice-ok-color, #2e7d32);
  }
  .notice-warn {
    background: var(--notice-warn-bg, #fff3e0);
    color: var(--notice-warn-color, #e65100);
  }
  .error {
    color: var(--error-color, #c62828);
  }
</style>
