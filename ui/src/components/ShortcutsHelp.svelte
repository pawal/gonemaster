<script>
  import { t } from "../i18n.js";

  let { open = false, onClose = () => {} } = $props();

  const globalRows = [
    { keys: ["?"], labelKey: "shortcut_help_toggle" },
    { keys: ["n"], labelKey: "shortcut_new_scan" },
    { keys: ["/"], labelKey: "shortcut_focus_filter" },
    { keys: ["Esc"], labelKey: "shortcut_close" },
  ];
  const navRows = [
    { keys: ["g", "s"], labelKey: "tab_single" },
    { keys: ["g", "r"], labelKey: "tab_recent" },
    { keys: ["g", "d"], labelKey: "tab_domains" },
    { keys: ["g", "t"], labelKey: "tab_tags" },
    { keys: ["g", "c"], labelKey: "tab_cohorts" },
    { keys: ["g", "b"], labelKey: "tab_batches" },
    { keys: ["g", "m"], labelKey: "tab_metrics" },
    { keys: ["g", ","], labelKey: "tab_settings" },
  ];
  const resultRows = [
    { keys: ["j", "↓"], labelKey: "shortcut_row_next" },
    { keys: ["k", "↑"], labelKey: "shortcut_row_prev" },
    { keys: ["Enter", "Space"], labelKey: "shortcut_row_open" },
  ];

  let closeEl = $state(null);

  // Move focus into the dialog each time it opens.
  let wasOpen = false;
  $effect(() => {
    if (open && !wasOpen) queueMicrotask(() => closeEl?.focus());
    wasOpen = open;
  });

  const onKeydown = (e) => {
    if (e.key === "Escape") { e.preventDefault(); onClose(); }
  };
</script>

{#if open}
  <div class="modal-backdrop" role="presentation" onclick={onClose}>
    <div
      class="modal-card shortcuts-card"
      role="dialog"
      tabindex="-1"
      aria-modal="true"
      aria-label={$t("shortcuts_help_aria")}
      onclick={(e) => e.stopPropagation()}
      onkeydown={onKeydown}
    >
      <h2>{$t("shortcuts_help_title")}</h2>

      <section class="shortcut-section">
        <h3>{$t("shortcuts_section_global")}</h3>
        <dl class="shortcut-list">
          {#each globalRows as row}
            <div class="shortcut-row">
              <dt class="shortcut-keys">
                {#each row.keys as key}<kbd>{key}</kbd>{/each}
              </dt>
              <dd>{$t(row.labelKey)}</dd>
            </div>
          {/each}
        </dl>
      </section>

      <section class="shortcut-section">
        <h3>{$t("shortcuts_section_navigation")}</h3>
        <p class="shortcut-hint">{$t("shortcut_go_hint")}</p>
        <dl class="shortcut-list">
          {#each navRows as row}
            <div class="shortcut-row">
              <dt class="shortcut-keys">
                {#each row.keys as key}<kbd>{key}</kbd>{/each}
              </dt>
              <dd>{$t(row.labelKey)}</dd>
            </div>
          {/each}
        </dl>
      </section>

      <section class="shortcut-section">
        <h3>{$t("shortcuts_section_results")}</h3>
        <dl class="shortcut-list">
          {#each resultRows as row}
            <div class="shortcut-row">
              <dt class="shortcut-keys">
                {#each row.keys as key}<kbd>{key}</kbd>{/each}
              </dt>
              <dd>{$t(row.labelKey)}</dd>
            </div>
          {/each}
        </dl>
      </section>

      <div class="actions">
        <button class="ghost" type="button" bind:this={closeEl} onclick={onClose}>{$t("shortcut_close")}</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .modal-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.5);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
    padding: 1rem;
  }
  .modal-card {
    background: var(--surface, #fff);
    color: var(--ink, #111);
    border-radius: 8px;
    padding: 1.5rem;
    max-width: 480px;
    width: 100%;
    max-height: 85vh;
    overflow-y: auto;
    box-shadow: 0 10px 30px rgba(0, 0, 0, 0.3);
  }
  h2 {
    margin-top: 0;
  }
  .shortcut-section {
    margin-top: 1rem;
  }
  .shortcut-section h3 {
    margin: 0 0 0.4rem;
    font-size: var(--text-sm);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--muted);
  }
  .shortcut-hint {
    margin: 0 0 0.4rem;
    font-size: var(--text-sm);
    color: var(--muted);
  }
  .shortcut-list {
    margin: 0;
  }
  .shortcut-row {
    display: flex;
    align-items: baseline;
    gap: 0.75rem;
    padding: 0.2rem 0;
  }
  .shortcut-keys {
    flex: 0 0 6.5rem;
    display: flex;
    gap: 0.25rem;
  }
  .shortcut-row dd {
    margin: 0;
  }
  kbd {
    display: inline-block;
    min-width: 1.4em;
    padding: 0.1em 0.4em;
    font-family: var(--mono, monospace);
    font-size: 0.85em;
    line-height: 1.4;
    text-align: center;
    color: var(--ink, #111);
    background: var(--surface-2, #f2f2f2);
    border: 1px solid var(--border, #ccc);
    border-radius: 4px;
    box-shadow: 0 1px 0 var(--border, #ccc);
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    margin-top: 1.25rem;
  }
</style>
