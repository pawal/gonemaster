<script>
  // The pinned object's tip lines, as ordinary DOM a tap and a keyboard reach.
  let { heading, lines, label, closeLabel, onclose } = $props();
</script>

<aside class="chain-detail" aria-label={label} data-testid="chain-detail">
  <div class="chain-detail-head">
    <h3>{heading}</h3>
    <button
      type="button"
      class="chain-detail-close"
      aria-label={closeLabel}
      data-testid="chain-detail-close"
      onclick={onclose}
      onkeydown={(e) => e.key === "Escape" && onclose()}
    >
      ×
    </button>
  </div>
  <ul>
    {#each lines as line, i (i)}
      <li>{line}</li>
    {/each}
  </ul>
</aside>

<style>
  .chain-detail {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    padding: 0.6rem 0.75rem;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--surface-2);
  }
  .chain-detail-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 0.5rem;
  }
  h3 {
    margin: 0;
    font-size: 0.9rem;
    color: var(--ink);
    overflow-wrap: anywhere;
  }
  ul {
    margin: 0;
    padding: 0;
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  li {
    font-size: 0.82rem;
    color: var(--ink-2);
    overflow-wrap: anywhere;
  }
  .chain-detail-close {
    padding: 0.1rem 0.4rem;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--surface);
    color: var(--ink-2);
    font: inherit;
    font-size: 0.85rem;
    line-height: 1.2;
    cursor: pointer;
  }
</style>
