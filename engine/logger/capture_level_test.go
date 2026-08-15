package logger

import (
	"testing"

	"codeberg.org/pawal/gonemaster/engine/profile"
)

// captureTestLogger returns a logger whose System module levels mirror the
// shipped profile closely enough for the capture tests: one tag per level of
// interest, so a test can name the level it wants without depending on the
// exact contents of share/profile.json.
func captureTestLogger(t *testing.T) *Logger {
	t.Helper()
	log := New()
	log.configMu.Lock()
	log.testLevels = map[string]map[string]string{
		"SYSTEM": {
			"CACHED_RETURN":  "DEBUG3",
			"QUERY":          "DEBUG2",
			"EXTERNAL_QUERY": "DEBUG",
			"KEEP_ME":        "NOTICE",
			"LOUD":           "ERROR",
		},
	}
	log.configMu.Unlock()
	return log
}

func tagsOf(entries []*Entry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry != nil {
			out = append(out, entry.Tag)
		}
	}
	return out
}

func TestCaptureLevelRetainsOnlyAtOrAboveTheFloor(t *testing.T) {
	log := captureTestLogger(t)
	if err := log.SetCaptureLevel("DEBUG"); err != nil {
		t.Fatalf("set capture level: %v", err)
	}

	for _, tag := range []string{"CACHED_RETURN", "QUERY", "EXTERNAL_QUERY", "KEEP_ME", "LOUD"} {
		if _, err := log.Add(tag, nil, "System", "Unspecified"); err != nil {
			t.Fatalf("add %s: %v", tag, err)
		}
	}

	got := tagsOf(log.Entries())
	want := []string{"EXTERNAL_QUERY", "KEEP_ME", "LOUD"}
	if len(got) != len(want) {
		t.Fatalf("retained %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("retained %v, want %v", got, want)
		}
	}
}

func TestCaptureLevelEmptyKeepsEverything(t *testing.T) {
	log := captureTestLogger(t)
	// An explicit reset has to undo a previously set floor, not just be ignored.
	if err := log.SetCaptureLevel("NOTICE"); err != nil {
		t.Fatalf("set capture level: %v", err)
	}
	if err := log.SetCaptureLevel(""); err != nil {
		t.Fatalf("reset capture level: %v", err)
	}
	if _, err := log.Add("CACHED_RETURN", nil, "System", "Unspecified"); err != nil {
		t.Fatalf("add entry: %v", err)
	}
	if got := len(log.Entries()); got != 1 {
		t.Fatalf("retained %d entries, want 1", got)
	}
}

func TestCaptureLevelRejectsUnknownLevel(t *testing.T) {
	log := captureTestLogger(t)
	if err := log.SetCaptureLevel("VERBOSE"); err == nil {
		t.Fatal("an unknown level should be rejected")
	}
	if _, err := log.Add("CACHED_RETURN", nil, "System", "Unspecified"); err != nil {
		t.Fatalf("add entry: %v", err)
	}
	if got := len(log.Entries()); got != 1 {
		t.Fatalf("a rejected level must leave the logger capturing everything, retained %d", got)
	}
}

// A dropped entry is still handed back to the caller: testcases append the
// returned entry to their own result slice, and that slice is what the run
// returns, so dropping from the logger must not turn into a nil dereference or
// a missing result.
func TestCaptureLevelStillReturnsDroppedEntry(t *testing.T) {
	log := captureTestLogger(t)
	if err := log.SetCaptureLevel("NOTICE"); err != nil {
		t.Fatalf("set capture level: %v", err)
	}
	entry, err := log.Add("CACHED_RETURN", map[string]any{"packet": "rendered"}, "System", "Unspecified")
	if err != nil {
		t.Fatalf("add entry: %v", err)
	}
	if entry == nil {
		t.Fatal("a dropped entry must still be returned")
	}
	if entry.Tag != "CACHED_RETURN" || entry.Args["packet"] != "rendered" {
		t.Fatalf("unexpected returned entry: %+v", entry)
	}
	if got := len(log.Entries()); got != 0 {
		t.Fatalf("retained %d entries, want 0", got)
	}
}

func TestCaptureLevelSkipsCallbacksForDroppedEntries(t *testing.T) {
	log := captureTestLogger(t)
	if err := log.SetCaptureLevel("DEBUG"); err != nil {
		t.Fatalf("set capture level: %v", err)
	}
	var seen []string
	log.Callback = func(entry *Entry) error {
		seen = append(seen, entry.Tag)
		return nil
	}

	for _, tag := range []string{"CACHED_RETURN", "EXTERNAL_QUERY"} {
		if _, err := log.Add(tag, nil, "System", "Unspecified"); err != nil {
			t.Fatalf("add %s: %v", tag, err)
		}
	}

	if len(seen) != 1 || seen[0] != "EXTERNAL_QUERY" {
		t.Fatalf("callback saw %v, want [EXTERNAL_QUERY]", seen)
	}
}

