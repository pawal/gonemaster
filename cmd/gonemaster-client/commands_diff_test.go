package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// diffFixture serves two runs whose entry sets the caller supplies. The
// server exposes each run twice: /runs/<id> for the metadata (domain name)
// and /runs/<id>/result for the entries, which is exactly the pair the diff
// command fetches per run.
func diffFixture(t *testing.T, entriesByRun map[string][]jobResultEntry) func() {
	t.Helper()
	old := newHTTPClient
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			path := strings.TrimPrefix(r.URL.Path, "/api/v1")
			id := strings.TrimPrefix(path, "/runs/")
			id = strings.TrimSuffix(id, "/result")
			entries, ok := entriesByRun[id]
			if !ok {
				return jsonResponse(404, `{"error":"not found"}`), nil
			}
			if strings.HasSuffix(path, "/result") {
				body, _ := json.Marshal(jobResult{
					JobID:  id,
					Status: "completed",
					Raw:    &jobResultRaw{Entries: entries},
				})
				return jsonResponse(200, string(body)), nil
			}
			body, _ := json.Marshal(runRecord{ID: id, Domain: "example.com", Status: "completed"})
			return jsonResponse(200, string(body)), nil
		})}
	}
	return func() { newHTTPClient = old }
}

// TestRunsDiffIdenticalRunsExitZero is the shape the zero-delta gates use:
// two runs of the same domain under different knob settings must produce
// byte-identical tag/severity sets, and the command has to say so with an
// exit code a shell loop can test.
func TestRunsDiffIdenticalRunsExitZero(t *testing.T) {
	entries := []jobResultEntry{
		{Module: "BASIC", Tag: "B01_CHILD_FOUND", Level: "INFO"},
		{Module: "NAMESERVER", Tag: "N01_NO_RESPONSE", Level: "WARNING"},
	}
	defer diffFixture(t, map[string][]jobResultEntry{"a": entries, "b": entries})()

	var out, errOut bytes.Buffer
	code := run([]string{"runs", "diff", "a", "b"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("identical runs must exit 0, got %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "No tag or severity changes") {
		t.Fatalf("expected a no-change line, got: %s", out.String())
	}
}

// TestRunsDiffReportsAddedRemovedChanged covers the three delta kinds and
// the non-zero exit that makes the command usable as a gate. The severity
// change is the subtle one: the tag is present in both runs, so a naive
// set difference would call the runs identical.
func TestRunsDiffReportsAddedRemovedChanged(t *testing.T) {
	before := []jobResultEntry{
		{Module: "BASIC", Tag: "B01_CHILD_FOUND", Level: "INFO"},
		{Module: "NAMESERVER", Tag: "N11_NO_RESPONSE", Level: "WARNING"},
		{Module: "CONSISTENCY", Tag: "ONE_SOA_SERIAL", Level: "INFO"},
	}
	after := []jobResultEntry{
		{Module: "BASIC", Tag: "B01_CHILD_FOUND", Level: "INFO"},
		{Module: "CONSISTENCY", Tag: "ONE_SOA_SERIAL", Level: "NOTICE"},
		{Module: "CONSISTENCY", Tag: "MULTIPLE_SOA_SERIALS", Level: "ERROR"},
	}
	defer diffFixture(t, map[string][]jobResultEntry{"a": before, "b": after})()

	var out, errOut bytes.Buffer
	code := run([]string{"--format", "json", "runs", "diff", "a", "b"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("differing runs must exit 1, got %d: %s", code, errOut.String())
	}
	var diff runDiffOutput
	if err := json.Unmarshal(out.Bytes(), &diff); err != nil {
		t.Fatalf("expected JSON output: %v (%s)", err, out.String())
	}
	if len(diff.Added) != 1 || diff.Added[0].Tag != "MULTIPLE_SOA_SERIALS" {
		t.Fatalf("added = %+v, want just MULTIPLE_SOA_SERIALS", diff.Added)
	}
	if len(diff.Removed) != 1 || diff.Removed[0].Tag != "N11_NO_RESPONSE" {
		t.Fatalf("removed = %+v, want just N11_NO_RESPONSE", diff.Removed)
	}
	if len(diff.Changed) != 1 || diff.Changed[0].Tag != "ONE_SOA_SERIAL" {
		t.Fatalf("changed = %+v, want just ONE_SOA_SERIAL", diff.Changed)
	}
	if diff.Changed[0].FromLevel != "INFO" || diff.Changed[0].ToLevel != "NOTICE" {
		t.Fatalf("changed levels = %+v, want INFO -> NOTICE", diff.Changed[0])
	}
}

// TestRunsDiffQuietSuppressesOutput keeps the gate usable inside a matrix
// script, where only the exit status matters and per-domain output would
// bury the summary.
func TestRunsDiffQuietSuppressesOutput(t *testing.T) {
	before := []jobResultEntry{{Module: "BASIC", Tag: "B01_CHILD_FOUND", Level: "INFO"}}
	after := []jobResultEntry{{Module: "BASIC", Tag: "B01_CHILD_FOUND", Level: "ERROR"}}
	defer diffFixture(t, map[string][]jobResultEntry{"a": before, "b": after})()

	var out, errOut bytes.Buffer
	code := run([]string{"runs", "diff", "--quiet", "a", "b"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d: %s", code, errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("quiet mode must print nothing, got: %s", out.String())
	}
}

// TestWorstLevelByTagKeepsHighestSeverity pins the collapse rule shared with
// the MCP run_diff tool. A tag emitted several times in one run (once per
// nameserver, typically) must be represented by its worst level, or a run
// where one address degraded would look unchanged as long as another
// address still emitted the tag at the old level.
func TestWorstLevelByTagKeepsHighestSeverity(t *testing.T) {
	result := jobResult{Raw: &jobResultRaw{Entries: []jobResultEntry{
		{Module: "NAMESERVER", Tag: "N11_NO_RESPONSE", Level: "INFO"},
		{Module: "NAMESERVER", Tag: "N11_NO_RESPONSE", Level: "ERROR"},
		{Module: "NAMESERVER", Tag: "N11_NO_RESPONSE", Level: "NOTICE"},
	}}}
	got := worstLevelByTag(result)
	if got["N11_NO_RESPONSE"].Level != "ERROR" {
		t.Fatalf("worst level = %q, want ERROR", got["N11_NO_RESPONSE"].Level)
	}
	if got["N11_NO_RESPONSE"].Module != "NAMESERVER" {
		t.Fatalf("module = %q, want NAMESERVER", got["N11_NO_RESPONSE"].Module)
	}
}

// TestWorstLevelByTagEmptyResult guards the missing-raw case: a run fetched
// without entries must yield an empty map rather than panic, since a diff
// against a still-running or purged run is an ordinary operator mistake.
func TestWorstLevelByTagEmptyResult(t *testing.T) {
	if got := worstLevelByTag(jobResult{}); len(got) != 0 {
		t.Fatalf("expected empty map for a result with no raw entries, got %+v", got)
	}
}

// TestRunsDiffRequiresTwoRunIDs keeps the argument error distinct from the
// "runs differ" exit, so a scripted gate cannot mistake a typo for a real
// finding delta.
func TestRunsDiffRequiresTwoRunIDs(t *testing.T) {
	defer diffFixture(t, map[string][]jobResultEntry{})()
	var out, errOut bytes.Buffer
	if code := run([]string{"runs", "diff", "only-one"}, &out, &errOut); code != 2 {
		t.Fatalf("expected usage exit 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "two run IDs are required") {
		t.Fatalf("expected an argument error, got: %s", errOut.String())
	}
}
