package server

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/i18n"
	"codeberg.org/pawal/gonemaster/engine/normalization"
)

const maxListLimit = 500

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		if !enforceCSRF(w, r) {
			return
		}
		s.handleCreateJob(w, r)
	case http.MethodGet:
		s.handleListJobs(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *Server) handleJobsBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	if !enforceCSRF(w, r) {
		return
	}
	var req JobBatchRequest
	if err := readJSON(r, s.cfg.MaxBodySize, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	if req.Nameservers != nil || req.DSInfo != nil {
		writeError(w, http.StatusBadRequest, "undelegated_not_supported_for_batch", "undelegated input is only supported for POST /jobs", nil)
		return
	}
	if req.FromTag != "" && len(req.Domains) > 0 {
		writeError(w, http.StatusBadRequest, "ambiguous_domains", "from_tag and domains are mutually exclusive", nil)
		return
	}

	// Resolve domain list: either from an explicit list or from a tag.
	domains := req.Domains
	if req.FromTag != "" {
		if _, ok := s.store.GetTag(req.FromTag); !ok {
			writeError(w, http.StatusBadRequest, "tag_not_found", "tag not found: "+req.FromTag, nil)
			return
		}
		list := s.store.ListDomainsByTag(req.FromTag, DomainFilter{Limit: 10000})
		for _, d := range list.Items {
			domains = append(domains, d.Name)
		}
	}
	if len(domains) == 0 {
		writeError(w, http.StatusBadRequest, "missing_domain", "domains is required", nil)
		return
	}

	tagNames, ok := validateTags(w, s, req.Tags)
	if !ok {
		return
	}
	resolvedProfile := resolvedProfileRef{}
	if req.ProfileID != nil {
		var code, message string
		resolvedProfile, code, message = s.resolveStoredProfile(req.ProfileID, false)
		if code != "" {
			writeError(w, http.StatusBadRequest, code, message, nil)
			return
		}
	} else if len(req.ProfileOverrides) == 0 && req.FromTag != "" {
		var code, message string
		resolvedProfile, code, message = s.resolveDefaultProfileFromTags([]string{req.FromTag})
		if code != "" {
			writeError(w, http.StatusBadRequest, code, message, nil)
			return
		}
	}

	batchID := newID("batch")
	now := time.Now().UTC()
	jobIDs := make([]string, 0, len(domains))
	var domainIDs []int64
	for _, domain := range domains {
		trimmed := strings.TrimSpace(domain)
		if trimmed == "" {
			writeError(w, http.StatusBadRequest, "missing_domain", "domains is required", nil)
			return
		}
		if errs, normalized := normalization.NormalizeName(trimmed); len(errs) > 0 {
			writeError(w, http.StatusBadRequest, "invalid_domain", errs[0].Message(), nil)
			return
		} else {
			trimmed = normalized
		}
		job := Job{
			ID:          newID("job"),
			BatchID:     batchID,
			Domain:      trimmed,
			Tests:       req.Tests,
			Overrides:   req.ProfileOverrides,
			MinLevel:    req.MinLevel,
			Priority:    PriorityBatch,
			Status:      JobQueued,
			CreatedAt:   now,
			Progress:    0,
			ProfileID:   cloneInt64Ptr(resolvedProfile.ID),
			ProfileName: resolvedProfile.Name,
		}
		created, err := s.store.Create(job)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
			return
		}
		_ = s.queue.Enqueue(created.ID, PriorityBatch)
		s.metrics.ObserveJobSubmittedWithContext(created.BatchID, created.Domain, JobQueued)
		jobIDs = append(jobIDs, created.ID)
		if len(tagNames) > 0 {
			d, err := s.store.GetOrCreateDomain(trimmed)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
				return
			}
			domainIDs = append(domainIDs, d.ID)
		}
	}

	for _, tagName := range tagNames {
		_ = s.store.TagDomains(tagName, domainIDs)
	}

	_ = s.store.CreateBatch(Batch{
		ID:          batchID,
		Tag:         req.FromTag,
		Description: req.Description,
		DomainCount: len(jobIDs),
		CreatedAt:   now,
	})

	writeJSON(w, http.StatusAccepted, JobBatchResponse{BatchID: batchID, JobIDs: jobIDs})
}

