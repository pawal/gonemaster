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
    url: new URL(`http://localhost/analysis/trends${overrides.search ?? ""}`)
  } as Parameters<typeof load>[0];
}

describe("+trends.load", () => {
  it("falls back to severity_distribution when ?category= is unknown or missing", async () => {
    const calls: string[] = [];
    const fetchImpl = vi.fn(async (input: RequestInfo | URL) => {
      calls.push(typeof input === "string" ? input : (input as URL).toString());
      return stubResponse({ dataset_tag: "tld", category: "severity_distribution", points: [] });
    }) as unknown as typeof fetch;

    const data = await load(evt({ resolvedCohort: "tld", fetchImpl }));
    expect(data.category).toBe("severity_distribution");
    expect(calls[0]).toContain("category=severity_distribution");
  });

  it("forwards a known ?category= to the trends endpoint", async () => {
    const calls: string[] = [];
    const fetchImpl = vi.fn(async (input: RequestInfo | URL) => {
      calls.push(typeof input === "string" ? input : (input as URL).toString());
      return stubResponse({ dataset_tag: "tld", category: "grade_distribution", points: [] });
    }) as unknown as typeof fetch;

    const data = await load(
      evt({ resolvedCohort: "tld", fetchImpl, search: "?category=grade_distribution" })
    );
    expect(data.category).toBe("grade_distribution");
    expect(calls[0]).toContain("category=grade_distribution");
  });

  it("returns points verbatim so the page can stack them", async () => {
    const points = [
      { slug: "2026-03-01", captured_at: "2026-03-01T00:00:00Z", payload: { A: 5, B: 3 } },
      { slug: "2026-04-01", captured_at: "2026-04-01T00:00:00Z", payload: { A: 6, B: 2, C: 1 } }
    ];
    const fetchImpl = vi.fn().mockResolvedValue(
      stubResponse({ dataset_tag: "tld", category: "grade_distribution", points })
    ) as unknown as typeof fetch;
    const data = await load(
      evt({ resolvedCohort: "tld", fetchImpl, search: "?category=grade_distribution" })
    );
    expect(data.points).toEqual(points);
  });

  it("surfaces errors instead of throwing", async () => {
    const fetchImpl = vi.fn().mockResolvedValue(stubResponse({ error: "boom" }, false)) as unknown as typeof fetch;
    const data = await load(evt({ resolvedCohort: "tld", fetchImpl }));
    expect(data.points).toEqual([]);
    expect(data.error).toMatch(/HTTP 500/);
  });
});
