<script>
  import { t } from "../i18n.js";
  import { hashFor } from "../router.js";

  let { entries = [], locale = "en", onclear } = $props();

  let fmt = $derived(new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short" }));

  function formatDate(iso) {
    if (!iso) return "";
    const ms = Date.parse(iso);
    return Number.isNaN(ms) ? "" : fmt.format(ms);
  }
</script>

<section class="recent-tests" data-testid="recent-tests" aria-label={$t("pub.history_title")}>
  <div class="recent-head">
    <h2 class="recent-title">{$t("pub.history_title")}</h2>
    <button type="button" class="recent-clear" onclick={() => onclear?.()}>{$t("pub.history_clear")}</button>
  </div>
  <ul class="recent-list">
    {#each entries as entry (entry.id)}
      <li class="recent-row">
        <a class="recent-domain" href={hashFor("result", entry.id)}>{entry.domain}</a>
        <span class="recent-meta">
          {#if entry.grade}
            <span class="grade-chip-letter" data-grade={entry.grade}>{entry.grade}</span>
          {/if}
          <span class="recent-date">{formatDate(entry.finishedAt)}</span>
        </span>
      </li>
    {/each}
  </ul>
</section>

<style>
  .recent-tests {
    margin-top: var(--space-8);
  }
  .recent-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--space-4);
  }
  .recent-title {
    margin: 0;
    font-size: var(--text-sm);
    font-weight: 600;
    color: var(--muted);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .recent-clear {
    background: none;
    border: none;
    padding: 0;
    color: var(--muted);
    font-size: var(--text-xs);
    cursor: pointer;
    text-decoration: underline;
  }
  .recent-clear:hover {
    color: var(--ink-2);
  }
  .recent-list {
    list-style: none;
    margin: var(--space-2) 0 0;
    padding: 0;
  }
  .recent-row {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    padding: var(--space-2) 0;
    border-bottom: 1px solid var(--border);
  }
  .recent-row:last-child {
    border-bottom: none;
  }
  .recent-domain {
    flex: 1 1 auto;
    min-width: 0;
    overflow-wrap: anywhere;
    color: var(--accent-2);
    font-size: var(--text-base);
    text-decoration: none;
  }
  .recent-domain:hover {
    text-decoration: underline;
  }
  .recent-meta {
    display: inline-flex;
    align-items: center;
    gap: var(--space-3);
    flex-shrink: 0;
  }
  .recent-date {
    color: var(--muted);
    font-size: var(--text-xs);
    font-variant-numeric: tabular-nums;
  }
</style>
