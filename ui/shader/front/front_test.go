package front

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/shader/ir"
)

var update = flag.Bool("update", false, "rewrite testdata/ok/*.ir snapshots")

func TestLoadOK(t *testing.T) {
	m, err := Load("testdata/ok")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := ir.Print(m)
	golden := filepath.Join("testdata", "ok", "basic.ir")
	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read snapshot (run with -update to create it): %v", err)
	}
	if got != string(want) {
		t.Errorf("ir.Print mismatch (run with -update after checking)\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}

	// Structural rulings that the printed form does not show directly.
	if m.Vertex == nil || m.Fragment == nil {
		t.Fatal("missing entry points")
	}
	for _, f := range m.Funcs {
		if f == m.Vertex || f == m.Fragment {
			t.Errorf("entry point %s listed in Funcs", f.Name)
		}
	}
	if len(m.Funcs) != 1 || m.Funcs[0].Name != "rrectSDF" {
		t.Errorf("Funcs = %v, want [rrectSDF]", funcNames(m.Funcs))
	}
	if m.Vertex.VertexIDParam == nil || m.Vertex.VertexIDParam.Type != ir.U32 {
		t.Errorf("vertex VertexIDParam = %+v, want u32", m.Vertex.VertexIDParam)
	}
	for _, p := range m.Vertex.Params {
		if p == m.Vertex.VertexIDParam {
			t.Error("VertexIDParam listed in Params")
		}
	}
	// Go layouts (the host buffer layouts) are recorded.
	for _, s := range m.Structs {
		var offs []int
		for _, f := range s.Fields {
			offs = append(offs, f.Offset)
		}
		switch s.Name {
		case "Instance":
			if s.Size != 40 || fmt.Sprint(offs) != "[0 16 32 36]" {
				t.Errorf("Instance layout: size %d, offsets %v; want 40, [0 16 32 36]", s.Size, offs)
			}
		case "Globals":
			if s.Size != 16 || fmt.Sprint(offs) != "[0 8 12]" {
				t.Errorf("Globals layout: size %d, offsets %v; want 16, [0 8 12]", s.Size, offs)
			}
		}
	}
	for _, s := range m.Structs {
		if s.Name != "Globals" {
			continue
		}
		for _, f := range s.Fields {
			if f.Attr.Location != -1 {
				t.Errorf("Globals.%s Location = %d, want -1", f.Name, f.Attr.Location)
			}
		}
	}
}

func funcNames(fs []*ir.Func) []string {
	var s []string
	for _, f := range fs {
		s = append(s, f.Name)
	}
	return s
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		file string
		msg  string
		line int
	}{
		{"pointer.go", "pointers are not supported", 20},
		{"slice.go", "slices are not supported", 20},
		{"closure.go", "function literals are not supported", 20},
		{"recursion.go", "recursion is not supported", 22},
		{"goroutine.go", "goroutines are not supported", 22},
		{"unbounded_for.go", "loops must have init, condition and post", 21},
		{"two_position.go", `exactly one sl:"position" field`, 11},
		{"no_vertex.go", "missing //sl:vertex", 1},
		// Host-visible layouts (uniform buffers, instance attributes).
		{"vec3_uniform.go", "sl.Vec3 field Tint in uniform struct Globals", 14},
		{"vec3_nested_uniform.go", "sl.Vec3 field Dirs in uniform struct Light", 15},
		{"vec3_instance.go", "sl.Vec3 field Color in vertex input In", 7},
		{"int_instance.go", "field Kind: int is 8 bytes in Go", 7},
		{"uniform_offset.go", "field Tint of uniform struct Globals is at byte 4 in Go but 16 in MSL", 15},
		{"uniform_size.go", "uniform struct Globals is 20 bytes in Go but 32 in MSL", 13},
		{"bool_uniform.go", "bool field On in uniform struct Globals: Go bool is 1 byte but HLSL bool is 4", 15},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			m, err := loadFiles([]string{filepath.Join("testdata", "bad", tt.file)})
			if err == nil {
				t.Fatalf("no error; module:\n%s", ir.Print(m))
			}
			var list ErrorList
			if !errors.As(err, &list) {
				t.Fatalf("error %T is not an ErrorList: %v", err, err)
			}
			for _, e := range list {
				if strings.Contains(e.Msg, tt.msg) && e.Pos.Line == tt.line &&
					filepath.Base(e.Pos.Filename) == tt.file {
					return
				}
			}
			t.Errorf("want %q at %s:%d, got:\n%v", tt.msg, tt.file, tt.line, err)
		})
	}
}

