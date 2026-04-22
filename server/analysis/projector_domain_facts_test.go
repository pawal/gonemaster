package analysis

import (
	"sort"
	"testing"

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

func TestExtractSignedStatus(t *testing.T) {
	signed := RunInput{
		Entries: []serverpkg.Entry{
			{Module: "DNSSEC", Testcase: "dnssec05", Tag: "DS05_ALGO_OK"},
		},
	}
	got := extractSignedStatus(signed)
	if len(got) != 1 || got[0].key != factKeySigned {
		t.Fatalf("expected signed fact, got %+v", got)
	}

	unsigned := RunInput{
		Entries: []serverpkg.Entry{
			{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED"},
			{Module: "DNSSEC", Testcase: "dnssec05", Tag: "DS05_ALGO_OK"},
		},
	}
	got = extractSignedStatus(unsigned)
	if len(got) != 1 || got[0].key != factKeyUnsigned {
		t.Fatalf("DS07_NOT_SIGNED should win regardless of DS05 presence, got %+v", got)
	}

	noDNSSEC := RunInput{
		Entries: []serverpkg.Entry{
			{Module: "BASIC", Testcase: "basic01", Tag: "B01_OK"},
		},
	}
	if got := extractSignedStatus(noDNSSEC); len(got) != 0 {
		t.Fatalf("no DNSSEC signal should emit nothing, got %+v", got)
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
	sawSigned := false
	for _, f := range got {
		if f.category == factCategoryDNSKEYAlgorithm && f.key == "13" {
			sawAlgo = true
		}
		if f.category == factCategorySigned && f.key == factKeySigned {
			sawSigned = true
		}
	}
	if !sawAlgo {
		t.Fatalf("expected DNSKEY algorithm fact, got %+v", got)
	}
	if !sawSigned {
		t.Fatalf("expected signed fact, got %+v", got)
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
		{category: factCategorySigned, key: factKeySigned},
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
