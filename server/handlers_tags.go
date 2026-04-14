package server

import (
	"net/http"
	"strconv"
	"strings"
)

// handleTags routes POST and GET on /api/v1/tags.
func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		if !enforceCSRF(w, r) {
			return
		}
		s.handleCreateTag(w, r)
	case http.MethodGet:
		s.handleListTags(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

// handleCreateTag handles POST /api/v1/tags.
func (s *Server) handleCreateTag(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := readJSON(r, s.cfg.MaxBodySize, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "missing_name", "name is required", nil)
		return
	}
	if err := s.store.CreateTag(name, strings.TrimSpace(req.Description)); err != nil {
		writeError(w, http.StatusConflict, "tag_exists", err.Error(), nil)
		return
	}
	tag, _ := s.store.GetTag(name)
	writeJSON(w, http.StatusCreated, tag)
}

// handleListTags handles GET /api/v1/tags.
func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
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
	writeJSON(w, http.StatusOK, s.store.ListTags(limit, offset))
}

// handleTagByName routes PUT, DELETE on /api/v1/tags/{name}.
func (s *Server) handleTagByName(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusNotFound, "not_found", "tag not found", nil)
		return
	}
	switch r.Method {
	case http.MethodPut:
		if !enforceCSRF(w, r) {
			return
		}
		s.handleUpdateTag(w, r, name)
	case http.MethodDelete:
		if !enforceCSRF(w, r) {
			return
		}
		s.handleDeleteTag(w, r, name)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

// handleUpdateTag handles PUT /api/v1/tags/{name}.
func (s *Server) handleUpdateTag(w http.ResponseWriter, r *http.Request, name string) {
	var req struct {
		Description string `json:"description"`
	}
	if err := readJSON(r, s.cfg.MaxBodySize, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	if err := s.store.UpdateTag(name, strings.TrimSpace(req.Description)); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "tag not found", nil)
		return
	}
	tag, _ := s.store.GetTag(name)
	writeJSON(w, http.StatusOK, tag)
}

// handleDeleteTag handles DELETE /api/v1/tags/{name}.
func (s *Server) handleDeleteTag(w http.ResponseWriter, r *http.Request, name string) {
	if _, ok := s.store.GetTag(name); !ok {
		writeError(w, http.StatusNotFound, "not_found", "tag not found", nil)
		return
	}
	if err := s.store.DeleteTag(name); err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleTagDomains routes POST and DELETE on /api/v1/tags/{name}/domains.
func (s *Server) handleTagDomains(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, ok := s.store.GetTag(name); !ok {
		writeError(w, http.StatusNotFound, "not_found", "tag not found", nil)
		return
	}
	switch r.Method {
	case http.MethodPost:
		if !enforceCSRF(w, r) {
			return
		}
		s.handleAddTagDomains(w, r, name)
	case http.MethodDelete:
		if !enforceCSRF(w, r) {
			return
		}
		s.handleRemoveTagDomains(w, r, name)
	case http.MethodGet:
		s.handleListTagDomains(w, r, name)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

// handleAddTagDomains handles POST /api/v1/tags/{name}/domains.
// Body: {"domains": ["example.com", ...]}
func (s *Server) handleAddTagDomains(w http.ResponseWriter, r *http.Request, tag string) {
	domainIDs, ok := resolveDomainNames(w, r, s)
	if !ok {
		return
	}
	if err := s.store.TagDomains(tag, domainIDs); err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRemoveTagDomains handles DELETE /api/v1/tags/{name}/domains.
// Body: {"domains": ["example.com", ...]}
func (s *Server) handleRemoveTagDomains(w http.ResponseWriter, r *http.Request, tag string) {
	domainIDs, ok := resolveDomainNames(w, r, s)
	if !ok {
		return
	}
	if err := s.store.UntagDomains(tag, domainIDs); err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListTagDomains handles GET /api/v1/tags/{name}/domains.
func (s *Server) handleListTagDomains(w http.ResponseWriter, r *http.Request, tag string) {
	filter := DomainFilter{Limit: 100, Tag: tag}
	q := r.URL.Query()
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
	filter.MinLevel = strings.ToUpper(strings.TrimSpace(q.Get("min_level")))
	filter.Sort = DomainSort(strings.TrimSpace(q.Get("sort")))
	list := s.store.ListDomainsByTag(tag, filter)
	if !s.cfg.ShowScoreAdmin {
		for i := range list.Items {
			list.Items[i].LatestScore = nil
			list.Items[i].LatestGrade = nil
		}
	}
	writeJSON(w, http.StatusOK, list)
}

// handleTagSummary handles GET /api/v1/tags/{name}/summary.
func (s *Server) handleTagSummary(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	summary, ok := s.store.GetTagSummary(name)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "tag not found", nil)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// resolveDomainNames reads {"domains": [...]} from the request body and
// resolves each name to its domain ID, creating the domain if it doesn't exist.
func resolveDomainNames(w http.ResponseWriter, r *http.Request, s *Server) ([]int64, bool) {
	var req struct {
		Domains []string `json:"domains"`
	}
	if err := readJSON(r, s.cfg.MaxBodySize, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return nil, false
	}
	if len(req.Domains) == 0 {
		writeError(w, http.StatusBadRequest, "missing_domains", "domains is required", nil)
		return nil, false
	}
	ids := make([]int64, 0, len(req.Domains))
	for _, name := range req.Domains {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		d, err := s.store.GetOrCreateDomain(name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
			return nil, false
		}
		ids = append(ids, d.ID)
	}
	return ids, true
}
