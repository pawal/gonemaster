package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type recordingAnalysisController struct {
	mu              sync.Mutex
	projectRunIDs   []string
	rebuildCohorts  []int64
	clearCohorts    []int64
	reconcileEvents []reconcileEvent
	rebuildErr      error
	clearErr        error
	reconcileErr    error
}

type reconcileEvent struct {
	before AnalysisCohort
	after  AnalysisCohort
}

func (c *recordingAnalysisController) RepairAllCohorts(context.Context) error { return nil }

func (c *recordingAnalysisController) ProjectRun(runID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.projectRunIDs = append(c.projectRunIDs, runID)
	return nil
}

func (c *recordingAnalysisController) RebuildCohort(_ context.Context, cohortID int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rebuildCohorts = append(c.rebuildCohorts, cohortID)
	return c.rebuildErr
}

func (c *recordingAnalysisController) ClearCohort(_ context.Context, cohortID int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clearCohorts = append(c.clearCohorts, cohortID)
	return c.clearErr
}

func (c *recordingAnalysisController) ReconcileCohortChange(_ context.Context, before, after AnalysisCohort) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reconcileEvents = append(c.reconcileEvents, reconcileEvent{before: before, after: after})
	return c.reconcileErr
}

func (c *recordingAnalysisController) snapshot() recordingAnalysisController {
	c.mu.Lock()
	defer c.mu.Unlock()
	return recordingAnalysisController{
		projectRunIDs:   append([]string(nil), c.projectRunIDs...),
		rebuildCohorts:  append([]int64(nil), c.rebuildCohorts...),
		clearCohorts:    append([]int64(nil), c.clearCohorts...),
		reconcileEvents: append([]reconcileEvent(nil), c.reconcileEvents...),
	}
}

func newAnalysisAdminTestServer(t *testing.T) (*Server, *recordingAnalysisController) {
	t.Helper()
	srv := New(DefaultConfig())
	spy := &recordingAnalysisController{}
	srv.SetAnalysisController(spy)
	return srv, spy
}

func decodeCohort(t *testing.T, body *bytes.Buffer) AnalysisCohort {
	t.Helper()
	var c AnalysisCohort
	if err := json.NewDecoder(body).Decode(&c); err != nil {
		t.Fatalf("decode cohort: %v", err)
	}
	return c
}

func createAnalysisCohort(t *testing.T, srv *Server, body string) AnalysisCohort {
	t.Helper()
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analysis/cohorts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("create cohort: expected 201, got %d: %s", resp.Code, resp.Body)
	}
	return decodeCohort(t, resp.Body)
}

func TestCreateAnalysisCohortTagDefaults(t *testing.T) {
	srv, spy := newAnalysisAdminTestServer(t)
	cohort := createAnalysisCohort(t, srv, `{"source_tag":"tld","label":"TLD","analysis_enabled":true}`)

	if cohort.ID == 0 {
		t.Fatalf("expected ID assigned, got %+v", cohort)
	}
	if cohort.SourceType != "tag" {
		t.Fatalf("expected source_type=tag, got %q", cohort.SourceType)
	}
	if !cohort.AnalysisEnabled || cohort.PublicEnabled || cohort.IsDefault {
		t.Fatalf("unexpected flags: %+v", cohort)
	}
	if cohort.MaterializationStatus != AnalysisMaterializationPending {
		t.Fatalf("expected pending materialization, got %q", cohort.MaterializationStatus)
	}
	snap := spy.snapshot()
	if len(snap.rebuildCohorts) != 1 || snap.rebuildCohorts[0] != cohort.ID {
		t.Fatalf("expected one rebuild for created cohort, got %+v", snap.rebuildCohorts)
	}
}

