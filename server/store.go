package server

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/normalization"
	"codeberg.org/pawal/gonemaster/scoring"
)

// severityRank returns a numeric rank for level comparisons (higher = worse).
func severityRank(level string) int {
	switch strings.ToUpper(level) {
	case "CRITICAL":
		return 4
	case "ERROR":
		return 3
	case "WARNING":
		return 2
	case "NOTICE":
		return 1
	default:
		return 0
	}
}

// sortDomainSlice sorts a domain slice in-place according to the given sort order.
func sortDomainSlice(items []Domain, ds DomainSort) {
	switch ds {
	case DomainSortNameDesc:
		sort.Slice(items, func(i, j int) bool { return items[i].Name > items[j].Name })
	case DomainSortLevelDesc:
		sort.Slice(items, func(i, j int) bool {
			ri, rj := severityRank(items[i].LatestLevel), severityRank(items[j].LatestLevel)
			if ri != rj {
				return ri > rj
			}
			return items[i].Name < items[j].Name
		})
	case DomainSortLevelAsc:
		sort.Slice(items, func(i, j int) bool {
			ri, rj := severityRank(items[i].LatestLevel), severityRank(items[j].LatestLevel)
			if ri != rj {
				return ri < rj
			}
			return items[i].Name < items[j].Name
		})
	case DomainSortScoreDesc:
		sort.Slice(items, func(i, j int) bool {
			si := derefIntOr(items[i].LatestScore, -1)
			sj := derefIntOr(items[j].LatestScore, -1)
			if si != sj {
				return si > sj
			}
			return items[i].Name < items[j].Name
		})
	case DomainSortScoreAsc:
		sort.Slice(items, func(i, j int) bool {
			si := derefIntOr(items[i].LatestScore, 9999)
			sj := derefIntOr(items[j].LatestScore, 9999)
			if si != sj {
				return si < sj
			}
			return items[i].Name < items[j].Name
		})
	case DomainSortLastRunDesc:
		sort.Slice(items, func(i, j int) bool {
			if !items[i].LatestRunAt.Equal(items[j].LatestRunAt) {
				return items[i].LatestRunAt.After(items[j].LatestRunAt)
			}
			return items[i].Name < items[j].Name
		})
	case DomainSortLastRunAsc:
		sort.Slice(items, func(i, j int) bool {
			if !items[i].LatestRunAt.Equal(items[j].LatestRunAt) {
				return items[i].LatestRunAt.Before(items[j].LatestRunAt)
			}
			return items[i].Name < items[j].Name
		})
	case DomainSortRunCountDesc:
		sort.Slice(items, func(i, j int) bool {
			if items[i].RunCount != items[j].RunCount {
				return items[i].RunCount > items[j].RunCount
			}
			return items[i].Name < items[j].Name
		})
	case DomainSortRunCountAsc:
		sort.Slice(items, func(i, j int) bool {
			if items[i].RunCount != items[j].RunCount {
				return items[i].RunCount < items[j].RunCount
			}
			return items[i].Name < items[j].Name
		})
	default:
		sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	}
}

func derefIntOr(p *int, fallback int) int {
	if p != nil {
		return *p
	}
	return fallback
}

// JobStore persists job metadata and results.
type JobStore interface {
	// Job queue management (in-flight jobs only).
	Create(job Job) (Job, error)
	Get(id string) (Job, bool)                 // checks jobs first, then reconstructs from runs
	GetByPublicID(publicID string) (Job, bool) // checks jobs then runs
	Update(job Job) error                      // only for in-flight jobs
	List(filter JobFilter) JobList             // only in-flight jobs

	// GraduateJob atomically creates a run + entries, updates domain latest_*,
	// and deletes the job from the queue. entries may be nil for canceled jobs.
	GraduateJob(job Job, entries []engine.LogEntry) error

	// GetResult reconstructs a JobResult from the runs+entries tables.
	GetResult(jobID string) (JobResult, bool)

	// Domain management.
	GetOrCreateDomain(name string) (Domain, error)
	GetDomain(id int64) (Domain, bool)
	GetDomainByName(name string) (Domain, bool)
	GetDomainNamesByIDs(ids []int64) map[int64]string
	ListDomains(filter DomainFilter) DomainList
	UpdateDomainLatest(domainID int64, runID string, finishedAt time.Time, status, level string) error

	// Tag management.
	CreateTag(name, description string) error
	GetTag(name string) (Tag, bool)
	UpdateTag(name, description string) error
	SetTagDefaultProfile(name string, profileID *int64) error
	DeleteTag(name string) error
	ListTags(limit, offset int) []Tag
	TagDomains(tag string, domainIDs []int64) error
	UntagDomains(tag string, domainIDs []int64) error
	GetDomainTags(domainID int64) []string
	ListDomainsByTag(tag string, filter DomainFilter) DomainList
	GetTagSummary(tag string) (TagSummary, bool)

	// Run management (completed/graduated jobs).
	GetRun(id string) (Run, bool)
	GetRunByPublicID(publicID string) (Run, bool)
	ListRuns(filter RunFilter) RunList
	ListRunsByDomain(domainID int64, limit, offset int) RunList

	// Entry queries (cross-run analysis).
	QueryEntries(filter EntryFilter) EntryList

	// Batch management.
	CreateBatch(batch Batch) error
	GetBatch(id string) (Batch, bool)
	SetBatchSnapshotIntent(batchID string, intent bool) error
	ListBatchesByTag(tag string, limit, offset int) BatchList
	BatchDeletePreviewStats(batchID string) (BatchDeletePreview, error)
	DeleteBatch(batchID string) ([]int64, error)
	BatchHasRuns(batchID string) bool

	// Analysis cohort catalog.
	ListAnalysisCohorts() []AnalysisCohort
	GetAnalysisCohort(id int64) (AnalysisCohort, bool)
	GetAnalysisCohortBySource(sourceType, sourceTag string) (AnalysisCohort, bool)
	UpsertAnalysisCohort(cohort AnalysisCohort) (AnalysisCohort, error)
	DeleteAnalysisCohort(id int64) error

	// Profile management.
	CreateProfile(p StoredProfile) (StoredProfile, error)
	GetProfile(id int64) (StoredProfile, bool)
	GetProfileByName(name string) (StoredProfile, bool)
	UpdateProfile(p StoredProfile) error
	DeleteProfile(id int64) error
	ListProfiles() []StoredProfile

	// Settings management.
	GetSetting(key string) (string, bool)
	SetSetting(key, value string) error
	DeleteSetting(key string) error
	ListSettings() map[string]string

	// PurgeOlderThan deletes terminal runs whose finished_at is before cutoff,
	// along with their entries. Returns the number of runs deleted.
	PurgeOlderThan(cutoff time.Time) (int64, error)
}

