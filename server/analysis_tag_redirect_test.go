package server

import (
	"net/http"
	"net/url"
	"testing"
)

func TestRenamedAnalysisURL(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{
			// The exact shape published to the RIPE dns-wg list before the
			// rename: the tag appears both in the path and in the tag list
			// filter carried for the breadcrumb back link.
			name: "path and search filter",
			in:   "/analysis/tags/OUT_OF_BAILIWICK_ADDR_MISMATCH?search=OUT_OF_BAILIWICK_ADDR_MISMATCH",
			want: "/analysis/tags/NOT_IN_DOMAIN_ADDR_MISMATCH?search=NOT_IN_DOMAIN_ADDR_MISMATCH",
			ok:   true,
		},
		{
			name: "path only",
			in:   "/analysis/tags/IN_BAILIWICK_ADDR_MISMATCH",
			want: "/analysis/tags/IN_DOMAIN_ADDR_MISMATCH",
			ok:   true,
		},
		{
			name: "delegation tag",
			in:   "/analysis/tags/IN_BAILIWICK_GLUE_MISSING",
			want: "/analysis/tags/IN_DOMAIN_GLUE_MISSING",
			ok:   true,
		},
		{
			// A filter on the tag list page, with no tag in the path.
			name: "query only",
			in:   "/analysis/tags?search=IN_BAILIWICK_GLUE_MISSING",
			want: "/analysis/tags?search=IN_DOMAIN_GLUE_MISSING",
			ok:   true,
		},
		{
			name: "other query parameters are preserved",
			in:   "/analysis/tags/OUT_OF_BAILIWICK_ADDR_MISMATCH?dataset_tag=TLDs&snapshot=2026-08-15-abc",
			want: "/analysis/tags/NOT_IN_DOMAIN_ADDR_MISMATCH?dataset_tag=TLDs&snapshot=2026-08-15-abc",
			ok:   true,
		},
		{
			name: "current tag is left alone",
			in:   "/analysis/tags/NOT_IN_DOMAIN_ADDR_MISMATCH?search=NOT_IN_DOMAIN_ADDR_MISMATCH",
			ok:   false,
		},
		{
			name: "unrelated tag is left alone",
			in:   "/analysis/tags/DS07_NOT_SIGNED",
			ok:   false,
		},
		{
			name: "unrelated analysis route is left alone",
			in:   "/analysis/domains/example.com",
			ok:   false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u, err := url.Parse(tc.in)
			if err != nil {
				t.Fatalf("parse %q: %v", tc.in, err)
			}
			got, ok := renamedAnalysisURL(u)
			if ok != tc.ok {
				t.Fatalf("renamedAnalysisURL(%q) ok = %v, want %v", tc.in, ok, tc.ok)
			}
			if !tc.ok {
				return
			}
			gotURL, err := url.Parse(got)
			if err != nil {
				t.Fatalf("parse result %q: %v", got, err)
			}
			wantURL, err := url.Parse(tc.want)
			if err != nil {
				t.Fatalf("parse want %q: %v", tc.want, err)
			}
			if gotURL.Path != wantURL.Path {
				t.Errorf("path = %q, want %q", gotURL.Path, wantURL.Path)
			}
			// Compare decoded parameters so encoding order cannot fail the test.
			if gotQ, wantQ := gotURL.Query(), wantURL.Query(); len(gotQ) != len(wantQ) {
				t.Errorf("query = %v, want %v", gotQ, wantQ)
			} else {
				for key, wantVals := range wantQ {
					gotVals := gotQ[key]
					if len(gotVals) != len(wantVals) {
						t.Errorf("query %q = %v, want %v", key, gotVals, wantVals)
						continue
					}
					for i := range wantVals {
						if gotVals[i] != wantVals[i] {
							t.Errorf("query %q[%d] = %q, want %q", key, i, gotVals[i], wantVals[i])
						}
					}
				}
			}
		})
	}
}

func TestLegacyTagRedirectServesPermanentRedirect(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("spa"))
	})
	h := legacyTagRedirect(next)

	rec := doHandler(t, h, http.MethodGet, "/analysis/tags/OUT_OF_BAILIWICK_ADDR_MISMATCH?search=OUT_OF_BAILIWICK_ADDR_MISMATCH", nil)

	wantStatus(t, rec, http.StatusMovedPermanently)
	location := rec.Header().Get("Location")
	got, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse Location %q: %v", location, err)
	}
	if got.Path != "/analysis/tags/NOT_IN_DOMAIN_ADDR_MISMATCH" {
		t.Errorf("Location path = %q, want /analysis/tags/NOT_IN_DOMAIN_ADDR_MISMATCH", got.Path)
	}
	if search := got.Query().Get("search"); search != "NOT_IN_DOMAIN_ADDR_MISMATCH" {
		t.Errorf("Location search = %q, want NOT_IN_DOMAIN_ADDR_MISMATCH", search)
	}
}

func TestAnalysisRouteRedirectsLegacyTag(t *testing.T) {
	// Through the real router, so the redirect cannot be lost by a rewiring of
	// the /analysis/ mount. Independent of whether the SPA bundle is built,
	// since the redirect answers before the request reaches the UI handler.
	srv := newTestServer(t)

	rec := doHandler(t, srv.mux, http.MethodGet, "/analysis/tags/OUT_OF_BAILIWICK_ADDR_MISMATCH?search=OUT_OF_BAILIWICK_ADDR_MISMATCH", nil)

	wantStatus(t, rec, http.StatusMovedPermanently)
	got, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	if got.Path != "/analysis/tags/NOT_IN_DOMAIN_ADDR_MISMATCH" {
		t.Errorf("Location path = %q, want /analysis/tags/NOT_IN_DOMAIN_ADDR_MISMATCH", got.Path)
	}
	if search := got.Query().Get("search"); search != "NOT_IN_DOMAIN_ADDR_MISMATCH" {
		t.Errorf("Location search = %q, want NOT_IN_DOMAIN_ADDR_MISMATCH", search)
	}
}

func TestLegacyTagRedirectPassesCurrentURLThrough(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	h := legacyTagRedirect(next)

	rec := doHandler(t, h, http.MethodGet, "/analysis/tags/NOT_IN_DOMAIN_ADDR_MISMATCH", nil)

	if !called {
		t.Fatal("expected the request to reach the analysis UI handler")
	}
	wantStatus(t, rec, http.StatusOK)
}
