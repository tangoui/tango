// Package hlsl generates HLSL (shader model 5.0) source from a shader
// ir.Module, for Direct3D 11. The host compiles it at run time with
// D3DCompile (targets vs_5_0 and ps_5_0, or vs_4_0 and ps_4_0 on a
// feature level 10_x device), so the output stays within what fxc accepts
// for shader model 4.0 as well: no HLSL 2021 features, no derivatives in
// sampling, no SM5-only intrinsics or resource types.
//
// # Interface mapping
//
// The entry points are always named vs_main (vertex) and ps_main (pixel).
// Bindings map to Direct3D 11 as follows:
//
//   - the vertex input struct is the first parameter of vs_main; each field
//     with location N gets the semantic TEXCOORD<N>. The host's input
//     layout uses SemanticName "TEXCOORD", SemanticIndex N, input slot 0,
//     D3D11_INPUT_PER_INSTANCE_DATA with step rate 1, and the Go struct's
//     field offsets.
//   - the vertex index parameter is "uint name : SV_VertexID".
//   - the vertex output struct (also the input of ps_main) marks the
//     position field SV_Position and location N as TEXCOORD<N>; flat
//     fields, and every integer or bool field (D3D cannot interpolate
//     them), get nointerpolation.
//   - ps_main returns float4 with the semantic SV_Target.
//   - uniform binding N is "cbuffer <Name>_cb : register(bN) { T Name; }",
//     so expressions keep the Name.Field form.
//   - texture binding N is "Texture2D<float4> name : register(tN)" with
//     "SamplerState name_s : register(sN)"; the host binds a clamp-to-edge
//     sampler with the texture's filter (point or linear) at sN.
//
// Unlike MSL, HLSL has program-scope resources, so helper functions use
// uniforms and textures directly and take no extra parameters.
//
// Generate relies on ir.Module.Funcs being ordered callees-first (as the
// front end produces it, and as the MSL backend relies on too): fxc
// needs every function declared before its first call, and the functions
// are emitted in module order without forward declarations.
//
// # Expressions
//
//   - sl Sample always becomes tex.SampleLevel(tex_s, uv, 0): the textures
//     have one mip level, and an implicit-derivative Sample inside
//     divergent control flow is an fxc error.
//   - HLSL has no erf; sl.Erf becomes sl_erf, a helper evaluating the same
//     Abramowitz–Stegun 7.1.26 polynomial as sl.Erf and the MSL backend.
//   - sl.Fract becomes frac and sl.Mix lerp; other builtins have the same
//     lower-case name. The scalar argument of MaxV2, MinV2 and MixV4 is
//     cast to the vector type.
//   - A vector splat is the cast (floatN)x: HLSL vector constructors need
//     every component.
//   - HLSL has no struct literals in expressions, so a struct Construct
//     becomes a call of a generated <Struct>_ctor function (emitted only
//     for structs that are constructed); an all-zero struct is (T)0. An
//     array Construct is supported only as a variable initializer, where
//     it becomes an initializer list; HLSL functions cannot return arrays.
//   - A Select is the ?: operator, which in shader model 5 evaluates both
//     operands and, for a bool-vector condition, picks per component: the
//     IR semantics, since IR expressions have no side effects. && and ||
//     also evaluate both operands in SM5, which is likewise unobservable.
//   - Integer / truncates toward zero, as in Go.
//
// # Names
//
// Identifiers are legalized as in the MSL backend, against HLSL keywords,
// reserved words, type names and intrinsic functions (the effect-framework
// keywords, which fxc matches case-insensitively, in any case), plus the
// generated names. Struct, helper, uniform, cbuffer, texture, sampler and
// generated helper names share the program scope; parameters and locals of
// each function live in a nested scope.
//
// # Coordinates
//
// The IR position output is in clip space with x right, y up and z in
// 0..1, which is Direct3D's convention, so positions are emitted
// unchanged; texture coordinates are normalized with a top-left origin,
// also as in Direct3D. A backend for OpenGL (-1..1 depth, bottom-left
// texture origin) or Vulkan (y-down clip space) must transform them.
package hlsl

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/tangoui/tango/ui/shader/ir"
)

// Entry-point names and compile targets of the generated source.
const (
	VertexEntry    = "vs_main"
	FragmentEntry  = "ps_main"
	VertexTarget   = "vs_5_0"
	FragmentTarget = "ps_5_0"
	// The shader model 4.0 targets the source also compiles for, used on
	// Direct3D feature level 10_0 and 10_1 devices.
	VertexTargetSM4   = "vs_4_0"
	FragmentTargetSM4 = "ps_4_0"
)

