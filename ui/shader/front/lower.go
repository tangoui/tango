package front

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"math"
	"reflect"
	"strconv"
	"strings"

	"github.com/tangoui/tango/ui/shader/ir"
)

// Lowering conventions (what the backends can rely on):
//
//   - sl vector methods a.Add(b), a.Sub(b), a.Mul(b), a.Div(b) become
//     Binary{"+"|"-"|"*"|"/"} of two vectors; a.Scale(s) becomes
//     Binary{"*", vector, scalar}.
//   - Swizzles become FieldExpr with lowercase component names, so a backend
//     can print them directly: v.X is FieldExpr{Name: "x"}, v.XY() is
//     FieldExpr{Name: "xy"}, v.XYZ() is FieldExpr{Name: "xyz"}. Struct field
//     accesses keep the Go field name (FieldExpr{Name: "Pos"}).
//   - Vector constructors sl.V2/V3/V4(...) and composite literals of
//     package-local structs or sl vectors become Construct with every
//     component in field order (omitted fields are zero constants).
//   - Every other sl function sl.F(...) becomes Call{Builtin: "F", T: result}.
//   - tex.Sample(uv) becomes Call{Builtin: "Sample", T: vec4<f32>,
//     Args: [GlobalRef{tex, Texture2D{}}, uv]}.
//   - Uniform and texture globals are GlobalRef{Name, T}; locals and
//     parameters are VarRef.
//   - Local variable names are unique within their function (a local that
//     reuses a parameter or earlier local name, e.g. a shadowing x := x + 1,
//     becomes x_1, x_2, ...), so backends can print names verbatim.
//   - Module.Structs lists each struct after the structs it contains.
//   - Constant expressions (including package-level consts) are folded to a
//     scalar Const by go/types; foldBinary also folds a Binary of two Consts.
//   - Go int is i32, uint32 is u32; x++ / x += y become Assign.

// lowerModule creates the IR functions (helpers callees-first, then the
// entry points) and lowers their bodies.
func (c *compiler) lowerModule(helpers []*ast.FuncDecl) {
	for _, d := range helpers {
		f := c.funcShell(d)
		c.mod.Funcs = append(c.mod.Funcs, f)
	}
	c.mod.Vertex = c.funcShell(c.vertex)
	c.mod.Fragment = c.funcShell(c.fragment)
	for _, d := range append(helpers, c.vertex, c.fragment) {
		fn := c.funcs[c.info.Defs[d.Name].(*types.Func)]
		l := &lowerer{c: c, fn: fn, names: map[string]bool{}}
		for _, p := range fn.Params {
			l.names[p.Name] = true
		}
		if fn.VertexIDParam != nil {
			l.names[fn.VertexIDParam.Name] = true
		}
		fn.Body = l.block(d.Body.List)
	}
}

// funcShell creates the ir.Func for d with its parameters and result type.
func (c *compiler) funcShell(d *ast.FuncDecl) *ir.Func {
	obj := c.info.Defs[d.Name].(*types.Func)
	sig := obj.Type().(*types.Signature)
	f := &ir.Func{Name: d.Name.Name}
	for i := 0; i < sig.Params().Len(); i++ {
		p := sig.Params().At(i)
		name := p.Name()
		if name == "" || name == "_" {
			name = "_p" + strconv.Itoa(i)
		}
		v := &ir.Var{Name: name, Type: c.valueType(p.Type(), p.Pos())}
		c.vars[p] = v
		if d == c.vertex && i == 1 {
			f.VertexIDParam = v
			continue
		}
		f.Params = append(f.Params, v)
	}
	if sig.Results().Len() == 1 {
		f.Result = c.valueType(sig.Results().At(0).Type(), d.Type.Results.Pos())
	}
	c.funcs[obj] = f
	return f
}

// valueType is irType for parameters, locals and results, which cannot be
// textures.
func (c *compiler) valueType(t types.Type, pos token.Pos) ir.Type {
	it := c.irType(t, pos)
	if _, ok := it.(ir.Texture2D); ok {
		c.errorf(pos, "sl.Texture2D is only allowed as a //sl:texture package-level var")
		return nil
	}
	return it
}

