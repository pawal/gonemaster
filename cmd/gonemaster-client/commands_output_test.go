package main

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/cmd/internal/clitest"
)

// stubJSON points newHTTPClient at a transport answering every request with
// body marshalled as JSON.
func stubJSON(t *testing.T, body any) {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal stub body: %v", err)
	}
	old := newHTTPClient
	t.Cleanup(func() { newHTTPClient = old })
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusOK, string(data)), nil
		})}
	}
}

func TestListCommandsPrettyOutput(t *testing.T) {
	for _, tc := range []struct {
		name string
		body any
		args []string
		want []string
	}{
		{
			name: "domains with latest run",
			body: domainList{
				Items: []domain{
					{
						ID:          1,
						Name:        "example.com",
						LatestLevel: "WARNING",
						LatestRunAt: "2026-03-15T10:00:00Z",
						RunCount:    3,
						Tags:        []string{"tld"},
					},
				},
				Total: 1,
			},
			args: []string{"domains", "list"},
			want: []string{"example.com", "2026-03-15", "WARNING", "tld"},
		},
		{
			// A domain never run shows "-" instead of a date.
			name: "domains without latest run",
			body: domainList{Items: []domain{{ID: 1, Name: "example.com", RunCount: 0}}, Total: 1},
			args: []string{"domains", "list"},
			want: []string{"-"},
		},
		{
			name: "runs with finished at",
			body: runList{
				Items: []runRecord{
					{
						ID:         "run-abc-123",
						Domain:     "example.com",
						Status:     "succeeded",
						WorstLevel: "WARNING",
						FinishedAt: "2026-03-15T10:00:00Z",
						DurationMs: 1250,
					},
				},
				Total: 1,
			},
			args: []string{"runs", "list"},
			want: []string{"run-abc-123", "2026-03-15", "1250ms"},
		},
		{
			name: "runs without finished at",
			body: runList{Items: []runRecord{{ID: "run-1", Domain: "example.com", Status: "succeeded"}}, Total: 1},
			args: []string{"runs", "list"},
			want: []string{"-"},
		},
		{
			name: "entries with domain name",
			body: entryList{
				Items: []entry{
					{
						ID:       1,
						RunID:    "run-1",
						DomainID: 7,
						Domain:   "example.com",
						Module:   "DNSSEC",
						Testcase: "DNSSEC02",
						Tag:      "DNSSEC_NSEC3",
						Level:    "WARNING",
					},
				},
				Total: 1,
			},
			args: []string{"entries", "query"},
			want: []string{"example.com", "DNSSEC", "WARNING"},
		},
		{
			// Without a domain name the row falls back to the domain id.
			name: "entries without domain name",
			body: entryList{Items: []entry{{ID: 1, DomainID: 42, Module: "Basic", Testcase: "Basic01", Level: "INFO"}}, Total: 1},
			args: []string{"entries", "query"},
			want: []string{"id:42"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stubJSON(t, tc.body)

			clitest.Run(t, run, tc.args...).
				RequireCode(t, 0).
				RequireOutContains(t, tc.want...)
		})
	}
}

func TestListCommandsJSONOutput(t *testing.T) {
	for _, tc := range []struct {
		name string
		body any
		args []string
		into any
	}{
		{"domains", domainList{Items: []domain{{ID: 1, Name: "example.com"}}, Total: 1}, []string{"domains", "list"}, &domainList{}},
		{"runs", runList{Items: []runRecord{{ID: "r1", Domain: "example.com", Status: "succeeded"}}, Total: 1}, []string{"runs", "list"}, &runList{}},
		{"tags", []tag{{Name: "tld", Description: "Top-level", DomainCount: 5}}, []string{"tags", "list"}, &[]tag{}},
		{"entries", entryList{Items: []entry{{ID: 1, Module: "Basic", Level: "INFO"}}, Total: 1}, []string{"entries", "query"}, &entryList{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stubJSON(t, tc.body)

			res := clitest.Run(t, run, append([]string{"--format", "json"}, tc.args...)...).RequireCode(t, 0)
			if err := json.Unmarshal([]byte(res.Out), tc.into); err != nil {
				t.Fatalf("expected valid JSON output: %v - got: %s", err, res.Out)
			}
		})
	}
}