// Generate returns the HLSL source for m.
func Generate(m *ir.Module) (string, error) {
	if m == nil || m.Vertex == nil || m.Fragment == nil {
		return "", fmt.Errorf("hlsl: module needs vertex and fragment entry points")
	}
	g := &gen{
		m:          m,
		global:     newScope(nil),
		structName: map[*ir.Struct]string{},
		fieldName:  map[*ir.Struct]map[string]string{},
		funcName:   map[*ir.Func]string{},
		globalName: map[string]string{},
		sampler:    map[string]string{},
		cbuffer:    map[string]string{},
		ctor:       map[*ir.Struct]string{},
		texture:    map[string]*ir.TextureVar{},
		uniform:    map[string]*ir.Uniform{},
		role:       map[*ir.Struct]role{},
	}
	if err := g.assignRoles(); err != nil {
		return "", err
	}
	g.declareGlobals()
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
	globalName map[string]string // uniform or texture IR name -> HLSL name
	sampler    map[string]string // texture IR name -> SamplerState name
	cbuffer    map[string]string // uniform IR name -> cbuffer name
	ctor       map[*ir.Struct]string
	texture    map[string]*ir.TextureVar
	uniform    map[string]*ir.Uniform
	role       map[*ir.Struct]role

	local map[*ir.Var]string // names in the function being emitted

	erfName string // the generated erf helper's name; "" when sl.Erf is not called
}

func (g *gen) fail(format string, args ...any) {
	if g.err == nil {
		g.err = fmt.Errorf("hlsl: "+format, args...)
	}
}

func (g *gen) allFuncs() []*ir.Func {
	return append(append([]*ir.Func{}, g.m.Funcs...), g.m.Vertex, g.m.Fragment)
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
	for _, u := range g.m.Uniforms {
		g.cbuffer[u.Name] = g.global.declare(u.Name + "_cb")
	}
	for _, t := range g.m.Textures {
		g.sampler[t.Name] = g.global.declare(t.Name + "_s")
	}
	built := map[*ir.Struct]bool{}
	for _, f := range g.allFuncs() {
		walkStmts(f.Body, func(e ir.Expr) {
			if c, ok := e.(*ir.Construct); ok && !isZero(c) {
				if r, ok := c.T.(ir.StructRef); ok {
					built[r.S] = true
				}
			}
		})
	}
	for _, s := range g.m.Structs {
		if built[s] {
			g.ctor[s] = g.global.declare(s.Name + "_ctor")
		}
	}
	if g.callsBuiltin("Erf") {
		g.erfName = g.global.declare("sl_erf")
	}
}

// callsBuiltin reports whether any function of the module calls the sl
// builtin name.
func (g *gen) callsBuiltin(name string) bool {
	found := false
	for _, f := range g.allFuncs() {
		walkStmts(f.Body, func(e ir.Expr) {
			if c, ok := e.(*ir.Call); ok && c.Func == nil && c.Builtin == name {
				found = true
			}
		})
	}
	return found
}