func (s *Server) handleJobByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/jobs/")
	if path == "" || path == r.URL.Path {
		writeError(w, http.StatusNotFound, "not_found", "not found", nil)
		return
	}
	parts := strings.Split(path, "/")
	jobID := parts[0]
	if jobID == "" {
		writeError(w, http.StatusNotFound, "not_found", "not found", nil)
		return
	}

	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
			return
		}
		s.handleGetJob(w, r, jobID)
		return
	}

	switch parts[1] {
	case "result":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
			return
		}
		s.handleGetJobResult(w, r, jobID)
	case "events":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
			return
		}
		s.handleJobEvents(w, r, jobID)
	case "cancel":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
			return
		}
		if !enforceCSRF(w, r) {
			return
		}
		s.handleCancelJob(w, r, jobID)
	default:
		writeError(w, http.StatusNotFound, "not_found", "not found", nil)
	}
}

func (s *Server) handleBatchByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/batches/")
	if path == "" || path == r.URL.Path {
		writeError(w, http.StatusNotFound, "not_found", "not found", nil)
		return
	}
	batchID := strings.TrimSpace(path)
	if batchID == "" {
		writeError(w, http.StatusNotFound, "not_found", "not found", nil)
		return
	}

	filter, code, message := parseListFilter(r, 100)
	if code != "" {
		writeError(w, http.StatusBadRequest, code, message, nil)
		return
	}

	// Verify batch exists via the batches table or fallback to jobs/runs.
	batchRecord, batchExists := s.store.GetBatch(batchID)
	if !batchExists {
		// Fallback: check if any jobs or runs have this batch_id.
		probe := s.store.List(JobFilter{BatchID: batchID, Limit: 1})
		if probe.Total == 0 {
			runsProbe := s.store.ListRuns(RunFilter{BatchID: batchID, Limit: 1})
			if runsProbe.Total == 0 {
				writeError(w, http.StatusNotFound, "not_found", "batch not found", nil)
				return
			}
		}
	}

	// Collect all in-flight jobs for this batch.
	allJobs := s.store.List(JobFilter{BatchID: batchID, Limit: 10000, Sort: JobSortCreatedAtAsc})
	// Collect all graduated runs for this batch.
	allRuns := s.store.ListRuns(RunFilter{BatchID: batchID, Limit: 10000})

	// Build grade distribution from graduated runs.
	grades := map[string]int{}
	for _, run := range allRuns.Items {
		if run.Grade != nil && *run.Grade != "" {
			grades[*run.Grade]++
		}
	}
	if len(grades) == 0 {
		grades = nil
	}

	// Build combined item list (jobs + runs converted to jobs).
	combined := make([]Job, 0, len(allJobs.Items)+len(allRuns.Items))
	combined = append(combined, allJobs.Items...)
	for _, run := range allRuns.Items {
		combined = append(combined, jobFromRun(run))
	}

	// Compute aggregate status counts and timing over the full combined list (all items, no filter).
	statusCounts := map[string]int{}
	var createdAt time.Time
	var startedAt *time.Time
	var finishedAtLatest time.Time
	allFinished := true

	for _, job := range combined {
		statusCounts[string(job.Status)]++
		if createdAt.IsZero() || job.CreatedAt.Before(createdAt) {
			createdAt = job.CreatedAt
		}
		if !job.StartedAt.IsZero() {
			if startedAt == nil || job.StartedAt.Before(*startedAt) {
				t := job.StartedAt
				startedAt = &t
			}
		}
		if job.FinishedAt.IsZero() {
			allFinished = false
		} else if finishedAtLatest.IsZero() || job.FinishedAt.After(finishedAtLatest) {
			finishedAtLatest = job.FinishedAt
		}
	}

	var finishedAt *time.Time
	if allFinished && !finishedAtLatest.IsZero() {
		finishedAt = &finishedAtLatest
	}

	// Apply optional filters (status, domain, created_after/before) to the combined list.
	filtered := combined[:0:0]
	for _, job := range combined {
		if filter.Status != "" && job.Status != filter.Status {
			continue
		}
		if filter.Domain != "" && !strings.Contains(job.Domain, filter.Domain) {
			continue
		}
		if !filter.CreatedAfter.IsZero() && !job.CreatedAt.After(filter.CreatedAfter) {
			continue
		}
		if !filter.CreatedBefore.IsZero() && job.CreatedAt.After(filter.CreatedBefore) {
			continue
		}
		filtered = append(filtered, job)
	}

	// Sort filtered list.
	sort.Slice(filtered, func(i, j int) bool {
		a, b := filtered[i], filtered[j]
		switch filter.Sort {
		case JobSortCreatedAtAsc:
			return a.CreatedAt.Before(b.CreatedAt)
		case JobSortStartedAtDesc:
			return a.StartedAt.After(b.StartedAt)
		case JobSortStartedAtAsc:
			return a.StartedAt.Before(b.StartedAt)
		case JobSortDomainAsc:
			return a.Domain < b.Domain
		case JobSortDomainDesc:
			return a.Domain > b.Domain
		default: // created_at_desc
			return a.CreatedAt.After(b.CreatedAt)
		}
	})

	// Apply pagination to filtered list.
	total := len(filtered)
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	start := offset
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	pageItems := filtered[start:end]

	var nextCursor, prevCursor string
	if end < total {
		nextCursor = strconv.Itoa(end)
	}
	if start > 0 {
		prev := start - limit
		if prev < 0 {
			prev = 0
		}
		prevCursor = strconv.Itoa(prev)
	}

	summary := BatchSummary{
		BatchID:      batchID,
		Tag:          batchRecord.Tag,
		Total:        total,
		StatusCounts: statusCounts,
		Grades:       grades,
		Items:        pageItems,
		Limit:        limit,
		Offset:       offset,
		Sort:         string(filter.Sort),
		NextCursor:   nextCursor,
		PrevCursor:   prevCursor,
		CreatedAt:    createdAt,
		StartedAt:    startedAt,
		FinishedAt:   finishedAt,
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	var req JobCreateRequest
	if err := readJSON(r, s.cfg.MaxBodySize, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	if strings.TrimSpace(req.Domain) == "" {
		writeError(w, http.StatusBadRequest, "missing_domain", "domain is required", nil)
		return
	}
	domain := strings.TrimSpace(req.Domain)
	if errs, normalized := normalization.NormalizeName(domain); len(errs) > 0 {
		writeError(w, http.StatusBadRequest, "invalid_domain", errs[0].Message(), nil)
		return
	} else {
		domain = normalized
	}
	undelegatedNS, undelegatedDS, err := normalizeUndelegatedInputs(req.Nameservers, req.DSInfo)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_undelegated", err.Error(), nil)
		return
	}
	tagNames, ok := validateTags(w, s, req.Tags)
	if !ok {
		return
	}
	resolvedProfile := resolvedProfileRef{}
	if req.ProfileID != nil {
		var code, message string
		resolvedProfile, code, message = s.resolveStoredProfile(req.ProfileID, false)
		if code != "" {
			writeError(w, http.StatusBadRequest, code, message, nil)
			return
		}
	} else if len(req.ProfileOverrides) == 0 {
		var code, message string
		resolvedProfile, code, message = s.resolveDefaultProfileFromTags(tagNames)
		if code != "" {
			writeError(w, http.StatusBadRequest, code, message, nil)
			return
		}
	}

	job := Job{
		ID:            newID("job"),
		Domain:        domain,
		Tests:         req.Tests,
		Overrides:     req.ProfileOverrides,
		UndelegatedNS: undelegatedNS,
		UndelegatedDS: undelegatedDS,
		MinLevel:      req.MinLevel,
		Status:        JobQueued,
		CreatedAt:     time.Now().UTC(),
		Progress:      0,
		ProfileID:     cloneInt64Ptr(resolvedProfile.ID),
		ProfileName:   resolvedProfile.Name,
	}
	created, err := s.store.Create(job)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	_ = s.queue.Enqueue(created.ID, PriorityNormal)
	s.metrics.ObserveJobSubmittedWithContext(created.BatchID, created.Domain, JobQueued)

	if len(tagNames) > 0 {
		d, err := s.store.GetOrCreateDomain(domain)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
			return
		}
		for _, tagName := range tagNames {
			_ = s.store.TagDomains(tagName, []int64{d.ID})
		}
	}

	writeJSON(w, http.StatusCreated, created)
}

