package engine

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// allPlanned returns every testcase in plan order.
func allPlanned(t *testing.T) []string {
	t.Helper()
	planned, err := PlannedTestcases(RunRequest{})
	if err != nil {
		t.Fatalf("PlannedTestcases: %v", err)
	}
	return planned
}

// without returns list minus drop, order kept.
func without(list []string, drop ...string) []string {
	return slices.DeleteFunc(slices.Clone(list), func(name string) bool { return slices.Contains(drop, name) })
}

func TestExpandExclusionsModule(t *testing.T) {
	got, err := ExpandExclusions([]string{"dnssec"})
	if err != nil {
		t.Fatalf("ExpandExclusions: %v", err)
	}
	if !slices.Equal(got, moduleTestcases["dnssec"]) {
		t.Fatalf("got %v, want %v", got, moduleTestcases["dnssec"])
	}
}

func TestExpandExclusionsCommaCaseDuplicates(t *testing.T) {
	got, err := ExpandExclusions([]string{" Zone01, BASIC03 ", "basic03,,", ""})
	if err != nil {
		t.Fatalf("ExpandExclusions: %v", err)
	}
	if want := []string{"basic03", "zone01"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestExpandExclusionsEmpty(t *testing.T) {
	got, err := ExpandExclusions(nil)
	if err != nil {
		t.Fatalf("ExpandExclusions: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("got %#v, want an empty list", got)
	}
}

func TestExpandExclusionsUnknown(t *testing.T) {
	_, err := ExpandExclusions([]string{"basic01,Nope99"})
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("got %v, want ErrNotImplemented", err)
	}
	if !strings.Contains(err.Error(), `"Nope99"`) {
		t.Fatalf("error %q does not name the value", err)
	}
}

func TestPlannedTestcasesExcludeModule(t *testing.T) {
	got, err := PlannedTestcases(RunRequest{Exclude: []string{"DNSSEC"}})
	if err != nil {
		t.Fatalf("PlannedTestcases: %v", err)
	}
	if want := without(allPlanned(t), moduleTestcases["dnssec"]...); !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPlannedTestcasesExcludeCommaList(t *testing.T) {
	got, err := PlannedTestcases(RunRequest{Exclude: []string{"dnssec10, Zone01", "dnssec10"}})
	if err != nil {
		t.Fatalf("PlannedTestcases: %v", err)
	}
	if want := without(allPlanned(t), "dnssec10", "zone01"); !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPlannedTestcasesExcludeUnknown(t *testing.T) {
	_, err := PlannedTestcases(RunRequest{Exclude: []string{"nope99"}})
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("got %v, want ErrNotImplemented", err)
	}
}

func TestPlannedTestcasesExcludeEmptiesPlan(t *testing.T) {
	_, err := PlannedTestcases(RunRequest{ProfileData: `{"test_cases":["basic02"]}`, Exclude: []string{"basic02"}})
	if !errors.Is(err, errAllExcluded) {
		t.Fatalf("got %v, want errAllExcluded", err)
	}
}

func TestPlannedTestcasesExcludeExplicitTestcase(t *testing.T) {
	_, err := PlannedTestcases(RunRequest{Testcases: []string{"basic01", "DNSSEC10"}, Exclude: []string{"dnssec10"}})
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("got %v, want ErrNotImplemented", err)
	}
	if !strings.Contains(err.Error(), `testcase "dnssec10" is excluded`) {
		t.Fatalf("error %q does not name the testcase", err)
	}
}

func TestPlannedTestcasesExcludeExplicitModule(t *testing.T) {
	_, err := PlannedTestcases(RunRequest{Module: "dnssec", Exclude: []string{"dnssec"}})
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("got %v, want ErrNotImplemented", err)
	}
	if !strings.Contains(err.Error(), `module "dnssec" is excluded`) {
		t.Fatalf("error %q does not name the module", err)
	}
}

func TestPlannedTestcasesModuleMinusOne(t *testing.T) {
	got, err := PlannedTestcases(RunRequest{Module: "dnssec", Exclude: []string{"dnssec10"}})
	if err != nil {
		t.Fatalf("PlannedTestcases: %v", err)
	}
	if want := without(moduleTestcases["dnssec"], "dnssec10"); !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPlannedTestcasesProfileCannotReenable(t *testing.T) {
	got, err := PlannedTestcases(RunRequest{ProfileData: `{"test_cases":["basic01","basic02"]}`, Exclude: []string{"basic02"}})
	if err != nil {
		t.Fatalf("PlannedTestcases: %v", err)
	}
	if want := []string{"basic01"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// profileTestcases returns the test_cases of the effective profile.
func profileTestcases(t *testing.T, req RunRequest) []string {
	t.Helper()
	p, err := EffectiveProfile(req)
	if err != nil {
		t.Fatalf("EffectiveProfile: %v", err)
	}
	out := []string{}
	for _, item := range p.TestCases {
		name, _ := item.(string)
		out = append(out, name)
	}
	return out
}

func TestEffectiveProfileExcludes(t *testing.T) {
	got := profileTestcases(t, RunRequest{ProfileData: `{"test_cases":["basic01","dnssec10","zone01"]}`, Exclude: []string{"dnssec"}})
	if want := []string{"basic01", "zone01"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestEffectiveProfileExcludesFromModule(t *testing.T) {
	got := profileTestcases(t, RunRequest{Module: "basic", Exclude: []string{"basic03"}})
	if want := []string{"basic01", "basic02"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestEffectiveProfileExcludeConflict(t *testing.T) {
	_, err := EffectiveProfile(RunRequest{Testcases: []string{"basic02"}, Exclude: []string{"basic"}})
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("got %v, want ErrNotImplemented", err)
	}
}

func TestRunWithRunnerExcludeSkipsTestcase(t *testing.T) {
	runner := newTestRunner(t, withRunLimits(1), withTestcases("syntax01", "syntax02", "syntax03"))
	_, err := RunWithRunner(RunRequest{Domain: "example.com", Module: "syntax", Exclude: []string{"syntax02"}}, runner)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	started := []string{}
	for _, entry := range runner.Logger.Entries() {
		if entry.Tag == "TEST_CASE_START" {
			started = append(started, strings.ToLower(entry.Testcase))
		}
	}
	if want := []string{"syntax01", "syntax03"}; !slices.Equal(started, want) {
		t.Fatalf("started %v, want %v", started, want)
	}
}

func TestRunWithRunnerExcludeConflictLogsNoUnknownMethod(t *testing.T) {
	runner := newTestRunner(t)
	_, err := RunWithRunner(RunRequest{Domain: "example.com", Testcases: []string{"syntax01"}, Exclude: []string{"syntax"}}, runner)
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("got %v, want ErrNotImplemented", err)
	}
	for _, entry := range runner.Logger.Entries() {
		if entry.Tag == "UNKNOWN_METHOD" {
			t.Fatalf("unexpected UNKNOWN_METHOD for an excluded testcase")
		}
	}
}
