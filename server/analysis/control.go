package analysis

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	serverpkg "codeberg.org/pawal/gonemaster/server"
)

const rebuildPageSize = 100

// progressWriteInterval is how often the rebuild loop persists the
// in-memory done/total counters to the cohort row. Chosen to keep the
// per-run overhead small while still giving the UI a visibly-moving
// progress bar on a 1400-run rebuild.
const progressWriteInterval = 500 * time.Millisecond

// ControlStore is the store surface needed for cohort-wide repair, rebuild,
// and clear operations.
type ControlStore interface {
	Store
	GetAnalysisCohort(id int64) (serverpkg.AnalysisCohort, bool)
	UpsertAnalysisCohort(cohort serverpkg.AnalysisCohort) (serverpkg.AnalysisCohort, error)
	ListRuns(filter serverpkg.RunFilter) serverpkg.RunList
	ClearAnalysisCohortMaterialization(cohortID int64) error
	// SetAnalysisCohortProgress writes just the materialization_done /
	// materialization_total counters. Called from the rebuild loop so the
	// admin UI can render a progress bar via polling.
	SetAnalysisCohortProgress(cohortID int64, done, total int) error

	// Batch + snapshot surface used by the cohort-snapshot lifecycle.
	GetBatch(id string) (serverpkg.Batch, bool)
	UpsertAnalysisCohortSnapshot(snap serverpkg.AnalysisCohortSnapshot) (serverpkg.AnalysisCohortSnapshot, error)
	GetAnalysisCohortSnapshotByBatch(cohortID int64, batchID string) (serverpkg.AnalysisCohortSnapshot, bool)
	ListPendingAnalysisCohortSnapshots() []serverpkg.AnalysisCohortSnapshot
	ClearAnalysisCohortSnapshots(cohortID int64) error
	CountBatchSnapshotRuns(cohortID int64, batchID string) (runCount, domainCount int, firstFinished, lastFinished time.Time, err error)
	CountOutstandingJobsForBatch(batchID string) (int, error)
	CountUnprojectedSnapshotRuns(cohortID int64, batchID string) (int, error)
	ComputeSnapshotOverview(cohortID int64, batchID string) (serverpkg.SnapshotOverviewV2, error)
	ReplaceSnapshotOverview(snapshotID int64, overview serverpkg.SnapshotOverviewV2) error
	ComputeSnapshotEntityViews(cohortID int64, batchID string) (serverpkg.SnapshotEntityViews, error)
	ReplaceSnapshotEntityViews(snapshotID int64, views serverpkg.SnapshotEntityViews) error

	// First-boot backfill surface.
	GetSetting(key string) (string, bool)
	SetSetting(key, value string) error
}

// Controller coordinates per-run projection with cohort-wide rebuild and clear
// operations. It satisfies serverpkg.AnalysisController.
type Controller struct {
	store     ControlStore
	projector *Projector
}

var _ serverpkg.AnalysisController = (*Controller)(nil)

// NewController creates an analysis runtime controller over the given store.
func NewController(store ControlStore) *Controller {
	return &Controller{
		store:     store,
		projector: NewProjector(store),
	}
}

// SetEnricher forwards an optional enricher to the underlying projector.
func (c *Controller) SetEnricher(enricher Enricher) {
	if c == nil {
		return
	}
	c.projector.SetEnricher(enricher)
}

// NewControllerFromJobStore returns a controller only when the given job store
// also exposes the analysis control/store surface.
func NewControllerFromJobStore(store serverpkg.JobStore) (*Controller, bool) {
	controlStore, ok := store.(ControlStore)
	if !ok {
		return nil, false
	}
	return NewController(controlStore), true
}