// InMemoryJobStore stores all data in memory.
type InMemoryJobStore struct {
	mu sync.RWMutex

	scoringCfg scoring.Config

	// In-flight jobs.
	jobs      map[string]Job    // jobID → Job
	publicIDs map[string]string // publicID → jobID

	// Graduated runs.
	runs         map[string]Run     // runID → Run
	runPublicIDs map[string]string  // publicID → runID
	entries      map[string][]Entry // runID → []Entry
	entryCounter int64

	// Domain registry.
	domains       map[string]*Domain // name → Domain
	domainsByID   map[int64]*Domain  // id → Domain
	domainCounter int64

	// Tags.
	tags       map[string]Tag     // name → Tag
	domainTags map[int64][]string // domainID → []tagName
	tagDomains map[string][]int64 // tagName → []domainID

	// Batches.
	batches map[string]Batch // batchID → Batch

	// Analysis cohort catalog.
	analysisCohorts       map[int64]AnalysisCohort
	analysisCohortCounter int64

	// Profiles.
	profiles       map[int64]StoredProfile // id → StoredProfile
	profileCounter int64

	// Settings.
	settings map[string]string // key → value
}

// SetScoringConfig sets the scoring configuration used when graduating jobs.
func (s *InMemoryJobStore) SetScoringConfig(cfg scoring.Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scoringCfg = cfg
}

// NewInMemoryJobStore creates an empty in-memory job store.
func NewInMemoryJobStore() *InMemoryJobStore {
	return &InMemoryJobStore{
		scoringCfg:      scoring.DefaultConfig(),
		jobs:            map[string]Job{},
		publicIDs:       map[string]string{},
		runs:            map[string]Run{},
		runPublicIDs:    map[string]string{},
		entries:         map[string][]Entry{},
		domains:         map[string]*Domain{},
		domainsByID:     map[int64]*Domain{},
		tags:            map[string]Tag{},
		domainTags:      map[int64][]string{},
		tagDomains:      map[string][]int64{},
		batches:         map[string]Batch{},
		analysisCohorts: map[int64]AnalysisCohort{},
		profiles:        map[int64]StoredProfile{},
		settings:        map[string]string{},
	}
}

// ListAnalysisCohorts returns all analysis cohort catalog entries ordered by
// sort_order, label, source_tag.
func (s *InMemoryJobStore) ListAnalysisCohorts() []AnalysisCohort {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]AnalysisCohort, 0, len(s.analysisCohorts))
	for _, c := range s.analysisCohorts {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		if out[i].Label != out[j].Label {
			return out[i].Label < out[j].Label
		}
		return out[i].SourceTag < out[j].SourceTag
	})
	return out
}

// GetAnalysisCohort returns one analysis cohort catalog entry by numeric id.
func (s *InMemoryJobStore) GetAnalysisCohort(id int64) (AnalysisCohort, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.analysisCohorts[id]
	return c, ok
}

// GetAnalysisCohortBySource returns one analysis cohort catalog entry by source key.
func (s *InMemoryJobStore) GetAnalysisCohortBySource(sourceType, sourceTag string) (AnalysisCohort, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, c := range s.analysisCohorts {
		if c.SourceType == sourceType && c.SourceTag == sourceTag {
			return c, true
		}
	}
	return AnalysisCohort{}, false
}

// UpsertAnalysisCohort inserts a new cohort or updates the existing row for
// the same source_type/source_tag pair.
func (s *InMemoryJobStore) UpsertAnalysisCohort(cohort AnalysisCohort) (AnalysisCohort, error) {
	if cohort.SourceType == "" {
		return AnalysisCohort{}, errors.New("source_type is required")
	}
	if cohort.SourceTag == "" {
		return AnalysisCohort{}, errors.New("source_tag is required")
	}
	if cohort.Label == "" {
		cohort.Label = cohort.SourceTag
	}
	if cohort.MaterializationStatus == "" {
		cohort.MaterializationStatus = AnalysisMaterializationPending
	}
	if cohort.DefaultSnapshotPolicy == "" {
		cohort.DefaultSnapshotPolicy = DefaultSnapshotPolicyAutoLatest
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	for id, existing := range s.analysisCohorts {
		if existing.SourceType == cohort.SourceType && existing.SourceTag == cohort.SourceTag {
			cohort.ID = id
			if cohort.CreatedAt.IsZero() {
				cohort.CreatedAt = existing.CreatedAt
			}
			if cohort.UpdatedAt.IsZero() {
				cohort.UpdatedAt = now
			}
			s.analysisCohorts[id] = cohort
			return cohort, nil
		}
	}
	if cohort.ID == 0 {
		s.analysisCohortCounter++
		cohort.ID = s.analysisCohortCounter
	}
	if cohort.CreatedAt.IsZero() {
		cohort.CreatedAt = now
	}
	if cohort.UpdatedAt.IsZero() {
		cohort.UpdatedAt = cohort.CreatedAt
	}
	s.analysisCohorts[cohort.ID] = cohort
	return cohort, nil
}

// DeleteAnalysisCohort removes one cohort catalog row.
func (s *InMemoryJobStore) DeleteAnalysisCohort(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.analysisCohorts, id)
	return nil
}

// Create inserts a new in-flight job. A PublicID is generated if not set.
func (s *InMemoryJobStore) Create(job Job) (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[job.ID]; exists {
		return Job{}, errors.New("job already exists")
	}
	if job.PublicID == "" {
		job.PublicID = GeneratePublicID()
	}
	s.jobs[job.ID] = job
	s.publicIDs[job.PublicID] = job.ID
	return job, nil
}

// Get returns a job by ID. If the job has graduated, it is reconstructed from
// the corresponding run so callers always get a result for known job IDs.
func (s *InMemoryJobStore) Get(id string) (Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if job, ok := s.jobs[id]; ok {
		return job, true
	}
	if run, ok := s.runs[id]; ok {
		return jobFromRun(run), true
	}
	return Job{}, false
}

// GetByPublicID returns a job by public_id, checking both in-flight jobs and
// graduated runs.
func (s *InMemoryJobStore) GetByPublicID(publicID string) (Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if id, ok := s.publicIDs[publicID]; ok {
		if job, ok := s.jobs[id]; ok {
			return job, true
		}
	}
	if runID, ok := s.runPublicIDs[publicID]; ok {
		if run, ok := s.runs[runID]; ok {
			return jobFromRun(run), true
		}
	}
	return Job{}, false
}

// Update replaces an in-flight job's mutable fields. Returns an error if the
// job does not exist in the queue (graduated jobs are immutable).
func (s *InMemoryJobStore) Update(job Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[job.ID]; !exists {
		return errors.New("job not found")
	}
	s.jobs[job.ID] = job
	return nil
}

