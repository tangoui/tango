package front

import (
	"go/types"
)

// Host-visible layouts.
//
// The host fills uniform buffers and the per-instance vertex buffer with
// the Go values of the shader package's structs, so their Go memory layout
// is the layout the GPU reads:
//
//   - A uniform struct is read by MSL as a constant-address-space struct,
//     whose layout has its own alignment rules: float2 is 8-byte aligned,
//     float3 and float4 16-byte aligned, a struct's size rounds up to its
//     alignment. In Go every sl vector is a struct of float32 (4-byte
//     aligned), and sl.Vec3 is 12 bytes. So every field offset and the
//     struct size must agree, or the shader silently reads shifted data.
//     sl.Vec3 is rejected outright (its 12 bytes never match float3's 16),
//     and so is Go's int (8 bytes against MSL int's 4), and bool (1 byte in
//     Go, 4 in an HLSL cbuffer, whose extra bytes are Go padding of
//     unspecified value; use uint32). Anything else that
//     does not line up is reported with both offsets: reorder the fields or
//     add explicit padding fields.
//   - The same struct is read by HLSL (Direct3D 11) from a cbuffer, packed
//     by different rules (see hlslLayout): no vector straddles a 16-byte
//     boundary, arrays and structs start on one, and array elements are
//     16 bytes apart. Every field offset must match Go there too, so for
//     example an array of float32 in a uniform struct is rejected (stride
//     4 in Go and MSL, 16 in HLSL); use an array of sl.Vec4.
//   - Instance attributes are read through a vertex descriptor whose
//     offsets and stride the host takes from the Go struct (the generator
//     emits them, with compile-time assertions, next to the MSL), so only
//     the attribute types themselves must have a vertex format of the same
//     size: 32-bit scalars, sl.Vec2 and sl.Vec4. sl.Vec3, int and bool are
//     rejected.
//
// Offsets and sizes are the gc compiler's (the same on every 32- and
// 64-bit target for these types, except int).

// goSizes is the Go layout the host uses.
var goSizes = types.SizesFor("gc", "arm64")

// mslLayout returns the size and alignment of t in MSL's constant address
// space; ok is false for a type with no host layout (texture, Go int).
func mslLayout(t types.Type) (size, align int64, ok bool) {
	t = types.Unalias(t)
	switch {
	case isSL(t, "Vec2"):
		return 8, 8, true
	case isSL(t, "Vec3"):
		return 16, 16, true
	case isSL(t, "Vec4"):
		return 16, 16, true
	}
	switch t := t.(type) {
	case *types.Basic:
		switch t.Kind() {
		case types.Float32, types.Int32, types.Uint32:
			return 4, 4, true
		case types.Bool:
			return 1, 1, true
		}
		return 0, 0, false
	case *types.Named:
		return mslLayout(t.Underlying())
	case *types.Array:
		s, a, ok := mslLayout(t.Elem())
		if !ok {
			return 0, 0, false
		}
		return roundUp(s, a) * t.Len(), a, true
	case *types.Struct:
		var off, maxA int64 = 0, 1
		for i := 0; i < t.NumFields(); i++ {
			s, a, ok := mslLayout(t.Field(i).Type())
			if !ok {
				return 0, 0, false
			}
			off = roundUp(off, a) + s
			maxA = max(maxA, a)
		}
		return roundUp(off, maxA), maxA, true
	}
	return 0, 0, false
}

func roundUp(n, a int64) int64 { return (n + a - 1) / a * a }

