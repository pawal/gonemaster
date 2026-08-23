import { appNavigation, appPaths, appState, loadEvent, stubResponse } from "../../../test/helpers";
import { describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/svelte";
import type { DomainDetail } from "$lib/api";
import { load, type DomainDetailPageData } from "./+page";

const h = vi.hoisted(() => ({ url: new URL("http://localhost/domains/alpha.example") }));
vi.mock("$app/navigation", () => appNavigation());
vi.mock("$app/paths", () => appPaths());
vi.mock("$app/state", () => appState(h));

import DomainDetailPage from "./+page.svelte";

const evt = (o: {
  domain: string;
  resolvedCohort: string | null;
  fetchImpl: ReturnType<typeof vi.fn>;
  effectiveSnapshotSlug?: string | null;
}) =>
  loadEvent<Parameters<typeof load>[0]>({
    resolvedCohort: o.resolvedCohort,
    effectiveSnapshotSlug: o.effectiveSnapshotSlug,
    fetch: o.fetchImpl as unknown as typeof fetch,
    params: { domain: o.domain }
  });

describe("/domains/[domain] +page.load", () => {
  it("returns null detail when no cohort is resolved", async () => {
    const fetchFn = vi.fn();
    const data = await load(evt({ domain: "alpha.example", resolvedCohort: null, fetchImpl: fetchFn }));
    expect(data.detail).toBeNull();
    expect(fetchFn).not.toHaveBeenCalled();
  });

  it("fetches domain detail using the resolved cohort and pinned snapshot", async () => {
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
      evt({
        domain: "alpha.example",
        resolvedCohort: "tld",
        fetchImpl: fetchFn,
        effectiveSnapshotSlug: "2026-04-26"
      })
    );

    expect(data.detail?.domain).toBe("alpha.example");
    expect(data.error).toBeNull();
    const urlCalled = fetchFn.mock.calls[0][0] as string;
    expect(urlCalled).toContain("/cohorts/tld/snapshots/2026-04-26/domains/alpha.example");
  });

  it("URL-encodes tricky domain labels", async () => {
    const fetchFn = vi.fn().mockResolvedValue(stubResponse({}));
    await load(
      evt({
        domain: "xn--bücher-kva.example",
        resolvedCohort: "tld",
        fetchImpl: fetchFn,
        effectiveSnapshotSlug: "2026-04-26"
      })
    );
    const urlCalled = fetchFn.mock.calls[0][0] as string;
    expect(urlCalled).toMatch(/xn--b%C3%BCcher-kva\.example/);
  });

  it("captures fetch errors without throwing", async () => {
    const fetchFn = vi.fn().mockResolvedValue(stubResponse({}, false));
    const data = await load(
      evt({
        domain: "alpha.example",
        resolvedCohort: "tld",
        fetchImpl: fetchFn,
        effectiveSnapshotSlug: "2026-04-26"
      })
    );
    expect(data.detail).toBeNull();
    expect(data.error).toMatch(/HTTP 500/);
  });
});

function nsDetail(over: Partial<DomainDetail> = {}): DomainDetail {
  return {
    domain: "alpha.example",
    score: 85,
    grade: "B",
    worst_level: "WARNING",
    nameserver_count: 1,
    endpoint_count: 2,
    asn_count: 1,
    prefix_count: 1,
    nameservers: [],
    addresses: [],
    ...over
  };
}

function nsPageData(detail: DomainDetail | null): DomainDetailPageData {
  return { domain: "alpha.example", datasetTag: "tld", detail, history: [], error: null };
}

describe("/domains/[domain] response-times table", () => {
  it("shows per-address rows with avg/min/max/samples", () => {
    render(DomainDetailPage, {
      data: nsPageData(
        nsDetail({
          nameserver_timings: [
            { nameserver: "ns1.example", address: "192.0.2.1", avg_ms: 20, min_ms: 18, max_ms: 25, median_ms: 20, stddev_ms: 2, count: 5, status: "ok" },
            { nameserver: "ns1.example", address: "2001:db8::1", avg_ms: 40, min_ms: 38, max_ms: 45, median_ms: 40, stddev_ms: 2, count: 6, status: "ok" }
          ]
        })
      )
    });
    const section = within(
      screen.getByText("Nameserver response times").closest("section") as HTMLElement
    );
    // Address and nameserver cross-link to their entity detail pages.
    expect(section.getByText("192.0.2.1").closest("a")?.getAttribute("href")).toContain(
      "/analysis/endpoints/192.0.2.1"
    );
    expect(section.getByText("2001:db8::1")).toBeInTheDocument();
    expect(section.getAllByText("ns1.example")[0].closest("a")?.getAttribute("href")).toContain(
      "/analysis/nameservers/ns1.example"
    );
    expect(section.getByText("20")).toBeInTheDocument();
    expect(section.getByText("40")).toBeInTheDocument();
  });

  it("marks unreachable with infinity and unresolved with a dash", () => {
    render(DomainDetailPage, {
      data: nsPageData(
        nsDetail({
          nameserver_timings: [
            { nameserver: "down.example", address: "192.0.2.9", avg_ms: 0, min_ms: 0, max_ms: 0, median_ms: 0, stddev_ms: 0, count: 0, status: "unreachable" },
            { nameserver: "gone.example", address: "", avg_ms: 0, min_ms: 0, max_ms: 0, median_ms: 0, stddev_ms: 0, count: 0, status: "unresolved" }
          ]
        })
      )
    });
    const section = within(
      screen.getByText("Nameserver response times").closest("section") as HTMLElement
    );
    // Avg/min/max all read infinity for a reachable-but-silent endpoint.
    expect(section.getAllByText("∞").length).toBeGreaterThan(0);
    expect(section.getByText("No response")).toBeInTheDocument();
    expect(section.getByText("Does not resolve")).toBeInTheDocument();
  });

  it("omits the table when the run carried no timings", () => {
    render(DomainDetailPage, { data: nsPageData(nsDetail()) });
    expect(screen.queryByText("Nameserver response times")).toBeNull();
  });
});
