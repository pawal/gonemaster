// Package recursortest provides test-only helpers for driving the
// recursor's cache directly, without simulating an upstream walk.
package recursortest

import (
	"codeberg.org/pawal/gonemaster/engine/recursor"
)

// SeedCNAMEError primes r's cache so a later Recurse for each
// (name, qtype, IN) returns err.
func SeedCNAMEError(r *recursor.Recursor, err *recursor.CNAMEError, name string, qtypes []string) {
	if r == nil || err == nil {
		return
	}
	for _, qtype := range qtypes {
		r.PrimeCacheError(name, qtype, err)
	}
}
