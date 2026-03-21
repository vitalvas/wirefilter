package wirefilter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSchemaFunctionControl(t *testing.T) {
	t.Run("default mode is blocklist - all functions allowed", func(t *testing.T) {
		schema := NewSchema().AddField("name", TypeString)

		_, err := Compile(`lower(name) == "test"`, schema)
		assert.NoError(t, err)

		_, err = Compile(`upper(name) == "TEST"`, schema)
		assert.NoError(t, err)

		_, err = Compile(`len(name) > 0`, schema)
		assert.NoError(t, err)
	})

	t.Run("blocklist mode - disable specific functions", func(t *testing.T) {
		schema := NewSchema().
			AddField("name", TypeString).
			DisableFunctions("lower")

		_, err := Compile(`lower(name) == "test"`, schema)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "function not allowed: lower")

		// Other functions still work
		_, err = Compile(`upper(name) == "TEST"`, schema)
		assert.NoError(t, err)
	})

	t.Run("blocklist mode - disable multiple functions", func(t *testing.T) {
		schema := NewSchema().
			AddField("name", TypeString).
			DisableFunctions("lower", "upper", "len")

		_, err := Compile(`lower(name) == "test"`, schema)
		assert.Error(t, err)

		_, err = Compile(`upper(name) == "TEST"`, schema)
		assert.Error(t, err)

		_, err = Compile(`len(name) > 0`, schema)
		assert.Error(t, err)

		// starts_with still works
		_, err = Compile(`starts_with(name, "test")`, schema)
		assert.NoError(t, err)
	})

	t.Run("allowlist mode - only enabled functions work", func(t *testing.T) {
		schema := NewSchema().
			AddField("name", TypeString).
			SetFunctionMode(FunctionModeAllowlist).
			EnableFunctions("lower")

		_, err := Compile(`lower(name) == "test"`, schema)
		assert.NoError(t, err)

		_, err = Compile(`upper(name) == "TEST"`, schema)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "function not allowed: upper")
	})

	t.Run("allowlist mode - enable multiple functions", func(t *testing.T) {
		schema := NewSchema().
			AddField("name", TypeString).
			SetFunctionMode(FunctionModeAllowlist).
			EnableFunctions("lower", "upper", "len")

		_, err := Compile(`lower(name) == "test"`, schema)
		assert.NoError(t, err)

		_, err = Compile(`upper(name) == "TEST"`, schema)
		assert.NoError(t, err)

		_, err = Compile(`len(name) > 0`, schema)
		assert.NoError(t, err)

		// starts_with is not enabled
		_, err = Compile(`starts_with(name, "test")`, schema)
		assert.Error(t, err)
	})

	t.Run("function names are case-insensitive", func(t *testing.T) {
		schema := NewSchema().
			AddField("name", TypeString).
			DisableFunctions("LOWER")

		_, err := Compile(`lower(name) == "test"`, schema)
		assert.Error(t, err)

		_, err = Compile(`LOWER(name) == "test"`, schema)
		assert.Error(t, err)

		_, err = Compile(`Lower(name) == "test"`, schema)
		assert.Error(t, err)
	})

	t.Run("enable after disable re-enables function", func(t *testing.T) {
		schema := NewSchema().
			AddField("name", TypeString).
			DisableFunctions("lower").
			EnableFunctions("lower")

		_, err := Compile(`lower(name) == "test"`, schema)
		assert.NoError(t, err)
	})

	t.Run("disable after enable disables function", func(t *testing.T) {
		schema := NewSchema().
			AddField("name", TypeString).
			SetFunctionMode(FunctionModeAllowlist).
			EnableFunctions("lower").
			DisableFunctions("lower")

		_, err := Compile(`lower(name) == "test"`, schema)
		assert.Error(t, err)
	})

	t.Run("allowlist mode with no functions enabled", func(t *testing.T) {
		schema := NewSchema().
			AddField("name", TypeString).
			SetFunctionMode(FunctionModeAllowlist)

		_, err := Compile(`lower(name) == "test"`, schema)
		assert.Error(t, err)

		// Non-function expressions still work
		_, err = Compile(`name == "test"`, schema)
		assert.NoError(t, err)
	})

	t.Run("IsFunctionAllowed - blocklist mode", func(t *testing.T) {
		schema := NewSchema().DisableFunctions("lower")

		assert.False(t, schema.IsFunctionAllowed("lower"))
		assert.False(t, schema.IsFunctionAllowed("LOWER"))
		assert.True(t, schema.IsFunctionAllowed("upper"))
	})

	t.Run("IsFunctionAllowed - allowlist mode", func(t *testing.T) {
		schema := NewSchema().
			SetFunctionMode(FunctionModeAllowlist).
			EnableFunctions("lower")

		assert.True(t, schema.IsFunctionAllowed("lower"))
		assert.True(t, schema.IsFunctionAllowed("LOWER"))
		assert.False(t, schema.IsFunctionAllowed("upper"))
	})

	t.Run("nested function calls respect rules", func(t *testing.T) {
		schema := NewSchema().
			AddField("name", TypeString).
			DisableFunctions("lower")

		// len(lower(name)) should fail because lower is disabled
		_, err := Compile(`len(lower(name)) > 0`, schema)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "function not allowed: lower")

		// len(upper(name)) should work
		_, err = Compile(`len(upper(name)) > 0`, schema)
		assert.NoError(t, err)
	})

	t.Run("nil schema allows all functions", func(t *testing.T) {
		_, err := Compile(`lower(name) == "test"`, nil)
		assert.NoError(t, err)
	})

	t.Run("blocklist with enabled function", func(t *testing.T) {
		schema := NewSchema().
			SetFunctionMode(FunctionModeBlocklist).
			DisableFunctions("lower").
			EnableFunctions("lower") // re-enable
		assert.True(t, schema.IsFunctionAllowed("lower"))
	})

	t.Run("validate unpack expression", func(t *testing.T) {
		schema := NewSchema().AddField("tags", TypeArray)
		_, err := Compile(`tags[*] == "a"`, schema)
		assert.NoError(t, err)
	})

	t.Run("validate unpack with unknown field", func(t *testing.T) {
		schema := NewSchema()
		_, err := Compile(`unknown[*] == "a"`, schema)
		assert.Error(t, err)
	})

	t.Run("validate index expression", func(t *testing.T) {
		schema := NewSchema().AddField("data", TypeMap)
		_, err := Compile(`data["key"] == "val"`, schema)
		assert.NoError(t, err)
	})

	t.Run("validate index with unknown field", func(t *testing.T) {
		schema := NewSchema()
		_, err := Compile(`unknown["key"] == "val"`, schema)
		assert.Error(t, err)
	})

	t.Run("validate index with unknown field as index key", func(t *testing.T) {
		schema := NewSchema().AddField("data", TypeMap)
		_, err := Compile(`data[unknown_field] == "val"`, schema)
		assert.Error(t, err)
	})

	t.Run("validate list ref expression", func(t *testing.T) {
		schema := NewSchema().AddField("ip", TypeIP)
		_, err := Compile(`ip in $blocked`, schema)
		assert.NoError(t, err)
	})

	t.Run("validate range expression", func(t *testing.T) {
		schema := NewSchema().AddField("x", TypeInt)
		_, err := Compile(`x in {1..10}`, schema)
		assert.NoError(t, err)
	})

	t.Run("validate function args with unknown field", func(t *testing.T) {
		schema := NewSchema()
		_, err := Compile(`lower(unknown) == "test"`, schema)
		assert.Error(t, err)
	})

	t.Run("schema with field map constructor", func(t *testing.T) {
		schema := NewSchema(map[string]Type{
			"name": TypeString,
			"age":  TypeInt,
		})
		_, ok := schema.GetField("name")
		assert.True(t, ok)
		_, ok = schema.GetField("age")
		assert.True(t, ok)
	})
}

