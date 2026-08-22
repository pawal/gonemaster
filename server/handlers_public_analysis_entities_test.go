package server

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// seedEndpoint inserts one (run, domain, nameserver, address) endpoint row
// plus the normalized entity rows. Runs and domains are upserted as needed.
func (f *analysisAPITestFixture) seedEndpoint(runID, domainName, nameserverName, address, family string, finishedAt time.Time, asn int64, prefix string, avgMS ...float64) {
	f.seedEndpointInBatch(f.batchID, runID, domainName, nameserverName, address, family, finishedAt, asn, prefix, avgMS...)
}

// seedEndpointInBatch is seedEndpoint with an explicit batch id. Used by
// tests that need to scope runs into different snapshots. An optional avgMS
// sets the endpoint's average response time so the capture-time aggregation
// produces latency; omitting it leaves the endpoint latency-free.
func (f *analysisAPITestFixture) seedEndpointInBatch(batchID, runID, domainName, nameserverName, address, family string, finishedAt time.Time, asn int64, prefix string, avgMS ...float64) {
	f.t.Helper()
	domain, err := f.store.GetOrCreateDomain(domainName)
	if err != nil {
		f.t.Fatalf("create domain: %v", err)
	}
	if _, ok := f.store.GetRun(runID); !ok {
		insertTestRun(f.t, f.store, Run{
			ID: runID, DomainID: domain.ID, Domain: domainName,
			BatchID: batchID,
			Status:  JobSucceeded, CreatedAt: finishedAt.Add(-time.Minute),
			StartedAt: finishedAt.Add(-time.Minute), FinishedAt: finishedAt,
		})
	}
	if _, ok := f.store.GetAnalysisRunDomainSummary(f.cohort.ID, runID, domain.ID); !ok {
		if err := f.store.UpsertAnalysisRunDomainSummary(AnalysisRunDomainSummary{
			CohortID: f.cohort.ID,
			RunID:    runID,
			DomainID: domain.ID,
		}); err != nil {
			f.t.Fatalf("upsert placeholder summary: %v", err)
		}
	}
	ns, err := f.store.UpsertAnalysisNameserver(nameserverName, finishedAt)
	if err != nil {
		f.t.Fatalf("upsert nameserver: %v", err)
	}
	addr, err := f.store.UpsertAnalysisAddress(address, family, finishedAt)
	if err != nil {
		f.t.Fatalf("upsert address: %v", err)
	}
	// Overwrite endpoints+address_asns for this run with additive semantics by
	// collecting the existing rows and re-emitting them.
	existingEndpoints := f.store.ListAnalysisRunNSEndpoints(f.cohort.ID, runID)
	newEndpoint := AnalysisRunNameserverEndpoint{
		CohortID: f.cohort.ID, RunID: runID, DomainID: domain.ID,
		NameserverID: ns.ID, AddressID: addr.ID,
		Role: "authoritative", Source: "timings", Family: family, QueryCount: 1,
	}
	if len(avgMS) > 0 {
		newEndpoint.AvgMS = avgMS[0]
	}
	existingEndpoints = append(existingEndpoints, newEndpoint)
	if err := f.store.ReplaceAnalysisRunNSEndpoints(f.cohort.ID, runID, existingEndpoints); err != nil {
		f.t.Fatalf("replace endpoints: %v", err)
	}

	var prefixID *int64
	if prefix != "" {
		pfx, err := f.store.UpsertAnalysisPrefix(prefix, family, finishedAt)
		if err != nil {
			f.t.Fatalf("upsert prefix: %v", err)
		}
		prefixID = &pfx.ID
	}
	if asn != 0 {
		if _, err := f.store.UpsertAnalysisASN(asn, "", finishedAt); err != nil {
			f.t.Fatalf("upsert asn: %v", err)
		}
	}

	existingASNs := f.store.ListAnalysisRunAddressASNs(f.cohort.ID, runID)
	found := false
	for i := range existingASNs {
		if existingASNs[i].AddressID == addr.ID {
			if asn != 0 {
				asnCopy := asn
				existingASNs[i].ASN = &asnCopy
			}
			existingASNs[i].PrefixID = prefixID
			found = true
			break
		}
	}
	if !found {
		fact := AnalysisRunAddressASN{
			CohortID: f.cohort.ID, RunID: runID, DomainID: domain.ID,
			AddressID: addr.ID, PrefixID: prefixID,
		}
		if asn != 0 {
			asnCopy := asn
			fact.ASN = &asnCopy
		}
		existingASNs = append(existingASNs, fact)
	}
	if err := f.store.ReplaceAnalysisRunAddressASNs(f.cohort.ID, runID, existingASNs); err != nil {
		f.t.Fatalf("replace address asns: %v", err)
	}
	f.refreshSnapshotViews(batchID)
}

