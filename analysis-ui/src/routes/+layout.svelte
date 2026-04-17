<script lang="ts">
  import "../app.css";

  let { children } = $props();
</script>

<div class="app-shell">
  <header class="app-header">
    <div class="brand">Gonemaster Analysis</div>
  </header>
  <main class="app-main">
    {@render children?.()}
  </main>
</div>