// irType maps a Go type to an IR type, reporting unsupported types at pos
// (and returning nil).
func (c *compiler) irType(t types.Type, pos token.Pos) ir.Type {
	switch t := types.Unalias(t).(type) {
	case *types.Basic:
		switch t.Kind() {
		case types.Float32, types.UntypedFloat:
			return ir.F32
		case types.Int, types.Int32, types.UntypedInt, types.UntypedRune:
			return ir.I32
		case types.Uint32:
			return ir.U32
		case types.Bool, types.UntypedBool:
			return ir.Bool
		case types.Float64:
			c.errorf(pos, "float64 is not supported; use float32")
		default:
			c.errorf(pos, "type %s is not supported", c.ts(t))
		}
	case *types.Named:
		obj := t.Obj()
		if obj.Pkg() != nil && obj.Pkg().Path() == slPath {
			switch obj.Name() {
			case "Vec2":
				return ir.Vector{Elem: ir.F32, N: 2}
			case "Vec3":
				return ir.Vector{Elem: ir.F32, N: 3}
			case "Vec4":
				return ir.Vector{Elem: ir.F32, N: 4}
			case "Texture2D":
				return ir.Texture2D{}
			}
			c.errorf(pos, "type sl.%s is not supported in shaders", obj.Name())
			return nil
		}
		if obj.Pkg() != c.pkg {
			c.errorf(pos, "type %s from another package is not supported", c.ts(t))
			return nil
		}
		if t.TypeArgs().Len() > 0 {
			c.errorf(pos, "generic types are not supported")
			return nil
		}
		switch u := t.Underlying().(type) {
		case *types.Struct:
			s := c.structFor(obj, u)
			if s == nil {
				return nil
			}
			return ir.StructRef{S: s}
		default:
			return c.irType(u, pos)
		}
	case *types.Array:
		elem := c.irType(t.Elem(), pos)
		if elem == nil {
			return nil
		}
		if _, ok := elem.(ir.Texture2D); ok {
			c.errorf(pos, "arrays of textures are not supported")
			return nil
		}
		if t.Len() <= 0 || t.Len() > math.MaxInt32 {
			c.errorf(pos, "array length %d is not supported", t.Len())
			return nil
		}
		return ir.Array{Elem: elem, Len: int(t.Len())}
	case *types.Pointer:
		c.errorf(pos, "pointers are not supported")
	case *types.Slice:
		c.errorf(pos, "slices are not supported")
	case *types.Map:
		c.errorf(pos, "maps are not supported")
	case *types.Chan:
		c.errorf(pos, "channels are not supported")
	case *types.Signature:
		c.errorf(pos, "function values are not supported")
	case *types.Interface:
		c.errorf(pos, "interfaces are not supported")
	case *types.Struct:
		c.errorf(pos, "anonymous structs are not supported")
	case *types.Tuple:
		c.errorf(pos, "multiple values are not supported")
	default:
		c.errorf(pos, "type %s is not supported", c.ts(t))
	}
	return nil
}

