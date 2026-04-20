package analysis

import (
	"context"
	"fmt"
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
// materialization state for the matched cohorts.
func (c *Controller) ProjectRun(runID string) error {
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
		if err := c.setCohortMaterialization(cohort, serverpkg.AnalysisMaterializationReady, projectedAt, ""); err != nil {
			return err
		}
	}
	return nil
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
			input, err := c.projector.LoadCompletedRunWithCatalog(run.ID, catalog)
			if err != nil {
				_ = c.setCohortMaterialization(cohort, serverpkg.AnalysisMaterializationFailed, cohort.LastMaterializedAt, err.Error())
				return fmt.Errorf("load run %s for cohort %d: %w", run.ID, cohort.ID, err)
			}
			if len(input.MatchingCohorts) == 0 {
				progress.increment()
				continue
			}
			if err := c.projector.ProjectLoaded(input); err != nil {
				_ = c.setCohortMaterialization(cohort, serverpkg.AnalysisMaterializationFailed, cohort.LastMaterializedAt, err.Error())
				return fmt.Errorf("project run %s for cohort %d: %w", run.ID, cohort.ID, err)
			}
			if run.FinishedAt.After(latest) {
				latest = run.FinishedAt
			}
			projected++
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

	projected := 0
	catalog := c.store.ListAnalysisCohorts()
	progress := newProgressTracker(c.store, cohort.ID)
	// Reset the persisted counters at the start so a retry after a
	// partial rebuild doesn't show stale done/total numbers.
	_ = c.store.SetAnalysisCohortProgress(cohort.ID, 0, 0)
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
			// Use the first page's Total as the denominator for the UI
			// progress bar. ListRuns re-counts on every call, but we only
			// need the count once per rebuild.
			progress.setTotal(list.Total)
		}
		for _, run := range list.Items {
			if err := ctx.Err(); err != nil {
				return err
			}
			input, err := c.projector.LoadCompletedRunWithCatalog(run.ID, catalog)
			if err != nil {
				_ = c.setCohortMaterialization(cohort, serverpkg.AnalysisMaterializationFailed, time.Time{}, err.Error())
				return fmt.Errorf("load run %s for cohort %d: %w", run.ID, cohort.ID, err)
			}
			if len(input.MatchingCohorts) == 0 {
				progress.increment()
				continue
			}
			if err := c.projector.ProjectLoaded(input); err != nil {
				_ = c.setCohortMaterialization(cohort, serverpkg.AnalysisMaterializationFailed, time.Time{}, err.Error())
				return fmt.Errorf("project run %s for cohort %d: %w", run.ID, cohort.ID, err)
			}
			projected++
			progress.increment()
		}
		offset += len(list.Items)
		if offset >= list.Total {
			break
		}
	}
	progress.flush()

	// Stamp the cohort's last_materialized_at with the time the rebuild
	// actually ran — that's what the "Last analyzed" label shows in the
	// UI. Keeping the latest run's FinishedAt would surprise users who
	// click Rebuild and see the timestamp not move. If no matching runs
	// were projected at all, leave the timestamp zero so the UI can tell
	// the cohort has no data.
	var completedAt time.Time
	if projected > 0 {
		completedAt = time.Now().UTC()
	}
	return c.setCohortMaterialization(cohort, serverpkg.AnalysisMaterializationReady, completedAt, "")
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
	return c.setCohortMaterialization(cohort, serverpkg.AnalysisMaterializationPending, time.Time{}, "")
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
