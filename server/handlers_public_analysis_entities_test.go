package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// seedEndpoint inserts one (run, domain, nameserver, address) endpoint row
// plus the normalized entity rows. Runs and domains are upserted as needed.
func (f *analysisAPITestFixture) seedEndpoint(runID, domainName, nameserverName, address, family string, finishedAt time.Time, asn int64, prefix string) {
	f.t.Helper()
	domain, err := f.store.GetOrCreateDomain(domainName)
	if err != nil {
		f.t.Fatalf("create domain: %v", err)
	}
	if _, ok := f.store.GetRun(runID); !ok {
		insertTestRun(f.t, f.store, Run{
			ID: runID, DomainID: domain.ID, Domain: domainName,
			Status: JobSucceeded, CreatedAt: finishedAt.Add(-time.Minute),
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
}

func TestPublicAnalysisEntitiesUseLatestRunPerDomain(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	t1 := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)

	f.seedEndpoint("run-old", "alpha.example", "ns-old.example", "192.0.2.10", "ipv4", t1, 64500, "192.0.2.0/24")
	f.seedEndpoint("run-new", "alpha.example", "ns-new.example", "198.51.100.20", "ipv4", t2, 64501, "198.51.100.0/24")

	nameservers := decodeJSON[PublicAnalysisListResponse[PublicAnalysisNameserverView]](
		t, getPublic(t, f.srv, "/pub/api/v1/analysis/nameservers"))
	if nameservers.Total != 1 || nameservers.Items[0].Nameserver != "ns-new.example" {
		t.Fatalf("expected only latest nameserver, got %+v", nameservers)
	}

	endpoints := decodeJSON[PublicAnalysisListResponse[PublicAnalysisEndpointView]](
		t, getPublic(t, f.srv, "/pub/api/v1/analysis/endpoints"))
	if endpoints.Total != 1 || endpoints.Items[0].Address != "198.51.100.20" {
		t.Fatalf("expected only latest endpoint, got %+v", endpoints)
	}

	asns := decodeJSON[PublicAnalysisListResponse[PublicAnalysisASNView]](
		t, getPublic(t, f.srv, "/pub/api/v1/analysis/asns"))
	if asns.Total != 1 || asns.Items[0].ASN != 64501 {
		t.Fatalf("expected only latest ASN, got %+v", asns)
	}

	prefixes := decodeJSON[PublicAnalysisListResponse[PublicAnalysisPrefixView]](
		t, getPublic(t, f.srv, "/pub/api/v1/analysis/prefixes"))
	if prefixes.Total != 1 || prefixes.Items[0].Prefix != "198.51.100.0/24" {
		t.Fatalf("expected only latest prefix, got %+v", prefixes)
	}
}

func decodeJSON[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func TestPublicAnalysisNameserversAggregates(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	ts := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	f.seedEndpoint("run-1", "alpha.example", "ns1.shared.example", "192.0.2.10", "ipv4", ts, 64500, "192.0.2.0/24")
	f.seedEndpoint("run-2", "beta.example", "ns1.shared.example", "192.0.2.10", "ipv4", ts, 64500, "192.0.2.0/24")
	f.seedEndpoint("run-2", "beta.example", "ns1.shared.example", "2001:db8::10", "ipv6", ts, 64500, "2001:db8::/32")
	f.seedEndpoint("run-3", "gamma.example", "ns2.other.example", "192.0.2.20", "ipv4", ts, 64600, "192.0.2.0/24")

	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/nameservers")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	got := decodeJSON[PublicAnalysisListResponse[PublicAnalysisNameserverView]](t, resp)
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

	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/nameservers?search=shared")
	got := decodeJSON[PublicAnalysisListResponse[PublicAnalysisNameserverView]](t, resp)
	if got.Total != 1 || got.Items[0].Nameserver != "ns1.shared.example" {
		t.Fatalf("unexpected search result: %+v", got)
	}
}

func TestPublicAnalysisEndpointsAggregates(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	ts := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	f.seedEndpoint("r1", "a.example", "ns.example", "192.0.2.10", "ipv4", ts, 64500, "192.0.2.0/24")
	f.seedEndpoint("r2", "b.example", "ns.example", "192.0.2.10", "ipv4", ts, 64500, "192.0.2.0/24")

	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/endpoints")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	got := decodeJSON[PublicAnalysisListResponse[PublicAnalysisEndpointView]](t, resp)
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

	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/asns?sort=domain_count_desc")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	got := decodeJSON[PublicAnalysisListResponse[PublicAnalysisASNView]](t, resp)
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

func TestPublicAnalysisASNsSearchByNumber(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	ts := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	f.seedEndpoint("r1", "a.example", "ns.example", "192.0.2.10", "ipv4", ts, 64500, "192.0.2.0/24")
	f.seedEndpoint("r2", "b.example", "ns.example", "192.0.2.20", "ipv4", ts, 64600, "192.0.2.0/24")

	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/asns?search=64500")
	got := decodeJSON[PublicAnalysisListResponse[PublicAnalysisASNView]](t, resp)
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

	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/prefixes")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	got := decodeJSON[PublicAnalysisListResponse[PublicAnalysisPrefixView]](t, resp)
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

func TestPublicAnalysisListEndpointsFailWithoutReadStore(t *testing.T) {
	srv := New(DefaultConfig())
	seedCohort(t, srv, AnalysisCohort{
		SourceType: "tag", SourceTag: "tld",
		AnalysisEnabled: true, PublicEnabled: true, IsDefault: true,
	})
	for _, path := range []string{
		"/pub/api/v1/analysis/nameservers",
		"/pub/api/v1/analysis/endpoints",
		"/pub/api/v1/analysis/asns",
		"/pub/api/v1/analysis/prefixes",
	} {
		resp := getPublic(t, srv, path)
		if resp.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s: expected 503 without read store, got %d", path, resp.Code)
		}
	}
}
