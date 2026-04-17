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

function evt(options: {
  domain: string;
  resolvedCohort: string | null;
  fetchImpl: ReturnType<typeof vi.fn>;
}) {
  return {
    parent: async () => ({
      catalog: null,
      catalogError: null,
      resolvedCohort: options.resolvedCohort
    }),
    fetch: options.fetchImpl as unknown as typeof fetch,
    params: { domain: options.domain }
  } as Parameters<typeof load>[0];
}

describe("/domains/[domain] +page.load", () => {
  it("returns null detail when no cohort is resolved", async () => {
    const fetchFn = vi.fn();
    const data = await load(evt({ domain: "alpha.example", resolvedCohort: null, fetchImpl: fetchFn }));
    expect(data.detail).toBeNull();
    expect(fetchFn).not.toHaveBeenCalled();
  });

  it("fetches domain detail using the resolved cohort as dataset_tag", async () => {
    const detail = {
      domain: "alpha.example",
      nameserver_count: 2,
      endpoint_count: 3,
      asn_count: 1,
      prefix_count: 2,
      nameservers: [],
      addresses: [],
      tags: []
    };
    const fetchFn = vi.fn().mockResolvedValue(stubResponse(detail));
    const data = await load(
      evt({ domain: "alpha.example", resolvedCohort: "tld", fetchImpl: fetchFn })
    );

    expect(data.detail?.domain).toBe("alpha.example");
    expect(data.error).toBeNull();
    const urlCalled = fetchFn.mock.calls[0][0] as string;
    expect(urlCalled).toMatch(/^\/pub\/api\/v1\/analysis\/domains\/alpha\.example/);
    expect(urlCalled).toContain("dataset_tag=tld");
  });

  it("URL-encodes tricky domain labels", async () => {
    const fetchFn = vi.fn().mockResolvedValue(stubResponse({}));
    await load(
      evt({ domain: "xn--bücher-kva.example", resolvedCohort: "tld", fetchImpl: fetchFn })
    );
    const urlCalled = fetchFn.mock.calls[0][0] as string;
    expect(urlCalled).toMatch(/xn--b%C3%BCcher-kva\.example/);
  });

  it("captures fetch errors without throwing", async () => {
    const fetchFn = vi.fn().mockResolvedValue(stubResponse({}, false));
    const data = await load(
      evt({ domain: "alpha.example", resolvedCohort: "tld", fetchImpl: fetchFn })
    );
    expect(data.detail).toBeNull();
    expect(data.error).toMatch(/HTTP 500/);
  });
});
