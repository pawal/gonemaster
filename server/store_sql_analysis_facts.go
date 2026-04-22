package server

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	analysisNameserverCols = `id, name, first_seen_at, last_seen_at`
	analysisAddressCols    = `id, address, family, first_seen_at, last_seen_at`
	analysisPrefixCols     = `id, prefix, family, first_seen_at, last_seen_at`
	analysisASNCols        = `asn, label, first_seen_at, last_seen_at`
	analysisRunNSEndCols   = `cohort_id, run_id, domain_id, nameserver_id, address_id,
		role, source, family, avg_ms, min_ms, max_ms, query_count`
	analysisRunAddrASNCols   = `cohort_id, run_id, domain_id, address_id, prefix_id, asn, lookup_status, source`
	analysisRunDomainASNCols = `cohort_id, run_id, domain_id, asn, family, source`
	analysisRunTagCols       = `cohort_id, run_id, domain_id, tag, module, testcase, level, occurrence_count`
	analysisRunDomainFactCols = `cohort_id, run_id, domain_id, category, fact_key, value_num`
	analysisRunSummaryCols = `cohort_id, run_id, domain_id, score, grade, nameserver_count,
		endpoint_count, asn_count, prefix_count, worst_level`
	analysisProjStateCols = `cohort_id, run_id, projector_version, status, projected_at, error`
)

func normalizeSeenAt(seenAt time.Time) time.Time {
	if seenAt.IsZero() {
		return time.Now().UTC()
	}
	return seenAt.UTC()
}

func minTime(a, b time.Time) time.Time {
	if a.IsZero() || b.Before(a) {
		return b
	}
	return a
}

func maxTime(a, b time.Time) time.Time {
	if a.IsZero() || b.After(a) {
		return b
	}
	return a
}

func nullStringPtr(v *string) sql.NullString {
	if v == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *v, Valid: true}
}

func nullInt64ScanPtr(ns sql.NullInt64) *int64 {
	if !ns.Valid {
		return nil
	}
	v := ns.Int64
	return &v
}

func nullStringScanPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	v := ns.String
	return &v
}

func scanAnalysisNameserverRow(row rowScanner) (AnalysisNameserver, error) {
	var (
		item                AnalysisNameserver
		firstSeen, lastSeen string
	)
	if err := row.Scan(&item.ID, &item.Name, &firstSeen, &lastSeen); err != nil {
		return AnalysisNameserver{}, err
	}
	item.FirstSeenAt = parseTimestampStr(firstSeen)
	item.LastSeenAt = parseTimestampStr(lastSeen)
	return item, nil
}

func scanAnalysisAddressRow(row rowScanner) (AnalysisAddress, error) {
	var (
		item                AnalysisAddress
		firstSeen, lastSeen string
	)
	if err := row.Scan(&item.ID, &item.Address, &item.Family, &firstSeen, &lastSeen); err != nil {
		return AnalysisAddress{}, err
	}
	item.FirstSeenAt = parseTimestampStr(firstSeen)
	item.LastSeenAt = parseTimestampStr(lastSeen)
	return item, nil
}

func scanAnalysisPrefixRow(row rowScanner) (AnalysisPrefix, error) {
	var (
		item                AnalysisPrefix
		firstSeen, lastSeen string
	)
	if err := row.Scan(&item.ID, &item.Prefix, &item.Family, &firstSeen, &lastSeen); err != nil {
		return AnalysisPrefix{}, err
	}
	item.FirstSeenAt = parseTimestampStr(firstSeen)
	item.LastSeenAt = parseTimestampStr(lastSeen)
	return item, nil
}

func scanAnalysisASNRow(row rowScanner) (AnalysisASN, error) {
	var (
		item                AnalysisASN
		firstSeen, lastSeen string
	)
	if err := row.Scan(&item.ASN, &item.Label, &firstSeen, &lastSeen); err != nil {
		return AnalysisASN{}, err
	}
	item.FirstSeenAt = parseTimestampStr(firstSeen)
	item.LastSeenAt = parseTimestampStr(lastSeen)
	return item, nil
}

