package server

import (
	"net/http"
	"strconv"
	"strings"
)

// handleListDomains handles GET /api/v1/domains.
// Accepts query params: tag, name, level, limit, offset.
func (s *Server) handleListDomains(w http.ResponseWriter, r *http.Request) {
	filter := DomainFilter{Limit: 100}
	q := r.URL.Query()
	filter.Tag = strings.TrimSpace(q.Get("tag"))
	filter.Name = strings.TrimSpace(q.Get("name"))
	filter.LatestLevel = strings.TrimSpace(q.Get("level"))
	filter.MinLevel = strings.TrimSpace(q.Get("min_level"))

	if limitRaw := strings.TrimSpace(q.Get("limit")); limitRaw != "" {
		v, err := strconv.Atoi(limitRaw)
		if err != nil || v <= 0 || v > maxListLimit {
			writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be between 1 and 500", nil)
			return
		}
		filter.Limit = v
	}
	if offsetRaw := strings.TrimSpace(q.Get("offset")); offsetRaw != "" {
		v, err := strconv.Atoi(offsetRaw)
		if err != nil || v < 0 {
			writeError(w, http.StatusBadRequest, "invalid_offset", "offset must be a non-negative integer", nil)
			return
		}
		filter.Offset = v
	}

	writeJSON(w, http.StatusOK, s.store.ListDomains(filter))
}

// handleGetDomain handles GET /api/v1/domains/{id}.
// Returns the domain record with its tags populated.
func (s *Server) handleGetDomain(w http.ResponseWriter, r *http.Request) {
	id, ok := parseDomainID(w, r)
	if !ok {
		return
	}
	domain, ok := s.store.GetDomain(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "domain not found", nil)
		return
	}
	domain.Tags = s.store.GetDomainTags(id)
	writeJSON(w, http.StatusOK, domain)
}

// handleGetDomainRuns handles GET /api/v1/domains/{id}/runs.
// Returns paginated run history for the domain.
func (s *Server) handleGetDomainRuns(w http.ResponseWriter, r *http.Request) {
	id, ok := parseDomainID(w, r)
	if !ok {
		return
	}
	if _, ok := s.store.GetDomain(id); !ok {
		writeError(w, http.StatusNotFound, "not_found", "domain not found", nil)
		return
	}

	limit := 100
	offset := 0
	q := r.URL.Query()
	if limitRaw := strings.TrimSpace(q.Get("limit")); limitRaw != "" {
		v, err := strconv.Atoi(limitRaw)
		if err != nil || v <= 0 || v > maxListLimit {
			writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be between 1 and 500", nil)
			return
		}
		limit = v
	}
	if offsetRaw := strings.TrimSpace(q.Get("offset")); offsetRaw != "" {
		v, err := strconv.Atoi(offsetRaw)
		if err != nil || v < 0 {
			writeError(w, http.StatusBadRequest, "invalid_offset", "offset must be a non-negative integer", nil)
			return
		}
		offset = v
	}

	writeJSON(w, http.StatusOK, s.store.ListRunsByDomain(id, limit, offset))
}

// parseDomainID extracts and validates the {id} path value as a positive int64.
// Writes a 404 and returns false on failure.
func parseDomainID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusNotFound, "not_found", "domain not found", nil)
		return 0, false
	}
	return id, true
}
