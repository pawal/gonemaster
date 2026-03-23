package server

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

var severityLevels = []string{"NOTICE", "WARNING", "ERROR", "CRITICAL"}

// JobStore persists job metadata and results.
type JobStore interface {
	// Job queue management (in-flight jobs only).
	Create(job Job) (Job, error)
	Get(id string) (Job, bool)            // checks jobs first, then reconstructs from runs
	GetByPublicID(publicID string) (Job, bool) // checks jobs then runs
	Update(job Job) error                 // only for in-flight jobs
	List(filter JobFilter) JobList        // only in-flight jobs

	// GraduateJob atomically creates a run + entries, updates domain latest_*,
	// and deletes the job from the queue. entries may be nil for canceled jobs.
	GraduateJob(job Job, entries []engine.LogEntry) error

	// GetResult reconstructs a JobResult from the runs+entries tables.
	GetResult(jobID string) (JobResult, bool)

	// Domain management.
	GetOrCreateDomain(name string) (Domain, error)
	GetDomain(id int64) (Domain, bool)
	GetDomainByName(name string) (Domain, bool)
	ListDomains(filter DomainFilter) DomainList
	UpdateDomainLatest(domainID int64, runID string, finishedAt time.Time, status, level string) error

	// Tag management.
	CreateTag(name, description string) error
	GetTag(name string) (Tag, bool)
	UpdateTag(name, description string) error
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

	// Batch management.
	CreateBatch(batch Batch) error
	GetBatch(id string) (Batch, bool)

	// PurgeOlderThan deletes terminal runs whose finished_at is before cutoff,
	// along with their entries. Returns the number of runs deleted.
	PurgeOlderThan(cutoff time.Time) (int64, error)
}

// InMemoryJobStore stores all data in memory.
type InMemoryJobStore struct {
	mu sync.RWMutex

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
	tags       map[string]Tag      // name → Tag
	domainTags map[int64][]string  // domainID → []tagName
	tagDomains map[string][]int64  // tagName → []domainID

	// Batches.
	batches map[string]Batch // batchID → Batch
}

