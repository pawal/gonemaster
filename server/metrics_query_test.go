package server

import (
	"net/url"
	"testing"
)

func TestParseMetricsQueryOptionsValid(t *testing.T) {
	values := url.Values{
		"window":        []string{"6h"},
		"include":       []string{"jobs,health,trends"},
		"limit_domains": []string{"7"},
		"limit_batches": []string{"9"},
	}

	options, code, message := parseMetricsQueryOptions(values)
	if code != "" || message != "" {
		t.Fatalf("expected valid options, got code=%q message=%q", code, message)
	}
	if options.format != metricsFormatJSON {
		t.Fatalf("format = %q, want %q", options.format, metricsFormatJSON)
	}
	if options.window != "6h" {
		t.Fatalf("window = %q, want 6h", options.window)
	}
	if options.includeAll {
		t.Fatal("includeAll = true, want false")
	}
	if !options.include["jobs"] || !options.include["health"] || !options.include["trends"] {
		t.Fatalf("include = %+v, want jobs/health/trends", options.include)
	}
	if options.limitDomains != 7 {
		t.Fatalf("limitDomains = %d, want 7", options.limitDomains)
	}
	if options.limitBatches != 9 {
		t.Fatalf("limitBatches = %d, want 9", options.limitBatches)
	}
}

func TestParseMetricsQueryOptionsPromFormat(t *testing.T) {
	options, code, message := parseMetricsQueryOptions(url.Values{
		"format": []string{"prom"},
	})
	if code != "" || message != "" {
		t.Fatalf("expected valid prom options, got code=%q message=%q", code, message)
	}
	if options.format != metricsFormatProm {
		t.Fatalf("format = %q, want %q", options.format, metricsFormatProm)
	}
}

func TestParseMetricsQueryOptionsInvalid(t *testing.T) {
	tests := []struct {
		name     string
		values   url.Values
		wantCode string
	}{
		{
			name:     "invalid window",
			values:   url.Values{"window": []string{"12h"}},
			wantCode: "invalid_window",
		},
		{
			name:     "invalid include",
			values:   url.Values{"include": []string{"bad"}},
			wantCode: "invalid_include",
		},
		{
			name:     "invalid format",
			values:   url.Values{"format": []string{"yaml"}},
			wantCode: "invalid_format",
		},
		{
			name:     "invalid limit domains",
			values:   url.Values{"limit_domains": []string{"999"}},
			wantCode: "invalid_limit_domains",
		},
		{
			name:     "invalid limit batches",
			values:   url.Values{"limit_batches": []string{"0"}},
			wantCode: "invalid_limit_batches",
		},
	}

	for _, tc := range tests {
		_, code, _ := parseMetricsQueryOptions(tc.values)
		if code != tc.wantCode {
			t.Fatalf("%s: code = %q, want %q", tc.name, code, tc.wantCode)
		}
	}
}

func TestMetricsQueryCacheKeyIsStable(t *testing.T) {
	left := metricsQueryOptions{
		format:       metricsFormatJSON,
		window:       "1h",
		includeAll:   false,
		include:      map[string]bool{"jobs": true, "health": true},
		limitDomains: 5,
		limitBatches: 6,
	}
	right := metricsQueryOptions{
		format:       metricsFormatJSON,
		window:       "1h",
		includeAll:   false,
		include:      map[string]bool{"health": true, "jobs": true},
		limitDomains: 5,
		limitBatches: 6,
	}

	if left.cacheKey() != right.cacheKey() {
		t.Fatalf("cache keys differ: %q vs %q", left.cacheKey(), right.cacheKey())
	}
}

func TestMetricsQueryCacheKeyForPromIgnoresJSONOnlyOptions(t *testing.T) {
	left := metricsQueryOptions{
		format:       metricsFormatProm,
		window:       "1h",
		includeAll:   false,
		include:      map[string]bool{"health": true},
		limitDomains: 5,
		limitBatches: 6,
	}
	right := metricsQueryOptions{
		format:       metricsFormatProm,
		window:       "48h",
		includeAll:   true,
		include:      map[string]bool{},
		limitDomains: 99,
		limitBatches: 100,
	}

	if left.cacheKey() != right.cacheKey() {
		t.Fatalf("cache keys differ: %q vs %q", left.cacheKey(), right.cacheKey())
	}
}
