package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

const severitySpec = `# zone99

## Emitted Tags
| Tag | Meaning |
| --- | --- |
| ` + "`A_TAG`" + ` | First. |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| ` + "`A_TAG`" + ` | ` + "`WARNING`" + ` | Default from ` + "`share/profile.json`" + `. |
| ` + "`B_TAG`" + ` | ` + "`debug2`" + ` | Lowercase level. |

## Differences From Upstream
| ` + "`C_TAG`" + ` | ` + "`INFO`" + ` | Outside the section. |
`

func TestParseSeverityLevelsReadsOnlyItsSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zone99.md")
	if err := os.WriteFile(path, []byte(severitySpec), 0o600); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	got, err := parseSeverityLevels(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 2 || got["A_TAG"] != "WARNING" || got["B_TAG"] != "DEBUG2" {
		t.Fatalf("levels = %v, want A_TAG WARNING and B_TAG DEBUG2", got)
	}
}

func TestSeverityMismatches(t *testing.T) {
	profile := map[string]map[string]string{"ZONE": {"A_TAG": "WARNING", "B_TAG": "DEBUG2"}}
	cases := []struct {
		name string
		spec map[string]string
		want []string
	}{
		{"match", map[string]string{"A_TAG": "WARNING", "B_TAG": "DEBUG2"}, nil},
		{"level differs", map[string]string{"A_TAG": "NOTICE"}, []string{
			"severity level mismatch in z.md: A_TAG is NOTICE, share/profile.json has WARNING",
		}},
		{"tag absent", map[string]string{"C_TAG": "DEBUG"}, []string{
			"severity level missing from share/profile.json test_levels.ZONE: C_TAG (z.md)",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := severityMismatches("z.md", "zone", tc.spec, profile)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("mismatches = %q, want %q", got, tc.want)
			}
		})
	}
}
