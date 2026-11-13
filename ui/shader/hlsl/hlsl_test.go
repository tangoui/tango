package hlsl

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/shader/front"
	"github.com/tangoui/tango/ui/shader/ir"
)

var update = flag.Bool("update", false, "rewrite testdata/basic.hlsl")

func generate(t *testing.T, dir string) string {
	t.Helper()
	m, err := front.Load(dir)
	if err != nil {
		t.Fatalf("front.Load(%s): %v", dir, err)
	}
	src, err := Generate(m)
	if err != nil {
		t.Fatalf("Generate(%s): %v", dir, err)
	}
	return src
}

func TestGenerateBasic(t *testing.T) {
	got := generate(t, "../front/testdata/ok")
	const golden = "testdata/basic.hlsl"
	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("Generate output differs from %s (run with -update to rewrite)\n--- got ---\n%s", golden, got)
	}
}

// TestGenerateQuad checks the bindings of the real quad shader: one
// cbuffer, five textures each with its sampler, flat varyings and
// derivative-free sampling.
func TestGenerateQuad(t *testing.T) {
	src := generate(t, "../../gpu/shaders")
	for _, want := range []string{
		"SampleLevel(", "nointerpolation ", "register(b0)",
		"float4 Pos : SV_Position;", "float2 P : TEXCOORD0;", "nointerpolation float4 Dash : TEXCOORD9;",
		"float4 Rect : TEXCOORD0;", "float4 Dash : TEXCOORD8;",
		"Varyings vs_main(Instance in_, uint vid : SV_VertexID) {",
		"float4 ps_main(Varyings in_) : SV_Target {",
		"cbuffer FrameU_cb : register(b0) {",
		"float sl_erf(float x) {",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("quad HLSL lacks %q", want)
		}
	}
	for i := range 5 {
		for _, r := range []string{fmt.Sprintf("register(t%d)", i), fmt.Sprintf("register(s%d)", i)} {
			if n := strings.Count(src, r); n != 1 {
				t.Errorf("quad HLSL has %d × %q, want 1", n, r)
			}
		}
	}
	for _, bad := range []string{".Sample(", "register(t5)", "register(s5)", "register(b1)"} {
		if strings.Contains(src, bad) {
			t.Errorf("quad HLSL contains %q", bad)
		}
	}
	// A texture's sampler is <name>_s, sampled at mip level 0.
	if !strings.Contains(src, "Mask.SampleLevel(Mask_s, ") || !strings.Contains(src, "Ramp.SampleLevel(Ramp_s, ") {
		t.Errorf("quad HLSL does not sample through <tex>_s")
	}
}

// dxcPath returns the dxc executable: $TANGO_DXC, else dxc on PATH.
func dxcPath() (string, bool) {
	if p := os.Getenv("TANGO_DXC"); p != "" {
		return p, true
	}
	p, err := exec.LookPath("dxc")
	return p, err == nil
}

// TestDXCCompiles compiles the committed ui/gpu/gen/quad.hlsl and the
// generated source of every test shader with DirectXShaderCompiler, when
// one is available (on PATH or at $TANGO_DXC). The device compiles with
// fxc (D3DCompile, shader model 5); dxc is the compiler that runs outside
// Windows, so the check uses -HV 2018, the HLSL version fxc implements
// (for example ?: on vectors is per-component there, removed in 2021).
func TestDXCCompiles(t *testing.T) {
	dxc, ok := dxcPath()
	if !ok {
		t.Skip("dxc not found (install DirectXShaderCompiler or set TANGO_DXC)")
	}
	srcs := map[string]string{}
	quad, err := os.ReadFile("../../gpu/gen/quad.hlsl")
	if err != nil {
		t.Fatal(err)
	}
	srcs["quad"] = string(quad)
	for _, dir := range []string{"../front/testdata/ok", "../front/testdata/scoping", "../msl/testdata/names", "../msl/testdata/exprs"} {
		srcs[filepath.Base(dir)] = generate(t, dir)
	}
	sel, err := Generate(selectModule())
	if err != nil {
		t.Fatal(err)
	}
	srcs["select"] = sel
	for name, src := range srcs {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name+".hlsl")
			if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
				t.Fatal(err)
			}
			for _, st := range [][2]string{{"vs_6_0", VertexEntry}, {"ps_6_0", FragmentEntry}} {
				out, err := exec.Command(dxc, "-T", st[0], "-E", st[1], "-HV", "2018", path).CombinedOutput()
				if err != nil {
					t.Errorf("dxc -T %s -E %s: %v\n%s\n--- source ---\n%s", st[0], st[1], err, diagnostics(out), src)
				} else if d := diagnostics(out); d != "" {
					t.Errorf("dxc -T %s -E %s reported diagnostics:\n%s", st[0], st[1], d)
				}
			}
		})
	}
}