// List returns in-flight jobs matching filter with sorting and pagination.
// Severity filtering is not applicable to in-flight jobs (no results yet)
// and is silently ignored.
func (s *InMemoryJobStore) List(filter JobFilter) JobList {
	s.mu.RLock()
	jobsSnapshot := make([]Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobsSnapshot = append(jobsSnapshot, job)
	}
	s.mu.RUnlock()

	items := make([]Job, 0, len(jobsSnapshot))
	for _, job := range jobsSnapshot {
		if filter.Status != "" && job.Status != filter.Status {
			continue
		}
		if filter.BatchID != "" && job.BatchID != filter.BatchID {
			continue
		}
		if filter.Domain != "" && !strings.Contains(strings.ToLower(job.Domain), strings.ToLower(filter.Domain)) {
			continue
		}
		if !filter.CreatedAfter.IsZero() && job.CreatedAt.Before(filter.CreatedAfter) {
			continue
		}
		if !filter.CreatedBefore.IsZero() && job.CreatedAt.After(filter.CreatedBefore) {
			continue
		}
		items = append(items, job)
	}

	normalizedSort := normalizeJobSort(filter.Sort)
	sortJobSlice(items, filter.Sort)

	total := len(items)
	offset := max(filter.Offset, 0)
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	start := min(offset, len(items))
	end := start + limit
	end = min(end, len(items))

	pageItems := make([]Job, end-start)
	copy(pageItems, items[start:end])

	list := JobList{
		Items:  pageItems,
		Total:  total,
		Limit:  limit,
		Offset: offset,
		Sort:   string(normalizedSort),
	}
	if start > 0 {
		prevOffset := max(start-limit, 0)
		list.PrevCursor = strconv.Itoa(prevOffset)
	}
	if end < total {
		list.NextCursor = strconv.Itoa(end)
	}
	return list
}

// GraduateJob atomically creates a run from the completed job, stores entries,
// updates the domain's latest_* fields, and removes the job from the queue.
// entries may be nil for canceled/failed jobs with no engine output.
func (s *InMemoryJobStore) GraduateJob(job Job, engineEntries []engine.LogEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.jobs[job.ID]; !exists {
		return errors.New("job not found")
	}

	// Get or create the domain.
	domain := s.getOrCreateDomainLocked(job.Domain)

	// Compute severity totals and worst level.
	sevNotice, sevWarning, sevError, sevCritical := 0, 0, 0, 0
	for _, e := range engineEntries {
		switch strings.ToUpper(strings.TrimSpace(e.Level)) {
		case "NOTICE":
			sevNotice++
		case "WARNING":
			sevWarning++
		case "ERROR":
			sevError++
		case "CRITICAL":
			sevCritical++
		}
	}
	worstLevel := computeWorstLevel(sevNotice, sevWarning, sevError, sevCritical)

	var durationMs int64
	if !job.StartedAt.IsZero() && !job.FinishedAt.IsZero() {
		durationMs = job.FinishedAt.Sub(job.StartedAt).Milliseconds()
	}

	// Compute score eagerly at graduation time.
	scoringEntries := make([]scoring.Entry, len(engineEntries))
	for i, e := range engineEntries {
		scoringEntries[i] = scoring.Entry{Module: e.Module, Tag: e.Tag, Level: e.Level}
	}
	scoreResult := scoring.Compute(job.Domain, scoringEntries, s.scoringCfg)
	scoreVal := scoreResult.Score
	gradeVal := scoreResult.Grade

	run := Run{
		ID:                job.ID,
		DomainID:          domain.ID,
		Domain:            job.Domain,
		BatchID:           job.BatchID,
		Status:            job.Status,
		CreatedAt:         job.CreatedAt,
		StartedAt:         job.StartedAt,
		FinishedAt:        job.FinishedAt,
		DurationMs:        durationMs,
		SevNotice:         sevNotice,
		SevWarning:        sevWarning,
		SevError:          sevError,
		SevCritical:       sevCritical,
		WorstLevel:        worstLevel,
		EntryCount:        len(engineEntries),
		Priority:          job.Priority,
		Profile:           job.Profile,
		ProfileID:         cloneInt64Ptr(job.ProfileID),
		ProfileName:       job.ProfileName,
		EffectiveProfile:  job.EffectiveProfile,
		PublicID:          job.PublicID,
		Score:             &scoreVal,
		Grade:             &gradeVal,
		NameserverTimings: cloneNameserverTimings(job.NameserverTimings),
	}
	run.SeverityTotals = map[string]int{
		"NOTICE":   sevNotice,
		"WARNING":  sevWarning,
		"ERROR":    sevError,
		"CRITICAL": sevCritical,
	}

	// Convert engine entries to stored Entry rows.
	stored := make([]Entry, len(engineEntries))
	for i, e := range engineEntries {
		s.entryCounter++
		stored[i] = Entry{
			ID:        s.entryCounter,
			RunID:     run.ID,
			DomainID:  domain.ID,
			Timestamp: e.Timestamp,
			Module:    e.Module,
			Testcase:  e.Testcase,
			Tag:       e.Tag,
			Level:     e.Level,
			Args:      e.Args,
		}
	}

	// Update domain latest_*.
	domain.LatestRunID = run.ID
	domain.LatestRunAt = run.FinishedAt
	domain.LatestStatus = string(run.Status)
	domain.LatestLevel = run.WorstLevel
	domain.LatestScore = run.Score
	domain.LatestGrade = run.Grade
	domain.RunCount++

	// Store run and entries.
	s.runs[run.ID] = run
	if run.PublicID != "" {
		s.runPublicIDs[run.PublicID] = run.ID
	}
	s.entries[run.ID] = stored

	// Remove job from queue.
	delete(s.publicIDs, job.PublicID)
	delete(s.jobs, job.ID)

	return nil
}

// GetResult reconstructs a JobResult from the run and its entries.
func (s *InMemoryJobStore) GetResult(jobID string) (JobResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	run, ok := s.runs[jobID]
	if !ok {
		return JobResult{}, false
	}
	entries := s.entries[jobID]
	scoringEntries := make([]scoring.Entry, len(entries))
	for i, e := range entries {
		scoringEntries[i] = scoring.Entry{Module: e.Module, Tag: e.Tag, Level: e.Level}
	}
	sr := scoring.Compute(run.Domain, scoringEntries, s.scoringCfg)
	return buildJobResult(run, entries, &sr), true
}

