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
    background: color-mix(in srgb, var(--accent-2) 12%, transparent);
    border-color: color-mix(in srgb, var(--accent-2) 25%, transparent);
    color: var(--accent-2);
  }

  .entity-chip-domain {
    background: var(--surface);
  }

  .entity-chip-asn {
    background: color-mix(in srgb, var(--accent-2) 8%, var(--surface-2));
    color: var(--ink);
    border-color: color-mix(in srgb, var(--accent-2) 25%, var(--border));
  }

  /* Mark chips that open in a new tab. Kept as ::after so it stays out of
     the accessibility tree; target="_blank" already conveys the semantics. */
  .entity-chip[target="_blank"]::after {
    content: "↗";
    font-size: 0.9em;
    opacity: 0.7;
  }
</style>
