package server

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/lib/pq"
	_ "modernc.org/sqlite"
)

// ---- Test infrastructure ---------------------------------------------------

// spyDialect wraps another dialect and counts Placeholder calls.
type spyDialect struct {
	inner            sqlDialect
	placeholderCalls int
}

func (d *spyDialect) Placeholder(n int) string     { d.placeholderCalls++; return d.inner.Placeholder(n) }
func (d *spyDialect) TimestampVal(t time.Time) any { return d.inner.TimestampVal(t) }
func (d *spyDialect) DriverName() string           { return d.inner.DriverName() }
func (d *spyDialect) UpsertResultSQL() string      { return d.inner.UpsertResultSQL() }
func (d *spyDialect) IsDuplicateKey(err error) bool { return d.inner.IsDuplicateKey(err) }

// testDollarDialect simulates PostgreSQL's $n placeholder style for testing.
type testDollarDialect struct{}

func (testDollarDialect) Placeholder(n int) string    { return fmt.Sprintf("$%d", n) }
func (testDollarDialect) TimestampVal(t time.Time) any { return sqliteDialect{}.TimestampVal(t) }
func (testDollarDialect) DriverName() string           { return "test-dollar" }
func (testDollarDialect) UpsertResultSQL() string {
	return `INSERT INTO results (job_id, batch_id, status, summary_json, raw_json)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (job_id) DO UPDATE SET
		 	batch_id=EXCLUDED.batch_id, status=EXCLUDED.status,
		 	summary_json=EXCLUDED.summary_json, raw_json=EXCLUDED.raw_json`
}
func (testDollarDialect) IsDuplicateKey(err error) bool {
	return strings.Contains(err.Error(), "duplicate key value violates unique constraint")
}

// testBackend describes a database backend for parameterized store tests.
type testBackend struct {
	name    string     // human-readable name used in t.Run
	driver  string     // sql.DB driver name ("sqlite", "postgres", "mysql")
	dsn     string     // data source name
	dialect sqlDialect
}

// testBackends returns the backends to run store tests against.
// SQLite always runs (in-memory). PostgreSQL and MariaDB run only when the
// corresponding TEST_POSTGRES_DSN / TEST_MARIADB_DSN environment variables
// are set.
func testBackends(t *testing.T) []testBackend {
	t.Helper()
	backends := []testBackend{
		{name: "sqlite", driver: "sqlite", dsn: ":memory:", dialect: sqliteDialect{}},
	}
	if dsn := os.Getenv("TEST_POSTGRES_DSN"); dsn != "" {
		backends = append(backends, testBackend{
			name:    "postgres",
			driver:  "postgres",
			dsn:     dsn,
			dialect: postgresDialect{},
		})
	}
	if dsn := os.Getenv("TEST_MARIADB_DSN"); dsn != "" {
		backends = append(backends, testBackend{
			name:    "mariadb",
			driver:  "mysql",
			dsn:     mariadbDSN(dsn),
			dialect: mariadbDialect{},
		})
	}
	return backends
}

// resetSchema drops all application tables so tests start from a clean state
// on persistent backends (PostgreSQL, MariaDB).
func resetSchema(db *sql.DB) error {
	for _, tbl := range []string{"results", "jobs", "schema_migrations"} {
		if _, err := db.Exec("DROP TABLE IF EXISTS " + tbl); err != nil {
			return fmt.Errorf("drop table %s: %w", tbl, err)
		}
	}
	return nil
}

// testStoreForBackend opens a *SQLJobStore for the given backend with a fresh
// schema. Persistent backends have their tables dropped and recreated; SQLite
// ":memory:" is always fresh.
func testStoreForBackend(t *testing.T, b testBackend) *SQLJobStore {
	t.Helper()
	db, err := sql.Open(b.driver, b.dsn)
	if err != nil {
		t.Fatalf("open %s: %v", b.name, err)
	}
	configurePool(db, b.driver)
	t.Cleanup(func() { _ = db.Close() })
	if b.driver != "sqlite" {
		if err := resetSchema(db); err != nil {
			t.Fatalf("reset schema (%s): %v", b.name, err)
		}
	}
	if err := runMigrations(db, b.dialect); err != nil {
		t.Fatalf("runMigrations (%s): %v", b.name, err)
	}
	return NewSQLJobStore(db, b.dialect)
}

// testSQLiteStore opens an in-memory SQLite store. Kept for migration tests
// that are SQLite-specific and cannot be parameterized.
func testSQLiteStore(t *testing.T) *SQLJobStore {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if err := runMigrations(db, sqliteDialect{}); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}
	return NewSQLJobStore(db, sqliteDialect{})
}

// ids extracts job IDs from a slice for readable error messages.
func ids(jobs []Job) []string {
	out := make([]string, len(jobs))
	for i, j := range jobs {
		out[i] = j.ID
	}
	return out
}

// ---- testBackends gating ---------------------------------------------------

func TestTestBackendsSQLiteAlwaysPresent(t *testing.T) {
	backends := testBackends(t)
	for _, b := range backends {
		if b.name == "sqlite" {
			return
		}
	}
	t.Fatal("testBackends: sqlite not present")
}

func TestTestBackendsPostgresExcludedWithoutEnv(t *testing.T) {
	t.Setenv("TEST_POSTGRES_DSN", "")
	for _, b := range testBackends(t) {
		if b.name == "postgres" {
			t.Fatal("testBackends: postgres present without TEST_POSTGRES_DSN")
		}
	}
}

func TestTestBackendsMariaDBExcludedWithoutEnv(t *testing.T) {
	t.Setenv("TEST_MARIADB_DSN", "")
	for _, b := range testBackends(t) {
		if b.name == "mariadb" {
			t.Fatal("testBackends: mariadb present without TEST_MARIADB_DSN")
		}
	}
}

func TestTestBackendsPostgresIncludedWhenEnvSet(t *testing.T) {
	// Use a sentinel DSN — we only test inclusion, not connectivity.
	t.Setenv("TEST_POSTGRES_DSN", "postgres://sentinel/test")
	found := false
	for _, b := range testBackends(t) {
		if b.name == "postgres" {
			found = true
			if b.driver != "postgres" {
				t.Errorf("postgres backend driver = %q, want \"postgres\"", b.driver)
			}
		}
	}
	if !found {
		t.Fatal("testBackends: postgres not present when TEST_POSTGRES_DSN is set")
	}
}

