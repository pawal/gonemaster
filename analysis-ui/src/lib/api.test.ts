import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  NoSnapshotError,
  PUBLIC_BASE,
  buildQuery,
  getCatalog,
  getCohortDetail,
  getDiff,
  getDomainDetail,
  getOverview,
  getPrefixDetail,
  getSnapshotDetail,
  getTrends,
  getVersion,
  listASNs,
  listDomains,
  listEndpoints,
  listNameservers,
  listPrefixes,
  listSnapshots,
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

    const pin = { dataset_tag: "tld", snapshot: "2026-04-20" };
    await getCatalog(stub);
    await getOverview(pin, stub);
    await getCohortDetail("tld", stub);
    await listDomains({ ...pin, limit: 25 }, stub);
    await listNameservers(pin, stub);
    await listEndpoints(pin, stub);
    await listASNs(pin, stub);
    await listPrefixes(pin, stub);
    await listTags(pin, stub);
    await getDomainDetail("example.com", pin, stub);
    await getPrefixDetail("192.0.2.0/24", pin, stub);
    await listSnapshots("tld", stub);
    await getSnapshotDetail("tld", "2026-04-20", stub);
    await getTrends("tld", {}, stub);
    await getDiff("tld", "2026-03-01", "2026-04-01", stub);
    await getVersion(stub);

    expect(calls.length).toBeGreaterThan(0);
    for (const { url } of calls) {
      expect(url.startsWith(PUBLIC_BASE), `leaked non-public URL: ${url}`).toBe(true);
      expect(url.startsWith("/api/v1/"), `leaked admin URL: ${url}`).toBe(false);
    }
  });

  it("lifts dataset_tag+snapshot into path-segmented URLs when both pinned", async () => {
    const { stub, calls } = recorder();
    await listDomains({ dataset_tag: "tld", snapshot: "2026-04-20" }, stub);
    await getDomainDetail("example.com", { dataset_tag: "tld", snapshot: "2026-04-20" }, stub);
    expect(calls.length).toBe(2);
    for (const { url } of calls) {
      expect(url).toContain("/cohorts/tld/snapshots/2026-04-20/");
      expect(url).not.toContain("snapshot=2026-04-20");
      expect(url).not.toContain("dataset_tag=tld");
    }
  });

  it("throws NoSnapshotError without making a request when a snapshot-scoped path has no slug", async () => {
    const { stub, calls } = recorder();
    await expect(listDomains({ dataset_tag: "tld" }, stub)).rejects.toBeInstanceOf(NoSnapshotError);
    await expect(getOverview({ dataset_tag: "tld" }, stub)).rejects.toBeInstanceOf(NoSnapshotError);
    await expect(getDomainDetail("example.com", { dataset_tag: "tld" }, stub)).rejects.toBeInstanceOf(
      NoSnapshotError
    );
    expect(calls.length).toBe(0);
  });

  it("snapshot endpoints target the cohort-scoped paths", async () => {
    const { stub, calls } = recorder();
    await listSnapshots("tld", stub);
    await getSnapshotDetail("tld", "2026-04-20", stub);
    await getTrends("tld", { category: "grade" }, stub);
    await getDiff("tld", "2026-03-01", "2026-04-01", stub);
    expect(calls[0].url).toContain("/cohorts/tld/snapshots");
    expect(calls[1].url).toContain("/cohorts/tld/snapshots/2026-04-20");
    expect(calls[2].url).toContain("/cohorts/tld/trends");
    expect(calls[2].url).toContain("category=grade");
    expect(calls[3].url).toContain("/cohorts/tld/diff");
    expect(calls[3].url).toContain("from=2026-03-01");
    expect(calls[3].url).toContain("to=2026-04-01");
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
