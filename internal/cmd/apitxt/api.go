package main

import (
	"fmt"
	"go/constant"
	"go/types"
	"sort"
	"strconv"
	"strings"
)

// generate builds the golden file contents: file name -> sorted lines.
// Unclassified packages are generated too (the tier check reports them);
// excluded and main packages are skipped.
func generate(m *module, tiers *tierSet) (map[string][]string, error) {
	files := map[string][]string{}
	owner := map[string]string{}
	for _, p := range m.sortedPkgs() {
		if p.isMain {
			continue
		}
		if t, ok := tiers.lookup(p.rel); ok && t == tierExclude {
			continue
		}
		name := fileName(p)
		if o, dup := owner[name]; dup {
			return nil, fmt.Errorf("packages %s and %s map to the same file %s", o, p.path, name)
		}
		owner[name] = p.path
		perPlat := map[string]map[string]bool{}
		for _, pp := range m.platforms(p) {
			set := map[string]bool{}
			for _, l := range pkgLines(m, pp) {
				set[l] = true
			}
			props, err := propLines(m, p, pp)
			if err != nil {
				return nil, err
			}
			for _, l := range props {
				set[l] = true
			}
			perPlat[pp.goos] = set
		}
		files[name] = mergePlatforms(m.goos, perPlat)
	}
	return files, nil
}

// mergePlatforms unions the per-platform line sets. A line missing on
// some platform gets a suffix listing where it exists, e.g. " [darwin]".
func mergePlatforms(goos []string, perPlat map[string]map[string]bool) []string {
	plats := append([]string(nil), goos...)
	sort.Strings(plats)
	where := map[string][]string{}
	for _, g := range plats {
		for l := range perPlat[g] {
			where[l] = append(where[l], g)
		}
	}
	out := make([]string, 0, len(where))
	for l, ps := range where {
		if len(ps) == len(plats) {
			out = append(out, l)
		} else {
			out = append(out, l+" ["+strings.Join(ps, ",")+"]")
		}
	}
	sort.Strings(out)
	return out
}

// pkgLines returns the API lines of one package on one platform (without
// prop lines).
func pkgLines(m *module, pp *platPkg) []string {
	pr := newPrinter(m, pp.types)
	prefix := "pkg " + pp.types.Path() + ", "
	var out []string
	emit := func(format string, args ...any) {
		out = append(out, prefix+fmt.Sprintf(format, args...))
	}
	scope := pp.types.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		if !obj.Exported() {
			continue
		}
		switch obj := obj.(type) {
		case *types.Const:
			emit("const %s %s = %s", name, pr.typ(obj.Type()), constString(obj.Val()))
		case *types.Var:
			emit("var %s %s", name, pr.typ(obj.Type()))
		case *types.Func:
			emit("func %s%s", name, pr.sig(obj.Type().(*types.Signature), true))
		case *types.TypeName:
			pr.typeLines(obj, emit)
		}
	}
	return out
}

func constString(v constant.Value) string {
	switch v.Kind() {
	case constant.String:
		return strconv.Quote(constant.StringVal(v))
	case constant.Float:
		f, _ := constant.Float64Val(v)
		return strconv.FormatFloat(f, 'g', -1, 64)
	case constant.Complex:
		return v.String()
	default:
		return v.ExactString()
	}
}

// printer writes types relative to one package: its own names are
// unqualified, other module packages use the module-relative path
// ("ui/geom.Rect"; the root package by name, "tango.App"), everything else
// the full import path ("net/http.Handler"). A named type from another
// package that this package re-exports under an alias is written with the
// alias name (tango writes AppEventKind, not internal/platform.AppEventKind).
type printer struct {
	m       *module
	pkg     *types.Package
	aliases map[*types.TypeName]string
}

func newPrinter(m *module, pkg *types.Package) *printer {
	pr := &printer{m: m, pkg: pkg, aliases: map[*types.TypeName]string{}}
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		tn, ok := scope.Lookup(name).(*types.TypeName)
		if !ok || !tn.Exported() || !tn.IsAlias() {
			continue
		}
		n, ok := types.Unalias(tn.Type()).(*types.Named)
		if !ok || n.TypeArgs().Len() > 0 || n.Obj().Pkg() == nil || n.Obj().Pkg() == pkg {
			continue
		}
		if _, seen := pr.aliases[n.Obj()]; !seen {
			pr.aliases[n.Obj()] = name
		}
	}
	return pr
}

