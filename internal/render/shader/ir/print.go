package ir

import (
	"fmt"
	"strconv"
	"strings"
)

// Print renders m as stable, human-readable text for test snapshots and
// debugging. The format is not a shading language; it is fixed so that
// equal modules always print identically:
//
//   - declarations in order: structs, uniforms, textures, helper Funcs,
//     then the Vertex and Fragment entry points (an entry point also put in
//     Funcs by mistake is printed once);
//   - entry points are prefixed with "vertex" / "fragment";
//   - binary, unary and select expressions are fully parenthesized;
//   - builtin calls print as sl.Name(...), user calls as name(...);
//   - f32 constants always carry a decimal point, i32 an "i" suffix and
//     u32 a "u" suffix.
func Print(m *Module) string {
	p := &printer{}
	for _, s := range m.Structs {
		p.structDecl(s)
	}
	for _, u := range m.Uniforms {
		name := "<nil>"
		if u.Type != nil {
			name = u.Type.Name
		}
		fmt.Fprintf(&p.b, "uniform %s: %s @binding(%d)\n", u.Name, name, u.Binding)
	}
	for _, t := range m.Textures {
		fmt.Fprintf(&p.b, "texture %s @binding(%d) %s\n", t.Name, t.Binding, t.Filter)
	}
	listed := map[*Func]bool{}
	for _, f := range m.Funcs {
		listed[f] = true
		p.funcDecl(m, f)
	}
	for _, f := range []*Func{m.Vertex, m.Fragment} {
		if f != nil && !listed[f] {
			listed[f] = true
			p.funcDecl(m, f)
		}
	}
	return p.b.String()
}

type printer struct {
	b      strings.Builder
	indent int
}

func (p *printer) line(format string, args ...any) {
	p.b.WriteString(strings.Repeat("  ", p.indent))
	fmt.Fprintf(&p.b, format, args...)
	p.b.WriteByte('\n')
}

func (p *printer) structDecl(s *Struct) {
	p.line("struct %s {", s.Name)
	p.indent++
	for _, f := range s.Fields {
		p.line("%s: %s%s", f.Name, typeString(f.Type), attrString(f.Attr))
	}
	p.indent--
	p.line("}")
}

func attrString(a Attr) string {
	var s string
	if a.Builtin != "" {
		s += " @builtin(" + a.Builtin + ")"
	}
	if a.Location >= 0 {
		s += " @location(" + strconv.Itoa(a.Location) + ")"
	}
	if a.Flat {
		s += " @flat"
	}
	return s
}

func (p *printer) funcDecl(m *Module, f *Func) {
	var prefix string
	if f == m.Vertex {
		prefix += "vertex "
	}
	if f == m.Fragment {
		prefix += "fragment "
	}
	param := func(v *Var) string {
		s := v.Name + ": " + typeString(v.Type)
		if v == f.VertexIDParam {
			s += " @builtin(vertex_id)"
		}
		return s
	}
	var params []string
	sawVID := false
	for _, v := range f.Params {
		sawVID = sawVID || v == f.VertexIDParam
		params = append(params, param(v))
	}
	if f.VertexIDParam != nil && !sawVID {
		params = append(params, param(f.VertexIDParam))
	}
	result := ""
	if f.Result != nil {
		result = " -> " + typeString(f.Result)
	}
	p.line("%sfunc %s(%s)%s {", prefix, f.Name, strings.Join(params, ", "), result)
	p.block(f.Body)
	p.line("}")
}

func (p *printer) block(body []Stmt) {
	p.indent++
	for _, s := range body {
		p.stmt(s)
	}
	p.indent--
}

func (p *printer) stmt(s Stmt) {
	switch s := s.(type) {
	case *If:
		p.ifStmt(s, strings.Repeat("  ", p.indent))
	case *For:
		p.line("for (%s; %s; %s) {", simpleStmt(s.Init), exprOrEmpty(s.Cond), simpleStmt(s.Post))
		p.block(s.Body)
		p.line("}")
	default:
		p.line("%s", simpleStmt(s))
	}
}

