import adapter from "@sveltejs/adapter-static";
import { vitePreprocess } from "@sveltejs/vite-plugin-svelte";

// SPA build targeting embedding inside gonemaster-server. The fallback file
// ("index.html") is what the Go handler serves for unknown paths so the
// client-side router can take over.
export default {
  preprocess: vitePreprocess(),
  kit: {
    adapter: adapter({
      pages: "../server/analysisui/dist",
      assets: "../server/analysisui/dist",
      fallback: "index.html",
      precompress: false,
      strict: false
    }),
    paths: {
      base: "/analysis"
    }
  }
};
