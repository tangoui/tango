// Package msl generates Metal Shading Language source from a shader ir.Module.
//
// # Interface mapping
//
// The entry points are always named vs_main (vertex) and fragment fs_main
// (fragment). Bindings map to Metal as follows:
//
//   - the vertex input struct is the [[stage_in]] parameter of vs_main; each
//     field with location N gets [[attribute(N)]]. Per-instance stepping is
//     configured by the host's MTLVertexDescriptor (buffer index 0), not in
//     the shader.
//   - the vertex index parameter is "uint name [[vertex_id]]".
//   - the vertex output struct (also the fragment input, [[stage_in]] of
//     fs_main) marks the position field [[position]], location N as
//     [[user(locN)]] and flat fields additionally with flat.
//   - uniform binding N is "constant T& name [[buffer(N+1)]]"; buffer(0) is
//     reserved for the instance data.
//   - texture binding N is "texture2d<float> name [[texture(N)]]". Its
//     sampler is a program-scope constexpr sampler (normalized coordinates,
//     clamp-to-edge, the texture's filter), so the host binds no sampler
//     object; [[sampler(N)]] is therefore never used.
//
// Only the resources an entry point actually uses (directly or through the
// helpers it calls) appear in its parameter list.
//
// # Globals in helper functions
//
// MSL has no program-scope resource variables, so uniforms and textures are
// entry-point parameters. A helper function that references a global, or
// calls a helper that does, receives it as an extra trailing parameter
// (constant T& for uniforms, texture2d<float> for textures, in module
// order: uniforms, then textures), and every call site passes it on under
// the same name. Because ir.Module.Funcs is ordered callees-first, the
// transitive use sets are computed in one pass.
//
// # Names
//
// Every identifier is legalized: names that are C++/Metal keywords, Metal
// types, Metal standard-library functions, common Metal macros or the
// generated names (vs_main, fs_main, ...) get a "_" suffix, and a name that
// is still taken gets "_1", "_2", ... (the first free one). Struct, helper,
// uniform, texture and sampler names share the program scope; parameters
// and locals of each function live in a scope nested in it, so a local can
// never hide a global, struct or helper it might need. Field names are
// legalized per struct. Swizzle names are printed unchanged.
//
// # Generated helpers
//
// MSL has no erf, so a call of sl.Erf becomes a call of sl_erf, a static
// inline function emitted (only when some function calls sl.Erf) after the
// samplers and before the shader's own helpers. It evaluates the same
// Abramowitz–Stegun 7.1.26 polynomial as sl.Erf. Its name is a
// program-scope name declared after all others, so a shader identifier
// called sl_erf keeps its name and the helper becomes sl_erf_1.
//
// # Coordinates
//
// The IR position output is in Metal clip space (x right, y up, z in 0..1),
// and the shader source itself converts pixel coordinates to clip space, so
// this backend emits positions unchanged. Texture coordinates are normalized
// with a top-left origin, which also matches Metal. A future backend for an
// API with a different convention (for example Vulkan's y-down clip space or
// OpenGL's -1..1 depth) must apply that transform to the position output.
package msl

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/tangoui/tango/ui/shader/ir"
)

// Entry-point names in the generated source.
const (
	VertexEntry   = "vs_main"
	FragmentEntry = "fs_main"
)

// Generate returns the Metal Shading Language source for m.
func Generate(m *ir.Module) (string, error) {
	if m == nil || m.Vertex == nil || m.Fragment == nil {
		return "", fmt.Errorf("msl: module needs vertex and fragment entry points")
	}
	g := &gen{
		m:          m,
		global:     newScope(nil),
		structName: map[*ir.Struct]string{},
		fieldName:  map[*ir.Struct]map[string]string{},
		funcName:   map[*ir.Func]string{},
		globalName: map[string]string{},
		sampler:    map[string]string{},
		texture:    map[string]*ir.TextureVar{},
		uniform:    map[string]*ir.Uniform{},
		uses:       map[*ir.Func]map[string]bool{},
		role:       map[*ir.Struct]role{},
	}
	g.declareGlobals()
	g.computeUses()
	if err := g.assignRoles(); err != nil {
		return "", err
	}
	g.emit()
	if g.err != nil {
		return "", g.err
	}
	return g.b.String(), nil
}

