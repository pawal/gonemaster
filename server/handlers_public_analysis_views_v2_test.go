package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

// TestASNDetailReadsFromViewTable proves the ASN detail handler serves
// from analysis_snapshot_asn_view rather than walking the address-fact
// table on each request: after seeding a captured snapshot, we wipe the
// underlying fact rows and the handler still renders the per-ASN page.
func TestASNDetailReadsFromViewTable(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", now, nil)
	f.seedGraduatedRun("beta.example", now, nil)
	runAlpha := "run-alpha.example-" + now.Format("20060102150405")
	runBeta := "run-beta.example-" + now.Format("20060102150405")
	f.seedEndpoint(runAlpha, "alpha.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")
	f.seedEndpoint(runBeta, "beta.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")

	for _, runID := range []string{runAlpha, runBeta} {
		if err := f.store.ReplaceAnalysisRunNSEndpoints(f.cohort.ID, runID, nil); err != nil {
			t.Fatalf("clear endpoints %s: %v", runID, err)
		}
		if err := f.store.ReplaceAnalysisRunAddressASNs(f.cohort.ID, runID, nil); err != nil {
			t.Fatalf("clear address asns %s: %v", runID, err)
		}
	}

	resp := getPublic(t, f.srv, f.publicURL("asns/64500"))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body)
	}
	var got PublicAnalysisASNDetail
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ASN != 64500 {
		t.Errorf("ASN = %d, want 64500", got.ASN)
	}
	if got.DomainCount != 2 {
		t.Errorf("DomainCount = %d, want 2 (must come from view)", got.DomainCount)
	}
	if len(got.Domains) != 2 || got.Domains[0] != "alpha.example" || got.Domains[1] != "beta.example" {
		t.Errorf("Domains = %v, want sorted [alpha.example beta.example]", got.Domains)
	}
	if len(got.Nameservers) != 1 || got.Nameservers[0] != "ns1.example" {
		t.Errorf("Nameservers = %v, want [ns1.example]", got.Nameservers)
	}
	if len(got.Prefixes) != 1 || got.Prefixes[0] != "192.0.2.0/24" {
		t.Errorf("Prefixes = %v, want [192.0.2.0/24]", got.Prefixes)
	}
}

func TestASNDetailNotFoundOnUnknownASN(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", now, nil)
	f.seedEndpoint("run-alpha.example-"+now.Format("20060102150405"),
		"alpha.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")

	resp := getPublic(t, f.srv, f.publicURL("asns/9999"))
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.Code)
	}
}

func TestASNDetailCacheHeadersExplicitSnapshot(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", now, nil)
	f.seedEndpoint("run-alpha.example-"+now.Format("20060102150405"),
		"alpha.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")

	resp := getPublic(t, f.srv, f.publicURL("asns/64500"))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body)
	}
	if cc := resp.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("explicit-snapshot Cache-Control = %q, want immutable", cc)
	}
}

// TestPrefixDetailReadsFromViewTable proves /prefix?prefix= serves from
// the per-snapshot prefix view, not from a fact-row walk.
func TestPrefixDetailReadsFromViewTable(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", now, nil)
	f.seedGraduatedRun("beta.example", now, nil)
	runAlpha := "run-alpha.example-" + now.Format("20060102150405")
	runBeta := "run-beta.example-" + now.Format("20060102150405")
	f.seedEndpoint(runAlpha, "alpha.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")
	f.seedEndpoint(runBeta, "beta.example", "ns1.example", "192.0.2.2", "ipv4", now, 64500, "192.0.2.0/24")

	for _, runID := range []string{runAlpha, runBeta} {
		if err := f.store.ReplaceAnalysisRunNSEndpoints(f.cohort.ID, runID, nil); err != nil {
			t.Fatalf("clear endpoints %s: %v", runID, err)
		}
		if err := f.store.ReplaceAnalysisRunAddressASNs(f.cohort.ID, runID, nil); err != nil {
			t.Fatalf("clear address asns %s: %v", runID, err)
		}
	}

	resp := getPublic(t, f.srv, f.publicURL("prefix?prefix=192.0.2.0/24"))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body)
	}
	var got PublicAnalysisPrefixDetail
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Prefix != "192.0.2.0/24" || got.Family != "ipv4" {
		t.Errorf("prefix detail = %+v", got)
	}
	if got.DomainCount != 2 || got.AddressCount != 2 {
		t.Errorf("counts = (domains=%d, addresses=%d), want (2,2)", got.DomainCount, got.AddressCount)
	}
	if len(got.ASNs) != 1 || got.ASNs[0] != 64500 {
		t.Errorf("ASNs = %v, want [64500]", got.ASNs)
	}
}

