package glsl

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/shader/front"
	"github.com/tangoui/tango/ui/shader/ir"
)

var update = flag.Bool("update", false, "rewrite testdata/basic.vert and basic.frag")

func load(t *testing.T, dir string) *ir.Module {
	t.Helper()
	m, err := front.Load(dir)
	if err != nil {
		t.Fatalf("front.Load(%s): %v", dir, err)
	}
	return m
}

func generate(t *testing.T, m *ir.Module, d Dialect) (vert, frag string) {
	t.Helper()
	vert, frag, err := Generate(m, d)
	if err != nil {
		t.Fatalf("Generate(%v): %v", d, err)
	}
	return vert, frag
}

func TestGenerateBasic(t *testing.T) {
	vert, frag := generate(t, load(t, "../front/testdata/ok"), GLSL330)
	for _, f := range []struct{ golden, got string }{
		{"testdata/basic.vert", vert},
		{"testdata/basic.frag", frag},
	} {
		if *update {
			if err := os.WriteFile(f.golden, []byte(f.got), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		want, err := os.ReadFile(f.golden)
		if err != nil {
			t.Fatal(err)
		}
		if f.got != string(want) {
			t.Errorf("Generate output differs from %s (run with -update to rewrite)\n--- got ---\n%s", f.golden, f.got)
		}
	}
}

// TestGenerateQuad checks the bindings of the real quad shader in both
// dialects: one std140 block, five samplers, flat varyings and
// explicit-LOD sampling.
func TestGenerateQuad(t *testing.T) {
	m := load(t, "../../gpu/shaders")
	for _, d := range []Dialect{GLSL330, ES300} {
		t.Run(d.String(), func(t *testing.T) {
			vert, frag := generate(t, m, d)
			version := map[Dialect]string{GLSL330: "#version 330 core\n", ES300: "#version 300 es\n"}[d]
			for name, src := range map[string]string{"vert": vert, "frag": frag} {
				if !strings.HasPrefix(src, version) {
					t.Errorf("%s does not start with %q:\n%.80s", name, version, src)
				}
				precision := "precision highp float;\nprecision highp int;\nprecision highp sampler2D;\n"
				if got := strings.Contains(src, precision); got != (d == ES300) {
					t.Errorf("%s: precision statements present = %v", name, got)
				}
				for _, want := range []string{
					"layout(std140) uniform FrameU_ub {\n    Frame FrameU;\n}; // binding 0",
				} {
					if !strings.Contains(src, want) {
						t.Errorf("%s lacks %q", name, want)
					}
				}
				for i, tex := range []string{"Mask", "Color", "Ramp", "Img", "Layer"} {
					decl := fmt.Sprintf("uniform sampler2D %s; // binding %d", tex, i)
					if n := strings.Count(src, decl); n != 1 {
						t.Errorf("%s has %d × %q, want 1", name, n, decl)
					}
				}
				if n := strings.Count(src, "uniform sampler2D "); n != 5 {
					t.Errorf("%s has %d samplers, want 5", name, n)
				}
				if regexp.MustCompile(`\btexture\(`).MatchString(src) {
					t.Errorf("%s uses implicit-LOD texture(", name)
				}
			}
			for _, want := range []string{
				"layout(location = 0) in vec4 a_Rect;", "layout(location = 8) in vec4 a_Dash;",
				"out vec2 v_P;", "flat out vec4 v_Dash;",
				"Varyings r = VS(in_, uint(gl_VertexID));",
				"v_Dash = r.Dash;",
			} {
				if !strings.Contains(vert, want) {
					t.Errorf("vertex shader lacks %q", want)
				}
			}
			for _, want := range []string{
				"in vec2 v_P;", "flat in vec4 v_Dash;",
				"layout(location = 0) out vec4 o_color;",
				"Varyings in_ = Varyings(gl_FragCoord, v_P, v_Rect, ",
				"o_color = FS(in_);",
				"textureLod(Mask, ", "textureLod(Ramp, ",
				"float sl_erf(float x) {",
			} {
				if !strings.Contains(frag, want) {
					t.Errorf("fragment shader lacks %q", want)
				}
			}
			// Each stage has only the helpers its entry point reaches.
			if strings.Contains(vert, "sl_erf") || strings.Contains(vert, "textureLod(") {
				t.Errorf("vertex shader contains fragment-only helpers")
			}
			if strings.Contains(frag, "a_Rect") || strings.Contains(vert, "o_color") {
				t.Errorf("stage interface leaked into the other stage")
			}
		})
	}
}

// TestPositionTransform: the GL device keeps "row 0 = top" in its targets,
// so y is negated, and the IR's 0..1 depth maps to GL's -1..1.
func TestPositionTransform(t *testing.T) {
	vert, frag := generate(t, load(t, "../front/testdata/ok"), GLSL330)
	const want = "gl_Position = vec4(r.Pos.x, -r.Pos.y, 2.0 * r.Pos.z - r.Pos.w, r.Pos.w);"
	if !strings.Contains(vert, want) {
		t.Errorf("vertex shader lacks %q:\n%s", want, vert)
	}
	if strings.Contains(frag, "gl_Position") {
		t.Errorf("fragment shader writes gl_Position")
	}
}

// selectModule is a hand-built module using IR forms the front end does not
// produce: scalar and vector Select, vector comparisons, integer division
// and a non-zero struct literal.
func selectModule() *ir.Module {
	f32, v2, v4 := ir.F32, ir.Vector{Elem: ir.F32, N: 2}, ir.Vector{Elem: ir.F32, N: 4}
	i2 := ir.Vector{Elem: ir.I32, N: 2}
	in := &ir.Struct{Name: "In", Fields: []ir.Field{
		{Name: "P", Type: v2, Attr: ir.Attr{Location: 0}},
		{Name: "N", Type: ir.I32, Attr: ir.Attr{Location: 1}},
	}}
	out := &ir.Struct{Name: "Out", Fields: []ir.Field{
		{Name: "Pos", Type: v4, Attr: ir.Attr{Builtin: "position", Location: -1}},
		{Name: "P", Type: v2, Attr: ir.Attr{Location: 0}},
		{Name: "N", Type: ir.I32, Attr: ir.Attr{Location: 1}},
	}}
	vin, vid := &ir.Var{Name: "in", Type: ir.StructRef{S: in}}, &ir.Var{Name: "vid", Type: ir.U32}
	o := &ir.Var{Name: "o", Type: ir.StructRef{S: out}}
	p := &ir.FieldExpr{X: &ir.VarRef{V: vin}, Name: "P"}
	n := &ir.FieldExpr{X: &ir.VarRef{V: vin}, Name: "N"}
	vs := &ir.Func{Name: "vs", Params: []*ir.Var{vin}, VertexIDParam: vid, Result: ir.StructRef{S: out}, Body: []ir.Stmt{
		&ir.Decl{V: o, Init: &ir.Construct{T: ir.StructRef{S: out}, Args: []ir.Expr{
			&ir.Construct{T: v4, Args: []ir.Expr{&ir.FieldExpr{X: p, Name: "x"}, &ir.Const{T: f32, Value: -1}, &ir.Const{T: f32}, &ir.Const{T: f32, Value: 1}}},
			p,
			&ir.Binary{Op: "/", X: n, Y: &ir.Const{T: ir.I32, Value: -3}},
		}}},
		&ir.Return{Value: &ir.VarRef{V: o}},
	}}
	fin := &ir.Var{Name: "in", Type: ir.StructRef{S: out}}
	q := &ir.FieldExpr{X: &ir.VarRef{V: fin}, Name: "P"}
	half := &ir.Const{T: v2, Value: 0.5}
	pick := &ir.Var{Name: "pick", Type: v2}
	ip := &ir.Var{Name: "ip", Type: i2}
	ivec := &ir.Construct{T: i2, Args: []ir.Expr{&ir.FieldExpr{X: &ir.VarRef{V: fin}, Name: "N"}, &ir.Const{T: ir.I32, Value: 2}}}
	fs := &ir.Func{Name: "fs", Params: []*ir.Var{fin}, Result: v4, Body: []ir.Stmt{
		&ir.Decl{V: pick, Init: &ir.Select{Cond: &ir.Binary{Op: "<", X: q, Y: half}, X: q, Y: half}},
		&ir.Decl{V: ip, Init: &ir.Select{Cond: &ir.Binary{Op: "==", X: ivec, Y: &ir.Const{T: i2, Value: 2}}, X: ivec, Y: &ir.Const{T: i2}}},
		&ir.Return{Value: &ir.Select{
			Cond: &ir.Binary{Op: ">", X: &ir.FieldExpr{X: &ir.VarRef{V: pick}, Name: "x"}, Y: &ir.Const{T: f32, Value: 0.25}},
			X:    &ir.Const{T: v4, Value: 1},
			Y:    &ir.Construct{T: v4, Args: []ir.Expr{&ir.VarRef{V: pick}, &ir.Unary{Op: "-", X: &ir.FieldExpr{X: &ir.VarRef{V: pick}, Name: "y"}}, &ir.Convert{T: f32, X: &ir.FieldExpr{X: &ir.VarRef{V: ip}, Name: "x"}}}},
		}},
	}}
	return &ir.Module{Structs: []*ir.Struct{in, out}, Vertex: vs, Fragment: fs}
}

func TestSelect(t *testing.T) {
	vert, frag := generate(t, selectModule(), GLSL330)
	for _, want := range []string{
		// A bool-vector condition selects per component through mix.
		"vec2 pick = mix(vec2(0.5), in_.P, lessThan(in_.P, vec2(0.5)));",
		// mix has no integer overloads before GLSL 4.50, so integer vectors
		// select component by component.
		"ivec2 ip = ivec2(",
		"equal(",
		// A scalar condition is ?:.
		"return (pick.x > 0.25) ? vec4(1.0) : vec4(pick, (-pick.y), float(ip.x));",
		// Integer varyings are flat on both sides.
		"flat in int v_N;",
	} {
		if !strings.Contains(frag, want) {
			t.Errorf("fragment shader lacks %q:\n%s", want, frag)
		}
	}
	for _, want := range []string{
		"Out o = Out(vec4(in_.P.x, (-1.0), 0.0, 1.0), in_.P, in_.N / (-3));",
		"flat out int v_N;",
	} {
		if !strings.Contains(vert, want) {
			t.Errorf("vertex shader lacks %q:\n%s", want, vert)
		}
	}
}

// TestIntegerDivision: GLSL integer / truncates toward zero, as Go's does,
// so it is emitted as the plain operator (no floor or helper).
func TestIntegerDivision(t *testing.T) {
	vert, _ := generate(t, selectModule(), GLSL330)
	if !strings.Contains(vert, "in_.N / (-3)") {
		t.Errorf("integer division is not the plain / operator:\n%s", vert)
	}
}

// TestNamesLegalized checks that identifiers colliding with GLSL keywords,
// types, built-in functions and generated names are renamed.
func TestNamesLegalized(t *testing.T) {
	vert, frag := generate(t, load(t, "../msl/testdata/names"), GLSL330)
	for _, want := range []string{
		"struct vertex {", "struct thread {",
		"vec2 half_;", "float float_;", "vec4 sample_;",
		"uniform sampler2D texture_; // binding 0",
		"layout(std140) uniform constant_ub {",
		"float length_(vec2 half_, float sampler_) {",
		"vec4 main_(vec2 uv) {",
		"float sl_erf_1(float x) {",
		"vec4 texture_1 = ",
		"void main() {",
	} {
		if !strings.Contains(frag, want) {
			t.Errorf("fragment shader lacks %q:\n%s", want, frag)
		}
	}
	if !strings.Contains(vert, "layout(location = 0) in vec2 a_half;") {
		t.Errorf("vertex shader lacks the a_half attribute:\n%s", vert)
	}
}

func TestSanitize(t *testing.T) {
	for in, want := range map[string]string{
		"a__b":  "a_b",
		"gl_X":  "v_gl_X",
		"x":     "x",
		"π":     "u03C0",
		"_":     "v_",
		"__x":   "v_x",
		"a___b": "a_b",
	} {
		if got := sanitize(in); got != want {
			t.Errorf("sanitize(%q) = %q, want %q", in, got, want)
		}
	}
	sc := newScope(nil)
	for _, n := range []string{"x_", "x_", "in", "in_"} {
		if got := sc.declare(n); strings.Contains(got, "__") || isReserved(got) {
			t.Errorf("declare(%q) = %q", n, got)
		}
	}
}

func TestConstLiterals(t *testing.T) {
	for _, c := range []struct {
		e    *ir.Const
		want string
	}{
		{&ir.Const{T: ir.F32, Value: 1}, "1.0"},
		{&ir.Const{T: ir.F32, Value: -2.25}, "(-2.25)"},
		{&ir.Const{T: ir.F32, Value: 1e20}, "1e+20"},
		{&ir.Const{T: ir.U32, Value: 3}, "3u"},
		{&ir.Const{T: ir.I32, Value: -4}, "(-4)"},
		{&ir.Const{T: ir.Bool, Value: 1}, "true"},
		{&ir.Const{T: ir.Vector{Elem: ir.F32, N: 2}, Value: 0}, "vec2(0.0)"},
		{&ir.Const{T: ir.Vector{Elem: ir.U32, N: 3}, Value: 1}, "uvec3(1u)"},
	} {
		if got := constLit(c.e); got != c.want {
			t.Errorf("constLit(%v %v) = %q, want %q", c.e.T, c.e.Value, got, c.want)
		}
	}
}

// TestErfHelper: GLSL has no erf; sl.Erf calls a generated helper with the
// same polynomial as sl.Erf, emitted only in the stages that use it.
func TestErfHelper(t *testing.T) {
	vert, frag := generate(t, load(t, "../msl/testdata/exprs"), GLSL330)
	if n := strings.Count(frag, "float sl_erf(float x) {"); n != 1 {
		t.Fatalf("%d sl_erf helper definitions, want 1:\n%s", n, frag)
	}
	for _, want := range []string{"sl_erf(x)", "0.3275911", "1.0614054", "exp((-a) * a)"} {
		if !strings.Contains(frag, want) {
			t.Errorf("fragment shader lacks %q:\n%s", want, frag)
		}
	}
	if strings.Contains(vert, "sl_erf") {
		t.Errorf("sl_erf emitted in a vertex shader that does not call sl.Erf")
	}
}

// glslangPath returns the glslangValidator executable: $TANGO_GLSLANG, else
// glslangValidator on PATH.
func glslangPath() (string, bool) {
	if p := os.Getenv("TANGO_GLSLANG"); p != "" {
		if lp, err := exec.LookPath(p); err == nil {
			return lp, true
		}
		return p, true
	}
	p, err := exec.LookPath("glslangValidator")
	return p, err == nil
}

// TestGlslangValidates checks every generated shader, in both dialects and
// both stages, with the Khronos reference compiler when it is available
// (on PATH or at $TANGO_GLSLANG).
func TestGlslangValidates(t *testing.T) {
	tool, ok := glslangPath()
	if !ok {
		t.Skip("glslangValidator not found (install glslang-tools or set TANGO_GLSLANG)")
	}
	mods := map[string]*ir.Module{"select": selectModule()}
	for _, dir := range []string{"../../gpu/shaders", "../front/testdata/ok", "../front/testdata/scoping", "../msl/testdata/names", "../msl/testdata/exprs"} {
		mods[filepath.Base(dir)] = load(t, dir)
	}
	for name, m := range mods {
		for _, d := range []Dialect{GLSL330, ES300} {
			t.Run(name+"/"+d.String(), func(t *testing.T) {
				vert, frag := generate(t, m, d)
				dir := t.TempDir()
				for ext, src := range map[string]string{"vert": vert, "frag": frag} {
					path := filepath.Join(dir, name+"."+ext)
					if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
						t.Fatal(err)
					}
					if out, err := exec.Command(tool, path).CombinedOutput(); err != nil {
						t.Errorf("glslangValidator %s.%s: %v\n%s\n--- source ---\n%s", name, ext, err, out, src)
					}
				}
			})
		}
	}
}
