package server

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
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

func (d *spyDialect) Placeholder(n int) string      { d.placeholderCalls++; return d.inner.Placeholder(n) }
func (d *spyDialect) TimestampVal(t time.Time) any  { return d.inner.TimestampVal(t) }
func (d *spyDialect) DriverName() string            { return d.inner.DriverName() }
func (d *spyDialect) IsDuplicateKey(err error) bool { return d.inner.IsDuplicateKey(err) }
func (d *spyDialect) IsRetryableConflict(err error) bool {
	return d.inner.IsRetryableConflict(err)
}
func (d *spyDialect) SupportsOnConflictReturning() bool {
	return d.inner.SupportsOnConflictReturning()
}
func (d *spyDialect) Least(a, b string) string    { return d.inner.Least(a, b) }
func (d *spyDialect) Greatest(a, b string) string { return d.inner.Greatest(a, b) }

// testDollarDialect simulates PostgreSQL's $n placeholder style for testing.
type testDollarDialect struct{}

func (testDollarDialect) Placeholder(n int) string     { return fmt.Sprintf("$%d", n) }
func (testDollarDialect) TimestampVal(t time.Time) any { return sqliteDialect{}.TimestampVal(t) }
func (testDollarDialect) DriverName() string           { return "test-dollar" }
func (testDollarDialect) IsDuplicateKey(err error) bool {
	return strings.Contains(err.Error(), "duplicate key value violates unique constraint")
}
func (testDollarDialect) IsRetryableConflict(err error) bool {
	return postgresDialect{}.IsRetryableConflict(err)
}
func (testDollarDialect) SupportsOnConflictReturning() bool { return true }
func (testDollarDialect) Least(a, b string) string          { return fmt.Sprintf("LEAST(%s, %s)", a, b) }
func (testDollarDialect) Greatest(a, b string) string       { return fmt.Sprintf("GREATEST(%s, %s)", a, b) }

// testBackend describes a database backend for parameterized store tests.
type testBackend struct {
	name    string // human-readable name used in t.Run
	driver  string // sql.DB driver name ("sqlite", "postgres", "mysql")
	dsn     string // data source name
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

// resetSchema drops every application table so tests start from a clean state
// on persistent backends (PostgreSQL, MariaDB). It must list every table the
// migrations create - a table left behind here keeps its old schema, so a
// later ALTER migration re-run fails with "column already exists". Keep this
// in sync with the CREATE TABLE statements in store_sql_migrate.go.
func resetSchema(db *sql.DB) error {
	for _, tbl := range []string{
		"analysis_cohort_snapshot_aggregates", // legacy, pre-view-table name
		"analysis_snapshot_overview_view",
		"analysis_snapshot_nameserver_view",
		"analysis_snapshot_endpoint_view",
		"analysis_snapshot_asn_view",
		"analysis_snapshot_domain_view",
		"analysis_snapshot_prefix_view",
		"analysis_snapshot_tag_view",
		"analysis_cohort_snapshots",
		"analysis_projection_state",
		"analysis_run_domain_summary",
		"analysis_run_domain_facts",
		"analysis_run_tag_summary",
		"analysis_run_domain_asns",
		"analysis_run_address_asns",
		"analysis_run_ns_endpoints",
		"analysis_asns",
		"analysis_prefixes",
		"analysis_addresses",
		"analysis_nameservers",
		"analysis_cohort_catalog",
		"run_dnssec_chain",
		"entries", "runs", "domain_tags", "domains", "tags", "jobs", "batches", "profiles", "settings", "schema_migrations",
	} {
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

// forEachBackend runs fn as a subtest against every configured backend, each
// with a freshly migrated store. SQLite always runs; PostgreSQL and MariaDB
// join when TEST_POSTGRES_DSN / TEST_MARIADB_DSN are set.
func forEachBackend(t *testing.T, fn func(t *testing.T, s *SQLJobStore)) {
	t.Helper()
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			fn(t, testStoreForBackend(t, b))
		})
	}
}

// newSQLTestServer returns a server backed by store, for the handler tests that
// need a real database rather than the in-memory job store.
func newSQLTestServer(t *testing.T, store *SQLJobStore) *Server {
	t.Helper()
	return newTestServer(t, withDB(store))
}

// ids extracts job IDs from a slice for readable error messages.
func ids(jobs []Job) []string {
	out := make([]string, len(jobs))
	for i, j := range jobs {
		out[i] = j.ID
	}
	return out
}

