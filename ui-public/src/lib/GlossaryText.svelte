<script>
  import { t } from "../i18n.js";
  import { glossarySlugs, buildEntries, linkify } from "./glossary.js";
  import GlossaryTerm from "./GlossaryTerm.svelte";

  let { text = "" } = $props();
  const slugs = glossarySlugs();
  let segments = $derived(linkify(text, buildEntries($t, slugs)));
</script>
{#each segments as seg, i (i)}{#if seg.type === "term"}<GlossaryTerm term={seg.value} slug={seg.slug} />{:else}{seg.value}{/if}{/each}
