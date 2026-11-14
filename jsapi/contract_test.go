package jsapi

// Wire contract between this package and runtime-js (@tangoui/runtime):
// every method, event and error code the runtime API puts on the wire is
// listed in api/jsapi-methods.txt. This test owns the Go columns (kind,
// name, cap) and derives them from the registration code itself:
//
//   - methods: api.Handle(cap, name, fn) calls,
//   - events: api.Emit(s, name, data) and api.Broadcast(name, data) calls,
//   - errors: the Code* string constants of package ipc and this package,
//     and string literals passed to ipc.NewError here.
//
// Names built by helpers (h := func(name string, ...) { api.Handle(Window,
// "window."+name, fn) }, api.launchEvent("openURL", ...)) are resolved by
// evaluating the helper at each call site with the literal arguments. A
// registration whose name cannot be resolved to a constant string fails
// the test instead of being skipped. The extracted method list is then
// cross-checked against a real Install (Router.Has).
//
// runtime-js/test/contract.test.mjs owns the js= column: which @tangoui/
// runtime functions use each name. Regenerate after changing jsapi:
//
//	go test ./jsapi -run TestWireContract -update
//	cd runtime-js && npm run contract:update
//
// This is a structural check of names only: argument and result shapes,
// capabilities enforced at run time and behaviour are covered by the
// other tests, not by this file.

import (
	"flag"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const contractFile = "../api/jsapi-methods.txt"

const contractHeader = `# Wire contract of the Tango runtime API: Go package jsapi <-> runtime-js
# (@tangoui/runtime). Every name below is part of the public API: renaming
# or removing one breaks pages, and every change shows up in review.
#
# Generated; do not edit by hand.
#   kind, name, cap: go test ./jsapi -run TestWireContract -update
#                    (jsapi/contract_test.go, from the registration code)
#   js=:             cd runtime-js && npm run contract:update
#                    (runtime-js/test/contract.test.mjs, from src/api/*.ts)
#
# Lines:
#   error <code> js=<ErrorCode|->        error codes a runtime API call can
#                                        reject with; js=ErrorCode when the
#                                        TS ErrorCode union names it
#   method tango:<module>.<fn> cap=<Cap> js=<Namespace.function,...|->
#   event tango:<module>.<event> js=<Namespace.function,...|->
# js=- means runtime-js has no wrapper (pages use call()/on() directly).
#
# Not covered here (see docs/runtime-api.md "API 稳定性"): app-defined
# names (call/on/emit/stream, Events.*), methods other packages register
# with API.Handle, the uivars page global and DOM event used by
# Theme.vars/onVarsChange, and argument/result JSON shapes.
`

var updateContract = flag.Bool("update", false, "rewrite "+contractFile)

type wireEntry struct {
	kind string // "error", "method", "event"
	name string // code, or full wire name with Prefix
	cap  string // methods only
}

func (e wireEntry) key() string { return e.kind + " " + e.name }

// ---- extraction ----

type goFunc struct {
	params []string
	body   *ast.BlockStmt
	name   string
}

type extractor struct {
	fset    *token.FileSet
	methods map[string]*goFunc // API methods by name
	out     map[string]wireEntry
	visited map[ast.Node]bool // sink calls evaluated at least once
	sinks   []*ast.CallExpr   // every sink call in the package
	errs    []string
}

func sinkKind(call *ast.CallExpr) (kind string, nameArg int) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", 0
	}
	if id, ok := sel.X.(*ast.Ident); !ok || id.Name != "api" {
		return "", 0
	}
	switch sel.Sel.Name {
	case "Handle":
		if len(call.Args) == 3 {
			return "method", 1
		}
	case "Emit":
		if len(call.Args) == 3 {
			return "event", 1
		}
	case "Broadcast":
		if len(call.Args) == 2 {
			return "event", 0
		}
	}
	return "", 0
}

// isSinkDef reports whether a method declaration is one of the sinks
// themselves (their bodies forward a parameter and are not registrations).
func isSinkDef(name string) bool {
	return name == "Handle" || name == "Emit" || name == "Broadcast"
}

