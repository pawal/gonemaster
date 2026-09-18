package profile

import (
	"encoding/json"
	"testing"
)

// baseProfile builds a small stand-in for the engine default profile so the
// diff tests stay readable and independent of the shipped default JSON.
func baseProfile(t *testing.T) *Profile {
	t.Helper()
	p := New()
	set := func(path string, value any) {
		t.Helper()
		if err := p.Set(path, value); err != nil {
			t.Fatalf("set %s: %v", path, err)
		}
	}
	set("resolver.defaults.timeout", 5)
	set("resolver.defaults.nameserver_max_total_ms", 0)
	set("net.ipv4", true)
	set("net.ipv6", true)
	set("badkeys.path", "")
	set("asn_db.sources", map[string]any{"Cymru": []any{"asnlookup.example"}})
	set("test_cases", []any{"basic01", "basic02", "zone13"})
	set("test_levels", map[string]any{
		"BASIC": map[string]any{
			"B01_CHILD_FOUND":    "INFO",
			"B01_CHILD_IS_ALIAS": "NOTICE",
		},
		"ZONE": map[string]any{
			"Z01_SOA_OK": "INFO",
		},
	})
	set("test_cases_vars.zone02.SOA_REFRESH_MINIMUM_VALUE", 14400)
	set("test_cases_vars.zone13.SPF_LOOKUP_LIMIT", 10)
	return p
}

// overrideFrom parses a profile override the way a stored profile config is
// parsed, so only the properties present in the JSON count as set.
func overrideFrom(t *testing.T, configJSON string) *Profile {
	t.Helper()
	p, err := FromJSON(configJSON)
	if err != nil {
		t.Fatalf("parse override: %v", err)
	}
	return p
}

// findProperty returns the diff entry for a path, or nil when the diff does
// not mention it at all.
func findProperty(result *DiffResult, path string) *DiffProperty {
	for i := range result.Properties {
		if result.Properties[i].Path == path {
			return &result.Properties[i]
		}
	}
	return nil
}

func TestDiffIgnoresUnsetProperties(t *testing.T) {
	base := baseProfile(t)
	result := Diff(overrideFrom(t, `{}`), base)

	if len(result.Properties) != 0 {
		t.Fatalf("expected no properties for an empty override, got %+v", result.Properties)
	}
	if result.Summary != (DiffSummary{}) {
		t.Fatalf("expected a zero summary, got %+v", result.Summary)
	}
}

func TestDiffScalarChangedAndRedundant(t *testing.T) {
	base := baseProfile(t)
	result := Diff(overrideFrom(t, `{
		"resolver": {"defaults": {"timeout": 5, "nameserver_max_total_ms": 60000}},
		"net": {"ipv6": false}
	}`), base)

	timeout := findProperty(result, "resolver.defaults.timeout")
	if timeout == nil || timeout.Kind != DiffKindRedundant || !timeout.Redundant {
		t.Fatalf("expected timeout to be redundant, got %+v", timeout)
	}

	totalMS := findProperty(result, "resolver.defaults.nameserver_max_total_ms")
	if totalMS == nil || totalMS.Kind != DiffKindChanged || totalMS.Redundant {
		t.Fatalf("expected the total-ms cap to be a deviation, got %+v", totalMS)
	}
	if totalMS.Default != 0 || totalMS.Value != 60000 {
		t.Fatalf("expected default 0 and value 60000, got default %v value %v", totalMS.Default, totalMS.Value)
	}

	// A boolean flipped to false must still report both values; the zero value
	// is a real value here, not an absent one.
	ipv6 := findProperty(result, "net.ipv6")
	if ipv6 == nil || ipv6.Kind != DiffKindChanged {
		t.Fatalf("expected net.ipv6 to be a deviation, got %+v", ipv6)
	}
	if ipv6.Default != true || ipv6.Value != false {
		t.Fatalf("expected default true and value false, got default %v value %v", ipv6.Default, ipv6.Value)
	}

	if result.Summary.Deviations != 2 || result.Summary.Redundant != 1 {
		t.Fatalf("expected 2 deviations and 1 redundant, got %+v", result.Summary)
	}
}