// hlslLayout returns the size of t in an HLSL (D3D11) constant buffer,
// without trailing padding, whether t starts on a 16-byte register
// boundary (arrays and structs do), and whether the member after it must
// start on a new register (structs, and arrays of them, force that). ok is
// false for a type with no host layout.
//
// HLSL cbuffer packing: a scalar or vector is 4-byte aligned but must not
// straddle a 16-byte boundary (it moves to the next register instead);
// every array element starts on a register, so an array's element stride
// is its element size rounded up to 16, but the last element is not padded
// and a following scalar may share its register; a struct starts on a
// register and the member after it starts on the next one. bool is 4
// bytes.
func hlslLayout(t types.Type) (size int64, reg, padAfter, ok bool) {
	t = types.Unalias(t)
	switch {
	case isSL(t, "Vec2"):
		return 8, false, false, true
	case isSL(t, "Vec3"):
		return 12, false, false, true
	case isSL(t, "Vec4"):
		return 16, false, false, true
	}
	switch t := t.(type) {
	case *types.Basic:
		switch t.Kind() {
		case types.Float32, types.Int32, types.Uint32, types.Bool:
			return 4, false, false, true
		}
		return 0, false, false, false
	case *types.Named:
		return hlslLayout(t.Underlying())
	case *types.Array:
		s, _, pad, ok := hlslLayout(t.Elem())
		if !ok {
			return 0, false, false, false
		}
		if t.Len() == 0 {
			return 0, true, false, true
		}
		return roundUp(s, 16)*(t.Len()-1) + s, true, pad, true
	case *types.Struct:
		_, end, ok := hlslOffsets(t)
		return end, true, true, ok
	}
	return 0, false, false, false
}

// hlslOffsets returns the HLSL cbuffer offsets of the fields of st
// (relative to the struct's start, which is register-aligned) and the end
// of its last field.
func hlslOffsets(st *types.Struct) (offs []int64, end int64, ok bool) {
	var off int64
	padNext := false
	for i := 0; i < st.NumFields(); i++ {
		s, reg, pad, has := hlslLayout(st.Field(i).Type())
		if !has {
			return nil, 0, false
		}
		switch {
		case reg || padNext:
			off = roundUp(off, 16)
		case s > 0 && off/16 != (off+s-1)/16:
			off = roundUp(off, 16)
		}
		offs = append(offs, off)
		off += s
		padNext = pad
	}
	return offs, off, true
}

// elemType strips array types from t.
func elemType(t types.Type) types.Type {
	for {
		a, ok := types.Unalias(t).Underlying().(*types.Array)
		if !ok {
			return t
		}
		t = a.Elem()
	}
}

// isGoInt reports whether t is int or uint (pointer-sized in Go).
func isGoInt(t types.Type) bool {
	b, ok := types.Unalias(t).Underlying().(*types.Basic)
	return ok && (b.Kind() == types.Int || b.Kind() == types.Uint)
}

// checkUniformLayout reports the fields of the uniform struct n (and of
// the structs it contains) whose Go layout differs from MSL's, and returns
// whether the layouts agree. Each struct is checked once.
func (c *compiler) checkUniformLayout(n *types.Named) bool {
	if ok, done := c.layoutOK[n.Obj()]; done {
		return ok
	}
	c.layoutOK[n.Obj()] = false // until checked (recursive types are rejected elsewhere)
	st, isStruct := n.Underlying().(*types.Struct)
	if !isStruct {
		return false
	}
	name := n.Obj().Name()
	fields := make([]*types.Var, st.NumFields())
	for i := range fields {
		fields[i] = st.Field(i)
	}
	goOffs := goSizes.Offsetsof(fields)
	ok := true
	var off, maxA int64 = 0, 1
	for i, fv := range fields {
		ft := fv.Type()
		el := elemType(ft)
		switch {
		case isSL(el, "Vec3"):
			c.errorf(fv.Pos(), "sl.Vec3 field %s in uniform struct %s: Go lays sl.Vec3 out in 12 bytes, MSL float3 in 16 (16-byte aligned); use sl.Vec4", fv.Name(), name)
			ok = false
			continue
		case isGoInt(el):
			c.errorf(fv.Pos(), "field %s: %s is 8 bytes in Go but 4 in MSL; use int32 or uint32", fv.Name(), c.ts(el))
			ok = false
			continue
		case isBasic(types.Unalias(el).Underlying(), types.Bool):
			c.errorf(fv.Pos(), "bool field %s in uniform struct %s: Go bool is 1 byte but HLSL bool is 4 (the padding bytes would be read as part of it); use uint32", fv.Name(), name)
			ok = false
			continue
		}
		if en, isNamed := types.Unalias(el).(*types.Named); isNamed && en.Obj().Pkg() == c.pkg {
			if _, isSt := en.Underlying().(*types.Struct); isSt && !c.checkUniformLayout(en) {
				ok = false
				continue
			}
		}
		s, a, has := mslLayout(ft)
		if !has {
			ok = false // reported by irType
			continue
		}
		off = roundUp(off, a)
		if ok && goOffs[i] != off {
			c.errorf(fv.Pos(), "field %s of uniform struct %s is at byte %d in Go but %d in MSL (%s is %d-byte aligned there); reorder the fields or add a padding field before it",
				fv.Name(), name, goOffs[i], off, c.ts(ft), a)
			ok = false
		}
		off += s
		maxA = max(maxA, a)
	}
	if ok {
		mslSize, goSize := roundUp(off, maxA), goSizes.Sizeof(st)
		if mslSize != goSize {
			c.errorf(n.Obj().Pos(), "uniform struct %s is %d bytes in Go but %d in MSL; add %d bytes of padding fields at the end",
				name, goSize, mslSize, mslSize-goSize)
			ok = false
		}
	}
	if !c.checkHLSLLayout(n, st, fields, goOffs) {
		ok = false
	}
	c.layoutOK[n.Obj()] = ok
	return ok
}

