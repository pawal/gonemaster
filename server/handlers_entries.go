package server

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// handleListEntries handles GET /api/v1/entries.
// Accepts: run, domain, tag, module, testcase, entry_tag, level, latest, batch, format, limit, offset.
func (s *Server) handleListEntries(w http.ResponseWriter, r *http.Request) {
	filter := EntryFilter{Limit: 100}
	q := r.URL.Query()
	filter.RunID = strings.TrimSpace(q.Get("run"))
	filter.Tag = strings.TrimSpace(q.Get("tag"))
	filter.Module = strings.TrimSpace(q.Get("module"))
	filter.Testcase = strings.TrimSpace(q.Get("testcase"))
	filter.EntryTag = strings.TrimSpace(q.Get("entry_tag"))
	filter.Level = strings.TrimSpace(q.Get("level"))
	filter.BatchID = strings.TrimSpace(q.Get("batch"))
	if v := strings.TrimSpace(q.Get("latest")); v == "1" || v == "true" {
		filter.LatestOnly = true
	}
	if v := strings.TrimSpace(q.Get("domain")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			writeError(w, http.StatusBadRequest, "invalid_domain", "domain must be a positive integer ID", nil)
			return
		}
		filter.DomainID = id
	}
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

	result := s.store.QueryEntries(filter)

	// Enrich entries with domain names.
	domainNames := map[int64]string{}
	for i, e := range result.Items {
		if e.DomainID == 0 {
			continue
		}
		if name, ok := domainNames[e.DomainID]; ok {
			result.Items[i].Domain = name
			continue
		}
		if d, ok := s.store.GetDomain(e.DomainID); ok {
			domainNames[e.DomainID] = d.Name
			result.Items[i].Domain = d.Name
		}
	}

	if strings.TrimSpace(q.Get("format")) == "csv" {
		writeEntriesCSV(w, result.Items)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// writeEntriesCSV writes entries as a CSV response with columns:
// id, run_id, domain_id, domain, timestamp, module, testcase, tag, level, args.
func writeEntriesCSV(w http.ResponseWriter, entries []Entry) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="entries.csv"`)
	w.WriteHeader(http.StatusOK)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"id", "run_id", "domain_id", "domain", "timestamp", "module", "testcase", "tag", "level", "args"})
	for _, e := range entries {
		args := ""
		if e.Args != nil {
			b, _ := json.Marshal(e.Args)
			args = string(b)
		}
		_ = cw.Write([]string{
			strconv.FormatInt(e.ID, 10),
			e.RunID,
			strconv.FormatInt(e.DomainID, 10),
			e.Domain,
			strconv.FormatFloat(e.Timestamp, 'f', 3, 64),
			e.Module,
			e.Testcase,
			e.Tag,
			e.Level,
			args,
		})
	}
	cw.Flush()
}