func TestDiffScalarChangedKeepsFalseAndZeroInJSON(t *testing.T) {
	base := baseProfile(t)
	result := Diff(overrideFrom(t, `{"net": {"ipv6": false}}`), base)

	raw, err := json.Marshal(findProperty(result, "net.ipv6"))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := decoded["value"]; !ok {
		t.Fatalf("expected a value key in %s", raw)
	}
	if decoded["value"] != false {
		t.Fatalf("expected value false, got %v", decoded["value"])
	}
}

func TestDiffOpaquePropertyEqualAndDifferent(t *testing.T) {
	base := baseProfile(t)

	same := Diff(overrideFrom(t, `{"asn_db": {"sources": {"Cymru": ["asnlookup.example"]}}}`), base)
	entry := findProperty(same, "asn_db.sources")
	if entry == nil || !entry.Redundant {
		t.Fatalf("expected an identical opaque property to be redundant, got %+v", entry)
	}

	other := Diff(overrideFrom(t, `{"asn_db": {"sources": {"Cymru": ["other.example"]}}}`), base)
	entry = findProperty(other, "asn_db.sources")
	if entry == nil || entry.Kind != DiffKindChanged || entry.Redundant {
		t.Fatalf("expected a differing opaque property to be a deviation, got %+v", entry)
	}
}

func TestDiffTestCasesSet(t *testing.T) {
	base := baseProfile(t)
	result := Diff(overrideFrom(t, `{"test_cases": ["basic01", "zone99"]}`), base)

	entry := findProperty(result, "test_cases")
	if entry == nil || entry.Kind != DiffKindSet {
		t.Fatalf("expected a set entry for test_cases, got %+v", entry)
	}
	if !entry.Wholesale {
		t.Fatal("test_cases replaces the default list wholesale and must say so")
	}
	if entry.Redundant {
		t.Fatal("expected the override to be a deviation")
	}
	if len(entry.Excluded) != 2 || entry.Excluded[0] != "basic02" || entry.Excluded[1] != "zone13" {
		t.Fatalf("expected basic02 and zone13 excluded, got %v", entry.Excluded)
	}
	if len(entry.Unknown) != 1 || entry.Unknown[0] != "zone99" {
		t.Fatalf("expected zone99 unknown, got %v", entry.Unknown)
	}
	// Excluded testcases are a deliberate selection, so they stay out of the
	// missing counter; unknown ones are dead config and are counted.
	if result.Summary.Missing != 0 || result.Summary.Unknown != 1 {
		t.Fatalf("expected 0 missing and 1 unknown, got %+v", result.Summary)
	}
}

func TestDiffTestCasesIdenticalIsRedundant(t *testing.T) {
	base := baseProfile(t)
	// Different order and casing, same set: the engine lowercases testcase
	// names when planning, so this restates the default exactly.
	result := Diff(overrideFrom(t, `{"test_cases": ["ZONE13", "basic02", "basic01"]}`), base)

	entry := findProperty(result, "test_cases")
	if entry == nil || !entry.Redundant {
		t.Fatalf("expected an identical set to be redundant, got %+v", entry)
	}
	if result.Summary.Redundant != 1 || result.Summary.Deviations != 0 {
		t.Fatalf("expected 1 redundant and 0 deviations, got %+v", result.Summary)
	}
}