// structFor returns the IR struct for the package-level struct type obj,
// converting it (and recording it in the module) on first use.
func (c *compiler) structFor(obj *types.TypeName, st *types.Struct) *ir.Struct {
	if s, ok := c.structs[obj]; ok {
		return s
	}
	if obj.Parent() != c.pkg.Scope() {
		c.errorf(obj.Pos(), "local type declarations are not supported")
		c.structs[obj] = nil
		return nil
	}
	s := &ir.Struct{Name: obj.Name()}
	c.structs[obj] = s // before the fields: arrays of structs refer back here
	ok := true
	vars := make([]*types.Var, st.NumFields())
	for i := range vars {
		vars[i] = st.Field(i)
	}
	offs := goSizes.Offsetsof(vars)
	s.Size = int(goSizes.Sizeof(st))
	for i := 0; i < st.NumFields(); i++ {
		fv := st.Field(i)
		if fv.Embedded() {
			c.errorf(fv.Pos(), "embedded fields are not supported")
			ok = false
			continue
		}
		ft := c.irType(fv.Type(), fv.Pos())
		if _, isTex := ft.(ir.Texture2D); isTex {
			c.errorf(fv.Pos(), "sl.Texture2D is only allowed as a //sl:texture package-level var")
			ft = nil
		}
		attr, err := parseTag(reflect.StructTag(st.Tag(i)).Get("sl"))
		if err != "" {
			c.errorf(fv.Pos(), "field %s: %s", fv.Name(), err)
			ok = false
		}
		if ft == nil {
			ok = false
		}
		s.Fields = append(s.Fields, ir.Field{Name: fv.Name(), Type: ft, Attr: attr, Offset: int(offs[i])})
	}
	if !ok {
		c.structs[obj] = nil
		return nil
	}
	// Appended after its fields were converted, so every struct follows the
	// structs it contains (declaration before use), whatever the source order.
	c.mod.Structs = append(c.mod.Structs, s)
	return s
}

// parseTag parses an sl struct tag: "", "position", "loc=N" or "loc=N,flat".
func parseTag(tag string) (ir.Attr, string) {
	a := ir.Attr{Location: -1}
	if tag == "" {
		return a, ""
	}
	if tag == "position" {
		a.Builtin = "position"
		return a, ""
	}
	loc, flat := strings.CutSuffix(tag, ",flat")
	n, ok := strings.CutPrefix(loc, "loc=")
	v, err := strconv.Atoi(n)
	if !ok || err != nil || v < 0 || strconv.Itoa(v) != n {
		return a, fmt.Sprintf(`invalid sl tag %q (want "position", "loc=N" or "loc=N,flat")`, tag)
	}
	a.Location, a.Flat = v, flat
	return a, ""
}

// lowerer lowers the body of one function.
type lowerer struct {
	c  *compiler
	fn *ir.Func
	// names holds every parameter and local name used so far in fn. Go
	// allows shadowing (x := x + 1 in an inner block), C-like backends do
	// not read it the same way, so each local gets a name unique in fn.
	names map[string]bool
}

// uniqueName returns name, or name_1, name_2, ... if it is already used in
// the function, and marks the result used.
func (l *lowerer) uniqueName(name string) string {
	n := name
	for i := 1; l.names[n]; i++ {
		n = name + "_" + strconv.Itoa(i)
	}
	l.names[n] = true
	return n
}

func (l *lowerer) errorf(pos token.Pos, format string, args ...any) {
	l.c.errorf(pos, format, args...)
}

func (l *lowerer) block(list []ast.Stmt) []ir.Stmt {
	var out []ir.Stmt
	for _, s := range list {
		out = append(out, l.stmt(s)...)
	}
	return out
}

// stmt lowers one statement; a multi-variable declaration yields several.
func (l *lowerer) stmt(s ast.Stmt) []ir.Stmt {
	switch s := s.(type) {
	case *ast.EmptyStmt:
		return nil
	case *ast.DeclStmt:
		return l.declStmt(s)
	case *ast.AssignStmt:
		return l.assign(s)
	case *ast.IncDecStmt:
		x := l.lvalue(s.X)
		op := "+"
		if s.Tok == token.DEC {
			op = "-"
		}
		one := &ir.Const{T: x.Type(), Value: 1}
		return []ir.Stmt{&ir.Assign{LHS: x, RHS: foldBinary(op, l.expr(s.X), one)}}
	case *ast.ExprStmt:
		if _, ok := ast.Unparen(s.X).(*ast.CallExpr); !ok {
			l.errorf(s.Pos(), "expression statements must be calls")
			return nil
		}
		return []ir.Stmt{&ir.ExprStmt{X: l.expr(s.X)}}
	case *ast.ReturnStmt:
		switch len(s.Results) {
		case 0:
			return []ir.Stmt{&ir.Return{}}
		case 1:
			return []ir.Stmt{&ir.Return{Value: l.expr(s.Results[0])}}
		}
		l.errorf(s.Pos(), "multiple return values are not supported")
	case *ast.IfStmt:
		return []ir.Stmt{l.ifStmt(s)}
	case *ast.ForStmt:
		f := &ir.For{Init: l.single(s.Init), Cond: l.expr(s.Cond), Post: l.single(s.Post)}
		f.Body = l.block(s.Body.List)
		return []ir.Stmt{f}
	case *ast.BranchStmt:
		switch s.Tok {
		case token.BREAK:
			return []ir.Stmt{&ir.Break{}}
		case token.CONTINUE:
			return []ir.Stmt{&ir.Continue{}}
		}
		l.errorf(s.Pos(), "%s is not supported", s.Tok)
	default:
		l.errorf(s.Pos(), "unsupported statement %T", s)
	}
	return nil
}

