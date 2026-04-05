package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
)

// parseOpenAPIPaths extracts (path, method) pairs from docs/openapi.yaml using
// simple line-by-line scanning. It does not need a YAML library.
func parseOpenAPIPaths(t *testing.T, data []byte) map[string][]string {
	t.Helper()
	paths := map[string][]string{}
	var currentPath string
	inPaths := false

	pathRe := regexp.MustCompile(`^  (/[^\s:]+):\s*$`)
	methodRe := regexp.MustCompile(`^    (get|post|put|patch|delete|head|options):\s*$`)

	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimRight(raw, "\r")
		if line == "paths:" {
			inPaths = true
			continue
		}
		if inPaths && line != "" && !strings.HasPrefix(line, " ") {
			// Hit a top-level YAML key after paths: (e.g. "components:").
			inPaths = false
			continue
		}
		if !inPaths {
			continue
		}
		if m := pathRe.FindStringSubmatch(line); m != nil {
			currentPath = m[1]
			if _, ok := paths[currentPath]; !ok {
				paths[currentPath] = nil
			}
			continue
		}
		if currentPath != "" {
			if m := methodRe.FindStringSubmatch(line); m != nil {
				paths[currentPath] = append(paths[currentPath], strings.ToUpper(m[1]))
			}
		}
	}
	return paths
}

// substitutePathParams replaces OpenAPI path parameters like {job_id} with
// fixed dummy values so we can make a real HTTP request to the route.
func substitutePathParams(path string) string {
	replacer := strings.NewReplacer(
		"{id}", "1",
		"{name}", "default",
		"{job_id}", "00000000-0000-0000-0000-000000000000",
		"{batch_id}", "00000000-0000-0000-0000-000000000000",
	)
	return replacer.Replace(path)
}

// TestOpenAPIPathCoverage verifies that every path+method defined in
// docs/openapi.yaml is actually wired up in the server (i.e. returns an
// application/json response rather than falling through to the UI handler or
// returning a plain 404 from the mux for an unregistered route).
//
// It does NOT validate response schemas — it only checks that routes exist.
func TestOpenAPIPathCoverage(t *testing.T) {
	data, err := os.ReadFile("../docs/openapi.yaml")
	if err != nil {
		t.Fatalf("read docs/openapi.yaml: %v", err)
	}

	specPaths := parseOpenAPIPaths(t, data)
	if len(specPaths) == 0 {
		t.Fatal("no paths parsed from openapi.yaml — check the parser")
	}

	srv := New(DefaultConfig())

	for specPath, methods := range specPaths {
		concretePath := substitutePathParams(specPath)
		for _, method := range methods {
			t.Run(method+" "+specPath, func(t *testing.T) {
				req := httptest.NewRequest(method, "/api/v1"+concretePath, nil)
				// POST/PUT/PATCH need a content-type header so we don't get a
				// parse error before the route is even matched.
				if method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch {
					req.Header.Set("Content-Type", "application/json")
				}
				resp := httptest.NewRecorder()
				srv.Handler().ServeHTTP(resp, req)

				ct := resp.Header().Get("Content-Type")
				if !strings.HasPrefix(ct, "application/json") {
					t.Errorf(
						"route %s %s returned Content-Type %q (status %d) — route may not be registered",
						method, specPath, ct, resp.Code,
					)
				}
			})
		}
	}
}

