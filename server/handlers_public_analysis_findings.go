package server

import (
	"net/http"
	"sort"
	"strings"
)

// PublicAnalysisTagView aggregates one finding tag's footprint in the cohort.
type PublicAnalysisTagView struct {
	Tag             string `json:"tag"`
	Module          string `json:"module,omitempty"`
	Level           string `json:"level,omitempty"`
	DomainCount     int    `json:"domain_count"`
	OccurrenceCount int    `json:"occurrence_count"`
}

// PublicAnalysisTestcaseView aggregates one (module, testcase) pair's footprint.
type PublicAnalysisTestcaseView struct {
	Module      string `json:"module"`
	Testcase    string `json:"testcase"`
	DomainCount int    `json:"domain_count"`
	EntryCount  int    `json:"entry_count"`
	WorstLevel  string `json:"worst_level,omitempty"`
	UniqueTags  int    `json:"unique_tags"`
}

// handlePublicAnalysisTags handles GET /pub/api/v1/analysis/tags. It aggregates
// finding tags observed across the latest run per domain in the cohort.
func (s *Server) handlePublicAnalysisTags(w http.ResponseWriter, r *http.Request) {
	cohort, ok := s.resolvePublicAnalysisCohort(w, r)
	if !ok {
		return
	}
	if _, ok := s.analysisReadStore(w); !ok {
		return
	}
	filter, ok := parseAnalysisListFilter(w, r)
	if !ok {
		return
	}
	minLevelRank := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("min_level")); raw != "" {
		minLevelRank = severityRank(raw)
		if minLevelRank == 0 {
			writeError(w, http.StatusBadRequest, "invalid_min_level",
				"min_level must be one of NOTICE, WARNING, ERROR, CRITICAL", nil)
			return
		}
	}

	data := s.latestMaterializationForCohort(cohort)

	// Aggregates come from the materialized analysis_run_tag_summary
	// table via the cohort cache — no per-run entries scan. One bucket
	// per distinct tag across all latest-per-domain runs.
	type tagAgg struct {
		module      string
		level       string
		domains     map[int64]struct{}
		occurrences int
	}
	buckets := map[string]*tagAgg{}
	for _, ts := range data.tagSummaries {
		tag := strings.TrimSpace(ts.Tag)
		if tag == "" {
			continue
		}
		b, exists := buckets[tag]
		if !exists {
			b = &tagAgg{
				module:  ts.Module,
				level:   ts.Level,
				domains: map[int64]struct{}{},
			}
			buckets[tag] = b
		}
		if severityRank(ts.Level) > severityRank(b.level) {
			b.level = ts.Level
		}
		if b.module == "" && ts.Module != "" {
			b.module = ts.Module
		}
		b.domains[ts.DomainID] = struct{}{}
		b.occurrences += ts.OccurrenceCount
	}

	items := make([]PublicAnalysisTagView, 0, len(buckets))
	for tag, b := range buckets {
		items = append(items, PublicAnalysisTagView{
			Tag:             tag,
			Module:          b.module,
			Level:           b.level,
			DomainCount:     len(b.domains),
			OccurrenceCount: b.occurrences,
		})
	}

	if minLevelRank > 0 {
		kept := items[:0]
		for _, it := range items {
			if severityRank(it.Level) >= minLevelRank {
				kept = append(kept, it)
			}
		}
		items = kept
	}
	if filter.Search != "" {
		needle := strings.ToLower(filter.Search)
		kept := items[:0]
		for _, it := range items {
			if strings.Contains(strings.ToLower(it.Tag), needle) ||
				strings.Contains(strings.ToLower(it.Module), needle) {
				kept = append(kept, it)
			}
		}
		items = kept
	}
	sort.Slice(items, func(i, j int) bool {
		switch filter.Sort {
		case "", "domain_count_desc":
			if items[i].DomainCount != items[j].DomainCount {
				return items[i].DomainCount > items[j].DomainCount
			}
		case "domain_count_asc":
			if items[i].DomainCount != items[j].DomainCount {
				return items[i].DomainCount < items[j].DomainCount
			}
		case "occurrence_count_desc":
			if items[i].OccurrenceCount != items[j].OccurrenceCount {
				return items[i].OccurrenceCount > items[j].OccurrenceCount
			}
		case "occurrence_count_asc":
			if items[i].OccurrenceCount != items[j].OccurrenceCount {
				return items[i].OccurrenceCount < items[j].OccurrenceCount
			}
		case "level_desc":
			li, lj := severityRank(items[i].Level), severityRank(items[j].Level)
			if li != lj {
				return li > lj
			}
		case "level_asc":
			li, lj := severityRank(items[i].Level), severityRank(items[j].Level)
			if li != lj {
				return li < lj
			}
		}
		return items[i].Tag < items[j].Tag
	})
	total := len(items)
	start, end := clampPage(filter.Limit, filter.Offset, total)
	writeJSON(w, http.StatusOK, PublicAnalysisListResponse[PublicAnalysisTagView]{
		Items:  items[start:end],
		Total:  total,
		Limit:  filter.Limit,
		Offset: filter.Offset,
	})
}