func TestPrefixDetailNotFoundOnUnknownPrefix(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", now, nil)
	f.seedEndpoint("run-alpha.example-"+now.Format("20060102150405"),
		"alpha.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")

	resp := getPublic(t, f.srv, f.publicURL("prefix?prefix=10.0.0.0/8"))
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.Code)
	}
}

// TestPrefixListingReadsFromViewTable confirms /prefixes is backed by
// ListSnapshotPrefixViews instead of an in-Go fact-row aggregation.
func TestPrefixListingReadsFromViewTable(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", now, nil)
	f.seedGraduatedRun("beta.example", now, nil)
	f.seedEndpoint("run-alpha.example-"+now.Format("20060102150405"),
		"alpha.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")
	f.seedEndpoint("run-beta.example-"+now.Format("20060102150405"),
		"beta.example", "ns1.example", "2001:db8::1", "ipv6", now, 64500, "2001:db8::/32")

	resp := getPublic(t, f.srv, f.publicURL("prefixes"))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body)
	}
	var got PublicAnalysisListResponse[PublicAnalysisPrefixView]
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Total != 2 {
		t.Errorf("total = %d, want 2", got.Total)
	}
	families := map[string]bool{}
	for _, item := range got.Items {
		families[item.Family] = true
	}
	if !families["ipv4"] || !families["ipv6"] {
		t.Errorf("families = %v, want both ipv4 + ipv6", families)
	}
}

// TestDomainsListingReadsFromViewTable proves /domains is wired to the
// per-snapshot domain view: the response is unchanged after the
// underlying fact rows are wiped.
func TestDomainsListingReadsFromViewTable(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", now, []engine.LogEntry{
		{Module: "BASIC", Testcase: "basic01", Tag: "B01_OK", Level: "NOTICE"},
	})
	f.seedGraduatedRun("beta.example", now, []engine.LogEntry{
		{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
	})
	runAlpha := "run-alpha.example-" + now.Format("20060102150405")
	runBeta := "run-beta.example-" + now.Format("20060102150405")
	f.seedEndpoint(runAlpha, "alpha.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")
	f.seedEndpoint(runBeta, "beta.example", "ns2.example", "2001:db8::1", "ipv6", now, 64600, "2001:db8::/32")

	for _, runID := range []string{runAlpha, runBeta} {
		if err := f.store.ReplaceAnalysisRunNSEndpoints(f.cohort.ID, runID, nil); err != nil {
			t.Fatalf("clear endpoints %s: %v", runID, err)
		}
		if err := f.store.ReplaceAnalysisRunAddressASNs(f.cohort.ID, runID, nil); err != nil {
			t.Fatalf("clear address asns %s: %v", runID, err)
		}
	}

	resp := getPublic(t, f.srv, f.publicURL("domains"))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body)
	}
	var got PublicAnalysisListResponse[PublicAnalysisDomainView]
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Total != 2 {
		t.Errorf("total = %d, want 2", got.Total)
	}
	domains := map[string]PublicAnalysisDomainView{}
	for _, v := range got.Items {
		domains[v.Domain] = v
	}
	alpha := domains["alpha.example"]
	if alpha.OperatorASN == nil || *alpha.OperatorASN != 64500 {
		t.Errorf("alpha OperatorASN = %v, want 64500", alpha.OperatorASN)
	}
	beta := domains["beta.example"]
	if beta.OperatorASN == nil || *beta.OperatorASN != 64600 {
		t.Errorf("beta OperatorASN = %v, want 64600", beta.OperatorASN)
	}
}

