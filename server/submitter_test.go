package server

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestClientField(t *testing.T) {
	cases := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"printable", "curl/8.4.0", 64, "curl/8.4.0"},
		{"control octets", "a\tb\nc\x00d\x7f", 64, "a?b?c?d?"},
		{"non ascii", "sök/1", 64, "s?k/1"},
		{"invalid utf8", "a\xffb", 64, "a?b"},
		{"cut", "abcdef", 4, "abcd"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := clientField(tc.in, tc.n); got != tc.want {
				t.Fatalf("clientField(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
			}
		})
	}
}

func TestClientIPFilter(t *testing.T) {
	cases := map[string]string{
		" 192.0.2.7 ":      "192.0.2.7",
		"2001:DB8::0:1":    "2001:db8::1",
		"::ffff:192.0.2.1": "192.0.2.1",
		"unknown":          "unknown",
		"":                 "",
	}
	for in, want := range cases {
		if got := clientIPFilter(in); got != want {
			t.Fatalf("clientIPFilter(%q) = %q, want %q", in, got, want)
		}
	}
}

// submitAt creates one job through path and returns its admin job id.
func submitAt(t *testing.T, srv *Server, path string, opts ...reqOpt) string {
	t.Helper()
	switch path {
	case "/pub/api/v1/jobs":
		resp := doJSON(t, srv, http.MethodPost, path, `{"domain":"example.com"}`, opts...)
		view := mustJSON[PublicJobView](t, resp, http.StatusCreated)
		if strings.Contains(resp.Body.String(), "client_") {
			t.Fatalf("public create response carries client fields: %s", resp.Body.String())
		}
		job, ok := srv.store.GetByPublicID(view.PublicID)
		if !ok {
			t.Fatalf("public id %q not stored", view.PublicID)
		}
		return job.ID
	case "/api/v1/jobs/batch":
		out := mustJSON[JobBatchResponse](t, doJSON(t, srv, http.MethodPost, path, `{"domains":["example.com"]}`, opts...), http.StatusAccepted)
		if len(out.JobIDs) != 1 {
			t.Fatalf("job ids = %v, want one", out.JobIDs)
		}
		return out.JobIDs[0]
	default:
		return mustJSON[Job](t, doJSON(t, srv, http.MethodPost, path, `{"domain":"example.com"}`, opts...), http.StatusCreated).ID
	}
}

func TestJobRecordsSubmitter(t *testing.T) {
	for _, path := range []string{"/pub/api/v1/jobs", "/api/v1/jobs", "/api/v1/jobs/batch"} {
		for _, tc := range []struct {
			name string
			opts []reqOpt
			via  string
		}{
			{"api", nil, ClientViaAPI},
			{"ui", []reqOpt{sameOrigin()}, ClientViaUI},
		} {
			t.Run(path+"/"+tc.name, func(t *testing.T) {
				srv := newTestServer(t)
				opts := append([]reqOpt{withRemoteAddr("198.51.100.7:4711"), withHeader("User-Agent", "probe/1.0")}, tc.opts...)

				id := submitAt(t, srv, path, opts...)

				job := mustJSON[Job](t, doJSON(t, srv, http.MethodGet, "/api/v1/jobs/"+id, nil), http.StatusOK)
				if job.ClientIP != "198.51.100.7" || job.ClientUserAgent != "probe/1.0" || job.ClientVia != tc.via {
					t.Fatalf("client = %q %q %q, want 198.51.100.7 probe/1.0 %s", job.ClientIP, job.ClientUserAgent, job.ClientVia, tc.via)
				}
			})
		}
	}
}

func TestJobRecordsSubmitterBehindTrustedProxy(t *testing.T) {
	srv := newTestServer(t, withConfig(func(c *Config) { c.TrustedProxyCIDRs = []string{"127.0.0.1/32"} }))

	id := submitAt(t, srv, "/pub/api/v1/jobs", withRemoteAddr("127.0.0.1:4711"), withHeader("X-Forwarded-For", "203.0.113.9"))

	job, ok := srv.store.Get(id)
	if !ok {
		t.Fatalf("job %q not stored", id)
	}
	if job.ClientIP != "203.0.113.9" {
		t.Fatalf("client ip = %q, want 203.0.113.9", job.ClientIP)
	}
}

