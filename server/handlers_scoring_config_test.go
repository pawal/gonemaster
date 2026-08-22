package server

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/scoring"
)

func TestGetScoringConfigDefault(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/scoring-config", nil)

	got := mustJSON[scoringConfigResponse](t, resp, http.StatusOK)
	if got.Source != SourceDefault {
		t.Fatalf("source: got %q, want %q", got.Source, SourceDefault)
	}
	if got.Readonly {
		t.Fatal("expected readonly=false for default source")
	}
	// Spot-check a known default value.
	if got.Config.SeverityPenalties["WARNING"] != 5 {
		t.Fatalf("WARNING penalty: got %d, want 5", got.Config.SeverityPenalties["WARNING"])
	}
}

func TestGetScoringConfigFromDatabase(t *testing.T) {
	srv := newTestServer(t)

	// Store a custom config in DB.
	cfg := scoring.DefaultConfig()
	cfg.SeverityPenalties["WARNING"] = 99
	raw, _ := json.Marshal(cfg)
	_ = srv.store.SetSetting("scoring_config", string(raw))

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/scoring-config", nil)

	got := mustJSON[scoringConfigResponse](t, resp, http.StatusOK)
	if got.Source != SourceDatabase {
		t.Fatalf("source: got %q, want %q", got.Source, SourceDatabase)
	}
	if got.Config.SeverityPenalties["WARNING"] != 99 {
		t.Fatalf("WARNING penalty: got %d, want 99", got.Config.SeverityPenalties["WARNING"])
	}
}

func TestGetScoringConfigCLIFlag(t *testing.T) {
	srv := newTestServer(t, withConfigSources(map[string]SettingSource{
		"scoring_config": SourceCLIFlag,
	}))
	// A stored value must not win over the CLI flag.
	_ = srv.store.SetSetting("scoring_config", `{"severity_penalties":{"WARNING":77}}`)

	if src := srv.scoringConfigSource(); src != SourceCLIFlag {
		t.Fatalf("source: got %q, want %q", src, SourceCLIFlag)
	}
}

func TestPutScoringConfig(t *testing.T) {
	srv := newTestServer(t)

	cfg := scoring.DefaultConfig()
	cfg.SeverityPenalties["ERROR"] = 50
	body, _ := json.Marshal(cfg)

	resp := doJSON(t, srv, http.MethodPut, "/api/v1/scoring-config", body)

	wantStatus(t, resp, http.StatusOK)

	// Verify stored in DB.
	raw, ok := srv.store.GetSetting("scoring_config")
	if !ok {
		t.Fatal("scoring_config not found in store after PUT")
	}
	var stored scoring.Config
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		t.Fatalf("unmarshal stored: %v", err)
	}
	if stored.SeverityPenalties["ERROR"] != 50 {
		t.Fatalf("ERROR penalty: got %d, want 50", stored.SeverityPenalties["ERROR"])
	}

	// Verify source now shows database.
	if srv.scoringConfigSource() != SourceDatabase {
		t.Fatalf("source after PUT: got %q, want database", srv.scoringConfigSource())
	}
}

func TestPutScoringConfigReadonlyRejected(t *testing.T) {
	srv := newTestServer(t)
	srv.SetConfigSources(map[string]SettingSource{
		"scoring_config": SourceCLIFlag,
	})

	body := `{"severity_penalties":{"WARNING":99}}`
	resp := doJSON(t, srv, http.MethodPut, "/api/v1/scoring-config", body)

	wantErrorCode(t, resp, http.StatusBadRequest, "readonly_scoring_config")
}

func TestPutScoringConfigInvalidJSON(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodPut, "/api/v1/scoring-config", "not json")

	wantStatus(t, resp, http.StatusBadRequest)
}

func TestDeleteScoringConfig(t *testing.T) {
	srv := newTestServer(t)

	// Plant a DB override.
	cfg := scoring.DefaultConfig()
	cfg.SeverityPenalties["WARNING"] = 77
	raw, _ := json.Marshal(cfg)
	_ = srv.store.SetSetting("scoring_config", string(raw))

	resp := doJSON(t, srv, http.MethodDelete, "/api/v1/scoring-config", nil, withHeader("Content-Type", "application/json"))

	wantStatus(t, resp, http.StatusOK)

	// DB setting should be gone.
	if _, ok := srv.store.GetSetting("scoring_config"); ok {
		t.Fatal("scoring_config still present in store after DELETE")
	}

	// Source should revert to default.
	if srv.scoringConfigSource() != SourceDefault {
		t.Fatalf("source after DELETE: got %q, want default", srv.scoringConfigSource())
	}
}

func TestDeleteScoringConfigReadonlyRejected(t *testing.T) {
	srv := newTestServer(t)
	srv.SetConfigSources(map[string]SettingSource{
		"scoring_config": SourceCLIFlag,
	})

	resp := doJSON(t, srv, http.MethodDelete, "/api/v1/scoring-config", nil)

	wantStatus(t, resp, http.StatusBadRequest)
}