func TestTestBackendsMariaDBIncludedWhenEnvSet(t *testing.T) {
	t.Setenv("TEST_MARIADB_DSN", "gonemaster:pass@tcp(localhost:3306)/test")
	found := false
	for _, b := range testBackends(t) {
		if b.name == "mariadb" {
			found = true
			if b.driver != "mysql" {
				t.Errorf("mariadb backend driver = %q, want \"mysql\"", b.driver)
			}
			if !strings.Contains(b.dsn, "parseTime=true") {
				t.Errorf("mariadb DSN missing parseTime=true: %q", b.dsn)
			}
		}
	}
	if !found {
		t.Fatal("testBackends: mariadb not present when TEST_MARIADB_DSN is set")
	}
}

// ---- Migration tests -------------------------------------------------------

func TestRunMigrationsFresh(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	if err := runMigrations(db, sqliteDialect{}); err != nil {
		t.Fatalf("first run: %v", err)
	}
	// Idempotent: second run must not error.
	if err := runMigrations(db, sqliteDialect{}); err != nil {
		t.Fatalf("second run (idempotent): %v", err)
	}

	for _, tbl := range []string{"jobs", "results", "schema_migrations"} {
		var name string
		if err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl,
		).Scan(&name); err != nil || name != tbl {
			t.Errorf("table %q not found after migration", tbl)
		}
	}

	var version int
	if err := db.QueryRow(`SELECT version FROM schema_migrations WHERE version=1`).Scan(&version); err != nil {
		t.Fatalf("migration version 1 not recorded: %v", err)
	}
}

func TestRunMigrationsUsesDialectPlaceholder(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	spy := &spyDialect{inner: sqliteDialect{}}
	if err := runMigrations(db, spy); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}
	if spy.placeholderCalls == 0 {
		t.Fatal("runMigrations did not call dialect.Placeholder")
	}
}

// ---- Create / Get ----------------------------------------------------------

func TestSQLJobStoreCreateGet(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			now := time.Now().UTC().Truncate(time.Microsecond)

			job := Job{
				ID:        "j1",
				BatchID:   "b1",
				Domain:    "example.com",
				Status:    JobQueued,
				CreatedAt: now,
				MinLevel:  "WARNING",
			}
			created, err := s.Create(job)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			if created.ID != job.ID {
				t.Fatalf("Create returned wrong id: %q", created.ID)
			}

			got, ok := s.Get(job.ID)
			if !ok {
				t.Fatal("Get: job not found")
			}
			if got.Domain != "example.com" {
				t.Fatalf("Domain: got %q", got.Domain)
			}
			if got.BatchID != "b1" {
				t.Fatalf("BatchID: got %q", got.BatchID)
			}
			if got.Status != JobQueued {
				t.Fatalf("Status: got %q", got.Status)
			}
			if got.MinLevel != "WARNING" {
				t.Fatalf("MinLevel: got %q", got.MinLevel)
			}
			if !got.CreatedAt.Equal(now) {
				t.Fatalf("CreatedAt: got %v, want %v", got.CreatedAt, now)
			}
			// SeverityTotals must be zero before SetResult.
			for _, lv := range []string{"NOTICE", "WARNING", "ERROR", "CRITICAL"} {
				if got.SeverityTotals[lv] != 0 {
					t.Errorf("SeverityTotals[%q] = %d, want 0", lv, got.SeverityTotals[lv])
				}
			}
		})
	}
}

func TestSQLJobStoreCreateDuplicate(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			job := Job{ID: "dup", Domain: "x.com", Status: JobQueued, CreatedAt: time.Now().UTC()}
			if _, err := s.Create(job); err != nil {
				t.Fatalf("first Create: %v", err)
			}
			if _, err := s.Create(job); err == nil {
				t.Fatal("expected error on duplicate Create")
			}
		})
	}
}

func TestSQLJobStoreGetMissing(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			_, ok := s.Get("does-not-exist")
			if ok {
				t.Fatal("expected ok=false for missing job")
			}
		})
	}
}

// ---- Update ----------------------------------------------------------------

func TestSQLJobStoreUpdate(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			now := time.Now().UTC().Truncate(time.Microsecond)
			job := Job{ID: "j2", Domain: "update.test", Status: JobQueued, CreatedAt: now}
			if _, err := s.Create(job); err != nil {
				t.Fatalf("Create: %v", err)
			}

			started := now.Add(time.Second)
			job.Status = JobRunning
			job.StartedAt = started
			job.Progress = 50
			if err := s.Update(job); err != nil {
				t.Fatalf("Update: %v", err)
			}

			got, ok := s.Get(job.ID)
			if !ok {
				t.Fatal("Get after Update: not found")
			}
			if got.Status != JobRunning {
				t.Fatalf("Status: got %q, want running", got.Status)
			}
			if got.Progress != 50 {
				t.Fatalf("Progress: got %d", got.Progress)
			}
			if !got.StartedAt.Equal(started) {
				t.Fatalf("StartedAt: got %v, want %v", got.StartedAt, started)
			}
		})
	}
}

func TestSQLJobStoreUpdateMissing(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			err := s.Update(Job{ID: "ghost", Status: JobRunning, CreatedAt: time.Now().UTC()})
			if err == nil {
				t.Fatal("expected error updating missing job")
			}
		})
	}
}

// ---- SetResult / GetResult -------------------------------------------------

func TestSQLJobStoreSetGetResult(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			job := Job{ID: "j3", Domain: "result.test", Status: JobSucceeded, CreatedAt: time.Now().UTC()}
			if _, err := s.Create(job); err != nil {
				t.Fatalf("Create: %v", err)
			}

			result := JobResult{
				JobID:   "j3",
				BatchID: "b2",
				Status:  JobSucceeded,
				Summary: map[string]any{
					"levels": map[string]int{"NOTICE": 1, "WARNING": 2, "ERROR": 3},
				},
			}
			if err := s.SetResult("j3", result); err != nil {
				t.Fatalf("SetResult: %v", err)
			}

			got, ok := s.GetResult("j3")
			if !ok {
				t.Fatal("GetResult: not found")
			}
			if got.JobID != "j3" || got.BatchID != "b2" || got.Status != JobSucceeded {
				t.Fatalf("GetResult fields: %+v", got)
			}
		})
	}
}

func TestSQLJobStoreSetResultUpdatesSeverityTotals(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			job := Job{ID: "j4", Domain: "sev.test", Status: JobSucceeded, CreatedAt: time.Now().UTC()}
			if _, err := s.Create(job); err != nil {
				t.Fatalf("Create: %v", err)
			}

			if err := s.SetResult("j4", JobResult{
				JobID:  "j4",
				Status: JobSucceeded,
				Summary: map[string]any{
					"levels": map[string]int{"NOTICE": 1, "WARNING": 2, "ERROR": 3, "CRITICAL": 4},
				},
			}); err != nil {
				t.Fatalf("SetResult: %v", err)
			}

			list := s.List(JobFilter{Limit: 10})
			if len(list.Items) != 1 {
				t.Fatalf("List: expected 1 item, got %d", len(list.Items))
			}
			tot := list.Items[0].SeverityTotals
			if tot["NOTICE"] != 1 || tot["WARNING"] != 2 || tot["ERROR"] != 3 || tot["CRITICAL"] != 4 {
				t.Fatalf("unexpected severity_totals: %+v", tot)
			}
		})
	}
}

