package server

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestNameserverDetailReadsFromViewTable(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
		f.seedEndpoint("run-a", "a.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")
		f.seedEndpoint("run-a", "a.example", "ns1.example", "2001:db8::1", "ipv6", now, 64500, "2001:db8::/32")
		f.seedEndpoint("run-b", "b.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")

		// Wipe the legacy fact rows; the view-backed detail handler must
		// still serve from the captured roster.
		for _, runID := range []string{"run-a", "run-b"} {
			if err := f.store.ReplaceAnalysisRunNSEndpoints(f.cohort.ID, runID, nil); err != nil {
				t.Fatalf("clear endpoints %s: %v", runID, err)
			}
			if err := f.store.ReplaceAnalysisRunAddressASNs(f.cohort.ID, runID, nil); err != nil {
				t.Fatalf("clear address asns %s: %v", runID, err)
			}
		}

		resp := getPublic(t, f.srv, f.publicURL("nameservers/ns1.example"))
		got := mustJSON[PublicAnalysisNameserverDetail](t, resp, http.StatusOK)
		if got.Nameserver != "ns1.example" {
			t.Errorf("Nameserver = %q, want ns1.example", got.Nameserver)
		}
		if got.DomainCount != 2 {
			t.Errorf("DomainCount = %d, want 2 (data must come from the view, not live facts)", got.DomainCount)
		}
		if got.EndpointCount != 2 {
			t.Errorf("EndpointCount = %d, want 2 (one v4 + one v6 address)", got.EndpointCount)
		}
		if got.IPv4Count != 1 || got.IPv6Count != 1 {
			t.Errorf("family counts = (v4=%d, v6=%d), want (1, 1)", got.IPv4Count, got.IPv6Count)
		}
		if len(got.Addresses) != 2 || got.Addresses[0] != "192.0.2.1" || got.Addresses[1] != "2001:db8::1" {
			t.Errorf("Addresses = %v, want sorted [192.0.2.1 2001:db8::1]", got.Addresses)
		}
		if len(got.Domains) != 2 || got.Domains[0] != "a.example" || got.Domains[1] != "b.example" {
			t.Errorf("Domains = %v, want sorted [a.example b.example]", got.Domains)
		}
		if len(got.ASNs) != 1 || got.ASNs[0] != 64500 {
			t.Errorf("ASNs = %v, want [64500]", got.ASNs)
		}
	})
}

func TestNameserverDetailCaseInsensitiveLookup(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
		f.seedEndpoint("run-a", "a.example", "NS1.MIXED.Example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")

		resp := getPublic(t, f.srv, f.publicURL("nameservers/ns1.mixed.example"))
		got := mustJSON[PublicAnalysisNameserverDetail](t, resp, http.StatusOK)
		if got.Nameserver != "NS1.MIXED.Example" {
			t.Errorf("Nameserver = %q, want %q (response should preserve the captured casing)", got.Nameserver, "NS1.MIXED.Example")
		}
	})
}

func TestNameserverDetailNotFoundOnUnknownName(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
		f.seedEndpoint("run-a", "a.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")

		resp := getPublic(t, f.srv, f.publicURL("nameservers/does-not-exist.example"))
		wantStatus(t, resp, http.StatusNotFound)
	})
}

func TestNameserverDetailCacheHeadersExplicitSnapshot(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
		f.seedEndpoint("run-a", "a.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")

		resp := getPublic(t, f.srv, f.publicURL("nameservers/ns1.example"))
		wantStatus(t, resp, http.StatusOK)
		// Snapshots can be rebuilt under the same slug, so the response must not be
		// immutable; it revalidates via the ETag, which changes on rebuild.
		if cc := resp.Header().Get("Cache-Control"); strings.Contains(cc, "immutable") {
			t.Errorf("explicit-snapshot Cache-Control = %q, must not be immutable", cc)
		}
		if resp.Header().Get("ETag") == "" {
			t.Error("explicit-snapshot response missing ETag for revalidation")
		}
		if etag := resp.Header().Get("ETag"); etag == "" {
			t.Error("explicit-snapshot must set ETag")
		}
	})
}