// GetOrCreateDomain returns the domain for name, creating it if necessary.
// name is normalized (lowercased, Unicode labels converted to ACE) before storage.
func (s *InMemoryJobStore) GetOrCreateDomain(name string) (Domain, error) {
	errs, normalized := normalization.NormalizeName(strings.TrimSpace(name))
	if len(errs) > 0 {
		return Domain{}, fmt.Errorf("invalid domain name %q: %s", name, errs[0].Message())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.getOrCreateDomainLocked(normalized)
	return *d, nil
}

// GetDomain returns a domain by its numeric ID.
func (s *InMemoryJobStore) GetDomain(id int64) (Domain, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.domainsByID[id]
	if !ok {
		return Domain{}, false
	}
	dc := *d
	dc.Tags = append([]string(nil), s.domainTags[d.ID]...)
	return dc, true
}

// GetDomainByName returns a domain by its name.
func (s *InMemoryJobStore) GetDomainByName(name string) (Domain, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.domains[name]
	if !ok {
		return Domain{}, false
	}
	dc := *d
	dc.Tags = append([]string(nil), s.domainTags[d.ID]...)
	return dc, true
}

// GetDomainNamesByIDs returns a map of id to domain name for the given
// ids. Mirrors SQLJobStore.GetDomainNamesByIDs so callers can bulk-load
// domain names off the list path.
func (s *InMemoryJobStore) GetDomainNamesByIDs(ids []int64) map[int64]string {
	out := make(map[int64]string, len(ids))
	if len(ids) == 0 {
		return out
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, id := range ids {
		if d, ok := s.domainsByID[id]; ok {
			out[id] = d.Name
		}
	}
	return out
}

// UpdateDomainLatest updates the latest_* denormalized fields on a domain.
func (s *InMemoryJobStore) UpdateDomainLatest(domainID int64, runID string, finishedAt time.Time, status, level string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.domainsByID[domainID]
	if !ok {
		return errors.New("domain not found")
	}
	d.LatestRunID = runID
	d.LatestRunAt = finishedAt
	d.LatestStatus = status
	d.LatestLevel = level
	d.RunCount++
	return nil
}

func (s *InMemoryJobStore) getOrCreateDomainLocked(name string) *Domain {
	if d, ok := s.domains[name]; ok {
		return d
	}
	s.domainCounter++
	d := &Domain{
		ID:        s.domainCounter,
		Name:      name,
		CreatedAt: time.Now().UTC(),
	}
	s.domains[name] = d
	s.domainsByID[d.ID] = d
	return d
}

// ListDomains returns paginated domains matching filter.
func (s *InMemoryJobStore) ListDomains(filter DomainFilter) DomainList {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]Domain, 0, len(s.domains))
	for _, d := range s.domains {
		if filter.Tag == "__none__" {
			if len(s.domainTags[d.ID]) > 0 {
				continue
			}
		} else if filter.Tag != "" {
			if !slices.Contains(s.domainTags[d.ID], filter.Tag) {
				continue
			}
		}
		if filter.Name != "" && !strings.Contains(strings.ToLower(d.Name), strings.ToLower(filter.Name)) {
			continue
		}
		if filter.LatestLevel != "" && d.LatestLevel != filter.LatestLevel {
			continue
		}
		if filter.MinLevel != "" && severityRank(d.LatestLevel) < severityRank(filter.MinLevel) {
			continue
		}
		dc := *d
		dc.Tags = append([]string(nil), s.domainTags[d.ID]...)
		items = append(items, dc)
	}

	sortDomainSlice(items, filter.Sort)

	total := len(items)
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	offset := max(filter.Offset, 0)
	start := min(offset, total)
	end := start + limit
	end = min(end, total)
	return DomainList{
		Items:  items[start:end],
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}
}

// CreateTag creates a new tag. Returns an error if the tag already exists.
func (s *InMemoryJobStore) CreateTag(name, description string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.tags[name]; exists {
		return errors.New("tag already exists")
	}
	s.tags[name] = Tag{
		Name:        name,
		Description: description,
		CreatedAt:   time.Now().UTC(),
	}
	return nil
}

// ListTags returns all tags sorted by name.
func (s *InMemoryJobStore) ListTags(limit, offset int) []Tag {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tags := make([]Tag, 0, len(s.tags))
	for _, t := range s.tags {
		t.DomainCount = len(s.tagDomains[t.Name])
		t.DefaultProfileID = cloneInt64Ptr(t.DefaultProfileID)
		tags = append(tags, t)
	}
	sort.Slice(tags, func(i, j int) bool {
		return tags[i].Name < tags[j].Name
	})
	if limit <= 0 {
		limit = 100
	}
	offset = max(offset, 0)
	if offset > len(tags) {
		return []Tag{}
	}
	end := min(offset+limit, len(tags))
	return tags[offset:end]
}

// TagDomains associates the given domain IDs with tag. The tag is
// auto-created if it does not exist.
func (s *InMemoryJobStore) TagDomains(tag string, domainIDs []int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.tags[tag]; !exists {
		s.tags[tag] = Tag{Name: tag, CreatedAt: time.Now().UTC()}
	}
	for _, id := range domainIDs {
		// Avoid duplicates.
		alreadyTagged := slices.Contains(s.domainTags[id], tag)
		if !alreadyTagged {
			s.domainTags[id] = append(s.domainTags[id], tag)
			s.tagDomains[tag] = append(s.tagDomains[tag], id)
		}
	}
	return nil
}

// GetTag returns a tag by name with its current domain count.
func (s *InMemoryJobStore) GetTag(name string) (Tag, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tags[name]
	if !ok {
		return Tag{}, false
	}
	t.DomainCount = len(s.tagDomains[name])
	t.DefaultProfileID = cloneInt64Ptr(t.DefaultProfileID)
	return t, true
}

// UpdateTag updates the description of an existing tag.
func (s *InMemoryJobStore) UpdateTag(name, description string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tags[name]
	if !ok {
		return errors.New("tag not found")
	}
	t.Description = description
	s.tags[name] = t
	return nil
}

// SetTagDefaultProfile updates a tag's default stored profile reference.
func (s *InMemoryJobStore) SetTagDefaultProfile(name string, profileID *int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.tags[name]
	if !ok {
		return errors.New("tag not found")
	}
	if profileID != nil {
		if _, ok := s.profiles[*profileID]; !ok {
			return fmt.Errorf("profile %d not found", *profileID)
		}
	}
	t.DefaultProfileID = cloneInt64Ptr(profileID)
	s.tags[name] = t
	return nil
}

// DeleteTag removes a tag and all its domain associations.
func (s *InMemoryJobStore) DeleteTag(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tags[name]; !ok {
		return errors.New("tag not found")
	}
	for _, domainID := range s.tagDomains[name] {
		updated := s.domainTags[domainID][:0]
		for _, t := range s.domainTags[domainID] {
			if t != name {
				updated = append(updated, t)
			}
		}
		s.domainTags[domainID] = updated
	}
	delete(s.tagDomains, name)
	delete(s.tags, name)
	return nil
}

