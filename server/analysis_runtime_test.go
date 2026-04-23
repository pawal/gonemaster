package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

type spyAnalysisController struct {
	repairCalls    chan struct{}
	projectRunIDs  chan string
	projectRunErr  error
	rebuildCalls   chan int64
	clearCalls     chan int64
	reconcileCalls chan struct{}
}

func newSpyAnalysisController() *spyAnalysisController {
	return &spyAnalysisController{
		repairCalls:    make(chan struct{}, 1),
		projectRunIDs:  make(chan string, 4),
		rebuildCalls:   make(chan int64, 1),
		clearCalls:     make(chan int64, 1),
		reconcileCalls: make(chan struct{}, 1),
	}
}

func (s *spyAnalysisController) RepairAllCohorts(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case s.repairCalls <- struct{}{}:
	default:
	}
	return nil
}

func (s *spyAnalysisController) ProjectRun(runID string) error {
	select {
	case s.projectRunIDs <- runID:
	default:
	}
	return s.projectRunErr
}

func (s *spyAnalysisController) RebuildCohort(_ context.Context, cohortID int64) error {
	select {
	case s.rebuildCalls <- cohortID:
	default:
	}
	return nil
}

func (s *spyAnalysisController) ClearCohort(_ context.Context, cohortID int64) error {
	select {
	case s.clearCalls <- cohortID:
	default:
	}
	return nil
}

func (s *spyAnalysisController) ReconcileCohortChange(_ context.Context, _, _ AnalysisCohort) error {
	select {
	case s.reconcileCalls <- struct{}{}:
	default:
	}
	return nil
}

func (s *spyAnalysisController) CaptureCompletedSnapshots(_ context.Context) error {
	return nil
}

func (s *spyAnalysisController) BackfillSnapshotsFromFacts(_ context.Context) (AnalysisSnapshotBackfillReport, error) {
	return AnalysisSnapshotBackfillReport{}, nil
}

func TestServerStartTriggersAnalysisRepair(t *testing.T) {
	cfg := DefaultConfig()
	cfg.WorkerCount = 1
	srv := New(cfg)
	spy := newSpyAnalysisController()
	srv.SetAnalysisController(spy)

	srv.Start()
	defer func() {
		if err := srv.Stop(context.Background()); err != nil {
			t.Fatalf("Stop: %v", err)
		}
	}()

	select {
	case <-spy.repairCalls:
	case <-time.After(time.Second):
		t.Fatal("expected startup analysis repair call")
	}
}

func TestRunJobInvokesAnalysisProjector(t *testing.T) {
	srv := New(DefaultConfig())
	spy := newSpyAnalysisController()
	srv.SetAnalysisController(spy)
	srv.engineRunner = func(_ engine.RunRequest) ([]engine.LogEntry, error) {
		return nil, nil
	}

	job := Job{
		ID:        "job-analysis-project",
		Domain:    "example.com",
		Status:    JobQueued,
		CreatedAt: time.Now().UTC(),
	}
	if _, err := srv.store.Create(job); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := srv.runJob(job.ID); err != nil {
		t.Fatalf("runJob: %v", err)
	}

	select {
	case got := <-spy.projectRunIDs:
		if got != job.ID {
			t.Fatalf("expected projected run %q, got %q", job.ID, got)
		}
	case <-time.After(time.Second):
		t.Fatal("expected projected run callback")
	}
}

func TestRunJobIgnoresAnalysisProjectionErrors(t *testing.T) {
	srv := New(DefaultConfig())
	spy := newSpyAnalysisController()
	spy.projectRunErr = errors.New("boom")
	srv.SetAnalysisController(spy)
	srv.engineRunner = func(_ engine.RunRequest) ([]engine.LogEntry, error) {
		return nil, nil
	}

	job := Job{
		ID:        "job-analysis-error",
		Domain:    "example.com",
		Status:    JobQueued,
		CreatedAt: time.Now().UTC(),
	}
	if _, err := srv.store.Create(job); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := srv.runJob(job.ID); err != nil {
		t.Fatalf("runJob should ignore analysis projector errors, got %v", err)
	}
}
