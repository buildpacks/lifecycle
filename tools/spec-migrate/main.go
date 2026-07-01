// spec-migrate rewrites test files that use github.com/sclevine/spec into
// idiomatic native Go subtests using t.Run and t.Cleanup.
//
// It handles the mechanical transformations described in the "Migrate Test
// Framework from sclevine/spec" RFC:
//
//   - Removes imports of github.com/sclevine/spec and .../spec/report.
//   - Rewrites `spec.Run(t, "Name", testX, spec.Report(...))` (called from a
//     TestX function that immediately delegates) into inlined subtests.
//   - Converts `when("desc", func() { ... })` and `it("desc", func() { ... })`
//     to `t.Run("desc", func(t *testing.T) { ... })`.
//   - Converts `it.After(func() { ... })` to `t.Cleanup(func() { ... })`.
//   - Adds `t.Parallel()` at the top of the test function when `spec.Parallel()`
//     was present on the spec.Run call.
//
// It DOES NOT attempt to convert `it.Before` blocks: those require human
// judgment about whether the enclosed subtests mutate shared state (see
// Option A vs. Option B in the RFC). Files containing `it.Before` are
// reported and left untouched unless --allow-before is passed.
//
// Usage:
//
//	go run ./tools/spec-migrate [flags] <file-or-package-path>...
//
// Flags:
//
//	-w        Write results back to source files (default: print diff to stdout).
//	-dry-run  Report what would be done without writing changes.
//	-allow-before  Convert files even when they contain it.Before (best-effort).
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	specPkgPath   = "github.com/sclevine/spec"
	reportPkgPath = "github.com/sclevine/spec/report"
)

type options struct {
	write        bool
	dryRun       bool
	allowBefore  bool
	writtenFiles []string
	skipped      []string
	unchanged    []string
	errors       []string
}

func main() {
	var opts options
	flag.BoolVar(&opts.write, "w", false, "write results back to source files")
	flag.BoolVar(&opts.dryRun, "dry-run", false, "report actions without writing")
	flag.BoolVar(&opts.allowBefore, "allow-before", false, "convert files with it.Before (best-effort; may leave TODOs)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: spec-migrate [flags] <path>...\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}

	for _, arg := range flag.Args() {
		if err := opts.processPath(arg); err != nil {
			opts.errors = append(opts.errors, fmt.Sprintf("%s: %v", arg, err))
		}
	}

	fmt.Fprintf(os.Stderr, "\n=== summary ===\n")
	fmt.Fprintf(os.Stderr, "converted: %d file(s)\n", len(opts.writtenFiles))
	for _, f := range opts.writtenFiles {
		fmt.Fprintf(os.Stderr, "  %s\n", f)
	}
	fmt.Fprintf(os.Stderr, "skipped (it.Before / non-mechanical): %d\n", len(opts.skipped))
	for _, f := range opts.skipped {
		fmt.Fprintf(os.Stderr, "  %s\n", f)
	}
	if len(opts.unchanged) > 0 {
		fmt.Fprintf(os.Stderr, "no spec usage: %d\n", len(opts.unchanged))
	}
	if len(opts.errors) > 0 {
		fmt.Fprintf(os.Stderr, "errors: %d\n", len(opts.errors))
		for _, e := range opts.errors {
			fmt.Fprintf(os.Stderr, "  %s\n", e)
		}
		os.Exit(1)
	}
}

func (o *options) processPath(root string) error {
	info, err := os.Stat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return o.processFile(root)
	}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		return o.processFile(path)
	})
}

func (o *options) processFile(path string) error {
	src, err := os.ReadFile(path) // #nosec G304 -- tool intentionally reads paths from CLI args
	if err != nil {
		return err
	}
	if !bytes.Contains(src, []byte(specPkgPath)) {
		o.unchanged = append(o.unchanged, path)
		return nil
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	hasBefore := containsIdent(file, "it", "Before")
	if hasBefore && !o.allowBefore {
		o.skipped = append(o.skipped, path)
		return nil
	}

	changed, err := transformFile(fset, file)
	if err != nil {
		return err
	}
	if !changed {
		o.unchanged = append(o.unchanged, path)
		return nil
	}

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, file); err != nil {
		return fmt.Errorf("format: %w", err)
	}
	out := buf.Bytes()

	switch {
	case o.write:
		if err := os.WriteFile(path, out, 0o600); err != nil {
			return err
		}
	case o.dryRun:
	default:
		fmt.Printf("=== %s ===\n%s\n", path, out)
	}
	o.writtenFiles = append(o.writtenFiles, path)
	return nil
}

// containsIdent reports whether pkg.name is referenced anywhere in the file
// as a selector expression (e.g., it.Before).
func containsIdent(file *ast.File, pkg, name string) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if found {
			return false
		}
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		x, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if x.Name == pkg && sel.Sel.Name == name {
			found = true
		}
		return true
	})
	return found
}