func (pr *printer) qualifier(p *types.Package) string {
	if p == nil || p == pr.pkg {
		return ""
	}
	path := p.Path()
	switch {
	case path == pr.m.path:
		return p.Name() + "."
	case strings.HasPrefix(path, pr.m.path+"/"):
		return path[len(pr.m.path)+1:] + "."
	}
	return path + "."
}

func (pr *printer) typ(t types.Type) string {
	var b strings.Builder
	pr.write(&b, t, true)
	return b.String()
}

// write appends t to b. subst enables the alias-name substitution.
func (pr *printer) write(b *strings.Builder, t types.Type, subst bool) {
	switch t := t.(type) {
	case nil:
		b.WriteString("<nil>")
	case *types.Basic:
		if t.Kind() == types.UnsafePointer {
			b.WriteString("unsafe.Pointer")
		} else {
			b.WriteString(t.Name())
		}
	case *types.Alias:
		obj := t.Obj()
		b.WriteString(pr.qualifier(obj.Pkg()))
		b.WriteString(obj.Name())
		pr.writeTypeArgs(b, t.TypeArgs())
	case *types.Named:
		obj := t.Obj()
		if a, ok := pr.aliases[obj]; ok && subst && t.TypeArgs().Len() == 0 {
			b.WriteString(a)
			return
		}
		b.WriteString(pr.qualifier(obj.Pkg()))
		b.WriteString(obj.Name())
		pr.writeTypeArgs(b, t.TypeArgs())
	case *types.TypeParam:
		b.WriteString(t.Obj().Name())
	case *types.Pointer:
		b.WriteByte('*')
		pr.write(b, t.Elem(), true)
	case *types.Slice:
		b.WriteString("[]")
		pr.write(b, t.Elem(), true)
	case *types.Array:
		fmt.Fprintf(b, "[%d]", t.Len())
		pr.write(b, t.Elem(), true)
	case *types.Map:
		b.WriteString("map[")
		pr.write(b, t.Key(), true)
		b.WriteByte(']')
		pr.write(b, t.Elem(), true)
	case *types.Chan:
		switch t.Dir() {
		case types.SendRecv:
			b.WriteString("chan ")
			if c, ok := t.Elem().(*types.Chan); ok && c.Dir() == types.RecvOnly {
				b.WriteByte('(')
				pr.write(b, t.Elem(), true)
				b.WriteByte(')')
				return
			}
		case types.SendOnly:
			b.WriteString("chan<- ")
		case types.RecvOnly:
			b.WriteString("<-chan ")
		}
		pr.write(b, t.Elem(), true)
	case *types.Signature:
		b.WriteString("func")
		b.WriteString(pr.sig(t, true))
	case *types.Struct:
		b.WriteString("struct{")
		for i := 0; i < t.NumFields(); i++ {
			if i > 0 {
				b.WriteString("; ")
			}
			f := t.Field(i)
			if !f.Embedded() {
				b.WriteString(f.Name())
				b.WriteByte(' ')
			}
			pr.write(b, f.Type(), true)
		}
		b.WriteByte('}')
	case *types.Interface:
		if t.IsImplicit() && t.NumEmbeddeds() == 1 {
			pr.write(b, t.EmbeddedType(0), true)
			return
		}
		if t.NumEmbeddeds() == 0 && t.NumExplicitMethods() == 0 {
			b.WriteString("interface{}")
			return
		}
		b.WriteString("interface{ ")
		first := true
		for i := 0; i < t.NumEmbeddeds(); i++ {
			if !first {
				b.WriteString("; ")
			}
			first = false
			pr.write(b, t.EmbeddedType(i), true)
		}
		for i := 0; i < t.NumExplicitMethods(); i++ {
			if !first {
				b.WriteString("; ")
			}
			first = false
			fn := t.ExplicitMethod(i)
			b.WriteString(fn.Name())
			b.WriteString(pr.sig(fn.Type().(*types.Signature), false))
		}
		b.WriteString(" }")
	case *types.Union:
		for i := 0; i < t.Len(); i++ {
			if i > 0 {
				b.WriteString(" | ")
			}
			term := t.Term(i)
			if term.Tilde() {
				b.WriteByte('~')
			}
			pr.write(b, term.Type(), true)
		}
	case *types.Tuple:
		b.WriteByte('(')
		for i := 0; i < t.Len(); i++ {
			if i > 0 {
				b.WriteString(", ")
			}
			pr.write(b, t.At(i).Type(), true)
		}
		b.WriteByte(')')
	default:
		b.WriteString(t.String())
	}
}

