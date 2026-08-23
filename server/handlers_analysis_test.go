package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// waitForCond polls until cond returns true or one second elapses. Used
// by handler tests to observe side effects of cohort rebuilds that now
// run in a goroutine instead of synchronously inside the request. It stays
// on the real clock: the rebuild does its own I/O outside any bubble.
func waitForCond(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met within timeout")
}

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

func (c *recordingAnalysisController) CaptureCompletedSnapshots(context.Context) error {
	return nil
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
	spy := &recordingAnalysisController{}
	return newTestServer(t, withAnalysisController(spy)), spy
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
	resp := doJSON(t, srv, http.MethodPost, "/api/v1/analysis/cohorts", body)
	wantStatus(t, resp, http.StatusAccepted)
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
	waitForCond(t, func() bool {
		snap := spy.snapshot()
		return len(snap.rebuildCohorts) == 1 && snap.rebuildCohorts[0] == cohort.ID
	})
}

func TestCreateAnalysisCohortRejectsMissingSourceTag(t *testing.T) {
	srv, _ := newAnalysisAdminTestServer(t)
	resp := doJSON(t, srv, http.MethodPost, "/api/v1/analysis/cohorts", `{"label":"no source"}`)
	wantStatus(t, resp, http.StatusBadRequest)
}

func TestCreateAnalysisCohortRejectsPublicWithoutAnalysis(t *testing.T) {
	srv, _ := newAnalysisAdminTestServer(t)
	resp := doJSON(t, srv, http.MethodPost, "/api/v1/analysis/cohorts", `{"source_tag":"x","public_enabled":true}`)
	wantStatus(t, resp, http.StatusBadRequest)
}

