import { vi } from "vitest";
import type { ReportResponse } from "../lib/api";

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

// A cohort report over two snapshots: one engine tag, one cohort tag, two movers, one cluster.
export function reportFixture(overrides: Partial<ReportResponse> = {}): ReportResponse {
  return {
    dataset_tag: "tld",
    from_slug: "s1",
    to_slug: "s2",
    min_cluster: 3,
    max_spread: 3,
    header: {
      from: { slug: "s1", captured_at: "2026-06-03T00:00:00Z", engine_version: "1.2.0", domain_count: 290 },
      to: { slug: "s2", captured_at: "2026-09-18T00:00:00Z", engine_version: "1.3.0", domain_count: 290 },
      engine: { from_engine_version: "1.2.0", to_engine_version: "1.3.0", crossed_engine_versions: true },
      vocabulary: {
        from_available: true,
        to_available: true,
        from_tag_count: 614,
        to_tag_count: 686,
        added: [{ tag: "Z15_NO_CAA", module: "ZONE", level: "NOTICE" }],
        removed: [],
        level_changed: []
      },
      scoring_config_changed: "false",
      tag_floor: "NOTICE"
    },
    totals: {
      from_domain_count: 290,
      to_domain_count: 290,
      both_domain_count: 290,
      added: 0,
      removed: 0,
      identical_score: 202,
      improved: 14,
      regressed: 18,
      from_mean_score: 91.07,
      to_mean_score: 91.21,
      domain_categories: { real: 10, measurement: 8, mixed: 4, unknown: 0 }
    },
    tags: {
      appeared: [
        {
          tag: "Z15_NO_CAA",
          module: "ZONE",
          to_level: "NOTICE",
          from_domain_count: 0,
          to_domain_count: 120,
          domain_delta: 120,
          classification: "new_in_engine"
        },
        {
          tag: "Z09_NO_RESPONSE_MX_QUERY",
          module: "ZONE",
          to_level: "WARNING",
          from_domain_count: 3,
          to_domain_count: 19,
          domain_delta: 16,
          classification: "cohort_change"
        }
      ],
      cleared: [],
      level_changed: []
    },
    domains: [
      {
        domain: "osteraker.se",
        from_score: 85,
        to_score: 65,
        score_delta: -20,
        from_grade: "B",
        to_grade: "D",
        grade_changed: true,
        category: "real",
        explained_delta: -20,
        unexplained_delta: 0,
        appeared: [
          { tag: "DS08_DNSKEY_RRSIG_EXPIRED", module: "DNSSEC", to_level: "ERROR", classification: "cohort_change" }
        ],
        cleared: [],
        level_changed: []
      },
      {
        domain: "salem.se",
        from_score: 100,
        to_score: 99,
        score_delta: -1,
        from_grade: "A",
        to_grade: "A",
        grade_changed: false,
        category: "measurement",
        explained_delta: -1,
        unexplained_delta: 0,
        appeared: [{ tag: "Z15_NO_CAA", module: "ZONE", to_level: "NOTICE", classification: "new_in_engine" }],
        cleared: [],
        level_changed: []
      }
    ],
    clusters: [
      {
        dimensions: [
          { dimension: "nameserver", value: "ns1.example", total_domains: 41 },
          { dimension: "software_version", value: "PowerDNS 5.0.7", total_domains: 9 }
        ],
        domains: ["a.se", "b.se", "c.se"],
        size: 3,
        min_delta: 8,
        max_delta: 9,
        direction: "improved"
      }
    ],
    domain_total: 2,
    ...overrides
  };
}
