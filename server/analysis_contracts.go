package server

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

const AnalysisDefaultTagSettingKey = "analysis_default_tag"

type AnalysisScopeMode string

const (
	AnalysisScopeLatestGlobal  AnalysisScopeMode = "latest_global"
	AnalysisScopeLatestInBatch AnalysisScopeMode = "latest_in_batch"
	AnalysisScopeBatch         AnalysisScopeMode = "batch"
	AnalysisScopeTimeWindow    AnalysisScopeMode = "time_window"
)

var (
	ErrInvalidAnalysisScope           = errors.New("invalid analysis scope")
	ErrNoSelectableAnalysisCohorts    = errors.New("no selectable analysis cohorts")
	ErrMultipleDefaultAnalysisCohorts = errors.New("multiple default public analysis cohorts")
	ErrInvalidAnalysisCohortCatalog   = errors.New("invalid analysis cohort catalog")
	ErrAnalysisCohortNotFound         = errors.New("analysis cohort not found")
	ErrNoDefaultAnalysisCohort        = errors.New("default public analysis cohort not configured")
)

// AnalysisScope describes how analysis queries should select runs before
// grouping/filtering at the dashboard layer.
type AnalysisScope struct {
	Mode    AnalysisScopeMode `json:"mode"`
	BatchID string            `json:"batch_id,omitempty"`
	From    time.Time         `json:"from"`
	To      time.Time         `json:"to"`
}

// NormalizeAnalysisScope validates and normalizes scope semantics.
//
// Semantics:
//   - zero-value mode resolves to latest_global
//   - latest_global selects the latest run per domain within the active cohort
//   - latest_in_batch selects the latest run per domain within the given batch
//   - batch selects all runs within the given batch
//   - time_window selects all runs within the given finished_at window
func NormalizeAnalysisScope(scope AnalysisScope) (AnalysisScope, error) {
	if scope.Mode == "" {
		scope.Mode = AnalysisScopeLatestGlobal
	}
	scope.From = scope.From.UTC()
	scope.To = scope.To.UTC()

	switch scope.Mode {
	case AnalysisScopeLatestGlobal:
		if scope.BatchID != "" || !scope.From.IsZero() || !scope.To.IsZero() {
			return AnalysisScope{}, fmt.Errorf("%w: latest_global does not allow batch_id or time bounds", ErrInvalidAnalysisScope)
		}
	case AnalysisScopeLatestInBatch, AnalysisScopeBatch:
		if scope.BatchID == "" {
			return AnalysisScope{}, fmt.Errorf("%w: %s requires batch_id", ErrInvalidAnalysisScope, scope.Mode)
		}
		if !scope.From.IsZero() || !scope.To.IsZero() {
			return AnalysisScope{}, fmt.Errorf("%w: %s does not allow time bounds", ErrInvalidAnalysisScope, scope.Mode)
		}
	case AnalysisScopeTimeWindow:
		if scope.BatchID != "" {
			return AnalysisScope{}, fmt.Errorf("%w: time_window does not allow batch_id", ErrInvalidAnalysisScope)
		}
		if scope.From.IsZero() && scope.To.IsZero() {
			return AnalysisScope{}, fmt.Errorf("%w: time_window requires at least one bound", ErrInvalidAnalysisScope)
		}
		if !scope.From.IsZero() && !scope.To.IsZero() && scope.To.Before(scope.From) {
			return AnalysisScope{}, fmt.Errorf("%w: time_window upper bound must not be before lower bound", ErrInvalidAnalysisScope)
		}
	default:
		return AnalysisScope{}, fmt.Errorf("%w: unknown scope mode %q", ErrInvalidAnalysisScope, scope.Mode)
	}

	return scope, nil
}

// SelectableAnalysisCohorts returns the public, analysis-enabled cohorts that
// are safe to expose through the public dashboard.
func SelectableAnalysisCohorts(cohorts []AnalysisCohort) ([]AnalysisCohort, error) {
	selectable := make([]AnalysisCohort, 0, len(cohorts))
	for _, cohort := range cohorts {
		if cohort.PublicEnabled && !cohort.AnalysisEnabled {
			return nil, fmt.Errorf("%w: public cohort %q is not analysis-enabled", ErrInvalidAnalysisCohortCatalog, cohort.SourceTag)
		}
		if cohort.PublicEnabled && cohort.AnalysisEnabled {
			selectable = append(selectable, cohort)
		}
	}
	if len(selectable) == 0 {
		return nil, ErrNoSelectableAnalysisCohorts
	}
	sort.Slice(selectable, func(i, j int) bool {
		if selectable[i].SortOrder != selectable[j].SortOrder {
			return selectable[i].SortOrder < selectable[j].SortOrder
		}
		if selectable[i].Label != selectable[j].Label {
			return selectable[i].Label < selectable[j].Label
		}
		return selectable[i].SourceTag < selectable[j].SourceTag
	})
	return selectable, nil
}

// ResolveAnalysisCohort resolves the active public cohort from the requested
// dataset_tag plus the admin-managed cohort catalog.
//
// Resolution rules:
//   - only public_enabled + analysis_enabled cohorts are selectable
//   - when dataset_tag is supplied, it must match a selectable cohort's source_tag
//   - when dataset_tag is omitted, exactly one default public cohort is required
//   - bootstrapDefaultTag is only consulted when no catalog default exists
func ResolveAnalysisCohort(cohorts []AnalysisCohort, datasetTag, bootstrapDefaultTag string) (AnalysisCohort, error) {
	selectable, err := SelectableAnalysisCohorts(cohorts)
	if err != nil {
		return AnalysisCohort{}, err
	}

	if datasetTag != "" {
		for _, cohort := range selectable {
			if cohort.SourceTag == datasetTag {
				return cohort, nil
			}
		}
		return AnalysisCohort{}, fmt.Errorf("%w: requested dataset_tag %q is not public", ErrAnalysisCohortNotFound, datasetTag)
	}

	var defaults []AnalysisCohort
	for _, cohort := range selectable {
		if cohort.IsDefault {
			defaults = append(defaults, cohort)
		}
	}
	switch len(defaults) {
	case 1:
		return defaults[0], nil
	case 0:
		if bootstrapDefaultTag != "" {
			for _, cohort := range selectable {
				if cohort.SourceTag == bootstrapDefaultTag {
					return cohort, nil
				}
			}
			return AnalysisCohort{}, fmt.Errorf("%w: bootstrap default tag %q does not match a selectable public cohort", ErrNoDefaultAnalysisCohort, bootstrapDefaultTag)
		}
		return AnalysisCohort{}, ErrNoDefaultAnalysisCohort
	default:
		return AnalysisCohort{}, ErrMultipleDefaultAnalysisCohorts
	}
}