// assignRoles marks the vertex input and vertex output structs, whose
// fields carry semantics.
func (g *gen) assignRoles() error {
	in, ok := g.entryStruct(g.m.Vertex)
	if !ok {
		return fmt.Errorf("hlsl: vertex entry %s must take one struct input", g.m.Vertex.Name)
	}
	out, ok := g.m.Vertex.Result.(ir.StructRef)
	if !ok || out.S == nil {
		return fmt.Errorf("hlsl: vertex entry %s must return a struct", g.m.Vertex.Name)
	}
	if in == out.S {
		return fmt.Errorf("hlsl: struct %s cannot be both vertex input and output", in.Name)
	}
	g.role[in] = roleVertexIn
	g.role[out.S] = roleVarying
	if len(g.m.Fragment.Params) > 1 {
		return fmt.Errorf("hlsl: fragment entry %s must take at most one input", g.m.Fragment.Name)
	}
	if len(g.m.Fragment.Params) == 1 {
		if r, ok := g.m.Fragment.Params[0].Type.(ir.StructRef); !ok || r.S != out.S {
			return fmt.Errorf("hlsl: fragment input must be the vertex output struct %s", out.S.Name)
		}
	}
	if g.m.Fragment.Result != (ir.Vector{Elem: ir.F32, N: 4}) {
		return fmt.Errorf("hlsl: fragment entry %s must return vec4<f32>", g.m.Fragment.Name)
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
	g.line("// Code generated by tango ui/shader/hlsl. DO NOT EDIT.")
	g.line("//")
	g.line("// HLSL shader model 5.0 (also SM4-compatible: %s/%s): compile %s with %s and %s with %s.", VertexTargetSM4, FragmentTargetSM4, VertexEntry, VertexTarget, FragmentEntry, FragmentTarget)
	g.line("// Positions are Direct3D clip space (y up, z 0..1) as computed by the shader.")
	for _, s := range g.m.Structs {
		g.line("")
		g.structDecl(s)
	}
	for _, u := range g.m.Uniforms {
		g.line("")
		g.line("cbuffer %s : register(b%d) {", g.cbuffer[u.Name], u.Binding)
		g.indent++
		g.line("%s %s;", g.structName[u.Type], g.globalName[u.Name])
		g.indent--
		g.line("};")
	}
	if len(g.m.Textures) > 0 {
		g.line("")
	}
	for _, t := range g.m.Textures {
		switch t.Filter {
		case "linear", "nearest":
		default:
			g.fail("texture %s: unknown filter %q", t.Name, t.Filter)
		}
		g.line("Texture2D<float4> %s : register(t%d); // filter %s", g.globalName[t.Name], t.Binding, t.Filter)
		g.line("SamplerState %s : register(s%d);", g.sampler[t.Name], t.Binding)
	}
	for _, s := range g.m.Structs {
		if g.ctor[s] != "" {
			g.line("")
			g.ctorDecl(s)
		}
	}
	if g.erfName != "" {
		g.line("")
		g.erfDecl()
	}
	for _, f := range g.m.Funcs {
		g.line("")
		g.funcDecl(f, g.funcName[f], false)
	}
	g.line("")
	g.funcDecl(g.m.Vertex, VertexEntry, true)
	g.line("")
	g.funcDecl(g.m.Fragment, FragmentEntry, true)
}

// erfDecl emits the helper sl.Erf maps to (HLSL has no erf): the same
// Abramowitz–Stegun 7.1.26 polynomial, operation by operation, as sl.Erf.
func (g *gen) erfDecl() {
	c := func(v float64) string { return constLit(&ir.Const{T: ir.F32, Value: float64(float32(v))}) }
	g.line("float %s(float x) {", g.erfName)
	g.indent++
	g.line("float a = abs(x);")
	g.line("float t = 1.0f / (1.0f + %s * a);", c(0.3275911))
	g.line("float y = 1.0f - t * (%s + t * (%s + t * (%s + t * (%s + t * %s)))) * exp((-a) * a);",
		c(0.254829592), c(-0.284496736), c(1.421413741), c(-1.453152027), c(1.061405429))
	g.line("return (x < 0.0f) ? (-y) : y;")
	g.indent--
	g.line("}")
}

// ctorDecl emits the constructor function of struct s, which stands in for
// struct literals (HLSL only has initializer lists in declarations).
func (g *gen) ctorDecl(s *ir.Struct) {
	sc := newScope(g.global)
	r := sc.declare("r")
	names := make([]string, len(s.Fields))
	params := make([]string, len(s.Fields))
	for i, f := range s.Fields {
		names[i] = sc.declare(f.Name)
		params[i] = g.decl(f.Type, names[i])
	}
	sn := g.structName[s]
	g.line("%s %s(%s) {", sn, g.ctor[s], strings.Join(params, ", "))
	g.indent++
	g.line("%s %s;", sn, r)
	for i, f := range s.Fields {
		g.line("%s.%s = %s;", r, g.fieldName[s][f.Name], names[i])
	}
	g.line("return %s;", r)
	g.indent--
	g.line("}")
}

func (g *gen) structDecl(s *ir.Struct) {
	g.line("struct %s {", g.structName[s])
	g.indent++
	for _, f := range s.Fields {
		mod, sem := g.semantic(s, f)
		g.line("%s%s%s;", mod, g.decl(f.Type, g.fieldName[s][f.Name]), sem)
	}
	g.indent--
	g.line("};")
}

// semantic returns the interpolation modifier prefix and the semantic
// suffix of field f of s.
func (g *gen) semantic(s *ir.Struct, f ir.Field) (mod, sem string) {
	switch g.role[s] {
	case roleVertexIn:
		if f.Attr.Location < 0 {
			g.fail("vertex input field %s.%s has no location", s.Name, f.Name)
			return "", ""
		}
		return "", fmt.Sprintf(" : TEXCOORD%d", f.Attr.Location)
	case roleVarying:
		switch {
		case f.Attr.Builtin == "position":
			return "", " : SV_Position"
		case f.Attr.Builtin != "":
			g.fail("field %s.%s: unknown builtin %q", s.Name, f.Name, f.Attr.Builtin)
			return "", ""
		case f.Attr.Location < 0:
			g.fail("vertex output field %s.%s has no location", s.Name, f.Name)
			return "", ""
		}
		if f.Attr.Flat || !isFloat(f.Type) {
			mod = "nointerpolation "
		}
		return mod, fmt.Sprintf(" : TEXCOORD%d", f.Attr.Location)
	}
	return "", ""
}

func isFloat(t ir.Type) bool {
	if v, ok := t.(ir.Vector); ok {
		t = v.Elem
	}
	return t == ir.F32
}

// funcDecl emits a helper or an entry point.
func (g *gen) funcDecl(f *ir.Func, name string, entry bool) {
	g.local = map[*ir.Var]string{}
	sc := newScope(g.global)
	var params []string
	for _, p := range f.Params {
		n := sc.declare(p.Name)
		g.local[p] = n
		params = append(params, g.decl(p.Type, n))
	}
	if v := f.VertexIDParam; v != nil {
		n := sc.declare(v.Name)
		g.local[v] = n
		params = append(params, "uint "+n+" : SV_VertexID")
	}
	walkDecls(f.Body, func(v *ir.Var) {
		if _, ok := g.local[v]; !ok {
			g.local[v] = sc.declare(v.Name)
		}
	})

	result := "void"
	if f.Result != nil {
		if _, ok := f.Result.(ir.Array); ok {
			g.fail("function %s returns an array, which HLSL does not support", f.Name)
		}
		result = g.typ(f.Result)
	}
	sem := ""
	if entry && f == g.m.Fragment {
		sem = " : SV_Target"
	}
	g.line("%s %s(%s)%s {", result, name, strings.Join(params, ", "), sem)
	g.indent++
	g.stmts(f.Body)
	g.indent--
	g.line("}")
}

// ---- types ----

var scalarNames = [...]string{ir.F32: "float", ir.I32: "int", ir.U32: "uint", ir.Bool: "bool"}

// typ returns the HLSL name of a non-array type.
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
	case ir.Texture2D:
		return "Texture2D<float4>"
	}
	g.fail("unsupported type %v", t)
	return "void"
}