type role int

const (
	rolePlain role = iota
	roleVertexIn
	roleVarying
)

type gen struct {
	m      *ir.Module
	b      strings.Builder
	indent int
	err    error

	global     *scope
	structName map[*ir.Struct]string
	fieldName  map[*ir.Struct]map[string]string
	funcName   map[*ir.Func]string
	globalName map[string]string // uniform or texture IR name -> MSL name
	sampler    map[string]string // texture IR name -> constexpr sampler name
	texture    map[string]*ir.TextureVar
	uniform    map[string]*ir.Uniform
	uses       map[*ir.Func]map[string]bool // globals used, transitively
	role       map[*ir.Struct]role

	local map[*ir.Var]string // names in the function being emitted

	erfName string // the generated erf helper's name; "" when sl.Erf is not called
}

func (g *gen) fail(format string, args ...any) {
	if g.err == nil {
		g.err = fmt.Errorf("msl: "+format, args...)
	}
}

// declareGlobals names everything in the program scope, in a fixed order so
// that renaming is deterministic.
func (g *gen) declareGlobals() {
	for _, s := range g.m.Structs {
		g.structName[s] = g.global.declare(s.Name)
		fs := newScope(nil)
		names := map[string]string{}
		for _, f := range s.Fields {
			names[f.Name] = fs.declare(f.Name)
		}
		g.fieldName[s] = names
	}
	for _, f := range g.m.Funcs {
		g.funcName[f] = g.global.declare(f.Name)
	}
	for _, u := range g.m.Uniforms {
		g.uniform[u.Name] = u
		g.globalName[u.Name] = g.global.declare(u.Name)
	}
	for _, t := range g.m.Textures {
		g.texture[t.Name] = t
		g.globalName[t.Name] = g.global.declare(t.Name)
	}
	for _, t := range g.m.Textures {
		g.sampler[t.Name] = g.global.declare(t.Name + "_sampler")
	}
	if g.callsBuiltin("Erf") {
		g.erfName = g.global.declare("sl_erf")
	}
}

// callsBuiltin reports whether any function of the module calls the sl
// builtin name.
func (g *gen) callsBuiltin(name string) bool {
	found := false
	for _, f := range append(append([]*ir.Func{}, g.m.Funcs...), g.m.Vertex, g.m.Fragment) {
		walkStmts(f.Body, func(e ir.Expr) {
			if c, ok := e.(*ir.Call); ok && c.Func == nil && c.Builtin == name {
				found = true
			}
		})
	}
	return found
}

// computeUses records, for every function, the globals it references
// directly or through the helpers it calls.
func (g *gen) computeUses() {
	for _, f := range append(append([]*ir.Func{}, g.m.Funcs...), g.m.Vertex, g.m.Fragment) {
		set := map[string]bool{}
		walkStmts(f.Body, func(e ir.Expr) {
			switch e := e.(type) {
			case *ir.GlobalRef:
				set[e.Name] = true
			case *ir.Call:
				if e.Func != nil {
					for n := range g.uses[e.Func] {
						set[n] = true
					}
				}
			}
		})
		g.uses[f] = set
	}
}

// assignRoles marks the vertex input and vertex output structs, whose
// fields carry stage attributes.
func (g *gen) assignRoles() error {
	in, ok := g.entryStruct(g.m.Vertex)
	if !ok {
		return fmt.Errorf("msl: vertex entry %s must take one struct input", g.m.Vertex.Name)
	}
	out, ok := g.m.Vertex.Result.(ir.StructRef)
	if !ok || out.S == nil {
		return fmt.Errorf("msl: vertex entry %s must return a struct", g.m.Vertex.Name)
	}
	if in == out.S {
		return fmt.Errorf("msl: struct %s cannot be both vertex input and output", in.Name)
	}
	g.role[in] = roleVertexIn
	g.role[out.S] = roleVarying
	if len(g.m.Fragment.Params) > 1 {
		return fmt.Errorf("msl: fragment entry %s must take at most one input", g.m.Fragment.Name)
	}
	if len(g.m.Fragment.Params) == 1 {
		if r, ok := g.m.Fragment.Params[0].Type.(ir.StructRef); !ok || r.S != out.S {
			return fmt.Errorf("msl: fragment input must be the vertex output struct %s", out.S.Name)
		}
	}
	if g.m.Fragment.Result != (ir.Vector{Elem: ir.F32, N: 4}) {
		return fmt.Errorf("msl: fragment entry %s must return vec4<f32>", g.m.Fragment.Name)
	}
	return nil
}