func (s *SQLJobStore) upsertSeenEntityIn(
	q sqlQuerier,
	selectQuery string,
	selectArgs []any,
	insertQuery string,
	insertArgs []any,
	updateQuery string,
	updateArgsFn func(id int64, existingFirst, existingLast time.Time) []any,
	scan func(row rowScanner) (int64, time.Time, time.Time, error),
) (int64, error) {
	row := q.QueryRow(selectQuery, selectArgs...)
	id, firstSeen, lastSeen, err := scan(row)
	if err == nil {
		args := updateArgsFn(id, firstSeen, lastSeen)
		if _, err := q.Exec(updateQuery, args...); err != nil {
			return 0, err
		}
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if _, err := q.Exec(insertQuery, insertArgs...); err != nil {
		return 0, err
	}
	row = q.QueryRow(selectQuery, selectArgs...)
	id, _, _, err = scan(row)
	if err != nil {
		return 0, err
	}
	return id, nil
}

// UpsertAnalysisNameserver inserts or updates one normalized nameserver row.
func (s *SQLJobStore) UpsertAnalysisNameserver(name string, seenAt time.Time) (AnalysisNameserver, error) {
	return s.upsertAnalysisNameserverIn(s.db, name, seenAt)
}

func (s *SQLJobStore) upsertAnalysisNameserverIn(q sqlQuerier, name string, seenAt time.Time) (AnalysisNameserver, error) {
	if name == "" {
		return AnalysisNameserver{}, errors.New("nameserver name is required")
	}
	seenAt = normalizeSeenAt(seenAt)
	if s.dialect.SupportsOnConflictReturning() {
		query := fmt.Sprintf(
			`INSERT INTO analysis_nameservers (name, first_seen_at, last_seen_at) VALUES (%s, %s, %s)
			 ON CONFLICT (name) DO UPDATE SET
			   first_seen_at = %s,
			   last_seen_at = %s
			 RETURNING id, first_seen_at, last_seen_at`,
			s.ph(1), s.ph(2), s.ph(3),
			s.dialect.Least("analysis_nameservers.first_seen_at", "EXCLUDED.first_seen_at"),
			s.dialect.Greatest("analysis_nameservers.last_seen_at", "EXCLUDED.last_seen_at"),
		)
		item := AnalysisNameserver{Name: name}
		var firstSeen, lastSeen string
		if err := q.QueryRow(query, name, s.ts(seenAt), s.ts(seenAt)).Scan(&item.ID, &firstSeen, &lastSeen); err != nil {
			return AnalysisNameserver{}, fmt.Errorf("upsert analysis nameserver: %w", err)
		}
		item.FirstSeenAt = parseTimestampStr(firstSeen)
		item.LastSeenAt = parseTimestampStr(lastSeen)
		return item, nil
	}
	id, err := s.upsertSeenEntityIn(
		q,
		fmt.Sprintf(`SELECT id, first_seen_at, last_seen_at FROM analysis_nameservers WHERE name = %s`, s.ph(1)),
		[]any{name},
		fmt.Sprintf(`INSERT INTO analysis_nameservers (name, first_seen_at, last_seen_at) VALUES (%s)`, s.phRange(1, 3)),
		[]any{name, s.ts(seenAt), s.ts(seenAt)},
		fmt.Sprintf(`UPDATE analysis_nameservers SET first_seen_at = %s, last_seen_at = %s WHERE id = %s`,
			s.ph(1), s.ph(2), s.ph(3)),
		func(id int64, existingFirst, existingLast time.Time) []any {
			return []any{s.ts(minTime(existingFirst, seenAt)), s.ts(maxTime(existingLast, seenAt)), id}
		},
		func(row rowScanner) (int64, time.Time, time.Time, error) {
			var (
				id                  int64
				firstSeen, lastSeen string
			)
			err := row.Scan(&id, &firstSeen, &lastSeen)
			return id, parseTimestampStr(firstSeen), parseTimestampStr(lastSeen), err
		},
	)
	if err != nil {
		return AnalysisNameserver{}, fmt.Errorf("upsert analysis nameserver: %w", err)
	}
	item, ok := s.getAnalysisNameserverIn(q, id)
	if !ok {
		return AnalysisNameserver{}, errors.New("analysis nameserver written but not readable")
	}
	return item, nil
}

func (s *SQLJobStore) getAnalysisNameserverIn(q sqlQuerier, id int64) (AnalysisNameserver, bool) {
	row := q.QueryRow(
		fmt.Sprintf(`SELECT %s FROM analysis_nameservers WHERE id = %s`, analysisNameserverCols, s.ph(1)),
		id,
	)
	item, err := scanAnalysisNameserverRow(row)
	if err == nil {
		return item, true
	}
	return AnalysisNameserver{}, false
}

// GetAnalysisNameserver returns a normalized nameserver row by id.
func (s *SQLJobStore) GetAnalysisNameserver(id int64) (AnalysisNameserver, bool) {
	return s.getAnalysisNameserverIn(s.db, id)
}

// GetAnalysisNameserverByName returns a normalized nameserver row by natural key.
func (s *SQLJobStore) GetAnalysisNameserverByName(name string) (AnalysisNameserver, bool) {
	row := s.db.QueryRow(
		fmt.Sprintf(`SELECT %s FROM analysis_nameservers WHERE name = %s`, analysisNameserverCols, s.ph(1)),
		name,
	)
	item, err := scanAnalysisNameserverRow(row)
	if err == nil {
		return item, true
	}
	return AnalysisNameserver{}, false
}

// UpsertAnalysisAddress inserts or updates one normalized address row.
func (s *SQLJobStore) UpsertAnalysisAddress(address, family string, seenAt time.Time) (AnalysisAddress, error) {
	return s.upsertAnalysisAddressIn(s.db, address, family, seenAt)
}

func (s *SQLJobStore) upsertAnalysisAddressIn(q sqlQuerier, address, family string, seenAt time.Time) (AnalysisAddress, error) {
	if address == "" {
		return AnalysisAddress{}, errors.New("address is required")
	}
	if family == "" {
		return AnalysisAddress{}, errors.New("address family is required")
	}
	seenAt = normalizeSeenAt(seenAt)
	if s.dialect.SupportsOnConflictReturning() {
		query := fmt.Sprintf(
			`INSERT INTO analysis_addresses (address, family, first_seen_at, last_seen_at) VALUES (%s, %s, %s, %s)
			 ON CONFLICT (address) DO UPDATE SET
			   family = EXCLUDED.family,
			   first_seen_at = %s,
			   last_seen_at = %s
			 RETURNING id, family, first_seen_at, last_seen_at`,
			s.ph(1), s.ph(2), s.ph(3), s.ph(4),
			s.dialect.Least("analysis_addresses.first_seen_at", "EXCLUDED.first_seen_at"),
			s.dialect.Greatest("analysis_addresses.last_seen_at", "EXCLUDED.last_seen_at"),
		)
		item := AnalysisAddress{Address: address}
		var firstSeen, lastSeen string
		if err := q.QueryRow(query, address, family, s.ts(seenAt), s.ts(seenAt)).Scan(&item.ID, &item.Family, &firstSeen, &lastSeen); err != nil {
			return AnalysisAddress{}, fmt.Errorf("upsert analysis address: %w", err)
		}
		item.FirstSeenAt = parseTimestampStr(firstSeen)
		item.LastSeenAt = parseTimestampStr(lastSeen)
		return item, nil
	}
	id, err := s.upsertSeenEntityIn(
		q,
		fmt.Sprintf(`SELECT id, first_seen_at, last_seen_at FROM analysis_addresses WHERE address = %s`, s.ph(1)),
		[]any{address},
		fmt.Sprintf(`INSERT INTO analysis_addresses (address, family, first_seen_at, last_seen_at) VALUES (%s)`, s.phRange(1, 4)),
		[]any{address, family, s.ts(seenAt), s.ts(seenAt)},
		fmt.Sprintf(`UPDATE analysis_addresses SET family = %s, first_seen_at = %s, last_seen_at = %s WHERE id = %s`,
			s.ph(1), s.ph(2), s.ph(3), s.ph(4)),
		func(id int64, existingFirst, existingLast time.Time) []any {
			return []any{family, s.ts(minTime(existingFirst, seenAt)), s.ts(maxTime(existingLast, seenAt)), id}
		},
		func(row rowScanner) (int64, time.Time, time.Time, error) {
			var (
				id                  int64
				firstSeen, lastSeen string
			)
			err := row.Scan(&id, &firstSeen, &lastSeen)
			return id, parseTimestampStr(firstSeen), parseTimestampStr(lastSeen), err
		},
	)
	if err != nil {
		return AnalysisAddress{}, fmt.Errorf("upsert analysis address: %w", err)
	}
	item, ok := s.getAnalysisAddressIn(q, id)
	if !ok {
		return AnalysisAddress{}, errors.New("analysis address written but not readable")
	}
	return item, nil
}

// GetAnalysisAddress returns a normalized address row by id.
func (s *SQLJobStore) GetAnalysisAddress(id int64) (AnalysisAddress, bool) {
	return s.getAnalysisAddressIn(s.db, id)
}

func (s *SQLJobStore) getAnalysisAddressIn(q sqlQuerier, id int64) (AnalysisAddress, bool) {
	row := q.QueryRow(
		fmt.Sprintf(`SELECT %s FROM analysis_addresses WHERE id = %s`, analysisAddressCols, s.ph(1)),
		id,
	)
	item, err := scanAnalysisAddressRow(row)
	if err == nil {
		return item, true
	}
	return AnalysisAddress{}, false
}

// GetAnalysisAddressByAddress returns a normalized address row by natural key.
func (s *SQLJobStore) GetAnalysisAddressByAddress(address string) (AnalysisAddress, bool) {
	row := s.db.QueryRow(
		fmt.Sprintf(`SELECT %s FROM analysis_addresses WHERE address = %s`, analysisAddressCols, s.ph(1)),
		address,
	)
	item, err := scanAnalysisAddressRow(row)
	if err == nil {
		return item, true
	}
	return AnalysisAddress{}, false
}

// UpsertAnalysisPrefix inserts or updates one normalized prefix row.
func (s *SQLJobStore) UpsertAnalysisPrefix(prefix, family string, seenAt time.Time) (AnalysisPrefix, error) {
	return s.upsertAnalysisPrefixIn(s.db, prefix, family, seenAt)
}

func (s *SQLJobStore) upsertAnalysisPrefixIn(q sqlQuerier, prefix, family string, seenAt time.Time) (AnalysisPrefix, error) {
	if prefix == "" {
		return AnalysisPrefix{}, errors.New("prefix is required")
	}
	if family == "" {
		return AnalysisPrefix{}, errors.New("prefix family is required")
	}
	seenAt = normalizeSeenAt(seenAt)
	if s.dialect.SupportsOnConflictReturning() {
		query := fmt.Sprintf(
			`INSERT INTO analysis_prefixes (prefix, family, first_seen_at, last_seen_at) VALUES (%s, %s, %s, %s)
			 ON CONFLICT (prefix) DO UPDATE SET
			   family = EXCLUDED.family,
			   first_seen_at = %s,
			   last_seen_at = %s
			 RETURNING id, family, first_seen_at, last_seen_at`,
			s.ph(1), s.ph(2), s.ph(3), s.ph(4),
			s.dialect.Least("analysis_prefixes.first_seen_at", "EXCLUDED.first_seen_at"),
			s.dialect.Greatest("analysis_prefixes.last_seen_at", "EXCLUDED.last_seen_at"),
		)
		item := AnalysisPrefix{Prefix: prefix}
		var firstSeen, lastSeen string
		if err := q.QueryRow(query, prefix, family, s.ts(seenAt), s.ts(seenAt)).Scan(&item.ID, &item.Family, &firstSeen, &lastSeen); err != nil {
			return AnalysisPrefix{}, fmt.Errorf("upsert analysis prefix: %w", err)
		}
		item.FirstSeenAt = parseTimestampStr(firstSeen)
		item.LastSeenAt = parseTimestampStr(lastSeen)
		return item, nil
	}
	id, err := s.upsertSeenEntityIn(
		q,
		fmt.Sprintf(`SELECT id, first_seen_at, last_seen_at FROM analysis_prefixes WHERE prefix = %s`, s.ph(1)),
		[]any{prefix},
		fmt.Sprintf(`INSERT INTO analysis_prefixes (prefix, family, first_seen_at, last_seen_at) VALUES (%s)`, s.phRange(1, 4)),
		[]any{prefix, family, s.ts(seenAt), s.ts(seenAt)},
		fmt.Sprintf(`UPDATE analysis_prefixes SET family = %s, first_seen_at = %s, last_seen_at = %s WHERE id = %s`,
			s.ph(1), s.ph(2), s.ph(3), s.ph(4)),
		func(id int64, existingFirst, existingLast time.Time) []any {
			return []any{family, s.ts(minTime(existingFirst, seenAt)), s.ts(maxTime(existingLast, seenAt)), id}
		},
		func(row rowScanner) (int64, time.Time, time.Time, error) {
			var (
				id                  int64
				firstSeen, lastSeen string
			)
			err := row.Scan(&id, &firstSeen, &lastSeen)
			return id, parseTimestampStr(firstSeen), parseTimestampStr(lastSeen), err
		},
	)
	if err != nil {
		return AnalysisPrefix{}, fmt.Errorf("upsert analysis prefix: %w", err)
	}
	item, ok := s.getAnalysisPrefixIn(q, id)
	if !ok {
		return AnalysisPrefix{}, errors.New("analysis prefix written but not readable")
	}
	return item, nil
}

// GetAnalysisPrefix returns a normalized prefix row by id.
func (s *SQLJobStore) GetAnalysisPrefix(id int64) (AnalysisPrefix, bool) {
	return s.getAnalysisPrefixIn(s.db, id)
}

func (s *SQLJobStore) getAnalysisPrefixIn(q sqlQuerier, id int64) (AnalysisPrefix, bool) {
	row := q.QueryRow(
		fmt.Sprintf(`SELECT %s FROM analysis_prefixes WHERE id = %s`, analysisPrefixCols, s.ph(1)),
		id,
	)
	item, err := scanAnalysisPrefixRow(row)
	if err == nil {
		return item, true
	}
	return AnalysisPrefix{}, false
}

// GetAnalysisPrefixByPrefix returns a normalized prefix row by natural key.
func (s *SQLJobStore) GetAnalysisPrefixByPrefix(prefix string) (AnalysisPrefix, bool) {
	row := s.db.QueryRow(
		fmt.Sprintf(`SELECT %s FROM analysis_prefixes WHERE prefix = %s`, analysisPrefixCols, s.ph(1)),
		prefix,
	)
	item, err := scanAnalysisPrefixRow(row)
	if err == nil {
		return item, true
	}
	return AnalysisPrefix{}, false
}

// UpsertAnalysisASN inserts or updates one normalized ASN row.
func (s *SQLJobStore) UpsertAnalysisASN(asn int64, label string, seenAt time.Time) (AnalysisASN, error) {
	return s.upsertAnalysisASNIn(s.db, asn, label, seenAt)
}

func (s *SQLJobStore) upsertAnalysisASNIn(q sqlQuerier, asn int64, label string, seenAt time.Time) (AnalysisASN, error) {
	if asn == 0 {
		return AnalysisASN{}, errors.New("asn is required")
	}
	seenAt = normalizeSeenAt(seenAt)
	if s.dialect.SupportsOnConflictReturning() {
		// Keep an already-resolved label rather than clobbering it with an
		// empty one from a caller that hadn't resolved a label yet.
		query := fmt.Sprintf(
			`INSERT INTO analysis_asns (asn, label, first_seen_at, last_seen_at) VALUES (%s, %s, %s, %s)
			 ON CONFLICT (asn) DO UPDATE SET
			   label = CASE WHEN EXCLUDED.label = '' THEN analysis_asns.label ELSE EXCLUDED.label END,
			   first_seen_at = %s,
			   last_seen_at = %s
			 RETURNING asn, label, first_seen_at, last_seen_at`,
			s.ph(1), s.ph(2), s.ph(3), s.ph(4),
			s.dialect.Least("analysis_asns.first_seen_at", "EXCLUDED.first_seen_at"),
			s.dialect.Greatest("analysis_asns.last_seen_at", "EXCLUDED.last_seen_at"),
		)
		var item AnalysisASN
		var firstSeen, lastSeen string
		if err := q.QueryRow(query, asn, label, s.ts(seenAt), s.ts(seenAt)).Scan(&item.ASN, &item.Label, &firstSeen, &lastSeen); err != nil {
			return AnalysisASN{}, fmt.Errorf("upsert analysis asn: %w", err)
		}
		item.FirstSeenAt = parseTimestampStr(firstSeen)
		item.LastSeenAt = parseTimestampStr(lastSeen)
		return item, nil
	}
	row := q.QueryRow(
		fmt.Sprintf(`SELECT label, first_seen_at, last_seen_at FROM analysis_asns WHERE asn = %s`, s.ph(1)),
		asn,
	)
	var existingLabel, firstSeen, lastSeen string
	err := row.Scan(&existingLabel, &firstSeen, &lastSeen)
	switch {
	case err == nil:
		// An empty label from the caller shouldn't blank out a previously
		// resolved one — that would happen every time an enricher fails
		// to reach the registry. Keep the existing label instead.
		effectiveLabel := label
		if effectiveLabel == "" {
			effectiveLabel = existingLabel
		}
		_, err = q.Exec(
			fmt.Sprintf(`UPDATE analysis_asns SET label = %s, first_seen_at = %s, last_seen_at = %s WHERE asn = %s`,
				s.ph(1), s.ph(2), s.ph(3), s.ph(4)),
			effectiveLabel, s.ts(minTime(parseTimestampStr(firstSeen), seenAt)), s.ts(maxTime(parseTimestampStr(lastSeen), seenAt)), asn,
		)
	case errors.Is(err, sql.ErrNoRows):
		_, err = q.Exec(
			fmt.Sprintf(`INSERT INTO analysis_asns (asn, label, first_seen_at, last_seen_at) VALUES (%s)`,
				s.phRange(1, 4)),
			asn, label, s.ts(seenAt), s.ts(seenAt),
		)
	}
	if err != nil {
		return AnalysisASN{}, fmt.Errorf("upsert analysis asn: %w", err)
	}
	item, ok := s.getAnalysisASNIn(q, asn)
	if !ok {
		return AnalysisASN{}, errors.New("analysis asn written but not readable")
	}
	return item, nil
}

// GetAnalysisASN returns a normalized ASN row by primary key.
func (s *SQLJobStore) GetAnalysisASN(asn int64) (AnalysisASN, bool) {
	return s.getAnalysisASNIn(s.db, asn)
}

func (s *SQLJobStore) getAnalysisASNIn(q sqlQuerier, asn int64) (AnalysisASN, bool) {
	row := q.QueryRow(
		fmt.Sprintf(`SELECT %s FROM analysis_asns WHERE asn = %s`, analysisASNCols, s.ph(1)),
		asn,
	)
	item, err := scanAnalysisASNRow(row)
	if err == nil {
		return item, true
	}
	return AnalysisASN{}, false
}

// replaceAnalysisRowsIn runs DELETE followed by a single multi-row INSERT
// against q. The caller passes the INSERT prefix (`INSERT INTO t (cols)
// VALUES `) and one []any per row; this helper stitches together the
// `($1,$2,...),($N+1,...)` placeholder groups so an N-row replace takes
// two round-trips instead of N+1. The caller is responsible for
// transactional framing: the public autocommit path wraps one tx around
// it; the per-run projection path reuses its outer ProjectLoaded tx so a
// mid-batch failure rolls back with everything else.
func (s *SQLJobStore) replaceAnalysisRowsIn(
	q sqlQuerier,
	deleteQuery string,
	deleteArgs []any,
	insertPrefix string,
	rows [][]any,
) error {
	if _, err := q.Exec(deleteQuery, deleteArgs...); err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	cols := len(rows[0])
	groups := make([]string, len(rows))
	args := make([]any, 0, len(rows)*cols)
	for i, row := range rows {
		placeholders := make([]string, cols)
		for j := 0; j < cols; j++ {
			placeholders[j] = s.ph(i*cols + j + 1)
		}
		groups[i] = "(" + strings.Join(placeholders, ", ") + ")"
		args = append(args, row...)
	}
	query := insertPrefix + " VALUES " + strings.Join(groups, ", ")
	if _, err := q.Exec(query, args...); err != nil {
		return err
	}
	return nil
}

// ReplaceAnalysisRunNSEndpoints replaces all run+cohort nameserver endpoint rows.
func (s *SQLJobStore) ReplaceAnalysisRunNSEndpoints(cohortID int64, runID string, items []AnalysisRunNameserverEndpoint) error {
	return s.inOwnTx(func(tx *sql.Tx) error {
		return s.replaceAnalysisRunNSEndpointsIn(tx, cohortID, runID, items)
	})
}

func (s *SQLJobStore) replaceAnalysisRunNSEndpointsIn(q sqlQuerier, cohortID int64, runID string, items []AnalysisRunNameserverEndpoint) error {
	if cohortID == 0 {
		return errors.New("cohort_id is required")
	}
	if runID == "" {
		return errors.New("run_id is required")
	}
	rows := make([][]any, 0, len(items))
	for _, item := range items {
		rows = append(rows, []any{
			cohortID, runID, item.DomainID, item.NameserverID, item.AddressID,
			item.Role, item.Source, item.Family, item.AvgMS, item.MinMS, item.MaxMS, item.QueryCount,
		})
	}
	err := s.replaceAnalysisRowsIn(q,
		fmt.Sprintf(`DELETE FROM analysis_run_ns_endpoints WHERE cohort_id = %s AND run_id = %s`, s.ph(1), s.ph(2)),
		[]any{cohortID, runID},
		`INSERT INTO analysis_run_ns_endpoints (
			cohort_id, run_id, domain_id, nameserver_id, address_id, role, source, family,
			avg_ms, min_ms, max_ms, query_count
		)`,
		rows,
	)
	if err != nil {
		return fmt.Errorf("replace analysis run ns endpoints: %w", err)
	}
	return nil
}

// inOwnTx wraps fn in a fresh transaction for an autocommit-safe public
// entry point. Per-run projection paths bypass this because they share one
// ProjectLoaded-wide transaction — see WithAnalysisWriteTx.
func (s *SQLJobStore) inOwnTx(fn func(*sql.Tx) error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// ListAnalysisRunNSEndpoints returns run+cohort nameserver endpoint rows in stable order.
func (s *SQLJobStore) ListAnalysisRunNSEndpoints(cohortID int64, runID string) []AnalysisRunNameserverEndpoint {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT %s FROM analysis_run_ns_endpoints
			WHERE cohort_id = %s AND run_id = %s
			ORDER BY domain_id, nameserver_id, address_id, role, source`,
			analysisRunNSEndCols, s.ph(1), s.ph(2)),
		cohortID, runID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []AnalysisRunNameserverEndpoint
	for rows.Next() {
		var item AnalysisRunNameserverEndpoint
		if err := rows.Scan(
			&item.CohortID, &item.RunID, &item.DomainID, &item.NameserverID, &item.AddressID,
			&item.Role, &item.Source, &item.Family, &item.AvgMS, &item.MinMS, &item.MaxMS, &item.QueryCount,
		); err != nil {
			return nil
		}
		out = append(out, item)
	}
	return out
}

// ListAnalysisRunNSEndpointsByCohort returns every nameserver endpoint row
// materialized for the cohort, across all runs.
func (s *SQLJobStore) ListAnalysisRunNSEndpointsByCohort(cohortID int64) []AnalysisRunNameserverEndpoint {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT %s FROM analysis_run_ns_endpoints
			WHERE cohort_id = %s
			ORDER BY domain_id, nameserver_id, address_id, role, source`,
			analysisRunNSEndCols, s.ph(1)),
		cohortID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []AnalysisRunNameserverEndpoint
	for rows.Next() {
		var item AnalysisRunNameserverEndpoint
		if err := rows.Scan(
			&item.CohortID, &item.RunID, &item.DomainID, &item.NameserverID, &item.AddressID,
			&item.Role, &item.Source, &item.Family, &item.AvgMS, &item.MinMS, &item.MaxMS, &item.QueryCount,
		); err != nil {
			return nil
		}
		out = append(out, item)
	}
	return out
}

// ReplaceAnalysisRunAddressASNs replaces all run+cohort address-to-ASN rows.
func (s *SQLJobStore) ReplaceAnalysisRunAddressASNs(cohortID int64, runID string, items []AnalysisRunAddressASN) error {
	return s.inOwnTx(func(tx *sql.Tx) error {
		return s.replaceAnalysisRunAddressASNsIn(tx, cohortID, runID, items)
	})
}

func (s *SQLJobStore) replaceAnalysisRunAddressASNsIn(q sqlQuerier, cohortID int64, runID string, items []AnalysisRunAddressASN) error {
	if cohortID == 0 {
		return errors.New("cohort_id is required")
	}
	if runID == "" {
		return errors.New("run_id is required")
	}
	rows := make([][]any, 0, len(items))
	for _, item := range items {
		rows = append(rows, []any{
			cohortID, runID, item.DomainID, item.AddressID, nullInt64Value(item.PrefixID),
			nullInt64Value(item.ASN), item.LookupStatus, item.Source,
		})
	}
	err := s.replaceAnalysisRowsIn(q,
		fmt.Sprintf(`DELETE FROM analysis_run_address_asns WHERE cohort_id = %s AND run_id = %s`, s.ph(1), s.ph(2)),
		[]any{cohortID, runID},
		`INSERT INTO analysis_run_address_asns (
			cohort_id, run_id, domain_id, address_id, prefix_id, asn, lookup_status, source
		)`,
		rows,
	)
	if err != nil {
		return fmt.Errorf("replace analysis run address asns: %w", err)
	}
	return nil
}

// ListAnalysisRunAddressASNs returns run+cohort address-to-ASN rows in stable order.
func (s *SQLJobStore) ListAnalysisRunAddressASNs(cohortID int64, runID string) []AnalysisRunAddressASN {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT %s FROM analysis_run_address_asns
			WHERE cohort_id = %s AND run_id = %s
			ORDER BY domain_id, address_id`,
			analysisRunAddrASNCols, s.ph(1), s.ph(2)),
		cohortID, runID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []AnalysisRunAddressASN
	for rows.Next() {
		var (
			item     AnalysisRunAddressASN
			prefixID sql.NullInt64
			asn      sql.NullInt64
		)
		if err := rows.Scan(
			&item.CohortID, &item.RunID, &item.DomainID, &item.AddressID,
			&prefixID, &asn, &item.LookupStatus, &item.Source,
		); err != nil {
			return nil
		}
		item.PrefixID = nullInt64ScanPtr(prefixID)
		item.ASN = nullInt64ScanPtr(asn)
		out = append(out, item)
	}
	return out
}

// ListAnalysisRunAddressASNsByCohort returns every address-to-ASN row
// materialized for the cohort, across all runs.
func (s *SQLJobStore) ListAnalysisRunAddressASNsByCohort(cohortID int64) []AnalysisRunAddressASN {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT %s FROM analysis_run_address_asns
			WHERE cohort_id = %s
			ORDER BY domain_id, address_id`,
			analysisRunAddrASNCols, s.ph(1)),
		cohortID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []AnalysisRunAddressASN
	for rows.Next() {
		var (
			item     AnalysisRunAddressASN
			prefixID sql.NullInt64
			asn      sql.NullInt64
		)
		if err := rows.Scan(
			&item.CohortID, &item.RunID, &item.DomainID, &item.AddressID,
			&prefixID, &asn, &item.LookupStatus, &item.Source,
		); err != nil {
			return nil
		}
		item.PrefixID = nullInt64ScanPtr(prefixID)
		item.ASN = nullInt64ScanPtr(asn)
		out = append(out, item)
	}
	return out
}

