package server

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	analysisNameserverCols = `id, name, first_seen_at, last_seen_at`
	analysisAddressCols    = `id, address, family, first_seen_at, last_seen_at`
	analysisPrefixCols     = `id, prefix, family, first_seen_at, last_seen_at`
	analysisASNCols        = `asn, label, first_seen_at, last_seen_at`
	analysisRunNSEndCols   = `cohort_id, run_id, domain_id, nameserver_id, address_id,
		role, source, family, avg_ms, min_ms, max_ms, query_count`
	analysisRunAddrASNCols = `cohort_id, run_id, domain_id, address_id, prefix_id, asn, lookup_status, source`
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

func (s *SQLJobStore) upsertSeenEntity(
	selectQuery string,
	selectArgs []any,
	insertQuery string,
	insertArgs []any,
	updateQuery string,
	updateArgsFn func(id int64, existingFirst, existingLast time.Time) []any,
	scan func(row rowScanner) (int64, time.Time, time.Time, error),
) (int64, error) {
	row := s.db.QueryRow(selectQuery, selectArgs...)
	id, firstSeen, lastSeen, err := scan(row)
	if err == nil {
		args := updateArgsFn(id, firstSeen, lastSeen)
		if _, err := s.db.Exec(updateQuery, args...); err != nil {
			return 0, err
		}
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if _, err := s.db.Exec(insertQuery, insertArgs...); err != nil {
		return 0, err
	}
	row = s.db.QueryRow(selectQuery, selectArgs...)
	id, _, _, err = scan(row)
	if err != nil {
		return 0, err
	}
	return id, nil
}

// UpsertAnalysisNameserver inserts or updates one normalized nameserver row.
func (s *SQLJobStore) UpsertAnalysisNameserver(name string, seenAt time.Time) (AnalysisNameserver, error) {
	if name == "" {
		return AnalysisNameserver{}, errors.New("nameserver name is required")
	}
	seenAt = normalizeSeenAt(seenAt)
	id, err := s.upsertSeenEntity(
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
	item, ok := s.GetAnalysisNameserver(id)
	if !ok {
		return AnalysisNameserver{}, errors.New("analysis nameserver written but not readable")
	}
	return item, nil
}

// GetAnalysisNameserver returns a normalized nameserver row by id.
func (s *SQLJobStore) GetAnalysisNameserver(id int64) (AnalysisNameserver, bool) {
	row := s.db.QueryRow(
		fmt.Sprintf(`SELECT %s FROM analysis_nameservers WHERE id = %s`, analysisNameserverCols, s.ph(1)),
		id,
	)
	item, err := scanAnalysisNameserverRow(row)
	if err == nil {
		return item, true
	}
	return AnalysisNameserver{}, false
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
	if address == "" {
		return AnalysisAddress{}, errors.New("address is required")
	}
	if family == "" {
		return AnalysisAddress{}, errors.New("address family is required")
	}
	seenAt = normalizeSeenAt(seenAt)
	id, err := s.upsertSeenEntity(
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
	item, ok := s.GetAnalysisAddress(id)
	if !ok {
		return AnalysisAddress{}, errors.New("analysis address written but not readable")
	}
	return item, nil
}

// GetAnalysisAddress returns a normalized address row by id.
func (s *SQLJobStore) GetAnalysisAddress(id int64) (AnalysisAddress, bool) {
	row := s.db.QueryRow(
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
	if prefix == "" {
		return AnalysisPrefix{}, errors.New("prefix is required")
	}
	if family == "" {
		return AnalysisPrefix{}, errors.New("prefix family is required")
	}
	seenAt = normalizeSeenAt(seenAt)
	id, err := s.upsertSeenEntity(
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
	item, ok := s.GetAnalysisPrefix(id)
	if !ok {
		return AnalysisPrefix{}, errors.New("analysis prefix written but not readable")
	}
	return item, nil
}

// GetAnalysisPrefix returns a normalized prefix row by id.
func (s *SQLJobStore) GetAnalysisPrefix(id int64) (AnalysisPrefix, bool) {
	row := s.db.QueryRow(
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
	if asn == 0 {
		return AnalysisASN{}, errors.New("asn is required")
	}
	seenAt = normalizeSeenAt(seenAt)
	row := s.db.QueryRow(
		fmt.Sprintf(`SELECT first_seen_at, last_seen_at FROM analysis_asns WHERE asn = %s`, s.ph(1)),
		asn,
	)
	var firstSeen, lastSeen string
	err := row.Scan(&firstSeen, &lastSeen)
	switch {
	case err == nil:
		_, err = s.db.Exec(
			fmt.Sprintf(`UPDATE analysis_asns SET label = %s, first_seen_at = %s, last_seen_at = %s WHERE asn = %s`,
				s.ph(1), s.ph(2), s.ph(3), s.ph(4)),
			label, s.ts(minTime(parseTimestampStr(firstSeen), seenAt)), s.ts(maxTime(parseTimestampStr(lastSeen), seenAt)), asn,
		)
	case errors.Is(err, sql.ErrNoRows):
		_, err = s.db.Exec(
			fmt.Sprintf(`INSERT INTO analysis_asns (asn, label, first_seen_at, last_seen_at) VALUES (%s)`,
				s.phRange(1, 4)),
			asn, label, s.ts(seenAt), s.ts(seenAt),
		)
	}
	if err != nil {
		return AnalysisASN{}, fmt.Errorf("upsert analysis asn: %w", err)
	}
	item, ok := s.GetAnalysisASN(asn)
	if !ok {
		return AnalysisASN{}, errors.New("analysis asn written but not readable")
	}
	return item, nil
}

// GetAnalysisASN returns a normalized ASN row by primary key.
func (s *SQLJobStore) GetAnalysisASN(asn int64) (AnalysisASN, bool) {
	row := s.db.QueryRow(
		fmt.Sprintf(`SELECT %s FROM analysis_asns WHERE asn = %s`, analysisASNCols, s.ph(1)),
		asn,
	)
	item, err := scanAnalysisASNRow(row)
	if err == nil {
		return item, true
	}
	return AnalysisASN{}, false
}

func (s *SQLJobStore) replaceAnalysisRows(
	deleteQuery string,
	deleteArgs []any,
	insertQuery string,
	rows [][]any,
) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(deleteQuery, deleteArgs...); err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := tx.Exec(insertQuery, row...); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ReplaceAnalysisRunNSEndpoints replaces all run+cohort nameserver endpoint rows.
func (s *SQLJobStore) ReplaceAnalysisRunNSEndpoints(cohortID int64, runID string, items []AnalysisRunNameserverEndpoint) error {
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
	err := s.replaceAnalysisRows(
		fmt.Sprintf(`DELETE FROM analysis_run_ns_endpoints WHERE cohort_id = %s AND run_id = %s`, s.ph(1), s.ph(2)),
		[]any{cohortID, runID},
		fmt.Sprintf(`INSERT INTO analysis_run_ns_endpoints (
			cohort_id, run_id, domain_id, nameserver_id, address_id, role, source, family,
			avg_ms, min_ms, max_ms, query_count
		) VALUES (%s)`, s.phRange(1, 12)),
		rows,
	)
	if err != nil {
		return fmt.Errorf("replace analysis run ns endpoints: %w", err)
	}
	return nil
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

// ReplaceAnalysisRunAddressASNs replaces all run+cohort address-to-ASN rows.
func (s *SQLJobStore) ReplaceAnalysisRunAddressASNs(cohortID int64, runID string, items []AnalysisRunAddressASN) error {
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
	err := s.replaceAnalysisRows(
		fmt.Sprintf(`DELETE FROM analysis_run_address_asns WHERE cohort_id = %s AND run_id = %s`, s.ph(1), s.ph(2)),
		[]any{cohortID, runID},
		fmt.Sprintf(`INSERT INTO analysis_run_address_asns (
			cohort_id, run_id, domain_id, address_id, prefix_id, asn, lookup_status, source
		) VALUES (%s)`, s.phRange(1, 8)),
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

// UpsertAnalysisRunDomainSummary inserts or updates one run+cohort summary row.
func (s *SQLJobStore) UpsertAnalysisRunDomainSummary(item AnalysisRunDomainSummary) error {
	if item.CohortID == 0 {
		return errors.New("cohort_id is required")
	}
	if item.RunID == "" {
		return errors.New("run_id is required")
	}
	if item.DomainID == 0 {
		return errors.New("domain_id is required")
	}

	row := s.db.QueryRow(
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
		_, err := s.db.Exec(
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

	_, err := s.db.Exec(
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

	row := s.db.QueryRow(
		fmt.Sprintf(`SELECT COUNT(*) FROM analysis_projection_state WHERE cohort_id = %s AND run_id = %s`,
			s.ph(1), s.ph(2)),
		item.CohortID, item.RunID,
	)
	var count int
	if err := row.Scan(&count); err != nil {
		return fmt.Errorf("check analysis projection state: %w", err)
	}
	if count > 0 {
		_, err := s.db.Exec(
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

	_, err := s.db.Exec(
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
		fmt.Sprintf(`DELETE FROM analysis_run_domain_summary WHERE cohort_id = %s`, s.ph(1)),
		fmt.Sprintf(`DELETE FROM analysis_run_address_asns WHERE cohort_id = %s`, s.ph(1)),
		fmt.Sprintf(`DELETE FROM analysis_run_ns_endpoints WHERE cohort_id = %s`, s.ph(1)),
	} {
		if _, err := s.db.Exec(query, cohortID); err != nil {
			return fmt.Errorf("clear analysis cohort %d materialization: %w", cohortID, err)
		}
	}
	return nil
}