// ProjectRun materializes one completed run and updates cohort-level
// materialization state for the matched cohorts. Two pollution gates run
// before any write:
//  1. A run with an empty batch_id never enters the analysis layer — public
//     UI one-offs and unbatched admin retests are excluded entirely.
//  2. A run whose batch carries snapshot_intent = false is also skipped;
//     that flag is the admin-UI checkbox's switch for "this batch becomes a
//     cohort snapshot".
//
// Both gates return nil so the worker's graduation path does not treat a
// deliberately-excluded run as a projection error.
func (c *Controller) ProjectRun(runID string) error {
	run, ok := c.store.GetRun(runID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrRunNotFound, runID)
	}
	if run.BatchID == "" {
		return nil
	}
	batch, ok := c.store.GetBatch(run.BatchID)
	if !ok || !batch.SnapshotIntent {
		return nil
	}

	input, err := c.projector.LoadCompletedRun(runID)
	if err != nil {
		return err
	}
	if len(input.MatchingCohorts) == 0 {
		return nil
	}
	if err := c.projector.ProjectLoaded(input); err != nil {
		for _, cohort := range input.MatchingCohorts {
			_ = c.setCohortMaterialization(cohort, serverpkg.AnalysisMaterializationFailed, time.Time{}, err.Error())
		}
		return err
	}

	projectedAt := input.Run.FinishedAt
	if projectedAt.IsZero() {
		projectedAt = time.Now().UTC()
	}
	for _, cohort := range input.MatchingCohorts {
		if err := c.accumulateSnapshot(cohort, batch, input.Run); err != nil {
			return err
		}
		if err := c.setCohortMaterialization(cohort, serverpkg.AnalysisMaterializationReady, projectedAt, ""); err != nil {
			return err
		}
	}
	return nil
}

// accumulateSnapshot is the per-run wrapper around applySnapshotState used
// by the realtime ProjectRun path; mixed-profile detection compares one
// run against the snapshot's stored profile.
func (c *Controller) accumulateSnapshot(cohort serverpkg.AnalysisCohort, batch serverpkg.Batch, run serverpkg.Run) error {
	return c.applySnapshotState(cohort, batch, run, false)
}

// applySnapshotState find-or-creates the pending snapshot for one
// (cohort, batch) and refreshes its denormalized counters/timestamps.
// forceMixed lets callers that have observed multiple runs across the
// rebuild loop record the failed_mixed_profiles state without needing
// this helper to see every run individually.
func (c *Controller) applySnapshotState(cohort serverpkg.AnalysisCohort, batch serverpkg.Batch, sampleRun serverpkg.Run, forceMixed bool) error {
	existing, found := c.store.GetAnalysisCohortSnapshotByBatch(cohort.ID, batch.ID)
	runCount, domainCount, firstRunAt, lastRunAt, err := c.store.CountBatchSnapshotRuns(cohort.ID, batch.ID)
	if err != nil {
		return fmt.Errorf("count snapshot runs for cohort %d batch %s: %w", cohort.ID, batch.ID, err)
	}

	snap := serverpkg.AnalysisCohortSnapshot{
		CohortID:    cohort.ID,
		BatchID:     batch.ID,
		RunCount:    runCount,
		DomainCount: domainCount,
		FirstRunAt:  firstRunAt,
		LastRunAt:   lastRunAt,
	}
	if found {
		snap.ID = existing.ID
		snap.Slug = existing.Slug
		snap.Label = existing.Label
		snap.Description = existing.Description
		snap.ProfileID = existing.ProfileID
		snap.ProfileName = existing.ProfileName
		snap.CapturedAt = existing.CapturedAt
		snap.Status = existing.Status
		snap.IsDefault = existing.IsDefault
		snap.IsPublic = existing.IsPublic
		snap.CreatedAt = existing.CreatedAt
	} else {
		snap.Slug = defaultSnapshotSlug(batch)
		snap.Status = serverpkg.AnalysisSnapshotStatusPending
		snap.IsPublic = true
		snap.ProfileID = cloneInt64Ptr(sampleRun.ProfileID)
		snap.ProfileName = sampleRun.ProfileName
	}

	if snap.Status != serverpkg.AnalysisSnapshotStatusFailedMixedProfiles {
		if forceMixed || mixedProfiles(snap.ProfileID, snap.ProfileName, sampleRun.ProfileID, sampleRun.ProfileName) {
			snap.Status = serverpkg.AnalysisSnapshotStatusFailedMixedProfiles
			snap.IsPublic = false
		}
	}

	if _, err := c.store.UpsertAnalysisCohortSnapshot(snap); err != nil {
		return fmt.Errorf("upsert snapshot for cohort %d batch %s: %w", cohort.ID, batch.ID, err)
	}
	if found && existing.Status == serverpkg.AnalysisSnapshotStatusCaptured {
		if err := c.materializeSnapshot(snap); err != nil {
			return fmt.Errorf("refresh captured snapshot %d: %w", snap.ID, err)
		}
	}
	return nil
}

