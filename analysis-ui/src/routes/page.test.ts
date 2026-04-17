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

function evt(overrides: { resolvedCohort: string | null; fetchImpl: typeof fetch }) {
  return {
    parent: async () => ({
      catalog: null,
      catalogError: null,
      resolvedCohort: overrides.resolvedCohort
    }),
    fetch: overrides.fetchImpl
  } as Parameters<typeof load>[0];
}

describe("+page.load (overview)", () => {
  it("returns datasetTag=null when no cohort has been resolved", async () => {
    const fetchFn = vi.fn();
    const data = await load(evt({ resolvedCohort: null, fetchImpl: fetchFn as unknown as typeof fetch }));
    expect(data.datasetTag).toBeNull();
    expect(data.detail).toBeNull();
    expect(fetchFn).not.toHaveBeenCalled();
  });

  it("fetches cohort detail for the resolved cohort", async () => {
    const detail = {
      dataset_tag: "tld",
      label: "TLD",
      materialization_status: "ready",
      domain_count: 42,
      nameserver_count: 11,
      endpoint_count: 17,
      asn_count: 4,
      prefix_count: 3
    };
    const fetchFn = vi.fn().mockResolvedValue(stubResponse(detail));
    const data = await load(evt({ resolvedCohort: "tld", fetchImpl: fetchFn as unknown as typeof fetch }));

    expect(data.datasetTag).toBe("tld");
    expect(data.detail?.domain_count).toBe(42);
    expect(data.detailError).toBeNull();
    expect(fetchFn).toHaveBeenCalledWith("/pub/api/v1/analysis/cohorts/tld");
  });

  it("captures detail fetch errors without throwing", async () => {
    const fetchFn = vi.fn().mockResolvedValue(stubResponse({ error: "boom" }, false));
    const data = await load(evt({ resolvedCohort: "tld", fetchImpl: fetchFn as unknown as typeof fetch }));
    expect(data.detail).toBeNull();
    expect(data.detailError).toMatch(/HTTP 500/);
  });
});
