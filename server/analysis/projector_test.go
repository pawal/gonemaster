package analysis

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	serverpkg "codeberg.org/pawal/gonemaster/server"
)

type fakeStore struct {
	runs    map[string]serverpkg.Run
	entries map[string][]serverpkg.Entry
	tags    map[int64][]string
	cohorts []serverpkg.AnalysisCohort

	// Queue + batch catalog used by the Phase 2 snapshot lifecycle tests.
	// These are intentionally simple maps — the unit tests exercise
	// controller logic, not storage semantics.
	queuedJobs map[string][]string // batchID → []jobID (only in-flight jobs)
	batches    map[string]serverpkg.Batch

	nextCohortID     int64
	nextNameserverID int64
	nextAddressID    int64
	nextPrefixID     int64

	nameserversByName map[string]serverpkg.AnalysisNameserver
	addressesByValue  map[string]serverpkg.AnalysisAddress
	prefixesByValue   map[string]serverpkg.AnalysisPrefix
	asnsByValue       map[int64]serverpkg.AnalysisASN

	nsEndpoints  map[string][]serverpkg.AnalysisRunNameserverEndpoint
	addrFacts    map[string][]serverpkg.AnalysisRunAddressASN
	domainASNs   map[string][]serverpkg.AnalysisRunDomainASN
	tagSummaries map[string][]serverpkg.AnalysisRunTagSummary
	domainFacts  map[string][]serverpkg.AnalysisRunDomainFact
	summaries    map[string]serverpkg.AnalysisRunDomainSummary
	states       map[string]serverpkg.AnalysisProjectionState

	// Snapshot state keyed by (cohortID, batchID).
	snapshots      map[snapshotKey]serverpkg.AnalysisCohortSnapshot
	snapshotByID   map[int64]snapshotKey
	snapshotAggs   map[int64][]serverpkg.AnalysisCohortSnapshotAggregate
	nextSnapshotID int64

	settings map[string]string
}

type snapshotKey struct {
	cohortID int64
	batchID  string
}

func (s *fakeStore) GetRun(id string) (serverpkg.Run, bool) {
	run, ok := s.runs[id]
	return run, ok
}

func (s *fakeStore) QueryEntries(filter serverpkg.EntryFilter) serverpkg.EntryList {
	items := append([]serverpkg.Entry(nil), s.entries[filter.RunID]...)
	if filter.Limit > 0 && len(items) > filter.Limit {
		items = items[:filter.Limit]
	}
	return serverpkg.EntryList{
		Items:  items,
		Total:  len(s.entries[filter.RunID]),
		Limit:  filter.Limit,
		Offset: filter.Offset,
	}
}

func (s *fakeStore) GetDomainTags(domainID int64) []string {
	return append([]string(nil), s.tags[domainID]...)
}

func (s *fakeStore) ListAnalysisCohorts() []serverpkg.AnalysisCohort {
	return append([]serverpkg.AnalysisCohort(nil), s.cohorts...)
}

func (s *fakeStore) GetAnalysisCohort(id int64) (serverpkg.AnalysisCohort, bool) {
	for _, cohort := range s.cohorts {
		if cohort.ID == id {
			return cohort, true
		}
	}
	return serverpkg.AnalysisCohort{}, false
}

func (s *fakeStore) SetAnalysisCohortProgress(cohortID int64, done, total int) error {
	for i, existing := range s.cohorts {
		if existing.ID == cohortID {
			s.cohorts[i].MaterializationDone = done
			s.cohorts[i].MaterializationTotal = total
			return nil
		}
	}
	return nil
}

func (s *fakeStore) UpsertAnalysisCohort(cohort serverpkg.AnalysisCohort) (serverpkg.AnalysisCohort, error) {
	now := time.Now().UTC()
	for i, existing := range s.cohorts {
		if cohort.ID != 0 && existing.ID == cohort.ID {
			if cohort.CreatedAt.IsZero() {
				cohort.CreatedAt = existing.CreatedAt
			}
			if cohort.UpdatedAt.IsZero() {
				cohort.UpdatedAt = now
			}
			s.cohorts[i] = cohort
			return cohort, nil
		}
		if existing.SourceType == cohort.SourceType && existing.SourceTag == cohort.SourceTag {
			cohort.ID = existing.ID
			if cohort.CreatedAt.IsZero() {
				cohort.CreatedAt = existing.CreatedAt
			}
			if cohort.UpdatedAt.IsZero() {
				cohort.UpdatedAt = now
			}
			s.cohorts[i] = cohort
			return cohort, nil
		}
	}
	if cohort.ID == 0 {
		s.nextCohortID++
		cohort.ID = s.nextCohortID
	}
	if cohort.CreatedAt.IsZero() {
		cohort.CreatedAt = now
	}
	if cohort.UpdatedAt.IsZero() {
		cohort.UpdatedAt = now
	}
	s.cohorts = append(s.cohorts, cohort)
	return cohort, nil
}

func (s *fakeStore) ClearAnalysisCohortMaterialization(cohortID int64) error {
	s.ensureMaterializedMaps()
	prefix := fmt.Sprintf("%d/", cohortID)
	for key := range s.nsEndpoints {
		if strings.HasPrefix(key, prefix) {
			delete(s.nsEndpoints, key)
		}
	}
	for key := range s.addrFacts {
		if strings.HasPrefix(key, prefix) {
			delete(s.addrFacts, key)
		}
	}
	for key := range s.domainASNs {
		if strings.HasPrefix(key, prefix) {
			delete(s.domainASNs, key)
		}
	}
	for key := range s.tagSummaries {
		if strings.HasPrefix(key, prefix) {
			delete(s.tagSummaries, key)
		}
	}
	for key := range s.domainFacts {
		if strings.HasPrefix(key, prefix) {
			delete(s.domainFacts, key)
		}
	}
	for key := range s.summaries {
		if strings.HasPrefix(key, prefix) {
			delete(s.summaries, key)
		}
	}
	for key := range s.states {
		if strings.HasPrefix(key, prefix) {
			delete(s.states, key)
		}
	}
	return nil
}

func (s *fakeStore) ListRuns(filter serverpkg.RunFilter) serverpkg.RunList {
	items := make([]serverpkg.Run, 0, len(s.runs))
	for _, run := range s.runs {
		if filter.DomainID != 0 && run.DomainID != filter.DomainID {
			continue
		}
		if filter.Tag != "" {
			if !containsString(s.tags[run.DomainID], filter.Tag) {
				continue
			}
		}
		if !filter.FinishedAfter.IsZero() && !run.FinishedAt.After(filter.FinishedAfter) {
			continue
		}
		items = append(items, run)
	}
	sort.Slice(items, func(i, j int) bool {
		if !items[i].FinishedAt.Equal(items[j].FinishedAt) {
			return items[i].FinishedAt.After(items[j].FinishedAt)
		}
		return items[i].ID < items[j].ID
	})
	total := len(items)
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return serverpkg.RunList{
		Items:  append([]serverpkg.Run(nil), items[offset:end]...),
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}
}

func (s *fakeStore) UpsertAnalysisNameserver(name string, seenAt time.Time) (serverpkg.AnalysisNameserver, error) {
	s.ensureMaterializedMaps()
	if item, ok := s.nameserversByName[name]; ok {
		item.FirstSeenAt = minTestTime(item.FirstSeenAt, seenAt)
		item.LastSeenAt = maxTestTime(item.LastSeenAt, seenAt)
		s.nameserversByName[name] = item
		return item, nil
	}
	s.nextNameserverID++
	item := serverpkg.AnalysisNameserver{
		ID:          s.nextNameserverID,
		Name:        name,
		FirstSeenAt: seenAt.UTC(),
		LastSeenAt:  seenAt.UTC(),
	}
	s.nameserversByName[name] = item
	return item, nil
}

