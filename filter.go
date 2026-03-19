// Package wirefilter implements a filtering expression language and execution engine.
// It allows you to compile and evaluate filter expressions against runtime data.
//
// The filter language supports:
//   - Logical operators: and, or, not, xor, &&, ||, !, ^^
//   - Comparison operators: ==, !=, <, >, <=, >=
//   - Array operators: === (all equal), !== (any not equal)
//   - Membership operators: in, contains, matches (~)
//   - Wildcard matching: wildcard, strict wildcard
//   - Range expressions: {1..10}
//   - Multiple data types: string, int, bool, IP, bytes, arrays, maps
//
// Example:
//
//	schema := wirefilter.NewSchema().
//	    AddField("http.host", wirefilter.TypeString).
//	    AddField("http.status", wirefilter.TypeInt)
//
//	filter, err := wirefilter.Compile(`http.host == "example.com" and http.status >= 400`, schema)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	ctx := wirefilter.NewExecutionContext().
//	    SetStringField("http.host", "example.com").
//	    SetIntField("http.status", 500)
//
//	result, err := filter.Execute(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(result) // true
package wirefilter

import (
	"encoding/hex"
	"hash/fnv"
	"net"
	"regexp"
	"sync"
	"time"
)

// RuleMeta holds metadata for a compiled filter rule.
type RuleMeta struct {
	ID   string
	Tags map[string]string
}

// Filter represents a compiled filter expression that can be executed against an execution context.
// Filter is safe for concurrent use across goroutines.
type Filter struct {
	expr       Expression
	schema     *Schema
	meta       RuleMeta
	regexCache map[string]*regexp.Regexp
	regexMu    sync.RWMutex
	cidrCache  map[string]*net.IPNet
	cidrMu     sync.RWMutex
}

// SetMeta attaches metadata to the compiled filter.
// The Tags map is defensively copied to prevent external mutation.
// Returns the filter to allow method chaining.
func (f *Filter) SetMeta(meta RuleMeta) *Filter {
	f.meta = RuleMeta{ID: meta.ID}
	if meta.Tags != nil {
		f.meta.Tags = make(map[string]string, len(meta.Tags))
		for k, v := range meta.Tags {
			f.meta.Tags[k] = v
		}
	}
	return f
}

// Meta returns the metadata attached to this filter.
// The Tags map is defensively copied to prevent external mutation.
func (f *Filter) Meta() RuleMeta {
	m := RuleMeta{ID: f.meta.ID}
	if f.meta.Tags != nil {
		m.Tags = make(map[string]string, len(f.meta.Tags))
		for k, v := range f.meta.Tags {
			m.Tags[k] = v
		}
	}
	return m
}

// Compile parses and compiles a filter expression string into an executable Filter.
// If a schema is provided, it validates that all fields referenced in the expression exist in the schema.
// Returns an error if the expression is malformed or references unknown fields.
func Compile(filterStr string, schema *Schema) (*Filter, error) {
	lexer := NewLexer(filterStr)
	parser := NewParser(lexer)

	expr, err := parser.Parse()
	if err != nil {
		return nil, err
	}

	if schema != nil {
		if err := schema.Validate(expr); err != nil {
			return nil, err
		}
	}

	return &Filter{
		expr:       expr,
		schema:     schema,
		regexCache: make(map[string]*regexp.Regexp),
		cidrCache:  make(map[string]*net.IPNet),
	}, nil
}

// Hash returns a hex-encoded hash of the compiled filter's canonical AST representation.
// Two expressions that are semantically identical produce the same hash, even if they
// differ in whitespace, operator aliases (and vs &&), or formatting.
// This can be used to deduplicate filter expressions.
func (f *Filter) Hash() string {
	data, err := f.MarshalBinary()
	if err != nil {
		return ""
	}

	h := fnv.New128a()
	h.Write(data)

	return hex.EncodeToString(h.Sum(nil))
}

// Execute evaluates the compiled filter against the provided execution context.
// Returns true if the filter matches, false otherwise.
// Returns an error if evaluation fails.
// If ctx is nil, an empty execution context is used.
func (f *Filter) Execute(ctx *ExecutionContext) (bool, error) {
	if ctx == nil {
		ctx = NewExecutionContext()
	}
	result, err := f.evaluate(f.expr, ctx)
	if err != nil {
		return false, err
	}

	if result == nil {
		return false, nil
	}

	return result.IsTruthy(), nil
}

func (f *Filter) evaluate(expr Expression, ctx *ExecutionContext) (Value, error) {
	// Check for context cancellation/timeout
	if err := ctx.checkContext(); err != nil {
		return nil, err
	}

	// Tracing: push before, pop after
	if ctx.traceEnabled() {
		ctx.pushTrace(exprString(expr))
		start := time.Now()
		result, err := f.evaluateInner(expr, ctx)
		ctx.popTrace(result, time.Since(start))
		return result, err
	}

	return f.evaluateInner(expr, ctx)
}

func (f *Filter) evaluateInner(expr Expression, ctx *ExecutionContext) (Value, error) {
	switch e := expr.(type) {
	case *BinaryExpr:
		return f.evaluateBinaryExpr(e, ctx)
	case *UnaryExpr:
		return f.evaluateUnaryExpr(e, ctx)
	case *FieldExpr:
		return f.evaluateFieldExpr(e, ctx)
	case *LiteralExpr:
		return e.Value, nil
	case *ArrayExpr:
		return f.evaluateArrayExpr(e, ctx)
	case *RangeExpr:
		return f.evaluateRangeExpr(e, ctx)
	case *IndexExpr:
		return f.evaluateIndexExpr(e, ctx)
	case *UnpackExpr:
		return f.evaluateUnpackExpr(e, ctx)
	case *ListRefExpr:
		return f.evaluateListRefExpr(e, ctx)
	case *FunctionCallExpr:
		return f.evaluateFunctionCall(e, ctx)
	}
	return nil, nil
}
