package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/internal/tbtest"
)

// recorderWith returns a recorder holding the given status and body, standing
// in for a served response.
func recorderWith(status int, body string) *httptest.ResponseRecorder {
	resp := httptest.NewRecorder()
	resp.Code = status
	resp.Body.WriteString(body)
	return resp
}

func TestMustJSONDecodesTheBody(t *testing.T) {
	type payload struct {
		Domain string `json:"domain"`
		Total  int    `json:"total"`
	}
	got := mustJSON[payload](t, recorderWith(http.StatusOK, `{"domain":"a.example","total":3}`), http.StatusOK)

	if got.Domain != "a.example" || got.Total != 3 {
		t.Fatalf("decoded %+v", got)
	}
}

func TestMustJSONFailsOnAWrongStatus(t *testing.T) {
	tbtest.MustFail(t, "status = 404, want 200", func(tb *tbtest.TB) {
		mustJSON[map[string]any](tb, recorderWith(http.StatusNotFound, `{}`), http.StatusOK)
	})
}

func TestMustJSONFailsOnAnUndecodableBody(t *testing.T) {
	// The raw body belongs in the message; without it the failure is unreadable.
	tbtest.MustFail(t, "decode response", func(tb *tbtest.TB) {
		mustJSON[map[string]any](tb, recorderWith(http.StatusOK, `not json`), http.StatusOK)
	})
	tbtest.MustFail(t, "not json", func(tb *tbtest.TB) {
		mustJSON[map[string]any](tb, recorderWith(http.StatusOK, `not json`), http.StatusOK)
	})
}

func TestWantStatusChecksTheCode(t *testing.T) {
	wantStatus(t, recorderWith(http.StatusAccepted, ``), http.StatusAccepted)

	tbtest.MustFail(t, "status = 400, want 200: bad", func(tb *tbtest.TB) {
		wantStatus(tb, recorderWith(http.StatusBadRequest, `bad`), http.StatusOK)
	})
}

func TestWantErrorCodeChecksStatusAndCode(t *testing.T) {
	body := wantErrorCode(t,
		recorderWith(http.StatusNotFound, `{"error":{"code":"not_found","message":"no such job"}}`),
		http.StatusNotFound, "not_found")

	if body.Error.Message != "no such job" {
		t.Fatalf("expected the body returned for further asserts, got %+v", body)
	}
}

func TestWantErrorCodeReportsAMismatchedCode(t *testing.T) {
	tb := &tbtest.TB{}
	wantErrorCode(tb, recorderWith(http.StatusBadRequest, `{"error":{"code":"other"}}`),
		http.StatusBadRequest, "not_found")

	// A wrong code is an Errorf, not a Fatalf, so the test keeps running.
	if len(tb.Errs) != 1 {
		t.Fatalf("expected one Errorf, got %v", tb.Errs)
	}
	if !strings.Contains(tb.Errs[0], `"other"`) || !strings.Contains(tb.Errs[0], `"not_found"`) {
		t.Fatalf("expected both codes in the message, got %q", tb.Errs[0])
	}
}

func TestWantErrorCodeFailsOnAWrongStatus(t *testing.T) {
	tbtest.MustFail(t, "status = 500, want 400", func(tb *tbtest.TB) {
		wantErrorCode(tb, recorderWith(http.StatusInternalServerError, `{"error":{"code":"x"}}`),
			http.StatusBadRequest, "x")
	})
}
