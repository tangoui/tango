// Package front is the shader compiler front end: it loads a Go package
// written against package sl, type-checks it with go/types, checks that it
// stays inside the shader subset, and lowers it to package ir.
//
// Directives (in the doc comment of the declaration they mark):
//
//	//sl:vertex                          func(in I, vid uint32) O
//	//sl:fragment                        func(in O) sl.Vec4
//	//sl:uniform N                       var X SomeStruct
//	//sl:texture N filter=nearest|linear var T sl.Texture2D
//
// I is the per-instance attribute struct (every field tagged sl:"loc=N");
// vid is the vertex index (0..3, a triangle-strip quad). O is the
// interpolated struct: exactly one sl.Vec4 field tagged sl:"position", the
// others sl:"loc=N" or sl:"loc=N,flat".
//
// Other package-level functions reachable from the entry points become
// helper functions; package-level constants are folded into the
// expressions that use them. Functions not reachable from an entry point are
// ignored, so the package may hold ordinary Go code as well.
package front

import (
	"errors"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/scanner"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tangoui/tango/ui/shader/ir"
)

// slPath is the import path of the shading-language package.
const slPath = "github.com/tangoui/tango/ui/shader/sl"

// Error is one front-end diagnostic.
type Error struct {
	Pos token.Position
	Msg string
}

func (e Error) Error() string {
	if e.Pos.IsValid() {
		return e.Pos.String() + ": " + e.Msg
	}
	return e.Msg
}

// ErrorList is the error type returned by Load. It is sorted by position.
type ErrorList []Error

func (l ErrorList) Error() string {
	parts := make([]string, len(l))
	for i, e := range l {
		parts[i] = e.Error()
	}
	return strings.Join(parts, "\n")
}

// Load reads the Go package in dir (its non-test .go files) and compiles it
// to a shader module. On failure the error is an ErrorList.
func Load(dir string) (*ir.Module, error) {
	files, err := packageFiles(dir)
	if err != nil {
		return nil, ErrorList{{Msg: err.Error()}}
	}
	return loadFiles(files)
}

// packageFiles lists the non-test .go files of dir in name order.
func packageFiles(dir string) ([]string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range ents {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		files = append(files, filepath.Join(dir, n))
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("front: no Go files in %s", dir)
	}
	return files, nil
}

// loadFiles compiles the package made of the given files.
func loadFiles(filenames []string) (*ir.Module, error) {
	fset := token.NewFileSet()
	var errs ErrorList
	var files []*ast.File
	for _, name := range filenames {
		f, err := parser.ParseFile(fset, name, nil, parser.ParseComments|parser.SkipObjectResolution)
		var serrs scanner.ErrorList
		switch {
		case errors.As(err, &serrs):
			for _, e := range serrs {
				errs = append(errs, Error{Pos: e.Pos, Msg: e.Msg})
			}
		case err != nil:
			errs = append(errs, Error{Msg: err.Error()})
		}
		if f != nil {
			files = append(files, f)
		}
	}
	if len(errs) > 0 {
		return nil, errs.sorted()
	}

	info := &types.Info{
		Types:      map[ast.Expr]types.TypeAndValue{},
		Defs:       map[*ast.Ident]types.Object{},
		Uses:       map[*ast.Ident]types.Object{},
		Selections: map[*ast.SelectorExpr]*types.Selection{},
	}
	conf := types.Config{
		Importer: importer.ForCompiler(fset, "source", nil),
		Error: func(err error) {
			if te, ok := err.(types.Error); ok {
				errs = append(errs, Error{Pos: fset.Position(te.Pos), Msg: te.Msg})
				return
			}
			errs = append(errs, Error{Msg: err.Error()})
		},
	}
	pkg, _ := conf.Check(files[0].Name.Name, fset, files, info)
	if len(errs) > 0 {
		return nil, errs.sorted()
	}

	c := &compiler{
		fset:     fset,
		files:    files,
		pkg:      pkg,
		info:     info,
		structs:  map[*types.TypeName]*ir.Struct{},
		globals:  map[*types.Var]ir.Expr{},
		funcDecl: map[*types.Func]*ast.FuncDecl{},
		funcs:    map[*types.Func]*ir.Func{},
		vars:     map[*types.Var]*ir.Var{},
		layoutOK: map[*types.TypeName]bool{},
	}
	m := c.compile()
	if len(c.errs) > 0 {
		return nil, c.errs.sorted()
	}
	return m, nil
}