func (g *gen) entryStruct(f *ir.Func) (*ir.Struct, bool) {
	if len(f.Params) != 1 {
		return nil, false
	}
	r, ok := f.Params[0].Type.(ir.StructRef)
	return r.S, ok && r.S != nil
}

// ---- output ----

func (g *gen) line(format string, args ...any) {
	g.b.WriteString(strings.Repeat("    ", g.indent))
	fmt.Fprintf(&g.b, format, args...)
	g.b.WriteByte('\n')
}

func (g *gen) emit() {
	g.line("// Code generated by tango ui/shader/msl. DO NOT EDIT.")
	g.line("")
	g.line("#include <metal_stdlib>")
	g.line("using namespace metal;")
	for _, s := range g.m.Structs {
		g.line("")
		g.structDecl(s)
	}
	if len(g.m.Textures) > 0 {
		g.line("")
	}
	for _, t := range g.m.Textures {
		filter := "linear"
		switch t.Filter {
		case "linear":
		case "nearest":
			filter = "nearest"
		default:
			g.fail("texture %s: unknown filter %q", t.Name, t.Filter)
		}
		g.line("constexpr sampler %s(coord::normalized, address::clamp_to_edge, filter::%s);", g.sampler[t.Name], filter)
	}
	if g.erfName != "" {
		g.line("")
		g.erfDecl()
	}
	for _, f := range g.m.Funcs {
		g.line("")
		g.funcDecl(f, "", g.funcName[f])
	}
	g.line("")
	g.funcDecl(g.m.Vertex, "vertex ", VertexEntry)
	g.line("")
	g.funcDecl(g.m.Fragment, "fragment ", FragmentEntry)
}

// erfDecl emits the helper sl.Erf maps to (MSL has no erf): the same
// Abramowitz–Stegun 7.1.26 polynomial, operation by operation, as sl.Erf.
func (g *gen) erfDecl() {
	c := func(v float64) string { return constLit(&ir.Const{T: ir.F32, Value: float64(float32(v))}) }
	g.line("static inline float %s(float x) {", g.erfName)
	g.indent++
	g.line("float a = abs(x);")
	g.line("float t = 1.0f / (1.0f + %s * a);", c(0.3275911))
	g.line("float y = 1.0f - t * (%s + t * (%s + t * (%s + t * (%s + t * %s)))) * exp((-a) * a);",
		c(0.254829592), c(-0.284496736), c(1.421413741), c(-1.453152027), c(1.061405429))
	g.line("return (x < 0.0f) ? (-y) : y;")
	g.indent--
	g.line("}")
}

func (g *gen) structDecl(s *ir.Struct) {
	g.line("struct %s {", g.structName[s])
	g.indent++
	for _, f := range s.Fields {
		g.line("%s %s%s;", g.typ(f.Type), g.fieldName[s][f.Name], g.attr(s, f))
	}
	g.indent--
	g.line("};")
}

func (g *gen) attr(s *ir.Struct, f ir.Field) string {
	switch g.role[s] {
	case roleVertexIn:
		if f.Attr.Location >= 0 {
			return fmt.Sprintf(" [[attribute(%d)]]", f.Attr.Location)
		}
	case roleVarying:
		var a []string
		if f.Attr.Builtin == "position" {
			a = append(a, "position")
		} else if f.Attr.Builtin != "" {
			g.fail("field %s.%s: unknown builtin %q", s.Name, f.Name, f.Attr.Builtin)
		}
		if f.Attr.Location >= 0 {
			a = append(a, fmt.Sprintf("user(loc%d)", f.Attr.Location))
		}
		if f.Attr.Flat {
			a = append(a, "flat")
		}
		if len(a) > 0 {
			return " [[" + strings.Join(a, ", ") + "]]"
		}
	}
	return ""
}