func paramNames(ft *ast.FuncType) []string {
	var out []string
	for _, f := range ft.Params.List {
		if len(f.Names) == 0 {
			out = append(out, "_")
		}
		for _, n := range f.Names {
			out = append(out, n.Name)
		}
	}
	return out
}

func exprText(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return exprText(x.X) + "." + x.Sel.Name
	case *ast.BinaryExpr:
		return exprText(x.X) + x.Op.String() + exprText(x.Y)
	case *ast.ParenExpr:
		return exprText(x.X)
	}
	return "?"
}

// eval resolves a constant string expression under env.
func eval(e ast.Expr, env map[string]string) (string, bool) {
	switch x := e.(type) {
	case *ast.BasicLit:
		if x.Kind == token.STRING {
			s, err := strconv.Unquote(x.Value)
			return s, err == nil
		}
	case *ast.Ident:
		v, ok := env[x.Name]
		return v, ok
	case *ast.ParenExpr:
		return eval(x.X, env)
	case *ast.BinaryExpr:
		if x.Op == token.ADD {
			a, ok1 := eval(x.X, env)
			b, ok2 := eval(x.Y, env)
			return a + b, ok1 && ok2
		}
	}
	return "", false
}

// bind evaluates call arguments for a helper's parameters: constant
// strings by value, identifiers (capabilities) by name.
func bind(params []string, args []ast.Expr, env map[string]string) map[string]string {
	next := map[string]string{}
	for i, p := range params {
		if i >= len(args) {
			break
		}
		if v, ok := eval(args[i], env); ok {
			next[p] = v
		} else if id, ok := args[i].(*ast.Ident); ok && env[id.Name] != "" {
			next[p] = env[id.Name]
		} else {
			next[p] = "\x00" + exprText(args[i]) // not a string; kept for cap
		}
	}
	return next
}

func (x *extractor) pos(n ast.Node) string {
	p := x.fset.Position(n.Pos())
	return filepath.Base(p.Filename) + ":" + strconv.Itoa(p.Line)
}

// walk evaluates every sink reachable from body with env; locals holds
// helper closures defined in enclosing scopes.
func (x *extractor) walk(body ast.Node, env map[string]string, locals map[string]*goFunc, depth int) {
	if depth > 8 {
		x.errs = append(x.errs, x.pos(body)+": helper nesting too deep")
		return
	}
	locals = copyLocals(locals)
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			// h := func(name string, ...) { ... }: a helper, evaluated at
			// its call sites rather than here.
			if len(n.Lhs) == 1 && len(n.Rhs) == 1 {
				if id, ok := n.Lhs[0].(*ast.Ident); ok {
					if fl, ok := n.Rhs[0].(*ast.FuncLit); ok && containsSink(fl.Body) {
						locals[id.Name] = &goFunc{params: paramNames(fl.Type), body: fl.Body, name: id.Name}
						return false
					}
				}
			}
		case *ast.CallExpr:
			if kind, ai := sinkKind(n); kind != "" {
				x.visited[n] = true
				name, ok := eval(n.Args[ai], env)
				if !ok {
					x.errs = append(x.errs, x.pos(n)+": cannot resolve "+kind+" name "+exprText(n.Args[ai]))
					return true
				}
				e := wireEntry{kind: kind, name: Prefix + name}
				if kind == "method" {
					e.cap = exprText(n.Args[0])
					if id, ok := n.Args[0].(*ast.Ident); ok {
						if v, ok := env[id.Name]; ok {
							e.cap = strings.TrimPrefix(v, "\x00")
						}
					}
				}
				if old, dup := x.out[e.key()]; dup && old.cap != e.cap {
					x.errs = append(x.errs, x.pos(n)+": "+e.name+" registered with caps "+old.cap+" and "+e.cap)
				}
				x.out[e.key()] = e
				return true
			}
			var f *goFunc
			switch fn := n.Fun.(type) {
			case *ast.Ident:
				f = locals[fn.Name]
			case *ast.SelectorExpr:
				if id, ok := fn.X.(*ast.Ident); ok && id.Name == "api" && !isSinkDef(fn.Sel.Name) {
					f = x.methods[fn.Sel.Name]
				}
			}
			if f != nil && containsSink(f.body) {
				x.walk(f.body, bind(f.params, n.Args, env), locals, depth+1)
			}
		}
		return true
	})
}

