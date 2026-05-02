package analysis

import (
	"reflect"
	"sort"
	"testing"
	"time"

	serverpkg "codeberg.org/pawal/gonemaster/server"
)

func TestExtractDNSKEYAlgorithms(t *testing.T) {
	input := RunInput{
		Entries: []serverpkg.Entry{
			{
				Module:   "DNSSEC",
				Testcase: "dnssec05",
				Tag:      "DS05_ALGO_OK",
				Args: map[string]any{
					"keytag":     uint16(1234),
					"algo_num":   uint8(8),
					"algo_mnemo": "RSASHA256",
				},
			},
			// Same (algo=8, keytag=1234) repeated across two nameservers
			// must collapse to one row and value_num=1 (one distinct keytag).
			{
				Module:   "DNSSEC",
				Testcase: "dnssec05",
				Tag:      "DS05_ALGO_OK",
				Args: map[string]any{
					"keytag":     uint16(1234),
					"algo_num":   uint8(8),
					"algo_mnemo": "RSASHA256",
				},
			},
			// Two more keytags on algo=13 — value_num should be 2.
			{
				Module:   "DNSSEC",
				Testcase: "dnssec05",
				Tag:      "DS05_ALGO_OK",
				Args: map[string]any{
					"keytag":   uint16(2000),
					"algo_num": uint8(13),
				},
			},
			{
				Module:   "DNSSEC",
				Testcase: "dnssec05",
				Tag:      "DS05_ALGO_OK",
				Args: map[string]any{
					"keytag":   uint16(2001),
					"algo_num": uint8(13),
				},
			},
			// Deprecated algorithm should still be extracted — the registry
			// colors it, not the projector.
			{
				Module:   "DNSSEC",
				Testcase: "dnssec05",
				Tag:      "DS05_ALGO_DEPRECATED",
				Args: map[string]any{
					"keytag":   uint16(3000),
					"algo_num": uint8(5),
				},
			},
			// Unrelated tag should be ignored.
			{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_SIGNED"},
		},
	}

	got := extractDNSKEYAlgorithms(input)
	sort.Slice(got, func(i, j int) bool { return got[i].key < got[j].key })

	if len(got) != 3 {
		t.Fatalf("expected 3 algorithms, got %d: %+v", len(got), got)
	}

	byKey := map[string]extractedDomainFact{}
	for _, f := range got {
		if f.category != factCategoryDNSKEYAlgorithm {
			t.Fatalf("unexpected category: %q", f.category)
		}
		byKey[f.key] = f
	}
	if _, ok := byKey["5"]; !ok {
		t.Fatalf("expected algo=5 present: %+v", got)
	}
	if f := byKey["8"]; f.valueNum == nil || *f.valueNum != 1 {
		t.Fatalf("expected algo=8 value_num=1, got %+v", f.valueNum)
	}
	if f := byKey["13"]; f.valueNum == nil || *f.valueNum != 2 {
		t.Fatalf("expected algo=13 value_num=2, got %+v", f.valueNum)
	}
}

func TestExtractDNSKEYAlgorithmsEmptyWhenNotSigned(t *testing.T) {
	input := RunInput{
		Entries: []serverpkg.Entry{
			{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED"},
		},
	}
	if got := extractDNSKEYAlgorithms(input); len(got) != 0 {
		t.Fatalf("expected no algorithm facts for unsigned zone, got %+v", got)
	}
}

func TestExtractDNSSECPosture(t *testing.T) {
	cases := []struct {
		name string
		in   []serverpkg.Entry
		want string // empty means: no fact emitted
	}{
		{
			name: "unsigned",
			in: []serverpkg.Entry{
				{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED"},
				{Module: "DNSSEC", Testcase: "dnssec10", Tag: "DS10_HAS_NSEC"},
			},
			want: factKeyUnsigned,
		},
		{
			name: "nsec only",
			in: []serverpkg.Entry{
				{Module: "DNSSEC", Testcase: "dnssec05", Tag: "DS05_ALGO_OK"},
				{Module: "DNSSEC", Testcase: "dnssec10", Tag: "DS10_HAS_NSEC"},
			},
			want: factKeyNSEC,
		},
		{
			name: "nsec3 only",
			in: []serverpkg.Entry{
				{Module: "DNSSEC", Testcase: "dnssec05", Tag: "DS05_ALGO_OK"},
				{Module: "DNSSEC", Testcase: "dnssec10", Tag: "DS10_HAS_NSEC3"},
			},
			want: factKeyNSEC3,
		},
		{
			name: "nsec and nsec3 from different servers",
			in: []serverpkg.Entry{
				{Module: "DNSSEC", Testcase: "dnssec10", Tag: "DS10_HAS_NSEC"},
				{Module: "DNSSEC", Testcase: "dnssec10", Tag: "DS10_HAS_NSEC3"},
			},
			want: factKeyNSECMixed,
		},
		{
			name: "explicit MIXED tag",
			in: []serverpkg.Entry{
				{Module: "DNSSEC", Testcase: "dnssec10", Tag: "DS10_MIXED_NSEC_NSEC3"},
			},
			want: factKeyNSECMixed,
		},
		{
			name: "signed but no DS10 mode signal",
			in: []serverpkg.Entry{
				{Module: "DNSSEC", Testcase: "dnssec05", Tag: "DS05_ALGO_OK"},
			},
			want: factKeySigned,
		},
		{
			name: "no DNSSEC signal at all",
			in: []serverpkg.Entry{
				{Module: "BASIC", Testcase: "basic01", Tag: "B01_OK"},
			},
			want: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractDNSSECPosture(RunInput{Entries: c.in})
			if c.want == "" {
				if len(got) != 0 {
					t.Fatalf("expected no fact, got %+v", got)
				}
				return
			}
			if len(got) != 1 || got[0].category != factCategoryDNSSECPosture || got[0].key != c.want {
				t.Fatalf("expected dnssec_posture=%q, got %+v", c.want, got)
			}
		})
	}
}

func TestExtractSeverity(t *testing.T) {
	cases := []struct {
		level string
		want  string
	}{
		{"", "OK"},
		{"   ", "OK"},
		{"OK", "OK"},
		{"notice", "NOTICE"},
		{"WARNING", "WARNING"},
		{"ERROR", "ERROR"},
		{"CRITICAL", "CRITICAL"},
	}
	for _, c := range cases {
		got := extractSeverity(RunInput{Run: serverpkg.Run{WorstLevel: c.level}})
		if len(got) != 1 || got[0].category != factCategorySeverity || got[0].key != c.want {
			t.Fatalf("WorstLevel=%q: expected severity=%q, got %+v", c.level, c.want, got)
		}
	}
}

func TestExtractDomainFactsDedupesAcrossExtractors(t *testing.T) {
	input := RunInput{
		Entries: []serverpkg.Entry{
			{
				Module:   "DNSSEC",
				Testcase: "dnssec05",
				Tag:      "DS05_ALGO_OK",
				Args: map[string]any{
					"keytag":   uint16(1),
					"algo_num": uint8(13),
				},
			},
			{
				Module:   "DNSSEC",
				Testcase: "dnssec05",
				Tag:      "DS05_ALGO_OK",
				Args: map[string]any{
					"keytag":   uint16(2),
					"algo_num": uint8(13),
				},
			},
		},
	}
	got := extractDomainFacts(input)
	sawAlgo := false
	sawPosture := false
	for _, f := range got {
		if f.category == factCategoryDNSKEYAlgorithm && f.key == "13" {
			sawAlgo = true
		}
		if f.category == factCategoryDNSSECPosture && f.key == factKeySigned {
			sawPosture = true
		}
	}
	if !sawAlgo {
		t.Fatalf("expected DNSKEY algorithm fact, got %+v", got)
	}
	if !sawPosture {
		t.Fatalf("expected dnssec_posture=signed fact, got %+v", got)
	}
}

func TestExtractGrade(t *testing.T) {
	grade := "B"
	got := extractGrade(RunInput{
		Run: serverpkg.Run{Grade: &grade},
	})
	if len(got) != 1 || got[0].category != factCategoryGrade || got[0].key != "B" {
		t.Fatalf("expected one grade=B fact, got %+v", got)
	}

	// Nil grade → no fact.
	if got := extractGrade(RunInput{}); len(got) != 0 {
		t.Fatalf("nil grade should emit nothing, got %+v", got)
	}

	// Empty/whitespace grade → no fact.
	empty := "   "
	if got := extractGrade(RunInput{Run: serverpkg.Run{Grade: &empty}}); len(got) != 0 {
		t.Fatalf("whitespace grade should emit nothing, got %+v", got)
	}

	// Custom scoring profile grade passes through verbatim — no
	// hard-coded letter set.
	custom := "Gold"
	got = extractGrade(RunInput{Run: serverpkg.Run{Grade: &custom}})
	if len(got) != 1 || got[0].key != "Gold" {
		t.Fatalf("expected custom grade to pass through, got %+v", got)
	}
}

func TestBuildDomainFactRowsAttachesRunCohortDomain(t *testing.T) {
	v := int64(2)
	facts := []extractedDomainFact{
		{category: factCategoryDNSKEYAlgorithm, key: "13", valueNum: &v},
		{category: factCategoryDNSSECPosture, key: factKeySigned},
	}
	rows := buildDomainFactRows(42, "run-x", 101, facts)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	for _, r := range rows {
		if r.CohortID != 42 || r.RunID != "run-x" || r.DomainID != 101 {
			t.Fatalf("wrong keys on row: %+v", r)
		}
	}
	if rows[0].ValueNum == nil || *rows[0].ValueNum != 2 {
		t.Fatalf("expected value_num=2, got %+v", rows[0].ValueNum)
	}
	if rows[1].ValueNum != nil {
		t.Fatalf("expected nil value_num for signed fact, got %+v", rows[1].ValueNum)
	}
}

// Reprojecting the same run must produce byte-identical domain-fact rows.
// Replace-per-run semantics are inherited from the shared store helper, but
// drift in extractor ordering or dedupe would only surface here.
func TestProjectorProjectRunDomainFactsIdempotent(t *testing.T) {
	grade := "A"
	run := serverpkg.Run{
		ID:         "run-facts",
		DomainID:   404,
		Domain:     "facts.test",
		Status:     serverpkg.JobSucceeded,
		EntryCount: 3,
		FinishedAt: time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC),
		Grade:      &grade,
	}
	store := &fakeStore{
		runs: map[string]serverpkg.Run{run.ID: run},
		entries: map[string][]serverpkg.Entry{
			run.ID: {
				{
					RunID: run.ID, DomainID: run.DomainID,
					Module: "DNSSEC", Testcase: "dnssec05", Tag: "DS05_ALGO_OK",
					Args: map[string]any{"keytag": uint16(1234), "algo_num": uint8(8)},
				},
				{
					RunID: run.ID, DomainID: run.DomainID,
					Module: "DNSSEC", Testcase: "dnssec05", Tag: "DS05_ALGO_OK",
					Args: map[string]any{"keytag": uint16(5678), "algo_num": uint8(13)},
				},
				{
					RunID: run.ID, DomainID: run.DomainID,
					Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_SIGNED",
				},
			},
		},
		tags:    map[int64][]string{run.DomainID: {"tld"}},
		cohorts: []serverpkg.AnalysisCohort{{ID: 11, SourceType: "tag", SourceTag: "tld", Label: "TLD", AnalysisEnabled: true, SortOrder: 10}},
	}

	projector := NewProjector(store)
	if err := projector.ProjectRun(run.ID); err != nil {
		t.Fatalf("ProjectRun first pass: %v", err)
	}
	first := append([]serverpkg.AnalysisRunDomainFact(nil), store.domainFacts[projectionKey(11, run.ID)]...)

	want := map[string]bool{
		factCategoryDNSKEYAlgorithm + "/8":            true,
		factCategoryDNSKEYAlgorithm + "/13":           true,
		factCategoryDNSSECPosture + "/" + factKeySigned: true,
		factCategoryGrade + "/A":                      true,
		factCategorySeverity + "/OK":                  true,
	}
	if len(first) != len(want) {
		t.Fatalf("expected %d domain-fact rows, got %d: %+v", len(want), len(first), first)
	}
	for _, row := range first {
		if !want[row.Category+"/"+row.Key] {
			t.Fatalf("unexpected row %+v", row)
		}
		if row.CohortID != 11 || row.RunID != run.ID || row.DomainID != run.DomainID {
			t.Fatalf("row identity wrong: %+v", row)
		}
	}

	if err := projector.ProjectRun(run.ID); err != nil {
		t.Fatalf("ProjectRun second pass: %v", err)
	}
	second := store.domainFacts[projectionKey(11, run.ID)]
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("domain facts changed across reprojection:\nfirst=%+v\nsecond=%+v", first, second)
	}
}

func TestNumericArgCoercesCommonJSONShapes(t *testing.T) {
	cases := []struct {
		name string
		raw  any
		want int64
		ok   bool
	}{
		{"uint8", uint8(13), 13, true},
		{"int", int(8), 8, true},
		{"float64", float64(10), 10, true},
		{"string", "15", 15, true},
		{"bogus-string", "not-a-number", 0, false},
		{"nil", nil, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := numericArg(map[string]any{"k": c.raw}, "k")
			if ok != c.ok || got != c.want {
				t.Fatalf("numericArg(%v) = (%d, %v), want (%d, %v)", c.raw, got, ok, c.want, c.ok)
			}
		})
	}
	if _, ok := numericArg(nil, "k"); ok {
		t.Fatalf("nil args should return ok=false")
	}
}