// ReplaceAnalysisRunDomainASNs replaces all run+cohort domain-to-ASN rows.
func (s *SQLJobStore) ReplaceAnalysisRunDomainASNs(cohortID int64, runID string, items []AnalysisRunDomainASN) error {
	return s.inOwnTx(func(tx *sql.Tx) error {
		return s.replaceAnalysisRunDomainASNsIn(tx, cohortID, runID, items)
	})
}

func (s *SQLJobStore) replaceAnalysisRunDomainASNsIn(q sqlQuerier, cohortID int64, runID string, items []AnalysisRunDomainASN) error {
	if cohortID == 0 {
		return errors.New("cohort_id is required")
	}
	if runID == "" {
		return errors.New("run_id is required")
	}
	rows := make([][]any, 0, len(items))
	for _, item := range items {
		rows = append(rows, []any{
			cohortID, runID, item.DomainID, item.ASN, item.Family, item.Source,
		})
	}
	err := s.replaceAnalysisRowsIn(q,
		fmt.Sprintf(`DELETE FROM analysis_run_domain_asns WHERE cohort_id = %s AND run_id = %s`, s.ph(1), s.ph(2)),
		[]any{cohortID, runID},
		`INSERT INTO analysis_run_domain_asns (
			cohort_id, run_id, domain_id, asn, family, source
		)`,
		rows,
	)
	if err != nil {
		return fmt.Errorf("replace analysis run domain asns: %w", err)
	}
	return nil
}

