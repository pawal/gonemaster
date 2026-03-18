package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

// jobCols is the canonical column list used in SELECT statements.
const jobCols = `id, batch_id, domain, status, created_at, started_at, finished_at,
	progress, result_url, error,
	sev_notice, sev_warning, sev_error, sev_critical,
	tests_json, overrides_json, undelegated_ns_json, undelegated_ds_json, min_level`

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

// SQLJobStore implements JobStore using a SQL database.
type SQLJobStore struct {
	db      *sql.DB
	dialect sqlDialect
}

// NewSQLJobStore creates a SQLJobStore backed by db using dialect.
func NewSQLJobStore(db *sql.DB, dialect sqlDialect) *SQLJobStore {
	return &SQLJobStore{db: db, dialect: dialect}
}

// ph returns the n-th (1-based) placeholder for this dialect.
func (s *SQLJobStore) ph(n int) string { return s.dialect.Placeholder(n) }

// phRange returns count comma-separated placeholders starting at position start.
// For SQLite: "?, ?, ?". For PostgreSQL: "$1, $2, $3".
func (s *SQLJobStore) phRange(start, count int) string {
	phs := make([]string, count)
	for i := range phs {
		phs[i] = s.ph(start + i)
	}
	return strings.Join(phs, ", ")
}

// ts returns the storable representation of a time.Time for this dialect.
func (s *SQLJobStore) ts(t time.Time) any { return s.dialect.TimestampVal(t) }

// toNullJSON marshals v to a JSON NullString. Empty slices/maps and nil
// values are stored as SQL NULL rather than "[]" / "{}".
func toNullJSON(v any) (sql.NullString, error) {
	if v == nil {
		return sql.NullString{}, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return sql.NullString{}, err
	}
	s := string(b)
	if s == "null" || s == "[]" || s == "{}" {
		return sql.NullString{}, nil
	}
	return sql.NullString{String: s, Valid: true}, nil
}

// unmarshalNullJSON decodes a NullString back into T.
// If the NullString is not valid (SQL NULL) the zero value is returned.
func unmarshalNullJSON[T any](ns sql.NullString) (T, error) {
	var zero T
	if !ns.Valid {
		return zero, nil
	}
	var v T
	if err := json.Unmarshal([]byte(ns.String), &v); err != nil {
		return zero, err
	}
	return v, nil
}

// parseTimestampNullStr parses an RFC3339Nano (or RFC3339) timestamp stored
// in a nullable TEXT column. Returns zero time when the value is NULL.
func parseTimestampNullStr(ns sql.NullString) time.Time {
	if !ns.Valid || ns.String == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, ns.String)
	if err != nil {
		t, _ = time.Parse(time.RFC3339, ns.String)
	}
	return t.UTC()
}

// parseTimestampStr parses an RFC3339Nano (or RFC3339) timestamp from a NOT
// NULL TEXT column.
func parseTimestampStr(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t, _ = time.Parse(time.RFC3339, s)
	}
	return t.UTC()
}

// sqlOrderByClause returns the ORDER BY expression for the given sort value.
// Timestamps are stored as fixed-width UTC strings and are lexicographically
// sortable as TEXT.
func sqlOrderByClause(sort JobSort) string {
	switch sort {
	case JobSortCreatedAtDesc:
		return "created_at DESC, id ASC"
	case JobSortCreatedAtAsc:
		return "created_at ASC, id ASC"
	case JobSortStartedAtDesc:
		return "COALESCE(started_at, created_at) DESC, created_at DESC, id ASC"
	case JobSortStartedAtAsc:
		return "COALESCE(started_at, created_at) ASC, created_at ASC, id ASC"
	case JobSortDomainAsc:
		return "LOWER(domain) ASC, created_at DESC, id ASC"
	case JobSortDomainDesc:
		return "LOWER(domain) DESC, created_at DESC, id ASC"
	case JobSortBatchIDAsc:
		return "LOWER(batch_id) ASC, created_at DESC, id ASC"
	case JobSortBatchIDDesc:
		return "LOWER(batch_id) DESC, created_at DESC, id ASC"
	case JobSortErrorDesc:
		return "(sev_error + sev_critical) DESC, sev_critical DESC, created_at DESC, id ASC"
	case JobSortCriticalDesc:
		return "sev_critical DESC, (sev_error + sev_critical) DESC, created_at DESC, id ASC"
	default:
		return "COALESCE(started_at, created_at) DESC, created_at DESC, id ASC"
	}
}

