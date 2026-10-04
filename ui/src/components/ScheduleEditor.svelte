<script>
  import { t, locale } from "../i18n.js";
  import { apiCall } from "../lib/api.js";
  import { formatTimestampLocal } from "../lib/format.js";
  import ConfirmDialog from "./ConfirmDialog.svelte";

  let {
    open = false,
    cohort = null,
    schedule = null,
    profiles = [],
    apiBase = "/api/v1",
    onsave = () => {},
    ondelete = () => {},
    onclose = () => {},
  } = $props();

  const KINDS = ["interval", "weekly", "monthly"];
  const WEEKDAYS = ["mon", "tue", "wed", "thu", "fri", "sat", "sun"];
  const DAYS = Array.from({ length: 28 }, (_, i) => i + 1);
  const PREVIEW_COUNT = 3;
  const PREVIEW_DELAY_MS = 250;
  const pad2 = (n) => String(n).padStart(2, "0");

  let draft = $state(draftFrom(null));
  let saving = $state(false);
  let error = $state("");
  let confirmRemove = $state(false);
  let preview = $state({ summary: "", next: [], error: "" });
  let dialogEl = $state(null);

  function todayLocal() {
    const d = new Date();
    return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`;
  }

  function draftFrom(s) {
    return {
      enabled: s ? s.enabled !== false : true,
      kind: s?.kind || "monthly",
      interval_days: s?.interval_days || 1,
      anchor_date: s?.anchor_date || todayLocal(),
      weekdays: [...(s?.weekdays || [])],
      days_of_month: [...(s?.days_of_month || (s ? [] : [1]))],
      last_day: !!s?.last_day,
      time_of_day: s?.time_of_day || "02:00",
      timezone: s?.timezone || "UTC",
      profile_id: s?.profile_id == null ? "" : String(s.profile_id),
      promote_default: !!s?.promote_default,
      catch_up: s ? s.catch_up !== false : true,
    };
  }

  // Reset the draft and move focus in each time the editor opens.
  let wasOpen = false;
  $effect(() => {
    if (open && !wasOpen) {
      draft = draftFrom(schedule);
      error = "";
      confirmRemove = false;
      preview = { summary: "", next: [], error: "" };
      queueMicrotask(() => focusables()[0]?.focus());
    }
    wasOpen = open;
  });

  const requestBody = $derived({
    enabled: draft.enabled,
    kind: draft.kind,
    interval_days: Number(draft.interval_days) || 0,
    anchor_date: draft.kind === "interval" ? draft.anchor_date : "",
    weekdays: WEEKDAYS.filter((d) => draft.weekdays.includes(d)),
    days_of_month: [...draft.days_of_month].sort((a, b) => a - b),
    last_day: draft.last_day,
    time_of_day: draft.time_of_day,
    timezone: draft.timezone,
    profile_id: draft.profile_id === "" ? null : Number(draft.profile_id),
    promote_default: draft.promote_default,
    catch_up: draft.catch_up,
  });
  const previewKey = $derived(JSON.stringify(requestBody));

  $effect(() => {
    if (!open || !cohort) return;
    const body = previewKey;
    const timer = setTimeout(() => loadPreview(body), PREVIEW_DELAY_MS);
    return () => clearTimeout(timer);
  });

  async function loadPreview(body) {
    try {
      const result = await apiCall(apiBase, "/analysis/schedules/preview", { method: "POST", body });
      if (body !== previewKey) return;
      preview = { summary: result?.summary || "", next: (result?.next || []).slice(0, PREVIEW_COUNT), error: "" };
    } catch (err) {
      if (body !== previewKey) return;
      preview = { summary: "", next: [], error: err.message || String(err) };
    }
  }

  const toggleIn = (list, value) =>
    list.includes(value) ? list.filter((v) => v !== value) : [...list, value];

  const weekdayLabel = (index, code) =>
    new Intl.DateTimeFormat(code, { weekday: "short", timeZone: "UTC" })
      .format(new Date(Date.UTC(2024, 0, 1 + index)));

  function zoneOptions(current) {
    const zones = typeof Intl.supportedValuesOf === "function" ? Intl.supportedValuesOf("timeZone") : null;
    if (!zones) return null;
    const list = ["UTC", ...zones.filter((z) => z !== "UTC")];
    if (current && !list.includes(current)) list.push(current);
    return list;
  }
  const zones = $derived(zoneOptions(draft.timezone));

  const useBrowserZone = () => {
    draft.timezone = Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
  };

  // Offset of the viewer's zone at that instant, e.g. "UTC+01:00".
  function offsetLabel(iso) {
    const minutes = -new Date(iso).getTimezoneOffset();
    const sign = minutes < 0 ? "-" : "+";
    const abs = Math.abs(minutes);
    return `UTC${sign}${pad2(Math.floor(abs / 60))}:${pad2(abs % 60)}`;
  }

  async function save() {
    saving = true;
    error = "";
    try {
      const stored = await apiCall(apiBase, `/analysis/cohorts/${cohort.id}/schedule`, {
        method: "PUT",
        body: JSON.stringify(requestBody),
      });
      onsave(stored);
    } catch (err) {
      error = err.message || String(err);
    } finally {
      saving = false;
    }
  }

  async function remove() {
    confirmRemove = false;
    saving = true;
    error = "";
    try {
      await apiCall(apiBase, `/analysis/cohorts/${cohort.id}/schedule`, { method: "DELETE" });
      ondelete();
    } catch (err) {
      error = err.message || String(err);
    } finally {
      saving = false;
    }
  }

  function focusables() {
    if (!dialogEl) return [];
    return [...dialogEl.querySelectorAll("button, input, select")].filter((el) => !el.disabled);
  }

  function onKeydown(e) {
    e.stopPropagation();
    if (e.key === "Escape" && !saving) {
      e.preventDefault();
      onclose();
      return;
    }
    if (e.key !== "Tab") return;
    const items = focusables();
    if (items.length === 0) return;
    const first = items[0];
    const last = items[items.length - 1];
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault();
      first.focus();
    }
  }
</script>

{#if open && cohort}
  <div
    class="modal-backdrop"
    role="presentation"
    onclick={() => { if (!saving) onclose(); }}
  >
    <div
      class="modal-card"
      role="dialog"
      tabindex="-1"
      aria-modal="true"
      aria-labelledby="schedule-editor-title"
      bind:this={dialogEl}
      onclick={(e) => e.stopPropagation()}
      onkeydown={onKeydown}
    >
      <h2 id="schedule-editor-title">{$t("analysis_schedule_editor_title", { tag: cohort.source_tag })}</h2>

      <label class="check-field">
        <input type="checkbox" bind:checked={draft.enabled} />
        <span>{$t("analysis_schedule_enabled")}</span>
      </label>

      <fieldset class="field-group">
        <legend class="field-label">{$t("analysis_schedule_kind")}</legend>
        <div class="kind-row">
          {#each KINDS as kind (kind)}
            <label class="check-field">
              <input type="radio" name="schedule-kind" value={kind} bind:group={draft.kind} />
              <span>{$t(`analysis_schedule_kind_${kind}`)}</span>
            </label>
          {/each}
        </div>

        {#if draft.kind === "interval"}
          <div class="field-row">
            <div class="field">
              <label class="field-label" for="schedule-interval">{$t("analysis_schedule_interval_days")}</label>
              <input id="schedule-interval" type="number" min="1" max="365" bind:value={draft.interval_days} />
            </div>
            <div class="field">
              <label class="field-label" for="schedule-anchor">{$t("analysis_schedule_anchor_date")}</label>
              <input id="schedule-anchor" type="date" bind:value={draft.anchor_date} />
            </div>
          </div>
        {:else if draft.kind === "weekly"}
          <div class="toggle-grid toggle-grid-week" role="group" aria-label={$t("analysis_schedule_weekdays")}>
            {#each WEEKDAYS as day, i (day)}
              <button
                type="button"
                class="day-toggle"
                class:day-on={draft.weekdays.includes(day)}
                aria-pressed={draft.weekdays.includes(day)}
                onclick={() => (draft.weekdays = toggleIn(draft.weekdays, day))}
              >{weekdayLabel(i, $locale)}</button>
            {/each}
          </div>
        {:else}
          <div class="toggle-grid" role="group" aria-label={$t("analysis_schedule_days_of_month")}>
            {#each DAYS as day (day)}
              <button
                type="button"
                class="day-toggle"
                class:day-on={draft.days_of_month.includes(day)}
                aria-pressed={draft.days_of_month.includes(day)}
                onclick={() => (draft.days_of_month = toggleIn(draft.days_of_month, day))}
              >{day}</button>
            {/each}
          </div>
          <button
            type="button"
            class="day-toggle day-toggle-wide"
            class:day-on={draft.last_day}
            aria-pressed={draft.last_day}
            onclick={() => (draft.last_day = !draft.last_day)}
          >{$t("analysis_schedule_last_day")}</button>
        {/if}
      </fieldset>

      <div class="field-row">
        <div class="field">
          <label class="field-label" for="schedule-time">{$t("analysis_schedule_time")}</label>
          <input id="schedule-time" type="time" bind:value={draft.time_of_day} />
        </div>
        <div class="field">
          <label class="field-label" for="schedule-zone">{$t("analysis_schedule_timezone")}</label>
          {#if zones}
            <select id="schedule-zone" bind:value={draft.timezone}>
              {#each zones as zone (zone)}
                <option value={zone}>{zone}</option>
              {/each}
            </select>
          {:else}
            <input id="schedule-zone" type="text" bind:value={draft.timezone} />
          {/if}
        </div>
        <button type="button" class="ghost small zone-button" onclick={useBrowserZone}>
          {$t("analysis_schedule_use_browser_zone")}
        </button>
      </div>

      <div class="field">
        <label class="field-label" for="schedule-profile">{$t("analysis_schedule_profile")}</label>
        <select id="schedule-profile" bind:value={draft.profile_id}>
          <option value="">{$t("analysis_schedule_profile_tag_default")}</option>
          {#each profiles as profile (profile.id)}
            <option value={String(profile.id)}>{profile.name}</option>
          {/each}
        </select>
      </div>

      <label class="check-field">
        <input type="checkbox" bind:checked={draft.promote_default} />
        <span>{$t("analysis_schedule_promote_default")}</span>
      </label>
      <label class="check-field">
        <input type="checkbox" bind:checked={draft.catch_up} />
        <span>{$t("analysis_schedule_catch_up")}</span>
      </label>

      <section class="preview" aria-live="polite">
        <h3>{$t("analysis_schedule_preview")}</h3>
        {#if preview.error}
          <p class="status-banner warn">{preview.error}</p>
        {:else if preview.next.length > 0}
          <p class="small">{preview.summary}</p>
          <ol class="preview-list">
            {#each preview.next as at (at)}
              <li><time datetime={at}>{formatTimestampLocal(at)} {offsetLabel(at)}</time></li>
            {/each}
          </ol>
        {:else}
          <p class="small muted">{$t("loading")}</p>
        {/if}
      </section>

      {#if error}
        <p class="status-banner warn" role="alert">{error}</p>
      {/if}

      <div class="actions">
        {#if schedule}
          <button type="button" class="ghost warn remove" disabled={saving} onclick={() => (confirmRemove = true)}>
            {$t("analysis_schedule_remove")}
          </button>
        {/if}
        <button type="button" class="ghost" disabled={saving} onclick={onclose}>{$t("cancel")}</button>
        <button type="button" disabled={saving} onclick={save}>
          {saving ? $t("submitting") : $t("analysis_schedule_save")}
        </button>
      </div>
    </div>
  </div>
{/if}

<ConfirmDialog
  open={confirmRemove}
  title={cohort ? $t("analysis_schedule_remove_confirm", { tag: cohort.source_tag }) : ""}
  confirmLabel={$t("analysis_schedule_remove")}
  onConfirm={remove}
  onCancel={() => (confirmRemove = false)}
/>

<style>
  .modal-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.45);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 900;
    padding: 1rem;
  }
  .modal-card {
    background: var(--surface);
    color: var(--ink);
    border-radius: 8px;
    padding: 1.5rem;
    max-width: 560px;
    width: 100%;
    max-height: 90vh;
    overflow-y: auto;
    box-shadow: 0 10px 30px rgba(0, 0, 0, 0.3);
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  h2 {
    margin: 0;
  }
  h3 {
    margin: 0 0 6px;
    font-size: var(--text-sm);
  }
  .field-group {
    margin: 0;
    padding: 10px 12px;
    border: 1px solid var(--border);
    border-radius: 8px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .field-label {
    color: var(--ink-2);
    font-size: var(--text-xs);
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }
  .kind-row,
  .field-row {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-end;
    gap: 12px 18px;
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: var(--text-sm);
  }
  .check-field {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    font-size: var(--text-sm);
    color: var(--ink);
  }
  .check-field input {
    width: auto;
    margin: 0;
    flex-shrink: 0;
  }
  .toggle-grid {
    display: grid;
    grid-template-columns: repeat(7, minmax(0, 1fr));
    gap: 4px;
  }
  .day-toggle {
    padding: 4px 0;
    font-size: var(--text-xs);
    font-weight: 500;
    color: var(--ink-2);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: 6px;
    box-shadow: none;
    cursor: pointer;
  }
  .day-toggle:hover {
    transform: none;
    box-shadow: none;
    color: var(--ink);
  }
  .day-toggle.day-on {
    color: var(--btn-fg);
    background: var(--accent-2);
    border-color: var(--accent-2);
    font-weight: 600;
  }
  .day-toggle-wide {
    align-self: flex-start;
    padding: 4px 12px;
  }
  .zone-button {
    align-self: flex-end;
  }
  .preview {
    padding: 10px 12px;
    background: var(--surface-2);
    border-radius: 8px;
  }
  .preview p {
    margin: 0 0 4px;
  }
  .preview-list {
    margin: 0;
    padding-left: 1.25rem;
    font-family: var(--mono);
    font-size: var(--text-sm);
  }
  .muted {
    color: var(--ink-2);
  }
  .actions {
    display: flex;
    gap: 0.5rem;
    justify-content: flex-end;
  }
  .actions .remove {
    margin-right: auto;
  }
</style>