// materializeSnapshot writes the per-snapshot overview row and the
// entity-view rows in sequence.
func (c *Controller) materializeSnapshot(snap serverpkg.AnalysisCohortSnapshot) error {
	overview, err := c.store.ComputeSnapshotOverview(snap.CohortID, snap.BatchID)
	if err != nil {
		return fmt.Errorf("compute overview: %w", err)
	}
	if err := c.store.ReplaceSnapshotOverview(snap.ID, overview); err != nil {
		return fmt.Errorf("write overview: %w", err)
	}
	views, err := c.store.ComputeSnapshotEntityViews(snap.CohortID, snap.BatchID)
	if err != nil {
		return fmt.Errorf("compute entity views: %w", err)
	}
	if err := c.store.ReplaceSnapshotEntityViews(snap.ID, views); err != nil {
		return fmt.Errorf("write entity views: %w", err)
	}
	return nil
}

// defaultSnapshotSlug renders a date-first slug with a stable hash suffix
// derived from the full batch ID. Generated batch IDs share a long prefix, so
// truncating the raw ID is not enough to keep same-day snapshots distinct.
func defaultSnapshotSlug(batch serverpkg.Batch) string {
	date := batch.CreatedAt.UTC().Format("2006-01-02")
	batchID := strings.TrimSpace(batch.ID)
	if batchID == "" {
		return date
	}
	sum := sha1.Sum([]byte(batchID))
	return date + "-" + hex.EncodeToString(sum[:])[:12]
}

// mixedProfiles returns true when the snapshot's denormalized profile
// disagrees with the run's profile. ID comparison wins when both are set;
// otherwise the name comparison catches the common case where the same
// profile is present under different IDs (e.g. after a profile rename).
func mixedProfiles(snapID *int64, snapName string, runID *int64, runName string) bool {
	if snapID != nil && runID != nil {
		return *snapID != *runID
	}
	snap := strings.TrimSpace(snapName)
	run := strings.TrimSpace(runName)
	if snap != "" && run != "" {
		return snap != run
	}
	return false
}

func cloneInt64Ptr(v *int64) *int64 {
	if v == nil {
		return nil
	}
	copy := *v
	return &copy
}

// RepairAllCohorts reconciles the materialized analysis state for every cohort
// in the catalog. Disabled cohorts are cleared. Enabled cohorts are either
// left untouched (already Ready and up to date), caught up incrementally
// (Ready with only newer runs to project), or fully rebuilt (any other
// state). Incremental catch-up avoids the clear-and-reproject blast radius
// at startup when all that changed is a handful of runs completing while
// the server was down.
func (c *Controller) RepairAllCohorts(ctx context.Context) error {
	for _, cohort := range c.store.ListAnalysisCohorts() {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !cohort.AnalysisEnabled {
			if err := c.ClearCohort(ctx, cohort.ID); err != nil {
				return err
			}
			continue
		}
		if err := c.repairEnabledCohort(ctx, cohort); err != nil {
			return err
		}
	}
	return nil
}

// repairEnabledCohort brings one enabled cohort back to a Ready state using
// the cheapest path that still guarantees correctness. A cohort that is not
// already Ready (Pending/Failed) falls back to a full rebuild; a Ready
// cohort with any runs finished after its stamp is caught up by projecting
// only those runs.
func (c *Controller) repairEnabledCohort(ctx context.Context, cohort serverpkg.AnalysisCohort) error {
	if cohort.MaterializationStatus != serverpkg.AnalysisMaterializationReady {
		return c.RebuildCohort(ctx, cohort.ID)
	}
	return c.catchUpCohort(ctx, cohort)
}

