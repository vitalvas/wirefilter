package wirefilter

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func BenchmarkCompile(b *testing.B) {
	schema := NewSchema().
		AddField("http.host", TypeString).
		AddField("http.status", TypeInt).
		AddField("ip.src", TypeIP)

	tests := []struct {
		name       string
		expression string
	}{
		{
			name:       "simple equality",
			expression: `http.host == "example.com"`,
		},
		{
			name:       "multiple conditions",
			expression: `http.host == "example.com" and http.status >= 400`,
		},
		{
			name:       "complex expression",
			expression: `(http.host == "example.com" or http.host == "test.com") and http.status >= 200 and http.status < 300`,
		},
		{
			name:       "ip in cidr",
			expression: `ip.src in "192.168.0.0/16"`,
		},
		{
			name:       "array membership",
			expression: `http.status in {200, 201, 204, 301, 302, 304}`,
		},
		{
			name:       "range expression",
			expression: `http.status in {200..299, 400..499}`,
		},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, err := Compile(tt.expression, schema)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkExecute(b *testing.B) {
	schema := NewSchema().
		AddField("http.host", TypeString).
		AddField("http.status", TypeInt).
		AddField("http.path", TypeString).
		AddField("ip.src", TypeIP).
		AddField("created_at", TypeTime).
		AddField("ttl", TypeDuration)

	tests := []struct {
		name       string
		expression string
		setup      func() *ExecutionContext
	}{
		{
			name:       "simple equality",
			expression: `http.host == "example.com"`,
			setup: func() *ExecutionContext {
				return NewExecutionContext().
					SetStringField("http.host", "example.com")
			},
		},
		{
			name:       "multiple conditions",
			expression: `http.host == "example.com" and http.status >= 400`,
			setup: func() *ExecutionContext {
				return NewExecutionContext().
					SetStringField("http.host", "example.com").
					SetIntField("http.status", 500)
			},
		},
		{
			name:       "complex boolean logic",
			expression: `(http.host == "example.com" or http.host == "test.com") and http.status >= 200 and http.status < 300`,
			setup: func() *ExecutionContext {
				return NewExecutionContext().
					SetStringField("http.host", "example.com").
					SetIntField("http.status", 200)
			},
		},
		{
			name:       "string contains",
			expression: `http.path contains "/api"`,
			setup: func() *ExecutionContext {
				return NewExecutionContext().
					SetStringField("http.path", "/api/v1/users")
			},
		},
		{
			name:       "regex match",
			expression: `http.host matches "^example\\..*"`,
			setup: func() *ExecutionContext {
				return NewExecutionContext().
					SetStringField("http.host", "example.com")
			},
		},
		{
			name:       "ip in cidr",
			expression: `ip.src in "192.168.0.0/16"`,
			setup: func() *ExecutionContext {
				return NewExecutionContext().
					SetIPField("ip.src", "192.168.1.1")
			},
		},
		{
			name:       "array membership",
			expression: `http.status in {200, 201, 204, 301, 302, 304}`,
			setup: func() *ExecutionContext {
				return NewExecutionContext().
					SetIntField("http.status", 200)
			},
		},
		{
			name:       "range expression",
			expression: `http.status in {200..299}`,
			setup: func() *ExecutionContext {
				return NewExecutionContext().
					SetIntField("http.status", 250)
			},
		},
		{
			name:       "time comparison",
			expression: `created_at >= 2026-03-19T10:00:00Z`,
			setup: func() *ExecutionContext {
				return NewExecutionContext().
					SetTimeField("created_at", time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC))
			},
		},
		{
			name:       "time arithmetic",
			expression: `created_at + 1h >= 2026-03-19T11:00:00Z`,
			setup: func() *ExecutionContext {
				return NewExecutionContext().
					SetTimeField("created_at", time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC))
			},
		},
		{
			name:       "now function",
			expression: `created_at <= now()`,
			setup: func() *ExecutionContext {
				fixed := time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC)
				return NewExecutionContext().
					SetTimeField("created_at", time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)).
					WithNow(func() time.Time { return fixed })
			},
		},
		{
			name:       "duration comparison",
			expression: `ttl >= 30m`,
			setup: func() *ExecutionContext {
				return NewExecutionContext().
					SetDurationField("ttl", time.Hour)
			},
		},
		{
			name:       "duration arithmetic",
			expression: `ttl * 2 > 1h`,
			setup: func() *ExecutionContext {
				return NewExecutionContext().
					SetDurationField("ttl", 45*time.Minute)
			},
		},
		{
			name:       "time range membership",
			expression: `created_at in {2026-03-19T00:00:00Z..2026-03-20T00:00:00Z}`,
			setup: func() *ExecutionContext {
				return NewExecutionContext().
					SetTimeField("created_at", time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC))
			},
		},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			filter, err := Compile(tt.expression, schema)
			if err != nil {
				b.Fatal(err)
			}

			ctx := tt.setup()

			b.ResetTimer()
			b.ReportAllocs()
			for b.Loop() {
				_, err := filter.Execute(ctx)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkExecuteUDF(b *testing.B) {
	filter, _ := Compile(`get_score(name) > 5.0 and is_active()`, nil)
	ctx := NewExecutionContext().
		SetStringField("name", "test").
		SetFunc("get_score", func(_ context.Context, _ []Value) (Value, error) {
			return FloatValue(7.5), nil
		}).
		SetFunc("is_active", func(_ context.Context, _ []Value) (Value, error) {
			return BoolValue(true), nil
		})

	b.ReportAllocs()
	for b.Loop() {
		_, _ = filter.Execute(ctx)
	}
}

func BenchmarkExecuteUDFCached(b *testing.B) {
	filter, _ := Compile(`get_score(name) > 5.0 and get_score(name) < 100.0`, nil)
	ctx := NewExecutionContext().
		EnableCache().
		SetStringField("name", "test").
		SetFunc("get_score", func(_ context.Context, _ []Value) (Value, error) {
			return FloatValue(7.5), nil
		})

	b.ReportAllocs()
	for b.Loop() {
		_, _ = filter.Execute(ctx)
	}
}

func BenchmarkSchemaValidation(b *testing.B) {
	schema := NewSchema().
		AddField("http.host", TypeString).
		AddField("http.status", TypeInt).
		AddField("ip.src", TypeIP).
		AddField("tags", TypeArray)

	expr := `http.host == "example.com" and http.status >= 400 and ip.src in "10.0.0.0/8"`

	b.ReportAllocs()
	for b.Loop() {
		_, _ = Compile(expr, schema)
	}
}

func FuzzCompile(f *testing.F) {
	f.Add(`http.host == "example.com"`)
	f.Add(`http.status >= 400`)
	f.Add(`http.host == "example.com" and http.status >= 400`)
	f.Add(`(http.host == "test.com" or http.path contains "/api") and http.status < 500`)
	f.Add(`http.status in {200, 201, 204, 301, 302, 304}`)
	f.Add(`port in {80..100, 443, 8000..9000}`)
	f.Add(`ip.src in "192.168.0.0/16"`)
	f.Add(`http.path matches "^/api/v[0-9]+/"`)
	f.Add(`not http.host == "blocked.com"`)
	f.Add(`true and false`)
	f.Add(`ip not in $blocked`)
	f.Add(`$geo[ip] == "US"`)
	f.Add(`role in $allowed[dept]`)
	f.Add(`$config["key"] == "val"`)
	f.Add(`name not contains "admin"`)
	f.Add(`cidr(ip, 24) == 10.0.0.0/24`)
	f.Add(`cidr6(ip, 64) == 2001:db8::/64`)
	f.Add(`lower(name) == "test"`)
	f.Add(`tags[*] == "prod"`)
	f.Add(`all(tags[*] contains "a")`)
	f.Add(`any(ports[*] > 80)`)
	f.Add(`data["key"] == "val"`)
	f.Add(`a xor b`)
	f.Add(`name wildcard "*.com"`)
	f.Add(`name strict wildcard "*.COM"`)
	f.Add(`tags === "a"`)
	f.Add(`tags !== "b"`)
	f.Add(`ip.src in 192.168.0.0/24`)
	f.Add(`concat("a", "b") == "ab"`)
	f.Add(`split(name, ",")[0] == "a"`)
	f.Add(`join(tags, ",") == "a,b"`)
	f.Add(`has_key(data, "key")`)
	f.Add(`has_value(tags, "a")`)
	f.Add(`starts_with(name, "test")`)
	f.Add(`ends_with(name, ".com")`)
	f.Add(`len(name) > 0`)
	f.Add(`url_decode(name) == "a b"`)
	f.Add(`substring(name, 0, 3) == "tes"`)
	f.Add(`trim(name) == "test"`)
	f.Add(`replace(name, "a", "b") == "b"`)
	f.Add(`regex_replace(name, "[0-9]+", "X") == "X"`)
	f.Add(`regex_extract(name, "[0-9]+") == "123"`)
	f.Add(`contains_word(name, "test")`)
	f.Add(`count(tags) > 0`)
	f.Add(`coalesce(a, b) == "x"`)
	f.Add(`abs(x) > 0`)
	f.Add(`ceil(x) == 4`)
	f.Add(`floor(x) == 3`)
	f.Add(`round(x) == 4`)
	f.Add(`is_ipv4(ip) == true`)
	f.Add(`is_loopback(ip) == true`)
	f.Add(`intersection(a, b)`)
	f.Add(`union(a, b)`)
	f.Add(`difference(a, b)`)
	f.Add(`contains_any(a, b)`)
	f.Add(`contains_all(a, b)`)
	f.Add(`custom_func() == true`)
	f.Add(`get_score(name) > 5.0`)
	f.Add(`is_tor(ip) and name == "test"`)
	f.Add(`ip in get_cidrs(name)`)
	f.Add(`exists(name)`)
	f.Add(`not exists(missing)`)
	f.Add(`exists(name) and name == "test"`)
	f.Add(`x + 1 > 5`)
	f.Add(`x * 2 == 10`)
	f.Add(`x / 3 == 1`)
	f.Add(`x % 2 == 0`)
	f.Add(`ts >= 2026-03-19T10:00:00Z`)
	f.Add(`ts + 1h >= 2026-03-19T11:00:00Z`)
	f.Add(`ts - 30m <= now()`)
	f.Add(`ts in {2026-03-19T00:00:00Z..2026-03-20T00:00:00Z}`)
	f.Add(`ttl == 30m`)
	f.Add(`ttl >= 2d4h30m15s`)
	f.Add(`ttl * 2 > 1h`)
	f.Add(`ttl / 30m == 2`)
	f.Add(`ttl % 1h == 30m`)
	f.Add(`ttl in {1h..3h}`)
	f.Add(`ts <= now()`)
	f.Add(`ts >= now() - 1h`)
	f.Add(`ts == "2026-03-19T10:00:00Z"`)
	f.Add(`1h + ts >= 2026-03-19T11:00:00Z`)

	f.Fuzz(func(t *testing.T, input string) {
		f1, err1 := Compile(input, nil)
		f2, err2 := Compile(input, nil)

		// Determinism: same input must produce same result
		if (err1 == nil) != (err2 == nil) {
			t.Fatalf("non-deterministic compile for %q", input)
		}
		if err1 != nil {
			return
		}

		// Hash stability
		if f1.Hash() != f2.Hash() {
			t.Fatalf("hash mismatch for %q: %s vs %s", input, f1.Hash(), f2.Hash())
		}

		// Execution determinism
		fixed := time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)
		ctx := NewExecutionContext().
			SetStringField("name", "test").
			SetIntField("x", 5).
			SetBoolField("active", true).
			SetIPField("ip", "10.0.0.1").
			SetArrayField("tags", []string{"a", "b"}).
			SetTimeField("ts", fixed).
			SetDurationField("ttl", time.Hour).
			WithNow(func() time.Time { return fixed })
		r1, e1 := f1.Execute(ctx)
		r2, e2 := f2.Execute(ctx)
		if (e1 == nil) != (e2 == nil) || r1 != r2 {
			t.Fatalf("non-deterministic execute for %q", input)
		}
	})
}