func TestSQLJobStoreSetResultWithRaw(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			job := Job{ID: "j5", Domain: "raw.test", Status: JobSucceeded, CreatedAt: time.Now().UTC()}
			if _, err := s.Create(job); err != nil {
				t.Fatalf("Create: %v", err)
			}

			raw := &JobResultRaw{
				Locale: "en",
				Entries: []JobResultEntry{
					{Module: "DNSSEC", Testcase: "DNSSEC01", Tag: "NO_NSEC", Level: "WARNING"},
				},
			}
			if err := s.SetResult("j5", JobResult{JobID: "j5", Status: JobSucceeded, Raw: raw}); err != nil {
				t.Fatalf("SetResult with raw: %v", err)
			}

			got, ok := s.GetResult("j5")
			if !ok {
				t.Fatal("GetResult: not found")
			}
			if got.Raw == nil {
				t.Fatal("Raw is nil")
			}
			if got.Raw.Locale != "en" {
				t.Fatalf("Locale: got %q", got.Raw.Locale)
			}
			if len(got.Raw.Entries) != 1 || got.Raw.Entries[0].Module != "DNSSEC" {
				t.Fatalf("Entries: %+v", got.Raw.Entries)
			}
		})
	}
}

func TestSQLJobStoreSetResultMissingJob(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			err := s.SetResult("ghost", JobResult{JobID: "ghost", Status: JobFailed})
			if err == nil {
				t.Fatal("expected error for missing job")
			}
		})
	}
}

func TestSQLJobStoreGetResultMissing(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			_, ok := s.GetResult("no-result")
			if ok {
				t.Fatal("expected ok=false for missing result")
			}
		})
	}
}

// ---- List filters ----------------------------------------------------------

func TestSQLJobStoreListFilters(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			base := time.Now().UTC().Add(-time.Minute).Truncate(time.Millisecond)

			for _, j := range []Job{
				{ID: "f1", BatchID: "batch1", Domain: "alpha.example.com", Status: JobQueued, CreatedAt: base},
				{ID: "f2", BatchID: "batch2", Domain: "beta.example.com", Status: JobRunning, CreatedAt: base.Add(time.Second)},
				{ID: "f3", BatchID: "batch2", Domain: "gamma.example.org", Status: JobSucceeded, CreatedAt: base.Add(2 * time.Second)},
			} {
				if _, err := s.Create(j); err != nil {
					t.Fatalf("Create %q: %v", j.ID, err)
				}
			}

			t.Run("status", func(t *testing.T) {
				list := s.List(JobFilter{Status: JobRunning, Limit: 10})
				if list.Total != 1 || list.Items[0].ID != "f2" {
					t.Fatalf("status filter: total=%d items=%v", list.Total, ids(list.Items))
				}
			})

			t.Run("batch_id", func(t *testing.T) {
				list := s.List(JobFilter{BatchID: "batch2", Limit: 10})
				if list.Total != 2 {
					t.Fatalf("batch_id filter: total=%d", list.Total)
				}
			})

			t.Run("domain substring", func(t *testing.T) {
				list := s.List(JobFilter{Domain: "alpha", Limit: 10})
				if list.Total != 1 || list.Items[0].ID != "f1" {
					t.Fatalf("domain filter: total=%d items=%v", list.Total, ids(list.Items))
				}
			})

			t.Run("domain case insensitive", func(t *testing.T) {
				list := s.List(JobFilter{Domain: "ALPHA", Limit: 10})
				if list.Total != 1 {
					t.Fatalf("domain case-insensitive filter: total=%d", list.Total)
				}
			})

			t.Run("created_after", func(t *testing.T) {
				list := s.List(JobFilter{CreatedAfter: base.Add(500 * time.Millisecond), Limit: 10})
				if list.Total != 2 {
					t.Fatalf("created_after: total=%d", list.Total)
				}
			})

			t.Run("created_before", func(t *testing.T) {
				list := s.List(JobFilter{CreatedBefore: base.Add(500 * time.Millisecond), Limit: 10})
				if list.Total != 1 || list.Items[0].ID != "f1" {
					t.Fatalf("created_before: total=%d items=%v", list.Total, ids(list.Items))
				}
			})
		})
	}
}

func TestSQLJobStoreListFiltersSecondBoundary(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			base := time.Date(2026, 2, 3, 0, 0, 0, 500_000_000, time.UTC)

			for _, j := range []Job{
				{ID: "b1", Domain: "alpha.example.com", Status: JobQueued, CreatedAt: base},
				{ID: "b2", Domain: "beta.example.com", Status: JobQueued, CreatedAt: base.Add(time.Second)},
				{ID: "b3", Domain: "gamma.example.com", Status: JobQueued, CreatedAt: base.Add(2 * time.Second)},
			} {
				if _, err := s.Create(j); err != nil {
					t.Fatalf("Create %q: %v", j.ID, err)
				}
			}

			cutoff := base.Add(500 * time.Millisecond) // 2026-02-03T00:00:01Z

			after := s.List(JobFilter{
				CreatedAfter: cutoff,
				Limit:        10,
				Sort:         JobSortCreatedAtAsc,
			})
			if after.Total != 2 || len(after.Items) != 2 || after.Items[0].ID != "b2" || after.Items[1].ID != "b3" {
				t.Fatalf("created_after boundary: total=%d items=%v", after.Total, ids(after.Items))
			}

			before := s.List(JobFilter{
				CreatedBefore: cutoff,
				Limit:         10,
				Sort:          JobSortCreatedAtAsc,
			})
			if before.Total != 1 || len(before.Items) != 1 || before.Items[0].ID != "b1" {
				t.Fatalf("created_before boundary: total=%d items=%v", before.Total, ids(before.Items))
			}
		})
	}
}

// ---- List severity filters -------------------------------------------------