func TestDiffTestLevelsChangedMissingUnknown(t *testing.T) {
	base := baseProfile(t)
	result := Diff(overrideFrom(t, `{"test_levels": {
		"BASIC": {"B01_CHILD_FOUND": "WARNING", "B01_GONE": "ERROR"},
		"ZONE": {"Z01_SOA_OK": "INFO"}
	}}`), base)

	entry := findProperty(result, "test_levels")
	if entry == nil || entry.Kind != DiffKindMap || !entry.Wholesale {
		t.Fatalf("expected a wholesale map entry, got %+v", entry)
	}
	if entry.Redundant {
		t.Fatal("expected the override to be a deviation")
	}
	// ZONE matches the default exactly, so only BASIC is reported.
	if len(entry.Modules) != 1 || entry.Modules[0].Module != "BASIC" {
		t.Fatalf("expected only BASIC reported, got %+v", entry.Modules)
	}
	basic := entry.Modules[0]
	if len(basic.Changed) != 1 || basic.Changed[0].Key != "B01_CHILD_FOUND" {
		t.Fatalf("expected B01_CHILD_FOUND changed, got %+v", basic.Changed)
	}
	if basic.Changed[0].Default != "INFO" || basic.Changed[0].Value != "WARNING" {
		t.Fatalf("expected INFO to WARNING, got %+v", basic.Changed[0])
	}
	if len(basic.Missing) != 1 || basic.Missing[0] != "B01_CHILD_IS_ALIAS" {
		t.Fatalf("expected B01_CHILD_IS_ALIAS missing, got %v", basic.Missing)
	}
	if len(basic.Unknown) != 1 || basic.Unknown[0] != "B01_GONE" {
		t.Fatalf("expected B01_GONE unknown, got %v", basic.Unknown)
	}
	if result.Summary.Missing != 1 || result.Summary.Unknown != 1 {
		t.Fatalf("expected 1 missing and 1 unknown, got %+v", result.Summary)
	}
}

func TestDiffTestLevelsMissingModuleReportsEveryTag(t *testing.T) {
	base := baseProfile(t)
	// Overriding test_levels replaces the whole table, so an omitted module
	// drops all of its tags to DEBUG.
	result := Diff(overrideFrom(t, `{"test_levels": {
		"BASIC": {"B01_CHILD_FOUND": "INFO", "B01_CHILD_IS_ALIAS": "NOTICE"}
	}}`), base)

	entry := findProperty(result, "test_levels")
	if entry == nil || entry.Redundant {
		t.Fatalf("expected a deviation, got %+v", entry)
	}
	if len(entry.Modules) != 1 || entry.Modules[0].Module != "ZONE" {
		t.Fatalf("expected the omitted ZONE module reported, got %+v", entry.Modules)
	}
	if len(entry.Modules[0].Missing) != 1 || entry.Modules[0].Missing[0] != "Z01_SOA_OK" {
		t.Fatalf("expected Z01_SOA_OK missing, got %v", entry.Modules[0].Missing)
	}
}

func TestDiffTestLevelsIdenticalIsRedundant(t *testing.T) {
	base := baseProfile(t)
	result := Diff(overrideFrom(t, `{"test_levels": {
		"BASIC": {"B01_CHILD_FOUND": "info", "B01_CHILD_IS_ALIAS": "NOTICE"},
		"ZONE": {"Z01_SOA_OK": "INFO"}
	}}`), base)

	entry := findProperty(result, "test_levels")
	if entry == nil || !entry.Redundant {
		t.Fatalf("expected a restated table to be redundant, got %+v", entry)
	}
	if len(entry.Modules) != 0 {
		t.Fatalf("expected no modules reported, got %+v", entry.Modules)
	}
}