// catchUpCohort projects runs whose FinishedAt is strictly after the
// cohort's LastMaterializedAt stamp, without clearing existing materialized
// rows. Returns immediately when no such runs exist — the common case after
// a clean restart.
func (c *Controller) catchUpCohort(ctx context.Context, cohort serverpkg.AnalysisCohort) error {
	latest := cohort.LastMaterializedAt
	projected := 0
	catalog := c.store.ListAnalysisCohorts()
	progress := newProgressTracker(c.store, cohort.ID)
	for offset := 0; ; {
		if err := ctx.Err(); err != nil {
			return err
		}
		list := c.store.ListRuns(serverpkg.RunFilter{
			Tag:           cohort.SourceTag,
			FinishedAfter: cohort.LastMaterializedAt,
			Limit:         rebuildPageSize,
			Offset:        offset,
		})
		if len(list.Items) == 0 {
			break
		}
		if offset == 0 {
			progress.setTotal(list.Total)
		}
		for _, run := range list.Items {
			if err := ctx.Err(); err != nil {
				return err
			}
			projectedThisRun, err := c.projectAndAccumulate(run.ID, catalog)
			if err != nil {
				_ = c.setCohortMaterialization(cohort, serverpkg.AnalysisMaterializationFailed, cohort.LastMaterializedAt, err.Error())
				return fmt.Errorf("project run %s for cohort %d: %w", run.ID, cohort.ID, err)
			}
			if projectedThisRun {
				if run.FinishedAt.After(latest) {
					latest = run.FinishedAt
				}
				projected++
			}
			progress.increment()
		}
		offset += len(list.Items)
		if offset >= list.Total {
			break
		}
	}
	progress.flush()
	if projected == 0 {
		return nil
	}
	return c.setCohortMaterialization(cohort, serverpkg.AnalysisMaterializationReady, latest, "")
}

// RebuildCohort clears and backfills one cohort from all existing runs whose
// domains currently belong to the cohort's source tag.
func (c *Controller) RebuildCohort(ctx context.Context, cohortID int64) error {
	cohort, err := c.lookupCohort(cohortID)
	if err != nil {
		return err
	}
	if !cohort.AnalysisEnabled {
		return c.ClearCohort(ctx, cohortID)
	}

	if err := c.setCohortMaterialization(cohort, serverpkg.AnalysisMaterializationPending, time.Time{}, ""); err != nil {
		return err
	}
	if err := c.store.ClearAnalysisCohortMaterialization(cohort.ID); err != nil {
		_ = c.setCohortMaterialization(cohort, serverpkg.AnalysisMaterializationFailed, time.Time{}, err.Error())
		return fmt.Errorf("clear cohort %d before rebuild: %w", cohort.ID, err)
	}
	// Snapshots are recreated by the post-loop reconciliation pass.
	if err := c.store.ClearAnalysisCohortSnapshots(cohort.ID); err != nil {
		_ = c.setCohortMaterialization(cohort, serverpkg.AnalysisMaterializationFailed, time.Time{}, err.Error())
		return fmt.Errorf("clear cohort %d snapshots before rebuild: %w", cohort.ID, err)
	}

	projected := 0
	catalog := c.store.ListAnalysisCohorts()
	progress := newProgressTracker(c.store, cohort.ID)
	_ = c.store.SetAnalysisCohortProgress(cohort.ID, 0, 0)

	// Defer per-(cohort, batch) snapshot accumulation: the per-run path
	// runs CountBatchSnapshotRuns + upsert for every run, and the count
	// scales with snapshot size. One pass at the end issues exactly one
	// count + upsert per snapshot regardless of contributing run count.
	type pendingKey struct {
		cohortID int64
		batchID  string
	}
	type pendingSnap struct {
		cohort serverpkg.AnalysisCohort
		batch  serverpkg.Batch
		sample serverpkg.Run
		mixed  bool
	}
	pending := map[pendingKey]*pendingSnap{}

	for offset := 0; ; {
		if err := ctx.Err(); err != nil {
			return err
		}
		list := c.store.ListRuns(serverpkg.RunFilter{
			Tag:    cohort.SourceTag,
			Limit:  rebuildPageSize,
			Offset: offset,
		})
		if len(list.Items) == 0 {
			break
		}
		if offset == 0 {
			progress.setTotal(list.Total)
		}
		for _, run := range list.Items {
			if err := ctx.Err(); err != nil {
				return err
			}
			input, batch, contributed, err := c.projectRunForRebuild(run, catalog)
			if err != nil {
				_ = c.setCohortMaterialization(cohort, serverpkg.AnalysisMaterializationFailed, time.Time{}, err.Error())
				return fmt.Errorf("project run %s for cohort %d: %w", run.ID, cohort.ID, err)
			}
			if contributed {
				for _, mc := range input.MatchingCohorts {
					key := pendingKey{cohortID: mc.ID, batchID: batch.ID}
					if ps, ok := pending[key]; ok {
						if !ps.mixed && mixedProfiles(ps.sample.ProfileID, ps.sample.ProfileName, run.ProfileID, run.ProfileName) {
							ps.mixed = true
						}
					} else {
						pending[key] = &pendingSnap{cohort: mc, batch: batch, sample: run}
					}
				}
				projected++
			}
			progress.increment()
		}
		offset += len(list.Items)
		if offset >= list.Total {
			break
		}
	}
	progress.flush()

	for _, ps := range pending {
		if err := c.applySnapshotState(ps.cohort, ps.batch, ps.sample, ps.mixed); err != nil {
			_ = c.setCohortMaterialization(cohort, serverpkg.AnalysisMaterializationFailed, time.Time{}, err.Error())
			return fmt.Errorf("finalize snapshot for cohort %d batch %s: %w", ps.cohort.ID, ps.batch.ID, err)
		}
	}

	// Stamp last_materialized_at with the rebuild time — clicking
	// Rebuild without a moving timestamp would surprise users.
	var completedAt time.Time
	if projected > 0 {
		completedAt = time.Now().UTC()
	}
	return c.setCohortMaterialization(cohort, serverpkg.AnalysisMaterializationReady, completedAt, "")
}

