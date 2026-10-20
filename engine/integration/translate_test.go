package integration

import (
	"slices"
	"testing"
)

func TestParseTranslateRequestRejectsUnknownFields(t *testing.T) {
	_, err := ParseTranslateRequest([]byte(`{"language":"en","entries":[],"locale":"sv"}`))
	if err == nil || err.(*Error).Code != CodeInvalidRequest {
		t.Fatalf("err = %v, want invalid_request", err)
	}
}

func TestTranslate(t *testing.T) {
	entry := TranslateEntry{Module: "System", Tag: "GLOBAL_VERSION", Args: map[string]any{"version": "v1"}}
	cases := []struct {
		name     string
		language string
		entry    TranslateEntry
		want     Translation
	}{
		{"english", "en", entry, Translation{Message: "Using version v1 of the Gonemaster engine.", Found: true}},
		{"empty language is english", "", entry, Translation{Message: "Using version v1 of the Gonemaster engine.", Found: true}},
		{"swedish", "sv", entry, Translation{Message: "Använder version v1 av Gonemasters testmotor.", Found: true}},
		{"full locale", "sv_SE.UTF-8", entry, Translation{Message: "Använder version v1 av Gonemasters testmotor.", Found: true}},
		{"unknown language falls back", "xx", entry, Translation{Message: "Using version v1 of the Gonemaster engine.", Found: true}},
		{"unknown tag", "en", TranslateEntry{Module: "Basic", Tag: "NOPE", Args: map[string]any{"n": 1}}, Translation{Message: "BASIC:NOPE", Found: false}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Translate(TranslateRequest{Language: tc.language, Entries: []TranslateEntry{tc.entry}})
			if len(got) != 1 || got[0] != tc.want {
				t.Fatalf("Translate = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestTranslateKeepsOrder(t *testing.T) {
	got := Translate(TranslateRequest{Language: "en", Entries: []TranslateEntry{
		{Module: "Basic", Tag: "NOPE"},
		{Module: "System", Tag: "GLOBAL_VERSION", Args: map[string]any{"version": "v2"}},
	}})
	if len(got) != 2 || got[0].Found || !got[1].Found || got[1].Message != "Using version v2 of the Gonemaster engine." {
		t.Fatalf("Translate = %+v", got)
	}
}

func TestLanguagesIncludeEnglishAndTheCatalogs(t *testing.T) {
	langs := Languages()
	for _, want := range []string{"en", "sv", "fr"} {
		if !slices.Contains(langs, want) {
			t.Errorf("Languages lacks %s: %v", want, langs)
		}
	}
	if !slices.IsSorted(langs) {
		t.Fatalf("Languages not sorted: %v", langs)
	}
}