// handlePublicAnalysisTestcases handles GET /pub/api/v1/analysis/testcases. It
// aggregates (module, testcase) pairs across the latest run per domain.
func (s *Server) handlePublicAnalysisTestcases(w http.ResponseWriter, r *http.Request) {
	cohort, ok := s.resolvePublicAnalysisCohort(w, r)
	if !ok {
		return
	}
	if _, ok := s.analysisReadStore(w); !ok {
		return
	}
	filter, ok := parseAnalysisListFilter(w, r)
	if !ok {
		return
	}

	data := s.latestMaterializationForCohort(cohort)

	// Same move as /tags: aggregate from the cached per-run tag summary
	// table instead of rescanning entries per domain. The tag summary
	// row carries (module, testcase, level, occurrence_count); summing
	// occurrence_count over all runs yields entry_count.
	type testcaseKey struct {
		module   string
		testcase string
	}
	type testcaseAgg struct {
		domains    map[int64]struct{}
		tags       map[string]struct{}
		entries    int
		worstLevel string
	}
	buckets := map[testcaseKey]*testcaseAgg{}
	for _, ts := range data.tagSummaries {
		if strings.TrimSpace(ts.Testcase) == "" {
			continue
		}
		key := testcaseKey{module: ts.Module, testcase: ts.Testcase}
		b, exists := buckets[key]
		if !exists {
			b = &testcaseAgg{
				domains: map[int64]struct{}{},
				tags:    map[string]struct{}{},
			}
			buckets[key] = b
		}
		b.domains[ts.DomainID] = struct{}{}
		b.tags[ts.Tag] = struct{}{}
		b.entries += ts.OccurrenceCount
		if severityRank(ts.Level) > severityRank(b.worstLevel) {
			b.worstLevel = ts.Level
		}
	}

	items := make([]PublicAnalysisTestcaseView, 0, len(buckets))
	for key, b := range buckets {
		items = append(items, PublicAnalysisTestcaseView{
			Module:      key.module,
			Testcase:    key.testcase,
			DomainCount: len(b.domains),
			EntryCount:  b.entries,
			WorstLevel:  b.worstLevel,
			UniqueTags:  len(b.tags),
		})
	}

	if filter.Search != "" {
		needle := strings.ToLower(filter.Search)
		kept := items[:0]
		for _, it := range items {
			if strings.Contains(strings.ToLower(it.Module), needle) ||
				strings.Contains(strings.ToLower(it.Testcase), needle) {
				kept = append(kept, it)
			}
		}
		items = kept
	}
	sort.Slice(items, func(i, j int) bool {
		switch filter.Sort {
		case "", "domain_count_desc":
			if items[i].DomainCount != items[j].DomainCount {
				return items[i].DomainCount > items[j].DomainCount
			}
		case "domain_count_asc":
			if items[i].DomainCount != items[j].DomainCount {
				return items[i].DomainCount < items[j].DomainCount
			}
		case "level_desc":
			li, lj := severityRank(items[i].WorstLevel), severityRank(items[j].WorstLevel)
			if li != lj {
				return li > lj
			}
		case "level_asc":
			li, lj := severityRank(items[i].WorstLevel), severityRank(items[j].WorstLevel)
			if li != lj {
				return li < lj
			}
		}
		if items[i].Module != items[j].Module {
			return items[i].Module < items[j].Module
		}
		return items[i].Testcase < items[j].Testcase
	})
	total := len(items)
	start, end := clampPage(filter.Limit, filter.Offset, total)
	writeJSON(w, http.StatusOK, PublicAnalysisListResponse[PublicAnalysisTestcaseView]{
		Items:  items[start:end],
		Total:  total,
		Limit:  filter.Limit,
		Offset: filter.Offset,
	})
}