func (l ErrorList) sorted() ErrorList {
	sort.SliceStable(l, func(i, j int) bool {
		a, b := l[i].Pos, l[j].Pos
		if a.Filename != b.Filename {
			return a.Filename < b.Filename
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Column < b.Column
	})
	return l
}

// compiler holds the state of one Load.
type compiler struct {
	fset  *token.FileSet
	files []*ast.File
	pkg   *types.Package
	info  *types.Info
	errs  ErrorList

	mod      ir.Module
	structs  map[*types.TypeName]*ir.Struct
	globals  map[*types.Var]ir.Expr // uniform and texture references
	funcDecl map[*types.Func]*ast.FuncDecl
	funcs    map[*types.Func]*ir.Func
	vars     map[*types.Var]*ir.Var // parameters and locals

	vertex, fragment *ast.FuncDecl

	layoutOK map[*types.TypeName]bool // checkUniformLayout results
}

// ts formats t with short package names (sl.Vec4, not the import path).
func (c *compiler) ts(t types.Type) string {
	return types.TypeString(t, func(p *types.Package) string { return p.Name() })
}

func (c *compiler) errorf(pos token.Pos, format string, args ...any) {
	c.errs = append(c.errs, Error{Pos: c.fset.Position(pos), Msg: fmt.Sprintf(format, args...)})
}

// compile runs the passes in order; each later pass relies on the earlier
// ones having succeeded.
func (c *compiler) compile() *ir.Module {
	c.collect()
	if len(c.errs) > 0 {
		return nil
	}
	order := c.callGraph()
	if len(c.errs) > 0 {
		return nil
	}
	for _, d := range append([]*ast.FuncDecl{c.vertex, c.fragment}, order...) {
		c.checkFunc(d)
	}
	if len(c.errs) > 0 {
		return nil
	}
	c.lowerModule(order)
	return &c.mod
}

// directive returns the //sl: directive in doc ("vertex", "uniform 0", ...)
// and its position, or "" when there is none.
func directive(doc *ast.CommentGroup) (string, token.Pos) {
	if doc == nil {
		return "", token.NoPos
	}
	for _, cm := range doc.List {
		if rest, ok := strings.CutPrefix(cm.Text, "//sl:"); ok {
			return strings.TrimSpace(rest), cm.Pos()
		}
	}
	return "", token.NoPos
}

// collect finds the entry points and resource bindings and records every
// package-level function declaration.
func (c *compiler) collect() {
	for _, f := range c.files {
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				c.collectFunc(d)
			case *ast.GenDecl:
				if d.Tok != token.VAR {
					if dir, pos := directive(d.Doc); dir != "" {
						c.errorf(pos, "//sl:%s must mark a package-level var", dir)
					}
					continue
				}
				for _, s := range d.Specs {
					vs := s.(*ast.ValueSpec)
					doc := vs.Doc
					if doc == nil && len(d.Specs) == 1 {
						doc = d.Doc
					}
					c.collectVar(vs, doc)
				}
			}
		}
	}
	pkgPos := c.files[0].Package
	if c.vertex == nil {
		c.errorf(pkgPos, "missing //sl:vertex function")
	}
	if c.fragment == nil {
		c.errorf(pkgPos, "missing //sl:fragment function")
	}
	if c.vertex == nil || c.fragment == nil || len(c.errs) > 0 {
		return
	}
	c.checkEntrySignatures()
}

func (c *compiler) collectFunc(d *ast.FuncDecl) {
	fn, _ := c.info.Defs[d.Name].(*types.Func)
	if fn == nil {
		return
	}
	if d.Recv == nil {
		c.funcDecl[fn] = d
	}
	dir, pos := directive(d.Doc)
	if dir == "" {
		return
	}
	var slot **ast.FuncDecl
	switch dir {
	case "vertex":
		slot = &c.vertex
	case "fragment":
		slot = &c.fragment
	default:
		c.errorf(pos, "unknown directive //sl:%s on a function", dir)
		return
	}
	switch {
	case d.Recv != nil:
		c.errorf(d.Name.Pos(), "//sl:%s must mark a function, not a method", dir)
	case d.Type.TypeParams != nil:
		c.errorf(d.Name.Pos(), "generic functions are not supported")
	case *slot != nil:
		c.errorf(d.Name.Pos(), "duplicate //sl:%s function (also %s)", dir, (*slot).Name.Name)
	default:
		*slot = d
	}
}

