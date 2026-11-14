package front

import (
	"go/types"
	"strings"
	"testing"
)

// slNamed returns a stand-in for the sl vector type name (a struct of n
// float32 fields in package sl), as isSL recognizes it.
func slNamed(name string, n int) *types.Named {
	pkg := types.NewPackage(slPath, "sl")
	fields := make([]*types.Var, n)
	for i := range fields {
		fields[i] = types.NewField(0, pkg, string("XYZW"[i]), types.Typ[types.Float32], false)
	}
	return types.NewNamed(types.NewTypeName(0, pkg, name, nil), types.NewStruct(fields, nil), nil)
}

func structOf(types_ ...types.Type) *types.Struct {
	fields := make([]*types.Var, len(types_))
	for i, t := range types_ {
		fields[i] = types.NewField(0, nil, string(rune('A'+i)), t, false)
	}
	return types.NewStruct(fields, nil)
}

func TestStd140Offsets(t *testing.T) {
	f32 := types.Typ[types.Float32]
	vec2, vec4 := slNamed("Vec2", 2), slNamed("Vec4", 4)
	pair := structOf(f32, f32)
	for _, c := range []struct {
		name string
		st   *types.Struct
		offs []int64
		size int64
	}{
		{"float, vec2", structOf(f32, vec2), []int64{0, 8}, 16},
		{"float, struct{float, float}", structOf(f32, pair), []int64{0, 16}, 32},
		{"struct{float, float}, float", structOf(pair, f32), []int64{0, 16}, 32},
		{"vec2, vec2", structOf(vec2, vec2), []int64{0, 8}, 16},
		{"float, vec4", structOf(f32, vec4), []int64{0, 16}, 32},
		{"[2]float, float", structOf(types.NewArray(f32, 2), f32), []int64{0, 32}, 48},
		{"[2]vec4, vec2", structOf(types.NewArray(vec4, 2), vec2), []int64{0, 32}, 48},
		{"float", structOf(f32), []int64{0}, 16},
	} {
		offs, size, ok := std140Offsets(c.st)
		if !ok {
			t.Errorf("%s: no std140 layout", c.name)
			continue
		}
		if size != c.size || len(offs) != len(c.offs) {
			t.Errorf("%s: offsets %v size %d, want %v size %d", c.name, offs, size, c.offs, c.size)
			continue
		}
		for i := range offs {
			if offs[i] != c.offs[i] {
				t.Errorf("%s: offsets %v, want %v", c.name, offs, c.offs)
				break
			}
		}
	}
}

// TestStd140LayoutMismatch calls the std140 check directly: the MSL and
// HLSL checks reject the structs whose std140 layout differs from Go's
// before it runs, so front.Load never reaches it with one.
func TestStd140LayoutMismatch(t *testing.T) {
	f32 := types.Typ[types.Float32]
	st := structOf(f32, types.NewArray(f32, 2)) // B: Go 4, std140 16
	fields := []*types.Var{st.Field(0), st.Field(1)}
	field, msg := std140Mismatch("Params", st, goSizes.Offsetsof(fields), goSizes.Sizeof(st))
	if field != 1 {
		t.Errorf("mismatch reported at field %d, want 1 (B)", field)
	}
	for _, want := range []string{"field B", "Params", "byte 4 in Go", "16 in std140"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q lacks %q", msg, want)
		}
	}

	// A struct whose fields match but whose std140 size exceeds its Go
	// size rounded up to 16 is reported against the struct (field -1).
	pair := structOf(f32, f32)
	st = structOf(slNamed("Vec4", 4), types.NewArray(pair, 2)) // std140 48, Go 32
	field, msg = std140Mismatch("Big", st, []int64{0, 16}, goSizes.Sizeof(st))
	if field != -1 || !strings.Contains(msg, "Big") || !strings.Contains(msg, "48 in std140") {
		t.Errorf("size mismatch: field %d, message %q", field, msg)
	}

	vec2 := slNamed("Vec2", 2)
	st = structOf(f32, f32, vec2)
	fields = []*types.Var{st.Field(0), st.Field(1), st.Field(2)}
	if field, msg := std140Mismatch("OK", st, goSizes.Offsetsof(fields), goSizes.Sizeof(st)); msg != "" {
		t.Errorf("matching layout reported at field %d: %s", field, msg)
	}
}
