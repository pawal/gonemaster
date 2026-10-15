package server

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

// seedHistoryFixture builds two captured snapshots for cohort "tld":
//   - older (2026-04-17): ns1.example serves a.example on AS 64500
//   - current (fixture default): ns1.example serves a.example AND b.example
//
// so a nameserver/ASN history has two points with a rising domain count.
func seedHistoryFixture(t *testing.T, f *analysisFixture) *analysisFixture {
	t.Helper()
	t1 := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)

	older := f.seedAlternateSnapshot("batch-old", "2026-04-17-old", t1.Add(-time.Hour))
	f.seedEndpointInBatch(older.BatchID, "run-old-a", "a.example", "ns1.example", "192.0.2.1", "ipv4", t1, 64500, "192.0.2.0/24")

	f.seedEndpointInBatch(f.batchID, "run-cur-a", "a.example", "ns1.example", "192.0.2.1", "ipv4", t2, 64500, "192.0.2.0/24")
	f.seedEndpointInBatch(f.batchID, "run-cur-b", "b.example", "ns1.example", "192.0.2.1", "ipv4", t2, 64500, "192.0.2.0/24")

	f.refreshSnapshotViews(older.BatchID)
	f.refreshSnapshotViews(f.batchID)
	return f
}

// historyURL builds the cohort-level (non-snapshot-scoped) history URL.
func (f *analysisFixture) historyURL(query string) string {
	return "/pub/api/v1/analysis/cohorts/" + f.cohort.SourceTag + "/history?" + query
}

// TestPublicEntityHistoryNameserver exercises the full endpoint: a nameserver
// history returns one point per captured snapshot, oldest first, with the
// per-snapshot domain count.
func TestPublicEntityHistoryNameserver(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		seedHistoryFixture(t, f)

		resp := getPublic(t, f.srv, f.historyURL("entity=nameserver&key=ns1.example"))
		got := mustJSON[PublicAnalysisEntityHistoryResponse](t, resp, http.StatusOK)
		if got.Entity != "nameserver" || got.Key != "ns1.example" {
			t.Errorf("echo = %s/%s, want nameserver/ns1.example", got.Entity, got.Key)
		}
		if len(got.Points) != 2 {
			t.Fatalf("points = %d, want 2: %+v", len(got.Points), got.Points)
		}
		// Oldest first: the 2026-04-17 snapshot precedes the fixture default.
		if got.Points[0].Slug != "2026-04-17-old" {
			t.Errorf("first point slug = %q, want 2026-04-17-old", got.Points[0].Slug)
		}
		if !got.Points[0].Present || got.Points[0].DomainCount != 1 {
			t.Errorf("older point = present %v count %d, want true 1", got.Points[0].Present, got.Points[0].DomainCount)
		}
		if !got.Points[1].Present || got.Points[1].DomainCount != 2 {
			t.Errorf("newer point = present %v count %d, want true 2", got.Points[1].Present, got.Points[1].DomainCount)
		}
	})
}

// TestAnalysisEntityHistoryBranches covers the asn, domain, and tag SQL
// branches at the store level. The tag branch has no data in the fixture, so
// every point reports Present=false - the graceful "absent" case.
func TestAnalysisEntityHistoryBranches(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		seedHistoryFixture(t, f)

		asn, err := f.store.AnalysisEntityHistory(f.cohort.ID, "asn", "64500")
		if err != nil {
			t.Fatalf("asn history: %v", err)
		}
		if len(asn) != 2 || !asn[1].Present || asn[1].DomainCount != 2 {
			t.Errorf("asn history = %+v, want 2 points ending present count 2", asn)
		}

		dom, err := f.store.AnalysisEntityHistory(f.cohort.ID, "domain", "a.example")
		if err != nil {
			t.Fatalf("domain history: %v", err)
		}
		if len(dom) != 2 || !dom[0].Present || !dom[1].Present {
			t.Errorf("domain history = %+v, want 2 present points", dom)
		}

		tag, err := f.store.AnalysisEntityHistory(f.cohort.ID, "tag", "DS07_NOT_SIGNED")
		if err != nil {
			t.Fatalf("tag history: %v", err)
		}
		if len(tag) != 2 {
			t.Fatalf("tag history points = %d, want 2", len(tag))
		}
		for _, p := range tag {
			if p.Present {
				t.Errorf("tag with no data should be absent, got present point %+v", p)
			}
		}

		if _, err := f.store.AnalysisEntityHistory(f.cohort.ID, "asn", "not-a-number"); err == nil {
			t.Errorf("expected error for non-numeric asn key")
		}
	})
}

// TestPublicEntityHistoryValidation rejects missing or unknown parameters.
func TestPublicEntityHistoryValidation(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		seedHistoryFixture(t, f)

		for _, tc := range []struct{ query, code string }{
			{"entity=nameserver", "missing_params"},
			{"key=ns1.example", "missing_params"},
			{"entity=bogus&key=x", "invalid_entity"},
			{"entity=asn&key=not-a-number", "invalid_key"},
			{"entity=asn&key=AS99999999999", "invalid_key"},
			{"entity=domain&key=" + strings.Repeat("a", 256), "invalid_key"},
		} {
			body := wantErrorCode(t, getPublic(t, f.srv, f.historyURL(tc.query)), http.StatusBadRequest, tc.code)
			if strings.Contains(body.Error.Message, "invalid asn") {
				t.Errorf("%s: message %q carries store text", tc.query, body.Error.Message)
			}
		}
		resp := getPublic(t, f.srv, f.historyURL("entity=asn&key=AS64500"))
		wantStatus(t, resp, http.StatusOK)
	})
}

// failingHistoryStore fails every history read with driver text.
type failingHistoryStore struct{ *SQLJobStore }

func (failingHistoryStore) AnalysisEntityHistory(int64, string, string) ([]AnalysisEntityHistoryPoint, error) {
	return nil, errors.New(`pq: relation "analysis_snapshot_tag_view" does not exist`)
}

func TestPublicEntityHistoryHidesStoreError(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	f.srv.store = failingHistoryStore{f.store}

	body := wantErrorCode(t, getPublic(t, f.srv, f.historyURL("entity=tag&key=X")), http.StatusInternalServerError, "history_error")
	if body.Error.Message != "history is unavailable" {
		t.Errorf("message = %q, want %q", body.Error.Message, "history is unavailable")
	}
}

// A rebuild restamps captured_at out of order; history stays oldest first.
func TestAnalysisEntityHistoryOrdersByRunWindow(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		seedHistoryFixture(t, f)

		older, ok := f.store.GetAnalysisCohortSnapshotByBatch(f.cohort.ID, "batch-old")
		if !ok {
			t.Fatal("missing older snapshot")
		}
		current, ok := f.store.GetAnalysisCohortSnapshotByBatch(f.cohort.ID, f.batchID)
		if !ok {
			t.Fatal("missing current snapshot")
		}
		older.CapturedAt = current.CapturedAt.Add(time.Second)
		if _, err := f.store.UpsertAnalysisCohortSnapshot(older); err != nil {
			t.Fatalf("restamp older snapshot: %v", err)
		}

		points, err := f.store.AnalysisEntityHistory(f.cohort.ID, "nameserver", "ns1.example")
		if err != nil {
			t.Fatalf("nameserver history: %v", err)
		}
		if len(points) != 2 {
			t.Fatalf("points = %d, want 2", len(points))
		}
		if points[0].Slug != "2026-04-17-old" {
			t.Errorf("first point slug = %q, want 2026-04-17-old", points[0].Slug)
		}
	})
}
