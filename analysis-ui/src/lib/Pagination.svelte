<script lang="ts">
  import { goto } from "$app/navigation";
  import { page } from "$app/state";
  import { formatCount } from "$lib/format";
  import { searchToString } from "$lib/filters";

  type Props = { total: number; offset: number; limit: number; itemCount: number };
  let { total, offset, limit, itemCount }: Props = $props();

  const pageStart = $derived(total === 0 ? 0 : offset + 1);
  const pageEnd = $derived(Math.min(offset + itemCount, total));
  const hasPrev = $derived(offset > 0);
  const hasNext = $derived(offset + limit < total);

  function gotoOffset(nextOffset: number) {
    const params = new URLSearchParams(page.url.searchParams);
    if (nextOffset > 0) params.set("offset", String(nextOffset));
    else params.delete("offset");
    goto(`${page.url.pathname}${searchToString(params)}`);
  }
</script>

<div class="pagination">
  <span class="hint">
    Showing {pageStart}–{pageEnd} of {formatCount(total)}
  </span>
  <div class="pagination-controls">
    <button
      type="button"
      class="ghost"
      disabled={!hasPrev}
      onclick={() => gotoOffset(Math.max(0, offset - limit))}
    >
      ← Previous
    </button>
    <button
      type="button"
      class="ghost"
      disabled={!hasNext}
      onclick={() => gotoOffset(offset + limit)}
    >
      Next →
    </button>
  </div>
</div>

<style>
  .pagination {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: var(--space-3);
    flex-wrap: wrap;
    margin-top: var(--space-3);
  }
  .pagination-controls { display: flex; gap: var(--space-2); }
</style>
