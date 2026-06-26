package codemetrics

import (
	"go/ast"
	"go/parser"
	"go/token"
)

// ParseGo computes cyclomatic and cognitive complexity for every function and
// method declaration with a body in a Go source file. Functions without a
// body (e.g. external or interface declarations) are skipped.
//
// Parsing is best-effort: if the parser recovers a partial syntax tree from
// malformed input, metrics are still computed for every function it found and
// the returned error is nil. A total parse failure (no tree) returns the
// parse error and no metrics.
func ParseGo(src []byte) ([]FunctionMetrics, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.SkipObjectResolution)
	if f == nil {
		return nil, err
	}
	var out []FunctionMetrics
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Body == nil {
			continue
		}
		cog := goCognitiveComplexity(fn)
		out = append(out, FunctionMetrics{
			Name:       fn.Name.Name,
			Receiver:   goReceiverType(fn),
			Cyclomatic: goComplexity(fn.Body),
			Cognitive:  &cog,
			StartLine:  fset.Position(fn.Pos()).Line,
			EndLine:    fset.Position(fn.End()).Line,
		})
	}
	return out, nil
}

// goComplexity returns the cyclomatic complexity of a function body using
// gocyclo's definition: 1 + one per branch point (if / for / range / case /
// comm-clause / && / ||).
func goComplexity(body *ast.BlockStmt) int {
	cx := 1
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.CaseClause, *ast.CommClause:
			cx++
		case *ast.BinaryExpr:
			if x.Op == token.LAND || x.Op == token.LOR {
				cx++
			}
		}
		return true
	})
	return cx
}

// goReceiverType renders the receiver type of a method (e.g. "*Buffer",
// "Buffer", or a generic "Stack" for `func (s *Stack[T]) ...`), or "" for a
// plain function or an unnamed/empty receiver.
func goReceiverType(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	return exprTypeName(fn.Recv.List[0].Type)
}

func exprTypeName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + exprTypeName(t.X)
	case *ast.IndexExpr: // generic receiver: T[P]
		return exprTypeName(t.X)
	case *ast.IndexListExpr: // generic receiver: T[P, Q]
		return exprTypeName(t.X)
	case *ast.SelectorExpr: // qualified, unusual for a receiver but be safe
		return exprTypeName(t.Sel)
	}
	return ""
}
