import adapter from "@sveltejs/adapter-static";
import { sveltekit } from "@sveltejs/kit/vite";
import { vitePreprocess } from "@sveltejs/vite-plugin-svelte";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [
    sveltekit({
      preprocess: vitePreprocess(),
      // SPA embedded in gonemaster-server; the Go handler serves index.html for unknown paths.
      adapter: adapter({
        pages: "../server/analysisui/dist",
        assets: "../server/analysisui/dist",
        fallback: "index.html",
        precompress: false,
        strict: false
      }),
      paths: {
        base: "/analysis"
      },
      version: {
        pollInterval: 0
      }
    })
  ],
  resolve: {
    // Svelte's browser entry under vitest; the server build throws in mount().
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
