package analysis

import (
	"context"
	"fmt"
	"time"

	serverpkg "codeberg.org/pawal/gonemaster/server"
)

const rebuildPageSize = 100

// ControlStore is the store surface needed for cohort-wide repair, rebuild,
// and clear operations.
type ControlStore interface {
	Store
	GetAnalysisCohort(id int64) (serverpkg.AnalysisCohort, bool)
	UpsertAnalysisCohort(cohort serverpkg.AnalysisCohort) (serverpkg.AnalysisCohort, error)
	ListRuns(filter serverpkg.RunFilter) serverpkg.RunList
	ClearAnalysisCohortMaterialization(cohortID int64) error
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
// in the catalog. Enabled cohorts are rebuilt; disabled cohorts are cleared.
func (c *Controller) RepairAllCohorts(ctx context.Context) error {
	for _, cohort := range c.store.ListAnalysisCohorts() {
		if err := ctx.Err(); err != nil {
			return err
		}
		if cohort.AnalysisEnabled {
			if err := c.RebuildCohort(ctx, cohort.ID); err != nil {
				return err
			}
			continue
		}
		if err := c.ClearCohort(ctx, cohort.ID); err != nil {
			return err
		}
	}
	return nil
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

	var lastMaterializedAt time.Time
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
		for _, run := range list.Items {
			if err := ctx.Err(); err != nil {
				return err
			}
			input, err := c.projector.LoadCompletedRun(run.ID)
			if err != nil {
				_ = c.setCohortMaterialization(cohort, serverpkg.AnalysisMaterializationFailed, time.Time{}, err.Error())
				return fmt.Errorf("load run %s for cohort %d: %w", run.ID, cohort.ID, err)
			}
			if len(input.MatchingCohorts) == 0 {
				continue
			}
			if err := c.projector.ProjectLoaded(input); err != nil {
				_ = c.setCohortMaterialization(cohort, serverpkg.AnalysisMaterializationFailed, time.Time{}, err.Error())
				return fmt.Errorf("project run %s for cohort %d: %w", run.ID, cohort.ID, err)
			}
			if input.Run.FinishedAt.After(lastMaterializedAt) {
				lastMaterializedAt = input.Run.FinishedAt
			}
		}
		offset += len(list.Items)
		if offset >= list.Total {
			break
		}
	}

	return c.setCohortMaterialization(cohort, serverpkg.AnalysisMaterializationReady, lastMaterializedAt, "")
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
