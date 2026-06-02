package main

import (
	"net/url"
	"testing"
	"time"
)

func TestBatchGet(t *testing.T) {
	fin := time.Unix(5000, 0)
	ts := newFakeServer(t, fakeOpts{batch: &batchSummaryView{
		BatchID:      "b1",
		Tag:          "se-weekly",
		Total:        3,
		StatusCounts: map[string]int{"succeeded": 3},
		Grades:       map[string]int{"A": 2, "C": 1},
		CreatedAt:    time.Unix(1000, 0),
		FinishedAt:   &fin,
	}})
	defer ts.Close()

	var out batchGetOutput
	res := callTool(t, clientFor(t, ts.URL, ""), "batch_get", map[string]any{"batch_id": "b1"}, &out)
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", errorText(res))
	}
	if out.Total != 3 || out.Tag != "se-weekly" {
		t.Errorf("batch fields wrong: %+v", out)
	}
	if !out.Done {
		t.Errorf("expected done=true with no queued/running")
	}
	if out.StatusCounts["succeeded"] != 3 {
		t.Errorf("status counts wrong: %+v", out.StatusCounts)
	}
}

func TestBatchGetNotDoneWhenRunning(t *testing.T) {
	ts := newFakeServer(t, fakeOpts{batch: &batchSummaryView{
		BatchID:      "b2",
		Total:        2,
		StatusCounts: map[string]int{"running": 1, "succeeded": 1},
	}})
	defer ts.Close()
	var out batchGetOutput
	callTool(t, clientFor(t, ts.URL, ""), "batch_get", map[string]any{"batch_id": "b2"}, &out)
	if out.Done {
		t.Errorf("expected done=false while a job is running")
	}
}

func TestCohortStats(t *testing.T) {
	ts := newFakeServer(t, fakeOpts{
		batch: &batchSummaryView{BatchID: "b1", Total: 3, Grades: map[string]int{"A": 2, "C": 1}},
		runs: []runView{
			{ID: "r1", WorstLevel: "INFO"},
			{ID: "r2", WorstLevel: "WARNING"},
			{ID: "r3", WorstLevel: "WARNING"},
		},
	})
	defer ts.Close()

	var out cohortStatsOutput
	res := callTool(t, clientFor(t, ts.URL, ""), "cohort_stats", map[string]any{"batch_id": "b1"}, &out)
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", errorText(res))
	}
	if out.Grades["A"] != 2 || out.Grades["C"] != 1 {
		t.Errorf("grades wrong: %+v", out.Grades)
	}
	if out.WorstLevels["WARNING"] != 2 || out.WorstLevels["INFO"] != 1 {
		t.Errorf("worst levels wrong: %+v", out.WorstLevels)
	}
	if out.Total != 3 {
		t.Errorf("total = %d, want 3", out.Total)
	}
}

func TestCohortTagValues(t *testing.T) {
	avg := 92.5
	var captured url.Values
	ts := newFakeServer(t, fakeOpts{
		tagValuesQuery: &captured,
		tagValues: &batchTagValuesView{
			BatchID: "b1", Tag: "IPV4_ONE_ASN", Arg: "asn", MinCount: 1, WeightByScore: true,
			Values: []tagValueRollupView{
				{Value: "13335", Count: 12, AvgScore: &avg, SampleDomains: []string{"a", "b"}},
			},
		},
	})
	defer ts.Close()

	var out cohortTagValuesOutput
	res := callTool(t, clientFor(t, ts.URL, ""), "cohort_tag_values", map[string]any{
		"batch_id": "b1", "tag": "IPV4_ONE_ASN", "arg": "asn", "weight_by_score": true,
	}, &out)
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", errorText(res))
	}
	// The tool must forward tag, arg, and weight_by_score to the server.
	if captured.Get("tag") != "IPV4_ONE_ASN" || captured.Get("arg") != "asn" || captured.Get("weight_by_score") != "true" {
		t.Errorf("forwarded query wrong: %v", captured)
	}
	if len(out.Values) != 1 {
		t.Fatalf("expected one value row, got %+v", out.Values)
	}
	v := out.Values[0]
	if v.Value != "13335" || v.Count != 12 || v.AvgScore == nil || *v.AvgScore != 92.5 {
		t.Errorf("value row wrong: %+v", v)
	}
}

func TestCohortTagValuesRequiresTagAndArg(t *testing.T) {
	ts := newFakeServer(t, fakeOpts{})
	defer ts.Close()
	var out cohortTagValuesOutput
	res := callTool(t, clientFor(t, ts.URL, ""), "cohort_tag_values", map[string]any{"batch_id": "b1", "arg": "asn"}, &out)
	if !res.IsError {
		t.Errorf("expected an error when tag is missing")
	}
}

func TestFailuresByTag(t *testing.T) {
	ts := newFakeServer(t, fakeOpts{entries: []entryRecord{
		{Domain: "a.example", Module: "consistency", Tag: "SOATIME", Level: "WARNING"},
		{Domain: "b.example", Module: "consistency", Tag: "SOATIME", Level: "WARNING"},
		{Domain: "c.example", Module: "dnssec", Tag: "DNSSEC09", Level: "ERROR"},
		// Below the default WARNING floor; must be excluded.
		{Domain: "d.example", Module: "nameserver", Tag: "NS01", Level: "INFO"},
	}})
	defer ts.Close()

	var out failuresByTagOutput
	res := callTool(t, clientFor(t, ts.URL, ""), "failures_by_tag", map[string]any{"batch_id": "b1"}, &out)
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", errorText(res))
	}
	if out.SeverityMin != "WARNING" {
		t.Errorf("severity_min = %q, want WARNING", out.SeverityMin)
	}
	if len(out.Tags) != 2 {
		t.Fatalf("expected 2 tags above WARNING, got %d: %+v", len(out.Tags), out.Tags)
	}
	// SOATIME (2) ranks above DNSSEC09 (1).
	if out.Tags[0].Tag != "SOATIME" || out.Tags[0].Count != 2 {
		t.Errorf("top tag wrong: %+v", out.Tags[0])
	}
	if len(out.Tags[0].ExampleDomains) != 2 {
		t.Errorf("expected 2 example domains for SOATIME, got %v", out.Tags[0].ExampleDomains)
	}
	for _, tg := range out.Tags {
		if tg.Tag == "NS01" {
			t.Errorf("INFO-level tag NS01 must be excluded at WARNING floor")
		}
	}
}

func TestFailuresByTagSeverityFloor(t *testing.T) {
	ts := newFakeServer(t, fakeOpts{entries: []entryRecord{
		{Domain: "a.example", Tag: "DNSSEC09", Level: "ERROR"},
		{Domain: "b.example", Tag: "SOATIME", Level: "WARNING"},
	}})
	defer ts.Close()

	var out failuresByTagOutput
	callTool(t, clientFor(t, ts.URL, ""), "failures_by_tag", map[string]any{"batch_id": "b1", "severity_min": "ERROR"}, &out)
	if len(out.Tags) != 1 || out.Tags[0].Tag != "DNSSEC09" {
		t.Errorf("ERROR floor should keep only DNSSEC09, got %+v", out.Tags)
	}
}