func TestPublicJobViewsOmitSubmitter(t *testing.T) {
	srv := newTestServer(t)
	id := submitAt(t, srv, "/pub/api/v1/jobs", withRemoteAddr("198.51.100.7:4711"), withHeader("User-Agent", "probe/1.0"))
	job, ok := srv.store.Get(id)
	if !ok {
		t.Fatalf("job %q not stored", id)
	}
	graduate(t, srv.store, job, systemStartEntry())

	for _, path := range []string{"/pub/api/v1/jobs/" + job.PublicID, "/pub/api/v1/jobs/" + job.PublicID + "/result"} {
		resp := doJSON(t, srv, http.MethodGet, path, nil)
		wantStatus(t, resp, http.StatusOK)
		if body := resp.Body.String(); strings.Contains(body, "198.51.100.7") || strings.Contains(body, "probe/1.0") || strings.Contains(body, "client_") {
			t.Fatalf("%s leaks the submitter: %s", path, body)
		}
	}
}

func TestStoreKeepsSubmitterOnRun(t *testing.T) {
	forEachStore(t, func(t *testing.T, store JobStore) {
		a := seedGraduatedRun(t, store, runSpec{Domain: "a.example", ClientIP: "192.0.2.1", ClientUserAgent: "probe/1.0", ClientVia: ClientViaUI})
		seedGraduatedRun(t, store, runSpec{Domain: "b.example", ClientIP: "192.0.2.2"})

		run, ok := store.GetRun(a.ID)
		if !ok {
			t.Fatalf("run %q missing", a.ID)
		}
		if run.ClientIP != "192.0.2.1" || run.ClientUserAgent != "probe/1.0" || run.ClientVia != "ui" {
			t.Fatalf("run client = %q %q %q, want 192.0.2.1 probe/1.0 ui", run.ClientIP, run.ClientUserAgent, run.ClientVia)
		}
		job, ok := store.Get(a.ID)
		if !ok {
			t.Fatalf("job %q missing", a.ID)
		}
		if job.ClientIP != "192.0.2.1" || job.ClientUserAgent != "probe/1.0" || job.ClientVia != "ui" {
			t.Fatalf("job from run client = %q %q %q, want 192.0.2.1 probe/1.0 ui", job.ClientIP, job.ClientUserAgent, job.ClientVia)
		}
		list := store.ListRuns(RunFilter{ClientIP: "192.0.2.1"})
		if list.Total != 1 || len(list.Items) != 1 || list.Items[0].ID != a.ID {
			t.Fatalf("filtered runs = %+v, want only %s", list.Items, a.ID)
		}
	})
}

// queueJob creates one in-flight job from ip.
func queueJob(t *testing.T, store JobStore, domain, ip string) Job {
	t.Helper()
	job, err := store.Create(Job{ID: newID("job"), Domain: domain, Status: JobQueued, CreatedAt: time.Now().UTC(), ClientIP: ip})
	if err != nil {
		t.Fatalf("create %s: %v", domain, err)
	}
	return job
}

func TestListJobsFiltersByClientIP(t *testing.T) {
	forEachStore(t, func(t *testing.T, store JobStore) {
		srv := newTestServer(t, withDB(store))
		done := seedGraduatedRun(t, store, runSpec{Domain: "a.example", ClientIP: "2001:db8::1"})
		seedGraduatedRun(t, store, runSpec{Domain: "b.example", ClientIP: "192.0.2.2"})
		queued := queueJob(t, store, "c.example", "2001:db8::1")
		queueJob(t, store, "d.example", "192.0.2.2")

		jobs := mustJSON[JobList](t, doJSON(t, srv, http.MethodGet, "/api/v1/jobs?client_ip=2001:DB8::1", nil), http.StatusOK)
		got := map[string]bool{}
		for _, j := range jobs.Items {
			got[j.ID] = true
		}
		if jobs.Total != 2 || len(jobs.Items) != 2 || !got[done.ID] || !got[queued.ID] {
			t.Fatalf("jobs = %+v, want %s and %s", jobs.Items, done.ID, queued.ID)
		}

		runs := mustJSON[RunList](t, doJSON(t, srv, http.MethodGet, "/api/v1/runs?client_ip=2001:db8::1", nil), http.StatusOK)
		if runs.Total != 1 || len(runs.Items) != 1 || runs.Items[0].ID != done.ID {
			t.Fatalf("runs = %+v, want only %s", runs.Items, done.ID)
		}
	})
}

func TestAccessLogCarriesUserAgent(t *testing.T) {
	var buf bytes.Buffer
	srv := newTestServer(t, withLogTo(&buf, "info"))

	doJSON(t, srv, http.MethodGet, "/api/v1/healthz", nil, withHeader("User-Agent", "probe/1.0\n"))

	if got := findLogLine(t, &buf, "http_request")["user_agent"]; got != "probe/1.0?" {
		t.Fatalf("user_agent = %v, want probe/1.0?", got)
	}
}