// single lowers a for-loop init or post statement, which must be exactly
// one IR statement.
func (l *lowerer) single(s ast.Stmt) ir.Stmt {
	out := l.stmt(s)
	if len(out) != 1 {
		l.errorf(s.Pos(), "loop init and post must be a single statement")
		return nil
	}
	return out[0]
}

func (l *lowerer) ifStmt(s *ast.IfStmt) *ir.If {
	n := &ir.If{Cond: l.expr(s.Cond), Then: l.block(s.Body.List)}
	switch e := s.Else.(type) {
	case *ast.IfStmt:
		n.Else = []ir.Stmt{l.ifStmt(e)}
	case *ast.BlockStmt:
		n.Else = l.block(e.List)
	}
	return n
}

func (l *lowerer) declStmt(s *ast.DeclStmt) []ir.Stmt {
	g := s.Decl.(*ast.GenDecl)
	if g.Tok != token.VAR {
		return nil // consts are folded at their uses; types were rejected
	}
	var out []ir.Stmt
	for _, spec := range g.Specs {
		vs := spec.(*ast.ValueSpec)
		if len(vs.Values) != 0 && len(vs.Values) != len(vs.Names) {
			l.errorf(vs.Pos(), "multiple values are not supported")
			continue
		}
		for i, id := range vs.Names {
			var init ir.Expr
			if len(vs.Values) != 0 {
				init = l.expr(vs.Values[i])
			}
			if d := l.define(id, init); d != nil {
				out = append(out, d)
			}
		}
	}
	return out
}

// define declares the local variable id with an optional initializer.
func (l *lowerer) define(id *ast.Ident, init ir.Expr) ir.Stmt {
	if id.Name == "_" {
		l.errorf(id.Pos(), "the blank identifier is not supported")
		return nil
	}
	obj := l.c.info.Defs[id].(*types.Var)
	v := &ir.Var{Name: l.uniqueName(id.Name), Type: l.c.valueType(obj.Type(), id.Pos())}
	l.c.vars[obj] = v
	return &ir.Decl{V: v, Init: init}
}

var assignOps = map[token.Token]string{
	token.ADD_ASSIGN: "+", token.SUB_ASSIGN: "-", token.MUL_ASSIGN: "*", token.QUO_ASSIGN: "/",
}

