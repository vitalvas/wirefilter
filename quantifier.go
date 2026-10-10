package wirefilter

import (
	"fmt"
	"strings"
)

// arrayBinding creates a private predicate tree whose unpack expressions refer
// to one element. The compiled AST and execution context remain untouched.
type arrayBinding struct {
	array   Expression
	key     string
	element LiteralExpr
}

func (b *arrayBinding) bind(expr Expression, depth int) (Expression, error) {
	if depth >= maxEvalDepth {
		return nil, fmt.Errorf("expression exceeds maximum evaluation depth of %d", maxEvalDepth)
	}
	depth++
	switch e := expr.(type) {
	case *UnpackExpr:
		w := &encWriter{}
		if err := w.writeExpr(e.Array); err != nil {
			return nil, err
		}
		key := string(w.buf)
		if b.array != nil && key != b.key {
			return nil, fmt.Errorf("all() requires unpack expressions from the same array")
		}
		b.array, b.key = e.Array, key
		return &b.element, nil
	case *BinaryExpr:
		left, err := b.bind(e.Left, depth)
		if err != nil {
			return nil, err
		}
		right, err := b.bind(e.Right, depth)
		return &BinaryExpr{Left: left, Operator: e.Operator, Right: right}, err
	case *UnaryExpr:
		operand, err := b.bind(e.Operand, depth)
		return &UnaryExpr{Operator: e.Operator, Operand: operand}, err
	case *IndexExpr:
		object, err := b.bind(e.Object, depth)
		if err != nil {
			return nil, err
		}
		index, err := b.bind(e.Index, depth)
		return &IndexExpr{Object: object, Index: index}, err
	case *RangeExpr:
		start, err := b.bind(e.Start, depth)
		if err != nil {
			return nil, err
		}
		end, err := b.bind(e.End, depth)
		return &RangeExpr{Start: start, End: end}, err
	case *ArrayExpr:
		elements, err := b.bindSlice(e.Elements, depth)
		return &ArrayExpr{Elements: elements}, err
	case *FunctionCallExpr:
		// Nested quantifiers own their unpack expressions.
		name := strings.ToLower(e.Name)
		if name == "all" || name == "any" {
			return e, nil
		}
		args, err := b.bindSlice(e.Arguments, depth)
		return &FunctionCallExpr{Name: e.Name, Arguments: args}, err
	default:
		return expr, nil
	}
}

func (b *arrayBinding) bindSlice(exprs []Expression, depth int) ([]Expression, error) {
	bound := make([]Expression, len(exprs))
	for i, expr := range exprs {
		var err error
		bound[i], err = b.bind(expr, depth)
		if err != nil {
			return nil, err
		}
	}
	return bound, nil
}