// graduateSQLJob is a test helper that graduates a job with the given entries.
func graduateSQLJob(t *testing.T, s *SQLJobStore, job Job, entries []engine.LogEntry) {
	t.Helper()
	graduate(t, s, job, entries)
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
	// Use a sentinel DSN - we only test inclusion, not connectivity.
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

	for _, tbl := range []string{
		"jobs", "runs", "entries", "domains", "profiles", "settings", "schema_migrations",
		"analysis_cohort_catalog", "analysis_nameservers", "analysis_addresses",
		"analysis_prefixes", "analysis_asns", "analysis_run_ns_endpoints",
		"analysis_run_address_asns", "analysis_run_domain_summary", "analysis_projection_state",
	} {
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

// TestResetSchemaAllowsMigrationRerun reproduces the persistent-backend reset
// cycle (drop everything, re-run migrations) on a single shared in-memory
// SQLite DB. If resetSchema leaves a table behind, that table keeps its old
// schema and a later ALTER migration re-run fails with a duplicate-column
// error - the exact failure the v5 latency columns hit on PostgreSQL/MariaDB
// because the snapshot view tables were missing from resetSchema.
func TestResetSchemaAllowsMigrationRerun(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// A single connection keeps the :memory: database alive across the reset
	// so leftover tables would persist, just like a real shared backend.
	db.SetMaxOpenConns(1)
	defer db.Close()

	if err := runMigrations(db, sqliteDialect{}); err != nil {
		t.Fatalf("first runMigrations: %v", err)
	}
	if err := resetSchema(db); err != nil {
		t.Fatalf("resetSchema: %v", err)
	}
	if err := runMigrations(db, sqliteDialect{}); err != nil {
		t.Fatalf("runMigrations after reset must succeed on a fully dropped schema: %v", err)
	}
}

func TestRunMigrationsRecordsVersion(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	if err := runMigrations(db, sqliteDialect{}); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}

	rows, err := db.Query(`SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	defer rows.Close()
	var versions []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("scan: %v", err)
		}
		versions = append(versions, v)
	}
	want := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	if len(versions) != len(want) {
		t.Fatalf("expected %d versions, got %d: %v", len(want), len(versions), versions)
	}
	for i, v := range want {
		if versions[i] != v {
			t.Fatalf("expected versions %v, got %v", want, versions)
		}
	}
}

func TestRunMigrationsRenamesBailiwickTags(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	if err := runMigrations(db, sqliteDialect{}); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}

	// Re-arm the rename so it sees the legacy rows inserted below, the way it
	// would on a database written before the rename landed.
	if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version = 7`); err != nil {
		t.Fatalf("reset migration 7: %v", err)
	}

	insert := func(runID, tag string) {
		t.Helper()
		if _, err := db.Exec(
			`INSERT INTO entries (run_id, domain_id, timestamp, module, testcase, tag, level, args_json)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			runID, 1, 0.0, "Consistency", "Consistency05", tag, "ERROR", "{}",
		); err != nil {
			t.Fatalf("insert %s: %v", tag, err)
		}
	}

	legacy := map[string]string{
		"IN_BAILIWICK_ADDR_MISMATCH":     "IN_DOMAIN_ADDR_MISMATCH",
		"OUT_OF_BAILIWICK_ADDR_MISMATCH": "NOT_IN_DOMAIN_ADDR_MISMATCH",
		"IN_BAILIWICK_GLUE_MISSING":      "IN_DOMAIN_GLUE_MISSING",
	}
	i := 0
	for old := range legacy {
		insert(fmt.Sprintf("job_legacy_%d", i), old)
		i++
	}
	// An unrelated tag must survive the backfill untouched.
	insert("job_untouched", "ADDRESSES_MATCH")

	if err := runMigrations(db, sqliteDialect{}); err != nil {
		t.Fatalf("rerun migrations: %v", err)
	}

	count := func(tag string) int {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM entries WHERE tag = ?`, tag).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", tag, err)
		}
		return n
	}

	for old, renamed := range legacy {
		if got := count(old); got != 0 {
			t.Fatalf("legacy tag %s still present in %d rows", old, got)
		}
		if got := count(renamed); got != 1 {
			t.Fatalf("renamed tag %s: got %d rows, want 1", renamed, got)
		}
	}
	if got := count("ADDRESSES_MATCH"); got != 1 {
		t.Fatalf("unrelated tag ADDRESSES_MATCH: got %d rows, want 1", got)
	}
}

