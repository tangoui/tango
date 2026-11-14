package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"sort"
	"strings"
)

// checkResult collects the static checks of docs/specs/api-stability.md
// §4.3. Errors always fail; warnings fail only with -strict (Phase 0 keeps
// the doc and leak checks as warnings until Phase 2 cleans them up).
type checkResult struct {
	errors      []string
	docWarnings []string // packages whose doc lacks "Experimental"/"实验性"
	leaks       []string // stable symbols that reference non-stable ones
}

func runChecks(m *module, tiers *tierSet, allow *allowlists) checkResult {
	var res checkResult
	res.errors = append(res.errors, allow.problems...)
	pkgs := m.sortedPkgs()

	// Every public package must be classified.
	for _, p := range pkgs {
		if _, ok := tiers.lookup(p.rel); !ok {
			res.errors = append(res.errors, fmt.Sprintf("package %s is not classified in api/tiers.txt (stable, experimental, split or exclude)", displayRel(p.rel)))
		}
	}

	// Allowlist entries must name existing exported symbols.
	for _, p := range pkgs {
		a := allow.byPkg[p.path]
		if a == nil {
			continue
		}
		for _, e := range a.entries {
			if !symbolExists(m, p, e.text) {
				res.errors = append(res.errors, fmt.Sprintf("api/stable/%s:%d: %s is not an exported symbol of %s", fileName(p), e.line, e.text, displayRel(p.rel)))
			}
		}
	}

	// Experimental and split packages must say so in the package doc.
	for _, p := range pkgs {
		t, _ := tiers.lookup(p.rel)
		if p.isMain || (t != tierExperimental && t != tierSplit) {
			continue
		}
		doc := packageDoc(m, p)
		if !strings.Contains(doc, "Experimental") && !strings.Contains(doc, "实验性") {
			res.docWarnings = append(res.docWarnings, fmt.Sprintf("%s (%s)", displayRel(p.rel), t))
		}
	}

	res.leaks = leaks(m, tiers, allow)
	return res
}

// report prints the result and reports whether the run must fail.
func (r checkResult) report(cfg config) bool {
	w := cfg.errOut
	for _, e := range r.errors {
		fmt.Fprintln(w, "apitxt: error:", e)
	}
	failed := len(r.errors) > 0
	level := "warning"
	if cfg.strict {
		level = "error"
		failed = failed || len(r.docWarnings) > 0 || len(r.leaks) > 0
	}
	if n := len(r.docWarnings); n > 0 {
		fmt.Fprintf(w, "apitxt: %s: %d experimental/split packages lack \"Experimental\" or \"实验性\" in the package doc:\n", level, n)
		fmt.Fprintf(w, "  %s\n", strings.Join(r.docWarnings, ", "))
	}
	if n := len(r.leaks); n > 0 {
		fmt.Fprintf(w, "apitxt: %s: %d cross-tier references from stable symbols:\n", level, n)
		const maxShown = 40
		for i, l := range r.leaks {
			if i == maxShown && !cfg.verbose {
				fmt.Fprintf(w, "  ... %d more (-v lists all)\n", n-maxShown)
				break
			}
			fmt.Fprintln(w, " ", l)
		}
	}
	if !cfg.strict && (len(r.docWarnings) > 0 || len(r.leaks) > 0) {
		fmt.Fprintln(w, "apitxt: warnings do not fail the check yet (Phase 0); -strict turns them into errors")
		if os.Getenv("GITHUB_ACTIONS") != "" {
			fmt.Fprintf(w, "::warning::apitxt: %d packages lack an Experimental package doc, %d cross-tier references (see the log)\n", len(r.docWarnings), len(r.leaks))
		}
	}
	return failed
}

// symbolExists reports whether "Name" or "Type.Member" is an exported
// symbol of p on any platform.
func symbolExists(m *module, p *pkgInfo, sym string) bool {
	top, member, _ := strings.Cut(sym, ".")
	for _, pp := range m.platforms(p) {
		obj := pp.types.Scope().Lookup(top)
		if obj == nil || !obj.Exported() {
			continue
		}
		if member == "" {
			return true
		}
		if _, ok := obj.(*types.TypeName); !ok || !token.IsExported(member) {
			continue
		}
		if o, _, _ := types.LookupFieldOrMethod(obj.Type(), true, pp.types, member); o != nil {
			return true
		}
	}
	return false
}

