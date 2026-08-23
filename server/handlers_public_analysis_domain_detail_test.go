package server

import (
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
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
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

		resp := getPublic(t, f.srv, f.publicURL("domains/alpha.example"))
		got := mustJSON[PublicAnalysisDomainDetail](t, resp, http.StatusOK)
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
	})
}

// TestDomainDetailCaseInsensitiveLookup proves the view's
// LOWER(domain_name) = LOWER(?) lookup works when the URL casing differs
// from the captured casing.
func TestDomainDetailCaseInsensitiveLookup(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
		f.seedGraduatedRun("Alpha.Example", now, []engine.LogEntry{
			{Module: "BASIC", Testcase: "basic01", Tag: "B01_OK", Level: "NOTICE"},
		})

		resp := getPublic(t, f.srv, f.publicURL("domains/alpha.example"))
		got := mustJSON[PublicAnalysisDomainDetail](t, resp, http.StatusOK)
		if got.Domain == "" {
			t.Errorf("Domain empty; expected captured casing preserved")
		}
	})
}

// TestDomainDetailIgnoresLocaleQueryParam verifies that ?locale= is dead:
// the analysis-ui has no language picker, so the endpoint always renders
// English log messages and any client-supplied locale value is ignored.
func TestDomainDetailIgnoresLocaleQueryParam(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
		f.seedGraduatedRun("alpha.example", now, []engine.LogEntry{
			{Module: "BASIC", Testcase: "basic01", Tag: "B01_NOTICE", Level: "NOTICE"},
		})

		plain := getPublic(t, f.srv, f.publicURL("domains/alpha.example"))
		wantStatus(t, plain, http.StatusOK)
		withLocale := getPublic(t, f.srv, f.publicURL("domains/alpha.example")+"?locale=../../etc/passwd")
		wantStatus(t, withLocale, http.StatusOK)
		if plain.Body.String() != withLocale.Body.String() {
			t.Fatalf("response differs when ?locale= is set; expected the parameter to be ignored")
		}
	})
}

// TestDomainDetailNotFoundOnUnknownName verifies the handler returns 404
// when the URL points at a domain absent from the snapshot.
func TestDomainDetailNotFoundOnUnknownName(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
		f.seedGraduatedRun("alpha.example", now, nil)

		resp := getPublic(t, f.srv, f.publicURL("domains/does-not-exist.example"))
		wantStatus(t, resp, http.StatusNotFound)
	})
}

// TestDomainDetailTagFloorFiltersBelowFloor proves the capture-time tag
// floor governs what tags appear in tags_json: with the default NOTICE
// floor, INFO chatter is dropped at capture and never surfaces.
func TestDomainDetailTagFloorFiltersBelowFloor(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
		f.seedGraduatedRun("alpha.example", now, []engine.LogEntry{
			{Module: "BASIC", Testcase: "basic01", Tag: "B01_INFO", Level: "INFO"},
			{Module: "BASIC", Testcase: "basic01", Tag: "B01_NOTICE", Level: "NOTICE"},
			{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
		})

		resp := getPublic(t, f.srv, f.publicURL("domains/alpha.example"))
		got := mustJSON[PublicAnalysisDomainDetail](t, resp, http.StatusOK)
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
	})
}

// TestDomainDetailTagFloorHonorsWarningOverride confirms that raising
// the floor at capture (via SetTagViewMinLevel) drops NOTICE+ tags that
// fall below the new floor on the next capture cycle.
func TestDomainDetailTagFloorHonorsWarningOverride(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		f.store.SetTagViewMinLevel("WARNING")
		now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
		f.seedGraduatedRun("alpha.example", now, []engine.LogEntry{
			{Module: "BASIC", Testcase: "basic01", Tag: "B01_NOTICE", Level: "NOTICE"},
			{Module: "DELEGATION", Testcase: "delegation02", Tag: "REFERRAL_SLOW", Level: "WARNING"},
			{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
		})

		resp := getPublic(t, f.srv, f.publicURL("domains/alpha.example"))
		got := mustJSON[PublicAnalysisDomainDetail](t, resp, http.StatusOK)
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
	})
}

// TestDomainDetailCacheHeadersExplicitSnapshot pins the revalidating
// Cache-Control + ETag emitted when the URL pins a captured snapshot.
// Snapshots can be rebuilt under the same slug, so the response must not
// be immutable.
func TestDomainDetailCacheHeadersExplicitSnapshot(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
		f.seedGraduatedRun("alpha.example", now, nil)

		resp := getPublic(t, f.srv, f.publicURL("domains/alpha.example"))
		wantStatus(t, resp, http.StatusOK)
		if cc := resp.Header().Get("Cache-Control"); strings.Contains(cc, "immutable") {
			t.Errorf("explicit-snapshot Cache-Control = %q, must not be immutable", cc)
		}
		if etag := resp.Header().Get("ETag"); etag == "" {
			t.Error("explicit-snapshot must set ETag")
		}
	})
}

// TestDomainViewBuilderAppliesNameserverStatus pins the projector-side
// bookkeeping for unresolved nameservers (AddressID=0) and unreachable
// addresses (QueryCount=0): the captured roster carries enough state
// for the UI to draw red badges without reloading facts.
func TestDomainViewBuilderAppliesNameserverStatus(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
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
	})
}

// TestDomainViewListRoundTripPreservesPayload asserts the full domain
// view round-trips through ReplaceSnapshotEntityViews → List unchanged:
// nameserver/address/tag JSON columns rehydrate into the same structs.
func TestDomainViewListRoundTripPreservesPayload(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
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
	})
}

// TestDomainDetailIncludesNameserverTimings proves the domain detail carries the
// run's per-nameserver response times so the analysis UI can show the same
// per-address (IPv4/IPv6) timing table the public result view has.
func TestDomainDetailIncludesNameserverTimings(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
		f.seedGraduatedRun("alpha.example", now, nil,
			NameserverTiming{Nameserver: "ns1.example", Address: "192.0.2.1", AvgMS: 20, MinMS: 18, MaxMS: 25, MedianMS: 20, Count: 5, Status: "ok"},
			NameserverTiming{Nameserver: "ns1.example", Address: "2001:db8::1", AvgMS: 40, MinMS: 38, MaxMS: 45, MedianMS: 40, Count: 5, Status: "ok"},
		)
		runID := "run-alpha.example-" + now.Format("20060102150405")
		f.seedEndpoint(runID, "alpha.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")
		f.seedEndpoint(runID, "alpha.example", "ns1.example", "2001:db8::1", "ipv6", now, 64500, "2001:db8::/32")

		got := mustJSON[PublicAnalysisDomainDetail](t, getPublic(t, f.srv, f.publicURL("domains/alpha.example")), http.StatusOK)
		if len(got.NameserverTimings) != 2 {
			t.Fatalf("nameserver_timings len = %d, want 2 (%+v)", len(got.NameserverTimings), got.NameserverTimings)
		}
		byAddr := map[string]NameserverTiming{}
		for _, tm := range got.NameserverTimings {
			byAddr[tm.Address] = tm
		}
		if v4, ok := byAddr["192.0.2.1"]; !ok || v4.AvgMS != 20 || v4.Count != 5 {
			t.Fatalf("v4 timing = %+v, want avg 20 count 5", v4)
		}
		if v6, ok := byAddr["2001:db8::1"]; !ok || v6.AvgMS != 40 {
			t.Fatalf("v6 timing = %+v, want avg 40", v6)
		}
	})
}