// The analysis layer keeps its own copies of a message tag, so migration 7
// alone left every snapshot captured before the rename serving the retired
// name: the tag page for the current name found nothing.
func TestRunMigrationsRenamesBailiwickTagsInAnalysisViews(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	if err := runMigrations(db, sqliteDialect{}); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}

	// Re-arm the rename so it sees the legacy rows inserted below, the way it
	// would on a database materialized before the rename landed.
	if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version = 9`); err != nil {
		t.Fatalf("reset migration 9: %v", err)
	}

	tagView := func(snapshotID int64, tag string, domainCount int) {
		t.Helper()
		if _, err := db.Exec(
			`INSERT INTO analysis_snapshot_tag_view
			   (snapshot_id, tag, module, testcase, level, domain_count, occurrence_count, domains_json)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			snapshotID, tag, "Consistency", "Consistency05", "ERROR", domainCount, domainCount, `["example.test"]`,
		); err != nil {
			t.Fatalf("insert tag view %s: %v", tag, err)
		}
	}
	runSummary := func(runID, tag string) {
		t.Helper()
		if _, err := db.Exec(
			`INSERT INTO analysis_run_tag_summary
			   (cohort_id, run_id, domain_id, tag, module, testcase, level, occurrence_count)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			1, runID, 1, tag, "Consistency", "Consistency05", "ERROR", 1,
		); err != nil {
			t.Fatalf("insert run summary %s: %v", tag, err)
		}
	}

	legacy := [][2]string{
		{"IN_BAILIWICK_ADDR_MISMATCH", "IN_DOMAIN_ADDR_MISMATCH"},
		{"OUT_OF_BAILIWICK_ADDR_MISMATCH", "NOT_IN_DOMAIN_ADDR_MISMATCH"},
		{"IN_BAILIWICK_GLUE_MISSING", "IN_DOMAIN_GLUE_MISSING"},
	}
	for i, pair := range legacy {
		tagView(1, pair[0], 7)
		runSummary(fmt.Sprintf("run_legacy_%d", i), pair[0])
	}
	// An unrelated tag must survive the rewrite untouched.
	tagView(1, "ADDRESSES_MATCH", 3)
	runSummary("run_untouched", "ADDRESSES_MATCH")

	if _, err := db.Exec(
		`INSERT INTO analysis_snapshot_domain_view
		   (snapshot_id, domain_id, domain_name, score, grade, worst_level, tags_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		1, 1, "example.test", 50, "C", "ERROR",
		`[{"tag":"OUT_OF_BAILIWICK_ADDR_MISMATCH","level":"ERROR"},{"tag":"ADDRESSES_MATCH","level":"INFO"}]`,
	); err != nil {
		t.Fatalf("insert domain view: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO analysis_snapshot_overview_view (snapshot_id, domain_count, top_tags_json)
		 VALUES (?, ?, ?)`,
		1, 1, `[{"tag":"IN_BAILIWICK_GLUE_MISSING","domain_count":7}]`,
	); err != nil {
		t.Fatalf("insert overview view: %v", err)
	}

	if err := runMigrations(db, sqliteDialect{}); err != nil {
		t.Fatalf("rerun migrations: %v", err)
	}

	count := func(query, tag string) int {
		t.Helper()
		var n int
		if err := db.QueryRow(query, tag).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", tag, err)
		}
		return n
	}
	const countTagView = `SELECT COUNT(*) FROM analysis_snapshot_tag_view WHERE tag = ?`
	const countRunSummary = `SELECT COUNT(*) FROM analysis_run_tag_summary WHERE tag = ?`

	for _, pair := range legacy {
		old, current := pair[0], pair[1]
		if got := count(countTagView, old); got != 0 {
			t.Fatalf("tag view still holds %s in %d rows", old, got)
		}
		if got := count(countTagView, current); got != 1 {
			t.Fatalf("tag view %s: got %d rows, want 1", current, got)
		}
		if got := count(countRunSummary, old); got != 0 {
			t.Fatalf("run summary still holds %s in %d rows", old, got)
		}
		if got := count(countRunSummary, current); got != 1 {
			t.Fatalf("run summary %s: got %d rows, want 1", current, got)
		}
	}
	if got := count(countTagView, "ADDRESSES_MATCH"); got != 1 {
		t.Fatalf("unrelated tag view row: got %d, want 1", got)
	}
	if got := count(countRunSummary, "ADDRESSES_MATCH"); got != 1 {
		t.Fatalf("unrelated run summary row: got %d, want 1", got)
	}

	// The counts must ride along with the rename, not reset.
	var domainCount int
	if err := db.QueryRow(
		`SELECT domain_count FROM analysis_snapshot_tag_view WHERE tag = ?`,
		"IN_DOMAIN_ADDR_MISMATCH",
	).Scan(&domainCount); err != nil {
		t.Fatalf("read renamed domain_count: %v", err)
	}
	if domainCount != 7 {
		t.Fatalf("renamed tag domain_count: got %d, want 7", domainCount)
	}

	var tagsJSON, topTagsJSON string
	if err := db.QueryRow(`SELECT tags_json FROM analysis_snapshot_domain_view`).Scan(&tagsJSON); err != nil {
		t.Fatalf("read tags_json: %v", err)
	}
	if strings.Contains(tagsJSON, "BAILIWICK") {
		t.Fatalf("tags_json still names a retired tag: %s", tagsJSON)
	}
	if !strings.Contains(tagsJSON, `"NOT_IN_DOMAIN_ADDR_MISMATCH"`) {
		t.Fatalf("tags_json missing the renamed tag: %s", tagsJSON)
	}
	if !strings.Contains(tagsJSON, `"ADDRESSES_MATCH"`) {
		t.Fatalf("tags_json dropped an unrelated tag: %s", tagsJSON)
	}
	if err := db.QueryRow(`SELECT top_tags_json FROM analysis_snapshot_overview_view`).Scan(&topTagsJSON); err != nil {
		t.Fatalf("read top_tags_json: %v", err)
	}
	if !strings.Contains(topTagsJSON, `"IN_DOMAIN_GLUE_MISSING"`) {
		t.Fatalf("top_tags_json missing the renamed tag: %s", topTagsJSON)
	}
}

// A batch that spanned the rename leaves both names in one snapshot, which
// would collide on the tag view's (snapshot_id, tag) key.
func TestRunMigrationsDropsRetiredTagOnRenameCollision(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	if err := runMigrations(db, sqliteDialect{}); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version = 9`); err != nil {
		t.Fatalf("reset migration 9: %v", err)
	}

	insert := func(snapshotID int64, tag string, domainCount int) {
		t.Helper()
		if _, err := db.Exec(
			`INSERT INTO analysis_snapshot_tag_view
			   (snapshot_id, tag, module, testcase, level, domain_count, occurrence_count, domains_json)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			snapshotID, tag, "Consistency", "Consistency05", "ERROR", domainCount, domainCount, `[]`,
		); err != nil {
			t.Fatalf("insert %s: %v", tag, err)
		}
	}
	insert(1, "OUT_OF_BAILIWICK_ADDR_MISMATCH", 4)
	insert(1, "NOT_IN_DOMAIN_ADDR_MISMATCH", 9)
	// A different snapshot holding only the retired name still renames.
	insert(2, "OUT_OF_BAILIWICK_ADDR_MISMATCH", 5)

	if err := runMigrations(db, sqliteDialect{}); err != nil {
		t.Fatalf("rerun migrations: %v", err)
	}

	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM analysis_snapshot_tag_view WHERE tag = ?`,
		"OUT_OF_BAILIWICK_ADDR_MISMATCH",
	).Scan(&count); err != nil {
		t.Fatalf("count retired: %v", err)
	}
	if count != 0 {
		t.Fatalf("retired tag survived in %d rows", count)
	}

	var domainCount int
	if err := db.QueryRow(
		`SELECT domain_count FROM analysis_snapshot_tag_view WHERE snapshot_id = 1 AND tag = ?`,
		"NOT_IN_DOMAIN_ADDR_MISMATCH",
	).Scan(&domainCount); err != nil {
		t.Fatalf("read surviving row: %v", err)
	}
	if domainCount != 9 {
		t.Fatalf("collision kept the wrong row: domain_count %d, want 9", domainCount)
	}
	if err := db.QueryRow(
		`SELECT domain_count FROM analysis_snapshot_tag_view WHERE snapshot_id = 2 AND tag = ?`,
		"NOT_IN_DOMAIN_ADDR_MISMATCH",
	).Scan(&domainCount); err != nil {
		t.Fatalf("read renamed row on the uncontested snapshot: %v", err)
	}
	if domainCount != 5 {
		t.Fatalf("uncontested snapshot domain_count: got %d, want 5", domainCount)
	}
}

func TestRunMigrationsAddsProfileEditingColumns(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	if err := runMigrations(db, sqliteDialect{}); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}

	for _, tc := range []struct {
		table  string
		column string
	}{
		{table: "tags", column: "default_profile_id"},
		{table: "jobs", column: "profile_id"},
		{table: "jobs", column: "profile_name"},
		{table: "runs", column: "profile_id"},
		{table: "runs", column: "profile_name"},
		{table: "runs", column: "effective_profile"},
	} {
		var name string
		if err := db.QueryRow(
			fmt.Sprintf(`SELECT name FROM pragma_table_info('%s') WHERE name = ?`, tc.table),
			tc.column,
		).Scan(&name); err != nil {
			t.Fatalf("%s.%s not found after migration: %v", tc.table, tc.column, err)
		}
		if name != tc.column {
			t.Fatalf("%s column mismatch: got %q, want %q", tc.table, name, tc.column)
		}
	}
}

func TestRunMigrationsAddsNameserverTimingsColumn(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	if err := runMigrations(db, sqliteDialect{}); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}

	var name string
	if err := db.QueryRow(
		`SELECT name FROM pragma_table_info('runs') WHERE name = ?`,
		"nameserver_timings_json",
	).Scan(&name); err != nil {
		t.Fatalf("runs.nameserver_timings_json not found after migration: %v", err)
	}
	if name != "nameserver_timings_json" {
		t.Fatalf("column mismatch: got %q", name)
	}
}

func TestRunMigrationsAddsAnalysisTables(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	if err := runMigrations(db, sqliteDialect{}); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}

	for _, tc := range []struct {
		table   string
		columns []string
	}{
		{
			table: "analysis_cohort_catalog",
			columns: []string{
				"source_type", "source_tag", "analysis_enabled", "public_enabled",
				"is_default", "materialization_status", "last_materialized_at",
				"last_materialization_error",
			},
		},
		{
			table:   "analysis_run_ns_endpoints",
			columns: []string{"cohort_id", "run_id", "domain_id", "nameserver_id", "address_id"},
		},
		{
			table:   "analysis_run_address_asns",
			columns: []string{"cohort_id", "run_id", "domain_id", "address_id", "prefix_id", "asn"},
		},
		{
			table:   "analysis_run_domain_summary",
			columns: []string{"cohort_id", "run_id", "domain_id", "score", "grade", "worst_level"},
		},
		{
			table:   "analysis_projection_state",
			columns: []string{"cohort_id", "run_id", "projector_version", "status", "projected_at", "error"},
		},
	} {
		for _, col := range tc.columns {
			var name string
			if err := db.QueryRow(
				fmt.Sprintf(`SELECT name FROM pragma_table_info('%s') WHERE name = ?`, tc.table),
				col,
			).Scan(&name); err != nil {
				t.Fatalf("%s.%s not found after migration: %v", tc.table, col, err)
			}
			if name != col {
				t.Fatalf("%s column mismatch: got %q, want %q", tc.table, name, col)
			}
		}
	}
}

