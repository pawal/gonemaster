package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// ── domains list ──────────────────────────────────────────────────────────────

func TestDomainsListPretty(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/api/v1/domains" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			body, _ := json.Marshal(domainList{
				Items: []domain{{ID: 1, Name: "example.com", LatestLevel: "WARNING", RunCount: 3}},
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
	if !strings.Contains(out.String(), "example.com") {
		t.Fatalf("expected example.com in output: %s", out.String())
	}
	if !strings.Contains(out.String(), "Domains: 1") {
		t.Fatalf("expected count in output: %s", out.String())
	}
}

func TestDomainsListJSON(t *testing.T) {
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
		t.Fatalf("expected JSON output: %v", err)
	}
}

// ── domains get ───────────────────────────────────────────────────────────────

func TestDomainsGet(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path == "/api/v1/domains" {
				body, _ := json.Marshal(domainList{
					Items: []domain{{ID: 7, Name: "example.com"}},
					Total: 1,
				})
				return jsonResponse(200, string(body)), nil
			}
			if r.URL.Path == "/api/v1/domains/7" {
				body, _ := json.Marshal(domain{ID: 7, Name: "example.com", RunCount: 5, Tags: []string{"tld"}})
				return jsonResponse(200, string(body)), nil
			}
			t.Fatalf("unexpected path: %s", r.URL.Path)
			return nil, nil
		})}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"domains", "get", "example.com"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "example.com") {
		t.Fatalf("expected domain name in output: %s", out.String())
	}
	if !strings.Contains(out.String(), "tld") {
		t.Fatalf("expected tag in output: %s", out.String())
	}
}

func TestDomainsGetNotFound(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			body, _ := json.Marshal(domainList{Items: []domain{}, Total: 0})
			return jsonResponse(200, string(body)), nil
		})}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"domains", "get", "ghost.example"}, &out, &errOut)
	if code == 0 {
		t.Fatal("expected non-zero exit for not found")
	}
}

// ── domains tag / untag ───────────────────────────────────────────────────────

func TestDomainsTag(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	var gotPath string
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			gotPath = r.URL.Path
			return jsonResponse(204, ""), nil
		})}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"domains", "tag", "example.com", "tld"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	if gotPath != "/api/v1/tags/tld/domains" {
		t.Fatalf("expected POST to /api/v1/tags/tld/domains, got %s", gotPath)
	}
	if !strings.Contains(out.String(), "Tagged") {
		t.Fatalf("expected confirmation in output: %s", out.String())
	}
}

func TestDomainsUntag(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	var gotMethod string
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			gotMethod = r.Method
			return jsonResponse(204, ""), nil
		})}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"domains", "untag", "example.com", "tld"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	if gotMethod != http.MethodDelete {
		t.Fatalf("expected DELETE, got %s", gotMethod)
	}
}

// ── tags list ─────────────────────────────────────────────────────────────────

func TestTagsList(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/api/v1/tags" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			body, _ := json.Marshal([]tag{{Name: "tld", DomainCount: 5}})
			return jsonResponse(200, string(body)), nil
		})}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"tags", "list"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "tld") {
		t.Fatalf("expected tld in output: %s", out.String())
	}
}

// ── tags create ───────────────────────────────────────────────────────────────

func TestTagsCreate(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPost || r.URL.Path != "/api/v1/tags" {
				t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
			}
			body, _ := json.Marshal(tag{Name: "municipalities", DomainCount: 0})
			return jsonResponse(201, string(body)), nil
		})}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"tags", "create", "municipalities", "--description", "Swedish municipalities"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "municipalities") {
		t.Fatalf("expected tag name in output: %s", out.String())
	}
}

// ── tags delete ───────────────────────────────────────────────────────────────

func TestTagsDelete(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	var gotMethod, gotPath string
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			gotMethod, gotPath = r.Method, r.URL.Path
			return jsonResponse(204, ""), nil
		})}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"tags", "delete", "tld"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	if gotMethod != http.MethodDelete || gotPath != "/api/v1/tags/tld" {
		t.Fatalf("expected DELETE /api/v1/tags/tld, got %s %s", gotMethod, gotPath)
	}
}