// TestPublicAnalysisEntitiesScopedToSnapshot verifies that the public
// read path is pinned to the resolved snapshot: the fixture's default
// (auto-latest captured) snapshot shows only its own batch's facts, and
// an explicit ?snapshot=<older-slug> flips to the older snapshot.
func TestPublicAnalysisEntitiesScopedToSnapshot(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	t1 := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)

	// Older snapshot: captured earlier, not auto-latest.
	older := f.seedAlternateSnapshot("batch-old", "2026-04-17-old", t1.Add(-time.Hour))
	f.seedEndpointInBatch(older.BatchID, "run-old", "alpha.example", "ns-old.example",
		"192.0.2.10", "ipv4", t1, 64500, "192.0.2.0/24")

	// Default snapshot (auto-latest): the fixture's own batch.
	f.seedEndpointInBatch(f.batchID, "run-new", "alpha.example", "ns-new.example",
		"198.51.100.20", "ipv4", t2, 64501, "198.51.100.0/24")

	nameservers := mustJSON[PublicAnalysisListResponse[PublicAnalysisNameserverView]](
		t, getPublic(t, f.srv, f.publicURL("nameservers")), http.StatusOK)
	if nameservers.Total != 1 || nameservers.Items[0].Nameserver != "ns-new.example" {
		t.Fatalf("auto-latest should expose only the newer snapshot, got %+v", nameservers)
	}

	// Pinning the older slug in the path flips to the older snapshot.
	oldNameservers := mustJSON[PublicAnalysisListResponse[PublicAnalysisNameserverView]](
		t, getPublic(t, f.srv, f.publicURLForSnapshot("2026-04-17-old", "nameservers")), http.StatusOK)
	if oldNameservers.Total != 1 || oldNameservers.Items[0].Nameserver != "ns-old.example" {
		t.Fatalf("explicit slug should pin to older snapshot, got %+v", oldNameservers)
	}

	endpoints := mustJSON[PublicAnalysisListResponse[PublicAnalysisEndpointView]](
		t, getPublic(t, f.srv, f.publicURL("endpoints")), http.StatusOK)
	if endpoints.Total != 1 || endpoints.Items[0].Address != "198.51.100.20" {
		t.Fatalf("auto-latest endpoints: got %+v", endpoints)
	}

	asns := mustJSON[PublicAnalysisListResponse[PublicAnalysisASNView]](
		t, getPublic(t, f.srv, f.publicURL("asns")), http.StatusOK)
	if asns.Total != 1 || asns.Items[0].ASN != 64501 {
		t.Fatalf("auto-latest ASNs: got %+v", asns)
	}

	prefixes := mustJSON[PublicAnalysisListResponse[PublicAnalysisPrefixView]](
		t, getPublic(t, f.srv, f.publicURL("prefixes")), http.StatusOK)
	if prefixes.Total != 1 || prefixes.Items[0].Prefix != "198.51.100.0/24" {
		t.Fatalf("auto-latest prefixes: got %+v", prefixes)
	}
}

func TestPublicAnalysisNameserversAggregates(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	ts := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	f.seedEndpoint("run-1", "alpha.example", "ns1.shared.example", "192.0.2.10", "ipv4", ts, 64500, "192.0.2.0/24")
	f.seedEndpoint("run-2", "beta.example", "ns1.shared.example", "192.0.2.10", "ipv4", ts, 64500, "192.0.2.0/24")
	f.seedEndpoint("run-2", "beta.example", "ns1.shared.example", "2001:db8::10", "ipv6", ts, 64500, "2001:db8::/32")
	f.seedEndpoint("run-3", "gamma.example", "ns2.other.example", "192.0.2.20", "ipv4", ts, 64600, "192.0.2.0/24")

	resp := getPublic(t, f.srv, f.publicURL("nameservers"))
	got := mustJSON[PublicAnalysisListResponse[PublicAnalysisNameserverView]](t, resp, http.StatusOK)
	if got.Total != 2 {
		t.Fatalf("expected 2 nameservers, got %d", got.Total)
	}
	byName := map[string]PublicAnalysisNameserverView{}
	for _, v := range got.Items {
		byName[v.Nameserver] = v
	}
	shared := byName["ns1.shared.example"]
	if shared.DomainCount != 2 {
		t.Fatalf("expected ns1 to serve 2 domains, got %d", shared.DomainCount)
	}
	if shared.EndpointCount != 2 {
		t.Fatalf("expected ns1 to have 2 endpoints (ipv4+ipv6), got %d", shared.EndpointCount)
	}
	if shared.IPv4Count != 1 || shared.IPv6Count != 1 {
		t.Fatalf("expected ns1 split ipv4=1/ipv6=1, got %+v", shared)
	}
	if shared.ASNCount != 1 {
		t.Fatalf("expected ns1 ASN count 1, got %d", shared.ASNCount)
	}
}