// projectRunForRebuild applies the same snapshot-intent gates as the
// realtime path and projects one run, deferring the per-(cohort, batch)
// snapshot upsert to the reconciliation pass.
func (c *Controller) projectRunForRebuild(run serverpkg.Run, catalog []serverpkg.AnalysisCohort) (RunInput, serverpkg.Batch, bool, error) {
	if run.BatchID == "" {
		return RunInput{}, serverpkg.Batch{}, false, nil
	}
	batch, ok := c.store.GetBatch(run.BatchID)
	if !ok || !batch.SnapshotIntent {
		return RunInput{}, serverpkg.Batch{}, false, nil
	}
	input, err := c.projector.LoadCompletedRunWithCatalog(run.ID, catalog)
	if err != nil {
		return RunInput{}, serverpkg.Batch{}, false, fmt.Errorf("load run %s: %w", run.ID, err)
	}
	if len(input.MatchingCohorts) == 0 {
		return RunInput{}, serverpkg.Batch{}, false, nil
	}
	if err := c.projector.ProjectLoaded(input); err != nil {
		return RunInput{}, serverpkg.Batch{}, false, fmt.Errorf("project run %s: %w", run.ID, err)
	}
	return input, batch, true, nil
}

// ClearCohort removes all materialized rows for one cohort and leaves it in a
// non-materialized pending state.
func (c *Controller) ClearCohort(ctx context.Context, cohortID int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	cohort, err := c.lookupCohort(cohortID)
	if err != nil {
		return err
	}
	if err := c.store.ClearAnalysisCohortMaterialization(cohort.ID); err != nil {
		_ = c.setCohortMaterialization(cohort, serverpkg.AnalysisMaterializationFailed, time.Time{}, err.Error())
		return fmt.Errorf("clear cohort %d: %w", cohort.ID, err)
	}
	if err := c.store.ClearAnalysisCohortSnapshots(cohort.ID); err != nil {
		_ = c.setCohortMaterialization(cohort, serverpkg.AnalysisMaterializationFailed, time.Time{}, err.Error())
		return fmt.Errorf("clear cohort %d snapshots: %w", cohort.ID, err)
	}
	return c.setCohortMaterialization(cohort, serverpkg.AnalysisMaterializationPending, time.Time{}, "")
}

