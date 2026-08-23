import { vi } from "vitest";

// JSON Response stub for a stubbed fetch.
export function stubResponse(body: unknown, ok = true): Response {
  return {
    ok,
    status: ok ? 200 : 500,
    statusText: ok ? "OK" : "Server Error",
    headers: new Headers({ "content-type": "application/json" }),
    json: async () => body,
    text: async () => JSON.stringify(body)
  } as unknown as Response;
}

export type FetchRoute = { match: (url: string) => boolean; body: unknown; ok?: boolean };

// A fetch stub that answers by URL and records every requested URL.
export function fetchRouter(
  routes: FetchRoute[],
  defaultResponse: { body: unknown; ok: boolean } = { body: { error: "unrouted" }, ok: false }
) {
  const calls: string[] = [];
  const impl = vi.fn(async (input: RequestInfo | URL, _init?: RequestInit) => {
    const url =
      typeof input === "string" ? input : input instanceof URL ? input.toString() : input.url;
    calls.push(url);
    for (const route of routes) {
      if (route.match(url)) return stubResponse(route.body, route.ok ?? true);
    }
    return stubResponse(defaultResponse.body, defaultResponse.ok);
  }) as unknown as typeof fetch;
  return { impl, calls };
}

export type LoadEventOptions = {
  // Layout data the route's parent() resolves to.
  catalog?: unknown;
  catalogError?: string | null;
  resolvedCohort?: string | null;
  effectiveSnapshotSlug?: string | null;
  snapshots?: { slug: string }[];
  fetch?: typeof fetch;
  url?: string;
  params?: Record<string, string>;
};

// A SvelteKit load event; T is the route's own Parameters<typeof load>[0].
export function loadEvent<T>(options: LoadEventOptions = {}): T {
  const { fetch: fetchImpl, url, params, ...parent } = options;
  return {
    parent: async () => ({
      catalog: null,
      catalogError: null,
      resolvedCohort: null,
      effectiveSnapshotSlug: null,
      snapshots: [],
      ...parent
    }),
    fetch: fetchImpl ?? (vi.fn() as unknown as typeof fetch),
    url: new URL(url ?? "http://localhost/analysis"),
    params: params ?? {}
  } as T;
}

// Mutable holder a test updates between renders; the $app/state mock reads it.
export type PageHolder = { url: URL; data?: unknown };

// Factories for the SvelteKit module mocks. A test calls them from its own
// vi.mock() factory, so this module must be imported before anything that
// pulls in $app/* - keep the helpers import first in the file.
export function appPaths(base = "/analysis") {
  return { base };
}

export function appNavigation() {
  return { goto: vi.fn() };
}

export function appState(holder: PageHolder) {
  return {
    page: {
      get url() {
        return holder.url;
      },
      get data() {
        return holder.data ?? {};
      }
    }
  };
}
