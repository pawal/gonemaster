package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/scoring"
)

// jobResultWithEntries returns a JSON body for a job result that includes raw
// entries so the scoring engine has data to work with.
func jobResultWithEntries(jobID string) string {
	body, _ := json.Marshal(jobResult{
		JobID:  jobID,
		Status: "succeeded",
		Raw: &jobResultRaw{
			Entries: []jobResultEntry{
				{Module: "DNSSEC", Tag: "DS07_SIGNED", Level: "INFO"},
				{Module: "DNSSEC", Tag: "DS05_ALGO_OK", Level: "INFO"},
				{Module: "NAMESERVER", Tag: "NS01_NO_RESPONSE", Level: "WARNING"},
			},
		},
	})
	return string(body)
}

// jobResultWithoutEntries returns a summary-only job result (no raw entries).
func jobResultWithoutEntries(jobID string) string {
	body, _ := json.Marshal(jobResult{
		JobID:   jobID,
		Status:  "succeeded",
		Summary: map[string]any{"total": 1, "levels": map[string]any{"WARNING": 1}},
	})
	return string(body)
}

func mockJobAndResult(jobID, domain, resultBody string) roundTripFunc {
	return func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/v1/jobs/" + jobID:
			body := `{"id":"` + jobID + `","domain":"` + domain + `","status":"succeeded","created_at":"2026-01-01T00:00:00Z","progress":100}`
			return jsonResponse(http.StatusOK, body), nil
		case "/api/v1/jobs/" + jobID + "/result":
			return jsonResponse(http.StatusOK, resultBody), nil
		default:
			return jsonResponse(http.StatusNotFound, `{"error":{"code":"not_found","message":"not found"}}`), nil
		}
	}
}

// ── --score flag ──────────────────────────────────────────────────────────────

func TestJobsResultsNoScoreByDefault(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: mockJobAndResult("job_1", "example.se", jobResultWithEntries("job_1"))}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"jobs", "results", "job_1"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	if strings.Contains(out.String(), "Score:") {
		t.Fatalf("expected no score by default, got: %s", out.String())
	}
}

func TestJobsResultsWithScore(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: mockJobAndResult("job_1", "example.se", jobResultWithEntries("job_1"))}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"jobs", "results", "--score", "job_1"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "Score:") {
		t.Fatalf("expected score in output, got: %s", out.String())
	}
}

func TestJobsResultsNoScoreSuppresses(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: mockJobAndResult("job_1", "example.se", jobResultWithEntries("job_1"))}
	}
	var out, errOut bytes.Buffer
	// --no-score should suppress even if both flags are given.
	code := run([]string{"jobs", "results", "--score", "--no-score", "job_1"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	if strings.Contains(out.String(), "Score:") {
		t.Fatalf("expected no score with --no-score, got: %s", out.String())
	}
}

func TestJobsResultsScoreShowsCategories(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: mockJobAndResult("job_1", "example.se", jobResultWithEntries("job_1"))}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"jobs", "results", "--score", "job_1"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	output := out.String()
	for _, cat := range []string{"dnssec:", "nameserver_health:", "connectivity:", "zone_consistency:"} {
		if !strings.Contains(output, cat) {
			t.Errorf("expected category %q in output, got:\n%s", cat, output)
		}
	}
}

func TestJobsResultsScoreNotAvailableWithoutEntries(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: mockJobAndResult("job_1", "example.se", jobResultWithoutEntries("job_1"))}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"jobs", "results", "--score", "job_1"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "N/A") {
		t.Fatalf("expected N/A when no entries, got: %s", out.String())
	}
}

// ── --scoring-config flag ─────────────────────────────────────────────────────

func TestJobsResultsScoringConfigImpliesScore(t *testing.T) {
	// Write a minimal valid scoring config to a temp file.
	cfg := scoring.DefaultConfig()
	data, _ := json.Marshal(cfg)
	f, err := os.CreateTemp(t.TempDir(), "scoring-*.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
	f.Close()

	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: mockJobAndResult("job_1", "example.se", jobResultWithEntries("job_1"))}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"jobs", "results", "--scoring-config", f.Name(), "job_1"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "Score:") {
		t.Fatalf("expected --scoring-config to imply --score, got: %s", out.String())
	}
}

