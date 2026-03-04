package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const schemaID = "gonemaster.logargs-inventory/1.0"

type payload struct {
	GeneratedBy        string              `json:"generated_by"`
	Schema             string              `json:"schema"`
	Source             []string            `json:"source"`
	Summary            summary             `json:"summary"`
	Keys               map[string]keyEntry `json:"keys"`
	NSStringByFile     map[string]int      `json:"ns_string_by_file,omitempty"`
	PackedListOnlyKeys []string            `json:"packed_list_only_keys,omitempty"`
	Unresolved         []unresolvedCall    `json:"unresolved_calls,omitempty"`
}

type summary struct {
	FileCount              int `json:"file_count"`
	LogCallCount           int `json:"log_call_count"`
	UniqueTagCount         int `json:"unique_tag_count"`
	UniqueKeyCount         int `json:"unique_key_count"`
	NSStringCallCount      int `json:"ns_string_call_count"`
	PackedListOnlyKeyCount int `json:"packed_list_only_key_count"`
	UnresolvedLogCallCount int `json:"unresolved_log_call_count"`
}

type keyEntry struct {
	Tags          []string      `json:"tags"`
	Shapes        []string      `json:"shapes"`
	ProducerFiles []string      `json:"producer_files"`
	Occurrences   int           `json:"occurrences"`
	Samples       []sampleEntry `json:"samples,omitempty"`
}

type sampleEntry struct {
	Tag   string `json:"tag"`
	File  string `json:"file"`
	Line  int    `json:"line"`
	Shape string `json:"shape"`
}

