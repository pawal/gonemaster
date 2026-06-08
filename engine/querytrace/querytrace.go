// Package querytrace is a per-run observer for the DNS query lifecycle: every
// transport attempt (including timeouts) and every slow-server control decision.
package querytrace

import "time"

// Outcome classifies how a single transport attempt ended.
type Outcome string

const (
	// OutcomeOK marks an attempt that received a DNS response.
	OutcomeOK Outcome = "ok"
	// OutcomeTimeout marks an attempt whose deadline fired with no response.
	OutcomeTimeout Outcome = "timeout"
	// OutcomeError marks an attempt that failed for a non-timeout reason.
	OutcomeError Outcome = "error"
)

// AttemptEvent describes one transport-level DNS attempt within the
// (1 + retries) budget, including attempts that time out.
type AttemptEvent struct {
	// NSName is the nameserver host name, when known.
	NSName string
	// NSAddr is the nameserver address (IP or IP:port) the attempt was sent to.
	NSAddr string
	// QName is the queried owner name.
	QName string
	// QType is the queried record type (e.g. "SOA").
	QType string
	// Protocol is "udp" or "tcp".
	Protocol string
	// Attempt is the 1-based attempt index within the retry budget.
	Attempt int
	// Elapsed is the attempt wall-clock; for a timeout, the budget consumed.
	Elapsed time.Duration
	// Outcome classifies the result.
	Outcome Outcome
	// Err is the error string for timeout/error outcomes, empty on success.
	Err string
}

// DecisionKind names a slow-server control action observed during a run.
type DecisionKind string

const (
	// DecisionFastFailBlocked: fast-fail just blocked a nameserver/protocol
	// after reaching the consecutive-timeout threshold.
	DecisionFastFailBlocked DecisionKind = "fastfail_blocked"
	// DecisionSkippedFastFail: a query was skipped because fast-fail had already
	// blocked this nameserver/protocol.
	DecisionSkippedFastFail DecisionKind = "skipped_fastfail"
	// DecisionSkippedErrorCache: a query was skipped because of a cached error.
	DecisionSkippedErrorCache DecisionKind = "skipped_errorcache"
	// DecisionSkippedBlacklist: a query was skipped because the nameserver was
	// blacklisted earlier in the run.
	DecisionSkippedBlacklist DecisionKind = "skipped_blacklist"
	// DecisionSkippedReachability: a query was skipped because the address was
	// marked unreachable earlier in the run.
	DecisionSkippedReachability DecisionKind = "skipped_reachability"
	// DecisionLatencyBudgetBlocked: a nameserver address exceeded its cumulative
	// latency budget and is now blocked for the remainder of the run.
	DecisionLatencyBudgetBlocked DecisionKind = "latency_budget_blocked"
	// DecisionSkippedLatencyBudget: a query was skipped because the address was
	// latency-budget blocked.
	DecisionSkippedLatencyBudget DecisionKind = "skipped_latency_budget"
	// DecisionErrorCached: a failed query was written to the error cache.
	DecisionErrorCached DecisionKind = "errorcached"
	// DecisionBlacklisted: a nameserver was blacklisted after a failed SOA query.
	DecisionBlacklisted DecisionKind = "blacklisted"
)

// DecisionEvent describes a slow-server control action against a nameserver.
type DecisionEvent struct {
	// Kind is the control action taken.
	Kind DecisionKind
	// NSName is the nameserver host name, when known.
	NSName string
	// NSAddr is the nameserver address the decision applies to.
	NSAddr string
	// QName is the query that triggered the decision, when applicable.
	QName string
	// QType is the query type that triggered the decision, when applicable.
	QType string
	// Protocol is "udp" or "tcp" when the decision is protocol-specific.
	Protocol string
}

// QueryTrace observes one run's DNS query lifecycle. Implementations must be
// concurrency-safe; a nil QueryTrace is never called.
type QueryTrace interface {
	// AttemptDone reports one completed transport attempt.
	AttemptDone(AttemptEvent)
	// Decision reports a slow-server control action.
	Decision(DecisionEvent)
}