func TestSchemaTypeValidation(t *testing.T) {
	schema := NewSchema().
		AddField("name", TypeString).
		AddField("status", TypeInt).
		AddField("active", TypeBool).
		AddField("ip", TypeIP).
		AddField("tags", TypeArray).
		AddField("data", TypeMap).
		AddField("body", TypeBytes)

	t.Run("string valid operators", func(t *testing.T) {
		valid := []string{
			`name == "test"`,
			`name != "test"`,
			`name contains "test"`,
			`name matches "^test"`,
			`name in {"a", "b"}`,
			`name wildcard "*.com"`,
			`name strict wildcard "*.COM"`,
		}
		for _, expr := range valid {
			_, err := Compile(expr, schema)
			assert.NoError(t, err, "should be valid: %s", expr)
		}
	})

	t.Run("string invalid operators", func(t *testing.T) {
		invalid := []string{
			`name > "test"`,
			`name < "test"`,
			`name >= "test"`,
			`name <= "test"`,
			`name === "test"`,
			`name !== "test"`,
		}
		for _, expr := range invalid {
			_, err := Compile(expr, schema)
			assert.Error(t, err, "should be invalid: %s", expr)
			assert.Contains(t, err.Error(), "not valid for field type")
		}
	})

	t.Run("int valid operators", func(t *testing.T) {
		valid := []string{
			`status == 200`,
			`status != 404`,
			`status > 400`,
			`status < 500`,
			`status >= 200`,
			`status <= 299`,
			`status in {200, 301, 404}`,
		}
		for _, expr := range valid {
			_, err := Compile(expr, schema)
			assert.NoError(t, err, "should be valid: %s", expr)
		}
	})

	t.Run("int invalid operators", func(t *testing.T) {
		invalid := []string{
			`status contains 200`,
			`status matches "200"`,
			`status wildcard "2*"`,
			`status === 200`,
		}
		for _, expr := range invalid {
			_, err := Compile(expr, schema)
			assert.Error(t, err, "should be invalid: %s", expr)
		}
	})

	t.Run("bool valid operators", func(t *testing.T) {
		valid := []string{
			`active == true`,
			`active != false`,
		}
		for _, expr := range valid {
			_, err := Compile(expr, schema)
			assert.NoError(t, err, "should be valid: %s", expr)
		}
	})

	t.Run("bool invalid operators", func(t *testing.T) {
		invalid := []string{
			`active > true`,
			`active contains true`,
			`active in {true}`,
		}
		for _, expr := range invalid {
			_, err := Compile(expr, schema)
			assert.Error(t, err, "should be invalid: %s", expr)
		}
	})

	t.Run("IP valid operators", func(t *testing.T) {
		valid := []string{
			`ip == 10.0.0.1`,
			`ip != 10.0.0.1`,
			`ip in "10.0.0.0/8"`,
			`ip in {10.0.0.1, 192.168.0.0/16}`,
		}
		for _, expr := range valid {
			_, err := Compile(expr, schema)
			assert.NoError(t, err, "should be valid: %s", expr)
		}
	})

	t.Run("IP invalid operators", func(t *testing.T) {
		invalid := []string{
			`ip > 10.0.0.1`,
			`ip contains "10"`,
			`ip matches "10\\..*"`,
			`ip wildcard "10.*"`,
		}
		for _, expr := range invalid {
			_, err := Compile(expr, schema)
			assert.Error(t, err, "should be invalid: %s", expr)
		}
	})

	t.Run("array valid operators", func(t *testing.T) {
		valid := []string{
			`tags == tags`,
			`tags contains "admin"`,
			`tags in {"a", "b"}`,
			`tags === "admin"`,
			`tags !== "admin"`,
		}
		for _, expr := range valid {
			_, err := Compile(expr, schema)
			assert.NoError(t, err, "should be valid: %s", expr)
		}
	})

	t.Run("array invalid operators", func(t *testing.T) {
		invalid := []string{
			`tags > "admin"`,
			`tags matches "admin"`,
			`tags wildcard "admin*"`,
		}
		for _, expr := range invalid {
			_, err := Compile(expr, schema)
			assert.Error(t, err, "should be invalid: %s", expr)
		}
	})

	t.Run("map valid operators", func(t *testing.T) {
		valid := []string{
			`data == data`,
			`data != data`,
		}
		for _, expr := range valid {
			_, err := Compile(expr, schema)
			assert.NoError(t, err, "should be valid: %s", expr)
		}
	})

	t.Run("map invalid operators", func(t *testing.T) {
		invalid := []string{
			`data > data`,
			`data contains "key"`,
		}
		for _, expr := range invalid {
			_, err := Compile(expr, schema)
			assert.Error(t, err, "should be invalid: %s", expr)
		}
	})

	t.Run("no schema skips type validation", func(t *testing.T) {
		_, err := Compile(`name > "test"`, nil)
		assert.NoError(t, err)
	})

	t.Run("logical operators always valid", func(t *testing.T) {
		valid := []string{
			`name == "test" and status > 200`,
			`name == "test" or active == true`,
			`active xor active`,
		}
		for _, expr := range valid {
			_, err := Compile(expr, schema)
			assert.NoError(t, err, "should be valid: %s", expr)
		}
	})

	t.Run("unpack skips type validation", func(t *testing.T) {
		// tags[*] unpacks array elements - operator applies to elements, not array
		valid := []string{
			`tags[*] matches "^admin"`,
			`tags[*] > 5`,
			`tags[*] wildcard "*.com"`,
		}
		for _, expr := range valid {
			_, err := Compile(expr, schema)
			assert.NoError(t, err, "should be valid (unpacked): %s", expr)
		}
	})

	t.Run("index skips element type validation", func(t *testing.T) {
		// data["key"] accesses map element - type of element unknown at schema level
		_, err := Compile(`data["key"] matches "^test"`, schema)
		assert.NoError(t, err)
	})
}

