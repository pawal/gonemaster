package util

import (
	"context"
	"math/rand"
	"net/netip"
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/constants"
	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/profile"
)

func TestParseHints(t *testing.T) {
	hintsText := strings.Join([]string{
		". 3600 IN NS a.root.",
		"a.root. 3600 IN A 192.0.2.1",
		"a.root. 3600 IN AAAA 2001:db8::1",
	}, "\n")

	hints, err := ParseHints(hintsText)
	if err != nil {
		t.Fatalf("parse hints: %v", err)
	}
	addrs := hints["a.root."]
	if len(addrs) != 2 {
		t.Fatalf("expected 2 addresses, got %d", len(addrs))
	}
}

func TestParseHintsRejectsDirective(t *testing.T) {
	if _, err := ParseHints("$TTL 3600\n."); err == nil {
		t.Fatalf("expected error for forbidden directive")
	}
}

func TestParseHintsMissingGlue(t *testing.T) {
	_, err := ParseHints(". 3600 IN NS a.root.")
	if err == nil {
		t.Fatalf("expected error for missing glue")
	}
}

func TestIPVersion(t *testing.T) {
	if IPVersion(netip.MustParseAddr("192.0.2.1")) != 4 {
		t.Fatalf("expected IPv4 version")
	}
	if IPVersion(netip.MustParseAddr("2001:db8::1")) != 6 {
		t.Fatalf("expected IPv6 version")
	}
}

func TestSerialGT(t *testing.T) {
	if SerialGT(1, 2) {
		t.Fatalf("expected 1 not greater than 2")
	}
	if !SerialGT(0, 0x80000001) {
		t.Fatalf("expected wrap-around comparison to be true")
	}
}

func TestScrambleCasePreservesLetters(t *testing.T) {
	scrambleRand = rand.New(rand.NewSource(1))
	input := "AbCdEf123"
	out := ScrambleCase(input)
	if len(out) != len(input) {
		t.Fatalf("unexpected length: %d", len(out))
	}
	if !strings.EqualFold(out, input) {
		t.Fatalf("expected same letters ignoring case, got %q", out)
	}
}

func TestInfoAddsEntry(t *testing.T) {
	log := logger.New()
	ctx := logger.WithContext(context.Background(), log)

	entry, err := Info(ctx, "TEST_TAG", map[string]any{"value": "ok"})
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	if entry == nil {
		t.Fatalf("expected entry")
	}
	entries := log.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Tag != "TEST_TAG" {
		t.Fatalf("unexpected tag %q", entries[0].Tag)
	}
}

func TestNSNameAndZoneHelpers(t *testing.T) {

	ns, err := NS("ns1.example", "192.0.2.1")
	if err != nil {
		t.Fatalf("ns helper: %v", err)
	}
	if ns.Name.String() != "ns1.example" {
		t.Fatalf("unexpected ns name %q", ns.Name.String())
	}
	if ns.Address.String() != "192.0.2.1" {
		t.Fatalf("unexpected ns address %q", ns.Address.String())
	}

	name := Name("Example.COM.")
	if name.String() != "Example.COM" {
		t.Fatalf("unexpected name %q", name.String())
	}

	z, err := Zone("example")
	if err != nil {
		t.Fatalf("zone helper: %v", err)
	}
	if z.Name.String() != "example" {
		t.Fatalf("unexpected zone name %q", z.Name.String())
	}
}

func TestShouldRunTest(t *testing.T) {
	prof := testhelpers.DefaultProfile(t)
	prof.TestCases = []any{"alpha", "beta"}
	ctx := profile.WithContext(context.Background(), prof)

	if !ShouldRunTest(ctx, "alpha") {
		t.Fatalf("expected alpha to be enabled")
	}
	if ShouldRunTest(ctx, "gamma") {
		t.Fatalf("expected gamma to be disabled")
	}
	if ShouldRunTest(ctx, "") {
		t.Fatalf("expected empty test name to be disabled")
	}
}

func TestIPVersionOK(t *testing.T) {
	prof := testhelpers.DefaultProfile(t)
	prof.Net.IPv4 = false
	prof.Net.IPv6 = true
	ctx := profile.WithContext(context.Background(), prof)

	if IPVersionOK(ctx, constants.IPVersion4) {
		t.Fatalf("expected IPv4 to be disabled")
	}
	if !IPVersionOK(ctx, constants.IPVersion6) {
		t.Fatalf("expected IPv6 to be enabled")
	}
	if IPVersionOK(ctx, 0) {
		t.Fatalf("expected unknown version to be disabled")
	}
}

func TestTestLevelsReturnsCopy(t *testing.T) {
	prof := testhelpers.DefaultProfile(t)
	prof.TestLevels = map[string]map[string]string{
		"MODULE": {"TAG": "WARNING"},
	}
	ctx := profile.WithContext(context.Background(), prof)

	levels := TestLevels(ctx)
	if levels == nil || levels["MODULE"]["TAG"] != "WARNING" {
		t.Fatalf("unexpected test levels: %#v", levels)
	}

	levels["MODULE"]["TAG"] = "INFO"
	if prof.TestLevels["MODULE"]["TAG"] != "WARNING" {
		t.Fatalf("expected effective profile to remain unchanged")
	}
}