func TestComputeSnapshotEntityViewsPopulatesNameserverRosters(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		cohortID, _ := snapshotViewFixture(t, s)

		views, err := s.ComputeSnapshotEntityViews(cohortID, "batch-x", "")
		if err != nil {
			t.Fatalf("ComputeSnapshotEntityViews: %v", err)
		}
		byNS := map[string]AnalysisSnapshotNameserverView{}
		for _, ns := range views.Nameservers {
			byNS[ns.NameserverName] = ns
		}

		ns1 := byNS["ns1.example"]
		if len(ns1.Addresses) != 2 || ns1.Addresses[0] != "192.0.2.1" || ns1.Addresses[1] != "2001:db8::1" {
			t.Errorf("ns1 addresses = %v, want [192.0.2.1 2001:db8::1]", ns1.Addresses)
		}
		if len(ns1.ASNs) != 2 {
			t.Errorf("ns1 asns = %v, want 2 entries", ns1.ASNs)
		}
		if len(ns1.Domains) != 2 || ns1.Domains[0] != "example.test" || ns1.Domains[1] != "other.test" {
			t.Errorf("ns1 domains = %v, want [example.test other.test]", ns1.Domains)
		}

		ns2 := byNS["ns2.example"]
		if len(ns2.Domains) != 1 || ns2.Domains[0] != "other.test" {
			t.Errorf("ns2 domains = %v, want [other.test] (out-of-batch leak.test must not show)", ns2.Domains)
		}
	})
}

func TestNameserverViewRoundTripPreservesRosters(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		cohortID, snapID := snapshotViewFixture(t, s)

		views, err := s.ComputeSnapshotEntityViews(cohortID, "batch-x", "")
		if err != nil {
			t.Fatalf("compute: %v", err)
		}
		if err := s.ReplaceSnapshotEntityViews(snapID, views); err != nil {
			t.Fatalf("replace: %v", err)
		}

		got := s.ListSnapshotNameserverViews(snapID)
		for _, row := range got {
			if row.Addresses == nil && row.EndpointCount > 0 {
				t.Errorf("ns %q: addresses round-trip lost (endpoint_count=%d but addresses nil)", row.NameserverName, row.EndpointCount)
			}
			if row.Domains == nil && row.DomainCount > 0 {
				t.Errorf("ns %q: domains round-trip lost (domain_count=%d but domains nil)", row.NameserverName, row.DomainCount)
			}
		}
	})
}

// TestNameserverDetailPerFamilyLatency proves the detail handler splits a
// dual-stack nameserver's latency into IPv4 and IPv6 from its per-address
// endpoints (v4 endpoint at 10ms, v6 at 50ms).
func TestNameserverDetailPerFamilyLatency(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
		f.seedEndpoint("run-a", "a.example", "ns.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24", 10)
		f.seedEndpoint("run-b", "b.example", "ns.example", "2001:db8::1", "ipv6", now, 64500, "2001:db8::/32", 50)

		got := mustJSON[PublicAnalysisNameserverDetail](t, getPublic(t, f.srv, f.publicURL("nameservers/ns.example")), http.StatusOK)
		if got.LatencyIPv4 == nil || got.LatencyIPv4.LatencyP50MS == nil || *got.LatencyIPv4.LatencyP50MS != 10 {
			t.Fatalf("IPv4 latency = %+v, want p50 10", got.LatencyIPv4)
		}
		if got.LatencyIPv6 == nil || got.LatencyIPv6.LatencyP50MS == nil || *got.LatencyIPv6.LatencyP50MS != 50 {
			t.Fatalf("IPv6 latency = %+v, want p50 50", got.LatencyIPv6)
		}
	})
}

// TestNameserverDetailPerFamilyLatencySingleStack leaves the missing family nil
// so the UI renders no comparison for a v4-only nameserver.
func TestNameserverDetailPerFamilyLatencySingleStack(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
		f.seedEndpoint("run-a", "a.example", "ns4.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24", 10)

		got := mustJSON[PublicAnalysisNameserverDetail](t, getPublic(t, f.srv, f.publicURL("nameservers/ns4.example")), http.StatusOK)
		if got.LatencyIPv4 == nil || got.LatencyIPv4.LatencyP50MS == nil || *got.LatencyIPv4.LatencyP50MS != 10 {
			t.Fatalf("IPv4 latency = %+v, want p50 10", got.LatencyIPv4)
		}
		if got.LatencyIPv6 != nil {
			t.Fatalf("IPv6 latency should be nil for a v4-only nameserver, got %+v", got.LatencyIPv6)
		}
	})
}
