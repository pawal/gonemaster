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
// to graduate it with (the ASN tags live here).
type seedRun struct {
	domain  string
	nsHosts []string
	entries []engine.LogEntry
}

// seedBatchRuns creates a batch and graduates one succeeded run per seedRun.
// Graduation computes a score for every run, so all of them are eligible for
// the operator rollup.
func seedBatchRuns(t *testing.T, srv *Server, batchID string, runs []seedRun) {
	t.Helper()
	now := time.Now().UTC()
	if err := srv.store.CreateBatch(Batch{ID: batchID, CreatedAt: now}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}
	for i, sr := range runs {
		domain, err := srv.store.GetOrCreateDomain(sr.domain)
		if err != nil {
			t.Fatalf("GetOrCreateDomain: %v", err)
		}
		timings := make([]NameserverTiming, 0, len(sr.nsHosts))
		for _, host := range sr.nsHosts {
			timings = append(timings, NameserverTiming{Nameserver: host})
		}
		job := Job{
			ID:                fmt.Sprintf("job_%s_%d", batchID, i),
			BatchID:           batchID,
			Domain:            sr.domain,
			DomainID:          domain.ID,
			Status:            JobSucceeded,
			CreatedAt:         now,
			StartedAt:         now,
			FinishedAt:        now,
			NameserverTimings: timings,
		}
		if _, err := srv.store.Create(job); err != nil {
			t.Fatalf("Create job: %v", err)
		}
		entries := sr.entries
		if len(entries) == 0 {
			entries = []engine.LogEntry{{Timestamp: 1.0, Module: "System", Tag: "MODULE_START", Level: "INFO"}}
		}
		if err := srv.store.GraduateJob(job, entries); err != nil {
			t.Fatalf("GraduateJob: %v", err)
		}
	}
}

func getOperators(t *testing.T, srv *Server, query string) (*httptest.ResponseRecorder, BatchOperatorsResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/batches/"+query, nil)
	resp := httptest.NewRecorder()
	srv.Handler().ServeHTTP(resp, req)
	var body BatchOperatorsResponse
	if resp.Code == http.StatusOK {
		if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
	}
	return resp, body
}

func TestHandleBatchOperatorsNSParent(t *testing.T) {
	// Two domains served by nameservers under the same registrable parent
	// (nic.example) must roll up into a single operator row counting both.
	srv := New(DefaultConfig())
	seedBatchRuns(t, srv, "batch_ns", []seedRun{
		{domain: "alpha", nsHosts: []string{"a.nic.example", "b.nic.example"}},
		{domain: "beta", nsHosts: []string{"a.nic.example"}},
	})

	resp, body := getOperators(t, srv, "batch_ns/operators?group_by=ns_parent&min_count=1")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if body.GroupBy != "ns_parent" || body.MinCount != 1 {
		t.Fatalf("unexpected echo fields: %+v", body)
	}
	if len(body.Operators) != 1 {
		t.Fatalf("expected one operator, got %+v", body.Operators)
	}
	op := body.Operators[0]
	if op.Key != "nic.example" || op.DomainCount != 2 {
		t.Fatalf("operator = %+v, want key nic.example with domain_count 2", op)
	}
}

func TestHandleBatchOperatorsASN(t *testing.T) {
	// Two domains whose Connectivity03 entries report AS64500 must roll up
	// into one ASN operator counting both domains.
	srv := New(DefaultConfig())
	asnEntry := engine.LogEntry{
		Timestamp: 1.0, Module: "Connectivity", Testcase: "Connectivity03",
		Tag: "IPV4_ONE_ASN", Level: "INFO", Args: map[string]any{"asn": float64(64500)},
	}
	seedBatchRuns(t, srv, "batch_asn", []seedRun{
		{domain: "alpha", entries: []engine.LogEntry{asnEntry}},
		{domain: "beta", entries: []engine.LogEntry{asnEntry}},
	})

	resp, body := getOperators(t, srv, "batch_asn/operators?group_by=asn&min_count=1")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(body.Operators) != 1 {
		t.Fatalf("expected one operator, got %+v", body.Operators)
	}
	if op := body.Operators[0]; op.Key != "64500" || op.DomainCount != 2 {
		t.Fatalf("operator = %+v, want key 64500 with domain_count 2", op)
	}
}

func TestHandleBatchOperatorsValidation(t *testing.T) {
	// group_by is required and constrained; min_count and limit must be
	// positive integers when supplied.
	srv := New(DefaultConfig())
	seedBatchRuns(t, srv, "batch_val", []seedRun{{domain: "alpha", nsHosts: []string{"a.nic.example"}}})

	bad := []string{
		"batch_val/operators",                          // missing group_by
		"batch_val/operators?group_by=bogus",           // unknown group_by
		"batch_val/operators?group_by=asn&min_count=0", // min_count below 1
		"batch_val/operators?group_by=asn&min_count=x", // min_count not an int
		"batch_val/operators?group_by=asn&limit=0",     // limit below 1
	}
	for _, q := range bad {
		resp, _ := getOperators(t, srv, q)
		if resp.Code != http.StatusBadRequest {
			t.Errorf("query %q: expected 400, got %d", q, resp.Code)
		}
	}
}

func TestHandleBatchOperatorsNotFound(t *testing.T) {
	srv := New(DefaultConfig())
	resp, _ := getOperators(t, srv, "missing/operators?group_by=asn")
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}

func TestHandleBatchOperatorsIncomplete(t *testing.T) {
	// A batch with a still-queued job is not complete, so the rollup is
	// refused with 409 rather than returning a partial answer.
	srv := New(DefaultConfig())
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

	resp, _ := getOperators(t, srv, "batch_busy/operators?group_by=asn")
	if resp.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", resp.Code, resp.Body.String())
	}
}