// ---- Create / Get ----------------------------------------------------------

func TestSQLJobStoreCreateGet(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
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
	})
}

func TestSQLJobStoreCreateDuplicate(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		job := Job{ID: "dup", Domain: "x.com", Status: JobQueued, CreatedAt: time.Now().UTC()}
		if _, err := s.Create(job); err != nil {
			t.Fatalf("first Create: %v", err)
		}
		if _, err := s.Create(job); err == nil {
			t.Fatal("expected error on duplicate Create")
		}
	})
}

func TestSQLJobStoreGetMissing(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		_, ok := s.Get("does-not-exist")
		if ok {
			t.Fatal("expected ok=false for missing job")
		}
	})
}

// ---- Update ----------------------------------------------------------------

func TestSQLJobStoreUpdate(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
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

func TestSQLJobStoreUpdateMissing(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		err := s.Update(Job{ID: "ghost", Status: JobRunning, CreatedAt: time.Now().UTC()})
		if err == nil {
			t.Fatal("expected error updating missing job")
		}
	})
}

// ---- GraduateJob / GetResult -----------------------------------------------

func TestSQLJobStoreGraduateJobAndGetResult(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		now := time.Now().UTC().Truncate(time.Millisecond)

		job := Job{
			ID:         "g1",
			Domain:     "grad.test",
			BatchID:    "batch1",
			Status:     JobSucceeded,
			CreatedAt:  now,
			StartedAt:  now.Add(time.Second),
			FinishedAt: now.Add(2 * time.Second),
			PublicID:   "pub00001",
			NameserverTimings: []NameserverTiming{
				{
					Nameserver: "ns1.grad.test",
					Address:    "192.0.2.10",
					AvgMS:      24,
					MinMS:      20,
					MaxMS:      30,
					MedianMS:   22,
					StddevMS:   4,
					Count:      3,
				},
			},
		}
		if _, err := s.Create(job); err != nil {
			t.Fatalf("Create: %v", err)
		}

		entries := []engine.LogEntry{
			{Module: "DNSSEC", Testcase: "DNSSEC01", Tag: "DS_ALGO_OK", Level: "INFO"},
			{Module: "DNSSEC", Testcase: "DNSSEC01", Tag: "NO_NSEC", Level: "WARNING"},
			{Module: "Zone", Testcase: "ZONE01", Tag: "ZONE_ERROR", Level: "ERROR"},
		}

		if err := s.GraduateJob(job, entries); err != nil {
			t.Fatalf("GraduateJob: %v", err)
		}

		// Run must exist.
		run, ok := s.GetRun(job.ID)
		if !ok {
			t.Fatal("GetRun: run not found after graduation")
		}
		if run.Domain != "grad.test" {
			t.Fatalf("run.Domain = %q", run.Domain)
		}
		if run.Status != JobSucceeded {
			t.Fatalf("run.Status = %q", run.Status)
		}
		if run.SevWarning != 1 {
			t.Fatalf("SevWarning = %d, want 1", run.SevWarning)
		}
		if run.SevError != 1 {
			t.Fatalf("SevError = %d, want 1", run.SevError)
		}
		if run.WorstLevel != "ERROR" {
			t.Fatalf("WorstLevel = %q, want ERROR", run.WorstLevel)
		}
		if run.EntryCount != 3 {
			t.Fatalf("EntryCount = %d, want 3", run.EntryCount)
		}
		if len(run.NameserverTimings) != 1 {
			t.Fatalf("run.NameserverTimings len = %d, want 1", len(run.NameserverTimings))
		}
		if run.NameserverTimings[0].Nameserver != "ns1.grad.test" {
			t.Fatalf("run nameserver = %q", run.NameserverTimings[0].Nameserver)
		}

		// GetResult must work.
		result, ok := s.GetResult(job.ID)
		if !ok {
			t.Fatal("GetResult: not found")
		}
		if result.JobID != job.ID {
			t.Fatalf("result.JobID = %q", result.JobID)
		}
		if result.Status != JobSucceeded {
			t.Fatalf("result.Status = %q", result.Status)
		}
		if result.Raw == nil {
			t.Fatal("result.Raw is nil")
		}
		if len(result.Raw.Entries) != 3 {
			t.Fatalf("result.Raw.Entries len = %d, want 3", len(result.Raw.Entries))
		}
		if len(result.NameserverTimings) != 1 {
			t.Fatalf("result.NameserverTimings len = %d, want 1", len(result.NameserverTimings))
		}
		if result.NameserverTimings[0].Address != "192.0.2.10" {
			t.Fatalf("result nameserver address = %q", result.NameserverTimings[0].Address)
		}

		// Get via job ID must reconstruct from runs.
		gotJob, ok := s.Get(job.ID)
		if !ok {
			t.Fatal("Get(graduated job): not found")
		}
		if gotJob.Status != JobSucceeded {
			t.Fatalf("reconstructed job.Status = %q", gotJob.Status)
		}

		// GetByPublicID must also work.
		gotByPub, ok := s.GetByPublicID("pub00001")
		if !ok {
			t.Fatal("GetByPublicID: not found")
		}
		if gotByPub.ID != job.ID {
			t.Fatalf("GetByPublicID returned ID %q", gotByPub.ID)
		}
	})
}

