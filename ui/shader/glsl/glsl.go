// Package glsl generates GLSL source from a shader ir.Module, for OpenGL
// 3.3 core (#version 330 core) or, as a dialect of the same output, OpenGL
// ES 3.0 (#version 300 es, which additionally declares highp default
// precision for float, int and sampler2D). Generate returns two sources,
// one per stage, each with its own main; the host compiles them as
// GL_VERTEX_SHADER and GL_FRAGMENT_SHADER and links them into one program.
// The output uses nothing beyond GLSL 3.30 and GLSL ES 3.00: no
// layout(binding), no explicit uniform locations, no arrays of arrays.
//
// # Interface mapping
//
// The IR entry points keep their (legalized) names; each stage's main
// copies the stage inputs into the entry point's parameters and its result
// into the stage outputs. Bindings map to OpenGL as follows:
//
//   - each field of the vertex input struct with location N is the
//     attribute "layout(location = N) in T a_<field>". The host enables
//     attribute N with glVertexAttribDivisor(N, 1) (per-instance), the Go
//     struct's stride and field offset, and glVertexAttribIPointer for
//     integer fields (glVertexAttribPointer, not normalized, for float).
//   - the vertex index parameter is uint(gl_VertexID).
//   - every field of the vertex output struct except the position is a
//     varying named v_<field>: "out" in the vertex shader, "in" in the
//     fragment shader. flat fields, and every integer field (GLSL cannot
//     interpolate them), are flat; bool varyings are not supported. The
//     varyings link by name, so their locations do not matter.
//   - the position field is written to gl_Position (see Coordinates); in
//     the fragment shader it reads gl_FragCoord.
//   - the fragment result is "layout(location = 0) out vec4 o_color".
//   - uniform binding N is the block "layout(std140) uniform <Name>_ub
//     { T Name; };" (no instance name, so expressions keep the Name.Field
//     form). GLSL 3.30 has no layout(binding), so the host sets
//     glUniformBlockBinding(prog, glGetUniformBlockIndex(prog,
//     "<Name>_ub"), N) after linking and binds the buffer with
//     glBindBufferRange(GL_UNIFORM_BUFFER, N, ...). The front end checks
//     that the Go layout of T equals std140; the host allocates the buffer
//     with the Go size rounded up to 16 bytes.
//   - texture binding N is "uniform sampler2D <name>;"; the host sets
//     glUniform1i(glGetUniformLocation(prog, "<name>"), N) after linking
//     and binds the texture to unit N. Filtering (nearest or linear, per
//     gen.QuadTextures) and clamp-to-edge wrapping are texture or sampler
//     object state set by the host; the shader does not carry them.
//
// Block and sampler names are the uniform's and texture's legalized names
// (see Names), so the host can look them up by the Go names unless those
// are GLSL reserved words. Every uniform block and sampler is declared in
// both stages (a block must be declared identically wherever it appears);
// a stage that does not use one may let the linker drop it, which the host
// must tolerate (an index of GL_INVALID_INDEX or a location of -1).
//
// Each stage contains every struct, but only the helper functions its
// entry point reaches. Generate relies on ir.Module.Funcs being ordered
// callees-first (as the front end produces it, and as the MSL and HLSL
// backends rely on too): GLSL needs every function declared before its
// first call, and the functions are emitted in module order without
// prototypes.
//
// # Expressions
//
//   - sl Sample always becomes textureLod(tex, uv, 0.0): the textures have
//     one mip level, and implicit derivatives are undefined in non-uniform
//     control flow.
//   - GLSL has no erf; sl.Erf becomes sl_erf, a helper evaluating the same
//     Abramowitz–Stegun 7.1.26 polynomial as sl.Erf and the MSL and HLSL
//     backends, emitted only in a stage that calls it.
//   - sl.Fract becomes fract and sl.Mix mix; other builtins have the same
//     lower-case name. GLSL has the (vector, scalar) overloads of min, max
//     and mix that MinV2, MaxV2 and MixV4 need.
//   - A vector splat is the constructor vecN(x).
//   - Comparisons of vectors, which yield a bool vector in the IR, become
//     lessThan, lessThanEqual, greaterThan, greaterThanEqual, equal and
//     notEqual (GLSL's == on vectors yields one bool); ! on a bool vector
//     becomes not.
//   - A struct or array Construct is the GLSL constructor S(...) or
//     T[N](...), and a zero value is a constructor of zero values.
//   - A Select with a scalar condition is the ?: operator. With a
//     bool-vector condition it picks per component: mix(y, x, cond) for
//     float vectors, and for integer or bool vectors (mix has no such
//     overloads before GLSL 4.50 and GLSL ES 3.10) a constructor of
//     per-component ?: operators. Operands may be evaluated more than once
//     or both evaluated, which is unobservable since IR expressions have
//     no side effects.
//   - Integer / truncates toward zero in GLSL, as in Go, so it is the plain
//     operator.
//
// # Names
//
// Identifiers are legalized as in the MSL and HLSL backends, against GLSL
// keywords and reserved words (of GLSL 3.30, GLSL ES 3.00 and later
// versions, so that the output stays valid for both dialects), type names
// and built-in functions, plus the generated names main and o_color.
// Identifiers starting with gl_ or GL_ (reserved, or macros such as GL_ES),
// starting with an underscore, or containing two consecutive underscores
// (reserved in GLSL) are rewritten first. Struct, helper, uniform, block,
// sampler, attribute (a_*), varying (v_*) and generated helper names share
// the program scope; parameters and locals of each function, including
// main, live in a nested scope.
//
// # Coordinates
//
// The IR position output is in clip space with x right, y up and z in
// 0..1 (Direct3D's and Metal's convention). The GL device stores its
// render targets with row 0 at the top, so that texture coordinates (top-
// left origin in the IR) and the viewport, scissor and read-back
// rectangles all use the frame's top-left coordinates unchanged; for that
// the vertex shader writes
//
//	gl_Position = vec4(p.x, -p.y, 2.0 * p.z - p.w, p.w)
//
// (y flipped, depth 0..1 mapped to OpenGL's -1..1). Texture coordinates are
// emitted unchanged. In the fragment shader the position field is
// gl_FragCoord, whose y then also counts from the top row like Metal's
// [[position]].
package glsl

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/tangoui/tango/ui/shader/ir"
)

