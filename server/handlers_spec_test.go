package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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

// TestEveryImplementedTestcaseHasUITitle pins a title per testcase in every UI catalog.
func TestEveryImplementedTestcaseHasUITitle(t *testing.T) {
	var ids []string
	for _, item := range engine.AvailableTestcases() {
		if _, tc, ok := splitTestcaseItem(item); ok {
			ids = append(ids, tc)
		}
	}
	if len(ids) == 0 {
		t.Fatal("engine.AvailableTestcases returned no testcases")
	}

	catalogs := []struct{ dir, prefix string }{
		{filepath.Join("..", "ui", "src", "i18n"), "tc."},
		{filepath.Join("..", "ui-public", "src", "i18n"), "pub.tc."},
	}
	for _, c := range catalogs {
		files, err := filepath.Glob(filepath.Join(c.dir, "*.json"))
		if err != nil {
			t.Fatalf("glob %s: %v", c.dir, err)
		}
		if len(files) != 12 {
			t.Fatalf("%s holds %d catalogs, want 12", c.dir, len(files))
		}
		for _, file := range files {
			t.Run(file, func(t *testing.T) {
				data, err := os.ReadFile(file)
				if err != nil {
					t.Fatalf("read %s: %v", file, err)
				}
				var keys map[string]string
				if err := json.Unmarshal(data, &keys); err != nil {
					t.Fatalf("decode %s: %v", file, err)
				}
				for _, id := range ids {
					if keys[c.prefix+id] == "" {
						t.Errorf("no %s%s", c.prefix, id)
					}
				}
			})
		}
	}

	t.Run("analysis-ui", func(t *testing.T) {
		path := filepath.Join("..", "analysis-ui", "src", "lib", "testcaseTitles.ts")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		have := map[string]bool{}
		for _, m := range regexp.MustCompile(`(?m)^\s+([a-z]+[0-9]+): "`).FindAllSubmatch(data, -1) {
			have[string(m[1])] = true
		}
		for _, id := range ids {
			if !have[id] {
				t.Errorf("no title for %s in %s", id, path)
			}
		}
	})
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

func TestSpecTestcasesMarkExcluded(t *testing.T) {
	srv := newTestServer(t, withConfig(func(c *Config) { c.Exclude = []string{"dnssec10"} }))
	var list SpecTestcaseList
	getSpec(t, srv, "/api/v1/spec/testcases?module=dnssec", &list)
	excluded := []string{}
	for _, it := range list.Items {
		if it.Excluded {
			excluded = append(excluded, it.ID)
		}
	}
	if !slices.Equal(excluded, []string{"dnssec10"}) || list.Total < 2 {
		t.Fatalf("excluded %v of %d, want only dnssec10 in the full list", excluded, list.Total)
	}
	var detail SpecTestcaseDetail
	getSpec(t, srv, "/api/v1/spec/testcases/dnssec10", &detail)
	if !detail.Excluded {
		t.Fatalf("detail excluded = false, want true")
	}
	var other SpecTestcaseDetail
	getSpec(t, srv, "/api/v1/spec/testcases/dnssec09", &other)
	if other.ID != "dnssec09" || other.Excluded {
		t.Fatalf("dnssec09 excluded = true, want false")
	}
}
