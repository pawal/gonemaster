package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
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

// batchDiffFixture serves two batches whose runs are keyed by domain. It
// answers the runs listing per batch and each run's result, which is the
// pair of calls the cohort diff makes per domain.
func batchDiffFixture(t *testing.T, batches map[string]map[string][]jobResultEntry) func() {
	t.Helper()
	runs := map[string][]jobResultEntry{}
	for batch, byDomain := range batches {
		for domain, entries := range byDomain {
			runs[batch+":"+domain] = entries
		}
	}
	old := newHTTPClient
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			path := strings.TrimPrefix(r.URL.Path, "/api/v1")
			if path == "/runs" {
				batch := r.URL.Query().Get("batch")
				byDomain, ok := batches[batch]
				if !ok {
					return jsonResponse(200, `{"items":[],"total":0}`), nil
				}
				items := []runRecord{}
				domains := make([]string, 0, len(byDomain))
				for domain := range byDomain {
					domains = append(domains, domain)
				}
				sort.Strings(domains)
				for _, domain := range domains {
					items = append(items, runRecord{ID: batch + ":" + domain, Domain: domain, Status: "completed"})
				}
				body, _ := json.Marshal(runList{Items: items, Total: len(items)})
				return jsonResponse(200, string(body)), nil
			}
			id := strings.TrimPrefix(path, "/runs/")
			id = strings.TrimSuffix(id, "/result")
			id, _ = url.PathUnescape(id)
			entries, ok := runs[id]
			if !ok {
				return jsonResponse(404, `{"error":"not found"}`), nil
			}
			if strings.HasSuffix(path, "/result") {
				body, _ := json.Marshal(jobResult{JobID: id, Status: "completed", Raw: &jobResultRaw{Entries: entries}})
				return jsonResponse(200, string(body)), nil
			}
			domain := id[strings.Index(id, ":")+1:]
			body, _ := json.Marshal(runRecord{ID: id, Domain: domain, Status: "completed"})
			return jsonResponse(200, string(body)), nil
		})}
	}
	return func() { newHTTPClient = old }
}

// TestBatchesDiffRollsUpTagsAcrossDomains is the instrument the farm
// characterization reads: two runs of the same corpus under different load,
// answering "how many domains gained a finding, and which finding". A
// per-domain view alone cannot answer that, and a batch-level grade
// histogram cannot say which tag moved.
func TestBatchesDiffRollsUpTagsAcrossDomains(t *testing.T) {
	clean := []jobResultEntry{{Module: "BASIC", Tag: "B01_CHILD_FOUND", Level: "INFO"}}
	broke := []jobResultEntry{
		{Module: "BASIC", Tag: "B01_CHILD_FOUND", Level: "INFO"},
		{Module: "NAMESERVER", Tag: "N11_NO_RESPONSE", Level: "WARNING"},
	}
	defer batchDiffFixture(t, map[string]map[string][]jobResultEntry{
		"serial": {"a.example": clean, "b.example": clean, "c.example": clean},
		"w16":    {"a.example": broke, "b.example": broke, "c.example": clean},
	})()

	var out, errOut bytes.Buffer
	code := run([]string{"--format", "json", "batches", "diff", "serial", "w16"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("differing batches must exit 1, got %d: %s", code, errOut.String())
	}
	var diff batchDiffOutput
	if err := json.Unmarshal(out.Bytes(), &diff); err != nil {
		t.Fatalf("expected JSON output: %v (%s)", err, out.String())
	}
	if diff.Compared != 3 || diff.Identical != 1 || diff.Differing != 2 {
		t.Fatalf("compared/identical/differing = %d/%d/%d, want 3/1/2", diff.Compared, diff.Identical, diff.Differing)
	}
	// Two of three domains gained N11_NO_RESPONSE under load. That count is
	// the spurious-finding rate the gate is written against.
	if diff.TagAdded["N11_NO_RESPONSE"] != 2 {
		t.Fatalf("tag_added = %+v, want N11_NO_RESPONSE on 2 domains", diff.TagAdded)
	}
	if len(diff.TagRemoved) != 0 || len(diff.TagChanged) != 0 {
		t.Fatalf("unexpected removed/changed rollups: %+v %+v", diff.TagRemoved, diff.TagChanged)
	}
}

// TestBatchesDiffReportsUncomparableDomains keeps a batch that lost domains
// from quietly shrinking the denominator: a domain tested in only one arm
// cannot contribute a delta, and silently dropping it would make a run that
// failed to complete look cleaner than one that did.
func TestBatchesDiffReportsUncomparableDomains(t *testing.T) {
	clean := []jobResultEntry{{Module: "BASIC", Tag: "B01_CHILD_FOUND", Level: "INFO"}}
	defer batchDiffFixture(t, map[string]map[string][]jobResultEntry{
		"serial": {"a.example": clean, "gone.example": clean},
		"w16":    {"a.example": clean, "new.example": clean},
	})()

	var out, errOut bytes.Buffer
	code := run([]string{"--format", "json", "batches", "diff", "serial", "w16"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("no domain differs, expected exit 0, got %d: %s", code, errOut.String())
	}
	var diff batchDiffOutput
	if err := json.Unmarshal(out.Bytes(), &diff); err != nil {
		t.Fatalf("expected JSON output: %v", err)
	}
	if diff.Compared != 1 {
		t.Fatalf("compared = %d, want 1", diff.Compared)
	}
	if len(diff.OnlyInA) != 1 || diff.OnlyInA[0] != "gone.example" {
		t.Fatalf("only_in_a = %+v, want [gone.example]", diff.OnlyInA)
	}
	if len(diff.OnlyInB) != 1 || diff.OnlyInB[0] != "new.example" {
		t.Fatalf("only_in_b = %+v, want [new.example]", diff.OnlyInB)
	}
}

// TestBatchesDiffOmitsPerDomainListByDefault keeps a 500-domain comparison
// readable: the rollup is the answer, the per-domain list is the follow-up.
func TestBatchesDiffOmitsPerDomainListByDefault(t *testing.T) {
	clean := []jobResultEntry{{Module: "BASIC", Tag: "B01_CHILD_FOUND", Level: "INFO"}}
	broke := []jobResultEntry{{Module: "NAMESERVER", Tag: "N11_NO_RESPONSE", Level: "WARNING"}}
	defer batchDiffFixture(t, map[string]map[string][]jobResultEntry{
		"serial": {"a.example": clean},
		"w16":    {"a.example": broke},
	})()

	var out, errOut bytes.Buffer
	if code := run([]string{"--format", "json", "batches", "diff", "serial", "w16"}, &out, &errOut); code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	var diff batchDiffOutput
	if err := json.Unmarshal(out.Bytes(), &diff); err != nil {
		t.Fatalf("expected JSON output: %v", err)
	}
	if len(diff.DomainDeltas) != 0 {
		t.Fatalf("per-domain list must be opt-in, got %+v", diff.DomainDeltas)
	}

	out.Reset()
	errOut.Reset()
	if code := run([]string{"--format", "json", "batches", "diff", "--per-domain", "serial", "w16"}, &out, &errOut); code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if err := json.Unmarshal(out.Bytes(), &diff); err != nil {
		t.Fatalf("expected JSON output: %v", err)
	}
	if len(diff.DomainDeltas) != 1 || diff.DomainDeltas[0].Domain != "a.example" {
		t.Fatalf("--per-domain must include the delta list, got %+v", diff.DomainDeltas)
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
