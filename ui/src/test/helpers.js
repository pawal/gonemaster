import { vi } from "vitest";
import { _resetForTests } from "../lib/theme.svelte.js";

// The admin API client reads ok, status, statusText, headers.get and json/text.
export const jsonResponse = (data, ok = true, status = ok ? 200 : 500) => ({
  ok,
  status,
  statusText: ok ? "OK" : "Bad Request",
  headers: { get: () => "application/json" },
  json: async () => data,
  text: async () => JSON.stringify(data)
});

export const emptyResponse = () => ({
  ok: true,
  status: 200,
  statusText: "No Content",
  headers: { get: () => "" },
  json: async () => ({}),
  text: async () => ""
});

export const noContentResponse = () => ({
  ok: true,
  status: 204,
  statusText: "No Content",
  headers: { get: () => "" },
  json: async () => ({}),
  text: async () => ""
});

// fetch is called with a string, a URL or a Request depending on the caller.
export const requestUrl = (url) =>
  typeof url === "string" ? url : String(url?.url || url?.href || url || "");

const isResponse = (value) => !!value && typeof value.json === "function";

// Installs a global.fetch answering by URL substring in declaration order and
// returns the URLs it was called with. A route value may be a function
// (url, options) => body; returning undefined falls through to the next route.
export const installFetchRoutes = (routes = {}, fallback = {}) => {
  const calls = [];
  const entries = Object.entries(routes);
  global.fetch.mockImplementation((url, options = {}) => {
    const value = requestUrl(url);
    calls.push(value);
    for (const [match, respond] of entries) {
      if (!value.includes(match)) continue;
      const body = typeof respond === "function" ? respond(value, options) : respond;
      if (body === undefined) continue;
      return isResponse(body) ? body : jsonResponse(body);
    }
    const body = typeof fallback === "function" ? fallback(value, options) : fallback;
    return isResponse(body) ? body : jsonResponse(body);
  });
  return calls;
};

export const stubMatchMedia = (matchesDark) => {
  const mock = vi.fn().mockImplementation((q) => ({
    matches: q === "(prefers-color-scheme: dark)" ? matchesDark : false,
    media: q,
    addEventListener: () => {},
    removeEventListener: () => {}
  }));
  vi.stubGlobal("matchMedia", mock);
  window.matchMedia = mock;
  return mock;
};

// Clears every place the theme store keeps state.
export const resetTheme = () => {
  localStorage.clear();
  document.documentElement.removeAttribute("data-theme");
  _resetForTests();
};

export const profileFixture = (overrides = {}) => ({
  id: 1,
  name: "alpha",
  description: "",
  config: {},
  public: false,
  created_at: "2026-04-01T10:00:00Z",
  updated_at: "2026-04-02T10:00:00Z",
  ...overrides
});

export const cohortFixture = (overrides = {}) => ({
  id: 1,
  source_type: "tag",
  source_tag: "tld",
  label: "TLD",
  description: "",
  analysis_enabled: true,
  public_enabled: true,
  is_default: false,
  sort_order: 10,
  materialization_status: "ready",
  last_materialized_at: "2026-04-17T12:00:00Z",
  last_materialization_error: "",
  ...overrides
});

const setting = (value, extra = {}) => ({ value, source: "default", ...extra });

export const serverSettingsFixture = (overrides = {}) => ({
  listen_addr: setting("127.0.0.1:8080", { readonly: true }),
  db_driver: setting("", { readonly: true }),
  db_dsn: setting("", { readonly: true }),
  profile_path: setting("", { readonly: true }),
  worker_count: setting(4),
  max_concurrent_jobs: setting(0),
  min_level: { value: "INFO", source: "config_file" },
  retention_days: setting(0),
  public_url: setting(""),
  rate_limit_enabled: setting(false),
  rate_limit_max: setting(10),
  rate_limit_window: setting("10m0s"),
  show_score_admin: setting(true),
  show_score_public: setting(true),
  show_nameserver_timings_admin: setting(true),
  show_nameserver_timings_public: setting(true),
  ...overrides
});
