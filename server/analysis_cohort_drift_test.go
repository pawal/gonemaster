package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func cohortPath(id int64) string {
	return fmt.Sprintf("/api/v1/analysis/cohorts/%d", id)
}

// fmtDriftName builds a distinct single-label name for the paging test.
func fmtDriftName(i int) string { return fmt.Sprintf("tld%04d", i) }

// fakeReferenceLists stands in for the external-data provider so drift tests
// never touch the network.
type fakeReferenceLists struct {
	name    string
	names   []string
	version string
	loaded  bool
	// asked records the list names the handler looked up.
	asked []string
}

func (f *fakeReferenceLists) ReferenceList(name string) ([]string, string, bool) {
	f.asked = append(f.asked, name)
	if !f.loaded || name != f.name {
		return nil, "", false
	}
	return f.names, f.version, true
}

// seedTaggedDomains creates each name and puts it in tag.
func seedTaggedDomains(t *testing.T, srv *Server, tag string, names ...string) {
	t.Helper()
	var ids []int64
	for _, name := range names {
		d, err := srv.store.GetOrCreateDomain(name)
		if err != nil {
			t.Fatalf("GetOrCreateDomain(%q): %v", name, err)
		}
		ids = append(ids, d.ID)
	}
	if err := srv.store.TagDomains(tag, ids); err != nil {
		t.Fatalf("TagDomains(%q): %v", tag, err)
	}
}

// getCohortDetailRaw reads the admin cohort detail as a generic object so a
// test can assert on the presence of a field, not only on its value.
func getCohortDetailRaw(t *testing.T, srv *Server, id int64) map[string]any {
	t.Helper()
	resp := doJSON(t, srv, http.MethodGet, cohortPath(id), "")
	wantStatus(t, resp, http.StatusOK)
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode cohort detail: %v", err)
	}
	return out
}

