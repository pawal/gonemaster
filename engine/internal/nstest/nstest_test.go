package nstest

import (
	"context"
	"math/rand"
	"strings"
	"testing"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/internal/tbtest"
)

func TestRecursorAddsFakeAddressesPerZone(t *testing.T) {
	r := Recursor(t, map[string]map[string][]string{
		".":            {"a.root": {"192.0.2.1"}},
		"example.test": {"ns1.example.test": {"192.0.2.11"}},
	})
	if r.Client() == nil {
		t.Fatal("expected the recursor to expose a client")
	}
}

func TestRecursorFailsOnABadAddress(t *testing.T) {
	tbtest.MustFail(t, "add fake addresses for .", func(tb *tbtest.TB) {
		Recursor(tb, map[string]map[string][]string{".": {"a.root": {"not-an-ip"}}})
	})
}

func TestRootRecursorSeedsTheRootOnly(t *testing.T) {
	r := RootRecursor(t, map[string][]string{"a.root": {"192.0.2.1"}})
	if r == nil {
		t.Fatal("expected a recursor")
	}
}

func TestHintedRecursorSeedsTheRootHints(t *testing.T) {
	fakes := map[string]map[string][]string{"example.test": {"ns1.example.test": {"192.0.2.11"}}}

	// The root hints are the whole difference between the two constructors.
	if r := Recursor(t, fakes); r.HasFakeAddresses(".") {
		t.Fatal("expected Recursor to leave the root empty")
	}
	r := HintedRecursor(t, fakes)
	if !r.HasFakeAddresses(".") {
		t.Fatal("expected HintedRecursor to seed the root hints")
	}
	if !r.HasFakeAddresses("example.test") {
		t.Fatal("expected HintedRecursor to add the given fake addresses")
	}
}

func TestHintedRecursorFailsOnABadAddress(t *testing.T) {
	tbtest.MustFail(t, "add fake addresses for example.test", func(tb *tbtest.TB) {
		HintedRecursor(tb, map[string]map[string][]string{"example.test": {"ns1.example.test": {"nope"}}})
	})
}

func TestHookedNSAnswersThroughTheHook(t *testing.T) {
	ctx := nameserver.WithCache(context.Background(), nameserver.NewCacheStore())
	r := RootRecursor(t, map[string][]string{"a.root": {"192.0.2.1"}})

	want := dnstest.Response(dnstest.Answers(dnstest.ARR("example.test", "192.0.2.9")))
	ns := HookedNS(t, ctx, r, "a.root", "192.0.2.1", PacketHook(want))

	got, err := ns.QueryWithOptions(ctx, "example.test", "A", nil)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(got.Msg.Answer) != 1 {
		t.Fatalf("expected the hook's answer, got %#v", got.Msg)
	}
}

func TestNSWithoutARecursorHasNoClient(t *testing.T) {
	ctx := nameserver.WithCache(context.Background(), nameserver.NewCacheStore())
	if ns := NS(t, ctx, nil, "ns1.example.test", "192.0.2.1"); ns.Name.String() != "ns1.example.test" {
		t.Fatalf("unexpected nameserver name: %s", ns.Name)
	}
}

func TestAnswerHookMatchesNameAndTypeCaseInsensitively(t *testing.T) {
	want := dnstest.Response(dnstest.Answers(dnstest.NSRR("example.test", "ns1.example.test")))
	hook := AnswerHook("example.test", "NS", want)

	got, err := hook(context.Background(), "EXAMPLE.TEST", "ns", "IN", nil)
	if err != nil {
		t.Fatalf("hook: %v", err)
	}
	if len(got.Msg.Answer) != 1 {
		t.Fatalf("expected the canned answer for a case-different match, got %#v", got.Msg)
	}

	// A server that holds no such data answers empty rather than failing.
	if got, err = hook(context.Background(), "other.test", "NS", "IN", nil); err != nil || got.Msg != nil {
		t.Fatalf("expected an empty packet for another name, got %#v / %v", got, err)
	}
	if got, err = hook(context.Background(), "example.test", "SOA", "IN", nil); err != nil || got.Msg != nil {
		t.Fatalf("expected an empty packet for another type, got %#v / %v", got, err)
	}
}

func TestPacketHookAnswersEverything(t *testing.T) {
	want := dnstest.Response(dnstest.Rcode(dns.RcodeServerFailure))
	hook := PacketHook(want)
	for _, qtype := range []string{"A", "NS", "SOA"} {
		got, err := hook(context.Background(), "anything.test", qtype, "IN", nil)
		if err != nil || got.Msg.Rcode != dns.RcodeServerFailure {
			t.Fatalf("expected the canned packet for %s, got %#v / %v", qtype, got, err)
		}
	}
}

func TestRandomNameSetShapeAndDeterminism(t *testing.T) {
	first := RandomNameSet(rand.New(rand.NewSource(7)), 5, "example.com")
	if len(first) != 5 {
		t.Fatalf("expected 5 names, got %d", len(first))
	}
	for _, name := range first {
		if !strings.HasSuffix(name, ".example.com") {
			t.Fatalf("expected every name under the suffix, got %q", name)
		}
	}
	// Same seed, same names: a failing property trial must be reproducible.
	second := RandomNameSet(rand.New(rand.NewSource(7)), 5, "example.com")
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("expected the same seed to produce the same names, got %q vs %q", first[i], second[i])
		}
	}
}

func TestIsSortedLowercase(t *testing.T) {
	if !IsSortedLowercase([]string{"a.test", "B.test", "c.test"}) {
		t.Fatal("expected case-insensitive sorted order to be accepted")
	}
	if IsSortedLowercase([]string{"c.test", "a.test"}) {
		t.Fatal("expected unsorted input to be rejected")
	}
}

func TestHasNoDuplicates(t *testing.T) {
	if !HasNoDuplicates([]string{"a.test", "b.test"}) {
		t.Fatal("expected distinct names to be accepted")
	}
	if HasNoDuplicates([]string{"a.test", "A.test"}) {
		t.Fatal("expected a case-different duplicate to be rejected")
	}
}