// ifStmt prints an if statement whose first line starts with lead (the
// indentation, or "} else " for an else-if chain).
func (p *printer) ifStmt(s *If, lead string) {
	fmt.Fprintf(&p.b, "%sif %s {\n", lead, expr(s.Cond))
	p.block(s.Then)
	ind := strings.Repeat("  ", p.indent)
	switch {
	case len(s.Else) == 1 && isIf(s.Else[0]):
		p.ifStmt(s.Else[0].(*If), ind+"} else ")
		return
	case len(s.Else) > 0:
		p.line("} else {")
		p.block(s.Else)
	}
	p.line("}")
}

func isIf(s Stmt) bool {
	_, ok := s.(*If)
	return ok
}

// simpleStmt renders a single-line statement (also used for For Init/Post).
func simpleStmt(s Stmt) string {
	switch s := s.(type) {
	case nil:
		return ""
	case *Decl:
		v := "var " + varName(s.V) + ": " + typeString(varType(s.V))
		if s.Init != nil {
			v += " = " + expr(s.Init)
		}
		return v
	case *Assign:
		return expr(s.LHS) + " = " + expr(s.RHS)
	case *Return:
		if s.Value == nil {
			return "return"
		}
		return "return " + expr(s.Value)
	case *Break:
		return "break"
	case *Continue:
		return "continue"
	case *ExprStmt:
		return expr(s.X)
	case *If:
		return "if " + expr(s.Cond) + " {...}"
	case *For:
		return "for {...}"
	}
	return fmt.Sprintf("<unknown stmt %T>", s)
}

func varName(v *Var) string {
	if v == nil {
		return "<nil var>"
	}
	return v.Name
}

func varType(v *Var) Type {
	if v == nil {
		return nil
	}
	return v.Type
}

func exprOrEmpty(e Expr) string {
	if e == nil {
		return ""
	}
	return expr(e)
}

func exprList(es []Expr) string {
	parts := make([]string, len(es))
	for i, e := range es {
		parts[i] = expr(e)
	}
	return strings.Join(parts, ", ")
}

func expr(e Expr) string {
	switch e := e.(type) {
	case nil:
		return "<nil>"
	case *Const:
		return constString(e)
	case *VarRef:
		return varName(e.V)
	case *GlobalRef:
		return e.Name
	case *Binary:
		return "(" + expr(e.X) + " " + e.Op + " " + expr(e.Y) + ")"
	case *Unary:
		return "(" + e.Op + expr(e.X) + ")"
	case *Call:
		name := "<nil func>"
		switch {
		case e.Builtin != "":
			name = "sl." + e.Builtin
		case e.Func != nil:
			name = e.Func.Name
		}
		return name + "(" + exprList(e.Args) + ")"
	case *FieldExpr:
		return expr(e.X) + "." + e.Name
	case *Index:
		return expr(e.X) + "[" + expr(e.I) + "]"
	case *Construct:
		return typeString(e.T) + "(" + exprList(e.Args) + ")"
	case *Convert:
		return "convert<" + typeString(e.T) + ">(" + expr(e.X) + ")"
	case *Select:
		return "(" + expr(e.Cond) + " ? " + expr(e.X) + " : " + expr(e.Y) + ")"
	}
	return fmt.Sprintf("<unknown expr %T>", e)
}

func constString(c *Const) string {
	switch t := c.T.(type) {
	case Scalar:
		return scalarConst(t, c.Value)
	case Vector:
		return t.String() + "(" + scalarConst(t.Elem, c.Value) + ")"
	}
	return typeString(c.T) + "(" + strconv.FormatFloat(c.Value, 'g', -1, 64) + ")"
}

func scalarConst(s Scalar, v float64) string {
	switch s {
	case F32:
		str := strconv.FormatFloat(v, 'g', -1, 32)
		if !strings.ContainsAny(str, ".eIN") { // keep 1e+20, +Inf, NaN as-is
			str += ".0"
		}
		return str
	case I32:
		return strconv.FormatInt(int64(v), 10) + "i"
	case U32:
		return strconv.FormatUint(uint64(v), 10) + "u"
	case Bool:
		return strconv.FormatBool(v != 0)
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}