// ListAnalysisRunDomainASNsByCohort returns every domain-to-ASN row
// materialized for the cohort, across all runs.
func (s *SQLJobStore) ListAnalysisRunDomainASNsByCohort(cohortID int64) []AnalysisRunDomainASN {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT %s FROM analysis_run_domain_asns
			WHERE cohort_id = %s
			ORDER BY domain_id, asn, family`,
			analysisRunDomainASNCols, s.ph(1)),
		cohortID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []AnalysisRunDomainASN
	for rows.Next() {
		var item AnalysisRunDomainASN
		if err := rows.Scan(
			&item.CohortID, &item.RunID, &item.DomainID, &item.ASN, &item.Family, &item.Source,
		); err != nil {
			return nil
		}
		out = append(out, item)
	}
	return out
}

// ReplaceAnalysisRunTagSummaries replaces all run+cohort tag aggregate rows.
func (s *SQLJobStore) ReplaceAnalysisRunTagSummaries(cohortID int64, runID string, items []AnalysisRunTagSummary) error {
	return s.inOwnTx(func(tx *sql.Tx) error {
		return s.replaceAnalysisRunTagSummariesIn(tx, cohortID, runID, items)
	})
}

func (s *SQLJobStore) replaceAnalysisRunTagSummariesIn(q sqlQuerier, cohortID int64, runID string, items []AnalysisRunTagSummary) error {
	if cohortID == 0 {
		return errors.New("cohort_id is required")
	}
	if runID == "" {
		return errors.New("run_id is required")
	}
	rows := make([][]any, 0, len(items))
	for _, item := range items {
		rows = append(rows, []any{
			cohortID, runID, item.DomainID, item.Tag,
			item.Module, item.Testcase, item.Level, item.OccurrenceCount,
		})
	}
	err := s.replaceAnalysisRowsIn(q,
		fmt.Sprintf(`DELETE FROM analysis_run_tag_summary WHERE cohort_id = %s AND run_id = %s`, s.ph(1), s.ph(2)),
		[]any{cohortID, runID},
		`INSERT INTO analysis_run_tag_summary (
			cohort_id, run_id, domain_id, tag, module, testcase, level, occurrence_count
		)`,
		rows,
	)
	if err != nil {
		return fmt.Errorf("replace analysis run tag summaries: %w", err)
	}
	return nil
}

// ListAnalysisRunTagSummariesByCohort returns every tag aggregate row
// materialized for the cohort, across all runs.
func (s *SQLJobStore) ListAnalysisRunTagSummariesByCohort(cohortID int64) []AnalysisRunTagSummary {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT %s FROM analysis_run_tag_summary
			WHERE cohort_id = %s
			ORDER BY run_id, tag`,
			analysisRunTagCols, s.ph(1)),
		cohortID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []AnalysisRunTagSummary
	for rows.Next() {
		var item AnalysisRunTagSummary
		if err := rows.Scan(
			&item.CohortID, &item.RunID, &item.DomainID, &item.Tag,
			&item.Module, &item.Testcase, &item.Level, &item.OccurrenceCount,
		); err != nil {
			return nil
		}
		out = append(out, item)
	}
	return out
}

