package server

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode"
)

const (
	asnNamesMax      = 32
	asnNamesParallel = 8
	asnNamesWait     = 3 * time.Second
	asnLookupTimeout = 10 * time.Second
	asnLabelMaxRunes = 255
)

// asnLabeler resolves the registry label of an AS number.
type asnLabeler interface {
	EnrichASNLabel(ctx context.Context, asn int64) (string, bool)
}

// SetASNLabeler wires the AS holder lookup behind the public ASN names endpoint.
func (s *Server) SetASNLabeler(l asnLabeler) {
	s.asnLabeler = l
}

func (s *Server) asnNamesEnabled() bool {
	return s.cfg.ShowASNNamesPublic && s.asnLabeler != nil
}

type asnName struct {
	ASN     int64  `json:"asn"`
	Name    string `json:"name"`
	Handle  string `json:"handle"`
	Country string `json:"country"`
	Label   string `json:"label"`
}

type asnNamesResponse struct {
	Complete bool      `json:"complete"`
	ASNs     []asnName `json:"asns"`
}

// handlePublicGetASNNames handles GET /pub/api/v1/jobs/{publicID}/asn-names.
func (s *Server) handlePublicGetASNNames(w http.ResponseWriter, r *http.Request) {
	if !s.asnNamesEnabled() {
		writeError(w, http.StatusNotFound, "not_found", "not found", nil)
		return
	}
	job, ok := s.store.GetByPublicID(r.PathValue("publicID"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "not found", nil)
		return
	}
	result, ok := s.store.GetResult(job.ID)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "not found", nil)
		return
	}
	var entries []JobResultEntry
	if result.Raw != nil {
		entries = result.Raw.Entries
	}
	names, complete := lookupASNNames(r.Context(), s.asnLabeler, collectASNs(entries))
	if complete {
		w.Header().Set("Cache-Control", "public, max-age=86400")
	} else {
		w.Header().Set("Cache-Control", "no-store")
	}
	writeJSON(w, http.StatusOK, asnNamesResponse{Complete: complete, ASNs: names})
}

// lookupASNNames detaches each lookup from ctx so a late answer still fills the cache.
func lookupASNNames(ctx context.Context, labeler asnLabeler, asns []int64) ([]asnName, bool) {
	type answer struct {
		i     int
		label string
	}
	answers := make(chan answer, len(asns))
	detached := context.WithoutCancel(ctx)
	go func() {
		sem := make(chan struct{}, asnNamesParallel)
		for i, asn := range asns {
			sem <- struct{}{}
			go func() {
				defer func() { <-sem }()
				lctx, cancel := context.WithTimeout(detached, asnLookupTimeout)
				defer cancel()
				label, _ := labeler.EnrichASNLabel(lctx, asn)
				answers <- answer{i: i, label: label}
			}()
		}
	}()

	labels := make([]string, len(asns))
	timer := time.NewTimer(asnNamesWait)
	defer timer.Stop()
	complete := true
wait:
	for range asns {
		select {
		case a := <-answers:
			labels[a.i] = a.label
		case <-timer.C:
			complete = false
			break wait
		case <-ctx.Done():
			complete = false
			break wait
		}
	}

	names := []asnName{}
	for i, asn := range asns {
		label := cleanASLabel(labels[i])
		handle, name, country := parseASLabel(label)
		if name == "" {
			continue
		}
		names = append(names, asnName{ASN: asn, Name: name, Handle: handle, Country: country, Label: label})
	}
	return names, complete
}

// collectASNs returns the distinct AS numbers in the asn and asns args, ascending, capped.
func collectASNs(entries []JobResultEntry) []int64 {
	seen := map[int64]struct{}{}
	for _, e := range entries {
		for _, key := range []string{"asn", "asns"} {
			for _, n := range asnArgValues(e.Args[key]) {
				seen[n] = struct{}{}
			}
		}
	}
	out := make([]int64, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	slices.Sort(out)
	if len(out) > asnNamesMax {
		out = out[:asnNamesMax]
	}
	return out
}

func asnArgValues(v any) []int64 {
	var out []int64
	add := func(x any) {
		if n, ok := asnValue(x); ok {
			out = append(out, n)
		}
	}
	switch v := v.(type) {
	case []any:
		for _, x := range v {
			add(x)
		}
	case []int:
		for _, x := range v {
			add(x)
		}
	case []int64:
		for _, x := range v {
			add(x)
		}
	default:
		add(v)
	}
	return out
}

func asnValue(v any) (int64, bool) {
	var n int64
	switch v := v.(type) {
	case int:
		n = int64(v)
	case int64:
		n = v
	case float64:
		if v != math.Trunc(v) || v < 1 || v > math.MaxUint32 {
			return 0, false
		}
		n = int64(v)
	case json.Number:
		i, err := v.Int64()
		if err != nil {
			return 0, false
		}
		n = i
	default:
		return 0, false
	}
	if n < 1 || n > math.MaxUint32 {
		return 0, false
	}
	return n, true
}

// cleanASLabel trims, drops control characters and caps the length.
func cleanASLabel(raw string) string {
	s := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(raw))
	if r := []rune(s); len(r) > asnLabelMaxRunes {
		s = string(r[:asnLabelMaxRunes])
	}
	return strings.TrimSpace(s)
}

// parseASLabel splits "<HANDLE> - <Name>, <CC>"; handle and country are optional.
func parseASLabel(label string) (handle, name, country string) {
	s := cleanASLabel(label)
	if n := len(s); n >= 4 && s[n-4:n-2] == ", " && isUpperASCII(s[n-2]) && isUpperASCII(s[n-1]) {
		country = s[n-2:]
		s = s[:n-4]
	}
	name = s
	if left, right, ok := strings.Cut(s, " - "); ok && left != "" && !strings.ContainsFunc(left, unicode.IsSpace) {
		handle, name = left, right
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = handle
	}
	return handle, name, country
}

func isUpperASCII(b byte) bool { return b >= 'A' && b <= 'Z' }
