package clitest

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/internal/tbtest"
	"codeberg.org/pawal/gonemaster/scoring"
)

// echo is a CLI shaped like the real ones: it prints its args on stdout, the
// first arg on stderr, and returns the length of the arg list.
func echo(args []string, out io.Writer, errOut io.Writer) int {
	fmt.Fprintf(out, "args: %s", strings.Join(args, " "))
	if len(args) > 0 {
		fmt.Fprintf(errOut, "first: %s", args[0])
	}
	return len(args)
}

func TestRunCapturesBothStreamsAndTheCode(t *testing.T) {
	res := Run(t, echo, "alpha", "beta")

	if res.Code != 2 {
		t.Fatalf("Code = %d, want 2", res.Code)
	}
	if res.Out != "args: alpha beta" {
		t.Fatalf("Out = %q", res.Out)
	}
	if res.Err != "first: alpha" {
		t.Fatalf("Err = %q", res.Err)
	}
}

func TestRunWithNoArgs(t *testing.T) {
	Run(t, echo).RequireCode(t, 0).RequireErrContains(t)
}

func TestRequireCodeReportsBothStreams(t *testing.T) {
	res := Run(t, echo, "alpha")
	tbtest.MustFail(t, "exit code = 1, want 0", func(tb *tbtest.TB) { res.RequireCode(tb, 0) })
	tbtest.MustFail(t, "first: alpha", func(tb *tbtest.TB) { res.RequireCode(tb, 0) })
}

func TestRequireContainsChecksEveryWant(t *testing.T) {
	res := Run(t, echo, "alpha", "beta")

	res.RequireOutContains(t, "alpha", "beta").RequireErrContains(t, "first: alpha")

	tbtest.MustFail(t, "stdout missing \"gamma\"", func(tb *tbtest.TB) {
		res.RequireOutContains(tb, "alpha", "gamma")
	})
	tbtest.MustFail(t, "stderr missing \"second\"", func(tb *tbtest.TB) {
		res.RequireErrContains(tb, "second")
	})
}

func TestRequireOutEmpty(t *testing.T) {
	silent := func([]string, io.Writer, io.Writer) int { return 0 }
	Run(t, silent).RequireOutEmpty(t)

	res := Run(t, echo, "alpha")
	tbtest.MustFail(t, "expected no stdout", func(tb *tbtest.TB) { res.RequireOutEmpty(tb) })
}

func TestRequireNoControlBytes(t *testing.T) {
	RequireNoControlBytes(t, []byte("plain ascii café 日本語"))
	RequireNoControlBytes(t, []byte("line1\nline2"), '\n')

	for _, tc := range []struct {
		name string
		in   []byte
	}{
		{"escape", []byte("\x1b[34mblue")},
		{"carriage return", []byte("real\rfake")},
		{"del", []byte("x\x7fy")},
		{"c1 rune", []byte("x\u009by")},
		{"invalid utf-8", []byte{'x', 0x9b, 'y'}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tbtest.MustFail(t, "reached the terminal", func(tb *tbtest.TB) {
				RequireNoControlBytes(tb, tc.in)
			})
		})
	}
}

func TestFileContains(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	FileContains(t, path, "alpha", "beta")

	// Every miss is reported, not just the first.
	tb := &tbtest.TB{}
	FileContains(tb, path, "gamma", "delta")
	if len(tb.Errs) != 2 {
		t.Fatalf("errors = %v, want one per missing fragment", tb.Errs)
	}

	tbtest.MustFail(t, "read", func(tb *tbtest.TB) { FileContains(tb, path+".missing", "alpha") })
}

func TestWriteScoringConfigRoundTrips(t *testing.T) {
	path := WriteScoringConfig(t)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var got scoring.Config
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := scoring.DefaultConfig()
	if len(got.SeverityPenalties) != len(want.SeverityPenalties) {
		t.Fatalf("severity penalties = %d, want %d", len(got.SeverityPenalties), len(want.SeverityPenalties))
	}
	if len(got.CategoryWeights) != len(want.CategoryWeights) {
		t.Fatalf("category weights = %d, want %d", len(got.CategoryWeights), len(want.CategoryWeights))
	}
}
