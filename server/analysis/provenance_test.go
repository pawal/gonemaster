package analysis

import (
	"context"
	"testing"
	"time"

	serverpkg "codeberg.org/pawal/gonemaster/server"
)

// withGlobalVersion appends the GLOBAL_VERSION entry the engine emits at the
// start of every run. The shared testAnalysisEntries fixture deliberately
// omits it so that other tests keep exercising the unknown-provenance path.
func withGlobalVersion(entries []serverpkg.Entry, run serverpkg.Run, version string) []serverpkg.Entry {
	return append(entries, serverpkg.Entry{
		RunID:    run.ID,
		DomainID: run.DomainID,
		Tag:      globalVersionTag,
		Args:     map[string]any{globalVersionArg: version},
	})
}

func TestRunEngineVersion(t *testing.T) {
	run := serverpkg.Run{ID: "run-1", DomainID: 100}

	tests := []struct {
		name    string
		entries []serverpkg.Entry
		want    string
	}{
		{
			name:    "no entries at all",
			entries: nil,
			want:    "",
		},
		{
			// The common pre-instrumentation case: runs exist but carry no
			// version entry. Must stay empty rather than borrowing the
			// running build's version, which would invent provenance.
			name: "entries without GLOBAL_VERSION",
			entries: []serverpkg.Entry{
				{RunID: "run-1", Tag: "DS07_NOT_SIGNED"},
			},
			want: "",
		},
		{
			name:    "GLOBAL_VERSION present",
			entries: withGlobalVersion(nil, run, "v1.6.6"),
			want:    "v1.6.6",
		},
		{
			// The extractor must find the tag wherever it sits, not assume
			// it is first.
			name: "GLOBAL_VERSION after other entries",
			entries: withGlobalVersion([]serverpkg.Entry{
				{RunID: "run-1", Tag: "DS07_NOT_SIGNED"},
			}, run, "v1.4.4"),
			want: "v1.4.4",
		},
		{
			name: "GLOBAL_VERSION with a blank version arg",
			entries: []serverpkg.Entry{
				{RunID: "run-1", Tag: globalVersionTag, Args: map[string]any{globalVersionArg: "  "}},
			},
			want: "",
		},
		{
			name: "GLOBAL_VERSION with a non-string version arg",
			entries: []serverpkg.Entry{
				{RunID: "run-1", Tag: globalVersionTag, Args: map[string]any{globalVersionArg: 166}},
			},
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := runEngineVersion(tc.entries); got != tc.want {
				t.Fatalf("runEngineVersion = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMixedEngineVersions(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{name: "same version", a: "v1.6.6", b: "v1.6.6", want: false},
		{name: "different versions", a: "v1.6.3", b: "v1.6.6", want: true},
		// An unknown side means we could not read provenance for that run.
		// That is a gap in what we know, not evidence of disagreement, so
		// it must not raise the mixed flag.
		{name: "unknown on the left", a: "", b: "v1.6.6", want: false},
		{name: "unknown on the right", a: "v1.6.6", b: "", want: false},
		{name: "unknown on both sides", a: "", b: "", want: false},
		{name: "whitespace only counts as unknown", a: "  ", b: "v1.6.6", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := mixedEngineVersions(tc.a, tc.b); got != tc.want {
				t.Fatalf("mixedEngineVersions(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// projectRunWithVersion seeds one run into the lifecycle store, tagging it
// into the "tld" cohort, and projects it. An empty version omits the
// GLOBAL_VERSION entry entirely.
func projectRunWithVersion(t *testing.T, store *fakeStore, controller *Controller, batchID, runID string, domainID int64, domain string, at time.Time, version string) {
	t.Helper()
	run := testAnalysisRun(runID, domainID, domain, at, "192.0.2.10", "2001:db8::10")
	run.BatchID = batchID
	entries := testAnalysisEntries(run)
	if version != "" {
		entries = withGlobalVersion(entries, run, version)
	}
	// The loader caps its entry query at run.EntryCount, so the fixture has
	// to agree with what it seeds or the version entry is truncated away.
	run.EntryCount = len(entries)
	store.runs[run.ID] = run
	store.entries[run.ID] = entries
	store.tags[run.DomainID] = []string{"tld"}
	if err := controller.ProjectRun(run.ID); err != nil {
		t.Fatalf("ProjectRun %s: %v", runID, err)
	}
}

// seedRunWithVersion adds a run and its entries to the store without
// projecting it, so backfill tests can simulate snapshots that were captured
// before provenance stamping existed.
func seedRunWithVersion(store *fakeStore, batchID, runID string, domainID int64, domain string, at time.Time, version string) {
	run := testAnalysisRun(runID, domainID, domain, at, "192.0.2.10", "2001:db8::10")
	run.BatchID = batchID
	entries := testAnalysisEntries(run)
	if version != "" {
		entries = withGlobalVersion(entries, run, version)
	}
	run.EntryCount = len(entries)
	store.runs[run.ID] = run
	store.entries[run.ID] = entries
	store.tags[run.DomainID] = []string{"tld"}
}

// seedUnstampedSnapshot writes a captured snapshot with no engine version,
// which is exactly the shape of the snapshots already in production.
func seedUnstampedSnapshot(t *testing.T, store *fakeStore, batchID, slug string) {
	t.Helper()
	if _, err := store.UpsertAnalysisCohortSnapshot(serverpkg.AnalysisCohortSnapshot{
		CohortID: 10,
		BatchID:  batchID,
		Slug:     slug,
		Status:   serverpkg.AnalysisSnapshotStatusCaptured,
		IsPublic: true,
	}); err != nil {
		t.Fatalf("seed snapshot %s: %v", slug, err)
	}
}

func TestBackfillSnapshotEngineVersionsStampsCapturedSnapshots(t *testing.T) {
	store, _ := snapshotLifecycleStore(t)
	seedSnapshotBatch(store, "batch-old", true)
	seedRunWithVersion(store, "batch-old", "run-a", 100, "alpha.example",
		time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC), "v1.4.4")
	seedRunWithVersion(store, "batch-old", "run-b", 101, "beta.example",
		time.Date(2026, 5, 5, 10, 5, 0, 0, time.UTC), "v1.4.4")
	seedUnstampedSnapshot(t, store, "batch-old", "2026-05-05-old")

	controller := NewController(store)
	if err := controller.BackfillSnapshotEngineVersions(context.Background()); err != nil {
		t.Fatalf("BackfillSnapshotEngineVersions: %v", err)
	}

	snap, ok := store.GetAnalysisCohortSnapshotByBatch(10, "batch-old")
	if !ok {
		t.Fatal("expected snapshot")
	}
	if snap.EngineVersion != "v1.4.4" {
		t.Fatalf("EngineVersion = %q, want v1.4.4 recovered from stored runs", snap.EngineVersion)
	}
	if snap.MixedEngineVersion {
		t.Fatal("a batch with one version must not be flagged mixed")
	}
}

// Backfill runs on every startup, so it has to be safe to repeat and must
// not overwrite provenance that capture already recorded correctly.
func TestBackfillSnapshotEngineVersionsIsIdempotent(t *testing.T) {
	store, _ := snapshotLifecycleStore(t)
	seedSnapshotBatch(store, "batch-old", true)
	seedRunWithVersion(store, "batch-old", "run-a", 100, "alpha.example",
		time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC), "v1.4.4")
	seedUnstampedSnapshot(t, store, "batch-old", "2026-05-05-old")

	controller := NewController(store)
	for i := 0; i < 3; i++ {
		if err := controller.BackfillSnapshotEngineVersions(context.Background()); err != nil {
			t.Fatalf("BackfillSnapshotEngineVersions pass %d: %v", i, err)
		}
	}

	snap, _ := store.GetAnalysisCohortSnapshotByBatch(10, "batch-old")
	if snap.EngineVersion != "v1.4.4" || snap.MixedEngineVersion {
		t.Fatalf("repeat backfill changed the result: version=%q mixed=%v",
			snap.EngineVersion, snap.MixedEngineVersion)
	}
}

// The whole point of the unknown state: a snapshot whose runs never recorded
// a version must stay unknown, not acquire the running build's version.
func TestBackfillSnapshotEngineVersionsLeavesUnrecoverableUnknown(t *testing.T) {
	store, _ := snapshotLifecycleStore(t)
	seedSnapshotBatch(store, "batch-blank", true)
	seedRunWithVersion(store, "batch-blank", "run-a", 100, "alpha.example",
		time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC), "")
	seedUnstampedSnapshot(t, store, "batch-blank", "2026-05-05-blank")

	controller := NewController(store)
	if err := controller.BackfillSnapshotEngineVersions(context.Background()); err != nil {
		t.Fatalf("BackfillSnapshotEngineVersions: %v", err)
	}

	snap, _ := store.GetAnalysisCohortSnapshotByBatch(10, "batch-blank")
	if snap.EngineVersion != "" {
		t.Fatalf("EngineVersion = %q, want empty when no run recorded one", snap.EngineVersion)
	}
}

// A batch that spanned an upgrade must come out of backfill flagged, so the
// historical snapshots carry the same warning a freshly captured one would.
func TestBackfillSnapshotEngineVersionsFlagsMixedBatch(t *testing.T) {
	store, _ := snapshotLifecycleStore(t)
	seedSnapshotBatch(store, "batch-span", true)
	seedRunWithVersion(store, "batch-span", "run-a", 100, "alpha.example",
		time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC), "v1.6.6")
	seedRunWithVersion(store, "batch-span", "run-b", 101, "beta.example",
		time.Date(2026, 7, 31, 10, 5, 0, 0, time.UTC), "v1.6.3")
	seedUnstampedSnapshot(t, store, "batch-span", "2026-07-31-span")

	controller := NewController(store)
	if err := controller.BackfillSnapshotEngineVersions(context.Background()); err != nil {
		t.Fatalf("BackfillSnapshotEngineVersions: %v", err)
	}

	snap, _ := store.GetAnalysisCohortSnapshotByBatch(10, "batch-span")
	if !snap.MixedEngineVersion {
		t.Fatal("a batch spanning two versions must be flagged mixed")
	}
	// Sorted-first sample, so the value is stable across reruns.
	if snap.EngineVersion != "v1.6.3" {
		t.Fatalf("EngineVersion = %q, want the deterministic v1.6.3 sample", snap.EngineVersion)
	}
}

func TestControllerProjectRunStampsEngineVersion(t *testing.T) {
	store, _ := snapshotLifecycleStore(t)
	seedSnapshotBatch(store, "batch-stamp", true)
	controller := NewController(store)

	projectRunWithVersion(t, store, controller, "batch-stamp", "run-a", 100, "alpha.example",
		time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC), "v1.6.6")
	projectRunWithVersion(t, store, controller, "batch-stamp", "run-b", 101, "beta.example",
		time.Date(2026, 8, 15, 10, 5, 0, 0, time.UTC), "v1.6.6")

	snap, ok := store.GetAnalysisCohortSnapshotByBatch(10, "batch-stamp")
	if !ok {
		t.Fatal("expected snapshot")
	}
	if snap.EngineVersion != "v1.6.6" {
		t.Fatalf("EngineVersion = %q, want v1.6.6", snap.EngineVersion)
	}
	if snap.MixedEngineVersion {
		t.Fatal("two runs on the same version must not set the mixed flag")
	}
}

// A batch that spanned an engine upgrade is flagged but still usable. This
// is the deliberate difference from the mixed-profile guard, which fails the
// snapshot outright: mixed profiles make scores incomparable, whereas a
// mid-batch upgrade produces data that is merely confounded, and failing it
// would discard a whole cohort run for any deploy landing during a batch.
func TestControllerProjectRunFlagsMixedEngineVersion(t *testing.T) {
	store, _ := snapshotLifecycleStore(t)
	seedSnapshotBatch(store, "batch-upgrade", true)
	controller := NewController(store)

	projectRunWithVersion(t, store, controller, "batch-upgrade", "run-a", 100, "alpha.example",
		time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC), "v1.6.3")
	projectRunWithVersion(t, store, controller, "batch-upgrade", "run-b", 101, "beta.example",
		time.Date(2026, 8, 15, 10, 5, 0, 0, time.UTC), "v1.6.6")

	snap, ok := store.GetAnalysisCohortSnapshotByBatch(10, "batch-upgrade")
	if !ok {
		t.Fatal("expected snapshot")
	}
	if !snap.MixedEngineVersion {
		t.Fatal("runs on different engine versions must set the mixed flag")
	}
	if snap.Status == serverpkg.AnalysisSnapshotStatusFailedMixedProfiles {
		t.Fatal("a mixed engine version must not fail the snapshot the way mixed profiles do")
	}
	if !snap.IsPublic {
		t.Fatal("a mixed-version snapshot stays public; it is flagged, not hidden")
	}
	if snap.EngineVersion == "" {
		t.Fatal("a mixed snapshot still keeps a sample version for display")
	}
}

// Runs that predate provenance stamping must report unknown rather than
// inheriting the version of whatever binary happens to be running.
func TestControllerProjectRunUnknownEngineVersionStaysEmpty(t *testing.T) {
	store, _ := snapshotLifecycleStore(t)
	seedSnapshotBatch(store, "batch-legacy", true)
	controller := NewController(store)

	projectRunWithVersion(t, store, controller, "batch-legacy", "run-a", 100, "alpha.example",
		time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC), "")

	snap, ok := store.GetAnalysisCohortSnapshotByBatch(10, "batch-legacy")
	if !ok {
		t.Fatal("expected snapshot")
	}
	if snap.EngineVersion != "" {
		t.Fatalf("EngineVersion = %q, want empty for a run with no GLOBAL_VERSION", snap.EngineVersion)
	}
	if snap.MixedEngineVersion {
		t.Fatal("an unknown version is not a disagreement")
	}
}

// One unreadable run should not permanently blank the snapshot's provenance:
// a later run in the same batch can still supply it, and that is a gap being
// filled rather than a conflict.
func TestControllerProjectRunLearnsEngineVersionFromLaterRun(t *testing.T) {
	store, _ := snapshotLifecycleStore(t)
	seedSnapshotBatch(store, "batch-late", true)
	controller := NewController(store)

	projectRunWithVersion(t, store, controller, "batch-late", "run-a", 100, "alpha.example",
		time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC), "")
	projectRunWithVersion(t, store, controller, "batch-late", "run-b", 101, "beta.example",
		time.Date(2026, 8, 15, 10, 5, 0, 0, time.UTC), "v1.6.6")

	snap, ok := store.GetAnalysisCohortSnapshotByBatch(10, "batch-late")
	if !ok {
		t.Fatal("expected snapshot")
	}
	if snap.EngineVersion != "v1.6.6" {
		t.Fatalf("EngineVersion = %q, want v1.6.6 learned from the later run", snap.EngineVersion)
	}
	if snap.MixedEngineVersion {
		t.Fatal("filling an unknown version is not a version disagreement")
	}
}

