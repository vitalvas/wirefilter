package wirefilter

import (
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalUnmarshal(t *testing.T) {
	expressions := []struct {
		name string
		expr string
	}{
		{"simple equality", `name == "test"`},
		{"integer comparison", `status >= 400`},
		{"logical and", `a == 1 and b == 2`},
		{"logical or", `a == 1 or b == 2`},
		{"logical xor", `a xor b`},
		{"not", `not active`},
		{"not in", `ip not in $nets`},
		{"not contains", `name not contains "admin"`},
		{"in array", `status in {200, 201, 204}`},
		{"in range", `port in {80..100}`},
		{"in CIDR string", `ip in "10.0.0.0/8"`},
		{"in CIDR native", `ip in 192.168.0.0/24`},
		{"contains", `path contains "/api"`},
		{"matches", `ua matches "^Mozilla"`},
		{"wildcard", `host wildcard "*.example.com"`},
		{"strict wildcard", `host strict wildcard "*.Example.com"`},
		{"all equal", `tags === "prod"`},
		{"any not equal", `tags !== "dev"`},
		{"field index string", `data["key"] == "val"`},
		{"field index int", `items[0] == "first"`},
		{"array unpack", `tags[*] == "prod"`},
		{"list ref", `ip in $blocked_ips`},
		{"function lower", `lower(name) == "test"`},
		{"function upper", `upper(name) == "TEST"`},
		{"function len", `len(name) > 5`},
		{"function starts_with", `starts_with(name, "pre")`},
		{"function ends_with", `ends_with(name, ".com")`},
		{"function concat", `concat("a", "b") == "ab"`},
		{"function substring", `substring(name, 0, 3) == "tes"`},
		{"function split", `split(name, ",")[0] == "a"`},
		{"function join", `join(tags, ",") == "a,b"`},
		{"function has_key", `has_key(data, "key")`},
		{"function has_value", `has_value(tags, "a")`},
		{"function url_decode", `url_decode(query) contains "test"`},
		{"function cidr", `cidr(ip, 24) == "10.0.0.0"`},
		{"function cidr6", `cidr6(ip, 64) == "2001:db8::"`},
		{"function any", `any(tags[*] == "prod")`},
		{"function all", `all(tags[*] contains "a")`},
		{"complex nested", `(lower(name) == "admin" or status >= 500) and ip not in $blocked`},
		{"bool literal true", `active == true`},
		{"bool literal false", `active == false`},
		{"negative int", `count > -1`},
		{"float literal", `score > 3.14`},
		{"float equality", `score == 99.5`},
		{"negative float", `temp > -10.5`},
		{"float in set", `score in {1.5, 2.5, 3.5}`},
		{"IP literal", `ip == 192.168.1.1`},
		{"empty array", `x in {}`},
		{"mixed array", `port in {80, 443, 8000..9000}`},
		{"table lookup scalar", `$geo[ip] == "US"`},
		{"table lookup with in", `name in $allowed[dept]`},
		{"table lookup literal key", `$config["mode"] == "prod"`},
		{"udf no args", `maintenance() == true`},
		{"udf with arg", `get_score(name) > 5.0`},
		{"udf with ip arg", `is_tor(ip) == true`},
		{"udf in operator", `ip in get_cidrs(name)`},
		{"udf combined", `is_tor(ip) and get_score(name) > 3.0`},
	}

	for _, tt := range expressions {
		t.Run(tt.name, func(t *testing.T) {
			original, err := Compile(tt.expr, nil)
			require.NoError(t, err)

			data, err := original.MarshalBinary()
			require.NoError(t, err)
			assert.True(t, len(data) > 3, "encoded data should have header + body")

			restored := &Filter{}
			err = restored.UnmarshalBinary(data)
			require.NoError(t, err)

			// Verify both filters produce the same results
			ctx := NewExecutionContext().
				SetStringField("name", "test").
				SetStringField("host", "api.example.com").
				SetStringField("path", "/api/v1/users").
				SetStringField("ua", "Mozilla/5.0").
				SetStringField("query", "search%20term").
				SetIntField("status", 500).
				SetIntField("port", 443).
				SetIntField("count", 10).
				SetIntField("x", 201).
				SetFloatField("score", 99.5).
				SetFloatField("temp", -5.0).
				SetBoolField("active", true).
				SetBoolField("a", true).
				SetBoolField("b", false).
				SetIPField("ip", "192.168.1.1").
				SetArrayField("tags", []string{"prod", "v2"}).
				SetArrayField("items", []string{"first", "second"}).
				SetMapField("data", map[string]string{"key": "val"}).
				SetList("names", []string{"admin", "user"}).
				SetIPList("blocked_ips", []string{"10.0.0.1", "192.168.0.0/16"}).
				SetIPList("blocked", []string{"10.0.0.0/8"}).
				SetIPList("nets", []string{"10.0.0.0/8", "172.16.0.0/12"}).
				SetTable("geo", map[string]string{"192.168.1.1": "US"}).
				SetTable("config", map[string]string{"mode": "prod"}).
				SetTableList("allowed", map[string][]string{"eng": {"dev", "sre"}}).
				SetStringField("dept", "eng").
				SetStringField("name", "dev").
				SetFunc("maintenance", func(_ context.Context, _ []Value) (Value, error) {
					return BoolValue(true), nil
				}).
				SetFunc("get_score", func(_ context.Context, _ []Value) (Value, error) {
					return FloatValue(7.5), nil
				}).
				SetFunc("is_tor", func(_ context.Context, _ []Value) (Value, error) {
					return BoolValue(false), nil
				}).
				SetFunc("get_cidrs", func(_ context.Context, _ []Value) (Value, error) {
					_, ipNet, _ := net.ParseCIDR("192.168.0.0/16")
					return ArrayValue{CIDRValue{IPNet: ipNet}}, nil
				})

			origResult, origErr := original.Execute(ctx)
			restoredResult, restoredErr := restored.Execute(ctx)

			assert.Equal(t, origErr, restoredErr)
			assert.Equal(t, origResult, restoredResult)
		})
	}
}

func TestMarshalBinaryCompactness(t *testing.T) {
	filter, err := Compile(`name == "test"`, nil)
	require.NoError(t, err)

	data, err := filter.MarshalBinary()
	require.NoError(t, err)

	// Header: "WF" (2) + version (1) = 3
	// BinaryExpr tag (1) + operator (1) = 2
	// FieldExpr tag (1) + name len varint (1) + "name" (4) = 6
	// LiteralExpr tag (1) + string val tag (1) + len varint (1) + "test" (4) = 7
	// Total: 3 + 2 + 6 + 7 = 18
	assert.Equal(t, 18, len(data))
}

func TestUnmarshalBinaryErrors(t *testing.T) {
	t.Run("empty data", func(t *testing.T) {
		f := &Filter{}
		err := f.UnmarshalBinary([]byte{})
		assert.Error(t, err)
	})

	t.Run("invalid magic", func(t *testing.T) {
		f := &Filter{}
		err := f.UnmarshalBinary([]byte("XX\x01"))
		assert.ErrorIs(t, err, errInvalidMagic)
	})

	t.Run("wrong version", func(t *testing.T) {
		f := &Filter{}
		err := f.UnmarshalBinary([]byte("WF\x99"))
		assert.ErrorIs(t, err, errInvalidVersion)
	})

	t.Run("truncated after header", func(t *testing.T) {
		f := &Filter{}
		err := f.UnmarshalBinary([]byte("WF\x01"))
		assert.Error(t, err)
	})

	t.Run("invalid node tag", func(t *testing.T) {
		f := &Filter{}
		err := f.UnmarshalBinary([]byte("WF\x01\xFF"))
		assert.Error(t, err)
	})

	t.Run("invalid value tag", func(t *testing.T) {
		f := &Filter{}
		// LiteralExpr node (0x04) followed by invalid value tag (0xFF)
		err := f.UnmarshalBinary([]byte("WF\x01\x04\xFF"))
		assert.Error(t, err)
	})

	t.Run("truncated string", func(t *testing.T) {
		f := &Filter{}
		// FieldExpr (0x03) + string length 10, but no data
		err := f.UnmarshalBinary([]byte("WF\x01\x03\x0A"))
		assert.Error(t, err)
	})
}

func TestMarshalUnmarshalRoundtrip(t *testing.T) {
	expr := `lower(name) == "admin" and ip not in $blocked and status in {400..599}`

	filter1, err := Compile(expr, nil)
	require.NoError(t, err)

	// Marshal
	data1, err := filter1.MarshalBinary()
	require.NoError(t, err)

	// Unmarshal
	filter2 := &Filter{}
	err = filter2.UnmarshalBinary(data1)
	require.NoError(t, err)

	// Marshal again
	data2, err := filter2.MarshalBinary()
	require.NoError(t, err)

	// Binary output should be identical
	assert.Equal(t, data1, data2)
}

func TestMarshalBinaryBytesValue(t *testing.T) {
	filter, err := Compile(`data == "hello"`, nil)
	require.NoError(t, err)

	// Replace the literal with a BytesValue to exercise that encoding path
	bin := filter.expr.(*BinaryExpr)
	bin.Right = &LiteralExpr{Value: BytesValue([]byte("hello"))}

	data, err := filter.MarshalBinary()
	require.NoError(t, err)

	restored := &Filter{}
	require.NoError(t, restored.UnmarshalBinary(data))

	ctx := NewExecutionContext().SetBytesField("data", []byte("hello"))
	r1, _ := filter.Execute(ctx)
	r2, _ := restored.Execute(ctx)
	assert.Equal(t, r1, r2)
}

func TestMarshalBinaryIPv6Value(t *testing.T) {
	filter, err := Compile(`ip == 2001:db8::1`, nil)
	require.NoError(t, err)

	data, err := filter.MarshalBinary()
	require.NoError(t, err)

	restored := &Filter{}
	require.NoError(t, restored.UnmarshalBinary(data))

	ctx := NewExecutionContext().SetIPField("ip", "2001:db8::1")
	r1, _ := filter.Execute(ctx)
	r2, _ := restored.Execute(ctx)
	assert.Equal(t, r1, r2)
}

func TestMarshalBinaryNilLiteral(t *testing.T) {
	filter, err := Compile(`name == "x"`, nil)
	require.NoError(t, err)

	// Replace right side with nil literal
	bin := filter.expr.(*BinaryExpr)
	bin.Right = &LiteralExpr{Value: nil}

	data, err := filter.MarshalBinary()
	require.NoError(t, err)

	restored := &Filter{}
	require.NoError(t, restored.UnmarshalBinary(data))
}

func TestWriteExprUnknownType(t *testing.T) {
	w := &encWriter{buf: make([]byte, 0, 64)}
	err := w.writeExpr(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown expression type")
}

func TestWriteValueUnknownType(t *testing.T) {
	w := &encWriter{buf: make([]byte, 0, 64)}
	err := w.writeValue(ArrayValue{StringValue("a")})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown value type")
}

func TestMarshalBinaryWriteExprError(t *testing.T) {
	filter, err := Compile(`name == "x"`, nil)
	require.NoError(t, err)

	// Inject an unsupported value type to trigger writeValue error
	bin := filter.expr.(*BinaryExpr)
	bin.Right = &LiteralExpr{Value: ArrayValue{StringValue("a")}}

	_, err = filter.MarshalBinary()
	assert.Error(t, err)
}

func TestUnmarshalBinaryTruncatedDecoding(t *testing.T) {
	// Helper: marshal a valid filter, then truncate at various offsets
	compile := func(expr string) []byte {
		f, err := Compile(expr, nil)
		require.NoError(t, err)
		data, err := f.MarshalBinary()
		require.NoError(t, err)
		return data
	}

	t.Run("truncated binary expr left", func(t *testing.T) {
		data := compile(`a == 1 and b == 2`)
		// Header(3) + BinaryExpr tag(1) + operator(1) = 5, truncate after operator
		f := &Filter{}
		err := f.UnmarshalBinary(data[:5])
		assert.Error(t, err)
	})

	t.Run("truncated binary expr right", func(t *testing.T) {
		data := compile(`a == 1 and b == 2`)
		// Truncate partway through - after left subtree but before right completes
		for i := 6; i < len(data)-1; i++ {
			f := &Filter{}
			err := f.UnmarshalBinary(data[:i])
			if err != nil {
				return // found a truncation point that errors
			}
		}
		t.Fatal("expected at least one truncation to error")
	})

	t.Run("truncated unary operand", func(t *testing.T) {
		data := compile(`not active`)
		// Header(3) + UnaryExpr tag(1) + operator(1) = 5
		f := &Filter{}
		err := f.UnmarshalBinary(data[:5])
		assert.Error(t, err)
	})

	t.Run("truncated field name", func(t *testing.T) {
		data := compile(`name == "x"`)
		// Header(3) + BinaryExpr(1) + op(1) + FieldExpr tag(1) + varint len(1) = 7
		// Name is 4 bytes, truncate mid-name
		f := &Filter{}
		err := f.UnmarshalBinary(data[:9])
		assert.Error(t, err)
	})

	t.Run("truncated literal string value", func(t *testing.T) {
		data := compile(`name == "hello"`)
		f := &Filter{}
		err := f.UnmarshalBinary(data[:len(data)-2])
		assert.Error(t, err)
	})

	t.Run("truncated int value", func(t *testing.T) {
		data := compile(`x == 999999999999`)
		f := &Filter{}
		// Truncate to cut into the varint
		err := f.UnmarshalBinary(data[:len(data)-1])
		assert.Error(t, err)
	})

	t.Run("truncated float value", func(t *testing.T) {
		data := compile(`x == 3.14`)
		f := &Filter{}
		err := f.UnmarshalBinary(data[:len(data)-3])
		assert.Error(t, err)
	})

	t.Run("truncated IP value", func(t *testing.T) {
		data := compile(`ip == 192.168.1.1`)
		f := &Filter{}
		err := f.UnmarshalBinary(data[:len(data)-2])
		assert.Error(t, err)
	})

	t.Run("truncated CIDR value ip", func(t *testing.T) {
		data := compile(`ip in 10.0.0.0/8`)
		f := &Filter{}
		err := f.UnmarshalBinary(data[:len(data)-3])
		assert.Error(t, err)
	})

	t.Run("truncated CIDR value mask", func(t *testing.T) {
		data := compile(`ip in 10.0.0.0/8`)
		f := &Filter{}
		err := f.UnmarshalBinary(data[:len(data)-1])
		assert.Error(t, err)
	})

	t.Run("truncated bytes value", func(t *testing.T) {
		f, err := Compile(`data == "x"`, nil)
		require.NoError(t, err)
		bin := f.expr.(*BinaryExpr)
		bin.Right = &LiteralExpr{Value: BytesValue([]byte("hello"))}
		data, err := f.MarshalBinary()
		require.NoError(t, err)

		restored := &Filter{}
		err = restored.UnmarshalBinary(data[:len(data)-2])
		assert.Error(t, err)
	})

	t.Run("truncated array count", func(t *testing.T) {
		data := compile(`x in {1, 2, 3}`)
		// Header(3) + BinaryExpr(1) + op(1) + FieldExpr(~) = some offset
		// Truncate just after the array tag
		f := &Filter{}
		for i := len(data) - 1; i > 3; i-- {
			err := f.UnmarshalBinary(data[:i])
			if err != nil {
				return
			}
		}
		t.Fatal("expected truncation error")
	})

	t.Run("truncated range end", func(t *testing.T) {
		data := compile(`x in {1..100}`)
		f := &Filter{}
		err := f.UnmarshalBinary(data[:len(data)-1])
		assert.Error(t, err)
	})

	t.Run("truncated index object", func(t *testing.T) {
		data := compile(`data["key"] == "val"`)
		f := &Filter{}
		// Truncate to cut into the index expression
		for i := len(data) - 1; i > 3; i-- {
			err := f.UnmarshalBinary(data[:i])
			if err != nil {
				return
			}
		}
		t.Fatal("expected truncation error")
	})

	t.Run("truncated unpack array", func(t *testing.T) {
		data := compile(`tags[*] == "x"`)
		f := &Filter{}
		err := f.UnmarshalBinary(data[:len(data)-1])
		assert.Error(t, err)
	})

	t.Run("truncated list ref name", func(t *testing.T) {
		data := compile(`ip in $blocked_ips`)
		f := &Filter{}
		err := f.UnmarshalBinary(data[:len(data)-3])
		assert.Error(t, err)
	})

	t.Run("truncated function call name", func(t *testing.T) {
		data := compile(`lower(name) == "x"`)
		f := &Filter{}
		err := f.UnmarshalBinary(data[:8])
		assert.Error(t, err)
	})

	t.Run("truncated function call arg count", func(t *testing.T) {
		data := compile(`lower(name) == "x"`)
		// Find where the function call starts and truncate after name but before arg count
		f := &Filter{}
		for i := 7; i < len(data)-1; i++ {
			err := f.UnmarshalBinary(data[:i])
			if err != nil {
				return
			}
		}
		t.Fatal("expected truncation error")
	})

	t.Run("truncated function call arg", func(t *testing.T) {
		data := compile(`get_score(name) > 5.0`)
		f := &Filter{}
		err := f.UnmarshalBinary(data[:len(data)-5])
		assert.Error(t, err)
	})

	t.Run("empty body after valid header", func(t *testing.T) {
		f := &Filter{}
		err := f.UnmarshalBinary([]byte("WF\x01"))
		assert.Error(t, err)
	})

	t.Run("readByte at EOF returns zero", func(t *testing.T) {
		r := &decReader{data: []byte{}, pos: 0}
		assert.Equal(t, byte(0), r.readByte())
	})

	t.Run("readN past end returns nil", func(t *testing.T) {
		r := &decReader{data: []byte{1, 2}, pos: 0}
		assert.Nil(t, r.readN(5))
	})

	t.Run("readUvarint truncated", func(t *testing.T) {
		r := &decReader{data: []byte{0x80}, pos: 0} // incomplete varint
		_, err := r.readUvarint()
		assert.Error(t, err)
	})

	t.Run("readVarint truncated", func(t *testing.T) {
		r := &decReader{data: []byte{0x80}, pos: 0} // incomplete varint
		_, err := r.readVarint()
		assert.Error(t, err)
	})

	t.Run("readUint64 truncated", func(t *testing.T) {
		r := &decReader{data: []byte{1, 2, 3}, pos: 0}
		_, err := r.readUint64()
		assert.Error(t, err)
	})

	t.Run("readString truncated length", func(t *testing.T) {
		r := &decReader{data: []byte{0x80}, pos: 0}
		_, err := r.readString()
		assert.Error(t, err)
	})

	t.Run("readByteSlice truncated length", func(t *testing.T) {
		r := &decReader{data: []byte{0x80}, pos: 0}
		_, err := r.readByteSlice()
		assert.Error(t, err)
	})

	t.Run("readByteSlice truncated body", func(t *testing.T) {
		r := &decReader{data: []byte{0x05, 0x01}, pos: 0} // says 5 bytes, only 1
		_, err := r.readByteSlice()
		assert.Error(t, err)
	})

	t.Run("readValue at EOF", func(t *testing.T) {
		r := &decReader{data: []byte{}, pos: 0}
		_, err := r.readValue()
		assert.Error(t, err)
	})

	t.Run("readExpr at EOF", func(t *testing.T) {
		r := &decReader{data: []byte{}, pos: 0}
		_, err := r.readExpr()
		assert.Error(t, err)
	})

	t.Run("truncated range start", func(t *testing.T) {
		// Range node tag + truncated start
		f := &Filter{}
		err := f.UnmarshalBinary([]byte{'W', 'F', 0x01, nodeTypeRange})
		assert.Error(t, err)
	})

	t.Run("truncated index index", func(t *testing.T) {
		data := compile(`data["key"] == "val"`)
		// Find the IndexExpr and truncate after object but before index completes
		// The binary structure: BinaryExpr > IndexExpr > (FieldExpr "data", LiteralExpr "key")
		// Truncate progressively to hit the index read error
		f := &Filter{}
		// Header(3) + BinaryExpr(1+1) + IndexExpr(1) + FieldExpr(1+1+4) = 12
		// Next is the index LiteralExpr - truncate right there
		err := f.UnmarshalBinary(data[:12])
		assert.Error(t, err)
	})

	t.Run("truncated array element", func(t *testing.T) {
		data := compile(`x in {1, 2, 3}`)
		// Truncate to keep array count but cut mid-element
		// Header(3) + BinaryExpr(1+1) + Field(1+1+1) + ArrayExpr(1) + count(1) = 10
		// Then 3 LiteralExprs, truncate after first
		f := &Filter{}
		err := f.UnmarshalBinary(data[:13])
		assert.Error(t, err)
	})

	t.Run("writeExpr error in binary right", func(t *testing.T) {
		// Create a BinaryExpr where Right contains an unsupported value
		w := &encWriter{buf: make([]byte, 0, 64)}
		expr := &BinaryExpr{
			Left:     &FieldExpr{Name: "x"},
			Operator: TokenEq,
			Right:    &LiteralExpr{Value: ArrayValue{StringValue("a")}},
		}
		err := w.writeExpr(expr)
		assert.Error(t, err)
	})

	t.Run("writeExpr error in binary left", func(t *testing.T) {
		w := &encWriter{buf: make([]byte, 0, 64)}
		expr := &BinaryExpr{
			Left:     &LiteralExpr{Value: ArrayValue{}},
			Operator: TokenEq,
			Right:    &FieldExpr{Name: "x"},
		}
		err := w.writeExpr(expr)
		assert.Error(t, err)
	})

	t.Run("writeExpr error in unary operand", func(t *testing.T) {
		w := &encWriter{buf: make([]byte, 0, 64)}
		expr := &UnaryExpr{
			Operator: TokenNot,
			Operand:  &LiteralExpr{Value: ArrayValue{}},
		}
		err := w.writeExpr(expr)
		assert.Error(t, err)
	})

	t.Run("writeExpr error in array element", func(t *testing.T) {
		w := &encWriter{buf: make([]byte, 0, 64)}
		expr := &ArrayExpr{
			Elements: []Expression{&LiteralExpr{Value: ArrayValue{}}},
		}
		err := w.writeExpr(expr)
		assert.Error(t, err)
	})

	t.Run("writeExpr error in range start", func(t *testing.T) {
		w := &encWriter{buf: make([]byte, 0, 64)}
		expr := &RangeExpr{
			Start: &LiteralExpr{Value: ArrayValue{}},
			End:   &LiteralExpr{Value: IntValue(10)},
		}
		err := w.writeExpr(expr)
		assert.Error(t, err)
	})

	t.Run("writeExpr error in range end", func(t *testing.T) {
		w := &encWriter{buf: make([]byte, 0, 64)}
		expr := &RangeExpr{
			Start: &LiteralExpr{Value: IntValue(1)},
			End:   &LiteralExpr{Value: ArrayValue{}},
		}
		err := w.writeExpr(expr)
		assert.Error(t, err)
	})

	t.Run("writeExpr error in index object", func(t *testing.T) {
		w := &encWriter{buf: make([]byte, 0, 64)}
		expr := &IndexExpr{
			Object: &LiteralExpr{Value: ArrayValue{}},
			Index:  &LiteralExpr{Value: StringValue("key")},
		}
		err := w.writeExpr(expr)
		assert.Error(t, err)
	})

	t.Run("writeExpr error in index key", func(t *testing.T) {
		w := &encWriter{buf: make([]byte, 0, 64)}
		expr := &IndexExpr{
			Object: &FieldExpr{Name: "data"},
			Index:  &LiteralExpr{Value: ArrayValue{}},
		}
		err := w.writeExpr(expr)
		assert.Error(t, err)
	})

	t.Run("writeExpr error in unpack array", func(t *testing.T) {
		w := &encWriter{buf: make([]byte, 0, 64)}
		expr := &UnpackExpr{
			Array: &LiteralExpr{Value: ArrayValue{}},
		}
		err := w.writeExpr(expr)
		assert.Error(t, err)
	})

	t.Run("writeExpr error in function call arg", func(t *testing.T) {
		w := &encWriter{buf: make([]byte, 0, 64)}
		expr := &FunctionCallExpr{
			Name:      "fn",
			Arguments: []Expression{&LiteralExpr{Value: ArrayValue{}}},
		}
		err := w.writeExpr(expr)
		assert.Error(t, err)
	})

	t.Run("readExpr truncated binary right", func(t *testing.T) {
		// Manually construct: BinaryExpr + op + valid left FieldExpr, then EOF
		data := []byte("WF\x01")
		data = append(data, nodeTypeBinary, byte(TokenEq))
		data = append(data, nodeTypeField, 0x01, 'x') // left = FieldExpr "x"
		// no right expr
		f := &Filter{}
		err := f.UnmarshalBinary(data)
		assert.Error(t, err)
	})

	t.Run("readExpr truncated function arg", func(t *testing.T) {
		// FunctionCall "fn" with 1 arg but no arg data
		data := []byte("WF\x01")
		data = append(data, nodeTypeFunctionCall)
		data = append(data, 0x02, 'f', 'n') // name = "fn"
		data = append(data, 0x01)           // 1 argument
		// no arg data
		f := &Filter{}
		err := f.UnmarshalBinary(data)
		assert.Error(t, err)
	})

	t.Run("readExpr array truncated count", func(t *testing.T) {
		// ArrayExpr tag + incomplete varint for count
		data := []byte("WF\x01")
		data = append(data, nodeTypeArray, 0x80) // 0x80 = incomplete varint
		f := &Filter{}
		err := f.UnmarshalBinary(data)
		assert.Error(t, err)
	})

	t.Run("readExpr index truncated object", func(t *testing.T) {
		// IndexExpr tag + EOF (no object)
		data := []byte("WF\x01")
		data = append(data, nodeTypeIndex)
		f := &Filter{}
		err := f.UnmarshalBinary(data)
		assert.Error(t, err)
	})

	t.Run("readExpr unpack truncated array", func(t *testing.T) {
		// UnpackExpr tag + EOF (no array)
		data := []byte("WF\x01")
		data = append(data, nodeTypeUnpack)
		f := &Filter{}
		err := f.UnmarshalBinary(data)
		assert.Error(t, err)
	})

	t.Run("readExpr function call truncated count", func(t *testing.T) {
		// FunctionCall tag + name "fn" + incomplete varint for arg count
		data := []byte("WF\x01")
		data = append(data, nodeTypeFunctionCall)
		data = append(data, 0x02, 'f', 'n') // name = "fn"
		data = append(data, 0x80)           // incomplete varint
		f := &Filter{}
		err := f.UnmarshalBinary(data)
		assert.Error(t, err)
	})

	t.Run("readValue CIDR truncated ip bytes", func(t *testing.T) {
		// LiteralExpr + CIDR tag + incomplete IP byte slice
		data := []byte("WF\x01")
		data = append(data, nodeTypeLiteral, valTypeCIDR, 0x80) // incomplete varint for IP length
		f := &Filter{}
		err := f.UnmarshalBinary(data)
		assert.Error(t, err)
	})

	t.Run("readValue truncated bool", func(t *testing.T) {
		r := &decReader{data: []byte{valTypeBool}, pos: 0}
		_, err := r.readValue()
		assert.Error(t, err)
	})
}

func BenchmarkMarshalBinary(b *testing.B) {
	filter, _ := Compile(
		`(lower(http.host) == "example.com" or http.host wildcard "*.example.com") and http.status >= 400 and ip.src not in $blocked_ips`,
		nil,
	)

	b.ReportAllocs()
	for b.Loop() {
		_, _ = filter.MarshalBinary()
	}
}

func BenchmarkUnmarshalBinary(b *testing.B) {
	filter, _ := Compile(
		`(lower(http.host) == "example.com" or http.host wildcard "*.example.com") and http.status >= 400 and ip.src not in $blocked_ips`,
		nil,
	)
	data, _ := filter.MarshalBinary()

	b.ReportAllocs()
	for b.Loop() {
		f := &Filter{}
		_ = f.UnmarshalBinary(data)
	}
}

func BenchmarkCompileVsUnmarshal(b *testing.B) {
	expr := `(lower(http.host) == "example.com" or http.host wildcard "*.example.com") and http.status >= 400 and ip.src not in $blocked_ips`

	filter, _ := Compile(expr, nil)
	data, _ := filter.MarshalBinary()

	b.Run("compile", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = Compile(expr, nil)
		}
	})

	b.Run("unmarshal", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			f := &Filter{}
			_ = f.UnmarshalBinary(data)
		}
	})
}

