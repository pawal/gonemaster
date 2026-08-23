package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/normalization"
	"codeberg.org/pawal/gonemaster/scoring"
)

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

// SQLJobStore implements JobStore using a SQL database.
type SQLJobStore struct {
	db              *sql.DB
	dialect         sqlDialect
	scoringCfg      scoring.Config
	tagViewMinLevel string
}

// NewSQLJobStore creates a SQLJobStore backed by db using dialect.
func NewSQLJobStore(db *sql.DB, dialect sqlDialect) *SQLJobStore {
	return &SQLJobStore{db: db, dialect: dialect, scoringCfg: scoring.DefaultConfig(), tagViewMinLevel: "NOTICE"}
}

// SetScoringConfig sets the scoring configuration used when graduating jobs.
func (s *SQLJobStore) SetScoringConfig(cfg scoring.Config) {
	s.scoringCfg = cfg
}

// SetTagViewMinLevel sets the capture-time floor for the snapshot tag view.
// Invalid input is ignored.
func (s *SQLJobStore) SetTagViewMinLevel(level string) {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "INFO", "NOTICE", "WARNING", "ERROR", "CRITICAL":
		s.tagViewMinLevel = strings.ToUpper(strings.TrimSpace(level))
	}
}

// TagViewMinLevel returns the server-wide tag-view floor.
func (s *SQLJobStore) TagViewMinLevel() string { return s.tagViewMinLevel }

// IsValidTagViewMinLevel reports whether level is an accepted severity token.
func IsValidTagViewMinLevel(level string) bool {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "INFO", "NOTICE", "WARNING", "ERROR", "CRITICAL":
		return true
	}
	return false
}

// ph returns the n-th (1-based) placeholder for this dialect.
func (s *SQLJobStore) ph(n int) string { return s.dialect.Placeholder(n) }

// phRange returns count comma-separated placeholders starting at position start.
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
// values are stored as SQL NULL.
func toNullJSON(v any) (sql.NullString, error) {
	if v == nil {
		return sql.NullString{}, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return sql.NullString{}, err
	}
	str := string(b)
	if str == "null" || str == "[]" || str == "{}" {
		return sql.NullString{}, nil
	}
	return sql.NullString{String: str, Valid: true}, nil
}

// unmarshalNullJSON decodes a NullString back into T.
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

// parseTimestampNullStr parses an RFC3339Nano timestamp from a nullable TEXT column.
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

// parseTimestampStr parses an RFC3339Nano timestamp from a NOT NULL TEXT column.
func parseTimestampStr(str string) time.Time {
	if str == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, str)
	if err != nil {
		t, _ = time.Parse(time.RFC3339, str)
	}
	return t.UTC()
}

func nullInt64Ptr(ns sql.NullInt64) *int64 {
	if !ns.Valid {
		return nil
	}
	v := ns.Int64
	return &v
}

func nullInt64Value(v *int64) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *v, Valid: true}
}

func nullFloat64Ptr(nf sql.NullFloat64) *float64 {
	if !nf.Valid {
		return nil
	}
	v := nf.Float64
	return &v
}

func nullFloat64Value(v *float64) sql.NullFloat64 {
	if v == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *v, Valid: true}
}

// ── job column helpers ────────────────────────────────────────────────────────

const jobCols = `id, domain_id, domain, batch_id, status, created_at, started_at,
	progress, error, profile, profile_id, profile_name, config_json, public_id, priority`

type jobConfigJSON struct {
	Tests         []string                       `json:"tests,omitempty"`
	Overrides     map[string]any                 `json:"overrides,omitempty"`
	UndelegatedNS []engine.UndelegatedNameserver `json:"undelegated_ns,omitempty"`
	UndelegatedDS []engine.UndelegatedDSInfo     `json:"undelegated_ds,omitempty"`
	MinLevel      string                         `json:"min_level,omitempty"`
	IPv4Disabled  bool                           `json:"ipv4_disabled,omitempty"`
	IPv6Disabled  bool                           `json:"ipv6_disabled,omitempty"`
	Origin        string                         `json:"origin,omitempty"`
}

func (s *SQLJobStore) scanJob(row rowScanner) (Job, error) {
	var (
		id, domain, batchID, status    string
		domainID                       int64
		createdAt                      string
		startedAt                      sql.NullString
		progress                       int
		jobError, profile, profileName string
		profileID                      sql.NullInt64
		configJSON                     sql.NullString
		publicID                       sql.NullString
		priority                       int
	)
	if err := row.Scan(
		&id, &domainID, &domain, &batchID, &status,
		&createdAt, &startedAt,
		&progress, &jobError, &profile,
		&profileID, &profileName, &configJSON, &publicID, &priority,
	); err != nil {
		return Job{}, err
	}

	var cfg jobConfigJSON
	if configJSON.Valid && configJSON.String != "" {
		if err := json.Unmarshal([]byte(configJSON.String), &cfg); err != nil {
			return Job{}, fmt.Errorf("unmarshal config_json: %w", err)
		}
	}

	return Job{
		ID:            id,
		PublicID:      publicID.String,
		DomainID:      domainID,
		BatchID:       batchID,
		Domain:        domain,
		Status:        JobStatus(status),
		CreatedAt:     parseTimestampStr(createdAt),
		StartedAt:     parseTimestampNullStr(startedAt),
		Progress:      progress,
		Error:         jobError,
		Profile:       profile,
		ProfileID:     nullInt64Ptr(profileID),
		ProfileName:   profileName,
		Priority:      JobPriority(priority),
		Tests:         cfg.Tests,
		Overrides:     cfg.Overrides,
		UndelegatedNS: cfg.UndelegatedNS,
		UndelegatedDS: cfg.UndelegatedDS,
		MinLevel:      cfg.MinLevel,
		IPv4Disabled:  cfg.IPv4Disabled,
		IPv6Disabled:  cfg.IPv6Disabled,
		Origin:        cfg.Origin,
	}, nil
}

// Create inserts a new in-flight job.
func (s *SQLJobStore) Create(job Job) (Job, error) {
	if job.PublicID == "" {
		job.PublicID = GeneratePublicID()
	}

	cfg := jobConfigJSON{
		Tests:         job.Tests,
		Overrides:     job.Overrides,
		UndelegatedNS: job.UndelegatedNS,
		UndelegatedDS: job.UndelegatedDS,
		MinLevel:      job.MinLevel,
		IPv4Disabled:  job.IPv4Disabled,
		IPv6Disabled:  job.IPv6Disabled,
		Origin:        job.Origin,
	}
	configJSON, err := toNullJSON(cfg)
	if err != nil {
		return Job{}, fmt.Errorf("marshal config_json: %w", err)
	}

	_, err = s.db.Exec(
		fmt.Sprintf(`INSERT INTO jobs (id, domain_id, domain, batch_id, status,
			created_at, started_at, progress, error, profile, profile_id, profile_name,
			config_json, public_id, priority
		) VALUES (%s)`, s.phRange(1, 15)),
		job.ID, job.DomainID, job.Domain, job.BatchID, string(job.Status),
		s.ts(job.CreatedAt), s.ts(job.StartedAt),
		job.Progress, job.Error, job.Profile, nullInt64Value(job.ProfileID), job.ProfileName,
		configJSON,
		sql.NullString{String: job.PublicID, Valid: job.PublicID != ""},
		int(job.Priority),
	)
	if err != nil {
		if s.dialect.IsDuplicateKey(err) {
			return Job{}, errors.New("job already exists")
		}
		return Job{}, fmt.Errorf("insert job: %w", err)
	}
	return job, nil
}

// Get returns the job with the given id, checking jobs first then runs.
func (s *SQLJobStore) Get(id string) (Job, bool) {
	row := s.db.QueryRow(
		fmt.Sprintf("SELECT %s FROM jobs WHERE id = %s", jobCols, s.ph(1)), id)
	job, err := s.scanJob(row)
	if err == nil {
		return job, true
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Job{}, false
	}
	// Check runs table.
	run, ok := s.GetRun(id)
	if !ok {
		return Job{}, false
	}
	return jobFromRun(run), true
}

// GetByPublicID returns the job with the given public_id.
func (s *SQLJobStore) GetByPublicID(publicID string) (Job, bool) {
	row := s.db.QueryRow(
		fmt.Sprintf("SELECT %s FROM jobs WHERE public_id = %s", jobCols, s.ph(1)), publicID)
	job, err := s.scanJob(row)
	if err == nil {
		return job, true
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Job{}, false
	}
	// Check runs table.
	run, ok := s.GetRunByPublicID(publicID)
	if !ok {
		return Job{}, false
	}
	return jobFromRun(run), true
}

// Update replaces a job's mutable fields for in-flight jobs.
func (s *SQLJobStore) Update(job Job) error {
	cfg := jobConfigJSON{
		Tests:         job.Tests,
		Overrides:     job.Overrides,
		UndelegatedNS: job.UndelegatedNS,
		UndelegatedDS: job.UndelegatedDS,
		MinLevel:      job.MinLevel,
		IPv4Disabled:  job.IPv4Disabled,
		IPv6Disabled:  job.IPv6Disabled,
		Origin:        job.Origin,
	}
	configJSON, err := toNullJSON(cfg)
	if err != nil {
		return fmt.Errorf("marshal config_json: %w", err)
	}

	res, err := s.db.Exec(
		fmt.Sprintf(`UPDATE jobs SET
			domain_id=%s, domain=%s, batch_id=%s, status=%s,
			started_at=%s, progress=%s, error=%s, profile=%s,
			profile_id=%s, profile_name=%s, config_json=%s
		 WHERE id=%s`,
			s.ph(1), s.ph(2), s.ph(3), s.ph(4),
			s.ph(5), s.ph(6), s.ph(7), s.ph(8),
			s.ph(9), s.ph(10), s.ph(11),
			s.ph(12)),
		job.DomainID, job.Domain, job.BatchID, string(job.Status),
		s.ts(job.StartedAt), job.Progress, job.Error, job.Profile,
		nullInt64Value(job.ProfileID), job.ProfileName, configJSON,
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

// sqlJobOrderBy returns the ORDER BY clause for the jobs table.
// Severity-based sorts fall back to started_at ordering since jobs have no
// sev_* columns (they are in-flight).
func sqlJobOrderBy(sort JobSort) string {
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
	default:
		// JobSortErrorDesc, JobSortCriticalDesc and unknown: fall back to started_at
		return "COALESCE(started_at, created_at) DESC, created_at DESC, id ASC"
	}
}

// List returns in-flight jobs matching filter with sorting and pagination.
func (s *SQLJobStore) List(filter JobFilter) JobList {
	normalizedSort := normalizeJobSort(filter.Sort)

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
	// Severity filtering is not applicable to in-flight jobs; silently ignored.

	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	orderBy := sqlJobOrderBy(normalizedSort)
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	offset := max(filter.Offset, 0)

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
		prevOffset := max(offset-limit, 0)
		list.PrevCursor = fmt.Sprintf("%d", prevOffset)
	}
	if offset+len(items) < total {
		list.NextCursor = fmt.Sprintf("%d", offset+len(items))
	}
	return list
}

