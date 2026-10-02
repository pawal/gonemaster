package server

import (
	"net/http"
	"net/url"
	"strings"

	serverpublic "codeberg.org/pawal/gonemaster/server/public"
)

// loginCSSPath is the login page stylesheet, under each gated page prefix.
const loginCSSPath = "/_auth/login.css"

// maxLoginBody caps the login form body.
const maxLoginBody = 4096

// publicAPIAuthMiddleware gates /pub/api/v1 with the admin tokens in protected mode.
func (s *Server) publicAPIAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts := s.authTokens()
		if !ts.protects() {
			next.ServeHTTP(w, r)
			return
		}
		if !requireToken(ts, w, r) {
			return
		}
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if !s.enforceCSRF(w, r) {
				return
			}
		}
		next.ServeHTTP(&privateCacheWriter{ResponseWriter: w}, r)
	})
}

// publicPageGate serves the login page in place of the pages under prefix in protected mode.
func (s *Server) publicPageGate(prefix string, next http.Handler) http.Handler {
	cssHref := prefix + loginCSSPath
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts := s.authTokens()
		if !ts.protects() {
			next.ServeHTTP(w, r)
			return
		}
		switch {
		case r.Method == http.MethodPost:
			s.handlePageLogin(ts, cssHref, w, r)
		case r.URL.Path == cssHref && (r.Method == http.MethodGet || r.Method == http.MethodHead):
			serverpublic.ServeLoginCSS(w, r)
		case ts.allows(r):
			next.ServeHTTP(&privateCacheWriter{ResponseWriter: w}, r)
		case isStaticAsset(r.URL.Path):
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "admin token required", http.StatusUnauthorized)
		default:
			serverpublic.RenderLogin(w, r, cssHref, false)
		}
	})
}

// handlePageLogin checks the token form and sends the visitor back to the same URL.
func (s *Server) handlePageLogin(ts *tokenSet, cssHref string, w http.ResponseWriter, r *http.Request) {
	if !s.enforceCSRF(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxLoginBody)
	tok := ""
	if err := r.ParseForm(); err == nil {
		tok = strings.TrimSpace(r.PostForm.Get("token"))
	}
	if _, ok := ts.match(tok); tok == "" || !ok {
		serverpublic.RenderLogin(w, r, cssHref, true)
		return
	}
	http.SetCookie(w, s.adminCookie(r, tok))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Location", selfLocation(r.URL))
	w.WriteHeader(http.StatusSeeOther)
}

// selfLocation is a relative reference to u that survives a proxy rewriting the path prefix.
func selfLocation(u *url.URL) string {
	p := u.EscapedPath()
	loc := "./" + p[strings.LastIndex(p, "/")+1:]
	if u.RawQuery != "" {
		loc += "?" + u.RawQuery
	}
	return loc
}

// privateCacheWriter rewrites a public Cache-Control directive to private.
type privateCacheWriter struct {
	http.ResponseWriter
	done bool
}

func (p *privateCacheWriter) rewrite() {
	if p.done {
		return
	}
	p.done = true
	if cc := p.Header().Get("Cache-Control"); cc != "" {
		p.Header().Set("Cache-Control", privateCacheControl(cc))
	}
}

func (p *privateCacheWriter) WriteHeader(code int) {
	p.rewrite()
	p.ResponseWriter.WriteHeader(code)
}

func (p *privateCacheWriter) Write(b []byte) (int, error) {
	p.rewrite()
	return p.ResponseWriter.Write(b)
}

func (p *privateCacheWriter) Flush() {
	p.rewrite()
	if f, ok := p.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (p *privateCacheWriter) SetErrorCode(code string) {
	if m, ok := p.ResponseWriter.(metricsAwareResponseWriter); ok {
		m.SetErrorCode(code)
	}
}

func (p *privateCacheWriter) Unwrap() http.ResponseWriter { return p.ResponseWriter }

// privateCacheControl replaces a public directive with private.
func privateCacheControl(cc string) string {
	parts := strings.Split(cc, ",")
	for i, part := range parts {
		if strings.EqualFold(strings.TrimSpace(part), "public") {
			parts[i] = strings.Replace(part, strings.TrimSpace(part), "private", 1)
		}
	}
	return strings.Join(parts, ",")
}