// funcDecl emits a helper (stage == "") or an entry point.
func (g *gen) funcDecl(f *ir.Func, stage, name string) {
	g.local = map[*ir.Var]string{}
	sc := newScope(g.global)
	var params []string
	for i, p := range f.Params {
		n := sc.declare(p.Name)
		g.local[p] = n
		s := g.typ(p.Type) + " " + n
		if stage != "" && i == 0 {
			s += " [[stage_in]]"
		}
		params = append(params, s)
	}
	if v := f.VertexIDParam; v != nil {
		n := sc.declare(v.Name)
		g.local[v] = n
		params = append(params, "uint "+n+" [[vertex_id]]")
	}
	params = append(params, g.globalParams(f, stage != "")...)
	walkDecls(f.Body, func(v *ir.Var) {
		if _, ok := g.local[v]; !ok {
			g.local[v] = sc.declare(v.Name)
		}
	})

	result := "void"
	if f.Result != nil {
		result = g.typ(f.Result)
	}
	g.line("%s%s %s(%s) {", stage, result, name, strings.Join(params, ", "))
	g.indent++
	g.stmts(f.Body)
	g.indent--
	g.line("}")
}

// usedGlobals lists the globals f uses, in module order.
func (g *gen) usedGlobals(f *ir.Func) []string {
	var names []string
	for _, u := range g.m.Uniforms {
		if g.uses[f][u.Name] {
			names = append(names, u.Name)
		}
	}
	for _, t := range g.m.Textures {
		if g.uses[f][t.Name] {
			names = append(names, t.Name)
		}
	}
	return names
}

func (g *gen) globalParams(f *ir.Func, entry bool) []string {
	var params []string
	for _, n := range g.usedGlobals(f) {
		if u := g.uniform[n]; u != nil {
			s := "constant " + g.structName[u.Type] + "& " + g.globalName[n]
			if entry {
				s += fmt.Sprintf(" [[buffer(%d)]]", u.Binding+1)
			}
			params = append(params, s)
		} else if t := g.texture[n]; t != nil {
			s := "texture2d<float> " + g.globalName[n]
			if entry {
				s += fmt.Sprintf(" [[texture(%d)]]", t.Binding)
			}
			params = append(params, s)
		} else {
			g.fail("reference to unknown global %s", n)
		}
	}
	return params
}

// ---- types ----

var scalarNames = [...]string{ir.F32: "float", ir.I32: "int", ir.U32: "uint", ir.Bool: "bool"}

func (g *gen) typ(t ir.Type) string {
	switch t := t.(type) {
	case ir.Scalar:
		if int(t) >= 0 && int(t) < len(scalarNames) {
			return scalarNames[t]
		}
	case ir.Vector:
		if int(t.Elem) >= 0 && int(t.Elem) < len(scalarNames) && t.N >= 2 && t.N <= 4 {
			return scalarNames[t.Elem] + strconv.Itoa(t.N)
		}
	case ir.StructRef:
		if n, ok := g.structName[t.S]; ok {
			return n
		}
	case ir.Array:
		return fmt.Sprintf("array<%s, %d>", g.typ(t.Elem), t.Len)
	case ir.Texture2D:
		return "texture2d<float>"
	}
	g.fail("unsupported type %v", t)
	return "void"
}

// ---- statements ----

func (g *gen) stmts(list []ir.Stmt) {
	for _, s := range list {
		g.stmt(s)
	}
}

func (g *gen) stmt(s ir.Stmt) {
	switch s := s.(type) {
	case *ir.If:
		g.line("if (%s) {", g.top(s.Cond))
		g.ifTail(s)
	case *ir.For:
		var init, cond, post string
		if s.Init != nil {
			init = g.simple(s.Init)
		}
		if s.Cond != nil {
			cond = " " + g.top(s.Cond)
		}
		if s.Post != nil {
			post = " " + g.simple(s.Post)
		}
		g.line("for (%s;%s;%s) {", init, cond, post)
		g.indent++
		g.stmts(s.Body)
		g.indent--
		g.line("}")
	default:
		g.line("%s;", g.simple(s))
	}
}

