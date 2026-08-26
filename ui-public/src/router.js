// Routes: BASE is home, BASE + "result/:id" is a result. Hash forms are legacy.

// Vite's base and the server's mount point. Not import.meta.env.BASE_URL:
// vitest reports "/" for it. Pinned by router.test.js.
export const BASE = "/public/";

export function parsePath(pathname) {
  const rest = String(pathname ?? "").startsWith(BASE) ? String(pathname).slice(BASE.length) : "";
  const m = rest.match(/^result\/([A-Za-z0-9]+)\/?$/);
  if (m) return { view: "result", publicID: m[1] };
  return { view: "home", publicID: null };
}

export function pathFor(view, publicID = null) {
  if (view === "result" && publicID) return `${BASE}result/${publicID}`;
  return BASE;
}

// Keeps the query so ?lang survives. Callers update app state themselves.
export function navigate(view, publicID = null) {
  window.history.pushState(null, "", pathFor(view, publicID) + window.location.search);
}

export function parseHash(hash) {
  const path = hash.startsWith("#") ? hash.slice(1) : hash;
  const m = path.match(/^\/result\/([A-Za-z0-9]+)$/);
  if (m) return { view: "result", publicID: m[1] };
  return { view: "home", publicID: null };
}

export function hashFor(view, publicID = null) {
  if (view === "result" && publicID) return `#/result/${publicID}`;
  return "#/";
}

// Rewrites an old #/result/:id link to the path form. A path route wins.
export function upgradeLegacyHash() {
  const { view, publicID } = parseHash(window.location.hash);
  if (view !== "result") return false;
  if (parsePath(window.location.pathname).view !== "home") return false;
  window.history.replaceState(null, "", pathFor("result", publicID) + window.location.search);
  return true;
}

// Middle- and modified clicks must still open a new tab, so only plain ones
// are intercepted.
export function isPlainClick(event) {
  return event.button === 0 && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey;
}

export function readLang() {
  return new URLSearchParams(window.location.search).get("lang") ?? "";
}

// Keeps the chosen language in the URL so it stays shareable, and so the
// server renders the same language for clients that do not run scripts.
export function writeLang(code) {
  const params = new URLSearchParams(window.location.search);
  params.set("lang", code);
  const query = params.toString();
  window.history.replaceState(null, "", window.location.pathname + (query ? `?${query}` : ""));
}