// TestTestcaseDetailReadsFromTagView proves the testcase detail handler
// aggregates from the per-snapshot tag view instead of rescanning the
// entries table per latest run.
func TestTestcaseDetailReadsFromTagView(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", now, []engine.LogEntry{
		{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
		{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_DETAIL", Level: "NOTICE"},
	})
	f.seedGraduatedRun("beta.example", now, []engine.LogEntry{
		{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
	})

	resp := getPublic(t, f.srv, f.publicURL("testcase?module=DNSSEC&testcase=dnssec07"))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body)
	}
	var got PublicAnalysisTestcaseDetail
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Testcase != "dnssec07" || got.Module != "DNSSEC" {
		t.Errorf("got module/testcase = %s/%s", got.Module, got.Testcase)
	}
	if got.DomainCount != 2 {
		t.Errorf("DomainCount = %d, want 2", got.DomainCount)
	}
	if len(got.Tags) != 2 {
		t.Errorf("Tags = %v, want 2 distinct tags", got.Tags)
	}
	if got.WorstLevel != "ERROR" {
		t.Errorf("WorstLevel = %q, want ERROR", got.WorstLevel)
	}
}

// TestDiffReadsFromDomainViews exercises the diff endpoint on two
// captured snapshots: added/removed/grade-changed/level-changed
// domains all surface from the per-snapshot domain views.
func TestDiffReadsFromDomainViews(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	t1 := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	t2 := t1.Add(24 * time.Hour)

	older := f.seedAlternateSnapshot("batch-old", "2026-04-17-old", t1.Add(-time.Hour))
	f.seedGraduatedRunInBatch(older.BatchID, "kept.example", t1, nil)
	f.seedGraduatedRunInBatch(older.BatchID, "removed.example", t1, nil)

	f.seedGraduatedRunInBatch(f.batchID, "kept.example", t2, nil)
	f.seedGraduatedRunInBatch(f.batchID, "added.example", t2, nil)

	d, _ := f.store.GetDomainByName("kept.example")
	gradeNew := "A"
	if err := f.store.UpsertAnalysisRunDomainSummary(AnalysisRunDomainSummary{
		CohortID: f.cohort.ID,
		RunID:    "run-kept.example-" + t2.Format("20060102150405"),
		DomainID: d.ID, Grade: &gradeNew, WorstLevel: "NOTICE",
	}); err != nil {
		t.Fatalf("upsert kept summary new: %v", err)
	}
	gradeOld := "C"
	if err := f.store.UpsertAnalysisRunDomainSummary(AnalysisRunDomainSummary{
		CohortID: f.cohort.ID,
		RunID:    "run-kept.example-" + t1.Format("20060102150405"),
		DomainID: d.ID, Grade: &gradeOld, WorstLevel: "ERROR",
	}); err != nil {
		t.Fatalf("upsert kept summary old: %v", err)
	}
	f.refreshSnapshotViews(older.BatchID)
	f.refreshSnapshotViews(f.batchID)

	url := "/pub/api/v1/analysis/cohorts/tld/diff?from=" + older.Slug + "&to=" + f.snapshot.Slug
	resp := getPublic(t, f.srv, url)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body)
	}
	var got PublicAnalysisDiffResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Added) != 1 || got.Added[0].Domain != "added.example" {
		t.Errorf("Added = %+v, want [added.example]", got.Added)
	}
	if len(got.Removed) != 1 || got.Removed[0].Domain != "removed.example" {
		t.Errorf("Removed = %+v, want [removed.example]", got.Removed)
	}
	foundGrade := false
	for _, e := range got.GradeChanged {
		if e.Domain == "kept.example" {
			foundGrade = true
		}
	}
	if !foundGrade {
		t.Errorf("GradeChanged should include kept.example, got %+v", got.GradeChanged)
	}
	foundLevel := false
	for _, e := range got.LevelChanged {
		if e.Domain == "kept.example" {
			foundLevel = true
		}
	}
	if !foundLevel {
		t.Errorf("LevelChanged should include kept.example, got %+v", got.LevelChanged)
	}
}