func TestSQLJobStoreListSeverityFilter(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			base := time.Now().UTC().Truncate(time.Millisecond)

			for _, id := range []string{"s1", "s2", "s3"} {
				if _, err := s.Create(Job{ID: id, Domain: id + ".test", Status: JobSucceeded, CreatedAt: base}); err != nil {
					t.Fatalf("Create: %v", err)
				}
			}
			_ = s.SetResult("s1", JobResult{
				JobID: "s1", Status: JobSucceeded,
				Summary: map[string]any{"levels": map[string]int{"WARNING": 1}},
			})
			_ = s.SetResult("s2", JobResult{
				JobID: "s2", Status: JobSucceeded,
				Summary: map[string]any{"levels": map[string]int{"CRITICAL": 1}},
			})
			// s3 has no result — sev_* all zero.

			t.Run("warnings_plus", func(t *testing.T) {
				list := s.List(JobFilter{Severity: JobSeverityWarningsPlus, Limit: 10})
				if list.Total != 2 {
					t.Fatalf("warnings_plus: total=%d", list.Total)
				}
			})

			t.Run("errors_only", func(t *testing.T) {
				list := s.List(JobFilter{Severity: JobSeverityErrorsOnly, Limit: 10})
				if list.Total != 1 || list.Items[0].ID != "s2" {
					t.Fatalf("errors_only: total=%d items=%v", list.Total, ids(list.Items))
				}
			})
		})
	}
}

// ---- List sorting ----------------------------------------------------------

func TestSQLJobStoreListSorting(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			base := time.Now().UTC().Truncate(time.Millisecond)

			for _, j := range []Job{
				{ID: "sort1", BatchID: "batch_b", Domain: "zeta.example", Status: JobQueued,
					CreatedAt: base, StartedAt: base.Add(2 * time.Second)},
				{ID: "sort2", BatchID: "batch_a", Domain: "alpha.example", Status: JobQueued,
					CreatedAt: base.Add(time.Second), StartedAt: base.Add(3 * time.Second)},
				{ID: "sort3", BatchID: "batch_c", Domain: "beta.example", Status: JobQueued,
					CreatedAt: base.Add(4 * time.Second)},
			} {
				if _, err := s.Create(j); err != nil {
					t.Fatalf("Create %q: %v", j.ID, err)
				}
			}
			_ = s.SetResult("sort1", JobResult{
				JobID: "sort1", Status: JobSucceeded,
				Summary: map[string]any{"levels": map[string]int{"ERROR": 1}},
			})
			_ = s.SetResult("sort2", JobResult{
				JobID: "sort2", Status: JobFailed,
				Summary: map[string]any{"levels": map[string]int{"CRITICAL": 2}},
			})

			t.Run("default started_at_desc", func(t *testing.T) {
				list := s.List(JobFilter{Limit: 10})
				// sort3 has no started_at so COALESCE gives created_at = base+4s, which is latest.
				if list.Items[0].ID != "sort3" {
					t.Fatalf("default sort: first=%q, want sort3", list.Items[0].ID)
				}
			})

			t.Run("created_at_asc", func(t *testing.T) {
				list := s.List(JobFilter{Limit: 10, Sort: JobSortCreatedAtAsc})
				if list.Items[0].ID != "sort1" {
					t.Fatalf("created_at_asc: first=%q", list.Items[0].ID)
				}
			})

			t.Run("created_at_desc", func(t *testing.T) {
				list := s.List(JobFilter{Limit: 10, Sort: JobSortCreatedAtDesc})
				if list.Items[0].ID != "sort3" {
					t.Fatalf("created_at_desc: first=%q", list.Items[0].ID)
				}
			})

			t.Run("domain_asc", func(t *testing.T) {
				list := s.List(JobFilter{Limit: 10, Sort: JobSortDomainAsc})
				if list.Items[0].ID != "sort2" {
					t.Fatalf("domain_asc: first=%q (want sort2/alpha)", list.Items[0].ID)
				}
			})

			t.Run("domain_desc", func(t *testing.T) {
				list := s.List(JobFilter{Limit: 10, Sort: JobSortDomainDesc})
				if list.Items[0].ID != "sort1" {
					t.Fatalf("domain_desc: first=%q (want sort1/zeta)", list.Items[0].ID)
				}
			})

			t.Run("batch_id_asc", func(t *testing.T) {
				list := s.List(JobFilter{Limit: 10, Sort: JobSortBatchIDAsc})
				if list.Items[0].ID != "sort2" {
					t.Fatalf("batch_id_asc: first=%q (want sort2/batch_a)", list.Items[0].ID)
				}
			})

			t.Run("batch_id_desc", func(t *testing.T) {
				list := s.List(JobFilter{Limit: 10, Sort: JobSortBatchIDDesc})
				if list.Items[0].ID != "sort3" {
					t.Fatalf("batch_id_desc: first=%q (want sort3/batch_c)", list.Items[0].ID)
				}
			})

			t.Run("started_at_asc", func(t *testing.T) {
				list := s.List(JobFilter{Limit: 10, Sort: JobSortStartedAtAsc})
				// sort1 has the earliest effective start time.
				if list.Items[0].ID != "sort1" {
					t.Fatalf("started_at_asc: first=%q (want sort1)", list.Items[0].ID)
				}
			})

			t.Run("error_desc", func(t *testing.T) {
				list := s.List(JobFilter{Limit: 10, Sort: JobSortErrorDesc})
				// sort2 has CRITICAL=2 (total err+crit=2), sort1 has ERROR=1 (total=1).
				if list.Items[0].ID != "sort2" || list.Items[1].ID != "sort1" {
					t.Fatalf("error_desc: got %v (want sort2, sort1)", ids(list.Items))
				}
			})

			t.Run("critical_desc", func(t *testing.T) {
				list := s.List(JobFilter{Limit: 10, Sort: JobSortCriticalDesc})
				if list.Items[0].ID != "sort2" {
					t.Fatalf("critical_desc: first=%q (want sort2/critical=2)", list.Items[0].ID)
				}
			})
		})
	}
}

// ---- Pagination ------------------------------------------------------------

func TestSQLJobStoreListPagination(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			base := time.Now().UTC().Truncate(time.Millisecond)

			for i := 0; i < 5; i++ {
				id := string(rune('1'+i)) // "1".."5"
				_, err := s.Create(Job{
					ID: "p" + id, Domain: "p" + id + ".test", Status: JobQueued,
					CreatedAt: base.Add(time.Duration(i) * time.Second),
				})
				if err != nil {
					t.Fatalf("Create p%s: %v", id, err)
				}
			}

			first := s.List(JobFilter{Limit: 2, Sort: JobSortCreatedAtAsc})
			if first.Total != 5 {
				t.Fatalf("total: got %d, want 5", first.Total)
			}
			if len(first.Items) != 2 || first.Items[0].ID != "p1" {
				t.Fatalf("first page: got %v", ids(first.Items))
			}
			if first.NextCursor != "2" || first.PrevCursor != "" {
				t.Fatalf("first page cursors: next=%q prev=%q", first.NextCursor, first.PrevCursor)
			}

			second := s.List(JobFilter{Limit: 2, Sort: JobSortCreatedAtAsc, Offset: 2})
			if len(second.Items) != 2 || second.Items[0].ID != "p3" {
				t.Fatalf("second page: got %v", ids(second.Items))
			}
			if second.PrevCursor != "0" || second.NextCursor != "4" {
				t.Fatalf("second page cursors: next=%q prev=%q", second.NextCursor, second.PrevCursor)
			}

			last := s.List(JobFilter{Limit: 2, Sort: JobSortCreatedAtAsc, Offset: 4})
			if len(last.Items) != 1 || last.Items[0].ID != "p5" {
				t.Fatalf("last page: got %v", ids(last.Items))
			}
			if last.NextCursor != "" {
				t.Fatalf("last page: unexpected NextCursor %q", last.NextCursor)
			}
		})
	}
}

