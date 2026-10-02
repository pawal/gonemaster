package server

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"
)

// countingAnalysisStore counts the analysis reads a handler makes.
type countingAnalysisStore struct {
	*SQLJobStore
	mu          sync.Mutex
	asnGets     int
	asnLists    [][]int64
	domainLists int
}

func (c *countingAnalysisStore) GetAnalysisASN(asn int64) (AnalysisASN, bool) {
	c.mu.Lock()
	c.asnGets++
	c.mu.Unlock()
	return c.SQLJobStore.GetAnalysisASN(asn)
}

func (c *countingAnalysisStore) ListAnalysisASNs(ctx context.Context, asns []int64) map[int64]AnalysisASN {
	c.mu.Lock()
	c.asnLists = append(c.asnLists, asns)
	c.mu.Unlock()
	return c.SQLJobStore.ListAnalysisASNs(ctx, asns)
}

func (c *countingAnalysisStore) ListSnapshotDomainViews(ctx context.Context, snapshotID int64) []AnalysisSnapshotDomainView {
	c.mu.Lock()
	c.domainLists++
	c.mu.Unlock()
	return c.SQLJobStore.ListSnapshotDomainViews(ctx, snapshotID)
}

// seedLabelledEndpoints seeds alpha on AS64500 and beta on AS64600, both labelled.
func seedLabelledEndpoints(t *testing.T, f *analysisFixture) {
	t.Helper()
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", now, nil)
	f.seedGraduatedRun("beta.example", now, nil)
	f.seedEndpoint("run-alpha.example-"+now.Format("20060102150405"),
		"alpha.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")
	f.seedEndpoint("run-beta.example-"+now.Format("20060102150405"),
		"beta.example", "ns2.example", "198.51.100.1", "ipv4", now, 64600, "198.51.100.0/24")
	for asn, label := range map[int64]string{64500: "Alpha AS", 64600: "Beta AS"} {
		if _, err := f.store.UpsertAnalysisASN(asn, label, now); err != nil {
			t.Fatalf("label asn %d: %v", asn, err)
		}
	}
}

func TestDomainsListReadsASNLabelsForThePageInOneQuery(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	seedLabelledEndpoints(t, f)
	counting := &countingAnalysisStore{SQLJobStore: f.store}
	f.srv.store = counting

	got := mustJSON[PublicAnalysisListResponse[PublicAnalysisDomainView]](t,
		getPublic(t, f.srv, f.publicURL("domains?limit=1")), http.StatusOK)

	if got.Total != 2 || len(got.Items) != 1 {
		t.Fatalf("total = %d, items = %d, want 2 and 1", got.Total, len(got.Items))
	}
	if got.Items[0].Domain != "alpha.example" || got.Items[0].Operator != "Alpha AS" {
		t.Errorf("item = %s operated by %q, want alpha.example by Alpha AS", got.Items[0].Domain, got.Items[0].Operator)
	}
	if counting.asnGets != 0 {
		t.Errorf("per-row ASN reads = %d, want 0", counting.asnGets)
	}
	if len(counting.asnLists) != 1 || len(counting.asnLists[0]) != 1 || counting.asnLists[0][0] != 64500 {
		t.Errorf("ASN list reads = %v, want one read of [64500]", counting.asnLists)
	}
}

func TestPrefixListReadsASNLabelsForThePageInOneQuery(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	seedLabelledEndpoints(t, f)
	counting := &countingAnalysisStore{SQLJobStore: f.store}
	f.srv.store = counting

	got := mustJSON[PublicAnalysisListResponse[PublicAnalysisPrefixView]](t,
		getPublic(t, f.srv, f.publicURL("prefixes?limit=1")), http.StatusOK)

	if got.Total != 2 || len(got.Items) != 1 {
		t.Fatalf("total = %d, items = %d, want 2 and 1", got.Total, len(got.Items))
	}
	if got.Items[0].Prefix != "192.0.2.0/24" || got.Items[0].ASNLabel != "Alpha AS" {
		t.Errorf("item = %s labelled %q, want 192.0.2.0/24 labelled Alpha AS", got.Items[0].Prefix, got.Items[0].ASNLabel)
	}
	if counting.asnGets != 0 {
		t.Errorf("per-row ASN reads = %d, want 0", counting.asnGets)
	}
	if len(counting.asnLists) != 1 || len(counting.asnLists[0]) != 1 {
		t.Errorf("ASN list reads = %v, want one read of one ASN", counting.asnLists)
	}
}

func TestDiffIsServedFromCacheOnRepeat(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	t1 := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	older := f.seedAlternateSnapshot("batch-old", "2026-04-17-old", t1.Add(-time.Hour))
	f.seedGraduatedRunInBatch(older.BatchID, "removed.example", t1, nil)
	f.seedGraduatedRunInBatch(f.batchID, "added.example", t1.Add(24*time.Hour), nil)
	f.refreshSnapshotViews(older.BatchID)
	f.refreshSnapshotViews(f.batchID)
	counting := &countingAnalysisStore{SQLJobStore: f.store}
	f.srv.store = counting

	url := "/pub/api/v1/analysis/cohorts/tld/diff?from=" + older.Slug + "&to=" + f.snapshot.Slug
	first := mustJSON[PublicAnalysisDiffResponse](t, getPublic(t, f.srv, url), http.StatusOK)
	second := mustJSON[PublicAnalysisDiffResponse](t, getPublic(t, f.srv, url), http.StatusOK)

	if counting.domainLists != 2 {
		t.Errorf("domain view reads = %d, want 2 for the first diff and none for the repeat", counting.domainLists)
	}
	if len(first.Added) != 1 || len(second.Added) != 1 || second.Added[0].Domain != "added.example" {
		t.Errorf("added = %+v then %+v, want added.example twice", first.Added, second.Added)
	}
}

func TestListSnapshotDomainViewsStopsOnCanceledContext(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	f.seedGraduatedRun("alpha.example", time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC), nil)
	if rows := f.store.ListSnapshotDomainViews(t.Context(), f.snapshot.ID); len(rows) != 1 {
		t.Fatalf("rows with a live context = %d, want 1", len(rows))
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if rows := f.store.ListSnapshotDomainViews(ctx, f.snapshot.ID); rows != nil {
		t.Errorf("rows with a canceled context = %d, want none", len(rows))
	}
}
