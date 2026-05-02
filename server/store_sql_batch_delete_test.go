package server

import (
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

// seedBatchForDeleteTest creates a batch row with one graduated run and a
// single log entry so tests have realistic state to assert against.
func seedBatchForDeleteTest(t *testing.T, s *SQLJobStore, batchID, tag string) (runID string, domainID int64) {
	t.Helper()
	now := time.Now().UTC()
	if err := s.CreateBatch(Batch{
		ID:             batchID,
		Tag:            tag,
		CreatedAt:      now,
		DomainCount:    1,
		SnapshotIntent: true,
	}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}
	domain, err := s.GetOrCreateDomain("example.com")
	if err != nil {
		t.Fatalf("GetOrCreateDomain: %v", err)
	}
	job := Job{
		ID:         "job-" + batchID,
		BatchID:    batchID,
		Domain:     "example.com",
		DomainID:   domain.ID,
		Status:     JobSucceeded,
		CreatedAt:  now,
		StartedAt:  now,
		FinishedAt: now,
	}
	if _, err := s.Create(job); err != nil {
		t.Fatalf("Create job: %v", err)
	}
	entries := []engine.LogEntry{{Timestamp: 1.0, Module: "System", Testcase: "", Tag: "MODULE_START", Level: "INFO"}}
	if err := s.GraduateJob(job, entries); err != nil {
		t.Fatalf("GraduateJob: %v", err)
	}
	return job.ID, domain.ID
}

func countRows(t *testing.T, s *SQLJobStore, query string, args ...any) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", query, err)
	}
	return n
}

func TestSQLJobStoreDeleteBatchRemovesBatchRunsJobsEntries(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			runID, _ := seedBatchForDeleteTest(t, s, "batch_del_1", "tld")

			snapshotIDs, err := s.DeleteBatch("batch_del_1")
			if err != nil {
				t.Fatalf("DeleteBatch: %v", err)
			}
			if len(snapshotIDs) != 0 {
				t.Fatalf("expected no snapshots removed, got %v", snapshotIDs)
			}
			if _, ok := s.GetBatch("batch_del_1"); ok {
				t.Fatal("batch row still present")
			}
			if _, ok := s.GetRun(runID); ok {
				t.Fatal("run still present")
			}
			if n := countRows(t, s, "SELECT COUNT(*) FROM entries WHERE run_id = "+s.ph(1), runID); n != 0 {
				t.Fatalf("entries remain: %d", n)
			}
			if n := countRows(t, s, "SELECT COUNT(*) FROM jobs WHERE batch_id = "+s.ph(1), "batch_del_1"); n != 0 {
				t.Fatalf("jobs remain: %d", n)
			}
		})
	}
}

func TestSQLJobStoreDeleteBatchClearsDomainLatest(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			_, domainID := seedBatchForDeleteTest(t, s, "batch_del_2", "tld")

			before, _ := s.GetDomain(domainID)
			if before.LatestRunID == "" {
				t.Fatal("precondition: domain latest_run_id should be set after graduation")
			}

			if _, err := s.DeleteBatch("batch_del_2"); err != nil {
				t.Fatalf("DeleteBatch: %v", err)
			}
			after, ok := s.GetDomain(domainID)
			if !ok {
				t.Fatal("domain should survive batch delete")
			}
			if after.LatestRunID != "" {
				t.Fatalf("domain latest_run_id = %q, want empty after delete", after.LatestRunID)
			}
			if after.RunCount != 0 {
				t.Fatalf("domain run_count = %d, want 0 after delete", after.RunCount)
			}
		})
	}
}

