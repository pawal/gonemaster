package analysis

import (
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