// decl returns the declaration of name with type t: "float x", or
// "float x[4]" for arrays (HLSL puts array sizes after the name).
func (g *gen) decl(t ir.Type, name string) string {
	dims := ""
	for {
		a, ok := t.(ir.Array)
		if !ok {
			break
		}
		dims += "[" + strconv.Itoa(a.Len) + "]"
		t = a.Elem
	}
	return g.typ(t) + " " + name + dims
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
		var init string
		switch {
		case s.Init == nil:
			init = g.zero(s.V.Type)
		default:
			init = g.initializer(s.Init)
		}
		return g.decl(s.V.Type, g.local[s.V]) + " = " + init
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

// initializer renders the initial value of a declaration, where an array
// Construct may become an initializer list.
func (g *gen) initializer(e ir.Expr) string {
	if c, ok := e.(*ir.Construct); ok {
		if _, ok := c.T.(ir.Array); ok {
			parts := make([]string, len(c.Args))
			for i, a := range c.Args {
				parts[i] = g.initializer(a)
			}
			return "{ " + strings.Join(parts, ", ") + " }"
		}
	}
	return g.top(e)
}

// zero renders the zero value of t for a declaration.
func (g *gen) zero(t ir.Type) string {
	switch t := t.(type) {
	case ir.Scalar, ir.Vector:
		return constLit(&ir.Const{T: t})
	case ir.StructRef:
		return "(" + g.typ(t) + ")0"
	case ir.Array:
		parts := make([]string, t.Len)
		for i := range parts {
			parts[i] = g.zero(t.Elem)
		}
		return "{ " + strings.Join(parts, ", ") + " }"
	}
	g.fail("no zero value for type %v", t)
	return "0"
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
		switch t := e.T.(type) {
		case ir.Vector:
			return g.typ(t) + "(" + g.exprs(e.Args) + ")"
		case ir.StructRef:
			if isZero(e) {
				return "((" + g.typ(t) + ")0)"
			}
			if n, ok := g.ctor[t.S]; ok {
				return n + "(" + g.exprs(e.Args) + ")"
			}
		case ir.Array:
			g.fail("array literal of %v outside a variable declaration is not supported in HLSL", t)
			return ""
		}
		g.fail("unsupported construct of %v", e.T)
		return ""
	case *ir.Convert:
		return g.typ(e.T) + "(" + g.top(e.X) + ")"
	case *ir.Select:
		switch e.X.Type().(type) {
		case ir.Scalar, ir.Vector:
		default:
			g.fail("select of %v is not supported in HLSL", e.X.Type())
		}
		// In SM5 ?: evaluates both operands and, for a vector condition,
		// selects per component.
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

// isZero reports whether e is a zero constant or a Construct of zeros.
func isZero(e ir.Expr) bool {
	switch e := e.(type) {
	case *ir.Const:
		return e.Value == 0 && !math.Signbit(e.Value)
	case *ir.Construct:
		for _, a := range e.Args {
			if !isZero(a) {
				return false
			}
		}
		return true
	}
	return false
}

// builtins maps sl functions to HLSL intrinsics with the same argument
// order. Entries with splat set cast the last (scalar) argument to the
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
	"Mix":        {"lerp", 3, false},
	"Step":       {"step", 2, false},
	"Smoothstep": {"smoothstep", 3, false},
	"Sqrt":       {"sqrt", 1, false},
	"Floor":      {"floor", 1, false},
	"Fract":      {"frac", 1, false},
	"Pow":        {"pow", 2, false},
	"Length":     {"length", 1, false},
	"Dot":        {"dot", 2, false},
	"AbsV2":      {"abs", 1, false},
	"MaxV2":      {"max", 2, true},
	"MinV2":      {"min", 2, true},
	"MixV4":      {"lerp", 3, true},
}

func (g *gen) call(e *ir.Call) string {
	if e.Func != nil {
		name, ok := g.funcName[e.Func]
		if !ok {
			g.fail("call to %s, which is not a helper function", e.Func.Name)
			return ""
		}
		return name + "(" + g.exprs(e.Args) + ")"
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
		return g.globalRef(tex.Name) + ".SampleLevel(" + g.sampler[tex.Name] + ", " + g.top(e.Args[1]) + ", 0)"
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
		args[last] = "(" + g.typ(e.Type()) + ")(" + args[last] + ")"
	}
	return b.name + "(" + strings.Join(args, ", ") + ")"
}

// constLit renders a constant as an HLSL literal. f32 values always carry
// a decimal point or exponent and an f suffix, u32 a u suffix; negative
// values are parenthesized. A vector constant is a splat, written as a
// cast because HLSL vector constructors take every component.
func constLit(c *ir.Const) string {
	switch t := c.T.(type) {
	case ir.Vector:
		return "((" + scalarNames[t.Elem] + strconv.Itoa(t.N) + ")" + constLit(&ir.Const{T: t.Elem, Value: c.Value}) + ")"
	case ir.Scalar:
		v := c.Value
		var s string
		switch t {
		case ir.F32:
			switch {
			case math.IsNaN(v):
				return "asfloat(0x7FC00000u)"
			case math.IsInf(v, 1):
				return "asfloat(0x7F800000u)"
			case math.IsInf(v, -1):
				return "asfloat(0xFF800000u)"
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
	if isReserved(cand) {
		cand = base + "_"
	}
	for i := 1; isReserved(cand) || s.taken(cand); i++ {
		cand = base + "_" + strconv.Itoa(i)
	}
	s.used[cand] = true
	return cand
}

// sanitize maps a Go identifier to an ASCII identifier that does not use
// the reserved forms __x and _X.
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

func isReserved(name string) bool {
	// Names starting with SV_ are not reserved: system-value semantics only
	// appear after ":", so a variable or field SV_Pos is legal.
	return reserved[name] || reservedFold[strings.ToLower(name)]
}

// reservedFold holds the effect-framework keywords, which fxc matches
// regardless of case (lower-cased here).
var reservedFold = func() map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(`blendstate compile compile_fragment computeshader
		depthstencilstate depthstencilview domainshader geometryshader hullshader
		pass pixelshader pixelfragment vertexfragment rasterizerstate
		rendertargetview sampler sampler1d sampler2d sampler3d samplercube
		sampler_state samplerstate samplercomparisonstate stateblock
		stateblock_state technique technique10 technique11 texture texture1d
		texture2d texture3d texturecube vertexshader string vector matrix fxgroup
		compileshader constructgswithso texture1darray texture2darray
		texture2dms texture2dmsarray texturecubearray`) {
		m[w] = true
	}
	return m
}()

// reserved holds identifiers the generated source must not declare: HLSL
// keywords and reserved words, type names, intrinsic functions and the
// names this backend generates itself.
var reserved = func() map[string]bool {
	m := map[string]bool{}
	for _, list := range []string{
		// Keywords.
		`AppendStructuredBuffer asm asm_fragment bool break Buffer ByteAddressBuffer
		case cbuffer centroid class column_major const ConsumeStructuredBuffer
		continue default discard do double dword else export extern false float
		for groupshared half if in inline inout InputPatch int interface line
		lineadj linear LineStream namespace nointerpolation noperspective NULL out
		OutputPatch packoffset point PointStream precise return register row_major
		RWBuffer RWByteAddressBuffer RWStructuredBuffer RWTexture1D
		RWTexture1DArray RWTexture2D RWTexture2DArray RWTexture3D sample shared
		snorm static struct switch StructuredBuffer tbuffer true typedef triangle
		triangleadj TriangleStream uint uniform unorm unsigned void volatile while
		Texture1DArray Texture2DArray Texture2DMS Texture2DMSArray TextureCubeArray
		RasterizerOrderedBuffer RasterizerOrderedTexture2D ConstantBuffer
		globallycoherent min16float min10float min16int min12int min16uint
		SamplerComparisonState this template operator export`,
		// Reserved words.
		`auto catch char const_cast delete dynamic_cast enum explicit friend goto
		long mutable new private protected public reinterpret_cast short signed
		sizeof static_cast throw try typename union using virtual main`,
		// Intrinsic functions.
		`abort abs acos all AllMemoryBarrier AllMemoryBarrierWithGroupSync any
		asdouble asfloat asin asint asuint atan atan2 ceil CheckAccessFullyMapped
		clamp clip cos cosh countbits cross D3DCOLORtoUBYTE4 ddx ddx_coarse
		ddx_fine ddy ddy_coarse ddy_fine degrees determinant DeviceMemoryBarrier
		DeviceMemoryBarrierWithGroupSync distance dot dst errorf
		EvaluateAttributeAtCentroid EvaluateAttributeAtSample
		EvaluateAttributeSnapped exp exp2 f16tof32 f32tof16 faceforward
		firstbithigh firstbitlow floor fma fmod frac frexp fwidth
		GetRenderTargetSampleCount GetRenderTargetSamplePosition
		GroupMemoryBarrier GroupMemoryBarrierWithGroupSync InterlockedAdd
		InterlockedAnd InterlockedCompareExchange InterlockedCompareStore
		InterlockedExchange InterlockedMax InterlockedMin InterlockedOr
		InterlockedXor isfinite isinf isnan ldexp length lerp lit log log10 log2
		mad max min modf msad4 mul noise normalize pow printf
		Process2DQuadTessFactorsAvg Process2DQuadTessFactorsMax
		Process2DQuadTessFactorsMin ProcessIsolineTessFactors
		ProcessQuadTessFactorsAvg ProcessQuadTessFactorsMax
		ProcessQuadTessFactorsMin ProcessTriTessFactorsAvg
		ProcessTriTessFactorsMax ProcessTriTessFactorsMin radians rcp reflect
		refract reversebits round rsqrt saturate sign sin sincos sinh smoothstep
		sqrt step tan tanh tex1D tex1Dbias tex1Dgrad tex1Dlod tex1Dproj tex2D
		tex2Dbias tex2Dgrad tex2Dlod tex2Dproj tex3D tex3Dbias tex3Dgrad tex3Dlod
		tex3Dproj texCUBE texCUBEbias texCUBEgrad texCUBElod texCUBEproj transpose
		trunc select and or`,
		// Names generated by this backend.
		VertexEntry + " " + FragmentEntry,
	} {
		for _, w := range strings.Fields(list) {
			m[w] = true
		}
	}
	for _, base := range []string{"bool", "int", "uint", "dword", "half", "float", "double",
		"min16float", "min10float", "min16int", "min12int", "min16uint",
		"int16_t", "uint16_t", "int32_t", "uint32_t", "int64_t", "uint64_t",
		"float16_t", "float32_t", "float64_t"} {
		m[base] = true
		for n := 1; n <= 4; n++ {
			v := base + strconv.Itoa(n)
			m[v] = true
			for k := 1; k <= 4; k++ {
				m[v+"x"+strconv.Itoa(k)] = true
			}
		}
	}
	return m
}()