func (l *lowerer) assign(s *ast.AssignStmt) []ir.Stmt {
	if len(s.Lhs) != len(s.Rhs) {
		l.errorf(s.Pos(), "multiple values are not supported")
		return nil
	}
	switch s.Tok {
	case token.DEFINE:
		// Several variables are fine when all are new: none of them is in
		// scope on the right-hand side, so declaring them in order is exact.
		var out []ir.Stmt
		for i, lhs := range s.Lhs {
			id := lhs.(*ast.Ident)
			if l.c.info.Defs[id] == nil && id.Name != "_" {
				if len(s.Lhs) > 1 {
					l.errorf(id.Pos(), "redeclaring %s in a multi-variable := is not supported", id.Name)
					return nil
				}
				return []ir.Stmt{&ir.Assign{LHS: l.lvalue(id), RHS: l.expr(s.Rhs[i])}}
			}
			if d := l.define(id, l.expr(s.Rhs[i])); d != nil {
				out = append(out, d)
			}
		}
		return out
	case token.ASSIGN:
		if len(s.Lhs) > 1 {
			l.errorf(s.Pos(), "multiple assignment is not supported; assign one variable per statement")
			return nil
		}
		return []ir.Stmt{&ir.Assign{LHS: l.lvalue(s.Lhs[0]), RHS: l.expr(s.Rhs[0])}}
	}
	op, ok := assignOps[s.Tok]
	if !ok {
		l.errorf(s.Pos(), "operator %s is not supported", s.Tok)
		return nil
	}
	return []ir.Stmt{&ir.Assign{LHS: l.lvalue(s.Lhs[0]), RHS: foldBinary(op, l.expr(s.Lhs[0]), l.expr(s.Rhs[0]))}}
}

// lvalue lowers an assignment target, which must be rooted at a local
// variable or parameter.
func (l *lowerer) lvalue(e ast.Expr) ir.Expr {
	if id, ok := ast.Unparen(e).(*ast.Ident); ok && id.Name == "_" {
		l.errorf(id.Pos(), "the blank identifier is not supported")
		return nil
	}
	x := l.expr(e)
	for root := x; root != nil; {
		switch r := root.(type) {
		case *ir.FieldExpr:
			root = r.X
		case *ir.Index:
			root = r.X
		case *ir.VarRef:
			return x
		case *ir.GlobalRef:
			l.errorf(e.Pos(), "cannot assign to //sl:uniform or //sl:texture var %s", r.Name)
			return x
		default:
			l.errorf(e.Pos(), "cannot assign to this expression")
			return x
		}
	}
	return x
}

var binaryOps = map[token.Token]string{
	token.ADD: "+", token.SUB: "-", token.MUL: "*", token.QUO: "/",
	token.LSS: "<", token.LEQ: "<=", token.GTR: ">", token.GEQ: ">=",
	token.EQL: "==", token.NEQ: "!=", token.LAND: "&&", token.LOR: "||",
}

// expr lowers an expression. On error it reports and returns a placeholder
// so lowering can continue and report further errors.
func (l *lowerer) expr(e ast.Expr) ir.Expr {
	c := l.c
	if tv, ok := c.info.Types[e]; ok && tv.Value != nil {
		return l.constant(tv.Type, tv.Value, e.Pos())
	}
	switch e := e.(type) {
	case *ast.ParenExpr:
		return l.expr(e.X)
	case *ast.Ident:
		return l.ident(e)
	case *ast.BinaryExpr:
		op, ok := binaryOps[e.Op]
		if !ok {
			l.errorf(e.OpPos, "operator %s is not supported", e.Op)
			return bad
		}
		if _, isScalar := c.info.Types[e.X].Type.Underlying().(*types.Basic); !isScalar {
			l.errorf(e.OpPos, "operator %s on %s values is not supported", e.Op, c.ts(c.info.Types[e.X].Type))
			return bad
		}
		return foldBinary(op, l.expr(e.X), l.expr(e.Y))
	case *ast.UnaryExpr:
		switch e.Op {
		case token.ADD:
			return l.expr(e.X)
		case token.SUB:
			return &ir.Unary{Op: "-", X: l.expr(e.X)}
		case token.NOT:
			return &ir.Unary{Op: "!", X: l.expr(e.X)}
		}
		l.errorf(e.OpPos, "operator %s is not supported", e.Op)
	case *ast.CallExpr:
		return l.call(e)
	case *ast.SelectorExpr:
		sel := c.info.Selections[e]
		if sel == nil || sel.Kind() != types.FieldVal {
			l.errorf(e.Sel.Pos(), "%s can only be called", e.Sel.Name)
			return bad
		}
		if len(sel.Index()) != 1 {
			l.errorf(e.Sel.Pos(), "embedded fields are not supported")
			return bad
		}
		x := l.expr(e.X)
		name := e.Sel.Name
		if _, ok := x.Type().(ir.Vector); ok {
			name = strings.ToLower(name)
		}
		return &ir.FieldExpr{X: x, Name: name}
	case *ast.IndexExpr:
		if _, ok := c.info.Types[e.X].Type.Underlying().(*types.Array); !ok {
			l.errorf(e.Pos(), "only arrays can be indexed")
			return bad
		}
		return &ir.Index{X: l.expr(e.X), I: l.expr(e.Index)}
	case *ast.CompositeLit:
		return l.composite(e)
	default:
		l.errorf(e.Pos(), "unsupported expression %T", e)
	}
	return bad
}