func (pr *printer) writeTypeArgs(b *strings.Builder, l *types.TypeList) {
	if l.Len() == 0 {
		return
	}
	b.WriteByte('[')
	for i := 0; i < l.Len(); i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		pr.write(b, l.At(i), true)
	}
	b.WriteByte(']')
}

func (pr *printer) tparams(l *types.TypeParamList) string {
	if l.Len() == 0 {
		return ""
	}
	parts := make([]string, l.Len())
	for i := 0; i < l.Len(); i++ {
		tp := l.At(i)
		parts[i] = tp.Obj().Name() + " " + pr.typ(tp.Constraint())
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// sig writes a signature without names: "[T any](int, ...string) (T, error)".
func (pr *printer) sig(s *types.Signature, withTParams bool) string {
	var b strings.Builder
	if withTParams {
		b.WriteString(pr.tparams(s.TypeParams()))
	}
	b.WriteByte('(')
	params := s.Params()
	for i := 0; i < params.Len(); i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		t := params.At(i).Type()
		if s.Variadic() && i == params.Len()-1 {
			b.WriteString("...")
			if sl, ok := t.(*types.Slice); ok {
				t = sl.Elem()
			}
		}
		pr.write(&b, t, true)
	}
	b.WriteByte(')')
	res := s.Results()
	switch res.Len() {
	case 0:
	case 1:
		b.WriteByte(' ')
		pr.write(&b, res.At(0).Type(), true)
	default:
		b.WriteByte(' ')
		pr.write(&b, res, true)
	}
	return b.String()
}

// typeLines emits the lines of an exported type name: the declaration,
// fields or interface methods, comparability and the method set. An alias
// whose target is a named type of an internal package of the module is
// expanded under the alias name, so changes to the internal type show up
// in the golden file (docs/specs/api-stability.md R8).
func (pr *printer) typeLines(obj *types.TypeName, emit func(string, ...any)) {
	name := obj.Name()
	if obj.IsAlias() {
		hdr := name
		var rhs types.Type = obj.Type()
		if a, ok := obj.Type().(*types.Alias); ok {
			hdr += pr.tparams(a.TypeParams())
			rhs = a.Rhs()
		}
		var b strings.Builder
		pr.write(&b, rhs, false)
		emit("type %s = %s", hdr, b.String())
		if n, ok := types.Unalias(obj.Type()).(*types.Named); ok && n.Obj().Pkg() != nil &&
			pr.m.isModuleInternal(n.Obj().Pkg().Path()) {
			pr.namedLines(name, name, name, n, emit)
		}
		return
	}
	n, ok := obj.Type().(*types.Named)
	if !ok {
		return
	}
	hdr, recv := name, name
	if tps := n.TypeParams(); tps.Len() > 0 {
		hdr += pr.tparams(tps)
		names := make([]string, tps.Len())
		for i := range names {
			names[i] = tps.At(i).Obj().Name()
		}
		recv += "[" + strings.Join(names, ", ") + "]"
	}
	pr.namedLines(name, hdr, recv, n, emit)
}

// namedLines emits the body of a named type under the given name, header
// (name plus type parameters) and receiver spelling.
func (pr *printer) namedLines(name, hdr, recv string, n *types.Named, emit func(string, ...any)) {
	switch u := n.Underlying().(type) {
	case *types.Struct:
		emit("type %s struct", hdr)
		pr.structFields(n, u, func(field string) {
			emit("type %s struct, %s", name, field)
		})
		if !types.Comparable(n) {
			emit("type %s struct, noncomparable", name)
		}
	case *types.Interface:
		var names []string
		unexported := false
		for i := 0; i < u.NumMethods(); i++ {
			fn := u.Method(i)
			if !fn.Exported() {
				unexported = true
				continue
			}
			names = append(names, fn.Name())
			emit("type %s interface, %s%s", name, fn.Name(), pr.sig(fn.Type().(*types.Signature), false))
		}
		if unexported {
			names = append(names, "unexported methods")
		}
		for i := 0; i < u.NumEmbeddeds(); i++ {
			if _, isIface := u.EmbeddedType(i).Underlying().(*types.Interface); !isIface {
				emit("type %s interface, %s", name, pr.typ(u.EmbeddedType(i)))
			}
		}
		emit("type %s interface { %s }", hdr, strings.Join(names, ", "))
		return
	default:
		emit("type %s %s", hdr, pr.typ(u))
	}
	if _, isPtr := n.Underlying().(*types.Pointer); isPtr {
		return
	}
	msP := types.NewMethodSet(types.NewPointer(n))
	msV := types.NewMethodSet(n)
	for i := 0; i < msP.Len(); i++ {
		sel := msP.At(i)
		fn := sel.Obj().(*types.Func)
		if !fn.Exported() {
			continue
		}
		if len(sel.Index()) > 1 && !pr.hiddenPath(n, sel.Index()) {
			continue // promoted through a visible embedded field: listed under that type
		}
		r := "*" + recv
		if msV.Lookup(fn.Pkg(), fn.Name()) != nil {
			r = recv
		}
		sig, _ := sel.Type().(*types.Signature)
		if sig == nil {
			sig = fn.Type().(*types.Signature)
		}
		emit("method (%s) %s%s", r, fn.Name(), pr.sig(sig, false))
	}
}

// structFields lists the exported fields of a struct ("Name Type"
// followed by the struct tag, if any) and its embedded fields ("embedded
// T"). Fields promoted through a hidden embedded field (unexported, or of
// an internal package type) are listed as own fields because nothing else
// documents them.
func (pr *printer) structFields(top types.Type, st *types.Struct, emit func(string)) {
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if f.Embedded() {
			if f.Exported() {
				emit("embedded " + pr.typ(f.Type()))
			}
			if !f.Exported() || pr.m.isModuleInternal(typePkgPath(f.Type())) {
				pr.promotedFields(top, f.Type(), map[types.Type]bool{}, emit)
			}
			continue
		}
		if f.Exported() {
			emit(f.Name() + " " + pr.typ(f.Type()) + tagSuffix(st.Tag(i)))
		}
	}
}