// TestHLSLLayoutMismatch: a uniform struct must have its Go layout under
// HLSL's cbuffer packing too; a mismatch names the field and both offsets.
// The brief's straddling struct {A Vec2; B float32; C Vec2} is rejected by
// MSL as well (float2 is 8-byte aligned there, so C moves to 16 in both
// backends), which is why hlsl_array.go adds a case only HLSL rejects.
func TestHLSLLayoutMismatch(t *testing.T) {
	tests := []struct {
		file    string
		want    []string // substrings of errors at line 20
		wantMSL bool     // whether MSL reports the struct too
	}{
		{"hlsl_straddle.go", []string{"field C of uniform struct Globals is at byte 12 in Go but 16 in HLSL", "field C of uniform struct Globals is at byte 12 in Go but 16 in MSL"}, true},
		{"hlsl_array.go", []string{"field Weights of uniform struct Globals is at byte 4 in Go but 16 in HLSL"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			_, err := loadFiles([]string{filepath.Join("testdata", "bad", tt.file)})
			var list ErrorList
			if !errors.As(err, &list) {
				t.Fatalf("error %v is not an ErrorList", err)
			}
			for _, want := range tt.want {
				found := false
				for _, e := range list {
					if strings.Contains(e.Msg, want) && e.Pos.Line == 20 {
						found = true
					}
				}
				if !found {
					t.Errorf("want %q at line 20, got:\n%v", want, err)
				}
			}
			if !tt.wantMSL && strings.Contains(err.Error(), "MSL") {
				t.Errorf("MSL layout should agree with Go, got:\n%v", err)
			}
		})
	}
}

func TestErrorListString(t *testing.T) {
	var err error = ErrorList{{Msg: "a"}, {Msg: "b"}}
	if s := err.Error(); !strings.Contains(s, "a") || !strings.Contains(s, "b") {
		t.Errorf("ErrorList.Error() = %q", s)
	}
}

func TestFoldBinary(t *testing.T) {
	f := func(v float64) *ir.Const { return &ir.Const{T: ir.F32, Value: v} }
	i := func(v float64) *ir.Const { return &ir.Const{T: ir.I32, Value: v} }
	v := &ir.VarRef{V: &ir.Var{Name: "v", Type: ir.F32}}
	tests := []struct {
		op   string
		x, y ir.Expr
		want string
	}{
		{"+", f(1.5), f(2), "3.5"},
		{"/", i(7), i(2), "3i"},
		{"/", i(-7), i(2), "-3i"},
		{"*", f(0.1), f(3), "0.3"},
		{"<", f(1), f(2), "true"},
		{"/", i(1), i(0), "(1i / 0i)"}, // integer division by zero is left alone
		{"+", v, f(1), "(v + 1.0)"},    // not constant: kept as Binary
	}
	for _, tt := range tests {
		m := &ir.Module{Funcs: []*ir.Func{{Name: "f", Body: []ir.Stmt{&ir.Return{Value: foldBinary(tt.op, tt.x, tt.y)}}}}}
		got := ir.Print(m)
		if !strings.Contains(got, "return "+tt.want+"\n") {
			t.Errorf("fold %v %s %v:\n%s\nwant return %s", tt.x, tt.op, tt.y, got, tt.want)
		}
	}
}

// TestStructOrder checks that a struct is emitted after the structs it
// contains, even when it is declared before them.
func TestStructOrder(t *testing.T) {
	m, err := loadFiles([]string{filepath.Join("testdata", "scoping", "scoping.go")})
	if err != nil {
		t.Fatal(err)
	}
	idx := map[string]int{}
	for i, s := range m.Structs {
		idx[s.Name] = i
	}
	li, lok := idx["Light"]
	gi, gok := idx["Globals"]
	if !lok || !gok || li > gi {
		t.Errorf("struct order %v: Light must come before Globals", structNames(m.Structs))
	}
}