// graduationValues is computed before the tx opens, so a retried attempt
// recomputes nothing.
type graduationValues struct {
	sevNotice             int
	sevWarning            int
	sevError              int
	sevCritical           int
	worstLevel            string
	durationMs            int64
	scoreVal              sql.NullInt64
	gradeVal              sql.NullString
	nameserverTimingsJSON any
}

// GraduateJob atomically creates a run, inserts entries, upserts the domain
// record, updates domain latest_* fields, and deletes the job from the queue.
// Contention aborts are retried on a fresh transaction.
func (s *SQLJobStore) GraduateJob(job Job, engineEntries []engine.LogEntry) error {
	// Compute severity totals and worst level.
	vals := graduationValues{}
	for _, e := range engineEntries {
		switch strings.ToUpper(strings.TrimSpace(e.Level)) {
		case "NOTICE":
			vals.sevNotice++
		case "WARNING":
			vals.sevWarning++
		case "ERROR":
			vals.sevError++
		case "CRITICAL":
			vals.sevCritical++
		}
	}
	vals.worstLevel = computeWorstLevel(vals.sevNotice, vals.sevWarning, vals.sevError, vals.sevCritical)

	if !job.StartedAt.IsZero() && !job.FinishedAt.IsZero() {
		vals.durationMs = job.FinishedAt.Sub(job.StartedAt).Milliseconds()
	}

	// Compute score eagerly at graduation time.
	scoringEntries := make([]scoring.Entry, len(engineEntries))
	for i, e := range engineEntries {
		scoringEntries[i] = scoring.Entry{Module: e.Module, Tag: e.Tag, Level: e.Level}
	}
	scoreResult := scoring.Compute(job.Domain, scoringEntries, s.scoringCfg)
	vals.scoreVal = sql.NullInt64{Int64: int64(scoreResult.Score), Valid: true}
	vals.gradeVal = sql.NullString{String: scoreResult.Grade, Valid: true}
	nameserverTimingsJSON, err := toNullJSON(job.NameserverTimings)
	if err != nil {
		return fmt.Errorf("marshal nameserver timings: %w", err)
	}
	vals.nameserverTimingsJSON = nameserverTimingsJSON

	return s.retryOnConflict("graduate job", func() error {
		return s.graduateJobOnce(job, engineEntries, vals)
	})
}

// graduateJobOnce runs one graduation transaction. A rolled back attempt
// leaves nothing behind, so the caller can just run it again.
func (s *SQLJobStore) graduateJobOnce(job Job, engineEntries []engine.LogEntry, vals graduationValues) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin graduation tx: %w", err)
	}

	// Check the job still exists. This opens the transaction's read view.
	var exists int
	if err := tx.QueryRow(
		fmt.Sprintf("SELECT COUNT(*) FROM jobs WHERE id = %s", s.ph(1)), job.ID,
	).Scan(&exists); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("check job: %w", err)
	}
	if exists == 0 {
		_ = tx.Rollback()
		return errors.New("job not found")
	}

	// Locks the domain row up front, so a competing graduation of the same
	// domain conflicts here rather than after the entry inserts.
	domainID, err := s.upsertDomainTx(tx, job.Domain)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("upsert domain: %w", err)
	}

	// Insert run.
	if _, err := tx.Exec(
		fmt.Sprintf(`INSERT INTO runs (
			id, domain_id, domain, batch_id, status,
			created_at, started_at, finished_at, duration_ms,
			sev_notice, sev_warning, sev_error, sev_critical,
			worst_level, entry_count, profile, profile_id, profile_name,
			effective_profile, public_id, priority, score, grade, nameserver_timings_json,
			error
		) VALUES (%s)`, s.phRange(1, 25)),
		job.ID, domainID, job.Domain, job.BatchID, string(job.Status),
		s.ts(job.CreatedAt), s.ts(job.StartedAt), s.ts(job.FinishedAt), vals.durationMs,
		vals.sevNotice, vals.sevWarning, vals.sevError, vals.sevCritical,
		vals.worstLevel, len(engineEntries), job.Profile, nullInt64Value(job.ProfileID), job.ProfileName,
		job.EffectiveProfile,
		sql.NullString{String: job.PublicID, Valid: job.PublicID != ""},
		int(job.Priority), vals.scoreVal, vals.gradeVal, vals.nameserverTimingsJSON,
		job.Error,
	); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("insert run: %w", err)
	}

	// Insert entries in batches to avoid huge parameter lists.
	if len(engineEntries) > 0 {
		if err := s.insertEntriesTx(tx, job.ID, domainID, engineEntries); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("insert entries: %w", err)
		}
	}

	// Update domain latest_*.
	if _, err := tx.Exec(
		fmt.Sprintf(`UPDATE domains SET
			latest_run_id=%s, latest_run_at=%s, latest_status=%s,
			latest_level=%s, latest_score=%s, latest_grade=%s, run_count=run_count+1
		 WHERE id=%s`,
			s.ph(1), s.ph(2), s.ph(3), s.ph(4), s.ph(5), s.ph(6), s.ph(7)),
		job.ID, s.ts(job.FinishedAt), string(job.Status),
		vals.worstLevel, vals.scoreVal, vals.gradeVal, domainID,
	); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("update domain: %w", err)
	}

	// Delete the job from the queue.
	if _, err := tx.Exec(
		fmt.Sprintf("DELETE FROM jobs WHERE id = %s", s.ph(1)), job.ID,
	); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("delete job: %w", err)
	}

	if job.DNSSECChainJSON != "" {
		if _, err := tx.Exec(
			fmt.Sprintf("INSERT INTO run_dnssec_chain (run_id, chain_json) VALUES (%s, %s)", s.ph(1), s.ph(2)),
			job.ID, job.DNSSECChainJSON,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("insert dnssec chain: %w", err)
		}
	}

	return tx.Commit()
}

// GetRunDNSSECChain returns the stored chain summary JSON for a run.
func (s *SQLJobStore) GetRunDNSSECChain(runID string) (string, bool, error) {
	var chain string
	err := s.db.QueryRow(
		fmt.Sprintf("SELECT chain_json FROM run_dnssec_chain WHERE run_id = %s", s.ph(1)), runID,
	).Scan(&chain)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return chain, true, nil
}

// upsertDomainTx gets or creates a domain row inside tx, returning its ID.
func (s *SQLJobStore) upsertDomainTx(tx *sql.Tx, name string) (int64, error) {
	now := formatSortableTimestamp(time.Now().UTC())
	switch s.dialect.(type) {
	case postgresDialect:
		if _, err := tx.Exec(
			`INSERT INTO domains (name, created_at) VALUES ($1, $2)
			 ON CONFLICT (name) DO NOTHING`,
			name, now,
		); err != nil {
			return 0, err
		}
	case mariadbDialect:
		if _, err := tx.Exec(
			`INSERT IGNORE INTO domains (name, created_at) VALUES (?, ?)`,
			name, now,
		); err != nil {
			return 0, err
		}
	default: // sqlite
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO domains (name, created_at) VALUES (?, ?)`,
			name, now,
		); err != nil {
			return 0, err
		}
	}
	var id int64
	if err := tx.QueryRow(
		fmt.Sprintf("SELECT id FROM domains WHERE name = %s%s", s.ph(1), s.forUpdate()), name,
	).Scan(&id); err != nil {
		return 0, fmt.Errorf("get domain id: %w", err)
	}
	return id, nil
}

// forUpdate returns the row-locking suffix for a SELECT, empty on SQLite
// which serialises writes on a single connection instead.
func (s *SQLJobStore) forUpdate() string {
	switch s.dialect.(type) {
	case postgresDialect, mariadbDialect:
		return " FOR UPDATE"
	default:
		return ""
	}
}

const insertEntriesBatchSize = 200

// insertEntriesTx inserts engine log entries into the entries table in batches.
func (s *SQLJobStore) insertEntriesTx(tx *sql.Tx, runID string, domainID int64, entries []engine.LogEntry) error {
	for start := 0; start < len(entries); start += insertEntriesBatchSize {
		end := min(start+insertEntriesBatchSize, len(entries))
		batch := entries[start:end]
		placeholders := make([]string, len(batch))
		args := make([]any, 0, len(batch)*8)
		argN := 0
		for _, e := range batch {
			argsJSON, err := toNullJSON(e.Args)
			if err != nil {
				return fmt.Errorf("marshal args: %w", err)
			}
			p := make([]string, 8)
			for i := range p {
				argN++
				p[i] = s.ph(argN)
			}
			placeholders[argN/8-1] = "(" + strings.Join(p, ", ") + ")"
			args = append(args,
				runID, domainID, e.Timestamp,
				e.Module, e.Testcase, e.Tag, e.Level,
				argsJSON,
			)
		}
		query := `INSERT INTO entries (run_id, domain_id, timestamp, module, testcase, tag, level, args_json) VALUES ` +
			strings.Join(placeholders, ", ")
		if _, err := tx.Exec(query, args...); err != nil {
			return err
		}
	}
	return nil
}

// GetResult reconstructs a JobResult from the runs and entries tables.
func (s *SQLJobStore) GetResult(jobID string) (JobResult, bool) {
	run, ok := s.GetRun(jobID)
	if !ok {
		return JobResult{}, false
	}
	entries, err := s.loadEntries(jobID)
	if err != nil {
		return JobResult{}, false
	}
	scoringEntries := make([]scoring.Entry, len(entries))
	for i, e := range entries {
		scoringEntries[i] = scoring.Entry{Module: e.Module, Tag: e.Tag, Level: e.Level}
	}
	sr := scoring.Compute(run.Domain, scoringEntries, s.scoringCfg)
	result := buildJobResult(run, entries, &sr)
	var probe int
	if err := s.db.QueryRow(
		fmt.Sprintf("SELECT 1 FROM run_dnssec_chain WHERE run_id = %s", s.ph(1)), jobID,
	).Scan(&probe); err == nil {
		result.HasDNSSECChain = true
	}
	return result, true
}

// loadEntries loads all entries for a run, ordered by timestamp.
func (s *SQLJobStore) loadEntries(runID string) ([]Entry, error) {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT id, run_id, domain_id, timestamp, module, testcase, tag, level, args_json
		 FROM entries WHERE run_id = %s ORDER BY timestamp ASC, id ASC`, s.ph(1)),
		runID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var (
			id                                int64
			rid, module, testcase, tag, level string
			domainID                          int64
			timestamp                         float64
			argsJSON                          sql.NullString
		)
		if err := rows.Scan(&id, &rid, &domainID, &timestamp, &module, &testcase, &tag, &level, &argsJSON); err != nil {
			return nil, err
		}
		var args map[string]any
		if argsJSON.Valid {
			_ = json.Unmarshal([]byte(argsJSON.String), &args)
		}
		entries = append(entries, Entry{
			ID:        id,
			RunID:     rid,
			DomainID:  domainID,
			Timestamp: timestamp,
			Module:    module,
			Testcase:  testcase,
			Tag:       tag,
			Level:     level,
			Args:      args,
		})
	}
	return entries, rows.Err()
}

