import { describe, expect, it, vi } from "vitest";
import { load } from "./+layout";
import type { CatalogResponse } from "$lib/api";

type FetchMock = ReturnType<typeof vi.fn>;

function stubResponse(body: unknown, ok = true) {
  return {
    ok,
    status: ok ? 200 : 500,
    statusText: ok ? "OK" : "Server Error",
    headers: new Headers({ "content-type": "application/json" }),
    json: async () => body,
    text: async () => JSON.stringify(body)
  } as unknown as Response;
}

function event(fetch: FetchMock, search = "") {
  return {
    fetch: fetch as unknown as typeof globalThis.fetch,
    url: new URL(`http://localhost${search}`)
  } as Parameters<typeof load>[0];
}

describe("+layout.load", () => {
  it("resolves catalog and picks the default_tag when dataset_tag is not requested", async () => {
    const catalog: CatalogResponse = {
      default_tag: "tld",
      cohorts: [
        { dataset_tag: "tld", label: "TLD", is_default: true },
        { dataset_tag: "gov", label: "Government", is_default: false }
      ],
      selector_enabled: true
    };
    const fetch = vi.fn().mockResolvedValue(stubResponse(catalog));

    const data = await load(event(fetch));
    expect(data.catalog).toEqual(catalog);
    expect(data.catalogError).toBeNull();
    expect(data.resolvedCohort).toBe("tld");
    expect(fetch).toHaveBeenCalledWith("/pub/api/v1/analysis/catalog");
  });

  it("prefers the requested dataset_tag from the URL over the default", async () => {
    const catalog: CatalogResponse = {
      default_tag: "tld",
      cohorts: [
        { dataset_tag: "tld", label: "TLD", is_default: true },
        { dataset_tag: "gov", label: "Government", is_default: false }
      ],
      selector_enabled: true
    };
    const fetch = vi.fn().mockResolvedValue(stubResponse(catalog));

    const data = await load(event(fetch, "?dataset_tag=gov"));
    expect(data.resolvedCohort).toBe("gov");
  });

  it("falls back to the first cohort when no default is set", async () => {
    const catalog: CatalogResponse = {
      cohorts: [
        { dataset_tag: "gov", label: "Government", is_default: false }
      ],
      selector_enabled: false
    };
    const fetch = vi.fn().mockResolvedValue(stubResponse(catalog));

    const data = await load(event(fetch));
    expect(data.resolvedCohort).toBe("gov");
  });

  it("returns null resolvedCohort and no error when the catalog is empty", async () => {
    const fetch = vi
      .fn()
      .mockResolvedValue(stubResponse({ cohorts: [], selector_enabled: false }));
    const data = await load(event(fetch));
    expect(data.resolvedCohort).toBeNull();
    expect(data.catalog?.cohorts).toHaveLength(0);
    expect(data.catalogError).toBeNull();
  });

  it("captures fetch errors without throwing", async () => {
    const fetch = vi.fn().mockResolvedValue(stubResponse({ error: "boom" }, false));
    const data = await load(event(fetch));
    expect(data.catalog).toBeNull();
    expect(data.resolvedCohort).toBeNull();
    expect(data.catalogError).toMatch(/HTTP 500/);
  });
});
