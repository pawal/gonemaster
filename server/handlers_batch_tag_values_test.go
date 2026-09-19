package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

// seedRun describes one graduated run to insert into a batch: its domain, the
// authoritative nameserver hostnames to attach as timings, and the log entries
// to graduate it with.
type seedRun struct {
	domain  string
	nsHosts []string
	entries []engine.LogEntry
}

// seedBatchRuns creates a batch and graduates one succeeded run per seedRun.
// Graduation computes a score for every run, so all of them are scored.
func seedBatchRuns(t *testing.T, srv *Server, batchID string, runs []seedRun) {
	t.Helper()
	now := time.Now().UTC()
	if err := srv.store.CreateBatch(Batch{ID: batchID, CreatedAt: now}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}
	for i, sr := range runs {
		timings := make([]NameserverTiming, 0, len(sr.nsHosts))
		for _, host := range sr.nsHosts {
			timings = append(timings, NameserverTiming{Nameserver: host})
		}
		entries := sr.entries
		if len(entries) == 0 {
			entries = systemStartEntry()
		}
		seedGraduatedRun(t, srv.store, runSpec{
			ID:            fmt.Sprintf("job_%s_%d", batchID, i),
			Domain:        sr.domain,
			BatchID:       batchID,
			At:            now,
			Entries:       entries,
			Timings:       timings,
			ResolveDomain: true,
		})
	}
}

func getTagValues(t *testing.T, srv *Server, query string) (*httptest.ResponseRecorder, BatchTagValuesResponse) {
	t.Helper()
	resp := doJSON(t, srv, http.MethodGet, "/api/v1/batches/"+query, nil)
	var body BatchTagValuesResponse
	if resp.Code == http.StatusOK {
		if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
	}
	return resp, body
}

// nsidEntry builds a single N16_HAS_NSID log entry carrying nsid=v.
func nsidEntry(v string) engine.LogEntry {
	return engine.LogEntry{
		Timestamp: 1.0, Module: "Nameserver", Testcase: "Nameserver16",
		Tag: "N16_HAS_NSID", Level: "INFO", Args: map[string]any{"nsid": v},
	}
}

func TestHandleBatchTagValuesScalar(t *testing.T) {
	// Three domains carry nsid values A, B, A. The rollup must count A twice
	// and B once, ranked by count descending (A first).
	srv := newTestServer(t)
	seedBatchRuns(t, srv, "batch_nsid", []seedRun{
		{domain: "alpha", entries: []engine.LogEntry{nsidEntry("A")}},
		{domain: "beta", entries: []engine.LogEntry{nsidEntry("B")}},
		{domain: "gamma", entries: []engine.LogEntry{nsidEntry("A")}},
	})

	resp, body := getTagValues(t, srv, "batch_nsid/tag-values?tag=N16_HAS_NSID&arg=nsid")
	wantStatus(t, resp, http.StatusOK)
	if body.Tag != "N16_HAS_NSID" || body.Arg != "nsid" || body.MinCount != 1 {
		t.Fatalf("unexpected echo fields: %+v", body)
	}
	if len(body.Values) != 2 {
		t.Fatalf("expected two values, got %+v", body.Values)
	}
	if v := body.Values[0]; v.Value != "A" || v.Count != 2 {
		t.Errorf("top value = %+v, want A with count 2", v)
	}
	if v := body.Values[1]; v.Value != "B" || v.Count != 1 {
		t.Errorf("second value = %+v, want B with count 1", v)
	}
	if body.Values[0].AvgScore != nil {
		t.Errorf("avg_score must be omitted without weight_by_score, got %v", *body.Values[0].AvgScore)
	}
}

func TestHandleBatchTagValuesListUnpack(t *testing.T) {
	// A list-valued arg is unpacked: a single run whose nameservers arg holds
	// three hosts contributes one count to each host.
	srv := newTestServer(t)
	listEntry := engine.LogEntry{
		Timestamp: 1.0, Module: "Delegation", Testcase: "Delegation01",
		Tag: "DEL_NS", Level: "INFO",
		Args: map[string]any{"nameservers": []any{"x.example", "y.example", "z.example"}},
	}
	seedBatchRuns(t, srv, "batch_list", []seedRun{
		{domain: "alpha", entries: []engine.LogEntry{listEntry}},
	})

	resp, body := getTagValues(t, srv, "batch_list/tag-values?tag=DEL_NS&arg=nameservers")
	wantStatus(t, resp, http.StatusOK)
	if len(body.Values) != 3 {
		t.Fatalf("expected three unpacked values, got %+v", body.Values)
	}
	for _, v := range body.Values {
		if v.Count != 1 {
			t.Errorf("value %q count = %d, want 1", v.Value, v.Count)
		}
		if len(v.SampleDomains) != 1 || v.SampleDomains[0] != "alpha" {
			t.Errorf("value %q samples = %v, want [alpha]", v.Value, v.SampleDomains)
		}
	}
}

