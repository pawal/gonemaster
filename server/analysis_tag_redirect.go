package server

import (
	"net/http"
	"net/url"
	"strings"

	engineprofile "codeberg.org/pawal/gonemaster/engine/profile"
)

// analysisTagsPrefix is the analysis UI route whose next segment is a tag.
const analysisTagsPrefix = "/analysis/tags/"

// legacyTagRedirect keeps published tag links working across a tag rename.
func legacyTagRedirect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		target, ok := renamedAnalysisURL(r.URL)
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	})
}

// renamedAnalysisURL rewrites a retired tag in the path segment and in any
// query value naming it. It reports false when nothing changed.
func renamedAnalysisURL(u *url.URL) (string, bool) {
	changed := false
	out := *u

	if rest, found := strings.CutPrefix(u.Path, analysisTagsPrefix); found {
		tag, tail, hasTail := strings.Cut(rest, "/")
		if current, renamed := engineprofile.RenamedTag(tag); renamed {
			out.Path = analysisTagsPrefix + current
			if hasTail {
				out.Path += "/" + tail
			}
			out.RawPath = ""
			changed = true
		}
	}

	if u.RawQuery != "" {
		query := u.Query()
		for key, values := range query {
			for i, value := range values {
				if current, renamed := engineprofile.RenamedTag(value); renamed {
					query[key][i] = current
					changed = true
				}
			}
		}
		if changed {
			out.RawQuery = query.Encode()
		}
	}

	if !changed {
		return "", false
	}
	return out.String(), true
}
