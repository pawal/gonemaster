package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/cmd/internal/clitest"
	"codeberg.org/pawal/gonemaster/internal/apitest"
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

func mockJobAndResult(jobID, domain, resultBody string) apitest.RoundTripFunc {
	return func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/v1/jobs/" + jobID:
			body := `{"id":"` + jobID + `","domain":"` + domain + `","status":"succeeded","created_at":"2026-01-01T00:00:00Z","progress":100}`
			return apitest.JSONResponse(http.StatusOK, body), nil
		case "/api/v1/jobs/" + jobID + "/result":
			return apitest.JSONResponse(http.StatusOK, resultBody), nil
		default:
			return apitest.JSONResponse(http.StatusNotFound, `{"error":{"code":"not_found","message":"not found"}}`), nil
		}
	}
}

// ── --score flag ──────────────────────────────────────────────────────────────

func TestJobsResultsNoScoreByDefault(t *testing.T) {
	apitest.StubClient(t, &newHTTPClient, mockJobAndResult("job_1", "example.se", jobResultWithEntries("job_1")))
	res := clitest.Run(t, run, "jobs", "results", "job_1")
	res.RequireCode(t, 0)
	if strings.Contains(res.Out, "Score:") {
		t.Fatalf("expected no score by default, got: %s", res.Out)
	}
}

func TestJobsResultsWithScore(t *testing.T) {
	apitest.StubClient(t, &newHTTPClient, mockJobAndResult("job_1", "example.se", jobResultWithEntries("job_1")))
	res := clitest.Run(t, run, "jobs", "results", "--score", "job_1")
	res.RequireCode(t, 0)
	res.RequireOutContains(t, "Score:")
}

func TestJobsResultsNoScoreSuppresses(t *testing.T) {
	apitest.StubClient(t, &newHTTPClient, mockJobAndResult("job_1", "example.se", jobResultWithEntries("job_1")))
	// --no-score should suppress even if both flags are given.
	res := clitest.Run(t, run, "jobs", "results", "--score", "--no-score", "job_1")
	res.RequireCode(t, 0)
	if strings.Contains(res.Out, "Score:") {
		t.Fatalf("expected no score with --no-score, got: %s", res.Out)
	}
}

func TestJobsResultsScoreShowsCategories(t *testing.T) {
	apitest.StubClient(t, &newHTTPClient, mockJobAndResult("job_1", "example.se", jobResultWithEntries("job_1")))
	res := clitest.Run(t, run, "jobs", "results", "--score", "job_1")
	res.RequireCode(t, 0)
	output := res.Out
	for _, cat := range []string{"dnssec:", "nameserver_health:", "connectivity:", "zone_consistency:"} {
		if !strings.Contains(output, cat) {
			t.Errorf("expected category %q in output, got:\n%s", cat, output)
		}
	}
}

func TestJobsResultsScoreNotAvailableWithoutEntries(t *testing.T) {
	apitest.StubClient(t, &newHTTPClient, mockJobAndResult("job_1", "example.se", jobResultWithoutEntries("job_1")))
	res := clitest.Run(t, run, "jobs", "results", "--score", "job_1")
	res.RequireCode(t, 0)
	res.RequireOutContains(t, "N/A")
}

// ── --scoring-config flag ─────────────────────────────────────────────────────

func TestJobsResultsScoringConfigImpliesScore(t *testing.T) {
	// Write a minimal valid scoring config to a temp file.
	cfgPath := clitest.WriteScoringConfig(t)

	apitest.StubClient(t, &newHTTPClient, mockJobAndResult("job_1", "example.se", jobResultWithEntries("job_1")))
	res := clitest.Run(t, run, "jobs", "results", "--scoring-config", cfgPath, "job_1")
	res.RequireCode(t, 0)
	res.RequireOutContains(t, "Score:")
}

func TestJobsResultsScoringConfigInvalidPath(t *testing.T) {
	res := clitest.Run(t, run, "jobs", "results", "--scoring-config", "/nonexistent/path.json", "job_1")
	if res.Code == 0 {
		t.Fatal("expected non-zero exit for invalid --scoring-config path")
	}
}

func TestScoringConfigNoScoreWins(t *testing.T) {
	cfgPath := clitest.WriteScoringConfig(t)

	apitest.StubClient(t, &newHTTPClient, mockJobAndResult("job_1", "example.se", jobResultWithEntries("job_1")))
	// --no-score should override --scoring-config.
	res := clitest.Run(t, run, "jobs", "results", "--scoring-config", cfgPath, "--no-score", "job_1")
	res.RequireCode(t, 0)
	if strings.Contains(res.Out, "Score:") {
		t.Fatalf("expected --no-score to suppress scoring config, got: %s", res.Out)
	}
}

// ── JSON output ───────────────────────────────────────────────────────────────

func TestJobsResultsScoreInJSONOutput(t *testing.T) {
	apitest.StubClient(t, &newHTTPClient, mockJobAndResult("job_1", "example.se", jobResultWithEntries("job_1")))
	res := clitest.Run(t, run, "--format", "json", "jobs", "results", "--score", "job_1")
	res.RequireCode(t, 0)
	var result jobResult
	if err := json.Unmarshal([]byte(res.Out), &result); err != nil {
		t.Fatalf("invalid JSON output: %v - got: %s", err, res.Out)
	}
	if result.Score == nil {
		t.Fatalf("expected score field in JSON output, got: %s", res.Out)
	}
	if result.Score.Grade == "" {
		t.Fatalf("expected non-empty grade in JSON score")
	}
}

func TestJobsResultsNoScoreInJSONOutputByDefault(t *testing.T) {
	apitest.StubClient(t, &newHTTPClient, mockJobAndResult("job_1", "example.se", jobResultWithEntries("job_1")))
	res := clitest.Run(t, run, "--format", "json", "jobs", "results", "job_1")
	res.RequireCode(t, 0)
	var result jobResult
	if err := json.Unmarshal([]byte(res.Out), &result); err != nil {
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
	cfgPath := clitest.WriteScoringConfig(t)

	opts, err := parseScoringOptions(false, false, cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !opts.enabled {
		t.Fatal("expected --scoring-config to imply enabled")
	}
}

func TestParseScoringOptionsConfigNoScoreWins(t *testing.T) {
	cfgPath := clitest.WriteScoringConfig(t)

	opts, err := parseScoringOptions(false, true, cfgPath)
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
