package server

import (
	"net/http"
	"testing"
	"time"
)

func f64(v float64) *float64 { return &v }

// TestCompareLatencyP50 pins the nil-last ordering the latency sort relies on:
// a real value always precedes a nil one in both directions, equal or both-nil
// pairs report undecided so the caller can fall back to its lexical tiebreak,
// and the desc flag flips only the value-vs-value comparison.
func TestCompareLatencyP50(t *testing.T) {
	cases := []struct {
		name        string
		a, b        *float64
		desc        bool
		wantLess    bool
		wantDecided bool
	}{
		{"asc a<b", f64(10), f64(20), false, true, true},
		{"asc a>b", f64(30), f64(20), false, false, true},
		{"desc a>b", f64(30), f64(20), true, true, true},
		{"desc a<b", f64(10), f64(20), true, false, true},
		{"equal undecided", f64(10), f64(10), false, false, false},
		{"both nil undecided", nil, nil, false, false, false},
		{"a nil sorts last (asc)", nil, f64(10), false, false, true},
		{"a nil sorts last (desc)", nil, f64(10), true, false, true},
		{"b nil sorts last (asc)", f64(10), nil, false, true, true},
		{"b nil sorts last (desc)", f64(10), nil, true, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			less, decided := compareLatencyP50(c.a, c.b, c.desc)
			if less != c.wantLess || decided != c.wantDecided {
				t.Fatalf("compareLatencyP50 = (%v,%v), want (%v,%v)", less, decided, c.wantLess, c.wantDecided)
			}
		})
	}
}

// TestFilterMinLatencySamples confirms a zero/negative floor is a no-op and a
// positive floor drops rows below it while preserving order of the survivors.
func TestFilterMinLatencySamples(t *testing.T) {
	type row struct{ n, samples int }
	get := func(r row) int { return r.samples }
	rows := []row{{1, 0}, {2, 4}, {3, 5}, {4, 10}}

	if got := filterMinLatencySamples(rows, 0, get); len(got) != 4 {
		t.Fatalf("min=0 should keep all, got %d", len(got))
	}
	got := filterMinLatencySamples(rows, 5, get)
	if len(got) != 2 || got[0].n != 3 || got[1].n != 4 {
		t.Fatalf("min=5 = %+v, want rows 3 and 4", got)
	}
}

// TestSortNameserverViewsByLatency orders by median latency with nil last and a
// stable name tiebreak, in both directions.
func TestSortNameserverViewsByLatency(t *testing.T) {
	base := func() []PublicAnalysisNameserverView {
		return []PublicAnalysisNameserverView{
			{Nameserver: "c.example", LatencyP50MS: f64(30)},
			{Nameserver: "no.example"}, // no latency
			{Nameserver: "a.example", LatencyP50MS: f64(10)},
			{Nameserver: "b.example", LatencyP50MS: f64(20)},
		}
	}

	asc := base()
	sortNameserverViews(asc, "latency_p50_asc")
	if order := nsOrder(asc); order != "a.example,b.example,c.example,no.example" {
		t.Fatalf("asc order = %s", order)
	}

	desc := base()
	sortNameserverViews(desc, "latency_p50_desc")
	if order := nsOrder(desc); order != "c.example,b.example,a.example,no.example" {
		t.Fatalf("desc order = %s (nil must stay last)", order)
	}
}

func nsOrder(items []PublicAnalysisNameserverView) string {
	names := make([]string, len(items))
	for i, it := range items {
		names[i] = it.Nameserver
	}
	return joinComma(names)
}

// TestSortEndpointViewsByLatency mirrors the nameserver test for the endpoint
// keys-based comparator.
func TestSortEndpointViewsByLatency(t *testing.T) {
	base := func() []PublicAnalysisEndpointView {
		return []PublicAnalysisEndpointView{
			{Address: "c", LatencyP50MS: f64(30)},
			{Address: "no"},
			{Address: "a", LatencyP50MS: f64(10)},
			{Address: "b", LatencyP50MS: f64(20)},
		}
	}
	asc := base()
	sortEndpointViews(asc, "latency_p50_asc")
	if order := epOrder(asc); order != "a,b,c,no" {
		t.Fatalf("asc order = %s", order)
	}
	desc := base()
	sortEndpointViews(desc, "latency_p50_desc")
	if order := epOrder(desc); order != "c,b,a,no" {
		t.Fatalf("desc order = %s (nil must stay last)", order)
	}
}

