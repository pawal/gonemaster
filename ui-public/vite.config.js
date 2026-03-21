import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";

export default defineConfig({
  plugins: [svelte()],
  base: "/public/",
  resolve: {
    conditions: ["browser"]
  },
  build: {
    outDir: "../server/public/dist",
    emptyOutDir: true
  },
  test: {
    environment: "jsdom",
    setupFiles: "./src/test/setup.js",
    clearMocks: true
  },
  server: {
    port: 5174
  }
});
