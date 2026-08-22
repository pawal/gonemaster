package clitest

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"codeberg.org/pawal/gonemaster/scoring"
)

// RunFunc is the entry point shape every gonemaster CLI shares.
type RunFunc func(args []string, out io.Writer, errOut io.Writer) int

// Result is one CLI invocation: its exit code and what it wrote.
type Result struct {
	Code int
	Out  string
	Err  string
}

// Run calls runFn with args and captures both streams.
func Run(t testing.TB, runFn RunFunc, args ...string) Result {
	t.Helper()
	var out, errOut bytes.Buffer
	code := runFn(args, &out, &errOut)
	return Result{Code: code, Out: out.String(), Err: errOut.String()}
}

// RequireCode fails unless the exit code matches, reporting both streams.
func (r Result) RequireCode(t testing.TB, want int) Result {
	t.Helper()
	if r.Code != want {
		t.Fatalf("exit code = %d, want %d (stdout=%q stderr=%q)", r.Code, want, r.Out, r.Err)
	}
	return r
}

// RequireOutContains fails unless stdout contains every want.
func (r Result) RequireOutContains(t testing.TB, want ...string) Result {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(r.Out, w) {
			t.Fatalf("stdout missing %q: %s", w, r.Out)
		}
	}
	return r
}

// RequireErrContains fails unless stderr contains every want.
func (r Result) RequireErrContains(t testing.TB, want ...string) Result {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(r.Err, w) {
			t.Fatalf("stderr missing %q: %s", w, r.Err)
		}
	}
	return r
}

// RequireOutEmpty fails unless stdout is empty.
func (r Result) RequireOutEmpty(t testing.TB) Result {
	t.Helper()
	if r.Out != "" {
		t.Fatalf("expected no stdout, got %q", r.Out)
	}
	return r
}

// RequireNoControlBytes fails if b carries anything cliterm.Sanitize escapes -
// a C0, DEL or C1 rune, or a bare invalid-UTF-8 byte - other than the runes in
// allow. It decodes runes rather than scanning bytes, so UTF-8 continuation
// bytes in the C1 range (any CJK text) are not mistaken for controls.
func RequireNoControlBytes(t testing.TB, b []byte, allow ...rune) {
	t.Helper()
	for i := 0; i < len(b); {
		r, size := utf8.DecodeRune(b[i:])
		if r == utf8.RuneError && size == 1 {
			t.Fatalf("invalid UTF-8 byte %#x reached the terminal: %q", b[i], b)
		}
		if (r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f)) && !slices.Contains(allow, r) {
			t.Fatalf("control rune %#x reached the terminal: %q", r, b)
		}
		i += size
	}
}

// FileContains reports every want the file at path is missing, so a docs or
// config check names all the gaps in one run rather than the first.
func FileContains(t testing.TB, path string, want ...string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	for _, w := range want {
		if !strings.Contains(string(data), w) {
			t.Errorf("%s missing %q", path, w)
		}
	}
}

// WriteScoringConfig writes the default scoring config to a temp file and
// returns its path, for the --scoring-config flag tests.
func WriteScoringConfig(t testing.TB) string {
	t.Helper()
	data, err := json.Marshal(scoring.DefaultConfig())
	if err != nil {
		t.Fatalf("marshal scoring config: %v", err)
	}
	path := filepath.Join(t.TempDir(), "scoring.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write scoring config: %v", err)
	}
	return path
}
