package server

import (
	"bytes"
	"fmt"
	"log"
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
