package tbtest

import (
	"strings"
	"testing"
)

func TestFatalfRecordsTheMessageAndPanics(t *testing.T) {
	tb := &TB{}
	func() {
		defer func() {
			if r := recover(); r != ErrFatal {
				t.Fatalf("expected a panic with ErrFatal, got %v", r)
			}
		}()
		tb.Fatalf("add %s: %d", "thing", 7)
		t.Fatal("expected Fatalf not to return")
	}()
	if tb.Msg != "add thing: 7" {
		t.Fatalf("expected the formatted message, got %q", tb.Msg)
	}
}

func TestCleanupsRunInReverseOrder(t *testing.T) {
	tb := &TB{}
	var order []string
	tb.Cleanup(func() { order = append(order, "first") })
	tb.Cleanup(func() { order = append(order, "second") })

	tb.RunCleanups()
	if strings.Join(order, ",") != "second,first" {
		t.Fatalf("expected reverse registration order, got %v", order)
	}

	// A second call must not re-run what it already ran.
	tb.RunCleanups()
	if len(order) != 2 {
		t.Fatalf("expected the cleanups to be dropped after running, got %v", order)
	}
}

func TestErrorfRecordsWithoutAborting(t *testing.T) {
	tb := &TB{}
	tb.Errorf("code = %q, want %q", "other", "not_found")
	tb.Errorf("second")

	if len(tb.Errs) != 2 {
		t.Fatalf("expected two recorded errors, got %v", tb.Errs)
	}
	if tb.Errs[0] != `code = "other", want "not_found"` {
		t.Fatalf("expected the formatted message, got %q", tb.Errs[0])
	}
	// Errorf must not look like a Fatalf.
	if tb.Msg != "" {
		t.Fatalf("expected Msg untouched, got %q", tb.Msg)
	}
}

func TestContextIsUsable(t *testing.T) {
	if (&TB{}).Context() == nil {
		t.Fatal("expected a non-nil context")
	}
}

func TestMustFailAcceptsAMatchingFailure(t *testing.T) {
	MustFail(t, "boom", func(tb *TB) { tb.Fatalf("boom: %v", 1) })
	// An empty want accepts any failure.
	MustFail(t, "", func(tb *TB) { tb.Fatalf("anything") })
}

func TestMustFailRepanicsOnAnUnrelatedPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != "unrelated" {
			t.Fatalf("expected the unrelated panic to propagate, got %v", r)
		}
	}()
	MustFail(t, "boom", func(tb *TB) { panic("unrelated") })
}