func (s *fakeStore) UpsertAnalysisAddress(address, family string, seenAt time.Time) (serverpkg.AnalysisAddress, error) {
	s.ensureMaterializedMaps()
	if item, ok := s.addressesByValue[address]; ok {
		item.Family = family
		item.FirstSeenAt = minTestTime(item.FirstSeenAt, seenAt)
		item.LastSeenAt = maxTestTime(item.LastSeenAt, seenAt)
		s.addressesByValue[address] = item
		return item, nil
	}
	s.nextAddressID++
	item := serverpkg.AnalysisAddress{
		ID:          s.nextAddressID,
		Address:     address,
		Family:      family,
		FirstSeenAt: seenAt.UTC(),
		LastSeenAt:  seenAt.UTC(),
	}
	s.addressesByValue[address] = item
	return item, nil
}

func (s *fakeStore) UpsertAnalysisPrefix(prefix, family string, seenAt time.Time) (serverpkg.AnalysisPrefix, error) {
	s.ensureMaterializedMaps()
	if item, ok := s.prefixesByValue[prefix]; ok {
		item.Family = family
		item.FirstSeenAt = minTestTime(item.FirstSeenAt, seenAt)
		item.LastSeenAt = maxTestTime(item.LastSeenAt, seenAt)
		s.prefixesByValue[prefix] = item
		return item, nil
	}
	s.nextPrefixID++
	item := serverpkg.AnalysisPrefix{
		ID:          s.nextPrefixID,
		Prefix:      prefix,
		Family:      family,
		FirstSeenAt: seenAt.UTC(),
		LastSeenAt:  seenAt.UTC(),
	}
	s.prefixesByValue[prefix] = item
	return item, nil
}

func (s *fakeStore) UpsertAnalysisASN(asn int64, label string, seenAt time.Time) (serverpkg.AnalysisASN, error) {
	s.ensureMaterializedMaps()
	if item, ok := s.asnsByValue[asn]; ok {
		if label != "" {
			item.Label = label
		}
		item.FirstSeenAt = minTestTime(item.FirstSeenAt, seenAt)
		item.LastSeenAt = maxTestTime(item.LastSeenAt, seenAt)
		s.asnsByValue[asn] = item
		return item, nil
	}
	item := serverpkg.AnalysisASN{
		ASN:         asn,
		Label:       label,
		FirstSeenAt: seenAt.UTC(),
		LastSeenAt:  seenAt.UTC(),
	}
	s.asnsByValue[asn] = item
	return item, nil
}

func (s *fakeStore) ReplaceAnalysisRunNSEndpoints(cohortID int64, runID string, items []serverpkg.AnalysisRunNameserverEndpoint) error {
	s.ensureMaterializedMaps()
	s.nsEndpoints[projectionKey(cohortID, runID)] = append([]serverpkg.AnalysisRunNameserverEndpoint(nil), items...)
	return nil
}

func (s *fakeStore) ReplaceAnalysisRunAddressASNs(cohortID int64, runID string, items []serverpkg.AnalysisRunAddressASN) error {
	s.ensureMaterializedMaps()
	s.addrFacts[projectionKey(cohortID, runID)] = append([]serverpkg.AnalysisRunAddressASN(nil), items...)
	return nil
}

func (s *fakeStore) ReplaceAnalysisRunDomainASNs(cohortID int64, runID string, items []serverpkg.AnalysisRunDomainASN) error {
	s.ensureMaterializedMaps()
	s.domainASNs[projectionKey(cohortID, runID)] = append([]serverpkg.AnalysisRunDomainASN(nil), items...)
	return nil
}

func (s *fakeStore) ReplaceAnalysisRunTagSummaries(cohortID int64, runID string, items []serverpkg.AnalysisRunTagSummary) error {
	s.ensureMaterializedMaps()
	s.tagSummaries[projectionKey(cohortID, runID)] = append([]serverpkg.AnalysisRunTagSummary(nil), items...)
	return nil
}

func (s *fakeStore) ReplaceAnalysisRunDomainFacts(cohortID int64, runID string, items []serverpkg.AnalysisRunDomainFact) error {
	s.ensureMaterializedMaps()
	s.domainFacts[projectionKey(cohortID, runID)] = append([]serverpkg.AnalysisRunDomainFact(nil), items...)
	return nil
}

func (s *fakeStore) UpsertAnalysisRunDomainSummary(item serverpkg.AnalysisRunDomainSummary) error {
	s.ensureMaterializedMaps()
	s.summaries[projectionKey(item.CohortID, item.RunID)] = item
	return nil
}

func (s *fakeStore) SetAnalysisProjectionState(item serverpkg.AnalysisProjectionState) error {
	s.ensureMaterializedMaps()
	s.states[projectionKey(item.CohortID, item.RunID)] = item
	return nil
}

func (s *fakeStore) GetBatch(id string) (serverpkg.Batch, bool) {
	b, ok := s.batches[id]
	return b, ok
}

// Backfill support — Phase 7 tests drive these directly.
func (s *fakeStore) ListCohortBatchesWithFacts() ([]serverpkg.CohortBatchFactStats, error) {
	type key struct {
		cohortID int64
		batchID  string
	}
	agg := map[key]*serverpkg.CohortBatchFactStats{}
	for k, summary := range s.summaries {
		parts := strings.SplitN(k, "/", 2)
		if len(parts) != 2 {
			continue
		}
		run, ok := s.runs[summary.RunID]
		if !ok || run.BatchID == "" {
			continue
		}
		var cohortID int64
		fmt.Sscanf(parts[0], "%d", &cohortID)
		gk := key{cohortID, run.BatchID}
		stat, ok := agg[gk]
		if !ok {
			stat = &serverpkg.CohortBatchFactStats{CohortID: cohortID, BatchID: run.BatchID}
			agg[gk] = stat
		}
		stat.RunCount++
		stat.DomainCount++
		if stat.FirstFinished.IsZero() || run.FinishedAt.Before(stat.FirstFinished) {
			stat.FirstFinished = run.FinishedAt
		}
		if run.FinishedAt.After(stat.LastFinished) {
			stat.LastFinished = run.FinishedAt
		}
	}
	out := make([]serverpkg.CohortBatchFactStats, 0, len(agg))
	for _, v := range agg {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CohortID != out[j].CohortID {
			return out[i].CohortID < out[j].CohortID
		}
		return out[i].BatchID < out[j].BatchID
	})
	return out, nil
}

func (s *fakeStore) GetSetting(key string) (string, bool) {
	if s.settings == nil {
		return "", false
	}
	v, ok := s.settings[key]
	return v, ok
}

func (s *fakeStore) SetSetting(key, value string) error {
	if s.settings == nil {
		s.settings = map[string]string{}
	}
	s.settings[key] = value
	return nil
}

func (s *fakeStore) UpsertAnalysisCohortSnapshot(snap serverpkg.AnalysisCohortSnapshot) (serverpkg.AnalysisCohortSnapshot, error) {
	s.ensureSnapshotMaps()
	now := time.Now().UTC()
	key := snapshotKey{cohortID: snap.CohortID, batchID: snap.BatchID}
	if existing, ok := s.snapshots[key]; ok {
		snap.ID = existing.ID
		if snap.CreatedAt.IsZero() {
			snap.CreatedAt = existing.CreatedAt
		}
		if snap.UpdatedAt.IsZero() {
			snap.UpdatedAt = now
		}
		s.snapshots[key] = snap
		s.snapshotByID[snap.ID] = key
		return snap, nil
	}
	s.nextSnapshotID++
	snap.ID = s.nextSnapshotID
	if snap.CreatedAt.IsZero() {
		snap.CreatedAt = now
	}
	if snap.UpdatedAt.IsZero() {
		snap.UpdatedAt = snap.CreatedAt
	}
	s.snapshots[key] = snap
	s.snapshotByID[snap.ID] = key
	return snap, nil
}

