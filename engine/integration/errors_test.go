package integration

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/profile"
)

func TestErrorFor(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		code    string
		message string
	}{
		{"typed", &Error{Code: CodeUnknownModule, Message: "m"}, CodeUnknownModule, "m"},
		{"wrapped typed", fmt.Errorf("run: %w", &Error{Code: CodeInvalidDomain, Message: "d"}), CodeInvalidDomain, "d"},
		{"panic", &engine.PanicError{Value: "boom"}, CodePanic, "panic: boom"},
		{"canceled", context.Canceled, CodeCanceled, "context canceled"},
		{"deadline", context.DeadlineExceeded, CodeCanceled, "context deadline exceeded"},
		{"not implemented", fmt.Errorf("unknown testcase %q: %w", "x", engine.ErrNotImplemented), CodeUnknownTestcase, `unknown testcase "x": engine not implemented`},
		{"profile", &profile.PropertyError{Path: "net", Unknown: true, Err: errors.New("unknown property \"net\"")}, CodeInvalidProfile, `unknown property "net"`},
		{"other", errors.New("disk full"), CodeRunFailed, "disk full"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := errorFor(tc.err)
			if got.Code != tc.code || got.Message != tc.message {
				t.Fatalf("errorFor = %+v, want {%s %s}", got, tc.code, tc.message)
			}
		})
	}
}

func TestErrorString(t *testing.T) {
	if got := (&Error{Code: CodeRunFailed, Message: "x"}).Error(); got != "run_failed: x" {
		t.Fatalf("Error() = %q", got)
	}
}