func (pr *printer) promotedFields(top, emb types.Type, seen map[types.Type]bool, emit func(string)) {
	t := deref(emb)
	if seen[t] {
		return
	}
	seen[t] = true
	st, ok := t.Underlying().(*types.Struct)
	if !ok {
		return
	}
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if f.Exported() {
			if obj, _, _ := types.LookupFieldOrMethod(top, true, pr.pkg, f.Name()); obj == f {
				emit(f.Name() + " " + pr.typ(f.Type()) + tagSuffix(st.Tag(i)))
			}
		}
		if f.Embedded() {
			pr.promotedFields(top, f.Type(), seen, emit)
		}
	}
}

// hiddenPath reports whether a promoted selection passes through an
// embedded field that is unexported or has an internal package type.
func (pr *printer) hiddenPath(t types.Type, index []int) bool {
	for _, idx := range index[:len(index)-1] {
		st, ok := deref(t).Underlying().(*types.Struct)
		if !ok {
			return false
		}
		f := st.Field(idx)
		if !f.Exported() || pr.m.isModuleInternal(typePkgPath(f.Type())) {
			return true
		}
		t = f.Type()
	}
	return false
}

func tagSuffix(tag string) string {
	if tag == "" {
		return ""
	}
	if !strings.Contains(tag, "`") {
		return " `" + tag + "`"
	}
	return " " + strconv.Quote(tag)
}

func deref(t types.Type) types.Type {
	t = types.Unalias(t)
	if p, ok := t.(*types.Pointer); ok {
		return types.Unalias(p.Elem())
	}
	return t
}

// typePkgPath returns the package path of a (pointer to a) named type.
func typePkgPath(t types.Type) string {
	if n, ok := deref(t).(*types.Named); ok && n.Obj().Pkg() != nil {
		return n.Obj().Pkg().Path()
	}
	return ""
}
