<script>
  import { t } from "../i18n.js";

  let {
    open = false,
    title = "",
    message = "",
    confirmLabel = "",
    cancelLabel = "",
    confirmPhrase = "",
    phrasePrompt = "",
    busy = false,
    onConfirm = () => {},
    onCancel = () => {},
  } = $props();

  let typed = $state("");
  let confirmEl;
  let phraseEl;

  const canConfirm = $derived(!confirmPhrase || typed.trim() === confirmPhrase);

  // Reset the typed phrase and move focus in each time the dialog opens.
  let wasOpen = false;
  $effect(() => {
    if (open && !wasOpen) {
      typed = "";
      queueMicrotask(() => (confirmPhrase ? phraseEl : confirmEl)?.focus());
    }
    wasOpen = open;
  });

  const onKeydown = (e) => {
    if (e.key === "Escape") { e.preventDefault(); onCancel(); }
  };
</script>

{#if open}
  <div class="modal-backdrop" role="presentation" onclick={onCancel}>
    <div
      class="modal-card confirm-card"
      role="dialog"
      aria-modal="true"
      aria-label={title}
      onclick={(e) => e.stopPropagation()}
      onkeydown={onKeydown}
    >
      <h2>{title}</h2>
      {#if message}<p>{message}</p>{/if}
      {#if confirmPhrase}
        <label class="confirm-phrase-label" for="confirm-phrase">{phrasePrompt}</label>
        <input id="confirm-phrase" bind:this={phraseEl} type="text" bind:value={typed} autocomplete="off" />
      {/if}
      <div class="actions">
        <button class="ghost" type="button" onclick={onCancel}>{cancelLabel || $t("cancel")}</button>
        <button
          class="warn"
          type="button"
          bind:this={confirmEl}
          disabled={!canConfirm || busy}
          onclick={onConfirm}
        >{busy ? $t("submitting") : confirmLabel}</button>
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
    max-width: 460px;
    width: 100%;
    box-shadow: 0 10px 30px rgba(0, 0, 0, 0.3);
  }
  h2 {
    margin-top: 0;
  }
  .confirm-phrase-label {
    display: block;
    margin-top: 0.75rem;
    font-size: 0.9em;
  }
  input[type="text"] {
    width: 100%;
    box-sizing: border-box;
    padding: 0.4rem 0.5rem;
    margin-top: 0.25rem;
  }
  .actions {
    display: flex;
    gap: 0.5rem;
    justify-content: flex-end;
    margin-top: 1.25rem;
  }
</style>