func TestDiffTestCasesVarsPerVar(t *testing.T) {
	base := baseProfile(t)
	// zone02 restates the default, zone13 is not restated at all. Only the
	// modules with something to report appear.
	result := Diff(overrideFrom(t, `{"test_cases_vars": {
		"zone02": {"SOA_REFRESH_MINIMUM_VALUE": 14400},
		"zone06": {"SOA_DEFAULT_TTL_MAXIMUM_VALUE": 3600}
	}}`), base)

	entry := findProperty(result, "test_cases_vars")
	if entry == nil || entry.Kind != DiffKindMap {
		t.Fatalf("expected a map entry for test_cases_vars, got %+v", entry)
	}
	if entry.Wholesale {
		t.Fatal("each tunable is its own property, so the map is not wholesale-replaced")
	}
	if entry.Redundant {
		t.Fatal("zone06 differs from the default, so the property is a deviation")
	}
	if len(entry.Modules) != 1 || entry.Modules[0].Module != "zone06" {
		t.Fatalf("expected only zone06 reported, got %+v", entry.Modules)
	}
	changed := entry.Modules[0].Changed
	if len(changed) != 1 || changed[0].Key != "SOA_DEFAULT_TTL_MAXIMUM_VALUE" || changed[0].Value != 3600 {
		t.Fatalf("expected the zone06 maximum TTL changed, got %+v", changed)
	}
	// zone06 also has a minimum the override does not restate.
	if len(entry.Modules[0].Missing) != 1 || entry.Modules[0].Missing[0] != "SOA_DEFAULT_TTL_MINIMUM_VALUE" {
		t.Fatalf("expected the zone06 minimum reported missing, got %v", entry.Modules[0].Missing)
	}
}

func TestDiffTestCasesVarsRedundantDespiteMissingKeys(t *testing.T) {
	base := baseProfile(t)
	// zone13 is not restated, but nothing that is restated differs. Dropping
	// the whole property leaves the merged profile identical, so it is safe to
	// strip and reported redundant.
	result := Diff(overrideFrom(t, `{"test_cases_vars": {
		"zone02": {"SOA_REFRESH_MINIMUM_VALUE": 14400}
	}}`), base)

	entry := findProperty(result, "test_cases_vars")
	if entry == nil || !entry.Redundant {
		t.Fatalf("expected the property to be redundant, got %+v", entry)
	}
	if len(entry.Modules) != 0 {
		t.Fatalf("expected no modules reported, got %+v", entry.Modules)
	}
	if result.Summary.Redundant != 1 {
		t.Fatalf("expected 1 redundant property, got %+v", result.Summary)
	}
}

func TestDiffTestCasesVarsAbsentIsNotReported(t *testing.T) {
	base := baseProfile(t)
	result := Diff(overrideFrom(t, `{"net": {"ipv4": true}}`), base)

	if entry := findProperty(result, "test_cases_vars"); entry != nil {
		t.Fatalf("expected no test_cases_vars entry, got %+v", entry)
	}
}

func TestDiffAgainstShippedDefaultIsClean(t *testing.T) {
	base, err := Default()
	if err != nil {
		t.Fatalf("default profile: %v", err)
	}
	configJSON, err := base.ToJSON()
	if err != nil {
		t.Fatalf("serialize default: %v", err)
	}

	result := Diff(overrideFrom(t, configJSON), base)

	if result.Summary.Deviations != 0 {
		var paths []string
		for _, entry := range result.Properties {
			if !entry.Redundant {
				paths = append(paths, entry.Path)
			}
		}
		t.Fatalf("expected the default profile to restate itself exactly, deviations in %v", paths)
	}
	if result.Summary.Missing != 0 || result.Summary.Unknown != 0 {
		t.Fatalf("expected no missing or unknown keys, got %+v", result.Summary)
	}
	if result.Summary.Redundant == 0 {
		t.Fatal("expected the restated default to report redundant properties")
	}
}

func TestDiffNilOverride(t *testing.T) {
	result := Diff(nil, baseProfile(t))
	if len(result.Properties) != 0 {
		t.Fatalf("expected no properties, got %+v", result.Properties)
	}
}

// vocabulary builds a profile carrying only a test_levels table.
func vocabulary(levels map[string]map[string]string) *Profile {
	p := New()
	p.TestLevels = levels
	return p
}