// Dialect selects the GLSL version of the output.
type Dialect int

const (
	GLSL330 Dialect = iota // OpenGL 3.3 core: #version 330 core
	ES300                  // OpenGL ES 3.0: #version 300 es
)

func (d Dialect) String() string {
	switch d {
	case GLSL330:
		return "GLSL330"
	case ES300:
		return "ES300"
	}
	return fmt.Sprintf("Dialect(%d)", int(d))
}

// Names of the generated stage interface.
const (
	fragOutput    = "o_color" // the fragment shader's color output
	attrPrefix    = "a_"
	varyingPrefix = "v_"
	blockSuffix   = "_ub"
)

// Generate returns the vertex and fragment shader sources for m in dialect
// d.
func Generate(m *ir.Module, d Dialect) (vert, frag string, err error) {
	if d != GLSL330 && d != ES300 {
		return "", "", fmt.Errorf("glsl: unknown dialect %v", d)
	}
	if m == nil || m.Vertex == nil || m.Fragment == nil {
		return "", "", fmt.Errorf("glsl: module needs vertex and fragment entry points")
	}
	g := &gen{
		m:          m,
		d:          d,
		global:     newScope(nil),
		structName: map[*ir.Struct]string{},
		fieldName:  map[*ir.Struct]map[string]string{},
		funcName:   map[*ir.Func]string{},
		globalName: map[string]string{},
		block:      map[string]string{},
		texture:    map[string]*ir.TextureVar{},
		attr:       map[string]string{},
		varying:    map[string]string{},
	}
	if err := g.assignRoles(); err != nil {
		return "", "", err
	}
	g.declareGlobals()
	vert = g.stage(m.Vertex)
	frag = g.stage(m.Fragment)
	if g.err != nil {
		return "", "", g.err
	}
	return vert, frag, nil
}

type gen struct {
	m      *ir.Module
	d      Dialect
	b      strings.Builder
	indent int
	err    error

	in, out *ir.Struct // the vertex input and output structs

	global     *scope
	structName map[*ir.Struct]string
	fieldName  map[*ir.Struct]map[string]string
	funcName   map[*ir.Func]string
	globalName map[string]string // uniform or texture IR name -> GLSL name
	block      map[string]string // uniform IR name -> block name
	texture    map[string]*ir.TextureVar
	attr       map[string]string // vertex input field -> attribute name
	varying    map[string]string // vertex output field -> varying name

	local map[*ir.Var]string // names in the function being emitted

	erfName string // the generated erf helper's name; "" when sl.Erf is not called
}

func (g *gen) fail(format string, args ...any) {
	if g.err == nil {
		g.err = fmt.Errorf("glsl: "+format, args...)
	}
}

