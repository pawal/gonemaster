package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestEndpointDetailReadsFromViewTable(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
		f.seedEndpoint("run-a", "a.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")
		f.seedEndpoint("run-b", "b.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")

		// Wipe legacy fact rows; the view-backed handler must still serve.
		for _, runID := range []string{"run-a", "run-b"} {
			if err := f.store.ReplaceAnalysisRunNSEndpoints(f.cohort.ID, runID, nil); err != nil {
				t.Fatalf("clear endpoints %s: %v", runID, err)
			}
			if err := f.store.ReplaceAnalysisRunAddressASNs(f.cohort.ID, runID, nil); err != nil {
				t.Fatalf("clear address asns %s: %v", runID, err)
			}
		}

		resp := getPublic(t, f.srv, f.publicURL("endpoints/192.0.2.1"))
		if resp.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", resp.Code, resp.Body)
		}
		var got PublicAnalysisEndpointDetail
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.Nameserver != "ns1.example" {
			t.Errorf("Nameserver = %q, want ns1.example", got.Nameserver)
		}
		if got.Address != "192.0.2.1" || got.Family != "ipv4" {
			t.Errorf("Address/family = %q/%q, want 192.0.2.1/ipv4", got.Address, got.Family)
		}
		if got.DomainCount != 2 {
			t.Errorf("DomainCount = %d, want 2 (data must come from the view, not live facts)", got.DomainCount)
		}
		if len(got.Domains) != 2 || got.Domains[0] != "a.example" || got.Domains[1] != "b.example" {
			t.Errorf("Domains = %v, want sorted [a.example b.example]", got.Domains)
		}
		if got.ASN == nil || *got.ASN != 64500 {
			t.Errorf("ASN = %v, want 64500", got.ASN)
		}
		if got.Prefix != "192.0.2.0/24" {
			t.Errorf("Prefix = %q, want 192.0.2.0/24", got.Prefix)
		}
	})
}

func TestEndpointDetailAmbiguousWithoutNameserverFilter(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
		// Same address shared across two nameservers (rare-but-real shared hosting).
		f.seedEndpoint("run-a", "a.example", "ns1.shared.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")
		f.seedEndpoint("run-b", "b.example", "ns2.shared.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")

		resp := getPublic(t, f.srv, f.publicURL("endpoints/192.0.2.1"))
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (ambiguous endpoint)", resp.Code)
		}
		if !strings.Contains(resp.Body.String(), "ambiguous_endpoint") {
			t.Errorf("body = %q, want ambiguous_endpoint marker", resp.Body.String())
		}
	})
}

func TestEndpointDetailWithNameserverDisambiguator(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
		f.seedEndpoint("run-a", "a.example", "ns1.shared.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")
		f.seedEndpoint("run-b", "b.example", "ns2.shared.example", "192.0.2.1", "ipv4", now, 64501, "192.0.2.0/24")

		resp := getPublic(t, f.srv, f.publicURL("endpoints/192.0.2.1?nameserver=ns2.shared.example"))
		if resp.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", resp.Code, resp.Body)
		}
		var got PublicAnalysisEndpointDetail
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.Nameserver != "ns2.shared.example" {
			t.Errorf("Nameserver = %q, want ns2.shared.example", got.Nameserver)
		}
		if got.ASN == nil || *got.ASN != 64501 {
			t.Errorf("ASN = %v, want 64501 (the AS attached to ns2)", got.ASN)
		}
	})
}

func TestEndpointDetailNotFound(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
		f.seedEndpoint("run-a", "a.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")

		resp := getPublic(t, f.srv, f.publicURL("endpoints/198.51.100.99"))
		if resp.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", resp.Code)
		}
	})
}

func TestEndpointDetailCaseInsensitiveAddressLookup(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
		f.seedEndpoint("run-a", "a.example", "ns1.example", "2001:DB8::1", "ipv6", now, 64500, "2001:db8::/32")

		resp := getPublic(t, f.srv, f.publicURL("endpoints/2001:db8::1"))
		if resp.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", resp.Code, resp.Body)
		}
	})
}

func TestEndpointDetailCacheHeadersExplicitSnapshot(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
		f.seedEndpoint("run-a", "a.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")

		resp := getPublic(t, f.srv, f.publicURL("endpoints/192.0.2.1"))
		if resp.Code != http.StatusOK {
			t.Fatalf("status = %d", resp.Code)
		}
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

func TestComputeSnapshotEntityViewsPopulatesEndpointDomains(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		cohortID, _ := snapshotViewFixture(t, s)

		views, err := s.ComputeSnapshotEntityViews(cohortID, "batch-x", "")
		if err != nil {
			t.Fatalf("ComputeSnapshotEntityViews: %v", err)
		}
		byKey := map[string]AnalysisSnapshotEndpointView{}
		for _, ep := range views.Endpoints {
			byKey[ep.NameserverName+"|"+ep.Address] = ep
		}

		// ns1.example | 192.0.2.1 is shared by example.test and other.test.
		ns1v4 := byKey["ns1.example|192.0.2.1"]
		if len(ns1v4.Domains) != 2 || ns1v4.Domains[0] != "example.test" || ns1v4.Domains[1] != "other.test" {
			t.Errorf("ns1+v4 domains = %v, want [example.test other.test]", ns1v4.Domains)
		}

		// ns1.example | 2001:db8::1 only serves example.test in this batch.
		ns1v6 := byKey["ns1.example|2001:db8::1"]
		if len(ns1v6.Domains) != 1 || ns1v6.Domains[0] != "example.test" {
			t.Errorf("ns1+v6 domains = %v, want [example.test]", ns1v6.Domains)
		}
	})
}