func FuzzExecute(f *testing.F) {
	f.Add(`http.host == "example.com"`, "example.com", int64(200))
	f.Add(`http.status >= 400`, "test.com", int64(500))
	f.Add(`http.host == "example.com" and http.status >= 400`, "example.com", int64(404))
	f.Add(`http.status in {200, 201, 204}`, "test.com", int64(200))
	f.Add(`http.host contains "example"`, "example.com", int64(200))
	f.Add(`http.status < 300`, "test.com", int64(250))
	f.Add(`not http.host == "blocked"`, "allowed.com", int64(200))

	schema := NewSchema().
		AddField("http.host", TypeString).
		AddField("http.status", TypeInt)

	f.Fuzz(func(t *testing.T, expression string, host string, status int64) {
		filter, err := Compile(expression, schema)
		if err != nil {
			return
		}

		ctx := NewExecutionContext().
			SetStringField("http.host", host).
			SetIntField("http.status", status)

		// Determinism: execute twice, same result
		r1, e1 := filter.Execute(ctx)
		r2, e2 := filter.Execute(ctx)
		if (e1 == nil) != (e2 == nil) || r1 != r2 {
			t.Fatalf("non-deterministic execute for %q", expression)
		}
	})
}

func FuzzExecuteTimeDuration(f *testing.F) {
	f.Add(`ts >= 2026-03-19T10:00:00Z`, int64(1742554800000000000), int64(3600000000000))
	f.Add(`ts + 1h >= 2026-03-19T11:00:00Z`, int64(1742554800000000000), int64(3600000000000))
	f.Add(`ttl == 30m`, int64(1742554800000000000), int64(1800000000000))
	f.Add(`ttl * 2 > 1h`, int64(1742554800000000000), int64(2700000000000))
	f.Add(`ts <= now()`, int64(1742554800000000000), int64(3600000000000))
	f.Add(`ttl in {1h..3h}`, int64(1742554800000000000), int64(7200000000000))
	f.Add(`ts >= now() - 1h`, int64(1742554800000000000), int64(3600000000000))

	schema := NewSchema().
		AddField("ts", TypeTime).
		AddField("ttl", TypeDuration)

	f.Fuzz(func(t *testing.T, expression string, tsNano int64, ttlNano int64) {
		filter, err := Compile(expression, schema)
		if err != nil {
			return
		}

		fixed := time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC)
		ctx := NewExecutionContext().
			SetTimeField("ts", time.Unix(0, tsNano).UTC()).
			SetDurationField("ttl", time.Duration(ttlNano)).
			WithNow(func() time.Time { return fixed })

		r1, e1 := filter.Execute(ctx)
		r2, e2 := filter.Execute(ctx)
		if (e1 == nil) != (e2 == nil) || r1 != r2 {
			t.Fatalf("non-deterministic execute for %q", expression)
		}
	})
}

