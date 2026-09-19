import { appNavigation, appPaths, appState, fetchRouter, loadEvent, reportFixture, stubResponse } from "../test/helpers";
import { describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/svelte";
import type { NameserverView } from "$lib/api";
import { load, type OverviewPageData } from "./+page";

const h = vi.hoisted(() => ({
  page: { url: new URL("http://localhost/analysis"), data: { snapshots: [] } as unknown }
}));
vi.mock("$app/navigation", () => appNavigation());
vi.mock("$app/paths", () => appPaths());
vi.mock("$app/state", () => appState(h.page));

import OverviewPage from "./+page.svelte";

const evt = (o: {
  resolvedCohort: string | null;
  fetchImpl: typeof fetch;
  effectiveSnapshotSlug?: string | null;
  snapshots?: { slug: string }[];
}) =>
  loadEvent<Parameters<typeof load>[0]>({
    resolvedCohort: o.resolvedCohort,
    effectiveSnapshotSlug: o.effectiveSnapshotSlug,
    snapshots: o.snapshots,
    fetch: o.fetchImpl
  });

const sampleOverview = {
  totals: {
    domain_count: 12,
    nameserver_count: 5,
    endpoint_count: 8,
    asn_count: 3,
    prefix_count: 2
  },
  top_tags: [{ tag: "DS07_NOT_SIGNED", level: "ERROR", domain_count: 4 }],
  top_nameservers: [{ nameserver: "ns1.example", domain_count: 7 }],
  top_asns: [{ asn: 64500, label: "Example AS", domain_count: 5 }],
  fact_distributions: {
    severity: {
      category: "severity",
      label: "Domain health",
      order: 5,
      buckets: [
        { key: "OK", label: "OK", tone: "ok", count: 8, order: 0 },
        { key: "WARNING", label: "Warning", tone: "warning", count: 4, order: 2 }
      ]
    },
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

  it("renders no_snapshot state when the cohort has no captured snapshot to query", async () => {
    const { impl, calls } = fetchRouter([]);
    const data = await load(evt({ resolvedCohort: "tld", fetchImpl: impl }));
    expect(data.noSnapshot).toBe(true);
    expect(data.totals).toBeNull();
    expect(data.snapshot).toBeNull();
    expect(data.loadError).toBeNull();
    // Client-side detection: no request should fire when there is no
    // snapshot slug to anchor against.
    expect(calls).toHaveLength(0);
  });

  it("reads totals + top-N from the bundled /overview payload", async () => {
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
    const data = await load(
      evt({ resolvedCohort: "tld", fetchImpl: impl, effectiveSnapshotSlug: "2026-04-26-fixture" })
    );

    // Overview data still comes from the single bundled call...
    expect(calls.find((c) => c.includes("/overview"))).toContain(
      "/cohorts/tld/snapshots/2026-04-26-fixture/overview"
    );
    expect(data.snapshot?.slug).toBe("2026-04-26-fixture");
    expect(data.totals?.domain_count).toBe(12);
    expect(data.totals?.nameserver_count).toBe(5);
    const severityBuckets = data.factDistributions?.severity.buckets ?? [];
    expect(severityBuckets.find((b) => b.key === "OK")?.count).toBe(8);
    expect(data.topTags[0].tag).toBe("DS07_NOT_SIGNED");
    expect(data.topNameservers[0].nameserver).toBe("ns1.example");
    expect(data.topASNs[0].asn).toBe(64500);
    expect(data.factDistributions?.grade.buckets).toHaveLength(3);
    expect(data.loadError).toBeNull();
  });

  it("enriches the hero with the severity and grade trend series", async () => {
    const { impl, calls } = fetchRouter([
      {
        match: (url) => url.includes("/overview"),
        body: {
          dataset_tag: "tld",
          label: "TLD",
          snapshot: { slug: "s2", run_count: 1, domain_count: 10 },
          overview: sampleOverview
        }
      },
      {
        match: (url) => url.includes("category=severity"),
        body: { dataset_tag: "tld", category: "severity", points: [{ slug: "s1", captured_at: "s1", payload: { ok: 8 } }], key_meta: {} }
      },
      {
        match: (url) => url.includes("category=grade"),
        body: { dataset_tag: "tld", category: "grade", points: [{ slug: "s1", captured_at: "s1", payload: { A: 6 } }] }
      },
      {
        match: (url) => url.includes("category=dnssec_posture"),
        body: { dataset_tag: "tld", category: "dnssec_posture", points: [{ slug: "s1", captured_at: "s1", payload: { signed: 5 } }] }
      }
    ]);
    const data = await load(evt({ resolvedCohort: "tld", fetchImpl: impl, effectiveSnapshotSlug: "s2" }));
    // Trend series are fetched at cohort scope (no snapshot slug in the path).
    expect(calls.some((c) => c.includes("category=severity"))).toBe(true);
    expect(calls.some((c) => c.includes("category=grade"))).toBe(true);
    expect(calls.some((c) => c.includes("category=dnssec_posture"))).toBe(true);
    expect(data.severityTrend.points).toHaveLength(1);
    expect(data.gradeTrend.points).toHaveLength(1);
    expect(data.dnssecTrend.points).toHaveLength(1);
  });

  it("keeps the overview when the trend enrichment fails", async () => {
    // Only /overview is routed; trend/diff calls hit the 500 default and must
    // be swallowed rather than blanking the page.
    const { impl } = fetchRouter([
      {
        match: (url) => url.includes("/overview"),
        body: { dataset_tag: "tld", label: "TLD", snapshot: { slug: "s2", run_count: 1, domain_count: 10 }, overview: sampleOverview }
      }
    ]);
    const data = await load(evt({ resolvedCohort: "tld", fetchImpl: impl, effectiveSnapshotSlug: "s2" }));
    expect(data.loadError).toBeNull();
    expect(data.totals?.domain_count).toBe(12);
    expect(data.severityTrend.points).toEqual([]);
    expect(data.diff).toBeNull();
  });

  it("fetches the diff-vs-previous when an earlier snapshot exists", async () => {
    const { impl, calls } = fetchRouter([
      {
        match: (url) => url.includes("/overview"),
        body: { dataset_tag: "tld", label: "TLD", snapshot: { slug: "s2", run_count: 1, domain_count: 10 }, overview: sampleOverview }
      },
      {
        match: (url) => url.includes("/diff"),
        body: { dataset_tag: "tld", from_slug: "s1", to_slug: "s2", added: [], removed: [], grade_changed: [], level_changed: [] }
      }
    ]);
    const data = await load(
      evt({
        resolvedCohort: "tld",
        fetchImpl: impl,
        effectiveSnapshotSlug: "s2",
        snapshots: [{ slug: "s2" }, { slug: "s1" }]
      })
    );
    // Newest-first list: previous of s2 is s1.
    expect(calls.some((c) => c.includes("/diff") && c.includes("from=s1") && c.includes("to=s2"))).toBe(true);
    expect(data.diffFrom).toBe("s1");
    expect(data.diffTo).toBe("s2");
    expect(data.diff).not.toBeNull();
  });

  it("surfaces a load error when /overview fails", async () => {
    const { impl } = fetchRouter([
      { match: (url) => url.includes("/overview"), body: { error: "boom" }, ok: false }
    ]);
    const data = await load(
      evt({ resolvedCohort: "tld", fetchImpl: impl, effectiveSnapshotSlug: "2026-04-26-fixture" })
    );
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
          // No `overview` field - pre-overview_v2 snapshot.
        }
      }
    ]);
    const data = await load(evt({ resolvedCohort: "tld", fetchImpl: impl, effectiveSnapshotSlug: "x" }));
    expect(data.snapshot?.slug).toBe("x");
    expect(data.totals).toBeNull();
    expect(data.topTags).toEqual([]);
    expect(data.factDistributions).toBeNull();
    expect(data.loadError).toBeNull();
  });
});

