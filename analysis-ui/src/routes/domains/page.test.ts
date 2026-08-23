import { loadEvent, stubResponse } from "../../test/helpers";
import { describe, expect, it, vi } from "vitest";
import { load } from "./+page";

const evt = (o: {
  resolvedCohort: string | null;
  fetchImpl: ReturnType<typeof vi.fn>;
  search?: string;
  effectiveSnapshotSlug?: string | null;
}) =>
  loadEvent<Parameters<typeof load>[0]>({
    resolvedCohort: o.resolvedCohort,
    effectiveSnapshotSlug: o.effectiveSnapshotSlug,
    fetch: o.fetchImpl as unknown as typeof fetch,
    url: `http://localhost/domains${o.search ?? ""}`
  });

describe("/domains +page.load", () => {
  it("short-circuits when no cohort is resolved", async () => {
    const fetchFn = vi.fn();
    const data = await load(evt({ resolvedCohort: null, fetchImpl: fetchFn }));
    expect(data.datasetTag).toBeNull();
    expect(data.list).toBeNull();
    expect(data.limit).toBe(50);
    expect(fetchFn).not.toHaveBeenCalled();
  });

  it("sends limit/offset/search/sort from the URL to the API", async () => {
    const fetchFn = vi
      .fn()
      .mockResolvedValue(stubResponse({ items: [], total: 0, limit: 25, offset: 100 }));
    const data = await load(
      evt({
        resolvedCohort: "tld",
        fetchImpl: fetchFn,
        effectiveSnapshotSlug: "2026-04-26",
        search: "?limit=25&offset=100&search=alpha&sort=score_desc"
      })
    );

    expect(data.limit).toBe(25);
    expect(data.offset).toBe(100);
    expect(fetchFn).toHaveBeenCalledTimes(1);
    const urlCalled = fetchFn.mock.calls[0][0] as string;
    expect(urlCalled).toMatch(/^\/pub\/api\/v1\/analysis\/cohorts\/tld\/snapshots\/2026-04-26\/domains\?/);
    expect(urlCalled).toContain("limit=25");
    expect(urlCalled).toContain("offset=100");
    expect(urlCalled).toContain("search=alpha");
    expect(urlCalled).toContain("sort=score_desc");
  });

  it("clamps invalid limit/offset back to defaults", async () => {
    const fetchFn = vi
      .fn()
      .mockResolvedValue(stubResponse({ items: [], total: 0, limit: 50, offset: 0 }));
    const data = await load(
      evt({
        resolvedCohort: "tld",
        fetchImpl: fetchFn,
        effectiveSnapshotSlug: "2026-04-26",
        search: "?limit=abc&offset=-5"
      })
    );
    expect(data.limit).toBe(50);
    expect(data.offset).toBe(0);
  });

  it("captures fetch errors without throwing", async () => {
    const fetchFn = vi.fn().mockResolvedValue(stubResponse({}, false));
    const data = await load(
      evt({ resolvedCohort: "tld", fetchImpl: fetchFn, effectiveSnapshotSlug: "2026-04-26" })
    );
    expect(data.list).toBeNull();
    expect(data.error).toMatch(/HTTP 500/);
  });
});
