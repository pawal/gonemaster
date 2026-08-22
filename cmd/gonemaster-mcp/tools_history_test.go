package main

import (
	"net/url"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/internal/apitest"
)

func TestRunSearchForwardsFiltersAndMaps(t *testing.T) {
	grade := "B"
	score := 80
	var captured url.Values
	api := fakeAPI(t, apitest.Opts{
		RunsQuery: &captured,
		Runs: []apitest.Run{
			{ID: "run_2", Domain: "x.example", Status: "succeeded", Grade: &grade, Score: &score, WorstLevel: "WARNING", DurationMs: 500, FinishedAt: time.Unix(2000, 0)},
		},
	})

	var out runSearchOutput
	res := callTool(t, api, "run_search", map[string]any{
		"domain": "x.example",
		"tag":    "SOATIME",
		"level":  "WARNING",
		"limit":  float64(10),
	}, &out)
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", errorText(res))
	}
	if out.Count != 1 || out.Runs[0].RunID != "run_2" || out.Runs[0].Grade != "B" {
		t.Errorf("mapping wrong: %+v", out)
	}
	// Filters must reach the server as query params. The tool's tag input maps
	// to the server's event_tag (log-event tag), not the domain-tag filter.
	if captured.Get("domain") != "x.example" || captured.Get("event_tag") != "SOATIME" || captured.Get("level") != "WARNING" {
		t.Errorf("filters not forwarded: %v", captured)
	}
	if captured.Get("tag") != "" {
		t.Errorf("tag input must not forward to the domain-tag filter, got tag=%q", captured.Get("tag"))
	}
	if captured.Get("limit") != "10" {
		t.Errorf("limit = %q, want 10", captured.Get("limit"))
	}
}

func TestRunSearchDefaultLimit(t *testing.T) {
	var captured url.Values
	api := fakeAPI(t, apitest.Opts{RunsQuery: &captured})

	var out runSearchOutput
	callTool(t, api, "run_search", map[string]any{"domain": "d"}, &out)
	if captured.Get("limit") != "20" {
		t.Errorf("default limit = %q, want 20", captured.Get("limit"))
	}
}

func TestRunDiff(t *testing.T) {
	// run_a: NS01 (INFO), SOATIME (NOTICE), GONE (WARNING)
	// run_b: NS01 (INFO), SOATIME (WARNING), NEW (ERROR)
	api := fakeAPI(t, apitest.Opts{ResultsByID: map[string]apitest.Result{
		"a": {
			JobID: "a", Status: "succeeded", Score: &apitest.Score{Score: 90, Grade: "A"},
			Raw: &apitest.ResultRaw{Entries: []apitest.Entry{
				{Module: "nameserver", Tag: "NS01", Level: "INFO"},
				{Module: "consistency", Tag: "SOATIME", Level: "NOTICE"},
				{Module: "zone", Tag: "GONE", Level: "WARNING"},
			}},
		},
		"b": {
			JobID: "b", Status: "succeeded", Score: &apitest.Score{Score: 70, Grade: "C"},
			Raw: &apitest.ResultRaw{Entries: []apitest.Entry{
				{Module: "nameserver", Tag: "NS01", Level: "INFO"},
				{Module: "consistency", Tag: "SOATIME", Level: "WARNING"},
				{Module: "zone", Tag: "NEW", Level: "ERROR"},
			}},
		},
	}})

	var out runDiffOutput
	res := callTool(t, api, "run_diff", map[string]any{"run_a": "a", "run_b": "b"}, &out)
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", errorText(res))
	}
	if out.GradeA != "A" || out.GradeB != "C" {
		t.Errorf("grades wrong: %s -> %s", out.GradeA, out.GradeB)
	}
	if len(out.Added) != 1 || out.Added[0].Tag != "NEW" || out.Added[0].Level != "ERROR" {
		t.Errorf("added wrong: %+v", out.Added)
	}
	if len(out.Removed) != 1 || out.Removed[0].Tag != "GONE" {
		t.Errorf("removed wrong: %+v", out.Removed)
	}
	if len(out.Changed) != 1 || out.Changed[0].Tag != "SOATIME" || out.Changed[0].FromLevel != "NOTICE" || out.Changed[0].ToLevel != "WARNING" {
		t.Errorf("changed wrong: %+v", out.Changed)
	}
}

func TestRunDiffRequiresBothIDs(t *testing.T) {
	api := fakeAPI(t, apitest.Opts{})
	res := callTool(t, api, "run_diff", map[string]any{"run_a": "a"}, nil)
	if !res.IsError {
		t.Fatalf("expected error when run_b missing")
	}
}
