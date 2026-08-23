package server

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestPublicAnalysisOverviewIncludesOverviewV2(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
		f.seedDomainSummary("a.example", "run-a", now, 90, "A", "OK")
		f.seedDomainSummary("b.example", "run-b", now.Add(time.Minute), 70, "C", "WARNING")
		f.seedEndpoint("run-a", "a.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")
		f.seedEndpoint("run-b", "b.example", "ns2.example", "192.0.2.2", "ipv4", now.Add(time.Minute), 64501, "192.0.2.0/24")

		resp := getPublic(t, f.srv, f.publicURL("overview"))
		payload := mustJSON[PublicAnalysisOverviewResponse](t, resp, http.StatusOK)
		if payload.Overview == nil {
			t.Fatal("expected Overview payload, got nil")
		}
		if payload.Overview.Totals.DomainCount != 2 {
			t.Errorf("DomainCount = %d, want 2", payload.Overview.Totals.DomainCount)
		}
		if payload.Overview.Totals.NameserverCount != 2 {
			t.Errorf("NameserverCount = %d, want 2", payload.Overview.Totals.NameserverCount)
		}
		if payload.Overview.Totals.EndpointCount != 2 {
			t.Errorf("EndpointCount = %d, want 2", payload.Overview.Totals.EndpointCount)
		}
		if payload.Overview.Totals.ASNCount != 2 {
			t.Errorf("ASNCount = %d, want 2", payload.Overview.Totals.ASNCount)
		}
		if payload.Overview.Totals.PrefixCount != 1 {
			t.Errorf("PrefixCount = %d, want 1", payload.Overview.Totals.PrefixCount)
		}
		grade, ok := payload.Overview.FactDistributions[FactCategoryGrade]
		if !ok {
			t.Fatalf("expected grade distribution, got %+v", payload.Overview.FactDistributions)
		}
		gradeCounts := map[string]int{}
		for _, b := range grade.Buckets {
			gradeCounts[b.Key] = b.Count
		}
		if gradeCounts["A"] != 1 || gradeCounts["C"] != 1 {
			t.Errorf("grade counts = %v", gradeCounts)
		}
	})
}

func TestPublicAnalysisOverviewWithoutSnapshotHasNoOverview(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		resp := getPublic(t, f.srv, f.publicURL("overview"))
		payload := mustJSON[PublicAnalysisOverviewResponse](t, resp, http.StatusOK)
		if payload.Overview != nil {
			t.Fatalf("Overview should be nil when no aggregate row exists, got %+v", payload.Overview)
		}
	})
}

func TestPublicAnalysisCacheHeadersExplicitSnapshot(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
		f.seedDomainSummary("a.example", "run-a", now, 90, "A", "OK")
		f.seedEndpoint("run-a", "a.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")

		resp := getPublic(t, f.srv, f.publicURL("nameservers"))
		wantStatus(t, resp, http.StatusOK)
		// Snapshots can be rebuilt under the same slug, so the response must not be
		// immutable; it revalidates via the ETag, which changes on rebuild.
		cc := resp.Header().Get("Cache-Control")
		if strings.Contains(cc, "immutable") {
			t.Errorf("explicit snapshot Cache-Control = %q, must not be immutable", cc)
		}
		if etag := resp.Header().Get("ETag"); etag == "" {
			t.Error("explicit snapshot must set ETag")
		}
	})
}

func TestComputeSnapshotOverviewProducesTotals(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		cohortID, _ := snapshotViewFixture(t, s)

		got, err := s.ComputeSnapshotOverview(cohortID, "batch-x")
		if err != nil {
			t.Fatalf("ComputeSnapshotOverview: %v", err)
		}
		if got.Totals.DomainCount != 2 {
			t.Errorf("totals.domain_count = %d, want 2", got.Totals.DomainCount)
		}
		if got.Totals.NameserverCount != 2 {
			t.Errorf("totals.nameserver_count = %d, want 2", got.Totals.NameserverCount)
		}
		if got.Totals.EndpointCount != 3 {
			t.Errorf("totals.endpoint_count = %d, want 3", got.Totals.EndpointCount)
		}
		if got.Totals.ASNCount != 2 {
			t.Errorf("totals.asn_count = %d, want 2 (out-of-batch ASN must not surface)", got.Totals.ASNCount)
		}
		if got.Totals.PrefixCount != 2 {
			t.Errorf("totals.prefix_count = %d, want 2", got.Totals.PrefixCount)
		}
	})
}

func TestPublicAnalysisNameserversReadsFromViewTable(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
		f.seedEndpoint("run-a", "a.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")
		f.seedEndpoint("run-a", "a.example", "ns1.example", "2001:db8::1", "ipv6", now, 64500, "2001:db8::/32")

		// Wipe the legacy fact rows for the seeded run so the only data left
		// is in the view tables. The view-backed handler should still serve.
		if err := f.store.ReplaceAnalysisRunNSEndpoints(f.cohort.ID, "run-a", nil); err != nil {
			t.Fatalf("clear ns endpoints: %v", err)
		}
		if err := f.store.ReplaceAnalysisRunAddressASNs(f.cohort.ID, "run-a", nil); err != nil {
			t.Fatalf("clear address asns: %v", err)
		}

		resp := getPublic(t, f.srv, f.publicURL("nameservers"))
		got := mustJSON[PublicAnalysisListResponse[PublicAnalysisNameserverView]](t, resp, http.StatusOK)
		if got.Total != 1 {
			t.Fatalf("total = %d, want 1 (data must come from view table, not facts)", got.Total)
		}
		if got.Items[0].Nameserver != "ns1.example" {
			t.Errorf("Nameserver = %q, want ns1.example", got.Items[0].Nameserver)
		}
		if got.Items[0].EndpointCount != 2 {
			t.Errorf("EndpointCount = %d, want 2", got.Items[0].EndpointCount)
		}
	})
}
