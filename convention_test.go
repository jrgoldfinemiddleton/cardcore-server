package cardcoreserver

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// funcGroup classifies where a function declaration belongs in the
// canonical ordering. Lower values must appear before higher values.
type funcGroup int

const (
	groupConstructor      funcGroup = iota // New* functions
	groupExportedMethod                    // Exported methods (A-Z receiver)
	groupExportedFunc                      // Exported package-level functions
	groupUnexportedMethod                  // Unexported methods (a-z receiver)
	groupUnexportedFunc                    // Unexported package-level functions
)

// testGroup classifies where a declaration belongs in test file ordering.
type testGroup int

const (
	testGroupInterfaceCheck  testGroup = iota // var _ T = (*Impl)(nil)
	testGroupUnitTest                         // func Test* (non-integration)
	testGroupIntegrationTest                  // func Test*Integration
	testGroupBenchmark                        // func Benchmark*
	testGroupFuzz                             // func Fuzz*
	testGroupExample                          // func Example*
	testGroupHelper                           // Non-Test/Benchmark/Fuzz/Example funcs
)

// funcInfo captures the ordering-relevant properties of a single
// function declaration.
type funcInfo struct {
	name     string
	group    funcGroup
	receiver string
	line     int
}

// testDeclInfo captures a declaration's position in a test file.
type testDeclInfo struct {
	name  string
	group testGroup
	line  int
}

// walkOpts configures walkGoFiles. Zero values give sensible defaults:
// root defaults to cwd, suffix defaults to ".go", and skipDirs/skipFiles
// add to the always-skipped set (.git, vendor, testdata, .sisyphus).
type walkOpts struct {
	root      string
	suffix    string
	skipDirs  []string
	skipFiles []string
}

// TestNoNolint walks every .go file in the module and fails if any
// //nolint directive is present. Lint errors must be fixed in code
// rather than suppressed.
func TestNoNolint(t *testing.T) {
	walkGoFiles(t, walkOpts{}, func(path, rel string) {
		checkNoNolint(t, path, rel)
	})
}

// TestFunctionOrdering walks every .go file in the module and verifies
// that function declarations follow the ordering conventions described
// in CONTRIBUTING.md.
func TestFunctionOrdering(t *testing.T) {
	walkGoFiles(t, walkOpts{skipDirs: []string{"doc"}, skipFiles: []string{"doc.go"}},
		func(path, rel string) {
			checkDeclsBeforeFuncs(t, path, rel)
			if strings.HasSuffix(path, "_test.go") {
				checkTestFile(t, path, rel)
			} else {
				checkProdFile(t, path, rel)
			}
		})
}

// TestDocComments walks every .go file in the module and verifies that
// every function and method has a doc comment starting with its name.
// For doc.go files, it verifies the package doc comment exists and
// starts with "Package <name>".
func TestDocComments(t *testing.T) {
	walkGoFiles(t, walkOpts{skipDirs: []string{"doc"}}, func(path, rel string) {
		if strings.HasSuffix(path, "doc.go") {
			checkPackageDoc(t, path, rel)
			return
		}
		checkDocComments(t, path, rel)
	})
}

// TestDocGoExists walks every directory containing .go files and fails
// if a doc.go file is missing.
func TestDocGoExists(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}

	seen := map[string]bool{}
	walkGoFiles(t, walkOpts{}, func(path, _ string) {
		dir := filepath.Dir(path)
		if seen[dir] {
			return
		}
		seen[dir] = true

		docPath := filepath.Join(dir, "doc.go")
		if _, err := os.Stat(docPath); os.IsNotExist(err) {
			rel, _ := filepath.Rel(cwd, dir)
			t.Errorf("%s: missing doc.go", rel)
		}
	})
}