// validateTags checks that all requested tag names exist in the store.
// Returns the deduplicated list of non-empty tag names, or writes a 400 and
// returns false if any tag is unknown.
func validateTags(w http.ResponseWriter, s *Server, tags []string) ([]string, bool) {
	var names []string
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, ok := s.store.GetTag(t); !ok {
			writeError(w, http.StatusBadRequest, "tag_not_found", "tag not found: "+t, nil)
			return nil, false
		}
		names = append(names, t)
	}
	return names, true
}

func normalizeUndelegatedInputs(nameservers []UndelegatedNameserverInput, dsInfo []UndelegatedDSInput) ([]engine.UndelegatedNameserver, []engine.UndelegatedDSInfo, error) {
	normalizedNS := make([]engine.UndelegatedNameserver, 0, len(nameservers))
	for i, item := range nameservers {
		name := strings.TrimSpace(item.NS)
		if name == "" {
			return nil, nil, fmt.Errorf("undelegated nameserver[%d]: ns is required", i)
		}
		normalizedNS = append(normalizedNS, engine.UndelegatedNameserver{
			Name: name,
			IP:   strings.TrimSpace(item.IP),
		})
	}

	normalizedDS := make([]engine.UndelegatedDSInfo, 0, len(dsInfo))
	for _, item := range dsInfo {
		normalizedDS = append(normalizedDS, engine.UndelegatedDSInfo{
			KeyTag:     item.KeyTag,
			Algorithm:  item.Algorithm,
			DigestType: item.DigType,
			Digest:     strings.TrimSpace(item.Digest),
		})
	}

	return engine.NormalizeUndelegatedInputs(normalizedNS, normalizedDS)
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	filter, code, message := parseListFilter(r, 100)
	if code != "" {
		writeError(w, http.StatusBadRequest, code, message, nil)
		return
	}

	// Determine which sources to query based on the status filter.
	// In-flight jobs (queued/running/paused) live in the jobs table.
	// Graduated jobs (succeeded/failed/canceled/expired) live in the runs table.
	isActiveOnly := filter.Status == JobQueued || filter.Status == JobRunning || filter.Status == JobPaused
	isTerminalOnly := !isActiveOnly && filter.Status != ""

	var allItems []Job

	if !isTerminalOnly {
		inFlightFilter := filter
		inFlightFilter.Limit = 10000
		inFlightFilter.Offset = 0
		allItems = append(allItems, s.store.List(inFlightFilter).Items...)
	}

	if !isActiveOnly {
		runFilter := RunFilter{
			Domain:  filter.Domain,
			BatchID: filter.BatchID,
			Limit:   10000,
			Offset:  0,
		}
		if isTerminalOnly {
			runFilter.Status = filter.Status
		}
		for _, run := range s.store.ListRuns(runFilter).Items {
			allItems = append(allItems, jobFromRun(run))
		}
	}

	sortJobSlice(allItems, filter.Sort)

	total := len(allItems)
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	start := offset
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	pageItems := make([]Job, end-start)
	copy(pageItems, allItems[start:end])

	list := JobList{
		Items:  pageItems,
		Total:  total,
		Limit:  limit,
		Offset: offset,
		Sort:   string(normalizeJobSort(filter.Sort)),
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
	writeJSON(w, http.StatusOK, list)
}

func parseListFilter(r *http.Request, defaultLimit int) (JobFilter, string, string) {
	filter := JobFilter{
		Limit: defaultLimit,
		Sort:  JobSortStartedAtDesc,
	}
	query := r.URL.Query()

	if status := strings.TrimSpace(query.Get("status")); status != "" {
		filter.Status = JobStatus(status)
	}
	filter.BatchID = strings.TrimSpace(query.Get("batch_id"))
	filter.Domain = strings.TrimSpace(query.Get("domain"))
	if rawSeverity := strings.TrimSpace(query.Get("severity")); rawSeverity != "" {
		severity := JobSeverityFilter(rawSeverity)
		if !isValidJobSeverityFilter(severity) {
			return JobFilter{}, "invalid_severity", "severity must be one of warnings_plus, errors_only"
		}
		filter.Severity = severity
	}

	if createdAfter := strings.TrimSpace(query.Get("created_after")); createdAfter != "" {
		timestamp, err := parseTime(createdAfter)
		if err != nil {
			return JobFilter{}, "invalid_created_after", "created_after must be RFC3339"
		}
		filter.CreatedAfter = timestamp
	}
	if createdBefore := strings.TrimSpace(query.Get("created_before")); createdBefore != "" {
		timestamp, err := parseTime(createdBefore)
		if err != nil {
			return JobFilter{}, "invalid_created_before", "created_before must be RFC3339"
		}
		filter.CreatedBefore = timestamp
	}
	if !filter.CreatedAfter.IsZero() && !filter.CreatedBefore.IsZero() && filter.CreatedBefore.Before(filter.CreatedAfter) {
		return JobFilter{}, "invalid_time_range", "created_before must be greater than or equal to created_after"
	}

	if rawSort := strings.TrimSpace(query.Get("sort")); rawSort != "" {
		sortValue := JobSort(rawSort)
		if !isValidJobSort(sortValue) {
			return JobFilter{}, "invalid_sort", "sort must be one of created_at_desc, created_at_asc, started_at_desc, started_at_asc, domain_asc, domain_desc, batch_id_asc, batch_id_desc, error_desc, critical_desc"
		}
		filter.Sort = sortValue
	}

	if limitRaw := strings.TrimSpace(query.Get("limit")); limitRaw != "" {
		limitValue, err := strconv.Atoi(limitRaw)
		if err != nil || limitValue <= 0 || limitValue > maxListLimit {
			return JobFilter{}, "invalid_limit", "limit must be between 1 and 500"
		}
		filter.Limit = limitValue
	} else if pageSizeRaw := strings.TrimSpace(query.Get("page_size")); pageSizeRaw != "" {
		limitValue, err := strconv.Atoi(pageSizeRaw)
		if err != nil || limitValue <= 0 || limitValue > maxListLimit {
			return JobFilter{}, "invalid_page_size", "page_size must be between 1 and 500"
		}
		filter.Limit = limitValue
	}

	if cursorRaw := strings.TrimSpace(query.Get("cursor")); cursorRaw != "" {
		offset, err := strconv.Atoi(cursorRaw)
		if err != nil || offset < 0 {
			return JobFilter{}, "invalid_cursor", "cursor must be a non-negative integer offset"
		}
		filter.Offset = offset
		return filter, "", ""
	}

	if pageRaw := strings.TrimSpace(query.Get("page")); pageRaw != "" {
		pageNumber, err := strconv.Atoi(pageRaw)
		if err != nil || pageNumber <= 0 {
			return JobFilter{}, "invalid_page", "page must be a positive integer"
		}
		filter.Offset = (pageNumber - 1) * filter.Limit
		return filter, "", ""
	}

	if offsetRaw := strings.TrimSpace(query.Get("offset")); offsetRaw != "" {
		offset, err := strconv.Atoi(offsetRaw)
		if err != nil || offset < 0 {
			return JobFilter{}, "invalid_offset", "offset must be a non-negative integer"
		}
		filter.Offset = offset
	}

	return filter, "", ""
}

func (s *Server) handleGetJob(w http.ResponseWriter, _ *http.Request, jobID string) {
	job, ok := s.store.Get(jobID)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "job not found", nil)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleGetJobResult(w http.ResponseWriter, r *http.Request, jobID string) {
	result, ok := s.store.GetResult(jobID)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "job result not found", nil)
		return
	}
	if result.Raw != nil && len(result.Raw.Entries) > 0 {
		locale := strings.TrimSpace(r.URL.Query().Get("locale"))
		if locale == "" {
			locale = "en"
		}
		s.metrics.ObserveResultLocale(locale)
		raw := *result.Raw
		raw.Locale = locale
		raw.Entries = localizeResultEntries(result.Raw.Entries, locale)
		result.Raw = &raw
	}
	if !s.cfg.ShowScoreAdmin {
		result.Score = nil
	}
	if !s.cfg.ShowNameserverTimingsAdmin {
		result.NameserverTimings = nil
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleCancelJob(w http.ResponseWriter, _ *http.Request, jobID string) {
	job, ok := s.store.Get(jobID)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "job not found", nil)
		return
	}
	fromStatus := job.Status
	if job.Status == JobRunning {
		// Just cancel the context; the worker goroutine handles graduation.
		_ = s.cancelJob(jobID)
	}
	if job.Status == JobQueued {
		_ = s.queue.Remove(jobID)
		job.Status = JobCanceled
		job.Error = "canceled"
		job.FinishedAt = time.Now().UTC()
		job.Progress = 100
		if err := s.store.GraduateJob(job, nil); err != nil {
			writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
			return
		}
		s.metrics.ObserveJobStatusTransition(fromStatus, job.Status)
		if !isTerminalMetricsStatus(fromStatus) && isTerminalMetricsStatus(job.Status) {
			duration := time.Duration(-1)
			if !job.StartedAt.IsZero() && !job.FinishedAt.IsZero() {
				duration = job.FinishedAt.Sub(job.StartedAt)
			}
			s.metrics.ObserveJobCompletionWithContext(job.BatchID, job.Domain, job.Status, duration, zeroMetricsSeverityTotals())
		}
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleJobEvents(w http.ResponseWriter, r *http.Request, jobID string) {
	_, ok := s.store.Get(jobID)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "job not found", nil)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	if _, err := fmt.Fprintf(w, "event: state\ndata: {\"status\":\"queued\"}\n\n"); err != nil {
		return
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	<-r.Context().Done()
}

func (s *Server) handleQueuePause(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	if !enforceCSRF(w, r) {
		return
	}
	if err := s.queue.Pause(); err != nil {
		writeError(w, http.StatusInternalServerError, "queue_error", err.Error(), nil)
		return
	}
	s.metrics.ObserveQueuePaused(true)
	writeJSON(w, http.StatusOK, map[string]string{"status": "paused"})
}

func (s *Server) handleQueueResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	if !enforceCSRF(w, r) {
		return
	}
	if err := s.queue.Resume(); err != nil {
		writeError(w, http.StatusInternalServerError, "queue_error", err.Error(), nil)
		return
	}
	s.metrics.ObserveQueuePaused(false)
	writeJSON(w, http.StatusOK, map[string]string{"status": "resumed"})
}