// ifTail emits the body of s and its else chain after the opening line.
func (g *gen) ifTail(s *ir.If) {
	g.indent++
	g.stmts(s.Then)
	g.indent--
	switch {
	case len(s.Else) == 0:
		g.line("}")
	case len(s.Else) == 1:
		if e, ok := s.Else[0].(*ir.If); ok {
			g.line("} else if (%s) {", g.top(e.Cond))
			g.ifTail(e)
			return
		}
		fallthrough
	default:
		g.line("} else {")
		g.indent++
		g.stmts(s.Else)
		g.indent--
		g.line("}")
	}
}

// simple renders a statement that fits on one line, without the semicolon.
func (g *gen) simple(s ir.Stmt) string {
	switch s := s.(type) {
	case *ir.Decl:
		init := "{}"
		if s.Init != nil {
			init = g.top(s.Init)
		}
		return fmt.Sprintf("%s %s = %s", g.typ(s.V.Type), g.local[s.V], init)
	case *ir.Assign:
		return g.expr(s.LHS) + " = " + g.top(s.RHS)
	case *ir.Return:
		if s.Value == nil {
			return "return"
		}
		return "return " + g.top(s.Value)
	case *ir.Break:
		return "break"
	case *ir.Continue:
		return "continue"
	case *ir.ExprStmt:
		return g.top(s.X)
	}
	g.fail("unsupported statement %T", s)
	return ""
}

// ---- expressions ----

// top renders e without the outer parentheses of a binary, unary or select
// expression; it is used where the context already delimits the expression.
func (g *gen) top(e ir.Expr) string {
	s := g.expr(e)
	switch e.(type) {
	case *ir.Binary, *ir.Select:
		return s[1 : len(s)-1]
	}
	return s
}

func (g *gen) expr(e ir.Expr) string {
	switch e := e.(type) {
	case *ir.Const:
		return constLit(e)
	case *ir.VarRef:
		if n, ok := g.local[e.V]; ok {
			return n
		}
		g.fail("reference to undeclared variable %v", e.V)
		return ""
	case *ir.GlobalRef:
		if _, ok := e.T.(ir.Texture2D); ok {
			g.fail("texture %s can only be used as the receiver of Sample", e.Name)
		}
		return g.globalRef(e.Name)
	case *ir.Binary:
		if vt, ok := e.Type().(ir.Vector); ok && vt.Elem == ir.Bool && (e.Op == "&&" || e.Op == "||") {
			g.fail("operator %s on bool vectors is not supported", e.Op)
		}
		return "(" + g.expr(e.X) + " " + e.Op + " " + g.expr(e.Y) + ")"
	case *ir.Unary:
		return "(" + e.Op + g.expr(e.X) + ")"
	case *ir.Call:
		return g.call(e)
	case *ir.FieldExpr:
		x := g.expr(e.X)
		if r, ok := e.X.Type().(ir.StructRef); ok {
			if n, ok := g.fieldName[r.S][e.Name]; ok {
				return x + "." + n
			}
			g.fail("struct %v has no field %s", r, e.Name)
			return ""
		}
		if _, ok := e.X.Type().(ir.Vector); ok && e.Type() != nil {
			return x + "." + strings.ToLower(e.Name)
		}
		g.fail("bad field selector .%s on %v", e.Name, e.X.Type())
		return ""
	case *ir.Index:
		return g.expr(e.X) + "[" + g.top(e.I) + "]"
	case *ir.Construct:
		args := g.exprs(e.Args)
		if _, ok := e.T.(ir.Vector); ok {
			return g.typ(e.T) + "(" + args + ")"
		}
		return g.typ(e.T) + "{" + args + "}"
	case *ir.Convert:
		return g.typ(e.T) + "(" + g.top(e.X) + ")"
	case *ir.Select:
		if _, ok := e.Cond.Type().(ir.Vector); ok {
			// select(a, b, c) picks b where c is true.
			return "(select(" + g.top(e.Y) + ", " + g.top(e.X) + ", " + g.top(e.Cond) + "))"
		}
		return "(" + g.expr(e.Cond) + " ? " + g.expr(e.X) + " : " + g.expr(e.Y) + ")"
	}
	g.fail("unsupported expression %T", e)
	return ""
}

func (g *gen) globalRef(name string) string {
	if n, ok := g.globalName[name]; ok {
		return n
	}
	g.fail("reference to unknown global %s", name)
	return ""
}

