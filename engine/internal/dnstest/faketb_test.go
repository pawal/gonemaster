package dnstest

import (
	"fmt"
	"strings"
	"testing"
)

// errFatal aborts a fakeTB the way testing.T.Fatalf aborts a real test.
var errFatal = fmt.Errorf("fatal")

// fakeTB records the first Fatalf message instead of failing the test, so the
// helpers' failure paths can be asserted on.
type fakeTB struct {
	testing.TB
	msg string
}

func (f *fakeTB) Helper() {}

func (f *fakeTB) Fatalf(format string, args ...any) {
	f.msg = fmt.Sprintf(format, args...)
	panic(errFatal)
}

// mustFail runs fn with a fakeTB and checks the message it failed with.
func mustFail(t *testing.T, wantMsg string, fn func(tb testing.TB)) {
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
