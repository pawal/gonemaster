package server

import (
	"encoding/json"
	"testing"
	"time"
)

// TestComputeSnapshotAggregatesSeverityAndGrade checks that the severity
// and grade distribution aggregates are keyed on distinct domains scoped
// to the batch, not all runs in the cohort.
func TestComputeSnapshotAggregatesSeverityAndGrade(t *testing.T) {
	s := testStoreForBackend(t, testBackends(t)[0])
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
		if _, err := s.Create(job); err != nil {
			t.Fatalf("create job: %v", err)
		}
		if err := s.GraduateJob(job, nil); err != nil {
			t.Fatalf("graduate job: %v", err)
		}
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
	}

	aggs, err := s.ComputeSnapshotAggregates(cohort.ID, "batch-x")
	if err != nil {
		t.Fatalf("ComputeSnapshotAggregates: %v", err)
	}
	byCategory := map[string]string{}
	for _, agg := range aggs {
		byCategory[agg.Category] = agg.PayloadJSON
	}

	var severity map[string]int
	if err := json.Unmarshal([]byte(byCategory[SnapshotAggregateSeverityDistribution]), &severity); err != nil {
		t.Fatalf("unmarshal severity: %v", err)
	}
	if severity["NOTICE"] != 1 || severity["WARNING"] != 1 {
		t.Fatalf("severity counts = %v", severity)
	}
	if _, ok := severity["CRITICAL"]; ok {
		t.Fatalf("severity payload leaked the out-of-batch CRITICAL row: %v", severity)
	}

	var grades map[string]int
	if err := json.Unmarshal([]byte(byCategory[SnapshotAggregateGradeDistribution]), &grades); err != nil {
		t.Fatalf("unmarshal grades: %v", err)
	}
	if grades["A"] != 1 || grades["B"] != 1 {
		t.Fatalf("grade counts = %v", grades)
	}
	if _, ok := grades["F"]; ok {
		t.Fatalf("grade payload leaked the out-of-batch F row: %v", grades)
	}
}

// TestCountOutstandingJobsForBatch verifies the capture gate query. The
// capture poller promotes pending snapshots only once their batch has
// drained; this test pins the query's scoping to the right batch.
func TestCountOutstandingJobsForBatch(t *testing.T) {
	s := testStoreForBackend(t, testBackends(t)[0])
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
}