// assignRoles finds the vertex input and output structs and checks the
// entry-point signatures and stage attributes.
func (g *gen) assignRoles() error {
	v, f := g.m.Vertex, g.m.Fragment
	if len(v.Params) != 1 {
		return fmt.Errorf("glsl: vertex entry %s must take one struct input", v.Name)
	}
	in, ok := v.Params[0].Type.(ir.StructRef)
	if !ok || in.S == nil {
		return fmt.Errorf("glsl: vertex entry %s must take one struct input", v.Name)
	}
	out, ok := v.Result.(ir.StructRef)
	if !ok || out.S == nil {
		return fmt.Errorf("glsl: vertex entry %s must return a struct", v.Name)
	}
	if in.S == out.S {
		return fmt.Errorf("glsl: struct %s cannot be both vertex input and output", in.S.Name)
	}
	g.in, g.out = in.S, out.S
	if len(f.Params) > 1 {
		return fmt.Errorf("glsl: fragment entry %s must take at most one input", f.Name)
	}
	if len(f.Params) == 1 {
		if r, ok := f.Params[0].Type.(ir.StructRef); !ok || r.S != out.S {
			return fmt.Errorf("glsl: fragment input must be the vertex output struct %s", out.S.Name)
		}
	}
	if f.Result != (ir.Vector{Elem: ir.F32, N: 4}) {
		return fmt.Errorf("glsl: fragment entry %s must return vec4<f32>", f.Name)
	}
	for _, fl := range in.S.Fields {
		if fl.Attr.Location < 0 {
			return fmt.Errorf("glsl: vertex input field %s.%s has no location", in.S.Name, fl.Name)
		}
		if !isScalarOrVector(fl.Type) || elem(fl.Type) == ir.Bool {
			return fmt.Errorf("glsl: vertex input field %s.%s has unsupported type %v", in.S.Name, fl.Name, fl.Type)
		}
	}
	positions := 0
	for _, fl := range out.S.Fields {
		switch {
		case fl.Attr.Builtin == "position":
			positions++
			if fl.Type != (ir.Vector{Elem: ir.F32, N: 4}) {
				return fmt.Errorf("glsl: position field %s.%s must be vec4<f32>", out.S.Name, fl.Name)
			}
		case fl.Attr.Builtin != "":
			return fmt.Errorf("glsl: field %s.%s: unknown builtin %q", out.S.Name, fl.Name, fl.Attr.Builtin)
		case fl.Attr.Location < 0:
			return fmt.Errorf("glsl: vertex output field %s.%s has no location", out.S.Name, fl.Name)
		case !isScalarOrVector(fl.Type) || elem(fl.Type) == ir.Bool:
			return fmt.Errorf("glsl: vertex output field %s.%s has unsupported type %v (GLSL varyings cannot be bool)", out.S.Name, fl.Name, fl.Type)
		}
	}
	if positions != 1 {
		return fmt.Errorf("glsl: vertex output struct %s needs exactly one position field", out.S.Name)
	}
	return nil
}

func isScalarOrVector(t ir.Type) bool {
	switch t.(type) {
	case ir.Scalar, ir.Vector:
		return true
	}
	return false
}

// elem returns the scalar type of a scalar or vector type.
func elem(t ir.Type) ir.Scalar {
	if v, ok := t.(ir.Vector); ok {
		return v.Elem
	}
	s, _ := t.(ir.Scalar)
	return s
}

func (g *gen) allFuncs() []*ir.Func {
	return append(append([]*ir.Func{}, g.m.Funcs...), g.m.Vertex, g.m.Fragment)
}

// declareGlobals names everything in the program scope, in a fixed order so
// that renaming is deterministic and the same in both stages.
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
	g.funcName[g.m.Vertex] = g.global.declare(g.m.Vertex.Name)
	g.funcName[g.m.Fragment] = g.global.declare(g.m.Fragment.Name)
	for _, u := range g.m.Uniforms {
		g.globalName[u.Name] = g.global.declare(u.Name)
	}
	for _, t := range g.m.Textures {
		g.texture[t.Name] = t
		g.globalName[t.Name] = g.global.declare(t.Name)
	}
	for _, u := range g.m.Uniforms {
		g.block[u.Name] = g.global.declare(g.globalName[u.Name] + blockSuffix)
	}
	for _, f := range g.in.Fields {
		g.attr[f.Name] = g.global.declare(attrPrefix + f.Name)
	}
	for _, f := range g.out.Fields {
		if f.Attr.Builtin == "" {
			g.varying[f.Name] = g.global.declare(varyingPrefix + f.Name)
		}
	}
	if callsBuiltin(g.allFuncs(), "Erf") {
		g.erfName = g.global.declare("sl_erf")
	}
}