func TestPublicAnalysisNameserversSearch(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	ts := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	f.seedEndpoint("r1", "a.example", "ns1.shared.example", "192.0.2.1", "ipv4", ts, 64500, "")
	f.seedEndpoint("r2", "b.example", "ns2.other.example", "192.0.2.2", "ipv4", ts, 64500, "")

	resp := getPublic(t, f.srv, f.publicURL("nameservers?search=shared"))
	got := mustJSON[PublicAnalysisListResponse[PublicAnalysisNameserverView]](t, resp, http.StatusOK)
	if got.Total != 1 || got.Items[0].Nameserver != "ns1.shared.example" {
		t.Fatalf("unexpected search result: %+v", got)
	}
}

func TestPublicAnalysisEndpointsAggregates(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	ts := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	f.seedEndpoint("r1", "a.example", "ns.example", "192.0.2.10", "ipv4", ts, 64500, "192.0.2.0/24")
	f.seedEndpoint("r2", "b.example", "ns.example", "192.0.2.10", "ipv4", ts, 64500, "192.0.2.0/24")

	resp := getPublic(t, f.srv, f.publicURL("endpoints"))
	got := mustJSON[PublicAnalysisListResponse[PublicAnalysisEndpointView]](t, resp, http.StatusOK)
	if got.Total != 1 {
		t.Fatalf("expected 1 endpoint entry, got %d", got.Total)
	}
	ep := got.Items[0]
	if ep.DomainCount != 2 {
		t.Fatalf("expected 2 domains on endpoint, got %d", ep.DomainCount)
	}
	if ep.Prefix != "192.0.2.0/24" {
		t.Fatalf("expected prefix attached, got %q", ep.Prefix)
	}
	if ep.ASN == nil || *ep.ASN != 64500 {
		t.Fatalf("expected asn=64500, got %+v", ep.ASN)
	}
}

func TestPublicAnalysisASNsAggregates(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	ts := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	f.seedEndpoint("r1", "a.example", "ns1.example", "192.0.2.10", "ipv4", ts, 64500, "192.0.2.0/24")
	f.seedEndpoint("r2", "b.example", "ns2.example", "192.0.2.20", "ipv4", ts, 64500, "192.0.2.0/24")
	f.seedEndpoint("r3", "c.example", "ns3.example", "2001:db8::30", "ipv6", ts, 64600, "2001:db8::/32")

	resp := getPublic(t, f.srv, f.publicURL("asns?sort=domain_count_desc"))
	got := mustJSON[PublicAnalysisListResponse[PublicAnalysisASNView]](t, resp, http.StatusOK)
	if got.Total != 2 {
		t.Fatalf("expected 2 ASNs, got %d", got.Total)
	}
	if got.Items[0].ASN != 64500 || got.Items[0].DomainCount != 2 {
		t.Fatalf("expected 64500 at top with 2 domains, got %+v", got.Items[0])
	}
	if got.Items[0].NameserverCount != 2 {
		t.Fatalf("expected 64500 to cover 2 nameservers, got %+v", got.Items[0])
	}
}

