import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/svelte";
import type { NameserverView } from "$lib/api";
import type { NameserversPageData } from "./+page";

// A mutable URL holder so each render controls the active query.
const h = vi.hoisted(() => ({ url: new URL("http://localhost/nameservers") }));
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

import NameserversPage from "./+page.svelte";

function pageData(rows: NameserverView[]): NameserversPageData {
  return {
    datasetTag: "tld",
    list: { items: rows, total: rows.length, limit: 50, offset: 0 },
    error: null,
    limit: 50,
    offset: 0
  };
}

const base: NameserverView = {
  nameserver: "ns1.example",
  domain_count: 10,
  endpoint_count: 2,
  ipv4_count: 1,
  ipv6_count: 1,
  asn_count: 1
};

describe("nameservers latency column", () => {
  it("shows the latency column and p50 value when a row has latency", () => {
    render(NameserversPage, {
      data: pageData([{ ...base, latency_p50_ms: 15, latency_p95_ms: 40, latency_samples: 3 }])
    });
    expect(screen.getByRole("columnheader", { name: "Latency" })).toBeInTheDocument();
    expect(screen.getByText("15 ms")).toBeInTheDocument();
    expect(screen.getByText("p95 40 ms")).toBeInTheDocument();
  });

  it("hides the latency column entirely on a snapshot with no latency data", () => {
    render(NameserversPage, { data: pageData([base]) });
    expect(screen.queryByRole("columnheader", { name: "Latency" })).toBeNull();
    // The row still renders its other columns.
    expect(screen.getByRole("link", { name: "ns1.example" })).toBeInTheDocument();
  });
});