// ReplaceAnalysisRunDomainFacts replaces all run+cohort domain-fact rows.
func (s *SQLJobStore) ReplaceAnalysisRunDomainFacts(cohortID int64, runID string, items []AnalysisRunDomainFact) error {
	return s.inOwnTx(func(tx *sql.Tx) error {
		return s.replaceAnalysisRunDomainFactsIn(tx, cohortID, runID, items)
	})
}

func (s *SQLJobStore) replaceAnalysisRunDomainFactsIn(q sqlQuerier, cohortID int64, runID string, items []AnalysisRunDomainFact) error {
	if cohortID == 0 {
		return errors.New("cohort_id is required")
	}
	if runID == "" {
		return errors.New("run_id is required")
	}
	rows := make([][]any, 0, len(items))
	for _, item := range items {
		var valueNum any
		if item.ValueNum != nil {
			valueNum = *item.ValueNum
		}
		rows = append(rows, []any{
			cohortID, runID, item.DomainID, item.Category, item.Key, valueNum,
		})
	}
	err := s.replaceAnalysisRowsIn(q,
		fmt.Sprintf(`DELETE FROM analysis_run_domain_facts WHERE cohort_id = %s AND run_id = %s`, s.ph(1), s.ph(2)),
		[]any{cohortID, runID},
		`INSERT INTO analysis_run_domain_facts (
			cohort_id, run_id, domain_id, category, fact_key, value_num
		)`,
		rows,
	)
	if err != nil {
		return fmt.Errorf("replace analysis run domain facts: %w", err)
	}
	return nil
}

