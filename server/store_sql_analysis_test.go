package server

import (
	"testing"
	"time"
)

func TestSQLJobStoreUpsertAnalysisCohort(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		now := time.Now().UTC().Truncate(time.Microsecond)

		created, err := s.UpsertAnalysisCohort(AnalysisCohort{
			SourceType:            "tag",
			SourceTag:             "tld",
			Label:                 "TLD",
			Description:           "Top-level domains",
			AnalysisEnabled:       true,
			PublicEnabled:         true,
			IsDefault:             true,
			SortOrder:             20,
			MaterializationStatus: AnalysisMaterializationPending,
			CreatedAt:             now,
			UpdatedAt:             now,
		})
		if err != nil {
			t.Fatalf("UpsertAnalysisCohort create: %v", err)
		}
		if created.ID == 0 {
			t.Fatal("expected non-zero ID")
		}
		if !created.AnalysisEnabled || !created.PublicEnabled || !created.IsDefault {
			t.Fatalf("unexpected flags on created cohort: %+v", created)
		}

		got, ok := s.GetAnalysisCohort(created.ID)
		if !ok {
			t.Fatal("expected GetAnalysisCohort to find created row")
		}
		if got.SourceTag != "tld" || got.Label != "TLD" {
			t.Fatalf("unexpected stored cohort: %+v", got)
		}

		readyAt := now.Add(10 * time.Minute)
		updated, err := s.UpsertAnalysisCohort(AnalysisCohort{
			SourceType:            "tag",
			SourceTag:             "tld",
			Label:                 "TLD public",
			Description:           "Updated description",
			AnalysisEnabled:       true,
			PublicEnabled:         false,
			IsDefault:             false,
			SortOrder:             30,
			MaterializationStatus: AnalysisMaterializationReady,
			LastMaterializedAt:    readyAt,
			UpdatedAt:             readyAt,
		})
		if err != nil {
			t.Fatalf("UpsertAnalysisCohort update: %v", err)
		}
		if updated.ID != created.ID {
			t.Fatalf("update changed ID: got %d want %d", updated.ID, created.ID)
		}
		if updated.CreatedAt.IsZero() || !updated.CreatedAt.Equal(created.CreatedAt) {
			t.Fatalf("expected CreatedAt to be preserved: created=%s updated=%s", created.CreatedAt, updated.CreatedAt)
		}
		if updated.PublicEnabled || updated.IsDefault {
			t.Fatalf("expected flags to update, got %+v", updated)
		}
		if updated.MaterializationStatus != AnalysisMaterializationReady {
			t.Fatalf("MaterializationStatus: got %q", updated.MaterializationStatus)
		}
		if !updated.LastMaterializedAt.Equal(readyAt) {
			t.Fatalf("LastMaterializedAt: got %s want %s", updated.LastMaterializedAt, readyAt)
		}

		second, err := s.UpsertAnalysisCohort(AnalysisCohort{
			SourceType:            "tag",
			SourceTag:             "gov",
			Label:                 "Government",
			AnalysisEnabled:       true,
			PublicEnabled:         true,
			SortOrder:             10,
			MaterializationStatus: AnalysisMaterializationPending,
		})
		if err != nil {
			t.Fatalf("UpsertAnalysisCohort second create: %v", err)
		}
		if second.ID == 0 {
			t.Fatal("expected non-zero ID for second cohort")
		}

		list := s.ListAnalysisCohorts()
		if len(list) != 2 {
			t.Fatalf("expected 2 cohorts, got %d", len(list))
		}
		if list[0].SourceTag != "gov" || list[1].SourceTag != "tld" {
			t.Fatalf("unexpected list order: %+v", list)
		}

		bySource, ok := s.GetAnalysisCohortBySource("tag", "tld")
		if !ok {
			t.Fatal("expected GetAnalysisCohortBySource to find tld cohort")
		}
		if bySource.ID != created.ID {
			t.Fatalf("GetAnalysisCohortBySource returned wrong row: got %d want %d", bySource.ID, created.ID)
		}
	})
}

func TestSQLJobStoreAnalysisCohortReferenceList(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		created, err := s.UpsertAnalysisCohort(AnalysisCohort{
			SourceType:      "tag",
			SourceTag:       "tld",
			AnalysisEnabled: true,
			ReferenceList:   AnalysisReferenceListIANATLDs,
		})
		if err != nil {
			t.Fatalf("UpsertAnalysisCohort create: %v", err)
		}
		if created.ReferenceList != AnalysisReferenceListIANATLDs {
			t.Fatalf("ReferenceList = %q, want %q", created.ReferenceList, AnalysisReferenceListIANATLDs)
		}
		got, ok := s.GetAnalysisCohort(created.ID)
		if !ok || got.ReferenceList != AnalysisReferenceListIANATLDs {
			t.Fatalf("stored cohort = %+v, want reference_list %q", got, AnalysisReferenceListIANATLDs)
		}

		cleared, err := s.UpsertAnalysisCohort(AnalysisCohort{
			SourceType:      "tag",
			SourceTag:       "tld",
			AnalysisEnabled: true,
		})
		if err != nil {
			t.Fatalf("UpsertAnalysisCohort update: %v", err)
		}
		if cleared.ReferenceList != "" {
			t.Fatalf("ReferenceList = %q, want cleared", cleared.ReferenceList)
		}
	})
}

func TestSQLJobStoreUpsertAnalysisCohortValidatesSource(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		if _, err := s.UpsertAnalysisCohort(AnalysisCohort{}); err == nil {
			t.Fatal("expected validation error for empty source_type/source_tag")
		}
	})
}