// ── Domain management ─────────────────────────────────────────────────────────

// GetOrCreateDomain returns the domain for name, creating it if necessary.
// name is normalized (lowercased, Unicode labels converted to ACE) before storage.
func (s *SQLJobStore) GetOrCreateDomain(name string) (Domain, error) {
	errs, normalized := normalization.NormalizeName(strings.TrimSpace(name))
	if len(errs) > 0 {
		return Domain{}, fmt.Errorf("invalid domain name %q: %s", name, errs[0].Message())
	}
	name = normalized
	now := formatSortableTimestamp(time.Now().UTC())
	switch s.dialect.(type) {
	case postgresDialect:
		_, err := s.db.Exec(
			`INSERT INTO domains (name, created_at) VALUES ($1, $2) ON CONFLICT (name) DO NOTHING`,
			name, now)
		if err != nil {
			return Domain{}, fmt.Errorf("upsert domain: %w", err)
		}
	case mariadbDialect:
		_, err := s.db.Exec(`INSERT IGNORE INTO domains (name, created_at) VALUES (?, ?)`, name, now)
		if err != nil {
			return Domain{}, fmt.Errorf("upsert domain: %w", err)
		}
	default:
		_, err := s.db.Exec(`INSERT OR IGNORE INTO domains (name, created_at) VALUES (?, ?)`, name, now)
		if err != nil {
			return Domain{}, fmt.Errorf("upsert domain: %w", err)
		}
	}
	return s.getDomainByName(name)
}

func (s *SQLJobStore) scanDomain(row *sql.Row) (Domain, error) {
	var (
		id                        int64
		domainName, createdAt     string
		latestRunID, latestRunAt  sql.NullString
		latestStatus, latestLevel sql.NullString
		latestScore               sql.NullInt64
		latestGrade               sql.NullString
		runCount                  int
	)
	err := row.Scan(&id, &domainName, &latestRunID, &latestRunAt, &latestStatus,
		&latestLevel, &latestScore, &latestGrade, &createdAt, &runCount)
	if err != nil {
		return Domain{}, err
	}
	d := Domain{
		ID:           id,
		Name:         domainName,
		LatestRunID:  latestRunID.String,
		LatestRunAt:  parseTimestampNullStr(latestRunAt),
		LatestStatus: latestStatus.String,
		LatestLevel:  latestLevel.String,
		CreatedAt:    parseTimestampStr(createdAt),
		RunCount:     runCount,
	}
	if latestScore.Valid {
		v := int(latestScore.Int64)
		d.LatestScore = &v
	}
	if latestGrade.Valid {
		d.LatestGrade = &latestGrade.String
	}
	return d, nil
}

const domainSelectCols = `SELECT id, name, latest_run_id, latest_run_at, latest_status,
	latest_level, latest_score, latest_grade, created_at, run_count FROM domains`

func (s *SQLJobStore) getDomainByName(name string) (Domain, error) {
	row := s.db.QueryRow(domainSelectCols+` WHERE name = `+s.ph(1), name)
	return s.scanDomain(row)
}

// GetDomain returns a domain by its numeric ID.
func (s *SQLJobStore) GetDomain(id int64) (Domain, bool) {
	row := s.db.QueryRow(domainSelectCols+` WHERE id = `+s.ph(1), id)
	d, err := s.scanDomain(row)
	if err != nil {
		return Domain{}, false
	}
	return d, true
}

// GetDomainNamesByIDs returns a map of id to domain name for the given
// ids in a single SELECT. Used by the cohort materialization preload
// path to replace a per-domain GetDomain N+1 inside the /domains list
// handler.
func (s *SQLJobStore) GetDomainNamesByIDs(ids []int64) map[int64]string {
	out := map[int64]string{}
	if len(ids) == 0 {
		return out
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = s.ph(i + 1)
		args[i] = id
	}
	query := `SELECT id, name FROM domains WHERE id IN (` + strings.Join(placeholders, ", ") + `)`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var (
			id   int64
			name string
		)
		if err := rows.Scan(&id, &name); err != nil {
			return out
		}
		out[id] = name
	}
	return out
}

// GetDomainByName returns a domain by its name.
func (s *SQLJobStore) GetDomainByName(name string) (Domain, bool) {
	d, err := s.getDomainByName(name)
	if err != nil {
		return Domain{}, false
	}
	return d, true
}

// UpdateDomainLatest updates the denormalized latest_* fields on a domain.
func (s *SQLJobStore) UpdateDomainLatest(domainID int64, runID string, finishedAt time.Time, status, level string) error {
	ph := s.ph
	finishedAtVal := s.dialect.TimestampVal(finishedAt)
	_, err := s.db.Exec(
		`UPDATE domains SET latest_run_id = `+ph(1)+`, latest_run_at = `+ph(2)+
			`, latest_status = `+ph(3)+`, latest_level = `+ph(4)+
			`, run_count = run_count + 1 WHERE id = `+ph(5),
		runID, finishedAtVal, status, level, domainID,
	)
	return err
}

// ListDomains returns paginated domains.
func (s *SQLJobStore) ListDomains(filter DomainFilter) DomainList {
	var conds []string
	var args []any
	argN := 0
	addArg := func(v any) string {
		args = append(args, v)
		argN++
		return s.dialect.Placeholder(argN)
	}

	if filter.Tag == "__none__" {
		conds = append(conds, "id NOT IN (SELECT domain_id FROM domain_tags)")
	} else if filter.Tag != "" {
		conds = append(conds, "id IN (SELECT domain_id FROM domain_tags WHERE tag = "+addArg(filter.Tag)+")")
	}
	if filter.Name != "" {
		conds = append(conds, "LOWER(name) LIKE "+addArg("%"+strings.ToLower(filter.Name)+"%"))
	}
	if filter.LatestLevel != "" {
		conds = append(conds, "latest_level = "+addArg(filter.LatestLevel))
	}
	if filter.MinLevel != "" {
		switch strings.ToUpper(filter.MinLevel) {
		case "WARNING":
			conds = append(conds, "latest_level IN ('WARNING', 'ERROR', 'CRITICAL')")
		case "ERROR":
			conds = append(conds, "latest_level IN ('ERROR', 'CRITICAL')")
		case "CRITICAL":
			conds = append(conds, "latest_level = 'CRITICAL'")
		}
	}

	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	offset := max(filter.Offset, 0)

	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM domains"+where, args...).Scan(&total); err != nil {
		return DomainList{Limit: limit}
	}

	limitPH := s.dialect.Placeholder(argN + 1)
	offsetPH := s.dialect.Placeholder(argN + 2)
	dataArgs := append(args, limit, offset)

	query := `SELECT id, name, latest_run_id, latest_run_at, latest_status,
		latest_level, latest_score, latest_grade, created_at, run_count FROM domains` + where +
		" ORDER BY " + sqlDomainOrderBy(filter.Sort) + " LIMIT " + limitPH + " OFFSET " + offsetPH
	rows, err := s.db.Query(query, dataArgs...)
	if err != nil {
		return DomainList{Limit: limit}
	}
	defer rows.Close()

	// Collect domains first; close cursor before making nested tag queries.
	var items []Domain
	for rows.Next() {
		var (
			id                        int64
			name, createdAt           string
			latestRunID, latestRunAt  sql.NullString
			latestStatus, latestLevel sql.NullString
			latestScore               sql.NullInt64
			latestGrade               sql.NullString
			runCount                  int
		)
		if err := rows.Scan(&id, &name, &latestRunID, &latestRunAt,
			&latestStatus, &latestLevel, &latestScore, &latestGrade, &createdAt, &runCount); err != nil {
			continue
		}
		d := Domain{
			ID:           id,
			Name:         name,
			LatestRunID:  latestRunID.String,
			LatestRunAt:  parseTimestampNullStr(latestRunAt),
			LatestStatus: latestStatus.String,
			LatestLevel:  latestLevel.String,
			CreatedAt:    parseTimestampStr(createdAt),
			RunCount:     runCount,
		}
		if latestScore.Valid {
			v := int(latestScore.Int64)
			d.LatestScore = &v
		}
		if latestGrade.Valid {
			d.LatestGrade = &latestGrade.String
		}
		items = append(items, d)
	}
	rows.Close()

	// Load tags after the main cursor is closed.
	for i := range items {
		items[i].Tags = s.loadDomainTags(items[i].ID)
	}

	return DomainList{
		Items:  items,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}
}

func (s *SQLJobStore) loadDomainTags(domainID int64) []string {
	rows, err := s.db.Query(
		fmt.Sprintf("SELECT tag FROM domain_tags WHERE domain_id = %s ORDER BY tag", s.ph(1)),
		domainID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err == nil {
			tags = append(tags, tag)
		}
	}
	return tags
}

// ── Tag management ────────────────────────────────────────────────────────────

// CreateTag creates a new tag or does nothing if it already exists.
func (s *SQLJobStore) CreateTag(name, description string) error {
	now := formatSortableTimestamp(time.Now().UTC())
	switch s.dialect.(type) {
	case postgresDialect:
		_, err := s.db.Exec(
			`INSERT INTO tags (name, description, created_at) VALUES ($1, $2, $3) ON CONFLICT (name) DO NOTHING`,
			name, description, now)
		return err
	case mariadbDialect:
		_, err := s.db.Exec(
			`INSERT IGNORE INTO tags (name, description, created_at) VALUES (?, ?, ?)`,
			name, description, now)
		return err
	default:
		_, err := s.db.Exec(
			`INSERT OR IGNORE INTO tags (name, description, created_at) VALUES (?, ?, ?)`,
			name, description, now)
		return err
	}
}

