// Package bindgen generates TypeScript clients for services a Tango app
// exposes with bind.Register.
//
// It reads the app's main package with go/parser and go/types (the
// "source" importer, so no extra dependencies and no compiled export data
// are needed), finds every bind.Register(router, "Name", svc) call whose
// name is a constant string, and maps the exported methods of svc's type
// to functions calling @tangoui/runtime's call("Name.Method", ...args).
//
// Type mapping follows encoding/json, which is what crosses the wire: see
// Mapper.
package bindgen

import (
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/constant"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// BindPath is the import path of the package whose Register calls are
// collected.
const BindPath = "github.com/tangoui/tango/bind"

// Service is one bind.Register call.
type Service struct {
	Name    string
	Pos     token.Position
	Methods []Method
}

// Method is one exported method of a service.
type Method struct {
	Name     string
	Doc      string
	Params   []Param
	Variadic bool   // the last parameter is variadic (its Type is the element type)
	Result   string // TS type; "void" when the method returns nothing (or only error)
}

// Param is a method parameter after the optional context.Context.
type Param struct{ Name, Type string }

// Result is what Analyze found.
type Result struct {
	Services []Service
	Models   []*Model // interfaces for named struct types, in first-use order
	Warnings []string
}

// buildDirMu serializes Analyze calls, which set build.Default.Dir.
var buildDirMu sync.Mutex

// Analyze type-checks the Go package in dir and collects its services.
// Type errors (for instance an import that cannot be resolved) become
// warnings; types they affect map to any.
func Analyze(dir string) (*Result, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	files, err := parseDir(fset, dir)
	if err != nil {
		return nil, err
	}
	// The source importer resolves imports through build.Default, whose go
	// list runs in build.Default.Dir, the working directory when empty: in
	// another module (tan generate -C elsewhere) the app's imports would be
	// resolved against that module. Point it at the app for the check.
	buildDirMu.Lock()
	defer buildDirMu.Unlock()
	defer func(saved string) { build.Default.Dir = saved }(build.Default.Dir)
	build.Default.Dir = dir
	res := &Result{}
	conf := types.Config{
		Importer: importer.ForCompiler(fset, "source", nil),
		Error: func(err error) {
			if len(res.Warnings) < 10 {
				res.Warnings = append(res.Warnings, err.Error())
			}
		},
	}
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
	}
	_, _ = conf.Check(files[0].Name.Name, fset, files, info) // errors went to conf.Error

	docs := methodDocs(files)
	m := NewMapper()
	for _, f := range files {
		names := bindNames(f)
		if len(names) == 0 {
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !isRegister(call, names) {
				return true
			}
			pos := fset.Position(call.Pos())
			svc, warn := service(call, info, m, docs)
			if warn != "" {
				res.Warnings = append(res.Warnings, fmt.Sprintf("%s: %s", pos, warn))
				return true
			}
			svc.Pos = pos
			res.Services = append(res.Services, svc)
			return true
		})
	}
	res.Models = m.Models()
	return res, nil
}

func parseDir(fset *token.FileSet, dir string) ([]*ast.File, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []*ast.File
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, n), nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		if len(files) > 0 && f.Name.Name != files[0].Name.Name {
			continue // a stray file of another package (e.g. a //go:build ignore tool)
		}
		files = append(files, f)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no Go files in %s", dir)
	}
	return files, nil
}

// bindNames returns the names f imports the bind package under.
func bindNames(f *ast.File) []string {
	var names []string
	for _, imp := range f.Imports {
		p, _ := strconv.Unquote(imp.Path.Value)
		if p != BindPath {
			continue
		}
		name := "bind"
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if name != "_" && name != "." {
			names = append(names, name)
		}
	}
	return names
}

func isRegister(call *ast.CallExpr, names []string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Register" || len(call.Args) != 3 {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	return ok && slices.Contains(names, x.Name)
}

