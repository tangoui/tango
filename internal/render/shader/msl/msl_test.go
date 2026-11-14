package msl

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tangoui/tango/internal/render/shader/front"
	"github.com/tangoui/tango/internal/render/shader/ir"
)

var update = flag.Bool("update", false, "rewrite testdata/basic.metal")

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
	const golden = "testdata/basic.metal"
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

// TestMetalCompiles runs the Metal compiler on the generated source of every
// test shader. It is skipped when the Metal toolchain is not installed.
func TestMetalCompiles(t *testing.T) {
	if err := exec.Command("xcrun", "-f", "metal").Run(); err != nil {
		t.Skip("xcrun metal not available:", err)
	}
	// Since Xcode 26 the Metal toolchain is a separately downloaded
	// component; xcrun finds the tool but cannot run it until installed.
	if out, _ := exec.Command("xcrun", "metal", "--version").CombinedOutput(); strings.Contains(string(out), "missing Metal Toolchain") {
		t.Skip("Metal toolchain not installed (xcodebuild -downloadComponent MetalToolchain)")
	}
	for _, dir := range []string{
		"../front/testdata/ok",
		"../front/testdata/scoping",
		"testdata/names",
		"testdata/exprs",
	} {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			metalCompile(t, generate(t, dir))
		})
	}
	t.Run("select", func(t *testing.T) {
		src, err := Generate(selectModule())
		if err != nil {
			t.Fatal(err)
		}
		metalCompile(t, src)
	})
}

func metalCompile(t *testing.T, src string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "x.metal")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("xcrun", "metal", "-c", "-o", os.DevNull, path).CombinedOutput()
	if err != nil {
		t.Fatalf("xcrun metal failed: %v\n%s\n--- source ---\n%s", err, out, src)
	}
	if len(out) > 0 {
		t.Errorf("xcrun metal reported diagnostics:\n%s\n--- source ---\n%s", out, src)
	}
}

// TestNamesLegalized checks that identifiers colliding with Metal keywords,
// library functions and generated names are renamed, and that a local that
// shadows a global gets a name distinct from the global.
func TestNamesLegalized(t *testing.T) {
	src := generate(t, "testdata/names")
	for _, bad := range []string{
		"struct vertex ", "struct fragment ", "struct thread ",
		" half;", " float;", " sample ", " kernel ", " device ",
		" min;", " max;", "& constant ", " texture [[", "float length(", "float4 main(", "__",
		"float vs_main(float", "float2 using ", "float namespace ", "float template ", "float class ",
	} {
		if strings.Contains(src, bad) {
			t.Errorf("output contains unescaped %q:\n%s", bad, src)
		}
	}
	for _, want := range []string{"vertex fragment_ vs_main(", "fragment float4 fs_main(",
		"float constant_1 = ", "float4 texture_1 = ", "constexpr sampler texture_sampler(",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("output lacks %q:\n%s", want, src)
		}
	}
}

func TestConstLiterals(t *testing.T) {
	for _, c := range []struct {
		e    *ir.Const
		want string
	}{
		{&ir.Const{T: ir.F32, Value: 1}, "1.0f"},
		{&ir.Const{T: ir.F32, Value: 0.5}, "0.5f"},
		{&ir.Const{T: ir.F32, Value: -2.25}, "(-2.25f)"},
		{&ir.Const{T: ir.F32, Value: 1e20}, "1e+20f"},
		{&ir.Const{T: ir.U32, Value: 3}, "3u"},
		{&ir.Const{T: ir.I32, Value: -4}, "(-4)"},
		{&ir.Const{T: ir.Bool, Value: 1}, "true"},
		{&ir.Const{T: ir.Vector{Elem: ir.F32, N: 2}, Value: 0}, "float2(0.0f)"},
	} {
		if got := constLit(c.e); got != c.want {
			t.Errorf("constLit(%v %v) = %q, want %q", c.e.T, c.e.Value, got, c.want)
		}
	}
}

// selectModule is a hand-built module using IR forms the front end does not
// produce from basic.go: scalar and vector Select, vector comparisons and a
// negative constant.
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
		"float2 pick = select(float2(0.5f), in.P, in.P < float2(0.5f));",
		"return (pick.x > 0.25f) ? float4(1.0f) : float4(pick, (-pick.y), 1.0f);",
		"float4(in.P.x, (-1.0f), 0.0f, 1.0f)",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("output lacks %q:\n%s", want, src)
		}
	}
}

// TestErfHelper: MSL has no erf, so a call of sl.Erf becomes a call of a
// generated helper (the same Abramowitz–Stegun polynomial as sl.Erf),
// emitted once before the shader's own functions, and only when used. Its
// name is legalized like any program-scope name: a shader helper already
// called sl_erf keeps that name and the generated one moves aside.
func TestErfHelper(t *testing.T) {
	src := generate(t, "testdata/exprs")
	if n := strings.Count(src, "static inline float sl_erf(float x) {"); n != 1 {
		t.Fatalf("%d sl_erf helper definitions, want 1:\n%s", n, src)
	}
	for _, want := range []string{"sl_erf(x)", "0.3275911f", "1.0614054f", "exp((-a) * a)"} {
		if !strings.Contains(src, want) {
			t.Errorf("output lacks %q:\n%s", want, src)
		}
	}
	if strings.Index(src, "sl_erf(float x)") > strings.Index(src, "float4 tint(") {
		t.Errorf("sl_erf is defined after the shader helpers:\n%s", src)
	}
	if basic := generate(t, "../front/testdata/ok"); strings.Contains(basic, "sl_erf") {
		t.Errorf("sl_erf emitted for a shader that does not call sl.Erf:\n%s", basic)
	}
	names := generate(t, "testdata/names")
	if !strings.Contains(names, "static inline float sl_erf_1(float x) {") || !strings.Contains(names, "sl_erf_1(") {
		t.Errorf("generated erf helper not renamed around the shader's own sl_erf:\n%s", names)
	}
}
