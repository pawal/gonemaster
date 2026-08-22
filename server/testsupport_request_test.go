package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

// echoRequest records the request it is served, so the request helpers can be
// asserted on without a real route.
type echoRequest struct {
	method      string
	path        string
	host        string
	remoteAddr  string
	contentType string
	header      http.Header
	body        string
}

func (e *echoRequest) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		e.method = req.Method
		e.path = req.URL.Path
		e.host = req.Host
		e.remoteAddr = req.RemoteAddr
		e.contentType = req.Header.Get("Content-Type")
		e.header = req.Header.Clone()
		if req.Body != nil {
			raw, _ := io.ReadAll(req.Body)
			e.body = string(raw)
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func TestDoHandlerSendsNoBodyForNil(t *testing.T) {
	echo := &echoRequest{}
	resp := doHandler(t, echo.handler(), http.MethodGet, "/api/v1/jobs", nil)

	if resp.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusNoContent)
	}
	if echo.method != http.MethodGet || echo.path != "/api/v1/jobs" {
		t.Fatalf("served %s %s", echo.method, echo.path)
	}
	if echo.body != "" {
		t.Fatalf("expected no body, got %q", echo.body)
	}
	// A nil body must not carry a Content-Type; the handlers branch on it.
	if echo.contentType != "" {
		t.Fatalf("expected no Content-Type, got %q", echo.contentType)
	}
}

func TestDoHandlerBodyForms(t *testing.T) {
	type payload struct {
		Domain string `json:"domain"`
	}
	cases := []struct {
		name string
		body any
		want string
	}{
		{"string", `{"domain":"a.example"}`, `{"domain":"a.example"}`},
		{"bytes", []byte(`{"domain":"b.example"}`), `{"domain":"b.example"}`},
		{"reader", stringReader(`{"domain":"c.example"}`), `{"domain":"c.example"}`},
		{"marshalled", payload{Domain: "d.example"}, `{"domain":"d.example"}`},
		// An empty string is a body, unlike nil: some handlers reject one and
		// accept the other.
		{"empty string", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			echo := &echoRequest{}
			doHandler(t, echo.handler(), http.MethodPost, "/api/v1/jobs", tc.body)

			if echo.body != tc.want {
				t.Fatalf("body = %q, want %q", echo.body, tc.want)
			}
			if echo.contentType != "application/json" {
				t.Fatalf("Content-Type = %q, want application/json", echo.contentType)
			}
		})
	}
}

func TestDoHandlerRequestOptions(t *testing.T) {
	echo := &echoRequest{}
	doHandler(t, echo.handler(), http.MethodPost, "/api/v1/jobs", `{}`,
		withOrigin("https://admin.example"),
		withHost("admin.example"),
		withRemoteAddr("192.0.2.7:1234"),
		withBearer("s3cret"),
		withHeader("X-Request-Id", "req-1"),
		withCookie(&http.Cookie{Name: "session", Value: "abc"}),
		withRequest(func(req *http.Request) { req.Header.Set("X-Forwarded-Proto", "https") }),
	)

	if got := echo.header.Get("Origin"); got != "https://admin.example" {
		t.Fatalf("Origin = %q", got)
	}
	if echo.host != "admin.example" {
		t.Fatalf("Host = %q", echo.host)
	}
	if echo.remoteAddr != "192.0.2.7:1234" {
		t.Fatalf("RemoteAddr = %q", echo.remoteAddr)
	}
	if got := echo.header.Get("Authorization"); got != "Bearer s3cret" {
		t.Fatalf("Authorization = %q", got)
	}
	if got := echo.header.Get("X-Request-Id"); got != "req-1" {
		t.Fatalf("X-Request-Id = %q", got)
	}
	if got := echo.header.Get("Cookie"); got != "session=abc" {
		t.Fatalf("Cookie = %q", got)
	}
	if got := echo.header.Get("X-Forwarded-Proto"); got != "https" {
		t.Fatalf("X-Forwarded-Proto = %q", got)
	}
}

func TestDoHandlerNoContentTypeDropsTheDefault(t *testing.T) {
	echo := &echoRequest{}
	doHandler(t, echo.handler(), http.MethodPost, "/jobs", `{}`, noContentType())

	if echo.contentType != "" {
		t.Fatalf("expected Content-Type dropped, got %q", echo.contentType)
	}
	if echo.body != `{}` {
		t.Fatalf("expected the body kept, got %q", echo.body)
	}
}

func TestDoHandlerContentTypeOnANilBody(t *testing.T) {
	echo := &echoRequest{}
	doHandler(t, echo.handler(), http.MethodDelete, "/api/v1/scoring-config", nil,
		withHeader("Content-Type", "application/json"))

	if echo.contentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", echo.contentType)
	}
}

func TestDoJSONServesTheServerHandler(t *testing.T) {
	srv := New(DefaultConfig())
	resp := doJSON(t, srv, http.MethodGet, "/api/v1/jobs", nil)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", resp.Code, http.StatusOK, resp.Body.String())
	}
	// srv.Handler() wraps the mux in the security-header middleware.
	if got := resp.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("expected the security headers, got X-Content-Type-Options=%q", got)
	}
}

// stringReader returns an io.Reader that is not one of requestBody's special
// cases, so the io.Reader branch is what handles it.
func stringReader(s string) io.Reader {
	return &sliceReader{data: []byte(s)}
}

type sliceReader struct {
	data []byte
	off  int
}

func (r *sliceReader) Read(p []byte) (int, error) {
	if r.off >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.off:])
	r.off += n
	return n, nil
}

func TestCSRFOriginMatrixSendsAbsentSameAndCrossOrigin(t *testing.T) {
	var seen []string
	// Stands in for a CSRF-checked endpoint: an Origin naming another host is
	// refused the way writeError would report it.
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		origin := req.Header.Get("Origin")
		seen = append(seen, origin)
		if origin != "" && origin != "http://"+req.Host {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"code":"csrf_origin_mismatch"}}`))
			return
		}
		w.WriteHeader(http.StatusCreated)
	})

	csrfOriginMatrix(t, http.StatusCreated, func(t *testing.T, opts ...reqOpt) *httptest.ResponseRecorder {
		return doHandler(t, handler, http.MethodPost, "/api/v1/jobs", `{}`, opts...)
	})

	want := []string{"", "http://example.com", "https://evil.example"}
	if !slices.Equal(seen, want) {
		t.Fatalf("origins sent = %v, want %v", seen, want)
	}
}