// checkHLSLLayout reports the fields of the uniform struct n whose Go
// offset differs from the HLSL cbuffer packing (see hlslLayout), and a
// struct whose fields extend past its Go size there. Field types that
// have no layout, or that checkUniformLayout already rejected (sl.Vec3,
// Go int, nested structs that do not match), are not reported again. The
// D3D11 host rounds the buffer size up to 16 bytes, so a Go size that is
// not a multiple of 16 is fine.
func (c *compiler) checkHLSLLayout(n *types.Named, st *types.Struct, fields []*types.Var, goOffs []int64) bool {
	for _, fv := range fields {
		el := elemType(fv.Type())
		if isSL(el, "Vec3") || isGoInt(el) || isBasic(types.Unalias(el).Underlying(), types.Bool) {
			return false
		}
	}
	offs, end, has := hlslOffsets(st)
	if !has {
		return false
	}
	name := n.Obj().Name()
	for i, fv := range fields {
		if goOffs[i] != offs[i] {
			c.errorf(fv.Pos(), "field %s of uniform struct %s is at byte %d in Go but %d in HLSL (cbuffer packing: vectors must not straddle a 16-byte boundary, arrays and structs start on one and every array element is 16-byte aligned); reorder the fields or add a padding field before it",
				fv.Name(), name, goOffs[i], offs[i])
			return false
		}
	}
	if goSize := goSizes.Sizeof(st); end > goSize {
		c.errorf(n.Obj().Pos(), "uniform struct %s is %d bytes in Go but %d in HLSL; add %d bytes of padding fields at the end",
			name, goSize, end, end-goSize)
		return false
	}
	return true
}

// checkAttrType reports a vertex input field whose type has no vertex
// format of its Go size.
func (c *compiler) checkAttrType(fv *types.Var, in string) {
	t := fv.Type()
	switch {
	case isSL(t, "Vec3"):
		c.errorf(fv.Pos(), "sl.Vec3 field %s in vertex input %s: instance attributes must not be sl.Vec3 (12 bytes in Go, padded to 16 in Metal buffers); use sl.Vec4", fv.Name(), in)
	case isGoInt(t):
		c.errorf(fv.Pos(), "field %s: %s is 8 bytes in Go but 4 in MSL; use int32 or uint32", fv.Name(), c.ts(t))
	case isBasic(types.Unalias(t).Underlying(), types.Bool):
		c.errorf(fv.Pos(), "field %s: bool instance attributes are not supported; use uint32", fv.Name())
	}
}
