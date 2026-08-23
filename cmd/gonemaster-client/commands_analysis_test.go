package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/cmd/internal/clitest"
	"codeberg.org/pawal/gonemaster/internal/apitest"
)

// ── domains list ──────────────────────────────────────────────────────────────

func TestDomainsListPretty(t *testing.T) {
	apitest.StubClient(t, &newHTTPClient, apitest.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/domains" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body, _ := json.Marshal(domainList{
			Items: []domain{{ID: 1, Name: "example.com", LatestLevel: "WARNING", RunCount: 3}},
			Total: 1,
		})
		return apitest.JSONResponse(200, string(body)), nil
	}))
	res := clitest.Run(t, run, "domains", "list")
	res.RequireCode(t, 0)
	res.RequireOutContains(t, "example.com")
	res.RequireOutContains(t, "Domains: 1")
}

func TestDomainsListJSON(t *testing.T) {
	apitest.StubClient(t, &newHTTPClient, apitest.RoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		body, _ := json.Marshal(domainList{Items: []domain{{ID: 1, Name: "example.com"}}, Total: 1})
		return apitest.JSONResponse(200, string(body)), nil
	}))
	res := clitest.Run(t, run, "--format", "json", "domains", "list")
	res.RequireCode(t, 0)
	var got domainList
	if err := json.Unmarshal([]byte(res.Out), &got); err != nil {
		t.Fatalf("expected JSON output: %v", err)
	}
}

// ── domains get ───────────────────────────────────────────────────────────────

func TestDomainsGet(t *testing.T) {
	apitest.StubClient(t, &newHTTPClient, apitest.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/api/v1/domains" {
			body, _ := json.Marshal(domainList{
				Items: []domain{{ID: 7, Name: "example.com"}},
				Total: 1,
			})
			return apitest.JSONResponse(200, string(body)), nil
		}
		if r.URL.Path == "/api/v1/domains/7" {
			body, _ := json.Marshal(domain{ID: 7, Name: "example.com", RunCount: 5, Tags: []string{"tld"}})
			return apitest.JSONResponse(200, string(body)), nil
		}
		t.Fatalf("unexpected path: %s", r.URL.Path)
		return nil, nil
	}))
	res := clitest.Run(t, run, "domains", "get", "example.com")
	res.RequireCode(t, 0)
	res.RequireOutContains(t, "example.com")
	res.RequireOutContains(t, "tld")
}

func TestDomainsGetNotFound(t *testing.T) {
	apitest.StubClient(t, &newHTTPClient, apitest.RoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		body, _ := json.Marshal(domainList{Items: []domain{}, Total: 0})
		return apitest.JSONResponse(200, string(body)), nil
	}))
	res := clitest.Run(t, run, "domains", "get", "ghost.example")
	if res.Code == 0 {
		t.Fatal("expected non-zero exit for not found")
	}
}

// ── domains tag / untag ───────────────────────────────────────────────────────

func TestDomainsTag(t *testing.T) {
	var gotPath string
	apitest.StubClient(t, &newHTTPClient, apitest.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotPath = r.URL.Path
		return apitest.JSONResponse(204, ""), nil
	}))
	res := clitest.Run(t, run, "domains", "tag", "example.com", "tld")
	res.RequireCode(t, 0)
	if gotPath != "/api/v1/tags/tld/domains" {
		t.Fatalf("expected POST to /api/v1/tags/tld/domains, got %s", gotPath)
	}
	res.RequireOutContains(t, "Tagged")
}

func TestDomainsUntag(t *testing.T) {
	var gotMethod string
	apitest.StubClient(t, &newHTTPClient, apitest.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotMethod = r.Method
		return apitest.JSONResponse(204, ""), nil
	}))
	res := clitest.Run(t, run, "domains", "untag", "example.com", "tld")
	res.RequireCode(t, 0)
	if gotMethod != http.MethodDelete {
		t.Fatalf("expected DELETE, got %s", gotMethod)
	}
}

// ── tags list ─────────────────────────────────────────────────────────────────

func TestTagsList(t *testing.T) {
	apitest.StubClient(t, &newHTTPClient, apitest.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/tags" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body, _ := json.Marshal([]tag{{Name: "tld", DomainCount: 5}})
		return apitest.JSONResponse(200, string(body)), nil
	}))
	res := clitest.Run(t, run, "tags", "list")
	res.RequireCode(t, 0)
	res.RequireOutContains(t, "tld")
}

// ── tags create ───────────────────────────────────────────────────────────────

func TestTagsCreate(t *testing.T) {
	apitest.StubClient(t, &newHTTPClient, apitest.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/tags" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		body, _ := json.Marshal(tag{Name: "municipalities", DomainCount: 0})
		return apitest.JSONResponse(201, string(body)), nil
	}))
	res := clitest.Run(t, run, "tags", "create", "municipalities", "--description", "Swedish municipalities")
	res.RequireCode(t, 0)
	res.RequireOutContains(t, "municipalities")
}

// ── tags delete ───────────────────────────────────────────────────────────────

