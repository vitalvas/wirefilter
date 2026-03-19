package wirefilter

import (
	"fmt"
	"strings"
)

// FunctionMode defines how function availability is controlled.
type FunctionMode int

const (
	// FunctionModeBlocklist allows all functions except those explicitly disabled.
	// This is the default mode.
	FunctionModeBlocklist FunctionMode = iota
	// FunctionModeAllowlist allows only functions that are explicitly enabled.
	FunctionModeAllowlist
)

// Default complexity limits. Zero means unlimited.
const (
	DefaultMaxDepth = 0
	DefaultMaxNodes = 0
)

// Field represents a named field with a specific type in a schema.
type Field struct {
	Name      string
	Type      Type
	ElemType  Type // For TypeArray: element type. For TypeMap: value type.
	ElemTyped bool // True if ElemType was explicitly set via AddArrayField/AddMapField.
}

// FuncSignature defines the compile-time signature of a user-defined function.
type FuncSignature struct {
	ArgTypes   []Type // expected argument types (nil means any count/type)
	ReturnType Type   // return type for schema validation
}

// Schema defines the structure of fields that can be used in filter expressions.
// It provides validation to ensure that filter expressions only reference defined fields,
// operators are valid for field types, and expression complexity is within limits.
type Schema struct {
	fields        map[string]Field
	functionMode  FunctionMode
	functionRules map[string]bool          // true = enabled, false = disabled
	customFuncs   map[string]FuncSignature // registered user-defined functions
	maxDepth      int                      // max AST nesting depth (0 = unlimited)
	maxNodes      int                      // max AST node count (0 = unlimited)
}

// operatorsByType defines which operators are valid for each field type.
var operatorsByType = map[Type]map[TokenType]bool{
	TypeString: {
		TokenEq: true, TokenNe: true,
		TokenContains: true, TokenMatches: true,
		TokenIn: true, TokenWildcard: true, TokenStrictWildcard: true,
	},
	TypeInt: {
		TokenEq: true, TokenNe: true,
		TokenLt: true, TokenGt: true, TokenLe: true, TokenGe: true,
		TokenIn:   true,
		TokenPlus: true, TokenMinus: true, TokenAsterisk: true, TokenDiv: true, TokenMod: true,
	},
	TypeFloat: {
		TokenEq: true, TokenNe: true,
		TokenLt: true, TokenGt: true, TokenLe: true, TokenGe: true,
		TokenIn:   true,
		TokenPlus: true, TokenMinus: true, TokenAsterisk: true, TokenDiv: true, TokenMod: true,
	},
	TypeBool: {
		TokenEq: true, TokenNe: true,
	},
	TypeIP: {
		TokenEq: true, TokenNe: true,
		TokenIn: true,
	},
	TypeCIDR: {
		TokenEq: true, TokenNe: true,
	},
	TypeBytes: {
		TokenEq: true, TokenNe: true,
		TokenContains: true,
	},
	TypeArray: {
		TokenEq: true, TokenNe: true,
		TokenAllEq: true, TokenAnyNe: true,
		TokenContains: true, TokenIn: true,
	},
	TypeMap: {
		TokenEq: true, TokenNe: true,
	},
	TypeTime: {
		TokenEq: true, TokenNe: true,
		TokenLt: true, TokenGt: true, TokenLe: true, TokenGe: true,
		TokenIn:   true,
		TokenPlus: true, TokenMinus: true,
	},
	TypeDuration: {
		TokenEq: true, TokenNe: true,
		TokenLt: true, TokenGt: true, TokenLe: true, TokenGe: true,
		TokenIn:   true,
		TokenPlus: true, TokenMinus: true,
		TokenAsterisk: true, TokenDiv: true, TokenMod: true,
	},
}

// NewSchema creates a new schema.
// If fields are provided, initializes the schema with those fields.
// Multiple field maps can be provided and will be merged.
// Otherwise, creates an empty schema.
// Default function mode is Blocklist (all functions allowed).
func NewSchema(fields ...map[string]Type) *Schema {
	s := &Schema{
		fields:        make(map[string]Field),
		functionMode:  FunctionModeBlocklist,
		functionRules: make(map[string]bool),
	}
	for _, fieldMap := range fields {
		for name, fieldType := range fieldMap {
			s.fields[name] = Field{
				Name: name,
				Type: fieldType,
			}
		}
	}
	return s
}

// SetFunctionMode sets the function availability mode.
// In Blocklist mode (default), all functions are allowed except those disabled.
// In Allowlist mode, only explicitly enabled functions are allowed.
// Returns the schema to allow method chaining.
func (s *Schema) SetFunctionMode(mode FunctionMode) *Schema {
	s.functionMode = mode
	return s
}

// EnableFunctions enables one or more functions by name.
// In Allowlist mode, this allows the functions to be used.
// In Blocklist mode, this removes the functions from the disabled list.
// Function names are case-insensitive.
// Returns the schema to allow method chaining.
func (s *Schema) EnableFunctions(names ...string) *Schema {
	for _, name := range names {
		s.functionRules[strings.ToLower(name)] = true
	}
	return s
}

