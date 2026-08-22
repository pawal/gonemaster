package server

import (
	"testing"
	"time"
)

// TestComputeSnapshotOverviewSeverityAndGrade checks that the severity
// and grade distributions are keyed on distinct domains scoped to the
// batch, not all runs in the cohort.
func TestComputeSnapshotOverviewSeverityAndGrade(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		now := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)

		cohort, err := s.UpsertAnalysisCohort(AnalysisCohort{
			SourceType:      "tag",
			SourceTag:       "tld",
			AnalysisEnabled: true,
		})
		if err != nil {
			t.Fatalf("seed cohort: %v", err)
		}
		if err := s.CreateBatch(Batch{
			ID:             "batch-x",
			Tag:            "tld",
			CreatedAt:      now,
			DomainCount:    2,
			SnapshotIntent: true,
		}); err != nil {
			t.Fatalf("CreateBatch: %v", err)
		}

		// Seed two runs in batch-x plus one run in another batch that must not
		// leak into the aggregates.
		for i, rec := range []struct {
			runID    string
			domainID int64
			batchID  string
			grade    string
			worst    string
		}{
			{"run-1", 100, "batch-x", "A", "NOTICE"},
			{"run-2", 101, "batch-x", "B", "WARNING"},
			{"run-3", 102, "batch-other", "F", "CRITICAL"},
		} {
			if _, err := s.GetOrCreateDomain(rec.runID + ".example"); err != nil {
				t.Fatalf("seed domain %d: %v", i, err)
			}
			job := Job{
				ID:         rec.runID,
				DomainID:   rec.domainID,
				Domain:     rec.runID + ".example",
				BatchID:    rec.batchID,
				Status:     JobSucceeded,
				CreatedAt:  now,
				StartedAt:  now,
				FinishedAt: now.Add(time.Duration(i) * time.Minute),
			}
			createAndGraduate(t, s, job, nil)
			gradeCopy := rec.grade
			if err := s.UpsertAnalysisRunDomainSummary(AnalysisRunDomainSummary{
				CohortID:   cohort.ID,
				RunID:      rec.runID,
				DomainID:   rec.domainID,
				Grade:      &gradeCopy,
				WorstLevel: rec.worst,
			}); err != nil {
				t.Fatalf("upsert summary %d: %v", i, err)
			}
			if err := s.ReplaceAnalysisRunDomainFacts(cohort.ID, rec.runID, []AnalysisRunDomainFact{
				{CohortID: cohort.ID, RunID: rec.runID, DomainID: rec.domainID, Category: FactCategorySeverity, Key: rec.worst},
				{CohortID: cohort.ID, RunID: rec.runID, DomainID: rec.domainID, Category: FactCategoryGrade, Key: rec.grade},
			}); err != nil {
				t.Fatalf("replace facts %d: %v", i, err)
			}
		}

		overview, err := s.ComputeSnapshotOverview(cohort.ID, "batch-x")
		if err != nil {
			t.Fatalf("ComputeSnapshotOverview: %v", err)
		}
		severity := bucketsToCounts(overview.FactDistributions[FactCategorySeverity].Buckets)
		if severity["NOTICE"] != 1 || severity["WARNING"] != 1 {
			t.Fatalf("severity counts = %v", severity)
		}
		if _, ok := severity["CRITICAL"]; ok {
			t.Fatalf("severity leaked the out-of-batch CRITICAL row: %v", severity)
		}
		grades := bucketsToCounts(overview.FactDistributions[FactCategoryGrade].Buckets)
		if grades["A"] != 1 || grades["B"] != 1 {
			t.Fatalf("grade counts = %v", grades)
		}
		if _, ok := grades["F"]; ok {
			t.Fatalf("grade leaked the out-of-batch F row: %v", grades)
		}
	})
}