func copyLocals(m map[string]*goFunc) map[string]*goFunc {
	out := make(map[string]*goFunc, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func containsSink(n ast.Node) bool {
	found := false
	ast.Inspect(n, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok {
			if k, _ := sinkKind(c); k != "" {
				found = true
			}
		}
		return !found
	})
	return found
}

// nameParams returns, for every function, the indexes of the parameters
// its registrations take their name or capability from (directly or via
// another such helper). A function with any is a helper, only meaningful
// at its call sites.
func nameParams(funcs []*goFunc, methods map[string]*goFunc) map[*goFunc]map[int]bool {
	res := map[*goFunc]map[int]bool{}
	for changed := true; changed; {
		changed = false
		for _, f := range funcs {
			idx := map[string]int{}
			for i, p := range f.params {
				idx[p] = i
			}
			mark := func(e ast.Expr) {
				ast.Inspect(e, func(n ast.Node) bool {
					if id, ok := n.(*ast.Ident); ok {
						if i, ok := idx[id.Name]; ok && !res[f][i] {
							if res[f] == nil {
								res[f] = map[int]bool{}
							}
							res[f][i] = true
							changed = true
						}
					}
					return true
				})
			}
			ast.Inspect(f.body, func(n ast.Node) bool {
				c, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if k, ai := sinkKind(c); k != "" {
					mark(c.Args[ai])
					if k == "method" {
						mark(c.Args[0])
					}
				} else if sel, ok := c.Fun.(*ast.SelectorExpr); ok && exprText(sel.X) == "api" {
					if m := methods[sel.Sel.Name]; m != nil && m != f {
						for i := range res[m] {
							if i < len(c.Args) {
								mark(c.Args[i])
							}
						}
					}
				}
				return true
			})
		}
	}
	return res
}

func parseDir(t *testing.T, fset *token.FileSet, dir string) []*ast.File {
	t.Helper()
	names, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	var files []*ast.File
	for _, n := range names {
		if strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, n, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	return files
}

// codeConsts returns the values of string constants named Code*.
func codeConsts(files []*ast.File) []string {
	var out []string
	for _, f := range files {
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, s := range gd.Specs {
				vs := s.(*ast.ValueSpec)
				for i, n := range vs.Names {
					if !strings.HasPrefix(n.Name, "Code") || i >= len(vs.Values) {
						continue
					}
					if v, ok := eval(vs.Values[i], nil); ok {
						out = append(out, v)
					}
				}
			}
		}
	}
	return out
}

func extractContract(t *testing.T) map[string]wireEntry {
	t.Helper()
	out, errs := extractFrom(t, ".", "../ipc")
	for _, e := range errs {
		t.Error(e)
	}
	return out
}

// extractFrom extracts the contract of the jsapi sources in dir, with the
// ipc error codes of ipcDir. errs lists names it could not resolve.
func extractFrom(t *testing.T, dir, ipcDir string) (out map[string]wireEntry, errs []string) {
	t.Helper()
	fset := token.NewFileSet()
	files := parseDir(t, fset, dir)
	x := &extractor{fset: fset, methods: map[string]*goFunc{}, out: map[string]wireEntry{}, visited: map[ast.Node]bool{}}

	var decls []*goFunc
	sinkDefs := map[ast.Node]bool{} // bodies of Handle, Emit and Broadcast
	for _, f := range files {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			g := &goFunc{params: paramNames(fd.Type), body: fd.Body, name: fd.Name.Name}
			if fd.Recv != nil {
				if isSinkDef(fd.Name.Name) {
					sinkDefs[fd.Body] = true
					continue
				}
				x.methods[fd.Name.Name] = g
			}
			decls = append(decls, g)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if sinkDefs[n] {
				return false
			}
			if c, ok := n.(*ast.CallExpr); ok {
				if k, _ := sinkKind(c); k != "" {
					x.sinks = append(x.sinks, c)
				}
			}
			return true
		})
	}
	// Roots: every function whose registrations do not depend on its own
	// parameters; helpers are evaluated from their call sites.
	helpers := nameParams(decls, x.methods)
	for _, g := range decls {
		if len(helpers[g]) == 0 {
			x.walk(g.body, map[string]string{}, nil, 0)
		}
	}
	for _, c := range x.sinks {
		// The bodies of Handle/Emit/Broadcast were never collected; any
		// other sink not reached means a helper nobody calls with
		// constants, or a pattern this extractor does not understand.
		if !x.visited[c] {
			x.errs = append(x.errs, x.pos(c)+": registration not reached from any call site with constant names")
		}
	}
	codes := codeConsts(files)
	codes = append(codes, codeConsts(parseDir(t, fset, ipcDir))...)
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			c, ok := n.(*ast.CallExpr)
			if !ok || len(c.Args) != 2 {
				return true
			}
			if sel, ok := c.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "NewError" && exprText(sel.X) == "ipc" {
				if v, ok := eval(c.Args[0], nil); ok {
					codes = append(codes, v)
				}
			}
			return true
		})
	}
	for _, c := range codes {
		e := wireEntry{kind: "error", name: c}
		x.out[e.key()] = e
	}
	return x.out, x.errs
}