func (s *Server) handleQueueReorder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	if !enforceCSRF(w, r) {
		return
	}
	var req QueueReorderRequest
	if err := readJSON(r, s.cfg.MaxBodySize, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	if err := s.queue.Reorder(req.JobIDs); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_queue", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "reordered"})
}

func (s *Server) handleQueueRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	if !enforceCSRF(w, r) {
		return
	}
	var req QueueRemoveRequest
	if err := readJSON(r, s.cfg.MaxBodySize, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	if len(req.JobIDs) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_queue", "job_ids is required", nil)
		return
	}
	removed := make([]string, 0, len(req.JobIDs))
	for _, jobID := range req.JobIDs {
		jobID = strings.TrimSpace(jobID)
		if jobID == "" {
			writeError(w, http.StatusBadRequest, "invalid_queue", "job_id is required", nil)
			return
		}
		if err := s.queue.Remove(jobID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_queue", err.Error(), nil)
			return
		}
		if job, ok := s.store.Get(jobID); ok {
			if job.Status == JobQueued {
				fromStatus := job.Status
				job.Status = JobCanceled
				job.Error = "removed_from_queue"
				job.FinishedAt = time.Now().UTC()
				job.Progress = 100
				if err := s.store.GraduateJob(job, nil); err != nil {
					writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
					return
				}
				s.metrics.ObserveJobStatusTransition(fromStatus, job.Status)
				if !isTerminalMetricsStatus(fromStatus) && isTerminalMetricsStatus(job.Status) {
					duration := time.Duration(-1)
					if !job.StartedAt.IsZero() && !job.FinishedAt.IsZero() {
						duration = job.FinishedAt.Sub(job.StartedAt)
					}
					s.metrics.ObserveJobCompletionWithContext(job.BatchID, job.Domain, job.Status, duration, zeroMetricsSeverityTotals())
				}
			}
		}
		removed = append(removed, jobID)
	}
	writeJSON(w, http.StatusOK, QueueRemoveResponse{Removed: removed})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	options, code, message := parseMetricsQueryOptions(r.URL.Query())
	if code != "" {
		writeError(w, http.StatusBadRequest, code, message, nil)
		return
	}

	now := s.metricsNow()
	cacheKey := options.cacheKey()
	if payload, ok := s.getMetricsCache(cacheKey, now); ok {
		writeRawMetrics(w, http.StatusOK, options.format, payload)
		return
	}

	payload, err := s.buildMetricsResponseBody(options)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "metrics_error", err.Error(), nil)
		return
	}
	s.putMetricsCache(cacheKey, payload, now)
	writeRawMetrics(w, http.StatusOK, options.format, payload)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleLocales(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string][]string{"locales": i18n.AvailableLocales()})
}

