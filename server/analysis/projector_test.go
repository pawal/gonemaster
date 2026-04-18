package analysis

import (
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

	nextCohortID     int64
	nextNameserverID int64
	nextAddressID    int64
	nextPrefixID     int64

	nameserversByName map[string]serverpkg.AnalysisNameserver
	addressesByValue  map[string]serverpkg.AnalysisAddress
	prefixesByValue   map[string]serverpkg.AnalysisPrefix
	asnsByValue       map[int64]serverpkg.AnalysisASN

	nsEndpoints map[string][]serverpkg.AnalysisRunNameserverEndpoint
	addrFacts   map[string][]serverpkg.AnalysisRunAddressASN
	domainASNs  map[string][]serverpkg.AnalysisRunDomainASN
	summaries   map[string]serverpkg.AnalysisRunDomainSummary
	states      map[string]serverpkg.AnalysisProjectionState
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
	if server := byKey["ns2.example.test|servers"]; server.address != "2001:db8::20" || server.family != "ipv6" || server.role != "authoritative" {
		t.Fatalf("unexpected server endpoint: %+v", server)
	}
	if parent := byKey["pns.example.test|parent_servers"]; parent.role != "parent" || parent.family != "ipv4" {
		t.Fatalf("unexpected parent endpoint: %+v", parent)
	}
	if single := byKey["ns3.example.test|entry"]; single.address != "192.0.2.30" || single.role != "authoritative" {
		t.Fatalf("unexpected singular endpoint: %+v", single)
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
	// 2 ASNs surface from per-address args (64510 from entry 1's asns[0], 64520
	// from entry 2's asn). The aggregate-ASN projection adds 64500 from
	// entry 1's asns list — we now upsert every ASN the engine emitted,
	// whether or not it pairs with a specific address.
	if len(store.asnsByValue) != 3 {
		t.Fatalf("expected 3 normalized ASNs, got %d", len(store.asnsByValue))
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
		if summary.NameserverCount != 3 || summary.EndpointCount != 3 || summary.ASNCount != 2 || summary.PrefixCount != 2 {
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
