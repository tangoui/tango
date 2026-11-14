// Package ir is the structured intermediate representation shared by the
// shader front end (Go AST lowering) and the backends (Metal emission, CPU
// execution).
//
// A Module holds struct declarations, resource bindings (uniform buffers and
// textures) and functions. Function bodies are trees of Stmt and Expr nodes.
// All statement and expression nodes are used through pointers (for example
// &Binary{...}); every Expr reports its result type through Type().
//
// Builtin calls refer to package sl functions by name (Call.Builtin, e.g.
// "Smoothstep"); this package does not import sl.
package ir

import (
	"fmt"
	"strings"
)

// Module is one compiled shader program.
type Module struct {
	Structs  []*Struct
	Uniforms []*Uniform
	Textures []*TextureVar
	// Funcs are the helper (non-entry) functions only.
	Funcs []*Func
	// Vertex and Fragment are the entry points. They are not listed in Funcs.
	Vertex, Fragment *Func
}

// Type is a shader value type: Scalar, Vector, StructRef, Array or Texture2D.
// Types are comparable with ==, and each implements fmt.Stringer.
type Type interface{ isType() }

// Scalar is a scalar type.
type Scalar int

const (
	F32 Scalar = iota
	I32
	U32
	Bool
)

// Vector is an N-component vector (N is 2, 3 or 4).
type Vector struct {
	Elem Scalar
	N    int
}

// StructRef refers to a declared struct type.
type StructRef struct{ S *Struct }

// Array is a fixed-length array.
type Array struct {
	Elem Type
	Len  int
}

// Texture2D is the type of a texture binding. TextureVar declares the
// binding; a GlobalRef to it has T: Texture2D{}. The sl method call
// tex.Sample(uv) is lowered to
//
//	Call{Builtin: "Sample", T: Vector{Elem: F32, N: 4},
//		Args: []Expr{&GlobalRef{Name: "tex", T: Texture2D{}}, uv}}
type Texture2D struct{}

func (Texture2D) isType()        {}
func (Texture2D) String() string { return "texture2d" }

func (Scalar) isType()    {}
func (Vector) isType()    {}
func (StructRef) isType() {}
func (Array) isType()     {}

func (s Scalar) String() string {
	switch s {
	case F32:
		return "f32"
	case I32:
		return "i32"
	case U32:
		return "u32"
	case Bool:
		return "bool"
	}
	return fmt.Sprintf("scalar(%d)", int(s))
}

func (v Vector) String() string { return fmt.Sprintf("vec%d<%s>", v.N, v.Elem) }

func (r StructRef) String() string {
	if r.S == nil {
		return "<nil struct>"
	}
	return r.S.Name
}

func (a Array) String() string { return fmt.Sprintf("array<%s, %d>", typeString(a.Elem), a.Len) }

func typeString(t Type) string {
	if s, ok := t.(fmt.Stringer); ok {
		return s.String()
	}
	return "<nil>"
}

// Struct is a struct declaration. Fields of entry-point input/output structs
// carry stage attributes in Attr.
type Struct struct {
	Name   string
	Fields []Field
	// Size is the size in bytes of the Go struct the shader was written
	// with (gc layout), which is the layout of host-filled buffers (uniform
	// blocks, per-instance vertex data). The front end checks that
	// host-visible structs lay out the same in the backends.
	Size int
}

// Field returns the field with the given name, or nil.
func (s *Struct) Field(name string) *Field {
	for i := range s.Fields {
		if s.Fields[i].Name == name {
			return &s.Fields[i]
		}
	}
	return nil
}

// Field is one struct member.
type Field struct {
	Name string
	Type Type
	Attr Attr
	// Offset is the field's byte offset in the Go struct (see Struct.Size).
	Offset int
}

// Attr holds stage-interface attributes of a struct field.
type Attr struct {
	Builtin  string // "position" or ""
	Location int    // -1 means none
	Flat     bool
}

// Uniform is a uniform buffer bound at Binding, holding one struct.
type Uniform struct {
	Name    string
	Type    *Struct
	Binding int
}

// TextureVar declares a 2D texture with its sampler, bound at Binding.
// Expressions refer to it through GlobalRef{Name, T: Texture2D{}}.
type TextureVar struct {
	Name    string
	Binding int
	Filter  string // "nearest" | "linear"
}

// Func is a shader function. For the vertex entry point VertexIDParam is the
// vertex-index input (u32); it is not listed in Params. It is nil for all
// other functions.
type Func struct {
	Name          string
	Params        []*Var
	Result        Type // nil means no result
	Body          []Stmt
	VertexIDParam *Var
}

// Var is a parameter or local variable. Identity is the pointer.
type Var struct {
	Name string
	Type Type
}

// Stmt is a statement node.
type Stmt interface{ isStmt() }

// Decl declares a local variable; Init may be nil (zero value).
type Decl struct {
	V    *Var
	Init Expr
}

// Assign stores RHS into the location LHS (VarRef, FieldExpr or Index).
type Assign struct{ LHS, RHS Expr }

// If is a conditional; Else may be empty.
type If struct {
	Cond       Expr
	Then, Else []Stmt
}

