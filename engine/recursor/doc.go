// Package recursor performs iterative DNS recursion for lookup helpers.
package recursor

import "errors"

// ErrRaceLost is the cause attached to the cancellation of a parallel
// fan-out's batch context when one goroutine has already produced a
// usable answer and the rest are being cancelled.
//
// Callers that observe context cancellation in code paths that may be
// reached via parallel recursion can distinguish a race-loss cancellation
// from a job-level cancellation by inspecting context.Cause(ctx).
var ErrRaceLost = errors.New("recursor: parallel race lost")
