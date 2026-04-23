import { describe, expect, it, vi } from "vitest";
import { load } from "./+page";

function stubResponse(body: unknown, ok = true): Response {
  return {
    ok,
    status: ok ? 200 : 500,
    statusText: ok ? "OK" : "Server Error",
    headers: new Headers({ "content-type": "application/json" }),
    json: async () => body,
    text: async () => JSON.stringify(body)
  } as unknown as Response;
}

// fetchRouter dispatches stub responses based on the request URL so a
// single mock covers the several parallel fetches the overview loader
// triggers. Unmatched URLs fall through to `defaultResponse`, which
// defaults to a 500 so unexpected calls surface as loud test failures.
function fetchRouter(
  routes: Array<{ match: (url: string) => boolean; body: unknown; ok?: boolean }>,
  defaultResponse: { body: unknown; ok: boolean } = { body: { error: "unrouted" }, ok: false }
) {
  const calls: string[] = [];
  const impl = vi.fn(async (input: RequestInfo | URL, _init?: RequestInit) => {
    const url =
      typeof input === "string" ? input : input instanceof URL ? input.toString() : input.url;
    calls.push(url);
    for (const route of routes) {
      if (route.match(url)) {
        return stubResponse(route.body, route.ok ?? true);
      }
    }
    return stubResponse(defaultResponse.body, defaultResponse.ok);
  }) as unknown as typeof fetch;
  return { impl, calls };
}

function evt(overrides: {
  resolvedCohort: string | null;
  fetchImpl: typeof fetch;
  search?: string;
}) {
  return {
    parent: async () => ({
      catalog: null,
      catalogError: null,
      resolvedCohort: overrides.resolvedCohort
    }),
    fetch: overrides.fetchImpl,
    url: new URL(`http://localhost/analysis${overrides.search ?? ""}`)
  } as Parameters<typeof load>[0];
}

describe("+page.load (overview)", () => {
  it("returns datasetTag=null when no cohort has been resolved", async () => {
    const fetchFn = vi.fn();
    const data = await load(
      evt({ resolvedCohort: null, fetchImpl: fetchFn as unknown as typeof fetch })
    );
    expect(data.datasetTag).toBeNull();
    expect(data.detail).toBeNull();
    expect(data.noSnapshot).toBe(false);
    expect(fetchFn).not.toHaveBeenCalled();
  });

  it("renders no_snapshot state when the overview returns status=no_snapshot", async () => {
    const { impl, calls } = fetchRouter([
      {
        match: (url) => url.includes("/overview"),
        body: { dataset_tag: "tld", label: "TLD", materialization_status: "pending", status: "no_snapshot", is_default: true }
      }
    ]);
    const data = await load(evt({ resolvedCohort: "tld", fetchImpl: impl }));
    expect(data.noSnapshot).toBe(true);
    expect(data.detail).toBeNull();
    expect(data.snapshot).toBeNull();
    // No downstream loaders fire once we know the cohort is empty.
    expect(calls.some((u) => u.includes("/cohorts/tld?"))).toBe(false);
    expect(calls.some((u) => u.includes("/tags"))).toBe(false);
  });

  it("passes through the resolved snapshot metadata", async () => {
    const snapshot = {
      slug: "2026-04-20-fixture",
      label: "Fixture",
      captured_at: "2026-04-20T12:00:00Z",
      run_count: 1,
      domain_count: 1
    };
    const { impl, calls } = fetchRouter([
      {
        match: (url) => url.includes("/overview"),
        body: {
          dataset_tag: "tld",
          label: "TLD",
          materialization_status: "ready",
          is_default: true,
          snapshot
        }
      },
      { match: (url) => url.includes("/cohorts/tld"), body: { dataset_tag: "tld", label: "TLD", materialization_status: "ready", domain_count: 1 } },
      { match: (url) => url.includes("/tags"), body: { items: [], total: 0, limit: 10, offset: 0 } },
      { match: (url) => url.includes("/nameservers"), body: { items: [], total: 0, limit: 10, offset: 0 } },
      { match: (url) => url.includes("/asns"), body: { items: [], total: 0, limit: 10, offset: 0 } }
    ]);
    const data = await load(evt({ resolvedCohort: "tld", fetchImpl: impl }));
    expect(data.snapshot?.slug).toBe("2026-04-20-fixture");
    expect(data.noSnapshot).toBe(false);
    expect(data.detailError).toBeNull();
    // The snapshot filter must not creep onto the overview itself in
    // this auto-latest call (no ?snapshot= on the URL).
    const overviewCall = calls.find((u) => u.includes("/overview"));
    expect(overviewCall).toBeDefined();
    expect(overviewCall).not.toContain("snapshot=");
  });

  it("forwards ?snapshot= through to the scoped list loaders", async () => {
    const snapshot = {
      slug: "2026-04-17-old",
      run_count: 1,
      domain_count: 1
    };
    const { impl, calls } = fetchRouter([
      {
        match: (url) => url.includes("/overview"),
        body: {
          dataset_tag: "tld",
          label: "TLD",
          materialization_status: "ready",
          is_default: false,
          snapshot
        }
      },
      { match: (url) => url.includes("/cohorts/tld"), body: { dataset_tag: "tld", label: "TLD", materialization_status: "ready", domain_count: 1 } },
      { match: (url) => url.includes("/tags"), body: { items: [], total: 0, limit: 10, offset: 0 } },
      { match: (url) => url.includes("/nameservers"), body: { items: [], total: 0, limit: 10, offset: 0 } },
      { match: (url) => url.includes("/asns"), body: { items: [], total: 0, limit: 10, offset: 0 } }
    ]);
    const data = await load(
      evt({ resolvedCohort: "tld", fetchImpl: impl, search: "?snapshot=2026-04-17-old" })
    );
    expect(data.snapshot?.slug).toBe("2026-04-17-old");
    const scoped = calls.filter((u) => u.includes("snapshot=2026-04-17-old"));
    // overview + cohort detail + tags + nameservers + asns all carry it.
    expect(scoped.length).toBeGreaterThanOrEqual(5);
  });
});
