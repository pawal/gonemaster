<script>
  import { t } from "../i18n.js";

  let { term, slug } = $props();
  let definition = $derived($t(`pub.glossary.${slug}`));

  let open = $state(false);
  let triggerEl = $state();
  let tipEl = $state();

  // Fixed positioning escapes the module-card's overflow:hidden. Anchor
  // above the term, flip below when there is no room, clamp to viewport.
  function reposition() {
    if (!triggerEl || !tipEl) return;
    const r = triggerEl.getBoundingClientRect();
    const w = tipEl.offsetWidth;
    const h = tipEl.offsetHeight;
    const left = Math.min(Math.max(8, r.left), window.innerWidth - w - 8);
    let top = r.top - h - 8;
    if (top < 8) top = r.bottom + 8;
    tipEl.style.left = `${left}px`;
    tipEl.style.top = `${top}px`;
  }

  const show = () => (open = true);
  const hide = () => (open = false);

  $effect(() => {
    if (!open) return;
    reposition();
    const onMove = () => hide();
    window.addEventListener("scroll", onMove, { passive: true, capture: true });
    window.addEventListener("resize", onMove, { passive: true });
    return () => {
      window.removeEventListener("scroll", onMove, { capture: true });
      window.removeEventListener("resize", onMove);
    };
  });
</script>

<button
  bind:this={triggerEl}
  type="button"
  class="glossary-trigger"
  data-testid="glossary-term"
  aria-label={`${term}: ${definition}`}
  onmouseenter={show}
  onmouseleave={hide}
  onfocus={show}
  onblur={hide}
  onclick={show}
>{term}</button>{#if open}<span bind:this={tipEl} class="glossary-tip" role="tooltip" data-testid="glossary-tip">{definition}</span>{/if}

<style>
  .glossary-trigger {
    font: inherit;
    color: inherit;
    background: none;
    border: none;
    padding: 0;
    cursor: help;
    text-decoration: underline dotted;
    text-underline-offset: 2px;
    text-decoration-thickness: 1px;
  }
  .glossary-trigger:focus-visible {
    outline: 2px solid var(--accent, #2563eb);
    outline-offset: 2px;
    border-radius: 2px;
  }
  /* Fixed dark palette reads well on both light and dark themes. */
  .glossary-tip {
    position: fixed;
    top: 0;
    left: 0;
    z-index: 1000;
    max-width: min(280px, calc(100vw - 16px));
    padding: 8px 10px;
    border-radius: 6px;
    background: #1f2937;
    color: #f9fafb;
    border: 1px solid rgba(255, 255, 255, 0.12);
    font-size: var(--text-sm, 0.8125rem);
    font-weight: 400;
    line-height: 1.4;
    text-align: left;
    box-shadow: 0 6px 20px rgba(0, 0, 0, 0.25);
    pointer-events: none;
  }
</style>
