package main

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/cmd/internal/clitest"
	"codeberg.org/pawal/gonemaster/internal/apitest"
)

func scheduleOpts() apitest.Opts {
	profileID := int64(4)
	return apitest.Opts{
		Cohorts: []apitest.AdminCohort{
			{ID: 1, SourceType: "tag", SourceTag: "tld", Label: "TLD", AnalysisEnabled: true},
			{ID: 2, SourceType: "tag", SourceTag: "gov", Label: "Government", AnalysisEnabled: true},
		},
		Schedules: []apitest.CohortSchedule{{
			CohortID: 2, SourceTag: "gov", Label: "Government", Enabled: true, Kind: "weekly",
			IntervalDays: 1, Weekdays: []string{"mon", "thu"}, DaysOfMonth: []int{}, TimeOfDay: "23:30",
			Timezone: "UTC", ProfileID: &profileID, CatchUp: true, Summary: "Weekly, Monday and Thursday, 23:30 UTC",
			NextRunAt: "2026-10-05T23:30:00Z", LastRunAt: "2026-10-01T23:30:00Z", LastBatchID: "batch_7",
			LastOutcome: "submitted",
		}},
		Profiles: []apitest.Profile{{ID: 9, Name: "strict"}},
	}
}

func TestCohortsSchedulesList(t *testing.T) {
	apitest.StubFake(t, &newHTTPClient, scheduleOpts())

	clitest.Run(t, run, "cohorts", "schedules").RequireCode(t, 0).RequireOutContains(t,
		"Schedules: 1",
		"gov",
		"Weekly, Monday and Thursday, 23:30 UTC",
		"next=2026-10-05T23:30:00Z",
		"last=2026-10-01T23:30:00Z submitted batch_7",
	)
}

func TestCohortsScheduleShow(t *testing.T) {
	apitest.StubFake(t, &newHTTPClient, scheduleOpts())

	clitest.Run(t, run, "cohorts", "schedule", "gov").RequireCode(t, 0).RequireOutContains(t,
		"Schedule of gov (enabled)",
		"Rule: Weekly, Monday and Thursday, 23:30 UTC",
		"Next: 2026-10-05T23:30:00Z",
		"Last: 2026-10-01T23:30:00Z submitted batch_7",
	)
	clitest.Run(t, run, "cohorts", "schedule", "tld").RequireCode(t, 2).RequireErrContains(t, "cohort has no schedule")
}

func TestCohortsScheduleShowJSON(t *testing.T) {
	apitest.StubFake(t, &newHTTPClient, scheduleOpts())

	res := clitest.Run(t, run, "--format", "json", "cohorts", "schedule", "gov").RequireCode(t, 0)
	var got apitest.CohortSchedule
	if err := json.Unmarshal([]byte(res.Out), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, res.Out)
	}
	if got.CohortID != 2 || got.Kind != "weekly" || got.LastBatchID != "batch_7" {
		t.Errorf("schedule = %+v, want cohort 2 weekly batch_7", got)
	}
}

