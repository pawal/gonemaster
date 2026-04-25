package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestPublicAnalysisOverviewIncludesOverviewV2(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	f.seedDomainSummary("a.example", "run-a", now, 90, "A", "OK")
	f.seedDomainSummary("b.example", "run-b", now.Add(time.Minute), 70, "C", "WARNING")
	f.seedEndpoint("run-a", "a.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")
	f.seedEndpoint("run-b", "b.example", "ns2.example", "192.0.2.2", "ipv4", now.Add(time.Minute), 64501, "192.0.2.0/24")

	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/overview")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	var payload PublicAnalysisOverviewResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
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
	if payload.Overview.GradeDistribution["A"] != 1 || payload.Overview.GradeDistribution["C"] != 1 {
		t.Errorf("GradeDistribution = %v", payload.Overview.GradeDistribution)
	}
}

func TestPublicAnalysisOverviewWithoutSnapshotHasNoOverview(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/overview")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	var payload PublicAnalysisOverviewResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Overview != nil {
		t.Fatalf("Overview should be nil when no aggregate row exists, got %+v", payload.Overview)
	}
}

func TestPublicAnalysisCacheHeadersExplicitSnapshot(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	f.seedDomainSummary("a.example", "run-a", now, 90, "A", "OK")
	f.seedEndpoint("run-a", "a.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")

	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/nameservers?snapshot="+f.snapshot.Slug)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	cc := resp.Header().Get("Cache-Control")
	if !strings.Contains(cc, "immutable") {
		t.Errorf("explicit snapshot Cache-Control = %q, want immutable", cc)
	}
	if etag := resp.Header().Get("ETag"); etag == "" {
		t.Error("explicit snapshot must set ETag")
	}
}

func TestPublicAnalysisCacheHeadersAutoLatest(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	f.seedDomainSummary("a.example", "run-a", now, 90, "A", "OK")
	f.seedEndpoint("run-a", "a.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")

	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/nameservers")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	cc := resp.Header().Get("Cache-Control")
	if !strings.Contains(cc, "no-cache") {
		t.Errorf("auto-latest Cache-Control = %q, want no-cache", cc)
	}
}

func TestComputeSnapshotAggregatesIncludesOverviewV2(t *testing.T) {
	s := testStoreForBackend(t, testBackends(t)[0])
	cohortID, _ := snapshotViewFixture(t, s)

	aggs, err := s.ComputeSnapshotAggregates(cohortID, "batch-x")
	if err != nil {
		t.Fatalf("ComputeSnapshotAggregates: %v", err)
	}
	var overview *AnalysisCohortSnapshotAggregate
	for i := range aggs {
		if aggs[i].Category == SnapshotAggregateOverviewV2 {
			overview = &aggs[i]
			break
		}
	}
	if overview == nil {
		t.Fatalf("overview_v2 row missing from aggregates: %+v", aggs)
	}
	var got SnapshotOverviewV2
	if err := json.Unmarshal([]byte(overview.PayloadJSON), &got); err != nil {
		t.Fatalf("unmarshal overview_v2: %v", err)
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
}

func TestPublicAnalysisNameserversReadsFromViewTable(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
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

	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/nameservers")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	var got PublicAnalysisListResponse[PublicAnalysisNameserverView]
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Total != 1 {
		t.Fatalf("total = %d, want 1 (data must come from view table, not facts)", got.Total)
	}
	if got.Items[0].Nameserver != "ns1.example" {
		t.Errorf("Nameserver = %q, want ns1.example", got.Items[0].Nameserver)
	}
	if got.Items[0].EndpointCount != 2 {
		t.Errorf("EndpointCount = %d, want 2", got.Items[0].EndpointCount)
	}
}