// ListTags returns all tags ordered by name.
func (s *SQLJobStore) ListTags(limit, offset int) []Tag {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT t.name, t.description, t.created_at, t.default_profile_id,
			COUNT(dt.domain_id) AS domain_count
		 FROM tags t
		 LEFT JOIN domain_tags dt ON dt.tag = t.name
		 GROUP BY t.name, t.description, t.created_at, t.default_profile_id
		 ORDER BY t.name ASC
		 LIMIT %s OFFSET %s`, s.ph(1), s.ph(2)),
		limit, offset,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var tags []Tag
	for rows.Next() {
		var (
			name, description, createdAt string
			defaultProfileID             sql.NullInt64
			domainCount                  int
		)
		if err := rows.Scan(&name, &description, &createdAt, &defaultProfileID, &domainCount); err != nil {
			continue
		}
		tags = append(tags, Tag{
			Name:             name,
			Description:      description,
			CreatedAt:        parseTimestampStr(createdAt),
			DomainCount:      domainCount,
			DefaultProfileID: nullInt64Ptr(defaultProfileID),
		})
	}
	return tags
}

// TagDomains associates the given domain IDs with tag.
func (s *SQLJobStore) TagDomains(tag string, domainIDs []int64) error {
	// Ensure tag exists.
	if err := s.CreateTag(tag, ""); err != nil {
		return err
	}
	for _, id := range domainIDs {
		switch s.dialect.(type) {
		case postgresDialect:
			_, err := s.db.Exec(
				`INSERT INTO domain_tags (domain_id, tag) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
				id, tag)
			if err != nil {
				return err
			}
		case mariadbDialect:
			_, err := s.db.Exec(`INSERT IGNORE INTO domain_tags (domain_id, tag) VALUES (?, ?)`, id, tag)
			if err != nil {
				return err
			}
		default:
			_, err := s.db.Exec(`INSERT OR IGNORE INTO domain_tags (domain_id, tag) VALUES (?, ?)`, id, tag)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// GetTag returns a tag by name with its current domain count.
func (s *SQLJobStore) GetTag(name string) (Tag, bool) {
	var (
		tagName, description, createdAt string
		defaultProfileID                sql.NullInt64
		domainCount                     int
	)
	err := s.db.QueryRow(
		fmt.Sprintf(`SELECT t.name, t.description, t.created_at, t.default_profile_id,
			COUNT(dt.domain_id) AS domain_count
		 FROM tags t
		 LEFT JOIN domain_tags dt ON dt.tag = t.name
		 WHERE t.name = %s
		 GROUP BY t.name, t.description, t.created_at, t.default_profile_id`, s.ph(1)),
		name,
	).Scan(&tagName, &description, &createdAt, &defaultProfileID, &domainCount)
	if err != nil {
		return Tag{}, false
	}
	return Tag{
		Name:             tagName,
		Description:      description,
		CreatedAt:        parseTimestampStr(createdAt),
		DomainCount:      domainCount,
		DefaultProfileID: nullInt64Ptr(defaultProfileID),
	}, true
}

// UpdateTag updates the description of an existing tag.
func (s *SQLJobStore) UpdateTag(name, description string) error {
	res, err := s.db.Exec(
		fmt.Sprintf(`UPDATE tags SET description = %s WHERE name = %s`, s.ph(1), s.ph(2)),
		description, name,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("tag not found")
	}
	return nil
}

// SetTagDefaultProfile updates a tag's default stored profile reference.
func (s *SQLJobStore) SetTagDefaultProfile(name string, profileID *int64) error {
	if profileID != nil {
		if _, ok := s.GetProfile(*profileID); !ok {
			return fmt.Errorf("profile %d not found", *profileID)
		}
	}
	res, err := s.db.Exec(
		fmt.Sprintf(`UPDATE tags SET default_profile_id = %s WHERE name = %s`, s.ph(1), s.ph(2)),
		nullInt64Value(profileID), name,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("tag not found")
	}
	return nil
}

// DeleteTag removes a tag and all its domain associations.
func (s *SQLJobStore) DeleteTag(name string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("delete tag begin tx: %w", err)
	}
	if _, err := tx.Exec(
		fmt.Sprintf(`DELETE FROM domain_tags WHERE tag = %s`, s.ph(1)), name,
	); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("delete domain_tags: %w", err)
	}
	res, err := tx.Exec(
		fmt.Sprintf(`DELETE FROM tags WHERE name = %s`, s.ph(1)), name,
	)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("delete tag: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		_ = tx.Rollback()
		return errors.New("tag not found")
	}
	return tx.Commit()
}

// UntagDomains removes the given domain IDs from a tag.
func (s *SQLJobStore) UntagDomains(tag string, domainIDs []int64) error {
	for _, id := range domainIDs {
		if _, err := s.db.Exec(
			fmt.Sprintf(`DELETE FROM domain_tags WHERE tag = %s AND domain_id = %s`,
				s.ph(1), s.ph(2)),
			tag, id,
		); err != nil {
			return err
		}
	}
	return nil
}

// GetDomainTags returns tag names for a domain.
func (s *SQLJobStore) GetDomainTags(domainID int64) []string {
	return s.loadDomainTags(domainID)
}

// ListDomainsByTag returns domains associated with tag.
func (s *SQLJobStore) ListDomainsByTag(tag string, filter DomainFilter) DomainList {
	filter.Tag = tag
	return s.ListDomains(filter)
}