func TestSQLJobStoreListDefaultLimit(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			list := s.List(JobFilter{})
			if list.Limit != 100 {
				t.Fatalf("default limit: got %d, want 100", list.Limit)
			}
		})
	}
}

// ---- Nullable timestamps & complex fields ----------------------------------

func TestSQLJobStoreNullTimestamps(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			job := Job{ID: "ts-null", Domain: "ts.test", Status: JobQueued, CreatedAt: time.Now().UTC()}
			if _, err := s.Create(job); err != nil {
				t.Fatalf("Create: %v", err)
			}
			got, ok := s.Get("ts-null")
			if !ok {
				t.Fatal("Get: not found")
			}
			if !got.StartedAt.IsZero() {
				t.Fatalf("StartedAt should be zero, got %v", got.StartedAt)
			}
			if !got.FinishedAt.IsZero() {
				t.Fatalf("FinishedAt should be zero, got %v", got.FinishedAt)
			}
		})
	}
}

func TestSQLJobStoreRoundTripComplexFields(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			job := Job{
				ID:        "complex",
				Domain:    "delegated.test",
				Status:    JobQueued,
				CreatedAt: time.Now().UTC(),
				Tests:     []string{"DNSSEC", "Zone"},
				Overrides: map[string]any{"key": "value", "num": float64(42)},
				MinLevel:  "NOTICE",
			}
			if _, err := s.Create(job); err != nil {
				t.Fatalf("Create: %v", err)
			}

			got, ok := s.Get("complex")
			if !ok {
				t.Fatal("Get: not found")
			}
			if len(got.Tests) != 2 || got.Tests[0] != "DNSSEC" {
				t.Fatalf("Tests: got %v", got.Tests)
			}
			if got.MinLevel != "NOTICE" {
				t.Fatalf("MinLevel: got %q", got.MinLevel)
			}
			if got.Overrides["key"] != "value" {
				t.Fatalf("Overrides: got %v", got.Overrides)
			}
		})
	}
}

// ---- Recovery --------------------------------------------------------------

func TestRecoverJobsRunningToFailed(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			base := time.Now().UTC()

			for _, job := range []Job{
				{ID: "r1", Domain: "r1.test", Status: JobRunning, CreatedAt: base},
				{ID: "q1", Domain: "q1.test", Status: JobQueued, CreatedAt: base.Add(time.Second)},
			} {
				if _, err := s.Create(job); err != nil {
					t.Fatalf("Create %q: %v", job.ID, err)
				}
			}

			q := NewInMemoryQueue()
			if err := RecoverJobs(s, q); err != nil {
				t.Fatalf("RecoverJobs: %v", err)
			}

			got, ok := s.Get("r1")
			if !ok {
				t.Fatal("r1 not found after recovery")
			}
			if got.Status != JobFailed {
				t.Fatalf("r1 status: got %q, want failed", got.Status)
			}
			if !strings.Contains(got.Error, "restarted") {
				t.Fatalf("r1 error should mention restart, got %q", got.Error)
			}
		})
	}
}

func TestRecoverJobsQueuedReenqueued(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			base := time.Now().UTC()

			for _, job := range []Job{
				{ID: "q1", Domain: "q1.test", Status: JobQueued, CreatedAt: base},
				{ID: "q2", Domain: "q2.test", Status: JobQueued, CreatedAt: base.Add(time.Second)},
				{ID: "done", Domain: "done.test", Status: JobSucceeded, CreatedAt: base},
			} {
				if _, err := s.Create(job); err != nil {
					t.Fatalf("Create %q: %v", job.ID, err)
				}
			}

			q := NewInMemoryQueue()
			if err := RecoverJobs(s, q); err != nil {
				t.Fatalf("RecoverJobs: %v", err)
			}

			// Queued jobs must have been enqueued in created_at ASC order.
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			id1, err := q.Dequeue(ctx)
			if err != nil || id1 != "q1" {
				t.Fatalf("dequeue 1: id=%q err=%v (want q1)", id1, err)
			}
			id2, err := q.Dequeue(ctx)
			if err != nil || id2 != "q2" {
				t.Fatalf("dequeue 2: id=%q err=%v (want q2)", id2, err)
			}

			// Only two items must have been enqueued (not "done").
			ctx2, cancel2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel2()
			extra, err := q.Dequeue(ctx2)
			if err == nil {
				t.Fatalf("unexpected third item in queue: %q", extra)
			}
		})
	}
}

func TestRecoverJobsNoopInMemory(t *testing.T) {
	store := NewInMemoryJobStore()
	queue := NewInMemoryQueue()
	if err := RecoverJobs(store, queue); err != nil {
		t.Fatalf("RecoverJobs on in-memory store: %v", err)
	}
}

// ---- NewWithOptions --------------------------------------------------------

func TestNewWithOptionsInMemory(t *testing.T) {
	cfg := DefaultConfig()
	srv, err := NewWithOptions(cfg)
	if err != nil {
		t.Fatalf("NewWithOptions (in-memory): %v", err)
	}
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestNewWithOptionsInvalidDriver(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Database.Driver = "notadriver"
	cfg.Database.DSN = "whatever"
	_, err := NewWithOptions(cfg)
	if err == nil {
		t.Fatal("expected error for unknown driver")
	}
}

// ---- Placeholder helpers ---------------------------------------------------

func TestPhRange(t *testing.T) {
	tests := []struct {
		name    string
		dialect sqlDialect
		start   int
		count   int
		want    string
	}{
		{"sqlite single", sqliteDialect{}, 1, 1, "?"},
		{"sqlite multi", sqliteDialect{}, 1, 3, "?, ?, ?"},
		{"sqlite offset", sqliteDialect{}, 5, 2, "?, ?"},
		{"dollar single", testDollarDialect{}, 1, 1, "$1"},
		{"dollar multi", testDollarDialect{}, 1, 3, "$1, $2, $3"},
		{"dollar offset", testDollarDialect{}, 5, 2, "$5, $6"},
		{"dollar high", testDollarDialect{}, 11, 5, "$11, $12, $13, $14, $15"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &SQLJobStore{dialect: tt.dialect}
			got := s.phRange(tt.start, tt.count)
			if got != tt.want {
				t.Fatalf("phRange(%d, %d) = %q, want %q", tt.start, tt.count, got, tt.want)
			}
		})
	}
}