func TestJobsResultsScoringConfigInvalidPath(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"jobs", "results", "--scoring-config", "/nonexistent/path.json", "job_1"}, &out, &errOut)
	if code == 0 {
		t.Fatal("expected non-zero exit for invalid --scoring-config path")
	}
}

func TestScoringConfigNoScoreWins(t *testing.T) {
	cfg := scoring.DefaultConfig()
	data, _ := json.Marshal(cfg)
	f, err := os.CreateTemp(t.TempDir(), "scoring-*.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
	f.Close()

	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: mockJobAndResult("job_1", "example.se", jobResultWithEntries("job_1"))}
	}
	var out, errOut bytes.Buffer
	// --no-score should override --scoring-config.
	code := run([]string{"jobs", "results", "--scoring-config", f.Name(), "--no-score", "job_1"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	if strings.Contains(out.String(), "Score:") {
		t.Fatalf("expected --no-score to suppress scoring config, got: %s", out.String())
	}
}

// ── JSON output ───────────────────────────────────────────────────────────────

func TestJobsResultsScoreInJSONOutput(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: mockJobAndResult("job_1", "example.se", jobResultWithEntries("job_1"))}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"--format", "json", "jobs", "results", "--score", "job_1"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	var result jobResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v - got: %s", err, out.String())
	}
	if result.Score == nil {
		t.Fatalf("expected score field in JSON output, got: %s", out.String())
	}
	if result.Score.Grade == "" {
		t.Fatalf("expected non-empty grade in JSON score")
	}
}

func TestJobsResultsNoScoreInJSONOutputByDefault(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: mockJobAndResult("job_1", "example.se", jobResultWithEntries("job_1"))}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"--format", "json", "jobs", "results", "job_1"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	var result jobResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if result.Score != nil {
		t.Fatalf("expected no score field by default, got score=%+v", result.Score)
	}
}

// ── parseScoringOptions unit tests ────────────────────────────────────────────

func TestParseScoringOptionsDefault(t *testing.T) {
	opts, err := parseScoringOptions(false, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if opts.enabled {
		t.Fatal("expected scoring disabled by default")
	}
}

func TestParseScoringOptionsScoreFlag(t *testing.T) {
	opts, err := parseScoringOptions(true, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if !opts.enabled {
		t.Fatal("expected scoring enabled with --score")
	}
}

func TestParseScoringOptionsNoScoreWins(t *testing.T) {
	opts, err := parseScoringOptions(true, true, "")
	if err != nil {
		t.Fatal(err)
	}
	if opts.enabled {
		t.Fatal("expected --no-score to override --score")
	}
}

func TestParseScoringOptionsConfigImpliesEnabled(t *testing.T) {
	cfg := scoring.DefaultConfig()
	data, _ := json.Marshal(cfg)
	f, err := os.CreateTemp(t.TempDir(), "scoring-*.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
	f.Close()

	opts, err := parseScoringOptions(false, false, f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !opts.enabled {
		t.Fatal("expected --scoring-config to imply enabled")
	}
}

func TestParseScoringOptionsConfigNoScoreWins(t *testing.T) {
	cfg := scoring.DefaultConfig()
	data, _ := json.Marshal(cfg)
	f, err := os.CreateTemp(t.TempDir(), "scoring-*.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
	f.Close()

	opts, err := parseScoringOptions(false, true, f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if opts.enabled {
		t.Fatal("expected --no-score to suppress even with --scoring-config")
	}
}

func TestParseScoringOptionsInvalidPath(t *testing.T) {
	_, err := parseScoringOptions(false, false, "/no/such/file.json")
	if err == nil {
		t.Fatal("expected error for missing config file")
	}
}