// TestOpenAPIParserExtractsPaths is a unit test for the parser itself to
// ensure it correctly identifies paths and methods from the spec.
func TestOpenAPIParserExtractsPaths(t *testing.T) {
	yaml := []byte(`
openapi: 3.0.3
paths:
  /healthz:
    get:
      summary: Health check
  /jobs:
    get:
      summary: List jobs
    post:
      summary: Create job
  /jobs/{job_id}:
    get:
      summary: Get job
components:
  schemas: {}
`)
	got := parseOpenAPIPaths(t, yaml)

	expect := map[string][]string{
		"/healthz":       {"GET"},
		"/jobs":          {"GET", "POST"},
		"/jobs/{job_id}": {"GET"},
	}
	for path, methods := range expect {
		gotMethods, ok := got[path]
		if !ok {
			t.Errorf("expected path %q to be parsed", path)
			continue
		}
		gotSet := map[string]bool{}
		for _, m := range gotMethods {
			gotSet[m] = true
		}
		for _, m := range methods {
			if !gotSet[m] {
				t.Errorf("expected method %s for path %q, got %v", m, path, gotMethods)
			}
		}
	}
	if _, ok := got["schemas"]; ok {
		t.Error("parser must not include components keys as paths")
	}
}

// TestOpenAPIJobSchemaIncludesPriority verifies that the Job schema in
// docs/openapi.yaml declares the priority field and that the real API
// response includes it with the correct values.
func TestOpenAPIJobSchemaIncludesPriority(t *testing.T) {
	data, err := os.ReadFile("../docs/openapi.yaml")
	if err != nil {
		t.Fatalf("read docs/openapi.yaml: %v", err)
	}
	if !strings.Contains(string(data), "priority:") {
		t.Fatal("docs/openapi.yaml Job schema is missing the priority field")
	}

	srv := New(DefaultConfig())

	// POST /jobs → priority must be 0 (normal) in the response.
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs",
		bytes.NewBufferString(`{"domain":"example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("POST /jobs: expected 201, got %d", resp.Code)
	}
	var job map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		t.Fatalf("decode job: %v", err)
	}
	if _, ok := job["priority"]; !ok {
		t.Fatal("POST /jobs response is missing the priority field")
	}
	if int(job["priority"].(float64)) != int(PriorityNormal) {
		t.Fatalf("POST /jobs priority: expected %d, got %v", PriorityNormal, job["priority"])
	}

	// POST /jobs/batch → each job must have priority 1 (batch).
	resp2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/batch",
		bytes.NewBufferString(`{"domains":["example.com"]}`))
	req2.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp2, req2)
	if resp2.Code != http.StatusAccepted {
		t.Fatalf("POST /jobs/batch: expected 202, got %d", resp2.Code)
	}
	var batchResp JobBatchResponse
	if err := json.NewDecoder(resp2.Body).Decode(&batchResp); err != nil {
		t.Fatalf("decode batch response: %v", err)
	}
	for _, jobID := range batchResp.JobIDs {
		resp3 := httptest.NewRecorder()
		req3 := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+jobID, nil)
		srv.Handler().ServeHTTP(resp3, req3)
		if resp3.Code != http.StatusOK {
			t.Fatalf("GET /jobs/%s: expected 200, got %d", jobID, resp3.Code)
		}
		var batchJob map[string]any
		if err := json.NewDecoder(resp3.Body).Decode(&batchJob); err != nil {
			t.Fatalf("decode batch job: %v", err)
		}
		if _, ok := batchJob["priority"]; !ok {
			t.Fatalf("GET /jobs/%s response is missing the priority field", jobID)
		}
		if int(batchJob["priority"].(float64)) != int(PriorityBatch) {
			t.Fatalf("GET /jobs/%s priority: expected %d, got %v", jobID, PriorityBatch, batchJob["priority"])
		}
	}
}

func TestOpenAPIProfileSchemaIncludesStoredProfileFields(t *testing.T) {
	data, err := os.ReadFile("../docs/openapi.yaml")
	if err != nil {
		t.Fatalf("read docs/openapi.yaml: %v", err)
	}
	spec := string(data)

	requiredSnippets := []string{
		"/profiles:",
		"/profiles/default:",
		"/profiles/{id}:",
		"/tags/{name}/profile:",
		"Profile:",
		"ProfileUpsertRequest:",
		"TagProfileRequest:",
		"default_profile_id:",
		"profile_id:",
		"profile_name:",
		"effective_profile:",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(spec, snippet) {
			t.Fatalf("docs/openapi.yaml is missing %q", snippet)
		}
	}
}