function overviewData(overrides: Partial<OverviewPageData> = {}): OverviewPageData {
  return {
    datasetTag: "tld",
    label: "TLD",
    description: "",
    lastMaterializedAt: null,
    totals: { domain_count: 120, nameserver_count: 5, endpoint_count: 8, asn_count: 3, prefix_count: 2 },
    factDistributions: null,
    topTags: [],
    topNameservers: [{ nameserver: "ns1.example", domain_count: 60 }],
    topASNs: [{ asn: 64500, label: "Example AS", domain_count: 48 }],
    latency: {
      nameservers: { fastest: [], slowest: [] },
      endpoints: { fastest: [], slowest: [] },
      asns: { fastest: [], slowest: [] }
    },
    severityTrend: {
      points: [
        { slug: "s1", captured_at: "s1", payload: { ok: 40, notice: 30, critical: 30 } },
        { slug: "s2", captured_at: "s2", payload: { ok: 60, notice: 30, critical: 30 } }
      ],
      keyMeta: {
        ok: { label: "OK", tone: "ok", order: 0 },
        notice: { label: "Notice", tone: "notice", order: 1 },
        critical: { label: "Crit", tone: "critical", order: 5 }
      }
    },
    gradeTrend: {
      points: [{ slug: "s2", captured_at: "s2", payload: { "A+": 10, A: 20, B: 90 } }],
      keyMeta: {}
    },
    dnssecTrend: {
      points: [
        { slug: "s1", captured_at: "s1", payload: { signed: 30, unsigned: 70 } },
        { slug: "s2", captured_at: "s2", payload: { signed: 50, unsigned: 70 } }
      ],
      keyMeta: {}
    },
    diff: {
      dataset_tag: "tld",
      from_slug: "s1",
      to_slug: "s2",
      added: [{ domain: "new.se" }],
      removed: [],
      grade_changed: [{ domain: "reg.se", from_grade: "A", to_grade: "D" }],
      level_changed: []
    },
    report: null,
    diffFrom: "s1",
    diffTo: "s2",
    snapshot: { slug: "s2", captured_at: "2026-04-20T00:00:00Z", run_count: 1, domain_count: 120 },
    noSnapshot: false,
    loadError: null,
    ...overrides
  };
}