// callsBuiltin reports whether any of funcs calls the sl builtin name.
func callsBuiltin(funcs []*ir.Func, name string) bool {
	found := false
	for _, f := range funcs {
		walkStmts(f.Body, func(e ir.Expr) {
			if c, ok := e.(*ir.Call); ok && c.Func == nil && c.Builtin == name {
				found = true
			}
		})
	}
	return found
}

// reachable returns the helper functions entry calls, directly or
// indirectly, in module order.
func (g *gen) reachable(entry *ir.Func) []*ir.Func {
	used := map[*ir.Func]bool{}
	var visit func(f *ir.Func)
	visit = func(f *ir.Func) {
		walkStmts(f.Body, func(e ir.Expr) {
			if c, ok := e.(*ir.Call); ok && c.Func != nil && !used[c.Func] {
				used[c.Func] = true
				visit(c.Func)
			}
		})
	}
	visit(entry)
	var list []*ir.Func
	for _, f := range g.m.Funcs {
		if used[f] {
			list = append(list, f)
		}
	}
	return list
}

// ---- output ----

func (g *gen) line(format string, args ...any) {
	g.b.WriteString(strings.Repeat("    ", g.indent))
	fmt.Fprintf(&g.b, format, args...)
	g.b.WriteByte('\n')
}

// stage returns the source of the stage whose entry point is entry.
func (g *gen) stage(entry *ir.Func) string {
	g.b.Reset()
	g.indent = 0
	vertex := entry == g.m.Vertex
	stageName := "fragment"
	if vertex {
		stageName = "vertex"
	}
	switch g.d {
	case GLSL330:
		g.line("#version 330 core")
	case ES300:
		g.line("#version 300 es")
	}
	g.line("// Code generated by tango ui/shader/glsl. DO NOT EDIT.")
	g.line("//")
	g.line("// %s %s shader. Bind uniform block <Name>_ub to binding N with glUniformBlockBinding", map[Dialect]string{GLSL330: "GLSL 3.30 core", ES300: "GLSL ES 3.00"}[g.d], stageName)
	g.line("// and sampler <name> to texture unit N with glUniform1i (see ui/shader/glsl).")
	if vertex {
		g.line("// gl_Position flips y and maps depth 0..1 to -1..1: render targets keep row 0 at the top.")
	}
	if g.d == ES300 {
		g.line("")
		g.line("precision highp float;")
		g.line("precision highp int;")
		g.line("precision highp sampler2D;")
	}
	for _, s := range g.m.Structs {
		g.line("")
		g.structDecl(s)
	}
	for _, u := range g.m.Uniforms {
		g.line("")
		g.line("layout(std140) uniform %s {", g.block[u.Name])
		g.indent++
		g.line("%s %s;", g.structName[u.Type], g.globalName[u.Name])
		g.indent--
		g.line("}; // binding %d", u.Binding)
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
		g.line("uniform sampler2D %s; // binding %d, filter %s", g.globalName[t.Name], t.Binding, t.Filter)
	}
	g.line("")
	if vertex {
		for _, f := range g.in.Fields {
			g.line("layout(location = %d) in %s %s;", f.Attr.Location, g.typ(f.Type), g.attr[f.Name])
		}
	}
	dir := "in"
	if vertex {
		dir = "out"
	}
	for _, f := range g.out.Fields {
		if f.Attr.Builtin != "" {
			continue
		}
		mod := ""
		if f.Attr.Flat || elem(f.Type) != ir.F32 {
			mod = "flat "
		}
		g.line("%s%s %s %s;", mod, dir, g.typ(f.Type), g.varying[f.Name])
	}
	if !vertex {
		g.line("layout(location = 0) out vec4 %s;", fragOutput)
	}
	helpers := g.reachable(entry)
	if g.erfName != "" && callsBuiltin(append(helpers, entry), "Erf") {
		g.line("")
		g.erfDecl()
	}
	for _, f := range helpers {
		g.line("")
		g.funcDecl(f)
	}
	g.line("")
	g.funcDecl(entry)
	g.line("")
	if vertex {
		g.vertexMain()
	} else {
		g.fragmentMain()
	}
	return g.b.String()
}

