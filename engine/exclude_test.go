package engine

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
)

// plannedTestcases returns the plan for req.
func plannedTestcases(t *testing.T, req RunRequest) []string {
	t.Helper()
	planned, err := PlannedTestcases(req)
	if err != nil {
		t.Fatalf("PlannedTestcases: %v", err)
	}
	return planned
}

// allPlanned returns every testcase in plan order.
func allPlanned(t *testing.T) []string {
	t.Helper()
	return plannedTestcases(t, RunRequest{})
}

// without returns list minus drop, order kept.
func without(list []string, drop ...string) []string {
	return slices.DeleteFunc(slices.Clone(list), func(name string) bool { return slices.Contains(drop, name) })
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

func TestExcludedTestcaseLists(t *testing.T) {
	all := allPlanned(t)
	tests := []struct {
		name string
		list func(t *testing.T, req RunRequest) []string
		req  RunRequest
		want []string
	}{
		{"planned minus module", plannedTestcases, RunRequest{Exclude: []string{"DNSSEC"}}, without(all, moduleTestcases["dnssec"]...)},
		{"planned minus comma list", plannedTestcases, RunRequest{Exclude: []string{"dnssec10, Zone01", "dnssec10"}}, without(all, "dnssec10", "zone01")},
		{"module minus one", plannedTestcases, RunRequest{Module: "dnssec", Exclude: []string{"dnssec10"}}, without(moduleTestcases["dnssec"], "dnssec10")},
		{"profile cannot reenable", plannedTestcases, RunRequest{ProfileData: `{"test_cases":["basic01","basic02"]}`, Exclude: []string{"basic02"}}, []string{"basic01"}},
		{"effective profile excludes", profileTestcases, RunRequest{ProfileData: `{"test_cases":["basic01","dnssec10","zone01"]}`, Exclude: []string{"dnssec"}}, []string{"basic01", "zone01"}},
		{"effective profile excludes from module", profileTestcases, RunRequest{Module: "basic", Exclude: []string{"basic03"}}, []string{"basic01", "basic02"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.list(t, tc.req); !slices.Equal(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestExcludeErrors(t *testing.T) {
	plannedErr := func(req RunRequest) error { _, err := PlannedTestcases(req); return err }
	effectiveErr := func(req RunRequest) error { _, err := EffectiveProfile(req); return err }
	tests := []struct {
		name    string
		call    func(RunRequest) error
		req     RunRequest
		want    error
		wantMsg string
	}{
		{"unknown exclusion", plannedErr, RunRequest{Exclude: []string{"nope99"}}, ErrNotImplemented, ""},
		{"exclusion empties the plan", plannedErr, RunRequest{ProfileData: `{"test_cases":["basic02"]}`, Exclude: []string{"basic02"}}, errAllExcluded, ""},
		{"explicit testcase excluded", plannedErr, RunRequest{Testcases: []string{"basic01", "DNSSEC10"}, Exclude: []string{"dnssec10"}}, ErrNotImplemented, `testcase "dnssec10" is excluded`},
		{"explicit module excluded", plannedErr, RunRequest{Module: "dnssec", Exclude: []string{"dnssec"}}, ErrNotImplemented, `module "dnssec" is excluded`},
		{"effective profile conflict", effectiveErr, RunRequest{Testcases: []string{"basic02"}, Exclude: []string{"basic"}}, ErrNotImplemented, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call(tc.req)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if tc.wantMsg != "" && !strings.Contains(err.Error(), tc.wantMsg) {
				t.Fatalf("error %q does not contain %q", err, tc.wantMsg)
			}
		})
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
	if dnstest.HasTag(runner.Logger.Entries(), "UNKNOWN_METHOD") {
		t.Fatalf("unexpected UNKNOWN_METHOD for an excluded testcase")
	}
}
