<script lang="ts">
  // Indeterminate top progress bar. Blocking SvelteKit loads give no feedback
  // otherwise, so a slow query looks like a dead click. Driven by a plain
  // boolean so it stays testable without the navigation store.
  type Props = { active: boolean };
  let { active }: Props = $props();
</script>

<div class="nav-progress" class:active aria-hidden="true"></div>

<style>
  .nav-progress {
    position: fixed;
    top: 0;
    left: 0;
    right: 0;
    height: 2px;
    z-index: 20;
    overflow: hidden;
    pointer-events: none;
    opacity: 0;
    transition: opacity 0.15s ease;
  }
  .nav-progress.active {
    opacity: 1;
  }
  .nav-progress.active::before {
    content: "";
    position: absolute;
    top: 0;
    height: 100%;
    width: 40%;
    background: var(--accent-2);
    animation: nav-progress-slide 1s ease-in-out infinite;
  }
  @keyframes nav-progress-slide {
    0% { left: -40%; }
    100% { left: 100%; }
  }
  @media (prefers-reduced-motion: reduce) {
    .nav-progress.active::before {
      animation: none;
      left: 0;
      width: 100%;
      opacity: 0.5;
    }
  }
</style>
