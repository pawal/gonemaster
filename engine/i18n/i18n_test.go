package i18n

import (
	"io/fs"
	"slices"
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/share"
)

func TestAvailableLocalesIncludesJapanese(t *testing.T) {
	locales := AvailableLocales()
	found := slices.Contains(locales, "ja")
	if !found {
		t.Fatalf("expected ja in available locales, got %v", locales)
	}
}

func TestParsePO(t *testing.T) {
	tests := []struct {
		name       string
		data       string
		wantIDs    map[string]string
		wantMsgs   map[string]string
		absentKeys []string
	}{
		{
			// Continuation lines are concatenated, and one entry can carry
			// several dot-comment keys.
			name: "multi-line entry with two dot-comment keys",
			data: `
#. ZONE:TEST_TAG
#. BASIC:TEST_TAG
msgid ""
"Hello "
"{name}"
msgstr ""
"Hej "
"{name}"
`,
			wantIDs: map[string]string{
				"ZONE:TEST_TAG":  "Hello {name}",
				"BASIC:TEST_TAG": "Hello {name}",
			},
			wantMsgs: map[string]string{
				"ZONE:TEST_TAG":  "Hej {name}",
				"BASIC:TEST_TAG": "Hej {name}",
			},
		},
		{
			// msgctxt wins over a preceding #. comment, so a typo in the
			// comment cannot silently break the lookup.
			name: "msgctxt overrides dot comment",
			data: `
#. NAMESERVER:TYPO_IN_COMMENT
msgctxt "NAMESERVER:N11_NO_EDNS"
msgid "No EDNS from {ns_ip_list}."
msgstr "Ingen EDNS från {ns_ip_list}."
`,
			wantIDs:    map[string]string{"NAMESERVER:N11_NO_EDNS": "No EDNS from {ns_ip_list}."},
			wantMsgs:   map[string]string{"NAMESERVER:N11_NO_EDNS": "Ingen EDNS från {ns_ip_list}."},
			absentKeys: []string{"NAMESERVER:TYPO_IN_COMMENT"},
		},
		{
			name: "msgctxt without a dot comment",
			data: `
msgctxt "ZONE:Z01_MNAME_NOT_RESOLVE"
msgid "SOA MNAME is not a master for the zone."
msgstr "SOA MNAME är inte en master för zonen."
`,
			wantIDs:  map[string]string{"ZONE:Z01_MNAME_NOT_RESOLVE": "SOA MNAME is not a master for the zone."},
			wantMsgs: map[string]string{"ZONE:Z01_MNAME_NOT_RESOLVE": "SOA MNAME är inte en master för zonen."},
		},
		{
			// The dot-comment fallback is the intended path for shared
			// messages like TEST_CASE_END.
			name: "dot comment fallback for a multi-key entry",
			data: `
#. ADDRESS:TEST_CASE_END
#. BASIC:TEST_CASE_END
#. ZONE:TEST_CASE_END
msgid "TEST_CASE_END {testcase}."
msgstr "Testfall {testcase} avslutat."
`,
			wantIDs: map[string]string{
				"ADDRESS:TEST_CASE_END": "TEST_CASE_END {testcase}.",
				"BASIC:TEST_CASE_END":   "TEST_CASE_END {testcase}.",
				"ZONE:TEST_CASE_END":    "TEST_CASE_END {testcase}.",
			},
			wantMsgs: map[string]string{
				"ADDRESS:TEST_CASE_END": "Testfall {testcase} avslutat.",
				"BASIC:TEST_CASE_END":   "Testfall {testcase} avslutat.",
				"ZONE:TEST_CASE_END":    "Testfall {testcase} avslutat.",
			},
		},
		{
			// A msgctxt that is not MODULE:TAG format must not clobber the
			// #. key, which has to survive.
			name: "malformed msgctxt keeps the dot-comment key",
			data: `
#. ZONE:Z02_NO_NS
msgctxt "not-a-valid-key"
msgid "No NS records."
msgstr "Inga NS-poster."
`,
			wantMsgs: map[string]string{"ZONE:Z02_NO_NS": "Inga NS-poster."},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msgs, ids := parsePO(tc.data)

			for key, want := range tc.wantIDs {
				if ids[key] != want {
					t.Errorf("ids[%q] = %q, want %q", key, ids[key], want)
				}
			}
			for key, want := range tc.wantMsgs {
				if msgs[key] != want {
					t.Errorf("msgs[%q] = %q, want %q", key, msgs[key], want)
				}
			}
			for _, key := range tc.absentKeys {
				if _, ok := ids[key]; ok {
					t.Errorf("ids must not contain %q, got %v", key, ids)
				}
				if _, ok := msgs[key]; ok {
					t.Errorf("msgs must not contain %q, got %v", key, msgs)
				}
			}
		})
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

// TestDS07NotSignedOnServerWording pins the English rendering of
// DNSSEC:DS07_NOT_SIGNED_ON_SERVER. The tag is RRSIG-based, so it also fires
// for zones that publish DNSKEYs without any covering signature; the older
// "respond with no DNSKEY" wording was false for those zones. Every locale that
// carries the entry must still expand {servers}.
func TestDS07NotSignedOnServerWording(t *testing.T) {
	args := map[string]any{"servers": "ns1.example.com/192.0.2.1"}
	expected := `The following name servers respond without a signed DNSKEY RRset (unsigned child zone). Name servers: "ns1.example.com/192.0.2.1".`

	out, found := TranslateWithStatus("en", "dnssec", "DS07_NOT_SIGNED_ON_SERVER", args)
	if !found {
		t.Fatalf("expected english message for DS07_NOT_SIGNED_ON_SERVER")
	}
	if out != expected {
		t.Fatalf("unexpected english message:\n got: %q\nwant: %q", out, expected)
	}

	for _, locale := range AvailableLocales() {
		out, found := TranslateWithStatus(locale, "dnssec", "DS07_NOT_SIGNED_ON_SERVER", args)
		if !found {
			continue // catalog has no entry for this locale
		}
		if strings.Contains(out, "{servers}") {
			t.Errorf("locale %s: unresolved placeholder: %q", locale, out)
		}
		if !strings.Contains(out, "ns1.example.com") {
			t.Errorf("locale %s: servers argument not interpolated: %q", locale, out)
		}
	}
}

// The English text is taken from the first catalog (alphabetically) that
// carries the msgid, so a catalog whose msgid drifts silently stops
// contributing the English wording it appears to define.
func TestDS07NotSignedOnServerMsgidIdenticalInAllCatalogs(t *testing.T) {
	const key = "DNSSEC:DS07_NOT_SIGNED_ON_SERVER"
	want := `The following name servers respond without a signed DNSKEY RRset (unsigned child zone). Name servers: "{servers}".`

	paths, err := fs.Glob(share.POFiles, "lang/*.po")
	if err != nil {
		t.Fatalf("glob catalogs: %v", err)
	}
	seen := 0
	for _, poPath := range paths {
		data, err := share.POFiles.ReadFile(poPath)
		if err != nil {
			t.Fatalf("read %s: %v", poPath, err)
		}
		_, ids := parsePO(string(data))
		msgid, ok := ids[key]
		if !ok {
			continue
		}
		seen++
		if msgid != want {
			t.Errorf("%s: msgid for %s differs:\n got: %q\nwant: %q", poPath, key, msgid, want)
		}
	}
	if seen == 0 {
		t.Fatalf("no catalog carries %s", key)
	}
}