// ---- the golden file ----

type contractLine struct {
	wireEntry
	js string
}

func parseContract(t *testing.T, data string) map[string]contractLine {
	t.Helper()
	out := map[string]contractLine{}
	for i, line := range strings.Split(data, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 3 {
			t.Fatalf("%s:%d: malformed line %q", contractFile, i+1, line)
		}
		l := contractLine{wireEntry: wireEntry{kind: f[0], name: f[1]}}
		for _, kv := range f[2:] {
			k, v, _ := strings.Cut(kv, "=")
			switch k {
			case "cap":
				l.cap = v
			case "js":
				l.js = v
			default:
				t.Fatalf("%s:%d: unknown field %q", contractFile, i+1, kv)
			}
		}
		out[l.key()] = l
	}
	return out
}

var kindOrder = map[string]int{"error": 0, "method": 1, "event": 2}

func formatContract(entries map[string]wireEntry, old map[string]contractLine) string {
	var lines []contractLine
	for k, e := range entries {
		js := "?" // filled in by npm run contract:update
		if o, ok := old[k]; ok && o.js != "" {
			js = o.js
		}
		lines = append(lines, contractLine{e, js})
	}
	sort.Slice(lines, func(i, j int) bool {
		a, b := lines[i], lines[j]
		if a.kind != b.kind {
			return kindOrder[a.kind] < kindOrder[b.kind]
		}
		return a.name < b.name
	})
	var b strings.Builder
	b.WriteString(contractHeader)
	prev := ""
	for _, l := range lines {
		if l.kind != prev {
			b.WriteString("\n")
			prev = l.kind
		}
		b.WriteString(l.kind + " " + l.name)
		if l.kind == "method" {
			b.WriteString(" cap=" + l.cap)
		}
		b.WriteString(" js=" + l.js + "\n")
	}
	return b.String()
}

