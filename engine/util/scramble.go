package util

import (
	"math/rand"
	"strings"
	"time"
)

var scrambleRand = rand.New(rand.NewSource(time.Now().UnixNano()))

// ScrambleCase randomly mixes upper and lower case, following the Perl algorithm.
func ScrambleCase(input string) string {
	return ScrambleCaseWith(input, scrambleRand)
}

// ScrambleCaseWith mixes upper and lower case using the provided rand source.
// Using a fixed-seed rand.Rand produces deterministic output, which is useful
// for testing.
func ScrambleCaseWith(input string, r *rand.Rand) string {
	chars := strings.Split(input, "")
	uppers := 2.0
	downers := 1.0

	for i, c := range chars {
		limit := 1.0 + (downers / uppers)
		uppity := int(r.Float64() * limit)
		if uppity != 0 {
			chars[i] = strings.ToUpper(c)
			uppers++
		} else {
			chars[i] = strings.ToLower(c)
			downers++
		}
	}

	return strings.Join(chars, "")
}
