import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  PUBLIC_BASE,
  buildQuery,
  getCatalog,
  getCohortDetail,
  getDomainDetail,
  getOverview,
  getPrefixDetail,
  listASNs,
  listDomains,
  listEndpoints,
  listNameservers,
  listPrefixes,
  listTags
} from "./api";

type FetchCall = { url: string; method: string | undefined };

function recorder(responseBody: unknown = { ok: true }, ok = true) {
  const calls: FetchCall[] = [];
  const stub: typeof fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === "string" ? input : input instanceof URL ? input.toString() : input.url;
    calls.push({ url, method: init?.method });
    return {
      ok,
      status: ok ? 200 : 500,
      statusText: ok ? "OK" : "Error",
      headers: new Headers({ "content-type": "application/json" }),
      json: async () => responseBody,
      text: async () => JSON.stringify(responseBody)
    } as unknown as Response;
  }) as unknown as typeof fetch;
  return { stub, calls };
}

describe("analysis API client", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("buildQuery drops empty values and serializes the rest", () => {
    expect(buildQuery({})).toBe("");
    expect(buildQuery({ dataset_tag: "", search: "", limit: undefined })).toBe("");
    expect(
      buildQuery({
        dataset_tag: "tld",
        limit: 50,
        offset: 0
      })
    ).toContain("dataset_tag=tld");
    expect(buildQuery({ dataset_tag: "tld", search: "example" })).toBe(
      "?dataset_tag=tld&search=example"
    );
  });

  it("every helper only calls URLs under /pub/api/v1/analysis", async () => {
    const { stub, calls } = recorder();

    await getCatalog(stub);
    await getOverview({ dataset_tag: "tld" }, stub);
    await getCohortDetail("tld", stub);
    await listDomains({ limit: 25 }, stub);
    await listNameservers({}, stub);
    await listEndpoints({}, stub);
    await listASNs({}, stub);
    await listPrefixes({}, stub);
    await listTags({}, stub);
    await getDomainDetail("example.com", {}, stub);
    await getPrefixDetail("192.0.2.0/24", {}, stub);

    expect(calls.length).toBeGreaterThan(0);
    for (const { url } of calls) {
      expect(url.startsWith(PUBLIC_BASE), `leaked non-public URL: ${url}`).toBe(true);
      expect(url.startsWith("/api/v1/"), `leaked admin URL: ${url}`).toBe(false);
    }
  });

  it("url-encodes path parameters", async () => {
    const { stub, calls } = recorder();
    await getDomainDetail("alpha.example", {}, stub);
    expect(calls.some((c) => c.url.includes("/domains/alpha.example"))).toBe(true);
  });

  it("prefix detail uses the /prefix query endpoint so CIDR slashes survive", async () => {
    const { stub, calls } = recorder();
    await getPrefixDetail("192.0.2.0/24", {}, stub);
    const url = calls.at(-1)?.url ?? "";
    expect(url.startsWith(`${PUBLIC_BASE}/prefix?`)).toBe(true);
    expect(url).toContain("prefix=192.0.2.0%2F24");
  });

  it("non-2xx responses surface a descriptive error", async () => {
    const { stub } = recorder({ error: { message: "boom" } }, false);
    await expect(getCatalog(stub)).rejects.toThrow(/HTTP 500/);
  });
});