func TestSQLJobStoreGetResultMissing(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		_, ok := s.GetResult("no-result")
		if ok {
			t.Fatal("expected ok=false for missing result")
		}
	})
}

// ---- Domain store ----------------------------------------------------------

func TestSQLJobStoreUpdateDomainLatestIncrements(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		d, _ := s.GetOrCreateDomain("counter.example")

		for i := range 3 {
			if err := s.UpdateDomainLatest(d.ID, fmt.Sprintf("run-%d", i), time.Now().UTC(), "succeeded", ""); err != nil {
				t.Fatalf("UpdateDomainLatest %d: %v", i, err)
			}
		}

		got, ok := s.GetDomain(d.ID)
		if !ok {
			t.Fatal("GetDomain: not found")
		}
		if got.RunCount != 3 {
			t.Fatalf("RunCount: got %d, want 3", got.RunCount)
		}
	})
}

// ---- List filters ----------------------------------------------------------

func TestSQLJobStoreListFilters(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
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

func TestSQLJobStoreListFiltersSecondBoundary(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
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

// ---- List runs (severity filters on graduated jobs) ------------------------

func TestSQLJobStoreListRunsSeverityFilter(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		base := time.Now().UTC().Truncate(time.Millisecond)

		for _, id := range []string{"s1", "s2", "s3"} {
			if _, err := s.Create(Job{ID: id, Domain: id + ".test", Status: JobSucceeded, CreatedAt: base}); err != nil {
				t.Fatalf("Create: %v", err)
			}
		}
		// s1: WARNING
		graduateSQLJob(t, s, Job{
			ID: "s1", Domain: "s1.test", Status: JobSucceeded, CreatedAt: base,
		}, []engine.LogEntry{{Level: "WARNING", Module: "M", Tag: "T"}})
		// s2: CRITICAL
		graduateSQLJob(t, s, Job{
			ID: "s2", Domain: "s2.test", Status: JobSucceeded, CreatedAt: base,
		}, []engine.LogEntry{{Level: "CRITICAL", Module: "M", Tag: "T"}})
		// s3: no entries

		t.Run("worst_level_warning", func(t *testing.T) {
			list := s.ListRuns(RunFilter{WorstLevel: "WARNING", Limit: 10})
			if list.Total != 1 || list.Items[0].ID != "s1" {
				t.Fatalf("WorstLevel=WARNING: total=%d items=%v", list.Total, runIDs(list.Items))
			}
		})

		t.Run("worst_level_critical", func(t *testing.T) {
			list := s.ListRuns(RunFilter{WorstLevel: "CRITICAL", Limit: 10})
			if list.Total != 1 || list.Items[0].ID != "s2" {
				t.Fatalf("WorstLevel=CRITICAL: total=%d items=%v", list.Total, runIDs(list.Items))
			}
		})
	})
}