func TestPhRangeCreatePlaceholderCount(t *testing.T) {
	// Verify Create's VALUES clause has the correct number of placeholders + literals.
	// The INSERT has 19 columns: 10 bound params, 4 literal zeros, 5 bound params = 15 total bound.
	s := &SQLJobStore{dialect: testDollarDialect{}}

	firstBlock := s.phRange(1, 10)
	secondBlock := s.phRange(11, 5)

	// Count dollar-placeholders in each block.
	firstCount := strings.Count(firstBlock, "$")
	secondCount := strings.Count(secondBlock, "$")
	if firstCount != 10 {
		t.Fatalf("first block has %d placeholders, want 10: %q", firstCount, firstBlock)
	}
	if secondCount != 5 {
		t.Fatalf("second block has %d placeholders, want 5: %q", secondCount, secondBlock)
	}

	// Verify sequential numbering.
	if !strings.Contains(firstBlock, "$1") || !strings.Contains(firstBlock, "$10") {
		t.Fatalf("first block should contain $1..$10: %q", firstBlock)
	}
	if !strings.Contains(secondBlock, "$11") || !strings.Contains(secondBlock, "$15") {
		t.Fatalf("second block should contain $11..$15: %q", secondBlock)
	}
}

func TestPhRangeUpdatePlaceholderCount(t *testing.T) {
	// Verify Update uses 14 placeholders (13 SET + 1 WHERE).
	s := &SQLJobStore{dialect: testDollarDialect{}}
	for i := 1; i <= 14; i++ {
		ph := s.ph(i)
		want := fmt.Sprintf("$%d", i)
		if ph != want {
			t.Fatalf("ph(%d) = %q, want %q", i, ph, want)
		}
	}
}

// ---- Dialect: UpsertResultSQL ----------------------------------------------

func TestUpsertResultSQLSQLite(t *testing.T) {
	sql := sqliteDialect{}.UpsertResultSQL()
	if !strings.Contains(sql, "INSERT OR REPLACE") {
		t.Fatalf("sqlite upsert must use INSERT OR REPLACE, got: %q", sql)
	}
	if !strings.Contains(sql, "results") {
		t.Fatalf("sqlite upsert must reference results table, got: %q", sql)
	}
	if count := strings.Count(sql, "?"); count != 5 {
		t.Fatalf("sqlite upsert must have 5 '?' placeholders, got %d: %q", count, sql)
	}
	for _, col := range []string{"job_id", "batch_id", "status", "summary_json", "raw_json"} {
		if !strings.Contains(sql, col) {
			t.Fatalf("sqlite upsert missing column %q: %q", col, sql)
		}
	}
}

func TestUpsertResultSQLDollar(t *testing.T) {
	sql := testDollarDialect{}.UpsertResultSQL()
	if !strings.Contains(sql, "ON CONFLICT") {
		t.Fatalf("dollar upsert must use ON CONFLICT, got: %q", sql)
	}
	if strings.Contains(sql, "INSERT OR REPLACE") {
		t.Fatalf("dollar upsert must not use INSERT OR REPLACE, got: %q", sql)
	}
	for _, ph := range []string{"$1", "$2", "$3", "$4", "$5"} {
		if !strings.Contains(sql, ph) {
			t.Fatalf("dollar upsert missing placeholder %q: %q", ph, sql)
		}
	}
	for _, col := range []string{"job_id", "batch_id", "status", "summary_json", "raw_json"} {
		if !strings.Contains(sql, col) {
			t.Fatalf("dollar upsert missing column %q: %q", col, sql)
		}
	}
}

func TestUpsertResultSQLDialectDifference(t *testing.T) {
	// The two dialects must produce different SQL.
	sqlit := sqliteDialect{}.UpsertResultSQL()
	dollar := testDollarDialect{}.UpsertResultSQL()
	if sqlit == dollar {
		t.Fatalf("sqlite and dollar dialects produced identical UpsertResultSQL")
	}
}

// ---- Dialect: IsDuplicateKey -----------------------------------------------

func TestIsDuplicateKeySQLite(t *testing.T) {
	d := sqliteDialect{}

	uniqueErr := fmt.Errorf("UNIQUE constraint failed: jobs.id")
	if !d.IsDuplicateKey(uniqueErr) {
		t.Fatal("expected true for UNIQUE constraint error")
	}

	otherErr := fmt.Errorf("database is locked")
	if d.IsDuplicateKey(otherErr) {
		t.Fatal("expected false for unrelated error")
	}
}

func TestIsDuplicateKeyDollar(t *testing.T) {
	d := testDollarDialect{}

	dupErr := fmt.Errorf(`pq: duplicate key value violates unique constraint "jobs_pkey"`)
	if !d.IsDuplicateKey(dupErr) {
		t.Fatal("expected true for duplicate key error")
	}

	otherErr := fmt.Errorf("connection refused")
	if d.IsDuplicateKey(otherErr) {
		t.Fatal("expected false for unrelated error")
	}
}

func TestIsDuplicateKeyDialectDifference(t *testing.T) {
	// The SQLite error string must not be detected as a duplicate by the dollar
	// dialect (and vice versa), confirming each dialect checks its own format.
	sqliteErr := fmt.Errorf("UNIQUE constraint failed: jobs.id")
	if (testDollarDialect{}).IsDuplicateKey(sqliteErr) {
		t.Fatal("dollar dialect should not match SQLite UNIQUE error")
	}

	pgErr := fmt.Errorf(`duplicate key value violates unique constraint "jobs_pkey"`)
	if (sqliteDialect{}).IsDuplicateKey(pgErr) {
		t.Fatal("sqlite dialect should not match PostgreSQL duplicate key error")
	}
}

// ---- postgresDialect -------------------------------------------------------

// Compile-time interface check.
var _ sqlDialect = postgresDialect{}