// scanJob reads one row from a SELECT jobCols query into a Job.
func (s *SQLJobStore) scanJob(row rowScanner) (Job, error) {
	var (
		id, batchID, domain, status string
		createdAt                   string
		startedAt, finishedAt       sql.NullString
		progress                    int
		resultURL, jobError         string
		sevNotice, sevWarn, sevErr, sevCrit int
		testsJSON, overridesJSON            sql.NullString
		nsJSON, dsJSON                      sql.NullString
		minLevel                            string
	)
	if err := row.Scan(
		&id, &batchID, &domain, &status,
		&createdAt, &startedAt, &finishedAt,
		&progress, &resultURL, &jobError,
		&sevNotice, &sevWarn, &sevErr, &sevCrit,
		&testsJSON, &overridesJSON, &nsJSON, &dsJSON,
		&minLevel,
	); err != nil {
		return Job{}, err
	}

	tests, err := unmarshalNullJSON[[]string](testsJSON)
	if err != nil {
		return Job{}, fmt.Errorf("unmarshal tests: %w", err)
	}
	overrides, err := unmarshalNullJSON[map[string]any](overridesJSON)
	if err != nil {
		return Job{}, fmt.Errorf("unmarshal overrides: %w", err)
	}
	ns, err := unmarshalNullJSON[[]engine.UndelegatedNameserver](nsJSON)
	if err != nil {
		return Job{}, fmt.Errorf("unmarshal undelegated_ns: %w", err)
	}
	ds, err := unmarshalNullJSON[[]engine.UndelegatedDSInfo](dsJSON)
	if err != nil {
		return Job{}, fmt.Errorf("unmarshal undelegated_ds: %w", err)
	}

	return Job{
		ID:         id,
		BatchID:    batchID,
		Domain:     domain,
		Status:     JobStatus(status),
		CreatedAt:  parseTimestampStr(createdAt),
		StartedAt:  parseTimestampNullStr(startedAt),
		FinishedAt: parseTimestampNullStr(finishedAt),
		Progress:   progress,
		ResultURL:  resultURL,
		Error:      jobError,
		SeverityTotals: map[string]int{
			"NOTICE":   sevNotice,
			"WARNING":  sevWarn,
			"ERROR":    sevErr,
			"CRITICAL": sevCrit,
		},
		Tests:         tests,
		Overrides:     overrides,
		UndelegatedNS: ns,
		UndelegatedDS: ds,
		MinLevel:      minLevel,
	}, nil
}

// Create inserts a new job and fails if the id already exists.
func (s *SQLJobStore) Create(job Job) (Job, error) {
	testsJSON, err := toNullJSON(job.Tests)
	if err != nil {
		return Job{}, fmt.Errorf("marshal tests: %w", err)
	}
	overridesJSON, err := toNullJSON(job.Overrides)
	if err != nil {
		return Job{}, fmt.Errorf("marshal overrides: %w", err)
	}
	nsJSON, err := toNullJSON(job.UndelegatedNS)
	if err != nil {
		return Job{}, fmt.Errorf("marshal undelegated_ns: %w", err)
	}
	dsJSON, err := toNullJSON(job.UndelegatedDS)
	if err != nil {
		return Job{}, fmt.Errorf("marshal undelegated_ds: %w", err)
	}

	_, err = s.db.Exec(
		fmt.Sprintf(`INSERT INTO jobs (
			id, batch_id, domain, status, created_at, started_at, finished_at,
			progress, result_url, error,
			sev_notice, sev_warning, sev_error, sev_critical,
			tests_json, overrides_json, undelegated_ns_json, undelegated_ds_json, min_level
		) VALUES (%s, 0, 0, 0, 0, %s)`, s.phRange(1, 10), s.phRange(11, 5)),
		job.ID, job.BatchID, job.Domain, string(job.Status),
		s.ts(job.CreatedAt), s.ts(job.StartedAt), s.ts(job.FinishedAt),
		job.Progress, job.ResultURL, job.Error,
		testsJSON, overridesJSON, nsJSON, dsJSON, job.MinLevel,
	)
	if err != nil {
		if s.dialect.IsDuplicateKey(err) {
			return Job{}, errors.New("job already exists")
		}
		return Job{}, fmt.Errorf("insert job: %w", err)
	}
	return job, nil
}