func TestCreateAnalysisCohortRejectsDefaultWithoutPublic(t *testing.T) {
	srv, _ := newAnalysisAdminTestServer(t)
	resp := doJSON(t, srv, http.MethodPost, "/api/v1/analysis/cohorts", `{"source_tag":"x","analysis_enabled":true,"is_default":true}`)
	wantStatus(t, resp, http.StatusBadRequest)
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

func TestAnalysisCohortJSONOmitsZeroLastMaterializedAt(t *testing.T) {
	srv, _ := newAnalysisAdminTestServer(t)
	createAnalysisCohort(t, srv, `{"source_tag":"tld"}`)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/analysis/cohorts", nil)
	wantStatus(t, resp, http.StatusOK)
	body := resp.Body.String()
	if strings.Contains(body, "last_materialized_at") {
		t.Fatalf("expected last_materialized_at to be omitted when zero, got body: %s", body)
	}
	if strings.Contains(body, "0001-01-01") {
		t.Fatalf("response leaked Go zero time: %s", body)
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
	resp := doJSON(t, srv, http.MethodPost, "/api/v1/analysis/cohorts", `{"source_tag":"tld"}`)
	wantStatus(t, resp, http.StatusConflict)
}

func TestListAnalysisCohortsSortedBySortOrder(t *testing.T) {
	srv, _ := newAnalysisAdminTestServer(t)
	createAnalysisCohort(t, srv, `{"source_tag":"gov","sort_order":20}`)
	createAnalysisCohort(t, srv, `{"source_tag":"tld","sort_order":10}`)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/analysis/cohorts", nil)
	cohorts := mustJSON[[]AnalysisCohort](t, resp, http.StatusOK)
	if len(cohorts) != 2 || cohorts[0].SourceTag != "tld" || cohorts[1].SourceTag != "gov" {
		t.Fatalf("unexpected ordering: %+v", cohorts)
	}
}

func TestGetAnalysisCohortByIDReturnsRow(t *testing.T) {
	srv, _ := newAnalysisAdminTestServer(t)
	cohort := createAnalysisCohort(t, srv, `{"source_tag":"tld"}`)

	resp := doJSON(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/analysis/cohorts/%d", cohort.ID), nil)
	wantStatus(t, resp, http.StatusOK)
	got := decodeCohort(t, resp.Body)
	if got.ID != cohort.ID || got.SourceTag != "tld" {
		t.Fatalf("unexpected cohort: %+v", got)
	}
}

func TestGetAnalysisCohortByIDNotFound(t *testing.T) {
	srv, _ := newAnalysisAdminTestServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/api/v1/analysis/cohorts/999", nil)
	wantStatus(t, resp, http.StatusNotFound)
}

func TestPatchAnalysisCohortTogglesAnalysisEnabledAndReconciles(t *testing.T) {
	srv, spy := newAnalysisAdminTestServer(t)
	cohort := createAnalysisCohort(t, srv, `{"source_tag":"tld"}`)
	// one rebuild from creation (analysis_enabled defaulted to false above → no
	// rebuild); reset spy to assert the PATCH triggers reconcile only.
	spy.mu.Lock()
	spy.rebuildCohorts = nil
	spy.mu.Unlock()

	resp := doJSON(t, srv, http.MethodPatch, fmt.Sprintf("/api/v1/analysis/cohorts/%d", cohort.ID), `{"analysis_enabled":true,"label":"TLD Analysis"}`)
	wantStatus(t, resp, http.StatusOK)
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

	resp := doJSON(t, srv, http.MethodPatch, fmt.Sprintf("/api/v1/analysis/cohorts/%d", b.ID), `{"is_default":true}`)
	wantStatus(t, resp, http.StatusOK)

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

	resp := doJSON(t, srv, http.MethodPatch, fmt.Sprintf("/api/v1/analysis/cohorts/%d", cohort.ID), `{"analysis_enabled":false,"public_enabled":true}`)
	wantStatus(t, resp, http.StatusBadRequest)
}

func TestAnalysisCohortRebuildInvokesController(t *testing.T) {
	srv, spy := newAnalysisAdminTestServer(t)
	cohort := createAnalysisCohort(t, srv, `{"source_tag":"tld"}`)
	spy.mu.Lock()
	spy.rebuildCohorts = nil
	spy.mu.Unlock()

	resp := doJSON(t, srv, http.MethodPost, fmt.Sprintf("/api/v1/analysis/cohorts/%d/rebuild", cohort.ID), nil)
	wantStatus(t, resp, http.StatusAccepted)
	// Rebuild is dispatched in a goroutine, so poll until the spy
	// observes the call instead of asserting synchronously.
	waitForCond(t, func() bool {
		snap := spy.snapshot()
		return len(snap.rebuildCohorts) == 1 && snap.rebuildCohorts[0] == cohort.ID
	})
}

func TestAnalysisCohortClearInvokesController(t *testing.T) {
	srv, spy := newAnalysisAdminTestServer(t)
	cohort := createAnalysisCohort(t, srv, `{"source_tag":"tld"}`)

	resp := doJSON(t, srv, http.MethodPost, fmt.Sprintf("/api/v1/analysis/cohorts/%d/clear", cohort.ID), nil)
	wantStatus(t, resp, http.StatusOK)
	snap := spy.snapshot()
	if len(snap.clearCohorts) != 1 || snap.clearCohorts[0] != cohort.ID {
		t.Fatalf("expected one clear call for cohort %d, got %+v", cohort.ID, snap.clearCohorts)
	}
}

func TestAnalysisCohortRebuildFailsWhenControllerMissing(t *testing.T) {
	srv := newTestServer(t)
	created, err := srv.store.UpsertAnalysisCohort(AnalysisCohort{SourceType: "tag", SourceTag: "tld"})
	if err != nil {
		t.Fatalf("upsert cohort: %v", err)
	}

	resp := doJSON(t, srv, http.MethodPost, fmt.Sprintf("/api/v1/analysis/cohorts/%d/rebuild", created.ID), nil)
	wantStatus(t, resp, http.StatusServiceUnavailable)
}

func TestDeleteAnalysisCohortRemovesCatalogAndClears(t *testing.T) {
	srv, spy := newAnalysisAdminTestServer(t)
	cohort := createAnalysisCohort(t, srv, `{"source_tag":"tld"}`)

	resp := doJSON(t, srv, http.MethodDelete, fmt.Sprintf("/api/v1/analysis/cohorts/%d", cohort.ID), nil)
	wantStatus(t, resp, http.StatusNoContent)
	if _, found := srv.store.GetAnalysisCohort(cohort.ID); found {
		t.Fatal("expected cohort row to be removed from catalog")
	}
	snap := spy.snapshot()
	if len(snap.clearCohorts) == 0 || snap.clearCohorts[len(snap.clearCohorts)-1] != cohort.ID {
		t.Fatalf("expected controller.ClearCohort to be called for %d, got %+v", cohort.ID, snap.clearCohorts)
	}
}

func TestDeleteAnalysisCohortNotFound(t *testing.T) {
	srv, _ := newAnalysisAdminTestServer(t)
	resp := doJSON(t, srv, http.MethodDelete, "/api/v1/analysis/cohorts/999", nil)
	wantStatus(t, resp, http.StatusNotFound)
}

func TestAnalysisCohortRebuildNotFound(t *testing.T) {
	srv, _ := newAnalysisAdminTestServer(t)
	resp := doJSON(t, srv, http.MethodPost, "/api/v1/analysis/cohorts/42/rebuild", nil)
	wantStatus(t, resp, http.StatusNotFound)
}

func TestCreateAnalysisCohortKeepsCatalogWhenAsyncRebuildFails(t *testing.T) {
	// Rebuilds now run in a detached goroutine after the handler has
	// already returned 202, so a rebuild failure cannot unwind the
	// catalog row. The failure is recorded in the cohort's
	// materialization_status / last_materialization_error fields by
	// Controller.RebuildCohort; here the spy just logs the error.
	srv, spy := newAnalysisAdminTestServer(t)
	spy.rebuildErr = errors.New("boom")

	resp := doJSON(t, srv, http.MethodPost, "/api/v1/analysis/cohorts", `{"source_tag":"tld","analysis_enabled":true}`)
	wantStatus(t, resp, http.StatusAccepted)
	waitForCond(t, func() bool {
		snap := spy.snapshot()
		return len(snap.rebuildCohorts) == 1
	})
	if got := srv.store.ListAnalysisCohorts(); len(got) != 1 {
		t.Fatalf("expected cohort to remain after async rebuild failure, got %+v", got)
	}
}

func TestPatchAnalysisCohortRollsBackWhenReconcileFails(t *testing.T) {
	srv, spy := newAnalysisAdminTestServer(t)
	cohort := createAnalysisCohort(t, srv, `{"source_tag":"tld","label":"Before","analysis_enabled":true}`)
	spy.reconcileErr = errors.New("boom")

	resp := doJSON(t, srv, http.MethodPatch, fmt.Sprintf("/api/v1/analysis/cohorts/%d", cohort.ID), `{"label":"After","public_enabled":true}`)
	wantStatus(t, resp, http.StatusInternalServerError)
	reloaded, ok := srv.store.GetAnalysisCohort(cohort.ID)
	if !ok {
		t.Fatalf("expected cohort to remain after rollback")
	}
	if reloaded.Label != "Before" || reloaded.PublicEnabled {
		t.Fatalf("expected cohort rollback to restore original row, got %+v", reloaded)
	}
}