func TestPostgresDialectPlaceholder(t *testing.T) {
	d := postgresDialect{}
	for _, tc := range []struct{ n int; want string }{
		{1, "$1"}, {2, "$2"}, {10, "$10"}, {15, "$15"},
	} {
		if got := d.Placeholder(tc.n); got != tc.want {
			t.Errorf("Placeholder(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestPostgresDialectTimestampVal(t *testing.T) {
	d := postgresDialect{}
	if v := d.TimestampVal(time.Time{}); v != nil {
		t.Fatalf("zero time should return nil, got %v", v)
	}
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	v := d.TimestampVal(ts)
	s, ok := v.(string)
	if !ok {
		t.Fatalf("non-zero time should return string, got %T", v)
	}
	if !strings.HasPrefix(s, "2026-01-02T03:04:05") {
		t.Fatalf("unexpected timestamp format: %q", s)
	}
}

func TestPostgresDialectUpsertResultSQL(t *testing.T) {
	sql := postgresDialect{}.UpsertResultSQL()
	if strings.Contains(sql, "INSERT OR REPLACE") {
		t.Fatal("postgres upsert must not use INSERT OR REPLACE")
	}
	if !strings.Contains(sql, "ON CONFLICT") {
		t.Fatalf("postgres upsert must use ON CONFLICT, got: %q", sql)
	}
	for _, ph := range []string{"$1", "$2", "$3", "$4", "$5"} {
		if !strings.Contains(sql, ph) {
			t.Fatalf("postgres upsert missing placeholder %q: %q", ph, sql)
		}
	}
	for _, col := range []string{"job_id", "batch_id", "status", "summary_json", "raw_json"} {
		if !strings.Contains(sql, col) {
			t.Fatalf("postgres upsert missing column %q: %q", col, sql)
		}
	}
}

func TestPostgresDialectIsDuplicateKey(t *testing.T) {
	d := postgresDialect{}

	// Exact pq error with code 23505.
	dupErr := &pq.Error{Code: "23505"}
	if !d.IsDuplicateKey(dupErr) {
		t.Fatal("expected true for pq.Error code 23505")
	}

	// Wrapped pq error must also match.
	wrappedErr := fmt.Errorf("db op failed: %w", &pq.Error{Code: "23505"})
	if !d.IsDuplicateKey(wrappedErr) {
		t.Fatal("expected true for wrapped pq.Error code 23505")
	}

	// A pq error with a different code must not match.
	syntaxErr := &pq.Error{Code: "42601"} // syntax_error
	if d.IsDuplicateKey(syntaxErr) {
		t.Fatal("expected false for pq.Error with non-23505 code")
	}

	// Plain (non-pq) errors must not match, even if the text looks right.
	plainErr := fmt.Errorf("duplicate key value violates unique constraint")
	if d.IsDuplicateKey(plainErr) {
		t.Fatal("expected false for plain error without pq.Error type")
	}

	// SQLite-style error must not match.
	sqliteErr := fmt.Errorf("UNIQUE constraint failed: jobs.id")
	if d.IsDuplicateKey(sqliteErr) {
		t.Fatal("postgres dialect should not match SQLite UNIQUE error")
	}
}

func TestPostgresDialectMatchesTestDollarDialect(t *testing.T) {
	// postgresDialect and testDollarDialect must agree on placeholder style
	// since testDollarDialect is used as a stand-in for postgres in other tests.
	pg := postgresDialect{}
	td := testDollarDialect{}
	for _, n := range []int{1, 5, 10, 19} {
		if pg.Placeholder(n) != td.Placeholder(n) {
			t.Errorf("Placeholder(%d): postgres=%q dollar=%q", n, pg.Placeholder(n), td.Placeholder(n))
		}
	}
}

// ---- dialectFor ------------------------------------------------------------

func TestDialectFor(t *testing.T) {
	tests := []struct {
		driver   string
		wantErr  bool
		wantType string
	}{
		{"sqlite",   false, "sqliteDialect"},
		{"postgres", false, "postgresDialect"},
		{"mariadb",  false, "mariadbDialect"},
		{"mysql",    true,  ""},
		{"mongodb",  true,  ""},
		{"",         true,  ""},
	}
	for _, tc := range tests {
		t.Run(tc.driver, func(t *testing.T) {
			d, err := dialectFor(tc.driver)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("dialectFor(%q): expected error, got dialect %T", tc.driver, d)
				}
				return
			}
			if err != nil {
				t.Fatalf("dialectFor(%q): unexpected error: %v", tc.driver, err)
			}
			if d == nil {
				t.Fatalf("dialectFor(%q): returned nil dialect", tc.driver)
			}
			switch tc.wantType {
			case "sqliteDialect":
				if _, ok := d.(sqliteDialect); !ok {
					t.Errorf("dialectFor(%q) = %T, want sqliteDialect", tc.driver, d)
				}
			case "postgresDialect":
				if _, ok := d.(postgresDialect); !ok {
					t.Errorf("dialectFor(%q) = %T, want postgresDialect", tc.driver, d)
				}
			case "mariadbDialect":
				if _, ok := d.(mariadbDialect); !ok {
					t.Errorf("dialectFor(%q) = %T, want mariadbDialect", tc.driver, d)
				}
			}
		})
	}
}

func TestDialectForMariadbDriverName(t *testing.T) {
	// "mariadb" is the user-facing name; the underlying sql.DB driver is "mysql".
	d, err := dialectFor("mariadb")
	if err != nil {
		t.Fatalf("dialectFor(\"mariadb\"): %v", err)
	}
	if got := d.DriverName(); got != "mysql" {
		t.Errorf("DriverName() = %q, want \"mysql\"", got)
	}
}

func TestDialectForErrorMessage(t *testing.T) {
	_, err := dialectFor("baddriver")
	if err == nil {
		t.Fatal("expected error for unknown driver")
	}
	if !strings.Contains(err.Error(), "baddriver") {
		t.Errorf("error should name the driver, got: %v", err)
	}
}

// ---- configurePool ---------------------------------------------------------

// openRawSQLite opens a bare in-memory SQLite DB without applying any pool
// config, so tests can call configurePool themselves and inspect the result.
func openRawSQLite(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestConfigurePoolSQLite(t *testing.T) {
	db := openRawSQLite(t)
	configurePool(db, "sqlite")
	if got := db.Stats().MaxOpenConnections; got != 1 {
		t.Errorf("sqlite MaxOpenConnections = %d, want 1", got)
	}
}

func TestConfigurePoolPostgres(t *testing.T) {
	// Use a SQLite DB as the target — configurePool calls Set* methods on
	// *sql.DB directly, so the underlying driver is irrelevant here.
	db := openRawSQLite(t)
	configurePool(db, "postgres")
	if got := db.Stats().MaxOpenConnections; got != 25 {
		t.Errorf("postgres MaxOpenConnections = %d, want 25", got)
	}
}

func TestConfigurePoolMySQL(t *testing.T) {
	db := openRawSQLite(t)
	configurePool(db, "mysql")
	if got := db.Stats().MaxOpenConnections; got != 25 {
		t.Errorf("mysql MaxOpenConnections = %d, want 25", got)
	}
}

func TestConfigurePoolMySQLMatchesPostgres(t *testing.T) {
	// MySQL and PostgreSQL should have identical pool defaults.
	pgDB := openRawSQLite(t)
	myDB := openRawSQLite(t)
	configurePool(pgDB, "postgres")
	configurePool(myDB, "mysql")
	if pgDB.Stats().MaxOpenConnections != myDB.Stats().MaxOpenConnections {
		t.Errorf("mysql MaxOpenConnections (%d) differs from postgres (%d)",
			myDB.Stats().MaxOpenConnections, pgDB.Stats().MaxOpenConnections)
	}
}

func TestConfigurePoolUnknownDriverNoChange(t *testing.T) {
	db := openRawSQLite(t)
	// Record default (0 = unlimited in sql.DB).
	before := db.Stats().MaxOpenConnections
	configurePool(db, "unknown")
	if got := db.Stats().MaxOpenConnections; got != before {
		t.Errorf("unknown driver changed MaxOpenConnections to %d", got)
	}
}

// ---- mariadbDialect --------------------------------------------------------

// Compile-time interface check.
var _ sqlDialect = mariadbDialect{}

func TestMariadbDialectPlaceholder(t *testing.T) {
	d := mariadbDialect{}
	// MariaDB uses "?" for all positions, same as SQLite.
	for _, n := range []int{1, 2, 10, 19} {
		if got := d.Placeholder(n); got != "?" {
			t.Errorf("Placeholder(%d) = %q, want \"?\"", n, got)
		}
	}
}

func TestMariadbDialectPlaceholderMatchesSQLite(t *testing.T) {
	md := mariadbDialect{}
	sd := sqliteDialect{}
	for _, n := range []int{1, 5, 10} {
		if md.Placeholder(n) != sd.Placeholder(n) {
			t.Errorf("Placeholder(%d): mariadb=%q sqlite=%q", n, md.Placeholder(n), sd.Placeholder(n))
		}
	}
}

func TestMariadbDialectTimestampVal(t *testing.T) {
	d := mariadbDialect{}
	if v := d.TimestampVal(time.Time{}); v != nil {
		t.Fatalf("zero time should return nil, got %v", v)
	}
	ts := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	v := d.TimestampVal(ts)
	s, ok := v.(string)
	if !ok {
		t.Fatalf("non-zero time should return string, got %T", v)
	}
	if !strings.HasPrefix(s, "2026-03-15T12:00:00") {
		t.Fatalf("unexpected timestamp format: %q", s)
	}
}

func TestMariadbDialectUpsertResultSQL(t *testing.T) {
	sql := mariadbDialect{}.UpsertResultSQL()
	if strings.Contains(sql, "INSERT OR REPLACE") {
		t.Fatal("mariadb upsert must not use INSERT OR REPLACE")
	}
	if strings.Contains(sql, "ON CONFLICT") {
		t.Fatal("mariadb upsert must not use ON CONFLICT (that is PostgreSQL syntax)")
	}
	if !strings.Contains(sql, "ON DUPLICATE KEY UPDATE") {
		t.Fatalf("mariadb upsert must use ON DUPLICATE KEY UPDATE, got: %q", sql)
	}
	if count := strings.Count(sql, "?"); count != 5 {
		t.Fatalf("mariadb upsert must have 5 '?' placeholders, got %d: %q", count, sql)
	}
	for _, col := range []string{"job_id", "batch_id", "status", "summary_json", "raw_json"} {
		if !strings.Contains(sql, col) {
			t.Fatalf("mariadb upsert missing column %q: %q", col, sql)
		}
	}
}

func TestMariadbDialectIsDuplicateKey(t *testing.T) {
	d := mariadbDialect{}

	// Exact mysql error with number 1062.
	dupErr := &mysql.MySQLError{Number: 1062, Message: "Duplicate entry 'abc' for key 'PRIMARY'"}
	if !d.IsDuplicateKey(dupErr) {
		t.Fatal("expected true for MySQLError number 1062")
	}

	// Wrapped mysql error must also match.
	wrappedErr := fmt.Errorf("db op failed: %w", &mysql.MySQLError{Number: 1062})
	if !d.IsDuplicateKey(wrappedErr) {
		t.Fatal("expected true for wrapped MySQLError number 1062")
	}

	// A mysql error with a different number must not match.
	otherMysqlErr := &mysql.MySQLError{Number: 1045} // access denied
	if d.IsDuplicateKey(otherMysqlErr) {
		t.Fatal("expected false for MySQLError with non-1062 number")
	}

	// Plain (non-mysql) errors must not match, even if the text looks right.
	plainErr := fmt.Errorf("Duplicate entry 'abc' for key 'PRIMARY'")
	if d.IsDuplicateKey(plainErr) {
		t.Fatal("expected false for plain error without MySQLError type")
	}

	// SQLite-style error must not match.
	sqliteErr := fmt.Errorf("UNIQUE constraint failed: jobs.id")
	if d.IsDuplicateKey(sqliteErr) {
		t.Fatal("mariadb dialect should not match SQLite UNIQUE error")
	}
}

// ---- mariadbDSN ------------------------------------------------------------

func TestMariadbDSNNoParams(t *testing.T) {
	in := "user:pass@tcp(host:3306)/dbname"
	got := mariadbDSN(in)
	want := "user:pass@tcp(host:3306)/dbname?parseTime=true"
	if got != want {
		t.Errorf("mariadbDSN(%q) = %q, want %q", in, got, want)
	}
}

func TestMariadbDSNExistingParams(t *testing.T) {
	in := "user:pass@tcp(host:3306)/dbname?charset=utf8mb4"
	got := mariadbDSN(in)
	want := "user:pass@tcp(host:3306)/dbname?charset=utf8mb4&parseTime=true"
	if got != want {
		t.Errorf("mariadbDSN(%q) = %q, want %q", in, got, want)
	}
}

func TestMariadbDSNAlreadyHasParseTime(t *testing.T) {
	cases := []string{
		"user:pass@tcp(host:3306)/dbname?parseTime=true",
		"user:pass@tcp(host:3306)/dbname?parseTime=false",
		"user:pass@tcp(host:3306)/dbname?charset=utf8mb4&parseTime=true",
	}
	for _, in := range cases {
		got := mariadbDSN(in)
		if got != in {
			t.Errorf("mariadbDSN(%q) = %q, want unchanged", in, got)
		}
	}
}

func TestMariadbDSNContainsParseTimeTrue(t *testing.T) {
	dsn := mariadbDSN("user:pass@tcp(host:3306)/dbname")
	if !strings.Contains(dsn, "parseTime=true") {
		t.Errorf("expected parseTime=true in %q", dsn)
	}
}
