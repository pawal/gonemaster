package server

import "time"

// analysisMatCacheEntry is one cached snapshot materialization stamped with
// the snapshot's CapturedAt. Captured snapshots are immutable, so the
// entry stays fresh for the snapshot's entire lifetime; the stamp exists
// only to detect the rare edge case of a snapshot being re-materialized
// after an admin-facing fix.
type analysisMatCacheEntry struct {
	stamp time.Time
	data  latestCohortMaterialization
}

// latestMaterializationForSnapshot returns the cached materialization for
// one (cohort, snapshot) pair, computing on first access. The cache key is
// the snapshot ID so every addressable snapshot has exactly one cached
// materialization, and auto-latest resolution naturally shares cache
// entries with explicit-slug requests for the same snapshot.
//
// Callers must first resolve the snapshot via
// (*Server).resolvePublicAnalysisCohortAndSnapshot so the empty-snapshot
// (no captured snapshot yet) state renders a helpful response instead of
// silently returning zero data.
func (s *Server) latestMaterializationForSnapshot(cohort AnalysisCohort, snapshot AnalysisCohortSnapshot) latestCohortMaterialization {
	readStore, ok := s.store.(AnalysisReadStore)
	if !ok {
		return latestCohortMaterialization{}
	}
	if snapshot.ID == 0 {
		return latestCohortMaterialization{}
	}
	s.analysisMatCacheMu.Lock()
	defer s.analysisMatCacheMu.Unlock()
	if entry, hit := s.analysisMatCache[snapshot.ID]; hit && entry.stamp.Equal(snapshot.CapturedAt) {
		return entry.data
	}
	data := computeSnapshotMaterialization(readStore, s.store, cohort.ID, snapshot.BatchID)
	s.analysisMatCache[snapshot.ID] = analysisMatCacheEntry{
		stamp: snapshot.CapturedAt,
		data:  data,
	}
	return data
}
