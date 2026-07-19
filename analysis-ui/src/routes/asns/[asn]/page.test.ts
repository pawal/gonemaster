import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/svelte";
import type { ASNDetail } from "$lib/api";
import type { ASNDetailPageData } from "./+page";

const h = vi.hoisted(() => ({ url: new URL("http://localhost/asns/64500") }));
vi.mock("$app/paths", () => ({ base: "/analysis" }));
vi.mock("$app/state", () => ({
  page: {
    get url() {
      return h.url;
    },
    data: {}
  }
}));

import ASNDetailPage from "./+page.svelte";

const base: ASNDetail = {
  asn: 64500,
  label: "Example AS",
  domain_count: 10,
  address_count: 3,
  nameserver_count: 2,
  prefix_count: 1,
  domains: ["a.example"],
  nameservers: ["ns1.example"],
  prefixes: ["192.0.2.0/24"]
};

function pageData(
  detail: ASNDetail,
  history: ASNDetailPageData["history"] = []
): ASNDetailPageData {
  return { asn: "64500", datasetTag: "tld", detail, history, error: null };
}

describe("asn detail latency", () => {
  it("shows median and p95 latency tiles when present", () => {
    render(ASNDetailPage, {
      data: pageData({ ...base, latency_p50_ms: 20, latency_p95_ms: 55, latency_samples: 4 })
    });
    expect(screen.getByText("Median latency")).toBeInTheDocument();
    expect(screen.getByText("20 ms")).toBeInTheDocument();
    expect(screen.getByText("p95 latency")).toBeInTheDocument();
    expect(screen.getByText("55 ms")).toBeInTheDocument();
  });

  it("omits latency tiles on a snapshot with no latency", () => {
    render(ASNDetailPage, { data: pageData(base) });
    expect(screen.queryByText("Median latency")).toBeNull();
  });

  it("renders a latency trend line when two snapshots carry latency", () => {
    render(ASNDetailPage, {
      data: pageData(base, [
        { slug: "s1", captured_at: "2026-04-17T00:00:00Z", present: true, domain_count: 8, latency_p50_ms: 12 },
        { slug: "s2", captured_at: "2026-04-20T00:00:00Z", present: true, domain_count: 10, latency_p50_ms: 18 }
      ])
    });
    expect(screen.getByText("Median latency over snapshots")).toBeInTheDocument();
    expect(
      screen.getByRole("img", { name: /Median latency over snapshots across 2 snapshots/ })
    ).toBeInTheDocument();
  });

  it("omits the latency trend line when snapshots have no latency", () => {
    render(ASNDetailPage, {
      data: pageData(base, [
        { slug: "s1", captured_at: "2026-04-17T00:00:00Z", present: true, domain_count: 8 },
        { slug: "s2", captured_at: "2026-04-20T00:00:00Z", present: true, domain_count: 10 }
      ])
    });
    expect(screen.getByText("Domains over snapshots")).toBeInTheDocument();
    expect(screen.queryByText("Median latency over snapshots")).toBeNull();
  });
});
