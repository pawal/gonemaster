package server

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"
)

const minGzipBytes = 256

var compressibleTypes = []string{
	"application/json",
	"application/javascript",
	"application/xml",
	"application/manifest+json",
	"image/svg+xml",
	"text/",
}

var gzipWriterPool = sync.Pool{
	New: func() any {
		w, _ := gzip.NewWriterLevel(io.Discard, gzip.DefaultCompression)
		return w
	},
}

// gzipMiddleware compresses responses with a compressible Content-Type
// when the client sent Accept-Encoding: gzip. The decision is made on
// first Write, after the handler has set Content-Type.
func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !acceptsGzip(r.Header.Get("Accept-Encoding")) {
			w.Header().Add("Vary", "Accept-Encoding")
			next.ServeHTTP(w, r)
			return
		}
		gw := &gzipResponseWriter{ResponseWriter: w}
		defer gw.Close()
		next.ServeHTTP(gw, r)
	})
}

func acceptsGzip(header string) bool {
	for _, part := range strings.Split(header, ",") {
		token := strings.TrimSpace(part)
		if i := strings.IndexByte(token, ';'); i >= 0 {
			token = token[:i]
		}
		if strings.EqualFold(token, "gzip") {
			return true
		}
	}
	return false
}

func isCompressibleContentType(ct string) bool {
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	ct = strings.TrimSpace(strings.ToLower(ct))
	for _, prefix := range compressibleTypes {
		if strings.HasPrefix(ct, prefix) {
			return true
		}
	}
	return false
}

type gzipResponseWriter struct {
	http.ResponseWriter
	gz         *gzip.Writer
	status     int
	decided    bool
	compress   bool
	pending    []byte
	flushAfter bool
}

func (g *gzipResponseWriter) Header() http.Header { return g.ResponseWriter.Header() }

func (g *gzipResponseWriter) WriteHeader(status int) {
	if g.status != 0 {
		return
	}
	g.status = status
	if status == http.StatusNoContent || status == http.StatusNotModified {
		g.decided = true
		g.compress = false
		g.ResponseWriter.Header().Add("Vary", "Accept-Encoding")
		g.ResponseWriter.WriteHeader(status)
	}
}

func (g *gzipResponseWriter) Write(p []byte) (int, error) {
	if !g.decided {
		g.pending = append(g.pending, p...)
		if len(g.pending) < minGzipBytes && !g.flushAfter {
			return len(p), nil
		}
		// decide() flushes the pending buffer (which already contains p).
		g.decide()
		return len(p), nil
	}
	if g.compress {
		return g.gz.Write(p)
	}
	return g.ResponseWriter.Write(p)
}

func (g *gzipResponseWriter) decide() {
	g.decided = true
	h := g.ResponseWriter.Header()
	ce := h.Get("Content-Encoding")
	ct := h.Get("Content-Type")
	if ce == "" && len(g.pending) >= minGzipBytes && isCompressibleContentType(ct) {
		g.compress = true
		h.Del("Content-Length")
		h.Set("Content-Encoding", "gzip")
		h.Add("Vary", "Accept-Encoding")
		gz := gzipWriterPool.Get().(*gzip.Writer)
		gz.Reset(g.ResponseWriter)
		g.gz = gz
	} else {
		h.Add("Vary", "Accept-Encoding")
	}
	if g.status == 0 {
		g.status = http.StatusOK
	}
	g.ResponseWriter.WriteHeader(g.status)
	if pending := g.pending; len(pending) > 0 {
		g.pending = nil
		if g.compress {
			_, _ = g.gz.Write(pending)
		} else {
			_, _ = g.ResponseWriter.Write(pending)
		}
	}
}

func (g *gzipResponseWriter) Close() {
	if !g.decided {
		g.flushAfter = true
		g.decide()
	}
	if g.gz != nil {
		_ = g.gz.Close()
		gzipWriterPool.Put(g.gz)
		g.gz = nil
	}
}

func (g *gzipResponseWriter) Flush() {
	if !g.decided {
		g.flushAfter = true
		g.decide()
	}
	if g.gz != nil {
		_ = g.gz.Flush()
	}
	if f, ok := g.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
