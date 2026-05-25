package recursor

import (
	"errors"
	"fmt"
	"time"
)

// CNAMEReason identifies which CNAME-handling failure the recursor hit.
type CNAMEReason int

const (
	// CNAMETooMany covers per-answer cardinality and chain-walk inside one answer.
	CNAMETooMany CNAMEReason = iota + 1
	// CNAMEChainTooLong is the chain-depth bound across recursion hops.
	CNAMEChainTooLong
	// CNAMEUnresolved is "target can't be reached": loop, broken chain, qtype mismatch.
	CNAMEUnresolved
)

// CNAMEError is returned by Recurse when CNAME handling fails.
type CNAMEError struct {
	Reason CNAMEReason
	Name   string
	Target string
	Detail string
}

func (e *CNAMEError) Error() string {
	if e == nil {
		return "<nil>"
	}
	reason := e.Reason.String()
	if e.Detail != "" {
		reason = reason + "/" + e.Detail
	}
	if e.Target != "" {
		return fmt.Sprintf("cname %s: %s -> %s", reason, e.Name, e.Target)
	}
	return fmt.Sprintf("cname %s: %s", reason, e.Name)
}

func (r CNAMEReason) String() string {
	switch r {
	case CNAMETooMany:
		return "too-many"
	case CNAMEChainTooLong:
		return "chain-too-long"
	case CNAMEUnresolved:
		return "unresolved"
	}
	return "unknown"
}

// IgnoreCNAMEError returns nil if err is a *CNAMEError, and err otherwise.
func IgnoreCNAMEError(err error) error {
	if err == nil {
		return nil
	}
	var ce *CNAMEError
	if errors.As(err, &ce) {
		return nil
	}
	return err
}

// SeedCNAMEError pre-populates the cache with a CNAME error for the given
// (name, qtypes) so tests can drive Recurse-via-cache without simulating
// the full upstream walk. Test helper.
func (r *Recursor) SeedCNAMEError(err *CNAMEError, name string, qtypes []string) {
	if r == nil || err == nil {
		return
	}
	prev := r.negativeCacheTTL
	if prev <= 0 {
		r.SetNegativeCacheTTL(time.Hour)
		defer r.SetNegativeCacheTTL(prev)
	}
	key := "root|" + name
	for _, qtype := range qtypes {
		r.cacheStoreNegative(key, qtype, "IN", err)
	}
}