func (s *Server) handleJobsPurge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}

	var req struct {
		OlderThanDays int `json:"older_than_days"`
	}
	// Body is optional; ignore EOF (no body sent).
	_ = readJSON(r, s.cfg.MaxBodySize, &req)

	days := req.OlderThanDays
	if days == 0 {
		days = s.cfg.Database.RetentionDays
	}
	if days == 0 {
		writeError(w, http.StatusBadRequest, "retention_not_configured",
			"retention_days not configured and older_than_days not specified", nil)
		return
	}

	cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	n, err := s.store.PurgeOlderThan(cutoff)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "purge_error", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"purged_jobs": n})
}

var sitemapLangs = []string{"da", "en", "es", "fi", "fr", "ja", "nb", "sl", "sv"}

func resolvePublicURL(configured string, r *http.Request) string {
	if configured != "" {
		return configured
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto == "https" || proto == "http" {
		scheme = proto
	}
	host := r.Host
	if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		host = fwdHost
	}
	return scheme + "://" + host + "/"
}

func (s *Server) handleRobotsTxt(w http.ResponseWriter, r *http.Request) {
	base := strings.TrimRight(resolvePublicURL(s.cfg.PublicURL, r), "/")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	fmt.Fprintf(w, "User-agent: *\nSitemap: %s/sitemap.xml\n", base)
}

func (s *Server) handleSitemap(w http.ResponseWriter, r *http.Request) {
	publicURL := resolvePublicURL(s.cfg.PublicURL, r)
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	b.WriteString("<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\"\n")
	b.WriteString("        xmlns:xhtml=\"http://www.w3.org/1999/xhtml\">\n")
	b.WriteString("  <url>\n")
	fmt.Fprintf(&b, "    <loc>%s</loc>\n", publicURL)
	for _, lang := range sitemapLangs {
		fmt.Fprintf(&b, "    <xhtml:link rel=\"alternate\" hreflang=\"%s\" href=\"%s\"/>\n", lang, publicURL)
	}
	fmt.Fprintf(&b, "    <xhtml:link rel=\"alternate\" hreflang=\"x-default\" href=\"%s\"/>\n", publicURL)
	b.WriteString("  </url>\n")
	b.WriteString("</urlset>\n")
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	fmt.Fprint(w, b.String())
}
