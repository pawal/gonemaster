package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// ── domains pretty-printer ────────────────────────────────────────────────────

func TestDomainsListPrettyWithRunAt(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			body, _ := json.Marshal(domainList{
				Items: []domain{
					{
						ID:          1,
						Name:        "example.com",
						LatestLevel: "WARNING",
						LatestRunAt: "2026-03-15T10:00:00Z",
						RunCount:    3,
						Tags:        []string{"tld"},
					},
				},
				Total: 1,
			})
			return jsonResponse(200, string(body)), nil
		})}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"domains", "list"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	output := out.String()
	if !strings.Contains(output, "example.com") {
		t.Fatalf("expected domain name in output: %s", output)
	}
	if !strings.Contains(output, "2026-03-15") {
		t.Fatalf("expected latest_run_at date in output: %s", output)
	}
	if !strings.Contains(output, "WARNING") {
		t.Fatalf("expected level in output: %s", output)
	}
	if !strings.Contains(output, "tld") {
		t.Fatalf("expected tag in output: %s", output)
	}
}

func TestDomainsListPrettyNoRunAt(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			body, _ := json.Marshal(domainList{
				Items: []domain{{ID: 1, Name: "example.com", RunCount: 0}},
				Total: 1,
			})
			return jsonResponse(200, string(body)), nil
		})}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"domains", "list"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	// Should show "-" for missing last date.
	if !strings.Contains(out.String(), "-") {
		t.Fatalf("expected '-' placeholder for missing run_at: %s", out.String())
	}
}

// ── runs pretty-printer ───────────────────────────────────────────────────────

func TestRunsListPrettyWithFinishedAt(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			body, _ := json.Marshal(runList{
				Items: []runRecord{
					{
						ID:         "run-abc-123",
						Domain:     "example.com",
						Status:     "succeeded",
						WorstLevel: "WARNING",
						FinishedAt: "2026-03-15T10:00:00Z",
						DurationMs: 1250,
					},
				},
				Total: 1,
			})
			return jsonResponse(200, string(body)), nil
		})}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"runs", "list"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	output := out.String()
	if !strings.Contains(output, "run-abc-123") {
		t.Fatalf("expected run ID in output: %s", output)
	}
	if !strings.Contains(output, "2026-03-15") {
		t.Fatalf("expected finished_at date in output: %s", output)
	}
	if !strings.Contains(output, "1250ms") {
		t.Fatalf("expected duration in output: %s", output)
	}
}

func TestRunsListPrettyNoFinishedAt(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			body, _ := json.Marshal(runList{
				Items: []runRecord{{ID: "run-1", Domain: "example.com", Status: "succeeded"}},
				Total: 1,
			})
			return jsonResponse(200, string(body)), nil
		})}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"runs", "list"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	// Should show "-" for missing finished_at.
	if !strings.Contains(out.String(), "-") {
		t.Fatalf("expected '-' placeholder: %s", out.String())
	}
}

// ── entries pretty-printer ────────────────────────────────────────────────────

func TestEntriesQueryPrettyWithDomain(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			body, _ := json.Marshal(entryList{
				Items: []entry{
					{
						ID:       1,
						RunID:    "run-1",
						DomainID: 7,
						Domain:   "example.com",
						Module:   "DNSSEC",
						Testcase: "DNSSEC02",
						Tag:      "DNSSEC_NSEC3",
						Level:    "WARNING",
					},
				},
				Total: 1,
			})
			return jsonResponse(200, string(body)), nil
		})}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"entries", "query"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	output := out.String()
	if !strings.Contains(output, "example.com") {
		t.Fatalf("expected domain name in output: %s", output)
	}
	if !strings.Contains(output, "DNSSEC") {
		t.Fatalf("expected module in output: %s", output)
	}
	if !strings.Contains(output, "WARNING") {
		t.Fatalf("expected level in output: %s", output)
	}
}

func TestEntriesQueryPrettyNoDomainFallsBackToID(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			body, _ := json.Marshal(entryList{
				Items: []entry{
					{ID: 1, DomainID: 42, Module: "Basic", Testcase: "Basic01", Level: "INFO"},
				},
				Total: 1,
			})
			return jsonResponse(200, string(body)), nil
		})}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"entries", "query"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	// Should show "id:42" when domain name is absent.
	if !strings.Contains(out.String(), "id:42") {
		t.Fatalf("expected 'id:42' fallback in output: %s", out.String())
	}
}

// ── JSON/JSONL output for new commands ────────────────────────────────────────

func TestDomainsListJSONOutput(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			body, _ := json.Marshal(domainList{Items: []domain{{ID: 1, Name: "example.com"}}, Total: 1})
			return jsonResponse(200, string(body)), nil
		})}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"--format", "json", "domains", "list"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	var got domainList
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("expected valid JSON output: %v — got: %s", err, out.String())
	}
}

func TestRunsListJSON(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			body, _ := json.Marshal(runList{Items: []runRecord{{ID: "r1", Domain: "example.com", Status: "succeeded"}}, Total: 1})
			return jsonResponse(200, string(body)), nil
		})}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"--format", "json", "runs", "list"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	var got runList
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("expected valid JSON output: %v — got: %s", err, out.String())
	}
}

func TestTagsListJSON(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			body, _ := json.Marshal([]tag{{Name: "tld", Description: "Top-level", DomainCount: 5}})
			return jsonResponse(200, string(body)), nil
		})}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"--format", "json", "tags", "list"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	var got []tag
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("expected valid JSON output: %v — got: %s", err, out.String())
	}
}

func TestEntriesQueryJSON(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			body, _ := json.Marshal(entryList{
				Items: []entry{{ID: 1, Module: "Basic", Level: "INFO"}},
				Total: 1,
			})
			return jsonResponse(200, string(body)), nil
		})}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"--format", "json", "entries", "query"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	var got entryList
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("expected valid JSON output: %v — got: %s", err, out.String())
	}
}
