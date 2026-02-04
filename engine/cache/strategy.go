package cache

// ErrorKind classifies query failures for caching decisions.
type ErrorKind int

const (
	ErrorNone ErrorKind = iota
	// ErrorNetwork covers timeouts, temporary network failures, or unreachable hosts.
	ErrorNetwork
	// ErrorCanceled covers context cancellation or deadline exceeded.
	ErrorCanceled
	// ErrorOther covers non-network errors (invalid query, parse errors, etc.).
	ErrorOther
)

// Decision indicates which global cache bucket should store a result.
type Decision int

const (
	DecisionNone Decision = iota
	DecisionPositive
	DecisionNegative
)

// Result summarizes a DNS query outcome for cache decisions.
// HasResponse means the resolver received a DNS response message.
// ErrorKind classifies the error when HasResponse is false.
type Result struct {
	HasResponse bool
	ErrorKind   ErrorKind
}

// Decide applies the global caching strategy for DNS query outcomes.
// Positive responses are cached in the positive cache, while network failures
// (timeouts, unreachable hosts) are cached in the negative cache.
func Decide(result Result) Decision {
	if result.HasResponse {
		return DecisionPositive
	}
	if result.ErrorKind == ErrorNetwork {
		return DecisionNegative
	}
	return DecisionNone
}