func TestGetScoringConfigDefaults(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/scoring-config/defaults", nil)

	got := mustJSON[scoring.Config](t, resp, http.StatusOK)
	defaults := scoring.DefaultConfig()
	if got.SeverityPenalties["WARNING"] != defaults.SeverityPenalties["WARNING"] {
		t.Fatalf("defaults WARNING penalty mismatch: got %d", got.SeverityPenalties["WARNING"])
	}
}

func TestScoringConfigMethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)

	resp := doJSON(t, srv, http.MethodPost, "/api/v1/scoring-config", nil)

	wantStatus(t, resp, http.StatusMethodNotAllowed)
}

func TestApplyDatabaseSettingsScoringConfig(t *testing.T) {
	srv := newTestServer(t)

	cfg := scoring.DefaultConfig()
	cfg.SeverityPenalties["WARNING"] = 42
	raw, _ := json.Marshal(cfg)
	_ = srv.store.SetSetting("scoring_config", string(raw))

	srv.ApplyDatabaseSettings()

	// Verify the live store uses the updated config by graduating a job.
	job := Job{
		ID:         "job-sc-test",
		Domain:     "example.com",
		Status:     JobSucceeded,
		CreatedAt:  time.Now().UTC(),
		StartedAt:  time.Now().UTC(),
		FinishedAt: time.Now().UTC(),
	}
	job.PublicID = GeneratePublicID()
	created, err := srv.store.Create(job)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	entries := []engine.LogEntry{
		{Module: "BASIC", Tag: "SOME_TAG", Level: "WARNING"},
	}
	if err := srv.store.GraduateJob(created, entries); err != nil {
		t.Fatalf("graduate: %v", err)
	}

	result, ok := srv.store.GetResult(created.ID)
	if !ok {
		t.Fatal("result not found")
	}
	if result.Score == nil {
		t.Fatal("score is nil")
	}
	// With WARNING penalty=42 applied to one WARNING entry in category nameserver_health
	// (weight 1.2), score should be less than default (penalty=5).
	defaultCfg := scoring.DefaultConfig()
	defaultEntries := []scoring.Entry{{Module: "BASIC", Tag: "SOME_TAG", Level: "WARNING"}}
	defaultResult := scoring.Compute("example.com", defaultEntries, defaultCfg)
	if result.Score.Score >= defaultResult.Score {
		t.Fatalf("expected lower score with higher penalty: got %d, default would be %d",
			result.Score.Score, defaultResult.Score)
	}
}

// TestGetResultRecomputesWithUpdatedConfig verifies that GetResult uses the
// current scoring config dynamically - changing the config after graduation
// changes the score returned by the next GetResult call.
func TestGetResultRecomputesWithUpdatedConfig(t *testing.T) {
	srv := newTestServer(t)

	// Graduate a job under the default config.
	job := Job{
		ID:         "job-recompute",
		Domain:     "recompute.example.com",
		Status:     JobSucceeded,
		PublicID:   GeneratePublicID(),
		CreatedAt:  time.Now().UTC(),
		StartedAt:  time.Now().UTC(),
		FinishedAt: time.Now().UTC(),
	}
	created, err := srv.store.Create(job)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	entries := []engine.LogEntry{
		{Module: "DNSSEC", Tag: "DS01_DIGEST_NOT_SUPPORTED_BY_NS", Level: "WARNING"},
	}
	if err := srv.store.GraduateJob(created, entries); err != nil {
		t.Fatalf("graduate: %v", err)
	}
	r1, ok := srv.store.GetResult(created.ID)
	if !ok || r1.Score == nil {
		t.Fatal("result or score missing after graduation")
	}
	scoreBefore := r1.Score.Score

	// Raise the WARNING penalty via the API.
	newCfg := scoring.DefaultConfig()
	newCfg.SeverityPenalties["WARNING"] = 80
	body, _ := json.Marshal(newCfg)
	resp := doJSON(t, srv, http.MethodPut, "/api/v1/scoring-config", body)
	wantStatus(t, resp, http.StatusOK)

	// GetResult should recompute using the new config.
	r2, ok := srv.store.GetResult(created.ID)
	if !ok || r2.Score == nil {
		t.Fatal("result or score missing after config change")
	}
	if r2.Score.Score >= scoreBefore {
		t.Fatalf("GetResult did not recompute: before=%d after=%d", scoreBefore, r2.Score.Score)
	}
}