// methodDocs maps "Recv.Method" to the method's doc comment.
func methodDocs(files []*ast.File) map[string]string {
	docs := map[string]string{}
	for _, f := range files {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Recv == nil || fd.Doc == nil || len(fd.Recv.List) == 0 {
				continue
			}
			t := fd.Recv.List[0].Type
			if s, ok := t.(*ast.StarExpr); ok {
				t = s.X
			}
			if ix, ok := t.(*ast.IndexExpr); ok {
				t = ix.X
			}
			if id, ok := t.(*ast.Ident); ok {
				docs[id.Name+"."+fd.Name.Name] = strings.TrimSpace(fd.Doc.Text())
			}
		}
	}
	return docs
}

var (
	errType = types.Universe.Lookup("error").Type()
)

func service(call *ast.CallExpr, info *types.Info, m *Mapper, docs map[string]string) (Service, string) {
	var svc Service
	tv, ok := info.Types[call.Args[1]]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return svc, "service name is not a constant string; skipped"
	}
	svc.Name = constant.StringVal(tv.Value)
	t := info.TypeOf(call.Args[2])
	if t == nil || t == types.Typ[types.Invalid] {
		return svc, fmt.Sprintf("cannot determine the type of service %q; skipped", svc.Name)
	}
	recv := t
	if p, ok := recv.(*types.Pointer); ok {
		recv = p.Elem()
	}
	recvName := ""
	if n, ok := recv.(*types.Named); ok {
		recvName = n.Obj().Name()
	}
	ms := types.NewMethodSet(t)
	for i := range ms.Len() {
		fn, ok := ms.At(i).Obj().(*types.Func)
		if !ok || !fn.Exported() {
			continue
		}
		sig := fn.Signature()
		meth := Method{Name: fn.Name(), Doc: docs[recvName+"."+fn.Name()], Variadic: sig.Variadic()}
		for j := range sig.Params().Len() {
			p := sig.Params().At(j)
			if j == 0 && isContext(p.Type()) {
				continue
			}
			pt := p.Type()
			if meth.Variadic && j == sig.Params().Len()-1 {
				pt = pt.(*types.Slice).Elem()
			}
			meth.Params = append(meth.Params, Param{Name: paramName(p.Name(), j), Type: m.TS(pt)})
		}
		meth.Result = "void"
		res := sig.Results()
		if res.Len() > 0 && !types.Identical(res.At(0).Type(), errType) {
			meth.Result = m.TS(res.At(0).Type())
		}
		svc.Methods = append(svc.Methods, meth)
	}
	slices.SortFunc(svc.Methods, func(a, b Method) int { return strings.Compare(a.Name, b.Name) })
	if len(svc.Methods) == 0 {
		return svc, fmt.Sprintf("service %q has no exported methods; skipped", svc.Name)
	}
	return svc, ""
}

func isContext(t types.Type) bool {
	n, ok := t.(*types.Named)
	return ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == "context" && n.Obj().Name() == "Context"
}

var tsReserved = map[string]bool{
	"break": true, "case": true, "catch": true, "class": true, "const": true, "continue": true,
	"debugger": true, "default": true, "delete": true, "do": true, "else": true, "enum": true,
	"export": true, "extends": true, "false": true, "finally": true, "for": true, "function": true,
	"if": true, "import": true, "in": true, "instanceof": true, "new": true, "null": true,
	"return": true, "super": true, "switch": true, "this": true, "throw": true, "true": true,
	"try": true, "typeof": true, "var": true, "void": true, "while": true, "with": true,
	"let": true, "static": true, "yield": true, "await": true, "arguments": true, "eval": true,
}

func paramName(name string, i int) string {
	if name == "" || name == "_" {
		return "arg" + strconv.Itoa(i)
	}
	if tsReserved[name] {
		return name + "_"
	}
	return name
}

// ErrNoServices is returned by Generate when there is nothing to write.
var ErrNoServices = errors.New("no bind.Register calls with a constant name found")