func TestSQLJobStoreListRunsEntryTagFilter(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		base := time.Now().UTC().Truncate(time.Millisecond)

		for _, id := range []string{"e1", "e2"} {
			if _, err := s.Create(Job{ID: id, Domain: id + ".test", Status: JobSucceeded, CreatedAt: base}); err != nil {
				t.Fatalf("Create: %v", err)
			}
		}
		graduateSQLJob(t, s, Job{ID: "e1", Domain: "e1.test", Status: JobSucceeded, CreatedAt: base, FinishedAt: base},
			[]engine.LogEntry{{Level: "ERROR", Module: "DNSSEC", Tag: "DS07_NOT_SIGNED"}})
		graduateSQLJob(t, s, Job{ID: "e2", Domain: "e2.test", Status: JobSucceeded, CreatedAt: base, FinishedAt: base},
			[]engine.LogEntry{{Level: "WARNING", Module: "Consistency", Tag: "CN04_IPV4_SAME_PREFIX"}})

		t.Run("matches by entry tag", func(t *testing.T) {
			list := s.ListRuns(RunFilter{EntryTag: "DS07_NOT_SIGNED", Limit: 10})
			if list.Total != 1 || list.Items[0].ID != "e1" {
				t.Fatalf("EntryTag=DS07_NOT_SIGNED: total=%d items=%v", list.Total, runIDs(list.Items))
			}
		})
		t.Run("unknown tag matches nothing", func(t *testing.T) {
			if list := s.ListRuns(RunFilter{EntryTag: "NOPE", Limit: 10}); list.Total != 0 {
				t.Fatalf("unknown EntryTag should match nothing, got %d", list.Total)
			}
		})
		t.Run("intersect with time window", func(t *testing.T) {
			past := base.Add(-time.Hour)
			if list := s.ListRuns(RunFilter{EntryTag: "DS07_NOT_SIGNED", FinishedBefore: past, Limit: 10}); list.Total != 0 {
				t.Fatalf("EntryTag plus a past finished_before should be empty, got %d", list.Total)
			}
		})
		t.Run("no entry tag returns all", func(t *testing.T) {
			if list := s.ListRuns(RunFilter{Limit: 10}); list.Total != 2 {
				t.Fatalf("no EntryTag should return both runs, got %d", list.Total)
			}
		})
	})
}

// runIDs extracts run IDs for error messages.
func runIDs(runs []Run) []string {
	out := make([]string, len(runs))
	for i, r := range runs {
		out[i] = r.ID
	}
	return out
}

// ---- List sorting ----------------------------------------------------------

func TestSQLJobStoreListSorting(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
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
	})
}

// ---- Pagination ------------------------------------------------------------

func TestSQLJobStoreListPagination(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		base := time.Now().UTC().Truncate(time.Millisecond)

		for i := range 5 {
			id := string(rune('1' + i)) // "1".."5"
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

func TestSQLJobStoreListDefaultLimit(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		list := s.List(JobFilter{})
		if list.Limit != 100 {
			t.Fatalf("default limit: got %d, want 100", list.Limit)
		}
	})
}

// ---- Nullable timestamps & complex fields ----------------------------------

func TestSQLJobStoreNullTimestamps(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
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
		// FinishedAt is not stored in the jobs table; it's always zero for in-flight jobs.
		if !got.FinishedAt.IsZero() {
			t.Fatalf("FinishedAt should be zero for in-flight job, got %v", got.FinishedAt)
		}
	})
}

func TestSQLJobStoreRoundTripComplexFields(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
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

// ---- PurgeOlderThan --------------------------------------------------------

func TestSQLJobStorePurgeDeletesInBatches(t *testing.T) {
	// Override batch size so that 5 runs require 3 iterations (2+2+1).
	old := purgeBatchSize
	purgeBatchSize = 2
	t.Cleanup(func() { purgeBatchSize = old })

	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		cutoff := time.Now().UTC()
		old := cutoff.Add(-24 * time.Hour)

		for i := 0; i < 5; i++ {
			id := fmt.Sprintf("b%d", i)
			job := Job{ID: id, Domain: "example.com", Status: JobSucceeded, CreatedAt: old, FinishedAt: old}
			if _, err := s.Create(job); err != nil {
				t.Fatalf("create %s: %v", id, err)
			}
			graduateSQLJob(t, s, job, nil)
		}

		n, err := s.PurgeOlderThan(cutoff)
		if err != nil {
			t.Fatalf("purge: %v", err)
		}
		if n != 5 {
			t.Fatalf("expected 5 purged, got %d", n)
		}
	})
}

// ---- PurgeByTag ------------------------------------------------------------

