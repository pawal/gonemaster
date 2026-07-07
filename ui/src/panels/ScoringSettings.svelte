<script>
  import { onMount, onDestroy } from "svelte";
  import { t } from "../i18n.js";
  import { apiCall } from "../lib/api.js";
  import { dirtyGuard } from "../lib/dirty.svelte.js";
  import InlineNotice from "../components/InlineNotice.svelte";

  let { apiBase = "/api/v1" } = $props();

  const SEVERITY_LEVELS = ["NOTICE", "WARNING", "ERROR", "CRITICAL"];

  const BONUS_FIELDS = [
    { key: "no_warnings_or_errors",  labelKey: "scoring_bonus_no_warnings_errors" },
    { key: "dnssec_enabled",         labelKey: "scoring_bonus_dnssec_enabled" },
    { key: "strong_algorithm",       labelKey: "scoring_bonus_strong_algorithm" },
    { key: "nsec3_non_optout",       labelKey: "scoring_bonus_nsec3_non_optout" },
    { key: "cds_cdnskey_published",  labelKey: "scoring_bonus_cds_cdnskey" },
    { key: "ipv6_all_nameservers",   labelKey: "scoring_bonus_ipv6_all_ns" },
    { key: "as_diversity",           labelKey: "scoring_bonus_as_diversity" },
  ];

  let loading = $state(false);
  let saving = $state(false);
  let loadError = $state("");
  let noticeMessage = $state("");
  let noticeTone = $state("");
  let source = $state("default");
  let readonly = $state(false);

  // Canonical loaded config (for change detection / discard).
  let loaded = $state(null);

  // Working copy - primitive fields bound directly.
  let draft = $state(null);

  // Current compiled defaults, used to surface default entries missing from a
  // saved config (e.g. new tag penalties shipped with a new testcase).
  let defaultsConfig = $state(null);

  // Map fields displayed as ordered arrays.
  let tagRows = $state([]);    // [{tag, penalty}]
  let moduleRows = $state([]); // [{module, category}]

  let showModuleMapping = $state(false);
  let showImport = $state(false);
  let importText = $state("");
  let importError = $state("");

  // ── helpers ──────────────────────────────────────────────────────────────────

  const apiFetch = async (path, options) => apiCall(apiBase, path, options);

  const setNotice = (msg, tone = "") => { noticeMessage = msg; noticeTone = tone; };
  const clearNotice = () => { noticeMessage = ""; noticeTone = ""; };

  function sourceLabel(src) {
    const map = {
      default:     $t("settings_server_source_default"),
      config_file: $t("settings_server_source_config_file"),
      database:    $t("settings_server_source_database"),
      cli_flag:    $t("settings_server_source_cli_flag"),
    };
    return map[src] || src;
  }

  // Convert config maps → editable arrays.
  function configToRows(cfg) {
    tagRows = Object.entries(cfg.tag_penalties || {})
      .map(([tag, penalty]) => ({ tag, penalty }));
    moduleRows = Object.entries(cfg.module_categories || {})
      .map(([module, category]) => ({ module, category }));
  }

  // Assemble final config from draft + editable arrays.
  function assembleDraft() {
    const tag_penalties = {};
    for (const r of tagRows) {
      if (r.tag.trim()) tag_penalties[r.tag.trim()] = Number(r.penalty) || 0;
    }
    const module_categories = {};
    for (const r of moduleRows) {
      if (r.module.trim()) module_categories[r.module.trim()] = r.category;
    }
    return { ...draft, tag_penalties, module_categories };
  }

  function cloneConfig(cfg) {
    return JSON.parse(JSON.stringify(cfg));
  }

  // Deep equality via JSON - good enough for this config shape.
  let hasChanges = $derived(
    loaded !== null && draft !== null && (
      JSON.stringify(assembleDraft()) !== JSON.stringify(loaded) ||
      JSON.stringify(tagRows) !== JSON.stringify(
        Object.entries(loaded.tag_penalties || {}).map(([tag, penalty]) => ({ tag, penalty }))
      ) ||
      JSON.stringify(moduleRows) !== JSON.stringify(
        Object.entries(loaded.module_categories || {}).map(([module, category]) => ({ module, category }))
      )
    )
  );

  $effect(() => { dirtyGuard.register(hasChanges, $t("settings_discard_confirm")); });
  onDestroy(() => dirtyGuard.clear());

  // Default entries absent from the current config (matched case-insensitively).
  let missingDefaults = $derived.by(() => {
    if (readonly || !defaultsConfig) return { tags: [], modules: [] };
    const haveTags = new Set(
      tagRows.map((r) => String(r.tag).trim().toLowerCase()).filter(Boolean)
    );
    const tags = Object.entries(defaultsConfig.tag_penalties || {})
      .filter(([tag]) => !haveTags.has(tag.toLowerCase()))
      .map(([tag, penalty]) => ({ tag, penalty }));
    const haveModules = new Set(
      moduleRows.map((r) => String(r.module).trim().toLowerCase()).filter(Boolean)
    );
    const modules = Object.entries(defaultsConfig.module_categories || {})
      .filter(([module]) => !haveModules.has(module.toLowerCase()))
      .map(([module, category]) => ({ module, category }));
    return { tags, modules };
  });
  let missingCount = $derived(missingDefaults.tags.length + missingDefaults.modules.length);

  // ── load ──────────────────────────────────────────────────────────────────────

  async function loadConfig() {
    loading = true;
    loadError = "";
    try {
      const resp = await apiFetch("/scoring-config");
      source = resp.source || "default";
      readonly = !!resp.readonly;
      loaded = cloneConfig(resp.config);
      draft = cloneConfig(resp.config);
      configToRows(resp.config);
      try {
        defaultsConfig = await apiFetch("/scoring-config/defaults");
      } catch (_) {
        defaultsConfig = null;
      }
    } catch (e) {
      loadError = e.message;
    } finally {
      loading = false;
    }
  }

  // ── save / discard / reset ────────────────────────────────────────────────────

  async function save() {
    saving = true;
    clearNotice();
    try {
      const cfg = assembleDraft();
      await apiFetch("/scoring-config", { method: "PUT", body: JSON.stringify(cfg) });
      await loadConfig();
      setNotice($t("scoring_saved"), "ok");
    } catch (e) {
      setNotice($t("scoring_save_error", { error: e.message }), "warn");
    } finally {
      saving = false;
    }
  }

  function discard() {
    if (!loaded) return;
    draft = cloneConfig(loaded);
    configToRows(loaded);
    clearNotice();
  }

  async function resetToDefaults() {
    clearNotice();
    try {
      const defaults = await apiFetch("/scoring-config/defaults");
      draft = cloneConfig(defaults);
      configToRows(defaults);
    } catch (e) {
      setNotice($t("scoring_save_error", { error: e.message }), "warn");
    }
  }

  // ── import / export ───────────────────────────────────────────────────────────

  function exportJSON() {
    const text = JSON.stringify(assembleDraft(), null, 2);
    const blob = new Blob([text], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = "scoring-config.json";
    a.click();
    URL.revokeObjectURL(url);
  }

  function applyImport() {
    importError = "";
    try {
      const parsed = JSON.parse(importText);
      draft = cloneConfig(parsed);
      configToRows(parsed);
      showImport = false;
      importText = "";
    } catch (e) {
      importError = $t("scoring_import_error", { error: e.message });
    }
  }

  // ── tag rows ──────────────────────────────────────────────────────────────────

  // Append default entries missing from the config; operator reviews and saves.
  function addMissingDefaults() {
    if (missingDefaults.tags.length) {
      tagRows = [...tagRows, ...missingDefaults.tags.map((m) => ({ tag: m.tag, penalty: m.penalty }))];
    }
    if (missingDefaults.modules.length) {
      moduleRows = [...moduleRows, ...missingDefaults.modules.map((m) => ({ module: m.module, category: m.category }))];
      showModuleMapping = true;
    }
    clearNotice();
  }

  function addTagRow() {
    tagRows = [...tagRows, { tag: "", penalty: 0 }];
  }

  function removeTagRow(i) {
    tagRows = tagRows.filter((_, idx) => idx !== i);
  }

  function addModuleRow() {
    moduleRows = [...moduleRows, { module: "", category: "" }];
  }

  function removeModuleRow(i) {
    moduleRows = moduleRows.filter((_, idx) => idx !== i);
  }

  onMount(() => { loadConfig(); });
</script>

<h2>{$t("settings_scoring_heading")}</h2>

{#if readonly}
  <div class="inline-notice inline-notice-warn" role="alert">{$t("scoring_config_readonly_notice")}</div>
{/if}

<InlineNotice message={noticeMessage} tone={noticeTone} />

{#if loading}
  <p>{$t("scoring_config_loading")}</p>
{:else if loadError}
  <p class="error">{$t("scoring_config_load_error", { error: loadError })}</p>
{:else if draft}
  <div class="scoring-source">
    {$t("scoring_config_source_label")}:
    <span class="source-badge">{sourceLabel(source)}</span>
  </div>

  {#if missingCount > 0}
    <div class="defaults-banner" role="status">
      <strong>{$t("scoring_defaults_available", { count: missingCount })}</strong>
      <p class="defaults-banner-hint">{$t("scoring_defaults_available_hint")}</p>
      <ul class="defaults-list">
        {#each missingDefaults.tags as m}
          <li><code>{m.tag} = {m.penalty}</code></li>
        {/each}
        {#each missingDefaults.modules as m}
          <li><code>{m.module} → {m.category}</code></li>
        {/each}
      </ul>
      <button type="button" class="btn-add" onclick={addMissingDefaults}>
        {$t("scoring_add_missing_defaults")}
      </button>
    </div>
  {/if}

  <!-- Severity, Category, Grade Bands, Bonus - single table so value column aligns across all sections -->
  <table class="config-table">
    <colgroup>
      <col class="col-key">
      <col class="col-val">
    </colgroup>

    <tbody>
      <tr><th colspan="2" class="section-head section-head--first">{$t("scoring_section_severity_penalties")}</th></tr>
      <tr class="col-headers">
        <th>{$t("scoring_col_level")}</th>
        <th>{$t("scoring_col_penalty")}</th>
      </tr>
      {#each SEVERITY_LEVELS as level}
        <tr>
          <td><code>{level}</code></td>
          <td>
            <input
              type="number"
              aria-label={`${$t("scoring_col_penalty")} ${level}`}
              value={draft.severity_penalties?.[level] ?? 0}
              disabled={readonly}
              oninput={(e) => {
                draft.severity_penalties = { ...draft.severity_penalties, [level]: Number(e.target.value) };
              }}
            />
          </td>
        </tr>
      {/each}
    </tbody>

    <tbody>
      <tr><th colspan="2" class="section-head">{$t("scoring_section_category_weights")}</th></tr>
      <tr class="col-headers">
        <th>{$t("scoring_col_category")}</th>
        <th>{$t("scoring_col_weight")}</th>
      </tr>
      {#each Object.entries(draft.category_weights || {}) as [cat, weight]}
        <tr>
          <td><code>{cat}</code></td>
          <td>
            <input
              type="number"
              step="0.1"
              aria-label={`${$t("scoring_col_weight")} ${cat}`}
              value={weight}
              disabled={readonly}
              oninput={(e) => {
                draft.category_weights = { ...draft.category_weights, [cat]: Number(e.target.value) };
              }}
            />
          </td>
        </tr>
      {/each}
    </tbody>

    <tbody>
      <tr><th colspan="2" class="section-head">{$t("scoring_section_grade_bands")}</th></tr>
      <tr class="col-headers">
        <th>{$t("scoring_col_grade")}</th>
        <th>{$t("scoring_col_min_score")}</th>
      </tr>
      {#each draft.grade_bands || [] as band, i}
        <tr>
          <td>
            <input
              type="text"
              aria-label={`${$t("scoring_col_grade")} ${i + 1}`}
              value={band.grade}
              disabled={readonly}
              oninput={(e) => {
                const bands = [...(draft.grade_bands || [])];
                bands[i] = { ...bands[i], grade: e.target.value };
                draft.grade_bands = bands;
              }}
            />
          </td>
          <td>
            <input
              type="number"
              aria-label={`${$t("scoring_col_min_score")} ${i + 1}`}
              value={band.min_score}
              disabled={readonly}
              oninput={(e) => {
                const bands = [...(draft.grade_bands || [])];
                bands[i] = { ...bands[i], min_score: Number(e.target.value) };
                draft.grade_bands = bands;
              }}
            />
          </td>
        </tr>
      {/each}
    </tbody>

    <tbody>
      <tr><th colspan="2" class="section-head">{$t("scoring_section_bonus_criteria")}</th></tr>
      {#each BONUS_FIELDS as field}
        <tr>
          <td><label for="bonus-{field.key}">{$t(field.labelKey)}</label></td>
          <td class="check-cell">
            <input
              id="bonus-{field.key}"
              type="checkbox"
              checked={!!draft.bonus_criteria?.[field.key]}
              disabled={readonly}
              onchange={(e) => {
                draft.bonus_criteria = { ...draft.bonus_criteria, [field.key]: e.target.checked };
              }}
            />
          </td>
        </tr>
      {/each}
    </tbody>
  </table>

  <!-- Tag Penalty Overrides -->
  <h3>{$t("scoring_section_tag_overrides")}</h3>
  <table class="config-table">
    <colgroup>
      <col class="col-tag">
      <col class="col-num">
      <col class="col-act">
    </colgroup>
    <thead>
      <tr class="col-headers">
        <th>{$t("scoring_col_tag")}</th>
        <th>{$t("scoring_col_penalty")}</th>
        <th></th>
      </tr>
    </thead>
    <tbody>
      {#each tagRows as row, i}
        <tr>
          <td>
            <input
              type="text"
              aria-label={`${$t("scoring_col_tag")} ${i + 1}`}
              value={row.tag}
              disabled={readonly}
              oninput={(e) => { tagRows[i] = { ...tagRows[i], tag: e.target.value }; tagRows = tagRows; }}
            />
          </td>
          <td>
            <input
              type="number"
              aria-label={`${$t("scoring_col_penalty")} ${i + 1}`}
              value={row.penalty}
              disabled={readonly}
              oninput={(e) => { tagRows[i] = { ...tagRows[i], penalty: Number(e.target.value) }; tagRows = tagRows; }}
            />
          </td>
          <td>
            {#if !readonly}
              <button type="button" class="btn-remove" onclick={() => removeTagRow(i)}>
                {$t("scoring_remove_row")}
              </button>
            {/if}
          </td>
        </tr>
      {/each}
    </tbody>
  </table>
  {#if !readonly}
    <button type="button" class="btn-add" onclick={addTagRow}>{$t("scoring_add_override")}</button>
  {/if}

  <!-- Module → Category Mapping (collapsible) -->
  <h3>
    <button
      type="button"
      class="collapse-toggle"
      aria-expanded={showModuleMapping}
      onclick={() => { showModuleMapping = !showModuleMapping; }}
    >
      {showModuleMapping ? $t("scoring_hide_module_mapping") : $t("scoring_show_module_mapping")}
    </button>
  </h3>
  {#if showModuleMapping}
    <table class="config-table">
      <colgroup>
        <col class="col-mod">
        <col class="col-mod">
        <col class="col-act">
      </colgroup>
      <thead>
        <tr class="col-headers">
          <th>{$t("scoring_col_module")}</th>
          <th>{$t("scoring_col_category")}</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        {#each moduleRows as row, i}
          <tr>
            <td>
              <input
                type="text"
                aria-label={`${$t("scoring_col_module")} ${i + 1}`}
                value={row.module}
                disabled={readonly}
                oninput={(e) => { moduleRows[i] = { ...moduleRows[i], module: e.target.value }; moduleRows = moduleRows; }}
              />
            </td>
            <td>
              <input
                type="text"
                aria-label={`${$t("scoring_col_category")} ${i + 1}`}
                value={row.category}
                disabled={readonly}
                oninput={(e) => { moduleRows[i] = { ...moduleRows[i], category: e.target.value }; moduleRows = moduleRows; }}
              />
            </td>
            <td>
              {#if !readonly}
                <button type="button" class="btn-remove" onclick={() => removeModuleRow(i)}>
                  {$t("scoring_remove_row")}
                </button>
              {/if}
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
    {#if !readonly}
      <button type="button" class="btn-add" onclick={addModuleRow}>{$t("scoring_add_mapping")}</button>
    {/if}
  {/if}

  <!-- Import JSON -->
  {#if showImport}
    <div class="import-panel">
      <textarea
        rows="8"
        placeholder={$t("scoring_import_placeholder")}
        aria-label={$t("scoring_import_json")}
        bind:value={importText}
      ></textarea>
      {#if importError}
        <p class="error">{importError}</p>
      {/if}
      <div class="import-actions">
        <button type="button" onclick={applyImport}>{$t("scoring_import_apply")}</button>
        <button type="button" onclick={() => { showImport = false; importText = ""; importError = ""; }}>
          {$t("scoring_import_cancel")}
        </button>
      </div>
    </div>
  {/if}

  <!-- Actions -->
  <div class="scoring-actions">
    {#if !readonly}
      <button
        type="button"
        disabled={!hasChanges || saving}
        onclick={save}
      >
        {saving ? $t("scoring_saving") : $t("scoring_save_button")}
      </button>
      <button
        type="button"
        disabled={!hasChanges || saving}
        onclick={discard}
      >
        {$t("scoring_discard_button")}
      </button>
      <button type="button" disabled={saving} onclick={resetToDefaults}>
        {$t("scoring_reset_defaults")}
      </button>
    {/if}
    <button type="button" onclick={exportJSON}>{$t("scoring_export_json")}</button>
    {#if !readonly && !showImport}
      <button type="button" onclick={() => { showImport = true; }}>
        {$t("scoring_import_json")}
      </button>
    {/if}
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
  .scoring-source {
    font-size: 0.85em;
    opacity: 0.75;
    margin-bottom: 12px;
  }
  .defaults-banner {
    border: 1px solid #fcd34d;
    background: #fef9ee;
    border-radius: 8px;
    padding: 12px 14px;
    margin-bottom: 16px;
    max-width: 480px;
  }
  .defaults-banner-hint {
    margin: 6px 0 8px;
    font-size: 0.85em;
    color: var(--muted, #777);
  }
  .defaults-list {
    margin: 0 0 10px;
    padding-left: 1.2em;
    font-size: 0.85em;
  }
  .defaults-list li {
    margin: 2px 0;
  }
  .source-badge {
    font-weight: 500;
  }
  .config-table {
    border-collapse: collapse;
    table-layout: fixed;
    width: 100%;
    max-width: 480px;
    font-size: 0.9em;
    margin-bottom: 4px;
  }
  /* 2-column layout: key 55 % / value 45 % */
  .col-key { width: 55%; }
  .col-val { width: 45%; }
  /* 3-column layout: tag/module rows */
  .col-tag { width: 52%; }
  .col-mod { width: 40%; }
  .col-num { width: 26%; }
  .col-act { width: 22%; }
  .config-table .section-head {
    padding: 14px 8px 4px;
    text-align: left;
    font-size: 0.95em;
    font-weight: 600;
    border-bottom: 1px solid var(--border, #e0e0e0);
  }
  .config-table .section-head--first {
    padding-top: 0;
  }
  .config-table .col-headers th {
    padding: 2px 8px 4px;
    font-size: 0.8em;
    font-weight: 600;
    color: var(--muted, #777);
    text-align: left;
  }
  .config-table td {
    padding: 3px 8px;
    vertical-align: middle;
  }
  .config-table input[type="number"],
  .config-table input[type="text"] {
    padding: 3px 5px;
    font-size: 0.9em;
    font-family: inherit;
    box-sizing: border-box;
  }
  .config-table input[type="number"] {
    width: 6em;
  }
  .config-table input[type="text"] {
    width: 100%;
  }
  .config-table input:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  .config-table td label {
    cursor: pointer;
  }
  .check-cell {
    text-align: left;
  }
  .scoring-actions {
    margin-top: 20px;
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
  }
  .btn-add {
    margin-top: 4px;
    font-size: 0.85em;
  }
  .btn-remove {
    font-size: 0.8em;
    padding: 2px 6px;
  }
  .collapse-toggle {
    background: none;
    border: none;
    padding: 0;
    font: inherit;
    font-weight: 600;
    cursor: pointer;
    color: var(--link, #1a73e8);
    font-size: 0.95em;
  }
  .import-panel {
    margin-top: 12px;
    display: flex;
    flex-direction: column;
    gap: 6px;
    max-width: 560px;
  }
  .import-panel textarea {
    font-family: monospace;
    font-size: 0.85em;
    padding: 6px 8px;
    resize: vertical;
  }
  .import-actions {
    display: flex;
    gap: 8px;
  }
  .error {
    color: var(--error-color, #c62828);
    font-size: 0.9em;
  }
</style>
