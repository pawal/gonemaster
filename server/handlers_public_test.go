package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

func TestPublicCreateJobReturnsPublicIDNotUUID(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodPost, "/pub/api/v1/jobs", `{"domain":"example.com"}`)

	view := mustJSON[map[string]any](t, resp, http.StatusCreated)
	if view["public_id"] == "" || view["public_id"] == nil {
		t.Fatal("expected public_id in response")
	}
	if _, hasID := view["id"]; hasID {
		t.Fatal("internal UUID must not appear in public API response")
	}
}

func TestPublicCreateJobReturnsDomainStatusProgress(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodPost, "/pub/api/v1/jobs", `{"domain":"example.com"}`)

	view := mustJSON[PublicJobView](t, resp, http.StatusCreated)
	if view.Domain != "example.com" {
		t.Fatalf("domain: got %q, want %q", view.Domain, "example.com")
	}
	if view.Status != JobQueued {
		t.Fatalf("status: got %q, want %q", view.Status, JobQueued)
	}
	if view.Progress != 0 {
		t.Fatalf("progress: got %d, want 0", view.Progress)
	}
}

func TestPublicCreateJobMissingDomainReturns400(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodPost, "/pub/api/v1/jobs", `{}`)

	wantStatus(t, resp, http.StatusBadRequest)
}

func TestPublicCreateJobWithPublicProfileID(t *testing.T) {
	srv := newTestServer(t)
	profile := createProfile(t, srv, `{"name":"public-profile","config":{"net":{"ipv4":true}},"public":true}`)

	resp := doJSON(t, srv, http.MethodPost, "/pub/api/v1/jobs", fmt.Sprintf(`{"domain":"example.com","profile_id":%d}`, profile.ID))

	created := mustJSON[PublicJobView](t, resp, http.StatusCreated)
	job, ok := srv.store.GetByPublicID(created.PublicID)
	if !ok {
		t.Fatal("expected stored public job")
	}
	if job.ProfileID == nil || *job.ProfileID != profile.ID {
		t.Fatalf("ProfileID: got %v, want %d", job.ProfileID, profile.ID)
	}
	if job.ProfileName != "public-profile" {
		t.Fatalf("ProfileName: got %q", job.ProfileName)
	}
}

func TestPublicCreateJobRejectsPrivateProfile(t *testing.T) {
	srv := newTestServer(t)
	profile := createProfile(t, srv, `{"name":"private-profile","config":{"net":{"ipv4":true}},"public":false}`)

	resp := doJSON(t, srv, http.MethodPost, "/pub/api/v1/jobs", fmt.Sprintf(`{"domain":"example.com","profile_id":%d}`, profile.ID))

	wantErrorCode(t, resp, http.StatusBadRequest, "profile_not_public")
}

func TestPublicCreateJobRejectsProfileOverrides(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodPost, "/pub/api/v1/jobs", `{"domain":"example.com","profile_overrides":{"net":{"ipv6":false}}}`)

	wantErrorCode(t, resp, http.StatusBadRequest, "profile_overrides_not_allowed")
}

func TestPublicCreateJobCSRFOriginMatrix(t *testing.T) {
	// A missing Origin passes so non-browser clients (gonemaster-client) work.
	csrfOriginMatrix(t, http.StatusCreated, func(t *testing.T, opts ...reqOpt) *httptest.ResponseRecorder {
		return doJSON(t, newTestServer(t), http.MethodPost, "/pub/api/v1/jobs", `{"domain":"example.com"}`, opts...)
	})
}

func TestPublicCreateJobCSRFAcceptsHTTPSOriginViaTrustedProxy(t *testing.T) {
	// Caddy/nginx terminate TLS upstream; gonemaster sees plain HTTP.
	// Without honoring X-Forwarded-Proto, port 443 (Origin) won't match
	// port 80 (assumed scheme=http) and the CSRF check would 403.
	srv := newTestServer(t, withConfig(func(c *Config) { c.TrustedProxyCIDRs = []string{"127.0.0.1/32"} }))

	resp := doJSON(t, srv, http.MethodPost, "/pub/api/v1/jobs", `{"domain":"example.com"}`, withHost("gonemaster.evilbit.de"), withRemoteAddr("127.0.0.1:54321"), withOrigin("https://gonemaster.evilbit.de"), withHeader("X-Forwarded-Proto", "https"))

	wantStatus(t, resp, http.StatusCreated)
}