func (s *fakeStore) GetAnalysisCohortSnapshotByBatch(cohortID int64, batchID string) (serverpkg.AnalysisCohortSnapshot, bool) {
	s.ensureSnapshotMaps()
	snap, ok := s.snapshots[snapshotKey{cohortID: cohortID, batchID: batchID}]
	return snap, ok
}

func (s *fakeStore) ListPendingAnalysisCohortSnapshots() []serverpkg.AnalysisCohortSnapshot {
	s.ensureSnapshotMaps()
	out := make([]serverpkg.AnalysisCohortSnapshot, 0)
	for _, snap := range s.snapshots {
		if snap.Status == serverpkg.AnalysisSnapshotStatusPending {
			out = append(out, snap)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CohortID != out[j].CohortID {
			return out[i].CohortID < out[j].CohortID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

func (s *fakeStore) ClearAnalysisCohortSnapshots(cohortID int64) error {
	s.ensureSnapshotMaps()
	for key, snap := range s.snapshots {
		if key.cohortID != cohortID {
			continue
		}
		delete(s.snapshots, key)
		delete(s.snapshotByID, snap.ID)
		delete(s.snapshotAggs, snap.ID)
	}
	return nil
}

func (s *fakeStore) CountBatchSnapshotRuns(cohortID int64, batchID string) (int, int, time.Time, time.Time, error) {
	runs := map[string]struct{}{}
	domains := map[int64]struct{}{}
	var first, last time.Time
	for _, run := range s.runs {
		if run.BatchID != batchID {
			continue
		}
		// Require the run to have produced a summary row for the target
		// cohort — otherwise rebuild scenarios would double-count runs that
		// belong to a different tag.
		if _, ok := s.summaries[projectionKey(cohortID, run.ID)]; !ok {
			continue
		}
		runs[run.ID] = struct{}{}
		domains[run.DomainID] = struct{}{}
		if first.IsZero() || run.FinishedAt.Before(first) {
			first = run.FinishedAt
		}
		if last.IsZero() || run.FinishedAt.After(last) {
			last = run.FinishedAt
		}
	}
	return len(runs), len(domains), first, last, nil
}

func (s *fakeStore) CountOutstandingJobsForBatch(batchID string) (int, error) {
	return len(s.queuedJobs[batchID]), nil
}

func (s *fakeStore) ComputeSnapshotAggregates(cohortID int64, batchID string) ([]serverpkg.AnalysisCohortSnapshotAggregate, error) {
	now := time.Now().UTC()
	// Minimal but non-empty payload so tests can observe that the capture
	// path wrote aggregates — real fact-based payloads live in the SQL
	// store's dedicated test file.
	grades := map[string]int{}
	prefix := fmt.Sprintf("%d/", cohortID)
	for key, summary := range s.summaries {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		run, ok := s.runs[summary.RunID]
		if !ok || run.BatchID != batchID {
			continue
		}
		if summary.Grade != nil && *summary.Grade != "" {
			grades[*summary.Grade]++
		}
	}
	payload, err := json.Marshal(grades)
	if err != nil {
		return nil, err
	}
	return []serverpkg.AnalysisCohortSnapshotAggregate{
		{SnapshotID: 0, Category: serverpkg.SnapshotAggregateGradeDistribution, PayloadJSON: string(payload), ComputedAt: now},
	}, nil
}

func (s *fakeStore) ReplaceSnapshotAggregates(snapshotID int64, aggs []serverpkg.AnalysisCohortSnapshotAggregate) error {
	s.ensureSnapshotMaps()
	out := make([]serverpkg.AnalysisCohortSnapshotAggregate, 0, len(aggs))
	for _, agg := range aggs {
		agg.SnapshotID = snapshotID
		out = append(out, agg)
	}
	s.snapshotAggs[snapshotID] = out
	return nil
}

func (s *fakeStore) ensureSnapshotMaps() {
	if s.snapshots == nil {
		s.snapshots = map[snapshotKey]serverpkg.AnalysisCohortSnapshot{}
	}
	if s.snapshotByID == nil {
		s.snapshotByID = map[int64]snapshotKey{}
	}
	if s.snapshotAggs == nil {
		s.snapshotAggs = map[int64][]serverpkg.AnalysisCohortSnapshotAggregate{}
	}
	if s.batches == nil {
		s.batches = map[string]serverpkg.Batch{}
	}
	if s.queuedJobs == nil {
		s.queuedJobs = map[string][]string{}
	}
}

func (s *fakeStore) ensureMaterializedMaps() {
	if s.nameserversByName == nil {
		s.nameserversByName = map[string]serverpkg.AnalysisNameserver{}
	}
	if s.addressesByValue == nil {
		s.addressesByValue = map[string]serverpkg.AnalysisAddress{}
	}
	if s.prefixesByValue == nil {
		s.prefixesByValue = map[string]serverpkg.AnalysisPrefix{}
	}
	if s.asnsByValue == nil {
		s.asnsByValue = map[int64]serverpkg.AnalysisASN{}
	}
	if s.nsEndpoints == nil {
		s.nsEndpoints = map[string][]serverpkg.AnalysisRunNameserverEndpoint{}
	}
	if s.addrFacts == nil {
		s.addrFacts = map[string][]serverpkg.AnalysisRunAddressASN{}
	}
	if s.domainASNs == nil {
		s.domainASNs = map[string][]serverpkg.AnalysisRunDomainASN{}
	}
	if s.tagSummaries == nil {
		s.tagSummaries = map[string][]serverpkg.AnalysisRunTagSummary{}
	}
	if s.domainFacts == nil {
		s.domainFacts = map[string][]serverpkg.AnalysisRunDomainFact{}
	}
	if s.summaries == nil {
		s.summaries = map[string]serverpkg.AnalysisRunDomainSummary{}
	}
	if s.states == nil {
		s.states = map[string]serverpkg.AnalysisProjectionState{}
	}
}

func minTestTime(a, b time.Time) time.Time {
	if a.IsZero() || b.Before(a) {
		return b.UTC()
	}
	return a.UTC()
}

func maxTestTime(a, b time.Time) time.Time {
	if a.IsZero() || b.After(a) {
		return b.UTC()
	}
	return a.UTC()
}

func projectionKey(cohortID int64, runID string) string {
	return fmt.Sprintf("%d/%s", cohortID, runID)
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func TestMatchAnalysisEnabledCohorts(t *testing.T) {
	cohorts := []serverpkg.AnalysisCohort{
		{
			ID:              1,
			SourceType:      "tag",
			SourceTag:       "gov",
			Label:           "Government",
			AnalysisEnabled: true,
			SortOrder:       20,
		},
		{
			ID:              2,
			SourceType:      "tag",
			SourceTag:       "tld",
			Label:           "TLD",
			AnalysisEnabled: true,
			SortOrder:       10,
		},
		{
			ID:              3,
			SourceType:      "tag",
			SourceTag:       "private",
			Label:           "Private",
			AnalysisEnabled: false,
			SortOrder:       5,
		},
		{
			ID:              4,
			SourceType:      "batch",
			SourceTag:       "ignored",
			Label:           "Ignored",
			AnalysisEnabled: true,
			SortOrder:       1,
		},
	}

	got := MatchAnalysisEnabledCohorts([]string{"tld", "gov"}, cohorts)
	if len(got) != 2 {
		t.Fatalf("expected 2 matching cohorts, got %d", len(got))
	}
	if got[0].SourceTag != "tld" || got[1].SourceTag != "gov" {
		t.Fatalf("unexpected cohort order: %+v", got)
	}
}

func TestProjectorLoadCompletedRun(t *testing.T) {
	run := serverpkg.Run{
		ID:         "run-1",
		DomainID:   101,
		Domain:     "example.test",
		Status:     serverpkg.JobSucceeded,
		EntryCount: 2,
		NameserverTimings: []serverpkg.NameserverTiming{
			{Nameserver: "ns1.example.test", Address: "192.0.2.10", AvgMS: 11},
		},
	}
	store := &fakeStore{
		runs: map[string]serverpkg.Run{
			run.ID: run,
		},
		entries: map[string][]serverpkg.Entry{
			run.ID: {
				{RunID: run.ID, DomainID: run.DomainID, Module: "BASIC", Tag: "TEST1", Level: "NOTICE"},
				{RunID: run.ID, DomainID: run.DomainID, Module: "DNSSEC", Tag: "TEST2", Level: "ERROR"},
			},
		},
		tags: map[int64][]string{
			run.DomainID: {"tld", "signed"},
		},
		cohorts: []serverpkg.AnalysisCohort{
			{
				ID:              2,
				SourceType:      "tag",
				SourceTag:       "tld",
				Label:           "TLD",
				AnalysisEnabled: true,
				SortOrder:       10,
			},
			{
				ID:              1,
				SourceType:      "tag",
				SourceTag:       "signed",
				Label:           "Signed",
				AnalysisEnabled: true,
				SortOrder:       20,
			},
			{
				ID:              3,
				SourceType:      "tag",
				SourceTag:       "hidden",
				Label:           "Hidden",
				AnalysisEnabled: false,
				SortOrder:       5,
			},
		},
	}

	projector := NewProjector(store)
	input, err := projector.LoadCompletedRun(run.ID)
	if err != nil {
		t.Fatalf("LoadCompletedRun: %v", err)
	}

	if input.Run.ID != run.ID {
		t.Fatalf("Run.ID: got %q want %q", input.Run.ID, run.ID)
	}
	if len(input.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(input.Entries))
	}
	if len(input.DomainTags) != 2 || input.DomainTags[0] != "tld" {
		t.Fatalf("unexpected domain tags: %+v", input.DomainTags)
	}
	if len(input.NameserverTimings) != 1 || input.NameserverTimings[0].Nameserver != "ns1.example.test" {
		t.Fatalf("unexpected nameserver timings: %+v", input.NameserverTimings)
	}
	if len(input.MatchingCohorts) != 2 {
		t.Fatalf("expected 2 matching cohorts, got %d", len(input.MatchingCohorts))
	}
	if input.MatchingCohorts[0].SourceTag != "tld" || input.MatchingCohorts[1].SourceTag != "signed" {
		t.Fatalf("unexpected matching cohorts: %+v", input.MatchingCohorts)
	}
}

func TestProjectorLoadCompletedRunMissingRun(t *testing.T) {
	projector := NewProjector(&fakeStore{})
	_, err := projector.LoadCompletedRun("missing")
	if !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("expected ErrRunNotFound, got %v", err)
	}
}

func TestProjectorLoadCompletedRunUsesRunEntryCountAsLimit(t *testing.T) {
	run := serverpkg.Run{
		ID:         "run-2",
		DomainID:   202,
		Domain:     "limit.test",
		Status:     serverpkg.JobSucceeded,
		EntryCount: 1,
		CreatedAt:  time.Now().UTC(),
	}
	store := &fakeStore{
		runs: map[string]serverpkg.Run{
			run.ID: run,
		},
		entries: map[string][]serverpkg.Entry{
			run.ID: {
				{RunID: run.ID, DomainID: run.DomainID, Tag: "ONE"},
				{RunID: run.ID, DomainID: run.DomainID, Tag: "TWO"},
			},
		},
		tags: map[int64][]string{
			run.DomainID: {"gov"},
		},
		cohorts: []serverpkg.AnalysisCohort{
			{ID: 1, SourceType: "tag", SourceTag: "gov", Label: "Government", AnalysisEnabled: true},
		},
	}

	input, err := NewProjector(store).LoadCompletedRun(run.ID)
	if err != nil {
		t.Fatalf("LoadCompletedRun: %v", err)
	}
	if len(input.Entries) != 1 {
		t.Fatalf("expected entry query to honor EntryCount=1, got %d entries", len(input.Entries))
	}
}

// TestDeriveRunDomainSummaryExcludesParentRoleEndpoints pins the fix for
// the bug where a TLD's per-domain nameserver_count included the 13 root
// servers seen while traversing the delegation chain. deriveRunDomainSummary
// must filter out parent-role endpoints so the stored summary matches the
// authoritative-only view the /domains detail and /nameservers list
// render from the materialization cache.
func TestDeriveRunDomainSummaryExcludesParentRoleEndpoints(t *testing.T) {
	input := RunInput{
		Run: serverpkg.Run{ID: "run-x", DomainID: 1, WorstLevel: "NOTICE"},
	}
	endpoints := []extractedEndpoint{
		{nameserver: "ns1.example", address: "192.0.2.1", family: "ipv4", role: "authoritative"},
		{nameserver: "ns1.example", address: "2001:db8::1", family: "ipv6", role: "authoritative"},
		{nameserver: "ns2.example", address: "192.0.2.2", family: "ipv4", role: "authoritative"},
		// Two parent-role endpoints that must NOT be counted.
		{nameserver: "a.root-servers.net", address: "198.41.0.4", family: "ipv4", role: "parent"},
		{nameserver: "b.root-servers.net", address: "170.247.170.2", family: "ipv4", role: "parent"},
	}
	got := deriveRunDomainSummary(input, endpoints, nil)
	if got.NameserverCount != 2 {
		t.Fatalf("NameserverCount: expected 2 authoritative, got %d (endpoints=%+v)", got.NameserverCount, endpoints)
	}
	// Three distinct (ns, addr) pairs among authoritative endpoints.
	if got.EndpointCount != 3 {
		t.Fatalf("EndpointCount: expected 3 authoritative pairs, got %d", got.EndpointCount)
	}
}

// TestDeriveRunDomainSummaryEndpointCountUsesPairs makes sure the count
// collapses by (nameserver, address), matching the cohort-level
// definition on /cohorts/{tag}. Counting distinct addresses alone would
// undercount when one IP is reached by two nameserver hostnames.
func TestDeriveRunDomainSummaryEndpointCountUsesPairs(t *testing.T) {
	input := RunInput{Run: serverpkg.Run{ID: "run-x", DomainID: 1}}
	endpoints := []extractedEndpoint{
		// Same address, two nameserver hostnames → two endpoints.
		{nameserver: "ns1.example", address: "192.0.2.1", family: "ipv4", role: "authoritative"},
		{nameserver: "ns2.example", address: "192.0.2.1", family: "ipv4", role: "authoritative"},
	}
	got := deriveRunDomainSummary(input, endpoints, nil)
	if got.NameserverCount != 2 {
		t.Fatalf("NameserverCount: expected 2, got %d", got.NameserverCount)
	}
	if got.EndpointCount != 2 {
		t.Fatalf("EndpointCount: expected 2 distinct (ns,addr) pairs, got %d", got.EndpointCount)
	}
}

func TestProjectorExtractNameserverEndpoints(t *testing.T) {
	input := RunInput{
		NameserverTimings: []serverpkg.NameserverTiming{
			{
				Nameserver: "NS1.Example.Test.",
				Address:    "192.0.2.10",
				AvgMS:      11,
				MinMS:      10,
				MaxMS:      13,
				Count:      3,
			},
		},
		Entries: []serverpkg.Entry{
			{
				Args: map[string]any{
					"servers": []any{
						map[string]any{"ns": "ns2.example.test.", "address": "2001:db8::20"},
					},
				},
			},
			{
				Args: map[string]any{
					"parent_servers": []any{
						map[string]any{"ns": "PNS.EXAMPLE.TEST.", "address": "198.51.100.53"},
					},
				},
			},
			{
				Args: map[string]any{
					"ns":      "ns3.example.test.",
					"address": "192.0.2.30",
				},
			},
		},
	}

	got := NewProjector(&fakeStore{}).extractNameserverEndpoints(input)
	if len(got) != 4 {
		t.Fatalf("expected 4 extracted endpoints, got %d", len(got))
	}

	byKey := map[string]extractedEndpoint{}
	for _, item := range got {
		byKey[item.nameserver+"|"+item.source] = item
	}

	if timing := byKey["ns1.example.test|timings"]; timing.queryCount != 3 || timing.avgMS != 11 || timing.family != "ipv4" || timing.role != "authoritative" {
		t.Fatalf("unexpected timing endpoint: %+v", timing)
	}
	// NameserverTimings is treated as the authoritative whitelist for the
	// run. ns2 (from a generic `servers` list) and ns3 (from a top-level
	// ns/address pair) are not in timings, so they are downgraded to the
	// "parent" role and filtered out of public views downstream.
	if server := byKey["ns2.example.test|servers"]; server.address != "2001:db8::20" || server.family != "ipv6" || server.role != "parent" {
		t.Fatalf("unexpected server endpoint: %+v", server)
	}
	if parent := byKey["pns.example.test|parent_servers"]; parent.role != "parent" || parent.family != "ipv4" {
		t.Fatalf("unexpected parent endpoint: %+v", parent)
	}
	if single := byKey["ns3.example.test|entry"]; single.address != "192.0.2.30" || single.role != "parent" {
		t.Fatalf("unexpected singular endpoint: %+v", single)
	}
}

// TestProjectorExtractNameserverEndpointsFallbackWithoutTimings verifies the
// legacy behavior: when NameserverTimings is empty, entry-derived endpoints
// still inherit the role tied to their source key. Without
// nameserver_timings we can't trust the generic `servers` list or a
// singleton {ns, address} entry as authoritative — zonemaster uses those
// same shapes for parent-side delegation traversal, and trusting them
// leaks root-server entries into TLD cohort analyses. Only the explicit
// child-side keys (child_servers / zone_servers / ns_set_servers) name
// the zone's own NSes; everything else defaults to parent.
func TestProjectorExtractNameserverEndpointsFallbackWithoutTimings(t *testing.T) {
	input := RunInput{
		Entries: []serverpkg.Entry{
			{
				Args: map[string]any{
					"servers": []any{
						map[string]any{"ns": "ns2.example.test.", "address": "2001:db8::20"},
					},
				},
			},
			{
				Args: map[string]any{
					"parent_servers": []any{
						map[string]any{"ns": "PNS.EXAMPLE.TEST.", "address": "198.51.100.53"},
					},
				},
			},
			{
				Args: map[string]any{
					"child_servers": []any{
						map[string]any{"ns": "ns4.example.test.", "address": "192.0.2.40"},
					},
				},
			},
			{
				Args: map[string]any{
					"ns":      "ns3.example.test.",
					"address": "192.0.2.30",
				},
			},
		},
	}

	got := NewProjector(&fakeStore{}).extractNameserverEndpoints(input)
	byKey := map[string]extractedEndpoint{}
	for _, item := range got {
		byKey[item.nameserver+"|"+item.source] = item
	}
	if server := byKey["ns2.example.test|servers"]; server.role != "parent" {
		t.Fatalf("expected servers-sourced endpoint to fall back to parent without timings: %+v", server)
	}
	if single := byKey["ns3.example.test|entry"]; single.role != "parent" {
		t.Fatalf("expected entry-sourced endpoint to fall back to parent without timings: %+v", single)
	}
	if parent := byKey["pns.example.test|parent_servers"]; parent.role != "parent" {
		t.Fatalf("expected parent_servers endpoint to stay parent without timings: %+v", parent)
	}
	if child := byKey["ns4.example.test|child_servers"]; child.role != "authoritative" {
		t.Fatalf("expected child_servers endpoint to stay authoritative without timings: %+v", child)
	}
}

// TestDelegationNSSetPrefersChild pins the extractor against the real
// Delegation01 tag shape the engine emits (`servers=[{ns:...}]`), both
// with the child-side and parent-side tags. Child-side wins when both
// are present; parent-side is the fallback.
func TestDelegationNSSetPrefersChild(t *testing.T) {
	entries := []serverpkg.Entry{
		{
			Module: "Delegation", Testcase: "Delegation01", Tag: "ENOUGH_NS_DEL",
			Args: map[string]any{
				"servers": []any{
					map[string]any{"ns": "parent1.example"},
					map[string]any{"ns": "PARENT2.EXAMPLE."},
				},
			},
		},
		{
			Module: "Delegation", Testcase: "Delegation01", Tag: "ENOUGH_NS_CHILD",
			Args: map[string]any{
				"servers": []any{
					map[string]any{"ns": "child1.example"},
					map[string]any{"ns": "child2.example."},
				},
			},
		},
	}
	got := delegationNSSet(entries)
	if len(got) != 2 {
		t.Fatalf("expected 2 child-side names, got %+v", got)
	}
	if _, ok := got["child1.example"]; !ok {
		t.Fatalf("expected child1.example in set, got %+v", got)
	}
	if _, ok := got["parent1.example"]; ok {
		t.Fatalf("child-side should take precedence, but parent1 leaked in: %+v", got)
	}
}

func TestDelegationNSSetFallsBackToParent(t *testing.T) {
	entries := []serverpkg.Entry{
		{
			Module: "Delegation", Testcase: "Delegation01", Tag: "NOT_ENOUGH_NS_DEL",
			Args: map[string]any{
				"servers": []any{map[string]any{"ns": "ns1.example"}},
			},
		},
	}
	got := delegationNSSet(entries)
	if len(got) != 1 {
		t.Fatalf("expected parent fallback with one name, got %+v", got)
	}
}

func TestDelegationNSSetEmptyWhenTagsMissing(t *testing.T) {
	entries := []serverpkg.Entry{
		{Module: "Basic", Testcase: "basic01", Tag: "B01_OK"},
	}
	if got := delegationNSSet(entries); len(got) != 0 {
		t.Fatalf("expected empty set for non-delegation entries, got %+v", got)
	}
}

// TestExtractNameserverEndpointsTimingsDrivesClassify covers the
// primary pipeline: the worker emits one NameserverTiming per delegated
// target (including unreachable / unresolved ones), and the projector
// uses that list as the authoritative NS set. Unreachable NSes (address
// present, no samples) are still authoritative even though generic
// `servers` entries would otherwise fall into the parent-side bucket.
func TestExtractNameserverEndpointsTimingsDrivesClassify(t *testing.T) {
	input := RunInput{
		NameserverTimings: []serverpkg.NameserverTiming{
			{Nameserver: "parau.oyster.net.ck", Address: "202.65.32.128", AvgMS: 10, Count: 3},
			// Unreachable — address present, zero samples, status from
			// the worker.
			{Nameserver: "circa.mcs.vuw.ac.nz", Address: "130.195.5.12"},
		},
		Entries: []serverpkg.Entry{
			// Both NSes' (ns, addr) pairs surface via a generic
			// "servers" key. Without the delegation-driven rule circa
			// would be parent.
			{
				Module: "Address", Testcase: "Address01", Tag: "A01_GLOBALLY_REACHABLE_ADDR",
				Args: map[string]any{
					"servers": []any{
						map[string]any{"ns": "circa.mcs.vuw.ac.nz", "address": "130.195.5.12"},
						map[string]any{"ns": "parau.oyster.net.ck", "address": "202.65.32.128"},
					},
				},
			},
		},
	}

	got := NewProjector(&fakeStore{}).extractNameserverEndpoints(input)
	byNS := map[string]extractedEndpoint{}
	for _, ep := range got {
		// Prefer non-timings rows so we see how the generic entry-side
		// pairs were classified.
		if ep.source != "timings" {
			byNS[ep.nameserver] = ep
		} else if _, seen := byNS[ep.nameserver]; !seen {
			byNS[ep.nameserver] = ep
		}
	}
	if byNS["circa.mcs.vuw.ac.nz"].role != "authoritative" {
		t.Fatalf("circa should be authoritative via timings name set, got %+v", byNS["circa.mcs.vuw.ac.nz"])
	}
	if byNS["parau.oyster.net.ck"].role != "authoritative" {
		t.Fatalf("parau should be authoritative, got %+v", byNS["parau.oyster.net.ck"])
	}
}

// TestExtractNameserverEndpointsSuppressesStaleGlueForUnresolvedNS
// pins the .ck "downstage" mismatch: Nameserver06 says the NS can't be
// resolved, but CN04 still lists a stale (ns, addr) pair for it.
// Trusting the CN04 address would make the analysis UI show a bogus IP
// with "No response" while the public UI (which uses live lookupNS)
// correctly says "Does not resolve". The projector trusts Nameserver06
// and drops the stale pair; the synthetic delegation-only endpoint
// represents the NS without an address.
func TestExtractNameserverEndpointsSuppressesStaleGlueForUnresolvedNS(t *testing.T) {
	input := RunInput{
		Entries: []serverpkg.Entry{
			{
				Module: "Delegation", Testcase: "Delegation01", Tag: "ENOUGH_NS_CHILD",
				Args: map[string]any{
					"servers": []any{
						map[string]any{"ns": "downstage.mcs.vuw.ac.nz"},
					},
				},
			},
			// Nameserver06 flags downstage as unresolvable.
			{
				Module: "Nameserver", Testcase: "Nameserver06", Tag: "CAN_NOT_BE_RESOLVED",
				Args: map[string]any{
					"servers": []any{
						map[string]any{"ns": "downstage.mcs.vuw.ac.nz"},
					},
				},
			},
			// CN04 still carries a stale-glue address for it.
			{
				Module: "Connectivity", Testcase: "Connectivity04", Tag: "CN04_IPV4_SAME_PREFIX",
				Args: map[string]any{
					"servers": []any{
						map[string]any{"ns": "downstage.mcs.vuw.ac.nz", "address": "130.195.6.10"},
					},
				},
			},
		},
	}
	got := NewProjector(&fakeStore{}).extractNameserverEndpoints(input)
	for _, ep := range got {
		if ep.nameserver == "downstage.mcs.vuw.ac.nz" && ep.address == "130.195.6.10" {
			t.Fatalf("stale-glue address leaked through to endpoint list: %+v", ep)
		}
	}
	// The synthetic delegation-only endpoint should still be present
	// so the NS itself shows up on the domain detail page.
	var synth *extractedEndpoint
	for i := range got {
		if got[i].nameserver == "downstage.mcs.vuw.ac.nz" && got[i].address == "" {
			synth = &got[i]
			break
		}
	}
	if synth == nil || synth.role != "authoritative" || synth.source != "delegation" {
		t.Fatalf("expected synthetic delegation-only endpoint for downstage, got %+v", got)
	}
}

// TestExtractNameserverEndpointsMergesDelegationTagsIntoTimings covers
// the legacy-data rollout: a run whose nameserver_timings_json predates
// the worker's per-target emission (so timings lists only the NSes the
// engine successfully probed). The Delegation01 tag parser supplements
// the timings-derived set so a pure cohort rebuild — no re-running of
// the DNS tests — still surfaces unreachable and unresolved NSes.
func TestExtractNameserverEndpointsMergesDelegationTagsIntoTimings(t *testing.T) {
	input := RunInput{
		NameserverTimings: []serverpkg.NameserverTiming{
			// Legacy timings: only the reachable NS made it in.
			{Nameserver: "parau.oyster.net.ck", Address: "202.65.32.128", AvgMS: 10, Count: 3},
		},
		Entries: []serverpkg.Entry{
			// Delegation entry lists all four, including the unreachable
			// circa and the unresolved downstage.
			{
				Module: "Delegation", Testcase: "Delegation01", Tag: "ENOUGH_NS_CHILD",
				Args: map[string]any{
					"servers": []any{
						map[string]any{"ns": "parau.oyster.net.ck"},
						map[string]any{"ns": "circa.mcs.vuw.ac.nz"},
						map[string]any{"ns": "downstage.mcs.vuw.ac.nz"},
					},
				},
			},
			// circa's (ns, addr) pair surfaces via generic "servers"
			// entries (resolved but no response, so it's not in timings).
			{
				Module: "Address", Testcase: "Address01", Tag: "A01_GLOBALLY_REACHABLE_ADDR",
				Args: map[string]any{
					"servers": []any{
						map[string]any{"ns": "circa.mcs.vuw.ac.nz", "address": "130.195.5.12"},
					},
				},
			},
		},
	}

	got := NewProjector(&fakeStore{}).extractNameserverEndpoints(input)
	byNS := map[string]extractedEndpoint{}
	for _, ep := range got {
		if ep.source != "timings" {
			byNS[ep.nameserver] = ep
		} else if _, seen := byNS[ep.nameserver]; !seen {
			byNS[ep.nameserver] = ep
		}
	}
	// circa gets promoted to authoritative via the merged delegation
	// set even though timings doesn't know about it.
	if byNS["circa.mcs.vuw.ac.nz"].role != "authoritative" {
		t.Fatalf("expected circa authoritative via merged delegation set, got %+v", byNS["circa.mcs.vuw.ac.nz"])
	}
	// downstage has no (ns, addr) pair anywhere, so it comes through
	// as a synthetic delegation-only endpoint.
	downstage := byNS["downstage.mcs.vuw.ac.nz"]
	if downstage.role != "authoritative" || downstage.source != "delegation" || downstage.address != "" {
		t.Fatalf("expected downstage synthetic authoritative endpoint, got %+v", downstage)
	}
}

// TestExtractNameserverEndpointsSyntheticFromTimingsUnresolvedRow pins
// the primary path: when the worker emits a timings row with empty
// address (status=unresolved), the projector must synthesize an
// authoritative, address-less endpoint so the NS still shows on the
// domain detail page.
func TestExtractNameserverEndpointsSyntheticFromTimingsUnresolvedRow(t *testing.T) {
	input := RunInput{
		NameserverTimings: []serverpkg.NameserverTiming{
			{Nameserver: "resolved.example", Address: "192.0.2.10", AvgMS: 10, Count: 1},
			// The worker's "unresolved" marker: name only, no address.
			{Nameserver: "ghost.example"},
		},
	}
	got := NewProjector(&fakeStore{}).extractNameserverEndpoints(input)
	var ghost *extractedEndpoint
	for i := range got {
		if got[i].nameserver == "ghost.example" {
			ghost = &got[i]
			break
		}
	}
	if ghost == nil {
		t.Fatalf("expected synthetic endpoint for ghost.example, got %+v", got)
	}
	if ghost.address != "" || ghost.role != "authoritative" || ghost.source != "delegation" {
		t.Fatalf("unexpected synthetic endpoint: %+v", ghost)
	}
}

// TestExtractNameserverEndpointsEmitsSyntheticForUnresolvedNS covers the
// .ck "downstage" case — NS name in delegation but no (ns, addr) pair
// anywhere in the run. A synthetic empty-address endpoint must be
// emitted so the NS still shows up on the domain detail page.
func TestExtractNameserverEndpointsEmitsSyntheticForUnresolvedNS(t *testing.T) {
	input := RunInput{
		Entries: []serverpkg.Entry{
			{
				Module: "Delegation", Testcase: "Delegation01", Tag: "ENOUGH_NS_CHILD",
				Args: map[string]any{
					"servers": []any{
						map[string]any{"ns": "resolved.example"},
						map[string]any{"ns": "ghost.example"},
					},
				},
			},
			{
				Module: "Address", Testcase: "Address01", Tag: "A01_GLOBALLY_REACHABLE_ADDR",
				Args: map[string]any{
					"servers": []any{
						map[string]any{"ns": "resolved.example", "address": "192.0.2.10"},
					},
				},
			},
			{
				Module: "Nameserver", Testcase: "Nameserver06", Tag: "CAN_NOT_BE_RESOLVED",
				Args: map[string]any{
					"servers": []any{
						map[string]any{"ns": "ghost.example"},
					},
				},
			},
		},
	}

	got := NewProjector(&fakeStore{}).extractNameserverEndpoints(input)
	var synthetic *extractedEndpoint
	for i := range got {
		if got[i].nameserver == "ghost.example" {
			synthetic = &got[i]
			break
		}
	}
	if synthetic == nil {
		t.Fatalf("expected a synthetic endpoint for ghost.example, got %+v", got)
	}
	if synthetic.address != "" {
		t.Fatalf("synthetic endpoint should carry no address, got %+v", synthetic)
	}
	if synthetic.role != "authoritative" || synthetic.source != "delegation" {
		t.Fatalf("synthetic endpoint should be authoritative+delegation, got %+v", synthetic)
	}
}

// TestExtractNameserverEndpointsLegacyFallbackStillWorks ensures runs
// without Delegation01 entries (old runs, non-DNSSEC tests) keep the
// legacy classification path so they don't silently reclassify every
// NS as parent and go dark.
func TestExtractNameserverEndpointsLegacyFallbackStillWorks(t *testing.T) {
	input := RunInput{
		NameserverTimings: []serverpkg.NameserverTiming{
			{Nameserver: "ns1.example", Address: "192.0.2.10", Count: 1},
		},
		Entries: []serverpkg.Entry{
			{
				Args: map[string]any{
					"servers": []any{
						map[string]any{"ns": "ns1.example", "address": "192.0.2.10"},
						map[string]any{"ns": "other.example", "address": "192.0.2.20"},
					},
				},
			},
		},
	}
	got := NewProjector(&fakeStore{}).extractNameserverEndpoints(input)
	byNS := map[string]string{}
	for _, ep := range got {
		if ep.source != "timings" {
			byNS[ep.nameserver] = ep.role
		}
	}
	if byNS["ns1.example"] != "authoritative" {
		t.Fatalf("timings-matched ns1 should be authoritative in fallback path, got %+v", byNS)
	}
	if byNS["other.example"] != "parent" {
		t.Fatalf("non-timings-matched other should be parent in fallback path, got %+v", byNS)
	}
}

func TestDeriveRunDomainSummarySkipsSyntheticEndpointsInCounts(t *testing.T) {
	input := RunInput{Run: serverpkg.Run{ID: "r", DomainID: 1}}
	endpoints := []extractedEndpoint{
		{nameserver: "ns1.example", address: "192.0.2.1", family: "ipv4", role: "authoritative"},
		// Synthetic delegation-only endpoint should bump NameserverCount but
		// NOT EndpointCount.
		{nameserver: "ghost.example", address: "", role: "authoritative", source: "delegation"},
	}
	got := deriveRunDomainSummary(input, endpoints, nil)
	if got.NameserverCount != 2 {
		t.Fatalf("NameserverCount: expected 2 (including synthetic), got %d", got.NameserverCount)
	}
	if got.EndpointCount != 1 {
		t.Fatalf("EndpointCount: synthetic should not count, got %d", got.EndpointCount)
	}
}

func TestExtractTagSummariesBucketsByTagAndTestcase(t *testing.T) {
	input := RunInput{
		Entries: []serverpkg.Entry{
			{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
			{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "WARNING"},
			{Module: "DNSSEC", Testcase: "dnssec01", Tag: "DS01_SIGNED", Level: "NOTICE"},
			// Same tag, different testcase: a separate bucket.
			{Module: "OTHER", Testcase: "other01", Tag: "DS01_SIGNED", Level: "INFO"},
			// Empty tag: skipped.
			{Module: "X", Testcase: "x", Tag: "  ", Level: "INFO"},
		},
	}

	got := extractTagSummaries(input)
	if len(got) != 3 {
		t.Fatalf("expected 3 buckets, got %d (%+v)", len(got), got)
	}
	byKey := map[string]tagSummary{}
	for _, ts := range got {
		byKey[ts.tag+"|"+ts.testcase] = ts
	}
	ds07 := byKey["DS07_NOT_SIGNED|dnssec07"]
	if ds07.occurrences != 2 || ds07.level != "ERROR" || ds07.module != "DNSSEC" {
		t.Fatalf("unexpected DS07 bucket: %+v", ds07)
	}
	ds01 := byKey["DS01_SIGNED|dnssec01"]
	if ds01.occurrences != 1 || ds01.level != "NOTICE" {
		t.Fatalf("unexpected DS01 bucket: %+v", ds01)
	}
	other := byKey["DS01_SIGNED|other01"]
	if other.occurrences != 1 || other.module != "OTHER" {
		t.Fatalf("unexpected DS01/other01 bucket: %+v", other)
	}
}

func TestProjectorExtractAddressFacts(t *testing.T) {
	input := RunInput{
		Entries: []serverpkg.Entry{
			{
				Args: map[string]any{
					"address":  "192.0.2.10",
					"prefixes": []any{"192.0.2.0/24"},
					"asns":     []any{float64(64510), float64(64500), float64(64510)},
				},
			},
			{
				Args: map[string]any{
					"address":  "2001:db8::10",
					"prefixes": []any{"2001:db8::/32"},
					"asn":      int64(64520),
				},
			},
			{
				Args: map[string]any{
					"address": "192.0.2.55",
				},
			},
		},
	}

	got := NewProjector(&fakeStore{}).extractAddressFacts(input)
	if len(got) != 3 {
		t.Fatalf("expected 3 address facts, got %d", len(got))
	}

	byAddress := map[string]extractedAddressFact{}
	for _, item := range got {
		byAddress[item.address] = item
	}

	first := byAddress["192.0.2.10"]
	if first.family != "ipv4" || first.prefix != "192.0.2.0/24" || first.prefixFamily != "ipv4" || first.status != "multiple_asns" || first.asn == nil || *first.asn != 64500 {
		t.Fatalf("unexpected ipv4 address fact: %+v", first)
	}

	second := byAddress["2001:db8::10"]
	if second.family != "ipv6" || second.prefix != "2001:db8::/32" || second.status != "ok" || second.asn == nil || *second.asn != 64520 {
		t.Fatalf("unexpected ipv6 address fact: %+v", second)
	}

	third := byAddress["192.0.2.55"]
	if third.status != "unknown" || third.asn != nil || third.prefix != "" {
		t.Fatalf("unexpected address-only fact: %+v", third)
	}
}

func TestProjectorProjectRunPersistsFactsIdempotently(t *testing.T) {
	score := 97
	grade := "A"
	finishedAt := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	run := serverpkg.Run{
		ID:         "run-3",
		DomainID:   303,
		Domain:     "example.test",
		Status:     serverpkg.JobSucceeded,
		EntryCount: 4,
		FinishedAt: finishedAt,
		Score:      &score,
		Grade:      &grade,
		WorstLevel: "ERROR",
		NameserverTimings: []serverpkg.NameserverTiming{
			{Nameserver: "ns1.example.test", Address: "192.0.2.10", AvgMS: 11, MinMS: 10, MaxMS: 12, Count: 3},
			{Nameserver: "ns2.example.test", Address: "2001:db8::20", AvgMS: 19, MinMS: 18, MaxMS: 22, Count: 4},
		},
	}
	store := &fakeStore{
		runs: map[string]serverpkg.Run{
			run.ID: run,
		},
		entries: map[string][]serverpkg.Entry{
			run.ID: {
				{
					RunID:    run.ID,
					DomainID: run.DomainID,
					Args: map[string]any{
						"servers": []any{
							map[string]any{"ns": "ns1.example.test.", "address": "192.0.2.10"},
							map[string]any{"ns": "ns2.example.test", "address": "2001:db8::20"},
						},
					},
				},
				{
					RunID:    run.ID,
					DomainID: run.DomainID,
					Args: map[string]any{
						"parent_servers": []any{
							map[string]any{"ns": "pns.example.test", "address": "198.51.100.53"},
						},
					},
				},
				{
					RunID:    run.ID,
					DomainID: run.DomainID,
					Args: map[string]any{
						"address":  "192.0.2.10",
						"prefixes": []any{"192.0.2.0/24"},
						"asns":     []any{float64(64510), float64(64500)},
					},
				},
				{
					RunID:    run.ID,
					DomainID: run.DomainID,
					Args: map[string]any{
						"address":  "2001:db8::20",
						"prefixes": []any{"2001:db8::/32"},
						"asn":      int64(64520),
					},
				},
			},
		},
		tags: map[int64][]string{
			run.DomainID: {"tld", "signed"},
		},
		cohorts: []serverpkg.AnalysisCohort{
			{ID: 10, SourceType: "tag", SourceTag: "tld", Label: "TLD", AnalysisEnabled: true, SortOrder: 10},
			{ID: 20, SourceType: "tag", SourceTag: "signed", Label: "Signed", AnalysisEnabled: true, SortOrder: 20},
			{ID: 30, SourceType: "tag", SourceTag: "hidden", Label: "Hidden", AnalysisEnabled: false, SortOrder: 30},
		},
	}

	projector := NewProjector(store)
	if err := projector.ProjectRun(run.ID); err != nil {
		t.Fatalf("ProjectRun first pass: %v", err)
	}

	firstNSEndpoints := append([]serverpkg.AnalysisRunNameserverEndpoint(nil), store.nsEndpoints[projectionKey(10, run.ID)]...)
	firstAddrFacts := append([]serverpkg.AnalysisRunAddressASN(nil), store.addrFacts[projectionKey(10, run.ID)]...)
	firstSummary := store.summaries[projectionKey(10, run.ID)]
	firstState := store.states[projectionKey(10, run.ID)]

	if len(store.nameserversByName) != 3 {
		t.Fatalf("expected 3 normalized nameservers, got %d", len(store.nameserversByName))
	}
	if len(store.addressesByValue) != 3 {
		t.Fatalf("expected 3 normalized addresses, got %d", len(store.addressesByValue))
	}
	if len(store.prefixesByValue) != 2 {
		t.Fatalf("expected 2 normalized prefixes, got %d", len(store.prefixesByValue))
	}
	// 2 ASNs surface from per-address args (64510 from entry 1's asns[0] and
	// 64520 from entry 2's asn). Entry 1's aggregate `asns` list also names
	// 64500, but restrictDomainASNsToAuthoritative drops aggregate ASNs
	// that have no address-level backing (e.g. parent-side registry ASNs
	// the engine traversed during delegation).
	if len(store.asnsByValue) != 2 {
		t.Fatalf("expected 2 normalized ASNs, got %d", len(store.asnsByValue))
	}

	for _, cohortID := range []int64{10, 20} {
		key := projectionKey(cohortID, run.ID)
		if len(store.nsEndpoints[key]) != 5 {
			t.Fatalf("cohort %d expected 5 endpoint rows, got %d", cohortID, len(store.nsEndpoints[key]))
		}
		if len(store.addrFacts[key]) != 2 {
			t.Fatalf("cohort %d expected 2 address fact rows, got %d", cohortID, len(store.addrFacts[key]))
		}

		summary := store.summaries[key]
		if summary.CohortID != cohortID || summary.RunID != run.ID || summary.DomainID != run.DomainID {
			t.Fatalf("cohort %d unexpected summary identity: %+v", cohortID, summary)
		}
		// Nameserver/endpoint counts exclude the parent-role entry
		// (pns.example.test), so 2 authoritative nameservers / 2
		// authoritative (ns, addr) pairs even though the fact table
		// retains 5 endpoint rows and 3 normalized nameservers.
		if summary.NameserverCount != 2 || summary.EndpointCount != 2 || summary.ASNCount != 2 || summary.PrefixCount != 2 {
			t.Fatalf("cohort %d unexpected summary counts: %+v", cohortID, summary)
		}
		if summary.Score == nil || *summary.Score != score || summary.Grade == nil || *summary.Grade != grade || summary.WorstLevel != "ERROR" {
			t.Fatalf("cohort %d unexpected summary scoring: %+v", cohortID, summary)
		}

		state := store.states[key]
		if state.CohortID != cohortID || state.RunID != run.ID || state.ProjectorVersion != projectorVersion || state.Status != serverpkg.AnalysisMaterializationReady || !state.ProjectedAt.Equal(finishedAt) {
			t.Fatalf("cohort %d unexpected projection state: %+v", cohortID, state)
		}
	}

	if err := projector.ProjectRun(run.ID); err != nil {
		t.Fatalf("ProjectRun second pass: %v", err)
	}

	if !reflect.DeepEqual(firstNSEndpoints, store.nsEndpoints[projectionKey(10, run.ID)]) {
		t.Fatalf("nameserver endpoints changed across idempotent reprojection:\nfirst=%+v\nsecond=%+v", firstNSEndpoints, store.nsEndpoints[projectionKey(10, run.ID)])
	}
	if !reflect.DeepEqual(firstAddrFacts, store.addrFacts[projectionKey(10, run.ID)]) {
		t.Fatalf("address facts changed across idempotent reprojection:\nfirst=%+v\nsecond=%+v", firstAddrFacts, store.addrFacts[projectionKey(10, run.ID)])
	}
	if !reflect.DeepEqual(firstSummary, store.summaries[projectionKey(10, run.ID)]) {
		t.Fatalf("summary changed across idempotent reprojection:\nfirst=%+v\nsecond=%+v", firstSummary, store.summaries[projectionKey(10, run.ID)])
	}
	if !reflect.DeepEqual(firstState, store.states[projectionKey(10, run.ID)]) {
		t.Fatalf("projection state changed across idempotent reprojection:\nfirst=%+v\nsecond=%+v", firstState, store.states[projectionKey(10, run.ID)])
	}
}