// UntagDomains removes the given domain IDs from a tag.
func (s *InMemoryJobStore) UntagDomains(tag string, domainIDs []int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	remove := make(map[int64]bool, len(domainIDs))
	for _, id := range domainIDs {
		remove[id] = true
	}
	kept := s.tagDomains[tag][:0]
	for _, id := range s.tagDomains[tag] {
		if !remove[id] {
			kept = append(kept, id)
		}
	}
	s.tagDomains[tag] = kept
	for _, domainID := range domainIDs {
		updated := s.domainTags[domainID][:0]
		for _, t := range s.domainTags[domainID] {
			if t != tag {
				updated = append(updated, t)
			}
		}
		s.domainTags[domainID] = updated
	}
	return nil
}

// GetDomainTags returns the tag names associated with a domain.
func (s *InMemoryJobStore) GetDomainTags(domainID int64) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tags := s.domainTags[domainID]
	if len(tags) == 0 {
		return nil
	}
	out := make([]string, len(tags))
	copy(out, tags)
	return out
}

// ListDomainsByTag returns domains associated with tag, applying filter.
func (s *InMemoryJobStore) ListDomainsByTag(tag string, filter DomainFilter) DomainList {
	filter.Tag = tag
	return s.ListDomains(filter)
}

// GetTagSummary returns the severity and grade distribution for domains with
// the latest run data in the given tag.
func (s *InMemoryJobStore) GetTagSummary(tag string) (TagSummary, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	domainIDs, ok := s.tagDomains[tag]
	if !ok {
		return TagSummary{}, false
	}
	summary := TagSummary{Tag: tag, DomainCount: len(domainIDs), Grades: map[string]int{}}
	for _, id := range domainIDs {
		d, ok := s.domainsByID[id]
		if !ok {
			continue
		}
		switch strings.ToUpper(d.LatestLevel) {
		case "CRITICAL":
			summary.Critical++
		case "ERROR":
			summary.Error++
		case "WARNING":
			summary.Warning++
		case "NOTICE":
			summary.Notice++
		default:
			summary.OK++
		}
		if d.LatestRunID != "" {
			if run, ok := s.runs[d.LatestRunID]; ok && run.Grade != nil {
				summary.Grades[*run.Grade]++
			}
		}
	}
	return summary, true
}

// GetRun returns a graduated run by its ID.
func (s *InMemoryJobStore) GetRun(id string) (Run, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	run, ok := s.runs[id]
	run.ProfileID = cloneInt64Ptr(run.ProfileID)
	run.NameserverTimings = cloneNameserverTimings(run.NameserverTimings)
	return run, ok
}

// GetRunByPublicID returns a graduated run by its public_id.
func (s *InMemoryJobStore) GetRunByPublicID(publicID string) (Run, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	runID, ok := s.runPublicIDs[publicID]
	if !ok {
		return Run{}, false
	}
	run, ok := s.runs[runID]
	run.ProfileID = cloneInt64Ptr(run.ProfileID)
	run.NameserverTimings = cloneNameserverTimings(run.NameserverTimings)
	return run, ok
}

// ListRuns returns graduated runs matching filter with sorting and pagination.
func (s *InMemoryJobStore) ListRuns(filter RunFilter) RunList {
	s.mu.RLock()
	snapshot := make([]Run, 0, len(s.runs))
	for _, r := range s.runs {
		r.ProfileID = cloneInt64Ptr(r.ProfileID)
		r.NameserverTimings = cloneNameserverTimings(r.NameserverTimings)
		snapshot = append(snapshot, r)
	}
	var tagDomainIDs map[int64]struct{}
	if filter.Tag != "" {
		tagDomainIDs = make(map[int64]struct{})
		for _, id := range s.tagDomains[filter.Tag] {
			tagDomainIDs[id] = struct{}{}
		}
	}
	s.mu.RUnlock()

	items := make([]Run, 0, len(snapshot))
	for _, r := range snapshot {
		if filter.DomainID != 0 && r.DomainID != filter.DomainID {
			continue
		}
		if tagDomainIDs != nil {
			if _, ok := tagDomainIDs[r.DomainID]; !ok {
				continue
			}
		}
		if filter.Domain != "" && !strings.Contains(strings.ToLower(r.Domain), strings.ToLower(filter.Domain)) {
			continue
		}
		if filter.BatchID != "" && r.BatchID != filter.BatchID {
			continue
		}
		if filter.Status != "" && r.Status != filter.Status {
			continue
		}
		if filter.WorstLevel != "" && r.WorstLevel != filter.WorstLevel {
			continue
		}
		if filter.Grade != "" && (r.Grade == nil || *r.Grade != filter.Grade) {
			continue
		}
		if !filter.FinishedAfter.IsZero() && r.FinishedAt.Before(filter.FinishedAfter) {
			continue
		}
		if !filter.FinishedBefore.IsZero() && r.FinishedAt.After(filter.FinishedBefore) {
			continue
		}
		items = append(items, r)
	}

	// Sort.
	sort.Slice(items, func(i, j int) bool {
		l, r := items[i], items[j]
		switch filter.Sort {
		case JobSortCreatedAtAsc:
			if !l.CreatedAt.Equal(r.CreatedAt) {
				return l.CreatedAt.Before(r.CreatedAt)
			}
		case JobSortCreatedAtDesc:
			if !l.CreatedAt.Equal(r.CreatedAt) {
				return l.CreatedAt.After(r.CreatedAt)
			}
		case JobSortDomainAsc:
			if l.Domain != r.Domain {
				return l.Domain < r.Domain
			}
		case JobSortDomainDesc:
			if l.Domain != r.Domain {
				return l.Domain > r.Domain
			}
		case JobSortBatchIDAsc:
			if l.BatchID != r.BatchID {
				return l.BatchID < r.BatchID
			}
		case JobSortBatchIDDesc:
			if l.BatchID != r.BatchID {
				return l.BatchID > r.BatchID
			}
		case JobSortErrorDesc:
			lErr := l.SevError + l.SevCritical
			rErr := r.SevError + r.SevCritical
			if lErr != rErr {
				return lErr > rErr
			}
		case JobSortCriticalDesc:
			if l.SevCritical != r.SevCritical {
				return l.SevCritical > r.SevCritical
			}
		case JobSortFinishedAtDesc:
			if !l.FinishedAt.Equal(r.FinishedAt) {
				return l.FinishedAt.After(r.FinishedAt)
			}
		case JobSortFinishedAtAsc:
			if !l.FinishedAt.Equal(r.FinishedAt) {
				return l.FinishedAt.Before(r.FinishedAt)
			}
		case JobSortWorstLevelDesc:
			li, ri := severityRank(l.WorstLevel), severityRank(r.WorstLevel)
			if li != ri {
				return li > ri
			}
		case JobSortWorstLevelAsc:
			li, ri := severityRank(l.WorstLevel), severityRank(r.WorstLevel)
			if li != ri {
				return li < ri
			}
		case JobSortScoreDesc:
			ls, rs := derefIntOr(l.Score, -1), derefIntOr(r.Score, -1)
			if ls != rs {
				return ls > rs
			}
		case JobSortScoreAsc:
			ls, rs := derefIntOr(l.Score, 9999), derefIntOr(r.Score, 9999)
			if ls != rs {
				return ls < rs
			}
		case JobSortDurationDesc:
			ld, rd := l.DurationMs, r.DurationMs
			if ld != rd {
				return ld > rd
			}
		case JobSortDurationAsc:
			ld, rd := l.DurationMs, r.DurationMs
			if ld != rd {
				return ld < rd
			}
		case JobSortEntryCountDesc:
			if l.EntryCount != r.EntryCount {
				return l.EntryCount > r.EntryCount
			}
		case JobSortEntryCountAsc:
			if l.EntryCount != r.EntryCount {
				return l.EntryCount < r.EntryCount
			}
		}
		if !l.FinishedAt.Equal(r.FinishedAt) {
			return l.FinishedAt.After(r.FinishedAt)
		}
		return l.ID < r.ID
	})

	total := len(items)
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	offset := max(filter.Offset, 0)
	start := min(offset, total)
	end := min(start+limit, total)

	list := RunList{
		Items:  items[start:end],
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}
	if start > 0 {
		prev := max(start-limit, 0)
		list.PrevCursor = strconv.Itoa(prev)
	}
	if end < total {
		list.NextCursor = strconv.Itoa(end)
	}
	return list
}

