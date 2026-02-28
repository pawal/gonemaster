package server

import (
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
		"/healthz":      {"GET"},
		"/jobs":         {"GET", "POST"},
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
