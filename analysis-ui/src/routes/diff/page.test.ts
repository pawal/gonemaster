import { appNavigation, appPaths, appState, loadEvent, stubResponse } from "../../test/helpers";
import { describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/svelte";
import { load, type DiffPageData } from "./+page";
import { previousSlug } from "$lib/diff";
import type { DiffResponse, ReportResponse, TagDiffResponse } from "$lib/api";

// A mutable URL holder so each render can set the active ?tab= before mounting.
const h = vi.hoisted(() => ({
  url: new URL("http://localhost/analysis/diff?from=s1&to=s2&tab=grade_changed")
}));
vi.mock("$app/navigation", () => appNavigation());
vi.mock("$app/paths", () => appPaths());
vi.mock("$app/state", () => appState(h));

import DiffPage from "./+page.svelte";

const evt = (o: { snapshots?: { slug: string }[]; fetchImpl?: typeof fetch; search?: string }) =>
  loadEvent<Parameters<typeof load>[0]>({
    resolvedCohort: "tld",
    snapshots: o.snapshots,
    fetch: o.fetchImpl,
    url: `http://localhost/analysis/diff${o.search ?? ""}`
  });

const sampleDiff: DiffResponse = {
  dataset_tag: "tld",
  from_slug: "s1",
  to_slug: "s2",
  added: [{ domain: "new.se", to_grade: "A", to_level: "NOTICE" }],
  removed: [{ domain: "gone.se", from_grade: "F" }],
  grade_changed: [
    { domain: "reg.se", from_grade: "B", to_grade: "D", from_level: "WARNING", to_level: "ERROR" }
  ],
  level_changed: [{ domain: "lvl.se", from_level: "NOTICE", to_level: "ERROR", to_grade: "C" }]
};

const sampleTagDiff: TagDiffResponse = {
  dataset_tag: "tld",
  from_slug: "s1",
  to_slug: "s2",
  granularity: "tags",
  appeared: [
    { tag: "NS_FEW", module: "DELEGATION", to_level: "WARNING", from_domain_count: 0, to_domain_count: 8, domain_delta: 8 }
  ],
  cleared: [
    { tag: "DS08_MISSING", module: "DNSSEC", from_level: "WARNING", from_domain_count: 3, to_domain_count: 0, domain_delta: -3 }
  ],
  level_changed: [
    { tag: "SOA_SERIAL", module: "CONSISTENCY", from_level: "NOTICE", to_level: "ERROR", from_domain_count: 4, to_domain_count: 6, domain_delta: 2 }
  ]
};

function pageData(overrides: Partial<DiffPageData> = {}): DiffPageData {
  return {
    datasetTag: "tld",
    fromSlug: "s1",
    toSlug: "s2",
    fromDefaulted: false,
    diff: sampleDiff,
    tagDiff: null,
    report: null,
    error: null,
    ...overrides
  };
}

describe("previousSlug", () => {
  const snaps = [{ slug: "2026-04" }, { slug: "2026-03" }, { slug: "2026-02" }];

  it("returns the next-older snapshot in a newest-first list", () => {
    expect(previousSlug(snaps, "2026-03")).toBe("2026-02");
  });

  it("returns empty for the oldest snapshot", () => {
    expect(previousSlug(snaps, "2026-02")).toBe("");
  });

  it("returns empty when the target is unknown or blank", () => {
    expect(previousSlug(snaps, "nope")).toBe("");
    expect(previousSlug(snaps, "")).toBe("");
  });
});

describe("+diff.load", () => {
  it("defaults From to the snapshot before To when only ?to= is set", async () => {
    const fetchImpl = vi.fn(async () => stubResponse(sampleDiff)) as unknown as typeof fetch;
    const data = await load(
      evt({ snapshots: [{ slug: "s2" }, { slug: "s1" }], fetchImpl, search: "?to=s2" })
    );
    expect(data.fromSlug).toBe("s1");
    expect(data.fromDefaulted).toBe(true);
    // Once a From is derived, both the domain diff and the tag-level diff
    // are fetched (in parallel), so exactly two requests fire.
    expect(fetchImpl).toHaveBeenCalledTimes(2);
  });

  it("does not mark From as defaulted when the URL supplies it", async () => {
    const fetchImpl = vi.fn(async () => stubResponse(sampleDiff)) as unknown as typeof fetch;
    const data = await load(evt({ fetchImpl, search: "?from=s1&to=s2" }));
    expect(data.fromDefaulted).toBe(false);
    expect(data.fromSlug).toBe("s1");
  });

  it("skips the fetch and stays error-free when a side is missing", async () => {
    const fetchImpl = vi.fn() as unknown as typeof fetch;
    const data = await load(evt({ fetchImpl, search: "?to=s2", snapshots: [{ slug: "s2" }] }));
    // s2 is the oldest (only) snapshot, so there is no previous -> no fetch.
    expect(data.fromSlug).toBe("");
    expect(data.diff).toBeNull();
    expect(data.error).toBeNull();
    expect(fetchImpl).not.toHaveBeenCalled();
  });

  it("captures fetch errors without throwing", async () => {
    const fetchImpl = vi.fn(async () => stubResponse({ error: "boom" }, false)) as unknown as typeof fetch;
    const data = await load(evt({ fetchImpl, search: "?from=s1&to=s2" }));
    expect(data.diff).toBeNull();
    expect(data.error).toMatch(/HTTP 500/);
  });
});

describe("diff page rendering", () => {
  it("shows headline regressed/improved/added/removed counts", () => {
    h.url = new URL("http://localhost/analysis/diff?from=s1&to=s2&tab=grade_changed");
    render(DiffPage, { data: pageData() });
    // reg.se (grade) + lvl.se (level) regressed; new.se added; gone.se removed.
    // Scope to the summary region so the "Added" tab label doesn't collide.
    const summary = within(screen.getByLabelText("Diff summary"));
    expect(summary.getByText("Regressed").closest(".stat")?.textContent).toContain("2");
    expect(summary.getByText("Added").closest(".stat")?.textContent).toContain("1");
  });

  it("links each domain to its detail page in the To snapshot", () => {
    h.url = new URL("http://localhost/analysis/diff?from=s1&to=s2&tab=grade_changed");
    render(DiffPage, { data: pageData() });
    const link = screen.getByRole("link", { name: "reg.se" });
    expect(link.getAttribute("href")).toContain("/analysis/domains/reg.se");
    expect(link.getAttribute("href")).toContain("snapshot=s2");
  });

  it("renders the grade-transition matrix when grades changed", () => {
    h.url = new URL("http://localhost/analysis/diff?from=s1&to=s2&tab=grade_changed");
    render(DiffPage, { data: pageData() });
    expect(screen.getByText(/grade transitions/i)).toBeInTheDocument();
  });

  it("shows the tab named by the URL", () => {
    h.url = new URL("http://localhost/analysis/diff?from=s1&to=s2&tab=level_changed");
    render(DiffPage, { data: pageData() });
    // The level_changed list contains lvl.se; the grade_changed reg.se is not
    // in this tab's table.
    expect(screen.getByRole("link", { name: "lvl.se" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "reg.se" })).toBeNull();
  });

  it("notes when From was auto-filled from the previous snapshot", () => {
    h.url = new URL("http://localhost/analysis/diff?to=s2&tab=added");
    render(DiffPage, { data: pageData({ fromDefaulted: true }) });
    expect(screen.getByText(/previous snapshot/i)).toBeInTheDocument();
  });

  it("lists appeared, cleared, and severity-changed tags with signed counts", () => {
    h.url = new URL("http://localhost/analysis/diff?from=s1&to=s2&tab=added");
    render(DiffPage, { data: pageData({ tagDiff: sampleTagDiff }) });
    const section = within(screen.getByLabelText("Tag changes"));
    expect(section.getByText("Appeared")).toBeInTheDocument();
    expect(section.getByText("Cleared")).toBeInTheDocument();
    expect(section.getByText("Severity changed")).toBeInTheDocument();
    // Each tag is a real link, and the domain delta is shown signed.
    expect(section.getByRole("link", { name: "NS_FEW" })).toBeInTheDocument();
    expect(section.getByText("+8")).toBeInTheDocument();
    expect(section.getByRole("link", { name: "DS08_MISSING" })).toBeInTheDocument();
    expect(section.getByText("-3")).toBeInTheDocument();
    expect(section.getByRole("link", { name: "SOA_SERIAL" })).toBeInTheDocument();
  });

  it("shows a friendly message when there are no tag changes", () => {
    h.url = new URL("http://localhost/analysis/diff?from=s1&to=s2&tab=added");
    render(DiffPage, {
      data: pageData({
        tagDiff: { ...sampleTagDiff, appeared: [], cleared: [], level_changed: [] }
      })
    });
    const section = within(screen.getByLabelText("Tag changes"));
    expect(section.getByText(/No finding tags appeared/i)).toBeInTheDocument();
  });

  it("notes when tag-level changes are unavailable", () => {
    h.url = new URL("http://localhost/analysis/diff?from=s1&to=s2&tab=added");
    render(DiffPage, { data: pageData({ tagDiff: null }) });
    const section = within(screen.getByLabelText("Tag changes"));
    expect(section.getByText(/not available/i)).toBeInTheDocument();
  });
});

// The diff page is where an engine upgrade is most likely to be misread as
// a wave of regressions, so the banner is load-bearing, not decoration.
describe("diff engine provenance", () => {
  it("warns when the two snapshots ran different engine versions", () => {
    const { container } = render(DiffPage, {
      data: pageData({
        diff: {
          ...sampleDiff,
          engine: {
            from_engine_version: "v1.6.3",
            to_engine_version: "v1.6.6",
            crossed_engine_versions: true
          }
        }
      })
    });
    const banner = container.querySelector(".engine-banner");
    expect(banner).not.toBeNull();
    expect(banner?.textContent).toContain("v1.6.3");
    expect(banner?.textContent).toContain("v1.6.6");
  });

  it("stays quiet when both snapshots ran the same engine version", () => {
    const { container } = render(DiffPage, {
      data: pageData({
        diff: {
          ...sampleDiff,
          engine: {
            from_engine_version: "v1.6.3",
            to_engine_version: "v1.6.3",
            crossed_engine_versions: false
          }
        }
      })
    });
    expect(container.querySelector(".engine-banner")).toBeNull();
  });

  // Unknown provenance is its own message: we are not claiming the engines
  // matched, we are saying we cannot tell.
  it("says so when provenance is unknown on one side", () => {
    const { container } = render(DiffPage, {
      data: pageData({
        diff: {
          ...sampleDiff,
          engine: { to_engine_version: "v1.6.6", crossed_engine_versions: false, engine_version_unknown: true }
        }
      })
    });
    const banner = container.querySelector(".engine-banner");
    expect(banner?.textContent).toContain("unknown");
  });

  it("renders without a banner when the server sent no provenance at all", () => {
    const { container } = render(DiffPage, { data: pageData() });
    expect(container.querySelector(".engine-banner")).toBeNull();
  });
});

const sampleReport: ReportResponse = {
  dataset_tag: "tld",
  from_slug: "s1",
  to_slug: "s2",
  min_cluster: 3,
  max_spread: 3,
  header: {
    from: { slug: "s1", captured_at: "s1", engine_version: "v1.6.3", profile_name: "default", domain_count: 3 },
    to: { slug: "s2", captured_at: "s2", engine_version: "v1.6.6", profile_name: "default", domain_count: 3 },
    engine: { from_engine_version: "v1.6.3", to_engine_version: "v1.6.6", crossed_engine_versions: true },
    vocabulary: {
      from_available: true,
      to_available: true,
      from_tag_count: 614,
      to_tag_count: 686,
      added: [{ tag: "Z15_NO_CAA", module: "ZONE", level: "NOTICE" }],
      removed: [],
      level_changed: []
    },
    scoring_config_changed: "false",
    tag_floor: "NOTICE"
  },
  totals: {
    from_domain_count: 3,
    to_domain_count: 3,
    both_domain_count: 3,
    added: 0,
    removed: 0,
    identical_score: 1,
    improved: 0,
    regressed: 2,
    from_mean_score: 95,
    to_mean_score: 88,
    domain_categories: { real: 1, measurement: 1 }
  },
  tags: {
    appeared: [
      {
        tag: "Z15_NO_CAA",
        module: "ZONE",
        to_level: "NOTICE",
        from_domain_count: 0,
        to_domain_count: 2,
        domain_delta: 2,
        classification: "new_in_engine"
      },
      {
        tag: "NS_FEW",
        module: "DELEGATION",
        to_level: "WARNING",
        from_domain_count: 0,
        to_domain_count: 8,
        domain_delta: 8,
        classification: "cohort_change"
      }
    ],
    cleared: [],
    level_changed: []
  },
  domains: [
    {
      domain: "reg.se",
      from_score: 90,
      to_score: 70,
      score_delta: -20,
      from_grade: "B",
      to_grade: "D",
      grade_changed: true,
      category: "real",
      explained_delta: -20,
      unexplained_delta: 0,
      appeared: [
        { tag: "NS_FEW", module: "DELEGATION", to_level: "WARNING", classification: "cohort_change" }
      ],
      cleared: [],
      level_changed: []
    },
    {
      domain: "calm.se",
      from_score: 100,
      to_score: 99,
      score_delta: -1,
      from_grade: "A",
      to_grade: "A",
      grade_changed: false,
      category: "measurement",
      explained_delta: -1,
      unexplained_delta: 0,
      appeared: [
        { tag: "Z15_NO_CAA", module: "ZONE", to_level: "NOTICE", classification: "new_in_engine" }
      ],
      cleared: [],
      level_changed: []
    }
  ],
  clusters: [
    {
      dimensions: [{ dimension: "nameserver", value: "ns1.example", total_domains: 41 }],
      domains: ["a.se", "b.se", "c.se"],
      size: 3,
      min_delta: 8,
      max_delta: 9,
      direction: "improved"
    }
  ]
};

// The report turns the diff page from "what moved" into "what moved and
// why", so the provenance, the causes and the clusters are load-bearing.
describe("diff report", () => {
  const renderReport = (overrides: Partial<DiffPageData> = {}, search = "") => {
    h.url = new URL(`http://localhost/analysis/diff?from=s1&to=s2${search}`);
    return render(DiffPage, { data: pageData({ report: sampleReport, ...overrides }) });
  };

  it("states both engine versions and the vocabulary move", () => {
    const { container } = renderReport();
    const banner = container.querySelector(".provenance");
    expect(banner).not.toBeNull();
    expect(banner?.textContent).toContain("v1.6.3");
    expect(banner?.textContent).toContain("v1.6.6");
    expect(banner?.textContent).toContain("614");
    expect(banner?.textContent).toContain("686");
    expect(banner?.textContent).toContain("Scoring configuration unchanged");
  });

  it("warns when the vocabulary is unknown on one side", () => {
    const { container } = renderReport({
      report: {
        ...sampleReport,
        header: {
          ...sampleReport.header,
          vocabulary: { ...sampleReport.header.vocabulary, from_available: false },
          scoring_config_changed: "unknown"
        }
      }
    });
    const banner = container.querySelector(".provenance");
    expect(banner?.textContent).toContain("tag vocabulary is unknown");
    expect(banner?.textContent).toContain("provenance is unknown");
  });

  it("counts the movers by cause and lists the clusters", () => {
    renderReport();
    const card = within(screen.getByLabelText("Report"));
    expect(card.getByText("Real").closest(".stat")?.textContent).toContain("1");
    expect(card.getByText("Measurement").closest(".stat")?.textContent).toContain("1");
    expect(card.getByText("ns1.example")).toBeInTheDocument();
    expect(card.getByText("3 of 41")).toBeInTheDocument();
    expect(card.getByText("+8 to +9")).toBeInTheDocument();
  });

  it("offers a Markdown export of the report", () => {
    renderReport();
    const card = within(screen.getByLabelText("Report"));
    expect(card.getByRole("button", { name: "Export Markdown" })).toBeInTheDocument();
  });

  it("shows movers with their delta, explained delta and cause", () => {
    renderReport();
    const movers = within(screen.getByLabelText("Movers"));
    expect(movers.getByRole("link", { name: "reg.se" })).toBeInTheDocument();
    expect(movers.getByText("90 → 70")).toBeInTheDocument();
    expect(movers.getAllByText("-20").length).toBe(2);
    const causes = screen.getByLabelText("Movers").querySelectorAll("td .cause");
    expect([...causes].map((c) => c.textContent?.trim())).toEqual(["Real", "Measurement"]);
  });

  it("filters the movers by cause from the URL", () => {
    renderReport({}, "&cause=measurement");
    const movers = within(screen.getByLabelText("Movers"));
    expect(movers.getByRole("link", { name: "calm.se" })).toBeInTheDocument();
    expect(movers.queryByRole("link", { name: "reg.se" })).toBeNull();
  });

  it("puts engine-driven tag rows behind a disclosure and leads with cohort changes", () => {
    renderReport();
    const section = within(screen.getByLabelText("Tag changes"));
    expect(section.getByRole("link", { name: "NS_FEW" })).toBeInTheDocument();
    const disclosure = section.getByText("1 explained by the engine's tag vocabulary");
    expect(disclosure.closest("details")).not.toBeNull();
    expect(section.getByRole("link", { name: "Z15_NO_CAA" })).toBeInTheDocument();
  });
});