// projectAndAccumulate is the rebuild-friendly counterpart of ProjectRun:
// it loads one run against a pre-fetched catalog, applies the two
// pollution gates, projects, and accumulates pending snapshots for every
// matching cohort. Returns (true, nil) when the run actually contributed
// facts; (false, nil) when a gate skipped it or no cohort matched.
func (c *Controller) projectAndAccumulate(runID string, catalog []serverpkg.AnalysisCohort) (bool, error) {
	run, ok := c.store.GetRun(runID)
	if !ok {
		return false, fmt.Errorf("%w: %s", ErrRunNotFound, runID)
	}
	if run.BatchID == "" {
		return false, nil
	}
	batch, ok := c.store.GetBatch(run.BatchID)
	if !ok || !batch.SnapshotIntent {
		return false, nil
	}
	input, err := c.projector.LoadCompletedRunWithCatalog(runID, catalog)
	if err != nil {
		return false, fmt.Errorf("load run %s: %w", runID, err)
	}
	if len(input.MatchingCohorts) == 0 {
		return false, nil
	}
	if err := c.projector.ProjectLoaded(input); err != nil {
		return false, fmt.Errorf("project run %s: %w", runID, err)
	}
	for _, cohort := range input.MatchingCohorts {
		if err := c.accumulateSnapshot(cohort, batch, input.Run); err != nil {
			return false, err
		}
	}
	return true, nil
}

// CaptureCompletedSnapshots walks the pending-snapshot table and promotes
// any snapshot whose batch has zero outstanding jobs. Promotion computes
// the aggregate payloads, writes them, and flips the row to captured so
// the read path can serve it. Safe to call concurrently with graduation:
// capture waits for both the job queue and the analysis projection state
// for the snapshot's cohort and batch.
func (c *Controller) CaptureCompletedSnapshots(ctx context.Context) error {
	for _, snap := range c.store.ListPendingAnalysisCohortSnapshots() {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := c.captureSnapshot(snap); err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) captureSnapshot(snap serverpkg.AnalysisCohortSnapshot) error {
	outstanding, err := c.store.CountOutstandingJobsForBatch(snap.BatchID)
	if err != nil {
		return fmt.Errorf("count outstanding jobs for batch %s: %w", snap.BatchID, err)
	}
	if outstanding > 0 {
		return nil
	}
	unprojected, err := c.store.CountUnprojectedSnapshotRuns(snap.CohortID, snap.BatchID)
	if err != nil {
		return fmt.Errorf("count unprojected runs for snapshot %d: %w", snap.ID, err)
	}
	if unprojected > 0 {
		return nil
	}

	// Failed snapshots stay pending-free too, but we don't want to emit
	// aggregates for a known-broken mix of profiles. Flip to captured only
	// when the projection sequence did not trip the mixed-profile error.
	if snap.Status == serverpkg.AnalysisSnapshotStatusFailedMixedProfiles {
		return nil
	}

	runCount, domainCount, firstRunAt, lastRunAt, err := c.store.CountBatchSnapshotRuns(snap.CohortID, snap.BatchID)
	if err != nil {
		return fmt.Errorf("count snapshot runs: %w", err)
	}
	if runCount == 0 {
		// Batch terminated with zero graduated runs for this cohort — nothing
		// to materialize. Leave the snapshot pending so a later rebuild can
		// retry; hiding it would drop a real operational condition the
		// admin should see.
		return nil
	}

	if err := c.materializeSnapshot(snap); err != nil {
		return fmt.Errorf("materialize snapshot %d: %w", snap.ID, err)
	}

	snap.RunCount = runCount
	snap.DomainCount = domainCount
	snap.FirstRunAt = firstRunAt
	snap.LastRunAt = lastRunAt
	snap.CapturedAt = time.Now().UTC()
	snap.Status = serverpkg.AnalysisSnapshotStatusCaptured
	if _, err := c.store.UpsertAnalysisCohortSnapshot(snap); err != nil {
		return fmt.Errorf("promote snapshot %d to captured: %w", snap.ID, err)
	}
	return nil
}

// snapshotCaptureInterval is how often the polling goroutine wakes up.
// Every 30 seconds is plenty for operator-scale cohorts; the loop is a
// pure read of the pending snapshot list plus one COUNT(*) per pending
// row, so it is cheap even on larger installations.
const snapshotCaptureInterval = 30 * time.Second

// StartSnapshotCaptureLoop launches a background goroutine that calls
// CaptureCompletedSnapshots on a fixed interval until ctx is done. Safe
// to call once at server startup; the returned channel is closed when
// the loop exits so tests can wait for a clean shutdown.
func (c *Controller) StartSnapshotCaptureLoop(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(snapshotCaptureInterval)
		defer ticker.Stop()
		// Tick once immediately so a server restart promotes batches that
		// finished while the process was down, without waiting a full
		// interval.
		if err := c.CaptureCompletedSnapshots(ctx); err != nil && ctx.Err() == nil {
			logSnapshotCaptureError(err)
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := c.CaptureCompletedSnapshots(ctx); err != nil && ctx.Err() == nil {
					logSnapshotCaptureError(err)
				}
			}
		}
	}()
	return done
}

