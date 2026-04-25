package server

import (
	"strconv"
	"time"

	"golang.org/x/sync/singleflight"
)

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
// one (cohort, snapshot) pair. Concurrent requests for the same snapshot
// share one in-flight compute via singleflight; concurrent requests for
// different snapshots run in parallel. The map mutation lock is only held
// for the slot read/write, never across the compute.
func (s *Server) latestMaterializationForSnapshot(cohort AnalysisCohort, snapshot AnalysisCohortSnapshot) latestCohortMaterialization {
	readStore, ok := s.store.(AnalysisReadStore)
	if !ok {
		return latestCohortMaterialization{}
	}
	if snapshot.ID == 0 {
		return latestCohortMaterialization{}
	}

	if entry, hit := s.lookupAnalysisMatCache(snapshot.ID); hit && entry.stamp.Equal(snapshot.CapturedAt) {
		return entry.data
	}

	key := matCacheKey(snapshot.ID, snapshot.CapturedAt)
	v, _, _ := s.analysisMatGroup.Do(key, func() (any, error) {
		if entry, hit := s.lookupAnalysisMatCache(snapshot.ID); hit && entry.stamp.Equal(snapshot.CapturedAt) {
			return entry.data, nil
		}
		data := computeSnapshotMaterialization(readStore, s.store, cohort.ID, snapshot.BatchID)
		s.storeAnalysisMatCache(snapshot.ID, analysisMatCacheEntry{stamp: snapshot.CapturedAt, data: data})
		return data, nil
	})
	return v.(latestCohortMaterialization)
}

func (s *Server) lookupAnalysisMatCache(snapshotID int64) (analysisMatCacheEntry, bool) {
	s.analysisMatCacheMu.Lock()
	defer s.analysisMatCacheMu.Unlock()
	entry, ok := s.analysisMatCache[snapshotID]
	return entry, ok
}

func (s *Server) storeAnalysisMatCache(snapshotID int64, entry analysisMatCacheEntry) {
	s.analysisMatCacheMu.Lock()
	defer s.analysisMatCacheMu.Unlock()
	s.analysisMatCache[snapshotID] = entry
}

// matCacheKey scopes the singleflight by (snapshot, captured_at) so a
// re-materialization (which bumps captured_at) does not coalesce with
// in-flight requests holding the older payload.
func matCacheKey(snapshotID int64, capturedAt time.Time) string {
	return strconv.FormatInt(snapshotID, 10) + ":" + strconv.FormatInt(capturedAt.Unix(), 10)
}

// matSingleflightGroup aliases the singleflight type so the Server fields
// stay narrow.
type matSingleflightGroup = singleflight.Group
