import { sveltekit } from "@sveltejs/kit/vite";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [sveltekit()],
  resolve: {
    // Ensure svelte resolves to its browser entry when running unit tests
    // under vitest; otherwise `mount()` hits the server build and throws.
    conditions: process.env.VITEST ? ["browser"] : undefined
  },
  server: {
    port: 5175
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: "./src/test/setup.ts",
    clearMocks: true
  }
});