// Get returns the job with the given id.
func (s *SQLJobStore) Get(id string) (Job, bool) {
	row := s.db.QueryRow(fmt.Sprintf("SELECT %s FROM jobs WHERE id = %s", jobCols, s.ph(1)), id)
	job, err := s.scanJob(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Job{}, false
		}
		return Job{}, false
	}
	return job, true
}

// Update replaces a job's mutable fields. Returns an error if the job does not
// exist. The sev_* columns are managed exclusively by SetResult.
func (s *SQLJobStore) Update(job Job) error {
	testsJSON, err := toNullJSON(job.Tests)
	if err != nil {
		return fmt.Errorf("marshal tests: %w", err)
	}
	overridesJSON, err := toNullJSON(job.Overrides)
	if err != nil {
		return fmt.Errorf("marshal overrides: %w", err)
	}
	nsJSON, err := toNullJSON(job.UndelegatedNS)
	if err != nil {
		return fmt.Errorf("marshal undelegated_ns: %w", err)
	}
	dsJSON, err := toNullJSON(job.UndelegatedDS)
	if err != nil {
		return fmt.Errorf("marshal undelegated_ds: %w", err)
	}

	res, err := s.db.Exec(
		fmt.Sprintf(`UPDATE jobs SET
			batch_id=%s, domain=%s, status=%s,
			started_at=%s, finished_at=%s,
			progress=%s, result_url=%s, error=%s,
			tests_json=%s, overrides_json=%s, undelegated_ns_json=%s, undelegated_ds_json=%s, min_level=%s
		 WHERE id=%s`,
			s.ph(1), s.ph(2), s.ph(3),
			s.ph(4), s.ph(5),
			s.ph(6), s.ph(7), s.ph(8),
			s.ph(9), s.ph(10), s.ph(11), s.ph(12), s.ph(13),
			s.ph(14)),
		job.BatchID, job.Domain, string(job.Status),
		s.ts(job.StartedAt), s.ts(job.FinishedAt),
		job.Progress, job.ResultURL, job.Error,
		testsJSON, overridesJSON, nsJSON, dsJSON, job.MinLevel,
		job.ID,
	)
	if err != nil {
		return fmt.Errorf("update job: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return errors.New("job not found")
	}
	return nil
}

// List returns jobs matching filter with sorting and pagination applied.
func (s *SQLJobStore) List(filter JobFilter) JobList {
	normalizedSort := normalizeJobSort(filter.Sort)
	normalizedSeverity := normalizeJobSeverityFilter(filter.Severity)

	var conds []string
	var args []any
	argN := 0
	addArg := func(v any) string {
		args = append(args, v)
		argN++
		return s.dialect.Placeholder(argN)
	}

	if filter.Status != "" {
		conds = append(conds, "status = "+addArg(string(filter.Status)))
	}
	if filter.BatchID != "" {
		conds = append(conds, "batch_id = "+addArg(filter.BatchID))
	}
	if filter.Domain != "" {
		conds = append(conds, "LOWER(domain) LIKE "+addArg("%"+strings.ToLower(filter.Domain)+"%"))
	}
	if !filter.CreatedAfter.IsZero() {
		conds = append(conds, "created_at > "+addArg(formatSortableTimestamp(filter.CreatedAfter)))
	}
	if !filter.CreatedBefore.IsZero() {
		conds = append(conds, "created_at < "+addArg(formatSortableTimestamp(filter.CreatedBefore)))
	}
	switch normalizedSeverity {
	case JobSeverityWarningsPlus:
		conds = append(conds, "(sev_warning + sev_error + sev_critical) > 0")
	case JobSeverityErrorsOnly:
		conds = append(conds, "(sev_error + sev_critical) > 0")
	}

	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	orderBy := sqlOrderByClause(normalizedSort)

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM jobs"+where, args...).Scan(&total); err != nil {
		return JobList{Sort: string(normalizedSort)}
	}

	limitPH := s.dialect.Placeholder(argN + 1)
	offsetPH := s.dialect.Placeholder(argN + 2)
	dataArgs := make([]any, len(args), len(args)+2)
	copy(dataArgs, args)
	dataArgs = append(dataArgs, limit, offset)

	query := "SELECT " + jobCols + " FROM jobs" + where +
		" ORDER BY " + orderBy +
		" LIMIT " + limitPH + " OFFSET " + offsetPH

	rows, err := s.db.Query(query, dataArgs...)
	if err != nil {
		return JobList{Sort: string(normalizedSort)}
	}
	defer rows.Close()

	items := []Job{}
	for rows.Next() {
		job, err := s.scanJob(rows)
		if err != nil {
			continue
		}
		items = append(items, job)
	}
	if err := rows.Err(); err != nil {
		return JobList{Sort: string(normalizedSort)}
	}

	list := JobList{
		Items:  items,
		Total:  total,
		Limit:  limit,
		Offset: offset,
		Sort:   string(normalizedSort),
	}
	if offset > 0 {
		prevOffset := offset - limit
		if prevOffset < 0 {
			prevOffset = 0
		}
		list.PrevCursor = strconv.Itoa(prevOffset)
	}
	if offset+len(items) < total {
		list.NextCursor = strconv.Itoa(offset + len(items))
	}
	return list
}

