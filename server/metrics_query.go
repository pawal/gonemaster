package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const metricsCacheTTL = time.Second

var metricsIncludeSections = map[string]struct{}{
	"health":   {},
	"jobs":     {},
	"api":      {},
	"quality":  {},
	"insights": {},
	"trends":   {},
}

type metricsQueryOptions struct {
	window       string
	includeAll   bool
	include      map[string]bool
	limitDomains int
	limitBatches int
}

type metricsCacheEntry struct {
	ExpiresAt time.Time
	Payload   []byte
}

func parseMetricsQueryOptions(values url.Values) (metricsQueryOptions, string, string) {
	options := metricsQueryOptions{
		includeAll: true,
		include:    map[string]bool{},
	}

	if raw := strings.TrimSpace(values.Get("window")); raw != "" {
		if _, ok := selectMetricsTrendWindow(raw); !ok {
			return metricsQueryOptions{}, "invalid_window", "window must be one of 1h, 6h, 24h, 48h"
		}
		options.window = strings.ToLower(raw)
	}

	if raw := strings.TrimSpace(values.Get("include")); raw != "" {
		options.includeAll = false
		for _, part := range strings.Split(raw, ",") {
			section := strings.ToLower(strings.TrimSpace(part))
			if section == "" {
				continue
			}
			if section == "all" {
				options.includeAll = true
				options.include = map[string]bool{}
				break
			}
			if _, ok := metricsIncludeSections[section]; !ok {
				return metricsQueryOptions{}, "invalid_include", "include must only contain all, health, jobs, api, quality, insights, trends"
			}
			options.include[section] = true
		}
		if !options.includeAll && len(options.include) == 0 {
			return metricsQueryOptions{}, "invalid_include", "include must only contain all, health, jobs, api, quality, insights, trends"
		}
	}

	if raw := strings.TrimSpace(values.Get("limit_domains")); raw != "" {
		limitValue, err := strconv.Atoi(raw)
		if err != nil || limitValue <= 0 || limitValue > metricsMaxDomainLimit {
			return metricsQueryOptions{}, "invalid_limit_domains", "limit_domains must be between 1 and 100"
		}
		options.limitDomains = limitValue
	}
	if raw := strings.TrimSpace(values.Get("limit_batches")); raw != "" {
		limitValue, err := strconv.Atoi(raw)
		if err != nil || limitValue <= 0 || limitValue > metricsMaxBatchLimit {
			return metricsQueryOptions{}, "invalid_limit_batches", "limit_batches must be between 1 and 100"
		}
		options.limitBatches = limitValue
	}

	return options, "", ""
}

func (o metricsQueryOptions) cacheKey() string {
	parts := []string{
		"window=" + o.window,
		"limit_domains=" + strconv.Itoa(o.limitDomains),
		"limit_batches=" + strconv.Itoa(o.limitBatches),
	}
	if o.includeAll {
		parts = append(parts, "include=all")
	} else {
		includes := make([]string, 0, len(o.include))
		for section := range o.include {
			includes = append(includes, section)
		}
		sort.Strings(includes)
		parts = append(parts, "include="+strings.Join(includes, ","))
	}
	return strings.Join(parts, "|")
}

func (s *Server) buildMetricsResponseBody(options metricsQueryOptions) ([]byte, error) {
	snapshot := s.metrics.SnapshotWithLimits(options.limitDomains, options.limitBatches)

	if options.window != "" {
		if selectedWindow, ok := snapshot.Trends.Windows[options.window]; ok {
			snapshot.Trends.Windows = map[string]MetricsTrendWindowSnapshot{
				options.window: selectedWindow,
			}
		} else {
			snapshot.Trends.Windows = map[string]MetricsTrendWindowSnapshot{}
		}
	}

	if options.includeAll {
		return json.Marshal(snapshot)
	}

	payload := map[string]any{
		"schema_version": snapshot.SchemaVersion,
		"generated_at":   snapshot.GeneratedAt,
	}
	for section := range options.include {
		switch section {
		case "health":
			payload["health"] = snapshot.Health
		case "jobs":
			payload["jobs"] = snapshot.Jobs
		case "api":
			payload["api"] = snapshot.API
		case "quality":
			payload["quality"] = snapshot.Quality
		case "insights":
			payload["insights"] = snapshot.Insights
		case "trends":
			payload["trends"] = snapshot.Trends
		}
	}
	return json.Marshal(payload)
}

func (s *Server) metricsNow() time.Time {
	if s != nil && s.metrics != nil && s.metrics.nowFn != nil {
		return s.metrics.nowFn().UTC()
	}
	return time.Now().UTC()
}

func (s *Server) getMetricsCache(key string, now time.Time) ([]byte, bool) {
	s.metricsCacheMu.Lock()
	defer s.metricsCacheMu.Unlock()
	entry, ok := s.metricsCache[key]
	if !ok {
		return nil, false
	}
	if now.After(entry.ExpiresAt) {
		delete(s.metricsCache, key)
		return nil, false
	}
	out := make([]byte, len(entry.Payload))
	copy(out, entry.Payload)
	return out, true
}

func (s *Server) putMetricsCache(key string, payload []byte, now time.Time) {
	clone := make([]byte, len(payload))
	copy(clone, payload)

	s.metricsCacheMu.Lock()
	defer s.metricsCacheMu.Unlock()
	for cacheKey, entry := range s.metricsCache {
		if now.After(entry.ExpiresAt) {
			delete(s.metricsCache, cacheKey)
		}
	}
	s.metricsCache[key] = metricsCacheEntry{
		ExpiresAt: now.Add(metricsCacheTTL),
		Payload:   clone,
	}
}

func writeRawJSON(w http.ResponseWriter, status int, payload []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(payload)
}
