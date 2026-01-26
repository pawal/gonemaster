package i18n

import (
	"strings"
	"testing"
)

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

func TestTranslateWithStatusMissing(t *testing.T) {
	msg, found := TranslateWithStatus("en", "basic", "B99_DOES_NOT_EXIST", nil)
	if found {
		t.Fatalf("expected missing translation to return found=false")
	}
	if msg != "BASIC:B99_DOES_NOT_EXIST" {
		t.Fatalf("unexpected fallback message: %q", msg)
	}
}