func TestPublicAnalysisASNsSortByNameserverAndPrefixCounts(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	ts := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	// AS64500: one nameserver, one prefix.
	f.seedEndpoint("r1", "a.example", "ns1.example", "192.0.2.10", "ipv4", ts, 64500, "192.0.2.0/24")
	// AS64600: two nameservers, two prefixes.
	f.seedEndpoint("r2", "b.example", "ns2.example", "198.51.100.10", "ipv4", ts, 64600, "198.51.100.0/24")
	f.seedEndpoint("r3", "c.example", "ns3.example", "203.0.113.10", "ipv4", ts, 64600, "203.0.113.0/24")

	cases := []struct {
		name    string
		sort    string
		wantTop int64
	}{
		{"nameserver_count_desc", "nameserver_count_desc", 64600},
		{"nameserver_count_asc", "nameserver_count_asc", 64500},
		{"prefix_count_desc", "prefix_count_desc", 64600},
		{"prefix_count_asc", "prefix_count_asc", 64500},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := getPublic(t, f.srv, f.publicURL("asns?sort=")+c.sort)
			if resp.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
			}
			got := mustJSON[PublicAnalysisListResponse[PublicAnalysisASNView]](t, resp, http.StatusOK)
			if len(got.Items) != 2 {
				t.Fatalf("expected 2 ASN rows, got %+v", got.Items)
			}
			if got.Items[0].ASN != c.wantTop {
				t.Fatalf("%s: top ASN = %d, want %d (items=%+v)",
					c.name, got.Items[0].ASN, c.wantTop, got.Items)
			}
		})
	}
}

func TestPublicAnalysisASNsSearchByNumber(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	ts := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	f.seedEndpoint("r1", "a.example", "ns.example", "192.0.2.10", "ipv4", ts, 64500, "192.0.2.0/24")
	f.seedEndpoint("r2", "b.example", "ns.example", "192.0.2.20", "ipv4", ts, 64600, "192.0.2.0/24")

	resp := getPublic(t, f.srv, f.publicURL("asns?search=64500"))
	got := mustJSON[PublicAnalysisListResponse[PublicAnalysisASNView]](t, resp, http.StatusOK)
	if got.Total != 1 || got.Items[0].ASN != 64500 {
		t.Fatalf("expected only AS64500, got %+v", got)
	}
}

func TestPublicAnalysisPrefixesAggregates(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	ts := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	f.seedEndpoint("r1", "a.example", "ns.example", "192.0.2.10", "ipv4", ts, 64500, "192.0.2.0/24")
	f.seedEndpoint("r2", "b.example", "ns.example", "192.0.2.11", "ipv4", ts, 64500, "192.0.2.0/24")
	f.seedEndpoint("r3", "c.example", "ns.example", "2001:db8::10", "ipv6", ts, 64500, "2001:db8::/32")

	resp := getPublic(t, f.srv, f.publicURL("prefixes"))
	got := mustJSON[PublicAnalysisListResponse[PublicAnalysisPrefixView]](t, resp, http.StatusOK)
	if got.Total != 2 {
		t.Fatalf("expected 2 prefixes, got %d", got.Total)
	}
	byPrefix := map[string]PublicAnalysisPrefixView{}
	for _, v := range got.Items {
		byPrefix[v.Prefix] = v
	}
	v4 := byPrefix["192.0.2.0/24"]
	if v4.DomainCount != 2 || v4.AddressCount != 2 {
		t.Fatalf("expected v4 prefix to cover 2 domains and 2 addresses, got %+v", v4)
	}
	if v4.ASN == nil || *v4.ASN != 64500 {
		t.Fatalf("expected v4 prefix ASN=64500, got %+v", v4.ASN)
	}
	if byPrefix["2001:db8::/32"].Family != "ipv6" {
		t.Fatalf("expected v6 prefix family=ipv6, got %+v", byPrefix["2001:db8::/32"])
	}
}

func TestPublicAnalysisListEndpointsRedactsInternalIDs(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	ts := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	f.seedEndpoint("r1", "a.example", "ns.example", "192.0.2.10", "ipv4", ts, 64500, "192.0.2.0/24")

	for _, path := range []string{
		"/pub/api/v1/analysis/nameservers",
		"/pub/api/v1/analysis/endpoints",
		"/pub/api/v1/analysis/asns",
		"/pub/api/v1/analysis/prefixes",
	} {
		resp := getPublic(t, f.srv, path)
		raw := resp.Body.String()
		for _, needle := range []string{
			`"domain_id"`, `"run_id"`, `"cohort_id"`,
			`"nameserver_id"`, `"address_id"`, `"prefix_id"`,
		} {
			if strings.Contains(raw, needle) {
				t.Fatalf("%s should not contain %s: %s", path, needle, raw)
			}
		}
	}
}