func TestSchemaComplexityLimits(t *testing.T) {
	t.Run("max depth - within limit", func(t *testing.T) {
		schema := NewSchema().
			AddField("a", TypeBool).
			SetMaxDepth(10)

		_, err := Compile(`a and a and a`, schema)
		assert.NoError(t, err)
	})

	t.Run("max depth - exceeds limit", func(t *testing.T) {
		schema := NewSchema().
			AddField("a", TypeBool).
			SetMaxDepth(3)

		// "a and a and a and a" creates nested binary exprs > depth 3
		_, err := Compile(`a and (a and (a and a))`, schema)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum depth")
	})

	t.Run("max depth - exact limit", func(t *testing.T) {
		schema := NewSchema().
			AddField("a", TypeInt).
			SetMaxDepth(5)

		_, err := Compile(`a == 1`, schema)
		assert.NoError(t, err)
	})

	t.Run("max nodes - within limit", func(t *testing.T) {
		schema := NewSchema().
			AddField("a", TypeBool).
			SetMaxNodes(20)

		_, err := Compile(`a and a`, schema)
		assert.NoError(t, err)
	})

	t.Run("max nodes - exceeds limit", func(t *testing.T) {
		schema := NewSchema().
			AddField("x", TypeInt).
			SetMaxNodes(5)

		_, err := Compile(`x == 1 and x == 2 and x == 3`, schema)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum node count")
	})

	t.Run("zero limits means unlimited", func(t *testing.T) {
		schema := NewSchema().
			AddField("x", TypeInt).
			SetMaxDepth(0).
			SetMaxNodes(0)

		_, err := Compile(`x == 1 and x == 2 and x == 3 and x == 4`, schema)
		assert.NoError(t, err)
	})

	t.Run("depth with nested functions", func(t *testing.T) {
		schema := NewSchema().
			AddField("name", TypeString).
			SetMaxDepth(3)

		// lower(name) == "test" has depth: BinaryExpr > FunctionCallExpr > FieldExpr = 3
		_, err := Compile(`lower(name) == "test"`, schema)
		assert.NoError(t, err)
	})

	t.Run("depth with deeply nested functions", func(t *testing.T) {
		schema := NewSchema().
			AddField("name", TypeString).
			SetMaxDepth(3)

		// nested: and > BinaryExpr > FunctionCallExpr > FieldExpr = 4
		_, err := Compile(`lower(name) == "test" and name == "x"`, schema)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum depth")
	})

	t.Run("nodes with array", func(t *testing.T) {
		schema := NewSchema().
			AddField("x", TypeInt).
			SetMaxNodes(10)

		// x in {1, 2, 3, 4, 5, 6, 7, 8} = BinaryExpr + FieldExpr + ArrayExpr + 8 literals = 11
		_, err := Compile(`x in {1, 2, 3, 4, 5, 6, 7, 8}`, schema)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum node count")
	})

	t.Run("combined depth and nodes", func(t *testing.T) {
		schema := NewSchema().
			AddField("a", TypeBool).
			SetMaxDepth(100).
			SetMaxNodes(5)

		_, err := Compile(`a and a and a and a`, schema)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum node count")
	})
}

