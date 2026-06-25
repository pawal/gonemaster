import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";

export default defineConfig({
  plugins: [svelte()],
  base: "/",
  resolve: {
    conditions: ["browser"]
  },
  build: {
    outDir: "../server/ui/dist",
    emptyOutDir: true,
    modulePreload: { polyfill: false }
  },
  test: {
    environment: "jsdom",
    setupFiles: "./src/test/setup.js",
    clearMocks: true,
    // CI containers oversubscribe CPU (workers = host cores, but the container
    // has a smaller quota), so wall-clock balloons under contention. Give the
    // 5s default headroom so a fast test is not killed mid-starvation.
    testTimeout: 20000,
    hookTimeout: 20000
  },
  server: {
    port: 5173
  }
});
