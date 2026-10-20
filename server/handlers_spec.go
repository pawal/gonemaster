package server

import (
	"net/http"
	"strings"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/i18n"
)

// SpecTestcase is one testcase in the catalog.
type SpecTestcase struct {
	ID          string `json:"id"`
	Module      string `json:"module"`
	Description string `json:"description,omitempty"`
	// Excluded is true when the instance never runs the testcase.
	Excluded bool `json:"excluded,omitempty"`
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

// handleSpecTestcases handles GET /api/v1/spec/testcases.
// Optional category/module query param filters to one module.
func (s *Server) handleSpecTestcases(w http.ResponseWriter, r *http.Request) {
	module := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("category")))
	if module == "" {
		module = strings.ToLower(strings.TrimSpace(r.URL.Query().Get("module")))
	}
	items := []SpecTestcase{}
	for _, m := range engine.Modules() {
		if module != "" && m.Name != module {
			continue
		}
		for _, tc := range m.Testcases {
			items = append(items, SpecTestcase{ID: tc.ID, Module: m.Name, Description: testcaseDescriptions[strings.ToUpper(tc.ID)], Excluded: s.isExcluded(tc.ID)})
		}
	}
	writeJSON(w, http.StatusOK, SpecTestcaseList{Items: items, Total: len(items)})
}

// handleSpecTestcase handles GET /api/v1/spec/testcases/{id}.
func (s *Server) handleSpecTestcase(w http.ResponseWriter, r *http.Request) {
	id := strings.ToLower(strings.TrimSpace(r.PathValue("id")))
	module := ""
	var tags []string
	for _, m := range engine.Modules() {
		for _, tc := range m.Testcases {
			if tc.ID == id {
				module, tags = m.Name, tc.Tags
			}
		}
	}
	if module == "" {
		writeError(w, http.StatusNotFound, "not_found", "testcase not found", nil)
		return
	}
	locale := resolveResultLocale(r.URL.Query().Get("locale"))
	detail := SpecTestcaseDetail{
		SpecTestcase: SpecTestcase{ID: id, Module: module, Description: testcaseDescriptions[strings.ToUpper(id)], Excluded: s.isExcluded(id)},
		Locale:       locale,
		Tags:         []SpecTag{},
	}
	for _, tag := range tags {
		detail.Tags = append(detail.Tags, SpecTag{Tag: tag, Message: i18n.Translate(locale, module, tag, nil)})
	}
	writeJSON(w, http.StatusOK, detail)
}