// ListAnalysisRunDomainFactsByCohort returns every domain-fact row
// materialized for the cohort, across all runs.
func (s *SQLJobStore) ListAnalysisRunDomainFactsByCohort(cohortID int64) []AnalysisRunDomainFact {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT %s FROM analysis_run_domain_facts
			WHERE cohort_id = %s
			ORDER BY run_id, category, fact_key`,
			analysisRunDomainFactCols, s.ph(1)),
		cohortID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []AnalysisRunDomainFact
	for rows.Next() {
		var (
			item     AnalysisRunDomainFact
			valueNum sql.NullInt64
		)
		if err := rows.Scan(
			&item.CohortID, &item.RunID, &item.DomainID,
			&item.Category, &item.Key, &valueNum,
		); err != nil {
			return nil
		}
		item.ValueNum = nullInt64ScanPtr(valueNum)
		out = append(out, item)
	}
	return out
}

// ListAnalysisRunDomainSummariesByCohort returns every run+domain summary row
// materialized for the cohort.
func (s *SQLJobStore) ListAnalysisRunDomainSummariesByCohort(cohortID int64) []AnalysisRunDomainSummary {
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT %s FROM analysis_run_domain_summary
			WHERE cohort_id = %s
			ORDER BY domain_id, run_id`,
			analysisRunSummaryCols, s.ph(1)),
		cohortID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []AnalysisRunDomainSummary
	for rows.Next() {
		var (
			item  AnalysisRunDomainSummary
			score sql.NullInt64
			grade sql.NullString
		)
		if err := rows.Scan(
			&item.CohortID, &item.RunID, &item.DomainID, &score, &grade,
			&item.NameserverCount, &item.EndpointCount, &item.ASNCount, &item.PrefixCount, &item.WorstLevel,
		); err != nil {
			return nil
		}
		if score.Valid {
			v := int(score.Int64)
			item.Score = &v
		}
		item.Grade = nullStringScanPtr(grade)
		out = append(out, item)
	}
	return out
}