func (c *compiler) collectVar(vs *ast.ValueSpec, doc *ast.CommentGroup) {
	dir, pos := directive(doc)
	if dir == "" {
		return // plain package vars are an error only when a shader uses them
	}
	fields := strings.Fields(dir)
	if len(vs.Names) != 1 || len(vs.Values) != 0 {
		c.errorf(vs.Pos(), "//sl:%s must mark a single var without initializer", fields[0])
		return
	}
	v, _ := c.info.Defs[vs.Names[0]].(*types.Var)
	if v == nil {
		return
	}
	var binding int
	if len(fields) < 2 {
		c.errorf(pos, "//sl:%s needs a binding index", fields[0])
		return
	}
	if _, err := fmt.Sscanf(fields[1], "%d", &binding); err != nil || binding < 0 || fmt.Sprint(binding) != fields[1] {
		c.errorf(pos, "invalid binding index %q", fields[1])
		return
	}
	name := v.Name()
	switch fields[0] {
	case "uniform":
		if len(fields) != 2 {
			c.errorf(pos, "unexpected arguments after //sl:uniform %s", fields[1])
			return
		}
		t := c.irType(v.Type(), vs.Pos())
		ref, ok := t.(ir.StructRef)
		if !ok {
			if t != nil {
				c.errorf(vs.Pos(), "//sl:uniform var %s must have a struct type", name)
			}
			return
		}
		for _, u := range c.mod.Uniforms {
			if u.Binding == binding {
				c.errorf(pos, "uniform binding %d already used by %s", binding, u.Name)
			}
		}
		if n, isNamed := types.Unalias(v.Type()).(*types.Named); isNamed {
			c.checkUniformLayout(n)
		}
		c.mod.Uniforms = append(c.mod.Uniforms, &ir.Uniform{Name: name, Type: ref.S, Binding: binding})
		c.globals[v] = &ir.GlobalRef{Name: name, T: ref}
	case "texture":
		if !isSL(v.Type(), "Texture2D") {
			c.errorf(vs.Pos(), "//sl:texture var %s must have type sl.Texture2D", name)
			return
		}
		filter := ""
		for _, a := range fields[2:] {
			if f, ok := strings.CutPrefix(a, "filter="); ok && (f == "nearest" || f == "linear") && filter == "" {
				filter = f
				continue
			}
			c.errorf(pos, "invalid //sl:texture argument %q (want filter=nearest or filter=linear)", a)
			return
		}
		if filter == "" {
			c.errorf(pos, "//sl:texture needs filter=nearest or filter=linear")
			return
		}
		for _, t := range c.mod.Textures {
			if t.Binding == binding {
				c.errorf(pos, "texture binding %d already used by %s", binding, t.Name)
			}
		}
		c.mod.Textures = append(c.mod.Textures, &ir.TextureVar{Name: name, Binding: binding, Filter: filter})
		c.globals[v] = &ir.GlobalRef{Name: name, T: ir.Texture2D{}}
	default:
		c.errorf(pos, "unknown directive //sl:%s on a var", fields[0])
	}
}

// isSL reports whether t is the named sl type name.
func isSL(t types.Type, name string) bool {
	n, ok := types.Unalias(t).(*types.Named)
	if !ok {
		return false
	}
	o := n.Obj()
	return o.Pkg() != nil && o.Pkg().Path() == slPath && o.Name() == name
}

// checkEntrySignatures validates the vertex and fragment signatures and the
// stage attributes of their structs.
func (c *compiler) checkEntrySignatures() {
	vsig := c.signature(c.vertex)
	if vsig.Params().Len() != 2 || vsig.Results().Len() != 1 ||
		!isBasic(vsig.Params().At(1).Type(), types.Uint32) {
		c.errorf(c.vertex.Name.Pos(), "//sl:vertex function must have signature func(in I, vid uint32) O")
		return
	}
	in := c.entryStruct(vsig.Params().At(0).Type(), c.vertex.Type.Params.Pos(), "vertex input")
	out := c.entryStruct(vsig.Results().At(0).Type(), c.vertex.Type.Results.Pos(), "vertex output")
	if in != nil {
		c.checkStageFields(in, false)
	}
	if out != nil {
		c.checkStageFields(out, true)
	}

	fsig := c.signature(c.fragment)
	if fsig.Params().Len() != 1 || fsig.Results().Len() != 1 || !isSL(fsig.Results().At(0).Type(), "Vec4") {
		c.errorf(c.fragment.Name.Pos(), "//sl:fragment function must have signature func(in O) sl.Vec4")
		return
	}
	if !types.Identical(fsig.Params().At(0).Type(), vsig.Results().At(0).Type()) {
		c.errorf(c.fragment.Type.Params.Pos(), "fragment input type %s must be the vertex output type %s",
			c.ts(fsig.Params().At(0).Type()), c.ts(vsig.Results().At(0).Type()))
	}
}

func (c *compiler) signature(d *ast.FuncDecl) *types.Signature {
	return c.info.Defs[d.Name].(*types.Func).Type().(*types.Signature)
}