func TestSchemaExport(t *testing.T) {
	t.Run("fields", func(t *testing.T) {
		schema := NewSchema().
			AddField("http.host", TypeString).
			AddField("http.status", TypeInt).
			AddField("ip.src", TypeIP).
			AddField("active", TypeBool)

		exported := schema.Export()
		assert.Equal(t, TypeString, exported["http.host"])
		assert.Equal(t, TypeInt, exported["http.status"])
		assert.Equal(t, TypeIP, exported["ip.src"])
		assert.Equal(t, TypeBool, exported["active"])
		assert.Len(t, exported, 4)
	})

	t.Run("empty schema", func(t *testing.T) {
		schema := NewSchema()
		exported := schema.Export()
		assert.Empty(t, exported)
	})
}

func TestSchemaValidationEdgeCases(t *testing.T) {
	t.Run("range start validation error", func(t *testing.T) {
		schema := NewSchema().AddField("x", TypeInt)
		// Hand-build AST with unknown field inside RangeExpr.Start
		expr := &BinaryExpr{
			Left:     &FieldExpr{Name: "x"},
			Operator: TokenIn,
			Right: &ArrayExpr{Elements: []Expression{
				&RangeExpr{
					Start: &FieldExpr{Name: "unknown"},
					End:   &LiteralExpr{Value: IntValue(10)},
				},
			}},
		}
		err := schema.Validate(expr)
		assert.Error(t, err)
	})

	t.Run("range end validation error", func(t *testing.T) {
		schema := NewSchema().AddField("x", TypeInt)
		expr := &BinaryExpr{
			Left:     &FieldExpr{Name: "x"},
			Operator: TokenIn,
			Right: &ArrayExpr{Elements: []Expression{
				&RangeExpr{
					Start: &LiteralExpr{Value: IntValue(1)},
					End:   &FieldExpr{Name: "unknown"},
				},
			}},
		}
		err := schema.Validate(expr)
		assert.Error(t, err)
	})

	t.Run("validate func args builtin with custom funcs registered", func(t *testing.T) {
		schema := NewSchema().
			AddField("name", TypeString).
			RegisterFunction("custom_fn", TypeBool, nil)
		// lower() is a built-in, not in customFuncs - hits the !ok return
		_, err := Compile(`lower(name) == "test"`, schema)
		assert.NoError(t, err)
	})

	t.Run("validate func args no custom funcs registered", func(t *testing.T) {
		schema := NewSchema().AddField("name", TypeString)
		_, err := Compile(`lower(name) == "test"`, schema)
		assert.NoError(t, err)
	})
}