// TestScoringConfigPersistsAcrossServerRestart verifies that a scoring config
// saved to the database is applied when a new server instance reuses the same
// store (simulating a server restart).
func TestScoringConfigPersistsAcrossServerRestart(t *testing.T) {
	b := testBackends(t)[0] // SQLite, always present
	store := testStoreForBackend(t, b)

	srv1 := newServer(DefaultConfig(), store, NewInMemoryQueue())

	// PUT a custom config on the first server.
	cfg := scoring.DefaultConfig()
	cfg.SeverityPenalties["WARNING"] = 88
	body, _ := json.Marshal(cfg)
	resp := doJSON(t, srv1, http.MethodPut, "/api/v1/scoring-config", body)
	wantStatus(t, resp, http.StatusOK)

	// Simulate a restart: new server instance with the same store.
	srv2 := newServer(DefaultConfig(), store, NewInMemoryQueue())
	srv2.ApplyDatabaseSettings()

	// Graduate a job on the restarted server and check the score.
	job := Job{
		ID:         "job-persist",
		Domain:     "persist.example.com",
		Status:     JobSucceeded,
		PublicID:   GeneratePublicID(),
		CreatedAt:  time.Now().UTC(),
		StartedAt:  time.Now().UTC(),
		FinishedAt: time.Now().UTC(),
	}
	created, err := store.Create(job)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	entries := []engine.LogEntry{
		{Module: "DNSSEC", Tag: "DS01_DIGEST_NOT_SUPPORTED_BY_NS", Level: "WARNING"},
	}
	if err := store.GraduateJob(created, entries); err != nil {
		t.Fatalf("graduate: %v", err)
	}
	result, ok := store.GetResult(created.ID)
	if !ok || result.Score == nil {
		t.Fatal("result or score missing")
	}

	// Score should be lower than default (penalty 5) because persisted config has penalty 88.
	scoringEntries := []scoring.Entry{{Module: "DNSSEC", Tag: "DS01_DIGEST_NOT_SUPPORTED_BY_NS", Level: "WARNING"}}
	defaultScore := scoring.Compute("persist.example.com", scoringEntries, scoring.DefaultConfig())
	if result.Score.Score >= defaultScore.Score {
		t.Fatalf("persisted config not applied: got %d, default is %d", result.Score.Score, defaultScore.Score)
	}
}

// TestCLIFlagOverrideScoringConfig verifies that when scoring_config is marked
// as a CLI flag source, GET returns readonly=true with the file's values and
// PUT is rejected with 400.
func TestCLIFlagOverrideScoringConfig(t *testing.T) {
	// Write a temp config file with a distinctive penalty.
	customCfg := scoring.DefaultConfig()
	customCfg.SeverityPenalties["ERROR"] = 99
	raw, _ := json.Marshal(customCfg)
	f, err := os.CreateTemp("", "scoring-*.json")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(raw); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_ = f.Close()

	cfg := DefaultConfig()
	cfg.ScoringConfigPath = f.Name()
	srv := New(cfg)
	srv.SetConfigSources(map[string]SettingSource{"scoring_config": SourceCLIFlag})

	// GET must return source=cli_flag, readonly=true, and values from the file.
	resp := doJSON(t, srv, http.MethodGet, "/api/v1/scoring-config", nil)
	got := mustJSON[scoringConfigResponse](t, resp, http.StatusOK)
	if got.Source != SourceCLIFlag {
		t.Fatalf("source: got %q, want %q", got.Source, SourceCLIFlag)
	}
	if !got.Readonly {
		t.Fatal("expected readonly=true for cli_flag source")
	}
	if got.Config.SeverityPenalties["ERROR"] != 99 {
		t.Fatalf("ERROR penalty from file: got %d, want 99", got.Config.SeverityPenalties["ERROR"])
	}

	// PUT must be rejected.
	putBody, _ := json.Marshal(scoring.DefaultConfig())
	putResp := doJSON(t, srv, http.MethodPut, "/api/v1/scoring-config", putBody)
	wantStatus(t, putResp, http.StatusBadRequest)
}

func TestScoringConfigRuntimeUpdateAffectsGraduation(t *testing.T) {
	srv := newTestServer(t)

	// PUT a config with a very high WARNING penalty.
	cfg := scoring.DefaultConfig()
	cfg.SeverityPenalties["WARNING"] = 80
	body, _ := json.Marshal(cfg)
	resp := doJSON(t, srv, http.MethodPut, "/api/v1/scoring-config", body)
	wantStatus(t, resp, http.StatusOK)

	// Graduate a job with one WARNING entry.
	job := Job{
		ID:         "job-runtime-sc",
		Domain:     "runtime.example.com",
		Status:     JobSucceeded,
		PublicID:   GeneratePublicID(),
		CreatedAt:  time.Now().UTC(),
		StartedAt:  time.Now().UTC(),
		FinishedAt: time.Now().UTC(),
	}
	created, _ := srv.store.Create(job)
	entries := []engine.LogEntry{
		{Module: "DNSSEC", Tag: "DS01_DIGEST_NOT_SUPPORTED_BY_NS", Level: "WARNING"},
	}
	_ = srv.store.GraduateJob(created, entries)

	result, ok := srv.store.GetResult(created.ID)
	if !ok {
		t.Fatal("result not found")
	}
	if result.Score == nil {
		t.Fatal("score is nil")
	}

	// Compare with score under default config (penalty=5).
	scoringEntries := []scoring.Entry{{Module: "DNSSEC", Tag: "DS01_DIGEST_NOT_SUPPORTED_BY_NS", Level: "WARNING"}}
	defaultScore := scoring.Compute("runtime.example.com", scoringEntries, scoring.DefaultConfig())
	if result.Score.Score >= defaultScore.Score {
		t.Fatalf("expected lower score with higher penalty: got %d, default is %d",
			result.Score.Score, defaultScore.Score)
	}
}