// walkGoFiles walks Go source files under opts.root (default: cwd) and
// invokes fn for each file matching opts.suffix (default: ".go").
// Directories named .git, vendor, testdata, or .sisyphus are always
// skipped, plus any in opts.skipDirs. Files whose basename appears in
// opts.skipFiles are skipped. rel is the path relative to the working
// directory.
func walkGoFiles(t *testing.T, opts walkOpts, fn func(path, rel string)) {
	t.Helper()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	root := opts.root
	if root == "" {
		root = cwd
	}
	suffix := opts.suffix
	if suffix == "" {
		suffix = ".go"
	}

	if _, err := os.Stat(root); os.IsNotExist(err) {
		return
	}

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			switch base {
			case ".git", "vendor", "testdata", ".sisyphus":
				return filepath.SkipDir
			}
			for _, s := range opts.skipDirs {
				if base == s {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(path, suffix) {
			return nil
		}
		base := filepath.Base(path)
		for _, s := range opts.skipFiles {
			if base == s {
				return nil
			}
		}

		rel, _ := filepath.Rel(cwd, path)
		fn(path, rel)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir: %v", err)
	}
}

// checkProdFile verifies production file ordering: constructors →
// exported methods → exported funcs → unexported methods → unexported
// funcs, with methods on the same receiver contiguous.
func checkProdFile(t *testing.T, path, rel string) {
	t.Helper()

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Errorf("%s: parse error: %v", rel, err)
		return
	}

	funcs := make([]funcInfo, 0, len(f.Decls))
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		fi := classifyFunc(fn)
		fi.line = fset.Position(fn.Pos()).Line
		funcs = append(funcs, fi)
	}

	if len(funcs) == 0 {
		return
	}

	for i := 1; i < len(funcs); i++ {
		prev := funcs[i-1]
		curr := funcs[i]
		if curr.group < prev.group {
			t.Errorf("%s:%d: %s (group %s) appears after %s:%d: %s (group %s) — wrong order",
				rel, curr.line, curr.name, groupName(curr.group),
				rel, prev.line, prev.name, groupName(prev.group))
		}
	}

	lastSeen := map[string]int{}
	for i, fi := range funcs {
		if fi.receiver == "" {
			continue
		}
		if prev, ok := lastSeen[fi.receiver]; ok {
			for j := prev + 1; j < i; j++ {
				between := funcs[j]
				if between.receiver != fi.receiver {
					t.Errorf(
						"%s:%d: %s (receiver %s) is separated from %s:%d: %s "+
							"by %s:%d: %s (receiver %q)",
						rel, fi.line, fi.name, fi.receiver,
						rel, funcs[prev].line, funcs[prev].name,
						rel, between.line, between.name, receiverLabel(between.receiver))
					break
				}
			}
		}
		lastSeen[fi.receiver] = i
	}
}

// checkTestFile verifies test file ordering: interface checks → unit
// tests → integration tests → benchmarks → fuzz → examples → helpers.
func checkTestFile(t *testing.T, path, rel string) {
	t.Helper()

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Errorf("%s: parse error: %v", rel, err)
		return
	}

	var decls []testDeclInfo

	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			td := classifyTestFunc(d)
			td.line = fset.Position(d.Pos()).Line
			decls = append(decls, td)
		case *ast.GenDecl:
			if d.Tok == token.VAR {
				for _, spec := range d.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					if isInterfaceCheck(vs) {
						decls = append(decls, testDeclInfo{
							name:  vs.Names[0].Name,
							group: testGroupInterfaceCheck,
							line:  fset.Position(d.Pos()).Line,
						})
					}
				}
			}
		}
	}

	if len(decls) == 0 {
		return
	}

	for i := 1; i < len(decls); i++ {
		prev := decls[i-1]
		curr := decls[i]
		if curr.group < prev.group {
			t.Errorf("%s:%d: %s (group %s) appears after %s:%d: %s (group %s) — wrong order",
				rel, curr.line, curr.name, testGroupName(curr.group),
				rel, prev.line, prev.name, testGroupName(prev.group))
		}
	}
}

// checkDeclsBeforeFuncs verifies that all type, const, and var
// declarations appear before any function or method declarations.
// Import declarations are exempt.
func checkDeclsBeforeFuncs(t *testing.T, path, rel string) {
	t.Helper()

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Errorf("%s: parse error: %v", rel, err)
		return
	}

	firstFuncLine := 0
	firstFuncName := ""
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		firstFuncLine = fset.Position(fn.Pos()).Line
		firstFuncName = fn.Name.Name
		break
	}

	if firstFuncLine == 0 {
		return
	}

	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		if gd.Tok == token.IMPORT {
			continue
		}
		line := fset.Position(gd.Pos()).Line
		if line > firstFuncLine {
			t.Errorf(
				"%s:%d: %s declaration appears after first function %s (line %d) — "+
					"declarations must precede all functions",
				rel, line, gd.Tok, firstFuncName, firstFuncLine)
		}
	}
}

// checkDocComments verifies that every function and method in the file
// has a doc comment whose first word is the function or method name.
func checkDocComments(t *testing.T, path, rel string) {
	t.Helper()

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		t.Errorf("%s: parse error: %v", rel, err)
		return
	}

	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		name := fn.Name.Name
		line := fset.Position(fn.Pos()).Line

		if fn.Doc == nil || len(fn.Doc.List) == 0 {
			t.Errorf("%s:%d: %s has no doc comment", rel, line, name)
			continue
		}

		first := fn.Doc.List[0].Text
		prefix := "// " + name + " "
		if !strings.HasPrefix(first, prefix) {
			t.Errorf("%s:%d: doc comment for %s must start with %q, got %q",
				rel, line, name, "// "+name+" ...", first)
		}
	}
}

