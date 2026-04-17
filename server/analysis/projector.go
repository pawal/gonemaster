package analysis

import (
	"errors"
	"fmt"
	"sort"

	serverpkg "codeberg.org/pawal/gonemaster/server"
)

var (
	ErrRunNotFound = errors.New("analysis projector run not found")
)

// Store is the minimal backing store surface needed by the Phase 2 projector
// loading and cohort-resolution logic.
type Store interface {
	GetRun(id string) (serverpkg.Run, bool)
	QueryEntries(filter serverpkg.EntryFilter) serverpkg.EntryList
	GetDomainTags(domainID int64) []string
	ListAnalysisCohorts() []serverpkg.AnalysisCohort
}

// Projector loads completed runs and resolves which analysis-enabled cohorts
// should receive projected facts for those runs.
type Projector struct {
	store Store
}

// RunInput is the fully loaded source material for projecting one completed run.
type RunInput struct {
	Run               serverpkg.Run
	Entries           []serverpkg.Entry
	DomainTags        []string
	MatchingCohorts   []serverpkg.AnalysisCohort
	NameserverTimings []serverpkg.NameserverTiming
}

// NewProjector creates a projector backed by the given store.
func NewProjector(store Store) *Projector {
	return &Projector{store: store}
}

// LoadCompletedRun loads the completed run, all of its entries, its domain tags,
// and the analysis-enabled cohorts that match those tags.
func (p *Projector) LoadCompletedRun(runID string) (RunInput, error) {
	run, ok := p.store.GetRun(runID)
	if !ok {
		return RunInput{}, fmt.Errorf("%w: %s", ErrRunNotFound, runID)
	}

	limit := run.EntryCount
	if limit <= 0 {
		limit = 1
	}
	entryList := p.store.QueryEntries(serverpkg.EntryFilter{
		RunID:  run.ID,
		Limit:  limit,
		Offset: 0,
	})
	tags := p.store.GetDomainTags(run.DomainID)
	cohorts := MatchAnalysisEnabledCohorts(tags, p.store.ListAnalysisCohorts())

	return RunInput{
		Run:               run,
		Entries:           entryList.Items,
		DomainTags:        tags,
		MatchingCohorts:   cohorts,
		NameserverTimings: run.NameserverTimings,
	}, nil
}

// MatchAnalysisEnabledCohorts returns the tag-backed cohorts that should be
// populated for a run with the given domain tags.
func MatchAnalysisEnabledCohorts(domainTags []string, cohorts []serverpkg.AnalysisCohort) []serverpkg.AnalysisCohort {
	if len(domainTags) == 0 || len(cohorts) == 0 {
		return nil
	}

	tagSet := make(map[string]struct{}, len(domainTags))
	for _, tag := range domainTags {
		tagSet[tag] = struct{}{}
	}

	var matched []serverpkg.AnalysisCohort
	for _, cohort := range cohorts {
		if !cohort.AnalysisEnabled {
			continue
		}
		if cohort.SourceType != "tag" {
			continue
		}
		if _, ok := tagSet[cohort.SourceTag]; !ok {
			continue
		}
		matched = append(matched, cohort)
	}

	sort.Slice(matched, func(i, j int) bool {
		if matched[i].SortOrder != matched[j].SortOrder {
			return matched[i].SortOrder < matched[j].SortOrder
		}
		if matched[i].Label != matched[j].Label {
			return matched[i].Label < matched[j].Label
		}
		return matched[i].SourceTag < matched[j].SourceTag
	})

	return matched
}