func (g *gen) exprs(list []ir.Expr) string {
	parts := make([]string, len(list))
	for i, a := range list {
		parts[i] = g.top(a)
	}
	return strings.Join(parts, ", ")
}

// builtins maps sl functions to Metal functions with the same argument
// order. Entries with splat set broadcast the last (scalar) argument to the
// call's vector type first.
var builtins = map[string]struct {
	name  string
	nargs int
	splat bool
}{
	"Abs":        {"abs", 1, false},
	"Min":        {"min", 2, false},
	"Max":        {"max", 2, false},
	"Clamp":      {"clamp", 3, false},
	"Mix":        {"mix", 3, false},
	"Step":       {"step", 2, false},
	"Smoothstep": {"smoothstep", 3, false},
	"Sqrt":       {"sqrt", 1, false},
	"Floor":      {"floor", 1, false},
	"Fract":      {"fract", 1, false},
	"Pow":        {"pow", 2, false},
	"Length":     {"length", 1, false},
	"Dot":        {"dot", 2, false},
	"AbsV2":      {"abs", 1, false},
	"MaxV2":      {"max", 2, true},
	"MinV2":      {"min", 2, true},
	"MixV4":      {"mix", 3, true},
}

func (g *gen) call(e *ir.Call) string {
	if e.Func != nil {
		name, ok := g.funcName[e.Func]
		if !ok {
			g.fail("call to %s, which is not a helper function", e.Func.Name)
			return ""
		}
		args := make([]string, 0, len(e.Args))
		for _, a := range e.Args {
			args = append(args, g.top(a))
		}
		for _, n := range g.usedGlobals(e.Func) {
			args = append(args, g.globalName[n])
		}
		return name + "(" + strings.Join(args, ", ") + ")"
	}
	if e.Builtin == "Sample" {
		var tex *ir.GlobalRef
		if len(e.Args) == 2 {
			tex, _ = e.Args[0].(*ir.GlobalRef)
		}
		if tex == nil || g.texture[tex.Name] == nil {
			g.fail("Sample must be called on a texture with one coordinate")
			return ""
		}
		return g.globalRef(tex.Name) + ".sample(" + g.sampler[tex.Name] + ", " + g.top(e.Args[1]) + ")"
	}
	if e.Builtin == "Erf" && len(e.Args) == 1 {
		return g.erfName + "(" + g.top(e.Args[0]) + ")"
	}
	b, ok := builtins[e.Builtin]
	if !ok {
		g.fail("unsupported builtin sl.%s", e.Builtin)
		return ""
	}
	if len(e.Args) != b.nargs {
		g.fail("sl.%s takes %d arguments, got %d", e.Builtin, b.nargs, len(e.Args))
		return ""
	}
	args := make([]string, len(e.Args))
	for i, a := range e.Args {
		args[i] = g.top(a)
	}
	if b.splat {
		last := len(args) - 1
		args[last] = g.typ(e.Type()) + "(" + args[last] + ")"
	}
	return b.name + "(" + strings.Join(args, ", ") + ")"
}

// constLit renders a constant as a Metal literal. f32 values always carry a
// decimal point or exponent and an f suffix, u32 a u suffix; negative values
// are parenthesized. A vector constant is a splat of its scalar value.
func constLit(c *ir.Const) string {
	switch t := c.T.(type) {
	case ir.Vector:
		return scalarNames[t.Elem] + strconv.Itoa(t.N) + "(" + constLit(&ir.Const{T: t.Elem, Value: c.Value}) + ")"
	case ir.Scalar:
		v := c.Value
		var s string
		switch t {
		case ir.F32:
			switch {
			case math.IsNaN(v):
				return "NAN"
			case math.IsInf(v, 1):
				return "INFINITY"
			case math.IsInf(v, -1):
				return "(-INFINITY)"
			}
			s = strconv.FormatFloat(v, 'g', -1, 32)
			if !strings.ContainsAny(s, ".e") {
				s += ".0"
			}
			s += "f"
		case ir.I32:
			if v == math.MinInt32 {
				return "(-2147483647 - 1)"
			}
			s = strconv.FormatInt(int64(v), 10)
		case ir.U32:
			s = strconv.FormatUint(uint64(v), 10) + "u"
		case ir.Bool:
			if v != 0 {
				return "true"
			}
			return "false"
		}
		if strings.HasPrefix(s, "-") {
			return "(" + s + ")"
		}
		return s
	}
	return "0"
}

