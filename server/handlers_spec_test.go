package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/engine"
)

// TestEveryImplementedTestcaseHasDescription guards against shipping a testcase
// without a short description in testcaseDescriptions (which would surface as an
// empty description in the spec endpoint and the MCP spec_list_testcases tool).
func TestEveryImplementedTestcaseHasDescription(t *testing.T) {
	for _, item := range engine.AvailableTestcases() {
		_, tc, ok := splitTestcaseItem(item)
		if !ok {
			continue
		}
		if testcaseDescriptions[strings.ToUpper(tc)] == "" {
			t.Errorf("testcase %q has no entry in testcaseDescriptions", tc)
		}
	}
}

func getSpec(t *testing.T, srv *Server, path string, out any) *httptest.ResponseRecorder {
	t.Helper()
	resp := doJSON(t, srv, http.MethodGet, path, nil)
	if out != nil && resp.Code == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
	}
	return resp
}

func TestSpecTestcasesList(t *testing.T) {
	srv := newTestServer(t)
	var list SpecTestcaseList
	resp := getSpec(t, srv, "/api/v1/spec/testcases", &list)
	wantStatus(t, resp, http.StatusOK)
	if list.Total == 0 || len(list.Items) != list.Total {
		t.Fatalf("expected non-empty list with consistent total, got total=%d items=%d", list.Total, len(list.Items))
	}
	var basic01 *SpecTestcase
	for i := range list.Items {
		if list.Items[i].ID == "basic01" {
			basic01 = &list.Items[i]
			break
		}
	}
	if basic01 == nil {
		t.Fatalf("expected basic01 in the catalog")
	}
	if basic01.Module != "basic" {
		t.Errorf("basic01 module = %q, want basic", basic01.Module)
	}
	if basic01.Description == "" {
		t.Errorf("basic01 description is empty")
	}
}

func TestSpecTestcasesFilterByCategory(t *testing.T) {
	srv := newTestServer(t)
	var list SpecTestcaseList
	getSpec(t, srv, "/api/v1/spec/testcases?category=dnssec", &list)
	if list.Total == 0 {
		t.Fatalf("expected dnssec testcases")
	}
	for _, it := range list.Items {
		if it.Module != "dnssec" {
			t.Fatalf("category filter leaked module %q", it.Module)
		}
	}
}

func TestSpecTestcaseDetail(t *testing.T) {
	srv := newTestServer(t)
	var detail SpecTestcaseDetail
	resp := getSpec(t, srv, "/api/v1/spec/testcases/basic01", &detail)
	wantStatus(t, resp, http.StatusOK)
	if detail.ID != "basic01" || detail.Module != "basic" {
		t.Errorf("detail id/module wrong: %+v", detail.SpecTestcase)
	}
	if detail.Description == "" {
		t.Errorf("detail description is empty")
	}
	if len(detail.Tags) == 0 {
		t.Fatalf("expected emitted tags for basic01")
	}
	// Tags are sorted and each carries a rendered message in the default locale.
	withMessage := 0
	for _, tg := range detail.Tags {
		if tg.Tag == "" {
			t.Errorf("empty tag in detail")
		}
		if tg.Message != "" {
			withMessage++
		}
	}
	if withMessage == 0 {
		t.Errorf("expected at least one tag with a rendered message")
	}
}

func TestSpecTestcaseCaseInsensitive(t *testing.T) {
	srv := newTestServer(t)
	var detail SpecTestcaseDetail
	resp := getSpec(t, srv, "/api/v1/spec/testcases/BASIC01", &detail)
	wantStatus(t, resp, http.StatusOK)
	if detail.ID != "basic01" {
		t.Errorf("id = %q, want basic01", detail.ID)
	}
}

func TestSpecTestcaseNotFound(t *testing.T) {
	srv := newTestServer(t)
	resp := getSpec(t, srv, "/api/v1/spec/testcases/nope99", nil)
	wantStatus(t, resp, http.StatusNotFound)
}
