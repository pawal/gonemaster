package main

import (
	"bytes"
	"slices"
	"strings"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine/querytrace"
)

// TestWriteQueryTraceCanceledColumn checks canceled attempts print in their own column, not under Errors.
func TestWriteQueryTraceCanceledColumn(t *testing.T) {
	c := querytrace.NewCollector()
	c.AttemptDone(querytrace.AttemptEvent{NSAddr: "192.0.2.1", Elapsed: 10 * time.Millisecond, Outcome: querytrace.OutcomeOK})
	c.AttemptDone(querytrace.AttemptEvent{NSAddr: "192.0.2.1", Elapsed: 5 * time.Millisecond, Outcome: querytrace.OutcomeCanceled})
	c.AttemptDone(querytrace.AttemptEvent{NSAddr: "192.0.2.1", Elapsed: 5 * time.Millisecond, Outcome: querytrace.OutcomeCanceled})
	c.AttemptDone(querytrace.AttemptEvent{NSAddr: "192.0.2.1", Elapsed: 5 * time.Millisecond, Outcome: querytrace.OutcomeError})

	var buf bytes.Buffer
	if err := writeQueryTrace(&buf, c); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 6 {
		t.Fatalf("expected 6 lines (summary, header, divider, row, divider, total), got %d:\n%s", len(lines), buf.String())
	}

	checks := []struct {
		name string
		line string
		want []string
	}{
		{"header", lines[1], []string{"Name", "servers", "Attempts", "Timeouts", "Errors", "Canceled", "Elapsed/ms", "Decisions"}},
		{"row", lines[3], []string{"192.0.2.1", "4", "0", "1", "2", "25.00"}},
		{"total", lines[5], []string{"Grand", "total", "4", "0", "1", "2", "25.00"}},
	}
	for _, ch := range checks {
		if got := strings.Fields(ch.line); !slices.Equal(got, ch.want) {
			t.Errorf("%s fields = %q, want %q", ch.name, got, ch.want)
		}
	}
}
