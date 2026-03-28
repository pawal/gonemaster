package server

import (
	"net/http"
	"strings"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/normalization"
)

// PublicJobView is the restricted job representation returned by the public API.
// The internal UUID (ID) is intentionally omitted.
type PublicJobView struct {
	PublicID   string     `json:"public_id"`
	Domain     string     `json:"domain"`
	Status     JobStatus  `json:"status"`
	Progress   int        `json:"progress"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

func publicJobView(job Job) PublicJobView {
	view := PublicJobView{
		PublicID: job.PublicID,
		Domain:   job.Domain,
		Status:   job.Status,
		Progress: job.Progress,
	}
	if !job.FinishedAt.IsZero() {
		view.FinishedAt = &job.FinishedAt
	}
	return view
}

// handlePublicVersion handles GET /pub/api/v1/version.
func (s *Server) handlePublicVersion(w http.ResponseWriter, r *http.Request) {
	resp := map[string]string{"gonemaster": engine.VersionFull()}
	if dns := engine.DNSLibVersion(); dns != "" {
		resp["dns"] = dns
	}
	writeJSON(w, http.StatusOK, resp)
}

// handlePublicCreateJob handles POST /pub/api/v1/jobs.
// Accepts the same body as the internal create endpoint but returns a
// PublicJobView — the internal UUID is never sent to the caller.
func (s *Server) handlePublicCreateJob(w http.ResponseWriter, r *http.Request) {
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
	}
	created, err := s.store.Create(job)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	_ = s.queue.Enqueue(created.ID, PriorityNormal)
	s.metrics.ObserveJobSubmittedWithContext(created.BatchID, created.Domain, JobQueued)

	writeJSON(w, http.StatusCreated, publicJobView(created))
}

// handlePublicGetJob handles GET /pub/api/v1/jobs/{publicID}.
// Returns job status and progress looked up by public ID. UUID is omitted.
func (s *Server) handlePublicGetJob(w http.ResponseWriter, r *http.Request) {
	publicID := r.PathValue("publicID")
	job, ok := s.store.GetByPublicID(publicID)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "job not found", nil)
		return
	}
	writeJSON(w, http.StatusOK, publicJobView(job))
}

// handlePublicGetResult handles GET /pub/api/v1/jobs/{publicID}/result.
// Returns the full result payload looked up by public ID.
func (s *Server) handlePublicGetResult(w http.ResponseWriter, r *http.Request) {
	publicID := r.PathValue("publicID")
	job, ok := s.store.GetByPublicID(publicID)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "job not found", nil)
		return
	}
	result, ok := s.store.GetResult(job.ID)
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
		result.TestcaseDescriptions = testcaseDescriptionsForEntries(raw.Entries)
	}
	writeJSON(w, http.StatusOK, result)
}
