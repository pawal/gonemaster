package i18n

import (
	"slices"
	"strings"
	"testing"
)

func TestAvailableLocalesIncludesJapanese(t *testing.T) {
	locales := AvailableLocales()
	found := slices.Contains(locales, "ja")
	if !found {
		t.Fatalf("expected ja in available locales, got %v", locales)
	}
}

func TestParsePOExtractsMessages(t *testing.T) {
	data := `
#. ZONE:TEST_TAG
#. BASIC:TEST_TAG
msgid ""
"Hello "
"{name}"
msgstr ""
"Hej "
"{name}"
`
	msgs, ids := parsePO(data)
	key := "ZONE:TEST_TAG"
	key2 := "BASIC:TEST_TAG"
	if ids[key] != "Hello {name}" {
		t.Fatalf("unexpected msgid: %q", ids[key])
	}
	if ids[key2] != "Hello {name}" {
		t.Fatalf("unexpected msgid: %q", ids[key2])
	}
	if msgs[key] != "Hej {name}" {
		t.Fatalf("unexpected msgstr: %q", msgs[key])
	}
	if msgs[key2] != "Hej {name}" {
		t.Fatalf("unexpected msgstr: %q", msgs[key2])
	}
}

func TestTranslateEnglishFallbackAndLocaleNormalization(t *testing.T) {
	english := Translate("en", "basic", "B01_ROOT_HAS_NO_PARENT", nil)
	if english == "" {
		t.Fatalf("expected english message")
	}

	da := Translate("da", "basic", "B01_ROOT_HAS_NO_PARENT", nil)
	if da == "" {
		t.Fatalf("expected danish message")
	}
	if da == english {
		t.Fatalf("expected danish translation, got english")
	}

	daRegion := Translate("da_DK.UTF-8", "basic", "B01_ROOT_HAS_NO_PARENT", nil)
	if daRegion != da {
		t.Fatalf("expected locale normalization to match base locale")
	}
}

func TestTranslateJapaneseUsesLocaleCatalog(t *testing.T) {
	out := Translate("ja", "zone", "Z12_NO_CSYNC", map[string]any{
		"servers": "ns1.example/192.0.2.1",
	})
	if out == "" {
		t.Fatal("expected japanese translation")
	}
	if strings.Contains(out, "No CSYNC record found") {
		t.Fatalf("expected non-english translation, got %q", out)
	}
	if !strings.Contains(out, "ns1.example") {
		t.Fatalf("expected interpolation to include servers argument, got %q", out)
	}
}

func TestTranslateInterpolatesArgs(t *testing.T) {
	out := Translate("en", "basic", "B02_NO_DELEGATION", map[string]any{
		"domain": "example",
	})
	expected := "There is no delegation (name servers) for \"example\" which means it does not exist as a zone."
	if out != expected {
		t.Fatalf("unexpected translation: %q", out)
	}
	if strings.Contains(out, "{domain}") {
		t.Fatalf("expected interpolation to replace {domain}")
	}
}

func TestInterpolateFallbacks(t *testing.T) {
	out := interpolate("Hello {name}", map[string]any{"name": "world"})
	if out != "Hello world" {
		t.Fatalf("unexpected interpolation: %q", out)
	}
}

func TestInterpolateFormatsServersList(t *testing.T) {
	out := interpolate("Servers: {servers}.", map[string]any{
		"servers": []map[string]any{
			{"ns": "ns1.example.org", "address": "192.0.2.1"},
			{"ns": "ns2.example.org"},
		},
	})
	expected := "Servers: ns1.example.org/192.0.2.1;ns2.example.org."
	if out != expected {
		t.Fatalf("unexpected interpolation: %q", out)
	}
}

func TestInterpolateFormatsLargeIntegerFloats(t *testing.T) {
	// JSON round-trip turns numeric args into float64. Without explicit
	// formatting, fmt.Sprint(float64(1777722813)) renders "1.777722813e+09".
	out := interpolate("ZONEMD serial {serial}.", map[string]any{
		"serial": float64(1777722813),
	})
	expected := "ZONEMD serial 1777722813."
	if out != expected {
		t.Fatalf("unexpected interpolation: %q", out)
	}
}

func TestInterpolateFormatsServerMap(t *testing.T) {
	out := interpolate("Server: {server}.", map[string]any{
		"server": map[string]any{
			"ns":      "ns1.example.org",
			"address": "2001:db8::53",
		},
	})
	expected := "Server: ns1.example.org/2001:db8::53."
	if out != expected {
		t.Fatalf("unexpected interpolation: %q", out)
	}
}

func TestTranslateWithStatusMissing(t *testing.T) {
	msg, found := TranslateWithStatus("en", "basic", "B99_DOES_NOT_EXIST", nil)
	if found {
		t.Fatalf("expected missing translation to return found=false")
	}
	if msg != "BASIC:B99_DOES_NOT_EXIST" {
		t.Fatalf("unexpected fallback message: %q", msg)
	}
}

