package server

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// TestPercentileMS pins the percentile math the latency aggregation relies
// on. It uses the "linear interpolation between closest ranks" definition:
//   - the 0th percentile is the min, the 100th is the max,
//   - a percentile landing exactly on a sample returns that sample,
//   - a percentile between two samples interpolates linearly,
//   - a single sample is returned for any percentile,
//   - an empty input reports ok=false so callers can leave latency nil.
func TestPercentileMS(t *testing.T) {
	// Samples deliberately out of order to prove the helper sorts a copy.
	samples := []float64{40, 10, 30, 20}

	cases := []struct {
		p    float64
		want float64
	}{
		{0, 10},   // min
		{100, 40}, // max
		{50, 25},  // between 20 and 30 -> midpoint
		{95, 38.5},
	}
	for _, c := range cases {
		got, ok := percentileMS(samples, c.p)
		if !ok {
			t.Fatalf("p%v: ok=false, want true", c.p)
		}
		if got != c.want {
			t.Errorf("p%v = %v, want %v", c.p, got, c.want)
		}
	}

	// The input slice must not be mutated by the sort.
	if samples[0] != 40 {
		t.Errorf("percentileMS mutated the caller's slice: %v", samples)
	}

	if _, ok := percentileMS(nil, 50); ok {
		t.Errorf("empty samples should report ok=false")
	}

	// A single sample answers every percentile with itself.
	if got, ok := percentileMS([]float64{7}, 95); !ok || got != 7 {
		t.Errorf("single sample p95 = (%v,%v), want (7,true)", got, ok)
	}
}

// TestAggregateLatency confirms the wrapper fills p50/p95 pointers and the
// sample count, and leaves the pointers nil (no data) for empty input so the
// view - and ultimately the UI - degrades gracefully.
func TestAggregateLatency(t *testing.T) {
	empty := aggregateLatency(nil)
	if empty.P50 != nil || empty.P95 != nil || empty.Samples != 0 {
		t.Errorf("empty aggregate = %+v, want all-zero/nil", empty)
	}

	got := aggregateLatency([]float64{10, 20, 30, 40})
	if got.Samples != 4 {
		t.Errorf("Samples = %d, want 4", got.Samples)
	}
	if got.P50 == nil || *got.P50 != 25 {
		t.Errorf("P50 = %v, want 25", got.P50)
	}
	if got.P95 == nil || *got.P95 != 38.5 {
		t.Errorf("P95 = %v, want 38.5", got.P95)
	}
}

// TestComputeSnapshotEntityViewsAggregatesLatency proves the capture-time
// builder rolls per-endpoint avg_ms up into the nameserver, endpoint, and ASN
// views. Endpoints with no measurement (avg_ms 0) contribute no samples, so
// entities without timing stay latency-free rather than reading a false 0ms.
func TestComputeSnapshotEntityViewsAggregatesLatency(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			cohortID, _ := snapshotViewFixture(t, s)

			// Overwrite run-a's endpoints with real latencies; run-b keeps its
			// zero-latency rows so ns2 (only in run-b) has no samples.
			now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
			addr4a, _ := s.UpsertAnalysisAddress("192.0.2.1", "ipv4", now)
			addr6, _ := s.UpsertAnalysisAddress("2001:db8::1", "ipv6", now)
			ns1, _ := s.UpsertAnalysisNameserver("ns1.example", now)
			domA, _ := s.GetOrCreateDomain("example.test")
			if err := s.ReplaceAnalysisRunNSEndpoints(cohortID, "run-a", []AnalysisRunNameserverEndpoint{
				{CohortID: cohortID, RunID: "run-a", DomainID: domA.ID, NameserverID: ns1.ID, AddressID: addr4a.ID, Role: "auth", Family: "ipv4", QueryCount: 3, AvgMS: 10},
				{CohortID: cohortID, RunID: "run-a", DomainID: domA.ID, NameserverID: ns1.ID, AddressID: addr6.ID, Role: "auth", Family: "ipv6", QueryCount: 2, AvgMS: 30},
			}); err != nil {
				t.Fatalf("reseed run-a endpoints: %v", err)
			}

			views, err := s.ComputeSnapshotEntityViews(cohortID, "batch-x", "")
			if err != nil {
				t.Fatalf("ComputeSnapshotEntityViews: %v", err)
			}

			// ns1 saw avg_ms {10, 30} (plus run-b's zero, excluded): p50 20, p95 29.
			byNS := map[string]AnalysisSnapshotNameserverView{}
			for _, ns := range views.Nameservers {
				byNS[ns.NameserverName] = ns
			}
			ns1v := byNS["ns1.example"]
			if ns1v.LatencySamples != 2 {
				t.Fatalf("ns1 latency samples = %d, want 2", ns1v.LatencySamples)
			}
			if ns1v.LatencyP50MS == nil || *ns1v.LatencyP50MS != 20 {
				t.Errorf("ns1 p50 = %v, want 20", ns1v.LatencyP50MS)
			}
			if ns1v.LatencyP95MS == nil || *ns1v.LatencyP95MS != 29 {
				t.Errorf("ns1 p95 = %v, want 29", ns1v.LatencyP95MS)
			}

			// ns2 (run-b only, avg_ms 0) must have no latency: graceful nil.
			ns2v := byNS["ns2.example"]
			if ns2v.LatencySamples != 0 || ns2v.LatencyP50MS != nil {
				t.Errorf("ns2 latency = (%d, %v), want no data", ns2v.LatencySamples, ns2v.LatencyP50MS)
			}

			// The v4 endpoint of ns1 saw only avg_ms 10.
			var epV4 *AnalysisSnapshotEndpointView
			for i := range views.Endpoints {
				e := &views.Endpoints[i]
				if e.NameserverName == "ns1.example" && e.Address == "192.0.2.1" {
					epV4 = e
				}
			}
			if epV4 == nil {
				t.Fatal("missing ns1/192.0.2.1 endpoint view")
			}
			if epV4.LatencyP50MS == nil || *epV4.LatencyP50MS != 10 {
				t.Errorf("endpoint p50 = %v, want 10", epV4.LatencyP50MS)
			}

			// AS 64496 is only reached via ns1's two addresses -> {10, 30}.
			byASN := map[int64]AnalysisSnapshotASNView{}
			for _, a := range views.ASNs {
				byASN[a.ASN] = a
			}
			asn := byASN[64496]
			if asn.LatencyP50MS == nil || *asn.LatencyP50MS != 20 {
				t.Errorf("asn 64496 p50 = %v, want 20", asn.LatencyP50MS)
			}
		})
	}
}

