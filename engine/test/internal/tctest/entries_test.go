package tctest

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/profile"
)

// errFatal aborts a fakeTB the way testing.T.Fatalf aborts a real test.
var errFatal = fmt.Errorf("fatal")

// fakeTB records the first Fatalf message instead of failing the test, so the
// helpers' failure paths can be asserted on.
type fakeTB struct {
	msg string
}

func (f *fakeTB) Helper() {}

func (f *fakeTB) Fatalf(format string, args ...any) {
	f.msg = fmt.Sprintf(format, args...)
	panic(errFatal)
}

// mustFail runs fn with a fakeTB and checks the message it failed with.
func mustFail(t *testing.T, wantMsg string, fn func(tb TB)) {
	t.Helper()
	tb := &fakeTB{}
	func() {
		defer func() {
			if r := recover(); r != errFatal {
				panic(r)
			}
		}()
		fn(tb)
		t.Fatalf("expected the helper to fail, got no failure")
	}()
	if wantMsg != "" && !strings.Contains(tb.msg, wantMsg) {
		t.Fatalf("expected failure mentioning %q, got %q", wantMsg, tb.msg)
	}
}

// entry builds a log entry directly so tests can pick module/testcase freely.
func entry(tag string, args map[string]any) *logger.Entry {
	return &logger.Entry{Tag: tag, Args: args, Testcase: "TEST01", Module: "Test"}
}

func TestHasCountFirstAll(t *testing.T) {
	entries := []*logger.Entry{
		entry("A", nil),
		nil,
		entry("B", map[string]any{"n": 1}),
		entry("B", map[string]any{"n": 2}),
	}

	if !Has(entries, "A") {
		t.Fatalf("expected Has to find A")
	}
	if Has(entries, "C") {
		t.Fatalf("did not expect Has to find C")
	}
	if got := Count(entries, "B"); got != 2 {
		t.Fatalf("expected 2 B entries, got %d", got)
	}
	if got := Count(entries, "C"); got != 0 {
		t.Fatalf("expected 0 C entries, got %d", got)
	}

	first := First(entries, "B")
	if first == nil || first.Args["n"] != 1 {
		t.Fatalf("expected the first B entry, got %#v", first)
	}
	if First(entries, "C") != nil {
		t.Fatalf("expected nil for a missing tag")
	}
	if got := All(entries, "B"); len(got) != 2 {
		t.Fatalf("expected 2 B entries, got %d", len(got))
	}
	if All(entries, "C") != nil {
		t.Fatalf("expected nil for a missing tag")
	}
}

func TestTagsSkipsNilEntries(t *testing.T) {
	entries := []*logger.Entry{entry("A", nil), nil, entry("DS21_B", nil)}

	if got, want := Tags(entries), []string{"A", "DS21_B"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	if got, want := TagsWithPrefix(entries, "DS21_"), []string{"DS21_B"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	if got := TagsWithPrefix(entries, "NONE_"); len(got) != 0 {
		t.Fatalf("expected no tags, got %v", got)
	}
}

func TestRequireHelpersPass(t *testing.T) {
	entries := []*logger.Entry{entry("A", nil), entry("B", nil), entry("B", nil)}

	if got := RequireTag(t, entries, "A"); got == nil || got.Tag != "A" {
		t.Fatalf("expected the A entry, got %#v", got)
	}
	RequireTags(t, entries, "A", "B")
	RequireNoTag(t, entries, "C", "D")
	RequireCount(t, entries, "B", 2)
	RequireCount(t, entries, "C", 0)
}

func TestRequireHelpersFail(t *testing.T) {
	entries := []*logger.Entry{entry("A", nil)}

	t.Run("missing tag", func(t *testing.T) {
		mustFail(t, "expected B", func(tb TB) { RequireTag(tb, entries, "B") })
	})
	t.Run("missing tag in list", func(t *testing.T) {
		mustFail(t, "expected B", func(tb TB) { RequireTags(tb, entries, "A", "B") })
	})
	t.Run("unwanted tag", func(t *testing.T) {
		mustFail(t, "did not expect A", func(tb TB) { RequireNoTag(tb, entries, "A") })
	})
	t.Run("wrong count", func(t *testing.T) {
		mustFail(t, "expected 2 A entries", func(tb TB) { RequireCount(tb, entries, "A", 2) })
	})
	t.Run("no entries at all", func(t *testing.T) {
		mustFail(t, "expected A", func(tb TB) { RequireTag(tb, nil, "A") })
	})
}

func TestNormalize(t *testing.T) {
	entries := []*logger.Entry{
		entry("A", nil),
		nil,
		entry("B", map[string]any{"ns": "ns1.example"}),
	}

	want := []string{"Test:TEST01:A", "Test:TEST01:B ns=ns1.example"}
	if got := Normalize(entries); !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestNormalizeStableDropsSystemDebugOnly(t *testing.T) {
	log := logger.New()
	log.SetProfile(&profile.Profile{TestLevels: map[string]map[string]string{
		"SYSTEM": {"SYSTEM_NOTICE": "NOTICE"},
	}})
	systemDebug := addEntry(t, log, "system_debug", "System", "Unspecified")
	systemNotice := addEntry(t, log, "system_notice", "System", "Unspecified")
	testcaseDebug := addEntry(t, log, "testcase_debug", "Test", "TEST01")

	entries := []*logger.Entry{systemDebug, systemNotice, testcaseDebug}
	if got := len(Normalize(entries)); got != 3 {
		t.Fatalf("expected Normalize to keep all three entries, got %d", got)
	}

	stable := NormalizeStable(entries)
	want := []string{"System:Unspecified:SYSTEM_NOTICE", "Test:TEST01:TESTCASE_DEBUG"}
	if !reflect.DeepEqual(stable, want) {
		t.Fatalf("expected %v, got %v", want, stable)
	}
}

func addEntry(t *testing.T, log *logger.Logger, tag string, module string, testcase string) *logger.Entry {
	t.Helper()
	e, err := log.Add(tag, nil, module, testcase)
	if err != nil {
		t.Fatalf("add %s: %v", tag, err)
	}
	return e
}
