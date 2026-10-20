package engine

import (
	"slices"
	"testing"
)

func TestModulesFollowRunOrder(t *testing.T) {
	modules := Modules()
	if len(modules) != len(moduleOrder) {
		t.Fatalf("len(Modules()) = %d, want %d", len(modules), len(moduleOrder))
	}
	for i, module := range modules {
		if module.Name != moduleOrder[i] {
			t.Errorf("module %d = %q, want %q", i, module.Name, moduleOrder[i])
		}
		if module.Display != moduleDisplayName(module.Name) {
			t.Errorf("module %q display = %q, want %q", module.Name, module.Display, moduleDisplayName(module.Name))
		}
		ids := make([]string, 0, len(module.Testcases))
		for _, tc := range module.Testcases {
			ids = append(ids, tc.ID)
		}
		if !slices.Equal(ids, moduleTestcases[module.Name]) {
			t.Errorf("module %q testcases = %v, want %v", module.Name, ids, moduleTestcases[module.Name])
		}
	}
}

func TestModulesMatchAvailableTestcases(t *testing.T) {
	var display []string
	for _, module := range Modules() {
		for _, tc := range module.Testcases {
			display = append(display, module.Display+":"+tc.Display)
		}
	}
	if !slices.Equal(display, AvailableTestcases()) {
		t.Fatalf("Modules() display names = %v, want %v", display, AvailableTestcases())
	}
}

func TestModulesCarrySortedUniqueTags(t *testing.T) {
	for _, module := range Modules() {
		for _, tc := range module.Testcases {
			if len(tc.Tags) == 0 {
				t.Errorf("testcase %q has no tags", tc.ID)
				continue
			}
			if !slices.IsSorted(tc.Tags) {
				t.Errorf("testcase %q tags are not sorted: %v", tc.ID, tc.Tags)
			}
			if sorted := slices.Compact(slices.Clone(tc.Tags)); len(sorted) != len(tc.Tags) {
				t.Errorf("testcase %q has duplicate tags: %v", tc.ID, tc.Tags)
			}
		}
	}
}

func TestNormalizeTags(t *testing.T) {
	got := normalizeTags(map[string][]string{" Basic01 ": {"B", " A ", "B", ""}, "": {"X"}})
	want := map[string][]string{"basic01": {"A", "B"}}
	if len(got) != len(want) || !slices.Equal(got["basic01"], want["basic01"]) {
		t.Fatalf("normalizeTags = %v, want %v", got, want)
	}
}
