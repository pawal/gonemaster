import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/svelte";
import { load, type TrendsPageData } from "./+page";

const h = vi.hoisted(() => ({
  page: { url: new URL("http://localhost/analysis/trends"), data: { snapshots: [] } as unknown }
}));
vi.mock("$app/navigation", () => ({ goto: vi.fn() }));
vi.mock("$app/paths", () => ({ base: "/analysis" }));
vi.mock("$app/state", () => ({
  page: {
    get url() {
      return h.page.url;
    },
    get data() {
      return h.page.data;
    }
  }
}));

import TrendsPage from "./+page.svelte";

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
  it("falls back to severity when ?category= is unknown or missing", async () => {
    const calls: string[] = [];
    const fetchImpl = vi.fn(async (input: RequestInfo | URL) => {
      calls.push(typeof input === "string" ? input : (input as URL).toString());
      return stubResponse({ dataset_tag: "tld", category: "severity", points: [] });
    }) as unknown as typeof fetch;

    const data = await load(evt({ resolvedCohort: "tld", fetchImpl }));
    expect(data.category).toBe("severity");
    expect(calls[0]).toContain("category=severity");
  });

  it("forwards a known ?category= to the trends endpoint", async () => {
    const calls: string[] = [];
    const fetchImpl = vi.fn(async (input: RequestInfo | URL) => {
      calls.push(typeof input === "string" ? input : (input as URL).toString());
      return stubResponse({ dataset_tag: "tld", category: "grade", points: [] });
    }) as unknown as typeof fetch;

    const data = await load(
      evt({ resolvedCohort: "tld", fetchImpl, search: "?category=grade" })
    );
    expect(data.category).toBe("grade");
    expect(calls[0]).toContain("category=grade");
  });

  it("returns points verbatim so the page can stack them", async () => {
    const points = [
      { slug: "2026-03-01", captured_at: "2026-03-01T00:00:00Z", payload: { A: 5, B: 3 } },
      { slug: "2026-04-01", captured_at: "2026-04-01T00:00:00Z", payload: { A: 6, B: 2, C: 1 } }
    ];
    const fetchImpl = vi.fn().mockResolvedValue(
      stubResponse({ dataset_tag: "tld", category: "grade", points })
    ) as unknown as typeof fetch;
    const data = await load(
      evt({ resolvedCohort: "tld", fetchImpl, search: "?category=grade" })
    );
    expect(data.points).toEqual(points);
  });

  it("forwards key_meta from the response so the page can label segments", async () => {
    const keyMeta = {
      "8": { label: "RSASHA256", tone: "notice", order: 8 },
      "13": { label: "ECDSAP256SHA256", tone: "ok", order: 13 }
    };
    const fetchImpl = vi.fn().mockResolvedValue(
      stubResponse({ dataset_tag: "tld", category: "dnskey_algo", points: [], key_meta: keyMeta })
    ) as unknown as typeof fetch;
    const data = await load(
      evt({ resolvedCohort: "tld", fetchImpl, search: "?category=dnskey_algo" })
    );
    expect(data.keyMeta).toEqual(keyMeta);
  });

  it("falls back to an empty key_meta when the response omits it", async () => {
    const fetchImpl = vi.fn().mockResolvedValue(
      stubResponse({ dataset_tag: "tld", category: "severity", points: [] })
    ) as unknown as typeof fetch;
    const data = await load(evt({ resolvedCohort: "tld", fetchImpl }));
    expect(data.keyMeta).toEqual({});
  });

  it("surfaces errors instead of throwing", async () => {
    const fetchImpl = vi.fn().mockResolvedValue(stubResponse({ error: "boom" }, false)) as unknown as typeof fetch;
    const data = await load(evt({ resolvedCohort: "tld", fetchImpl }));
    expect(data.points).toEqual([]);
    expect(data.error).toMatch(/HTTP 500/);
  });
});

function trendsData(overrides: Partial<TrendsPageData> = {}): TrendsPageData {
  return {
    datasetTag: "tld",
    category: "severity",
    points: [
      { slug: "2026-03-01", captured_at: "2026-03-01T00:00:00Z", payload: { ok: 8, critical: 2 } }
    ],
    keyMeta: {
      ok: { label: "OK", tone: "ok", order: 0 },
      critical: { label: "Critical", tone: "critical", order: 5 }
    },
    error: null,
    ...overrides
  };
}

describe("trends page rendering", () => {
  it("links severity segments to the domains behind them in that snapshot", () => {
    h.page.url = new URL("http://localhost/analysis/trends?category=severity");
    render(TrendsPage, { data: trendsData() });
    const links = screen.getAllByRole("link");
    const critical = links.find((a) => a.getAttribute("href")?.includes("worst_level=critical"));
    expect(critical).toBeTruthy();
    expect(critical?.getAttribute("href")).toContain("snapshot=2026-03-01");
  });

  it("keeps a sub-percent bucket visible instead of dropping it", () => {
    const { container } = render(TrendsPage, {
      data: trendsData({
        points: [
          { slug: "2026-03-01", captured_at: "2026-03-01T00:00:00Z", payload: { ok: 9999, critical: 1 } }
        ]
      })
    });
    // 1 in 10000 rounds to 0.0% but must still render as a tiny sliver, and
    // still carry its count for assistive tech.
    const tiny = container.querySelector(".trend-segment.tiny");
    expect(tiny).not.toBeNull();
    expect(tiny?.getAttribute("aria-label")).toContain("1 domains");
  });

  it("labels every segment with its count and share for assistive tech", () => {
    render(TrendsPage, { data: trendsData() });
    // Non-linked-category segments render as role=img; linked ones as links.
    // The severity fixture is linked, so assert the accessible name carries
    // the count and share.
    const critical = screen.getByRole("link", { name: /Critical: 2 domains, 20%/ });
    expect(critical).toBeInTheDocument();
  });

  it("switches to the focus line chart when a bucket is pinned via ?key=", () => {
    h.page.url = new URL("http://localhost/analysis/trends?category=severity&key=critical");
    const { container } = render(TrendsPage, {
      data: trendsData({
        points: [
          { slug: "2026-02-01", captured_at: "2026-02-01T00:00:00Z", payload: { ok: 9, critical: 1 } },
          { slug: "2026-03-01", captured_at: "2026-03-01T00:00:00Z", payload: { ok: 6, critical: 4 } }
        ]
      })
    });
    // The stacked list is replaced by the focus chart.
    expect(container.querySelector(".trend-list")).toBeNull();
    expect(screen.getByRole("img", { name: /Critical across snapshots/ })).toBeInTheDocument();
  });

  it("marks the pinned bucket's legend button as pressed", () => {
    h.page.url = new URL("http://localhost/analysis/trends?category=severity&key=critical");
    render(TrendsPage, { data: trendsData() });
    const active = screen.getByRole("button", { name: /Critical/, pressed: true });
    expect(active).toBeInTheDocument();
  });

  it("stays on the stacked view when ?key= names an unknown bucket", () => {
    h.page.url = new URL("http://localhost/analysis/trends?category=severity&key=bogus");
    const { container } = render(TrendsPage, { data: trendsData() });
    expect(container.querySelector(".trend-list")).not.toBeNull();
  });
});