// transformFile mutates file in place and returns true if anything changed.
func transformFile(fset *token.FileSet, file *ast.File) (bool, error) {
	changed := false

	// 1. Remove imports of sclevine/spec and sclevine/spec/report.
	newImports := file.Imports[:0]
	newDecls := file.Decls[:0]
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.IMPORT {
			newDecls = append(newDecls, decl)
			continue
		}
		specs := gd.Specs[:0]
		for _, s := range gd.Specs {
			is := s.(*ast.ImportSpec)
			path := strings.Trim(is.Path.Value, `"`)
			if path == specPkgPath || path == reportPkgPath {
				changed = true
				continue
			}
			specs = append(specs, s)
		}
		gd.Specs = specs
		if len(specs) > 0 {
			newDecls = append(newDecls, gd)
		} else {
			changed = true
		}
	}
	file.Decls = newDecls

	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if path == specPkgPath || path == reportPkgPath {
			continue
		}
		newImports = append(newImports, imp)
	}
	file.Imports = newImports

	// 2. Find pairs of (TestX func that calls spec.Run, testX func with (t, when, it)).
	//    Merge them into a single native TestX(t *testing.T).
	testFns := map[string]*ast.FuncDecl{}
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		testFns[fd.Name.Name] = fd
	}

	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if !isTestFunc(fd) {
			continue
		}
		specRunIdx, specRun := findSpecRunStmt(fd)
		if specRun == nil {
			continue
		}
		if len(specRun.Args) < 3 {
			return false, fmt.Errorf("%s: unexpected spec.Run signature", posOf(fset, specRun))
		}
		bodyFnName, ok := identOf(specRun.Args[2])
		if !ok {
			// Not a simple identifier reference (e.g., testPlatform(api)); leave alone.
			continue
		}
		bodyFn, ok := testFns[bodyFnName]
		if !ok {
			continue
		}
		parallel := hasSpecOption(specRun.Args[3:], "Parallel")

		// Rewrite the body of bodyFn.
		if err := rewriteSpecBody(bodyFn); err != nil {
			return false, err
		}

		// Splice the rewritten body in place of the spec.Run statement so any
		// pre-setup or post-cleanup around spec.Run is preserved.
		prefix := append([]ast.Stmt(nil), fd.Body.List[:specRunIdx]...)
		suffix := append([]ast.Stmt(nil), fd.Body.List[specRunIdx+1:]...)
		var replacement []ast.Stmt
		if parallel {
			replacement = append(replacement, tParallelStmt())
		}
		replacement = append(replacement, bodyFn.Body.List...)
		fd.Body.List = append(append(prefix, replacement...), suffix...)
		removeFuncDecl(file, bodyFn)
		changed = true
	}

	// 3. Recursively convert any remaining when/it/it.After calls in the file
	//    (covers files where spec.Run wraps something more elaborate).
	ast.Inspect(file, func(n ast.Node) bool {
		bs, ok := n.(*ast.BlockStmt)
		if !ok {
			return true
		}
		for i, stmt := range bs.List {
			bs.List[i] = convertStmt(stmt)
		}
		return true
	})

	return changed, nil
}

func isTestFunc(fd *ast.FuncDecl) bool {
	if fd.Recv != nil {
		return false
	}
	if !strings.HasPrefix(fd.Name.Name, "Test") {
		return false
	}
	if fd.Type.Params == nil || len(fd.Type.Params.List) != 1 {
		return false
	}
	return true
}

func findSpecRunStmt(fd *ast.FuncDecl) (int, *ast.CallExpr) {
	if fd.Body == nil {
		return -1, nil
	}
	for i, stmt := range fd.Body.List {
		es, ok := stmt.(*ast.ExprStmt)
		if !ok {
			continue
		}
		call, ok := es.X.(*ast.CallExpr)
		if !ok {
			continue
		}
		if isSelectorCall(call, "spec", "Run") {
			return i, call
		}
	}
	return -1, nil
}

func isSelectorCall(call *ast.CallExpr, pkg, name string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return id.Name == pkg && sel.Sel.Name == name
}

func identOf(e ast.Expr) (string, bool) {
	id, ok := e.(*ast.Ident)
	if !ok {
		return "", false
	}
	return id.Name, true
}

func hasSpecOption(args []ast.Expr, name string) bool {
	for _, a := range args {
		call, ok := a.(*ast.CallExpr)
		if !ok {
			continue
		}
		if isSelectorCall(call, "spec", name) {
			return true
		}
	}
	return false
}

func removeFuncDecl(file *ast.File, target *ast.FuncDecl) {
	out := file.Decls[:0]
	for _, d := range file.Decls {
		if d == target {
			continue
		}
		out = append(out, d)
	}
	file.Decls = out
}

// rewriteSpecBody rewrites a func(t, when spec.G, it spec.S) so that:
//   - its parameter list becomes (t *testing.T),
//   - the body's when()/it()/it.After() calls are converted.
func rewriteSpecBody(fd *ast.FuncDecl) error {
	if fd.Type == nil || fd.Type.Params == nil {
		return nil
	}
	// Force signature to (t *testing.T).
	fd.Type.Params.List = []*ast.Field{
		{
			Names: []*ast.Ident{ast.NewIdent("t")},
			Type: &ast.StarExpr{X: &ast.SelectorExpr{
				X:   ast.NewIdent("testing"),
				Sel: ast.NewIdent("T"),
			}},
		},
	}
	if fd.Body != nil {
		for i, s := range fd.Body.List {
			fd.Body.List[i] = convertStmt(s)
		}
	}
	return nil
}

