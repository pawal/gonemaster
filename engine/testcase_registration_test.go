package engine

import "testing"

// TestEveryPlannedTestcaseIsRegistered guards against the class of drift where a
// testcase is listed in moduleTestcases (and enabled in profile.json) but never
// added to its module's *Tests map. Such a testcase runs in a full run via the
// module All() function, but testcaseModule cannot resolve it, so selecting it
// individually (--testcase zoneNN) fails with ErrNotImplemented.
func TestEveryPlannedTestcaseIsRegistered(t *testing.T) {
	for module, testcases := range moduleTestcases {
		for _, tc := range testcases {
			got := testcaseModule(tc)
			if got != module {
				t.Errorf("testcase %q: testcaseModule = %q, want %q (missing from the module's *Tests map?)", tc, got, module)
			}
		}
	}
}

// TestPlannedTestcasesSelectsZone13 is a regression test for zone13 being absent
// from the zoneTests map: selecting it individually must plan it, not error.
func TestPlannedTestcasesSelectsZone13(t *testing.T) {
	planned, err := PlannedTestcases(RunRequest{Testcases: []string{"zone13"}})
	if err != nil {
		t.Fatalf("PlannedTestcases(--testcase zone13): %v", err)
	}
	if len(planned) != 1 || planned[0] != "zone13" {
		t.Fatalf("expected [zone13], got %v", planned)
	}
}