// UpsertAnalysisRunDomainSummary inserts or updates one run+cohort summary row.
func (s *SQLJobStore) UpsertAnalysisRunDomainSummary(item AnalysisRunDomainSummary) error {
	return s.upsertAnalysisRunDomainSummaryIn(s.db, item)
}

func (s *SQLJobStore) upsertAnalysisRunDomainSummaryIn(q sqlQuerier, item AnalysisRunDomainSummary) error {
	if item.CohortID == 0 {
		return errors.New("cohort_id is required")
	}
	if item.RunID == "" {
		return errors.New("run_id is required")
	}
	if item.DomainID == 0 {
		return errors.New("domain_id is required")
	}

	if s.dialect.SupportsOnConflictReturning() {
		query := fmt.Sprintf(
			`INSERT INTO analysis_run_domain_summary (
				cohort_id, run_id, domain_id, score, grade, nameserver_count,
				endpoint_count, asn_count, prefix_count, worst_level
			) VALUES (%s)
			 ON CONFLICT (cohort_id, run_id, domain_id) DO UPDATE SET
			   score = EXCLUDED.score,
			   grade = EXCLUDED.grade,
			   nameserver_count = EXCLUDED.nameserver_count,
			   endpoint_count = EXCLUDED.endpoint_count,
			   asn_count = EXCLUDED.asn_count,
			   prefix_count = EXCLUDED.prefix_count,
			   worst_level = EXCLUDED.worst_level`,
			s.phRange(1, 10),
		)
		if _, err := q.Exec(query,
			item.CohortID,
			item.RunID,
			item.DomainID,
			nullIntValue(item.Score),
			nullStringPtr(item.Grade),
			item.NameserverCount,
			item.EndpointCount,
			item.ASNCount,
			item.PrefixCount,
			item.WorstLevel,
		); err != nil {
			return fmt.Errorf("upsert analysis run domain summary: %w", err)
		}
		return nil
	}

	row := q.QueryRow(
		fmt.Sprintf(`SELECT COUNT(*) FROM analysis_run_domain_summary
			WHERE cohort_id = %s AND run_id = %s AND domain_id = %s`,
			s.ph(1), s.ph(2), s.ph(3)),
		item.CohortID, item.RunID, item.DomainID,
	)
	var count int
	if err := row.Scan(&count); err != nil {
		return fmt.Errorf("check analysis run domain summary: %w", err)
	}
	if count > 0 {
		_, err := q.Exec(
			fmt.Sprintf(`UPDATE analysis_run_domain_summary SET
				score = %s, grade = %s, nameserver_count = %s, endpoint_count = %s,
				asn_count = %s, prefix_count = %s, worst_level = %s
			WHERE cohort_id = %s AND run_id = %s AND domain_id = %s`,
				s.ph(1), s.ph(2), s.ph(3), s.ph(4), s.ph(5), s.ph(6), s.ph(7),
				s.ph(8), s.ph(9), s.ph(10)),
			nullIntValue(item.Score),
			nullStringPtr(item.Grade),
			item.NameserverCount,
			item.EndpointCount,
			item.ASNCount,
			item.PrefixCount,
			item.WorstLevel,
			item.CohortID,
			item.RunID,
			item.DomainID,
		)
		if err != nil {
			return fmt.Errorf("update analysis run domain summary: %w", err)
		}
		return nil
	}

	_, err := q.Exec(
		fmt.Sprintf(`INSERT INTO analysis_run_domain_summary (
			cohort_id, run_id, domain_id, score, grade, nameserver_count,
			endpoint_count, asn_count, prefix_count, worst_level
		) VALUES (%s)`, s.phRange(1, 10)),
		item.CohortID,
		item.RunID,
		item.DomainID,
		nullIntValue(item.Score),
		nullStringPtr(item.Grade),
		item.NameserverCount,
		item.EndpointCount,
		item.ASNCount,
		item.PrefixCount,
		item.WorstLevel,
	)
	if err != nil {
		return fmt.Errorf("insert analysis run domain summary: %w", err)
	}
	return nil
}

