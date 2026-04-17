package server

import (
	"errors"
	"testing"
	"time"
)

func TestNormalizeAnalysisScope(t *testing.T) {
	t.Run("defaults to latest_global", func(t *testing.T) {
		scope, err := NormalizeAnalysisScope(AnalysisScope{})
		if err != nil {
			t.Fatalf("NormalizeAnalysisScope: %v", err)
		}
		if scope.Mode != AnalysisScopeLatestGlobal {
			t.Fatalf("Mode: got %q want %q", scope.Mode, AnalysisScopeLatestGlobal)
		}
	})

	t.Run("latest_global rejects batch and time filters", func(t *testing.T) {
		_, err := NormalizeAnalysisScope(AnalysisScope{
			Mode:    AnalysisScopeLatestGlobal,
			BatchID: "batch-1",
		})
		if !errors.Is(err, ErrInvalidAnalysisScope) {
			t.Fatalf("expected ErrInvalidAnalysisScope, got %v", err)
		}
	})

	t.Run("latest_in_batch requires batch_id", func(t *testing.T) {
		_, err := NormalizeAnalysisScope(AnalysisScope{Mode: AnalysisScopeLatestInBatch})
		if !errors.Is(err, ErrInvalidAnalysisScope) {
			t.Fatalf("expected ErrInvalidAnalysisScope, got %v", err)
		}
		scope, err := NormalizeAnalysisScope(AnalysisScope{
			Mode:    AnalysisScopeLatestInBatch,
			BatchID: "batch-1",
		})
		if err != nil {
			t.Fatalf("NormalizeAnalysisScope: %v", err)
		}
		if scope.BatchID != "batch-1" {
			t.Fatalf("BatchID: got %q", scope.BatchID)
		}
	})

	t.Run("batch requires batch_id", func(t *testing.T) {
		_, err := NormalizeAnalysisScope(AnalysisScope{Mode: AnalysisScopeBatch})
		if !errors.Is(err, ErrInvalidAnalysisScope) {
			t.Fatalf("expected ErrInvalidAnalysisScope, got %v", err)
		}
	})

	t.Run("time_window normalizes UTC and validates range", func(t *testing.T) {
		from := time.Date(2026, 4, 18, 10, 0, 0, 0, time.FixedZone("UTC+2", 2*3600))
		to := time.Date(2026, 4, 18, 11, 0, 0, 0, time.FixedZone("UTC+2", 2*3600))
		scope, err := NormalizeAnalysisScope(AnalysisScope{
			Mode: AnalysisScopeTimeWindow,
			From: from,
			To:   to,
		})
		if err != nil {
			t.Fatalf("NormalizeAnalysisScope: %v", err)
		}
		if scope.From.Location() != time.UTC || scope.To.Location() != time.UTC {
			t.Fatalf("expected UTC-normalized bounds: %+v", scope)
		}

		_, err = NormalizeAnalysisScope(AnalysisScope{
			Mode: AnalysisScopeTimeWindow,
			From: to.UTC(),
			To:   from.UTC(),
		})
		if !errors.Is(err, ErrInvalidAnalysisScope) {
			t.Fatalf("expected ErrInvalidAnalysisScope, got %v", err)
		}
	})
}

