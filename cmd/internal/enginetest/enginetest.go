package enginetest

import (
	"testing"

	"codeberg.org/pawal/gonemaster/engine"
)

// RunFunc aliases the seam's type, so *RunFunc accepts a &runEngine of the
// binary's own unnamed function type.
type RunFunc = func(engine.RunRequest) ([]engine.LogEntry, error)

// Stub swaps *seam for fn and restores it when the test ends.
func Stub(t testing.TB, seam *RunFunc, fn RunFunc) {
	t.Helper()
	previous := *seam
	*seam = fn
	t.Cleanup(func() { *seam = previous })
}

// Capture stubs the seam to record the request it was handed and report
// nothing. A nil captured just swallows the run.
func Capture(t testing.TB, seam *RunFunc, captured *engine.RunRequest) {
	t.Helper()
	Stub(t, seam, func(req engine.RunRequest) ([]engine.LogEntry, error) {
		if captured != nil {
			*captured = req
		}
		return nil, nil
	})
}

// Entries stubs the seam to report entries for every run.
func Entries(t testing.TB, seam *RunFunc, entries ...engine.LogEntry) {
	t.Helper()
	Stub(t, seam, func(engine.RunRequest) ([]engine.LogEntry, error) {
		return entries, nil
	})
}
