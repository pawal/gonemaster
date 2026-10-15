package server

import (
	"context"
	"strconv"
)

// analysisViewCacheSize caps the snapshot versions each view cache keeps.
const analysisViewCacheSize = 4

// snapshotViewCache holds the view tables of recently read snapshots.
type snapshotViewCache struct {
	domains     *analysisCache[[]AnalysisSnapshotDomainView]
	nameservers *analysisCache[[]AnalysisSnapshotNameserverView]
	endpoints   *analysisCache[[]AnalysisSnapshotEndpointView]
	asns        *analysisCache[[]AnalysisSnapshotASNView]
	prefixes    *analysisCache[[]AnalysisSnapshotPrefixView]
	tags        *analysisCache[[]AnalysisSnapshotTagView]
}

func newSnapshotViewCache() *snapshotViewCache {
	return &snapshotViewCache{
		domains:     newSizedAnalysisCache[[]AnalysisSnapshotDomainView](analysisViewCacheSize),
		nameservers: newSizedAnalysisCache[[]AnalysisSnapshotNameserverView](analysisViewCacheSize),
		endpoints:   newSizedAnalysisCache[[]AnalysisSnapshotEndpointView](analysisViewCacheSize),
		asns:        newSizedAnalysisCache[[]AnalysisSnapshotASNView](analysisViewCacheSize),
		prefixes:    newSizedAnalysisCache[[]AnalysisSnapshotPrefixView](analysisViewCacheSize),
		tags:        newSizedAnalysisCache[[]AnalysisSnapshotTagView](analysisViewCacheSize),
	}
}

// snapshotKey addresses one version of a snapshot.
func snapshotKey(snap AnalysisCohortSnapshot) string {
	return strconv.FormatInt(snap.ID, 10) + ":" + strconv.FormatInt(snap.UpdatedAt.UnixNano(), 10)
}

// cachedViews reads snap's rows once per version and keeps a non-empty read; callers must not modify them.
func cachedViews[T any](c *analysisCache[[]T], snap AnalysisCohortSnapshot, load func() []T) []T {
	key := snapshotKey(snap)
	if rows, ok := c.get(key); ok {
		return rows
	}
	v, _, _ := c.group.Do(key, func() (any, error) {
		if rows, ok := c.get(key); ok {
			return rows, nil
		}
		rows := load()
		if len(rows) > 0 {
			c.put(key, rows)
		}
		return rows, nil
	})
	return v.([]T)
}

func (s *Server) snapshotDomainViews(ctx context.Context, rs AnalysisReadStore, snap AnalysisCohortSnapshot) []AnalysisSnapshotDomainView {
	// Detached: a shared build must not stop on one caller's timeout.
	ctx = context.WithoutCancel(ctx)
	return cachedViews(s.views.domains, snap, func() []AnalysisSnapshotDomainView {
		return rs.ListSnapshotDomainViews(ctx, snap.ID)
	})
}

func (s *Server) snapshotNameserverViews(rs AnalysisReadStore, snap AnalysisCohortSnapshot) []AnalysisSnapshotNameserverView {
	return cachedViews(s.views.nameservers, snap, func() []AnalysisSnapshotNameserverView {
		return rs.ListSnapshotNameserverViews(snap.ID)
	})
}

func (s *Server) snapshotEndpointViews(rs AnalysisReadStore, snap AnalysisCohortSnapshot) []AnalysisSnapshotEndpointView {
	return cachedViews(s.views.endpoints, snap, func() []AnalysisSnapshotEndpointView {
		return rs.ListSnapshotEndpointViews(snap.ID)
	})
}

func (s *Server) snapshotASNViews(rs AnalysisReadStore, snap AnalysisCohortSnapshot) []AnalysisSnapshotASNView {
	return cachedViews(s.views.asns, snap, func() []AnalysisSnapshotASNView {
		return rs.ListSnapshotASNViews(snap.ID)
	})
}

func (s *Server) snapshotPrefixViews(rs AnalysisReadStore, snap AnalysisCohortSnapshot) []AnalysisSnapshotPrefixView {
	return cachedViews(s.views.prefixes, snap, func() []AnalysisSnapshotPrefixView {
		return rs.ListSnapshotPrefixViews(snap.ID)
	})
}

func (s *Server) snapshotTagViews(rs AnalysisReadStore, snap AnalysisCohortSnapshot) []AnalysisSnapshotTagView {
	return cachedViews(s.views.tags, snap, func() []AnalysisSnapshotTagView {
		return rs.ListSnapshotTagViews(snap.ID)
	})
}