// bad is the placeholder for an expression that failed to lower.
var bad = &ir.Const{T: ir.F32}

func (l *lowerer) ident(id *ast.Ident) ir.Expr {
	switch obj := l.c.info.Uses[id].(type) {
	case *types.Var:
		if v, ok := l.c.vars[obj]; ok {
			return &ir.VarRef{V: v}
		}
		if g, ok := l.c.globals[obj]; ok {
			r := *g.(*ir.GlobalRef)
			return &r
		}
		if obj.Parent() == l.c.pkg.Scope() {
			l.errorf(id.Pos(), "package-level var %s must be marked //sl:uniform or //sl:texture", id.Name)
			return bad
		}
	case *types.Func:
		l.errorf(id.Pos(), "function values are not supported")
		return bad
	case *types.Nil:
		l.errorf(id.Pos(), "nil is not supported")
		return bad
	}
	l.errorf(id.Pos(), "unsupported identifier %s", id.Name)
	return bad
}

// constant lowers a go/types constant value of type t.
func (l *lowerer) constant(t types.Type, v constant.Value, pos token.Pos) ir.Expr {
	it := l.c.irType(t, pos)
	s, ok := it.(ir.Scalar)
	if !ok {
		if it != nil {
			l.errorf(pos, "constant of type %s is not supported", l.c.ts(t))
		}
		return bad
	}
	if v.Kind() == constant.Bool {
		if constant.BoolVal(v) {
			return &ir.Const{T: ir.Bool, Value: 1}
		}
		return &ir.Const{T: ir.Bool, Value: 0}
	}
	f, _ := constant.Float64Val(constant.ToFloat(v))
	switch s {
	case ir.F32:
		f = float64(float32(f))
	case ir.I32:
		if f != math.Trunc(f) || f < math.MinInt32 || f > math.MaxInt32 {
			l.errorf(pos, "constant %s overflows i32", v)
		}
	case ir.U32:
		if f != math.Trunc(f) || f < 0 || f > math.MaxUint32 {
			l.errorf(pos, "constant %s overflows u32", v)
		}
	}
	return &ir.Const{T: s, Value: f}
}

// foldBinary builds x op y, evaluating it when both sides are scalar
// constants of the same type.
func foldBinary(op string, x, y ir.Expr) ir.Expr {
	cx, ok1 := x.(*ir.Const)
	cy, ok2 := y.(*ir.Const)
	if !ok1 || !ok2 || cx.T != cy.T {
		return &ir.Binary{Op: op, X: x, Y: y}
	}
	s, ok := cx.T.(ir.Scalar)
	if !ok {
		return &ir.Binary{Op: op, X: x, Y: y}
	}
	a, b := cx.Value, cy.Value
	boolean := func(v bool) ir.Expr {
		if v {
			return &ir.Const{T: ir.Bool, Value: 1}
		}
		return &ir.Const{T: ir.Bool, Value: 0}
	}
	var r float64
	switch op {
	case "+":
		r = a + b
	case "-":
		r = a - b
	case "*":
		r = a * b
	case "/":
		if s != ir.F32 {
			if b == 0 {
				return &ir.Binary{Op: op, X: x, Y: y} // leave the trap to run time
			}
			r = math.Trunc(a / b)
		} else {
			r = a / b
		}
	case "<":
		return boolean(a < b)
	case "<=":
		return boolean(a <= b)
	case ">":
		return boolean(a > b)
	case ">=":
		return boolean(a >= b)
	case "==":
		return boolean(a == b)
	case "!=":
		return boolean(a != b)
	case "&&":
		return boolean(a != 0 && b != 0)
	case "||":
		return boolean(a != 0 || b != 0)
	default:
		return &ir.Binary{Op: op, X: x, Y: y}
	}
	switch s {
	case ir.F32:
		r = float64(float32(r))
	case ir.I32:
		r = float64(int32(int64(r)))
	case ir.U32:
		r = float64(uint32(int64(r)))
	}
	return &ir.Const{T: s, Value: r}
}