// Log filters run before the capture check and can raise an entry's level from
// its arguments. An entry a filter promotes is one the operator asked to see,
// so it must survive a floor that would otherwise drop its tag.
func TestCaptureLevelKeepsEntriesPromotedByLogFilter(t *testing.T) {
	log := captureTestLogger(t)
	log.configMu.Lock()
	log.logFilter = map[string]map[string][]profile.LogFilterRule{
		"SYSTEM": {
			"CACHED_RETURN": {{When: map[string]any{"packet": "interesting"}, Set: "WARNING"}},
		},
	}
	log.configMu.Unlock()
	if err := log.SetCaptureLevel("NOTICE"); err != nil {
		t.Fatalf("set capture level: %v", err)
	}

	if _, err := log.Add("CACHED_RETURN", map[string]any{"packet": "interesting"}, "System", "Unspecified"); err != nil {
		t.Fatalf("add promoted entry: %v", err)
	}
	if _, err := log.Add("CACHED_RETURN", map[string]any{"packet": "boring"}, "System", "Unspecified"); err != nil {
		t.Fatalf("add unpromoted entry: %v", err)
	}

	entries := log.Entries()
	if len(entries) != 1 {
		t.Fatalf("retained %d entries, want 1 (only the promoted one)", len(entries))
	}
	if entries[0].Level() != "WARNING" || entries[0].Args["packet"] != "interesting" {
		t.Fatalf("unexpected retained entry: %+v", entries[0])
	}
}

func TestWantsReportsWhatTheFloorWouldKeep(t *testing.T) {
	cases := []struct {
		name    string
		floor   string
		filter  bool
		module  string
		tag     string
		want    bool
		comment string
	}{
		{name: "no floor captures everything", floor: "", module: "System", tag: "CACHED_RETURN", want: true},
		{name: "tag below the floor", floor: "DEBUG", module: "System", tag: "CACHED_RETURN", want: false},
		{name: "tag at the floor", floor: "DEBUG", module: "System", tag: "EXTERNAL_QUERY", want: true},
		{name: "tag above the floor", floor: "DEBUG", module: "System", tag: "LOUD", want: true},
		{name: "unknown tag defaults to DEBUG and is kept", floor: "DEBUG", module: "System", tag: "MYSTERY", want: true},
		{name: "unknown tag defaults to DEBUG and is dropped", floor: "NOTICE", module: "System", tag: "MYSTERY", want: false},
		{name: "unknown module defaults to DEBUG", floor: "NOTICE", module: "Basic", tag: "CACHED_RETURN", want: false},
		{name: "a filter rule keeps the tag capturable", floor: "NOTICE", filter: true, module: "System", tag: "CACHED_RETURN", want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			log := captureTestLogger(t)
			if tc.filter {
				log.configMu.Lock()
				log.logFilter = map[string]map[string][]profile.LogFilterRule{
					"SYSTEM": {"CACHED_RETURN": {{Set: "WARNING"}}},
				}
				log.configMu.Unlock()
			}
			if err := log.SetCaptureLevel(tc.floor); err != nil {
				t.Fatalf("set capture level: %v", err)
			}
			if got := log.Wants(tc.module, tc.tag); got != tc.want {
				t.Errorf("Wants(%q, %q) = %v, want %v", tc.module, tc.tag, got, tc.want)
			}
		})
	}
}

// Every testcase runs against its own buffer logger built with CopyConfigFrom.
// If the capture level did not travel with the rest of the config, the gate
// would be inert exactly where the queries happen.
func TestCopyConfigFromCarriesTheCaptureLevel(t *testing.T) {
	parent := captureTestLogger(t)
	if err := parent.SetCaptureLevel("NOTICE"); err != nil {
		t.Fatalf("set capture level: %v", err)
	}

	buf := New()
	buf.CopyConfigFrom(parent)

	if buf.Wants("System", "CACHED_RETURN") {
		t.Error("the buffer logger should have inherited the floor")
	}
	if _, err := buf.Add("CACHED_RETURN", nil, "System", "Unspecified"); err != nil {
		t.Fatalf("add entry: %v", err)
	}
	if got := len(buf.Entries()); got != 0 {
		t.Fatalf("buffer retained %d entries, want 0", got)
	}
	if _, err := buf.Add("LOUD", nil, "System", "Unspecified"); err != nil {
		t.Fatalf("add entry: %v", err)
	}
	if got := len(buf.Entries()); got != 1 {
		t.Fatalf("buffer retained %d entries, want 1", got)
	}
}