func getCohortDrift(t *testing.T, srv *Server, id int64) *AnalysisCohortSourceDrift {
	t.Helper()
	resp := doJSON(t, srv, http.MethodGet, cohortPath(id), "")
	wantStatus(t, resp, http.StatusOK)
	var out struct {
		SourceDrift *AnalysisCohortSourceDrift `json:"source_drift"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode cohort detail: %v", err)
	}
	return out.SourceDrift
}

func TestCreateAnalysisCohortStoresReferenceList(t *testing.T) {
	srv, _ := newAnalysisAdminTestServer(t)
	cohort := createAnalysisCohort(t, srv,
		`{"source_tag":"tld","analysis_enabled":true,"reference_list":"iana_tlds"}`)

	if cohort.ReferenceList != AnalysisReferenceListIANATLDs {
		t.Fatalf("reference_list = %q, want %q", cohort.ReferenceList, AnalysisReferenceListIANATLDs)
	}
	stored, ok := srv.store.GetAnalysisCohort(cohort.ID)
	if !ok || stored.ReferenceList != AnalysisReferenceListIANATLDs {
		t.Fatalf("stored cohort = %+v, want reference_list %q", stored, AnalysisReferenceListIANATLDs)
	}
}

func TestCreateAnalysisCohortRejectsUnknownReferenceList(t *testing.T) {
	srv, _ := newAnalysisAdminTestServer(t)
	resp := doJSON(t, srv, http.MethodPost, "/api/v1/analysis/cohorts",
		`{"source_tag":"tld","analysis_enabled":true,"reference_list":"nope"}`)
	wantStatus(t, resp, http.StatusBadRequest)
}

func TestPatchAnalysisCohortSetsAndClearsReferenceList(t *testing.T) {
	srv, _ := newAnalysisAdminTestServer(t)
	cohort := createAnalysisCohort(t, srv, `{"source_tag":"tld","analysis_enabled":true}`)

	resp := doJSON(t, srv, http.MethodPatch, cohortPath(cohort.ID), `{"reference_list":"iana_tlds"}`)
	wantStatus(t, resp, http.StatusOK)
	if got := decodeCohort(t, resp.Body); got.ReferenceList != AnalysisReferenceListIANATLDs {
		t.Fatalf("reference_list = %q, want %q", got.ReferenceList, AnalysisReferenceListIANATLDs)
	}

	resp = doJSON(t, srv, http.MethodPatch, cohortPath(cohort.ID), `{"reference_list":""}`)
	wantStatus(t, resp, http.StatusOK)
	if got := decodeCohort(t, resp.Body); got.ReferenceList != "" {
		t.Fatalf("reference_list = %q, want empty", got.ReferenceList)
	}
}

func TestPatchAnalysisCohortRejectsUnknownReferenceList(t *testing.T) {
	srv, _ := newAnalysisAdminTestServer(t)
	cohort := createAnalysisCohort(t, srv, `{"source_tag":"tld","analysis_enabled":true}`)

	resp := doJSON(t, srv, http.MethodPatch, cohortPath(cohort.ID), `{"reference_list":"nope"}`)
	wantStatus(t, resp, http.StatusBadRequest)
	if stored, _ := srv.store.GetAnalysisCohort(cohort.ID); stored.ReferenceList != "" {
		t.Fatalf("reference_list = %q, want the rejected value not stored", stored.ReferenceList)
	}
}

func TestAnalysisCohortDetailOmitsDriftWithoutReferenceList(t *testing.T) {
	srv, _ := newAnalysisAdminTestServer(t)
	lists := &fakeReferenceLists{name: AnalysisReferenceListIANATLDs, names: []string{"se"}, loaded: true}
	srv.refLists = lists
	cohort := createAnalysisCohort(t, srv, `{"source_tag":"tld","analysis_enabled":true}`)

	raw := getCohortDetailRaw(t, srv, cohort.ID)
	if _, present := raw["source_drift"]; present {
		t.Error("source_drift present for a cohort with no reference list")
	}
	if len(lists.asked) != 0 {
		t.Errorf("looked up %v, want no lookup at all", lists.asked)
	}
}

func TestAnalysisCohortDetailOmitsDriftWhenProviderDisabled(t *testing.T) {
	srv, _ := newAnalysisAdminTestServer(t)
	cohort := createAnalysisCohort(t, srv,
		`{"source_tag":"tld","analysis_enabled":true,"reference_list":"iana_tlds"}`)

	raw := getCohortDetailRaw(t, srv, cohort.ID)
	if _, present := raw["source_drift"]; present {
		t.Error("source_drift present with the provider disabled")
	}
}

func TestAnalysisCohortDetailOmitsDriftWhenListNotLoaded(t *testing.T) {
	srv, _ := newAnalysisAdminTestServer(t)
	srv.refLists = &fakeReferenceLists{name: AnalysisReferenceListIANATLDs, loaded: false}
	cohort := createAnalysisCohort(t, srv,
		`{"source_tag":"tld","analysis_enabled":true,"reference_list":"iana_tlds"}`)

	raw := getCohortDetailRaw(t, srv, cohort.ID)
	// An unloaded list must not report the whole tag as extra.
	if _, present := raw["source_drift"]; present {
		t.Error("source_drift present before the dataset was fetched")
	}
}

func TestAnalysisCohortDetailReportsDrift(t *testing.T) {
	srv, _ := newAnalysisAdminTestServer(t)
	srv.refLists = &fakeReferenceLists{
		name:    AnalysisReferenceListIANATLDs,
		names:   []string{"se", "nu", "example"},
		version: "2026090700",
		loaded:  true,
	}
	cohort := createAnalysisCohort(t, srv,
		`{"source_tag":"tld","analysis_enabled":true,"reference_list":"iana_tlds"}`)
	seedTaggedDomains(t, srv, "tld", "se", "nu", "retired")

	drift := getCohortDrift(t, srv, cohort.ID)
	if drift == nil {
		t.Fatal("source_drift missing")
	}
	if drift.ListVersion != "2026090700" {
		t.Errorf("list_version = %q, want 2026090700", drift.ListVersion)
	}
	if drift.CheckedAt.IsZero() {
		t.Error("checked_at is zero")
	}
	if len(drift.Missing) != 1 || drift.Missing[0] != "example" {
		t.Errorf("missing = %v, want [example]", drift.Missing)
	}
	if len(drift.Extra) != 1 || drift.Extra[0] != "retired" {
		t.Errorf("extra = %v, want [retired]", drift.Extra)
	}
}

func TestAnalysisCohortDriftNormalizesCase(t *testing.T) {
	srv, _ := newAnalysisAdminTestServer(t)
	srv.refLists = &fakeReferenceLists{
		name:   AnalysisReferenceListIANATLDs,
		names:  []string{"XN--MGBAAM7A8H", "se."},
		loaded: true,
	}
	cohort := createAnalysisCohort(t, srv,
		`{"source_tag":"tld","analysis_enabled":true,"reference_list":"iana_tlds"}`)
	seedTaggedDomains(t, srv, "tld", "xn--mgbaam7a8h", "SE")

	drift := getCohortDrift(t, srv, cohort.ID)
	if drift == nil {
		t.Fatal("source_drift missing")
	}
	if len(drift.Missing) != 0 || len(drift.Extra) != 0 {
		t.Errorf("drift = %+v, want no difference after normalization", drift)
	}
}

func TestAnalysisCohortDriftEmptyTagReportsEveryListMember(t *testing.T) {
	srv, _ := newAnalysisAdminTestServer(t)
	srv.refLists = &fakeReferenceLists{
		name:   AnalysisReferenceListIANATLDs,
		names:  []string{"se", "nu"},
		loaded: true,
	}
	cohort := createAnalysisCohort(t, srv,
		`{"source_tag":"tld","analysis_enabled":true,"reference_list":"iana_tlds"}`)

	drift := getCohortDrift(t, srv, cohort.ID)
	if drift == nil {
		t.Fatal("source_drift missing")
	}
	if len(drift.Missing) != 2 || len(drift.Extra) != 0 {
		t.Errorf("drift = %+v, want both list members missing", drift)
	}
}

func TestAnalysisCohortDriftReadsEveryPageOfALargeTag(t *testing.T) {
	srv, _ := newAnalysisAdminTestServer(t)
	// One name past a full page, so a single-page read would report the
	// remainder as missing.
	names := make([]string, 0, driftDomainPage+1)
	for i := 0; i <= driftDomainPage; i++ {
		names = append(names, fmtDriftName(i))
	}
	srv.refLists = &fakeReferenceLists{name: AnalysisReferenceListIANATLDs, names: names, loaded: true}
	cohort := createAnalysisCohort(t, srv,
		`{"source_tag":"tld","analysis_enabled":true,"reference_list":"iana_tlds"}`)
	seedTaggedDomains(t, srv, "tld", names...)

	drift := getCohortDrift(t, srv, cohort.ID)
	if drift == nil {
		t.Fatal("source_drift missing")
	}
	if len(drift.Missing) != 0 || len(drift.Extra) != 0 {
		t.Errorf("missing %d, extra %d, want none", len(drift.Missing), len(drift.Extra))
	}
}

func TestAnalysisCohortDetailOmitsZeroLastMaterializedAt(t *testing.T) {
	srv, _ := newAnalysisAdminTestServer(t)
	cohort := createAnalysisCohort(t, srv, `{"source_tag":"tld"}`)

	raw := getCohortDetailRaw(t, srv, cohort.ID)
	if _, present := raw["last_materialized_at"]; present {
		t.Error("last_materialized_at present on a never-materialized cohort")
	}
	if raw["source_tag"] != "tld" {
		t.Errorf("source_tag = %v, want the cohort fields inlined", raw["source_tag"])
	}
}