func TestSQLJobStorePurgeByTagDeletesInBatches(t *testing.T) {
	old := purgeBatchSize
	purgeBatchSize = 2
	t.Cleanup(func() { purgeBatchSize = old })

	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		now := time.Now().UTC()
		if err := s.CreateTag("tld", ""); err != nil {
			t.Fatalf("CreateTag: %v", err)
		}
		for i := 0; i < 5; i++ {
			name := fmt.Sprintf("d%d.example", i)
			job := Job{ID: name, Domain: name, Status: JobSucceeded, CreatedAt: now, FinishedAt: now}
			if _, err := s.Create(job); err != nil {
				t.Fatalf("create %s: %v", name, err)
			}
			graduateSQLJob(t, s, job, nil)
			d, ok := s.GetDomainByName(name)
			if !ok {
				t.Fatalf("domain %s not found", name)
			}
			if err := s.TagDomains("tld", []int64{d.ID}); err != nil {
				t.Fatalf("TagDomains %s: %v", name, err)
			}
		}

		n, err := s.PurgeByTag("tld")
		if err != nil {
			t.Fatalf("purge by tag: %v", err)
		}
		if n != 5 {
			t.Fatalf("expected 5 purged, got %d", n)
		}
	})
}

// ---- Recovery --------------------------------------------------------------

func TestRecoverJobsRunningToFailed(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
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

func TestRecoverJobsQueuedReenqueued(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
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

func TestRecoverJobsPreservesPriority(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		base := time.Now().UTC()

		// Batch job created first (older), normal job created second.
		// After recovery, normal must be dequeued before batch.
		for _, job := range []Job{
			{ID: "b1", Domain: "b1.test", Status: JobQueued, Priority: PriorityBatch, CreatedAt: base},
			{ID: "n1", Domain: "n1.test", Status: JobQueued, Priority: PriorityNormal, CreatedAt: base.Add(time.Second)},
		} {
			if _, err := s.Create(job); err != nil {
				t.Fatalf("Create %q: %v", job.ID, err)
			}
		}

		q := NewInMemoryQueue()
		if err := RecoverJobs(s, q); err != nil {
			t.Fatalf("RecoverJobs: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		first, err := q.Dequeue(ctx)
		if err != nil {
			t.Fatalf("dequeue: %v", err)
		}
		if first != "n1" {
			t.Fatalf("expected n1 (normal) first, got %q", first)
		}
		second, err := q.Dequeue(ctx)
		if err != nil {
			t.Fatalf("dequeue: %v", err)
		}
		if second != "b1" {
			t.Fatalf("expected b1 (batch) second, got %q", second)
		}
	})
}

func TestRecoverJobsNoopInMemory(t *testing.T) {
	store := NewInMemoryJobStore()
	queue := NewInMemoryQueue()
	if err := RecoverJobs(store, queue); err != nil {
		t.Fatalf("RecoverJobs on in-memory store: %v", err)
	}
}

// TestRecoverJobsGraduatesRunningAndOrphans confirms that a running job
// killed by a prior shutdown lands in the runs table as a failed run
// (carrying its error), and that orphan terminal-status rows already
// sitting in the jobs table are likewise moved out so they can't keep
// snapshot capture blocked.
func TestRecoverJobsGraduatesRunningAndOrphans(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		base := time.Now().UTC().Truncate(time.Second)

		for _, job := range []Job{
			{ID: "running-1", Domain: "r.test", Status: JobRunning, CreatedAt: base, StartedAt: base.Add(time.Second)},
			{ID: "orphan-failed", Domain: "f.test", Status: JobFailed, Error: "earlier crash", CreatedAt: base, FinishedAt: base.Add(2 * time.Second)},
			{ID: "orphan-succeeded", Domain: "s.test", Status: JobSucceeded, CreatedAt: base, FinishedAt: base.Add(3 * time.Second)},
			{ID: "queued-1", Domain: "q.test", Status: JobQueued, CreatedAt: base},
			{ID: "paused-1", Domain: "p.test", Status: JobPaused, CreatedAt: base},
		} {
			if _, err := s.Create(job); err != nil {
				t.Fatalf("Create %q: %v", job.ID, err)
			}
		}

		q := NewInMemoryQueue()
		if err := RecoverJobs(s, q); err != nil {
			t.Fatalf("RecoverJobs: %v", err)
		}

		jobsByID := map[string]Job{}
		for _, j := range s.List(JobFilter{Limit: 100}).Items {
			jobsByID[j.ID] = j
		}

		for _, id := range []string{"running-1", "orphan-failed", "orphan-succeeded"} {
			if _, stillThere := jobsByID[id]; stillThere {
				t.Fatalf("%s should have been graduated out of the jobs table", id)
			}
			run, ok := s.GetRun(id)
			if !ok {
				t.Fatalf("%s missing from runs table after recovery", id)
			}
			if run.Status != JobFailed {
				t.Fatalf("%s status = %q, want failed", id, run.Status)
			}
			if run.Error == "" {
				t.Fatalf("%s graduated without an error message", id)
			}
		}

		if got, ok := s.GetRun("running-1"); !ok || got.Error != "server restarted during job" {
			t.Fatalf("running-1 error = %q, want server-restarted", got.Error)
		}
		if got, ok := s.GetRun("orphan-failed"); !ok || got.Error != "earlier crash" {
			t.Fatalf("orphan-failed error = %q, want preserved 'earlier crash'", got.Error)
		}

		for _, id := range []string{"queued-1", "paused-1"} {
			job, ok := jobsByID[id]
			if !ok {
				t.Fatalf("%s should still be in the jobs table", id)
			}
			if id == "queued-1" && job.Status != JobQueued {
				t.Fatalf("queued-1 status = %q, want queued", job.Status)
			}
			if id == "paused-1" && job.Status != JobPaused {
				t.Fatalf("paused-1 status = %q, want paused", job.Status)
			}
		}
	})
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
	// Verify Create's VALUES clause has the correct number of placeholders.
	// The INSERT has 12 bound params.
	s := &SQLJobStore{dialect: testDollarDialect{}}

	block := s.phRange(1, 12)

	count := strings.Count(block, "$")
	if count != 12 {
		t.Fatalf("create block has %d placeholders, want 12: %q", count, block)
	}

	if !strings.Contains(block, "$1") || !strings.Contains(block, "$12") {
		t.Fatalf("block should contain $1..$12: %q", block)
	}
}

