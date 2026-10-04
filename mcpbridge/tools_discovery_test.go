package mcpbridge

import (
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/internal/apitest"
)

func TestProfileList(t *testing.T) {
	api := fakeAPI(t, apitest.Opts{Profiles: []apitest.Profile{{ID: 1, Name: "default", Public: true}, {ID: 2, Name: "strict"}}})
	var out profileListOutput
	res := callTool(t, api, "profile_list", map[string]any{}, &out)
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", errorText(res))
	}
	if out.Count != 2 || out.Profiles[0].ID != 1 || out.Profiles[0].Name != "default" || !out.Profiles[0].Public {
		t.Errorf("profiles = %+v", out)
	}
}

func TestDomainTagList(t *testing.T) {
	pid := int64(2)
	api := fakeAPI(t, apitest.Opts{Tags: []apitest.Tag{{Name: "se-weekly", DomainCount: 1437, DefaultProfileID: &pid}}})
	var out domainTagListOutput
	res := callTool(t, api, "domain_tag_list", map[string]any{}, &out)
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", errorText(res))
	}
	if out.Count != 1 || out.Tags[0].Name != "se-weekly" || out.Tags[0].DomainCount != 1437 || out.Tags[0].DefaultProfileID == nil || *out.Tags[0].DefaultProfileID != 2 {
		t.Errorf("tags = %+v", out)
	}
}

func TestCohortListWithoutTag(t *testing.T) {
	api := fakeAPI(t, apitest.Opts{AnalysisCatalog: &apitest.AnalysisCatalog{
		DefaultTag: "se",
		Cohorts:    []apitest.AnalysisCohortView{{DatasetTag: "se", Label: "Sweden", IsDefault: true}, {DatasetTag: "nu"}},
	}})
	var out cohortListOutput
	res := callTool(t, api, "cohort_list", map[string]any{}, &out)
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", errorText(res))
	}
	if out.DefaultTag != "se" || len(out.Cohorts) != 2 || !out.Cohorts[0].IsDefault {
		t.Errorf("cohorts = %+v", out)
	}
	if out.Snapshots != nil || out.DatasetTag != "" {
		t.Errorf("no snapshots expected without dataset_tag: %+v", out)
	}
}

func TestCohortListWithSnapshots(t *testing.T) {
	api := fakeAPI(t, apitest.Opts{
		AnalysisCatalog: &apitest.AnalysisCatalog{Cohorts: []apitest.AnalysisCohortView{{DatasetTag: "se"}}},
		AnalysisSnapshots: &apitest.AnalysisSnapshotList{DatasetTag: "se", Snapshots: []apitest.AnalysisSnapshot{
			{Slug: "2026-09-28", CapturedAt: time.Unix(1_700_000_000, 0), DomainCount: 1437},
			{Slug: "2026-09-21", DomainCount: 1430},
		}},
	})
	var out cohortListOutput
	res := callTool(t, api, "cohort_list", map[string]any{"dataset_tag": "se"}, &out)
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", errorText(res))
	}
	if out.DatasetTag != "se" || len(out.Snapshots) != 2 || out.Snapshots[0].Slug != "2026-09-28" || out.Snapshots[0].DomainCount != 1437 {
		t.Errorf("snapshots = %+v", out)
	}
	if out.Snapshots[0].CapturedAt == "" || out.Snapshots[1].CapturedAt != "" {
		t.Errorf("captured_at = %q / %q", out.Snapshots[0].CapturedAt, out.Snapshots[1].CapturedAt)
	}
}

func TestCohortScheduleList(t *testing.T) {
	api := fakeAPI(t, apitest.Opts{Schedules: []apitest.CohortSchedule{
		{CohortID: 1, SourceTag: "tld", Label: "TLD", Enabled: true, Summary: "Monthly, days 1 and 15, 02:00 Europe/Stockholm",
			NextRunAt: "2026-11-01T01:00:00Z", LastRunAt: "2026-10-15T00:00:00Z", LastOutcome: "submitted", LastBatchID: "batch_9"},
		{CohortID: 2, SourceTag: "gov", Summary: "Weekly, Monday, 23:30 UTC", NextRunAt: "2026-10-05T23:30:00Z"},
	}})
	var out cohortScheduleListOutput
	res := callTool(t, api, "cohort_schedule_list", map[string]any{}, &out)
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", errorText(res))
	}
	want := cohortScheduleOut{DatasetTag: "tld", Label: "TLD", Enabled: true, Summary: "Monthly, days 1 and 15, 02:00 Europe/Stockholm",
		NextRunAt: "2026-11-01T01:00:00Z", LastRunAt: "2026-10-15T00:00:00Z", LastOutcome: "submitted", LastBatchID: "batch_9"}
	if out.Count != 2 || len(out.Schedules) != 2 || out.Schedules[0] != want {
		t.Fatalf("schedules = %+v", out)
	}
	if got := out.Schedules[1]; got.DatasetTag != "gov" || got.Enabled || got.LastOutcome != "" {
		t.Errorf("paused schedule = %+v", got)
	}
}

func TestCohortScheduleListEmpty(t *testing.T) {
	api := fakeAPI(t, apitest.Opts{})
	var out cohortScheduleListOutput
	res := callTool(t, api, "cohort_schedule_list", map[string]any{}, &out)
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", errorText(res))
	}
	if out.Count != 0 || out.Schedules == nil {
		t.Errorf("schedules = %+v, want an empty list", out)
	}
}