func TestDiffTestLevels(t *testing.T) {
	june := vocabulary(map[string]map[string]string{
		"DNSSEC": {"DS02_NO_MATCHING_DNSKEY_RRSIG": "WARNING"},
		"ZONE":   {"Z01_SOA_OK": "INFO"},
	})
	september := vocabulary(map[string]map[string]string{
		"DNSSEC": {"DS02_NO_MATCHING_DNSKEY_RRSIG": "ERROR"},
		"ZONE":   {"Z15_NO_CAA": "NOTICE"},
	})

	t.Run("added", func(t *testing.T) {
		diff := DiffTestLevels(june, september)
		if len(diff.Added) != 1 {
			t.Fatalf("added: got %d, want 1: %+v", len(diff.Added), diff.Added)
		}
		want := TestLevelEntry{Module: "ZONE", Tag: "Z15_NO_CAA", Level: "NOTICE"}
		if diff.Added[0] != want {
			t.Fatalf("added: got %+v, want %+v", diff.Added[0], want)
		}
	})

	t.Run("removed", func(t *testing.T) {
		diff := DiffTestLevels(june, september)
		if len(diff.Removed) != 1 {
			t.Fatalf("removed: got %d, want 1: %+v", len(diff.Removed), diff.Removed)
		}
		want := TestLevelEntry{Module: "ZONE", Tag: "Z01_SOA_OK", Level: "INFO"}
		if diff.Removed[0] != want {
			t.Fatalf("removed: got %+v, want %+v", diff.Removed[0], want)
		}
	})

	t.Run("level changed", func(t *testing.T) {
		diff := DiffTestLevels(june, september)
		if len(diff.LevelChanged) != 1 {
			t.Fatalf("level changed: got %d, want 1: %+v", len(diff.LevelChanged), diff.LevelChanged)
		}
		want := TestLevelChange{
			Module: "DNSSEC", Tag: "DS02_NO_MATCHING_DNSKEY_RRSIG", From: "WARNING", To: "ERROR",
		}
		if diff.LevelChanged[0] != want {
			t.Fatalf("level changed: got %+v, want %+v", diff.LevelChanged[0], want)
		}
	})

	t.Run("identical vocabularies", func(t *testing.T) {
		diff := DiffTestLevels(june, june)
		if !diff.Empty() {
			t.Fatalf("expected an empty diff, got %+v", diff)
		}
	})
}

func TestDiffTestLevelsWholeModules(t *testing.T) {
	a := vocabulary(map[string]map[string]string{
		"BASIC": {"B01_CHILD_FOUND": "INFO"},
	})
	b := vocabulary(map[string]map[string]string{
		"BASIC":        {"B01_CHILD_FOUND": "info"},
		"CONNECTIVITY": {"CN05_NO_RESPONSE": "ERROR", "CN05_OK": "INFO"},
	})
	diff := DiffTestLevels(a, b)
	if len(diff.Added) != 2 || len(diff.Removed) != 0 {
		t.Fatalf("got %+v", diff)
	}
	if diff.Added[0].Tag != "CN05_NO_RESPONSE" || diff.Added[1].Tag != "CN05_OK" {
		t.Fatalf("expected the added tags sorted: %+v", diff.Added)
	}
	// Level case is not a change.
	if len(diff.LevelChanged) != 0 {
		t.Fatalf("level changed: got %+v, want none", diff.LevelChanged)
	}
}

func TestDiffTestLevelsNilProfile(t *testing.T) {
	only := vocabulary(map[string]map[string]string{"ZONE": {"Z01_SOA_OK": "INFO"}})
	if diff := DiffTestLevels(nil, only); len(diff.Added) != 1 || len(diff.Removed) != 0 {
		t.Fatalf("nil from: got %+v", diff)
	}
	if diff := DiffTestLevels(only, nil); len(diff.Removed) != 1 || len(diff.Added) != 0 {
		t.Fatalf("nil to: got %+v", diff)
	}
	if !DiffTestLevels(nil, nil).Empty() {
		t.Fatal("expected two nil profiles to produce an empty diff")
	}
}