// NewInMemoryJobStore creates an empty in-memory job store.
func NewInMemoryJobStore() *InMemoryJobStore {
	return &InMemoryJobStore{
		jobs:         map[string]Job{},
		publicIDs:    map[string]string{},
		runs:         map[string]Run{},
		runPublicIDs: map[string]string{},
		entries:      map[string][]Entry{},
		domains:      map[string]*Domain{},
		domainsByID:  map[int64]*Domain{},
		tags:         map[string]Tag{},
		domainTags:   map[int64][]string{},
		tagDomains:   map[string][]int64{},
		batches:      map[string]Batch{},
	}
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

	total := len(items)
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	start := offset
	if start > len(items) {
		start = len(items)
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}

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
		prevOffset := start - limit
		if prevOffset < 0 {
			prevOffset = 0
		}
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

	run := Run{
		ID:          job.ID,
		DomainID:    domain.ID,
		Domain:      job.Domain,
		BatchID:     job.BatchID,
		Status:      job.Status,
		CreatedAt:   job.CreatedAt,
		StartedAt:   job.StartedAt,
		FinishedAt:  job.FinishedAt,
		DurationMs:  durationMs,
		SevNotice:   sevNotice,
		SevWarning:  sevWarning,
		SevError:    sevError,
		SevCritical: sevCritical,
		WorstLevel:  worstLevel,
		EntryCount:  len(engineEntries),
		Profile:     job.Profile,
		PublicID:    job.PublicID,
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
	return buildJobResult(run, entries), true
}

// GetOrCreateDomain returns the domain for name, creating it if necessary.
func (s *InMemoryJobStore) GetOrCreateDomain(name string) (Domain, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.getOrCreateDomainLocked(name)
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
		if filter.Tag != "" {
			tags := s.domainTags[d.ID]
			found := false
			for _, t := range tags {
				if t == filter.Tag {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if filter.Name != "" && !strings.Contains(strings.ToLower(d.Name), strings.ToLower(filter.Name)) {
			continue
		}
		if filter.LatestLevel != "" && d.LatestLevel != filter.LatestLevel {
			continue
		}
		dc := *d
		dc.Tags = append([]string(nil), s.domainTags[d.ID]...)
		items = append(items, dc)
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
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
	start := offset
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
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
		tags = append(tags, t)
	}
	sort.Slice(tags, func(i, j int) bool {
		return tags[i].Name < tags[j].Name
	})
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	if offset > len(tags) {
		return []Tag{}
	}
	end := offset + limit
	if end > len(tags) {
		end = len(tags)
	}
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
		alreadyTagged := false
		for _, existing := range s.domainTags[id] {
			if existing == tag {
				alreadyTagged = true
				break
			}
		}
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

// GetTagSummary returns the severity distribution for domains with the latest
// run data in the given tag.
func (s *InMemoryJobStore) GetTagSummary(tag string) (TagSummary, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	domainIDs, ok := s.tagDomains[tag]
	if !ok {
		return TagSummary{}, false
	}
	summary := TagSummary{Tag: tag, DomainCount: len(domainIDs)}
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
	}
	return summary, true
}

// GetRun returns a graduated run by its ID.
func (s *InMemoryJobStore) GetRun(id string) (Run, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	run, ok := s.runs[id]
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
	return run, ok
}

// ListRuns returns graduated runs matching filter with sorting and pagination.
func (s *InMemoryJobStore) ListRuns(filter RunFilter) RunList {
	s.mu.RLock()
	snapshot := make([]Run, 0, len(s.runs))
	for _, r := range s.runs {
		snapshot = append(snapshot, r)
	}
	s.mu.RUnlock()

	items := make([]Run, 0, len(snapshot))
	for _, r := range snapshot {
		if filter.DomainID != 0 && r.DomainID != filter.DomainID {
			continue
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
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	start := offset
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}

	list := RunList{
		Items:  items[start:end],
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}
	if start > 0 {
		prev := start - limit
		if prev < 0 {
			prev = 0
		}
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
		ID:       r.ID,
		PublicID: r.PublicID,
		BatchID:  r.BatchID,
		DomainID: r.DomainID,
		Domain:   r.Domain,
		Status:   r.Status,
		FinishedAt: r.FinishedAt,
		SeverityTotals: map[string]int{
			"NOTICE":   r.SevNotice,
			"WARNING":  r.SevWarning,
			"ERROR":    r.SevError,
			"CRITICAL": r.SevCritical,
		},
		CreatedAt: r.CreatedAt,
		StartedAt: r.StartedAt,
		Progress:  100,
		Profile:   r.Profile,
	}
}

// buildJobResult constructs a JobResult from a run and its stored entries.
func buildJobResult(r Run, entries []Entry) JobResult {
	result := JobResult{
		JobID:   r.ID,
		BatchID: r.BatchID,
		Status:  r.Status,
		Summary: map[string]any{
			"total": r.EntryCount,
			"levels": map[string]int{
				"NOTICE":   r.SevNotice,
				"WARNING":  r.SevWarning,
				"ERROR":    r.SevError,
				"CRITICAL": r.SevCritical,
			},
		},
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

func normalizeJobSeverityFilter(value JobSeverityFilter) JobSeverityFilter {
	switch value {
	case JobSeverityWarningsPlus, JobSeverityErrorsOnly:
		return value
	default:
		return ""
	}
}

func effectiveStartTime(job Job) time.Time {
	if !job.StartedAt.IsZero() {
		return job.StartedAt
	}
	return job.CreatedAt
}

func zeroSeverityTotals() map[string]int {
	return map[string]int{
		"NOTICE":   0,
		"WARNING":  0,
		"ERROR":    0,
		"CRITICAL": 0,
	}
}