func structNames(ss []*ir.Struct) []string {
	var s []string
	for _, x := range ss {
		s = append(s, x.Name)
	}
	return s
}

// TestShadowedNamesUnique checks that locals shadowing an outer variable or
// a parameter get a fresh name, and that the initializer still reads the
// outer variable.
func TestShadowedNamesUnique(t *testing.T) {
	m, err := loadFiles([]string{filepath.Join("testdata", "scoping", "scoping.go")})
	if err != nil {
		t.Fatal(err)
	}
	fs := m.Fragment
	seen := map[string]bool{}
	for _, p := range fs.Params {
		seen[p.Name] = true
	}
	var decls []*ir.Decl
	var walk func([]ir.Stmt)
	walk = func(body []ir.Stmt) {
		for _, s := range body {
			switch s := s.(type) {
			case *ir.Decl:
				decls = append(decls, s)
			case *ir.If:
				walk(s.Then)
				walk(s.Else)
			case *ir.For:
				walk([]ir.Stmt{s.Init})
				walk(s.Body)
			}
		}
	}
	walk(fs.Body)
	for _, d := range decls {
		if seen[d.V.Name] {
			t.Errorf("local name %q declared twice in %s", d.V.Name, fs.Name)
		}
		seen[d.V.Name] = true
	}

	outer := fs.Body[0].(*ir.Decl).V
	if outer.Name != "x" {
		t.Errorf("first declaration renamed to %q, want x", outer.Name)
	}
	readsOuter := func(where string, d *ir.Decl) {
		t.Helper()
		b, ok := d.Init.(*ir.Binary)
		if !ok {
			t.Fatalf("%s: init %T, want Binary", where, d.Init)
		}
		if r, ok := b.X.(*ir.VarRef); !ok || r.V != outer {
			t.Errorf("%s: x := x + c must read the outer x, got %#v", where, b.X)
		}
		if d.V == outer || d.V.Name == outer.Name {
			t.Errorf("%s: shadowing x not renamed (%q)", where, d.V.Name)
		}
	}
	readsOuter("if body", fs.Body[1].(*ir.If).Then[0].(*ir.Decl))
	loop := fs.Body[2].(*ir.For)
	readsOuter("for body", loop.Body[0].(*ir.Decl))
	if in := loop.Body[1].(*ir.Decl); in.V.Name == fs.Params[0].Name {
		t.Errorf("local shadowing parameter %q not renamed", in.V.Name)
	}
}

// TestUniformLayoutOK: uniform structs whose Go layout matches MSL's and
// HLSL's (nested structs, arrays, explicit padding) load without errors.
func TestUniformLayoutOK(t *testing.T) {
	dir := t.TempDir()
	src := `package good

import "github.com/tangoui/tango/ui/shader/sl"

type In struct {
	P    sl.Vec2 ` + "`sl:\"loc=0\"`" + `
	K    uint32  ` + "`sl:\"loc=1\"`" + `
	S    float32 ` + "`sl:\"loc=2\"`" + `
	Tint sl.Vec4 ` + "`sl:\"loc=3\"`" + `
}

type Out struct {
	Pos sl.Vec4 ` + "`sl:\"position\"`" + `
	C   sl.Vec3 ` + "`sl:\"loc=0\"`" + ` // varyings never reach the host
}

type Light struct {
	Color sl.Vec4
	Dir   sl.Vec2
	Pad   sl.Vec2
}

type Globals struct {
	Lights  [2]Light
	Weights [1]sl.Vec4 // arrays of scalars have a 16-byte stride in HLSL
	Size    sl.Vec2
	Scale   float32
	On      int32
}

//sl:uniform 0
var U Globals

//sl:vertex
func vs(in In, vid uint32) Out {
	return Out{Pos: sl.V4(in.P.X, in.P.Y, U.Scale, U.Lights[1].Color.W), C: sl.V3(in.S, 0, 0)}
}

//sl:fragment
func fs(in Out) sl.Vec4 {
	return sl.V4(in.C.X, 0, 0, 1)
}
`
	if err := os.WriteFile(filepath.Join(dir, "good.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range m.Structs {
		if s.Name == "Globals" && s.Size != 96 {
			t.Errorf("Globals size %d, want 96", s.Size)
		}
	}
}