func FuzzExecuteMultiType(f *testing.F) {
	f.Add(`name == value`, "test", "test", int64(0), "10.0.0.1")
	f.Add(`name contains value`, "hello world", "world", int64(0), "10.0.0.1")
	f.Add(`count > 5`, "x", "x", int64(10), "10.0.0.1")
	f.Add(`ip == "10.0.0.1"`, "x", "x", int64(0), "10.0.0.1")
	f.Add(`name not contains "admin"`, "user", "admin", int64(0), "10.0.0.1")
	f.Add(`count in {1..100}`, "x", "x", int64(50), "10.0.0.1")
	f.Add(`lower(name) == value`, "TEST", "test", int64(0), "10.0.0.1")
	f.Add(`len(name) > count`, "hello", "x", int64(3), "10.0.0.1")
	f.Add(`$geo[ip] == "US"`, "x", "US", int64(0), "10.0.0.1")
	f.Add(`name in $allowed[value]`, "dev", "eng", int64(0), "10.0.0.1")
	f.Add(`custom_func() == true`, "x", "x", int64(0), "10.0.0.1")
	f.Add(`get_score(name) > 5.0`, "test", "x", int64(0), "10.0.0.1")

	f.Fuzz(func(_ *testing.T, expression, strVal1, strVal2 string, intVal int64, ipVal string) {
		filter, err := Compile(expression, nil)
		if err != nil {
			return
		}

		ctx := NewExecutionContext().
			SetStringField("name", strVal1).
			SetStringField("value", strVal2).
			SetIntField("count", intVal).
			SetIPField("ip", ipVal).
			SetBoolField("active", intVal > 0).
			SetArrayField("tags", []string{strVal1, strVal2}).
			SetIntArrayField("ports", []int64{intVal, intVal + 1}).
			SetMapField("data", map[string]string{"key": strVal1}).
			SetList("names", []string{strVal1, strVal2}).
			SetIPList("nets", []string{"10.0.0.0/8", "192.168.0.0/16"}).
			SetTable("geo", map[string]string{ipVal: "US", strVal1: strVal2}).
			SetTableList("allowed", map[string][]string{strVal1: {strVal2}}).
			SetTableIPList("blocked", map[string][]string{"office": {"10.0.0.0/8"}}).
			SetTimeField("ts", time.Unix(intVal, 0).UTC()).
			SetDurationField("ttl", time.Duration(intVal)*time.Second).
			WithNow(func() time.Time { return time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC) }).
			SetFunc("custom_func", func(_ context.Context, _ []Value) (Value, error) {
				return BoolValue(true), nil
			}).
			SetFunc("get_score", func(_ context.Context, _ []Value) (Value, error) {
				return FloatValue(7.5), nil
			})

		_, _ = filter.Execute(ctx)
	})
}

func FuzzIPListOperations(f *testing.F) {
	f.Add("10.0.0.1", "10.0.0.0/8")
	f.Add("192.168.1.100", "192.168.0.0/16")
	f.Add("172.16.5.1", "172.16.0.0/12")
	f.Add("8.8.8.8", "8.8.8.0/24")
	f.Add("2001:db8::1", "2001:db8::/32")
	f.Add("fe80::1", "fe80::/10")
	f.Add("invalid", "invalid/cidr")

	f.Fuzz(func(_ *testing.T, ipStr, cidrStr string) {
		filter, err := Compile(`ip not in $nets`, nil)
		if err != nil {
			return
		}

		ctx := NewExecutionContext().
			SetIPField("ip", ipStr).
			SetIPList("nets", []string{cidrStr})

		_, _ = filter.Execute(ctx)
	})
}

func FuzzFunctions(f *testing.F) {
	f.Add(`lower(name)`, "HELLO")
	f.Add(`upper(name)`, "hello")
	f.Add(`len(name)`, "test")
	f.Add(`starts_with(name, "he")`, "hello")
	f.Add(`ends_with(name, "lo")`, "hello")
	f.Add(`concat(name, "!")`, "hello")
	f.Add(`substring(name, 0, 3)`, "hello")
	f.Add(`split(name, ",")`, "a,b,c")
	f.Add(`url_decode(name)`, "hello%20world")
	f.Add(`cidr(ip, 24)`, "192.168.1.100")
	f.Add(`cidr6(ip, 64)`, "2001:db8::1")
	f.Add(`trim(name)`, "  hello  ")
	f.Add(`trim_left(name)`, "  hello")
	f.Add(`trim_right(name)`, "hello  ")
	f.Add(`replace(name, "a", "b")`, "aaa")
	f.Add(`regex_replace(name, "[0-9]+", "X")`, "abc123")
	f.Add(`regex_extract(name, "[0-9]+")`, "abc123")
	f.Add(`contains_word(name, "hello")`, "hello world")
	f.Add(`abs(n)`, "test")
	f.Add(`ceil(score)`, "test")
	f.Add(`floor(score)`, "test")
	f.Add(`round(score)`, "test")
	f.Add(`is_ipv4(ip)`, "192.168.1.1")
	f.Add(`is_ipv6(ip)`, "2001:db8::1")
	f.Add(`is_loopback(ip)`, "127.0.0.1")
	f.Add(`count(tags)`, "test")
	f.Add(`coalesce(name, "default")`, "test")
	f.Add(`exists(name)`, "test")
	f.Add(`any(tags[*] == "a")`, "test")
	f.Add(`all(tags[*] == "a")`, "test")
	f.Add(`has_key(data, "key")`, "test")
	f.Add(`has_value(tags, "a")`, "test")
	f.Add(`join(tags, ",")`, "test")
	f.Add(`contains_any(tags, other)`, "test")
	f.Add(`contains_all(tags, other)`, "test")
	f.Add(`len(intersection(tags, other))`, "test")
	f.Add(`len(union(tags, other))`, "test")
	f.Add(`len(difference(tags, other))`, "test")

	f.Fuzz(func(_ *testing.T, expression, value string) {
		filter, err := Compile(expression, nil)
		if err != nil {
			return
		}

		ctx := NewExecutionContext().
			SetStringField("name", value).
			SetIPField("ip", value).
			SetIntField("n", int64(len(value))).
			SetFloatField("score", 3.14).
			SetArrayField("tags", []string{"a", "b"}).
			SetArrayField("other", []string{"b", "c"}).
			SetMapField("data", map[string]string{"key": "val"})

		_, _ = filter.Execute(ctx)
	})
}

func FuzzSchemaValidation(f *testing.F) {
	f.Add(`name == "test"`)
	f.Add(`name contains "test"`)
	f.Add(`count > 5`)
	f.Add(`ip in $blocked`)
	f.Add(`tags[*] == "a"`)
	f.Add(`data["key"] == "val"`)
	f.Add(`lower(name) == "test"`)
	f.Add(`name not in {"a", "b"}`)
	f.Add(`unknown_field == "x"`)

	schema := NewSchema().
		AddField("name", TypeString).
		AddField("count", TypeInt).
		AddField("ip", TypeIP).
		AddField("tags", TypeArray).
		AddField("data", TypeMap)

	f.Fuzz(func(_ *testing.T, expression string) {
		_, _ = Compile(expression, schema)
	})
}

func FuzzExecuteWithTrace(f *testing.F) {
	f.Add(`name == "test" and status > 200`)
	f.Add(`lower(name) == "hello" or status in {200..299}`)
	f.Add(`not active`)
	f.Add(`tags[*] contains "a"`)
	f.Add(`ip in "10.0.0.0/8"`)

	f.Fuzz(func(t *testing.T, expression string) {
		filter, err := Compile(expression, nil)
		if err != nil {
			return
		}

		ctx := NewExecutionContext().
			EnableTrace().
			SetStringField("name", "test").
			SetIntField("status", 200).
			SetBoolField("active", true).
			SetIPField("ip", "10.0.0.1").
			SetArrayField("tags", []string{"a", "b"})

		r1, e1 := filter.Execute(ctx)
		trace := ctx.Trace()
		if e1 == nil && trace == nil {
			t.Fatal("trace should not be nil after execution")
		}

		// Result with tracing must match result without tracing
		ctx2 := NewExecutionContext().
			SetStringField("name", "test").
			SetIntField("status", 200).
			SetBoolField("active", true).
			SetIPField("ip", "10.0.0.1").
			SetArrayField("tags", []string{"a", "b"})

		r2, e2 := filter.Execute(ctx2)
		if (e1 == nil) != (e2 == nil) || r1 != r2 {
			t.Fatalf("trace should not affect result for %q", expression)
		}
	})
}

func FuzzExecuteWithCache(f *testing.F) {
	f.Add(`get_score(name) > 5.0`)
	f.Add(`get_score(name) > 5.0 and get_score(name) < 100.0`)
	f.Add(`lower(name) == "test"`)

	f.Fuzz(func(t *testing.T, expression string) {
		filter, err := Compile(expression, nil)
		if err != nil {
			return
		}

		ctx := NewExecutionContext().
			EnableCache().
			SetStringField("name", "test").
			SetIntField("status", 200).
			SetFunc("get_score", func(_ context.Context, _ []Value) (Value, error) {
				return FloatValue(7.5), nil
			})

		r1, e1 := filter.Execute(ctx)

		// Result with cache must match result without cache
		ctx2 := NewExecutionContext().
			SetStringField("name", "test").
			SetIntField("status", 200).
			SetFunc("get_score", func(_ context.Context, _ []Value) (Value, error) {
				return FloatValue(7.5), nil
			})

		r2, e2 := filter.Execute(ctx2)
		if (e1 == nil) != (e2 == nil) || r1 != r2 {
			t.Fatalf("cache should not affect result for %q", expression)
		}
	})
}

