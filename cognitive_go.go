package codemetrics

import (
	"go/ast"
	"go/token"
)

// goCognitiveComplexity returns the SonarSource cognitive complexity of a Go
// function. Unlike cyclomatic complexity — which counts decision points
// flatly — cognitive complexity weights *nested* control flow more heavily, so
// deeply-nested logic scores higher than a flat sequence of the same number of
// branches, tracking how hard code is to understand.
//
// The algorithm follows the SonarSource specification (the reference
// implementation is github.com/uudashr/gocognit):
//   - B1 (structural increment, +1): if / else-if / else, switch / type-switch
//     / select, for / range loops, and a labelled break / continue / goto.
//   - B2 (nesting increment, +nesting): each structural increment from if /
//     switch / loop also adds the current nesting depth.
//   - B3 (nesting level): if / else / switch / loop bodies and nested function
//     literals each raise the nesting depth for what they contain.
//   - Hybrid (+1, no nesting): each *sequence* of like binary logical
//     operators (&& / ||), and direct recursion (a call to the function
//     being analysed).
func goCognitiveComplexity(fn *ast.FuncDecl) int {
	v := &goCognitiveVisitor{
		name:       fn.Name.Name,
		receiver:   goReceiverName(fn),
		elseIf:     map[*ast.IfStmt]bool{},
		calculated: map[*ast.BinaryExpr]bool{},
	}
	ast.Walk(v, fn)
	return v.complexity
}

// goReceiverName returns the receiver variable name of a method (e.g. "b" for
// `func (b *Buffer) …`), or "" for a plain function or an unnamed receiver.
func goReceiverName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 || len(fn.Recv.List[0].Names) == 0 {
		return ""
	}
	return fn.Recv.List[0].Names[0].Name
}

type goCognitiveVisitor struct {
	complexity int
	nesting    int
	name       string                   // enclosing function/method name, for recursion detection
	receiver   string                   // receiver variable name (methods), for `recv.Method()` recursion
	elseIf     map[*ast.IfStmt]bool     // if-statements reached as an `else if` (no nesting penalty)
	calculated map[*ast.BinaryExpr]bool // logical sub-expressions already folded into a sequence
}

func (v *goCognitiveVisitor) Visit(n ast.Node) ast.Visitor {
	switch n := n.(type) {
	case *ast.IfStmt:
		v.ifStmt(n)
		return nil
	case *ast.SwitchStmt:
		v.complexity += 1 + v.nesting
		v.walkNested(n.Body)
		return nil
	case *ast.TypeSwitchStmt:
		v.complexity += 1 + v.nesting
		v.walkNested(n.Body)
		return nil
	case *ast.SelectStmt:
		v.complexity += 1 + v.nesting
		v.walkNested(n.Body)
		return nil
	case *ast.ForStmt:
		v.complexity += 1 + v.nesting
		v.walkChildren(n.Init, n.Cond, n.Post)
		v.walkNested(n.Body)
		return nil
	case *ast.RangeStmt:
		v.complexity += 1 + v.nesting
		v.walkChildren(n.Key, n.Value, n.X)
		v.walkNested(n.Body)
		return nil
	case *ast.FuncLit:
		// A nested function raises the nesting level but is not itself an
		// increment.
		v.nesting++
		ast.Walk(v, n.Body)
		v.nesting--
		return nil
	case *ast.BranchStmt:
		if n.Label != nil { // labelled break / continue / goto
			v.complexity++
		}
	case *ast.BinaryExpr:
		v.binaryExpr(n)
	case *ast.CallExpr:
		if v.isRecursion(n.Fun) {
			v.complexity++ // direct recursion
		}
	}
	return v
}

// ifStmt handles the if / else-if / else chain so that an `else if` adds a
// flat increment (no nesting penalty) while a fresh `if` adds 1 + nesting.
func (v *goCognitiveVisitor) ifStmt(n *ast.IfStmt) {
	if v.elseIf[n] {
		v.complexity++
	} else {
		v.complexity += 1 + v.nesting
	}
	v.walkChildren(n.Init, n.Cond)
	v.walkNested(n.Body)

	switch e := n.Else.(type) {
	case *ast.IfStmt: // else if — continuation, no extra nesting
		v.elseIf[e] = true
		ast.Walk(v, e)
	case *ast.BlockStmt: // plain else — +1, then nest its body
		v.complexity++
		v.walkNested(e)
	}
}

// walkNested walks n at one deeper nesting level.
func (v *goCognitiveVisitor) walkNested(n ast.Node) {
	if n == nil {
		return
	}
	v.nesting++
	ast.Walk(v, n)
	v.nesting--
}

// walkChildren walks each non-nil node at the current nesting level (used for
// the init / cond / post parts of a control structure, where logical-operator
// sequences and func literals still count but no nesting is added).
func (v *goCognitiveVisitor) walkChildren(nodes ...ast.Node) {
	for _, n := range nodes {
		if n != nil {
			ast.Walk(v, n)
		}
	}
}

// binaryExpr counts each maximal sequence of like logical operators once:
// `a && b && c` is +1, `a && b || c` is +2. Parenthesising resets the run, so
// `a && (b || c)` is +2.
func (v *goCognitiveVisitor) binaryExpr(n *ast.BinaryExpr) {
	if v.calculated[n] || !isLogicalOp(n.Op) {
		return
	}
	var ops []token.Token
	v.collectLogicalOps(n, &ops)
	last := token.ILLEGAL
	for _, op := range ops {
		if op != last {
			v.complexity++
		}
		last = op
	}
}

// collectLogicalOps flattens a logical-operator tree in source order, marking
// each visited logical BinaryExpr as calculated so Visit doesn't recount it,
// and walking non-logical operands normally (to find nested func lits / calls).
func (v *goCognitiveVisitor) collectLogicalOps(n *ast.BinaryExpr, ops *[]token.Token) {
	v.calculated[n] = true
	v.collectOperand(n.X, ops)
	*ops = append(*ops, n.Op)
	v.collectOperand(n.Y, ops)
}

func (v *goCognitiveVisitor) collectOperand(e ast.Expr, ops *[]token.Token) {
	if be, ok := e.(*ast.BinaryExpr); ok && isLogicalOp(be.Op) {
		v.collectLogicalOps(be, ops)
		return
	}
	// A parenthesised or non-logical operand breaks the sequence; walk it so
	// any logical sub-expressions inside form their own runs.
	ast.Walk(v, e)
}

func isLogicalOp(op token.Token) bool {
	return op == token.LAND || op == token.LOR
}

// isRecursion reports whether a call targets the function being analysed —
// either a plain `f(...)` (Ident == name) or, for a method, a call on the
// receiver `recv.Method(...)` (SelectorExpr with X == receiver, Sel == name).
// Name-based; it does not resolve types.
func (v *goCognitiveVisitor) isRecursion(fun ast.Expr) bool {
	switch f := fun.(type) {
	case *ast.Ident:
		return f.Name == v.name
	case *ast.SelectorExpr:
		if v.receiver == "" || f.Sel == nil || f.Sel.Name != v.name {
			return false
		}
		recv, ok := f.X.(*ast.Ident)
		return ok && recv.Name == v.receiver
	}
	return false
}
