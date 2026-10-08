package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureModules() map[string]map[string][]string {
	return map[string]map[string][]string{
		"zone": {
			"zone01": {"SHARED_TAG", "Z01_A"},
		},
		"basic": {
			"basic01": {"SHARED_TAG", "B01_A"},
			"basic02": {"B02_A"},
		},
	}
}

func TestBuildPayloadCountsUniqueTags(t *testing.T) {
	got := buildPayload(fixtureModules()).Summary
	want := exportSummary{ModuleCount: 2, TestcaseCount: 3, UniqueTagCount: 4}
	if got != want {
		t.Fatalf("summary = %+v, want %+v", got, want)
	}
}

func TestRenderMarkdownSummary(t *testing.T) {
	got := renderMarkdown(buildPayload(fixtureModules()))

	wants := []string{
		"Do not edit by hand.",
		"  - `engine/test/basic/basic.go:Metadata`\n",
		"- Modules: 2\n- Testcases: 3\n- Unique tags: 4\n",
		"Module testcase counts:\n- `basic`: 2\n- `zone`: 1\n",
	}
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in output:\n%s", want, got)
		}
	}
}

func TestRunCheckDetectsDrift(t *testing.T) {
	dir := t.TempDir()
	jsonOut := filepath.Join(dir, "tags.json")
	markdownOut := filepath.Join(dir, "tags.md")

	if err := run(fixtureModules(), jsonOut, markdownOut, false); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := run(fixtureModules(), jsonOut, markdownOut, true); err != nil {
		t.Fatalf("check after write: %v", err)
	}

	if err := os.WriteFile(markdownOut, []byte("- Testcases: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := run(fixtureModules(), jsonOut, markdownOut, true)
	if err == nil {
		t.Fatal("check passed on a stale markdown file")
	}
	if want := markdownOut + " is out of date; run: make spec-export-tags"; err.Error() != want {
		t.Fatalf("error = %q, want %q", err, want)
	}
}
