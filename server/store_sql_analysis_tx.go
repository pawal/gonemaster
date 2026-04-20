package server

import (
	"database/sql"
	"fmt"
	"time"
)

// AnalysisWriteStore is the narrow write surface the projector drives per
// run. Declared here (not in server/analysis) so server can add a
// transaction-backed implementation without importing analysis, which
// would cycle: analysis already imports server for model types.
type AnalysisWriteStore interface {
	UpsertAnalysisNameserver(name string, seenAt time.Time) (AnalysisNameserver, error)
	UpsertAnalysisAddress(address, family string, seenAt time.Time) (AnalysisAddress, error)
	UpsertAnalysisPrefix(prefix, family string, seenAt time.Time) (AnalysisPrefix, error)
	UpsertAnalysisASN(asn int64, label string, seenAt time.Time) (AnalysisASN, error)
	ReplaceAnalysisRunNSEndpoints(cohortID int64, runID string, items []AnalysisRunNameserverEndpoint) error
	ReplaceAnalysisRunAddressASNs(cohortID int64, runID string, items []AnalysisRunAddressASN) error
	ReplaceAnalysisRunDomainASNs(cohortID int64, runID string, items []AnalysisRunDomainASN) error
	UpsertAnalysisRunDomainSummary(item AnalysisRunDomainSummary) error
	SetAnalysisProjectionState(item AnalysisProjectionState) error
}

// sqlQuerier is the subset of *sql.DB and *sql.Tx used by the analysis
// write helpers. Letting the helpers take a querier instead of pinning
// them to s.db means one ProjectLoaded call can run all its upserts and
// replace-blocks inside a single outer transaction — one commit per run
// instead of one commit per statement.
type sqlQuerier interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// analysisWriteTxStore is the transaction-bound implementation of
// analysis.WriteStore. Every method forwards to the parent SQLJobStore's
// tx-aware helper using the transaction, so all writes for one run share
// one commit.
type analysisWriteTxStore struct {
	parent *SQLJobStore
	tx     *sql.Tx
}

func (t *analysisWriteTxStore) UpsertAnalysisNameserver(name string, seenAt time.Time) (AnalysisNameserver, error) {
	return t.parent.upsertAnalysisNameserverIn(t.tx, name, seenAt)
}

func (t *analysisWriteTxStore) UpsertAnalysisAddress(address, family string, seenAt time.Time) (AnalysisAddress, error) {
	return t.parent.upsertAnalysisAddressIn(t.tx, address, family, seenAt)
}

func (t *analysisWriteTxStore) UpsertAnalysisPrefix(prefix, family string, seenAt time.Time) (AnalysisPrefix, error) {
	return t.parent.upsertAnalysisPrefixIn(t.tx, prefix, family, seenAt)
}

func (t *analysisWriteTxStore) UpsertAnalysisASN(asn int64, label string, seenAt time.Time) (AnalysisASN, error) {
	return t.parent.upsertAnalysisASNIn(t.tx, asn, label, seenAt)
}

func (t *analysisWriteTxStore) ReplaceAnalysisRunNSEndpoints(cohortID int64, runID string, items []AnalysisRunNameserverEndpoint) error {
	return t.parent.replaceAnalysisRunNSEndpointsIn(t.tx, cohortID, runID, items)
}

func (t *analysisWriteTxStore) ReplaceAnalysisRunAddressASNs(cohortID int64, runID string, items []AnalysisRunAddressASN) error {
	return t.parent.replaceAnalysisRunAddressASNsIn(t.tx, cohortID, runID, items)
}

func (t *analysisWriteTxStore) ReplaceAnalysisRunDomainASNs(cohortID int64, runID string, items []AnalysisRunDomainASN) error {
	return t.parent.replaceAnalysisRunDomainASNsIn(t.tx, cohortID, runID, items)
}

func (t *analysisWriteTxStore) UpsertAnalysisRunDomainSummary(item AnalysisRunDomainSummary) error {
	return t.parent.upsertAnalysisRunDomainSummaryIn(t.tx, item)
}

func (t *analysisWriteTxStore) SetAnalysisProjectionState(item AnalysisProjectionState) error {
	return t.parent.setAnalysisProjectionStateIn(t.tx, item)
}

// WithAnalysisWriteTx runs fn inside one transaction and commits if fn
// returns nil. The projector calls this so every per-run upsert and
// replace-block for one ProjectLoaded call commits together instead of
// paying one fsync per statement.
func (s *SQLJobStore) WithAnalysisWriteTx(fn func(AnalysisWriteStore) error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin analysis write tx: %w", err)
	}
	writer := &analysisWriteTxStore{parent: s, tx: tx}
	if err := fn(writer); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit analysis write tx: %w", err)
	}
	return nil
}