// TestComputeSnapshotOverviewTopTagsExcludesInfoAndNotice pins the
// "Top issues" filter: the overview panel advertises WARNING-and-worse
// only, so INFO / NOTICE / blank-level tag rows must not surface even
// when they dominate by domain count.
func TestComputeSnapshotOverviewTopTagsExcludesInfoAndNotice(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)

		cohort, err := s.UpsertAnalysisCohort(AnalysisCohort{
			SourceType: "tag", SourceTag: "tld", AnalysisEnabled: true,
		})
		if err != nil {
			t.Fatalf("seed cohort: %v", err)
		}
		if err := s.CreateBatch(Batch{
			ID: "batch-tags", Tag: "tld", CreatedAt: now, DomainCount: 4, SnapshotIntent: true,
		}); err != nil {
			t.Fatalf("create batch: %v", err)
		}

		// Four runs in batch-tags with one tag each at varying levels. The
		// INFO and NOTICE tags would dominate the unfiltered top-N because
		// they hit the most domains; the filter must drop them.
		for i, rec := range []struct {
			runID    string
			domainID int64
			tag      string
			level    string
		}{
			{"run-info-1", 100, "MODULE_OK", "INFO"},
			{"run-info-2", 101, "MODULE_OK", "INFO"},
			{"run-info-3", 102, "MODULE_OK", "INFO"},
			{"run-notice-1", 103, "ZONE_EXISTS", "NOTICE"},
			{"run-notice-2", 104, "ZONE_EXISTS", "NOTICE"},
			{"run-warn", 105, "DS07_NOT_SIGNED", "WARNING"},
			{"run-err", 106, "BROKEN_DNSSEC", "ERROR"},
		} {
			if _, err := s.GetOrCreateDomain(rec.runID + ".example"); err != nil {
				t.Fatalf("seed domain %d: %v", i, err)
			}
			job := Job{
				ID: rec.runID, DomainID: rec.domainID, Domain: rec.runID + ".example",
				BatchID: "batch-tags", Status: JobSucceeded,
				CreatedAt: now, StartedAt: now, FinishedAt: now.Add(time.Duration(i) * time.Minute),
			}
			createAndGraduate(t, s, job, nil)
			if err := s.UpsertAnalysisRunDomainSummary(AnalysisRunDomainSummary{
				CohortID: cohort.ID, RunID: rec.runID, DomainID: rec.domainID,
			}); err != nil {
				t.Fatalf("summary %d: %v", i, err)
			}
			if err := s.ReplaceAnalysisRunTagSummaries(cohort.ID, rec.runID, []AnalysisRunTagSummary{{
				CohortID: cohort.ID, RunID: rec.runID, DomainID: rec.domainID,
				Tag: rec.tag, Level: rec.level, OccurrenceCount: 1,
			}}); err != nil {
				t.Fatalf("tag summary %d: %v", i, err)
			}
		}

		overview, err := s.ComputeSnapshotOverview(cohort.ID, "batch-tags")
		if err != nil {
			t.Fatalf("ComputeSnapshotOverview: %v", err)
		}
		top := overview.TopTags
		if len(top) != 2 {
			t.Fatalf("top_tags = %d entries, want 2 (one WARNING + one ERROR): %+v", len(top), top)
		}
		for _, entry := range top {
			switch entry.Level {
			case "WARNING", "ERROR", "CRITICAL":
			default:
				t.Errorf("top_tags includes %q at level %q; only WARNING+ should surface", entry.Tag, entry.Level)
			}
		}
	})
}

