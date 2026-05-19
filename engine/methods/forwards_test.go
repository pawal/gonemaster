package methods

import (
	"context"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

// Forwarding parity tests: each semantic-name function must return the same
// result as its deprecated MethodN counterpart on a shared fixture. These
// tests survive any future flip of the forwarding direction since they
// only assert that both names produce equivalent output.

// fwdFixture builds a fully-populated undelegated zone with two glue NSes
// reachable via fake addresses. Returns the shared context so callers reuse
// the same nameserver cache (otherwise the hooked Nameservers would not be
// found by zone.Glue's internal lookups and queries would time out).
func fwdFixture(t *testing.T) (context.Context, *zone.Zone) {
	t.Helper()
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)

	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"a.root": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add root: %v", err)
	}
	if err := r.AddFakeAddresses("example.com", map[string][]string{
		"ns1.example.com": {"192.0.2.11"},
		"ns2.example.com": {"192.0.2.12"},
	}); err != nil {
		t.Fatalf("add zone: %v", err)
	}
	setNSHook(ctx, t, r, "ns1.example.com", "192.0.2.11", "example.com", "ns1.example.com", "ns2.example.com")
	setNSHook(ctx, t, r, "ns2.example.com", "192.0.2.12", "example.com", "ns1.example.com", "ns2.example.com")

	z, err := zone.NewWithRecursor("example.com", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}
	return ctx, &z
}

// TestParentZoneEqualsMethod1 verifies forwarding via the root zone, which
// avoids the parent-chain walk that would time out in an undelegated test
// fixture. ParentZone is a one-line forward to Method1.
func TestParentZoneEqualsMethod1(t *testing.T) {
	rootZone := zone.Zone{}
	rootZone, err := zone.New(".")
	if err != nil {
		t.Fatalf("new root zone: %v", err)
	}

	a, errA := ParentZone(context.Background(), &rootZone)
	b, errB := Method1(context.Background(), &rootZone)
	if (errA == nil) != (errB == nil) {
		t.Fatalf("error parity broken: %v vs %v", errA, errB)
	}
	if (a == nil) != (b == nil) {
		t.Fatalf("nil parity broken: %v vs %v", a, b)
	}
	if a != nil && a.Name.String() != b.Name.String() {
		t.Fatalf("parent name mismatch: %s vs %s", a.Name.String(), b.Name.String())
	}

	// Nil zone must produce the same error on both sides.
	if _, errA := ParentZone(context.Background(), nil); errA == nil {
		t.Fatalf("ParentZone(nil) should error")
	}
	if _, errB := Method1(context.Background(), nil); errB == nil {
		t.Fatalf("Method1(nil) should error")
	}
}

func TestGlueNamesEqualsMethod2(t *testing.T) {
	ctx, z := fwdFixture(t)

	a, errA := GlueNames(ctx, z)
	b, errB := Method2(ctx, z)
	if (errA == nil) != (errB == nil) {
		t.Fatalf("error parity broken: %v vs %v", errA, errB)
	}
	if len(a) != len(b) {
		t.Fatalf("length differs: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].String() != b[i].String() {
			t.Fatalf("element %d differs: %s vs %s", i, a[i].String(), b[i].String())
		}
	}
}

func TestApexNSNamesEqualsMethod3(t *testing.T) {
	ctx, z := fwdFixture(t)

	a, errA := ApexNSNames(ctx, z)
	b, errB := Method3(ctx, z)
	if (errA == nil) != (errB == nil) {
		t.Fatalf("error parity broken: %v vs %v", errA, errB)
	}
	if len(a) != len(b) {
		t.Fatalf("length differs: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].String() != b[i].String() {
			t.Fatalf("element %d differs: %s vs %s", i, a[i].String(), b[i].String())
		}
	}
}

func TestGlueNameserversEqualsMethod4(t *testing.T) {
	ctx, z := fwdFixture(t)

	a, errA := GlueNameservers(ctx, z)
	b, errB := Method4(ctx, z)
	if (errA == nil) != (errB == nil) {
		t.Fatalf("error parity broken: %v vs %v", errA, errB)
	}
	if len(a) != len(b) {
		t.Fatalf("length differs: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].String() != b[i].String() {
			t.Fatalf("element %d differs: %s vs %s", i, a[i].String(), b[i].String())
		}
	}
}

func TestApexNameserversEqualsMethod5(t *testing.T) {
	ctx, z := fwdFixture(t)

	a, errA := ApexNameservers(ctx, z)
	b, errB := Method5(ctx, z)
	if (errA == nil) != (errB == nil) {
		t.Fatalf("error parity broken: %v vs %v", errA, errB)
	}
	if len(a) != len(b) {
		t.Fatalf("length differs: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].String() != b[i].String() {
			t.Fatalf("element %d differs: %s vs %s", i, a[i].String(), b[i].String())
		}
	}
}

func TestAllNSNamesEqualsMethod2and3(t *testing.T) {
	ctx, z := fwdFixture(t)

	a, errA := AllNSNames(ctx, z)
	b, errB := Method2and3(ctx, z)
	if (errA == nil) != (errB == nil) {
		t.Fatalf("error parity broken: %v vs %v", errA, errB)
	}
	if len(a) != len(b) {
		t.Fatalf("length differs: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].String() != b[i].String() {
			t.Fatalf("element %d differs: %s vs %s", i, a[i].String(), b[i].String())
		}
	}
}

func TestAllNameserversEqualsMethod4and5(t *testing.T) {
	ctx, z := fwdFixture(t)

	a, errA := AllNameservers(ctx, z)
	b, errB := Method4and5(ctx, z)
	if (errA == nil) != (errB == nil) {
		t.Fatalf("error parity broken: %v vs %v", errA, errB)
	}
	if len(a) != len(b) {
		t.Fatalf("length differs: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].String() != b[i].String() {
			t.Fatalf("element %d differs: %s vs %s", i, a[i].String(), b[i].String())
		}
	}
}
