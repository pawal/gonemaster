import { describe, expect, it, vi, beforeAll, beforeEach } from "vitest";
import { render, screen, within, fireEvent } from "@testing-library/svelte";
import { load, type PrefixesPageData } from "./+page";
import { goto } from "$app/navigation";
import type { PrefixView } from "$lib/api";

// A mutable URL holder so each render can set the active query before mounting.
const h = vi.hoisted(() => ({ url: new URL("http://localhost/prefixes") }));
vi.mock("$app/navigation", () => ({ goto: vi.fn() }));
vi.mock("$app/paths", () => ({ base: "/analysis" }));
vi.mock("$app/state", () => ({
  page: {
    get url() {
      return h.url;
    },
    data: {}
  }
}));

import PrefixesPage from "./+page.svelte";

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
  resolvedCohort: string | null;
  fetchImpl: ReturnType<typeof vi.fn>;
  search?: string;
  effectiveSnapshotSlug?: string | null;
}) {
  return {
    parent: async () => ({
      catalog: null,
      catalogError: null,
      resolvedCohort: options.resolvedCohort,
      effectiveSnapshotSlug: options.effectiveSnapshotSlug ?? null
    }),
    fetch: options.fetchImpl as unknown as typeof fetch,
    url: new URL(`http://localhost/prefixes${options.search ?? ""}`)
  } as Parameters<typeof load>[0];
}

describe("/prefixes +page.load", () => {
  it("short-circuits when no cohort is resolved", async () => {
    const fetchFn = vi.fn();
    const data = await load(evt({ resolvedCohort: null, fetchImpl: fetchFn }));
    expect(data.datasetTag).toBeNull();
    expect(data.list).toBeNull();
    expect(data.limit).toBe(50);
    expect(fetchFn).not.toHaveBeenCalled();
  });

  it("sends limit/offset/search/sort from the URL to the prefixes endpoint", async () => {
    const fetchFn = vi
      .fn()
      .mockResolvedValue(stubResponse({ items: [], total: 0, limit: 25, offset: 100 }));
    const data = await load(
      evt({
        resolvedCohort: "tld",
        fetchImpl: fetchFn,
        effectiveSnapshotSlug: "2026-04-26",
        search: "?limit=25&offset=100&search=2001&sort=domain_count_desc"
      })
    );

    expect(data.limit).toBe(25);
    expect(data.offset).toBe(100);
    expect(fetchFn).toHaveBeenCalledTimes(1);
    const urlCalled = fetchFn.mock.calls[0][0] as string;
    expect(urlCalled).toMatch(/^\/pub\/api\/v1\/analysis\/cohorts\/tld\/snapshots\/2026-04-26\/prefixes\?/);
    expect(urlCalled).toContain("limit=25");
    expect(urlCalled).toContain("offset=100");
    expect(urlCalled).toContain("search=2001");
    expect(urlCalled).toContain("sort=domain_count_desc");
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

// One prefix carries a single origin ASN; the other is multi-origin (no asn),
// which the list must render as a blank "-" operator.
const rows: PrefixView[] = [
  {
    prefix: "192.0.2.0/24",
    family: "ipv4",
    domain_count: 42,
    address_count: 5,
    asn: 64500,
    asn_label: "EXAMPLE-AS"
  },
  { prefix: "2001:db8::/32", family: "ipv6", domain_count: 7, address_count: 2 }
];

function pageData(overrides: Partial<PrefixesPageData> = {}): PrefixesPageData {
  return {
    datasetTag: "tld",
    list: { items: rows, total: rows.length, limit: 50, offset: 0 },
    error: null,
    limit: 50,
    offset: 0,
    ...overrides
  };
}

describe("prefixes page rendering", () => {
  let capturedDownload = "";

  beforeAll(() => {
    // Boundary stubs: jsdom implements neither the object-URL API nor real
    // downloads, so capture the anchor's filename instead of navigating.
    (URL as unknown as { createObjectURL: unknown }).createObjectURL = vi.fn(() => "blob:mock");
    (URL as unknown as { revokeObjectURL: unknown }).revokeObjectURL = vi.fn();
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (
      this: HTMLAnchorElement
    ) {
      capturedDownload = this.download;
    });
  });

  beforeEach(() => {
    h.url = new URL("http://localhost/prefixes");
    capturedDownload = "";
    vi.mocked(goto).mockClear();
  });

  it("renders one row per prefix with family, counts, and operator", () => {
    render(PrefixesPage, { data: pageData() });
    expect(screen.getByRole("link", { name: "192.0.2.0/24" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "2001:db8::/32" })).toBeInTheDocument();
    expect(screen.getByText("IPv4")).toBeInTheDocument();
    expect(screen.getByText("IPv6")).toBeInTheDocument();
    expect(screen.getByText("42")).toBeInTheDocument();
    expect(screen.getByText("EXAMPLE-AS")).toBeInTheDocument();
  });

  it("shows a dash operator for multi-origin prefixes and a chip for single-origin", () => {
    render(PrefixesPage, { data: pageData() });
    const multiRow = screen.getByRole("link", { name: "2001:db8::/32" }).closest("tr");
    expect(within(multiRow as HTMLElement).getByText("-")).toBeInTheDocument();
    expect(within(multiRow as HTMLElement).queryByText(/^AS\d/)).toBeNull();
    const singleRow = screen.getByRole("link", { name: "192.0.2.0/24" }).closest("tr");
    expect(within(singleRow as HTMLElement).getByText("EXAMPLE-AS")).toBeInTheDocument();
  });

  it("wires the Domains sort header to a sort URL update", async () => {
    render(PrefixesPage, { data: pageData() });
    await fireEvent.click(screen.getByRole("button", { name: /Sort by Domains/i }));
    expect(goto).toHaveBeenCalledWith(
      expect.stringMatching(/sort=domain_count_(asc|desc)/),
      expect.anything()
    );
  });

  it("renders the empty state when the cohort has no prefixes", () => {
    render(PrefixesPage, { data: pageData({ list: { items: [], total: 0, limit: 50, offset: 0 } }) });
    expect(screen.getByText(/No prefixes materialized/i)).toBeInTheDocument();
  });

  it("renders the no-cohort state", () => {
    render(PrefixesPage, { data: pageData({ datasetTag: null, list: null }) });
    expect(screen.getByText(/No cohort resolved/i)).toBeInTheDocument();
  });

  it("surfaces a load error", () => {
    render(PrefixesPage, { data: pageData({ error: "boom", list: null }) });
    expect(screen.getByText(/Failed to load prefixes: boom/i)).toBeInTheDocument();
  });

  it("exports a CSV whose filename carries the cohort tag", async () => {
    render(PrefixesPage, { data: pageData() });
    await fireEvent.click(screen.getByRole("button", { name: "CSV" }));
    expect(capturedDownload).toBe("tld-prefixes.csv");
  });
});