func TestTypedArrayField(t *testing.T) {
	t.Run("valid unpack operations on typed array", func(t *testing.T) {
		schema := NewSchema().
			AddArrayField("tags", TypeString).
			AddArrayField("ports", TypeInt)

		valid := []string{
			`tags[*] == "admin"`,
			`tags[*] != "blocked"`,
			`tags[*] contains "prod"`,
			`tags[*] matches "^admin"`,
			`tags[*] in {"a", "b"}`,
			`tags[*] wildcard "*.com"`,
			`ports[*] >= 1024`,
			`ports[*] < 65535`,
			`ports[*] == 443`,
			`ports[*] in {80, 443}`,
		}
		for _, expr := range valid {
			_, err := Compile(expr, schema)
			assert.NoError(t, err, "should be valid: %s", expr)
		}
	})

	t.Run("invalid unpack operations on typed array", func(t *testing.T) {
		schema := NewSchema().
			AddArrayField("tags", TypeString).
			AddArrayField("ports", TypeInt)

		invalid := []string{
			`tags[*] > 10`,
			`tags[*] >= 5`,
			`ports[*] contains "x"`,
			`ports[*] matches "^[0-9]+"`,
			`ports[*] wildcard "80*"`,
		}
		for _, expr := range invalid {
			_, err := Compile(expr, schema)
			assert.Error(t, err, "should be invalid: %s", expr)
			assert.Contains(t, err.Error(), "not valid for field type")
		}
	})

	t.Run("untyped array still skips element validation", func(t *testing.T) {
		schema := NewSchema().AddField("tags", TypeArray)
		_, err := Compile(`tags[*] > 10`, schema)
		assert.NoError(t, err)
	})

	t.Run("array field type is TypeArray", func(t *testing.T) {
		schema := NewSchema().AddArrayField("tags", TypeString)
		field, ok := schema.GetField("tags")
		assert.True(t, ok)
		assert.Equal(t, TypeArray, field.Type)
		assert.Equal(t, TypeString, field.ElemType)
	})
}

