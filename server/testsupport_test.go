package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// reqOpt customizes a request before it is served.
type reqOpt func(*http.Request)

// withHeader sets a request header.
func withHeader(key string, value string) reqOpt {
	return func(req *http.Request) { req.Header.Set(key, value) }
}

// withOrigin sets the Origin header, for the CSRF checks.
func withOrigin(origin string) reqOpt {
	return withHeader("Origin", origin)
}

// withHost sets the request Host, which is a field rather than a header.
func withHost(host string) reqOpt {
	return func(req *http.Request) { req.Host = host }
}

// withRemoteAddr sets the peer address the rate limiter and access log read.
func withRemoteAddr(addr string) reqOpt {
	return func(req *http.Request) { req.RemoteAddr = addr }
}

// withBearer sets an admin bearer token.
func withBearer(token string) reqOpt {
	return withHeader("Authorization", "Bearer "+token)
}

// withCookie adds a cookie.
func withCookie(c *http.Cookie) reqOpt {
	return func(req *http.Request) { req.AddCookie(c) }
}

// noContentType drops the Content-Type doJSON sets for a request with a body.
func noContentType() reqOpt {
	return func(req *http.Request) { req.Header.Del("Content-Type") }
}

// withRequest is the escape hatch for a one-off request tweak.
func withRequest(fn func(*http.Request)) reqOpt {
	return fn
}

// doJSON serves method and path against srv's root handler and returns the
// recorder. See requestBody for how body is encoded.
func doJSON(t *testing.T, srv *Server, method string, path string, body any, opts ...reqOpt) *httptest.ResponseRecorder {
	t.Helper()
	return doHandler(t, srv.Handler(), method, path, body, opts...)
}

// doHandler is doJSON against a handler built by the test, for the middleware
// and router tests that deliberately bypass srv.Handler().
func doHandler(t *testing.T, h http.Handler, method string, path string, body any, opts ...reqOpt) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, requestBody(t, body))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, opt := range opts {
		opt(req)
	}
	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, req)
	return resp
}

// requestBody encodes a doJSON body: nil sends no body, a string or []byte is
// sent verbatim, an io.Reader is passed through, anything else is marshalled.
// An empty string still sends a body, which some handlers distinguish.
func requestBody(t *testing.T, body any) io.Reader {
	t.Helper()
	switch v := body.(type) {
	case nil:
		return nil
	case string:
		return strings.NewReader(v)
	case []byte:
		return bytes.NewReader(v)
	case io.Reader:
		return v
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		return bytes.NewReader(raw)
	}
}
