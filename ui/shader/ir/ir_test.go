package ir

import "testing"

var (
	vec2 = Vector{Elem: F32, N: 2}
	vec4 = Vector{Elem: F32, N: 4}
)

func TestPrintModule(t *testing.T) {
	params := &Struct{Name: "Params", Fields: []Field{
		{Name: "Color", Type: vec4, Attr: Attr{Location: -1}},
		{Name: "Size", Type: vec2, Attr: Attr{Location: -1}},
	}}
	varyings := &Struct{Name: "VSOut", Fields: []Field{
		{Name: "Pos", Type: vec4, Attr: Attr{Builtin: "position", Location: -1}},
		{Name: "UV", Type: vec2, Attr: Attr{Location: 0}},
		{Name: "Kind", Type: U32, Attr: Attr{Location: 1, Flat: true}},
	}}
	u := &Uniform{Name: "u", Type: params, Binding: 0}
	tex := &TextureVar{Name: "tex", Binding: 1, Filter: "linear"}

	in := &Var{Name: "in", Type: StructRef{S: varyings}}
	d := &Var{Name: "d", Type: F32}
	uRef := &GlobalRef{Name: "u", T: StructRef{S: params}}
	frag := &Func{
		Name:   "frag",
		Params: []*Var{in},
		Result: vec4,
		Body: []Stmt{
			&Decl{V: d, Init: &Call{Builtin: "Smoothstep", T: F32, Args: []Expr{
				&Const{T: F32, Value: 0},
				&Const{T: F32, Value: 1},
				&FieldExpr{X: &FieldExpr{X: &VarRef{V: in}, Name: "UV"}, Name: "X"},
			}}},
			&If{
				Cond: &Binary{Op: "<", X: &VarRef{V: d}, Y: &Const{T: F32, Value: 0.5}},
				Then: []Stmt{&Return{Value: &FieldExpr{X: uRef, Name: "Color"}}},
				Else: []Stmt{&Assign{LHS: &VarRef{V: d}, RHS: &Unary{Op: "-", X: &VarRef{V: d}}}},
			},
			&Return{Value: &Construct{T: vec4, Args: []Expr{
				&VarRef{V: d},
				&Const{T: F32, Value: 0},
				&Convert{T: F32, X: &Const{T: I32, Value: 2}},
				&Const{T: F32, Value: 1},
			}}},
		},
	}
	m := &Module{
		Structs:  []*Struct{params, varyings},
		Uniforms: []*Uniform{u},
		Textures: []*TextureVar{tex},
		Fragment: frag,
	}

	want := `struct Params {
  Color: vec4<f32>
  Size: vec2<f32>
}
struct VSOut {
  Pos: vec4<f32> @builtin(position)
  UV: vec2<f32> @location(0)
  Kind: u32 @location(1) @flat
}
uniform u: Params @binding(0)
texture tex @binding(1) linear
fragment func frag(in: VSOut) -> vec4<f32> {
  var d: f32 = sl.Smoothstep(0.0, 1.0, in.UV.X)
  if (d < 0.5) {
    return u.Color
  } else {
    d = (-d)
  }
  return vec4<f32>(d, 0.0, convert<f32>(2i), 1.0)
}
`
	if got := Print(m); got != want {
		t.Errorf("Print mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestPrintStatementsAndExprs(t *testing.T) {
	q := &Struct{Name: "Quad", Fields: []Field{{Name: "Pos", Type: vec4, Attr: Attr{Builtin: "position", Location: -1}}}}
	vid := &Var{Name: "vid", Type: U32}
	i := &Var{Name: "i", Type: I32}
	corners := &Var{Name: "corners", Type: Array{Elem: vec2, Len: 4}}
	helper := &Func{Name: "helper", Params: []*Var{{Name: "x", Type: F32}}, Result: F32,
		Body: []Stmt{&Return{Value: &VarRef{V: &Var{Name: "x", Type: F32}}}}}
	vert := &Func{
		Name:          "vert",
		Result:        StructRef{S: q},
		VertexIDParam: vid,
		Body: []Stmt{
			&Decl{V: corners},
			&For{
				Init: &Decl{V: i, Init: &Const{T: I32, Value: 0}},
				Cond: &Binary{Op: "<", X: &VarRef{V: i}, Y: &Const{T: I32, Value: 4}},
				Post: &Assign{LHS: &VarRef{V: i}, RHS: &Binary{Op: "+", X: &VarRef{V: i}, Y: &Const{T: I32, Value: 1}}},
				Body: []Stmt{
					&If{Cond: &Binary{Op: "==", X: &VarRef{V: i}, Y: &Const{T: I32, Value: 2}}, Then: []Stmt{&Continue{}}},
					&If{
						Cond: &Unary{Op: "!", X: &Const{T: Bool, Value: 1}},
						Then: []Stmt{&Break{}},
						Else: []Stmt{&If{
							Cond: &Const{T: Bool, Value: 0},
							Then: []Stmt{&ExprStmt{X: &Call{Func: helper, Args: []Expr{&Const{T: F32, Value: 1.25}}}}},
						}},
					},
				},
			},
			&For{Body: []Stmt{&Break{}}},
			&Return{Value: &Construct{T: StructRef{S: q}, Args: []Expr{
				&Construct{T: vec4, Args: []Expr{
					&Index{X: &VarRef{V: corners}, I: &Convert{T: I32, X: &VarRef{V: vid}}},
					&Select{Cond: &Const{T: Bool, Value: 1}, X: &Const{T: F32, Value: 0}, Y: &Const{T: F32, Value: -1}},
					&Const{T: F32, Value: 1},
				}},
			}}},
		},
	}
	m := &Module{Structs: []*Struct{q}, Funcs: []*Func{helper}, Vertex: vert}
	want := `struct Quad {
  Pos: vec4<f32> @builtin(position)
}
func helper(x: f32) -> f32 {
  return x
}
vertex func vert(vid: u32 @builtin(vertex_id)) -> Quad {
  var corners: array<vec2<f32>, 4>
  for (var i: i32 = 0i; (i < 4i); i = (i + 1i)) {
    if (i == 2i) {
      continue
    }
    if (!true) {
      break
    } else if false {
      helper(1.25)
    }
  }
  for (; ; ) {
    break
  }
  return Quad(vec4<f32>(corners[convert<i32>(vid)], (true ? 0.0 : -1.0), 1.0))
}
`
	if got := Print(m); got != want {
		t.Errorf("Print mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestPrintEntryNotInFuncs(t *testing.T) {
	f := &Func{Name: "main", Body: []Stmt{&Return{}}}
	m := &Module{Fragment: f}
	want := "fragment func main() {\n  return\n}\n"
	if got := Print(m); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPrintTextureSample(t *testing.T) {
	tex := &TextureVar{Name: "atlas", Binding: 2, Filter: "nearest"}
	uv := &Var{Name: "uv", Type: vec2}
	frag := &Func{
		Name:   "frag",
		Params: []*Var{uv},
		Result: vec4,
		Body: []Stmt{&Return{Value: &Call{
			Builtin: "Sample",
			T:       vec4,
			Args:    []Expr{&GlobalRef{Name: "atlas", T: Texture2D{}}, &VarRef{V: uv}},
		}}},
	}
	m := &Module{Textures: []*TextureVar{tex}, Fragment: frag}
	want := `texture atlas @binding(2) nearest
fragment func frag(uv: vec2<f32>) -> vec4<f32> {
  return sl.Sample(atlas, uv)
}
`
	if got := Print(m); got != want {
		t.Errorf("Print mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	if got := (&GlobalRef{Name: "atlas", T: Texture2D{}}).Type(); got != (Texture2D{}) {
		t.Errorf("texture GlobalRef Type() = %v, want texture2d", got)
	}
	if got := typeString(Texture2D{}); got != "texture2d" {
		t.Errorf("Texture2D String = %q", got)
	}
}

func TestExprTypes(t *testing.T) {
	s := &Struct{Name: "S", Fields: []Field{{Name: "A", Type: vec2}, {Name: "B", Type: U32}}}
	sv := &VarRef{V: &Var{Name: "s", Type: StructRef{S: s}}}
	v4 := &VarRef{V: &Var{Name: "v", Type: vec4}}
	f := &Const{T: F32, Value: 2}
	userFn := &Func{Name: "g", Result: vec2}
	cases := []struct {
		name string
		e    Expr
		want Type
	}{
		{"Const", f, F32},
		{"VarRef", v4, vec4},
		{"GlobalRef", &GlobalRef{Name: "u", T: U32}, U32},
		{"Binary arith vec*scalar", &Binary{Op: "*", X: v4, Y: f}, vec4},
		{"Binary arith scalar*vec", &Binary{Op: "*", X: f, Y: v4}, vec4},
		{"Binary compare scalar", &Binary{Op: "<", X: f, Y: f}, Bool},
		{"Binary compare vector", &Binary{Op: "==", X: v4, Y: v4}, Vector{Elem: Bool, N: 4}},
		{"Binary logical", &Binary{Op: "&&", X: &Const{T: Bool, Value: 1}, Y: &Const{T: Bool}}, Bool},
		{"Unary neg", &Unary{Op: "-", X: v4}, vec4},
		{"Unary not", &Unary{Op: "!", X: &Const{T: Bool}}, Bool},
		{"Call builtin", &Call{Builtin: "Length", T: F32}, F32},
		{"Call user", &Call{Func: userFn}, vec2},
		{"Field struct", &FieldExpr{X: sv, Name: "B"}, U32},
		{"Field struct missing", &FieldExpr{X: sv, Name: "Z"}, nil},
		{"Field nested swizzle", &FieldExpr{X: &FieldExpr{X: sv, Name: "A"}, Name: "Y"}, F32},
		{"Field swizzle 1", &FieldExpr{X: v4, Name: "W"}, F32},
		{"Field swizzle 2", &FieldExpr{X: v4, Name: "XY"}, vec2},
		{"Field swizzle 3", &FieldExpr{X: v4, Name: "XYZ"}, Vector{Elem: F32, N: 3}},
		{"Index array", &Index{X: &VarRef{V: &Var{Type: Array{Elem: vec2, Len: 3}}}, I: f}, vec2},
		{"Index vector", &Index{X: v4, I: f}, F32},
		{"Construct", &Construct{T: StructRef{S: s}}, StructRef{S: s}},
		{"Convert", &Convert{T: I32, X: f}, I32},
		{"Select", &Select{Cond: &Const{T: Bool}, X: v4, Y: v4}, vec4},
	}
	for _, c := range cases {
		if got := c.e.Type(); got != c.want {
			t.Errorf("%s: Type() = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestStructFieldLookup(t *testing.T) {
	s := &Struct{Name: "S", Fields: []Field{{Name: "A", Type: vec2}}}
	if f := s.Field("A"); f == nil || f.Type != vec2 {
		t.Fatalf("Field(A) = %v", f)
	}
	if f := s.Field("B"); f != nil {
		t.Fatalf("Field(B) = %v, want nil", f)
	}
}