// ListRunsByDomain returns paginated runs for a specific domain.
func (s *InMemoryJobStore) ListRunsByDomain(domainID int64, limit, offset int) RunList {
	return s.ListRuns(RunFilter{DomainID: domainID, Limit: limit, Offset: offset})
}

// QueryEntries returns entries across runs matching the given filter.
func (s *InMemoryJobStore) QueryEntries(filter EntryFilter) EntryList {
	s.mu.RLock()

	// Build set of allowed run IDs when LatestOnly is requested.
	var latestRunIDs map[string]struct{}
	if filter.LatestOnly {
		latestRunIDs = make(map[string]struct{})
		for _, d := range s.domainsByID {
			if d.LatestRunID != "" {
				latestRunIDs[d.LatestRunID] = struct{}{}
			}
		}
	}

	// Build set of domain IDs allowed by Tag filter.
	var tagDomainIDs map[int64]struct{}
	if filter.Tag != "" {
		tagDomainIDs = make(map[int64]struct{})
		for _, id := range s.tagDomains[filter.Tag] {
			tagDomainIDs[id] = struct{}{}
		}
	}

	// Build set of run IDs allowed by BatchID filter.
	var batchRunIDs map[string]struct{}
	if filter.BatchID != "" {
		batchRunIDs = make(map[string]struct{})
		for _, r := range s.runs {
			if r.BatchID == filter.BatchID {
				batchRunIDs[r.ID] = struct{}{}
			}
		}
	}

	var all []Entry
	for runID, runEntries := range s.entries {
		if filter.RunID != "" && runID != filter.RunID {
			continue
		}
		if filter.LatestOnly {
			if _, ok := latestRunIDs[runID]; !ok {
				continue
			}
		}
		if filter.BatchID != "" {
			if _, ok := batchRunIDs[runID]; !ok {
				continue
			}
		}
		for _, e := range runEntries {
			if filter.DomainID != 0 && e.DomainID != filter.DomainID {
				continue
			}
			if filter.Tag != "" {
				if _, ok := tagDomainIDs[e.DomainID]; !ok {
					continue
				}
			}
			if filter.Module != "" && !strings.EqualFold(e.Module, filter.Module) {
				continue
			}
			if filter.Testcase != "" && !strings.EqualFold(e.Testcase, filter.Testcase) {
				continue
			}
			if filter.EntryTag != "" && e.Tag != filter.EntryTag {
				continue
			}
			if filter.Level != "" && e.Level != filter.Level {
				continue
			}
			all = append(all, e)
		}
	}
	s.mu.RUnlock()

	// Sort by run_id then id for determinism.
	sort.Slice(all, func(i, j int) bool {
		if all[i].RunID != all[j].RunID {
			return all[i].RunID < all[j].RunID
		}
		return all[i].ID < all[j].ID
	})

	total := len(all)
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	offset := max(filter.Offset, 0)
	start := min(offset, total)
	end := min(start+limit, total)

	list := EntryList{
		Items:  all[start:end],
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}
	if start > 0 {
		prev := max(start-limit, 0)
		list.PrevCursor = strconv.Itoa(prev)
	}
	if end < total {
		list.NextCursor = strconv.Itoa(end)
	}
	return list
}

// CreateBatch stores a batch record.
func (s *InMemoryJobStore) CreateBatch(batch Batch) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.batches[batch.ID] = batch
	return nil
}

// GetBatch returns a batch by ID.
func (s *InMemoryJobStore) GetBatch(id string) (Batch, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.batches[id]
	return b, ok
}

// SetBatchSnapshotIntent flips snapshot_intent on an existing batch.
func (s *InMemoryJobStore) SetBatchSnapshotIntent(batchID string, intent bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.batches[batchID]
	if !ok {
		return ErrBatchNotFound
	}
	b.SnapshotIntent = intent
	s.batches[batchID] = b
	return nil
}

// ListBatchesByTag returns batches whose Tag matches, newest first.
func (s *InMemoryJobStore) ListBatchesByTag(tag string, limit, offset int) BatchList {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	items := make([]Batch, 0)
	for _, b := range s.batches {
		if b.Tag == tag {
			items = append(items, b)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if !items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
		return items[i].ID > items[j].ID
	})
	total := len(items)
	start := min(offset, total)
	end := min(start+limit, total)
	return BatchList{
		Items:  items[start:end],
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}
}

// BatchDeletePreviewStats returns per-batch row counts for the admin
// confirmation modal. Snapshot list is left to the handler.
func (s *InMemoryJobStore) BatchDeletePreviewStats(batchID string) (BatchDeletePreview, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := BatchDeletePreview{BatchID: batchID}
	if b, ok := s.batches[batchID]; ok {
		out.Exists = true
		out.Tag = b.Tag
		out.CreatedAt = b.CreatedAt
		out.SnapshotIntent = b.SnapshotIntent
	}
	for _, job := range s.jobs {
		if job.BatchID != batchID {
			continue
		}
		out.Exists = true
		switch job.Status {
		case JobQueued:
			out.QueuedJobs++
		case JobRunning:
			out.RunningJobs++
		}
	}
	for _, r := range s.runs {
		if r.BatchID != batchID {
			continue
		}
		out.Exists = true
		out.CompletedRuns++
		out.Entries += len(s.entries[r.ID])
	}
	return out, nil
}