func isBasic(t types.Type, k types.BasicKind) bool {
	b, ok := t.(*types.Basic)
	return ok && b.Kind() == k
}

// entryStruct returns the package-level named struct t, or reports an error.
func (c *compiler) entryStruct(t types.Type, pos token.Pos, what string) *types.Named {
	n, ok := types.Unalias(t).(*types.Named)
	if ok && n.Obj().Pkg() == c.pkg {
		if _, ok := n.Underlying().(*types.Struct); ok {
			return n
		}
	}
	c.errorf(pos, "%s must be a struct type declared in this package, not %s", what, c.ts(t))
	return nil
}

// checkStageFields validates the sl tags of a vertex input (output=false)
// or output (output=true) struct. Field errors point at the field.
func (c *compiler) checkStageFields(n *types.Named, output bool) {
	s := c.irType(n, n.Obj().Pos())
	ref, ok := s.(ir.StructRef)
	if !ok {
		return
	}
	st := n.Underlying().(*types.Struct)
	var positions []*types.Var
	locs := map[int]string{}
	for i, f := range ref.S.Fields {
		fv := st.Field(i)
		switch {
		case f.Attr.Builtin == "position":
			if !output {
				c.errorf(fv.Pos(), "vertex input field %s cannot be sl:\"position\"", f.Name)
				continue
			}
			positions = append(positions, fv)
			if !isSL(fv.Type(), "Vec4") {
				c.errorf(fv.Pos(), "sl:\"position\" field %s must have type sl.Vec4", f.Name)
			}
			if len(positions) == 2 {
				c.errorf(fv.Pos(), "vertex output %s must have exactly one sl:\"position\" field (also %s)",
					n.Obj().Name(), positions[0].Name())
			}
		case f.Attr.Location < 0:
			c.errorf(fv.Pos(), "%s field %s needs an sl:\"loc=N\" tag", stageName(output), f.Name)
		default:
			if f.Attr.Flat && !output {
				c.errorf(fv.Pos(), "flat applies only to vertex output fields")
			}
			if !output {
				c.checkAttrType(fv, n.Obj().Name())
			}
			if prev, dup := locs[f.Attr.Location]; dup {
				c.errorf(fv.Pos(), "location %d already used by field %s", f.Attr.Location, prev)
			}
			locs[f.Attr.Location] = f.Name
			if _, ok := f.Type.(ir.StructRef); ok {
				c.errorf(fv.Pos(), "stage field %s cannot be a struct", f.Name)
			}
			if _, ok := f.Type.(ir.Array); ok {
				c.errorf(fv.Pos(), "stage field %s cannot be an array", f.Name)
			}
		}
	}
	if output && len(positions) == 0 {
		c.errorf(n.Obj().Pos(), "vertex output %s must have exactly one sl:\"position\" field", n.Obj().Name())
	}
}

func stageName(output bool) string {
	if output {
		return "vertex output"
	}
	return "vertex input"
}

// callGraph walks the calls from the entry points, reports recursion, and
// returns the reachable helper functions callees-first.
func (c *compiler) callGraph() []*ast.FuncDecl {
	const (
		unvisited = iota
		active
		done
	)
	state := map[*ast.FuncDecl]int{}
	var order []*ast.FuncDecl
	var stack []string
	var visit func(d *ast.FuncDecl, helper bool)
	visit = func(d *ast.FuncDecl, helper bool) {
		state[d] = active
		stack = append(stack, d.Name.Name)
		ast.Inspect(d.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			callee := c.calledFunc(call)
			if callee == nil {
				return true
			}
			if callee == c.vertex || callee == c.fragment {
				c.errorf(call.Pos(), "entry point %s cannot be called", callee.Name.Name)
				return true
			}
			switch state[callee] {
			case active:
				i := len(stack) - 1
				for stack[i] != callee.Name.Name {
					i--
				}
				cycle := append(append([]string{}, stack[i:]...), callee.Name.Name)
				c.errorf(call.Pos(), "recursion is not supported: %s", strings.Join(cycle, " -> "))
			case unvisited:
				visit(callee, true)
			}
			return true
		})
		stack = stack[:len(stack)-1]
		state[d] = done
		if helper {
			order = append(order, d)
		}
	}
	visit(c.vertex, false)
	visit(c.fragment, false)
	return order
}

// calledFunc returns the declaration of the package-level function that
// call calls, or nil.
func (c *compiler) calledFunc(call *ast.CallExpr) *ast.FuncDecl {
	id, ok := ast.Unparen(call.Fun).(*ast.Ident)
	if !ok {
		return nil
	}
	fn, ok := c.info.Uses[id].(*types.Func)
	if !ok {
		return nil
	}
	return c.funcDecl[fn]
}