func TestPhRangeUpdatePlaceholderCount(t *testing.T) {
	// Verify Update uses 10 placeholders (9 SET + 1 WHERE).
	s := &SQLJobStore{dialect: testDollarDialect{}}
	for i := 1; i <= 10; i++ {
		ph := s.ph(i)
		want := fmt.Sprintf("$%d", i)
		if ph != want {
			t.Fatalf("ph(%d) = %q, want %q", i, ph, want)
		}
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
	for _, tc := range []struct {
		n    int
		want string
	}{
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
		{"sqlite", false, "sqliteDialect"},
		{"postgres", false, "postgresDialect"},
		{"mariadb", false, "mariadbDialect"},
		{"mysql", false, "mariadbDialect"},
		{"mongodb", true, ""},
		{"", true, ""},
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
	// Use a SQLite DB as the target - configurePool calls Set* methods on
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

// ---- Scoring integration tests ---------------------------------------------

func TestGraduateJobStoresScore(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		now := time.Now().UTC()
		job := Job{
			ID:         "score-job-1",
			Domain:     "example.com",
			Status:     JobSucceeded,
			CreatedAt:  now.Add(-time.Minute),
			StartedAt:  now.Add(-30 * time.Second),
			FinishedAt: now,
		}
		if _, err := s.Create(job); err != nil {
			t.Fatalf("create job: %v", err)
		}
		entries := []engine.LogEntry{
			{Module: "DNSSEC", Tag: "DS07_NOT_SIGNED", Level: "WARNING"},
			{Module: "BASIC", Tag: "SOME_NOTICE", Level: "NOTICE"},
		}
		if err := s.GraduateJob(job, entries); err != nil {
			t.Fatalf("GraduateJob: %v", err)
		}

		run, ok := s.GetRun(job.ID)
		if !ok {
			t.Fatal("GetRun returned false")
		}
		if run.Score == nil {
			t.Fatal("run.Score is nil after graduation")
		}
		if run.Grade == nil {
			t.Fatal("run.Grade is nil after graduation")
		}

		// Verify the score is also persisted in the DB (not just in memory).
		var dbScore sql.NullInt64
		var dbGrade sql.NullString
		err := s.db.QueryRow(
			"SELECT score, grade FROM runs WHERE id = "+s.ph(1), job.ID,
		).Scan(&dbScore, &dbGrade)
		if err != nil {
			t.Fatalf("query score/grade from DB: %v", err)
		}
		if !dbScore.Valid {
			t.Fatal("score column is NULL in DB")
		}
		if !dbGrade.Valid {
			t.Fatal("grade column is NULL in DB")
		}
		if int(dbScore.Int64) != *run.Score {
			t.Fatalf("DB score %d != run.Score %d", dbScore.Int64, *run.Score)
		}
		if dbGrade.String != *run.Grade {
			t.Fatalf("DB grade %q != run.Grade %q", dbGrade.String, *run.Grade)
		}
	})
}

func TestGetRunLazyScoreComputation(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		now := time.Now().UTC()
		job := Job{
			ID:         "lazy-score-job",
			Domain:     "lazy.example",
			Status:     JobSucceeded,
			CreatedAt:  now.Add(-time.Minute),
			StartedAt:  now.Add(-30 * time.Second),
			FinishedAt: now,
		}
		if _, err := s.Create(job); err != nil {
			t.Fatalf("create job: %v", err)
		}
		entries := []engine.LogEntry{
			{Module: "DNSSEC", Tag: "DS_ALGO_OK", Level: "INFO"},
			{Module: "BASIC", Tag: "SOME_WARNING", Level: "WARNING"},
		}
		if err := s.GraduateJob(job, entries); err != nil {
			t.Fatalf("GraduateJob: %v", err)
		}

		// NULL out score/grade to simulate rows stored before scoring existed.
		ph := s.ph(1)
		_, err := s.db.Exec("UPDATE runs SET score = NULL, grade = NULL WHERE id = "+ph, job.ID)
		if err != nil {
			t.Fatalf("null out score: %v", err)
		}

		// GetRun should recompute lazily.
		run, ok := s.GetRun(job.ID)
		if !ok {
			t.Fatal("GetRun returned false")
		}
		if run.Score == nil {
			t.Fatal("run.Score is nil after lazy computation")
		}
		if run.Grade == nil {
			t.Fatal("run.Grade is nil after lazy computation")
		}

		// Score must have been cached back into the DB.
		var dbScore sql.NullInt64
		if err := s.db.QueryRow("SELECT score FROM runs WHERE id = "+ph, job.ID).Scan(&dbScore); err != nil {
			t.Fatalf("query score from DB: %v", err)
		}
		if !dbScore.Valid {
			t.Fatal("score not cached back to DB after lazy computation")
		}
		if int(dbScore.Int64) != *run.Score {
			t.Fatalf("cached DB score %d != run.Score %d", dbScore.Int64, *run.Score)
		}
	})
}
