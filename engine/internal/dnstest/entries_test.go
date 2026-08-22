package dnstest

import (
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/internal/tbtest"
)

// entry builds a log entry directly so tests can pick module/testcase freely.
func entry(tag string, args map[string]any) *logger.Entry {
	return &logger.Entry{Tag: tag, Args: args, Testcase: "TEST01", Module: "Test"}
}

func TestHasTagAndEntryByTag(t *testing.T) {
	entries := []*logger.Entry{entry("A", nil), nil, entry("B", map[string]any{"ns": "ns1.test"})}

	if !HasTag(entries, "B") {
		t.Fatal("expected HasTag to find B")
	}
	if HasTag(entries, "MISSING") {
		t.Fatal("expected HasTag not to find MISSING")
	}
	if got := EntryByTag(entries, "B"); got == nil || got.Args["ns"] != "ns1.test" {
		t.Fatalf("unexpected entry for B: %#v", got)
	}
	if got := EntryByTag(entries, "MISSING"); got != nil {
		t.Fatalf("expected nil for a missing tag, got %#v", got)
	}
}

func TestTagsSkipsNilEntries(t *testing.T) {
	got := Tags([]*logger.Entry{entry("A", nil), nil, entry("B", nil)})
	if strings.Join(got, ",") != "A,B" {
		t.Fatalf("unexpected tags: %v", got)
	}
}

func TestSignatureRendersModuleTestcaseTagLevel(t *testing.T) {
	got := Signature([]*logger.Entry{entry("A", nil), nil})
	if len(got) != 1 {
		t.Fatalf("expected one signature line, got %v", got)
	}
	if !strings.HasPrefix(got[0], "Test/TEST01/A/") {
		t.Fatalf("unexpected signature: %q", got[0])
	}
}

func TestRequireEntryByTagAndStringArg(t *testing.T) {
	entries := []*logger.Entry{entry("A", map[string]any{"ns": "ns1.test", "n": 1})}

	got := RequireEntryByTag(t, entries, "A")
	if RequireStringArg(t, got, "ns") != "ns1.test" {
		t.Fatalf("unexpected ns arg: %#v", got.Args["ns"])
	}

	tbtest.MustFail(t, "expected a MISSING entry", func(tb *tbtest.TB) { RequireEntryByTag(tb, entries, "MISSING") })
	tbtest.MustFail(t, "expected string arg n", func(tb *tbtest.TB) { RequireStringArg(tb, got, "n") })
}

func TestAssertOnlyTag(t *testing.T) {
	entries := []*logger.Entry{entry("WANTED", nil), entry("ALSO", nil)}

	AssertOnlyTag(t, entries, "WANTED", []string{"NOT_FIRED"})

	tbtest.MustFail(t, "expected MISSING to fire", func(tb *tbtest.TB) {
		AssertOnlyTag(tb, entries, "MISSING", nil)
	})
	tbtest.MustFail(t, "expected ALSO not to fire alongside WANTED", func(tb *tbtest.TB) {
		AssertOnlyTag(tb, entries, "WANTED", []string{"ALSO"})
	})
}