// vertexMain emits the vertex shader's main: attributes into the input
// struct, the entry point's result into gl_Position and the varyings.
func (g *gen) vertexMain() {
	v := g.m.Vertex
	sc := newScope(g.global)
	in, r := sc.declare("in"), sc.declare("r")
	attrs := make([]string, len(g.in.Fields))
	for i, f := range g.in.Fields {
		attrs[i] = g.attr[f.Name]
	}
	args := in
	if v.VertexIDParam != nil {
		args += ", uint(gl_VertexID)"
	}
	g.line("void main() {")
	g.indent++
	g.line("%s %s = %s(%s);", g.structName[g.in], in, g.structName[g.in], strings.Join(attrs, ", "))
	g.line("%s %s = %s(%s);", g.structName[g.out], r, g.funcName[v], args)
	for _, f := range g.out.Fields {
		fn := r + "." + g.fieldName[g.out][f.Name]
		if f.Attr.Builtin == "position" {
			g.line("gl_Position = vec4(%[1]s.x, -%[1]s.y, 2.0 * %[1]s.z - %[1]s.w, %[1]s.w);", fn)
			continue
		}
		g.line("%s = %s;", g.varying[f.Name], fn)
	}
	g.indent--
	g.line("}")
}

// fragmentMain emits the fragment shader's main: the varyings (and
// gl_FragCoord) into the input struct, the entry point's result into
// o_color.
func (g *gen) fragmentMain() {
	f := g.m.Fragment
	g.line("void main() {")
	g.indent++
	if len(f.Params) == 0 {
		g.line("%s = %s();", fragOutput, g.funcName[f])
	} else {
		sc := newScope(g.global)
		in := sc.declare("in")
		vals := make([]string, len(g.out.Fields))
		for i, fl := range g.out.Fields {
			if fl.Attr.Builtin == "position" {
				vals[i] = "gl_FragCoord"
			} else {
				vals[i] = g.varying[fl.Name]
			}
		}
		g.line("%s %s = %s(%s);", g.structName[g.out], in, g.structName[g.out], strings.Join(vals, ", "))
		g.line("%s = %s(%s);", fragOutput, g.funcName[f], in)
	}
	g.indent--
	g.line("}")
}

// erfDecl emits the helper sl.Erf maps to (GLSL has no erf): the same
// Abramowitz–Stegun 7.1.26 polynomial, operation by operation, as sl.Erf.
func (g *gen) erfDecl() {
	c := func(v float64) string { return constLit(&ir.Const{T: ir.F32, Value: float64(float32(v))}) }
	g.line("float %s(float x) {", g.erfName)
	g.indent++
	g.line("float a = abs(x);")
	g.line("float t = 1.0 / (1.0 + %s * a);", c(0.3275911))
	g.line("float y = 1.0 - t * (%s + t * (%s + t * (%s + t * (%s + t * %s)))) * exp((-a) * a);",
		c(0.254829592), c(-0.284496736), c(1.421413741), c(-1.453152027), c(1.061405429))
	g.line("return (x < 0.0) ? (-y) : y;")
	g.indent--
	g.line("}")
}

func (g *gen) structDecl(s *ir.Struct) {
	g.line("struct %s {", g.structName[s])
	g.indent++
	for _, f := range s.Fields {
		g.line("%s;", g.decl(f.Type, g.fieldName[s][f.Name]))
	}
	g.indent--
	g.line("};")
}

// funcDecl emits a helper or an entry point (as an ordinary function).
func (g *gen) funcDecl(f *ir.Func) {
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
		params = append(params, "uint "+n)
	}
	walkDecls(f.Body, func(v *ir.Var) {
		if _, ok := g.local[v]; !ok {
			g.local[v] = sc.declare(v.Name)
		}
	})
	result := "void"
	if f.Result != nil {
		result = g.typ(f.Result)
	}
	g.line("%s %s(%s) {", result, g.funcName[f], strings.Join(params, ", "))
	g.indent++
	g.stmts(f.Body)
	g.indent--
	g.line("}")
}

// ---- types ----

var scalarNames = [...]string{ir.F32: "float", ir.I32: "int", ir.U32: "uint", ir.Bool: "bool"}
var vectorPrefix = [...]string{ir.F32: "vec", ir.I32: "ivec", ir.U32: "uvec", ir.Bool: "bvec"}

// typ returns the GLSL name of a type; an array is T[N] (GLSL 3.30 has no
// arrays of arrays).
func (g *gen) typ(t ir.Type) string {
	switch t := t.(type) {
	case ir.Scalar:
		if int(t) >= 0 && int(t) < len(scalarNames) {
			return scalarNames[t]
		}
	case ir.Vector:
		if int(t.Elem) >= 0 && int(t.Elem) < len(vectorPrefix) && t.N >= 2 && t.N <= 4 {
			return vectorPrefix[t.Elem] + strconv.Itoa(t.N)
		}
	case ir.StructRef:
		if n, ok := g.structName[t.S]; ok {
			return n
		}
	case ir.Array:
		if _, ok := t.Elem.(ir.Array); ok {
			g.fail("array of arrays %v is not supported in GLSL 3.30", t)
			return "void"
		}
		return g.typ(t.Elem) + "[" + strconv.Itoa(t.Len) + "]"
	case ir.Texture2D:
		return "sampler2D"
	}
	g.fail("unsupported type %v", t)
	return "void"
}