// TestComputeSnapshotEntityViewsPopulatesPrefixView asserts the
// capture-time builder writes a per-snapshot prefix row per CIDR seen
// in the batch's authoritative-address fact set.
func TestComputeSnapshotEntityViewsPopulatesPrefixView(t *testing.T) {
	s := testStoreForBackend(t, testBackends(t)[0])
	cohortID, snapID := snapshotViewFixture(t, s)

	views, err := s.ComputeSnapshotEntityViews(cohortID, "batch-x")
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	if err := s.ReplaceSnapshotEntityViews(snapID, views); err != nil {
		t.Fatalf("replace: %v", err)
	}
	rows := s.ListSnapshotPrefixViews(snapID)
	if len(rows) == 0 {
		t.Fatal("expected prefix view rows, got none")
	}
	byPrefix := map[string]AnalysisSnapshotPrefixView{}
	for _, row := range rows {
		byPrefix[row.Prefix] = row
	}
	v4 := byPrefix["192.0.2.0/24"]
	if v4.Family != "ipv4" {
		t.Errorf("v4 family = %q, want ipv4", v4.Family)
	}
	if v4.AddressCount == 0 {
		t.Errorf("v4 address_count = 0, want >0")
	}
	if len(v4.Domains) == 0 {
		t.Errorf("v4 domains roster is empty")
	}
}

// TestASNViewRoundTripPreservesRosters round-trips ASN view rows
// through ReplaceSnapshotEntityViews → List, asserting the new
// domains/nameservers/prefixes JSON columns rehydrate correctly.
func TestASNViewRoundTripPreservesRosters(t *testing.T) {
	s := testStoreForBackend(t, testBackends(t)[0])
	cohortID, snapID := snapshotViewFixture(t, s)

	views, err := s.ComputeSnapshotEntityViews(cohortID, "batch-x")
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	if err := s.ReplaceSnapshotEntityViews(snapID, views); err != nil {
		t.Fatalf("replace: %v", err)
	}
	rows := s.ListSnapshotASNViews(snapID)
	for _, row := range rows {
		if row.DomainCount > 0 && row.Domains == nil {
			t.Errorf("asn %d: Domains lost (domain_count=%d)", row.ASN, row.DomainCount)
		}
		if row.NameserverCount > 0 && row.Nameservers == nil {
			t.Errorf("asn %d: Nameservers lost", row.ASN)
		}
		if row.PrefixCount > 0 && row.Prefixes == nil {
			t.Errorf("asn %d: Prefixes lost", row.ASN)
		}
	}
}

// TestLegacyMaterializationCacheGone is a compile-time guard: if the
// legacy cache infrastructure is reintroduced, the symbol references
// here will fail to compile, alerting whoever did it.
//
// This test does no runtime work; it merely asserts that the public
// surface of the rewritten read paths still returns 200 on a captured
// snapshot when the underlying fact tables are wiped — proving nothing
// in the request path silently rebuilds them.
func TestPublicReadsAreFactRowIndependent(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", now, []engine.LogEntry{
		{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
	})
	runID := "run-alpha.example-" + now.Format("20060102150405")
	f.seedEndpoint(runID, "alpha.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")

	if err := f.store.ReplaceAnalysisRunNSEndpoints(f.cohort.ID, runID, nil); err != nil {
		t.Fatalf("clear endpoints: %v", err)
	}
	if err := f.store.ReplaceAnalysisRunAddressASNs(f.cohort.ID, runID, nil); err != nil {
		t.Fatalf("clear address asns: %v", err)
	}
	if err := f.store.ReplaceAnalysisRunTagSummaries(f.cohort.ID, runID, nil); err != nil {
		t.Fatalf("clear tag summaries: %v", err)
	}

	for _, path := range []string{
		"/pub/api/v1/analysis/cohorts/tld",
		f.publicURL("domains"),
		f.publicURL("asns/64500"),
		f.publicURL("prefix?prefix=192.0.2.0/24"),
		f.publicURL("prefixes"),
		f.publicURL("nameservers/ns1.example"),
		f.publicURL("endpoints/192.0.2.1"),
		f.publicURL("domains/alpha.example"),
		f.publicURL("tags/DS07_NOT_SIGNED"),
		f.publicURL("testcase?module=DNSSEC&testcase=dnssec07"),
	} {
		resp := getPublic(t, f.srv, path)
		if resp.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200 (handler is reading wiped fact rows)", path, resp.Code)
		}
	}
}
