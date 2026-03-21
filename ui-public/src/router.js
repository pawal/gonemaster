/**
 * Minimal hash router for the public UI.
 *
 * Routes:
 *   #/              → { view: "home",   publicID: null }
 *   #/result/:id    → { view: "result", publicID: id  }
 */

/** Parse a location.hash string into a route object. */
export function parseHash(hash) {
  const path = hash.startsWith("#") ? hash.slice(1) : hash;
  const m = path.match(/^\/result\/([A-Za-z0-9]+)$/);
  if (m) return { view: "result", publicID: m[1] };
  return { view: "home", publicID: null };
}

/** Build the hash string for a given view + optional publicID. */
export function hashFor(view, publicID = null) {
  if (view === "result" && publicID) return `#/result/${publicID}`;
  return "#/";
}

/** Navigate to a view by setting location.hash. */
export function navigate(view, publicID = null) {
  window.location.hash = hashFor(view, publicID).slice(1);
}
