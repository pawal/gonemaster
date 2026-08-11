package nameserver

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The engine builds isolated state per run, and engine.Run is documented as
// safe to call concurrently. A package-level var holding a cache breaks that:
// every run shares it, no caller can see it, and no test can isolate it. This
// guard fails when one is reintroduced in the query path.
//
// Read-only lookup tables, sentinels and tuning knobs are fine; what is
// rejected is state built by a constructor call or a composite literal.

func TestNoCacheShapedPackageVars(t *testing.T) {
	for _, dir := range []string{".", "../recursor"} {
		offenders, err := cacheShapedVars(dir)
		if err != nil {
			t.Fatalf("scan %s: %v", dir, err)
		}
		for _, name := range offenders {
			t.Errorf("%s declares package-level mutable state %q; give it an owner on CacheStore and inject it instead", dir, name)
		}
	}
}

// The classifier itself needs testing: a checker that accepts everything would
// pass silently on today's tree and never catch the regression it exists for.
func TestCacheShapedVarClassifier(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		wantBad bool
	}{
		{"constructor call", "package p\nvar c = newFooCache()", true},
		{"composite literal pointer", "package p\nvar c = &fooCache{}", true},
		{"composite literal value", "package p\nvar c = fooCache{}", true},
		{"map literal", "package p\nvar c = map[string]int{}", true},
		{"int literal", "package p\nvar maxEntries = 256", false},
		{"string literal", "package p\nvar version = \"1.0\"", false},
		{"errors.New sentinel", "package p\nvar errGone = errors.New(\"gone\")", false},
		{"regexp.MustCompile", "package p\nvar re = regexp.MustCompile(\"x\")", false},
		{"sync.Pool", "package p\nvar pool = sync.Pool{}", false},
		{"package selector", "package p\nvar root = share.NamedRoot", false},
		{"declaration without value", "package p\nvar buf []byte", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "x.go", tc.src, 0)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			got := len(cacheShapedVarsInFile(file)) > 0
			if got != tc.wantBad {
				t.Errorf("classifier returned bad=%v, want %v for: %s", got, tc.wantBad, tc.src)
			}
		})
	}
}

func cacheShapedVars(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var offenders []string
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			return nil, err
		}
		offenders = append(offenders, cacheShapedVarsInFile(file)...)
	}
	return offenders, nil
}

// cacheShapedVarsInFile reports file-scope vars whose initializer builds state.
func cacheShapedVarsInFile(file *ast.File) []string {
	var offenders []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range value.Names {
				if i >= len(value.Values) {
					continue
				}
				if !allowedPackageVarValue(value.Values[i]) {
					offenders = append(offenders, name.Name)
				}
			}
		}
	}
	return offenders
}

func allowedPackageVarValue(expr ast.Expr) bool {
	switch v := expr.(type) {
	case *ast.BasicLit:
		return true
	case *ast.SelectorExpr:
		// A reference to another package's value, e.g. embedded root data.
		return true
	case *ast.UnaryExpr:
		return v.Op != token.AND && allowedPackageVarValue(v.X)
	case *ast.CompositeLit:
		return isSyncPoolType(v.Type)
	case *ast.CallExpr:
		sel, ok := v.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok {
			return false
		}
		switch pkg.Name + "." + sel.Sel.Name {
		case "errors.New", "fmt.Errorf", "regexp.MustCompile":
			return true
		}
		return false
	}
	return false
}

func isSyncPoolType(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "sync" && sel.Sel.Name == "Pool"
}