func TestPublicCreateJobCSRFRejectsForgedXForwardedProtoFromUntrustedRemote(t *testing.T) {
	// No trusted proxies: X-Forwarded-Proto is ignored. The forged header
	// must not let an attacker make port 443 match.
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodPost, "/pub/api/v1/jobs", `{"domain":"example.com"}`, withHost("gonemaster.evilbit.de"), withRemoteAddr("203.0.113.99:54321"), withOrigin("https://gonemaster.evilbit.de"), withHeader("X-Forwarded-Proto", "https"))

	wantStatus(t, resp, http.StatusForbidden)
}

func TestPublicCreateJobRejectsOversizedInput(t *testing.T) {
	// One element past each cap, so the limit itself is what rejects.
	list := func(n int, item func(i int) string) string {
		items := make([]string, 0, n)
		for i := range n {
			items = append(items, item(i))
		}
		return strings.Join(items, ",")
	}

	for _, tc := range []struct {
		field string
		body  string
		code  string
	}{
		{
			field: "nameservers",
			body: fmt.Sprintf(`{"domain":"example.com","nameservers":[%s]}`,
				list(MaxUndelegatedNameservers+1, func(i int) string {
					return fmt.Sprintf(`{"ns":"ns%d.example.","ip":"198.51.100.%d"}`, i, i+1)
				})),
			code: "invalid_undelegated",
		},
		{
			field: "ds_info",
			body: fmt.Sprintf(`{"domain":"example.com","ds_info":[%s]}`,
				list(MaxUndelegatedDSRecords+1, func(int) string {
					return `{"keytag":1,"algorithm":8,"digtype":2,"digest":"` + strings.Repeat("a", 64) + `"}`
				})),
			code: "invalid_undelegated",
		},
		{
			field: "tests",
			body: fmt.Sprintf(`{"domain":"example.com","tests":[%s]}`,
				list(MaxPublicTests+1, func(int) string { return `"x"` })),
			code: "too_many_tests",
		},
	} {
		t.Run(tc.field, func(t *testing.T) {
			resp := doJSON(t, newTestServer(t), http.MethodPost, "/pub/api/v1/jobs", tc.body)
			wantErrorCode(t, resp, http.StatusBadRequest, tc.code)
		})
	}
}

func TestPublicCreateJobStoreErrorDoesNotLeakDBDetails(t *testing.T) {
	srv := newTestServer(t)
	canary := "ERROR: duplicate key value violates unique constraint \"jobs_pkey\""
	wrapStore(t, srv).createErr = errors.New(canary)

	resp := doJSON(t, srv, http.MethodPost, "/pub/api/v1/jobs", `{"domain":"example.com"}`)

	wantStatus(t, resp, http.StatusInternalServerError)
	body := resp.Body.String()
	if strings.Contains(body, canary) || strings.Contains(body, "jobs_pkey") {
		t.Fatalf("response leaked raw store error: %s", body)
	}
	var out ErrorResponse
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Error.Code != "store_error" {
		t.Fatalf("expected code store_error, got %q", out.Error.Code)
	}
	if out.Error.Message == "" || strings.Contains(out.Error.Message, "duplicate") {
		t.Fatalf("expected sanitized message, got %q", out.Error.Message)
	}
}

func TestPublicProfilesReturnsOnlyPublicProfilesWithoutConfig(t *testing.T) {
	srv := newTestServer(t)
	createProfile(t, srv, `{"name":"public-profile","description":"Shown","config":{"net":{"ipv4":true}},"public":true}`)
	createProfile(t, srv, `{"name":"private-profile","description":"Hidden","config":{"net":{"ipv6":false}},"public":false}`)

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/profiles", nil)

	views := mustJSON[[]map[string]any](t, resp, http.StatusOK)
	if len(views) != 1 {
		t.Fatalf("expected 1 public profile, got %d", len(views))
	}
	if views[0]["name"] != "public-profile" {
		t.Fatalf("expected public-profile, got %#v", views[0])
	}
	if _, ok := views[0]["config"]; ok {
		t.Fatalf("public profile payload must not expose config: %#v", views[0])
	}
}

func TestPublicGetJobReturnsPublicIDNotUUID(t *testing.T) {
	srv := newTestServer(t)

	// Create via public API.
	resp := doJSON(t, srv, http.MethodPost, "/pub/api/v1/jobs", `{"domain":"example.com"}`)
	created := mustJSON[PublicJobView](t, resp, http.StatusCreated)

	// Get via public ID.
	resp = doJSON(t, srv, http.MethodGet, "/pub/api/v1/jobs/"+created.PublicID, nil)

	got := mustJSON[map[string]any](t, resp, http.StatusOK)
	if got["public_id"] != created.PublicID {
		t.Fatalf("public_id: got %v, want %q", got["public_id"], created.PublicID)
	}
	if _, hasID := got["id"]; hasID {
		t.Fatal("internal UUID must not appear in public get response")
	}
}