func TestTypedMapField(t *testing.T) {
	t.Run("valid index operations on typed map", func(t *testing.T) {
		schema := NewSchema().
			AddMapField("headers", TypeString).
			AddMapField("scores", TypeFloat)

		valid := []string{
			`headers["x-env"] == "prod"`,
			`headers["host"] contains "example"`,
			`headers["ua"] matches "^Mozilla"`,
			`scores["risk"] > 0.8`,
			`scores["risk"] >= 0.5`,
			`scores["risk"] == 1.0`,
		}
		for _, expr := range valid {
			_, err := Compile(expr, schema)
			assert.NoError(t, err, "should be valid: %s", expr)
		}
	})

	t.Run("invalid index operations on typed map", func(t *testing.T) {
		schema := NewSchema().
			AddMapField("headers", TypeString).
			AddMapField("scores", TypeFloat)

		invalid := []string{
			`headers["x-env"] > 10`,
			`headers["host"] >= 5`,
			`scores["risk"] contains "x"`,
			`scores["risk"] matches "^[0-9]+"`,
		}
		for _, expr := range invalid {
			_, err := Compile(expr, schema)
			assert.Error(t, err, "should be invalid: %s", expr)
			assert.Contains(t, err.Error(), "not valid for field type")
		}
	})

	t.Run("untyped map still skips value validation", func(t *testing.T) {
		schema := NewSchema().AddField("data", TypeMap)
		_, err := Compile(`data["key"] > 10`, schema)
		assert.NoError(t, err)
	})

	t.Run("map field type is TypeMap", func(t *testing.T) {
		schema := NewSchema().AddMapField("headers", TypeString)
		field, ok := schema.GetField("headers")
		assert.True(t, ok)
		assert.Equal(t, TypeMap, field.Type)
		assert.Equal(t, TypeString, field.ElemType)
	})
}

