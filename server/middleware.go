package server

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"
)

const debugBodyLimit = 4096

var forwardedHeaders = []string{"X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto"}

// stripUntrustedForwardedHeaders drops forwarded headers from untrusted peers.
func stripUntrustedForwardedHeaders(trusted []netip.Prefix, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !remoteIsTrusted(r.RemoteAddr, trusted) {
			for _, h := range forwardedHeaders {
				r.Header.Del(h)
			}
		}
		next.ServeHTTP(w, r)
	})
}

type responseRecorder struct {
	http.ResponseWriter
	status    int
	bytes     int
	body      *bytes.Buffer
	truncated bool
	maxBody   int
}

func newResponseRecorder(w http.ResponseWriter, maxBody int) *responseRecorder {
	return &responseRecorder{
		ResponseWriter: w,
		maxBody:        maxBody,
		body:           &bytes.Buffer{},
	}
}

// WriteHeader records and forwards the HTTP status code.
func (r *responseRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// Write records response size/body preview and forwards bytes to the client.
func (r *responseRecorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(p)
	r.bytes += n
	if r.body != nil && n > 0 {
		remaining := r.maxBody - r.body.Len()
		if remaining > 0 {
			toCopy := min(n, remaining)
			_, _ = r.body.Write(p[:toCopy])
			if toCopy < n {
				r.truncated = true
			}
		} else {
			r.truncated = true
		}
	}
	return n, err
}

// Flush forwards flush requests when supported by the wrapped writer.
func (r *responseRecorder) Flush() {
	flusher, ok := r.ResponseWriter.(http.Flusher)
	if !ok {
		return
	}
	flusher.Flush()
}

// remoteIsTrustedProxy reports whether the request's RemoteAddr falls inside a
// configured trusted-proxy CIDR. Only trusted peers may supply X-Request-Id.
func (s *Server) remoteIsTrustedProxy(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	return ipInPrefixes(addr, s.trustedProxies)
}

// requestIDMiddleware puts a correlation ID on the request context and echoes it
// in the X-Request-Id response header. An inbound header is honored only from a
// trusted proxy; otherwise a fresh ID is generated so clients cannot spoof it.
func (s *Server) requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := ""
		if inbound := r.Header.Get("X-Request-Id"); inbound != "" && s.remoteIsTrustedProxy(r) {
			id = sanitizeRequestID(inbound)
		}
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set("X-Request-Id", id)
		ctx := context.WithValue(r.Context(), requestIDContextKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// reqLog emits a structured line tagged with the request's correlation ID so
// handler-level events can be joined to the matching access-log line.
func (s *Server) reqLog(r *http.Request, level slog.Level, msg string, args ...any) {
	args = append(args, "request_id", requestIDFromContext(r.Context()))
	s.logger.Log(r.Context(), level, msg, args...)
}

// statusLevel maps an HTTP status to a log level so operators can alert on
// error lines without parsing status codes: 5xx->error, 4xx->warn, else info.
func statusLevel(status int) slog.Level {
	switch {
	case status >= 500:
		return slog.LevelError
	case status >= 400:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}

// accessLogMiddleware emits one structured line per request. The response body
// is captured only when debug logging is on, to avoid logging bodies by default.
// fallbackRoute labels requests the mount's router did not match.
func (s *Server) accessLogMiddleware(fallbackRoute string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		captureBody := s.logger.Enabled(r.Context(), slog.LevelDebug)
		maxBody := 0
		if captureBody {
			maxBody = debugBodyLimit
		}
		holder := &routeHolder{}
		r = r.WithContext(context.WithValue(r.Context(), routeHolderContextKey, holder))
		rec := newResponseRecorder(w, maxBody)
		next.ServeHTTP(rec, r)
		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		route := fallbackRoute
		if p := holder.route.Load(); p != nil {
			route = *p
		}
		attrs := []slog.Attr{
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("route", route),
			slog.Int("status", status),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			slog.Int("bytes", rec.bytes),
			slog.String("remote", clientIP(r, s.trustedProxies)),
			slog.String("request_id", requestIDFromContext(r.Context())),
		}
		if captureBody && rec.body != nil && rec.body.Len() > 0 {
			body := strings.TrimSpace(rec.body.String())
			if rec.truncated {
				body += "..."
			}
			attrs = append(attrs, slog.String("body", body))
		}
		s.logger.LogAttrs(r.Context(), statusLevel(status), "http_request", attrs...)
	})
}

type metricsAwareResponseWriter interface {
	http.ResponseWriter
	SetErrorCode(string)
}

type apiMetricsResponseRecorder struct {
	http.ResponseWriter
	status    int
	errorCode string
}

func newAPIMetricsResponseRecorder(w http.ResponseWriter) *apiMetricsResponseRecorder {
	return &apiMetricsResponseRecorder{ResponseWriter: w}
}

// WriteHeader records and forwards the HTTP status code.
func (r *apiMetricsResponseRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// Write ensures status tracking and forwards bytes to the client.
func (r *apiMetricsResponseRecorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(p)
}

// Flush forwards flush requests when supported by the wrapped writer.
func (r *apiMetricsResponseRecorder) Flush() {
	flusher, ok := r.ResponseWriter.(http.Flusher)
	if !ok {
		return
	}
	flusher.Flush()
}

// SetErrorCode stores an API error code for metrics attribution.
func (r *apiMetricsResponseRecorder) SetErrorCode(code string) {
	r.errorCode = code
}

// Hijack forwards connection hijacking when supported by the wrapped writer.
func (r *apiMetricsResponseRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("hijacker not supported")
	}
	return hijacker.Hijack()
}

// Push forwards HTTP/2 server push when supported by the wrapped writer.
func (r *apiMetricsResponseRecorder) Push(target string, opts *http.PushOptions) error {
	pusher, ok := r.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, opts)
}

