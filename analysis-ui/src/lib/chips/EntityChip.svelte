<script lang="ts">
  import type { Snippet } from "svelte";

  type Variant =
    | "cohort"
    | "domain"
    | "nameserver"
    | "endpoint"
    | "asn"
    | "prefix"
    | "tag"
    | "testcase";

  type Props = {
    href: string;
    variant: Variant;
    title?: string;
    external?: boolean;
    children?: Snippet;
  };

  let { href, variant, title, external = false, children }: Props = $props();
</script>

<a
  class={`entity-chip entity-chip-${variant}`}
  {href}
  title={title ?? undefined}
  target={external ? "_blank" : undefined}
  rel={external ? "noopener noreferrer" : undefined}
>
  {@render children?.()}
</a>

<style>
  .entity-chip {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 2px 8px;
    border-radius: 6px;
    font-family: var(--mono);
    font-size: var(--text-xs);
    font-weight: 600;
    line-height: 1.4;
    text-decoration: none;
    border: 1px solid var(--border);
    background: var(--surface-2);
    color: var(--ink);
    white-space: nowrap;
  }

  .entity-chip:hover {
    background: var(--accent-2);
    color: var(--btn-fg);
    border-color: var(--accent-2);
  }

  .entity-chip-cohort,
  .entity-chip-tag {
    background: rgba(3, 105, 161, 0.12);
    border-color: rgba(3, 105, 161, 0.25);
    color: var(--accent-2);
  }

  .entity-chip-domain {
    background: var(--surface);
  }

  .entity-chip-asn {
    background: #eef2f7;
    color: #334155;
    border-color: #c8d3e0;
  }
</style>