// diagnostics returns the error and warning lines of dxc output (which
// otherwise is the disassembly).
func diagnostics(out []byte) string {
	var lines []string
	for _, l := range strings.Split(string(out), "\n") {
		if strings.Contains(l, "error") || strings.Contains(l, "warning") {
			lines = append(lines, l)
		}
	}
	return strings.Join(lines, "\n")
}

// TestNamesLegalized checks that identifiers colliding with HLSL keywords,
// types, intrinsics and generated names are renamed.
func TestNamesLegalized(t *testing.T) {
	src := generate(t, "../msl/testdata/names")
	for _, want := range []string{
		"struct vertex {", "struct thread {", // not reserved in HLSL
		"float2 half_ : TEXCOORD0;", "float float_ : TEXCOORD1;",
		"float min_;", "float2 max_;",
		"float4 sample_ : SV_Position;",
		"Texture2D<float4> texture_ : register(t0);", "SamplerState texture_s : register(s0);",
		"float length_(float2 half_, float sampler_) {",
		"float vs_main_(float x) {", "float4 main_(float2 uv) {",
		"vertex in_", "float4 texture_1 = ",
		// SV_ names are legal identifiers (semantics follow ":").
		"float sv_count = ", "float4 SV_Pos;", "float sv_blend(float x) {",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("output lacks %q:\n%s", want, src)
		}
	}
}

func TestIsReservedFoldsEffectKeywords(t *testing.T) {
	for _, n := range []string{"Texture", "SAMPLER", "Technique", "in", "out", "lerp", "float2x3", "CompileShader", "compileshader", "ConstructGSWithSO"} {
		if !isReserved(n) {
			t.Errorf("%s is not reserved", n)
		}
	}
	for _, n := range []string{"Clip", "Color", "Mask", "Lerp", "In", "SV_Pos", "sv_count", "SV_Target"} {
		if isReserved(n) {
			t.Errorf("%s is reserved", n)
		}
	}
	// declare must terminate and keep SV_ names (it used to loop forever
	// when a reserved prefix survived every suffix).
	sc := newScope(nil)
	for _, n := range []string{"sv_count", "SV_Pos", "SV_Pos"} {
		got := sc.declare(n)
		if !strings.HasPrefix(got, n) {
			t.Errorf("declare(%q) = %q", n, got)
		}
	}
}

func TestConstLiterals(t *testing.T) {
	for _, c := range []struct {
		e    *ir.Const
		want string
	}{
		{&ir.Const{T: ir.F32, Value: 1}, "1.0f"},
		{&ir.Const{T: ir.F32, Value: -2.25}, "(-2.25f)"},
		{&ir.Const{T: ir.F32, Value: 1e20}, "1e+20f"},
		{&ir.Const{T: ir.U32, Value: 3}, "3u"},
		{&ir.Const{T: ir.I32, Value: -4}, "(-4)"},
		{&ir.Const{T: ir.Bool, Value: 1}, "true"},
		{&ir.Const{T: ir.Vector{Elem: ir.F32, N: 2}, Value: 0}, "((float2)0.0f)"},
	} {
		if got := constLit(c.e); got != c.want {
			t.Errorf("constLit(%v %v) = %q, want %q", c.e.T, c.e.Value, got, c.want)
		}
	}
}