// call lowers conversions, sl builtins and methods, and helper calls.
func (l *lowerer) call(e *ast.CallExpr) ir.Expr {
	c := l.c
	if tv := c.info.Types[e.Fun]; tv.IsType() {
		return l.conversion(e, tv.Type)
	}
	switch fun := ast.Unparen(e.Fun).(type) {
	case *ast.SelectorExpr:
		if sel := c.info.Selections[fun]; sel != nil {
			if sel.Kind() != types.MethodVal {
				break
			}
			return l.method(e, fun, sel)
		}
		fn, ok := c.info.Uses[fun.Sel].(*types.Func)
		if !ok || fn.Pkg() == nil || fn.Pkg().Path() != slPath {
			l.errorf(fun.Pos(), "calls to %s are not supported", types.ExprString(fun))
			return bad
		}
		return l.builtin(e, fn)
	case *ast.Ident:
		fn, ok := c.info.Uses[fun].(*types.Func)
		if !ok {
			break
		}
		f := c.funcs[fn]
		if f == nil {
			l.errorf(fun.Pos(), "call to %s cannot be lowered", fun.Name)
			return bad
		}
		return &ir.Call{Func: f, Args: l.args(e.Args)}
	}
	l.errorf(e.Pos(), "unsupported call")
	return bad
}

func (l *lowerer) args(list []ast.Expr) []ir.Expr {
	out := make([]ir.Expr, len(list))
	for i, a := range list {
		out[i] = l.expr(a)
	}
	return out
}

// conversion lowers T(x) to Convert, or to x itself when the IR types match
// (for example int32(i) with i an int).
func (l *lowerer) conversion(e *ast.CallExpr, to types.Type) ir.Expr {
	t := l.c.irType(to, e.Pos())
	x := l.expr(e.Args[0])
	if t == nil {
		return bad
	}
	if x.Type() == t {
		return x
	}
	_, ts := t.(ir.Scalar)
	_, xs := x.Type().(ir.Scalar)
	if !ts || !xs {
		l.errorf(e.Pos(), "conversion to %s is not supported", l.c.ts(to))
		return bad
	}
	return &ir.Convert{T: t, X: x}
}

var vectorOps = map[string]string{"Add": "+", "Sub": "-", "Mul": "*", "Div": "/", "Scale": "*"}

// method lowers sl vector and texture method calls.
func (l *lowerer) method(e *ast.CallExpr, fun *ast.SelectorExpr, sel *types.Selection) ir.Expr {
	recv := sel.Recv()
	name := fun.Sel.Name
	switch {
	case isSL(recv, "Vec2") || isSL(recv, "Vec3") || isSL(recv, "Vec4"):
		x := l.expr(fun.X)
		if op, ok := vectorOps[name]; ok {
			return &ir.Binary{Op: op, X: x, Y: l.expr(e.Args[0])}
		}
		switch name {
		case "XY", "ZW", "XYZ":
			return &ir.FieldExpr{X: x, Name: strings.ToLower(name)}
		}
	case isSL(recv, "Texture2D") && name == "Sample":
		tex := l.expr(fun.X)
		if _, ok := tex.(*ir.GlobalRef); !ok {
			l.errorf(fun.X.Pos(), "Sample must be called on a //sl:texture var")
			return bad
		}
		return &ir.Call{Builtin: "Sample", T: ir.Vector{Elem: ir.F32, N: 4}, Args: []ir.Expr{tex, l.expr(e.Args[0])}}
	case !isSLPkg(recv):
		l.errorf(fun.Sel.Pos(), "methods are not supported")
		return bad
	}
	l.errorf(fun.Sel.Pos(), "method %s is not supported in shaders", name)
	return bad
}

