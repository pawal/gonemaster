package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

// TestDomainDetailReadsFromViewTable proves the slice-8 read path serves
// from analysis_snapshot_domain_view rather than re-aggregating the
// underlying fact rows on every request: after seeding a captured
// snapshot, we wipe the legacy fact tables and the handler must still
// render the per-domain detail.
func TestDomainDetailReadsFromViewTable(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", now, []engine.LogEntry{
		{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
	})
	runID := "run-alpha.example-" + now.Format("20060102150405")
	f.seedEndpoint(runID, "alpha.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")
	f.seedEndpoint(runID, "alpha.example", "ns1.example", "2001:db8::1", "ipv6", now, 64500, "2001:db8::/32")

	if err := f.store.ReplaceAnalysisRunNSEndpoints(f.cohort.ID, runID, nil); err != nil {
		t.Fatalf("clear endpoints: %v", err)
	}
	if err := f.store.ReplaceAnalysisRunAddressASNs(f.cohort.ID, runID, nil); err != nil {
		t.Fatalf("clear address asns: %v", err)
	}
	if err := f.store.ReplaceAnalysisRunTagSummaries(f.cohort.ID, runID, nil); err != nil {
		t.Fatalf("clear tag summaries: %v", err)
	}

	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/domains/alpha.example")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	var got PublicAnalysisDomainDetail
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Domain != "alpha.example" {
		t.Errorf("Domain = %q, want alpha.example", got.Domain)
	}
	if len(got.Nameservers) != 1 || got.Nameservers[0].Nameserver != "ns1.example" {
		t.Errorf("Nameservers = %+v, want one ns1.example row", got.Nameservers)
	}
	ns := got.Nameservers[0]
	if ns.IPv4Count != 1 || ns.IPv6Count != 1 {
		t.Errorf("ns dual-stack counts = (v4=%d, v6=%d), want (1,1)", ns.IPv4Count, ns.IPv6Count)
	}
	if len(ns.Addresses) != 2 {
		t.Errorf("ns addresses = %v, want both v4 + v6", ns.Addresses)
	}
	if len(got.Addresses) != 2 {
		t.Errorf("flat addresses = %v, want 2", got.Addresses)
	}
	tagsByName := map[string]PublicAnalysisDomainTag{}
	for _, ti := range got.Tags {
		tagsByName[ti.Tag] = ti
	}
	if ds07, ok := tagsByName["DS07_NOT_SIGNED"]; !ok || ds07.Level != "ERROR" || ds07.Testcase != "dnssec07" {
		t.Errorf("DS07 tag = %+v (full tags = %+v)", ds07, got.Tags)
	}
}

// TestDomainDetailCaseInsensitiveLookup proves the view's
// LOWER(domain_name) = LOWER(?) lookup works when the URL casing differs
// from the captured casing.
func TestDomainDetailCaseInsensitiveLookup(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("Alpha.Example", now, []engine.LogEntry{
		{Module: "BASIC", Testcase: "basic01", Tag: "B01_OK", Level: "NOTICE"},
	})

	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/domains/alpha.example")
	if resp.Code != http.StatusOK {
		t.Fatalf("lower-case lookup status = %d, body = %s", resp.Code, resp.Body)
	}
	var got PublicAnalysisDomainDetail
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Domain == "" {
		t.Errorf("Domain empty; expected captured casing preserved")
	}
}

// TestDomainDetailNotFoundOnUnknownName verifies the handler returns 404
// when the URL points at a domain absent from the snapshot.
func TestDomainDetailNotFoundOnUnknownName(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", now, nil)

	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/domains/does-not-exist.example")
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body %s)", resp.Code, resp.Body)
	}
}