// selectModule is a hand-built module using IR forms the front end does not
// produce: scalar and vector Select, vector comparisons, a negative
// constant and a non-zero struct literal.
func selectModule() *ir.Module {
	f32, v2, v4 := ir.F32, ir.Vector{Elem: ir.F32, N: 2}, ir.Vector{Elem: ir.F32, N: 4}
	in := &ir.Struct{Name: "In", Fields: []ir.Field{{Name: "P", Type: v2, Attr: ir.Attr{Location: 0}}}}
	out := &ir.Struct{Name: "Out", Fields: []ir.Field{
		{Name: "Pos", Type: v4, Attr: ir.Attr{Builtin: "position", Location: -1}},
		{Name: "P", Type: v2, Attr: ir.Attr{Location: 0}},
	}}
	vin, vid := &ir.Var{Name: "in", Type: ir.StructRef{S: in}}, &ir.Var{Name: "vid", Type: ir.U32}
	o := &ir.Var{Name: "o", Type: ir.StructRef{S: out}}
	p := &ir.FieldExpr{X: &ir.VarRef{V: vin}, Name: "P"}
	vs := &ir.Func{Name: "vs", Params: []*ir.Var{vin}, VertexIDParam: vid, Result: ir.StructRef{S: out}, Body: []ir.Stmt{
		&ir.Decl{V: o, Init: &ir.Construct{T: ir.StructRef{S: out}, Args: []ir.Expr{
			&ir.Construct{T: v4, Args: []ir.Expr{&ir.FieldExpr{X: p, Name: "x"}, &ir.Const{T: f32, Value: -1}, &ir.Const{T: f32}, &ir.Const{T: f32, Value: 1}}},
			p,
		}}},
		&ir.Return{Value: &ir.VarRef{V: o}},
	}}
	fin := &ir.Var{Name: "in", Type: ir.StructRef{S: out}}
	q := &ir.FieldExpr{X: &ir.VarRef{V: fin}, Name: "P"}
	half := &ir.Const{T: v2, Value: 0.5}
	pick := &ir.Var{Name: "pick", Type: v2}
	fs := &ir.Func{Name: "fs", Params: []*ir.Var{fin}, Result: v4, Body: []ir.Stmt{
		&ir.Decl{V: pick, Init: &ir.Select{Cond: &ir.Binary{Op: "<", X: q, Y: half}, X: q, Y: half}},
		&ir.Return{Value: &ir.Select{
			Cond: &ir.Binary{Op: ">", X: &ir.FieldExpr{X: &ir.VarRef{V: pick}, Name: "x"}, Y: &ir.Const{T: f32, Value: 0.25}},
			X:    &ir.Const{T: v4, Value: 1},
			Y:    &ir.Construct{T: v4, Args: []ir.Expr{&ir.VarRef{V: pick}, &ir.Unary{Op: "-", X: &ir.FieldExpr{X: &ir.VarRef{V: pick}, Name: "y"}}, &ir.Const{T: f32, Value: 1}}},
		}},
	}}
	return &ir.Module{Structs: []*ir.Struct{in, out}, Vertex: vs, Fragment: fs}
}

func TestSelect(t *testing.T) {
	src, err := Generate(selectModule())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		// Per-component ?: with a bool2 condition (SM5 semantics).
		"float2 pick = (in_.P < ((float2)0.5f)) ? in_.P : ((float2)0.5f);",
		"return (pick.x > 0.25f) ? ((float4)1.0f) : float4(pick, (-pick.y), 1.0f);",
		"Out o = Out_ctor(float4(in_.P.x, (-1.0f), 0.0f, 1.0f), in_.P);",
		"Out Out_ctor(float4 Pos, float2 P) {",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("output lacks %q:\n%s", want, src)
		}
	}
}

// TestErfHelper: HLSL has no erf; sl.Erf calls a generated helper with the
// same polynomial, emitted only when used and renamed around a shader
// function of the same name.
func TestErfHelper(t *testing.T) {
	src := generate(t, "../msl/testdata/exprs")
	if n := strings.Count(src, "float sl_erf(float x) {"); n != 1 {
		t.Fatalf("%d sl_erf helper definitions, want 1:\n%s", n, src)
	}
	for _, want := range []string{"sl_erf(x)", "0.3275911f", "1.0614054f", "exp((-a) * a)"} {
		if !strings.Contains(src, want) {
			t.Errorf("output lacks %q:\n%s", want, src)
		}
	}
	if basic := generate(t, "../front/testdata/ok"); strings.Contains(basic, "sl_erf") {
		t.Errorf("sl_erf emitted for a shader that does not call sl.Erf")
	}
	if names := generate(t, "../msl/testdata/names"); !strings.Contains(names, "float sl_erf_1(float x) {") {
		t.Errorf("generated erf helper not renamed around the shader's own sl_erf:\n%s", names)
	}
}

// TestArrayLiteralOutsideDecl: HLSL has no array expressions, so an array
// literal anywhere but a declaration is an error, not invalid output.
func TestArrayLiteralOutsideDecl(t *testing.T) {
	m := selectModule()
	arr := ir.Array{Elem: ir.F32, Len: 2}
	a := &ir.Var{Name: "a", Type: arr}
	lit := &ir.Construct{T: arr, Args: []ir.Expr{&ir.Const{T: ir.F32, Value: 1}, &ir.Const{T: ir.F32, Value: 2}}}
	m.Fragment.Body = append([]ir.Stmt{&ir.Decl{V: a, Init: lit}, &ir.Assign{LHS: &ir.VarRef{V: a}, RHS: lit}}, m.Fragment.Body...)
	_, err := Generate(m)
	if err == nil || !strings.Contains(err.Error(), "array literal") {
		t.Fatalf("Generate: err = %v, want an array literal error", err)
	}
	m.Fragment.Body = m.Fragment.Body[:1]
	m.Fragment.Body = append(m.Fragment.Body, &ir.Return{Value: &ir.Const{T: ir.Vector{Elem: ir.F32, N: 4}}})
	src, err := Generate(m)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(src, "float a[2] = { 1.0f, 2.0f };") {
		t.Errorf("array initializer not emitted as a list:\n%s", src)
	}
}