func TestPublicGetJobUnknownPublicIDReturns404(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/jobs/notexist1", nil)

	wantStatus(t, resp, http.StatusNotFound)
}

func TestPublicGetResultReturnsResultByPublicID(t *testing.T) {
	srv := newTestServer(t)

	// Create a job and manually store a result for it.
	job := Job{
		ID:        newID("job"),
		Domain:    "example.com",
		Status:    JobSucceeded,
		CreatedAt: time.Now().UTC(),
		Progress:  100,
		NameserverTimings: []NameserverTiming{
			{
				Nameserver: "ns1.example.com",
				Address:    "192.0.2.10",
				AvgMS:      24,
				MinMS:      20,
				MaxMS:      30,
				MedianMS:   22,
				StddevMS:   4,
				Count:      3,
			},
		},
	}
	created, err := srv.store.Create(job)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := srv.store.GraduateJob(created, nil); err != nil {
		t.Fatalf("GraduateJob: %v", err)
	}

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/jobs/"+created.PublicID+"/result", nil)

	result := mustJSON[JobResult](t, resp, http.StatusOK)
	if result.Status != JobSucceeded {
		t.Fatalf("status: got %q, want %q", result.Status, JobSucceeded)
	}
	if len(result.NameserverTimings) != 1 {
		t.Fatalf("nameserver timings len = %d, want 1", len(result.NameserverTimings))
	}
}

func TestPublicGetResultUnknownPublicIDReturns404(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/jobs/notexist1/result", nil)

	wantStatus(t, resp, http.StatusNotFound)
	if cc := resp.Header().Get("Cache-Control"); cc != "" {
		t.Fatalf("Cache-Control should not be set on 404, got %q", cc)
	}
}

func TestPublicGetResultSetsCacheControlOnSuccess(t *testing.T) {
	srv := newTestServer(t)

	job := Job{
		ID:        newID("job"),
		Domain:    "example.com",
		Status:    JobSucceeded,
		CreatedAt: time.Now().UTC(),
		Progress:  100,
	}
	created, err := srv.store.Create(job)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := srv.store.GraduateJob(created, nil); err != nil {
		t.Fatalf("GraduateJob: %v", err)
	}

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/jobs/"+created.PublicID+"/result", nil)

	wantStatus(t, resp, http.StatusOK)
	if got := resp.Header().Get("Cache-Control"); got != "public, max-age=300" {
		t.Fatalf("Cache-Control: got %q, want %q", got, "public, max-age=300")
	}
}

func TestPublicLocalesEndpointAccessible(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/locales", nil)

	wantStatus(t, resp, http.StatusOK)
}

func TestPublicGetResultNoResultYetReturns404(t *testing.T) {
	srv := newTestServer(t)

	// Create a job but do not store a result for it.
	resp := doJSON(t, srv, http.MethodPost, "/pub/api/v1/jobs", `{"domain":"example.com"}`)
	created := mustJSON[PublicJobView](t, resp, http.StatusCreated)

	resp = doJSON(t, srv, http.MethodGet, "/pub/api/v1/jobs/"+created.PublicID+"/result", nil)

	wantStatus(t, resp, http.StatusNotFound)
}

func TestPublicGetResultRespectsLocaleParam(t *testing.T) {
	srv := newTestServer(t)

	job := Job{
		ID:        newID("job"),
		Domain:    "example.com",
		Status:    JobSucceeded,
		CreatedAt: time.Now().UTC(),
		Progress:  100,
	}
	created, _ := srv.store.Create(job)
	if err := srv.store.GraduateJob(created, []engine.LogEntry{
		{Module: "BASIC", Testcase: "basic01", Tag: "BASIC01", Level: "NOTICE"},
	}); err != nil {
		t.Fatalf("GraduateJob: %v", err)
	}

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/jobs/"+created.PublicID+"/result?locale=sv", nil)

	result := mustJSON[JobResult](t, resp, http.StatusOK)
	if result.Raw == nil || result.Raw.Locale != "sv" {
		t.Fatalf("expected locale=sv in result, got %v", result.Raw)
	}
}

func TestPublicGetResultUnknownLocaleForcedToEnglish(t *testing.T) {
	// Unknown / malicious locale must be silently forced to "en" so the
	// metrics layer never sees an attacker-controlled bucket key and the
	// response never echoes the unknown value as result.locale.
	srv := newTestServer(t)

	job := Job{
		ID:        newID("job"),
		Domain:    "example.com",
		Status:    JobSucceeded,
		CreatedAt: time.Now().UTC(),
		Progress:  100,
	}
	created, _ := srv.store.Create(job)
	if err := srv.store.GraduateJob(created, []engine.LogEntry{
		{Module: "BASIC", Testcase: "basic01", Tag: "BASIC01", Level: "NOTICE"},
	}); err != nil {
		t.Fatalf("GraduateJob: %v", err)
	}

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/jobs/"+created.PublicID+"/result?locale=../../etc/passwd", nil)

	result := mustJSON[JobResult](t, resp, http.StatusOK)
	if result.Raw == nil || result.Raw.Locale != "en" {
		t.Fatalf("expected forced locale=en, got %v", result.Raw)
	}
}