func TestParsePOMsgctxtOverridesDotComment(t *testing.T) {
	// When msgctxt is present it must win over any preceding #. comment,
	// so a typo in the comment cannot silently break the lookup.
	data := `
#. NAMESERVER:TYPO_IN_COMMENT
msgctxt "NAMESERVER:N11_NO_EDNS"
msgid "No EDNS from {ns_ip_list}."
msgstr "Ingen EDNS från {ns_ip_list}."
`
	msgs, ids := parsePO(data)

	if ids["NAMESERVER:N11_NO_EDNS"] != "No EDNS from {ns_ip_list}." {
		t.Fatalf("expected msgctxt key to be used, got ids=%v", ids)
	}
	if msgs["NAMESERVER:N11_NO_EDNS"] != "Ingen EDNS från {ns_ip_list}." {
		t.Fatalf("expected msgctxt key to be used, got msgs=%v", msgs)
	}
	if _, ok := ids["NAMESERVER:TYPO_IN_COMMENT"]; ok {
		t.Fatal("dot-comment key must not appear when msgctxt is present")
	}
}

func TestParsePOMsgctxtOnly(t *testing.T) {
	// msgctxt with no preceding #. comment works on its own.
	data := `
msgctxt "ZONE:Z01_MNAME_NOT_RESOLVE"
msgid "SOA MNAME is not a master for the zone."
msgstr "SOA MNAME är inte en master för zonen."
`
	msgs, ids := parsePO(data)

	if ids["ZONE:Z01_MNAME_NOT_RESOLVE"] != "SOA MNAME is not a master for the zone." {
		t.Fatalf("unexpected msgid: %v", ids)
	}
	if msgs["ZONE:Z01_MNAME_NOT_RESOLVE"] != "SOA MNAME är inte en master för zonen." {
		t.Fatalf("unexpected msgstr: %v", msgs)
	}
}

func TestParsePODotCommentFallbackForMultiKey(t *testing.T) {
	// Multi-key entries (no msgctxt) still work via #. comment fallback,
	// which is the intended path for shared messages like TEST_CASE_END.
	data := `
#. ADDRESS:TEST_CASE_END
#. BASIC:TEST_CASE_END
#. ZONE:TEST_CASE_END
msgid "TEST_CASE_END {testcase}."
msgstr "Testfall {testcase} avslutat."
`
	msgs, ids := parsePO(data)

	for _, key := range []string{"ADDRESS:TEST_CASE_END", "BASIC:TEST_CASE_END", "ZONE:TEST_CASE_END"} {
		if ids[key] != "TEST_CASE_END {testcase}." {
			t.Fatalf("expected %q in ids, got %v", key, ids)
		}
		if msgs[key] != "Testfall {testcase} avslutat." {
			t.Fatalf("expected %q in msgs, got %v", key, msgs)
		}
	}
}

// TestDS01AlgoDeprecatedAllLocalesExpandArgs verifies that the
// DNSSEC:DS01_DS_ALGO_DEPRECATED translation resolves every placeholder in all
// supported locales. This guards against stale PO files that still use the old
// upstream placeholder names ({domain}, {ds_algo_mnemo}, {addresses}) which
// the engine never emits.
func TestDS01AlgoDeprecatedAllLocalesExpandArgs(t *testing.T) {
	args := map[string]any{
		"keytag":        12345,
		"ds_algo_num":   1,
		"ds_algo_descr": "SHA-1",
		"servers":       "ns1.example.com/192.0.2.1",
	}
	staleKeys := []string{"{domain}", "{ds_algo_mnemo}", "{addresses}"}
	for _, locale := range AvailableLocales() {
		out := Translate(locale, "dnssec", "DS01_DS_ALGO_DEPRECATED", args)
		if out == "" {
			continue // no translation for this locale; skip
		}
		for _, stale := range staleKeys {
			if strings.Contains(out, stale) {
				t.Errorf("locale %s: translation still contains stale placeholder %q: %q", locale, stale, out)
			}
		}
		if strings.Contains(out, "{keytag}") || strings.Contains(out, "{ds_algo_num}") ||
			strings.Contains(out, "{ds_algo_descr}") || strings.Contains(out, "{servers}") {
			t.Errorf("locale %s: unresolved placeholder in translation: %q", locale, out)
		}
	}
}

func TestParsePOMsgctxtMalformedIsIgnored(t *testing.T) {
	// A msgctxt that is not MODULE:TAG format must not clobber #. keys.
	data := `
#. ZONE:Z02_NO_NS
msgctxt "not-a-valid-key"
msgid "No NS records."
msgstr "Inga NS-poster."
`
	msgs, _ := parsePO(data)

	// The dot-comment key should survive because msgctxt had no valid colon-pair.
	if msgs["ZONE:Z02_NO_NS"] != "Inga NS-poster." {
		t.Fatalf("expected dot-comment fallback when msgctxt is malformed, got msgs=%v", msgs)
	}
}
