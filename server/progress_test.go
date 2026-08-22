package server

import (
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/logger"
)

func TestProgressUpdatesForFullSuite(t *testing.T) {
	srv := newTestServer(t)
	spy := newProgressSpy()
	srv.store = spy

	srv.engineRunner = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		if req.LogCallback != nil {
			_ = req.LogCallback(&logger.Entry{Tag: "TEST_CASE_START", Testcase: "basic01", Module: "basic"})
			_ = req.LogCallback(&logger.Entry{Tag: "TEST_CASE_END", Testcase: "basic01", Module: "basic"})
			_ = req.LogCallback(&logger.Entry{Tag: "TEST_CASE_START", Testcase: "basic02", Module: "basic"})
			_ = req.LogCallback(&logger.Entry{Tag: "TEST_CASE_END", Testcase: "basic02", Module: "basic"})
		}
		return nil, nil
	}

	job := Job{
		ID:        "job-full-suite",
		Domain:    "example.com",
		Status:    JobQueued,
		CreatedAt: time.Now().UTC(),
		Progress:  0,
		Overrides: map[string]any{
			"test_cases": []any{"basic01", "basic02"},
		},
	}
	if _, err := srv.store.Create(job); err != nil {
		t.Fatalf("create job: %v", err)
	}

	if err := srv.runJob(job.ID); err != nil {
		t.Fatalf("run job: %v", err)
	}

	progresses := spy.Progresses()
	seen := map[int]bool{}
	for _, value := range progresses {
		seen[value] = true
	}
	if !seen[50] || !seen[100] {
		t.Fatalf("expected progress updates including 50 and 100, got %v", progresses)
	}
}