// isSLPkg reports whether t (or *t) is a named type from package sl.
func isSLPkg(t types.Type) bool {
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	n, ok := types.Unalias(t).(*types.Named)
	return ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == slPath
}

// builtin lowers a call of an sl package function.
func (l *lowerer) builtin(e *ast.CallExpr, fn *types.Func) ir.Expr {
	args := l.args(e.Args)
	switch fn.Name() {
	case "V2", "V3", "V4":
		return &ir.Construct{T: ir.Vector{Elem: ir.F32, N: len(args)}, Args: args}
	case "NewTexture":
		l.errorf(e.Pos(), "sl.NewTexture is not available in shaders")
		return bad
	}
	res := fn.Type().(*types.Signature).Results()
	if res.Len() != 1 {
		l.errorf(e.Pos(), "sl.%s is not supported in shaders", fn.Name())
		return bad
	}
	return &ir.Call{Builtin: fn.Name(), T: l.c.valueType(res.At(0).Type(), e.Pos()), Args: args}
}

// composite lowers a struct, vector or array literal to Construct with every
// element present.
func (l *lowerer) composite(e *ast.CompositeLit) ir.Expr {
	gt := l.c.info.Types[e].Type
	t := l.c.valueType(gt, e.Pos())
	if t == nil {
		return bad
	}
	switch u := gt.Underlying().(type) {
	case *types.Struct:
		args := make([]ir.Expr, u.NumFields())
		for i, el := range e.Elts {
			if kv, ok := el.(*ast.KeyValueExpr); ok {
				name := kv.Key.(*ast.Ident).Name
				for j := 0; j < u.NumFields(); j++ {
					if u.Field(j).Name() == name {
						args[j] = l.expr(kv.Value)
					}
				}
				continue
			}
			args[i] = l.expr(el)
		}
		for i, a := range args {
			if a == nil {
				args[i] = l.zero(u.Field(i).Type(), e.Pos())
			}
		}
		return &ir.Construct{T: t, Args: args}
	case *types.Array:
		args := make([]ir.Expr, u.Len())
		for i, el := range e.Elts {
			if _, ok := el.(*ast.KeyValueExpr); ok {
				l.errorf(el.Pos(), "indexed array literals are not supported")
				return bad
			}
			args[i] = l.expr(el)
		}
		for i, a := range args {
			if a == nil {
				args[i] = l.zero(u.Elem(), e.Pos())
			}
		}
		return &ir.Construct{T: t, Args: args}
	}
	l.errorf(e.Pos(), "composite literal of %s is not supported", l.c.ts(gt))
	return bad
}

// zero returns the zero value of t: a (splat) Const for scalars and
// vectors, a Construct of zeros for structs and arrays.
func (l *lowerer) zero(t types.Type, pos token.Pos) ir.Expr {
	it := l.c.valueType(t, pos)
	switch it.(type) {
	case ir.Scalar, ir.Vector:
		return &ir.Const{T: it}
	case ir.StructRef:
		st := t.Underlying().(*types.Struct)
		args := make([]ir.Expr, st.NumFields())
		for i := range args {
			args[i] = l.zero(st.Field(i).Type(), pos)
		}
		return &ir.Construct{T: it, Args: args}
	case ir.Array:
		at := t.Underlying().(*types.Array)
		args := make([]ir.Expr, at.Len())
		for i := range args {
			args[i] = l.zero(at.Elem(), pos)
		}
		return &ir.Construct{T: it, Args: args}
	}
	return bad
}
