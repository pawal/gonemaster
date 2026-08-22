package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/cmd/internal/clitest"
	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
)

func TestWriteNSTimesEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := writeNSTimes(&buf, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected empty output, got %q", buf.String())
	}
}

func TestWriteNSTimesFormat(t *testing.T) {
	timings := map[string][]time.Duration{
		"ns1.example.com/192.0.2.1": {
			10 * time.Millisecond,
			20 * time.Millisecond,
			30 * time.Millisecond,
		},
		"ns2.example.com/192.0.2.2": {
			5 * time.Millisecond,
		},
	}

	var buf bytes.Buffer
	if err := writeNSTimes(&buf, timings, nil, nil); err != nil {
		t.Fatal(err)
	}

	output := buf.String()

	// Check header
	if !strings.Contains(output, "Name servers") {
		t.Fatal("missing header")
	}
	if !strings.Contains(output, "Max") {
		t.Fatal("missing Max column")
	}
	if !strings.Contains(output, "Median") {
		t.Fatal("missing Median column")
	}

	// Check nameserver entries appear
	if !strings.Contains(output, "ns1.example.com/192.0.2.1") {
		t.Fatal("missing ns1 entry")
	}
	if !strings.Contains(output, "ns2.example.com/192.0.2.2") {
		t.Fatal("missing ns2 entry")
	}

	// Check sorted order: ns1 has 3 queries, ns2 has 1 → ns1 first (descending count)
	idx1 := strings.Index(output, "ns1.example.com")
	idx2 := strings.Index(output, "ns2.example.com")
	if idx1 > idx2 {
		t.Fatal("entries not sorted by descending query count")
	}

	// Check grand total
	if !strings.Contains(output, "Grand total") {
		t.Fatal("missing grand total")
	}

	// Check count: ns1 has 3 queries, ns2 has 1 = 4 total
	lines := strings.Split(strings.TrimSpace(output), "\n")
	lastLine := lines[len(lines)-1]
	if !strings.Contains(lastLine, "4") {
		t.Fatalf("grand total line should contain count 4: %q", lastLine)
	}
}

// TestWriteNSTimesReportsTimeoutAndRefusedColumns pins the two counter
// columns and their grand totals. The interesting row is the mixed one: an
// address that answers most queries and still drops or refuses some looks
// perfectly healthy in every other column, which is precisely why the
// counters were added. The two columns are adjacent and hold small
// integers, so a swap between them would go unnoticed without an assertion
// that distinguishes their values.
func TestWriteNSTimesReportsTimeoutAndRefusedColumns(t *testing.T) {
	timings := map[string][]time.Duration{
		"ns1.example.com/192.0.2.1": {10 * time.Millisecond, 20 * time.Millisecond},
		"ns2.example.com/192.0.2.2": {30 * time.Millisecond},
	}
	timeouts := map[string]int{"ns1.example.com/192.0.2.1": 6}
	refused := map[string]int{"ns1.example.com/192.0.2.1": 3}

	var buf bytes.Buffer
	if err := writeNSTimes(&buf, timings, timeouts, refused); err != nil {
		t.Fatal(err)
	}
	output := buf.String()

	var mixed, clean, total string
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		switch {
		case strings.Contains(line, "ns1.example.com/192.0.2.1"):
			mixed = line
		case strings.Contains(line, "ns2.example.com/192.0.2.2"):
			clean = line
		case strings.Contains(line, "Grand total"):
			total = line
		}
	}
	if mixed == "" || clean == "" || total == "" {
		t.Fatalf("missing expected rows in output:\n%s", output)
	}

	// Trailing fields are Count, Timeout, Refused. Asserting the tail as a
	// sequence catches a swap of the last two, which comparing them
	// individually against "contains 6" and "contains 3" would not.
	if got := lastFields(mixed, 3); got != "2 6 3" {
		t.Fatalf("mixed row tail = %q, want count/timeout/refused of 2 6 3 in %q", got, mixed)
	}
	if got := lastFields(clean, 3); got != "1 0 0" {
		t.Fatalf("clean row tail = %q, want 1 0 0 in %q", got, clean)
	}
	if got := lastFields(total, 3); got != "3 6 3" {
		t.Fatalf("grand total tail = %q, want 3 6 3 in %q", got, total)
	}
}

// TestWriteNSTimesCountsTimeoutOnlyRowInTheTotals covers the row shape that
// carries no samples: its stat columns are dashes, but its timeout count is
// a real number and must reach the grand total, or an address that dropped
// everything would contribute nothing to the summary.
func TestWriteNSTimesCountsTimeoutOnlyRowInTheTotals(t *testing.T) {
	timings := map[string][]time.Duration{
		"ns1.example.com/192.0.2.1": {10 * time.Millisecond},
	}
	timeouts := map[string]int{"dead.example.com/192.0.2.9": 5}
	refused := map[string]int{"dead.example.com/192.0.2.9": 2}

	var buf bytes.Buffer
	if err := writeNSTimes(&buf, timings, timeouts, refused); err != nil {
		t.Fatal(err)
	}

	var dead, total string
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if strings.Contains(line, "dead.example.com") {
			dead = line
		}
		if strings.Contains(line, "Grand total") {
			total = line
		}
	}
	if dead == "" || total == "" {
		t.Fatalf("missing expected rows:\n%s", buf.String())
	}
	if got := lastFields(dead, 2); got != "5 2" {
		t.Fatalf("timeout-only row tail = %q, want 5 2 in %q", got, dead)
	}
	// Sample count stays 1: a timeout is not a response.
	if got := lastFields(total, 3); got != "1 5 2" {
		t.Fatalf("grand total tail = %q, want 1 5 2 in %q", got, total)
	}
}