func TestTypedFieldsCombined(t *testing.T) {
	t.Run("mixed typed and untyped fields", func(t *testing.T) {
		schema := NewSchema().
			AddField("name", TypeString).
			AddArrayField("tags", TypeString).
			AddArrayField("ports", TypeInt).
			AddMapField("headers", TypeString).
			AddMapField("scores", TypeFloat).
			AddField("data", TypeMap)

		valid := []string{
			`name == "test" and tags[*] contains "prod"`,
			`ports[*] >= 1024 and headers["host"] == "example.com"`,
			`scores["risk"] > 0.5 or name contains "admin"`,
			`data["key"] > 10`,
		}
		for _, expr := range valid {
			_, err := Compile(expr, schema)
			assert.NoError(t, err, "should be valid: %s", expr)
		}

		invalid := []string{
			`tags[*] > 10 and name == "test"`,
			`scores["risk"] contains "x"`,
		}
		for _, expr := range invalid {
			_, err := Compile(expr, schema)
			assert.Error(t, err, "should be invalid: %s", expr)
		}
	})

	t.Run("export includes typed fields", func(t *testing.T) {
		schema := NewSchema().
			AddArrayField("tags", TypeString).
			AddMapField("headers", TypeString)

		exported := schema.Export()
		assert.Equal(t, TypeArray, exported["tags"])
		assert.Equal(t, TypeMap, exported["headers"])
	})
}

func TestFunctionReturnTypeInference(t *testing.T) {
	schema := NewSchema().
		AddMapField("user", TypeString).
		RegisterFunction("get_score", TypeFloat, []Type{TypeString}).
		RegisterFunction("get_name", TypeString, []Type{TypeString})

	t.Run("valid operations on function return type", func(t *testing.T) {
		valid := []string{
			`get_score(user["email"]) > 0.5`,
			`get_score(user["email"]) >= 0.0`,
			`get_score(user["email"]) == 1.0`,
			`get_score(user["email"]) in {0.5, 1.0}`,
			`get_name(user["email"]) == "admin"`,
			`get_name(user["email"]) contains "admin"`,
			`get_name(user["email"]) matches "^admin"`,
		}
		for _, expr := range valid {
			_, err := Compile(expr, schema)
			assert.NoError(t, err, "should be valid: %s", expr)
		}
	})

	t.Run("invalid operations on function return type", func(t *testing.T) {
		invalid := []string{
			`get_score(user["email"]) contains "x"`,
			`get_score(user["email"]) matches "^[0-9]"`,
			`get_name(user["email"]) > 5`,
			`get_name(user["email"]) >= 0`,
		}
		for _, expr := range invalid {
			_, err := Compile(expr, schema)
			assert.Error(t, err, "should be invalid: %s", expr)
			assert.Contains(t, err.Error(), "not valid for field type")
		}
	})

	t.Run("unregistered function skips validation", func(t *testing.T) {
		_, err := Compile(`unknown_func() > 5`, schema)
		assert.NoError(t, err)
	})

	t.Run("full chain inference", func(t *testing.T) {
		_, err := Compile(`get_score(user["email"]) > 0.5 and get_name(user["email"]) contains "admin"`, schema)
		assert.NoError(t, err)
	})
}

