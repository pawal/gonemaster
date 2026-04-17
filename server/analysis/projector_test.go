package analysis

import (
	"errors"
	"testing"
	"time"

	serverpkg "codeberg.org/pawal/gonemaster/server"
)

type fakeStore struct {
	runs    map[string]serverpkg.Run
	entries map[string][]serverpkg.Entry
	tags    map[int64][]string
	cohorts []serverpkg.AnalysisCohort
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
