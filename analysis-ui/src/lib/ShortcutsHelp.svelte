<script lang="ts">
  import { navItems } from "$lib/nav";
  import { shortcutNavRows } from "$lib/shortcuts";

  let { open = false, onClose = () => {} }: { open?: boolean; onClose?: () => void } = $props();

  const globalRows = [
    { keys: ["?"], label: "Show or hide this help" },
    { keys: ["/"], label: "Focus the search filter" },
    { keys: ["Esc"], label: "Close this help" }
  ];
  const navRows = shortcutNavRows(navItems);

  let closeEl = $state<HTMLButtonElement | null>(null);

  // Move focus into the dialog each time it opens.
  let wasOpen = false;
  $effect(() => {
    if (open && !wasOpen) queueMicrotask(() => closeEl?.focus());
    wasOpen = open;
  });

  function onKeydown(e: KeyboardEvent) {
    if (e.key === "Escape") {
      e.preventDefault();
      onClose();
    }
  }
</script>

{#if open}
  <div class="modal-backdrop" role="presentation" onclick={onClose}>
    <div
      class="modal-card"
      role="dialog"
      tabindex="-1"
      aria-modal="true"
      aria-label="Keyboard shortcuts help"
      onclick={(e) => e.stopPropagation()}
      onkeydown={onKeydown}
    >
      <h2>Keyboard shortcuts</h2>

      <section class="shortcut-section">
        <h3>Global</h3>
        <dl class="shortcut-list">
          {#each globalRows as row (row.label)}
            <div class="shortcut-row">
              <dt class="shortcut-keys">
                {#each row.keys as key (key)}<kbd>{key}</kbd>{/each}
              </dt>
              <dd>{row.label}</dd>
            </div>
          {/each}
        </dl>
      </section>

      <section class="shortcut-section">
        <h3>Go to section</h3>
        <p class="shortcut-hint">Press g, then a letter.</p>
        <dl class="shortcut-list">
          {#each navRows as row (row.href)}
            <div class="shortcut-row">
              <dt class="shortcut-keys">
                <kbd>g</kbd><kbd>{row.key}</kbd>
              </dt>
              <dd>{row.label}</dd>
            </div>
          {/each}
        </dl>
      </section>

      <div class="actions">
        <button class="ghost" type="button" bind:this={closeEl} onclick={onClose}>Close</button>
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
    padding: var(--space-4);
  }
  .modal-card {
    background: var(--card);
    color: var(--ink);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: var(--space-5);
    max-width: 460px;
    width: 100%;
    max-height: 85vh;
    overflow-y: auto;
    box-shadow: 0 10px 30px rgba(0, 0, 0, 0.3);
  }
  h2 {
    margin-top: 0;
    font-size: var(--text-lg);
  }
  .shortcut-section {
    margin-top: var(--space-4);
  }
  .shortcut-section h3 {
    margin: 0 0 var(--space-2);
    font-size: var(--text-xs);
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--muted);
  }
  .shortcut-hint {
    margin: 0 0 var(--space-2);
    font-size: var(--text-sm);
    color: var(--muted);
  }
  .shortcut-list {
    margin: 0;
  }
  .shortcut-row {
    display: flex;
    align-items: baseline;
    gap: var(--space-3);
    padding: 3px 0;
  }
  .shortcut-keys {
    flex: 0 0 5rem;
    display: flex;
    gap: 4px;
  }
  .shortcut-row dd {
    margin: 0;
    color: var(--ink);
  }
  kbd {
    display: inline-block;
    min-width: 1.4em;
    padding: 1px 6px;
    font-family: var(--mono);
    font-size: 0.85em;
    line-height: 1.4;
    text-align: center;
    color: var(--ink);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: 4px;
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    margin-top: var(--space-4);
  }
  .ghost {
    padding: 6px 14px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--surface);
    color: var(--ink);
    font: inherit;
    font-size: var(--text-sm);
    cursor: pointer;
  }
  .ghost:hover {
    background: var(--surface-2);
  }
</style>
