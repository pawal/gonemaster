package server

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/normalization"
)

// MaxPublicTests caps how many testcase IDs a single public create request
// may filter on. The engine itself has fewer than 100 testcases; 256 is
// generous, anything more is a payload-DoS attempt.
const MaxPublicTests = 256

// PublicJobView is the restricted job representation returned by the public API.
// The internal UUID (ID) is intentionally omitted.
type PublicJobView struct {
	PublicID   string     `json:"public_id"`
	Domain     string     `json:"domain"`
	Status     JobStatus  `json:"status"`
	Progress   int        `json:"progress"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

type PublicProfileView struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
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

func publicProfileView(profile StoredProfile) PublicProfileView {
	return PublicProfileView{
		ID:          profile.ID,
		Name:        profile.Name,
		Description: profile.Description,
	}
}

// publicInfoResponse holds public-facing server feature flags.
type publicInfoResponse struct {
	ShowScorePublic             bool `json:"show_score_public"`
	ShowNameserverTimingsPublic bool `json:"show_nameserver_timings_public"`
	ShowDNSSECChainPublic       bool `json:"show_dnssec_chain_public"`
}

// handlePublicInfo handles GET /pub/api/v1/info.
// Returns feature flags the public UI reads on startup to decide which UI
// components to display. On fetch failure, the public UI defaults to hiding
// scoring (fail-safe).
func (s *Server) handlePublicInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, publicInfoResponse{
		ShowScorePublic:             s.cfg.ShowScorePublic,
		ShowNameserverTimingsPublic: s.cfg.ShowNameserverTimingsPublic,
		ShowDNSSECChainPublic:       s.cfg.ShowDNSSECChainPublic,
	})
}

// handlePublicVersion handles GET /pub/api/v1/version.
func (s *Server) handlePublicVersion(w http.ResponseWriter, r *http.Request) {
	resp := map[string]string{"gonemaster": engine.VersionFull()}
	if dns := engine.DNSLibVersion(); dns != "" {
		resp["dns"] = dns
	}
	w.Header().Set("Cache-Control", "public, max-age=3600")
	writeJSON(w, http.StatusOK, resp)
}

// handlePublicProfiles handles GET /pub/api/v1/profiles.
// Only public profiles are returned, without exposing config.
func (s *Server) handlePublicProfiles(w http.ResponseWriter, r *http.Request) {
	storedProfiles := s.store.ListProfiles()
	views := make([]PublicProfileView, 0, len(storedProfiles))
	for _, stored := range storedProfiles {
		if !stored.Public {
			continue
		}
		views = append(views, publicProfileView(stored))
	}
	writeJSON(w, http.StatusOK, views)
}

// handlePublicCreateJob handles POST /pub/api/v1/jobs.
// Accepts the same body as the internal create endpoint but returns a
// PublicJobView - the internal UUID is never sent to the caller.
func (s *Server) handlePublicCreateJob(w http.ResponseWriter, r *http.Request) {
	if !s.enforceCSRF(w, r) {
		return
	}
	var req JobCreateRequest
	if err := readJSON(r, s.cfg.MaxBodySize, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	if strings.TrimSpace(req.Domain) == "" {
		writeError(w, http.StatusBadRequest, "missing_domain", "domain is required", nil)
		return
	}
	if len(req.Tests) > MaxPublicTests {
		writeError(w, http.StatusBadRequest, "too_many_tests",
			fmt.Sprintf("tests list too long: %d (max %d)", len(req.Tests), MaxPublicTests), nil)
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
	if !s.cfg.PublicAPI.AllowPrivateUndelegatedIP {
		for i, ns := range undelegatedNS {
			if blocked, reason := isBlockedPublicNameserverIP(ns.IP); blocked {
				writeError(w, http.StatusBadRequest, "private_undelegated_ip",
					fmt.Sprintf("undelegated nameserver[%d]: %s IP %q is not allowed on the public API", i, reason, ns.IP), nil)
				return
			}
		}
	}
	if len(req.ProfileOverrides) > 0 {
		writeError(w, http.StatusBadRequest, "profile_overrides_not_allowed", "public API requests must use profile_id instead of profile_overrides", nil)
		return
	}
	resolvedProfile, code, message := s.resolveStoredProfile(req.ProfileID, true)
	if code != "" {
		writeError(w, http.StatusBadRequest, code, message, nil)
		return
	}

	job := Job{
		ID:            newID("job"),
		Domain:        domain,
		Tests:         req.Tests,
		UndelegatedNS: undelegatedNS,
		UndelegatedDS: undelegatedDS,
		MinLevel:      req.MinLevel,
		Status:        JobQueued,
		CreatedAt:     time.Now().UTC(),
		Progress:      0,
		ProfileID:     cloneInt64Ptr(resolvedProfile.ID),
		ProfileName:   resolvedProfile.Name,
		IPv4Disabled:  req.IPv4Disabled,
		IPv6Disabled:  req.IPv6Disabled,
		Origin:        JobOriginPublic,
	}
	created, err := s.store.Create(job)
	if err != nil {
		log.Printf("public create job: store error: %v", err)
		writeError(w, http.StatusInternalServerError, "store_error", "job submission failed", nil)
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
		locale := resolveResultLocale(r.URL.Query().Get("locale"))
		s.metrics.ObserveResultLocale(locale)
		raw := *result.Raw
		raw.Locale = locale
		raw.Entries = localizeResultEntries(result.Raw.Entries, locale)
		result.Raw = &raw
		result.TestcaseDescriptions = testcaseDescriptionsForEntries(raw.Entries)
	}
	if !s.cfg.ShowScorePublic {
		result.Score = nil
	}
	if !s.cfg.ShowNameserverTimingsPublic {
		result.NameserverTimings = nil
	}
	if !s.cfg.ShowDNSSECChainPublic {
		result.HasDNSSECChain = false
	}
	// Let a CDN absorb repeat reads; short window so show_* flips propagate.
	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, http.StatusOK, result)
}

// handlePublicGetDNSSECChain handles GET /pub/api/v1/jobs/{publicID}/dnssec-chain.
// Flag-off and unknown id answer 404 not_found; a run without a blob answers
// 404 no_chain_data with no cache header so a re-run is not masked.
func (s *Server) handlePublicGetDNSSECChain(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.ShowDNSSECChainPublic {
		writeError(w, http.StatusNotFound, "not_found", "not found", nil)
		return
	}
	publicID := r.PathValue("publicID")
	job, ok := s.store.GetByPublicID(publicID)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "not found", nil)
		return
	}
	chain, ok := s.store.GetRunDNSSECChain(job.ID)
	if !ok {
		writeError(w, http.StatusNotFound, "no_chain_data", "no DNSSEC chain data for this run", nil)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write([]byte(chain))
}
