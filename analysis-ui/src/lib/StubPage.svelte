<script lang="ts">
  type Props = {
    heading: string;
    note?: string;
  };

  let { heading, note = "Populated in phase 5 — the SvelteKit routing scaffold is in place." }: Props =
    $props();
</script>

<section class="card">
  <h2>{heading}</h2>
  <p class="hint">{note}</p>
  <p class="placeholder-note">Route wired · data binding pending.</p>
</section>