// The posture bar segments must reach the domains behind the count.
describe("overview posture drill-down", () => {
  const postureData = () =>
    overviewData({
      factDistributions: {
        dnssec_posture: {
          category: "dnssec_posture",
          label: "DNSSEC posture",
          order: 10,
          buckets: [
            { key: "unsigned", label: "Unsigned", tone: "warning", count: 70, order: 0 },
            { key: "nsec", label: "NSEC", tone: "notice", count: 20, order: 2 },
            { key: "nsec3", label: "NSEC3", tone: "ok", count: 25, order: 3 },
            { key: "mixed", label: "Mixed NSEC/NSEC3", tone: "warning", count: 5, order: 4 }
          ]
        },
        ipv6_coverage: {
          category: "ipv6_coverage",
          label: "IPv6 coverage",
          order: 12,
          buckets: [{ key: "full", label: "Full", tone: "ok", count: 100, order: 0 }]
        }
      }
    });

  it("links each posture segment to the domains list filtered on that bucket", () => {
    h.page.url = new URL("http://localhost/analysis?dataset_tag=tld");
    render(OverviewPage, { data: postureData() });
    const nsec3 = screen.getByRole("link", { name: /NSEC3: 25/ });
    expect(nsec3.getAttribute("href")).toBe("/analysis/domains?dataset_tag=tld&dnssec_posture=nsec3");
    // Mixed is its own bucket, not folded into nsec3.
    const mixed = screen.getByRole("link", { name: /Mixed NSEC\/NSEC3: 5/ });
    expect(mixed.getAttribute("href")).toBe("/analysis/domains?dataset_tag=tld&dnssec_posture=mixed");
  });

  it("leaves categories without a domains filter as plain segments", () => {
    h.page.url = new URL("http://localhost/analysis?dataset_tag=tld");
    render(OverviewPage, { data: postureData() });
    expect(screen.queryByRole("link", { name: /Full: 100/ })).toBeNull();
  });
});

