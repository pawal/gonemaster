package server

import "context"

// AnalysisController is the runtime hook surface for the analysis projector
// and cohort repair/control logic. The concrete implementation lives outside
// the root server package to avoid import cycles.
type AnalysisController interface {
	RepairAllCohorts(ctx context.Context) error
	ProjectRun(runID string) error
	RebuildCohort(ctx context.Context, cohortID int64) error
	ClearCohort(ctx context.Context, cohortID int64) error
	ReconcileCohortChange(ctx context.Context, before, after AnalysisCohort) error
	// CaptureCompletedSnapshots walks pending snapshots and promotes any
	// whose batches have drained. Called periodically by the server's
	// snapshot capture goroutine and once at startup so a server restart
	// promotes batches that finished while the process was down.
	CaptureCompletedSnapshots(ctx context.Context) error
	// BackfillSnapshotsFromFacts is the Phase 7 retroactive migration
	// that creates a captured snapshot row per historical (cohort,
	// batch) pair. Server startup calls it once, gated by a setting.
	BackfillSnapshotsFromFacts(ctx context.Context) (AnalysisSnapshotBackfillReport, error)
}

// AnalysisSnapshotBackfillReport mirrors analysis.SnapshotBackfillReport
// without the import cycle; the concrete analysis package implements
// the method and returns a matching struct.
type AnalysisSnapshotBackfillReport struct {
	CohortsScanned   int
	SnapshotsMade    int
	SnapshotsSkipped int
}

// SetAnalysisController installs the runtime controller used for startup
// repair and run-level projection hooks.
func (s *Server) SetAnalysisController(ctrl AnalysisController) {
	s.analysis = ctrl
}

// AnalysisController returns the configured runtime analysis controller.
func (s *Server) AnalysisController() AnalysisController {
	return s.analysis
}