// BatchHasRuns reports whether any run row still exists for batchID.
func (s *InMemoryJobStore) BatchHasRuns(batchID string) bool {
	if batchID == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range s.runs {
		if r.BatchID == batchID {
			return true
		}
	}
	return false
}

// DeleteBatch removes the batch metadata plus every in-memory row
// derived from it: queued/graduated runs for the batch, their entries,
// and domain latest_* pointers that referenced a deleted run.
func (s *InMemoryJobStore) DeleteBatch(batchID string) ([]int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if batchID == "" {
		return nil, errors.New("batch id is required")
	}
	deletedRunIDs := map[string]struct{}{}
	affectedDomains := map[int64]struct{}{}
	for id, r := range s.runs {
		if r.BatchID != batchID {
			continue
		}
		deletedRunIDs[id] = struct{}{}
		if r.DomainID != 0 {
			affectedDomains[r.DomainID] = struct{}{}
		}
		if r.PublicID != "" {
			delete(s.runPublicIDs, r.PublicID)
		}
		delete(s.entries, id)
		delete(s.runs, id)
	}
	for id, job := range s.jobs {
		if job.BatchID != batchID {
			continue
		}
		if job.PublicID != "" {
			delete(s.publicIDs, job.PublicID)
		}
		delete(s.jobs, id)
	}
	delete(s.batches, batchID)
	for domainID := range affectedDomains {
		d, ok := s.domainsByID[domainID]
		if !ok || d == nil {
			continue
		}
		if _, stale := deletedRunIDs[d.LatestRunID]; !stale {
			continue
		}
		d.LatestRunID = ""
		d.LatestRunAt = time.Time{}
		d.LatestStatus = ""
		d.LatestLevel = ""
		d.LatestScore = nil
		d.LatestGrade = nil
		var best *Run
		for _, r := range s.runs {
			if r.DomainID != domainID {
				continue
			}
			if best == nil || r.FinishedAt.After(best.FinishedAt) {
				runCopy := r
				best = &runCopy
			}
		}
		if best != nil {
			d.LatestRunID = best.ID
			d.LatestRunAt = best.FinishedAt
			d.LatestStatus = string(best.Status)
			d.LatestLevel = best.WorstLevel
			if best.Score != nil {
				score := *best.Score
				d.LatestScore = &score
			}
			if best.Grade != nil {
				grade := *best.Grade
				d.LatestGrade = &grade
			}
		}
		remaining := 0
		for _, r := range s.runs {
			if r.DomainID == domainID {
				remaining++
			}
		}
		d.RunCount = remaining
	}
	return nil, nil
}

// CreateProfile stores a new profile and assigns an auto-incremented ID.
func (s *InMemoryJobStore) CreateProfile(p StoredProfile) (StoredProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.profiles {
		if existing.Name == p.Name {
			return StoredProfile{}, fmt.Errorf("profile name %q already exists", p.Name)
		}
	}
	s.profileCounter++
	p.ID = s.profileCounter
	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = now
	}
	s.profiles[p.ID] = p
	return p, nil
}

// GetProfile returns a profile by ID.
func (s *InMemoryJobStore) GetProfile(id int64) (StoredProfile, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.profiles[id]
	return p, ok
}

// GetProfileByName returns a profile by its unique name.
func (s *InMemoryJobStore) GetProfileByName(name string) (StoredProfile, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.profiles {
		if p.Name == name {
			return p, true
		}
	}
	return StoredProfile{}, false
}

// UpdateProfile updates an existing profile.
func (s *InMemoryJobStore) UpdateProfile(p StoredProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.profiles[p.ID]; !ok {
		return fmt.Errorf("profile %d not found", p.ID)
	}
	for _, existing := range s.profiles {
		if existing.Name == p.Name && existing.ID != p.ID {
			return fmt.Errorf("profile name %q already exists", p.Name)
		}
	}
	p.UpdatedAt = time.Now().UTC()
	s.profiles[p.ID] = p
	return nil
}

// DeleteProfile removes a profile by ID.
func (s *InMemoryJobStore) DeleteProfile(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.profiles[id]; !ok {
		return fmt.Errorf("profile %d not found", id)
	}
	delete(s.profiles, id)
	for name, tag := range s.tags {
		if tag.DefaultProfileID != nil && *tag.DefaultProfileID == id {
			tag.DefaultProfileID = nil
			s.tags[name] = tag
		}
	}
	for jobID, job := range s.jobs {
		if job.ProfileID != nil && *job.ProfileID == id {
			job.ProfileID = nil
			s.jobs[jobID] = job
		}
	}
	for runID, run := range s.runs {
		if run.ProfileID != nil && *run.ProfileID == id {
			run.ProfileID = nil
			s.runs[runID] = run
		}
	}
	return nil
}

// ListProfiles returns all profiles ordered by name.
func (s *InMemoryJobStore) ListProfiles() []StoredProfile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]StoredProfile, 0, len(s.profiles))
	for _, p := range s.profiles {
		result = append(result, p)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// ── Settings ──────────────────────────────────────────────────────────────────

// GetSetting returns a setting value by key.
func (s *InMemoryJobStore) GetSetting(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.settings[key]
	return v, ok
}

// SetSetting creates or updates a setting.
func (s *InMemoryJobStore) SetSetting(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings[key] = value
	return nil
}

// DeleteSetting removes a setting by key.
func (s *InMemoryJobStore) DeleteSetting(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.settings[key]; !ok {
		return errors.New("setting not found")
	}
	delete(s.settings, key)
	return nil
}

// ListSettings returns all settings as a map.
func (s *InMemoryJobStore) ListSettings() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]string, len(s.settings))
	maps.Copy(result, s.settings)
	return result
}

// PurgeOlderThan deletes terminal-status runs whose finished_at is before
// cutoff, along with their entries. Returns the number of runs deleted.
func (s *InMemoryJobStore) PurgeOlderThan(cutoff time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var count int64
	for id, run := range s.runs {
		if !isTerminalStatus(run.Status) {
			continue
		}
		if run.FinishedAt.IsZero() || !run.FinishedAt.Before(cutoff) {
			continue
		}
		delete(s.runPublicIDs, run.PublicID)
		delete(s.entries, id)
		delete(s.runs, id)
		count++
	}
	return count, nil
}

