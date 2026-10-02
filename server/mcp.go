package server

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/mcpbridge"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mcpWriteDeadlineSlack extends the write deadline past the longest tool call.
const mcpWriteDeadlineSlack = 30 * time.Second

// mcpHandler serves the MCP tools at /api/v1/mcp; the admin auth runs before it.
func (s *Server) mcpHandler() http.Handler {
	streamable := mcp.NewStreamableHTTPHandler(s.mcpServerFor, &mcp.StreamableHTTPOptions{
		Stateless:                    true,
		DisableLocalhostProtection:   true,
		PropagateRequestCancellation: true,
		Logger:                       s.logger,
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.liveConfig().MCPEnabled {
			writeError(w, http.StatusNotFound, "not_found", "mcp endpoint disabled", nil)
			return
		}
		if s.authTokens().enabled && !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeError(w, http.StatusUnauthorized, "unauthorized", "bearer token required", nil)
			return
		}
		// A test_domain call outlives the server's write timeout.
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(mcpbridge.MaxTestTimeout + mcpWriteDeadlineSlack))
		streamable.ServeHTTP(w, r)
	})
}

// mcpServerFor builds a per-request tool server that calls the admin API in-process.
func (s *Server) mcpServerFor(r *http.Request) *mcp.Server {
	hc := &http.Client{Transport: &inprocTransport{handler: s.mux, outer: r}}
	api, err := mcpbridge.NewClient("http://inproc/api/v1", "", hc)
	if err != nil {
		return nil
	}
	api.WithHints("the bearer token was rejected", "the admin API is not mounted at /api/v1")
	return mcpbridge.NewServer(api, mcpbridge.Options{
		AllowWrite: s.liveConfig().MCPAllowWrite,
		Name:       "gonemaster-server",
		Version:    engine.VersionFull(),
		Logger:     s.logger,
		OnToolCall: func(tool string, _ time.Duration, failed bool) { s.metrics.ObserveMCPToolCall(tool, failed) },
	})
}

// inprocTransport serves admin API requests in-process under the caller's credential.
type inprocTransport struct {
	handler http.Handler
	outer   *http.Request
}

func (t *inprocTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	if id := requestIDFromContext(t.outer.Context()); id != "" {
		ctx = context.WithValue(ctx, requestIDContextKey, id)
	}
	inner := req.Clone(ctx)
	inner.RequestURI = req.URL.RequestURI()
	inner.Host = t.outer.Host
	inner.RemoteAddr = t.outer.RemoteAddr
	inner.Header.Del("Authorization")
	if auth := t.outer.Header.Get("Authorization"); auth != "" {
		inner.Header.Set("Authorization", auth)
	}
	rec := &inprocResponse{header: http.Header{}, status: http.StatusOK}
	t.handler.ServeHTTP(rec, inner)
	return &http.Response{
		StatusCode:    rec.status,
		Status:        http.StatusText(rec.status),
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        rec.header,
		Body:          io.NopCloser(bytes.NewReader(rec.body.Bytes())),
		ContentLength: int64(rec.body.Len()),
		Request:       req,
	}, nil
}

// inprocResponse captures an in-process handler's response.
type inprocResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
	wrote  bool
}

func (r *inprocResponse) Header() http.Header { return r.header }

func (r *inprocResponse) WriteHeader(status int) {
	if !r.wrote {
		r.status = status
		r.wrote = true
	}
}

func (r *inprocResponse) Write(p []byte) (int, error) {
	r.wrote = true
	return r.body.Write(p)
}
