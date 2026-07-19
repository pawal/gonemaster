import { describe, expect, it } from "vitest";
import type { ASNView, EndpointView, NameserverView } from "$lib/api";
import {
  asnLatencyRows,
  endpointLatencyRows,
  hasAnyLatencyRanking,
  nameserverLatencyRows
} from "./latencyRanking";

describe("latencyRanking", () => {
  it("maps nameserver rows and drops those without a median", () => {
    const items: NameserverView[] = [
      { nameserver: "a.example", domain_count: 1, endpoint_count: 1, ipv4_count: 1, ipv6_count: 0, asn_count: 1, latency_p50_ms: 12, latency_p95_ms: 20, latency_samples: 6 },
      // No latency -> dropped even if the caller passed it in.
      { nameserver: "b.example", domain_count: 1, endpoint_count: 1, ipv4_count: 1, ipv6_count: 0, asn_count: 1 }
    ];
    const rows = nameserverLatencyRows(items);
    expect(rows).toHaveLength(1);
    expect(rows[0]).toMatchObject({
      key: "a.example",
      label: "a.example",
      sublabel: "",
      latencyP50: 12,
      latencyP95: 20,
      samples: 6
    });
  });

  it("maps endpoint rows with the nameserver as sublabel and a composite key", () => {
    const items: EndpointView[] = [
      { nameserver: "ns1.example", address: "192.0.2.1", family: "ipv4", domain_count: 1, latency_p50_ms: 30, latency_samples: 5 }
    ];
    const rows = endpointLatencyRows(items);
    expect(rows[0]).toMatchObject({
      key: "192.0.2.1|ns1.example",
      label: "192.0.2.1",
      sublabel: "ns1.example",
      latencyP50: 30,
      // p95 absent -> null, not undefined, so the template test is simple.
      latencyP95: null
    });
  });

  it("labels ASNs with the AS prefix and keeps the numeric key", () => {
    const items: ASNView[] = [
      { asn: 64500, label: "Example AS", domain_count: 1, address_count: 1, nameserver_count: 1, prefix_count: 1, ipv4_count: 1, ipv6_count: 0, latency_p50_ms: 42, latency_samples: 8 }
    ];
    const rows = asnLatencyRows(items);
    expect(rows[0]).toMatchObject({ key: "64500", label: "AS64500", sublabel: "Example AS", latencyP50: 42 });
  });

  it("hasAnyLatencyRanking is false only when every list is empty", () => {
    expect(hasAnyLatencyRanking([[], []])).toBe(false);
    expect(
      hasAnyLatencyRanking([
        [],
        [{ key: "x", label: "x", sublabel: "", latencyP50: 1, latencyP95: null, samples: 5 }]
      ])
    ).toBe(true);
  });
});