func TestSchemaTimeAndDuration(t *testing.T) {
	schema := NewSchema().
		AddField("created_at", TypeTime).
		AddField("ttl", TypeDuration).
		AddField("name", TypeString)

	t.Run("valid time operators", func(t *testing.T) {
		tests := []string{
			`created_at == 2026-03-19T10:00:00Z`,
			`created_at != 2026-03-19T10:00:00Z`,
			`created_at < 2026-03-19T10:00:00Z`,
			`created_at > 2026-03-19T10:00:00Z`,
			`created_at <= 2026-03-19T10:00:00Z`,
			`created_at >= 2026-03-19T10:00:00Z`,
			`created_at + 1h >= 2026-03-19T10:00:00Z`,
			`created_at - 1h <= 2026-03-19T10:00:00Z`,
		}
		for _, expr := range tests {
			_, err := Compile(expr, schema)
			assert.NoError(t, err, "expression: %s", expr)
		}
	})

	t.Run("invalid time operators", func(t *testing.T) {
		tests := []string{
			`created_at contains "test"`,
			`created_at matches "pattern"`,
			`created_at * 2`,
		}
		for _, expr := range tests {
			_, err := Compile(expr, schema)
			assert.Error(t, err, "expression: %s", expr)
		}
	})

	t.Run("valid duration operators", func(t *testing.T) {
		tests := []string{
			`ttl == 30m`,
			`ttl != 30m`,
			`ttl < 1h`,
			`ttl > 5m`,
			`ttl <= 1h`,
			`ttl >= 5m`,
			`ttl + 30m > 1h`,
			`ttl - 5m < 1h`,
			`ttl * 2 > 1h`,
			`ttl / 2 < 1h`,
			`ttl % 1h == 30m`,
		}
		for _, expr := range tests {
			_, err := Compile(expr, schema)
			assert.NoError(t, err, "expression: %s", expr)
		}
	})

	t.Run("invalid duration operators", func(t *testing.T) {
		tests := []string{
			`ttl contains "test"`,
			`ttl matches "pattern"`,
		}
		for _, expr := range tests {
			_, err := Compile(expr, schema)
			assert.Error(t, err, "expression: %s", expr)
		}
	})

	t.Run("now resolves to TypeTime", func(t *testing.T) {
		_, err := Compile(`created_at >= now() - 1h`, schema)
		assert.NoError(t, err)
	})

	t.Run("time in set", func(t *testing.T) {
		_, err := Compile(`created_at in {2026-03-19T10:00:00Z, 2026-03-20T10:00:00Z}`, schema)
		assert.NoError(t, err)
	})

	t.Run("duration in set", func(t *testing.T) {
		_, err := Compile(`ttl in {30m, 1h, 2h}`, schema)
		assert.NoError(t, err)
	})
}

func TestDisableRegex(t *testing.T) {
	schema := NewSchema().
		AddField("name", TypeString).
		AddField("path", TypeString).
		DisableRegex()

	t.Run("matches operator blocked", func(t *testing.T) {
		_, err := Compile(`name matches "^test"`, schema)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "regex is disabled")
	})

	t.Run("tilde alias blocked", func(t *testing.T) {
		_, err := Compile(`name ~ "^test"`, schema)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "regex is disabled")
	})

	t.Run("regex_replace blocked", func(t *testing.T) {
		_, err := Compile(`regex_replace(name, "[0-9]+", "X") == "X"`, schema)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "regex is disabled")
	})

	t.Run("regex_extract blocked", func(t *testing.T) {
		_, err := Compile(`regex_extract(path, "/v[0-9]+/") == ""`, schema)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "regex is disabled")
	})

	t.Run("contains_word blocked", func(t *testing.T) {
		_, err := Compile(`contains_word(name, "admin")`, schema)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "regex is disabled")
	})

	t.Run("wildcard still allowed", func(t *testing.T) {
		_, err := Compile(`name wildcard "*.example.com"`, schema)
		assert.NoError(t, err)
	})

	t.Run("strict wildcard still allowed", func(t *testing.T) {
		_, err := Compile(`name strict wildcard "*.Example.com"`, schema)
		assert.NoError(t, err)
	})

	t.Run("contains still allowed", func(t *testing.T) {
		_, err := Compile(`name contains "test"`, schema)
		assert.NoError(t, err)
	})

	t.Run("equality still allowed", func(t *testing.T) {
		_, err := Compile(`name == "test"`, schema)
		assert.NoError(t, err)
	})

	t.Run("other functions still allowed", func(t *testing.T) {
		_, err := Compile(`lower(name) == "test"`, schema)
		assert.NoError(t, err)
	})

	t.Run("regex allowed without flag", func(t *testing.T) {
		noFlag := NewSchema().AddField("name", TypeString)
		_, err := Compile(`name matches "^test"`, noFlag)
		assert.NoError(t, err)
		_, err = Compile(`regex_replace(name, "a", "b") == "x"`, noFlag)
		assert.NoError(t, err)
	})
}
