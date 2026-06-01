package main

import (
	"strings"
	"testing"
)

func TestRenderTOMLSortedLowercasedQuoted(t *testing.T) {
	got := renderTOML(map[string]string{
		"ZONE05":  "SOA 'expire' minimum value",
		"BASIC01": "The domain must have a parent domain",
	})

	if !strings.Contains(got, "Do not edit by hand") {
		t.Fatalf("missing generated-file guard header:\n%s", got)
	}

	// Keys are lowercased and the value is emitted as a TOML basic string,
	// preserving the apostrophes verbatim.
	wantBasic := `"basic01" = "The domain must have a parent domain"`
	wantZone := `"zone05" = "SOA 'expire' minimum value"`
	if !strings.Contains(got, wantBasic) {
		t.Errorf("missing %q in output:\n%s", wantBasic, got)
	}
	if !strings.Contains(got, wantZone) {
		t.Errorf("missing %q in output:\n%s", wantZone, got)
	}

	// Output is sorted: basic01 before zone05.
	if strings.Index(got, wantBasic) > strings.Index(got, wantZone) {
		t.Errorf("expected basic01 to sort before zone05:\n%s", got)
	}
}
