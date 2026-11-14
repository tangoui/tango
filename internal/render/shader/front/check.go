package front

import (
	"go/ast"
	"go/token"
	"go/types"
)

// checkFunc reports the constructs in d that are outside the shader subset.
// It runs before lowering, so the lowering can assume a well-formed subset
// (it still reports types it cannot map, such as float64).
func (c *compiler) checkFunc(d *ast.FuncDecl) {
	if d.Recv != nil {
		c.errorf(d.Name.Pos(), "methods are not supported")
		return
	}
	if d.Type.TypeParams != nil {
		c.errorf(d.Name.Pos(), "generic functions are not supported")
		return
	}
	sig := c.signature(d)
	if sig.Variadic() {
		c.errorf(d.Name.Pos(), "variadic functions are not supported")
	}
	if sig.Results().Len() > 1 {
		c.errorf(d.Type.Results.Pos(), "multiple return values are not supported")
	}
	if r := d.Type.Results; r != nil && len(r.List) > 0 && len(r.List[0].Names) > 0 {
		c.errorf(r.Pos(), "named results are not supported")
	}
	c.checkNode(d.Type.Params)
	c.checkNode(d.Body)
}

// checkNode walks n and reports unsupported syntax at its position.
func (c *compiler) checkNode(n ast.Node) {
	ast.Inspect(n, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			c.errorf(n.Pos(), "function literals are not supported")
			return false
		case *ast.GoStmt:
			c.errorf(n.Pos(), "goroutines are not supported")
		case *ast.DeferStmt:
			c.errorf(n.Pos(), "defer is not supported")
		case *ast.SelectStmt, *ast.SendStmt, *ast.ChanType:
			c.errorf(n.Pos(), "channels are not supported")
		case *ast.SwitchStmt, *ast.TypeSwitchStmt:
			c.errorf(n.Pos(), "switch statements are not supported; use if/else")
		case *ast.RangeStmt:
			c.errorf(n.Pos(), "range loops are not supported; use for i := 0; i < n; i++")
		case *ast.ForStmt:
			if n.Init == nil || n.Cond == nil || n.Post == nil {
				c.errorf(n.Pos(), "loops must have init, condition and post")
			}
		case *ast.IfStmt:
			if n.Init != nil {
				c.errorf(n.Init.Pos(), "if statements with an init statement are not supported")
			}
		case *ast.LabeledStmt:
			c.errorf(n.Pos(), "labels are not supported")
		case *ast.BranchStmt:
			if n.Tok == token.GOTO || n.Tok == token.FALLTHROUGH {
				c.errorf(n.Pos(), "%s is not supported", n.Tok)
			} else if n.Label != nil {
				c.errorf(n.Pos(), "labeled %s is not supported", n.Tok)
			}
		case *ast.StarExpr:
			c.errorf(n.Pos(), "pointers are not supported")
		case *ast.UnaryExpr:
			switch n.Op {
			case token.AND:
				c.errorf(n.Pos(), "pointers are not supported")
			case token.ARROW:
				c.errorf(n.Pos(), "channels are not supported")
			}
		case *ast.SliceExpr:
			c.errorf(n.Pos(), "slices are not supported")
		case *ast.ArrayType:
			if n.Len == nil {
				c.errorf(n.Pos(), "slices are not supported")
				return false
			}
		case *ast.Ellipsis:
			c.errorf(n.Pos(), "variadic arguments are not supported")
		case *ast.MapType:
			c.errorf(n.Pos(), "maps are not supported")
		case *ast.InterfaceType:
			c.errorf(n.Pos(), "interfaces are not supported")
		case *ast.TypeAssertExpr:
			c.errorf(n.Pos(), "type assertions are not supported")
		case *ast.DeclStmt:
			if g, ok := n.Decl.(*ast.GenDecl); ok && g.Tok != token.VAR {
				if g.Tok == token.TYPE {
					c.errorf(n.Pos(), "local type declarations are not supported")
				}
				// Local consts are folded by go/types; nothing to emit.
			}
		case *ast.CallExpr:
			c.checkCall(n)
		}
		return true
	})
	c.checkBlocks(n)
}

// checkCall rejects Go builtins (len, append, new, ...) used as calls.
func (c *compiler) checkCall(call *ast.CallExpr) {
	id, ok := ast.Unparen(call.Fun).(*ast.Ident)
	if !ok {
		return
	}
	if b, ok := c.info.Uses[id].(*types.Builtin); ok {
		switch b.Name() {
		case "new":
			c.errorf(call.Pos(), "pointers are not supported")
		case "make", "append":
			c.errorf(call.Pos(), "slices are not supported")
		case "min", "max":
			c.errorf(call.Pos(), "builtin %s is not supported; use sl.Min or sl.Max", b.Name())
		default:
			c.errorf(call.Pos(), "builtin %s is not supported", b.Name())
		}
	}
}

// checkBlocks reports bare { ... } blocks among statements.
func (c *compiler) checkBlocks(n ast.Node) {
	ast.Inspect(n, func(n ast.Node) bool {
		var list []ast.Stmt
		switch n := n.(type) {
		case *ast.BlockStmt:
			list = n.List
		case *ast.FuncLit:
			return false
		}
		for _, s := range list {
			if b, ok := s.(*ast.BlockStmt); ok {
				c.errorf(b.Pos(), "bare blocks are not supported")
			}
		}
		return true
	})
}