// GetTagSummary returns the severity distribution of latest runs for domains in tag.
func (s *SQLJobStore) GetTagSummary(tag string) (TagSummary, bool) {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT d.latest_level, r.grade
		 FROM domains d
		 JOIN domain_tags dt ON dt.domain_id = d.id
		 LEFT JOIN runs r ON r.id = d.latest_run_id
		 WHERE dt.tag = %s`, s.ph(1)),
		tag,
	)
	if err != nil {
		return TagSummary{}, false
	}
	defer rows.Close()

	summary := TagSummary{Tag: tag, Grades: map[string]int{}}
	found := false
	for rows.Next() {
		found = true
		var level sql.NullString
		var grade sql.NullString
		if err := rows.Scan(&level, &grade); err != nil {
			continue
		}
		summary.DomainCount++
		switch strings.ToUpper(level.String) {
		case "CRITICAL":
			summary.Critical++
		case "ERROR":
			summary.Error++
		case "WARNING":
			summary.Warning++
		case "NOTICE":
			summary.Notice++
		default:
			summary.OK++
		}
		if grade.Valid && grade.String != "" {
			summary.Grades[grade.String]++
		}
	}
	if !found {
		return TagSummary{}, false
	}
	return summary, true
}

// ── Run management ────────────────────────────────────────────────────────────

const runCols = `id, domain_id, domain, batch_id, status,
	created_at, started_at, finished_at, duration_ms,
	sev_notice, sev_warning, sev_error, sev_critical,
	worst_level, entry_count, profile, profile_id, profile_name,
	effective_profile, public_id, priority, score, grade, nameserver_timings_json,
	error`

func (s *SQLJobStore) scanRun(row rowScanner) (Run, error) {
	var (
		id, domain, batchID, status, worstLevel, profile, profileName, effectiveProfile string
		domainID, durationMs                                                            int64
		profileID                                                                       sql.NullInt64
		sevNotice, sevWarning, sevError, sevCritical                                    int
		entryCount                                                                      int
		createdAt                                                                       string
		startedAt, finishedAt                                                           sql.NullString
		publicID                                                                        sql.NullString
		priority                                                                        int
		score                                                                           sql.NullInt64
		grade                                                                           sql.NullString
		nameserverTimingsJSON                                                           sql.NullString
		runError                                                                        string
	)
	if err := row.Scan(
		&id, &domainID, &domain, &batchID, &status,
		&createdAt, &startedAt, &finishedAt, &durationMs,
		&sevNotice, &sevWarning, &sevError, &sevCritical,
		&worstLevel, &entryCount, &profile, &profileID, &profileName,
		&effectiveProfile, &publicID, &priority, &score, &grade, &nameserverTimingsJSON,
		&runError,
	); err != nil {
		return Run{}, err
	}
	r := Run{
		ID:               id,
		DomainID:         domainID,
		Domain:           domain,
		BatchID:          batchID,
		Status:           JobStatus(status),
		CreatedAt:        parseTimestampStr(createdAt),
		StartedAt:        parseTimestampNullStr(startedAt),
		FinishedAt:       parseTimestampNullStr(finishedAt),
		DurationMs:       durationMs,
		SevNotice:        sevNotice,
		SevWarning:       sevWarning,
		SevError:         sevError,
		SevCritical:      sevCritical,
		WorstLevel:       worstLevel,
		EntryCount:       entryCount,
		Profile:          profile,
		ProfileID:        nullInt64Ptr(profileID),
		ProfileName:      profileName,
		EffectiveProfile: effectiveProfile,
		PublicID:         publicID.String,
		Priority:         JobPriority(priority),
		Error:            runError,
	}
	timings, err := unmarshalNullJSON[[]NameserverTiming](nameserverTimingsJSON)
	if err != nil {
		return Run{}, fmt.Errorf("unmarshal nameserver_timings_json: %w", err)
	}
	r.NameserverTimings = timings
	if score.Valid {
		v := int(score.Int64)
		r.Score = &v
	}
	if grade.Valid {
		r.Grade = &grade.String
	}
	r.SeverityTotals = map[string]int{
		"NOTICE":   sevNotice,
		"WARNING":  sevWarning,
		"ERROR":    sevError,
		"CRITICAL": sevCritical,
	}
	return r, nil
}

// lazyComputeScore loads entries for runID, computes the score, writes it back
// to the DB, and sets Score/Grade on run. Used for rows stored before scoring
// existed, which carry a NULL score.
func (s *SQLJobStore) lazyComputeScore(run *Run) {
	entries, err := s.loadEntries(run.ID)
	if err != nil {
		return
	}
	scoringEntries := make([]scoring.Entry, len(entries))
	for i, e := range entries {
		scoringEntries[i] = scoring.Entry{Module: e.Module, Tag: e.Tag, Level: e.Level}
	}
	sr := scoring.Compute(run.Domain, scoringEntries, s.scoringCfg)
	scoreVal := int(sr.Score)
	run.Score = &scoreVal
	run.Grade = &sr.Grade
	// Cache in DB; ignore errors - the in-memory value is still set.
	_, _ = s.db.Exec(
		fmt.Sprintf("UPDATE runs SET score = %s, grade = %s WHERE id = %s",
			s.ph(1), s.ph(2), s.ph(3)),
		sr.Score, sr.Grade, run.ID,
	)
}

// GetRun returns a graduated run by ID.
func (s *SQLJobStore) GetRun(id string) (Run, bool) {
	row := s.db.QueryRow(
		fmt.Sprintf("SELECT %s FROM runs WHERE id = %s", runCols, s.ph(1)), id)
	run, err := s.scanRun(row)
	if err != nil {
		return Run{}, false
	}
	if run.Score == nil {
		s.lazyComputeScore(&run)
	}
	return run, true
}

// GetRunByPublicID returns a graduated run by public_id.
func (s *SQLJobStore) GetRunByPublicID(publicID string) (Run, bool) {
	row := s.db.QueryRow(
		fmt.Sprintf("SELECT %s FROM runs WHERE public_id = %s", runCols, s.ph(1)), publicID)
	run, err := s.scanRun(row)
	if err != nil {
		return Run{}, false
	}
	if run.Score == nil {
		s.lazyComputeScore(&run)
	}
	return run, true
}

// sqlDomainOrderBy returns the ORDER BY clause for the domains table.
func sqlDomainOrderBy(sort DomainSort) string {
	switch sort {
	case DomainSortNameDesc:
		return "LOWER(name) DESC, id ASC"
	case DomainSortLevelDesc:
		return "CASE latest_level WHEN 'CRITICAL' THEN 4 WHEN 'ERROR' THEN 3 WHEN 'WARNING' THEN 2 WHEN 'NOTICE' THEN 1 ELSE 0 END DESC, LOWER(name) ASC, id ASC"
	case DomainSortLevelAsc:
		return "CASE latest_level WHEN 'CRITICAL' THEN 4 WHEN 'ERROR' THEN 3 WHEN 'WARNING' THEN 2 WHEN 'NOTICE' THEN 1 ELSE 0 END ASC, LOWER(name) ASC, id ASC"
	case DomainSortScoreDesc:
		return "COALESCE(latest_score, -1) DESC, LOWER(name) ASC, id ASC"
	case DomainSortScoreAsc:
		return "COALESCE(latest_score, 9999) ASC, LOWER(name) ASC, id ASC"
	case DomainSortLastRunDesc:
		return "COALESCE(latest_run_at, '') DESC, LOWER(name) ASC, id ASC"
	case DomainSortLastRunAsc:
		return "COALESCE(latest_run_at, '') ASC, LOWER(name) ASC, id ASC"
	case DomainSortRunCountDesc:
		return "run_count DESC, LOWER(name) ASC, id ASC"
	case DomainSortRunCountAsc:
		return "run_count ASC, LOWER(name) ASC, id ASC"
	default:
		return "LOWER(name) ASC, id ASC"
	}
}

// sqlRunOrderBy returns the ORDER BY clause for the runs table.
func sqlRunOrderBy(sort JobSort) string {
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
		return "LOWER(domain) ASC, finished_at DESC, id ASC"
	case JobSortDomainDesc:
		return "LOWER(domain) DESC, finished_at DESC, id ASC"
	case JobSortBatchIDAsc:
		return "LOWER(batch_id) ASC, finished_at DESC, id ASC"
	case JobSortBatchIDDesc:
		return "LOWER(batch_id) DESC, finished_at DESC, id ASC"
	case JobSortErrorDesc:
		return "(sev_error + sev_critical) DESC, sev_critical DESC, finished_at DESC, id ASC"
	case JobSortCriticalDesc:
		return "sev_critical DESC, (sev_error + sev_critical) DESC, finished_at DESC, id ASC"
	case JobSortFinishedAtDesc:
		return "finished_at DESC, id ASC"
	case JobSortFinishedAtAsc:
		return "finished_at ASC, id ASC"
	case JobSortWorstLevelDesc:
		return "CASE worst_level WHEN 'CRITICAL' THEN 4 WHEN 'ERROR' THEN 3 WHEN 'WARNING' THEN 2 WHEN 'NOTICE' THEN 1 ELSE 0 END DESC, finished_at DESC, id ASC"
	case JobSortWorstLevelAsc:
		return "CASE worst_level WHEN 'CRITICAL' THEN 4 WHEN 'ERROR' THEN 3 WHEN 'WARNING' THEN 2 WHEN 'NOTICE' THEN 1 ELSE 0 END ASC, finished_at DESC, id ASC"
	case JobSortScoreDesc:
		return "COALESCE(score, -1) DESC, finished_at DESC, id ASC"
	case JobSortScoreAsc:
		return "COALESCE(score, 9999) ASC, finished_at DESC, id ASC"
	case JobSortDurationDesc:
		return "COALESCE(duration_ms, -1) DESC, finished_at DESC, id ASC"
	case JobSortDurationAsc:
		return "COALESCE(duration_ms, 9999999) ASC, finished_at DESC, id ASC"
	case JobSortEntryCountDesc:
		return "entry_count DESC, finished_at DESC, id ASC"
	case JobSortEntryCountAsc:
		return "entry_count ASC, finished_at DESC, id ASC"
	default:
		return "finished_at DESC, id ASC"
	}
}

// ListRuns returns paginated graduated runs matching filter.
func (s *SQLJobStore) ListRuns(filter RunFilter) RunList {
	var conds []string
	var args []any
	argN := 0
	addArg := func(v any) string {
		args = append(args, v)
		argN++
		return s.dialect.Placeholder(argN)
	}

	if filter.DomainID != 0 {
		conds = append(conds, "domain_id = "+addArg(filter.DomainID))
	}
	if filter.Tag != "" {
		conds = append(conds, "domain_id IN (SELECT domain_id FROM domain_tags WHERE tag = "+addArg(filter.Tag)+")")
	}
	if filter.EntryTag != "" {
		conds = append(conds, "EXISTS (SELECT 1 FROM entries e WHERE e.run_id = runs.id AND e.tag = "+addArg(filter.EntryTag)+")")
	}
	if filter.Domain != "" {
		conds = append(conds, "LOWER(domain) LIKE "+addArg("%"+strings.ToLower(filter.Domain)+"%"))
	}
	if filter.BatchID != "" {
		conds = append(conds, "batch_id = "+addArg(filter.BatchID))
	}
	if filter.Status != "" {
		conds = append(conds, "status = "+addArg(string(filter.Status)))
	}
	if filter.WorstLevel != "" {
		conds = append(conds, "worst_level = "+addArg(filter.WorstLevel))
	}
	if filter.Severity == JobSeverityWarningsPlus {
		conds = append(conds, "(sev_warning > 0 OR sev_error > 0 OR sev_critical > 0)")
	} else if filter.Severity == JobSeverityErrorsOnly {
		conds = append(conds, "(sev_error > 0 OR sev_critical > 0)")
	}
	if filter.Grade != "" {
		conds = append(conds, "grade = "+addArg(filter.Grade))
	}
	if !filter.FinishedAfter.IsZero() {
		conds = append(conds, "finished_at > "+addArg(formatSortableTimestamp(filter.FinishedAfter)))
	}
	if !filter.FinishedBefore.IsZero() {
		conds = append(conds, "finished_at < "+addArg(formatSortableTimestamp(filter.FinishedBefore)))
	}

	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	orderBy := sqlRunOrderBy(filter.Sort)
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	offset := max(filter.Offset, 0)

	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM runs"+where, args...).Scan(&total); err != nil {
		return RunList{Limit: limit}
	}

	limitPH := s.dialect.Placeholder(argN + 1)
	offsetPH := s.dialect.Placeholder(argN + 2)
	dataArgs := append(args, limit, offset)

	rows, err := s.db.Query(
		"SELECT "+runCols+" FROM runs"+where+
			" ORDER BY "+orderBy+
			" LIMIT "+limitPH+" OFFSET "+offsetPH,
		dataArgs...,
	)
	if err != nil {
		return RunList{Limit: limit}
	}
	defer rows.Close()

	var items []Run
	for rows.Next() {
		run, err := s.scanRun(rows)
		if err != nil {
			continue
		}
		items = append(items, run)
	}
	if err := rows.Err(); err != nil {
		return RunList{Limit: limit}
	}

	list := RunList{
		Items:  items,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}
	if offset > 0 {
		prev := max(offset-limit, 0)
		list.PrevCursor = fmt.Sprintf("%d", prev)
	}
	if offset+len(items) < total {
		list.NextCursor = fmt.Sprintf("%d", offset+len(items))
	}
	return list
}

// ListRunsByDomain returns paginated runs for a specific domain ordered by finished_at DESC.
func (s *SQLJobStore) ListRunsByDomain(domainID int64, limit, offset int) RunList {
	return s.ListRuns(RunFilter{DomainID: domainID, Limit: limit, Offset: offset})
}

// ── Entry queries ─────────────────────────────────────────────────────────────

// QueryEntries returns entries across runs matching the given filter.
func (s *SQLJobStore) QueryEntries(filter EntryFilter) EntryList {
	var conds []string
	var args []any
	argN := 0
	addArg := func(v any) string {
		args = append(args, v)
		argN++
		return s.dialect.Placeholder(argN)
	}

	base := "FROM entries e"

	if filter.Tag != "" {
		base += " JOIN domain_tags dt ON dt.domain_id = e.domain_id AND dt.tag = " + addArg(filter.Tag)
	}
	if filter.LatestOnly {
		base += " JOIN domains d ON d.id = e.domain_id AND d.latest_run_id = e.run_id"
	}
	if filter.BatchID != "" {
		base += " JOIN runs r ON r.id = e.run_id AND r.batch_id = " + addArg(filter.BatchID)
	}

	if filter.RunID != "" {
		conds = append(conds, "e.run_id = "+addArg(filter.RunID))
	}
	if filter.DomainID != 0 {
		conds = append(conds, "e.domain_id = "+addArg(filter.DomainID))
	}
	if filter.Module != "" {
		conds = append(conds, "LOWER(e.module) = LOWER("+addArg(filter.Module)+")")
	}
	if filter.Testcase != "" {
		conds = append(conds, "LOWER(e.testcase) = LOWER("+addArg(filter.Testcase)+")")
	}
	if filter.EntryTag != "" {
		conds = append(conds, "e.tag = "+addArg(filter.EntryTag))
	}
	if filter.Level != "" {
		conds = append(conds, "e.level = "+addArg(filter.Level))
	}

	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	offset := max(filter.Offset, 0)

	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) "+base+where, args...).Scan(&total); err != nil {
		return EntryList{Limit: limit}
	}

	limitPH := s.dialect.Placeholder(argN + 1)
	offsetPH := s.dialect.Placeholder(argN + 2)
	dataArgs := append(args, limit, offset)

	rows, err := s.db.Query(
		"SELECT e.id, e.run_id, e.domain_id, e.timestamp, e.module, e.testcase, e.tag, e.level, e.args_json "+
			base+where+
			" ORDER BY e.run_id ASC, e.id ASC"+
			" LIMIT "+limitPH+" OFFSET "+offsetPH,
		dataArgs...,
	)
	if err != nil {
		return EntryList{Limit: limit}
	}
	defer rows.Close()

	var items []Entry
	for rows.Next() {
		var (
			id                                int64
			runID, module, testcase, tag, lvl string
			domainID                          int64
			timestamp                         float64
			argsJSON                          sql.NullString
		)
		if err := rows.Scan(&id, &runID, &domainID, &timestamp, &module, &testcase, &tag, &lvl, &argsJSON); err != nil {
			continue
		}
		var entryArgs map[string]any
		if argsJSON.Valid {
			_ = json.Unmarshal([]byte(argsJSON.String), &entryArgs)
		}
		items = append(items, Entry{
			ID:        id,
			RunID:     runID,
			DomainID:  domainID,
			Timestamp: timestamp,
			Module:    module,
			Testcase:  testcase,
			Tag:       tag,
			Level:     lvl,
			Args:      entryArgs,
		})
	}
	if err := rows.Err(); err != nil {
		return EntryList{Limit: limit}
	}

	list := EntryList{
		Items:  items,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}
	if offset > 0 {
		prev := max(offset-limit, 0)
		list.PrevCursor = fmt.Sprintf("%d", prev)
	}
	if offset+len(items) < total {
		list.NextCursor = fmt.Sprintf("%d", offset+len(items))
	}
	return list
}

// ── Batch management ──────────────────────────────────────────────────────────

// CreateBatch inserts a batch record.
func (s *SQLJobStore) CreateBatch(batch Batch) error {
	snapshotIntent := boolToInt(batch.SnapshotIntent)
	switch s.dialect.(type) {
	case postgresDialect:
		_, err := s.db.Exec(
			`INSERT INTO batches (id, tag, created_at, domain_count, description, snapshot_intent)
			 VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (id) DO NOTHING`,
			batch.ID, batch.Tag, formatSortableTimestamp(batch.CreatedAt),
			batch.DomainCount, batch.Description, snapshotIntent)
		return err
	case mariadbDialect:
		_, err := s.db.Exec(
			`INSERT IGNORE INTO batches (id, tag, created_at, domain_count, description, snapshot_intent)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			batch.ID, batch.Tag, formatSortableTimestamp(batch.CreatedAt),
			batch.DomainCount, batch.Description, snapshotIntent)
		return err
	default:
		_, err := s.db.Exec(
			`INSERT OR IGNORE INTO batches (id, tag, created_at, domain_count, description, snapshot_intent)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			batch.ID, batch.Tag, formatSortableTimestamp(batch.CreatedAt),
			batch.DomainCount, batch.Description, snapshotIntent)
		return err
	}
}

// ListBatchesByTag returns batches whose tag matches, newest first.
func (s *SQLJobStore) ListBatchesByTag(tag string, limit, offset int) BatchList {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	out := BatchList{Limit: limit, Offset: offset}
	if err := s.db.QueryRow(
		fmt.Sprintf(`SELECT COUNT(*) FROM batches WHERE tag = %s`, s.ph(1)),
		tag,
	).Scan(&out.Total); err != nil {
		return out
	}
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT id, tag, created_at, domain_count, description, snapshot_intent
			FROM batches WHERE tag = %s
			ORDER BY created_at DESC, id DESC
			LIMIT %s OFFSET %s`, s.ph(1), s.ph(2), s.ph(3)),
		tag, limit, offset,
	)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var (
			b              Batch
			createdAt      string
			snapshotIntent int
		)
		if err := rows.Scan(&b.ID, &b.Tag, &createdAt, &b.DomainCount, &b.Description, &snapshotIntent); err != nil {
			return out
		}
		b.CreatedAt = parseTimestampStr(createdAt)
		b.SnapshotIntent = intToBool(snapshotIntent)
		out.Items = append(out.Items, b)
	}
	return out
}