// ReadFrom optimizes streaming writes when the wrapped writer supports ReaderFrom.
func (r *apiMetricsResponseRecorder) ReadFrom(reader io.Reader) (int64, error) {
	if rf, ok := r.ResponseWriter.(io.ReaderFrom); ok {
		if r.status == 0 {
			r.status = http.StatusOK
		}
		return rf.ReadFrom(reader)
	}
	return io.Copy(r.ResponseWriter, reader)
}

// recoverMiddleware turns panics into a clean 500 + structured log line.
// Without it, Go's stdlib closes the connection mid-response and dumps an
// unstructured stack trace to stderr.
func (s *Server) recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if rec == http.ErrAbortHandler {
				panic(rec)
			}
			s.metrics.ObservePanic()
			s.logger.LogAttrs(r.Context(), slog.LevelError, "panic",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("request_id", requestIDFromContext(r.Context())),
				slog.Any("value", rec),
				slog.String("stack", string(debug.Stack())),
			)
			writeError(w, http.StatusInternalServerError, "internal_error", "request processing failed", nil)
		}()
		next.ServeHTTP(w, r)
	})
}

// apiMetricsMiddleware records one API request observation, labelled by the
// router's matched pattern so metrics and the access log agree.
func (s *Server) apiMetricsMiddleware(fallbackRoute string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now()

		recorder := newAPIMetricsResponseRecorder(w)
		next.ServeHTTP(recorder, r)

		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		route := fallbackRoute
		if holder, ok := r.Context().Value(routeHolderContextKey).(*routeHolder); ok {
			if p := holder.route.Load(); p != nil {
				route = *p
			}
		}
		s.metrics.ObserveAPIRequest(route, r.Method, status, time.Since(startedAt), recorder.errorCode)
	})
}

// analysisTimeoutMiddleware caps the wall time of /analysis/* requests
// so slow handlers do not pile up under load.
func analysisTimeoutMiddleware(d time.Duration, next http.Handler) http.Handler {
	if d <= 0 {
		return next
	}
	const body = `{"error":"request_timeout","message":"analysis request timed out"}`
	timed := http.TimeoutHandler(next, d, body)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/pub/api/v1/analysis/") || r.URL.Path == "/pub/api/v1/analysis" {
			timed.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// routeHolder carries the router's matched pattern to the access log.
type routeHolder struct {
	route atomic.Pointer[string]
}

// captureRoute records the mux's matched pattern as a mount-qualified route.
func captureRoute(prefix string, mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r)
		holder, ok := r.Context().Value(routeHolderContextKey).(*routeHolder)
		if !ok {
			return
		}
		label := routeLabel(prefix, r.Pattern)
		holder.route.Store(&label)
	})
}

// routeLabel maps "GET /jobs/{id}" to a method-free "/api/v1/jobs/{id}". A
// subtree pattern keeps its trailing slash, or it would share a bucket with the
// exact route of the same name.
func routeLabel(prefix, pattern string) string {
	if pattern == "" {
		return prefix + "/unknown"
	}
	if i := strings.IndexByte(pattern, ' '); i >= 0 {
		pattern = pattern[i+1:]
	}
	return prefix + pattern
}
