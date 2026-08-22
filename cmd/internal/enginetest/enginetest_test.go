package enginetest

import (
	"errors"
	"testing"

	"codeberg.org/pawal/gonemaster/engine"
)

// seam stands in for a binary's runEngine var.
var seam RunFunc = func(engine.RunRequest) ([]engine.LogEntry, error) {
	return nil, errors.New("the real engine")
}

func TestStubRestoresTheSeam(t *testing.T) {
	original := seam

	t.Run("stubbed", func(t *testing.T) {
		Stub(t, &seam, func(engine.RunRequest) ([]engine.LogEntry, error) {
			return []engine.LogEntry{{Tag: "STUBBED"}}, nil
		})
		entries, err := seam(engine.RunRequest{})
		if err != nil || len(entries) != 1 || entries[0].Tag != "STUBBED" {
			t.Fatalf("entries = %+v, err = %v", entries, err)
		}
	})

	if _, err := seam(engine.RunRequest{}); err == nil || err.Error() != "the real engine" {
		t.Fatalf("seam not restored, err = %v", err)
	}
	_ = original
}

func TestCaptureRecordsTheRequest(t *testing.T) {
	var captured engine.RunRequest
	Capture(t, &seam, &captured)

	entries, err := seam(engine.RunRequest{Domain: "example.com"})
	if err != nil || entries != nil {
		t.Fatalf("entries = %+v, err = %v", entries, err)
	}
	if captured.Domain != "example.com" {
		t.Fatalf("captured domain = %q", captured.Domain)
	}
}

func TestCaptureAcceptsNil(t *testing.T) {
	Capture(t, &seam, nil)

	if _, err := seam(engine.RunRequest{Domain: "example.com"}); err != nil {
		t.Fatalf("err = %v", err)
	}
}

func TestEntriesReportsWhatItWasGiven(t *testing.T) {
	Entries(t, &seam, engine.LogEntry{Tag: "A"}, engine.LogEntry{Tag: "B"})

	entries, err := seam(engine.RunRequest{})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(entries) != 2 || entries[0].Tag != "A" || entries[1].Tag != "B" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestEntriesWithNoneReportsAnEmptyRun(t *testing.T) {
	Entries(t, &seam)

	entries, err := seam(engine.RunRequest{})
	if err != nil || len(entries) != 0 {
		t.Fatalf("entries = %+v, err = %v", entries, err)
	}
}