// packageDoc returns the package comments of p's files on all platforms.
func packageDoc(m *module, p *pkgInfo) string {
	seen := map[string]bool{}
	var b strings.Builder
	fset := token.NewFileSet()
	for _, pp := range m.platforms(p) {
		for _, name := range pp.files {
			if seen[name] {
				continue
			}
			seen[name] = true
			f, err := parser.ParseFile(fset, name, nil, parser.PackageClauseOnly|parser.ParseComments)
			if err == nil && f.Doc != nil {
				b.WriteString(f.Doc.Text())
			}
		}
	}
	return b.String()
}

// leaks lists references from stable symbols (all exported symbols of
// stable packages, allowlisted symbols of split packages) to symbols of
// experimental or excluded packages and to non-allowlisted symbols of split
// packages. Standard library, third-party and internal packages are not
// tiers; internal types re-exported through an alias are followed into
// their fields and methods, as the golden files expand them.
func leaks(m *module, tiers *tierSet, allow *allowlists) []string {
	found := map[string]bool{}
	for _, p := range m.sortedPkgs() {
		t, _ := tiers.lookup(p.rel)
		if p.isMain || (t != tierStable && t != tierSplit) {
			continue
		}
		a := allow.byPkg[p.path]
		for _, pp := range m.platforms(p) {
			scope := pp.types.Scope()
			for _, name := range scope.Names() {
				obj := scope.Lookup(name)
				if !obj.Exported() {
					continue
				}
				from := qualName(p.rel, p.name, name)
				lc := &leakWalker{m: m, tiers: tiers, allow: allow, from: from, found: found, seen: map[types.Type]bool{}}
				if t == tierStable || a.hasName(name) {
					lc.object(obj)
					continue
				}
				// Allowlisted members of a type that is not whole-stable.
				if tn, ok := obj.(*types.TypeName); ok && a != nil {
					for k := range a.members {
						top, member, _ := strings.Cut(k, ".")
						if top != name {
							continue
						}
						if o, _, _ := types.LookupFieldOrMethod(tn.Type(), true, pp.types, member); o != nil {
							lc.from = from + "." + member
							lc.typ(o.Type())
						}
					}
				}
			}
		}
	}
	out := make([]string, 0, len(found))
	for l := range found {
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}

type leakWalker struct {
	m     *module
	tiers *tierSet
	allow *allowlists
	from  string
	found map[string]bool
	seen  map[types.Type]bool
}

func (w *leakWalker) object(obj types.Object) {
	switch obj := obj.(type) {
	case *types.Const, *types.Var, *types.Func:
		w.typ(obj.Type())
	case *types.TypeName:
		if obj.IsAlias() {
			rhs := obj.Type()
			if a, ok := rhs.(*types.Alias); ok {
				w.tparams(a.TypeParams())
				rhs = a.Rhs()
			}
			w.typ(rhs)
			if n, ok := types.Unalias(obj.Type()).(*types.Named); ok && n.Obj().Pkg() != nil &&
				w.m.isModuleInternal(n.Obj().Pkg().Path()) {
				w.members(n)
			}
			return
		}
		if n, ok := obj.Type().(*types.Named); ok {
			w.tparams(n.TypeParams())
			w.members(n)
		}
	}
}

// members walks the exported surface of a named type.
func (w *leakWalker) members(n *types.Named) {
	if w.seen[n] {
		return
	}
	w.seen[n] = true
	switch u := n.Underlying().(type) {
	case *types.Struct:
		for i := 0; i < u.NumFields(); i++ {
			f := u.Field(i)
			hidden := !f.Exported() || w.m.isModuleInternal(typePkgPath(f.Type()))
			switch {
			case f.Embedded() && hidden:
				if en, ok := deref(f.Type()).(*types.Named); ok {
					w.members(en)
				} else {
					w.typ(f.Type())
				}
			case f.Exported():
				w.typ(f.Type())
			}
		}
	case *types.Interface:
		for i := 0; i < u.NumMethods(); i++ {
			if fn := u.Method(i); fn.Exported() {
				w.typ(fn.Type())
			}
		}
		for i := 0; i < u.NumEmbeddeds(); i++ {
			if _, isIface := u.EmbeddedType(i).Underlying().(*types.Interface); !isIface {
				w.typ(u.EmbeddedType(i))
			}
		}
		return
	default:
		w.typ(u)
	}
	ms := types.NewMethodSet(types.NewPointer(n))
	for i := 0; i < ms.Len(); i++ {
		sel := ms.At(i)
		if !sel.Obj().Exported() || len(sel.Index()) > 1 {
			continue // promoted methods are covered by the embedded type
		}
		w.typ(sel.Type())
	}
}

func (w *leakWalker) tparams(l *types.TypeParamList) {
	for i := 0; i < l.Len(); i++ {
		w.typ(l.At(i).Constraint())
	}
}

// typ walks a type expression, checking each named type it mentions
// without descending into it.
func (w *leakWalker) typ(t types.Type) {
	switch t := t.(type) {
	case *types.Named:
		w.ref(t.Obj())
		for i := 0; i < t.TypeArgs().Len(); i++ {
			w.typ(t.TypeArgs().At(i))
		}
	case *types.Alias:
		w.ref(t.Obj())
		for i := 0; i < t.TypeArgs().Len(); i++ {
			w.typ(t.TypeArgs().At(i))
		}
	case *types.Pointer:
		w.typ(t.Elem())
	case *types.Slice:
		w.typ(t.Elem())
	case *types.Array:
		w.typ(t.Elem())
	case *types.Chan:
		w.typ(t.Elem())
	case *types.Map:
		w.typ(t.Key())
		w.typ(t.Elem())
	case *types.Signature:
		w.tparams(t.TypeParams())
		w.typ(t.Params())
		w.typ(t.Results())
	case *types.Tuple:
		for i := 0; i < t.Len(); i++ {
			w.typ(t.At(i).Type())
		}
	case *types.Struct:
		for i := 0; i < t.NumFields(); i++ {
			w.typ(t.Field(i).Type())
		}
	case *types.Interface:
		if w.seen[t] {
			return
		}
		w.seen[t] = true
		for i := 0; i < t.NumEmbeddeds(); i++ {
			w.typ(t.EmbeddedType(i))
		}
		for i := 0; i < t.NumExplicitMethods(); i++ {
			w.typ(t.ExplicitMethod(i).Type())
		}
	case *types.Union:
		for i := 0; i < t.Len(); i++ {
			w.typ(t.Term(i).Type())
		}
	}
}

// ref checks one referenced type name.
func (w *leakWalker) ref(obj *types.TypeName) {
	pkg := obj.Pkg()
	if pkg == nil || !w.m.isAPIPkg(pkg.Path()) {
		return
	}
	rel := relPath(w.m.path, pkg.Path())
	t, ok := w.tiers.lookup(rel)
	var why string
	switch {
	case !ok:
		why = "unclassified package"
	case t == tierStable:
		return
	case t == tierSplit:
		if w.allow.byPkg[pkg.Path()].covers(obj.Name(), "") {
			return
		}
		why = "not in api/stable/" + fileNameRel(rel, pkg.Name())
	default:
		why = t
	}
	w.found[fmt.Sprintf("%s -> %s (%s)", w.from, qualName(rel, pkg.Name(), obj.Name()), why)] = true
}

func fileNameRel(rel, name string) string {
	return fileName(&pkgInfo{rel: rel, name: name})
}

// qualName writes a symbol for messages: "ui/theme.Theme"; root package
// symbols use the package name ("tango.App").
func qualName(rel, pkgName, name string) string {
	if rel == "" {
		return pkgName + "." + name
	}
	return rel + "." + name
}