// For is a loop; Init, Cond and Post may each be nil.
type For struct {
	Init Stmt
	Cond Expr
	Post Stmt
	Body []Stmt
}

// Return returns from the function; Value is nil for a bare return.
type Return struct{ Value Expr }

// Break exits the innermost loop.
type Break struct{}

// Continue jumps to the next iteration of the innermost loop.
type Continue struct{}

// ExprStmt evaluates X for its side effects.
type ExprStmt struct{ X Expr }

func (*Decl) isStmt()     {}
func (*Assign) isStmt()   {}
func (*If) isStmt()       {}
func (*For) isStmt()      {}
func (*Return) isStmt()   {}
func (*Break) isStmt()    {}
func (*Continue) isStmt() {}
func (*ExprStmt) isStmt() {}

// Expr is an expression node.
type Expr interface {
	Type() Type
	isExpr()
}

// Const is a scalar constant (or a vector splat when T is a Vector).
// Booleans use 0 and 1.
type Const struct {
	T     Type
	Value float64
}

// VarRef reads a parameter or local variable.
type VarRef struct{ V *Var }

// GlobalRef reads a module-level binding (a uniform or texture) by name.
type GlobalRef struct {
	Name string
	T    Type
}

// Binary is a binary operation. Op is one of
// + - * / < <= > >= == != && ||.
type Binary struct {
	Op   string
	X, Y Expr
}

// Unary is a unary operation. Op is "-" or "!".
type Unary struct {
	Op string
	X  Expr
}

// Call calls either an sl builtin (Builtin set, e.g. "Smoothstep") or a user
// function (Func set). T is the result type of a builtin call; for a user
// function call T may be left nil and Func.Result is used.
type Call struct {
	Builtin string
	Func    *Func
	Args    []Expr
	T       Type
}

// FieldExpr selects a struct field, or a swizzle (X, Y, Z, W, XY, XYZ, ...)
// of a vector. It is the field-access expression of the brief, renamed
// because Field already names struct members.
type FieldExpr struct {
	X    Expr
	Name string
}

// Index indexes an array or vector.
type Index struct{ X, I Expr }

// Construct builds a vector or struct value from Args.
type Construct struct {
	T    Type
	Args []Expr
}

// Convert converts X to scalar or vector type T.
type Convert struct {
	T Type
	X Expr
}

// Select evaluates to X when Cond is true, otherwise Y.
type Select struct{ Cond, X, Y Expr }

func (*Const) isExpr()     {}
func (*VarRef) isExpr()    {}
func (*GlobalRef) isExpr() {}
func (*Binary) isExpr()    {}
func (*Unary) isExpr()     {}
func (*Call) isExpr()      {}
func (*FieldExpr) isExpr() {}
func (*Index) isExpr()     {}
func (*Construct) isExpr() {}
func (*Convert) isExpr()   {}
func (*Select) isExpr()    {}

func (e *Const) Type() Type { return e.T }
func (e *VarRef) Type() Type {
	if e.V == nil {
		return nil
	}
	return e.V.Type
}
func (e *GlobalRef) Type() Type { return e.T }
func (e *Construct) Type() Type { return e.T }
func (e *Convert) Type() Type   { return e.T }
func (e *Select) Type() Type    { return e.X.Type() }

// Type of a comparison is bool (or a bool vector for vector operands); of a
// logical operator bool; of arithmetic the vector operand's type if either
// side is a vector, else X's type.
func (e *Binary) Type() Type {
	switch e.Op {
	case "&&", "||":
		return Bool
	case "<", "<=", ">", ">=", "==", "!=":
		if v, ok := e.X.Type().(Vector); ok {
			return Vector{Elem: Bool, N: v.N}
		}
		return Bool
	}
	xt := e.X.Type()
	if _, ok := xt.(Vector); ok {
		return xt
	}
	if yt := e.Y.Type(); yt != nil {
		if _, ok := yt.(Vector); ok {
			return yt
		}
	}
	return xt
}

func (e *Unary) Type() Type {
	if e.Op == "!" {
		return Bool
	}
	return e.X.Type()
}

func (e *Call) Type() Type {
	if e.T != nil {
		return e.T
	}
	if e.Func != nil {
		return e.Func.Result
	}
	return nil
}

// Type returns the field's type, the swizzle's type, or nil when the name
// does not resolve.
func (e *FieldExpr) Type() Type {
	switch t := e.X.Type().(type) {
	case StructRef:
		if t.S != nil {
			if f := t.S.Field(e.Name); f != nil {
				return f.Type
			}
		}
	case Vector:
		n := len(e.Name)
		if n < 1 || n > 4 || strings.Trim(e.Name, "XYZWxyzwRGBArgba") != "" {
			return nil
		}
		if n == 1 {
			return t.Elem
		}
		return Vector{Elem: t.Elem, N: n}
	}
	return nil
}

func (e *Index) Type() Type {
	switch t := e.X.Type().(type) {
	case Array:
		return t.Elem
	case Vector:
		return t.Elem
	}
	return nil
}