// ListBatches returns batches newest first, optionally filtered by a
// case-insensitive substring of the batch tag.
func (s *SQLJobStore) ListBatches(tagLike string, limit, offset int) BatchList {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	out := BatchList{Limit: limit, Offset: offset}

	where := ""
	var filterArgs []any
	if tagLike != "" {
		where = fmt.Sprintf(" WHERE LOWER(tag) LIKE %s", s.ph(1))
		filterArgs = append(filterArgs, "%"+strings.ToLower(tagLike)+"%")
	}

	if err := s.db.QueryRow(`SELECT COUNT(*) FROM batches`+where, filterArgs...).Scan(&out.Total); err != nil {
		return out
	}
	dataArgs := append(append([]any{}, filterArgs...), limit, offset)
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT id, tag, created_at, domain_count, description, snapshot_intent
			FROM batches%s
			ORDER BY created_at DESC, id DESC
			LIMIT %s OFFSET %s`, where, s.ph(len(filterArgs)+1), s.ph(len(filterArgs)+2)),
		dataArgs...,
	)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var (
			b              Batch
			createdAt      string
			snapshotIntent int
		)
		if err := rows.Scan(&b.ID, &b.Tag, &createdAt, &b.DomainCount, &b.Description, &snapshotIntent); err != nil {
			return out
		}
		b.CreatedAt = parseTimestampStr(createdAt)
		b.SnapshotIntent = intToBool(snapshotIntent)
		out.Items = append(out.Items, b)
	}
	return out
}

// BatchDeletePreviewStats counts the rows that a DeleteBatch call
// would remove. Snapshot list is left to the handler to fill in.
func (s *SQLJobStore) BatchDeletePreviewStats(batchID string) (BatchDeletePreview, error) {
	out := BatchDeletePreview{BatchID: batchID}
	if b, ok := s.GetBatch(batchID); ok {
		out.Exists = true
		out.Tag = b.Tag
		out.CreatedAt = b.CreatedAt
		out.SnapshotIntent = b.SnapshotIntent
	}

	queryCount := func(q string, args ...any) (int, error) {
		var n int
		if err := s.db.QueryRow(q, args...).Scan(&n); err != nil {
			return 0, err
		}
		return n, nil
	}

	n, err := queryCount(
		fmt.Sprintf(`SELECT COUNT(*) FROM jobs WHERE batch_id = %s AND status = 'queued'`, s.ph(1)),
		batchID,
	)
	if err != nil {
		return out, fmt.Errorf("count queued jobs: %w", err)
	}
	out.QueuedJobs = n
	if n > 0 {
		out.Exists = true
	}

	n, err = queryCount(
		fmt.Sprintf(`SELECT COUNT(*) FROM jobs WHERE batch_id = %s AND status = 'running'`, s.ph(1)),
		batchID,
	)
	if err != nil {
		return out, fmt.Errorf("count running jobs: %w", err)
	}
	out.RunningJobs = n
	if n > 0 {
		out.Exists = true
	}

	n, err = queryCount(
		fmt.Sprintf(`SELECT COUNT(*) FROM runs WHERE batch_id = %s`, s.ph(1)),
		batchID,
	)
	if err != nil {
		return out, fmt.Errorf("count runs: %w", err)
	}
	out.CompletedRuns = n
	if n > 0 {
		out.Exists = true
	}

	n, err = queryCount(
		fmt.Sprintf(`SELECT COUNT(*) FROM entries WHERE run_id IN (SELECT id FROM runs WHERE batch_id = %s)`, s.ph(1)),
		batchID,
	)
	if err != nil {
		return out, fmt.Errorf("count entries: %w", err)
	}
	out.Entries = n

	factTables := []string{
		"analysis_run_domain_summary",
		"analysis_run_ns_endpoints",
		"analysis_run_address_asns",
		"analysis_run_domain_asns",
		"analysis_run_tag_summary",
		"analysis_run_domain_facts",
	}
	for _, tbl := range factTables {
		n, err := queryCount(
			fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE run_id IN (SELECT id FROM runs WHERE batch_id = %s)`, tbl, s.ph(1)),
			batchID,
		)
		if err != nil {
			return out, fmt.Errorf("count %s: %w", tbl, err)
		}
		out.FactRows += n
	}

	return out, nil
}

// BatchHasRuns reports whether any run row still exists for batchID.
func (s *SQLJobStore) BatchHasRuns(batchID string) bool {
	if batchID == "" {
		return false
	}
	var present int
	err := s.db.QueryRow(
		fmt.Sprintf(`SELECT 1 FROM runs WHERE batch_id = %s LIMIT 1`, s.ph(1)),
		batchID,
	).Scan(&present)
	if err != nil {
		return false
	}
	return present == 1
}