// DisableFunctions disables one or more functions by name.
// In Blocklist mode, this prevents the functions from being used.
// In Allowlist mode, this removes the functions from the enabled list.
// Function names are case-insensitive.
// Returns the schema to allow method chaining.
func (s *Schema) DisableFunctions(names ...string) *Schema {
	for _, name := range names {
		s.functionRules[strings.ToLower(name)] = false
	}
	return s
}

// IsFunctionAllowed checks if a function is allowed based on the current mode and rules.
// Function names are case-insensitive.
// builtinFunctions is the set of built-in functions that are always allowed
// regardless of function mode settings.
var builtinFunctions = map[string]bool{
	"now": true,
}

func (s *Schema) IsFunctionAllowed(name string) bool {
	name = strings.ToLower(name)

	// Built-in special functions are always allowed
	if builtinFunctions[name] {
		return true
	}

	// Custom registered functions are always allowed
	if s.customFuncs != nil {
		if _, ok := s.customFuncs[name]; ok {
			return true
		}
	}

	enabled, hasRule := s.functionRules[name]

	switch s.functionMode {
	case FunctionModeAllowlist:
		// In allowlist mode, function must be explicitly enabled
		return hasRule && enabled
	case FunctionModeBlocklist:
		// In blocklist mode, function is allowed unless explicitly disabled
		if hasRule {
			return enabled
		}
		return true
	}
	return true
}

// RegisterFunction registers a user-defined function with its argument and return types.
// This enables compile-time validation of argument count and types.
// The actual function handler is bound at runtime via ExecutionContext.SetFunc.
// If argTypes is nil, argument validation is skipped.
// Returns the schema to allow method chaining.
func (s *Schema) RegisterFunction(name string, returnType Type, argTypes []Type) *Schema {
	if s.customFuncs == nil {
		s.customFuncs = make(map[string]FuncSignature)
	}
	s.customFuncs[strings.ToLower(name)] = FuncSignature{
		ArgTypes:   argTypes,
		ReturnType: returnType,
	}
	return s
}

// SetMaxDepth sets the maximum allowed AST nesting depth.
// Zero means unlimited (default). This prevents deeply nested expressions
// that could cause stack overflows or excessive resource consumption.
// Returns the schema to allow method chaining.
func (s *Schema) SetMaxDepth(depth int) *Schema {
	s.maxDepth = depth
	return s
}

// SetMaxNodes sets the maximum allowed number of AST nodes.
// Zero means unlimited (default). This prevents overly complex expressions
// that could cause excessive evaluation time.
// Returns the schema to allow method chaining.
func (s *Schema) SetMaxNodes(nodes int) *Schema {
	s.maxNodes = nodes
	return s
}

// AddField adds a field to the schema with the specified name and type.
// Returns the schema to allow method chaining.
func (s *Schema) AddField(name string, fieldType Type) *Schema {
	s.fields[name] = Field{
		Name: name,
		Type: fieldType,
	}
	return s
}

// AddArrayField adds a typed array field to the schema.
// The elemType specifies the type of elements in the array,
// enabling compile-time validation of operations on unpacked elements (e.g., tags[*] > 10).
// Returns the schema to allow method chaining.
func (s *Schema) AddArrayField(name string, elemType Type) *Schema {
	s.fields[name] = Field{
		Name:      name,
		Type:      TypeArray,
		ElemType:  elemType,
		ElemTyped: true,
	}
	return s
}

// AddMapField adds a typed map field to the schema.
// The valueType specifies the type of values in the map,
// enabling compile-time validation of operations on indexed values (e.g., scores["risk"] > 0.8).
// Map keys are always strings in the wirefilter DSL.
// Returns the schema to allow method chaining.
func (s *Schema) AddMapField(name string, valueType Type) *Schema {
	s.fields[name] = Field{
		Name:      name,
		Type:      TypeMap,
		ElemType:  valueType,
		ElemTyped: true,
	}
	return s
}

// GetField retrieves a field from the schema by name.
// Returns the field and true if found, or an empty field and false if not found.
func (s *Schema) GetField(name string) (Field, bool) {
	field, ok := s.fields[name]
	return field, ok
}

// Validate checks that all field references in the expression exist in the schema,
// operators are valid for field types, and expression complexity is within limits.
// Returns an error if validation fails.
func (s *Schema) Validate(expr Expression) error {
	v := &validator{schema: s}
	return v.validate(expr, 0)
}

// validator tracks state during expression validation.
type validator struct {
	schema *Schema
	nodes  int
}