func TestCohortsScheduleSet(t *testing.T) {
	gov := int64(4)
	strict := int64(9)
	today := time.Now().Format(time.DateOnly)
	for _, tc := range []struct {
		name string
		args []string
		out  string
		want apitest.CohortSchedule
	}{
		{
			name: "new monthly",
			args: []string{"tld", "--monthly", "1,15", "--at", "02:00", "--tz", "Europe/Stockholm"},
			out:  "Next: 2026-11-01T01:00:00Z",
			want: apitest.CohortSchedule{Enabled: true, Kind: "monthly", IntervalDays: 1, DaysOfMonth: []int{1, 15}, TimeOfDay: "02:00", Timezone: "Europe/Stockholm", CatchUp: true},
		},
		{
			name: "new last day",
			args: []string{"tld", "--monthly", "last", "--at", "02:00"},
			out:  "Next: 2026-11-01T01:00:00Z",
			want: apitest.CohortSchedule{Enabled: true, Kind: "monthly", IntervalDays: 1, DaysOfMonth: []int{}, LastDay: true, TimeOfDay: "02:00", Timezone: "UTC", CatchUp: true},
		},
		{
			name: "new interval",
			args: []string{"tld", "--every", "3", "--from", "2026-11-01", "--at", "02:00", "--profile", "strict", "--promote-default", "--no-catch-up"},
			out:  "Next: 2026-11-01T01:00:00Z",
			want: apitest.CohortSchedule{Enabled: true, Kind: "interval", IntervalDays: 3, AnchorDate: "2026-11-01", TimeOfDay: "02:00", Timezone: "UTC", ProfileID: &strict, PromoteDefault: true},
		},
		{
			name: "interval starts today",
			args: []string{"tld", "--every", "14", "--at", "02:00"},
			out:  "Next: 2026-11-01T01:00:00Z",
			want: apitest.CohortSchedule{Enabled: true, Kind: "interval", IntervalDays: 14, AnchorDate: today, TimeOfDay: "02:00", Timezone: "UTC", CatchUp: true},
		},
		{
			name: "pause keeps the rest",
			args: []string{"gov", "--disable"},
			out:  "Schedule of gov (paused)",
			want: apitest.CohortSchedule{Kind: "weekly", IntervalDays: 1, Weekdays: []string{"mon", "thu"}, DaysOfMonth: []int{}, TimeOfDay: "23:30", Timezone: "UTC", ProfileID: &gov, CatchUp: true},
		},
		{
			name: "clear the profile",
			args: []string{"gov", "--profile="},
			out:  "Next: 2026-11-01T01:00:00Z",
			want: apitest.CohortSchedule{Enabled: true, Kind: "weekly", IntervalDays: 1, Weekdays: []string{"mon", "thu"}, DaysOfMonth: []int{}, TimeOfDay: "23:30", Timezone: "UTC", CatchUp: true},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var put apitest.CohortSchedule
			opts := scheduleOpts()
			opts.SchedulePut = &put
			apitest.StubFake(t, &newHTTPClient, opts)

			clitest.Run(t, run, append([]string{"cohorts", "schedule", "set"}, tc.args...)...).
				RequireCode(t, 0).RequireOutContains(t, tc.out)

			if !sameSchedule(put, tc.want) {
				t.Errorf("PUT body = %+v, want %+v", put, tc.want)
			}
		})
	}
}

func sameSchedule(a, b apitest.CohortSchedule) bool {
	samePtr := (a.ProfileID == nil) == (b.ProfileID == nil) && (a.ProfileID == nil || *a.ProfileID == *b.ProfileID)
	return samePtr && a.Enabled == b.Enabled && a.Kind == b.Kind && a.IntervalDays == b.IntervalDays &&
		a.AnchorDate == b.AnchorDate && slices.Equal(a.Weekdays, b.Weekdays) && slices.Equal(a.DaysOfMonth, b.DaysOfMonth) &&
		a.LastDay == b.LastDay && a.TimeOfDay == b.TimeOfDay && a.Timezone == b.Timezone &&
		a.PromoteDefault == b.PromoteDefault && a.CatchUp == b.CatchUp && a.CohortID == 0 && a.Summary == ""
}

func TestCohortsScheduleSetRejects(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"two kinds", []string{"tld", "--monthly", "1", "--weekly", "mon"}, "use one of --monthly, --weekly and --every"},
		{"both toggles", []string{"tld", "--enable", "--disable"}, "--enable and --disable are mutually exclusive"},
		{"bad day", []string{"tld", "--monthly", "1,first"}, `--monthly takes days 1 to 28 and last, not "first"`},
		{"unknown cohort", []string{"nope", "--monthly", "1"}, `no cohort has source tag "nope"`},
		{"unknown profile", []string{"tld", "--profile", "lax"}, `no stored profile named "lax"`},
		{"no tag", []string{"--monthly", "1"}, "cohorts schedule set requires one cohort tag"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var put apitest.CohortSchedule
			opts := scheduleOpts()
			opts.SchedulePut = &put
			apitest.StubFake(t, &newHTTPClient, opts)

			clitest.Run(t, run, append([]string{"cohorts", "schedule", "set"}, tc.args...)...).
				RequireCode(t, 2).RequireErrContains(t, tc.want)
			if put.Kind != "" {
				t.Errorf("rejected set sent a PUT: %+v", put)
			}
		})
	}
}

func TestCohortsScheduleRemove(t *testing.T) {
	var deletes []string
	opts := scheduleOpts()
	opts.ScheduleDeletes = &deletes
	apitest.StubFake(t, &newHTTPClient, opts)

	clitest.Run(t, run, "cohorts", "schedule", "remove", "gov").RequireCode(t, 0).RequireOutContains(t, "Removed the schedule of gov")
	if !slices.Equal(deletes, []string{"2"}) {
		t.Errorf("DELETE cohort ids = %v, want [2]", deletes)
	}
}