func epOrder(items []PublicAnalysisEndpointView) string {
	addrs := make([]string, len(items))
	for i, it := range items {
		addrs[i] = it.Address
	}
	return joinComma(addrs)
}

func joinComma(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ","
		}
		out += p
	}
	return out
}

// TestPublicAnalysisASNsSortByLatency drives the real ASN list handler end to
// end: the aggregation turns seeded per-endpoint avg times into per-ASN medians,
// the latency_p50 sort orders them (nil-last handled by the unit tests), and
// min_latency_samples drops thinly-sampled ASNs from the ranking. AS64500 has
// two samples (10,30 -> p50 20), AS64600 one (100), AS64700 two (200,220 -> 210).
func TestPublicAnalysisASNsSortByLatency(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		ts := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
		f.seedEndpoint("r1", "a.example", "ns1.example", "192.0.2.10", "ipv4", ts, 64500, "192.0.2.0/24", 10)
		f.seedEndpoint("r2", "b.example", "ns1.example", "192.0.2.11", "ipv4", ts, 64500, "192.0.2.0/24", 30)
		f.seedEndpoint("r3", "c.example", "ns2.example", "198.51.100.10", "ipv4", ts, 64600, "198.51.100.0/24", 100)
		f.seedEndpoint("r4", "d.example", "ns3.example", "203.0.113.10", "ipv4", ts, 64700, "203.0.113.0/24", 200)
		f.seedEndpoint("r5", "e.example", "ns3.example", "203.0.113.11", "ipv4", ts, 64700, "203.0.113.0/24", 220)

		asc := mustJSON[PublicAnalysisListResponse[PublicAnalysisASNView]](
			t, getPublic(t, f.srv, f.publicURL("asns?sort=latency_p50_asc")), http.StatusOK)
		if len(asc.Items) != 3 || asc.Items[0].ASN != 64500 || asc.Items[2].ASN != 64700 {
			t.Fatalf("asc order wrong: %+v", asc.Items)
		}
		if asc.Items[0].LatencyP50MS == nil || *asc.Items[0].LatencyP50MS != 20 {
			t.Fatalf("AS64500 p50 = %v, want 20", asc.Items[0].LatencyP50MS)
		}

		desc := mustJSON[PublicAnalysisListResponse[PublicAnalysisASNView]](
			t, getPublic(t, f.srv, f.publicURL("asns?sort=latency_p50_desc")), http.StatusOK)
		if len(desc.Items) != 3 || desc.Items[0].ASN != 64700 || desc.Items[2].ASN != 64500 {
			t.Fatalf("desc order wrong: %+v", desc.Items)
		}

		// min_latency_samples=2 drops the single-sample AS64600.
		filtered := mustJSON[PublicAnalysisListResponse[PublicAnalysisASNView]](
			t, getPublic(t, f.srv, f.publicURL("asns?sort=latency_p50_desc&min_latency_samples=2")), http.StatusOK)
		if filtered.Total != 2 {
			t.Fatalf("min_latency_samples=2 total = %d, want 2 (%+v)", filtered.Total, filtered.Items)
		}
		for _, it := range filtered.Items {
			if it.ASN == 64600 {
				t.Fatalf("AS64600 (1 sample) should be filtered out: %+v", filtered.Items)
			}
		}
	})
}

// TestParseAnalysisListFilterMinLatencySamples rejects a negative floor.
func TestParseAnalysisListFilterMinLatencySamples(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		resp := getPublic(t, f.srv, f.publicURL("nameservers?min_latency_samples=-1"))
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("negative min_latency_samples = %d, want 400", resp.Code)
		}
	})
}