func (v *validator) validate(expr Expression, depth int) error {
	v.nodes++
	depth++

	if v.schema.maxDepth > 0 && depth > v.schema.maxDepth {
		return fmt.Errorf("expression exceeds maximum depth of %d", v.schema.maxDepth)
	}
	if v.schema.maxNodes > 0 && v.nodes > v.schema.maxNodes {
		return fmt.Errorf("expression exceeds maximum node count of %d", v.schema.maxNodes)
	}

	switch e := expr.(type) {
	case *BinaryExpr:
		if err := v.validate(e.Left, depth); err != nil {
			return err
		}
		if err := v.validate(e.Right, depth); err != nil {
			return err
		}
		return v.validateOperatorType(e)

	case *UnaryExpr:
		return v.validate(e.Operand, depth)

	case *FieldExpr:
		if _, ok := v.schema.GetField(e.Name); !ok {
			return fmt.Errorf("unknown field: %s", e.Name)
		}

	case *ArrayExpr:
		for _, elem := range e.Elements {
			if err := v.validate(elem, depth); err != nil {
				return err
			}
		}

	case *RangeExpr:
		if err := v.validate(e.Start, depth); err != nil {
			return err
		}
		return v.validate(e.End, depth)

	case *IndexExpr:
		if err := v.validate(e.Object, depth); err != nil {
			return err
		}
		return v.validate(e.Index, depth)

	case *UnpackExpr:
		return v.validate(e.Array, depth)

	case *ListRefExpr:
		// List references are validated at runtime

	case *FunctionCallExpr:
		if !v.schema.IsFunctionAllowed(e.Name) {
			return fmt.Errorf("function not allowed: %s", e.Name)
		}
		for _, arg := range e.Arguments {
			if err := v.validate(arg, depth); err != nil {
				return err
			}
		}
		if err := v.validateFuncArgs(e); err != nil {
			return err
		}
	}

	return nil
}

// validateFuncArgs checks argument count and types for registered custom functions.
func (v *validator) validateFuncArgs(expr *FunctionCallExpr) error {
	if v.schema.customFuncs == nil {
		return nil
	}
	sig, ok := v.schema.customFuncs[strings.ToLower(expr.Name)]
	if !ok {
		return nil // built-in function, skip custom validation
	}
	if sig.ArgTypes == nil {
		return nil // no type constraints
	}
	if len(expr.Arguments) != len(sig.ArgTypes) {
		return fmt.Errorf("function %s expects %d arguments, got %d", expr.Name, len(sig.ArgTypes), len(expr.Arguments))
	}
	for i, argExpr := range expr.Arguments {
		if argType, ok := v.resolveFieldType(argExpr); ok {
			if argType != sig.ArgTypes[i] {
				return fmt.Errorf("function %s argument %d: expected %s, got %s", expr.Name, i+1, sig.ArgTypes[i], argType)
			}
		}
	}
	return nil
}

// validateOperatorType checks that the operator in a binary expression is valid
// for the field type on the left side. This is only checked when the left side
// is a FieldExpr or UnpackExpr with a known field type.
func (v *validator) validateOperatorType(expr *BinaryExpr) error {
	// Skip logical operators - they work on any type
	switch expr.Operator {
	case TokenAnd, TokenOr, TokenXor:
		return nil
	}

	fieldType, ok := v.resolveFieldType(expr.Left)
	if !ok {
		return nil // Can't determine type, skip validation
	}

	validOps, exists := operatorsByType[fieldType]
	if !exists {
		return nil // Unknown type, skip validation
	}

	if !validOps[expr.Operator] {
		return fmt.Errorf("operator %s is not valid for field type %s", expr.Operator, fieldType)
	}

	return nil
}

// Export returns a flat map of field names to their types for use in audit logs.
func (s *Schema) Export() map[string]Type {
	result := make(map[string]Type, len(s.fields))
	for name, field := range s.fields {
		result[name] = field.Type
	}
	return result
}

// resolveFieldType returns the type of an expression for operator validation.
// For direct field references, returns the field type.
// For unpack expressions (field[*]), returns the array element type if known.
// For index expressions (field["key"]), returns the map value type if known.
// For function calls, returns the registered return type if known.
func (v *validator) resolveFieldType(expr Expression) (Type, bool) {
	switch e := expr.(type) {
	case *FieldExpr:
		if field, ok := v.schema.GetField(e.Name); ok {
			return field.Type, true
		}
	case *UnpackExpr:
		if fieldExpr, ok := e.Array.(*FieldExpr); ok {
			if field, ok := v.schema.GetField(fieldExpr.Name); ok {
				if field.Type == TypeArray && field.ElemTyped {
					return field.ElemType, true
				}
			}
		}
	case *IndexExpr:
		if fieldExpr, ok := e.Object.(*FieldExpr); ok {
			if field, ok := v.schema.GetField(fieldExpr.Name); ok {
				if field.Type == TypeMap && field.ElemTyped {
					return field.ElemType, true
				}
			}
		}
	case *FunctionCallExpr:
		if strings.ToLower(e.Name) == "now" {
			return TypeTime, true
		}
		if v.schema.customFuncs != nil {
			if sig, ok := v.schema.customFuncs[strings.ToLower(e.Name)]; ok {
				return sig.ReturnType, true
			}
		}
	}
	return 0, false
}