func TestHandleBatchTagValuesWeightByScore(t *testing.T) {
	// With weight_by_score, every value row carries an avg_score and the
	// response echoes the flag.
	srv := newTestServer(t)
	seedBatchRuns(t, srv, "batch_w", []seedRun{
		{domain: "alpha", entries: []engine.LogEntry{nsidEntry("A")}},
		{domain: "beta", entries: []engine.LogEntry{nsidEntry("B")}},
	})

	resp, body := getTagValues(t, srv, "batch_w/tag-values?tag=N16_HAS_NSID&arg=nsid&weight_by_score=true")
	wantStatus(t, resp, http.StatusOK)
	if body.WeightByScore != true {
		t.Fatalf("expected weight_by_score echo true, got %+v", body)
	}
	if len(body.Values) != 2 {
		t.Fatalf("values = %d, want the 2 seeded NSID strings", len(body.Values))
	}
	for _, v := range body.Values {
		if v.AvgScore == nil {
			t.Errorf("value %q missing avg_score under weight_by_score", v.Value)
		}
	}
}

func TestHandleBatchTagValuesValidation(t *testing.T) {
	// tag and arg are required; min_count, limit, and weight_by_score must
	// parse when supplied.
	srv := newTestServer(t)
	seedBatchRuns(t, srv, "batch_val", []seedRun{{domain: "alpha", entries: []engine.LogEntry{nsidEntry("A")}}})

	bad := []string{
		"batch_val/tag-values",                                                 // missing tag and arg
		"batch_val/tag-values?arg=nsid",                                        // missing tag
		"batch_val/tag-values?tag=N16_HAS_NSID",                                // missing arg
		"batch_val/tag-values?tag=N16_HAS_NSID&arg=nsid&min_count=0",           // min_count below 1
		"batch_val/tag-values?tag=N16_HAS_NSID&arg=nsid&min_count=x",           // min_count not an int
		"batch_val/tag-values?tag=N16_HAS_NSID&arg=nsid&limit=0",               // limit below 1
		"batch_val/tag-values?tag=N16_HAS_NSID&arg=nsid&weight_by_score=maybe", // bad bool
	}
	for _, q := range bad {
		resp, _ := getTagValues(t, srv, q)
		if resp.Code != http.StatusBadRequest {
			t.Errorf("query %q: expected 400, got %d", q, resp.Code)
		}
	}
}

func TestHandleBatchTagValuesMinCount(t *testing.T) {
	// min_count drops values that fewer than that many domains carry.
	srv := newTestServer(t)
	seedBatchRuns(t, srv, "batch_mc", []seedRun{
		{domain: "alpha", entries: []engine.LogEntry{nsidEntry("A")}},
		{domain: "beta", entries: []engine.LogEntry{nsidEntry("A")}},
		{domain: "gamma", entries: []engine.LogEntry{nsidEntry("B")}},
	})

	resp, body := getTagValues(t, srv, "batch_mc/tag-values?tag=N16_HAS_NSID&arg=nsid&min_count=2")
	wantStatus(t, resp, http.StatusOK)
	if len(body.Values) != 1 || body.Values[0].Value != "A" {
		t.Fatalf("min_count=2 should leave only A, got %+v", body.Values)
	}
}

func TestHandleBatchTagValuesNotFound(t *testing.T) {
	srv := newTestServer(t)
	resp, _ := getTagValues(t, srv, "missing/tag-values?tag=N16_HAS_NSID&arg=nsid")
	wantStatus(t, resp, http.StatusNotFound)
}

func TestHandleBatchTagValuesIncomplete(t *testing.T) {
	// A batch with a still-queued job is not complete, so the rollup is
	// refused with 409 rather than returning a partial answer.
	srv := newTestServer(t)
	now := time.Now().UTC()
	if err := srv.store.CreateBatch(Batch{ID: "batch_busy", CreatedAt: now}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}
	domain, err := srv.store.GetOrCreateDomain("pending.example")
	if err != nil {
		t.Fatalf("GetOrCreateDomain: %v", err)
	}
	if _, err := srv.store.Create(Job{
		ID: "job_busy", BatchID: "batch_busy", Domain: "pending.example",
		DomainID: domain.ID, Status: JobQueued, CreatedAt: now,
	}); err != nil {
		t.Fatalf("Create job: %v", err)
	}

	resp, _ := getTagValues(t, srv, "batch_busy/tag-values?tag=N16_HAS_NSID&arg=nsid")
	wantStatus(t, resp, http.StatusConflict)
}
