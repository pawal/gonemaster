package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestNameserverDetailReadsFromViewTable(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
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
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	var got PublicAnalysisNameserverDetail
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
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
}

func TestNameserverDetailCaseInsensitiveLookup(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedEndpoint("run-a", "a.example", "NS1.MIXED.Example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")

	resp := getPublic(t, f.srv, f.publicURL("nameservers/ns1.mixed.example"))
	if resp.Code != http.StatusOK {
		t.Fatalf("lower-case lookup: status = %d, body = %s", resp.Code, resp.Body)
	}
	var got PublicAnalysisNameserverDetail
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Nameserver != "NS1.MIXED.Example" {
		t.Errorf("Nameserver = %q, want %q (response should preserve the captured casing)", got.Nameserver, "NS1.MIXED.Example")
	}
}

func TestNameserverDetailNotFoundOnUnknownName(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedEndpoint("run-a", "a.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")

	resp := getPublic(t, f.srv, f.publicURL("nameservers/does-not-exist.example"))
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.Code)
	}
}

func TestNameserverDetailCacheHeadersExplicitSnapshot(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedEndpoint("run-a", "a.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")

	resp := getPublic(t, f.srv, f.publicURL("nameservers/ns1.example"))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d", resp.Code)
	}
	if cc := resp.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("explicit-snapshot Cache-Control = %q, want immutable", cc)
	}
	if etag := resp.Header().Get("ETag"); etag == "" {
		t.Error("explicit-snapshot must set ETag")
	}
}

func TestComputeSnapshotEntityViewsPopulatesNameserverRosters(t *testing.T) {
	s := testStoreForBackend(t, testBackends(t)[0])
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
}

func TestNameserverViewRoundTripPreservesRosters(t *testing.T) {
	s := testStoreForBackend(t, testBackends(t)[0])
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
}