func TestFilterCompile(t *testing.T) {
	t.Run("compile without schema", func(t *testing.T) {
		filter, err := Compile(`http.host == "test"`, nil)
		assert.NoError(t, err)
		assert.NotNil(t, filter)
	})

	t.Run("parse error - invalid expression", func(t *testing.T) {
		_, err := Compile(`http.host ==`, nil)
		assert.Error(t, err)
	})

	t.Run("parse error - unclosed parenthesis", func(t *testing.T) {
		_, err := Compile(`(http.host == "test"`, nil)
		assert.Error(t, err)
	})

	t.Run("parse error - unclosed brace", func(t *testing.T) {
		_, err := Compile(`status in {200, 201`, nil)
		assert.Error(t, err)
	})

	t.Run("schema validation - unknown field", func(t *testing.T) {
		schema := NewSchema().
			AddField("http.host", TypeString)

		_, err := Compile(`http.unknown == "test"`, schema)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown field")
	})

	t.Run("schema validation - nested unknown field", func(t *testing.T) {
		schema := NewSchema().
			AddField("http.host", TypeString)

		_, err := Compile(`http.host == "test" and http.unknown == "test"`, schema)
		assert.Error(t, err)
	})

	t.Run("schema validation - unary expression", func(t *testing.T) {
		schema := NewSchema().
			AddField("http.host", TypeString)

		_, err := Compile(`not http.unknown`, schema)
		assert.Error(t, err)
	})

	t.Run("schema with initial fields map", func(t *testing.T) {
		fields := map[string]Type{
			"http.host":   TypeString,
			"http.status": TypeInt,
			"http.secure": TypeBool,
		}

		schema := NewSchema(fields)

		filter, err := Compile(`http.host == "example.com" and http.status == 200 and http.secure == true`, schema)
		assert.NoError(t, err)

		ctx := NewExecutionContext().
			SetStringField("http.host", "example.com").
			SetIntField("http.status", 200).
			SetBoolField("http.secure", true)

		result, err := filter.Execute(ctx)
		assert.NoError(t, err)
		assert.True(t, result)

		field, ok := schema.GetField("http.host")
		assert.True(t, ok)
		assert.Equal(t, "http.host", field.Name)
		assert.Equal(t, TypeString, field.Type)
	})

	t.Run("schema with multiple field maps", func(t *testing.T) {
		httpFields := map[string]Type{
			"http.host":   TypeString,
			"http.status": TypeInt,
		}

		ipFields := map[string]Type{
			"ip.src": TypeIP,
			"ip.dst": TypeIP,
		}

		schema := NewSchema(httpFields, ipFields)

		filter, err := Compile(`http.host == "example.com" and ip.src in "192.168.0.0/16"`, schema)
		assert.NoError(t, err)

		ctx := NewExecutionContext().
			SetStringField("http.host", "example.com").
			SetIPField("ip.src", "192.168.1.1")

		result, err := filter.Execute(ctx)
		assert.NoError(t, err)
		assert.True(t, result)

		httpField, ok := schema.GetField("http.host")
		assert.True(t, ok)
		assert.Equal(t, TypeString, httpField.Type)

		ipField, ok := schema.GetField("ip.src")
		assert.True(t, ok)
		assert.Equal(t, TypeIP, ipField.Type)
	})

	t.Run("schema validation with range in array", func(t *testing.T) {
		schema := NewSchema().
			AddField("status", TypeInt)

		filter, err := Compile(`status in {200..299}`, schema)
		assert.NoError(t, err)

		ctx := NewExecutionContext().
			SetIntField("status", 250)

		result, err := filter.Execute(ctx)
		assert.NoError(t, err)
		assert.True(t, result)
	})

	t.Run("schema validation - unpack expression", func(t *testing.T) {
		schema := NewSchema().
			AddField("tags", TypeArray)

		_, err := Compile(`tags[*] == "test"`, schema)
		assert.NoError(t, err)

		// Unknown field in unpack expression
		_, err = Compile(`unknown[*] == "test"`, schema)
		assert.Error(t, err)
	})

	t.Run("schema validation - list reference", func(t *testing.T) {
		schema := NewSchema().
			AddField("role", TypeString)

		// List references are validated at runtime, not compile time
		_, err := Compile(`role in $any_list`, schema)
		assert.NoError(t, err)
	})

	t.Run("all equal operator - non-array value rejected by schema", func(t *testing.T) {
		schema := NewSchema().
			AddField("name", TypeString)

		_, err := Compile(`name === "test"`, schema)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not valid for field type")
	})

	t.Run("any not equal operator - non-array value rejected by schema", func(t *testing.T) {
		schema := NewSchema().
			AddField("name", TypeString)

		_, err := Compile(`name !== "test"`, schema)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not valid for field type")
	})

	t.Run("wildcard with non-string types rejected by schema", func(t *testing.T) {
		schema := NewSchema().
			AddField("count", TypeInt)

		_, err := Compile(`count wildcard "123"`, schema)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not valid for field type")
	})

	t.Run("grouped expression", func(t *testing.T) {
		filter, err := Compile(`(http.status == 200 or http.status == 201) and http.host == "test"`, nil)
		assert.NoError(t, err)

		ctx := NewExecutionContext().
			SetIntField("http.status", 200).
			SetStringField("http.host", "test")

		result, err := filter.Execute(ctx)
		assert.NoError(t, err)
		assert.True(t, result)
	})

	t.Run("execute returns error on nil result", func(t *testing.T) {
		filter, err := Compile(`http.host`, nil)
		assert.NoError(t, err)

		ctx := NewExecutionContext()
		result, err := filter.Execute(ctx)
		assert.NoError(t, err)
		assert.False(t, result)
	})

	t.Run("raw string - no escape processing", func(t *testing.T) {
		schema := NewSchema().
			AddField("path", TypeString)

		filter, err := Compile(`path matches r"^C:\\Users\\.*"`, schema)
		assert.NoError(t, err)

		ctx := NewExecutionContext().
			SetStringField("path", `C:\Users\admin`)

		result, err := filter.Execute(ctx)
		assert.NoError(t, err)
		assert.True(t, result)
	})

	t.Run("raw string - regex pattern", func(t *testing.T) {
		schema := NewSchema().
			AddField("email", TypeString)

		filter, err := Compile(`email matches r"^\w+@\w+\.\w+$"`, schema)
		assert.NoError(t, err)

		ctx := NewExecutionContext().
			SetStringField("email", "user@example.com")

		result, err := filter.Execute(ctx)
		assert.NoError(t, err)
		assert.True(t, result)
	})

	t.Run("raw string - empty", func(t *testing.T) {
		filter, err := Compile(`field == r""`, nil)
		assert.NoError(t, err)

		ctx := NewExecutionContext().
			SetStringField("field", "")

		result, err := filter.Execute(ctx)
		assert.NoError(t, err)
		assert.True(t, result)
	})

	t.Run("lexer error - unterminated string", func(t *testing.T) {
		_, err := Compile(`name == "unterminated`, nil)
		assert.Error(t, err)
	})

	t.Run("lexer error - integer overflow", func(t *testing.T) {
		_, err := Compile(`x == 99999999999999999999999999999`, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "integer overflow")
	})

	t.Run("lexer error - unknown character", func(t *testing.T) {
		// A single @ at the start triggers lexer error
		_, err := Compile(`@`, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected character")
	})

	t.Run("unterminated raw string", func(t *testing.T) {
		_, err := Compile(`name == r"unterminated`, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unterminated raw string")
	})

	t.Run("trailing garbage - single ampersand", func(t *testing.T) {
		// "a & b" should fail - single & is not a valid operator
		_, err := Compile(`a & b`, nil)
		assert.Error(t, err)
	})

	t.Run("trailing garbage - unterminated string after valid expr", func(t *testing.T) {
		// Should fail with lexer error in trailing position
		_, err := Compile(`a "unterminated`, nil)
		assert.Error(t, err)
	})

	t.Run("trailing garbage - extra identifier", func(t *testing.T) {
		_, err := Compile(`a b`, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected trailing token")
	})

	t.Run("function result indexing is valid", func(t *testing.T) {
		// split(x, ",")[0] should be valid - indexing function result
		filter, err := Compile(`split(name, ",")[0] == "a"`, nil)
		assert.NoError(t, err)

		ctx := NewExecutionContext().SetStringField("name", "a,b,c")
		result, err := filter.Execute(ctx)
		assert.NoError(t, err)
		assert.True(t, result)
	})

	t.Run("schema validates range expr with unknown field", func(t *testing.T) {
		schema := NewSchema().AddField("x", TypeInt)
		_, err := Compile(`x in {unknown_start..10}`, schema)
		assert.Error(t, err)
	})

	t.Run("schema validates range expr end with unknown field", func(t *testing.T) {
		schema := NewSchema().AddField("x", TypeInt)
		_, err := Compile(`x in {1..unknown_end}`, schema)
		assert.Error(t, err)
	})

	t.Run("schema validates array elements with unknown field", func(t *testing.T) {
		schema := NewSchema().AddField("x", TypeInt)
		_, err := Compile(`x in {unknown, 1}`, schema)
		assert.Error(t, err)
	})

	t.Run("lexer unterminated string with escape", func(t *testing.T) {
		lexer := NewLexer(`"test\`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenError, tok.Type)
	})

	t.Run("lexer CIDR in number context", func(t *testing.T) {
		lexer := NewLexer(`192.168.0.0/24`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenCIDR, tok.Type)
	})

	t.Run("lexer IPv6 CIDR", func(t *testing.T) {
		lexer := NewLexer(`2001:db8::/32`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenCIDR, tok.Type)
	})

	t.Run("lexer negative number overflow", func(t *testing.T) {
		lexer := NewLexer(`-99999999999999999999999`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenError, tok.Type)
	})
}

func TestFilterHash(t *testing.T) {
	t.Run("identical expressions produce same hash", func(t *testing.T) {
		f1, err := Compile(`name == "test"`, nil)
		require.NoError(t, err)
		f2, err := Compile(`name == "test"`, nil)
		require.NoError(t, err)

		assert.Equal(t, f1.Hash(), f2.Hash())
		assert.Len(t, f1.Hash(), 32) // 128-bit FNV = 16 bytes = 32 hex chars
	})

	t.Run("extra whitespace ignored", func(t *testing.T) {
		f1, err := Compile(`name=="test"`, nil)
		require.NoError(t, err)
		f2, err := Compile(`name   ==   "test"`, nil)
		require.NoError(t, err)

		assert.Equal(t, f1.Hash(), f2.Hash())
	})

	t.Run("tabs and newlines ignored", func(t *testing.T) {
		f1, err := Compile(`name == "test"`, nil)
		require.NoError(t, err)
		f2, err := Compile("name\t==\n\"test\"", nil)
		require.NoError(t, err)

		assert.Equal(t, f1.Hash(), f2.Hash())
	})

	t.Run("operator aliases produce same hash", func(t *testing.T) {
		f1, err := Compile(`a and b`, nil)
		require.NoError(t, err)
		f2, err := Compile(`a && b`, nil)
		require.NoError(t, err)

		assert.Equal(t, f1.Hash(), f2.Hash())
	})

	t.Run("or alias", func(t *testing.T) {
		f1, err := Compile(`a or b`, nil)
		require.NoError(t, err)
		f2, err := Compile(`a || b`, nil)
		require.NoError(t, err)

		assert.Equal(t, f1.Hash(), f2.Hash())
	})

	t.Run("not alias", func(t *testing.T) {
		f1, err := Compile(`not a`, nil)
		require.NoError(t, err)
		f2, err := Compile(`! a`, nil)
		require.NoError(t, err)

		assert.Equal(t, f1.Hash(), f2.Hash())
	})

	t.Run("xor alias", func(t *testing.T) {
		f1, err := Compile(`a xor b`, nil)
		require.NoError(t, err)
		f2, err := Compile(`a ^^ b`, nil)
		require.NoError(t, err)

		assert.Equal(t, f1.Hash(), f2.Hash())
	})

	t.Run("matches alias", func(t *testing.T) {
		f1, err := Compile(`name matches "^test"`, nil)
		require.NoError(t, err)
		f2, err := Compile(`name ~ "^test"`, nil)
		require.NoError(t, err)

		assert.Equal(t, f1.Hash(), f2.Hash())
	})

	t.Run("different expressions produce different hash", func(t *testing.T) {
		f1, err := Compile(`name == "test"`, nil)
		require.NoError(t, err)
		f2, err := Compile(`name == "other"`, nil)
		require.NoError(t, err)

		assert.NotEqual(t, f1.Hash(), f2.Hash())
	})

	t.Run("different operators produce different hash", func(t *testing.T) {
		f1, err := Compile(`x == 1`, nil)
		require.NoError(t, err)
		f2, err := Compile(`x != 1`, nil)
		require.NoError(t, err)

		assert.NotEqual(t, f1.Hash(), f2.Hash())
	})

	t.Run("different fields produce different hash", func(t *testing.T) {
		f1, err := Compile(`name == "test"`, nil)
		require.NoError(t, err)
		f2, err := Compile(`host == "test"`, nil)
		require.NoError(t, err)

		assert.NotEqual(t, f1.Hash(), f2.Hash())
	})

	t.Run("complex expression with aliases", func(t *testing.T) {
		f1, err := Compile(`name == "test" and status >= 400 or not active`, nil)
		require.NoError(t, err)
		f2, err := Compile(`name == "test" && status >= 400 || ! active`, nil)
		require.NoError(t, err)

		assert.Equal(t, f1.Hash(), f2.Hash())
	})
}

func TestFilterHashStable(t *testing.T) {
	expected := map[string]string{
		`name == "test"`:                       "c2889f4a7ccca7ff44f3d705ede3a9d2",
		`status >= 400`:                        "4d0d67a73f751e14aacca5bb3502c749",
		`a and b`:                              "8d37d268ba0d433e9085647d4515db7e",
		`a or b`:                               "8cc4a49ad50d433e83bc73aaacd7db57",
		`not a`:                                "c8194b89c2659af17cda0cbf1bbcba23",
		`a xor b`:                              "ac37ffb8bf0d433e7b23fe3a6bcf6e85",
		`name matches "^test"`:                 "be80d3f9b61f58a7a706ad74e2763340",
		`ip in "10.0.0.0/8"`:                   "a90ffc4474bd91cc1e8539ee6c34dbee",
		`ip not in $blocked`:                   "39d433affcd7e7a207116d845e2a9a90",
		`tags[*] == "prod"`:                    "ff16911f0e06efe44d5eb8288add6bef",
		`lower(name) == "admin"`:               "ee93a7331fa511f603da995bab8a3ad5",
		`cidr(ip, 24) == 10.0.0.0/24`:          "7b88e911d95eb856e3d2bcc7a7da10a5",
		`x in {1..100}`:                        "22fa482e0fc63ac8541c43d27a475328",
		`name not contains "admin"`:            "4074d01cb7420c78dea8b032e8307b1f",
		`data["key"] == "val"`:                 "3368e39bf8c533c6cff6b3a987a43ef5",
		`(a == 1 or b == 2) and c == 3`:        "6acb7ec503e4dfdf1a5391f787845042",
		`status != 404`:                        "6d9bc0ab5b3bf4228039a8eadeb75443",
		`status < 400`:                         "ec985621acad63b4d0e28d28ef703dc2",
		`status > 200`:                         "f8d47a95382a47c2c750b627043918e8",
		`status <= 299`:                        "e6ea4b85031cc59ab574c68a76d6adcc",
		`tags === "prod"`:                      "c3a1db339fdfa8125fd143a1185e67bf",
		`tags !== "test"`:                      "593f34ce1e94d3b21d6cc25e5c50077d",
		`name contains "admin"`:                "243b9170faeaf7ac31a25c2ee776bfb0",
		`port in {80, 443, 8080}`:              "f9baff96461e48a2292c81ecb8520600",
		`role in $admin_roles`:                 "6643d2ca15b55649ea01cb62b3f4ad6d",
		`host wildcard "*.example.com"`:        "af143d5c66c07d190f43f3998598dfa4",
		`host strict wildcard "*.Example.com"`: "941fd5732aa132aa04e497d956eed7af",
		`path matches r"^/api/v[0-9]+"`:        "8b79de183f8d344305dfa820ed2314ef",
		`score > 3.14`:                         "b01644f6aead54ea1558253ea46db124",
		`enabled == true`:                      "aa55337fb66b56325c759717370cac60",
		`ip == 192.168.1.1`:                    "d0c382637d7831c651612e82e8d3f6b1",
		`x + 1 > 10`:                           "4c2ea504fee33c7ae110aea28160f5b1",
		`x - 5 < 0`:                            "b9a40d52bec1922b4a5b68f1741f68f3",
		`x * 2 == 100`:                         "748077c322afebfdbf4575f3836ada05",
		`x / 3 > 0`:                            "70693f4736d33909641a86ff68998957",
		`x % 2 == 0`:                           "e658a427200d9235b6fbf01f4a80da1f",
		`tags[0] == "first"`:                   "c0465432e1d65c7a2a8c551480a9b58a",
		`a == b`:                               "db5cf383420d433e6e5b1939cbe80c66",
		`x == -10`:                             "dc1744c38a0d433e6e5b19731ce00392",
		`starts_with(name, "admin")`:           "b6f4729507c3bad94586755fcad80ef1",
		`x + 1.5 > 3.0`:                        "405f0ebdedb1357eae2ff1d1481be5fe",
		`x not in {1, 2, 3}`:                   "e52fdcad97194438d53ba5022bd83252",
		`ip in 10.0.0.0/8`:                     "bc10572ddbc4218bc9c50823a7c6f3b5",
		`ts >= 2026-03-19T10:00:00Z`:           "138d0b1f2ddd2ee9c9daba0dc96a7340",
		`ttl == 30m`:                           "cbe46531d88f2f459257de96604a33b0",
		`ts + 1h >= 2026-03-19T11:00:00Z`:      "8749a5932e80196b6513f41b91ea41ca",
		`ttl in {1h..3h}`:                      "9677c65b457238b09861f8ad134277e6",
		`ts <= now()`:                          "375ec8d2b256a223cd933c1dc4ba016c",
		`ts >= now() - 1h`:                     "0c62988079706816f2a11eb7a7030410",
		`ttl * 2 > 1h`:                         "a855f47427619c6302e5cfeb41ad1330",
		`ttl / 30m == 2`:                       "655bd5c241b2b50b07e60f32ae3c64e9",
		`ttl % 1h == 30m`:                      "e2214246f9c1a21c4e7aaedc2297983b",
		`ttl + 30m > 1h`:                       "50dfd33c3a29f764c385f8a3dbadb2b1",
		`ts - 1h >= 2026-03-19T09:00:00Z`:      "aa5dea18632ce66bd04f5d960ecc3442",
		`ts in {2026-03-19T00:00:00Z..2026-03-20T00:00:00Z}`: "526f788a3e9ae7786ecdcb1464371a3c",
		`ttl >= 2d4h30m15s`:            "1f8d1d4dd35ebbe8b3428ed3a93252cc",
		`ts == "2026-03-19T10:00:00Z"`: "fee4391c38705084283ba6cfaa8d4c7e",
	}

	for expr, wantHash := range expected {
		t.Run(expr, func(t *testing.T) {
			f, err := Compile(expr, nil)
			require.NoError(t, err)
			assert.Equal(t, wantHash, f.Hash())
		})
	}
}

func TestRuleMeta(t *testing.T) {
	t.Run("set and get meta", func(t *testing.T) {
		filter, _ := Compile(`name == "test"`, nil)
		filter.SetMeta(RuleMeta{
			ID:   "WAF-1001",
			Tags: map[string]string{"severity": "high", "category": "xss"},
		})

		meta := filter.Meta()
		assert.Equal(t, "WAF-1001", meta.ID)
		assert.Equal(t, "high", meta.Tags["severity"])
		assert.Equal(t, "xss", meta.Tags["category"])
	})

	t.Run("default meta is empty", func(t *testing.T) {
		filter, _ := Compile(`name == "test"`, nil)
		meta := filter.Meta()
		assert.Empty(t, meta.ID)
		assert.Nil(t, meta.Tags)
	})

	t.Run("chaining", func(t *testing.T) {
		filter, _ := Compile(`name == "test"`, nil)
		f := filter.SetMeta(RuleMeta{ID: "R1"})
		assert.Equal(t, "R1", f.Meta().ID)
	})

	t.Run("SetMeta defensively copies tags", func(t *testing.T) {
		filter, _ := Compile(`name == "test"`, nil)
		tags := map[string]string{"env": "prod"}
		filter.SetMeta(RuleMeta{ID: "R1", Tags: tags})
		tags["env"] = "staging"
		assert.Equal(t, "prod", filter.Meta().Tags["env"])
	})

	t.Run("Meta returns defensive copy of tags", func(t *testing.T) {
		filter, _ := Compile(`name == "test"`, nil)
		filter.SetMeta(RuleMeta{ID: "R1", Tags: map[string]string{"env": "prod"}})
		meta := filter.Meta()
		meta.Tags["env"] = "staging"
		assert.Equal(t, "prod", filter.Meta().Tags["env"])
	})
}

func TestExecuteNilContext(t *testing.T) {
	filter, err := Compile(`name == "test"`, nil)
	require.NoError(t, err)

	result, err := filter.Execute(nil)
	assert.NoError(t, err)
	assert.False(t, result)
}

func TestEvaluateInnerEdgeCases(t *testing.T) {
	t.Run("standalone range expr", func(t *testing.T) {
		f := &Filter{expr: &RangeExpr{
			Start: &LiteralExpr{Value: IntValue(1)},
			End:   &LiteralExpr{Value: IntValue(5)},
		}}
		ctx := NewExecutionContext()
		result, err := f.Execute(ctx)
		assert.NoError(t, err)
		assert.True(t, result) // ArrayValue is truthy
	})

	t.Run("unknown expression type", func(t *testing.T) {
		f := &Filter{expr: nil}
		ctx := NewExecutionContext()
		result, err := f.Execute(ctx)
		assert.NoError(t, err)
		assert.False(t, result) // nil result
	})
}

// --- Property-based testing helpers ---

func randomAST(rng *rand.Rand, depth int) string {
	if depth <= 0 {
		return randomLeaf(rng)
	}

	switch rng.IntN(10) {
	case 0:
		ops := []string{"and", "or", "xor"}
		return fmt.Sprintf("%s %s %s", randomAST(rng, depth-1), ops[rng.IntN(len(ops))], randomAST(rng, depth-1))
	case 1:
		ops := []string{"==", "!=", "<", ">", "<=", ">="}
		return fmt.Sprintf("%s %s %s", randomFieldOrFunc(rng), ops[rng.IntN(len(ops))], randomLiteral(rng))
	case 2:
		return fmt.Sprintf("%s contains \"%s\"", randomStringField(rng), randomWord(rng))
	case 3:
		return fmt.Sprintf("%s in %s", randomFieldOrFunc(rng), randomSet(rng))
	case 4:
		return fmt.Sprintf("not %s", randomAST(rng, depth-1))
	case 5:
		return fmt.Sprintf("(%s)", randomAST(rng, depth-1))
	case 6:
		ops := []string{"+", "-", "*"}
		return fmt.Sprintf("%s %s %s > 0", randomNumField(rng), ops[rng.IntN(len(ops))], randomInt(rng))
	case 7:
		return fmt.Sprintf("ts %s %s", randomCmpOp(rng), randomTimestamp(rng))
	case 8:
		return fmt.Sprintf("ttl %s %s", randomCmpOp(rng), randomDuration(rng))
	default:
		return randomField(rng)
	}
}

func randomLeaf(rng *rand.Rand) string {
	switch rng.IntN(5) {
	case 0:
		return fmt.Sprintf("%s == \"%s\"", randomField(rng), randomWord(rng))
	case 1:
		return fmt.Sprintf("%s > %s", randomNumField(rng), randomInt(rng))
	case 2:
		return randomField(rng)
	case 3:
		return fmt.Sprintf("ts >= %s", randomTimestamp(rng))
	default:
		return fmt.Sprintf("ttl > %s", randomDuration(rng))
	}
}

func randomField(rng *rand.Rand) string {
	fields := []string{"name", "host", "path", "method", "ua", "country", "region"}
	return fields[rng.IntN(len(fields))]
}

func randomStringField(rng *rand.Rand) string {
	fields := []string{"name", "host", "path", "method", "ua"}
	return fields[rng.IntN(len(fields))]
}

func randomNumField(rng *rand.Rand) string {
	fields := []string{"status", "score", "port", "count"}
	return fields[rng.IntN(len(fields))]
}

func randomFieldOrFunc(rng *rand.Rand) string {
	if rng.IntN(4) == 0 {
		funcs := []string{"lower(name)", "upper(host)", "len(path)"}
		return funcs[rng.IntN(len(funcs))]
	}
	return randomField(rng)
}

func randomLiteral(rng *rand.Rand) string {
	switch rng.IntN(5) {
	case 0:
		return fmt.Sprintf("\"%s\"", randomWord(rng))
	case 1:
		return randomInt(rng)
	case 2:
		return fmt.Sprintf("%d.%d", rng.IntN(100), rng.IntN(100))
	case 3:
		return randomTimestamp(rng)
	default:
		return randomDuration(rng)
	}
}

func randomWord(rng *rand.Rand) string {
	words := []string{"test", "admin", "api", "prod", "dev", "user", "root", "login"}
	return words[rng.IntN(len(words))]
}

func randomInt(rng *rand.Rand) string {
	return fmt.Sprintf("%d", rng.IntN(1000))
}

func randomCmpOp(rng *rand.Rand) string {
	ops := []string{"==", "!=", "<", ">", "<=", ">="}
	return ops[rng.IntN(len(ops))]
}

func randomSet(rng *rand.Rand) string {
	n := 2 + rng.IntN(5)
	parts := make([]string, n)
	for i := range n {
		parts[i] = randomInt(rng)
	}
	return fmt.Sprintf("{%s}", strings.Join(parts, ", "))
}

func randomTimestamp(rng *rand.Rand) string {
	return fmt.Sprintf("%04d-%02d-%02dT%02d:00:00Z", 2024+rng.IntN(4), 1+rng.IntN(12), 1+rng.IntN(28), rng.IntN(24))
}

func randomDuration(rng *rand.Rand) string {
	units := []string{"s", "m", "h", "d"}
	return fmt.Sprintf("%d%s", 1+rng.IntN(59), units[rng.IntN(len(units))])
}

func propertySchema() *Schema {
	return NewSchema().
		AddField("name", TypeString).
		AddField("host", TypeString).
		AddField("path", TypeString).
		AddField("method", TypeString).
		AddField("ua", TypeString).
		AddField("country", TypeString).
		AddField("region", TypeString).
		AddField("status", TypeInt).
		AddField("score", TypeInt).
		AddField("port", TypeInt).
		AddField("count", TypeInt).
		AddField("ts", TypeTime).
		AddField("ttl", TypeDuration)
}

func propertyContext(rng *rand.Rand) *ExecutionContext {
	fixed := time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)
	return NewExecutionContext().
		SetStringField("name", randomWord(rng)).
		SetStringField("host", fmt.Sprintf("%s.example.com", randomWord(rng))).
		SetStringField("path", fmt.Sprintf("/%s", randomWord(rng))).
		SetStringField("method", "GET").
		SetStringField("ua", "Mozilla/5.0").
		SetStringField("country", "US").
		SetStringField("region", "us-west-2").
		SetIntField("status", int64(200+rng.IntN(400))).
		SetIntField("score", int64(rng.IntN(100))).
		SetIntField("port", int64(80+rng.IntN(9000))).
		SetIntField("count", int64(rng.IntN(1000))).
		SetTimeField("ts", fixed.Add(-time.Duration(rng.IntN(86400))*time.Second)).
		SetDurationField("ttl", time.Duration(rng.IntN(86400))*time.Second).
		WithNow(func() time.Time { return fixed })
}

// --- Property-based tests ---

func TestPropertyCompileDeterminism(t *testing.T) {
	schema := propertySchema()
	rng := rand.New(rand.NewPCG(42, 0))

	for range 1000 {
		expr := randomAST(rng, 2+rng.IntN(3))
		f1, err1 := Compile(expr, schema)
		f2, err2 := Compile(expr, schema)
		if (err1 == nil) != (err2 == nil) {
			t.Fatalf("non-deterministic compile for %q", expr)
		}
		if err1 != nil {
			continue
		}
		assert.Equal(t, f1.Hash(), f2.Hash(), "hash mismatch for %q", expr)
	}
}

func TestPropertyExecuteDeterminism(t *testing.T) {
	schema := propertySchema()
	rng := rand.New(rand.NewPCG(42, 0))

	for range 1000 {
		expr := randomAST(rng, 2+rng.IntN(3))
		f, err := Compile(expr, schema)
		if err != nil {
			continue
		}
		ctx := propertyContext(rng)
		r1, e1 := f.Execute(ctx)
		r2, e2 := f.Execute(ctx)
		if (e1 == nil) != (e2 == nil) || r1 != r2 {
			t.Fatalf("non-deterministic execute for %q", expr)
		}
	}
}

func TestPropertyMarshalRoundtrip(t *testing.T) {
	schema := propertySchema()
	rng := rand.New(rand.NewPCG(42, 0))

	for range 1000 {
		expr := randomAST(rng, 2+rng.IntN(3))
		f, err := Compile(expr, schema)
		if err != nil {
			continue
		}
		data, err := f.MarshalBinary()
		require.NoError(t, err, "marshal failed for %q", expr)

		restored := &Filter{}
		require.NoError(t, restored.UnmarshalBinary(data), "unmarshal failed for %q", expr)
		assert.Equal(t, f.Hash(), restored.Hash(), "hash mismatch after roundtrip for %q", expr)

		ctx := propertyContext(rng)
		r1, e1 := f.Execute(ctx)
		r2, e2 := restored.Execute(ctx)
		if (e1 == nil) != (e2 == nil) || r1 != r2 {
			t.Fatalf("execution mismatch after roundtrip for %q", expr)
		}
	}
}

func TestPropertySchemaValidationConsistency(t *testing.T) {
	schema := propertySchema()
	rng := rand.New(rand.NewPCG(42, 0))

	for range 1000 {
		expr := randomAST(rng, 2+rng.IntN(3))
		f, err := Compile(expr, schema)
		if err != nil {
			continue
		}
		fNoSchema, err := Compile(expr, nil)
		require.NoError(t, err, "compiles with schema but not without for %q", expr)

		ctx := propertyContext(rng)
		r1, _ := f.Execute(ctx)
		r2, _ := fNoSchema.Execute(ctx)
		assert.Equal(t, r1, r2, "schema/no-schema execution mismatch for %q", expr)
	}
}

func TestPropertyNilContextSafe(_ *testing.T) {
	schema := propertySchema()
	rng := rand.New(rand.NewPCG(42, 0))

	for range 1000 {
		expr := randomAST(rng, 2+rng.IntN(3))
		f, err := Compile(expr, schema)
		if err != nil {
			continue
		}
		_, _ = f.Execute(nil)
		_, _ = f.Execute(NewExecutionContext())
	}
}

func TestPropertyBoolResult(t *testing.T) {
	schema := propertySchema()
	rng := rand.New(rand.NewPCG(42, 0))

	for range 1000 {
		expr := randomAST(rng, 2+rng.IntN(3))
		f, err := Compile(expr, schema)
		if err != nil {
			continue
		}
		ctx := propertyContext(rng)
		result, err := f.Execute(ctx)
		if err != nil {
			continue
		}
		assert.IsType(t, true, result, "Execute returned non-bool for %q", expr)
	}
}

func TestPropertyNotInvertsTruthiness(t *testing.T) {
	schema := propertySchema()
	rng := rand.New(rand.NewPCG(42, 0))

	for range 500 {
		inner := randomAST(rng, 1+rng.IntN(2))
		fPos, errPos := Compile(inner, schema)
		fNeg, errNeg := Compile(fmt.Sprintf("not (%s)", inner), schema)
		if errPos != nil || errNeg != nil {
			continue
		}
		ctx := propertyContext(rng)
		rPos, ePos := fPos.Execute(ctx)
		rNeg, eNeg := fNeg.Execute(ctx)
		if ePos != nil || eNeg != nil {
			continue
		}
		assert.NotEqual(t, rPos, rNeg, "not(...) did not invert for %q", inner)
	}
}

func TestPropertyIdempotentDoubleNegation(t *testing.T) {
	schema := propertySchema()
	rng := rand.New(rand.NewPCG(42, 0))

	for range 500 {
		inner := randomAST(rng, 1+rng.IntN(2))
		fSingle, errS := Compile(inner, schema)
		fDouble, errD := Compile(fmt.Sprintf("not not (%s)", inner), schema)
		if errS != nil || errD != nil {
			continue
		}
		ctx := propertyContext(rng)
		rSingle, eSingle := fSingle.Execute(ctx)
		rDouble, eDouble := fDouble.Execute(ctx)
		if eSingle != nil || eDouble != nil {
			continue
		}
		assert.Equal(t, rSingle, rDouble, "not not X != X for %q", inner)
	}
}

func TestPropertyLogicalOperators(t *testing.T) {
	schema := propertySchema()

	tests := []struct {
		name string
		op   string
		eval func(a, b bool) bool
	}{
		{"or", "or", func(a, b bool) bool { return a || b }},
		{"and", "and", func(a, b bool) bool { return a && b }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rng := rand.New(rand.NewPCG(42, 0))
			for range 500 {
				left := randomAST(rng, 1+rng.IntN(2))
				right := randomAST(rng, 1+rng.IntN(2))
				combined := fmt.Sprintf("(%s) %s (%s)", left, tt.op, right)

				fC, errC := Compile(combined, schema)
				fL, errL := Compile(left, schema)
				fR, errR := Compile(right, schema)
				if errC != nil || errL != nil || errR != nil {
					continue
				}
				ctx := propertyContext(rng)
				rC, eC := fC.Execute(ctx)
				rL, eL := fL.Execute(ctx)
				rR, eR := fR.Execute(ctx)
				if eC != nil || eL != nil || eR != nil {
					continue
				}
				expected := tt.eval(rL, rR)
				assert.Equal(t, expected, rC,
					"%s semantics broken for (%s) %s (%s)", tt.op, left, tt.op, right)
			}
		})
	}
}

func TestPropertyEqNeComplement(t *testing.T) {
	schema := propertySchema()
	rng := rand.New(rand.NewPCG(42, 0))

	for range 500 {
		field := randomField(rng)
		word := randomWord(rng)
		fEq, errEq := Compile(fmt.Sprintf("%s == \"%s\"", field, word), schema)
		fNe, errNe := Compile(fmt.Sprintf("%s != \"%s\"", field, word), schema)
		if errEq != nil || errNe != nil {
			continue
		}
		ctx := propertyContext(rng)
		rEq, eEq := fEq.Execute(ctx)
		rNe, eNe := fNe.Execute(ctx)
		if eEq != nil || eNe != nil {
			continue
		}
		assert.NotEqual(t, rEq, rNe, "== and != same result for %s \"%s\"", field, word)
	}
}

func TestPropertyIntRangeContainment(t *testing.T) {
	rng := rand.New(rand.NewPCG(42, 0))

	for range 500 {
		lo := rng.IntN(500)
		hi := lo + 1 + rng.IntN(500)
		val := int64(lo + rng.IntN(hi-lo+1))

		f, err := Compile(fmt.Sprintf("x in {%d..%d}", lo, hi), nil)
		require.NoError(t, err)

		ctx := NewExecutionContext().SetIntField("x", val)
		result, err := f.Execute(ctx)
		require.NoError(t, err)
		assert.True(t, result, "x=%d should be in {%d..%d}", val, lo, hi)

		outside := int64(hi + 1 + rng.IntN(100))
		ctx2 := NewExecutionContext().SetIntField("x", outside)
		result2, _ := f.Execute(ctx2)
		assert.False(t, result2, "x=%d should NOT be in {%d..%d}", outside, lo, hi)
	}
}

func TestPropertyTimeRangeContainment(t *testing.T) {
	rng := rand.New(rand.NewPCG(42, 0))
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for range 200 {
		startOff := rng.IntN(365 * 24)
		endOff := startOff + 1 + rng.IntN(24*30)
		start := base.Add(time.Duration(startOff) * time.Hour)
		end := base.Add(time.Duration(endOff) * time.Hour)
		mid := start.Add(end.Sub(start) / 2)

		f, err := Compile(fmt.Sprintf("ts in {%s..%s}", start.Format(time.RFC3339), end.Format(time.RFC3339)), nil)
		if err != nil {
			continue
		}
		ctx := NewExecutionContext().SetTimeField("ts", mid)
		result, _ := f.Execute(ctx)
		assert.True(t, result, "ts=%s should be in {%s..%s}",
			mid.Format(time.RFC3339), start.Format(time.RFC3339), end.Format(time.RFC3339))
	}
}

func TestFilterConcurrency(t *testing.T) {
	t.Run("concurrent meta access", func(t *testing.T) {
		schema := NewSchema().AddField("x", TypeInt)
		filter, err := Compile(`x == 1`, schema)
		require.NoError(t, err)

		var wg sync.WaitGroup
		for i := range 100 {
			wg.Add(2)
			go func(n int) {
				defer wg.Done()
				filter.SetMeta(RuleMeta{
					ID:   fmt.Sprintf("rule-%d", n),
					Tags: map[string]string{"i": fmt.Sprintf("%d", n)},
				})
			}(i)
			go func() {
				defer wg.Done()
				m := filter.Meta()
				_ = m.ID
			}()
		}
		wg.Wait()
	})

	t.Run("concurrent hash and execute", func(t *testing.T) {
		schema := NewSchema().AddField("x", TypeInt)
		filter, err := Compile(`x > 0`, schema)
		require.NoError(t, err)

		ctx := NewExecutionContext().SetIntField("x", 42)

		var wg sync.WaitGroup
		for range 100 {
			wg.Add(2)
			go func() {
				defer wg.Done()
				h := filter.Hash()
				assert.NotEmpty(t, h)
			}()
			go func() {
				defer wg.Done()
				result, err := filter.Execute(ctx)
				assert.NoError(t, err)
				assert.True(t, result)
			}()
		}
		wg.Wait()
	})

	t.Run("concurrent marshal and execute", func(t *testing.T) {
		schema := NewSchema().AddField("x", TypeInt)
		filter, err := Compile(`x == 1`, schema)
		require.NoError(t, err)

		ctx := NewExecutionContext().SetIntField("x", 1)

		var wg sync.WaitGroup
		for range 100 {
			wg.Add(2)
			go func() {
				defer wg.Done()
				data, err := filter.MarshalBinary()
				assert.NoError(t, err)
				assert.NotEmpty(t, data)
			}()
			go func() {
				defer wg.Done()
				result, err := filter.Execute(ctx)
				assert.NoError(t, err)
				assert.True(t, result)
			}()
		}
		wg.Wait()
	})
}

func TestEvalDepthLimit(t *testing.T) {
	var sb strings.Builder
	for range maxEvalDepth + 10 {
		sb.WriteString("(")
	}
	sb.WriteString("x == 1")
	for range maxEvalDepth + 10 {
		sb.WriteString(") and x == 1")
	}
	expr := sb.String()

	filter, err := Compile(expr, nil)
	require.NoError(t, err)

	ctx := NewExecutionContext().SetIntField("x", 1)
	_, err = filter.Execute(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "maximum evaluation depth")
}
