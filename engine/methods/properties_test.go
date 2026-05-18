package methods

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

// Property-style tests: invariants that must hold across many input shapes.
// Each test runs several seeded scenarios; if any fails, the seed is logged
// so the failure is reproducible.

// randomNameSet returns a slice of N pseudo-random label names under
// "example.com". Names may include duplicates and mixed case so callers can
// verify deduplication and case folding.
func randomNameSet(rng *rand.Rand, n int) []string {
	letters := []byte("abcdefghijklmnopqrstuvwxyz")
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		label := make([]byte, 1+rng.Intn(4))
		for j := range label {
			c := letters[rng.Intn(len(letters))]
			// 30% chance of uppercasing to exercise case folding.
			if rng.Intn(10) < 3 {
				c -= 32
			}
			label[j] = c
		}
		out = append(out, string(label)+".example.com")
	}
	return out
}

func isSortedLowercase(names []string) bool {
	cmp := make([]string, len(names))
	for i, n := range names {
		cmp[i] = strings.ToLower(n)
	}
	return sort.StringsAreSorted(cmp)
}

func hasNoDuplicates(names []string) bool {
	seen := map[string]bool{}
	for _, n := range names {
		key := strings.ToLower(n)
		if seen[key] {
			return false
		}
		seen[key] = true
	}
	return true
}

// runMethod3Property sets up a zone with the given apex-reported NS names
// and returns Method3's output as a string slice.
func runMethod3Property(t *testing.T, names []string) []string {
	t.Helper()
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)

	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{"a.root": {"192.0.2.1"}}); err != nil {
		t.Fatalf("add root: %v", err)
	}
	if err := r.AddFakeAddresses("example.com", map[string][]string{
		"ns1.example.com": {"192.0.2.11"},
	}); err != nil {
		t.Fatalf("add zone: %v", err)
	}
	// Single apex server returns the entire (possibly duplicated, mixed-case)
	// random name list as its NS RRset.
	setNSHook(ctx, t, r, "ns1.example.com", "192.0.2.11", "example.com", names...)

	z, err := zone.NewWithRecursor("example.com", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	got, err := Method3(ctx, &z)
	if err != nil {
		t.Fatalf("method3: %v", err)
	}
	out := make([]string, 0, len(got))
	for _, n := range got {
		out = append(out, n.String())
	}
	return out
}

// TestMethod3PropertyAlwaysSortedAndDeduped runs 30 seeded scenarios with
// random NS name lists (varying sizes, duplicates, mixed case) and asserts
// the output is always lowercase-sorted and contains no duplicates.
func TestMethod3PropertyAlwaysSortedAndDeduped(t *testing.T) {
	for trial := 0; trial < 30; trial++ {
		seed := int64(trial * 17)
		rng := rand.New(rand.NewSource(seed))
		size := 1 + rng.Intn(8)
		names := randomNameSet(rng, size)
		t.Run(fmt.Sprintf("seed=%d-size=%d", seed, size), func(t *testing.T) {
			out := runMethod3Property(t, names)
			if !isSortedLowercase(out) {
				t.Errorf("output not sorted: %#v (input: %#v)", out, names)
			}
			if !hasNoDuplicates(out) {
				t.Errorf("output has duplicates: %#v (input: %#v)", out, names)
			}
		})
	}
}

// runMethod2and3Property sets up an undelegated zone where the glue (from
// AddFakeAddresses) and the apex-reported NS sets may overlap or diverge.
// Returns Method2and3's output as a string slice.
func runMethod2and3Property(t *testing.T, glueNames []string, apexNames []string) []string {
	t.Helper()
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)

	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{"a.root": {"192.0.2.1"}}); err != nil {
		t.Fatalf("add root: %v", err)
	}
	glue := map[string][]string{}
	for i, name := range glueNames {
		glue[strings.ToLower(name)] = []string{fmt.Sprintf("192.0.2.%d", 20+i)}
	}
	if err := r.AddFakeAddresses("example.com", glue); err != nil {
		t.Fatalf("add zone: %v", err)
	}
	for i, name := range glueNames {
		setNSHook(ctx, t, r, strings.ToLower(name), fmt.Sprintf("192.0.2.%d", 20+i), "example.com", apexNames...)
	}

	z, err := zone.NewWithRecursor("example.com", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	got, err := Method2and3(ctx, &z)
	if err != nil {
		t.Fatalf("method2and3: %v", err)
	}
	out := make([]string, 0, len(got))
	for _, n := range got {
		out = append(out, n.String())
	}
	return out
}

