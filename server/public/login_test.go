package public

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestLoginTextCoversShippedLocales(t *testing.T) {
	for _, lang := range hreflangLangs {
		s, ok := loginText[lang]
		if !ok {
			t.Errorf("no login strings for %s", lang)
			continue
		}
		for name, v := range map[string]string{"Title": s.Title, "Label": s.Label, "Placeholder": s.Placeholder, "Submit": s.Submit, "Invalid": s.Invalid} {
			if v == "" {
				t.Errorf("%s: %s is empty", lang, name)
			}
		}
	}
	for lang := range loginText {
		if !slices.Contains(hreflangLangs, lang) {
			t.Errorf("login strings for unshipped locale %s", lang)
		}
	}
}

func TestRenderLoginFailedShowsError(t *testing.T) {
	for failed, want := range map[bool]bool{true: true, false: false} {
		rec := httptest.NewRecorder()
		RenderLogin(rec, httptest.NewRequest(http.MethodGet, "/public/", nil), "/public/_auth/login.css", failed)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("failed=%v: status = %d, want 401", failed, rec.Code)
		}
		if got := strings.Contains(rec.Body.String(), "Invalid token"); got != want {
			t.Errorf("failed=%v: error line shown = %v, want %v", failed, got, want)
		}
	}
}

func TestRenderLoginEscapesStylesheetHref(t *testing.T) {
	rec := httptest.NewRecorder()
	RenderLogin(rec, httptest.NewRequest(http.MethodGet, "/public/", nil), `/x"><script>`, false)
	if strings.Contains(rec.Body.String(), "<script>") {
		t.Error("stylesheet href is not escaped")
	}
}

func TestRenderLoginHasNoInlineStyleOrScript(t *testing.T) {
	rec := httptest.NewRecorder()
	RenderLogin(rec, httptest.NewRequest(http.MethodGet, "/public/", nil), "/public/_auth/login.css", true)
	body := rec.Body.String()
	for _, banned := range []string{"<style", "style=", "<script"} {
		if strings.Contains(body, banned) {
			t.Errorf("login page contains %q", banned)
		}
	}
}