func TestCreateAnalysisCohortRejectsMissingSourceTag(t *testing.T) {
	srv, _ := newAnalysisAdminTestServer(t)
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analysis/cohorts",
		bytes.NewBufferString(`{"label":"no source"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.Code, resp.Body)
	}
}

func TestCreateAnalysisCohortRejectsPublicWithoutAnalysis(t *testing.T) {
	srv, _ := newAnalysisAdminTestServer(t)
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analysis/cohorts",
		bytes.NewBufferString(`{"source_tag":"x","public_enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.Code, resp.Body)
	}
}

func TestCreateAnalysisCohortRejectsDefaultWithoutPublic(t *testing.T) {
	srv, _ := newAnalysisAdminTestServer(t)
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analysis/cohorts",
		bytes.NewBufferString(`{"source_tag":"x","analysis_enabled":true,"is_default":true}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.Code, resp.Body)
	}
}

func TestCreateAnalysisCohortAutoCreatesMissingTag(t *testing.T) {
	srv, _ := newAnalysisAdminTestServer(t)
	if _, exists := srv.store.GetTag("tld"); exists {
		t.Fatal("precondition: tag should not exist yet")
	}
	createAnalysisCohort(t, srv, `{"source_tag":"tld"}`)
	if _, exists := srv.store.GetTag("tld"); !exists {
		t.Fatal("expected cohort creation to auto-create the backing tag")
	}
}

func TestCreateAnalysisCohortReusesExistingTag(t *testing.T) {
	srv, _ := newAnalysisAdminTestServer(t)
	if err := srv.store.CreateTag("tld", "preexisting"); err != nil {
		t.Fatalf("seed tag: %v", err)
	}
	createAnalysisCohort(t, srv, `{"source_tag":"tld"}`)
	tag, _ := srv.store.GetTag("tld")
	if tag.Description != "preexisting" {
		t.Fatalf("expected existing tag description to be preserved, got %q", tag.Description)
	}
}

func TestCreateAnalysisCohortRejectsDuplicateSource(t *testing.T) {
	srv, _ := newAnalysisAdminTestServer(t)
	createAnalysisCohort(t, srv, `{"source_tag":"tld"}`)
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analysis/cohorts",
		bytes.NewBufferString(`{"source_tag":"tld"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", resp.Code, resp.Body)
	}
}

func TestListAnalysisCohortsSortedBySortOrder(t *testing.T) {
	srv, _ := newAnalysisAdminTestServer(t)
	createAnalysisCohort(t, srv, `{"source_tag":"gov","sort_order":20}`)
	createAnalysisCohort(t, srv, `{"source_tag":"tld","sort_order":10}`)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/analysis/cohorts", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var cohorts []AnalysisCohort
	if err := json.NewDecoder(resp.Body).Decode(&cohorts); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(cohorts) != 2 || cohorts[0].SourceTag != "tld" || cohorts[1].SourceTag != "gov" {
		t.Fatalf("unexpected ordering: %+v", cohorts)
	}
}

func TestGetAnalysisCohortByIDReturnsRow(t *testing.T) {
	srv, _ := newAnalysisAdminTestServer(t)
	cohort := createAnalysisCohort(t, srv, `{"source_tag":"tld"}`)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/analysis/cohorts/%d", cohort.ID), nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	got := decodeCohort(t, resp.Body)
	if got.ID != cohort.ID || got.SourceTag != "tld" {
		t.Fatalf("unexpected cohort: %+v", got)
	}
}

func TestGetAnalysisCohortByIDNotFound(t *testing.T) {
	srv, _ := newAnalysisAdminTestServer(t)
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/analysis/cohorts/999", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}

func TestPatchAnalysisCohortTogglesAnalysisEnabledAndReconciles(t *testing.T) {
	srv, spy := newAnalysisAdminTestServer(t)
	cohort := createAnalysisCohort(t, srv, `{"source_tag":"tld"}`)
	// one rebuild from creation (analysis_enabled defaulted to false above → no
	// rebuild); reset spy to assert the PATCH triggers reconcile only.
	spy.mu.Lock()
	spy.rebuildCohorts = nil
	spy.mu.Unlock()

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch,
		fmt.Sprintf("/api/v1/analysis/cohorts/%d", cohort.ID),
		bytes.NewBufferString(`{"analysis_enabled":true,"label":"TLD Analysis"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	got := decodeCohort(t, resp.Body)
	if !got.AnalysisEnabled {
		t.Fatalf("expected analysis_enabled=true, got %+v", got)
	}
	if got.Label != "TLD Analysis" {
		t.Fatalf("expected label update, got %q", got.Label)
	}
	snap := spy.snapshot()
	if len(snap.reconcileEvents) != 1 {
		t.Fatalf("expected one reconcile event, got %d", len(snap.reconcileEvents))
	}
	ev := snap.reconcileEvents[0]
	if ev.before.AnalysisEnabled || !ev.after.AnalysisEnabled {
		t.Fatalf("reconcile event should show false→true: %+v", ev)
	}
}

func TestPatchAnalysisCohortSetDefaultClearsOthers(t *testing.T) {
	srv, _ := newAnalysisAdminTestServer(t)
	a := createAnalysisCohort(t, srv, `{"source_tag":"a","analysis_enabled":true,"public_enabled":true,"is_default":true}`)
	b := createAnalysisCohort(t, srv, `{"source_tag":"b","analysis_enabled":true,"public_enabled":true}`)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch,
		fmt.Sprintf("/api/v1/analysis/cohorts/%d", b.ID),
		bytes.NewBufferString(`{"is_default":true}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}

	reloadedA, _ := srv.store.GetAnalysisCohort(a.ID)
	reloadedB, _ := srv.store.GetAnalysisCohort(b.ID)
	if reloadedA.IsDefault {
		t.Fatalf("expected cohort a to no longer be default, got %+v", reloadedA)
	}
	if !reloadedB.IsDefault {
		t.Fatalf("expected cohort b to be default, got %+v", reloadedB)
	}
}

func TestPatchAnalysisCohortRejectsPublicWithoutAnalysis(t *testing.T) {
	srv, _ := newAnalysisAdminTestServer(t)
	cohort := createAnalysisCohort(t, srv, `{"source_tag":"tld","analysis_enabled":true}`)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch,
		fmt.Sprintf("/api/v1/analysis/cohorts/%d", cohort.ID),
		bytes.NewBufferString(`{"analysis_enabled":false,"public_enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.Code, resp.Body)
	}
}

func TestAnalysisCohortRebuildInvokesController(t *testing.T) {
	srv, spy := newAnalysisAdminTestServer(t)
	cohort := createAnalysisCohort(t, srv, `{"source_tag":"tld"}`)
	spy.mu.Lock()
	spy.rebuildCohorts = nil
	spy.mu.Unlock()

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/analysis/cohorts/%d/rebuild", cohort.ID), nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	snap := spy.snapshot()
	if len(snap.rebuildCohorts) != 1 || snap.rebuildCohorts[0] != cohort.ID {
		t.Fatalf("expected one rebuild call for cohort %d, got %+v", cohort.ID, snap.rebuildCohorts)
	}
}

func TestAnalysisCohortClearInvokesController(t *testing.T) {
	srv, spy := newAnalysisAdminTestServer(t)
	cohort := createAnalysisCohort(t, srv, `{"source_tag":"tld"}`)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/analysis/cohorts/%d/clear", cohort.ID), nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	snap := spy.snapshot()
	if len(snap.clearCohorts) != 1 || snap.clearCohorts[0] != cohort.ID {
		t.Fatalf("expected one clear call for cohort %d, got %+v", cohort.ID, snap.clearCohorts)
	}
}

func TestAnalysisCohortRebuildFailsWhenControllerMissing(t *testing.T) {
	srv := New(DefaultConfig())
	created, err := srv.store.UpsertAnalysisCohort(AnalysisCohort{SourceType: "tag", SourceTag: "tld"})
	if err != nil {
		t.Fatalf("upsert cohort: %v", err)
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/analysis/cohorts/%d/rebuild", created.ID), nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", resp.Code, resp.Body)
	}
}

func TestAnalysisCohortRebuildNotFound(t *testing.T) {
	srv, _ := newAnalysisAdminTestServer(t)
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analysis/cohorts/42/rebuild", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", resp.Code, resp.Body)
	}
}

func TestCreateAnalysisCohortRollsBackWhenRebuildFails(t *testing.T) {
	srv, spy := newAnalysisAdminTestServer(t)
	spy.rebuildErr = errors.New("boom")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analysis/cohorts",
		bytes.NewBufferString(`{"source_tag":"tld","analysis_enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", resp.Code, resp.Body)
	}
	if got := srv.store.ListAnalysisCohorts(); len(got) != 0 {
		t.Fatalf("expected create rollback to remove cohort, got %+v", got)
	}
}

func TestPatchAnalysisCohortRollsBackWhenReconcileFails(t *testing.T) {
	srv, spy := newAnalysisAdminTestServer(t)
	cohort := createAnalysisCohort(t, srv, `{"source_tag":"tld","label":"Before","analysis_enabled":true}`)
	spy.reconcileErr = errors.New("boom")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch,
		fmt.Sprintf("/api/v1/analysis/cohorts/%d", cohort.ID),
		bytes.NewBufferString(`{"label":"After","public_enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", resp.Code, resp.Body)
	}
	reloaded, ok := srv.store.GetAnalysisCohort(cohort.ID)
	if !ok {
		t.Fatalf("expected cohort to remain after rollback")
	}
	if reloaded.Label != "Before" || reloaded.PublicEnabled {
		t.Fatalf("expected cohort rollback to restore original row, got %+v", reloaded)
	}
}
