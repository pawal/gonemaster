package dnssecchain

import (
	"slices"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
)

func TestCollectNSNameWithoutCollectorIsNoOp(t *testing.T) {
	ctx := t.Context()
	CollectNSName(ctx, NSName{Name: "ns.example.", Status: NSNameOrphan})
	if got := NSNamesFromContext(ctx); got != nil {
		t.Errorf("want nil without a collector, got %+v", got)
	}
}

func TestCollectNSNameWorstStatusWins(t *testing.T) {
	ctx := WithNSNames(t.Context())
	CollectNSName(ctx, NSName{Name: "a.ns.example.", Status: NSNameValidates, Signer: "ns.example.", Servers: []string{"192.0.2.1"}})
	CollectNSName(ctx, NSName{Name: "a.ns.example.", Status: NSNameOrphan, Signer: "a.ns.example.", Servers: []string{"192.0.2.2"}})
	CollectNSName(ctx, NSName{Name: "a.ns.example.", Status: NSNameValidates, Servers: []string{"192.0.2.3"}})

	got := NSNamesFromContext(ctx)
	if len(got) != 1 {
		t.Fatalf("want 1 entry, got %d", len(got))
	}
	if got[0].Status != NSNameOrphan {
		t.Errorf("status = %q, want %q", got[0].Status, NSNameOrphan)
	}
	if got[0].Signer != "a.ns.example." {
		t.Errorf("signer = %q, want the orphan apex", got[0].Signer)
	}
	// The servers of the losing status are not merged into the winner.
	if want := []string{"192.0.2.2"}; !slices.Equal(got[0].Servers, want) {
		t.Errorf("servers = %v, want %v", got[0].Servers, want)
	}
}

func TestCollectNSNameMergesEqualStatus(t *testing.T) {
	ctx := WithNSNames(t.Context())
	CollectNSName(ctx, NSName{Name: "a.ns.example.", Status: NSNameOrphan, Servers: []string{"192.0.2.2"}})
	CollectNSName(ctx, NSName{Name: "a.ns.example.", Status: NSNameOrphan, Signer: "a.ns.example.", Servers: []string{"192.0.2.1", "192.0.2.2"}})

	got := NSNamesFromContext(ctx)
	if len(got) != 1 {
		t.Fatalf("want 1 entry, got %d", len(got))
	}
	if want := []string{"192.0.2.1", "192.0.2.2"}; !slices.Equal(got[0].Servers, want) {
		t.Errorf("servers = %v, want the sorted union %v", got[0].Servers, want)
	}
	if got[0].Signer != "a.ns.example." {
		t.Errorf("signer = %q, want the one the later observation supplied", got[0].Signer)
	}
}

func TestNSNamesFromContextSortsByName(t *testing.T) {
	ctx := WithNSNames(t.Context())
	for _, name := range []string{"c.ns.example.", "a.ns.example.", "b.ns.example."} {
		CollectNSName(ctx, NSName{Name: name, Status: NSNameValidates, Servers: []string{"192.0.2.1"}})
	}
	var names []string
	for _, n := range NSNamesFromContext(ctx) {
		names = append(names, n.Name)
	}
	if want := []string{"a.ns.example.", "b.ns.example.", "c.ns.example."}; !slices.Equal(names, want) {
		t.Errorf("names = %v, want %v", names, want)
	}
}

func TestExtractCarriesCollectedNSNames(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	ctx = WithNSNames(ctx)
	CollectNSName(ctx, NSName{Name: "a.ns." + testZone, Status: NSNameOrphan, Signer: "a.ns." + testZone, Servers: []string{"192.0.2.1"}})
	CollectNSName(ctx, NSName{Name: "b.ns." + testZone, Status: NSNameValidates, Signer: "ns." + testZone, Servers: []string{"192.0.2.1"}})
	in := buildInput(t, ctx, fixtureOpts{})

	got := Extract(ctx, in)
	if got == nil {
		t.Fatal("expected a summary, got nil")
	}
	if len(got.NSNames) != 2 {
		t.Fatalf("want 2 ns_names entries, got %+v", got.NSNames)
	}
	if got.NSNames[0].Status != NSNameOrphan || got.NSNames[1].Status != NSNameValidates {
		t.Errorf("ns_names = %+v, want the orphan first by name", got.NSNames)
	}
	// The branch is reported beside the zone's own chain, which stays secure.
	if got.Status != StatusSecure {
		t.Errorf("status = %q, want %q", got.Status, StatusSecure)
	}
	if got.Truncated {
		t.Error("two names must not truncate")
	}
}

func TestExtractWithoutCollectorHasNoNSNames(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	in := buildInput(t, ctx, fixtureOpts{})

	got := Extract(ctx, in)
	if got == nil {
		t.Fatal("expected a summary, got nil")
	}
	if got.NSNames != nil {
		t.Errorf("want no ns_names section, got %+v", got.NSNames)
	}
}

func TestExtractCapsNSNames(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	ctx = WithNSNames(ctx)
	for i := range maxNSNames + 5 {
		CollectNSName(ctx, NSName{
			Name:    string(rune('a'+i%26)) + string(rune('a'+i/26)) + ".ns." + testZone,
			Status:  NSNameValidates,
			Servers: []string{"192.0.2.1"},
		})
	}
	in := buildInput(t, ctx, fixtureOpts{})

	got := Extract(ctx, in)
	if got == nil {
		t.Fatal("expected a summary, got nil")
	}
	if len(got.NSNames) != maxNSNames {
		t.Errorf("ns_names length = %d, want the cap %d", len(got.NSNames), maxNSNames)
	}
	if !got.Truncated {
		t.Error("hitting the cap must set truncated")
	}
}
