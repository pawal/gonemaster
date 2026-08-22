package server

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/lib/pq"
)

// swapRetrySleep removes the backoff so the retry tests do not actually
// wait, and records the delays that would have been slept.
func swapRetrySleep(t *testing.T) *[]time.Duration {
	t.Helper()
	slept := []time.Duration{}
	original := txRetrySleep
	txRetrySleep = func(d time.Duration) { slept = append(slept, d) }
	t.Cleanup(func() { txRetrySleep = original })
	return &slept
}

func TestDialectIsRetryableConflict(t *testing.T) {
	// Each dialect must recognise its own contention aborts and reject
	// everything else, including the duplicate-key errors that share the
	// same error type and are emphatically not retryable.
	cases := []struct {
		name    string
		dialect sqlDialect
		err     error
		want    bool
	}{
		{"mariadb check read", mariadbDialect{}, &mysql.MySQLError{Number: 1020}, true},
		{"mariadb deadlock", mariadbDialect{}, &mysql.MySQLError{Number: 1213}, true},
		{"mariadb lock wait timeout", mariadbDialect{}, &mysql.MySQLError{Number: 1205}, true},
		{"mariadb duplicate key", mariadbDialect{}, &mysql.MySQLError{Number: 1062}, false},
		{"mariadb plain error", mariadbDialect{}, errors.New("boom"), false},
		{"postgres serialization failure", postgresDialect{}, &pq.Error{Code: "40001"}, true},
		{"postgres deadlock detected", postgresDialect{}, &pq.Error{Code: "40P01"}, true},
		{"postgres duplicate key", postgresDialect{}, &pq.Error{Code: "23505"}, false},
		{"postgres plain error", postgresDialect{}, errors.New("boom"), false},
		{"sqlite busy", sqliteDialect{}, errors.New("database is locked (SQLITE_BUSY)"), true},
		{"sqlite unique violation", sqliteDialect{}, errors.New("UNIQUE constraint failed: domains.name"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.dialect.IsRetryableConflict(tc.err); got != tc.want {
				t.Fatalf("IsRetryableConflict(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestDialectIsRetryableConflictWrapped(t *testing.T) {
	// The store wraps driver errors with fmt.Errorf on every failure path,
	// so detection has to survive an errors.As unwrap.
	wrapped := fmt.Errorf("update domain: %w", &mysql.MySQLError{Number: 1020})
	if !(mariadbDialect{}).IsRetryableConflict(wrapped) {
		t.Fatal("wrapped ER_CHECKREAD should be retryable")
	}
}

func TestRetryOnConflictSucceedsAfterConflicts(t *testing.T) {
	slept := swapRetrySleep(t)
	store := &SQLJobStore{dialect: mariadbDialect{}}

	attempts := 0
	err := store.retryOnConflict("test op", func() error {
		attempts++
		if attempts < 3 {
			return &mysql.MySQLError{Number: 1020}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("retryOnConflict: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
	if len(*slept) != 2 {
		t.Fatalf("backoff slept %d times, want 2 (one per retry)", len(*slept))
	}
}

func TestRetryOnConflictStopsAtBudget(t *testing.T) {
	swapRetrySleep(t)
	store := &SQLJobStore{dialect: mariadbDialect{}}

	attempts := 0
	err := store.retryOnConflict("test op", func() error {
		attempts++
		return &mysql.MySQLError{Number: 1020}
	})
	if err == nil {
		t.Fatal("expected an error once the budget is exhausted")
	}
	if attempts != txMaxAttempts {
		t.Fatalf("attempts = %d, want %d", attempts, txMaxAttempts)
	}
	// The caller must still be able to identify the underlying cause.
	var mysqlErr *mysql.MySQLError
	if !errors.As(err, &mysqlErr) || mysqlErr.Number != 1020 {
		t.Fatalf("final error should wrap the driver error, got %v", err)
	}
}

func TestRetryOnConflictDoesNotRetryOtherErrors(t *testing.T) {
	swapRetrySleep(t)
	store := &SQLJobStore{dialect: mariadbDialect{}}

	attempts := 0
	sentinel := errors.New("job not found")
	err := store.retryOnConflict("test op", func() error {
		attempts++
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want the sentinel returned untouched", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestRetryOnConflictSucceedsFirstTry(t *testing.T) {
	slept := swapRetrySleep(t)
	store := &SQLJobStore{dialect: sqliteDialect{}}

	attempts := 0
	if err := store.retryOnConflict("test op", func() error {
		attempts++
		return nil
	}); err != nil {
		t.Fatalf("retryOnConflict: %v", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
	if len(*slept) != 0 {
		t.Fatalf("a successful first attempt must not back off, slept %d times", len(*slept))
	}
}

func TestTxRetryBackoffGrows(t *testing.T) {
	// Each attempt must wait at least as long as the previous one's base,
	// and jitter must keep it under twice that base.
	for attempt := 1; attempt <= 4; attempt++ {
		base := txRetryBaseGap << (attempt - 1)
		got := txRetryBackoff(attempt)
		if got < base || got >= 2*base {
			t.Fatalf("attempt %d backoff %v outside [%v, %v)", attempt, got, base, 2*base)
		}
	}
}

func TestForUpdateSuffixPerDialect(t *testing.T) {
	// SQLite has no FOR UPDATE and serialises writes on one connection.
	cases := []struct {
		dialect sqlDialect
		want    string
	}{
		{mariadbDialect{}, " FOR UPDATE"},
		{postgresDialect{}, " FOR UPDATE"},
		{sqliteDialect{}, ""},
	}
	for _, tc := range cases {
		store := &SQLJobStore{dialect: tc.dialect}
		if got := store.forUpdate(); got != tc.want {
			t.Fatalf("%T forUpdate() = %q, want %q", tc.dialect, got, tc.want)
		}
	}
}

func TestGraduateJobConcurrentSameDomain(t *testing.T) {
	// The reported wedge: two runs of one domain graduating at the same
	// moment. On MariaDB with innodb_snapshot_isolation on (the default
	// since 11.6.2) the loser aborts with ER_CHECKREAD, and without the
	// retry its job stays at "running" forever. Both must graduate, and
	// the domain's run_count must land on exactly the number of runs.
	forEachBackend(t, func(t *testing.T, store *SQLJobStore) {
		const domain = "concurrent.example"
		const jobCount = 4

		jobs := make([]Job, jobCount)
		for i := range jobs {
			jobs[i] = Job{
				ID:         fmt.Sprintf("job-concurrent-%d", i),
				Domain:     domain,
				Status:     JobSucceeded,
				CreatedAt:  time.Now().UTC().Add(-time.Minute),
				StartedAt:  time.Now().UTC().Add(-30 * time.Second),
				FinishedAt: time.Now().UTC(),
			}
			if _, err := store.Create(jobs[i]); err != nil {
				t.Fatalf("Create %s: %v", jobs[i].ID, err)
			}
		}

		// Release all graduations at once to maximise the overlap.
		start := make(chan struct{})
		errs := make(chan error, jobCount)
		var wg sync.WaitGroup
		for _, job := range jobs {
			wg.Add(1)
			go func(job Job) {
				defer wg.Done()
				<-start
				errs <- store.GraduateJob(job, nil)
			}(job)
		}
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("concurrent GraduateJob: %v", err)
			}
		}

		var runs int
		if err := store.db.QueryRow(
			fmt.Sprintf(`SELECT COUNT(*) FROM runs WHERE domain = %s`, store.ph(1)), domain,
		).Scan(&runs); err != nil {
			t.Fatalf("count runs: %v", err)
		}
		if runs != jobCount {
			t.Fatalf("runs = %d, want %d", runs, jobCount)
		}

		var remaining int
		if err := store.db.QueryRow(`SELECT COUNT(*) FROM jobs`).Scan(&remaining); err != nil {
			t.Fatalf("count jobs: %v", err)
		}
		if remaining != 0 {
			t.Fatalf("%d job rows left behind, every graduation must delete its own", remaining)
		}

		var runCount int
		if err := store.db.QueryRow(
			fmt.Sprintf(`SELECT run_count FROM domains WHERE name = %s`, store.ph(1)), domain,
		).Scan(&runCount); err != nil {
			t.Fatalf("read run_count: %v", err)
		}
		if runCount != jobCount {
			t.Fatalf("run_count = %d, want %d", runCount, jobCount)
		}
	})
}

func TestGraduateJobMissingJobIsNotRetried(t *testing.T) {
	// "job not found" is permanent, so it must return on the first attempt
	// rather than burning the retry budget and its backoff on it.
	slept := swapRetrySleep(t)
	store := testStoreForBackend(t, testBackends(t)[0])

	if err := store.GraduateJob(Job{ID: "nope", Domain: "missing.example"}, nil); err == nil {
		t.Fatal("expected an error for a job that does not exist")
	}
	if len(*slept) != 0 {
		t.Fatalf("a missing job must not back off, slept %d times", len(*slept))
	}
}