func FuzzMarshalUnmarshal(f *testing.F) {
	f.Add(`name == "test"`)
	f.Add(`status >= 400`)
	f.Add(`ip in $blocked`)
	f.Add(`tags[*] contains "prod"`)
	f.Add(`cidr(ip, 24) == "10.0.0.0"`)
	f.Add(`a and b or not c`)
	f.Add(`x in {1..100}`)
	f.Add(`lower(name) not contains "admin"`)
	f.Add(`data["key"] == "val"`)
	f.Add(`$geo[ip] == "US"`)
	f.Add(`role in $allowed[dept]`)
	f.Add(`$config["mode"] == "prod"`)
	f.Add(`maintenance() == true`)
	f.Add(`get_score(name) > 5.0`)
	f.Add(`is_tor(ip) and name == "test"`)
	f.Add(`ts >= 2026-03-19T10:00:00Z`)
	f.Add(`ttl == 30m`)
	f.Add(`ts + 1h >= 2026-03-19T11:00:00Z`)
	f.Add(`ttl * 2 > 1h`)
	f.Add(`ts in {2026-03-19T00:00:00Z..2026-03-20T00:00:00Z}`)
	f.Add(`ttl in {1h..3h}`)
	f.Add(`ts <= now()`)
	f.Add(`ttl >= 2d4h30m15s`)

	f.Fuzz(func(t *testing.T, expr string) {
		filter, err := Compile(expr, nil)
		if err != nil {
			return
		}

		data, err := filter.MarshalBinary()
		if err != nil {
			t.Fatalf("MarshalBinary failed for %q: %v", expr, err)
		}

		restored := &Filter{}
		if err := restored.UnmarshalBinary(data); err != nil {
			t.Fatalf("UnmarshalBinary failed for %q: %v", expr, err)
		}

		// Re-marshal should produce identical bytes (idempotency)
		data2, err := restored.MarshalBinary()
		if err != nil {
			t.Fatalf("second MarshalBinary failed for %q: %v", expr, err)
		}

		if len(data) != len(data2) {
			t.Fatalf("roundtrip mismatch for %q: %d vs %d bytes", expr, len(data), len(data2))
		}

		// Execution results must match after roundtrip
		ctx := NewExecutionContext().
			SetStringField("name", "test").
			SetIntField("status", 200).
			SetBoolField("active", true).
			SetIPField("ip", "10.0.0.1").
			SetArrayField("tags", []string{"a", "b"}).
			SetMapField("data", map[string]string{"key": "val"}).
			SetList("names", []string{"test"}).
			SetIPList("blocked", []string{"10.0.0.0/8"}).
			SetTable("geo", map[string]string{"10.0.0.1": "US"}).
			SetTimeField("ts", time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)).
			SetDurationField("ttl", time.Hour).
			WithNow(func() time.Time { return time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC) }).
			SetFunc("maintenance", func(_ context.Context, _ []Value) (Value, error) {
				return BoolValue(true), nil
			}).
			SetFunc("get_score", func(_ context.Context, _ []Value) (Value, error) {
				return FloatValue(7.5), nil
			}).
			SetFunc("is_tor", func(_ context.Context, _ []Value) (Value, error) {
				return BoolValue(false), nil
			})

		r1, err1 := filter.Execute(ctx)
		r2, err2 := restored.Execute(ctx)
		if (err1 == nil) != (err2 == nil) {
			t.Fatalf("execution error mismatch for %q: %v vs %v", expr, err1, err2)
		}
		if r1 != r2 {
			t.Fatalf("execution result mismatch for %q: %v vs %v", expr, r1, r2)
		}

		// Hash must be stable after roundtrip
		if filter.Hash() != restored.Hash() {
			t.Fatalf("hash mismatch for %q", expr)
		}
	})
}