// DeleteBatch removes a batch and every row derived from it inside a
// single transaction: snapshot aggregates, snapshots, analysis_run_*
// fact rows, entries, runs, and jobs whose batch_id matches. Returns
// the snapshot IDs that were removed so callers can evict the
// materialization cache.
func (s *SQLJobStore) DeleteBatch(batchID string) ([]int64, error) {
	if batchID == "" {
		return nil, errors.New("batch id is required")
	}
	ph := s.ph(1)

	snapshotRows, err := s.db.Query(
		fmt.Sprintf(`SELECT id FROM analysis_cohort_snapshots WHERE batch_id = %s`, ph),
		batchID,
	)
	if err != nil {
		return nil, fmt.Errorf("list snapshots for batch: %w", err)
	}
	var snapshotIDs []int64
	for snapshotRows.Next() {
		var id int64
		if err := snapshotRows.Scan(&id); err != nil {
			snapshotRows.Close()
			return nil, fmt.Errorf("scan snapshot id: %w", err)
		}
		snapshotIDs = append(snapshotIDs, id)
	}
	snapshotRows.Close()

	runRows, err := s.db.Query(
		fmt.Sprintf(`SELECT id, domain_id FROM runs WHERE batch_id = %s`, ph),
		batchID,
	)
	if err != nil {
		return nil, fmt.Errorf("list runs for batch: %w", err)
	}
	var runIDs []string
	affectedDomains := map[int64]struct{}{}
	for runRows.Next() {
		var (
			id       string
			domainID int64
		)
		if err := runRows.Scan(&id, &domainID); err != nil {
			runRows.Close()
			return nil, fmt.Errorf("scan run id: %w", err)
		}
		runIDs = append(runIDs, id)
		if domainID != 0 {
			affectedDomains[domainID] = struct{}{}
		}
	}
	runRows.Close()

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("delete batch begin tx: %w", err)
	}
	rollback := func(cause error) ([]int64, error) {
		_ = tx.Rollback()
		return nil, cause
	}

	runSetSubquery := fmt.Sprintf(`SELECT id FROM runs WHERE batch_id = %s`, ph)
	steps := []struct {
		label string
		query string
	}{
		{"analysis_snapshot_overview_view", fmt.Sprintf(
			`DELETE FROM analysis_snapshot_overview_view
			  WHERE snapshot_id IN (SELECT id FROM analysis_cohort_snapshots WHERE batch_id = %s)`, ph)},
		{"analysis_snapshot_domain_view", fmt.Sprintf(
			`DELETE FROM analysis_snapshot_domain_view
			  WHERE snapshot_id IN (SELECT id FROM analysis_cohort_snapshots WHERE batch_id = %s)`, ph)},
		{"analysis_snapshot_prefix_view", fmt.Sprintf(
			`DELETE FROM analysis_snapshot_prefix_view
			  WHERE snapshot_id IN (SELECT id FROM analysis_cohort_snapshots WHERE batch_id = %s)`, ph)},
		{"analysis_snapshot_nameserver_view", fmt.Sprintf(
			`DELETE FROM analysis_snapshot_nameserver_view
			  WHERE snapshot_id IN (SELECT id FROM analysis_cohort_snapshots WHERE batch_id = %s)`, ph)},
		{"analysis_snapshot_endpoint_view", fmt.Sprintf(
			`DELETE FROM analysis_snapshot_endpoint_view
			  WHERE snapshot_id IN (SELECT id FROM analysis_cohort_snapshots WHERE batch_id = %s)`, ph)},
		{"analysis_snapshot_asn_view", fmt.Sprintf(
			`DELETE FROM analysis_snapshot_asn_view
			  WHERE snapshot_id IN (SELECT id FROM analysis_cohort_snapshots WHERE batch_id = %s)`, ph)},
		{"analysis_snapshot_tag_view", fmt.Sprintf(
			`DELETE FROM analysis_snapshot_tag_view
			  WHERE snapshot_id IN (SELECT id FROM analysis_cohort_snapshots WHERE batch_id = %s)`, ph)},
		{"snapshots", fmt.Sprintf(`DELETE FROM analysis_cohort_snapshots WHERE batch_id = %s`, ph)},
		{"analysis_run_ns_endpoints", fmt.Sprintf(`DELETE FROM analysis_run_ns_endpoints WHERE run_id IN (%s)`, runSetSubquery)},
		{"analysis_run_address_asns", fmt.Sprintf(`DELETE FROM analysis_run_address_asns WHERE run_id IN (%s)`, runSetSubquery)},
		{"analysis_run_domain_asns", fmt.Sprintf(`DELETE FROM analysis_run_domain_asns WHERE run_id IN (%s)`, runSetSubquery)},
		{"analysis_run_tag_summary", fmt.Sprintf(`DELETE FROM analysis_run_tag_summary WHERE run_id IN (%s)`, runSetSubquery)},
		{"analysis_run_domain_facts", fmt.Sprintf(`DELETE FROM analysis_run_domain_facts WHERE run_id IN (%s)`, runSetSubquery)},
		{"analysis_run_domain_summary", fmt.Sprintf(`DELETE FROM analysis_run_domain_summary WHERE run_id IN (%s)`, runSetSubquery)},
		{"entries", fmt.Sprintf(`DELETE FROM entries WHERE run_id IN (%s)`, runSetSubquery)},
		{"run_dnssec_chain", fmt.Sprintf(`DELETE FROM run_dnssec_chain WHERE run_id IN (%s)`, runSetSubquery)},
		{"runs", fmt.Sprintf(`DELETE FROM runs WHERE batch_id = %s`, ph)},
		{"jobs", fmt.Sprintf(`DELETE FROM jobs WHERE batch_id = %s`, ph)},
		{"batches", fmt.Sprintf(`DELETE FROM batches WHERE id = %s`, ph)},
	}
	for _, step := range steps {
		if _, err := tx.Exec(step.query, batchID); err != nil {
			return rollback(fmt.Errorf("delete batch %s step %s: %w", batchID, step.label, err))
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("delete batch commit: %w", err)
	}

	if err := s.refreshDomainsAfterBatchDelete(affectedDomains, runIDs); err != nil {
		return snapshotIDs, fmt.Errorf("refresh domain latest after delete: %w", err)
	}
	return snapshotIDs, nil
}

// refreshDomainsAfterBatchDelete re-derives domains.latest_* for every
// domain whose prior latest_run_id was among the batch's deleted runs
// and recomputes its run_count. Runs outside the deleted set are
// authoritative; the new latest is the most-recent remaining run for
// that domain, or empty when no runs remain.
func (s *SQLJobStore) refreshDomainsAfterBatchDelete(domainIDs map[int64]struct{}, deletedRunIDs []string) error {
	if len(domainIDs) == 0 {
		return nil
	}
	deletedSet := make(map[string]struct{}, len(deletedRunIDs))
	for _, id := range deletedRunIDs {
		deletedSet[id] = struct{}{}
	}
	for domainID := range domainIDs {
		var latestRunID sql.NullString
		if err := s.db.QueryRow(
			fmt.Sprintf(`SELECT latest_run_id FROM domains WHERE id = %s`, s.ph(1)),
			domainID,
		).Scan(&latestRunID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return err
		}
		if latestRunID.Valid && latestRunID.String != "" {
			if _, stale := deletedSet[latestRunID.String]; !stale {
				continue
			}
		}
		var (
			newRunID     sql.NullString
			newFinished  sql.NullString
			newStatus    sql.NullString
			newLevel     sql.NullString
			newScore     sql.NullInt64
			newGrade     sql.NullString
			remainingRow int
		)
		err := s.db.QueryRow(
			fmt.Sprintf(`SELECT id, finished_at, status, worst_level, score, grade FROM runs
				WHERE domain_id = %s
				ORDER BY finished_at DESC, id DESC
				LIMIT 1`, s.ph(1)),
			domainID,
		).Scan(&newRunID, &newFinished, &newStatus, &newLevel, &newScore, &newGrade)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err := s.db.QueryRow(
			fmt.Sprintf(`SELECT COUNT(*) FROM runs WHERE domain_id = %s`, s.ph(1)),
			domainID,
		).Scan(&remainingRow); err != nil {
			return err
		}
		if _, err := s.db.Exec(
			fmt.Sprintf(`UPDATE domains SET
				latest_run_id = %s,
				latest_run_at = %s,
				latest_status = %s,
				latest_level  = %s,
				latest_score  = %s,
				latest_grade  = %s,
				run_count     = %s
				WHERE id = %s`,
				s.ph(1), s.ph(2), s.ph(3), s.ph(4), s.ph(5), s.ph(6), s.ph(7), s.ph(8)),
			nullStringOrEmpty(newRunID),
			nullStringOrNil(newFinished),
			nullStringOrEmpty(newStatus),
			nullStringOrEmpty(newLevel),
			nullInt64Or(newScore),
			nullStringOrNil(newGrade),
			remainingRow,
			domainID,
		); err != nil {
			return err
		}
	}
	return nil
}

func nullStringOrEmpty(ns sql.NullString) any {
	if !ns.Valid {
		return ""
	}
	return ns.String
}

func nullStringOrNil(ns sql.NullString) any {
	if !ns.Valid {
		return nil
	}
	return ns.String
}

func nullInt64Or(ni sql.NullInt64) any {
	if !ni.Valid {
		return nil
	}
	return ni.Int64
}

// SetBatchSnapshotIntent flips snapshot_intent on an existing batch.
func (s *SQLJobStore) SetBatchSnapshotIntent(batchID string, intent bool) error {
	res, err := s.db.Exec(
		fmt.Sprintf(`UPDATE batches SET snapshot_intent = %s WHERE id = %s`, s.ph(1), s.ph(2)),
		boolToInt(intent), batchID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrBatchNotFound
	}
	return nil
}

// GetBatch returns a batch by ID.
func (s *SQLJobStore) GetBatch(id string) (Batch, bool) {
	var (
		batchID, tag, createdAt, description string
		domainCount                          int
		snapshotIntent                       int
	)
	err := s.db.QueryRow(
		fmt.Sprintf(`SELECT id, tag, created_at, domain_count, description, snapshot_intent FROM batches WHERE id = %s`, s.ph(1)),
		id,
	).Scan(&batchID, &tag, &createdAt, &domainCount, &description, &snapshotIntent)
	if err != nil {
		return Batch{}, false
	}
	return Batch{
		ID:             batchID,
		Tag:            tag,
		CreatedAt:      parseTimestampStr(createdAt),
		DomainCount:    domainCount,
		Description:    description,
		SnapshotIntent: intToBool(snapshotIntent),
	}, true
}

// ── Profiles ──────────────────────────────────────────────────────────────────

// CreateProfile inserts a new profile and returns it with the assigned ID.
func (s *SQLJobStore) CreateProfile(p StoredProfile) (StoredProfile, error) {
	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = now
	}
	publicInt := 0
	if p.Public {
		publicInt = 1
	}
	result, err := s.db.Exec(
		fmt.Sprintf(`INSERT INTO profiles (name, description, config, public, schema_version, created_at, updated_at)
			VALUES (%s, %s, %s, %s, %s, %s, %s)`,
			s.ph(1), s.ph(2), s.ph(3), s.ph(4), s.ph(5), s.ph(6), s.ph(7)),
		p.Name, p.Description, p.Config, publicInt, p.SchemaVersion,
		formatSortableTimestamp(p.CreatedAt), formatSortableTimestamp(p.UpdatedAt))
	if err != nil {
		return StoredProfile{}, fmt.Errorf("create profile: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		// PostgreSQL doesn't support LastInsertId; query for it.
		err2 := s.db.QueryRow(
			fmt.Sprintf(`SELECT id FROM profiles WHERE name = %s`, s.ph(1)), p.Name,
		).Scan(&id)
		if err2 != nil {
			return StoredProfile{}, fmt.Errorf("create profile: get id: %w", err2)
		}
	}
	p.ID = id
	return p, nil
}