// testEffectiveProfile is the shape a run stores: a full profile of which
// only test_levels is provenance.
const testEffectiveProfile = `{"net":{"ipv6":true},"test_levels":{"DNSSEC":{"DS02_NO_MATCHING_DNSKEY_RRSIG":"ERROR"},"ZONE":{"Z15_NO_CAA":"NOTICE"}}}`

// testVocabulary is what capture stores for testEffectiveProfile.
const testVocabulary = `{"DNSSEC":{"DS02_NO_MATCHING_DNSKEY_RRSIG":"ERROR"},"ZONE":{"Z15_NO_CAA":"NOTICE"}}`

func TestRunVocabulary(t *testing.T) {
	tests := []struct {
		name             string
		effectiveProfile string
		want             string
	}{
		{name: "no effective profile", effectiveProfile: "", want: ""},
		{name: "whitespace only", effectiveProfile: "   ", want: ""},
		{name: "unparseable JSON", effectiveProfile: "{", want: ""},
		{name: "unknown property", effectiveProfile: `{"not_a_property":1}`, want: ""},
		{name: "no test_levels", effectiveProfile: `{"net":{"ipv6":true}}`, want: ""},
		{name: "test_levels present", effectiveProfile: testEffectiveProfile, want: testVocabulary},
		{
			// Modules and tags come out sorted, so two captures of the
			// same vocabulary compare equal byte for byte.
			name:             "key order is canonical",
			effectiveProfile: `{"test_levels":{"ZONE":{"Z15_NO_CAA":"NOTICE"},"DNSSEC":{"DS02_NO_MATCHING_DNSKEY_RRSIG":"ERROR"}}}`,
			want:             testVocabulary,
		},
		{
			// Retired tag identifiers migrate on parse, as they do
			// everywhere else a stored profile is read.
			name:             "renamed tag migrates",
			effectiveProfile: `{"test_levels":{"NAMESERVER":{"IN_BAILIWICK_ADDR_MISMATCH":"WARNING"}}}`,
			want:             `{"NAMESERVER":{"IN_DOMAIN_ADDR_MISMATCH":"WARNING"}}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			run := serverpkg.Run{ID: "run-1", EffectiveProfile: tc.effectiveProfile}
			if got := runVocabulary(run); got != tc.want {
				t.Fatalf("runVocabulary = %q, want %q", got, tc.want)
			}
		})
	}
}

// scoringConfigHashWarning7 is the sha256 of `{"severity_penalties":{"WARNING":7}}`.
const scoringConfigHashWarning7 = "b522aeef6c4386637f21ec3bc705332200dbbc41f4f5d0e86d8533ab48616687"

func TestScoringConfigIdentity(t *testing.T) {
	store, _ := snapshotLifecycleStore(t)
	controller := NewController(store)

	if got := controller.scoringConfigIdentity(); got != scoringConfigDefault {
		t.Fatalf("unset setting = %q, want %q", got, scoringConfigDefault)
	}
	if err := store.SetSetting(scoringConfigSetting, "   "); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	if got := controller.scoringConfigIdentity(); got != scoringConfigDefault {
		t.Fatalf("blank setting = %q, want %q", got, scoringConfigDefault)
	}

	if err := store.SetSetting(scoringConfigSetting, `{"severity_penalties":{"WARNING":7}}`); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	first := controller.scoringConfigIdentity()
	if first != scoringConfigHashWarning7 {
		t.Fatalf("hash = %q, want %q", first, scoringConfigHashWarning7)
	}
	if controller.scoringConfigIdentity() != first {
		t.Fatal("expected the hash to be stable for one configuration")
	}
	if err := store.SetSetting(scoringConfigSetting, `{"severity_penalties":{"WARNING":8}}`); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	if controller.scoringConfigIdentity() == first {
		t.Fatal("expected a different configuration to hash differently")
	}
}

// seedRunWithProfile adds an unprojected run carrying an effective profile,
// which is what the backfill reads.
func seedRunWithProfile(store *fakeStore, batchID, runID string, domainID int64, domain string, at time.Time, effectiveProfile string) {
	seedRunWithVersion(store, batchID, runID, domainID, domain, at, "v1.7.10")
	run := store.runs[runID]
	run.EffectiveProfile = effectiveProfile
	store.runs[runID] = run
}

func TestControllerProjectRunStampsVocabularyAndScoringHash(t *testing.T) {
	store, _ := snapshotLifecycleStore(t)
	seedSnapshotBatch(store, "batch-vocab", true)
	if err := store.SetSetting(scoringConfigSetting, `{"severity_penalties":{"WARNING":7}}`); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	controller := NewController(store)

	run := testAnalysisRun("run-a", 100, "alpha.example",
		time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC), "192.0.2.10", "2001:db8::10")
	run.BatchID = "batch-vocab"
	run.EffectiveProfile = testEffectiveProfile
	entries := withGlobalVersion(testAnalysisEntries(run), run, "v1.7.10")
	run.EntryCount = len(entries)
	store.runs[run.ID] = run
	store.entries[run.ID] = entries
	store.tags[run.DomainID] = []string{"tld"}
	if err := controller.ProjectRun(run.ID); err != nil {
		t.Fatalf("ProjectRun: %v", err)
	}

	snap, ok := store.GetAnalysisCohortSnapshotByBatch(10, "batch-vocab")
	if !ok {
		t.Fatal("expected snapshot")
	}
	if snap.Vocabulary != testVocabulary {
		t.Fatalf("Vocabulary = %q, want %q", snap.Vocabulary, testVocabulary)
	}
	if snap.ScoringConfigHash != scoringConfigHashWarning7 {
		t.Fatalf("ScoringConfigHash = %q, want %q", snap.ScoringConfigHash, scoringConfigHashWarning7)
	}
}

func TestControllerProjectRunStampsDefaultScoringHash(t *testing.T) {
	store, _ := snapshotLifecycleStore(t)
	seedSnapshotBatch(store, "batch-default", true)
	controller := NewController(store)

	projectRunWithVersion(t, store, controller, "batch-default", "run-a", 100, "alpha.example",
		time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC), "v1.7.10")

	snap, _ := store.GetAnalysisCohortSnapshotByBatch(10, "batch-default")
	if snap.ScoringConfigHash != scoringConfigDefault {
		t.Fatalf("ScoringConfigHash = %q, want %q", snap.ScoringConfigHash, scoringConfigDefault)
	}
	// The fixture run carries no effective profile, so the vocabulary is
	// unknown rather than invented from the running build.
	if snap.Vocabulary != "" {
		t.Fatalf("Vocabulary = %q, want empty", snap.Vocabulary)
	}
}

func TestBackfillSnapshotVocabulariesStampsCapturedSnapshots(t *testing.T) {
	store, _ := snapshotLifecycleStore(t)
	seedSnapshotBatch(store, "batch-old", true)
	seedRunWithProfile(store, "batch-old", "run-a", 100, "alpha.example",
		time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC), testEffectiveProfile)
	seedUnstampedSnapshot(t, store, "batch-old", "2026-06-03-old")

	controller := NewController(store)
	if err := controller.BackfillSnapshotVocabularies(t.Context()); err != nil {
		t.Fatalf("BackfillSnapshotVocabularies: %v", err)
	}

	snap, ok := store.GetAnalysisCohortSnapshotByBatch(10, "batch-old")
	if !ok {
		t.Fatal("expected snapshot")
	}
	if snap.Vocabulary != testVocabulary {
		t.Fatalf("Vocabulary = %q, want %q recovered from a stored run", snap.Vocabulary, testVocabulary)
	}
	// What was in force at capture is not recoverable, so it stays unknown.
	if snap.ScoringConfigHash != "" {
		t.Fatalf("ScoringConfigHash = %q, want empty: it is not backfillable", snap.ScoringConfigHash)
	}
}

// The first run of a batch may have no effective profile; a later one still
// answers for the batch, because mixed-profile batches fail capture.
func TestBackfillSnapshotVocabulariesSkipsRunsWithoutProfile(t *testing.T) {
	store, _ := snapshotLifecycleStore(t)
	seedSnapshotBatch(store, "batch-partial", true)
	seedRunWithProfile(store, "batch-partial", "run-a", 100, "alpha.example",
		time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC), "")
	seedRunWithProfile(store, "batch-partial", "run-b", 101, "beta.example",
		time.Date(2026, 6, 3, 10, 5, 0, 0, time.UTC), testEffectiveProfile)
	seedUnstampedSnapshot(t, store, "batch-partial", "2026-06-03-partial")

	controller := NewController(store)
	if err := controller.BackfillSnapshotVocabularies(t.Context()); err != nil {
		t.Fatalf("BackfillSnapshotVocabularies: %v", err)
	}

	snap, _ := store.GetAnalysisCohortSnapshotByBatch(10, "batch-partial")
	if snap.Vocabulary != testVocabulary {
		t.Fatalf("Vocabulary = %q, want %q", snap.Vocabulary, testVocabulary)
	}
}

// A snapshot whose runs are gone stays unknown, which the report reads as
// "cannot classify", never as "nothing changed".
func TestBackfillSnapshotVocabulariesLeavesUnrecoverableUnknown(t *testing.T) {
	store, _ := snapshotLifecycleStore(t)
	seedSnapshotBatch(store, "batch-purged", true)
	seedUnstampedSnapshot(t, store, "batch-purged", "2026-06-03-purged")

	controller := NewController(store)
	if err := controller.BackfillSnapshotVocabularies(t.Context()); err != nil {
		t.Fatalf("BackfillSnapshotVocabularies: %v", err)
	}

	snap, _ := store.GetAnalysisCohortSnapshotByBatch(10, "batch-purged")
	if snap.Vocabulary != "" {
		t.Fatalf("Vocabulary = %q, want empty when no run survives", snap.Vocabulary)
	}
}

// Backfill runs on every startup, so it must not rewrite a vocabulary that
// capture already recorded.
func TestBackfillSnapshotVocabulariesIsIdempotent(t *testing.T) {
	store, _ := snapshotLifecycleStore(t)
	seedSnapshotBatch(store, "batch-old", true)
	seedRunWithProfile(store, "batch-old", "run-a", 100, "alpha.example",
		time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC), testEffectiveProfile)
	if _, err := store.UpsertAnalysisCohortSnapshot(serverpkg.AnalysisCohortSnapshot{
		CohortID:   10,
		BatchID:    "batch-old",
		Slug:       "2026-06-03-old",
		Status:     serverpkg.AnalysisSnapshotStatusCaptured,
		IsPublic:   true,
		Vocabulary: `{"ZONE":{"Z01_SOA_OK":"INFO"}}`,
	}); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}

	controller := NewController(store)
	for i := range 3 {
		if err := controller.BackfillSnapshotVocabularies(t.Context()); err != nil {
			t.Fatalf("BackfillSnapshotVocabularies pass %d: %v", i, err)
		}
	}

	snap, _ := store.GetAnalysisCohortSnapshotByBatch(10, "batch-old")
	if snap.Vocabulary != `{"ZONE":{"Z01_SOA_OK":"INFO"}}` {
		t.Fatalf("backfill overwrote a captured vocabulary: %q", snap.Vocabulary)
	}
}

// A rebuild recovers the vocabulary from the runs and leaves the scoring hash unknown.
func TestRebuildCohortRecoversVocabularyButNotScoringHash(t *testing.T) {
	store, _ := snapshotLifecycleStore(t)
	seedSnapshotBatch(store, "batch-rebuilt", true)
	if err := store.SetSetting(scoringConfigSetting, `{"severity_penalties":{"WARNING":7}}`); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	seedRunWithProfile(store, "batch-rebuilt", "run-a", 100, "alpha.example",
		time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC), testEffectiveProfile)

	controller := NewController(store)
	if err := controller.RebuildCohort(t.Context(), 10); err != nil {
		t.Fatalf("RebuildCohort: %v", err)
	}

	snap, ok := store.GetAnalysisCohortSnapshotByBatch(10, "batch-rebuilt")
	if !ok {
		t.Fatal("expected snapshot")
	}
	if snap.Vocabulary != testVocabulary {
		t.Fatalf("Vocabulary = %q, want %q recovered from the run", snap.Vocabulary, testVocabulary)
	}
	if snap.ScoringConfigHash != "" {
		t.Fatalf("ScoringConfigHash = %q, want empty on a rebuild", snap.ScoringConfigHash)
	}
}

// A later projection never replaces an unknown scoring hash.
func TestApplySnapshotStateNeverBackfillsScoringHash(t *testing.T) {
	store, _ := snapshotLifecycleStore(t)
	seedSnapshotBatch(store, "batch-unknown", true)
	if _, err := store.UpsertAnalysisCohortSnapshot(serverpkg.AnalysisCohortSnapshot{
		CohortID: 10,
		BatchID:  "batch-unknown",
		Slug:     "2026-06-03-unknown",
		Status:   serverpkg.AnalysisSnapshotStatusCaptured,
		IsPublic: true,
	}); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	if err := store.SetSetting(scoringConfigSetting, `{"severity_penalties":{"WARNING":7}}`); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}

	controller := NewController(store)
	projectRunWithVersion(t, store, controller, "batch-unknown", "run-a", 100, "alpha.example",
		time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC), "v1.7.10")

	snap, _ := store.GetAnalysisCohortSnapshotByBatch(10, "batch-unknown")
	if snap.ScoringConfigHash != "" {
		t.Fatalf("ScoringConfigHash = %q, want the unknown state preserved", snap.ScoringConfigHash)
	}
}
