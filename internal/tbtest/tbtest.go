package tbtest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// ErrFatal is what TB panics with when a helper calls Fatalf.
var ErrFatal = errors.New("tbtest: fatal")

// TB records the first Fatalf message and panics with ErrFatal, the way
// testing.T.Fatalf aborts a real test. Methods a helper does not call are left
// to the embedded nil testing.TB and panic if reached.
type TB struct {
	testing.TB
	Msg      string
	Errs     []string
	cleanups []func()
}

// Helper is a no-op.
func (f *TB) Helper() {}

// Context stands in for t.Context().
func (f *TB) Context() context.Context { return context.Background() }

// Cleanup collects the functions a helper registers, for RunCleanups.
func (f *TB) Cleanup(fn func()) { f.cleanups = append(f.cleanups, fn) }

// RunCleanups runs the collected cleanups in reverse registration order.
func (f *TB) RunCleanups() {
	for i := len(f.cleanups) - 1; i >= 0; i-- {
		f.cleanups[i]()
	}
	f.cleanups = nil
}

// Fatalf records the message and aborts with ErrFatal.
func (f *TB) Fatalf(format string, args ...any) {
	f.Msg = fmt.Sprintf(format, args...)
	panic(ErrFatal)
}

// Errorf records the message without aborting, so a helper that reports with
// Errorf rather than Fatalf can be told apart.
func (f *TB) Errorf(format string, args ...any) {
	f.Errs = append(f.Errs, fmt.Sprintf(format, args...))
}

// MustFail runs fn with a fresh TB and checks it failed with a message
// containing wantMsg. An empty wantMsg accepts any failure.
func MustFail(t *testing.T, wantMsg string, fn func(tb *TB)) {
	t.Helper()
	tb := &TB{}
	func() {
		defer func() {
			// A nil recover means fn returned and the t.Fatalf below is
			// unwinding this goroutine; anything else is a real panic.
			if r := recover(); r != nil && r != ErrFatal {
				panic(r)
			}
		}()
		fn(tb)
		t.Fatalf("expected the helper to fail, got no failure")
	}()
	if wantMsg != "" && !strings.Contains(tb.Msg, wantMsg) {
		t.Fatalf("expected failure mentioning %q, got %q", wantMsg, tb.Msg)
	}
}