describe("overview page rendering", () => {
  it("leads with hero tiles for domains, health, and signed share", () => {
    h.page.url = new URL("http://localhost/analysis?dataset_tag=tld");
    render(OverviewPage, { data: overviewData() });
    const hero = within(screen.getByLabelText("Cohort headline metrics"));
    // Domains tile uses the latest trend total (120).
    expect(hero.getByText("120")).toBeInTheDocument();
    expect(hero.getByText("Healthy")).toBeInTheDocument();
    // Signed tile from the dnssec_posture trend: signed / (signed+unsigned)
    // at s2 = 50/120 = 41.7%.
    expect(hero.getByText("Signed")).toBeInTheDocument();
    expect(hero.getByText("41.7%")).toBeInTheDocument();
  });

  it("counts OK and NOTICE domains as healthy, not just the OK bucket", () => {
    render(OverviewPage, { data: overviewData() });
    const hero = within(screen.getByLabelText("Cohort headline metrics"));
    // s2 = ok 60 + notice 30 + critical 30. Healthy = (60+30)/120 = 75%,
    // not 60/120 = 50% that OK-only would give.
    expect(hero.getByText("75%")).toBeInTheDocument();
  });

  it("explains each hero tile on hover and for screen readers", () => {
    render(OverviewPage, { data: overviewData() });
    const healthyTile = screen.getByText("Healthy").closest(".hero-tile");
    // Native title drives the hover tooltip.
    expect(healthyTile?.getAttribute("title")).toMatch(/WARNING level or worse/);
    // A visually-hidden copy carries the same help to assistive tech.
    expect(healthyTile?.querySelector(".sr-only")?.textContent).toMatch(/WARNING level or worse/);
    // Every rendered tile carries a help title.
    const tiles = within(screen.getByLabelText("Cohort headline metrics")).getAllByRole("link");
    expect(tiles.every((t) => (t.getAttribute("title") ?? "").length > 0)).toBe(true);
  });

  it("shows single-provider concentration in the infra cards", () => {
    h.page.url = new URL("http://localhost/analysis?dataset_tag=tld");
    render(OverviewPage, { data: overviewData() });
    // Leader nameserver 60/120 = 50%, leader ASN 48/120 = 40%.
    expect(screen.getByText(/Leader hosts 50% of domains/)).toBeInTheDocument();
    expect(screen.getByText(/Leader hosts 40% of domains/)).toBeInTheDocument();
  });

  it("shows a 'since the previous snapshot' movers card with regressed domains", () => {
    h.page.url = new URL("http://localhost/analysis?dataset_tag=tld");
    render(OverviewPage, { data: overviewData() });
    expect(screen.getByText(/since the previous snapshot/i)).toBeInTheDocument();
    const link = screen.getByRole("link", { name: "reg.se" });
    expect(link.getAttribute("href")).toContain("/analysis/domains/reg.se");
    // Full-diff link carries the two slugs.
    const full = screen.getByRole("link", { name: /view full diff/i });
    expect(full.getAttribute("href")).toContain("from=s1");
    expect(full.getAttribute("href")).toContain("to=s2");
  });

  // The movers card labels each row with the report's cause.
  it("labels each mover with its cause when the report loaded", () => {
    h.page.url = new URL("http://localhost/analysis?dataset_tag=tld");
    render(OverviewPage, {
      data: overviewData({
        report: reportFixture({
          domains: [
            {
              domain: "reg.se",
              score_delta: -20,
              grade_changed: true,
              category: "measurement",
              explained_delta: -20,
              unexplained_delta: 0,
              appeared: [],
              cleared: [],
              level_changed: []
            }
          ]
        })
      })
    });
    const chip = screen.getByRole("link", { name: "reg.se" }).closest("li")?.querySelector(".cause");
    expect(chip?.textContent?.trim()).toBe("Measurement");
  });

  it("omits the movers card when there is no diff", () => {
    render(OverviewPage, { data: overviewData({ diff: null }) });
    expect(screen.queryByText(/since the previous snapshot/i)).toBeNull();
  });

  it("offers a diff-vs-previous link on the snapshot pill", () => {
    h.page.url = new URL("http://localhost/analysis?dataset_tag=tld");
    render(OverviewPage, { data: overviewData() });
    const pill = within(screen.getByLabelText("Active snapshot"));
    const link = pill.getByRole("link", { name: /diff vs previous/i });
    expect(link.getAttribute("href")).toContain("from=s1");
    expect(link.getAttribute("href")).toContain("to=s2");
  });

  it("hides the pill diff link when there is no previous snapshot", () => {
    render(OverviewPage, { data: overviewData({ diffFrom: "" }) });
    const pill = within(screen.getByLabelText("Active snapshot"));
    expect(pill.queryByRole("link", { name: /diff vs previous/i })).toBeNull();
  });

  it("links the prefix count to the prefixes list", () => {
    render(OverviewPage, { data: overviewData() });
    // Prefixes has a browsable list route again, so its tile is a real link.
    const prefixCard = screen.getByText("Prefixes").closest(".summary-card");
    expect(prefixCard?.tagName).toBe("A");
    expect(prefixCard?.getAttribute("href")).toContain("/prefixes");
    // Sibling entity cards that also have list routes stay real links.
    const nsCard = screen.getByText("Nameservers").closest(".summary-card");
    expect(nsCard?.tagName).toBe("A");
  });

  it("renders the latency rankings with fastest/slowest entities and links", () => {
    h.page.url = new URL("http://localhost/analysis?dataset_tag=tld");
    render(OverviewPage, {
      data: overviewData({
        latency: {
          nameservers: {
            fastest: [nsView("fast.ns", 8)],
            slowest: [nsView("slow.ns", 400)]
          },
          endpoints: {
            fastest: [{ nameserver: "ns.x", address: "192.0.2.9", family: "ipv4", domain_count: 1, latency_p50_ms: 5, latency_samples: 6 }],
            slowest: []
          },
          asns: { fastest: [], slowest: [] }
        }
      })
    });
    const section = within(screen.getByText("Response times").closest("section") as HTMLElement);
    // Both directions render, each with its heading.
    expect(section.getAllByText("Fastest").length).toBeGreaterThan(0);
    expect(section.getAllByText("Slowest").length).toBeGreaterThan(0);
    // Entity rows link to their detail pages.
    expect(screen.getByText("fast.ns").closest("a")?.getAttribute("href")).toContain(
      "/analysis/nameservers/fast.ns"
    );
    expect(screen.getByText("slow.ns")).toBeInTheDocument();
    const addrLink = screen.getByText("192.0.2.9").closest("a");
    expect(addrLink?.getAttribute("href")).toContain("/analysis/endpoints/192.0.2.9");
    expect(addrLink?.getAttribute("href")).toContain("nameserver=ns.x");
    // The ASNs column is omitted when it has no eligible entities.
    expect(section.queryByText("ASNs")).toBeNull();
  });

  it("hides the latency rankings when no entity has latency", () => {
    render(OverviewPage, { data: overviewData() });
    expect(screen.queryByText("Response times")).toBeNull();
  });
});