func TestSQLJobStoreDeleteBatchRemovesSnapshotAndAggregates(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			seedBatchForDeleteTest(t, s, "batch_del_3", "tld")

			cohort, err := s.UpsertAnalysisCohort(AnalysisCohort{
				SourceType:      "tag",
				SourceTag:       "tld",
				Label:           "TLDs",
				AnalysisEnabled: true,
				PublicEnabled:   true,
			})
			if err != nil {
				t.Fatalf("UpsertAnalysisCohort: %v", err)
			}
			now := time.Now().UTC()
			snap, err := s.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
				CohortID:    cohort.ID,
				BatchID:     "batch_del_3",
				Slug:        "2026-04-20",
				Label:       "April 2026",
				CapturedAt:  now,
				FirstRunAt:  now,
				LastRunAt:   now,
				RunCount:    1,
				DomainCount: 1,
				Status:      AnalysisSnapshotStatusCaptured,
				IsPublic:    true,
				CreatedAt:   now,
				UpdatedAt:   now,
			})
			if err != nil {
				t.Fatalf("UpsertAnalysisCohortSnapshot: %v", err)
			}
			if err := s.ReplaceSnapshotOverview(snap.ID, SnapshotOverviewV2{
				FactDistributions: map[string]PublicAnalysisFactDistribution{
					FactCategoryGrade: {
						Category: FactCategoryGrade,
						Buckets:  []PublicAnalysisFactBucket{{Key: "A", Count: 1}},
					},
				},
			}); err != nil {
				t.Fatalf("ReplaceSnapshotOverview: %v", err)
			}
			if err := s.ReplaceSnapshotEntityViews(snap.ID, SnapshotEntityViews{
				Nameservers: []AnalysisSnapshotNameserverView{{NameserverID: 1, NameserverName: "ns1.example", DomainCount: 1}},
				Endpoints:   []AnalysisSnapshotEndpointView{{NameserverID: 1, AddressID: 1, NameserverName: "ns1.example", Address: "192.0.2.1", Family: "ipv4", DomainCount: 1}},
				ASNs:        []AnalysisSnapshotASNView{{ASN: 64496, Label: "Example AS", DomainCount: 1}},
			}); err != nil {
				t.Fatalf("ReplaceSnapshotEntityViews: %v", err)
			}

			removed, err := s.DeleteBatch("batch_del_3")
			if err != nil {
				t.Fatalf("DeleteBatch: %v", err)
			}
			if len(removed) != 1 || removed[0] != snap.ID {
				t.Fatalf("removed snapshot ids = %v, want [%d]", removed, snap.ID)
			}
			if _, ok := s.GetAnalysisCohortSnapshot(snap.ID); ok {
				t.Fatal("snapshot still present")
			}
			if n := countRows(t, s,
				"SELECT COUNT(*) FROM analysis_snapshot_overview_view WHERE snapshot_id = "+s.ph(1),
				snap.ID,
			); n != 0 {
				t.Fatalf("overview view rows remain: %d", n)
			}
			for _, table := range []string{
				"analysis_snapshot_nameserver_view",
				"analysis_snapshot_endpoint_view",
				"analysis_snapshot_asn_view",
				"analysis_snapshot_domain_view",
				"analysis_snapshot_prefix_view",
			} {
				if n := countRows(t, s,
					"SELECT COUNT(*) FROM "+table+" WHERE snapshot_id = "+s.ph(1),
					snap.ID,
				); n != 0 {
					t.Fatalf("%s rows remain: %d", table, n)
				}
			}
		})
	}
}

func TestSQLJobStoreDeleteBatchIdempotentOnMissing(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			removed, err := s.DeleteBatch("batch_does_not_exist")
			if err != nil {
				t.Fatalf("DeleteBatch missing: %v", err)
			}
			if len(removed) != 0 {
				t.Fatalf("removed = %v, want empty", removed)
			}
		})
	}
}

func TestSQLJobStoreBatchDeletePreviewStats(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			seedBatchForDeleteTest(t, s, "batch_preview_1", "tld")

			preview, err := s.BatchDeletePreviewStats("batch_preview_1")
			if err != nil {
				t.Fatalf("BatchDeletePreviewStats: %v", err)
			}
			if !preview.Exists {
				t.Fatal("expected Exists=true for seeded batch")
			}
			if preview.CompletedRuns != 1 {
				t.Fatalf("CompletedRuns = %d, want 1", preview.CompletedRuns)
			}
			if preview.Entries < 1 {
				t.Fatalf("Entries = %d, want >=1", preview.Entries)
			}
			if preview.Tag != "tld" {
				t.Fatalf("Tag = %q, want \"tld\"", preview.Tag)
			}
			if !preview.SnapshotIntent {
				t.Fatal("SnapshotIntent should round-trip as true")
			}
		})
	}
}

func TestSQLJobStoreBatchDeletePreviewStatsMissing(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			preview, err := s.BatchDeletePreviewStats("missing")
			if err != nil {
				t.Fatalf("BatchDeletePreviewStats: %v", err)
			}
			if preview.Exists {
				t.Fatal("expected Exists=false for missing batch")
			}
		})
	}
}

func TestSQLJobStoreListBatchesByTag(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			base := time.Now().UTC()
			for i, id := range []string{"b_tld_a", "b_tld_b", "b_other"} {
				tag := "tld"
				if id == "b_other" {
					tag = "muni"
				}
				if err := s.CreateBatch(Batch{
					ID:        id,
					Tag:       tag,
					CreatedAt: base.Add(time.Duration(i) * time.Minute),
				}); err != nil {
					t.Fatalf("CreateBatch %s: %v", id, err)
				}
			}
			list := s.ListBatchesByTag("tld", 10, 0)
			if list.Total != 2 {
				t.Fatalf("Total = %d, want 2", list.Total)
			}
			if len(list.Items) != 2 {
				t.Fatalf("len(Items) = %d, want 2", len(list.Items))
			}
			if list.Items[0].ID != "b_tld_b" || list.Items[1].ID != "b_tld_a" {
				t.Fatalf("order = %v, want [b_tld_b b_tld_a]", []string{list.Items[0].ID, list.Items[1].ID})
			}
		})
	}
}
