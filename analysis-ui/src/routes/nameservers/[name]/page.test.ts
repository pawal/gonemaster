import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/svelte";
import type { NameserverDetail } from "$lib/api";
import type { NameserverDetailPageData } from "./+page";

const h = vi.hoisted(() => ({ url: new URL("http://localhost/nameservers/ns1.example") }));
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

import NameserverDetailPage from "./+page.svelte";

const base: NameserverDetail = {
  nameserver: "ns1.example",
  domain_count: 10,
  endpoint_count: 2,
  ipv4_count: 1,
  ipv6_count: 1,
  addresses: ["192.0.2.1"],
  domains: ["a.example"],
  asns: [64500]
};

function pageData(detail: NameserverDetail): NameserverDetailPageData {
  return { nameserver: "ns1.example", datasetTag: "tld", detail, error: null };
}

describe("nameserver detail latency stat", () => {
  it("shows median and p95 latency tiles when present", () => {
    render(NameserverDetailPage, {
      data: pageData({ ...base, latency_p50_ms: 20, latency_p95_ms: 55, latency_samples: 4 })
    });
    expect(screen.getByText("Median latency")).toBeInTheDocument();
    expect(screen.getByText("20 ms")).toBeInTheDocument();
    expect(screen.getByText("p95 latency")).toBeInTheDocument();
    expect(screen.getByText("55 ms")).toBeInTheDocument();
  });

  it("omits latency tiles on a snapshot with no latency", () => {
    render(NameserverDetailPage, { data: pageData(base) });
    expect(screen.queryByText("Median latency")).toBeNull();
  });
});
