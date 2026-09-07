<script lang="ts">
  import type { ExternalLink } from "$lib/externalLinks";

  type Props = {
    links: ExternalLink[];
    heading?: string;
  };

  let { links, heading = "Elsewhere" }: Props = $props();
</script>

{#if links.length > 0}
  <nav class="external-links" aria-label="External references">
    <span class="external-links-heading">{heading}</span>
    <ul role="list">
      {#each links as link (link.href)}
        <li>
          <a href={link.href} title={link.title} target="_blank" rel="noopener noreferrer">
            {link.label}
          </a>
        </li>
      {/each}
    </ul>
  </nav>
{/if}

<style>
  .external-links {
    display: flex;
    align-items: baseline;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
  .external-links-heading {
    font-size: var(--text-xs);
    color: var(--ink-2);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  ul {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    list-style: none;
    margin: 0;
    padding: 0;
  }
  a {
    font-size: var(--text-sm);
    color: var(--accent-2);
    text-decoration: none;
  }
  a:hover {
    text-decoration: underline;
  }

  /* Same new-tab marker the entity chips use; ::after keeps it out of the
     accessibility tree, where target="_blank" already carries the meaning. */
  a[target="_blank"]::after {
    content: "↗";
    font-size: 0.9em;
    opacity: 0.7;
  }
</style>
