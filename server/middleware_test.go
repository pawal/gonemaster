package server

import "testing"

func TestAPIRouteTemplate(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{path: "/api/v1", want: "/api/v1"},
		{path: "/api/v1/", want: "/api/v1/"},
		{path: "/api/v1/jobs", want: "/api/v1/jobs"},
		{path: "/api/v1/jobs/batch", want: "/api/v1/jobs/batch"},
		{path: "/api/v1/jobs/job-1", want: "/api/v1/jobs/{job_id}"},
		{path: "/api/v1/jobs/job-1/result", want: "/api/v1/jobs/{job_id}/result"},
		{path: "/api/v1/jobs/job-1/events", want: "/api/v1/jobs/{job_id}/events"},
		{path: "/api/v1/jobs/job-1/cancel", want: "/api/v1/jobs/{job_id}/cancel"},
		{path: "/api/v1/jobs/job-1/extra", want: "/api/v1/jobs/unknown"},
		{path: "/api/v1/batches/batch-1", want: "/api/v1/batches/{batch_id}"},
		{path: "/api/v1/queue/pause", want: "/api/v1/queue/pause"},
		{path: "/api/v1/queue/resume", want: "/api/v1/queue/resume"},
		{path: "/api/v1/queue/reorder", want: "/api/v1/queue/reorder"},
		{path: "/api/v1/queue/remove", want: "/api/v1/queue/remove"},
		{path: "/api/v1/metrics", want: "/api/v1/metrics"},
		{path: "/api/v1/healthz", want: "/api/v1/healthz"},
		{path: "/api/v1/unknown/path", want: "/api/v1/unknown"},
		{path: "/not-api", want: "/api/v1/unknown"},
	}

	for _, tc := range tests {
		if got := apiRouteTemplate(tc.path); got != tc.want {
			t.Fatalf("apiRouteTemplate(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}
