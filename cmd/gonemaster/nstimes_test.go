package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
)

func TestWriteNSTimesEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := writeNSTimes(&buf, nil); err != nil {
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
	if err := writeNSTimes(&buf, timings); err != nil {
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

func TestWriteNSTimesLongNames(t *testing.T) {
	longName := strings.Repeat("a", 60) + ".example.com/192.0.2.1"
	timings := map[string][]time.Duration{
		longName: {10 * time.Millisecond},
	}

	var buf bytes.Buffer
	if err := writeNSTimes(&buf, timings); err != nil {
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
	if err := writeNSTimes(&buf, timings); err != nil {
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

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{"--json", "--nstimes", "--domain", "example.com"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: %s", code, errOut.String())
	}

	var result struct {
		Entries           []any                      `json:"entries"`
		NameserverTimings []nameserver.NameserverTiming `json:"nameserver_timings"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("failed to decode JSON: %v\noutput: %s", err, out.String())
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

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{"--json", "--domain", "example.com"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: %s", code, errOut.String())
	}

	// Without --nstimes the output must still be a bare JSON array.
	var arr []any
	if err := json.Unmarshal(out.Bytes(), &arr); err != nil {
		t.Fatalf("output should be a JSON array without --nstimes: %v\noutput: %s", err, out.String())
	}
}
