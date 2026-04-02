package server

import (
	"database/sql"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

func BenchmarkSQLJobStoreUpdateInFlightSQLite(b *testing.B) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		b.Fatalf("open sqlite: %v", err)
	}
	configurePool(db, "sqlite")
	defer db.Close()

	if err := runMigrations(db, sqliteDialect{}); err != nil {
		b.Fatalf("runMigrations: %v", err)
	}

	store := NewSQLJobStore(db, sqliteDialect{})
	now := time.Now().UTC().Truncate(time.Microsecond)
	job := Job{
		ID:        "bench-sql-update",
		PublicID:  "benchupd",
		DomainID:  7,
		BatchID:   "batch-bench",
		Domain:    "benchmark.example",
		Status:    JobQueued,
		CreatedAt: now,
		Priority:  PriorityBatch,
		Profile:   "profiles/original.yaml",
		Tests:     []string{"basic01", "zone01", "delegation01"},
		Overrides: map[string]any{
			"resolver": map[string]any{
				"timeout_ms":                1500,
				"rate_limit_pacing_enabled": true,
			},
		},
		UndelegatedNS: []engine.UndelegatedNameserver{
			{Name: "ns1.example."},
			{Name: "ns2.example."},
		},
		UndelegatedDS: []engine.UndelegatedDSInfo{
			{KeyTag: 12345, Algorithm: 8, DigestType: 2, Digest: "abcd"},
		},
		MinLevel: "WARNING",
	}
	if _, err := store.Create(job); err != nil {
		b.Fatalf("Create: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		update := job
		update.Status = JobRunning
		update.StartedAt = now.Add(time.Second)
		update.Progress = i % 101
		if i%25 == 0 {
			update.Error = "transient"
		}
		if err := store.Update(update); err != nil {
			b.Fatalf("Update: %v", err)
		}
	}
}