// logSnapshotCaptureError keeps the capture loop's error-reporting behind
// a single hook so tests can substitute without pulling in the log pkg.
var logSnapshotCaptureError = func(err error) {
	// Default implementation intentionally no-op; production callers
	// install a logging variant via ReplaceSnapshotCaptureErrorLogger.
}

// ReplaceSnapshotCaptureErrorLogger installs a custom error sink for the
// capture loop. The server startup wiring uses this to forward errors to
// the server's structured logger; tests can use it to assert on failures.
func ReplaceSnapshotCaptureErrorLogger(fn func(err error)) {
	if fn == nil {
		logSnapshotCaptureError = func(error) {}
		return
	}
	logSnapshotCaptureError = fn
}

// ReconcileCohortChange applies the materialization side effect for one cohort
// catalog transition.
func (c *Controller) ReconcileCohortChange(ctx context.Context, before, after serverpkg.AnalysisCohort) error {
	switch {
	case before.AnalysisEnabled && !after.AnalysisEnabled:
		return c.ClearCohort(ctx, after.ID)
	case !before.AnalysisEnabled && after.AnalysisEnabled:
		return c.RebuildCohort(ctx, after.ID)
	default:
		return nil
	}
}

func (c *Controller) lookupCohort(cohortID int64) (serverpkg.AnalysisCohort, error) {
	cohort, ok := c.store.GetAnalysisCohort(cohortID)
	if !ok {
		return serverpkg.AnalysisCohort{}, fmt.Errorf("%w: %d", serverpkg.ErrAnalysisCohortNotFound, cohortID)
	}
	return cohort, nil
}

// progressTracker buffers materialization_done / materialization_total
// counters in memory and flushes them to the cohort row at a bounded
// rate. Writing per-run would add one UPDATE round-trip to every
// projection; once per ~500ms is plenty for the UI poll cadence.
type progressTracker struct {
	store        ControlStore
	cohortID     int64
	total        int
	done         int
	persistedAt  time.Time
	persistedDue bool
}

func newProgressTracker(store ControlStore, cohortID int64) *progressTracker {
	return &progressTracker{store: store, cohortID: cohortID}
}

func (p *progressTracker) setTotal(total int) {
	if p == nil {
		return
	}
	p.total = total
	p.persistedDue = true
	p.maybePersist()
}

func (p *progressTracker) increment() {
	if p == nil {
		return
	}
	p.done++
	p.persistedDue = true
	p.maybePersist()
}

func (p *progressTracker) maybePersist() {
	if !p.persistedDue {
		return
	}
	now := time.Now()
	if !p.persistedAt.IsZero() && now.Sub(p.persistedAt) < progressWriteInterval {
		return
	}
	if err := p.store.SetAnalysisCohortProgress(p.cohortID, p.done, p.total); err == nil {
		p.persistedAt = now
		p.persistedDue = false
	}
}

// flush forces a final write so the terminal counters land in the cohort
// row regardless of the last throttle window.
func (p *progressTracker) flush() {
	if p == nil || !p.persistedDue {
		return
	}
	if err := p.store.SetAnalysisCohortProgress(p.cohortID, p.done, p.total); err == nil {
		p.persistedAt = time.Now()
		p.persistedDue = false
	}
}

func (c *Controller) setCohortMaterialization(cohort serverpkg.AnalysisCohort, status string, at time.Time, materializationErr string) error {
	cohort.MaterializationStatus = status
	cohort.LastMaterializedAt = at
	cohort.LastMaterializationError = materializationErr
	_, err := c.store.UpsertAnalysisCohort(cohort)
	if err != nil {
		return fmt.Errorf("update cohort %d materialization state: %w", cohort.ID, err)
	}
	return nil
}