// GetProfile returns a profile by ID.
func (s *SQLJobStore) GetProfile(id int64) (StoredProfile, bool) {
	return s.scanProfile(
		fmt.Sprintf(`SELECT id, name, description, config, public, schema_version, created_at, updated_at FROM profiles WHERE id = %s`, s.ph(1)),
		id)
}

// GetProfileByName returns a profile by its unique name.
func (s *SQLJobStore) GetProfileByName(name string) (StoredProfile, bool) {
	return s.scanProfile(
		fmt.Sprintf(`SELECT id, name, description, config, public, schema_version, created_at, updated_at FROM profiles WHERE name = %s`, s.ph(1)),
		name)
}

func (s *SQLJobStore) scanProfile(query string, args ...any) (StoredProfile, bool) {
	var (
		p                    StoredProfile
		publicInt            int
		createdAt, updatedAt string
	)
	err := s.db.QueryRow(query, args...).Scan(
		&p.ID, &p.Name, &p.Description, &p.Config, &publicInt, &p.SchemaVersion, &createdAt, &updatedAt)
	if err != nil {
		return StoredProfile{}, false
	}
	p.Public = publicInt != 0
	p.CreatedAt = parseTimestampStr(createdAt)
	p.UpdatedAt = parseTimestampStr(updatedAt)
	return p, true
}

// UpdateProfile updates an existing profile.
func (s *SQLJobStore) UpdateProfile(p StoredProfile) error {
	p.UpdatedAt = time.Now().UTC()
	publicInt := 0
	if p.Public {
		publicInt = 1
	}
	result, err := s.db.Exec(
		fmt.Sprintf(`UPDATE profiles SET name = %s, description = %s, config = %s, public = %s, schema_version = %s, updated_at = %s WHERE id = %s`,
			s.ph(1), s.ph(2), s.ph(3), s.ph(4), s.ph(5), s.ph(6), s.ph(7)),
		p.Name, p.Description, p.Config, publicInt, p.SchemaVersion,
		formatSortableTimestamp(p.UpdatedAt), p.ID)
	if err != nil {
		return fmt.Errorf("update profile: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("profile %d not found", p.ID)
	}
	return nil
}

// DeleteProfile removes a profile by ID.
func (s *SQLJobStore) DeleteProfile(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("delete profile begin tx: %w", err)
	}
	for _, query := range []string{
		fmt.Sprintf(`UPDATE tags SET default_profile_id = NULL WHERE default_profile_id = %s`, s.ph(1)),
		fmt.Sprintf(`UPDATE jobs SET profile_id = NULL WHERE profile_id = %s`, s.ph(1)),
		fmt.Sprintf(`UPDATE runs SET profile_id = NULL WHERE profile_id = %s`, s.ph(1)),
	} {
		if _, err := tx.Exec(query, id); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("delete profile clear references: %w", err)
		}
	}
	result, err := tx.Exec(
		fmt.Sprintf(`DELETE FROM profiles WHERE id = %s`, s.ph(1)), id)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("delete profile: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		_ = tx.Rollback()
		return fmt.Errorf("profile %d not found", id)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("delete profile commit: %w", err)
	}
	return nil
}

// ListProfiles returns all profiles ordered by name.
func (s *SQLJobStore) ListProfiles() []StoredProfile {
	rows, err := s.db.Query(
		`SELECT id, name, description, config, public, schema_version, created_at, updated_at FROM profiles ORDER BY name`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []StoredProfile
	for rows.Next() {
		var (
			p                    StoredProfile
			publicInt            int
			createdAt, updatedAt string
		)
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Config, &publicInt, &p.SchemaVersion, &createdAt, &updatedAt); err != nil {
			continue
		}
		p.Public = publicInt != 0
		p.CreatedAt = parseTimestampStr(createdAt)
		p.UpdatedAt = parseTimestampStr(updatedAt)
		result = append(result, p)
	}
	return result
}

// ── Settings ─────────────────────────────────────────────────────────────────

// qkey returns the quoted identifier for the "key" column in the settings
// table. "key" is a reserved word in MariaDB and must be quoted.
func (s *SQLJobStore) qkey() string {
	if _, ok := s.dialect.(mariadbDialect); ok {
		return "`key`"
	}
	return `"key"`
}

// GetSetting returns a setting value by key.
func (s *SQLJobStore) GetSetting(key string) (string, bool) {
	var value string
	err := s.db.QueryRow(
		`SELECT value FROM settings WHERE `+s.qkey()+` = `+s.ph(1), key,
	).Scan(&value)
	if err != nil {
		return "", false
	}
	return value, true
}

// SetSetting creates or updates a setting.
func (s *SQLJobStore) SetSetting(key, value string) error {
	qk := s.qkey()
	switch s.dialect.(type) {
	case postgresDialect:
		_, err := s.db.Exec(
			`INSERT INTO settings(`+qk+`, value) VALUES (`+s.ph(1)+`, `+s.ph(2)+`)
			 ON CONFLICT(`+qk+`) DO UPDATE SET value = `+s.ph(3),
			key, value, value)
		return err
	case mariadbDialect:
		_, err := s.db.Exec(
			`INSERT INTO settings(`+qk+`, value) VALUES (?, ?) ON DUPLICATE KEY UPDATE value = ?`,
			key, value, value)
		return err
	default: // sqlite
		_, err := s.db.Exec(
			`INSERT INTO settings(`+qk+`, value) VALUES (?, ?)
			 ON CONFLICT(`+qk+`) DO UPDATE SET value = ?`,
			key, value, value)
		return err
	}
}

// DeleteSetting removes a setting by key.
func (s *SQLJobStore) DeleteSetting(key string) error {
	res, err := s.db.Exec(
		`DELETE FROM settings WHERE `+s.qkey()+` = `+s.ph(1), key)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("setting not found")
	}
	return nil
}

// ListSettings returns all settings as a map.
func (s *SQLJobStore) ListSettings() map[string]string {
	rows, err := s.db.Query(`SELECT ` + s.qkey() + `, value FROM settings ORDER BY ` + s.qkey())
	if err != nil {
		return nil
	}
	defer rows.Close()
	result := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			continue
		}
		result[k] = v
	}
	return result
}

// ── Purge ─────────────────────────────────────────────────────────────────────

// purgeBatchSize limits runs deleted per transaction to avoid long lock holds on MariaDB/MySQL.
var purgeBatchSize = 500

// PurgeOlderThan deletes terminal runs whose finished_at is before cutoff,
// along with their entries. Returns the number of runs deleted.
func (s *SQLJobStore) PurgeOlderThan(cutoff time.Time) (int64, error) {
	statuses := "'succeeded','failed','canceled','expired'"
	cutoffVal := s.dialect.TimestampVal(cutoff)

	var total int64
	for {
		rows, err := s.db.Query(
			fmt.Sprintf(
				`SELECT id FROM runs WHERE finished_at < %s AND status IN (%s) LIMIT %d`,
				s.ph(1), statuses, purgeBatchSize,
			),
			cutoffVal,
		)
		if err != nil {
			return total, fmt.Errorf("purge select: %w", err)
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return total, fmt.Errorf("purge scan: %w", err)
			}
			ids = append(ids, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return total, fmt.Errorf("purge rows: %w", err)
		}
		if len(ids) == 0 {
			break
		}

		args := make([]any, len(ids))
		for i, id := range ids {
			args[i] = id
		}
		inPH := s.phRange(1, len(ids))

		tx, err := s.db.Begin()
		if err != nil {
			return total, fmt.Errorf("purge begin tx: %w", err)
		}
		if _, err := tx.Exec(`DELETE FROM entries WHERE run_id IN (`+inPH+`)`, args...); err != nil {
			_ = tx.Rollback()
			return total, fmt.Errorf("purge entries: %w", err)
		}
		if _, err := tx.Exec(`DELETE FROM run_dnssec_chain WHERE run_id IN (`+inPH+`)`, args...); err != nil {
			_ = tx.Rollback()
			return total, fmt.Errorf("purge dnssec chains: %w", err)
		}
		res, err := tx.Exec(`DELETE FROM runs WHERE id IN (`+inPH+`)`, args...)
		if err != nil {
			_ = tx.Rollback()
			return total, fmt.Errorf("purge runs: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return total, fmt.Errorf("purge commit: %w", err)
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, nil
}

// PurgeByTag deletes terminal runs (and entries) for domains in tag.
func (s *SQLJobStore) PurgeByTag(tag string) (int64, error) {
	statuses := "'succeeded','failed','canceled','expired'"

	var total int64
	for {
		rows, err := s.db.Query(
			fmt.Sprintf(
				`SELECT r.id FROM runs r
				 JOIN domain_tags dt ON dt.domain_id = r.domain_id
				 WHERE dt.tag = %s AND r.status IN (%s) LIMIT %d`,
				s.ph(1), statuses, purgeBatchSize,
			),
			tag,
		)
		if err != nil {
			return total, fmt.Errorf("purge by tag select: %w", err)
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return total, fmt.Errorf("purge by tag scan: %w", err)
			}
			ids = append(ids, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return total, fmt.Errorf("purge by tag rows: %w", err)
		}
		if len(ids) == 0 {
			break
		}

		args := make([]any, len(ids))
		for i, id := range ids {
			args[i] = id
		}
		inPH := s.phRange(1, len(ids))

		tx, err := s.db.Begin()
		if err != nil {
			return total, fmt.Errorf("purge by tag begin tx: %w", err)
		}
		if _, err := tx.Exec(`DELETE FROM entries WHERE run_id IN (`+inPH+`)`, args...); err != nil {
			_ = tx.Rollback()
			return total, fmt.Errorf("purge by tag entries: %w", err)
		}
		if _, err := tx.Exec(`DELETE FROM run_dnssec_chain WHERE run_id IN (`+inPH+`)`, args...); err != nil {
			_ = tx.Rollback()
			return total, fmt.Errorf("purge by tag dnssec chains: %w", err)
		}
		res, err := tx.Exec(`DELETE FROM runs WHERE id IN (`+inPH+`)`, args...)
		if err != nil {
			_ = tx.Rollback()
			return total, fmt.Errorf("purge by tag runs: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return total, fmt.Errorf("purge by tag commit: %w", err)
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, nil
}