function nsView(nameserver: string, p50: number): NameserverView {
  return {
    nameserver,
    domain_count: 1,
    endpoint_count: 1,
    ipv4_count: 1,
    ipv6_count: 0,
    asn_count: 1,
    latency_p50_ms: p50,
    latency_p95_ms: p50 + 10,
    latency_samples: 6
  };
}

describe("overview snapshot provenance", () => {
  it("shows the engine version on the snapshot pill", () => {
    h.page.url = new URL("http://localhost/analysis?dataset_tag=tld");
    const { container } = render(OverviewPage, {
      data: overviewData({
        snapshot: {
          slug: "s2",
          captured_at: "2026-04-20T00:00:00Z",
          run_count: 1,
          domain_count: 120,
          engine_version: "v1.6.6"
        }
      })
    });
    expect(container.querySelector(".snapshot-engine")?.textContent).toContain("v1.6.6");
  });

  // "Unknown" must be stated rather than left blank, so a reader never
  // assumes the snapshot was produced by the current build.
  it("says the engine is unknown when the snapshot carries no version", () => {
    h.page.url = new URL("http://localhost/analysis?dataset_tag=tld");
    const { container } = render(OverviewPage, { data: overviewData() });
    const engine = container.querySelector(".snapshot-engine.unknown");
    expect(engine?.textContent).toContain("unknown");
  });

  it("flags a snapshot whose batch spanned an engine upgrade", () => {
    h.page.url = new URL("http://localhost/analysis?dataset_tag=tld");
    const { container } = render(OverviewPage, {
      data: overviewData({
        snapshot: {
          slug: "s2",
          captured_at: "2026-04-20T00:00:00Z",
          run_count: 1,
          domain_count: 120,
          engine_version: "v1.6.3",
          mixed_engine_version: true
        }
      })
    });
    expect(container.querySelector(".snapshot-engine.mixed")).not.toBeNull();
  });
});