func nullIntValue(v *int) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*v), Valid: true}
}

// GetAnalysisRunDomainSummary returns one run+cohort summary row.
func (s *SQLJobStore) GetAnalysisRunDomainSummary(cohortID int64, runID string, domainID int64) (AnalysisRunDomainSummary, bool) {
	row := s.db.QueryRow(
		fmt.Sprintf(`SELECT %s FROM analysis_run_domain_summary
			WHERE cohort_id = %s AND run_id = %s AND domain_id = %s`,
			analysisRunSummaryCols, s.ph(1), s.ph(2), s.ph(3)),
		cohortID, runID, domainID,
	)
	var (
		item  AnalysisRunDomainSummary
		score sql.NullInt64
		grade sql.NullString
	)
	if err := row.Scan(
		&item.CohortID, &item.RunID, &item.DomainID, &score, &grade,
		&item.NameserverCount, &item.EndpointCount, &item.ASNCount, &item.PrefixCount, &item.WorstLevel,
	); err != nil {
		return AnalysisRunDomainSummary{}, false
	}
	if score.Valid {
		v := int(score.Int64)
		item.Score = &v
	}
	item.Grade = nullStringScanPtr(grade)
	return item, true
}

// SetAnalysisProjectionState inserts or updates projection bookkeeping for one run+cohort.
func (s *SQLJobStore) SetAnalysisProjectionState(item AnalysisProjectionState) error {
	return s.setAnalysisProjectionStateIn(s.db, item)
}

func (s *SQLJobStore) setAnalysisProjectionStateIn(q sqlQuerier, item AnalysisProjectionState) error {
	if item.CohortID == 0 {
		return errors.New("cohort_id is required")
	}
	if item.RunID == "" {
		return errors.New("run_id is required")
	}
	if item.ProjectorVersion == "" {
		return errors.New("projector_version is required")
	}
	if item.Status == "" {
		return errors.New("status is required")
	}

	if s.dialect.SupportsOnConflictReturning() {
		query := fmt.Sprintf(
			`INSERT INTO analysis_projection_state (
				cohort_id, run_id, projector_version, status, projected_at, error
			) VALUES (%s)
			 ON CONFLICT (cohort_id, run_id) DO UPDATE SET
			   projector_version = EXCLUDED.projector_version,
			   status = EXCLUDED.status,
			   projected_at = EXCLUDED.projected_at,
			   error = EXCLUDED.error`,
			s.phRange(1, 6),
		)
		if _, err := q.Exec(query,
			item.CohortID, item.RunID, item.ProjectorVersion, item.Status, s.ts(item.ProjectedAt), item.Error,
		); err != nil {
			return fmt.Errorf("upsert analysis projection state: %w", err)
		}
		return nil
	}

	row := q.QueryRow(
		fmt.Sprintf(`SELECT COUNT(*) FROM analysis_projection_state WHERE cohort_id = %s AND run_id = %s`,
			s.ph(1), s.ph(2)),
		item.CohortID, item.RunID,
	)
	var count int
	if err := row.Scan(&count); err != nil {
		return fmt.Errorf("check analysis projection state: %w", err)
	}
	if count > 0 {
		_, err := q.Exec(
			fmt.Sprintf(`UPDATE analysis_projection_state SET projector_version = %s, status = %s,
				projected_at = %s, error = %s WHERE cohort_id = %s AND run_id = %s`,
				s.ph(1), s.ph(2), s.ph(3), s.ph(4), s.ph(5), s.ph(6)),
			item.ProjectorVersion, item.Status, s.ts(item.ProjectedAt), item.Error, item.CohortID, item.RunID,
		)
		if err != nil {
			return fmt.Errorf("update analysis projection state: %w", err)
		}
		return nil
	}

	_, err := q.Exec(
		fmt.Sprintf(`INSERT INTO analysis_projection_state (
			cohort_id, run_id, projector_version, status, projected_at, error
		) VALUES (%s)`, s.phRange(1, 6)),
		item.CohortID, item.RunID, item.ProjectorVersion, item.Status, s.ts(item.ProjectedAt), item.Error,
	)
	if err != nil {
		return fmt.Errorf("insert analysis projection state: %w", err)
	}
	return nil
}

// GetAnalysisProjectionState returns one projection bookkeeping row.
func (s *SQLJobStore) GetAnalysisProjectionState(cohortID int64, runID string) (AnalysisProjectionState, bool) {
	row := s.db.QueryRow(
		fmt.Sprintf(`SELECT %s FROM analysis_projection_state WHERE cohort_id = %s AND run_id = %s`,
			analysisProjStateCols, s.ph(1), s.ph(2)),
		cohortID, runID,
	)
	var (
		item        AnalysisProjectionState
		projectedAt sql.NullString
	)
	if err := row.Scan(
		&item.CohortID, &item.RunID, &item.ProjectorVersion, &item.Status, &projectedAt, &item.Error,
	); err != nil {
		return AnalysisProjectionState{}, false
	}
	item.ProjectedAt = parseTimestampNullStr(projectedAt)
	return item, true
}

// ClearAnalysisCohortMaterialization removes all materialized rows for one cohort.
func (s *SQLJobStore) ClearAnalysisCohortMaterialization(cohortID int64) error {
	for _, query := range []string{
		fmt.Sprintf(`DELETE FROM analysis_projection_state WHERE cohort_id = %s`, s.ph(1)),
		fmt.Sprintf(`DELETE FROM analysis_run_tag_summary WHERE cohort_id = %s`, s.ph(1)),
		fmt.Sprintf(`DELETE FROM analysis_run_domain_summary WHERE cohort_id = %s`, s.ph(1)),
		fmt.Sprintf(`DELETE FROM analysis_run_address_asns WHERE cohort_id = %s`, s.ph(1)),
		fmt.Sprintf(`DELETE FROM analysis_run_domain_asns WHERE cohort_id = %s`, s.ph(1)),
		fmt.Sprintf(`DELETE FROM analysis_run_ns_endpoints WHERE cohort_id = %s`, s.ph(1)),
	} {
		if _, err := s.db.Exec(query, cohortID); err != nil {
			return fmt.Errorf("clear analysis cohort %d materialization: %w", cohortID, err)
		}
	}
	return nil
}