// TestMethod2and3PropertyAlwaysSortedAndDeduped runs 20 seeded scenarios
// with random glue and apex name sets and asserts the union output is
// always lowercase-sorted and contains no duplicates.
func TestMethod2and3PropertyAlwaysSortedAndDeduped(t *testing.T) {
	for trial := 0; trial < 20; trial++ {
		seed := int64(trial * 31)
		rng := rand.New(rand.NewSource(seed))
		glue := randomNameSet(rng, 1+rng.Intn(4))
		apex := randomNameSet(rng, 1+rng.Intn(4))
		t.Run(fmt.Sprintf("seed=%d", seed), func(t *testing.T) {
			out := runMethod2and3Property(t, glue, apex)
			if !isSortedLowercase(out) {
				t.Errorf("output not sorted: %#v (glue=%#v apex=%#v)", out, glue, apex)
			}
			if !hasNoDuplicates(out) {
				t.Errorf("output has duplicates: %#v (glue=%#v apex=%#v)", out, glue, apex)
			}
		})
	}
}

// TestMethod4and5PropertyAlwaysSortedAndDedupedByString runs 20 seeded
// scenarios and asserts Method4and5's output is sorted lexicographically by
// ns.String() and contains no duplicates by that same key.
func TestMethod4and5PropertyAlwaysSortedAndDedupedByString(t *testing.T) {
	for trial := 0; trial < 20; trial++ {
		seed := int64(trial * 13)
		rng := rand.New(rand.NewSource(seed))
		glue := randomNameSet(rng, 1+rng.Intn(4))

		nameserver.EmptyCache()

		ctx, prof, _ := testhelpers.Context(t)
		prof.Net.IPv4 = true
		prof.Net.IPv6 = true

		r := &recursor.Recursor{}
		if err := r.AddFakeAddresses(".", map[string][]string{"a.root": {"192.0.2.1"}}); err != nil {
			t.Fatalf("add root: %v", err)
		}
		fakes := map[string][]string{}
		for i, name := range glue {
			fakes[strings.ToLower(name)] = []string{fmt.Sprintf("192.0.2.%d", 30+i)}
		}
		if err := r.AddFakeAddresses("example.com", fakes); err != nil {
			t.Fatalf("add zone: %v", err)
		}
		for i, name := range glue {
			setNSHook(ctx, t, r, strings.ToLower(name), fmt.Sprintf("192.0.2.%d", 30+i), "example.com", glue...)
		}

		z, err := zone.NewWithRecursor("example.com", r)
		if err != nil {
			t.Fatalf("new zone: %v", err)
		}

		out, err := Method4and5(ctx, &z)
		if err != nil {
			t.Fatalf("method4and5: %v", err)
		}

		// Output sort key is ns.String() = name + "/" + address.
		strs := make([]string, len(out))
		for i, ns := range out {
			strs[i] = strings.ToLower(ns.String())
		}
		if !sort.StringsAreSorted(strs) {
			t.Errorf("trial seed=%d: output not sorted: %#v", seed, strs)
		}
		seen := map[string]bool{}
		for _, s := range strs {
			if seen[s] {
				t.Errorf("trial seed=%d: duplicate %q", seed, s)
			}
			seen[s] = true
		}

		nameserver.EmptyCache()
	}
	// Outer cleanup so the last trial's cache is wiped.
	t.Cleanup(nameserver.EmptyCache)
}
