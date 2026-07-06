// Hash router: the URL hash is the single source of truth for navigation.

export const TABS = [
  "single",
  "recent",
  "domains",
  "tags",
  "cohorts",
  "batches",
  "metrics",
  "settings",
];

export const SETTINGS_SUBS = ["system", "profiles", "scoring"];

const TAB_ALIASES = {
  single: "single", job: "single", jobs: "single", home: "single",
  recent: "recent", tests: "recent",
  domains: "domains", domain: "domains",
  tags: "tags", tag: "tags",
  cohorts: "cohorts", cohort: "cohorts", analysis: "cohorts",
  batches: "batches", batch: "batches",
  metrics: "metrics", metric: "metrics",
  settings: "settings", setting: "settings",
};

// Raw segment to known tab id, or "" if unknown.
export const normalizeTab = (value) => {
  const key = String(value || "").replace(/^\/+/, "").toLowerCase();
  return TAB_ALIASES[key] || "";
};

export const normalizeSettingsSub = (value) => {
  const sub = String(value || "").toLowerCase();
  return SETTINGS_SUBS.includes(sub) ? sub : "system";
};

const decode = (value) => {
  try {
    return decodeURIComponent(value);
  } catch (_) {
    return value;
  }
};

const baseRoute = (tab) => ({
  tab,
  jobId: null,
  domainName: null,
  runId: null,
  tagName: null,
  batchId: null,
  settingsSub: tab === "settings" ? "system" : null,
});

// Parse a location hash into a route object; unknown routes fall back to Single.
export const parseRoute = (hash) => {
  const raw = String(hash || "").replace(/^#\/?/, "");
  const parts = raw.split("/").filter((p) => p !== "");
  const segment = parts[0] || "";

  // Legacy #/settings/analysis bookmark now maps to the Cohorts tab.
  if (normalizeTab(segment) === "settings" && (parts[1] || "").toLowerCase() === "analysis") {
    return baseRoute("cohorts");
  }

  const tab = normalizeTab(segment) || "single";
  const route = baseRoute(tab);

  if (tab === "single" && parts[1]) {
    route.jobId = decode(parts[1]);
  } else if (tab === "domains" && parts[1]) {
    route.domainName = decode(parts[1]);
    if ((parts[2] || "").toLowerCase() === "runs" && parts[3]) {
      route.runId = decode(parts[3]);
    }
  } else if (tab === "tags" && parts[1]) {
    route.tagName = decode(parts[1]);
  } else if (tab === "batches" && parts[1]) {
    route.batchId = decode(parts[1]);
  } else if (tab === "settings") {
    route.settingsSub = normalizeSettingsSub(parts[1]);
  }

  return route;
};

// Format a route object into a "#/..." hash.
export const formatRoute = (route) => {
  if (!route || !route.tab) return "#/single";
  const enc = (v) => encodeURIComponent(v);
  switch (route.tab) {
    case "single":
      return route.jobId ? `#/single/${enc(route.jobId)}` : "#/single";
    case "domains":
      if (!route.domainName) return "#/domains";
      return route.runId
        ? `#/domains/${enc(route.domainName)}/runs/${enc(route.runId)}`
        : `#/domains/${enc(route.domainName)}`;
    case "tags":
      return route.tagName ? `#/tags/${enc(route.tagName)}` : "#/tags";
    case "batches":
      return route.batchId ? `#/batches/${enc(route.batchId)}` : "#/batches";
    case "settings": {
      const sub = normalizeSettingsSub(route.settingsSub);
      return sub === "system" ? "#/settings" : `#/settings/${sub}`;
    }
    default:
      return `#/${route.tab}`;
  }
};

// Hash string for an <a href>, so middle-click and new-tab work.
export const href = (routeOrTab, extra = {}) => {
  if (typeof routeOrTab === "string") {
    return formatRoute({ ...baseRoute(normalizeTab(routeOrTab) || "single"), ...extra });
  }
  return formatRoute(routeOrTab);
};

const initialHash = typeof window === "undefined" ? "" : window.location.hash;
let current = $state(parseRoute(initialHash));

export const router = {
  get route() { return current; },
  get tab() { return current.tab; },
};

// Re-parse the reactive route from window.location (hashchange handler / init).
export const syncFromLocation = () => {
  current = parseRoute(window.location.hash);
  return current;
};

const writeHash = (route, replace) => {
  const hash = formatRoute(route);
  const url = `${window.location.pathname}${window.location.search}${hash}`;
  if (replace || window.location.hash === hash) {
    window.history.replaceState(null, "", url);
  } else {
    window.history.pushState(null, "", url);
  }
  current = route;
};

// Navigate to a route object, or a tab id plus extra detail fields.
export const navigate = (routeOrTab, { replace = false, ...extra } = {}) => {
  const route =
    typeof routeOrTab === "string"
      ? { ...baseRoute(normalizeTab(routeOrTab) || "single"), ...extra }
      : routeOrTab;
  writeHash(route, replace);
  return current;
};

// Rewrite the hash to match a route without adding history depth.
export const canonicalize = (route) => writeHash(route, true);
