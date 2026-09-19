package profile

import (
	"maps"
	"reflect"
	"slices"
	"strings"
)

// Diff entry kinds.
const (
	DiffKindChanged   = "changed"   // scalar or opaque property differing from the default
	DiffKindRedundant = "redundant" // scalar or opaque property restating the default
	DiffKindSet       = "set"       // order-insensitive set property (test_cases)
	DiffKindMap       = "map"       // keyed map property (test_levels, test_cases_vars)
)

const testCasesVarsPrefix = "test_cases_vars."

// DiffKey is one key whose value differs between override and default.
type DiffKey struct {
	Key     string `json:"key"`
	Default any    `json:"default"`
	Value   any    `json:"value"`
}

// DiffModule collects the per-key findings for one module of a map property.
type DiffModule struct {
	Module  string    `json:"module"`
	Changed []DiffKey `json:"changed,omitempty"`
	Missing []string  `json:"missing,omitempty"`
	Unknown []string  `json:"unknown,omitempty"`
}

// DiffProperty is one property the override sets, compared against the default.
// Default and Value carry the compared values for kind "changed" only.
type DiffProperty struct {
	Path      string       `json:"path"`
	Kind      string       `json:"kind"`
	Redundant bool         `json:"redundant"`
	Wholesale bool         `json:"wholesale,omitempty"`
	Default   any          `json:"default,omitempty"`
	Value     any          `json:"value,omitempty"`
	Excluded  []string     `json:"excluded,omitempty"`
	Unknown   []string     `json:"unknown,omitempty"`
	Modules   []DiffModule `json:"modules,omitempty"`
}

// DiffSummary counts the diff at a glance.
type DiffSummary struct {
	Deviations int `json:"deviations"`
	Redundant  int `json:"redundant"`
	Missing    int `json:"missing"`
	Unknown    int `json:"unknown"`
}

// DiffResult is the comparison of an override profile against a base profile.
type DiffResult struct {
	Summary    DiffSummary    `json:"summary"`
	Properties []DiffProperty `json:"properties"`
}

// Diff compares an override profile against a base profile, considering only
// the properties the override actually sets. An unset property is inherited
// and never a deviation.
//
// A property reported redundant restates the base value exactly, so removing
// it from the override leaves the merged profile unchanged.
func Diff(override, base *Profile) *DiffResult {
	result := &DiffResult{Properties: []DiffProperty{}}
	if override == nil {
		return result
	}
	varsDone := false
	for _, name := range propertyNames() {
		switch {
		case name == "test_cases":
			if !override.isSet(name) {
				continue
			}
			result.add(diffTestCases(override, base))
		case name == "test_levels":
			if !override.isSet(name) {
				continue
			}
			result.add(diffTestLevels(override, base))
		case strings.HasPrefix(name, testCasesVarsPrefix):
			if varsDone {
				continue
			}
			varsDone = true
			if entry, ok := diffTestCasesVars(override, base); ok {
				result.add(entry)
			}
		default:
			if !override.isSet(name) {
				continue
			}
			result.add(diffScalar(name, override, base))
		}
	}
	return result
}

func (r *DiffResult) add(entry DiffProperty) {
	r.Properties = append(r.Properties, entry)
	if entry.Redundant {
		r.Summary.Redundant++
	} else {
		r.Summary.Deviations++
	}
	r.Summary.Unknown += len(entry.Unknown)
	for _, module := range entry.Modules {
		r.Summary.Missing += len(module.Missing)
		r.Summary.Unknown += len(module.Unknown)
	}
}

// diffScalar compares one property by value. Structured properties without a
// key model (logfilter, cache, asn_db.sources) are compared deep-equal only.
func diffScalar(name string, override, base *Profile) DiffProperty {
	value := propertyValue(override, name)
	baseValue := propertyValue(base, name)
	if baseValue != nil && reflect.DeepEqual(value, baseValue) {
		return DiffProperty{Path: name, Kind: DiffKindRedundant, Redundant: true}
	}
	return DiffProperty{Path: name, Kind: DiffKindChanged, Default: baseValue, Value: value}
}