// ── Helper functions ──────────────────────────────────────────────────────────

// jobFromRun reconstructs a Job from a graduated Run for API compatibility.
func jobFromRun(r Run) Job {
	return Job{
		ID:         r.ID,
		PublicID:   r.PublicID,
		BatchID:    r.BatchID,
		DomainID:   r.DomainID,
		Domain:     r.Domain,
		Status:     r.Status,
		FinishedAt: r.FinishedAt,
		SeverityTotals: map[string]int{
			"NOTICE":   r.SevNotice,
			"WARNING":  r.SevWarning,
			"ERROR":    r.SevError,
			"CRITICAL": r.SevCritical,
		},
		CreatedAt:         r.CreatedAt,
		StartedAt:         r.StartedAt,
		Progress:          100,
		Priority:          r.Priority,
		Profile:           r.Profile,
		ProfileID:         cloneInt64Ptr(r.ProfileID),
		ProfileName:       r.ProfileName,
		EffectiveProfile:  r.EffectiveProfile,
		Score:             r.Score,
		Grade:             r.Grade,
		NameserverTimings: cloneNameserverTimings(r.NameserverTimings),
		Error:             r.Error,
	}
}

func cloneInt64Ptr(v *int64) *int64 {
	if v == nil {
		return nil
	}
	cloned := *v
	return &cloned
}

// buildJobResult constructs a JobResult from a run, its stored entries, and a
// pre-computed scoring result. scoreResult may be nil when scoring is unavailable.
func buildJobResult(r Run, entries []Entry, scoreResult *scoring.Result) JobResult {
	result := JobResult{
		JobID:             r.ID,
		BatchID:           r.BatchID,
		Status:            r.Status,
		NameserverTimings: cloneNameserverTimings(r.NameserverTimings),
		Summary: map[string]any{
			"total": r.EntryCount,
			"levels": map[string]int{
				"NOTICE":   r.SevNotice,
				"WARNING":  r.SevWarning,
				"ERROR":    r.SevError,
				"CRITICAL": r.SevCritical,
			},
		},
		Score: scoreResult,
	}
	if len(entries) > 0 {
		resultEntries := make([]JobResultEntry, len(entries))
		for i, e := range entries {
			resultEntries[i] = JobResultEntry{
				Timestamp: e.Timestamp,
				Module:    e.Module,
				Testcase:  e.Testcase,
				Tag:       e.Tag,
				Level:     e.Level,
				Args:      e.Args,
			}
		}
		result.Raw = &JobResultRaw{Entries: resultEntries}
	}
	return result
}

// computeWorstLevel returns the highest severity level present.
func computeWorstLevel(notice, warning, errCount, critical int) string {
	switch {
	case critical > 0:
		return "CRITICAL"
	case errCount > 0:
		return "ERROR"
	case warning > 0:
		return "WARNING"
	case notice > 0:
		return "NOTICE"
	default:
		return ""
	}
}

func isTerminalStatus(status JobStatus) bool {
	switch status {
	case JobSucceeded, JobFailed, JobCanceled, JobExpired:
		return true
	default:
		return false
	}
}

func parseTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

func normalizeJobSort(value JobSort) JobSort {
	switch value {
	case JobSortCreatedAtDesc, JobSortCreatedAtAsc,
		JobSortStartedAtDesc, JobSortStartedAtAsc,
		JobSortDomainAsc, JobSortDomainDesc,
		JobSortBatchIDAsc, JobSortBatchIDDesc,
		JobSortErrorDesc, JobSortCriticalDesc:
		return value
	default:
		return JobSortStartedAtDesc
	}
}

func isValidJobSort(value JobSort) bool {
	switch value {
	case JobSortCreatedAtDesc, JobSortCreatedAtAsc,
		JobSortStartedAtDesc, JobSortStartedAtAsc,
		JobSortDomainAsc, JobSortDomainDesc,
		JobSortBatchIDAsc, JobSortBatchIDDesc,
		JobSortErrorDesc, JobSortCriticalDesc:
		return true
	default:
		return false
	}
}

func isValidJobSeverityFilter(value JobSeverityFilter) bool {
	switch value {
	case JobSeverityWarningsPlus, JobSeverityErrorsOnly:
		return true
	default:
		return false
	}
}

// sortJobSlice sorts a slice of jobs in-place by sortOrder.
func sortJobSlice(items []Job, sortOrder JobSort) {
	normalizedSort := normalizeJobSort(sortOrder)
	sort.Slice(items, func(i, j int) bool {
		left := items[i]
		right := items[j]
		switch normalizedSort {
		case JobSortCreatedAtAsc:
			if !left.CreatedAt.Equal(right.CreatedAt) {
				return left.CreatedAt.Before(right.CreatedAt)
			}
		case JobSortCreatedAtDesc:
			if !left.CreatedAt.Equal(right.CreatedAt) {
				return left.CreatedAt.After(right.CreatedAt)
			}
		case JobSortStartedAtDesc:
			leftStarted := effectiveStartTime(left)
			rightStarted := effectiveStartTime(right)
			if !leftStarted.Equal(rightStarted) {
				return leftStarted.After(rightStarted)
			}
		case JobSortStartedAtAsc:
			leftStarted := effectiveStartTime(left)
			rightStarted := effectiveStartTime(right)
			if !leftStarted.Equal(rightStarted) {
				return leftStarted.Before(rightStarted)
			}
		case JobSortDomainAsc:
			leftDomain := strings.ToLower(left.Domain)
			rightDomain := strings.ToLower(right.Domain)
			if leftDomain != rightDomain {
				return leftDomain < rightDomain
			}
		case JobSortDomainDesc:
			leftDomain := strings.ToLower(left.Domain)
			rightDomain := strings.ToLower(right.Domain)
			if leftDomain != rightDomain {
				return leftDomain > rightDomain
			}
		case JobSortBatchIDAsc:
			leftBatchID := strings.ToLower(left.BatchID)
			rightBatchID := strings.ToLower(right.BatchID)
			if leftBatchID != rightBatchID {
				return leftBatchID < rightBatchID
			}
		case JobSortBatchIDDesc:
			leftBatchID := strings.ToLower(left.BatchID)
			rightBatchID := strings.ToLower(right.BatchID)
			if leftBatchID != rightBatchID {
				return leftBatchID > rightBatchID
			}
		}
		if !left.CreatedAt.Equal(right.CreatedAt) {
			return left.CreatedAt.After(right.CreatedAt)
		}
		return left.ID < right.ID
	})
}

func effectiveStartTime(job Job) time.Time {
	if !job.StartedAt.IsZero() {
		return job.StartedAt
	}
	return job.CreatedAt
}