// checkPackageDoc verifies that a doc.go file has a package doc comment
// starting with "Package <name>".
func checkPackageDoc(t *testing.T, path, rel string) {
	t.Helper()

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		t.Errorf("%s: parse error: %v", rel, err)
		return
	}

	if f.Doc == nil || len(f.Doc.List) == 0 {
		t.Errorf("%s: doc.go has no package doc comment", rel)
		return
	}

	first := f.Doc.List[0].Text
	prefix := "// Package " + f.Name.Name + " "
	if !strings.HasPrefix(first, prefix) {
		t.Errorf("%s: package doc comment must start with %q, got %q",
			rel, "// Package "+f.Name.Name+" ...", first)
	}
}

// checkNoNolint reports any //nolint directive in the file.
func checkNoNolint(t *testing.T, path, rel string) {
	t.Helper()

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		t.Errorf("%s: parse error: %v", rel, err)
		return
	}

	for _, cg := range f.Comments {
		for _, c := range cg.List {
			text := strings.TrimPrefix(c.Text, "//")
			text = strings.TrimPrefix(text, "/*")
			text = strings.TrimSpace(text)
			if strings.HasPrefix(text, "nolint") {
				line := fset.Position(c.Pos()).Line
				t.Errorf("%s:%d: nolint directive forbidden — fix the code instead",
					rel, line)
			}
		}
	}
}

// classifyFunc determines the group and receiver for a production
// function declaration.
func classifyFunc(fn *ast.FuncDecl) funcInfo {
	name := fn.Name.Name
	exported := ast.IsExported(name)
	recv := receiverType(fn)

	var g funcGroup
	switch {
	case recv == "" && exported && strings.HasPrefix(name, "New"):
		g = groupConstructor
	case recv != "" && exported:
		g = groupExportedMethod
	case recv == "" && exported:
		g = groupExportedFunc
	case recv != "" && !exported:
		g = groupUnexportedMethod
	default:
		g = groupUnexportedFunc
	}

	return funcInfo{name: name, group: g, receiver: recv}
}

// classifyTestFunc determines the test group for a function in a test file.
func classifyTestFunc(fn *ast.FuncDecl) testDeclInfo {
	name := fn.Name.Name

	var g testGroup
	switch {
	case strings.HasPrefix(name, "Test"):
		if isIntegrationTestName(name) {
			g = testGroupIntegrationTest
		} else {
			g = testGroupUnitTest
		}
	case strings.HasPrefix(name, "Benchmark"):
		g = testGroupBenchmark
	case strings.HasPrefix(name, "Fuzz"):
		g = testGroupFuzz
	case strings.HasPrefix(name, "Example"):
		g = testGroupExample
	default:
		g = testGroupHelper
	}

	return testDeclInfo{name: name, group: g}
}

// isIntegrationTestName reports whether a test function name indicates
// an integration test.
func isIntegrationTestName(name string) bool {
	return strings.HasSuffix(name, "Integration")
}

// receiverType returns the base type name of a method's receiver, or
// "" for package-level functions.
func receiverType(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	t := fn.Recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	if ident, ok := t.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

// isInterfaceCheck reports whether a var spec looks like
// var _ SomeType = (*Impl)(nil).
func isInterfaceCheck(vs *ast.ValueSpec) bool {
	if len(vs.Names) != 1 || vs.Names[0].Name != "_" {
		return false
	}
	return vs.Type != nil
}

// groupName returns a human-readable label for a production function group.
func groupName(g funcGroup) string {
	switch g {
	case groupConstructor:
		return "constructor"
	case groupExportedMethod:
		return "exported method"
	case groupExportedFunc:
		return "exported func"
	case groupUnexportedMethod:
		return "unexported method"
	case groupUnexportedFunc:
		return "unexported func"
	default:
		return "unknown"
	}
}

// testGroupName returns a human-readable label for a test declaration group.
func testGroupName(g testGroup) string {
	switch g {
	case testGroupInterfaceCheck:
		return "interface check"
	case testGroupUnitTest:
		return "unit test"
	case testGroupIntegrationTest:
		return "integration test"
	case testGroupBenchmark:
		return "benchmark"
	case testGroupFuzz:
		return "fuzz"
	case testGroupExample:
		return "example"
	case testGroupHelper:
		return "helper"
	default:
		return "unknown"
	}
}

// receiverLabel returns a display string for a receiver, or
// "package-level" if empty.
func receiverLabel(recv string) string {
	if recv == "" {
		return "package-level"
	}
	return recv
}