// TestDomainDetailTagFloorFiltersBelowFloor proves the capture-time tag
// floor governs what tags appear in tags_json: with the default NOTICE
// floor, INFO chatter is dropped at capture and never surfaces.
func TestDomainDetailTagFloorFiltersBelowFloor(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", now, []engine.LogEntry{
		{Module: "BASIC", Testcase: "basic01", Tag: "B01_INFO", Level: "INFO"},
		{Module: "BASIC", Testcase: "basic01", Tag: "B01_NOTICE", Level: "NOTICE"},
		{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
	})

	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/domains/alpha.example")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	var got PublicAnalysisDomainDetail
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	tags := map[string]string{}
	for _, ti := range got.Tags {
		tags[ti.Tag] = ti.Level
	}
	if _, ok := tags["B01_INFO"]; ok {
		t.Errorf("INFO tag leaked under default NOTICE floor: %+v", tags)
	}
	if _, ok := tags["B01_NOTICE"]; !ok {
		t.Errorf("NOTICE tag missing: %+v", tags)
	}
	if _, ok := tags["DS07_NOT_SIGNED"]; !ok {
		t.Errorf("ERROR tag missing: %+v", tags)
	}
}

// TestDomainDetailTagFloorHonorsWarningOverride confirms that raising
// the floor at capture (via SetTagViewMinLevel) drops NOTICE+ tags that
// fall below the new floor on the next capture cycle.
func TestDomainDetailTagFloorHonorsWarningOverride(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	f.store.SetTagViewMinLevel("WARNING")
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", now, []engine.LogEntry{
		{Module: "BASIC", Testcase: "basic01", Tag: "B01_NOTICE", Level: "NOTICE"},
		{Module: "DELEGATION", Testcase: "delegation02", Tag: "REFERRAL_SLOW", Level: "WARNING"},
		{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
	})

	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/domains/alpha.example")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	var got PublicAnalysisDomainDetail
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	tags := map[string]string{}
	for _, ti := range got.Tags {
		tags[ti.Tag] = ti.Level
	}
	if _, ok := tags["B01_NOTICE"]; ok {
		t.Errorf("NOTICE tag leaked under WARNING floor: %+v", tags)
	}
	if _, ok := tags["REFERRAL_SLOW"]; !ok {
		t.Errorf("WARNING tag missing under WARNING floor: %+v", tags)
	}
	if _, ok := tags["DS07_NOT_SIGNED"]; !ok {
		t.Errorf("ERROR tag missing under WARNING floor: %+v", tags)
	}
}

// TestDomainDetailCacheHeadersExplicitSnapshot pins the immutable
// Cache-Control + ETag emitted when the URL pins a captured snapshot.
func TestDomainDetailCacheHeadersExplicitSnapshot(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", now, nil)

	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/domains/alpha.example?snapshot="+f.snapshot.Slug)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body)
	}
	if cc := resp.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("explicit-snapshot Cache-Control = %q, want immutable", cc)
	}
	if etag := resp.Header().Get("ETag"); etag == "" {
		t.Error("explicit-snapshot must set ETag")
	}
}

// TestDomainDetailCacheHeadersAutoLatest confirms the legacy auto-latest
// URL 307s with no-cache to the path-segmented form.
func TestDomainDetailCacheHeadersAutoLatest(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", now, nil)

	resp := getPublicNoRedirect(t, f.srv, "/pub/api/v1/analysis/domains/alpha.example")
	if resp.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want 307", resp.Code)
	}
	if cc := resp.Header().Get("Cache-Control"); !strings.Contains(cc, "no-cache") {
		t.Errorf("auto-latest redirect Cache-Control = %q, want no-cache", cc)
	}
}