// ---- walking ----

// walkStmts calls fn for every expression (including nested ones) in list.
func walkStmts(list []ir.Stmt, fn func(ir.Expr)) {
	for _, s := range list {
		walkStmt(s, fn)
	}
}

func walkStmt(s ir.Stmt, fn func(ir.Expr)) {
	switch s := s.(type) {
	case *ir.Decl:
		walkExpr(s.Init, fn)
	case *ir.Assign:
		walkExpr(s.LHS, fn)
		walkExpr(s.RHS, fn)
	case *ir.If:
		walkExpr(s.Cond, fn)
		walkStmts(s.Then, fn)
		walkStmts(s.Else, fn)
	case *ir.For:
		if s.Init != nil {
			walkStmt(s.Init, fn)
		}
		walkExpr(s.Cond, fn)
		if s.Post != nil {
			walkStmt(s.Post, fn)
		}
		walkStmts(s.Body, fn)
	case *ir.Return:
		walkExpr(s.Value, fn)
	case *ir.ExprStmt:
		walkExpr(s.X, fn)
	}
}

func walkExpr(e ir.Expr, fn func(ir.Expr)) {
	if e == nil {
		return
	}
	fn(e)
	switch e := e.(type) {
	case *ir.Binary:
		walkExpr(e.X, fn)
		walkExpr(e.Y, fn)
	case *ir.Unary:
		walkExpr(e.X, fn)
	case *ir.Call:
		for _, a := range e.Args {
			walkExpr(a, fn)
		}
	case *ir.FieldExpr:
		walkExpr(e.X, fn)
	case *ir.Index:
		walkExpr(e.X, fn)
		walkExpr(e.I, fn)
	case *ir.Construct:
		for _, a := range e.Args {
			walkExpr(a, fn)
		}
	case *ir.Convert:
		walkExpr(e.X, fn)
	case *ir.Select:
		walkExpr(e.Cond, fn)
		walkExpr(e.X, fn)
		walkExpr(e.Y, fn)
	}
}

// walkDecls calls fn for every local declared in list, in source order.
func walkDecls(list []ir.Stmt, fn func(*ir.Var)) {
	for _, s := range list {
		switch s := s.(type) {
		case *ir.Decl:
			fn(s.V)
		case *ir.If:
			walkDecls(s.Then, fn)
			walkDecls(s.Else, fn)
		case *ir.For:
			if s.Init != nil {
				walkDecls([]ir.Stmt{s.Init}, fn)
			}
			walkDecls(s.Body, fn)
		}
	}
}

// ---- names ----

type scope struct {
	parent *scope
	used   map[string]bool
}

func newScope(parent *scope) *scope { return &scope{parent: parent, used: map[string]bool{}} }

func (s *scope) taken(name string) bool {
	for ; s != nil; s = s.parent {
		if s.used[name] {
			return true
		}
	}
	return false
}

// declare returns a legal, unused name for name and reserves it: name itself
// if possible, else name+"_" for reserved words, else the first free
// name_1, name_2, ...
func (s *scope) declare(name string) string {
	base := sanitize(name)
	cand := base
	if reserved[cand] {
		cand = base + "_"
	}
	for i := 1; reserved[cand] || s.taken(cand); i++ {
		cand = base + "_" + strconv.Itoa(i)
	}
	s.used[cand] = true
	return cand
}

// sanitize maps a Go identifier to an ASCII C++ identifier that does not use
// the implementation-reserved forms __x and _X.
func sanitize(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			fmt.Fprintf(&b, "u%04X", r)
		}
	}
	s := b.String()
	if s == "" {
		return "v"
	}
	if strings.HasPrefix(s, "__") || len(s) > 1 && s[0] == '_' && s[1] >= 'A' && s[1] <= 'Z' {
		s = "v" + s
	}
	return s
}

