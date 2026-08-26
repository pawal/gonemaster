package server

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	serverpublic "codeberg.org/pawal/gonemaster/server/public"
)

// seedPublicJob stores a job in the given status. Only a succeeded one is
// graduated; the rest never reach a stored result, which is what the lookup
// has to cope with.
func seedPublicJob(t *testing.T, srv *Server, status JobStatus, entries []engine.LogEntry) Job {
	t.Helper()
	job := Job{
		ID:        newID("job"),
		Domain:    "example.com",
		Status:    status,
		CreatedAt: time.Now().UTC(),
	}
	if status == JobSucceeded {
		return createAndGraduate(t, srv.store, job, entries)
	}
	created, err := srv.store.Create(job)
	if err != nil {
		t.Fatalf("create job %q: %v", job.ID, err)
	}
	return created
}

func TestPublicResultLookupStatusMapping(t *testing.T) {
	tests := []struct {
		name   string
		status JobStatus
		want   serverpublic.LookupStatus
	}{
		{name: "queued is pending", status: JobQueued, want: serverpublic.LookupPending},
		{name: "running is pending", status: JobRunning, want: serverpublic.LookupPending},
		{name: "paused is pending", status: JobPaused, want: serverpublic.LookupPending},
		{name: "succeeded is found", status: JobSucceeded, want: serverpublic.LookupFound},
		{name: "failed is not found", status: JobFailed, want: serverpublic.LookupNotFound},
		{name: "canceled is not found", status: JobCanceled, want: serverpublic.LookupNotFound},
		{name: "expired is not found", status: JobExpired, want: serverpublic.LookupNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t)
			job := seedPublicJob(t, srv, tt.status, []engine.LogEntry{
				{Module: "BASIC", Testcase: "basic01", Tag: "BASIC01", Level: "NOTICE"},
			})
			_, got := srv.publicResultLookup()(job.PublicID, "en")
			if got != tt.want {
				t.Fatalf("status %s: got %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestPublicResultLookupUnknownIDIsNotFound(t *testing.T) {
	srv := newTestServer(t)
	if _, got := srv.publicResultLookup()("nosuchid", "en"); got != serverpublic.LookupNotFound {
		t.Fatalf("got %v, want LookupNotFound", got)
	}
}

func TestPublicResultLookupSummarizesFindings(t *testing.T) {
	srv := newTestServer(t)
	job := seedPublicJob(t, srv, JobSucceeded, []engine.LogEntry{
		{Module: "BASIC", Testcase: "basic01", Tag: "BASIC01", Level: "INFO"},
		{Module: "ZONE", Testcase: "zone01", Tag: "Z_RETRY_MINIMUM_VALUE_LOWER", Level: "WARNING"},
		{Module: "ZONE", Testcase: "zone02", Tag: "Z_MISSING_SOA", Level: "ERROR"},
		{Module: "DELEGATION", Testcase: "delegation01", Tag: "D_NOT_ENOUGH_NS", Level: "CRITICAL"},
		{Module: "BASIC", Testcase: "basic02", Tag: "B_NOTICE", Level: "NOTICE"},
	})

	summary, status := srv.publicResultLookup()(job.PublicID, "en")

	if status != serverpublic.LookupFound {
		t.Fatalf("status = %v, want LookupFound", status)
	}
	if summary.Domain != "example.com" {
		t.Errorf("domain = %q", summary.Domain)
	}
	// INFO and NOTICE are below the threshold and must not be counted.
	if summary.Warnings != 1 || summary.Errors != 1 || summary.Criticals != 1 {
		t.Errorf("counts = %d/%d/%d, want 1/1/1", summary.Warnings, summary.Errors, summary.Criticals)
	}
	if summary.Issues() != 3 {
		t.Errorf("Issues() = %d, want 3", summary.Issues())
	}
	if len(summary.Findings) != 3 {
		t.Fatalf("findings = %d, want 3", len(summary.Findings))
	}
	for _, f := range summary.Findings {
		if f.Level != strings.ToUpper(f.Level) {
			t.Errorf("level %q is not upper case", f.Level)
		}
		if f.Message == "" {
			t.Errorf("finding %v has no message", f)
		}
	}
}

// The cap bounds what a scraper hit can allocate, but the counts stay whole.
func TestPublicResultLookupCapsFindingsButNotCounts(t *testing.T) {
	srv := newTestServer(t)
	entries := make([]engine.LogEntry, 0, serverpublic.MaxSummaryFindings+7)
	for i := 0; i < serverpublic.MaxSummaryFindings+7; i++ {
		entries = append(entries, engine.LogEntry{
			Module: "ZONE", Testcase: fmt.Sprintf("zone%02d", i),
			Tag: "Z_RETRY_MINIMUM_VALUE_LOWER", Level: "WARNING",
		})
	}
	job := seedPublicJob(t, srv, JobSucceeded, entries)

	summary, _ := srv.publicResultLookup()(job.PublicID, "en")

	if len(summary.Findings) != serverpublic.MaxSummaryFindings {
		t.Errorf("findings = %d, want %d", len(summary.Findings), serverpublic.MaxSummaryFindings)
	}
	if summary.Warnings != serverpublic.MaxSummaryFindings+7 {
		t.Errorf("warnings = %d, want %d", summary.Warnings, serverpublic.MaxSummaryFindings+7)
	}
}

// A rendered page must never show a grade the public API refuses to serve.
func TestPublicResultLookupHonoursShowScorePublic(t *testing.T) {
	entries := []engine.LogEntry{
		{Module: "ZONE", Testcase: "zone01", Tag: "Z_RETRY_MINIMUM_VALUE_LOWER", Level: "WARNING"},
	}
	for _, show := range []bool{true, false} {
		t.Run(fmt.Sprintf("show=%v", show), func(t *testing.T) {
			srv := newTestServer(t, withConfig(func(cfg *Config) { cfg.ShowScorePublic = show }))
			job := seedPublicJob(t, srv, JobSucceeded, entries)

			summary, _ := srv.publicResultLookup()(job.PublicID, "en")

			if show && summary.Grade == "" {
				t.Error("grade is hidden while scoring is enabled")
			}
			if !show && summary.Grade != "" {
				t.Errorf("grade %q leaked while scoring is disabled", summary.Grade)
			}
		})
	}
}

func TestPublicResultLookupLocalizesMessages(t *testing.T) {
	srv := newTestServer(t)
	entries := []engine.LogEntry{{
		Module: "ADDRESS", Testcase: "address01", Tag: "NO_RESPONSE_PTR_QUERY", Level: "WARNING",
		Args: map[string]any{"domain": "example.com"},
	}}
	job := seedPublicJob(t, srv, JobSucceeded, entries)

	en, _ := srv.publicResultLookup()(job.PublicID, "en")
	sv, _ := srv.publicResultLookup()(job.PublicID, "sv")

	if len(en.Findings) != 1 || len(sv.Findings) != 1 {
		t.Fatal("expected one finding per locale")
	}
	if en.Findings[0].Message == sv.Findings[0].Message {
		t.Errorf("sv message was not localized: %q", sv.Findings[0].Message)
	}
}

// End to end through the mux: the result path must render server-side. Only
// exercised with the SPA embedded; the placeholder build serves a static
// unavailable page instead.
func TestPublicUIServesRenderedResultPage(t *testing.T) {
	if !serverpublic.IsBuilt() {
		t.Skip("public UI dist not built; the render path is not exercised")
	}
	srv := newTestServer(t)
	job := seedPublicJob(t, srv, JobSucceeded, []engine.LogEntry{
		{Module: "ZONE", Testcase: "zone01", Tag: "Z_RETRY_MINIMUM_VALUE_LOWER", Level: "WARNING"},
	})

	rr := doJSON(t, srv, http.MethodGet, "/public/result/"+job.PublicID, nil)

	wantStatus(t, rr, http.StatusOK)
	if !strings.Contains(rr.Body.String(), "example.com") {
		t.Error("rendered page does not name the domain")
	}
	if rr.Header().Get("X-Robots-Tag") != "noindex" {
		t.Errorf("X-Robots-Tag = %q, want noindex", rr.Header().Get("X-Robots-Tag"))
	}
}

func TestPublicUIUnknownResultIs404(t *testing.T) {
	if !serverpublic.IsBuilt() {
		t.Skip("public UI dist not built; the unavailable page answers 200")
	}
	srv := newTestServer(t)

	rr := doJSON(t, srv, http.MethodGet, "/public/result/nosuchid", nil)

	wantStatus(t, rr, http.StatusNotFound)
}

// The public limiter meters POST only, so result pages are no more metered
// than the public API GETs they read from. Sharing the submission bucket with
// page views would exhaust it in normal use, so this is deliberate.
func TestResultPageGetsShareTheAPIsUnmeteredPath(t *testing.T) {
	srv := newTestServer(t, withConfig(func(cfg *Config) {
		cfg.PublicAPI.RateLimitEnabled = true
		cfg.PublicAPI.RateLimitMax = 1
		cfg.PublicAPI.RateLimitWindow = Duration{time.Hour}
	}))
	if srv.rateLimiter == nil {
		t.Fatal("rate limiter did not come up, the rest of this test proves nothing")
	}

	do := func(method, path string, body any) int {
		return doJSON(t, srv, method, path, body, withRemoteAddr("192.0.2.10:1234")).Code
	}

	for i := 0; i < 5; i++ {
		for _, path := range []string{"/public/", "/public/result/abc12345", "/pub/api/v1/jobs/abc12345"} {
			if code := do(http.MethodGet, path, nil); code == http.StatusTooManyRequests {
				t.Fatalf("GET %s was rate limited on request %d", path, i+1)
			}
		}
	}

	// Proves the limiter is live in this setup, so the GETs above are not
	// passing merely because nothing is metered at all.
	limited := false
	for i := 0; i < 4; i++ {
		if do(http.MethodPost, "/pub/api/v1/jobs", map[string]string{"domain": "example.com"}) == http.StatusTooManyRequests {
			limited = true
			break
		}
	}
	if !limited {
		t.Error("POST /pub/api/v1/jobs was never limited, so the limiter is inert")
	}
}
