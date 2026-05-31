package server

import (
	"net/http"
	"sort"
	"strings"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/i18n"
	addresspkg "codeberg.org/pawal/gonemaster/engine/test/address"
	basicpkg "codeberg.org/pawal/gonemaster/engine/test/basic"
	connectivitypkg "codeberg.org/pawal/gonemaster/engine/test/connectivity"
	consistencypkg "codeberg.org/pawal/gonemaster/engine/test/consistency"
	delegationpkg "codeberg.org/pawal/gonemaster/engine/test/delegation"
	dnssecpkg "codeberg.org/pawal/gonemaster/engine/test/dnssec"
	nameserverpkg "codeberg.org/pawal/gonemaster/engine/test/nameserver"
	syntaxpkg "codeberg.org/pawal/gonemaster/engine/test/syntax"
	zonepkg "codeberg.org/pawal/gonemaster/engine/test/zone"
)

// SpecTestcase is one testcase in the catalog.
type SpecTestcase struct {
	ID          string `json:"id"`
	Module      string `json:"module"`
	Description string `json:"description,omitempty"`
}

// SpecTestcaseList is the GET /api/v1/spec/testcases response.
type SpecTestcaseList struct {
	Items []SpecTestcase `json:"items"`
	Total int            `json:"total"`
}

// SpecTag is a tag a testcase can emit, with its rendered message.
type SpecTag struct {
	Tag     string `json:"tag"`
	Message string `json:"message,omitempty"`
}

// SpecTestcaseDetail is the GET /api/v1/spec/testcases/{id} response.
type SpecTestcaseDetail struct {
	SpecTestcase
	Locale string    `json:"locale"`
	Tags   []SpecTag `json:"tags"`
}

// knownTagsByModule mirrors tools/specifications/internal/specdata.KnownTagsByModule;
// that package is internal to tools/ and cannot be imported here.
func knownTagsByModule() map[string]map[string][]string {
	return map[string]map[string][]string{
		"basic":        basicpkg.Metadata(),
		"address":      addresspkg.AddressMetadata(),
		"connectivity": connectivitypkg.Metadata(),
		"consistency":  consistencypkg.Metadata(),
		"delegation":   delegationpkg.Metadata(),
		"dnssec":       dnssecpkg.Metadata(),
		"nameserver":   nameserverpkg.Metadata(),
		"syntax":       syntaxpkg.Metadata(),
		"zone":         zonepkg.Metadata(),
	}
}

// splitTestcaseItem parses an "MODULE:testcase" item from engine.AvailableTestcases.
func splitTestcaseItem(item string) (module, testcase string, ok bool) {
	parts := strings.SplitN(item, ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	module = strings.ToLower(strings.TrimSpace(parts[0]))
	testcase = strings.ToLower(strings.TrimSpace(parts[1]))
	return module, testcase, module != "" && testcase != ""
}

// handleSpecTestcases handles GET /api/v1/spec/testcases.
// Optional category/module query param filters to one module.
func (s *Server) handleSpecTestcases(w http.ResponseWriter, r *http.Request) {
	module := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("category")))
	if module == "" {
		module = strings.ToLower(strings.TrimSpace(r.URL.Query().Get("module")))
	}
	items := []SpecTestcase{}
	for _, item := range engine.AvailableTestcases() {
		m, tc, ok := splitTestcaseItem(item)
		if !ok || (module != "" && m != module) {
			continue
		}
		items = append(items, SpecTestcase{ID: tc, Module: m, Description: testcaseDescriptions[strings.ToUpper(tc)]})
	}
	writeJSON(w, http.StatusOK, SpecTestcaseList{Items: items, Total: len(items)})
}

// handleSpecTestcase handles GET /api/v1/spec/testcases/{id}.
func (s *Server) handleSpecTestcase(w http.ResponseWriter, r *http.Request) {
	id := strings.ToLower(strings.TrimSpace(r.PathValue("id")))
	module := ""
	for _, item := range engine.AvailableTestcases() {
		m, tc, ok := splitTestcaseItem(item)
		if ok && tc == id {
			module = m
			break
		}
	}
	if module == "" {
		writeError(w, http.StatusNotFound, "not_found", "testcase not found", nil)
		return
	}
	locale := resolveResultLocale(r.URL.Query().Get("locale"))
	detail := SpecTestcaseDetail{
		SpecTestcase: SpecTestcase{ID: id, Module: module, Description: testcaseDescriptions[strings.ToUpper(id)]},
		Locale:       locale,
		Tags:         []SpecTag{},
	}
	tags := append([]string(nil), knownTagsByModule()[module][id]...)
	sort.Strings(tags)
	for _, tag := range tags {
		detail.Tags = append(detail.Tags, SpecTag{Tag: tag, Message: i18n.Translate(locale, module, tag, nil)})
	}
	writeJSON(w, http.StatusOK, detail)
}