// SetResult stores a result for an existing job and updates severity totals
// atomically in a single transaction.
func (s *SQLJobStore) SetResult(jobID string, result JobResult) error {
	summaryJSON, err := toNullJSON(result.Summary)
	if err != nil {
		return fmt.Errorf("marshal summary: %w", err)
	}

	var rawJSON sql.NullString
	if result.Raw != nil {
		b, err := json.Marshal(result.Raw)
		if err != nil {
			return fmt.Errorf("marshal raw: %w", err)
		}
		rawJSON = sql.NullString{String: string(b), Valid: true}
	}

	totals := severityTotalsFromSummary(result.Summary)

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	var exists int
	if err := tx.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM jobs WHERE id = %s", s.ph(1)), jobID).Scan(&exists); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("check job: %w", err)
	}
	if exists == 0 {
		_ = tx.Rollback()
		return errors.New("job not found")
	}

	if _, err := tx.Exec(
		s.dialect.UpsertResultSQL(),
		jobID, result.BatchID, string(result.Status), summaryJSON, rawJSON,
	); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("upsert result: %w", err)
	}

	if _, err := tx.Exec(
		fmt.Sprintf(`UPDATE jobs SET sev_notice=%s, sev_warning=%s, sev_error=%s, sev_critical=%s WHERE id=%s`,
			s.ph(1), s.ph(2), s.ph(3), s.ph(4), s.ph(5)),
		totals["NOTICE"], totals["WARNING"], totals["ERROR"], totals["CRITICAL"], jobID,
	); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("update severity totals: %w", err)
	}

	return tx.Commit()
}

// GetResult returns the stored result for jobID.
func (s *SQLJobStore) GetResult(jobID string) (JobResult, bool) {
	var (
		id, batchID, status    string
		summaryJSON, rawJSON   sql.NullString
	)
	err := s.db.QueryRow(
		fmt.Sprintf(`SELECT job_id, batch_id, status, summary_json, raw_json FROM results WHERE job_id = %s`, s.ph(1)),
		jobID,
	).Scan(&id, &batchID, &status, &summaryJSON, &rawJSON)
	if err != nil {
		return JobResult{}, false
	}

	result := JobResult{
		JobID:   id,
		BatchID: batchID,
		Status:  JobStatus(status),
	}

	if summaryJSON.Valid {
		var summary map[string]any
		if err := json.Unmarshal([]byte(summaryJSON.String), &summary); err == nil {
			result.Summary = summary
		}
	}

	if rawJSON.Valid {
		var raw JobResultRaw
		if err := json.Unmarshal([]byte(rawJSON.String), &raw); err == nil {
			result.Raw = &raw
		}
	}

	return result, true
}


// PurgeOlderThan deletes terminal-status jobs whose finished_at is before
// cutoff, along with their results. Returns the number of jobs deleted.
func (s *SQLJobStore) PurgeOlderThan(cutoff time.Time) (int64, error) {
	ph := s.ph(1)
	statuses := "'succeeded','failed','canceled','expired'"
	cutoffVal := s.dialect.TimestampVal(cutoff)

	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("purge begin tx: %w", err)
	}
	_, err = tx.Exec(
		`DELETE FROM results WHERE job_id IN `+
			`(SELECT id FROM jobs WHERE finished_at < `+ph+` AND status IN (`+statuses+`))`,
		cutoffVal,
	)
	if err != nil {
		_ = tx.Rollback()
		return 0, fmt.Errorf("purge results: %w", err)
	}
	res, err := tx.Exec(
		`DELETE FROM jobs WHERE finished_at < `+ph+` AND status IN (`+statuses+`)`,
		cutoffVal,
	)
	if err != nil {
		_ = tx.Rollback()
		return 0, fmt.Errorf("purge jobs: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("purge commit: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
