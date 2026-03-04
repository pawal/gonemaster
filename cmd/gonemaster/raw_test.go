package main

import (
	"bytes"
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/logger"
)

func TestFormatRawArgValueEndpointList(t *testing.T) {
	value := []map[string]any{
		{"ns": "ns1.example", "address": "192.0.2.1"},
		{"ns": "ns2.example", "address": "192.0.2.2"},
	}
	got := formatRawArgValue(value)
	want := "ns1.example/192.0.2.1,ns2.example/192.0.2.2"
	if got != want {
		t.Fatalf("unexpected formatted endpoint list: got %q want %q", got, want)
	}
}

func TestFormatRawArgValueGenericMap(t *testing.T) {
	value := map[string]any{
		"query_type": "SOA",
		"query_name": "example.com",
	}
	got := formatRawArgValue(value)
	want := "{query_name=example.com;query_type=SOA}"
	if got != want {
		t.Fatalf("unexpected formatted map: got %q want %q", got, want)
	}
}

func TestRawReporterFormatsCollectionsWithoutJSON(t *testing.T) {
	var out bytes.Buffer
	reporter := newRawReporter(&out, "")

	entry, err := logger.NewEntry(
		"EXTRA_PROCESSING_OK",
		map[string]any{
			"servers": []map[string]any{
				{"ns": "ns1.example", "address": "192.0.2.1"},
				{"ns": "ns2.example", "address": "192.0.2.2"},
			},
			"mail_targets": []string{"mx1.example", "mx2.example"},
			"details": map[string]any{
				"query_type": "SOA",
				"query_name": "example.com",
			},
		},
		"DNSSEC06",
		"DNSSEC",
	)
	if err != nil {
		t.Fatalf("new entry: %v", err)
	}

	if err := reporter.Callback(entry); err != nil {
		t.Fatalf("callback: %v", err)
	}
	line := strings.TrimSpace(out.String())
	if !strings.Contains(line, "servers=ns1.example/192.0.2.1,ns2.example/192.0.2.2") {
		t.Fatalf("expected human-readable servers list, got %q", line)
	}
	if !strings.Contains(line, "mail_targets=mx1.example,mx2.example") {
		t.Fatalf("expected human-readable string list, got %q", line)
	}
	if !strings.Contains(line, "details={query_name=example.com;query_type=SOA}") {
		t.Fatalf("expected human-readable map, got %q", line)
	}
	if strings.Contains(line, `{"ns":"ns1.example"`) || strings.Contains(line, `["mx1.example"`) {
		t.Fatalf("did not expect JSON-formatted collections in raw output: %q", line)
	}
}
