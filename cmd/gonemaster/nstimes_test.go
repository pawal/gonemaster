package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
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
