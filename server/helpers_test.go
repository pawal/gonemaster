package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Error bodies echo user input, so the encoder must keep escaping HTML.
func TestWriteErrorEscapesHTML(t *testing.T) {
	w := httptest.NewRecorder()

	writeError(w, http.StatusBadRequest, "invalid_json", `json: unknown field "<script>alert(1)</script>"`, nil)

	body := w.Body.String()
	for _, banned := range []string{"<", ">", "&"} {
		if strings.Contains(body, banned) {
			t.Fatalf("body contains a literal %q: %s", banned, body)
		}
	}
	if !strings.Contains(body, `\u003cscript\u003e`) {
		t.Fatalf("expected the escaped form in the body: %s", body)
	}
}

// Escaping must not change what a JSON client decodes.
func TestWriteErrorStaysDecodable(t *testing.T) {
	const message = `bad <input> & "quotes"`
	w := httptest.NewRecorder()

	writeError(w, http.StatusBadRequest, "invalid_json", message, nil)

	var got ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v: %s", err, w.Body.String())
	}
	if got.Error.Message != message {
		t.Fatalf("message = %q, want %q", got.Error.Message, message)
	}
}

func TestWriteJSONEscapesHTML(t *testing.T) {
	w := httptest.NewRecorder()

	writeJSON(w, http.StatusOK, map[string]string{"domain": "<b>example.com</b>"})

	if body := w.Body.String(); strings.Contains(body, "<b>") {
		t.Fatalf("body contains unescaped markup: %s", body)
	}
}
