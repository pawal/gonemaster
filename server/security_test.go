package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// adminRoutePatterns returns the apiMux patterns registered in routes().
func adminRoutePatterns(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "server.go", nil, 0)
	if err != nil {
		t.Fatalf("parse server.go: %v", err)
	}
	var patterns []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "routes" {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (sel.Sel.Name != "Handle" && sel.Sel.Name != "HandleFunc") {
				return true
			}
			if x, ok := sel.X.(*ast.Ident); !ok || x.Name != "apiMux" {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok {
				t.Fatalf("apiMux route with a non-literal pattern")
			}
			p, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatalf("unquote %s: %v", lit.Value, err)
			}
			patterns = append(patterns, p)
			return true
		})
	}
	return patterns
}

func TestAdminMutatingRoutesRefuseCrossOrigin(t *testing.T) {
	patterns := adminRoutePatterns(t)
	if !slices.Contains(patterns, "/jobs/purge") || len(patterns) < 50 {
		t.Fatalf("route scan found %d patterns: %v", len(patterns), patterns)
	}
	// Sub-routes dispatched inside a handler rather than by apiMux.
	paths := []string{"/jobs/x/cancel"}
	wildcard := regexp.MustCompile(`\{[^}]+\}`)
	for _, p := range patterns {
		if _, path, ok := strings.Cut(p, " "); ok {
			p = path
		}
		paths = append(paths, wildcard.ReplaceAllString(p, "x"))
	}

	srv := newTestServer(t, withAuth("tok"), withConfig(func(c *Config) { c.MCPEnabled = true }))
	cookie := &http.Cookie{Name: adminCookieName, Value: "tok"}
	for _, path := range paths {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
			resp := doJSON(t, srv, method, "/api/v1"+path, nil, withCookie(cookie), withOrigin("https://evil.example"))
			if resp.Code != http.StatusForbidden || !strings.Contains(resp.Body.String(), "csrf_origin_mismatch") {
				t.Errorf("%s %s = %d, want 403 csrf_origin_mismatch: %s", method, path, resp.Code, resp.Body.String())
			}
		}
	}
}