// reserved holds identifiers the generated source must not declare: C++ and
// Metal keywords, Metal types, Metal standard-library functions, common
// macros, and the names this backend generates itself.
var reserved = func() map[string]bool {
	m := map[string]bool{}
	for _, list := range []string{
		// C++ keywords and alternative tokens.
		`alignas alignof and and_eq asm auto bitand bitor bool break case catch char
		char8_t char16_t char32_t class compl concept const consteval constexpr
		constinit const_cast continue co_await co_return co_yield decltype default
		delete do double dynamic_cast else enum explicit export extern false float
		for friend goto if inline int long mutable namespace new noexcept not not_eq
		nullptr operator or or_eq private protected public register reinterpret_cast
		requires return short signed sizeof static static_assert static_cast struct
		switch template this thread_local throw true try typedef typeid typename union
		unsigned using virtual void volatile wchar_t while xor xor_eq final override
		import module main std size_t ptrdiff_t nullptr_t NULL offsetof`,
		// Metal keywords, address spaces, types and sampler/texture enums.
		`kernel vertex fragment compute constant device thread threadgroup
		threadgroup_imageblock ray_data object_data visible stitchable intersection
		mesh object metal half bfloat uchar ushort uint ulong vec matrix packed_vec
		array array_ref sampler texture texture1d texture1d_array texture2d
		texture2d_array texture3d texturecube texturecube_array texture2d_ms
		texture2d_ms_array depth2d depth2d_array depthcube depthcube_array
		depth2d_ms depth2d_ms_array texture_buffer imageblock acceleration_structure
		visible_function_table intersection_function_table atomic atomic_int
		atomic_uint atomic_bool atomic_float access coord address filter
		mag_filter min_filter mip_filter compare_func border_color lod_options
		bias level gradient2d min_lod_clamp sample read write`,
		// Metal standard-library functions.
		`abs absdiff acos acosh addsat all any as_type asin asinh atan atan2 atanh
		ceil clamp clz copysign cos cosh cospi cross ctz degrees determinant distance
		distance_squared divide dot exp exp2 exp10 extract_bits fabs faceforward fdim
		floor fma fmax fmax3 fmedian3 fmin fmin3 fmod fract frexp fwidth dfdx dfdy
		hadd ilogb insert_bits isfinite isinf isnan isnormal isordered isunordered
		ldexp length length_squared log log2 log10 mad madhi madsat max max3 median3
		min min3 mix modf mulhi nan nextafter normalize popcount pow powr radians
		reflect refract reverse_bits rhadd rint rotate round rsqrt saturate select
		sign signbit sin sincos sinh sinpi smoothstep sqrt step subsat tan tanh tanpi
		transpose trunc discard_fragment threadgroup_barrier simdgroup_barrier
		as_type_ptr pack_float_to_unorm4x8 unpack_unorm4x8_to_float precise fast`,
		// Common Metal macros.
		`INFINITY NAN MAXFLOAT HUGE_VALF HUGE_VALH FLT_MAX FLT_MIN FLT_EPSILON
		FLT_DIG FLT_MANT_DIG FLT_MAX_EXP FLT_MIN_EXP HALF_MAX HALF_MIN HALF_EPSILON
		M_E_F M_LOG2E_F M_LOG10E_F M_LN2_F M_LN10_F M_PI_F M_PI_2_F M_PI_4_F
		M_1_PI_F M_2_PI_F M_2_SQRTPI_F M_SQRT2_F M_SQRT1_2_F CHAR_BIT CHAR_MAX
		CHAR_MIN INT_MAX INT_MIN UINT_MAX SHRT_MAX SHRT_MIN USHRT_MAX LONG_MAX
		LONG_MIN ULONG_MAX SCHAR_MAX SCHAR_MIN UCHAR_MAX FP_ILOGB0 FP_ILOGBNAN
		__METAL_VERSION__`,
		// Names generated by this backend.
		VertexEntry + " " + FragmentEntry,
	} {
		for _, w := range strings.Fields(list) {
			m[w] = true
		}
	}
	for _, base := range []string{"bool", "char", "uchar", "short", "ushort", "int", "uint", "long", "ulong", "half", "float", "bfloat"} {
		for n := 2; n <= 4; n++ {
			v := base + strconv.Itoa(n)
			m[v] = true
			m["packed_"+v] = true
			if base == "half" || base == "float" || base == "bfloat" {
				for k := 2; k <= 4; k++ {
					m[v+"x"+strconv.Itoa(k)] = true
				}
			}
		}
	}
	return m
}()
