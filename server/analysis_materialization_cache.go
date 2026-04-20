package server

import "time"

// analysisMatCacheEntry is one cached cohort materialization stamped with the
// cohort's LastMaterializedAt at the time it was computed. The entry is
// considered fresh as long as the cohort's stamp still matches.
type analysisMatCacheEntry struct {
	stamp time.Time
	data  latestCohortMaterialization
}

// latestMaterializationForCohort returns the latest materialization for a
// cohort, reusing a cached value when the cohort's LastMaterializedAt hasn't
// advanced since the last compute. Returns an empty materialization if the
// store does not implement AnalysisReadStore.
//
// The method serializes concurrent misses under a single mutex: on a cold
// cache, two parallel requests for the same cohort will not both recompute.
// This is intentionally simple; upgrade to singleflight if per-cohort
// contention becomes a bottleneck.
func (s *Server) latestMaterializationForCohort(cohort AnalysisCohort) latestCohortMaterialization {
	readStore, ok := s.store.(AnalysisReadStore)
	if !ok {
		return latestCohortMaterialization{}
	}
	s.analysisMatCacheMu.Lock()
	defer s.analysisMatCacheMu.Unlock()
	if entry, hit := s.analysisMatCache[cohort.ID]; hit && entry.stamp.Equal(cohort.LastMaterializedAt) {
		return entry.data
	}
	data := computeLatestMaterializationForCohort(readStore, s.store, cohort.ID)
	s.analysisMatCache[cohort.ID] = analysisMatCacheEntry{
		stamp: cohort.LastMaterializedAt,
		data:  data,
	}
	return data
}