func TestResolveResultLocale(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "en"},
		{"  ", "en"},
		{"en", "en"},
		{"sv", "sv"},
		{"SV", "sv"},
		{"  sv  ", "sv"},
		{"../../etc/passwd", "en"},
		{"zh-CN", "en"}, // not in the catalog list
	}
	for _, tc := range cases {
		if got := resolveResultLocale(tc.in); got != tc.want {
			t.Fatalf("resolveResultLocale(%q): got %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPublicAPIJobCreatedViaInternalAPIFetchableByPublicID(t *testing.T) {
	srv := newTestServer(t)

	// Create via internal API.
	resp := doJSON(t, srv, http.MethodPost, "/api/v1/jobs", `{"domain":"example.com"}`)
	internalJob := mustJSON[Job](t, resp, http.StatusCreated)
	if internalJob.PublicID == "" {
		t.Fatal("internal API response must include public_id")
	}

	// Fetch via public API using the public_id from the internal response.
	resp = doJSON(t, srv, http.MethodGet, "/pub/api/v1/jobs/"+internalJob.PublicID, nil)

	view := mustJSON[PublicJobView](t, resp, http.StatusOK)
	if view.PublicID != internalJob.PublicID {
		t.Fatalf("public_id mismatch: got %q, want %q", view.PublicID, internalJob.PublicID)
	}
}

func TestPublicAPIEndpointsUnreachableViaInternalPrefix(t *testing.T) {
	srv := newTestServer(t)

	// Admin-only endpoints must not be reachable via /pub/
	for _, path := range []string{
		"/pub/api/v1/metrics",
		"/pub/api/v1/queue/pause",
		"/pub/api/v1/batches",
	} {
		resp := doJSON(t, srv, http.MethodGet, path, nil)
		if resp.Code == http.StatusOK {
			t.Errorf("path %q should not return 200 via public prefix", path)
		}
	}
}

func TestPublicResultOmitsScoreWhenPublicScoringDisabled(t *testing.T) {
	srv := newTestServer(t,
		withConfig(func(c *Config) {
			c.ShowScorePublic = false
			c.ShowNameserverTimingsPublic = false
		}))

	job := Job{
		ID:        newID("job"),
		Domain:    "example.com",
		Status:    JobSucceeded,
		CreatedAt: time.Now().UTC(),
		Progress:  100,
		NameserverTimings: []NameserverTiming{
			{Nameserver: "ns1.example.com", Address: "192.0.2.10", AvgMS: 24, MinMS: 20, MaxMS: 30, Count: 3},
		},
	}
	created, err := srv.store.Create(job)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := srv.store.GraduateJob(created, nil); err != nil {
		t.Fatalf("GraduateJob: %v", err)
	}

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/jobs/"+created.PublicID+"/result", nil)
	result := mustJSON[JobResult](t, resp, http.StatusOK)
	if result.Score != nil {
		t.Fatal("expected Score to be nil when ShowScorePublic=false")
	}
	if result.NameserverTimings != nil {
		t.Fatal("expected NameserverTimings to be nil when ShowNameserverTimingsPublic=false")
	}
}

func TestPublicCreateJobAcceptsIPv6Disabled(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodPost, "/pub/api/v1/jobs", `{"domain":"example.com","ipv6_disabled":true}`)

	view := mustJSON[PublicJobView](t, resp, http.StatusCreated)
	job, ok := srv.store.GetByPublicID(view.PublicID)
	if !ok {
		t.Fatal("expected stored public job")
	}
	if !job.IPv6Disabled {
		t.Fatal("expected job.IPv6Disabled=true")
	}
	if job.IPv4Disabled {
		t.Fatal("expected job.IPv4Disabled=false")
	}
}

func TestPublicCreateJobAcceptsIPv4Disabled(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodPost, "/pub/api/v1/jobs", `{"domain":"example.com","ipv4_disabled":true}`)

	view := mustJSON[PublicJobView](t, resp, http.StatusCreated)
	job, ok := srv.store.GetByPublicID(view.PublicID)
	if !ok {
		t.Fatal("expected stored public job")
	}
	if !job.IPv4Disabled {
		t.Fatal("expected job.IPv4Disabled=true")
	}
}