// ── tags summary ──────────────────────────────────────────────────────────────

func TestTagsSummary(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/api/v1/tags/tld/summary" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			body, _ := json.Marshal(tagSummary{Tag: "tld", DomainCount: 10, OK: 7, Warning: 2, Critical: 1})
			return jsonResponse(200, string(body)), nil
		})}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"tags", "summary", "tld"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "tld") {
		t.Fatalf("expected tag name in output: %s", out.String())
	}
	if !strings.Contains(out.String(), "10") {
		t.Fatalf("expected domain count in output: %s", out.String())
	}
}

// ── tags add-domains ──────────────────────────────────────────────────────────

func TestTagsAddDomains(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	var gotPath string
	var gotDomains []string
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			gotPath = r.URL.Path
			var body map[string][]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			gotDomains = body["domains"]
			return jsonResponse(204, ""), nil
		})}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"tags", "add-domains", "tld", "example.com", "example.net"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	if gotPath != "/api/v1/tags/tld/domains" {
		t.Fatalf("expected /api/v1/tags/tld/domains, got %s", gotPath)
	}
	if len(gotDomains) != 2 {
		t.Fatalf("expected 2 domains, got %v", gotDomains)
	}
}

// ── runs list ─────────────────────────────────────────────────────────────────

func TestRunsList(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/api/v1/runs" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			body, _ := json.Marshal(runList{
				Items: []runRecord{{ID: "run-1", Domain: "example.com", Status: "succeeded", WorstLevel: "WARNING"}},
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
	if !strings.Contains(out.String(), "example.com") {
		t.Fatalf("expected domain in output: %s", out.String())
	}
}

func TestRunsListPassesFilters(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	var gotQuery string
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			gotQuery = r.URL.RawQuery
			body, _ := json.Marshal(runList{Items: nil, Total: 0})
			return jsonResponse(200, string(body)), nil
		})}
	}
	var out, errOut bytes.Buffer
	run([]string{"runs", "list", "--tag", "tld", "--level", "WARNING"}, &out, &errOut)
	if !strings.Contains(gotQuery, "tag=tld") {
		t.Fatalf("expected tag filter in query: %s", gotQuery)
	}
	if !strings.Contains(gotQuery, "level=WARNING") {
		t.Fatalf("expected level filter in query: %s", gotQuery)
	}
}

// ── runs get ──────────────────────────────────────────────────────────────────

func TestRunsGet(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/api/v1/runs/abc123" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			body, _ := json.Marshal(runRecord{ID: "abc123", Domain: "example.com", Status: "succeeded", EntryCount: 42})
			return jsonResponse(200, string(body)), nil
		})}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"runs", "get", "abc123"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "abc123") {
		t.Fatalf("expected run ID in output: %s", out.String())
	}
	if !strings.Contains(out.String(), "42") {
		t.Fatalf("expected entry count in output: %s", out.String())
	}
}

// ── entries query ─────────────────────────────────────────────────────────────

func TestEntriesQuery(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/api/v1/entries" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			body, _ := json.Marshal(entryList{
				Items: []entry{{ID: 1, Module: "DNSSEC", Testcase: "DNSSEC02", Level: "WARNING"}},
				Total: 1,
			})
			return jsonResponse(200, string(body)), nil
		})}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"entries", "query", "--level", "WARNING"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "DNSSEC") {
		t.Fatalf("expected module in output: %s", out.String())
	}
}

func TestEntriesQueryCSV(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Query().Get("format") != "csv" {
				t.Fatalf("expected format=csv in query: %s", r.URL.RawQuery)
			}
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader("id,run_id,domain_id,timestamp,module,testcase,tag,level,args\n1,run-1,1,0.000,DNSSEC,DNSSEC02,NO_KEYS,WARNING,{}\n")),
				Header:     http.Header{"Content-Type": []string{"text/csv"}},
			}, nil
		})}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"--format", "csv", "entries", "query"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "DNSSEC") {
		t.Fatalf("expected CSV data in output: %s", out.String())
	}
}
