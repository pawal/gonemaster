package nsdiscovery

import (
	"fmt"
	"math/rand"
	"net/netip"
	"sort"
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/nstest"
	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

// Property-style tests: invariants that must hold across many input shapes.
// Each test runs several seeded scenarios; if any fails the seed is logged.

// runAllNSNamesProperty sets up an undelegated zone where the glue and the
// apex-reported NS sets may overlap or diverge. Returns AllNSNames's output
// as a string slice.
func runAllNSNamesProperty(t *testing.T, glueNames []string, apexNames []string) []string {
	t.Helper()

	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := nstest.Recursor(t, map[string]map[string][]string{".": map[string][]string{"a.root": {"192.0.2.1"}}})
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

	got, err := AllNSNames(ctx, &z)
	if err != nil {
		t.Fatalf("AllNSNames: %v", err)
	}
	out := make([]string, 0, len(got))
	for _, n := range got {
		out = append(out, n.String())
	}
	return out
}

// TestAllNSNamesPropertyAlwaysSortedAndDeduped runs 20 seeded scenarios with
// random glue and apex name sets and asserts the union output is always
// lowercase-sorted and contains no duplicates.
func TestAllNSNamesPropertyAlwaysSortedAndDeduped(t *testing.T) {
	for trial := 0; trial < 20; trial++ {
		seed := int64(trial * 31)
		rng := rand.New(rand.NewSource(seed))
		glue := nstest.RandomNameSet(rng, 1+rng.Intn(4), "example.com")
		apex := nstest.RandomNameSet(rng, 1+rng.Intn(4), "example.com")
		t.Run(fmt.Sprintf("seed=%d", seed), func(t *testing.T) {
			out := runAllNSNamesProperty(t, glue, apex)
			if !nstest.IsSortedLowercase(out) {
				t.Errorf("output not sorted: %#v (glue=%#v apex=%#v)", out, glue, apex)
			}
			if !nstest.HasNoDuplicates(out) {
				t.Errorf("output has duplicates: %#v (glue=%#v apex=%#v)", out, glue, apex)
			}
		})
	}
}

// TestAllNameserversPropertyAlwaysSortedAndDedupedByString runs 20 seeded
// scenarios and asserts AllNameservers's output is sorted lexicographically
// by ns.String() and contains no duplicates by that same key.
func TestAllNameserversPropertyAlwaysSortedAndDedupedByString(t *testing.T) {
	for trial := 0; trial < 20; trial++ {
		seed := int64(trial * 13)
		rng := rand.New(rand.NewSource(seed))
		glue := nstest.RandomNameSet(rng, 1+rng.Intn(4), "example.com")

		ctx, prof, _ := testhelpers.Context(t)
		prof.Net.IPv4 = true
		prof.Net.IPv6 = true

		r := nstest.Recursor(t, map[string]map[string][]string{".": map[string][]string{"a.root": {"192.0.2.1"}}})
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

		out, err := AllNameservers(ctx, &z)
		if err != nil {
			t.Fatalf("AllNameservers: %v", err)
		}

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
	}
}

// TestDelegationNameserversNoNilNamesInOutput runs several seeded scenarios
// with random nameserver-name sets and asserts that every returned NSItem
// has a non-empty Name.
func TestDelegationNameserversNoNilNamesInOutput(t *testing.T) {
	letters := []byte("abcdefghijklmnopqrstuvwxyz")
	for trial := 0; trial < 20; trial++ {
		seed := int64(trial * 11)
		rng := rand.New(rand.NewSource(seed))
		size := 1 + rng.Intn(5)

		ctx, prof, _ := testhelpers.Context(t)
		prof.Net.IPv4 = true
		prof.Net.IPv6 = true

		r := nstest.Recursor(t, map[string]map[string][]string{".": map[string][]string{"a.root": {"192.0.2.1"}}})
		glue := map[string][]string{}
		for i := 0; i < size; i++ {
			labelLen := 1 + rng.Intn(4)
			label := make([]byte, labelLen)
			for j := range label {
				label[j] = letters[rng.Intn(len(letters))]
			}
			name := string(label) + ".example.com"
			glue[name] = []string{fmt.Sprintf("192.0.2.%d", 50+i)}
		}
		if err := r.AddFakeAddresses("example.com", glue); err != nil {
			t.Fatalf("trial %d: add zone: %v", trial, err)
		}

		z, err := zone.NewWithRecursor("example.com", r)
		if err != nil {
			t.Fatalf("trial %d: new zone: %v", trial, err)
		}

		items, err := DelegationNameservers(ctx, &z)
		if err != nil {
			t.Fatalf("trial %d (seed=%d): %v", trial, seed, err)
		}
		for i, item := range items {
			if item.Name.String() == "" {
				t.Errorf("trial seed=%d item %d: empty Name", seed, i)
			}
		}
	}
}

// TestNSItemStringStableForSort verifies that NSItem.String() is a
// deterministic total order key suitable for sort.SliceStable.
func TestNSItemStringStableForSort(t *testing.T) {
	items := []NSItem{
		{Name: dnsname.New("b.example"), Address: netip.MustParseAddr("192.0.2.2"), HasAddress: true},
		{Name: dnsname.New("a.example"), Address: netip.MustParseAddr("192.0.2.1"), HasAddress: true},
		{Name: dnsname.New("c.example"), HasAddress: false},
		{Name: dnsname.New("a.example"), Address: netip.MustParseAddr("192.0.2.10"), HasAddress: true},
	}

	sortByString := func(in []NSItem) []NSItem {
		out := append([]NSItem(nil), in...)
		sort.SliceStable(out, func(i, j int) bool { return out[i].String() < out[j].String() })
		return out
	}

	first := sortByString(items)
	for trial := 0; trial < 20; trial++ {
		shuffled := append([]NSItem(nil), items...)
		shuffled = append(shuffled[trial%len(shuffled):], shuffled[:trial%len(shuffled)]...)
		got := sortByString(shuffled)
		if len(got) != len(first) {
			t.Fatalf("trial %d: length mismatch", trial)
		}
		for i := range got {
			if got[i].String() != first[i].String() {
				t.Fatalf("trial %d index %d: expected %q, got %q",
					trial, i, first[i].String(), got[i].String())
			}
		}
	}
}
