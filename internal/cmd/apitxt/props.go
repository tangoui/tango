package main

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"go/types"
)

// propPkgs are the packages (module-relative) whose call sites of the
// string-keyed ConfigProvider readers are listed as prop lines
// (docs/specs/api-stability.md R11).
var propPkgs = map[string]bool{"ui": true, "ui/mobile": true}

// propReader describes a function that reads a ConfigProvider default
// by component name and key. Calls whose component and key are string
// constants become `prop "<Comp>" "<key>" <type>` lines; calls with
// computed names are not listed.
type propReader struct {
	pkg    string // module-relative package of the function
	recv   string // receiver type name for methods
	name   string
	comp   int    // argument index of the component name
	key    int    // argument index of the key; -1 uses fixedKey
	fixKey string // key for readers that take none
	prefix string // prepended to the component ("mobile.")
	typ    string // value type; "" means the type argument of the call
}

var propReaders = []propReader{
	{pkg: "ui", name: "Prop", comp: 1, key: 2},
	{pkg: "ui", recv: "Context", name: "ConfigProp", comp: 0, key: 1, typ: "any"},
	// ui/mobile's readers add the "mobile." prefix (ui/mobile/config.go).
	{pkg: "ui/mobile", name: "cfgBool", comp: 1, key: 2, prefix: "mobile.", typ: "bool"},
	{pkg: "ui/mobile", name: "cfgNum", comp: 1, key: 2, prefix: "mobile.", typ: "number"},
	{pkg: "ui/mobile", name: "cfgStr", comp: 1, key: 2, prefix: "mobile.", typ: "string"},
	{pkg: "ui/mobile", name: "cfgDur", comp: 1, key: 2, prefix: "mobile.", typ: "time.Duration|number"},
	{pkg: "ui/mobile", name: "cfgSize", comp: 1, key: -1, fixKey: "size", prefix: "mobile.", typ: "Size|ui.ControlSize|string"},
	{pkg: "ui/mobile", name: "cfgPosition", comp: 1, key: -1, fixKey: "position", prefix: "mobile.", typ: "Position|string"},
	{pkg: "ui/mobile", name: "cfgRound", comp: 1, key: -1, fixKey: "shape", prefix: "mobile.", typ: "string"},
}

// propLines type-checks a prop package from source (imports come from the
// export data) and returns its prop lines.
func propLines(m *module, p *pkgInfo, pp *platPkg) ([]string, error) {
	if !propPkgs[p.rel] {
		return nil, nil
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, name := range pp.files {
		f, err := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	info := &types.Info{
		Types:     map[ast.Expr]types.TypeAndValue{},
		Uses:      map[*ast.Ident]types.Object{},
		Instances: map[*ast.Ident]types.Instance{},
	}
	var firstErr error
	conf := types.Config{Importer: pp.imp, Error: func(err error) {
		if firstErr == nil {
			firstErr = err
		}
	}}
	tpkg, _ := conf.Check(p.path, fset, files, info)
	if firstErr != nil {
		return nil, fmt.Errorf("type-checking %s (%s) for prop lines: %v", p.path, pp.goos, firstErr)
	}
	pr := newPrinter(m, tpkg)
	var lines []string
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			id := calleeIdent(call.Fun)
			if id == nil {
				return true
			}
			fn, ok := info.Uses[id].(*types.Func)
			if !ok {
				return true
			}
			r := matchReader(m, fn.Origin())
			if r == nil {
				return true
			}
			comp, ok := stringArg(info, call.Args, r.comp)
			if !ok {
				return true
			}
			key := r.fixKey
			if r.key >= 0 {
				if key, ok = stringArg(info, call.Args, r.key); !ok {
					return true
				}
			}
			typ := r.typ
			if typ == "" {
				if inst, ok := info.Instances[id]; ok && inst.TypeArgs.Len() > 0 {
					typ = pr.typ(inst.TypeArgs.At(0))
				} else {
					typ = pr.typ(info.TypeOf(call))
				}
			}
			lines = append(lines, fmt.Sprintf("pkg %s, prop %q %q %s", p.path, r.prefix+comp, key, typ))
			return true
		})
	}
	return lines, nil
}

// calleeIdent returns the identifier naming the called function of
// f, g[T], pkg.F or x.M.
func calleeIdent(fun ast.Expr) *ast.Ident {
	fun = ast.Unparen(fun)
	switch f := fun.(type) {
	case *ast.IndexExpr:
		fun = f.X
	case *ast.IndexListExpr:
		fun = f.X
	}
	switch f := fun.(type) {
	case *ast.Ident:
		return f
	case *ast.SelectorExpr:
		return f.Sel
	}
	return nil
}

func matchReader(m *module, fn *types.Func) *propReader {
	if fn.Pkg() == nil || !m.isAPIPkg(fn.Pkg().Path()) {
		return nil
	}
	rel := relPath(m.path, fn.Pkg().Path())
	recv := ""
	if sig, ok := fn.Type().(*types.Signature); ok && sig.Recv() != nil {
		if n, ok := deref(sig.Recv().Type()).(*types.Named); ok {
			recv = n.Obj().Name()
		}
	}
	for i := range propReaders {
		r := &propReaders[i]
		if r.pkg == rel && r.recv == recv && r.name == fn.Name() {
			return r
		}
	}
	return nil
}

func stringArg(info *types.Info, args []ast.Expr, i int) (string, bool) {
	if i >= len(args) {
		return "", false
	}
	tv, ok := info.Types[args[i]]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(tv.Value), true
}