func TestWireContract(t *testing.T) {
	got := extractContract(t)
	if t.Failed() {
		t.FailNow()
	}
	raw, err := os.ReadFile(contractFile)
	if err != nil && !(*updateContract && os.IsNotExist(err)) {
		t.Fatalf("%v (create it with: go test ./jsapi -run TestWireContract -update)", err)
	}
	old := parseContract(t, string(raw))
	want := formatContract(got, old)

	if *updateContract {
		if string(raw) != want {
			if err := os.WriteFile(contractFile, []byte(want), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Logf("updated %s; now run: cd runtime-js && npm run contract:update", contractFile)
		}
		return
	}
	// Compare only the Go-owned columns; js= belongs to runtime-js.
	var problems []string
	for k, e := range got {
		o, ok := old[k]
		switch {
		case !ok:
			problems = append(problems, "missing from the contract: "+e.kind+" "+e.name)
		case o.cap != e.cap:
			problems = append(problems, e.name+": cap is "+e.cap+" in jsapi, "+o.cap+" in the contract")
		}
	}
	for k, o := range old {
		if _, ok := got[k]; !ok {
			problems = append(problems, "no longer in jsapi: "+o.kind+" "+o.name)
		}
	}
	if len(problems) == 0 && string(raw) != want {
		problems = append(problems, "file is not in canonical form")
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		t.Errorf("%s is out of date:\n  %s\nIf the change is intended: go test ./jsapi -run TestWireContract -update, then cd runtime-js && npm run contract:update, and describe it in CHANGELOG.md",
			contractFile, strings.Join(problems, "\n  "))
	}
}

// TestWireContractRegistered cross-checks the statically extracted method
// names with what Install (and InstallUpdater) really register.
func TestWireContractRegistered(t *testing.T) {
	got := extractContract(t)
	e := setup(t, Options{})
	e.api.InstallUpdater(updateServer(t), UpdaterOptions{})
	n := 0
	for _, w := range got {
		if w.kind != "method" {
			continue
		}
		n++
		if !e.r.Has(w.name) {
			t.Errorf("%s was extracted from the source but is not registered by Install/InstallUpdater", w.name)
		}
	}
	if n == 0 {
		t.Fatal("no methods extracted")
	}
	if e.r.Has(Prefix + "window.noSuchMethod") {
		t.Fatal("Router.Has sanity check failed")
	}
}

// TestWireContractDetectsDrift runs the extractor on edited copies of the
// sources: a renamed registration and a name the extractor cannot resolve
// must both be noticed.
func TestWireContractDetectsDrift(t *testing.T) {
	copySrc := func(edit func(name, src string) string) string {
		dir := t.TempDir()
		names, _ := filepath.Glob("*.go")
		for _, n := range names {
			if strings.HasSuffix(n, "_test.go") {
				continue
			}
			b, err := os.ReadFile(n)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, n), []byte(edit(n, string(b))), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}
	base, errs := extractFrom(t, ".", "../ipc")
	if len(errs) > 0 {
		t.Fatal(errs)
	}

	dir := copySrc(func(name, src string) string {
		if name == "window.go" {
			src = strings.Replace(src, `h("setTitle",`, `h("setCaption",`, 1)
		}
		if name == "app.go" {
			src = strings.Replace(src, `api.launchEvent("reopen", nil)`, `api.launchEvent("reOpen", nil)`, 1)
		}
		return src
	})
	got, errs := extractFrom(t, dir, "../ipc")
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	for _, k := range []string{"method tango:window.setTitle", "event tango:app.reopen"} {
		if _, ok := base[k]; !ok {
			t.Fatalf("baseline lacks %s", k)
		}
		if _, ok := got[k]; ok {
			t.Errorf("%s still extracted after renaming it", k)
		}
	}
	for _, k := range []string{"method tango:window.setCaption", "event tango:app.reOpen"} {
		if _, ok := got[k]; !ok {
			t.Errorf("%s not extracted after renaming", k)
		}
	}

	dir = copySrc(func(name, src string) string {
		if name == "system.go" {
			src = strings.Replace(src, `api.Handle(Dialogs, "dialog.save",`, `api.Handle(Dialogs, dialogSaveName(),`, 1)
			src += "\nfunc dialogSaveName() string { return \"dialog.save\" }\n"
		}
		return src
	})
	if _, errs := extractFrom(t, dir, "../ipc"); len(errs) == 0 {
		t.Error("a registration with a non-constant name was not reported")
	}
}