// diffTestCases compares the test_cases set. The engine lowercases and trims
// testcase names when planning a run, so the comparison does too.
func diffTestCases(override, base *Profile) DiffProperty {
	entry := DiffProperty{Path: "test_cases", Kind: DiffKindSet, Wholesale: true}
	overrideSet := testCaseSet(override)
	baseSet := testCaseSet(base)
	for _, name := range slices.Sorted(maps.Keys(baseSet)) {
		if !overrideSet[name] {
			entry.Excluded = append(entry.Excluded, name)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(overrideSet)) {
		if !baseSet[name] {
			entry.Unknown = append(entry.Unknown, name)
		}
	}
	entry.Redundant = len(entry.Excluded) == 0 && len(entry.Unknown) == 0
	return entry
}

// diffTestLevels compares test_levels per module. Overriding the property
// replaces the whole table, so a tag or a module the override omits resolves
// to DEBUG at run time.
func diffTestLevels(override, base *Profile) DiffProperty {
	entry := DiffProperty{Path: "test_levels", Kind: DiffKindMap, Wholesale: true}
	overrideLevels := testLevels(override)
	baseLevels := testLevels(base)
	clean := true
	for _, module := range sortedUnion(overrideLevels, baseLevels) {
		overrideTags, inOverride := overrideLevels[module]
		baseTags, inBase := baseLevels[module]
		found := DiffModule{Module: module}
		switch {
		case !inOverride:
			found.Missing = slices.Sorted(maps.Keys(baseTags))
		case !inBase:
			found.Unknown = slices.Sorted(maps.Keys(overrideTags))
		default:
			for _, tag := range slices.Sorted(maps.Keys(baseTags)) {
				value, covered := overrideTags[tag]
				if !covered {
					found.Missing = append(found.Missing, tag)
					continue
				}
				if !strings.EqualFold(value, baseTags[tag]) {
					found.Changed = append(found.Changed, DiffKey{Key: tag, Default: baseTags[tag], Value: value})
				}
			}
			for _, tag := range slices.Sorted(maps.Keys(overrideTags)) {
				if _, known := baseTags[tag]; !known {
					found.Unknown = append(found.Unknown, tag)
				}
			}
		}
		if len(found.Changed)+len(found.Missing)+len(found.Unknown) == 0 {
			continue
		}
		entry.Modules = append(entry.Modules, found)
		clean = false
	}
	entry.Redundant = clean
	return entry
}

// diffTestCasesVars compares the testcase tunables per module. Each tunable is
// its own property, so a tunable the override omits stays inherited and the
// property is redundant whenever no set tunable differs. The second return
// value is false when the override sets no tunable at all.
func diffTestCasesVars(override, base *Profile) (DiffProperty, bool) {
	entry := DiffProperty{Path: "test_cases_vars", Kind: DiffKindMap}
	modules := []string{}
	leaves := map[string][]string{}
	for _, name := range propertyNames() {
		if !strings.HasPrefix(name, testCasesVarsPrefix) {
			continue
		}
		parts := strings.Split(name, ".")
		if len(parts) != 3 {
			continue
		}
		module := parts[1]
		if _, seen := leaves[module]; !seen {
			modules = append(modules, module)
		}
		leaves[module] = append(leaves[module], name)
	}

	overridden := false
	changed := false
	for _, module := range modules {
		if !anySet(override, leaves[module]) {
			continue
		}
		overridden = true
		found := DiffModule{Module: module}
		for _, path := range leaves[module] {
			key := path[strings.LastIndex(path, ".")+1:]
			if !override.isSet(path) {
				found.Missing = append(found.Missing, key)
				continue
			}
			value := propertyValue(override, path)
			baseValue := propertyValue(base, path)
			if baseValue != nil && reflect.DeepEqual(value, baseValue) {
				continue
			}
			found.Changed = append(found.Changed, DiffKey{Key: key, Default: baseValue, Value: value})
			changed = true
		}
		if len(found.Changed)+len(found.Missing) == 0 {
			continue
		}
		entry.Modules = append(entry.Modules, found)
	}
	entry.Redundant = !changed
	return entry, overridden
}

func propertyValue(p *Profile, name string) any {
	if p == nil || !p.isSet(name) {
		return nil
	}
	return deepCopy(propertyDefs[name].getter(p))
}

func anySet(p *Profile, paths []string) bool {
	for _, path := range paths {
		if p.isSet(path) {
			return true
		}
	}
	return false
}

func testCaseSet(p *Profile) map[string]bool {
	out := map[string]bool{}
	raw, ok := propertyValue(p, "test_cases").([]any)
	if !ok {
		return out
	}
	for _, item := range raw {
		name, ok := item.(string)
		if !ok {
			continue
		}
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "" {
			out[name] = true
		}
	}
	return out
}

func testLevels(p *Profile) map[string]map[string]string {
	levels, ok := propertyValue(p, "test_levels").(map[string]map[string]string)
	if !ok {
		return map[string]map[string]string{}
	}
	return levels
}

func sortedUnion[V any](left, right map[string]V) []string {
	union := map[string]bool{}
	for key := range left {
		union[key] = true
	}
	for key := range right {
		union[key] = true
	}
	return slices.Sorted(maps.Keys(union))
}

// TestLevelEntry is one tag of a profile's test_levels table.
type TestLevelEntry struct {
	Module string `json:"module"`
	Tag    string `json:"tag"`
	Level  string `json:"level"`
}

// TestLevelChange is one tag the two profiles level differently.
type TestLevelChange struct {
	Module string `json:"module"`
	Tag    string `json:"tag"`
	From   string `json:"from"`
	To     string `json:"to"`
}

// TestLevelsDiff is the tag vocabulary comparison of two profiles.
type TestLevelsDiff struct {
	Added        []TestLevelEntry  `json:"added,omitempty"`
	Removed      []TestLevelEntry  `json:"removed,omitempty"`
	LevelChanged []TestLevelChange `json:"level_changed,omitempty"`
}

// Empty reports whether the two vocabularies agree.
func (d *TestLevelsDiff) Empty() bool {
	if d == nil {
		return true
	}
	return len(d.Added)+len(d.Removed)+len(d.LevelChanged) == 0
}

// DiffTestLevels compares the test_levels tables of two profiles as two tag
// vocabularies: a tag only in b is added, a tag only in a is removed, a tag
// in both at different levels is level-changed. Unlike Diff this is a
// symmetric comparison of two full profiles, not an override against a base,
// so it reads the table directly and never treats an unset property as
// inherited. Levels compare case-insensitively and are reported as stored.
// A nil profile is an empty vocabulary. Results are sorted by module, then
// tag.
func DiffTestLevels(a, b *Profile) *TestLevelsDiff {
	left := profileTestLevels(a)
	right := profileTestLevels(b)
	diff := &TestLevelsDiff{}
	for _, module := range sortedUnion(left, right) {
		leftTags := left[module]
		rightTags := right[module]
		for _, tag := range sortedUnion(leftTags, rightTags) {
			fromLevel, inLeft := leftTags[tag]
			toLevel, inRight := rightTags[tag]
			switch {
			case !inLeft:
				diff.Added = append(diff.Added, TestLevelEntry{Module: module, Tag: tag, Level: toLevel})
			case !inRight:
				diff.Removed = append(diff.Removed, TestLevelEntry{Module: module, Tag: tag, Level: fromLevel})
			case !strings.EqualFold(fromLevel, toLevel):
				diff.LevelChanged = append(diff.LevelChanged,
					TestLevelChange{Module: module, Tag: tag, From: fromLevel, To: toLevel})
			}
		}
	}
	return diff
}

// profileTestLevels reads the table off the struct, ignoring the set map.
func profileTestLevels(p *Profile) map[string]map[string]string {
	if p == nil {
		return map[string]map[string]string{}
	}
	return p.TestLevels
}