func TestTagsDelete(t *testing.T) {
	var gotMethod, gotPath string
	apitest.StubClient(t, &newHTTPClient, apitest.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotMethod, gotPath = r.Method, r.URL.Path
		return apitest.JSONResponse(204, ""), nil
	}))
	res := clitest.Run(t, run, "tags", "delete", "tld")
	res.RequireCode(t, 0)
	if gotMethod != http.MethodDelete || gotPath != "/api/v1/tags/tld" {
		t.Fatalf("expected DELETE /api/v1/tags/tld, got %s %s", gotMethod, gotPath)
	}
}

// ── tags summary ──────────────────────────────────────────────────────────────

func TestTagsSummary(t *testing.T) {
	apitest.StubClient(t, &newHTTPClient, apitest.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/tags/tld/summary" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body, _ := json.Marshal(tagSummary{Tag: "tld", DomainCount: 10, OK: 7, Warning: 2, Critical: 1})
		return apitest.JSONResponse(200, string(body)), nil
	}))
	res := clitest.Run(t, run, "tags", "summary", "tld")
	res.RequireCode(t, 0)
	res.RequireOutContains(t, "tld")
	res.RequireOutContains(t, "10")
}

// ── tags add-domains ──────────────────────────────────────────────────────────

func TestTagsAddDomains(t *testing.T) {
	var gotBody map[string][]string
	rec := apitest.CaptureJSON(t, &gotBody, http.StatusNoContent, "")
	apitest.StubClient(t, &newHTTPClient, rec)
	res := clitest.Run(t, run, "tags", "add-domains", "tld", "example.com", "example.net")
	res.RequireCode(t, 0)
	if rec.Path() != "/api/v1/tags/tld/domains" {
		t.Fatalf("expected /api/v1/tags/tld/domains, got %s", rec.Path())
	}
	if len(gotBody["domains"]) != 2 {
		t.Fatalf("expected 2 domains, got %v", gotBody["domains"])
	}
}

// ── runs list ─────────────────────────────────────────────────────────────────

func TestRunsList(t *testing.T) {
	apitest.StubClient(t, &newHTTPClient, apitest.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/runs" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body, _ := json.Marshal(runList{
			Items: []runRecord{{ID: "run-1", Domain: "example.com", Status: "succeeded", WorstLevel: "WARNING"}},
			Total: 1,
		})
		return apitest.JSONResponse(200, string(body)), nil
	}))
	res := clitest.Run(t, run, "runs", "list")
	res.RequireCode(t, 0)
	res.RequireOutContains(t, "example.com")
}

func TestRunsListPassesFilters(t *testing.T) {
	var gotQuery string
	apitest.StubClient(t, &newHTTPClient, apitest.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotQuery = r.URL.RawQuery
		body, _ := json.Marshal(runList{Items: nil, Total: 0})
		return apitest.JSONResponse(200, string(body)), nil
	}))
	clitest.Run(t, run, "runs", "list", "--tag", "tld", "--level", "WARNING")
	if !strings.Contains(gotQuery, "tag=tld") {
		t.Fatalf("expected tag filter in query: %s", gotQuery)
	}
	if !strings.Contains(gotQuery, "level=WARNING") {
		t.Fatalf("expected level filter in query: %s", gotQuery)
	}
}

// ── runs get ──────────────────────────────────────────────────────────────────

func TestRunsGet(t *testing.T) {
	apitest.StubClient(t, &newHTTPClient, apitest.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/runs/abc123" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body, _ := json.Marshal(runRecord{ID: "abc123", Domain: "example.com", Status: "succeeded", EntryCount: 42})
		return apitest.JSONResponse(200, string(body)), nil
	}))
	res := clitest.Run(t, run, "runs", "get", "abc123")
	res.RequireCode(t, 0)
	res.RequireOutContains(t, "abc123")
	res.RequireOutContains(t, "42")
}

// ── entries query ─────────────────────────────────────────────────────────────

func TestEntriesQuery(t *testing.T) {
	apitest.StubClient(t, &newHTTPClient, apitest.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/entries" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body, _ := json.Marshal(entryList{
			Items: []entry{{ID: 1, Module: "DNSSEC", Testcase: "DNSSEC02", Level: "WARNING"}},
			Total: 1,
		})
		return apitest.JSONResponse(200, string(body)), nil
	}))
	res := clitest.Run(t, run, "entries", "query", "--level", "WARNING")
	res.RequireCode(t, 0)
	res.RequireOutContains(t, "DNSSEC")
}

func TestEntriesQueryCSV(t *testing.T) {
	apitest.StubClient(t, &newHTTPClient, apitest.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("format") != "csv" {
			t.Fatalf("expected format=csv in query: %s", r.URL.RawQuery)
		}
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader("id,run_id,domain_id,timestamp,module,testcase,tag,level,args\n1,run-1,1,0.000,DNSSEC,DNSSEC02,NO_KEYS,WARNING,{}\n")),
			Header:     http.Header{"Content-Type": []string{"text/csv"}},
		}, nil
	}))
	res := clitest.Run(t, run, "--format", "csv", "entries", "query")
	res.RequireCode(t, 0)
	res.RequireOutContains(t, "DNSSEC")
}