// decl returns the declaration of name with type t: "float x", or
// "float x[4]" for arrays.
func (g *gen) decl(t ir.Type, name string) string {
	if a, ok := t.(ir.Array); ok {
		if _, ok := a.Elem.(ir.Array); ok {
			g.fail("array of arrays %v is not supported in GLSL 3.30", t)
		}
		return g.typ(a.Elem) + " " + name + "[" + strconv.Itoa(a.Len) + "]"
	}
	return g.typ(t) + " " + name
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
		init := g.zero(s.V.Type)
		if s.Init != nil {
			init = g.top(s.Init)
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

// zero renders the zero value of t.
func (g *gen) zero(t ir.Type) string {
	switch t := t.(type) {
	case ir.Scalar, ir.Vector:
		return constLit(&ir.Const{T: t})
	case ir.StructRef:
		if t.S == nil {
			break
		}
		parts := make([]string, len(t.S.Fields))
		for i, f := range t.S.Fields {
			parts[i] = g.zero(f.Type)
		}
		return g.typ(t) + "(" + strings.Join(parts, ", ") + ")"
	case ir.Array:
		parts := make([]string, t.Len)
		for i := range parts {
			parts[i] = g.zero(t.Elem)
		}
		return g.typ(t) + "(" + strings.Join(parts, ", ") + ")"
	}
	g.fail("no zero value for type %v", t)
	return "0"
}

// ---- expressions ----

// compareFuncs are the GLSL functions for comparisons of vectors.
var compareFuncs = map[string]string{
	"<": "lessThan", "<=": "lessThanEqual", ">": "greaterThan", ">=": "greaterThanEqual",
	"==": "equal", "!=": "notEqual",
}

// parenthesized reports whether expr renders e as an operator expression
// enclosed in parentheses, which top may strip.
func parenthesized(e ir.Expr) bool {
	switch e := e.(type) {
	case *ir.Binary:
		_, vec := e.X.Type().(ir.Vector)
		return !(vec && compareFuncs[e.Op] != "")
	case *ir.Select:
		_, vec := e.Cond.Type().(ir.Vector)
		return !vec
	}
	return false
}

// top renders e without the outer parentheses of a binary or select
// expression; it is used where the context already delimits the expression.
func (g *gen) top(e ir.Expr) string {
	s := g.expr(e)
	if parenthesized(e) && len(s) >= 2 {
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
		if _, ok := e.X.Type().(ir.Vector); ok {
			if fn := compareFuncs[e.Op]; fn != "" {
				return fn + "(" + g.top(e.X) + ", " + g.top(e.Y) + ")"
			}
		}
		if vt, ok := e.Type().(ir.Vector); ok && vt.Elem == ir.Bool && (e.Op == "&&" || e.Op == "||") {
			g.fail("operator %s on bool vectors is not supported", e.Op)
		}
		return "(" + g.expr(e.X) + " " + e.Op + " " + g.expr(e.Y) + ")"
	case *ir.Unary:
		if _, ok := e.X.Type().(ir.Vector); ok && e.Op == "!" {
			return "not(" + g.top(e.X) + ")"
		}
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
		case ir.Vector, ir.Array:
			return g.typ(t) + "(" + g.exprs(e.Args) + ")"
		case ir.StructRef:
			if len(e.Args) == 0 {
				return g.zero(t)
			}
			if t.S != nil && len(e.Args) == len(t.S.Fields) {
				return g.typ(t) + "(" + g.exprs(e.Args) + ")"
			}
		}
		g.fail("unsupported construct of %v", e.T)
		return ""
	case *ir.Convert:
		return g.typ(e.T) + "(" + g.top(e.X) + ")"
	case *ir.Select:
		return g.selectExpr(e)
	}
	g.fail("unsupported expression %T", e)
	return ""
}

// selectExpr renders a Select: ?: for a scalar condition; per component
// for a bool-vector condition.
func (g *gen) selectExpr(e *ir.Select) string {
	if _, ok := e.X.Type().(ir.Array); ok {
		g.fail("select of %v is not supported in GLSL", e.X.Type())
		return ""
	}
	cv, ok := e.Cond.Type().(ir.Vector)
	if !ok {
		return "(" + g.expr(e.Cond) + " ? " + g.expr(e.X) + " : " + g.expr(e.Y) + ")"
	}
	vt, ok := e.X.Type().(ir.Vector)
	if !ok || vt.N != cv.N {
		g.fail("select of %v with condition %v", e.X.Type(), cv)
		return ""
	}
	if vt.Elem == ir.F32 {
		// mix(y, x, c) picks x where c is true.
		return "mix(" + g.top(e.Y) + ", " + g.top(e.X) + ", " + g.top(e.Cond) + ")"
	}
	c, x, y := g.expr(e.Cond), g.expr(e.X), g.expr(e.Y)
	parts := make([]string, vt.N)
	for i := range parts {
		comp := "." + string("xyzw"[i])
		parts[i] = c + comp + " ? " + x + comp + " : " + y + comp
	}
	return g.typ(vt) + "(" + strings.Join(parts, ", ") + ")"
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

// builtins maps sl functions to GLSL built-in functions with the same
// argument order and count.
var builtins = map[string]struct {
	name  string
	nargs int
}{
	"Abs":        {"abs", 1},
	"Min":        {"min", 2},
	"Max":        {"max", 2},
	"Clamp":      {"clamp", 3},
	"Mix":        {"mix", 3},
	"Step":       {"step", 2},
	"Smoothstep": {"smoothstep", 3},
	"Sqrt":       {"sqrt", 1},
	"Floor":      {"floor", 1},
	"Fract":      {"fract", 1},
	"Pow":        {"pow", 2},
	"Length":     {"length", 1},
	"Dot":        {"dot", 2},
	"AbsV2":      {"abs", 1},
	"MaxV2":      {"max", 2},
	"MinV2":      {"min", 2},
	"MixV4":      {"mix", 3},
}

func (g *gen) call(e *ir.Call) string {
	if e.Func != nil {
		name, ok := g.funcName[e.Func]
		if !ok || e.Func == g.m.Vertex || e.Func == g.m.Fragment {
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
		return "textureLod(" + g.globalRef(tex.Name) + ", " + g.top(e.Args[1]) + ", 0.0)"
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
	return b.name + "(" + g.exprs(e.Args) + ")"
}

// constLit renders a constant as a GLSL literal. f32 values always carry a
// decimal point or exponent (no suffix), u32 a u suffix; negative values
// are parenthesized. A vector constant is a splat constructor.
func constLit(c *ir.Const) string {
	switch t := c.T.(type) {
	case ir.Vector:
		return vectorPrefix[t.Elem] + strconv.Itoa(t.N) + "(" + constLit(&ir.Const{T: t.Elem, Value: c.Value}) + ")"
	case ir.Scalar:
		v := c.Value
		var s string
		switch t {
		case ir.F32:
			switch {
			case math.IsNaN(v):
				return "uintBitsToFloat(0x7FC00000u)"
			case math.IsInf(v, 1):
				return "uintBitsToFloat(0x7F800000u)"
			case math.IsInf(v, -1):
				return "uintBitsToFloat(0xFF800000u)"
			}
			s = strconv.FormatFloat(v, 'g', -1, 32)
			if !strings.ContainsAny(s, ".e") {
				s += ".0"
			}
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
// (sanitized) if possible, else name+"_" for reserved words, else the first
// free name_1, name_2, ... (name1, name2, ... when name ends in "_", so
// that no "__" appears).
func (s *scope) declare(name string) string {
	base := sanitize(name)
	cand := base
	if isReserved(cand) {
		cand = base + "_"
	}
	sep := "_"
	if strings.HasSuffix(base, "_") {
		sep = ""
	}
	for i := 1; isReserved(cand) || s.taken(cand); i++ {
		cand = base + sep + strconv.Itoa(i)
	}
	s.used[cand] = true
	return cand
}

// sanitize maps a Go identifier to an ASCII identifier that GLSL does not
// reserve: non-ASCII letters become uXXXX, runs of underscores collapse to
// one ("__" anywhere is reserved), and a name starting with an underscore
// gets a "v" prefix and one starting with gl_ or GL_ a "v_" prefix.
func sanitize(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r == '_':
			if !strings.HasSuffix(b.String(), "_") {
				b.WriteRune(r)
			}
		case r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			fmt.Fprintf(&b, "u%04X", r)
		}
	}
	s := b.String()
	switch {
	case s == "":
		return "v"
	case strings.HasPrefix(s, "gl_") || strings.HasPrefix(s, "GL_"):
		return "v_" + s
	case s[0] == '_' || s[0] >= '0' && s[0] <= '9':
		return "v" + s
	}
	return s
}

func isReserved(name string) bool { return reserved[name] }

// reserved holds identifiers the generated source must not declare: GLSL
// keywords and reserved words, type names, built-in functions and the
// names this backend generates itself.
var reserved = func() map[string]bool {
	m := map[string]bool{}
	for _, list := range []string{
		// Keywords (GLSL 3.30, GLSL ES 3.00 and later versions).
		`attribute const uniform varying layout centroid flat smooth noperspective
		patch sample break continue do for while switch case default if else
		subroutine in out inout float double int void bool true false invariant
		precise discard return lowp mediump highp precision struct coherent
		volatile restrict readonly writeonly buffer shared atomic_uint uint
		sampler texture image`,
		// Reserved for future use.
		`common partition active asm class union enum typedef template this
		packed resource goto inline noinline public static extern external
		interface long short half fixed unsigned superp input output filter
		sizeof cast namespace using row_major demote`,
		// Built-in functions.
		`radians degrees sin cos tan asin acos atan sinh cosh tanh asinh acosh
		atanh pow exp log exp2 log2 sqrt inversesqrt abs sign floor trunc round
		roundEven ceil fract mod modf min max clamp mix step smoothstep isnan
		isinf floatBitsToInt floatBitsToUint intBitsToFloat uintBitsToFloat fma
		frexp ldexp packUnorm2x16 packSnorm2x16 packUnorm4x8 packSnorm4x8
		unpackUnorm2x16 unpackSnorm2x16 unpackUnorm4x8 unpackSnorm4x8
		packHalf2x16 unpackHalf2x16 packDouble2x32 unpackDouble2x32 length
		distance dot cross normalize ftransform faceforward reflect refract
		matrixCompMult outerProduct transpose determinant inverse lessThan
		lessThanEqual greaterThan greaterThanEqual equal notEqual any all not
		uaddCarry usubBorrow umulExtended imulExtended bitfieldExtract
		bitfieldInsert bitfieldReverse bitCount findLSB findMSB textureSize
		textureQueryLod textureQueryLevels textureSamples textureProj
		textureLod textureOffset texelFetch texelFetchOffset textureProjOffset
		textureLodOffset textureProjLod textureProjLodOffset textureGrad
		textureGradOffset textureProjGrad textureProjGradOffset textureGather
		textureGatherOffset textureGatherOffsets texture1D texture1DProj
		texture1DLod texture1DProjLod texture2D texture2DProj texture2DLod
		texture2DProjLod texture3D texture3DProj texture3DLod texture3DProjLod
		textureCube textureCubeLod shadow1D shadow2D shadow1DProj shadow2DProj
		shadow1DLod shadow2DLod shadow1DProjLod shadow2DProjLod dFdx dFdy fwidth
		dFdxFine dFdyFine dFdxCoarse dFdyCoarse fwidthFine fwidthCoarse
		interpolateAtCentroid interpolateAtSample interpolateAtOffset noise1
		noise2 noise3 noise4 EmitVertex EndPrimitive EmitStreamVertex
		EndStreamPrimitive barrier memoryBarrier groupMemoryBarrier`,
		// Names generated by this backend.
		"main " + fragOutput,
	} {
		for _, w := range strings.Fields(list) {
			m[w] = true
		}
	}
	for _, p := range []string{"vec", "ivec", "uvec", "bvec", "dvec", "hvec", "fvec"} {
		for n := 2; n <= 4; n++ {
			m[p+strconv.Itoa(n)] = true
		}
	}
	for _, p := range []string{"mat", "dmat"} {
		for n := 2; n <= 4; n++ {
			m[p+strconv.Itoa(n)] = true
			for k := 2; k <= 4; k++ {
				m[p+strconv.Itoa(n)+"x"+strconv.Itoa(k)] = true
			}
		}
	}
	for _, prefix := range []string{"", "i", "u"} {
		for _, s := range []string{"sampler1D", "sampler2D", "sampler3D", "samplerCube",
			"sampler1DArray", "sampler2DArray", "samplerCubeArray", "sampler2DRect",
			"samplerBuffer", "sampler2DMS", "sampler2DMSArray", "sampler3DRect",
			"image1D", "image2D", "image3D", "imageCube", "image1DArray", "image2DArray",
			"imageCubeArray", "image2DRect", "imageBuffer", "image2DMS", "image2DMSArray",
			"texture1D", "texture2D", "texture3D", "textureCube", "texture1DArray",
			"texture2DArray", "textureCubeArray", "texture2DRect", "textureBuffer",
			"texture2DMS", "texture2DMSArray", "subpassInput", "subpassInputMS"} {
			m[prefix+s] = true
		}
	}
	for _, s := range []string{"sampler1DShadow", "sampler2DShadow", "samplerCubeShadow",
		"sampler1DArrayShadow", "sampler2DArrayShadow", "samplerCubeArrayShadow",
		"sampler2DRectShadow", "samplerShadow"} {
		m[s] = true
	}
	return m
}()