// TestDomainViewBuilderAppliesNameserverStatus pins the projector-side
// bookkeeping for unresolved nameservers (AddressID=0) and unreachable
// addresses (QueryCount=0): the captured roster carries enough state
// for the UI to draw red badges without reloading facts.
func TestDomainViewBuilderAppliesNameserverStatus(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("ck.example", now, nil)
	runID := "run-ck.example-" + now.Format("20060102150405")
	f.seedEndpoint(runID, "ck.example", "ok.example", "192.0.2.10", "ipv4", now, 64500, "192.0.2.0/24")

	dead, err := f.store.UpsertAnalysisAddress("192.0.2.99", "ipv4", now)
	if err != nil {
		t.Fatalf("upsert dead address: %v", err)
	}
	deadNS, err := f.store.UpsertAnalysisNameserver("dead.example", now)
	if err != nil {
		t.Fatalf("upsert dead ns: %v", err)
	}
	ghostNS, err := f.store.UpsertAnalysisNameserver("ghost.example", now)
	if err != nil {
		t.Fatalf("upsert ghost ns: %v", err)
	}
	domain, _ := f.store.GetDomainByName("ck.example")
	existing := f.store.ListAnalysisRunNSEndpoints(f.cohort.ID, runID)
	existing = append(existing,
		AnalysisRunNameserverEndpoint{
			CohortID: f.cohort.ID, RunID: runID, DomainID: domain.ID,
			NameserverID: deadNS.ID, AddressID: dead.ID,
			Role: "authoritative", Source: "timings", Family: "ipv4", QueryCount: 0,
		},
		AnalysisRunNameserverEndpoint{
			CohortID: f.cohort.ID, RunID: runID, DomainID: domain.ID,
			NameserverID: ghostNS.ID, AddressID: 0,
			Role: "authoritative", Source: "delegation",
		},
	)
	if err := f.store.ReplaceAnalysisRunNSEndpoints(f.cohort.ID, runID, existing); err != nil {
		t.Fatalf("replace endpoints: %v", err)
	}
	f.refreshSnapshotViews(f.batchID)

	view, ok := f.store.GetSnapshotDomainViewByName(f.snapshot.ID, "ck.example")
	if !ok {
		t.Fatal("expected ck.example domain view row")
	}
	byName := map[string]DomainViewNameserver{}
	for _, ns := range view.Nameservers {
		byName[ns.Name] = ns
	}
	okNS := byName["ok.example"]
	if len(okNS.Addresses) != 1 || okNS.Addresses[0].Status != "" {
		t.Errorf("ok.example NS = %+v, want one reachable address", okNS)
	}
	deadNSView := byName["dead.example"]
	if len(deadNSView.Addresses) != 1 || deadNSView.Addresses[0].Status != "unreachable" {
		t.Errorf("dead.example NS = %+v, want one unreachable address", deadNSView)
	}
	ghost := byName["ghost.example"]
	if len(ghost.Addresses) != 0 || ghost.Status != "unresolved" {
		t.Errorf("ghost.example NS = %+v, want unresolved with no addresses", ghost)
	}
}

// TestDomainViewListRoundTripPreservesPayload asserts the full domain
// view round-trips through ReplaceSnapshotEntityViews → List unchanged:
// nameserver/address/tag JSON columns rehydrate into the same structs.
func TestDomainViewListRoundTripPreservesPayload(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", now, []engine.LogEntry{
		{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
	})
	runID := "run-alpha.example-" + now.Format("20060102150405")
	f.seedEndpoint(runID, "alpha.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")

	rows := f.store.ListSnapshotDomainViews(f.snapshot.ID)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.SnapshotID != f.snapshot.ID {
		t.Errorf("SnapshotID = %d, want %d", row.SnapshotID, f.snapshot.ID)
	}
	if row.DomainName != "alpha.example" {
		t.Errorf("DomainName = %q", row.DomainName)
	}
	if len(row.Nameservers) != 1 || row.Nameservers[0].Name != "ns1.example" {
		t.Errorf("Nameservers round-trip lost: %+v", row.Nameservers)
	}
	if len(row.Tags) != 1 || row.Tags[0].Tag != "DS07_NOT_SIGNED" {
		t.Errorf("Tags round-trip lost: %+v", row.Tags)
	}
	if len(row.Addresses) != 1 || row.Addresses[0].Address != "192.0.2.1" {
		t.Errorf("Addresses round-trip lost: %+v", row.Addresses)
	}
}
