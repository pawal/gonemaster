package server

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

const debugBodyLimit = 4096

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
			toCopy := n
			if toCopy > remaining {
				toCopy = remaining
			}
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

func debugMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := newResponseRecorder(w, debugBodyLimit)
		next.ServeHTTP(rec, r)
		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		duration := time.Since(start)
		msg := fmt.Sprintf("%s %s %d %s bytes=%d", r.Method, r.URL.Path, status, duration, rec.bytes)
		if rec.body != nil && rec.body.Len() > 0 {
			body := strings.TrimSpace(rec.body.String())
			if rec.truncated {
				body = body + "..."
			}
			msg = fmt.Sprintf("%s body=%q", msg, body)
		}
		log.Printf("api: %s", msg)
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
			log.Printf("panic: method=%s path=%s value=%v\n%s", r.Method, r.URL.Path, rec, debug.Stack())
			writeError(w, http.StatusInternalServerError, "internal_error", "request processing failed", nil)
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) apiMetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now()
		route := apiRouteTemplate(r.URL.Path)

		recorder := newAPIMetricsResponseRecorder(w)
		next.ServeHTTP(recorder, r)

		status := recorder.status
		if status == 0 {
			status = http.StatusOK
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

func apiRouteTemplate(path string) string {
	const apiPrefix = "/api/v1"

	if path == apiPrefix || path == apiPrefix+"/" {
		return path
	}
	if !strings.HasPrefix(path, apiPrefix+"/") {
		return "/api/v1/unknown"
	}
	if path == "/api/v1/jobs" {
		return "/api/v1/jobs"
	}
	if path == "/api/v1/jobs/batch" {
		return "/api/v1/jobs/batch"
	}
	if strings.HasPrefix(path, "/api/v1/jobs/") {
		tail := strings.TrimPrefix(path, "/api/v1/jobs/")
		parts := strings.Split(strings.Trim(tail, "/"), "/")
		if len(parts) == 1 && parts[0] != "" {
			return "/api/v1/jobs/{job_id}"
		}
		if len(parts) >= 2 && parts[0] != "" {
			switch parts[1] {
			case "result":
				return "/api/v1/jobs/{job_id}/result"
			case "events":
				return "/api/v1/jobs/{job_id}/events"
			case "cancel":
				return "/api/v1/jobs/{job_id}/cancel"
			}
		}
		return "/api/v1/jobs/unknown"
	}
	if strings.HasPrefix(path, "/api/v1/batches/") {
		tail := strings.TrimPrefix(path, "/api/v1/batches/")
		parts := strings.Split(strings.Trim(tail, "/"), "/")
		if len(parts) >= 2 && parts[1] == "delete-preview" {
			return "/api/v1/batches/{batch_id}/delete-preview"
		}
		if strings.TrimSpace(tail) != "" {
			return "/api/v1/batches/{batch_id}"
		}
		return "/api/v1/batches/unknown"
	}
	switch path {
	case "/api/v1/queue/pause",
		"/api/v1/queue/resume",
		"/api/v1/queue/reorder",
		"/api/v1/queue/remove",
		"/api/v1/metrics",
		"/api/v1/healthz":
		return path
	default:
		return "/api/v1/unknown"
	}
}