// convertStmt walks a statement and converts recognized when/it/it.After
// call expressions to their native Go equivalents.
func convertStmt(stmt ast.Stmt) ast.Stmt {
	if stmt == nil {
		return stmt
	}
	// Handle ExprStmt: when(...), it(...), it.After(...), it.Focus(...), etc.
	if es, ok := stmt.(*ast.ExprStmt); ok {
		if call, ok := es.X.(*ast.CallExpr); ok {
			if newCall, ok := convertCall(call); ok {
				es.X = newCall
			}
		}
	}
	// Recurse into nested blocks.
	ast.Inspect(stmt, func(n ast.Node) bool {
		bs, ok := n.(*ast.BlockStmt)
		if !ok {
			return true
		}
		for i, s := range bs.List {
			bs.List[i] = convertInner(s)
		}
		return true
	})
	return stmt
}

func convertInner(stmt ast.Stmt) ast.Stmt {
	if stmt == nil {
		return stmt
	}
	if es, ok := stmt.(*ast.ExprStmt); ok {
		if call, ok := es.X.(*ast.CallExpr); ok {
			if newCall, ok := convertCall(call); ok {
				es.X = newCall
			}
		}
	}
	return stmt
}

// convertCall rewrites one of:
//
//	when("desc", func() { ... })       -> t.Run("desc", func(t *testing.T) { ... })
//	it("desc", func() { ... })         -> t.Run("desc", func(t *testing.T) { ... })
//	it.After(func() { ... })           -> t.Cleanup(func() { ... })
//	it.Focus("desc", func() { ... })   -> t.Run("desc", func(t *testing.T) { ... })
//
// It returns the new expression and true if a rewrite happened.
func convertCall(call *ast.CallExpr) (ast.Expr, bool) {
	// Detect when(...) or it(...).
	if id, ok := call.Fun.(*ast.Ident); ok {
		switch id.Name {
		case "when", "it":
			return toTRun(call), true
		}
	}
	// Detect it.After / it.Focus / it.Pend etc.
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		x, ok := sel.X.(*ast.Ident)
		if !ok {
			return call, false
		}
		if x.Name != "it" && x.Name != "when" {
			return call, false
		}
		switch sel.Sel.Name {
		case "After":
			return toTCleanup(call), true
		case "Before":
			// Left for humans; converters may leave this in and mark it.
			return call, false
		case "Focus", "Pend":
			return toTRun(call), true
		}
	}
	return call, false
}

// toTRun rewrites `X("desc", func() { ... })` to
// `t.Run("desc", func(t *testing.T) { ... })`. The inner function body is
// also walked so nested calls get converted.
func toTRun(call *ast.CallExpr) *ast.CallExpr {
	if len(call.Args) < 2 {
		return call
	}
	fnLit, ok := call.Args[len(call.Args)-1].(*ast.FuncLit)
	if !ok {
		return call
	}
	replaceFuncLitParamsWithT(fnLit)
	// Walk body to convert nested when/it/it.After calls.
	if fnLit.Body != nil {
		for i, s := range fnLit.Body.List {
			fnLit.Body.List[i] = convertInner(s)
		}
	}
	return &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent("t"),
			Sel: ast.NewIdent("Run"),
		},
		Args: []ast.Expr{call.Args[0], fnLit},
	}
}

// toTCleanup rewrites `it.After(func() { ... })` to
// `t.Cleanup(func() { ... })`. The func literal parameter list stays empty.
func toTCleanup(call *ast.CallExpr) *ast.CallExpr {
	if len(call.Args) < 1 {
		return call
	}
	fnLit, ok := call.Args[0].(*ast.FuncLit)
	if !ok {
		return call
	}
	// Cleanup takes func(), so leave the parameter list empty.
	if fnLit.Type != nil {
		fnLit.Type.Params = &ast.FieldList{}
	}
	if fnLit.Body != nil {
		for i, s := range fnLit.Body.List {
			fnLit.Body.List[i] = convertInner(s)
		}
	}
	return &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent("t"),
			Sel: ast.NewIdent("Cleanup"),
		},
		Args: []ast.Expr{fnLit},
	}
}

func replaceFuncLitParamsWithT(fn *ast.FuncLit) {
	if fn.Type == nil {
		return
	}
	fn.Type.Params = &ast.FieldList{List: []*ast.Field{
		{
			Names: []*ast.Ident{ast.NewIdent("t")},
			Type: &ast.StarExpr{X: &ast.SelectorExpr{
				X:   ast.NewIdent("testing"),
				Sel: ast.NewIdent("T"),
			}},
		},
	}}
}

func tParallelStmt() ast.Stmt {
	return &ast.ExprStmt{X: &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent("t"),
			Sel: ast.NewIdent("Parallel"),
		},
	}}
}

func posOf(fset *token.FileSet, n ast.Node) string {
	return fset.Position(n.Pos()).String()
}
