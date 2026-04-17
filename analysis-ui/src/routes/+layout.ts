// The analysis UI is a pure client-side SPA embedded in the Go server.
// Disable SSR and prerendering so adapter-static emits a single fallback HTML.
export const ssr = false;
export const prerender = false;
export const trailingSlash = "never";
