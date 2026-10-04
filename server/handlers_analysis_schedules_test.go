package server

import (
	"fmt"
	"net/http"
	"slices"
	"testing"
	"testing/synctest"
	"time"
)

func newScheduleAPIServer(t *testing.T) (*Server, AnalysisCohort) {
	t.Helper()
	srv := newTestServer(t, withAnalysisController(newSpyAnalysisController()))
	return srv, upsertCohort(t, srv.store, "tld")
}

func schedulePath(id int64) string {
	return fmt.Sprintf("/api/v1/analysis/cohorts/%d/schedule", id)
}

func monthlyScheduleBody() map[string]any {
	return map[string]any{"kind": "monthly", "days_of_month": []int{1, 15}, "time_of_day": "02:00", "timezone": "Europe/Stockholm"}
}

func TestPutAnalysisCohortScheduleValidation(t *testing.T) {
	srv, cohort := newScheduleAPIServer(t)
	for _, tc := range []struct {
		name    string
		body    map[string]any
		code    string
		message string
	}{
		{"unknown kind", map[string]any{"kind": "cron", "time_of_day": "02:00"}, "invalid_schedule", "kind must be interval, weekly or monthly"},
		{"interval zero", map[string]any{"kind": "interval", "interval_days": 0, "anchor_date": "2026-11-01", "time_of_day": "02:00"}, "invalid_schedule", "interval must be 1 to 365 days"},
		{"interval without start", map[string]any{"kind": "interval", "interval_days": 3, "time_of_day": "02:00"}, "invalid_schedule", "start date must be a valid YYYY-MM-DD date"},
		{"empty weekdays", map[string]any{"kind": "weekly", "weekdays": []string{}, "time_of_day": "02:00"}, "invalid_schedule", "weekdays must name at least one day of mon, tue, wed, thu, fri, sat, sun"},
		{"unknown weekday", map[string]any{"kind": "weekly", "weekdays": []string{"monday"}, "time_of_day": "02:00"}, "invalid_schedule", "weekdays must name at least one day of mon, tue, wed, thu, fri, sat, sun"},
		{"empty days", map[string]any{"kind": "monthly", "time_of_day": "02:00"}, "invalid_schedule", "days of month must be 1 to 28, at least one day or the last day"},
		{"day 29", map[string]any{"kind": "monthly", "days_of_month": []int{29}, "time_of_day": "02:00"}, "invalid_schedule", "days of month must be 1 to 28, at least one day or the last day"},
		{"bad time", map[string]any{"kind": "monthly", "last_day": true, "time_of_day": "25:00"}, "invalid_schedule", "time of day must be HH:MM, 00:00 to 23:59"},
		{"unknown zone", map[string]any{"kind": "monthly", "last_day": true, "time_of_day": "02:00", "timezone": "Mars/Olympus"}, "invalid_schedule", `unknown time zone: "Mars/Olympus"`},
		{"unknown profile", map[string]any{"kind": "monthly", "last_day": true, "time_of_day": "02:00", "profile_id": 999}, "profile_not_found", "profile not found"},
		{"unknown field", map[string]any{"kind": "monthly", "last_day": true, "time_of_day": "02:00", "cron": "* * * * *"}, "invalid_json", `json: unknown field "cron"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := wantErrorCode(t, doJSON(t, srv, http.MethodPut, schedulePath(cohort.ID), tc.body), http.StatusBadRequest, tc.code)
			if body.Error.Message != tc.message {
				t.Errorf("message = %q, want %q", body.Error.Message, tc.message)
			}
			if _, ok := srv.store.GetAnalysisCohortSchedule(cohort.ID); ok {
				t.Error("a rejected PUT stored a schedule")
			}
		})
	}
}

func TestPutAnalysisCohortSchedule(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		srv, cohort := newScheduleAPIServer(t)

		got := mustJSON[map[string]any](t, doJSON(t, srv, http.MethodPut, schedulePath(cohort.ID), monthlyScheduleBody()), http.StatusOK)
		for key, want := range map[string]any{
			"cohort_id":       float64(cohort.ID),
			"enabled":         true,
			"kind":            "monthly",
			"interval_days":   float64(1),
			"anchor_date":     "",
			"last_day":        false,
			"time_of_day":     "02:00",
			"timezone":        "Europe/Stockholm",
			"profile_id":      nil,
			"promote_default": false,
			"catch_up":        true,
			"summary":         "Monthly, days 1 and 15, 02:00 Europe/Stockholm",
			"next_run_at":     "2000-01-01T01:00:00Z",
		} {
			if got[key] != want {
				t.Errorf("%s = %#v, want %#v", key, got[key], want)
			}
		}
		if days, _ := got["days_of_month"].([]any); !slices.Equal(days, []any{float64(1), float64(15)}) {
			t.Errorf("days_of_month = %v, want [1 15]", got["days_of_month"])
		}
		if weekdays, ok := got["weekdays"].([]any); !ok || len(weekdays) != 0 {
			t.Errorf("weekdays = %#v, want []", got["weekdays"])
		}
		if _, ok := got["last_run_at"]; ok {
			t.Errorf("last_run_at = %v, want absent", got["last_run_at"])
		}

		read := mustJSON[AnalysisCohortSchedule](t, doJSON(t, srv, http.MethodGet, schedulePath(cohort.ID), nil), http.StatusOK)
		if read.Rule.String() != "Monthly, days 1 and 15, 02:00 Europe/Stockholm" {
			t.Errorf("GET rule = %q", read.Rule)
		}
	})
}

func TestPutAnalysisCohortScheduleRestartsFromNow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		srv, cohort := newScheduleAPIServer(t)
		off := monthlyScheduleBody()
		off["enabled"] = false
		wantStatus(t, doJSON(t, srv, http.MethodPut, schedulePath(cohort.ID), off), http.StatusOK)

		time.Sleep(40 * 24 * time.Hour)
		got := mustJSON[AnalysisCohortSchedule](t, doJSON(t, srv, http.MethodPut, schedulePath(cohort.ID), monthlyScheduleBody()), http.StatusOK)

		if want := time.Date(2000, 2, 15, 1, 0, 0, 0, time.UTC); !got.Enabled || !got.NextRunAt.Equal(want) {
			t.Errorf("re-enabled = %v next %s, want true %s", got.Enabled, got.NextRunAt, want)
		}
	})
}

func TestAnalysisCohortScheduleGetAndDelete(t *testing.T) {
	srv, cohort := newScheduleAPIServer(t)
	wantErrorCode(t, doJSON(t, srv, http.MethodGet, schedulePath(cohort.ID), nil), http.StatusNotFound, "no_schedule")
	wantErrorCode(t, doJSON(t, srv, http.MethodGet, schedulePath(cohort.ID+1), nil), http.StatusNotFound, "not_found")

	wantStatus(t, doJSON(t, srv, http.MethodPut, schedulePath(cohort.ID), monthlyScheduleBody()), http.StatusOK)
	wantStatus(t, doJSON(t, srv, http.MethodDelete, schedulePath(cohort.ID), nil), http.StatusNoContent)
	wantErrorCode(t, doJSON(t, srv, http.MethodGet, schedulePath(cohort.ID), nil), http.StatusNotFound, "no_schedule")
	wantStatus(t, doJSON(t, srv, http.MethodDelete, schedulePath(cohort.ID), nil), http.StatusNoContent)
}

func TestPutAnalysisCohortScheduleRefusesDisabledCohort(t *testing.T) {
	srv, cohort := newScheduleAPIServer(t)
	cohort.AnalysisEnabled = false
	if _, err := srv.store.UpsertAnalysisCohort(cohort); err != nil {
		t.Fatalf("UpsertAnalysisCohort: %v", err)
	}
	wantErrorCode(t, doJSON(t, srv, http.MethodPut, schedulePath(cohort.ID), monthlyScheduleBody()), http.StatusConflict, "cohort_analysis_disabled")
}

func TestAnalysisScheduleRoutesNeedAnalysis(t *testing.T) {
	srv := newTestServer(t)
	cohort := upsertCohort(t, srv.store, "tld")
	for _, tc := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, schedulePath(cohort.ID), nil},
		{http.MethodPut, schedulePath(cohort.ID), monthlyScheduleBody()},
		{http.MethodDelete, schedulePath(cohort.ID), nil},
		{http.MethodGet, "/api/v1/analysis/schedules", nil},
		{http.MethodPost, "/api/v1/analysis/schedules/preview", monthlyScheduleBody()},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			wantErrorCode(t, doJSON(t, srv, tc.method, tc.path, tc.body), http.StatusServiceUnavailable, "analysis_unavailable")
		})
	}
}

func TestAnalysisSchedulePreview(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		srv, _ := newScheduleAPIServer(t)
		body := map[string]any{"kind": "weekly", "weekdays": []string{"thu", "mon"}, "time_of_day": "23:30"}

		got := mustJSON[SchedulePreviewResponse](t, doJSON(t, srv, http.MethodPost, "/api/v1/analysis/schedules/preview", body), http.StatusOK)

		if got.Summary != "Weekly, Monday and Thursday, 23:30 UTC" {
			t.Errorf("summary = %q", got.Summary)
		}
		var days []string
		for _, at := range got.Next {
			days = append(days, at.UTC().Format(time.RFC3339))
		}
		want := []string{"2000-01-03T23:30:00Z", "2000-01-06T23:30:00Z", "2000-01-10T23:30:00Z", "2000-01-13T23:30:00Z", "2000-01-17T23:30:00Z"}
		if !slices.Equal(days, want) {
			t.Errorf("next = %v, want %v", days, want)
		}

		bad := map[string]any{"kind": "weekly", "time_of_day": "23:30"}
		wantErrorCode(t, doJSON(t, srv, http.MethodPost, "/api/v1/analysis/schedules/preview", bad), http.StatusBadRequest, "invalid_schedule")
	})
}

func TestListAnalysisSchedules(t *testing.T) {
	srv, tld := newScheduleAPIServer(t)
	gov, err := srv.store.UpsertAnalysisCohort(AnalysisCohort{SourceType: "tag", SourceTag: "gov", Label: "Government", AnalysisEnabled: true})
	if err != nil {
		t.Fatalf("UpsertAnalysisCohort: %v", err)
	}
	empty := mustJSON[[]AnalysisCohortSchedule](t, doJSON(t, srv, http.MethodGet, "/api/v1/analysis/schedules", nil), http.StatusOK)
	if len(empty) != 0 {
		t.Fatalf("schedules before PUT = %d, want 0", len(empty))
	}
	for _, id := range []int64{gov.ID, tld.ID} {
		wantStatus(t, doJSON(t, srv, http.MethodPut, schedulePath(id), monthlyScheduleBody()), http.StatusOK)
	}

	got := mustJSON[[]AnalysisCohortSchedule](t, doJSON(t, srv, http.MethodGet, "/api/v1/analysis/schedules", nil), http.StatusOK)

	var rows []string
	for _, sched := range got {
		rows = append(rows, fmt.Sprintf("%d %s %s", sched.CohortID, sched.SourceTag, sched.Label))
	}
	want := []string{fmt.Sprintf("%d tld tld", tld.ID), fmt.Sprintf("%d gov Government", gov.ID)}
	if !slices.Equal(rows, want) {
		t.Errorf("schedules = %v, want %v", rows, want)
	}
}

func TestBatchResponsesCarryOrigin(t *testing.T) {
	srv := newTestServer(t)
	if err := srv.store.CreateBatch(Batch{ID: "batch-sched", Tag: "tld", CreatedAt: time.Now().UTC(), Origin: BatchOriginSchedule}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}

	list := mustJSON[BatchListResponse](t, doJSON(t, srv, http.MethodGet, "/api/v1/batches", nil), http.StatusOK)
	if len(list.Items) != 1 || list.Items[0].Origin != BatchOriginSchedule {
		t.Errorf("list items = %+v, want one with origin schedule", list.Items)
	}
	summary := mustJSON[BatchSummary](t, doJSON(t, srv, http.MethodGet, "/api/v1/batches/batch-sched", nil), http.StatusOK)
	if summary.Origin != BatchOriginSchedule {
		t.Errorf("summary origin = %q, want schedule", summary.Origin)
	}
}