type unresolvedCall struct {
	Tag      string `json:"tag"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	ArgsExpr string `json:"args_expr"`
}

type env struct {
	stringVars map[string]string
	mapVars    map[string]*mapVarInfo
}

type mapVarInfo struct {
	keys map[string]map[string]struct{}
}

func newMapVarInfo() *mapVarInfo {
	return &mapVarInfo{
		keys: map[string]map[string]struct{}{},
	}
}

func (m *mapVarInfo) add(key string, shape string) {
	if key == "" {
		return
	}
	shape = normalizeShape(shape)
	if _, ok := m.keys[key]; !ok {
		m.keys[key] = map[string]struct{}{}
	}
	m.keys[key][shape] = struct{}{}
}

func (m *mapVarInfo) addAll(other *mapVarInfo) {
	if other == nil {
		return
	}
	for key, shapes := range other.keys {
		for shape := range shapes {
			m.add(key, shape)
		}
	}
}

func (m *mapVarInfo) clone() *mapVarInfo {
	out := newMapVarInfo()
	out.addAll(m)
	return out
}

type keyAccumulator struct {
	tags        map[string]struct{}
	shapes      map[string]struct{}
	files       map[string]struct{}
	occurrences int
	samples     []sampleEntry
}

type inventory struct {
	keys           map[string]*keyAccumulator
	allTags        map[string]struct{}
	unresolved     []unresolvedCall
	logCalls       int
	nsStringByFile map[string]int
	nsStringCalls  int
}

func newInventory() *inventory {
	return &inventory{
		keys:           map[string]*keyAccumulator{},
		allTags:        map[string]struct{}{},
		nsStringByFile: map[string]int{},
	}
}

func (i *inventory) addObservedTag(tag string) {
	if tag == "" {
		return
	}
	i.allTags[tag] = struct{}{}
}

func (i *inventory) addKeyObservation(key string, tag string, shape string, file string, line int) {
	if key == "" {
		return
	}
	shape = normalizeShape(shape)

	acc, ok := i.keys[key]
	if !ok {
		acc = &keyAccumulator{
			tags:   map[string]struct{}{},
			shapes: map[string]struct{}{},
			files:  map[string]struct{}{},
		}
		i.keys[key] = acc
	}

	acc.tags[tag] = struct{}{}
	acc.shapes[shape] = struct{}{}
	acc.files[file] = struct{}{}
	acc.occurrences++

	if len(acc.samples) < 8 {
		acc.samples = append(acc.samples, sampleEntry{
			Tag:   tag,
			File:  file,
			Line:  line,
			Shape: shape,
		})
	}
}

func (i *inventory) addUnresolved(tag string, file string, line int, argsExpr string) {
	i.unresolved = append(i.unresolved, unresolvedCall{
		Tag:      tag,
		File:     file,
		Line:     line,
		ArgsExpr: argsExpr,
	})
}

func (i *inventory) asPayload(fileCount int) payload {
	keys := make(map[string]keyEntry, len(i.keys))
	for key, acc := range i.keys {
		keys[key] = keyEntry{
			Tags:          sortedSet(acc.tags),
			Shapes:        sortedSet(acc.shapes),
			ProducerFiles: sortedSet(acc.files),
			Occurrences:   acc.occurrences,
			Samples:       sortSamples(acc.samples),
		}
	}

	unresolved := make([]unresolvedCall, len(i.unresolved))
	copy(unresolved, i.unresolved)
	sort.Slice(unresolved, func(a, b int) bool {
		if unresolved[a].Tag != unresolved[b].Tag {
			return unresolved[a].Tag < unresolved[b].Tag
		}
		if unresolved[a].File != unresolved[b].File {
			return unresolved[a].File < unresolved[b].File
		}
		return unresolved[a].Line < unresolved[b].Line
	})

	return payload{
		GeneratedBy: "go run ./tools/specifications/export-log-args",
		Schema:      schemaID,
		Source: []string{
			"engine/**/*.go (non-test files)",
			"logger.Add(tag, args, ...)",
			"append*Log(..., tag, args)",
			"logSystem(tag, args), logSystemWithLogger(..., tag, args)",
		},
		Summary: summary{
			FileCount:              fileCount,
			LogCallCount:           i.logCalls,
			UniqueTagCount:         len(i.allTags),
			UniqueKeyCount:         len(i.keys),
			NSStringCallCount:      i.nsStringCalls,
			UnresolvedLogCallCount: len(unresolved),
		},
		Keys:               keys,
		NSStringByFile:     copyIntMap(i.nsStringByFile),
		PackedListOnlyKeys: packedListOnlyKeys(keys),
		Unresolved:         unresolved,
	}
}

func main() {
	var (
		rootDir         string
		markdownOut     string
		checkCoherency  bool
		nsAllowlist     string
		packedAllowlist string
	)
	flag.StringVar(&rootDir, "root", "engine", "Root directory to scan for Go files")
	flag.StringVar(&markdownOut, "markdown-out", "docs/specifications/log-args-inventory.md", "Path to write markdown report")
	flag.BoolVar(&checkCoherency, "check-coherency", false, "Validate coherency guardrails against allowlists")
	flag.StringVar(&nsAllowlist, "ns-string-allowlist", "docs/specifications/coherency/ns-string-allowlist.txt", "Allowlist file mapping path to max allowed ns=.String occurrences")
	flag.StringVar(&packedAllowlist, "packed-list-allowlist", "docs/specifications/coherency/packed-list-key-allowlist.txt", "Allowlist of existing packed-list-only arg keys")
	flag.Parse()

	files, err := collectGoFiles(rootDir)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "collect go files: %v\n", err)
		os.Exit(1)
	}

	inv := newInventory()
	for _, file := range files {
		if err := scanFile(file, inv); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "scan %s: %v\n", file, err)
			os.Exit(1)
		}
	}

	data := inv.asPayload(len(files))
	data.Summary.PackedListOnlyKeyCount = len(data.PackedListOnlyKeys)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "encode json: %v\n", err)
		os.Exit(1)
	}

	if markdownOut != "" {
		md := renderMarkdown(data)
		if err := os.WriteFile(markdownOut, []byte(md), 0o644); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "write markdown: %v\n", err)
			os.Exit(1)
		}
	}

	if checkCoherency {
		if err := checkCoherencyGuardrails(data, nsAllowlist, packedAllowlist); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "coherency check failed: %v\n", err)
			os.Exit(1)
		}
	}
}

func collectGoFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			if base == ".git" || base == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		files = append(files, filepath.ToSlash(path))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func scanFile(path string, inv *inventory) error {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return err
	}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		base := env{
			stringVars: map[string]string{},
			mapVars:    map[string]*mapVarInfo{},
		}
		analyzeBlock(fset, path, fn.Body, base, inv)
	}
	scanNSStringMisuse(fset, path, file, inv)
	return nil
}

func scanNSStringMisuse(fset *token.FileSet, path string, file *ast.File, inv *inventory) {
	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.CompositeLit:
			if !isStringKeyMapLiteral(n) {
				return true
			}
			for _, elt := range n.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				keyLit, ok := kv.Key.(*ast.BasicLit)
				if !ok || keyLit.Kind != token.STRING {
					continue
				}
				key, err := strconv.Unquote(keyLit.Value)
				if err != nil || key != "ns" {
					continue
				}
				if isStringMethodCall(kv.Value) {
					fp := filepath.ToSlash(path)
					inv.nsStringByFile[fp]++
					inv.nsStringCalls++
				}
			}
		case *ast.AssignStmt:
			if len(n.Lhs) != len(n.Rhs) {
				return true
			}
			for i := 0; i < len(n.Lhs); i++ {
				indexExpr, ok := n.Lhs[i].(*ast.IndexExpr)
				if !ok {
					continue
				}
				keyLit, ok := indexExpr.Index.(*ast.BasicLit)
				if !ok || keyLit.Kind != token.STRING {
					continue
				}
				key, err := strconv.Unquote(keyLit.Value)
				if err != nil || key != "ns" {
					continue
				}
				if isStringMethodCall(n.Rhs[i]) {
					fp := filepath.ToSlash(path)
					inv.nsStringByFile[fp]++
					inv.nsStringCalls++
				}
			}
		}
		return true
	})
	_ = fset
}

func isStringMethodCall(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return sel.Sel.Name == "String"
}

func analyzeBlock(fset *token.FileSet, filePath string, block *ast.BlockStmt, parent env, inv *inventory) {
	local := cloneEnv(parent)
	collectBindings(block, &local)
	scanLoggingCalls(fset, filePath, block, local, inv)

	for _, lit := range collectFuncLiterals(block) {
		analyzeBlock(fset, filePath, lit.Body, local, inv)
	}
}

func collectBindings(block *ast.BlockStmt, local *env) {
	inspectWithoutFuncLits(block, func(n ast.Node) {
		switch node := n.(type) {
		case *ast.ValueSpec:
			handleValueSpec(node, local)
		case *ast.AssignStmt:
			handleAssign(node, local)
		}
	})
}

func scanLoggingCalls(fset *token.FileSet, filePath string, block *ast.BlockStmt, local env, inv *inventory) {
	inspectWithoutFuncLits(block, func(n ast.Node) {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return
		}

		tagExpr, argsExpr, ok := extractLogCallParts(call)
		if !ok {
			return
		}
		if shouldSkipWrapperCall(tagExpr, argsExpr) {
			return
		}

		tag := resolveTag(tagExpr, local, fset)
		if tag == "" {
			tag = "<unknown-tag>"
		}

		inv.logCalls++
		inv.addObservedTag(tag)

		keys, resolved := resolveArgsKeys(argsExpr, local, fset)
		pos := fset.Position(call.Pos())
		if !resolved {
			inv.addUnresolved(tag, filepath.ToSlash(filePath), pos.Line, renderExpr(argsExpr, fset))
			return
		}

		for key, shapes := range keys {
			for shape := range shapes {
				inv.addKeyObservation(key, tag, shape, filepath.ToSlash(filePath), pos.Line)
			}
		}
	})
}

func handleValueSpec(spec *ast.ValueSpec, local *env) {
	if spec == nil {
		return
	}
	if spec.Names == nil {
		return
	}

	if len(spec.Values) == 0 && isStringAnyMapType(spec.Type) {
		for _, name := range spec.Names {
			local.mapVars[name.Name] = newMapVarInfo()
		}
		return
	}

	for i, name := range spec.Names {
		var value ast.Expr
		if i < len(spec.Values) {
			value = spec.Values[i]
		}

		if value == nil {
			continue
		}

		if lit, ok := asStringAnyMapLiteral(value); ok {
			local.mapVars[name.Name] = lit
			continue
		}
		if isMakeStringAnyMap(value) {
			local.mapVars[name.Name] = newMapVarInfo()
			continue
		}

		if s, ok := evalStringExpr(value, *local); ok {
			local.stringVars[name.Name] = s
			continue
		}

		delete(local.stringVars, name.Name)
	}
}

func handleAssign(assign *ast.AssignStmt, local *env) {
	if assign == nil || len(assign.Lhs) == 0 {
		return
	}

	for i := 0; i < len(assign.Lhs); i++ {
		lhs := assign.Lhs[i]
		var rhs ast.Expr
		if i < len(assign.Rhs) {
			rhs = assign.Rhs[i]
		}

		switch l := lhs.(type) {
		case *ast.Ident:
			if rhs == nil {
				continue
			}
			if lit, ok := asStringAnyMapLiteral(rhs); ok {
				local.mapVars[l.Name] = lit
				continue
			}
			if isMakeStringAnyMap(rhs) {
				local.mapVars[l.Name] = newMapVarInfo()
				continue
			}
			if s, ok := evalStringExpr(rhs, *local); ok {
				local.stringVars[l.Name] = s
			} else {
				delete(local.stringVars, l.Name)
			}
		case *ast.IndexExpr:
			mapName, ok := l.X.(*ast.Ident)
			if !ok {
				continue
			}
			key, ok := stringExprValue(l.Index, *local)
			if !ok {
				continue
			}
			info, exists := local.mapVars[mapName.Name]
			if !exists {
				continue
			}
			shape := inferShape(rhs, *local)
			info.add(key, shape)
		}
	}
}

func extractLogCallParts(call *ast.CallExpr) (ast.Expr, ast.Expr, bool) {
	if call == nil {
		return nil, nil, false
	}

	switch fun := call.Fun.(type) {
	case *ast.Ident:
		name := fun.Name
		if name == "logSystem" && len(call.Args) >= 3 {
			return call.Args[1], call.Args[2], true
		}
		if name == "logSystem" && len(call.Args) == 2 {
			return call.Args[0], call.Args[1], true
		}
		if name == "logSystemWithLogger" && len(call.Args) >= 3 {
			return call.Args[1], call.Args[2], true
		}
		if strings.HasPrefix(name, "append") && strings.HasSuffix(name, "Log") && len(call.Args) >= 2 {
			return call.Args[len(call.Args)-2], call.Args[len(call.Args)-1], true
		}
	case *ast.SelectorExpr:
		if fun.Sel.Name == "logSystem" && len(call.Args) >= 2 {
			return call.Args[0], call.Args[1], true
		}
		if fun.Sel.Name == "logSystemWithLogger" && len(call.Args) >= 3 {
			return call.Args[1], call.Args[2], true
		}
		if fun.Sel.Name == "Add" && len(call.Args) >= 2 {
			if !isLikelyStringTagExpr(call.Args[0]) {
				return nil, nil, false
			}
			return call.Args[0], call.Args[1], true
		}
	}

	return nil, nil, false
}

func shouldSkipWrapperCall(tagExpr ast.Expr, argsExpr ast.Expr) bool {
	tagIdent, ok := tagExpr.(*ast.Ident)
	if !ok {
		return false
	}
	argsIdent, ok := argsExpr.(*ast.Ident)
	if !ok {
		return false
	}
	return tagIdent.Name == "tag" && argsIdent.Name == "args"
}

func resolveTag(expr ast.Expr, local env, fset *token.FileSet) string {
	if expr == nil {
		return ""
	}
	if value, ok := evalStringExpr(expr, local); ok {
		return value
	}
	return renderExpr(expr, fset)
}

func resolveArgsKeys(expr ast.Expr, local env, fset *token.FileSet) (map[string]map[string]struct{}, bool) {
	if expr == nil {
		return map[string]map[string]struct{}{}, true
	}

	switch node := expr.(type) {
	case *ast.Ident:
		if node.Name == "nil" {
			return map[string]map[string]struct{}{}, true
		}
		info, ok := local.mapVars[node.Name]
		if !ok {
			return nil, false
		}
		return cloneShapes(info.keys), true
	case *ast.CompositeLit:
		if !isStringKeyMapLiteral(node) {
			return nil, false
		}
		return mapLiteralShapes(node, local), true
	case *ast.CallExpr:
		if isMakeStringAnyMap(node) {
			return map[string]map[string]struct{}{}, true
		}
		return nil, false
	default:
		_ = fset
		return nil, false
	}
}

func mapLiteralShapes(lit *ast.CompositeLit, local env) map[string]map[string]struct{} {
	out := map[string]map[string]struct{}{}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := stringExprValue(kv.Key, local)
		if !ok || key == "" {
			continue
		}
		shape := normalizeShape(inferShape(kv.Value, local))
		if _, exists := out[key]; !exists {
			out[key] = map[string]struct{}{}
		}
		out[key][shape] = struct{}{}
	}
	return out
}

func asStringAnyMapLiteral(expr ast.Expr) (*mapVarInfo, bool) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok || !isStringKeyMapLiteral(lit) {
		return nil, false
	}
	info := newMapVarInfo()
	local := env{
		stringVars: map[string]string{},
		mapVars:    map[string]*mapVarInfo{},
	}
	shapes := mapLiteralShapes(lit, local)
	for key, s := range shapes {
		for shape := range s {
			info.add(key, shape)
		}
	}
	return info, true
}

func isLikelyStringTagExpr(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return e.Kind == token.STRING
	case *ast.Ident:
		return true
	case *ast.BinaryExpr:
		return e.Op == token.ADD
	case *ast.SelectorExpr:
		return true
	case *ast.CallExpr:
		return true
	default:
		return false
	}
}

func inferShape(expr ast.Expr, local env) string {
	if expr == nil {
		return "unknown"
	}

	switch e := expr.(type) {
	case *ast.BasicLit:
		switch e.Kind {
		case token.STRING, token.CHAR:
			return "string"
		case token.INT:
			return "int"
		case token.FLOAT:
			return "float"
		default:
			return "unknown"
		}
	case *ast.Ident:
		switch e.Name {
		case "true", "false":
			return "bool"
		case "nil":
			return "null"
		}
		if _, ok := local.stringVars[e.Name]; ok {
			return "string"
		}
		if _, ok := local.mapVars[e.Name]; ok {
			return "object"
		}
		return "unknown"
	case *ast.CompositeLit:
		switch e.Type.(type) {
		case *ast.ArrayType:
			return "array"
		case *ast.MapType:
			return "object"
		default:
			return "object"
		}
	case *ast.BinaryExpr:
		if e.Op == token.ADD {
			if _, ok := evalStringExpr(e, local); ok {
				return "string"
			}
			return "unknown"
		}
		return "bool"
	case *ast.UnaryExpr:
		return inferShape(e.X, local)
	case *ast.CallExpr:
		name := calledName(e.Fun)
		if isStringCall(name) {
			return "string"
		}
		if isIntCall(name) {
			return "int"
		}
		if isBoolCall(name) {
			return "bool"
		}
		if name == "append" {
			return "array"
		}
		return "unknown"
	default:
		return "unknown"
	}
}

func isStringCall(name string) bool {
	switch name {
	case "fmt.Sprintf", "fmt.Sprint", "fmt.Sprintln", "strings.Join", "strings.ToLower", "strings.ToUpper", "string":
		return true
	}
	if strings.HasSuffix(name, ".String") || strings.HasSuffix(name, ".Error") {
		return true
	}
	if strings.HasPrefix(name, "strconv.Format") || name == "strconv.Itoa" || name == "strconv.Quote" {
		return true
	}
	return false
}

func isIntCall(name string) bool {
	switch name {
	case "len", "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
		return true
	default:
		return false
	}
}

func isBoolCall(name string) bool {
	return name == "bool"
}

func calledName(fun ast.Expr) string {
	switch f := fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		left := calledName(f.X)
		if left == "" {
			return f.Sel.Name
		}
		return left + "." + f.Sel.Name
	default:
		return ""
	}
}

func evalStringExpr(expr ast.Expr, local env) (string, bool) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(e.Value)
		if err != nil {
			return "", false
		}
		return s, true
	case *ast.Ident:
		value, ok := local.stringVars[e.Name]
		return value, ok
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return "", false
		}
		left, ok := evalStringExpr(e.X, local)
		if !ok {
			return "", false
		}
		right, ok := evalStringExpr(e.Y, local)
		if !ok {
			return "", false
		}
		return left + right, true
	default:
		return "", false
	}
}

func stringExprValue(expr ast.Expr, local env) (string, bool) {
	if s, ok := evalStringExpr(expr, local); ok {
		return s, true
	}
	return "", false
}

func isStringAnyMapType(expr ast.Expr) bool {
	mt, ok := expr.(*ast.MapType)
	if !ok {
		return false
	}
	key, ok := mt.Key.(*ast.Ident)
	if !ok || key.Name != "string" {
		return false
	}
	switch v := mt.Value.(type) {
	case *ast.Ident:
		return v.Name == "any" || v.Name == "interface{}"
	case *ast.InterfaceType:
		return v.Methods == nil || len(v.Methods.List) == 0
	default:
		return false
	}
}

func isStringKeyMapLiteral(lit *ast.CompositeLit) bool {
	if lit == nil {
		return false
	}
	mt, ok := lit.Type.(*ast.MapType)
	if !ok {
		return false
	}
	key, ok := mt.Key.(*ast.Ident)
	if !ok {
		return false
	}
	return key.Name == "string"
}

func isMakeStringAnyMap(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	fun, ok := call.Fun.(*ast.Ident)
	if !ok || fun.Name != "make" || len(call.Args) == 0 {
		return false
	}
	return isStringAnyMapType(call.Args[0])
}

func renderExpr(expr ast.Expr, fset *token.FileSet) string {
	if expr == nil {
		return ""
	}
	var b bytes.Buffer
	if err := printer.Fprint(&b, fset, expr); err != nil {
		return "<expr>"
	}
	return strings.TrimSpace(b.String())
}

func inspectWithoutFuncLits(root ast.Node, fn func(ast.Node)) {
	ast.Inspect(root, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		fn(n)
		return true
	})
}

func collectFuncLiterals(root ast.Node) []*ast.FuncLit {
	var out []*ast.FuncLit
	ast.Inspect(root, func(n ast.Node) bool {
		lit, ok := n.(*ast.FuncLit)
		if !ok {
			return true
		}
		out = append(out, lit)
		return false
	})
	return out
}

func cloneEnv(in env) env {
	out := env{
		stringVars: map[string]string{},
		mapVars:    map[string]*mapVarInfo{},
	}
	for key, value := range in.stringVars {
		out.stringVars[key] = value
	}
	for key, value := range in.mapVars {
		out.mapVars[key] = value.clone()
	}
	return out
}

func cloneShapes(in map[string]map[string]struct{}) map[string]map[string]struct{} {
	out := map[string]map[string]struct{}{}
	for key, values := range in {
		out[key] = map[string]struct{}{}
		for value := range values {
			out[key][value] = struct{}{}
		}
	}
	return out
}

func sortedSet(in map[string]struct{}) []string {
	out := make([]string, 0, len(in))
	for item := range in {
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func sortSamples(in []sampleEntry) []sampleEntry {
	out := make([]sampleEntry, len(in))
	copy(out, in)
	sort.Slice(out, func(a, b int) bool {
		if out[a].Tag != out[b].Tag {
			return out[a].Tag < out[b].Tag
		}
		if out[a].File != out[b].File {
			return out[a].File < out[b].File
		}
		return out[a].Line < out[b].Line
	})
	return out
}

func normalizeShape(shape string) string {
	shape = strings.TrimSpace(shape)
	if shape == "" {
		return "unknown"
	}
	return shape
}

func renderMarkdown(data payload) string {
	var b strings.Builder

	b.WriteString("# Current Log Arg-Key Inventory\n\n")
	b.WriteString("This report is generated from current logging callsites in code.\n\n")
	b.WriteString("Generated by: `go run ./tools/specifications/export-log-args`\n\n")
	b.WriteString("## Summary\n")
	fmt.Fprintf(&b, "- Scanned files: %d\n", data.Summary.FileCount)
	fmt.Fprintf(&b, "- Logging calls scanned: %d\n", data.Summary.LogCallCount)
	fmt.Fprintf(&b, "- Unique tags observed: %d\n", data.Summary.UniqueTagCount)
	fmt.Fprintf(&b, "- Unique arg keys observed: %d\n", data.Summary.UniqueKeyCount)
	fmt.Fprintf(&b, "- Unresolved args callsites: %d\n\n", data.Summary.UnresolvedLogCallCount)

	b.WriteString("## Key Index\n\n")
	b.WriteString("| Key | Tags | Shapes | Producer files | Occurrences |\n")
	b.WriteString("| --- | ---: | --- | ---: | ---: |\n")

	keys := make([]string, 0, len(data.Keys))
	for key := range data.Keys {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		item := data.Keys[key]
		fmt.Fprintf(
			&b,
			"| `%s` | %d | `%s` | %d | %d |\n",
			key,
			len(item.Tags),
			strings.Join(item.Shapes, "`, `"),
			len(item.ProducerFiles),
			item.Occurrences,
		)
	}
	b.WriteString("\n")

	b.WriteString("## Key Details\n\n")
	for _, key := range keys {
		item := data.Keys[key]
		fmt.Fprintf(&b, "### `%s`\n\n", key)
		fmt.Fprintf(&b, "- Tags (%d): %s\n", len(item.Tags), inlineCodeList(item.Tags))
		fmt.Fprintf(&b, "- Shapes: %s\n", inlineCodeList(item.Shapes))
		fmt.Fprintf(&b, "- Producer files (%d): %s\n", len(item.ProducerFiles), inlineCodeList(item.ProducerFiles))
		if len(item.Samples) > 0 {
			b.WriteString("- Sample callsites:\n")
			for _, s := range item.Samples {
				fmt.Fprintf(&b, "  - `%s` at `%s:%d` (%s)\n", s.Tag, s.File, s.Line, s.Shape)
			}
		}
		b.WriteString("\n")
	}

	if len(data.Unresolved) > 0 {
		b.WriteString("## Unresolved Callsites\n\n")
		b.WriteString("These callsites use non-literal/non-tracked args expressions and require manual review.\n\n")
		b.WriteString("| Tag | File | Line | Args expression |\n")
		b.WriteString("| --- | --- | ---: | --- |\n")
		for _, u := range data.Unresolved {
			fmt.Fprintf(&b, "| `%s` | `%s` | %d | `%s` |\n", u.Tag, u.File, u.Line, escapePipes(u.ArgsExpr))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func inlineCodeList(items []string) string {
	if len(items) == 0 {
		return "`-`"
	}
	quoted := make([]string, 0, len(items))
	for _, item := range items {
		quoted = append(quoted, "`"+item+"`")
	}
	return strings.Join(quoted, ", ")
}

func escapePipes(value string) string {
	return strings.ReplaceAll(value, "|", "\\|")
}

func copyIntMap(input map[string]int) map[string]int {
	out := map[string]int{}
	for key, value := range input {
		out[key] = value
	}
	return out
}

func packedListOnlyKeys(keys map[string]keyEntry) []string {
	candidates := make([]string, 0)
	for key, item := range keys {
		if !isListLikeKey(key) {
			continue
		}
		if !containsString(item.Shapes, "string") {
			continue
		}
		if containsString(item.Shapes, "array") || containsString(item.Shapes, "object") {
			continue
		}
		candidates = append(candidates, key)
	}
	sort.Strings(candidates)
	return candidates
}

func isListLikeKey(key string) bool {
	if strings.Contains(key, "list") {
		return true
	}
	switch key {
	case "names", "servers", "addresses", "asns", "prefixes", "parent_addresses", "zone_addresses":
		return true
	default:
		return false
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func checkCoherencyGuardrails(data payload, nsAllowlistPath string, packedAllowlistPath string) error {
	nsAllowlist, err := loadNSAllowlist(nsAllowlistPath)
	if err != nil {
		return fmt.Errorf("load ns-string allowlist: %w", err)
	}
	packedAllowlist, err := loadStringSetAllowlist(packedAllowlistPath)
	if err != nil {
		return fmt.Errorf("load packed-list allowlist: %w", err)
	}

	var errs []string

	for path, count := range data.NSStringByFile {
		limit, ok := nsAllowlist[path]
		if !ok {
			errs = append(errs, fmt.Sprintf("new ns=.String() usage file not in allowlist: %s (%d)", path, count))
			continue
		}
		if count > limit {
			errs = append(errs, fmt.Sprintf("ns=.String() usage increased: %s (current=%d allowed=%d)", path, count, limit))
		}
	}

	for _, key := range data.PackedListOnlyKeys {
		if _, ok := packedAllowlist[key]; !ok {
			errs = append(errs, fmt.Sprintf("new packed-list-only arg key not in allowlist: %s", key))
		}
	}

	if len(errs) > 0 {
		sort.Strings(errs)
		for _, line := range errs {
			_, _ = fmt.Fprintf(os.Stderr, "error: %s\n", line)
		}
		return errors.New("guardrail violations detected")
	}

	_, _ = fmt.Fprintf(os.Stderr, "coherency check passed: ns_string_files=%d packed_list_keys=%d\n", len(data.NSStringByFile), len(data.PackedListOnlyKeys))
	return nil
}

func loadNSAllowlist(path string) (map[string]int, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	out := map[string]int{}
	scanner := bufio.NewScanner(file)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("%s:%d: expected '<path> <count>'", path, lineNo)
		}
		count, err := strconv.Atoi(fields[1])
		if err != nil || count < 0 {
			return nil, fmt.Errorf("%s:%d: invalid count %q", path, lineNo, fields[1])
		}
		out[fields[0]] = count
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func loadStringSetAllowlist(path string) (map[string]struct{}, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	out := map[string]struct{}{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out[line] = struct{}{}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