// TestCountOutstandingJobsForBatch verifies the capture gate query. The
// capture poller promotes pending snapshots only once their batch has
// drained; this test pins the query's scoping to the right batch.
func TestCountOutstandingJobsForBatch(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		now := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)

		if _, err := s.Create(Job{
			ID: "job-a", Domain: "a.example", BatchID: "batch-x", Status: JobQueued, CreatedAt: now,
		}); err != nil {
			t.Fatalf("create A: %v", err)
		}
		if _, err := s.Create(Job{
			ID: "job-b", Domain: "b.example", BatchID: "batch-x", Status: JobQueued, CreatedAt: now,
		}); err != nil {
			t.Fatalf("create B: %v", err)
		}
		if _, err := s.Create(Job{
			ID: "job-c", Domain: "c.example", BatchID: "batch-other", Status: JobQueued, CreatedAt: now,
		}); err != nil {
			t.Fatalf("create C: %v", err)
		}

		count, err := s.CountOutstandingJobsForBatch("batch-x")
		if err != nil {
			t.Fatalf("CountOutstandingJobsForBatch: %v", err)
		}
		if count != 2 {
			t.Fatalf("outstanding jobs = %d, want 2", count)
		}

		// Orphan rows in the jobs table - terminal statuses that never made it
		// through GraduateJob cleanly - must not be counted as outstanding,
		// otherwise the snapshot capture gate stays closed forever.
		if _, err := s.Create(Job{
			ID: "job-orphan-ok", Domain: "ok.example", BatchID: "batch-x", Status: JobSucceeded, CreatedAt: now,
		}); err != nil {
			t.Fatalf("create orphan succeeded: %v", err)
		}
		if _, err := s.Create(Job{
			ID: "job-orphan-fail", Domain: "fail.example", BatchID: "batch-x", Status: JobFailed, CreatedAt: now,
		}); err != nil {
			t.Fatalf("create orphan failed: %v", err)
		}

		count, err = s.CountOutstandingJobsForBatch("batch-x")
		if err != nil {
			t.Fatalf("CountOutstandingJobsForBatch after orphans: %v", err)
		}
		if count != 2 {
			t.Fatalf("outstanding jobs after orphans = %d, want 2", count)
		}
	})
}

func TestCountUnprojectedSnapshotRuns(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		now := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
		cohort, err := s.UpsertAnalysisCohort(AnalysisCohort{
			SourceType:      "tag",
			SourceTag:       "tld",
			AnalysisEnabled: true,
		})
		if err != nil {
			t.Fatalf("seed cohort: %v", err)
		}

		for i, rec := range []struct {
			runID string
			tag   string
			ready bool
		}{
			{"run-ready", "tld", true},
			{"run-waiting", "tld", false},
			{"run-other-tag", "other", false},
		} {
			domain, err := s.GetOrCreateDomain(rec.runID + ".example")
			if err != nil {
				t.Fatalf("seed domain %d: %v", i, err)
			}
			if err := s.CreateTag(rec.tag, ""); err != nil {
				t.Fatalf("create tag %s: %v", rec.tag, err)
			}
			if err := s.TagDomains(rec.tag, []int64{domain.ID}); err != nil {
				t.Fatalf("tag domain %d: %v", i, err)
			}
			job := Job{
				ID:         rec.runID,
				DomainID:   domain.ID,
				Domain:     domain.Name,
				BatchID:    "batch-x",
				Status:     JobSucceeded,
				CreatedAt:  now,
				StartedAt:  now,
				FinishedAt: now.Add(time.Duration(i) * time.Minute),
			}
			createAndGraduate(t, s, job, nil)
			if rec.ready {
				if err := s.SetAnalysisProjectionState(AnalysisProjectionState{
					CohortID:         cohort.ID,
					RunID:            rec.runID,
					ProjectorVersion: "test",
					Status:           AnalysisMaterializationReady,
					ProjectedAt:      now,
				}); err != nil {
					t.Fatalf("set ready state %d: %v", i, err)
				}
			}
		}

		count, err := s.CountUnprojectedSnapshotRuns(cohort.ID, "batch-x")
		if err != nil {
			t.Fatalf("CountUnprojectedSnapshotRuns: %v", err)
		}
		if count != 1 {
			t.Fatalf("unprojected runs = %d, want 1", count)
		}

		if err := s.SetAnalysisProjectionState(AnalysisProjectionState{
			CohortID:         cohort.ID,
			RunID:            "run-waiting",
			ProjectorVersion: "test",
			Status:           AnalysisMaterializationReady,
			ProjectedAt:      now,
		}); err != nil {
			t.Fatalf("set waiting ready: %v", err)
		}
		count, err = s.CountUnprojectedSnapshotRuns(cohort.ID, "batch-x")
		if err != nil {
			t.Fatalf("CountUnprojectedSnapshotRuns after ready: %v", err)
		}
		if count != 0 {
			t.Fatalf("unprojected runs after ready = %d, want 0", count)
		}
	})
}