func TestSelectableAnalysisCohorts(t *testing.T) {
	cohorts := []AnalysisCohort{
		{
			ID:              1,
			SourceType:      "tag",
			SourceTag:       "hidden",
			Label:           "Hidden",
			AnalysisEnabled: true,
			PublicEnabled:   false,
			SortOrder:       50,
		},
		{
			ID:              2,
			SourceType:      "tag",
			SourceTag:       "public-b",
			Label:           "B",
			AnalysisEnabled: true,
			PublicEnabled:   true,
			SortOrder:       20,
		},
		{
			ID:              3,
			SourceType:      "tag",
			SourceTag:       "public-a",
			Label:           "A",
			AnalysisEnabled: true,
			PublicEnabled:   true,
			SortOrder:       10,
		},
	}
	selectable, err := SelectableAnalysisCohorts(cohorts)
	if err != nil {
		t.Fatalf("SelectableAnalysisCohorts: %v", err)
	}
	if len(selectable) != 2 {
		t.Fatalf("expected 2 selectable cohorts, got %d", len(selectable))
	}
	if selectable[0].SourceTag != "public-a" || selectable[1].SourceTag != "public-b" {
		t.Fatalf("unexpected selectable order: %+v", selectable)
	}

	_, err = SelectableAnalysisCohorts([]AnalysisCohort{{
		SourceTag:       "broken",
		AnalysisEnabled: false,
		PublicEnabled:   true,
	}})
	if !errors.Is(err, ErrInvalidAnalysisCohortCatalog) {
		t.Fatalf("expected ErrInvalidAnalysisCohortCatalog, got %v", err)
	}
}

func TestResolveAnalysisCohort(t *testing.T) {
	cohorts := []AnalysisCohort{
		{
			ID:              1,
			SourceType:      "tag",
			SourceTag:       "tld",
			Label:           "TLD",
			AnalysisEnabled: true,
			PublicEnabled:   true,
			IsDefault:       true,
			SortOrder:       10,
		},
		{
			ID:              2,
			SourceType:      "tag",
			SourceTag:       "gov",
			Label:           "Government",
			AnalysisEnabled: true,
			PublicEnabled:   true,
			SortOrder:       20,
		},
	}

	t.Run("missing dataset_tag resolves default public cohort", func(t *testing.T) {
		cohort, err := ResolveAnalysisCohort(cohorts, "", "")
		if err != nil {
			t.Fatalf("ResolveAnalysisCohort: %v", err)
		}
		if cohort.SourceTag != "tld" {
			t.Fatalf("SourceTag: got %q want tld", cohort.SourceTag)
		}
	})

	t.Run("dataset_tag resolves explicit public cohort", func(t *testing.T) {
		cohort, err := ResolveAnalysisCohort(cohorts, "gov", "")
		if err != nil {
			t.Fatalf("ResolveAnalysisCohort: %v", err)
		}
		if cohort.SourceTag != "gov" {
			t.Fatalf("SourceTag: got %q want gov", cohort.SourceTag)
		}
	})

	t.Run("missing default can fall back to bootstrap tag", func(t *testing.T) {
		noDefault := append([]AnalysisCohort(nil), cohorts...)
		noDefault[0].IsDefault = false
		cohort, err := ResolveAnalysisCohort(noDefault, "", "gov")
		if err != nil {
			t.Fatalf("ResolveAnalysisCohort: %v", err)
		}
		if cohort.SourceTag != "gov" {
			t.Fatalf("SourceTag: got %q want gov", cohort.SourceTag)
		}
	})

	t.Run("missing default without bootstrap fails", func(t *testing.T) {
		noDefault := append([]AnalysisCohort(nil), cohorts...)
		noDefault[0].IsDefault = false
		_, err := ResolveAnalysisCohort(noDefault, "", "")
		if !errors.Is(err, ErrNoDefaultAnalysisCohort) {
			t.Fatalf("expected ErrNoDefaultAnalysisCohort, got %v", err)
		}
	})

	t.Run("invalid requested dataset_tag fails", func(t *testing.T) {
		_, err := ResolveAnalysisCohort(cohorts, "private", "")
		if !errors.Is(err, ErrAnalysisCohortNotFound) {
			t.Fatalf("expected ErrAnalysisCohortNotFound, got %v", err)
		}
	})

	t.Run("multiple defaults fail", func(t *testing.T) {
		multiDefault := append([]AnalysisCohort(nil), cohorts...)
		multiDefault[1].IsDefault = true
		_, err := ResolveAnalysisCohort(multiDefault, "", "")
		if !errors.Is(err, ErrMultipleDefaultAnalysisCohorts) {
			t.Fatalf("expected ErrMultipleDefaultAnalysisCohorts, got %v", err)
		}
	})
}
