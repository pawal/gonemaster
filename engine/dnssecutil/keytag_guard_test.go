package dnssecutil

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// dns.DNSKEY.KeyTag() memoizes the computed tag into the record it is called
// on. DNSKEYs handed to a testcase point into a cached packet that the server
// shares with concurrent runs, so that write is a data race between jobs.
// Engine code goes through dnssecutil.KeyTag instead, which computes on a copy.
//
// The distinction is by argument count: k.KeyTag() is the library method and is
// rejected, dnssecutil.KeyTag(k) and the keyTag(k) alias take the key as an
// argument and are fine.
func TestNoDirectKeyTagMethodCalls(t *testing.T) {
	offenders, err := keyTagMethodCalls("..")
	if err != nil {
		t.Fatalf("scan engine tree: %v", err)
	}
	for _, where := range offenders {
		t.Errorf("%s calls the KeyTag method directly; use dnssecutil.KeyTag so a shared cached record is not written to", where)
	}
}

// keyTagMethodCalls reports every zero-argument .KeyTag() call in non-test Go
// files under root, except the one inside the dnssecutil.KeyTag helper itself.
func keyTagMethodCalls(root string) ([]string, error) {
	var offenders []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		isHelper := filepath.Base(path) == "dnssecutil.go"
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if isHelper && fn.Name.Name == "KeyTag" {
				continue
			}
			for _, call := range keyTagCallsIn(fn) {
				offenders = append(offenders, fset.Position(call).String())
			}
		}
		return nil
	})
	return offenders, err
}

func keyTagCallsIn(node ast.Node) []token.Pos {
	var found []token.Pos
	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 0 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if ok && sel.Sel.Name == "KeyTag" {
			found = append(found, call.Pos())
		}
		return true
	})
	return found
}

// A guard that accepts everything would pass on today's tree and never catch
// the regression it exists for, so the classifier is tested on its own.
func TestKeyTagCallClassifier(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		wantBad bool
	}{
		{"library method on a key", "package p\nfunc f() { _ = k.KeyTag() }", true},
		{"library method on an embedded key", "package p\nfunc f() { _ = ck.DNSKEY.KeyTag() }", true},
		{"shared helper", "package p\nfunc f() { _ = dnssecutil.KeyTag(k) }", false},
		{"package-local alias", "package p\nfunc f() { _ = keyTag(k) }", false},
		{"rrsig keytag field", "package p\nfunc f() { _ = sig.KeyTag }", false},
		{"ds keytag comparison", "package p\nfunc f() { _ = ds.KeyTag == want }", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "x.go", tc.src, 0)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			var got int
			for _, decl := range file.Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok {
					got += len(keyTagCallsIn(fn))
				}
			}
			if bad := got > 0; bad != tc.wantBad {
				t.Errorf("flagged=%v, want %v (%d call sites)", bad, tc.wantBad, got)
			}
		})
	}
}
