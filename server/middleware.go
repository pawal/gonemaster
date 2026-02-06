package server

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
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

func (r *responseRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

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

func (r *apiMetricsResponseRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *apiMetricsResponseRecorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(p)
}

func (r *apiMetricsResponseRecorder) Flush() {
	flusher, ok := r.ResponseWriter.(http.Flusher)
	if !ok {
		return
	}
	flusher.Flush()
}

func (r *apiMetricsResponseRecorder) SetErrorCode(code string) {
	r.errorCode = code
}

func (r *apiMetricsResponseRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("hijacker not supported")
	}
	return hijacker.Hijack()
}

func (r *apiMetricsResponseRecorder) Push(target string, opts *http.PushOptions) error {
	pusher, ok := r.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, opts)
}

func (r *apiMetricsResponseRecorder) ReadFrom(reader io.Reader) (int64, error) {
	if rf, ok := r.ResponseWriter.(io.ReaderFrom); ok {
		if r.status == 0 {
			r.status = http.StatusOK
		}
		return rf.ReadFrom(reader)
	}
	return io.Copy(r.ResponseWriter, reader)
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