// lastFields returns the final n whitespace-separated fields of a line,
// space-joined, so a test can assert a column sequence without depending on
// the exact padding widths.
func lastFields(line string, n int) string {
	fields := strings.Fields(line)
	if len(fields) < n {
		return strings.Join(fields, " ")
	}
	return strings.Join(fields[len(fields)-n:], " ")
}

// TestWriteNSTimesUnreachable checks that a nameserver that only ever timed
// out (present in timeouts, absent from timings) is still listed, rendered
// with dashes for every stat column instead of a fabricated response time, and
// left out of the grand-total count.
func TestWriteNSTimesUnreachable(t *testing.T) {
	timings := map[string][]time.Duration{
		"ns1.example.com/192.0.2.1": {10 * time.Millisecond, 20 * time.Millisecond},
	}
	timeouts := map[string]int{
		"ns.cocca.fr/192.0.2.9": 4,
	}

	var buf bytes.Buffer
	if err := writeNSTimes(&buf, timings, timeouts, nil); err != nil {
		t.Fatal(err)
	}
	output := buf.String()

	var deadLine string
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "ns.cocca.fr/192.0.2.9") {
			deadLine = line
			break
		}
	}
	if deadLine == "" {
		t.Fatalf("timed-out server missing from output:\n%s", output)
	}
	if strings.Count(deadLine, "-") < 7 {
		t.Fatalf("expected dashes in every stat column for the dead server, got %q", deadLine)
	}

	// Grand total count must be 2 (ns1's samples only), not inflated by the
	// four timed-out queries.
	lines := strings.Split(strings.TrimSpace(output), "\n")
	lastLine := lines[len(lines)-1]
	if !strings.Contains(lastLine, "Grand total") || !strings.Contains(lastLine, "2") {
		t.Fatalf("grand total should count only real samples (2): %q", lastLine)
	}
}

func TestWriteNSTimesLongNames(t *testing.T) {
	longName := strings.Repeat("a", 60) + ".example.com/192.0.2.1"
	timings := map[string][]time.Duration{
		longName: {10 * time.Millisecond},
	}

	var buf bytes.Buffer
	if err := writeNSTimes(&buf, timings, nil, nil); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(buf.String(), longName) {
		t.Fatal("long name should be fully visible")
	}
}

func TestWriteNSTimesSortedByName(t *testing.T) {
	timings := map[string][]time.Duration{
		"ns2.example.com/192.0.2.2": {30 * time.Millisecond},
		"ns1.example.com/192.0.2.1": {10 * time.Millisecond},
	}

	var buf bytes.Buffer
	if err := writeNSTimes(&buf, timings, nil, nil); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	idx1 := strings.Index(output, "ns1.example.com")
	idx2 := strings.Index(output, "ns2.example.com")
	if idx1 > idx2 {
		t.Fatal("entries not sorted by nameserver name ascending")
	}
}

func TestRunJSONWithNSTimes(t *testing.T) {
	previous := runEngine
	runEngine = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		if req.NameserverCache != nil {
			req.NameserverCache.RecordQueryTime("ns1.example.com/192.0.2.1", 10*time.Millisecond)
			req.NameserverCache.RecordQueryTime("ns1.example.com/192.0.2.1", 20*time.Millisecond)
		}
		return nil, nil
	}
	t.Cleanup(func() { runEngine = previous })

	res := clitest.Run(t, run, "--json", "--nstimes", "--domain", "example.com")
	res.RequireCode(t, 0)

	var result struct {
		Entries           []any                         `json:"entries"`
		NameserverTimings []nameserver.NameserverTiming `json:"nameserver_timings"`
	}
	if err := json.Unmarshal([]byte(res.Out), &result); err != nil {
		t.Fatalf("failed to decode JSON: %v\noutput: %s", err, res.Out)
	}
	if len(result.NameserverTimings) != 1 {
		t.Fatalf("expected 1 nameserver timing, got %d", len(result.NameserverTimings))
	}
	nt := result.NameserverTimings[0]
	if nt.Nameserver != "ns1.example.com" {
		t.Fatalf("Nameserver = %q, want %q", nt.Nameserver, "ns1.example.com")
	}
	if nt.Address != "192.0.2.1" {
		t.Fatalf("Address = %q, want %q", nt.Address, "192.0.2.1")
	}
	if nt.Count != 2 {
		t.Fatalf("Count = %d, want 2", nt.Count)
	}
	if nt.AvgMS != 15 {
		t.Fatalf("AvgMS = %f, want 15", nt.AvgMS)
	}
	if nt.Status != "ok" {
		t.Fatalf("Status = %q, want %q", nt.Status, "ok")
	}
}

func TestRunJSONWithoutNSTimesIsArray(t *testing.T) {
	previous := runEngine
	runEngine = func(req engine.RunRequest) ([]engine.LogEntry, error) { return nil, nil }
	t.Cleanup(func() { runEngine = previous })

	res := clitest.Run(t, run, "--json", "--domain", "example.com")
	res.RequireCode(t, 0)

	// Without --nstimes the output must still be a bare JSON array.
	var arr []any
	if err := json.Unmarshal([]byte(res.Out), &arr); err != nil {
		t.Fatalf("output should be a JSON array without --nstimes: %v\noutput: %s", err, res.Out)
	}
}
