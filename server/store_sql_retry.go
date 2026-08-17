package server

import (
	"fmt"
	"math/rand/v2"
	"time"
)

// Retry budget for transactions aborted by a concurrency conflict.
const (
	txMaxAttempts  = 5
	txRetryBaseGap = 10 * time.Millisecond
)

// txRetrySleep is a var so tests can drain the backoff.
var txRetrySleep = time.Sleep

// retryOnConflict runs fn until it succeeds, fails with a non-conflict
// error, or exhausts the attempt budget. fn must open its own transaction
// (only a fresh Begin gets a fresh read view) and be idempotent across
// attempts.
func (s *SQLJobStore) retryOnConflict(op string, fn func() error) error {
	var err error
	for attempt := 1; attempt <= txMaxAttempts; attempt++ {
		err = fn()
		if err == nil {
			return nil
		}
		if !s.dialect.IsRetryableConflict(err) {
			return err
		}
		if attempt < txMaxAttempts {
			txRetrySleep(txRetryBackoff(attempt))
		}
	}
	return fmt.Errorf("%s: gave up after %d conflicting attempts: %w", op, txMaxAttempts, err)
}

// txRetryBackoff returns a jittered exponential delay for a 1-based
// attempt. Jitter keeps two colliding transactions from re-colliding.
func txRetryBackoff(attempt int) time.Duration {
	gap := txRetryBaseGap << (attempt - 1)
	return gap + rand.N(gap)
}