// TestPublicNameserverListSurfacesLatency proves the aggregated latency is
// carried all the way out to the public nameserver list JSON.
func TestPublicNameserverListSurfacesLatency(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	f.seedEndpoint("run-lat", "a.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")

	// Re-emit the run's endpoint with a real response time, then rebuild views.
	dom, _ := f.store.GetOrCreateDomain("a.example")
	ns, _ := f.store.UpsertAnalysisNameserver("ns1.example", now)
	addr, _ := f.store.UpsertAnalysisAddress("192.0.2.1", "ipv4", now)
	if err := f.store.ReplaceAnalysisRunNSEndpoints(f.cohort.ID, "run-lat", []AnalysisRunNameserverEndpoint{
		{CohortID: f.cohort.ID, RunID: "run-lat", DomainID: dom.ID, NameserverID: ns.ID, AddressID: addr.ID, Role: "authoritative", Family: "ipv4", QueryCount: 1, AvgMS: 15},
	}); err != nil {
		t.Fatalf("reseed endpoint with latency: %v", err)
	}
	f.refreshSnapshotViews(f.batchID)

	resp := getPublic(t, f.srv, f.publicURL("nameservers"))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body)
	}
	var got PublicAnalysisListResponse[PublicAnalysisNameserverView]
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var ns1 *PublicAnalysisNameserverView
	for i := range got.Items {
		if got.Items[i].Nameserver == "ns1.example" {
			ns1 = &got.Items[i]
		}
	}
	if ns1 == nil {
		t.Fatalf("ns1.example missing from list: %+v", got.Items)
	}
	if ns1.LatencyP50MS == nil || *ns1.LatencyP50MS != 15 {
		t.Errorf("ns1 latency_p50_ms = %v, want 15", ns1.LatencyP50MS)
	}
	if ns1.LatencySamples != 1 {
		t.Errorf("ns1 latency_samples = %d, want 1", ns1.LatencySamples)
	}
}

// TestPublicNameserverDetailSurfacesLatency proves the per-nameserver detail
// endpoint carries the aggregated latency too.
func TestPublicNameserverDetailSurfacesLatency(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	f.seedEndpoint("run-lat", "a.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")
	dom, _ := f.store.GetOrCreateDomain("a.example")
	ns, _ := f.store.UpsertAnalysisNameserver("ns1.example", now)
	addr, _ := f.store.UpsertAnalysisAddress("192.0.2.1", "ipv4", now)
	if err := f.store.ReplaceAnalysisRunNSEndpoints(f.cohort.ID, "run-lat", []AnalysisRunNameserverEndpoint{
		{CohortID: f.cohort.ID, RunID: "run-lat", DomainID: dom.ID, NameserverID: ns.ID, AddressID: addr.ID, Role: "authoritative", Family: "ipv4", QueryCount: 1, AvgMS: 22},
	}); err != nil {
		t.Fatalf("reseed endpoint with latency: %v", err)
	}
	f.refreshSnapshotViews(f.batchID)

	resp := getPublic(t, f.srv, f.publicURL("nameservers/ns1.example"))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body)
	}
	var got PublicAnalysisNameserverDetail
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.LatencyP50MS == nil || *got.LatencyP50MS != 22 {
		t.Errorf("detail latency_p50_ms = %v, want 22", got.LatencyP50MS)
	}
	if got.LatencySamples != 1 {
		t.Errorf("detail latency_samples = %d, want 1", got.LatencySamples)
	}
}