func TestEncodingTimeAndDuration(t *testing.T) {
	t.Run("time roundtrip", func(t *testing.T) {
		schema := NewSchema().AddField("ts", TypeTime)
		filter, err := Compile(`ts >= 2026-03-19T10:00:00Z`, schema)
		require.NoError(t, err)

		data, err := filter.MarshalBinary()
		require.NoError(t, err)

		restored := &Filter{}
		err = restored.UnmarshalBinary(data)
		require.NoError(t, err)

		now := time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC)
		ctx := NewExecutionContext().SetTimeField("ts", now)

		r1, err := filter.Execute(ctx)
		require.NoError(t, err)
		r2, err := restored.Execute(ctx)
		require.NoError(t, err)
		assert.Equal(t, r1, r2)
		assert.True(t, r1)

		assert.Equal(t, filter.Hash(), restored.Hash())
	})

	t.Run("duration roundtrip", func(t *testing.T) {
		schema := NewSchema().AddField("ttl", TypeDuration)
		filter, err := Compile(`ttl == 30m`, schema)
		require.NoError(t, err)

		data, err := filter.MarshalBinary()
		require.NoError(t, err)

		restored := &Filter{}
		err = restored.UnmarshalBinary(data)
		require.NoError(t, err)

		ctx := NewExecutionContext().SetDurationField("ttl", 30*time.Minute)

		r1, err := filter.Execute(ctx)
		require.NoError(t, err)
		r2, err := restored.Execute(ctx)
		require.NoError(t, err)
		assert.Equal(t, r1, r2)
		assert.True(t, r1)

		assert.Equal(t, filter.Hash(), restored.Hash())
	})

	t.Run("time with fractional seconds roundtrip", func(t *testing.T) {
		filter, err := Compile(`ts >= 2026-03-19T10:00:00.123456789Z`, nil)
		require.NoError(t, err)

		data, err := filter.MarshalBinary()
		require.NoError(t, err)

		restored := &Filter{}
		err = restored.UnmarshalBinary(data)
		require.NoError(t, err)

		assert.Equal(t, filter.Hash(), restored.Hash())
	})

	t.Run("compound duration roundtrip", func(t *testing.T) {
		filter, err := Compile(`ttl >= 2d4h30m`, nil)
		require.NoError(t, err)

		data, err := filter.MarshalBinary()
		require.NoError(t, err)

		restored := &Filter{}
		err = restored.UnmarshalBinary(data)
		require.NoError(t, err)

		ctx := NewExecutionContext().SetDurationField("ttl", 3*24*time.Hour)

		r1, _ := filter.Execute(ctx)
		r2, _ := restored.Execute(ctx)
		assert.Equal(t, r1, r2)
		assert.True(t, r1)
	})
}

