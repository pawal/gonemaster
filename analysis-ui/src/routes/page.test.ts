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

const sampleOverview = {
  totals: {
    domain_count: 12,
    nameserver_count: 5,
    endpoint_count: 8,
    asn_count: 3,
    prefix_count: 2
  },
  severity_distribution: { OK: 8, WARNING: 4 },
  grade_distribution: { A: 6, B: 4, C: 2 },
  signed: { signed: 8, unsigned: 4 },
  dnskey_algo: { "8": 6, "13": 2 },
  top_tags: [{ tag: "DS07_NOT_SIGNED", level: "ERROR", domain_count: 4 }],
  top_nameservers: [{ nameserver: "ns1.example", domain_count: 7 }],
  top_asns: [{ asn: 64500, label: "Example AS", domain_count: 5 }],
  fact_distributions: {
    grade: {
      category: "grade",
      label: "Grade distribution",
      order: 15,
      buckets: [
        { key: "A", label: "A", tone: "ok", count: 6, order: 1 },
        { key: "B", label: "B", tone: "notice", count: 4, order: 2 },
        { key: "C", label: "C", tone: "warning", count: 2, order: 3 }
      ]
    }
  }
};

describe("+page.load (overview)", () => {
  it("returns datasetTag=null when no cohort has been resolved", async () => {
    const fetchFn = vi.fn();
    const data = await load(
      evt({ resolvedCohort: null, fetchImpl: fetchFn as unknown as typeof fetch })
    );
    expect(data.datasetTag).toBeNull();
    expect(data.totals).toBeNull();
    expect(data.noSnapshot).toBe(false);
    expect(fetchFn).not.toHaveBeenCalled();
  });

  it("renders no_snapshot state when the overview returns status=no_snapshot", async () => {
    const { impl, calls } = fetchRouter([
      {
        match: (url) => url.includes("/overview"),
        body: {
          dataset_tag: "tld",
          label: "TLD",
          materialization_status: "pending",
          status: "no_snapshot",
          is_default: true
        }
      }
    ]);
    const data = await load(evt({ resolvedCohort: "tld", fetchImpl: impl }));
    expect(data.noSnapshot).toBe(true);
    expect(data.totals).toBeNull();
    expect(data.snapshot).toBeNull();
    // Only the /overview call fires; the legacy 4-call fan-out is gone.
    expect(calls).toHaveLength(1);
    expect(calls[0]).toContain("/overview");
  });

  it("collapses to a single /overview call and reads totals + top-N from the payload", async () => {
    const snapshot = {
      slug: "2026-04-26-fixture",
      label: "Fixture",
      captured_at: "2026-04-26T12:00:00Z",
      run_count: 12,
      domain_count: 12
    };
    const { impl, calls } = fetchRouter([
      {
        match: (url) => url.includes("/overview"),
        body: {
          dataset_tag: "tld",
          label: "TLD",
          materialization_status: "ready",
          is_default: true,
          snapshot,
          overview: sampleOverview
        }
      }
    ]);
    const data = await load(evt({ resolvedCohort: "tld", fetchImpl: impl }));

    expect(calls).toHaveLength(1);
    expect(calls[0]).toContain("/overview");
    expect(data.snapshot?.slug).toBe("2026-04-26-fixture");
    expect(data.totals?.domain_count).toBe(12);
    expect(data.totals?.nameserver_count).toBe(5);
    expect(data.severityDistribution?.OK).toBe(8);
    expect(data.topTags[0].tag).toBe("DS07_NOT_SIGNED");
    expect(data.topNameservers[0].nameserver).toBe("ns1.example");
    expect(data.topASNs[0].asn).toBe(64500);
    expect(data.factDistributions?.grade.buckets).toHaveLength(3);
    expect(data.loadError).toBeNull();
  });

  it("forwards ?snapshot= to the overview call only", async () => {
    const snapshot = { slug: "2026-04-17-old", run_count: 1, domain_count: 1 };
    const { impl, calls } = fetchRouter([
      {
        match: (url) => url.includes("/overview"),
        body: {
          dataset_tag: "tld",
          label: "TLD",
          materialization_status: "ready",
          is_default: false,
          snapshot,
          overview: sampleOverview
        }
      }
    ]);
    const data = await load(
      evt({ resolvedCohort: "tld", fetchImpl: impl, search: "?snapshot=2026-04-17-old" })
    );
    expect(data.snapshot?.slug).toBe("2026-04-17-old");
    expect(calls).toHaveLength(1);
    expect(calls[0]).toContain("/cohorts/tld/snapshots/2026-04-17-old/overview");
  });

  it("surfaces a load error when /overview fails", async () => {
    const { impl } = fetchRouter([
      { match: (url) => url.includes("/overview"), body: { error: "boom" }, ok: false }
    ]);
    const data = await load(evt({ resolvedCohort: "tld", fetchImpl: impl }));
    expect(data.loadError).toMatch(/HTTP 500/);
    expect(data.totals).toBeNull();
  });

  it("renders without overview payload when the snapshot has no aggregate row yet", async () => {
    const { impl } = fetchRouter([
      {
        match: (url) => url.includes("/overview"),
        body: {
          dataset_tag: "tld",
          label: "TLD",
          materialization_status: "ready",
          is_default: true,
          snapshot: { slug: "x", run_count: 0, domain_count: 0 }
          // No `overview` field — pre-overview_v2 snapshot.
        }
      }
    ]);
    const data = await load(evt({ resolvedCohort: "tld", fetchImpl: impl }));
    expect(data.snapshot?.slug).toBe("x");
    expect(data.totals).toBeNull();
    expect(data.topTags).toEqual([]);
    expect(data.factDistributions).toBeNull();
    expect(data.loadError).toBeNull();
  });
});