func TestUnmarshalBinaryDecodeLimits(t *testing.T) {
	header := func() []byte {
		return []byte{'W', 'F', encodingVersion}
	}

	appendUvarint := func(buf []byte, v uint64) []byte {
		var tmp [binary.MaxVarintLen64]byte
		n := binary.PutUvarint(tmp[:], v)
		return append(buf, tmp[:n]...)
	}

	t.Run("string length exceeds limit", func(t *testing.T) {
		data := header()
		data = append(data, nodeTypeField)
		data = appendUvarint(data, maxDecodeStringLen+1)

		f := &Filter{}
		err := f.UnmarshalBinary(data)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "string length")
	})

	t.Run("array element count exceeds limit", func(t *testing.T) {
		data := header()
		data = append(data, nodeTypeArray)
		data = appendUvarint(data, maxDecodeArrayLen+1)

		f := &Filter{}
		err := f.UnmarshalBinary(data)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "element count")
	})

	t.Run("function argument count exceeds limit", func(t *testing.T) {
		data := header()
		data = append(data, nodeTypeFunctionCall)
		data = appendUvarint(data, 4)
		data = append(data, "test"...)
		data = appendUvarint(data, maxDecodeArrayLen+1)

		f := &Filter{}
		err := f.UnmarshalBinary(data)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "element count")
	})

	t.Run("depth exceeds limit", func(t *testing.T) {
		data := header()
		for range maxDecodeDepth + 1 {
			data = append(data, nodeTypeUnary)
			data = append(data, byte(TokenNot))
		}
		data = append(data, nodeTypeField)
		data = appendUvarint(data, 1)
		data = append(data, 'x')

		f := &Filter{}
		err := f.UnmarshalBinary(data)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "depth exceeds")
	})

	t.Run("node count exceeds limit", func(t *testing.T) {
		data := header()
		for range maxDecodeNodes {
			data = append(data, nodeTypeUnary)
			data = append(data, byte(TokenNot))
		}
		data = append(data, nodeTypeField)
		data = appendUvarint(data, 1)
		data = append(data, 'x')

		f := &Filter{}
		err := f.UnmarshalBinary(data)
		assert.Error(t, err)
	})

	t.Run("valid binary within limits", func(t *testing.T) {
		filter, err := Compile(`x == 1 and y == 2`, nil)
		require.NoError(t, err)

		data, err := filter.MarshalBinary()
		require.NoError(t, err)

		restored := &Filter{}
		err = restored.UnmarshalBinary(data)
		assert.NoError(t, err)

		ctx := NewExecutionContext().SetIntField("x", 1).SetIntField("y", 2)
		result, err := restored.Execute(ctx)
		assert.NoError(t, err)
		assert.True(t, result)
	})
}
