# Wirefilter

Wirefilter is a filtering expression language and execution engine for Go.
It allows you to compile and evaluate filter expressions against runtime data,
inspired by Cloudflare's Wirefilter.

## Features

- Logical operators: `and`, `or`, `not`, `xor`, `&&`, `||`, `!`, `^^` (short-circuit evaluation)
- Comparison operators: `==`, `!=`, `<`, `>`, `<=`, `>=`
- Arithmetic operators: `+`, `-`, `*`, `/`, `%` (with standard precedence)
- Array operators: `===` (all equal), `!==` (any not equal)
- Membership operators: `in`, `contains`, `matches` (`~`)
- Negated membership: `not in`, `not contains`
- Wildcard matching: `wildcard` (case-insensitive), `strict wildcard` (case-sensitive)
- Field presence/absence checking
- Field-to-field comparisons
- Range expressions: `{1..10}`
- Multiple data types: string, int, float, bool, IP, CIDR, bytes, arrays, maps, time, duration
- Map field access with bracket notation: `map["key"]`
- Array index access: `tags[0]` (negative/out-of-bounds return nil)
- Array unpack operations: `tags[*] == "value"` (ANY semantics)
- Raw strings: `r"..."` (no escape processing)
- Custom lists: `$list_name` for external list references
- Lookup tables: `$table_name[field]` for key-value lookups
- Native time and duration types with temporal arithmetic and `now()` built-in
- Negated duration literals: `-30m`, `-2d4h`
- Type coercion: IP/CIDR/Time equality with string values
- Built-in functions: `lower()`, `upper()`, `len()`, `starts_with()`, `now()`, and 30+ more
- User-defined functions with compile-time signature validation
- IP/CIDR matching for IPv4 and IPv6
- Regular expression matching (can be disabled via `DisableRegex()`)
- Schema validation for field references and operator-type compatibility
- Compile-time type validation for typed arrays, maps, and UDF return types
- Function availability control (allowlist/blocklist modes)
- Expression complexity limits (max depth and node count)
- Expression hashing for deduplication (`Hash()`)
- Rule metadata (ID, tags)
- Execution timeout via `context.Context`
- Injectable clock via `WithNow` for deterministic testing
- Expression evaluation tracing for debugging
- Configurable result caching for UDF calls
- Context export for audit logging (`Export()`, `ExportLists()`)
- Binary serialization for pre-compiled filter storage and fast loading
- Thread-safe filter execution (concurrent `Execute()` calls)
- Multi-error recovery in parser (reports multiple errors in a single pass)

## Installation

```bash
go get github.com/vitalvas/wirefilter
```

## Quick Start

```go
package main

import (
    "fmt"
    "log"

    "github.com/vitalvas/wirefilter"
)

func main() {
    schema := wirefilter.NewSchema().
        AddField("http.host", wirefilter.TypeString).
        AddField("http.status", wirefilter.TypeInt)

    filter, err := wirefilter.Compile(
        `http.host == "example.com" and http.status >= 400`, schema)
    if err != nil {
        log.Fatal(err)
    }

    ctx := wirefilter.NewExecutionContext().
        SetStringField("http.host", "example.com").
        SetIntField("http.status", 500)

    result, err := filter.Execute(ctx)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(result)
}
```

## Language Syntax

### Basic Comparisons

```go
http.status == 200
http.status != 404
http.status > 400
http.status >= 500
```

### String Operations

```go
http.host == "example.com"
http.path contains "/api"
http.user_agent matches "^Mozilla.*"
http.user_agent ~ "^Mozilla.*"              // ~ is alias for matches
```

### Raw Strings

Raw strings use the `r"..."` syntax and do not process escape sequences.
Useful for regex patterns and file paths:

```go
// Regular string (escape sequences processed)
path matches "^C:\\Users\\.*"               // backslashes need escaping

// Raw string (no escape processing)
path matches r"^C:\Users\.*"                // backslashes preserved as-is
email matches r"^\w+@\w+\.\w+$"             // cleaner regex patterns
```

### Wildcard Matching

Glob-style pattern matching with `*` (any chars) and `?` (single char):

```go
http.host wildcard "*.example.com"          // case-insensitive
http.host wildcard "api?.example.com"       // ? matches single char
http.host strict wildcard "*.Example.com"   // case-sensitive
```

Examples:

- `"www.example.com" wildcard "*.example.com"` - true
- `"WWW.EXAMPLE.COM" wildcard "*.example.com"` - true (case-insensitive)
- `"WWW.Example.com" strict wildcard "*.Example.com"` - true
- `"www.example.com" strict wildcard "*.Example.com"` - false

### Combining Conditions

```go
http.host == "example.com" and http.status == 200
http.host == "example.com" && http.status == 200   // && is alias for and
http.status == 404 or http.status == 500
http.status == 404 || http.status == 500           // || is alias for or
not (http.status >= 500)
! http.secure                                      // ! is alias for not
http.secure xor http.authenticated                 // XOR: exactly one true
http.secure ^^ http.authenticated                  // ^^ is alias for xor
```

### Field-to-Field Comparisons

Compare two fields directly:

```go
user.login == device.owner
user.age >= minimum.age
request.region == server.region
```

### Map Field Access

Access values in map fields using bracket notation:

```go
user.attributes["region"] == "us-west"
config["timeout"] == 30
user.attributes["role"] == device.settings["required_role"]
```

### Field Presence Checking

Check if a field is present (has been set):

```go
http.host                    // true if http.host is set
not http.error               // true if http.error is not set
http.host and not http.error // true if host is set and error is not set
```

Presence checking uses existence-based truthiness:

- Any field that exists is truthy (including zero values and empty strings)
- Missing fields are considered falsy
- For boolean fields, the actual boolean value is used

### IP and CIDR Matching

```go
ip.src == 192.168.1.1
ip.src in "192.168.0.0/16"
ip.src in "2001:db8::/32"
```

### Array Membership

```go
http.status in {200, 201, 204}
port in {80, 443, 8080}
```

### Array Index Access

Access individual elements of an array by index (0-based):

```go
tags[0] == "first"                          // first element
tags[1] == "second"                         // second element
ports[0] > 1000                             // comparison on array element
```

Out-of-bounds or negative indices return no match (false).

### Array Unpack

Apply operations to all array elements with `[*]` syntax (ANY semantics):

```go
tags[*] == "admin"                          // true if ANY tag equals "admin"
tags[*] contains "test"                     // true if ANY tag contains "test"
tags[*] matches "^prod.*"                   // true if ANY tag matches pattern
ports[*] > 1000                             // true if ANY port > 1000
hosts[*] wildcard "*.example.com"           // true if ANY host matches
roles[*] in {"admin", "superuser"}          // true if ANY role is in the set
```

Example:

```go
tags := ["user", "admin", "guest"]

tags[*] == "admin"                          // true (admin matches)
tags[*] == "root"                           // false (no match)
tags[*] contains "min"                      // true (admin contains "min")
```

### Custom Lists

Reference external lists defined at runtime with `$list_name` syntax:

```go
role in $admin_roles         // check if role is in the admin_roles list
ip.src in $blocked_ips       // check if IP is in the blocked list
http.host in $allowed_hosts  // check if host is allowed
```

Lists are defined in the execution context (see API Reference below).

### Lookup Tables

Named key-value tables for dynamic lookups using `$table_name[field]` syntax:

```go
// Scalar lookup: returns a single value
$geo_table[ip.src] == "US"
$rate_limits[user.role] >= 100

// Array lookup: returns an array for use with in/contains
user.role in $allowed_roles[department]
ip.src in $blocked_nets[region]
ip.src not in $allowed_nets[zone]
```

Tables support different value types per key:

```go
// String values
ctx.SetTable("geo", map[string]string{"10.0.0.1": "US", "8.8.8.8": "DE"})

// Mixed value types
ctx.SetTableValues("limits", map[string]wirefilter.Value{
    "admin": wirefilter.IntValue(1000),
    "user":  wirefilter.IntValue(100),
})

// String arrays per key
ctx.SetTableList("roles", map[string][]string{
    "eng":   {"dev", "sre", "lead"},
    "sales": {"account", "manager"},
})

// IP/CIDR arrays per key
ctx.SetTableIPList("nets", map[string][]string{
    "office": {"10.0.0.0/8"},
    "vpn":    {"172.16.0.0/12"},
})
```

### Array-to-Array Operations

Check if an array field has any or all elements from a set:

```go
// OR logic: true if ANY element from user.groups is in the set
user.groups in {"guest", "admin"}

// AND logic: true if ALL elements from the set exist in user.groups
user.groups contains {"guest", "admin"}
```

Example:

```go
groups := ["admin", "guest", "user"]

groups in {"guest", "test"}       // true  (guest matches)
groups in {"foo", "bar"}          // false (no match)
groups contains {"guest", "user"} // true  (both exist in groups)
groups contains {"guest", "test"} // false (test is missing)
```

### Range Expressions

```go
port in {80..100, 443, 8000..9000}
http.status in {200..299}
```

### Time and Duration Literals

Timestamps use RFC 3339 format, durations use compound `d/h/m/s` notation:

```go
// Timestamp comparisons
created_at >= 2026-03-19T10:00:00Z
expires_at < 2026-12-31T23:59:59+05:00

// Duration comparisons
ttl >= 30m
session.timeout < 2d4h30m15s

// Temporal arithmetic
created_at + 1h >= 2026-03-19T11:00:00Z
expires_at < now() + 30m
ttl * 2 > 1h
ttl / 30m == 2

// Time range membership
created_at in {2026-03-19T00:00:00Z..2026-03-20T00:00:00Z}
ttl in {1h..3h}

// String-to-time coercion in equality
created_at == "2026-03-19T10:00:00Z"
```

Duration units:

- `d` - days (24 hours)
- `h` - hours
- `m` - minutes
- `s` - seconds

Compound durations combine units: `2d4h30m15s` = 2 days, 4 hours, 30 minutes, 15 seconds.

The `now()` function returns the current time (injectable via `WithNow` for testing).

### Array Comparison

```go
tags === "production"
tags !== "deprecated"
```

## API Reference

### Creating a Schema

Define the fields that can be used in filter expressions:

#### Method 1: Using method chaining

```go
schema := wirefilter.NewSchema().
    AddField("http.host", wirefilter.TypeString).
    AddField("http.status", wirefilter.TypeInt).
    AddField("http.secure", wirefilter.TypeBool).
    AddField("ip.src", wirefilter.TypeIP)
```

#### Method 2: Using a fields map

```go
fields := map[string]wirefilter.Type{
    "http.host":   wirefilter.TypeString,
    "http.status": wirefilter.TypeInt,
    "http.secure": wirefilter.TypeBool,
    "ip.src":      wirefilter.TypeIP,
}

schema := wirefilter.NewSchema(fields)
```

#### Method 3: Using multiple field maps (merged)

```go
httpFields := map[string]wirefilter.Type{
    "http.host":   wirefilter.TypeString,
    "http.status": wirefilter.TypeInt,
}

networkFields := map[string]wirefilter.Type{
    "ip.src": wirefilter.TypeIP,
    "ip.dst": wirefilter.TypeIP,
}

schema := wirefilter.NewSchema(httpFields, networkFields)
```

#### Typed Arrays and Maps

Use `AddArrayField` and `AddMapField` to enable compile-time validation of
operations on array elements and map values:

```go
schema := wirefilter.NewSchema().
    AddArrayField("tags", wirefilter.TypeString).
    AddArrayField("ports", wirefilter.TypeInt).
    AddMapField("headers", wirefilter.TypeString).
    AddMapField("scores", wirefilter.TypeFloat)
```

With typed fields, the compiler rejects invalid operations at compile time:

```go
tags[*] == "admin"          // valid: string equality on string array
tags[*] contains "prod"     // valid: string contains on string array
tags[*] > 10                // compile error: > not valid for string
ports[*] >= 1024            // valid: int comparison on int array
ports[*] contains "x"       // compile error: contains not valid for int
headers["x-env"] == "prod"  // valid: string equality on string map
scores["risk"] > 0.8        // valid: float comparison on float map
scores["risk"] contains "x" // compile error: contains not valid for float
```

Untyped `AddField("tags", TypeArray)` still works but skips element-level validation.

### Controlling Function Availability

You can control which functions are available in filter expressions
using allowlist or blocklist modes.

#### Blocklist Mode (Default)

All functions are allowed by default. Disable specific functions:

```go
schema := wirefilter.NewSchema().
    AddField("name", wirefilter.TypeString).
    DisableFunctions("lower", "upper", "concat")

// This will fail: lower is disabled
_, err := wirefilter.Compile(`lower(name) == "test"`, schema)
// Error: function not allowed: lower
```

#### Allowlist Mode

Only explicitly enabled functions are allowed:

```go
schema := wirefilter.NewSchema().
    AddField("name", wirefilter.TypeString).
    SetFunctionMode(wirefilter.FunctionModeAllowlist).
    EnableFunctions("lower", "upper", "len", "starts_with")

// This works: lower is enabled
_, err := wirefilter.Compile(`lower(name) == "test"`, schema)

// This fails: concat is not enabled
_, err = wirefilter.Compile(`concat(name, "!") == "test!"`, schema)
// Error: function not allowed: concat
```

Function names are case-insensitive.
Disabling "lower" also disables "LOWER" and "Lower".

### Disabling Regex

Disable all regex functionality at compile time with `DisableRegex()`.
This blocks the `matches`/`~` operator and regex-based functions
(`regex_replace`, `regex_extract`, `contains_word`).
Wildcard matching is not affected.

```go
schema := wirefilter.NewSchema().
    AddField("name", wirefilter.TypeString).
    DisableRegex()

// These will fail at compile time:
_, err := wirefilter.Compile(`name matches "^test"`, schema)
// Error: regex is disabled: matches operator is not allowed

_, err = wirefilter.Compile(`regex_replace(name, "[0-9]+", "X") == "X"`, schema)
// Error: regex is disabled: function regex_replace is not allowed

// These still work:
_, err = wirefilter.Compile(`name wildcard "*.example.com"`, schema)  // OK
_, err = wirefilter.Compile(`name contains "test"`, schema)           // OK
```

### Type Validation

When a schema is provided, the compiler validates that operators are compatible
with field types. This catches errors at compile time rather than runtime:

```go
schema := wirefilter.NewSchema().
    AddField("status", wirefilter.TypeInt).
    AddField("name", wirefilter.TypeString).
    AddField("ip", wirefilter.TypeIP)

// Valid: > works on Int
_, err := wirefilter.Compile(`status > 400`, schema)

// Invalid: > does not work on string
_, err = wirefilter.Compile(`name > "test"`, schema)
// Error: operator > is not valid for field type string

// Invalid: contains does not work on ip
_, err = wirefilter.Compile(`ip contains "10"`, schema)
// Error: operator contains is not valid for field type ip
```

Type validation rules:

| Type | Valid Operators |
|------|----------------|
| String | `==`, `!=`, `contains`, `matches`, `in`, `wildcard`, `strict wildcard` |
| Int | `==`, `!=`, `<`, `>`, `<=`, `>=`, `in`, `+`, `-`, `*`, `/`, `%` |
| Float | `==`, `!=`, `<`, `>`, `<=`, `>=`, `in`, `+`, `-`, `*`, `/`, `%` |
| Bool | `==`, `!=` |
| IP | `==`, `!=`, `in` |
| CIDR | `==`, `!=` |
| Array | `==`, `!=`, `===`, `!==`, `contains`, `in` |
| Map | `==`, `!=` |
| Bytes | `==`, `!=`, `contains` |
| Time | `==`, `!=`, `<`, `>`, `<=`, `>=`, `in`, `+`, `-` |
| Duration | `==`, `!=`, `<`, `>`, `<=`, `>=`, `in`, `+`, `-`, `*`, `/`, `%` |

The compiler infers types through compound expressions and validates operators
at each level:

- **Array unpack** (`field[*]`): inferred from `AddArrayField` element type
- **Map index** (`field["key"]`): inferred from `AddMapField` value type
- **Function calls**: inferred from `RegisterFunction` return type

```go
schema := wirefilter.NewSchema().
    AddArrayField("ports", wirefilter.TypeInt).
    AddMapField("user", wirefilter.TypeString).
    RegisterFunction("get_score", wirefilter.TypeFloat, []wirefilter.Type{wirefilter.TypeString})

ports[*] > 1000                         // valid: int comparison
ports[*] contains "x"                   // compile error: contains not valid for int
user["role"] > 5                        // compile error: > not valid for string
get_score(user["email"]) > 0.5          // valid: full chain inferred
get_score(user["email"]) contains "x"   // compile error: contains not valid for float
```

Untyped fields (`AddField`) and unregistered functions skip type validation.

### Expression Complexity Limits

Prevent overly complex or deeply nested expressions that could cause excessive
resource consumption:

```go
schema := wirefilter.NewSchema().
    AddField("x", wirefilter.TypeInt).
    SetMaxDepth(32).   // max AST nesting depth
    SetMaxNodes(1000)  // max total AST nodes

// Deeply nested expressions are rejected
_, err := wirefilter.Compile(deeplyNestedExpr, schema)
// Error: expression exceeds maximum depth of 32

// Extremely large expressions are rejected
_, err = wirefilter.Compile(hugeExpr, schema)
// Error: expression exceeds maximum node count of 1000
```

Both limits default to zero (unlimited). Set them on the schema to enable.

### Schema Inspection

Query the schema for field definitions and function availability:

```go
field, ok := schema.GetField("http.host") // (Field, bool)
// field.Name, field.Type, field.ElemType, field.ElemTyped

allowed := schema.IsFunctionAllowed("lower") // bool
```

The `Field` struct contains:

| Field | Type | Description |
|-------|------|-------------|
| `Name` | `string` | Field name |
| `Type` | `Type` | Field type (`TypeString`, `TypeInt`, etc.) |
| `ElemType` | `Type` | Element type for typed arrays/maps (set via `AddArrayField`/`AddMapField`) |
| `ElemTyped` | `bool` | True if `ElemType` was explicitly set |

### Explicit Validation

Validate an expression against the schema without compiling:

```go
lexer := wirefilter.NewLexer(expression)
parser := wirefilter.NewParser(lexer)
expr, err := parser.Parse()
if err != nil {
    log.Fatal(err)
}

if err := schema.Validate(expr); err != nil {
    log.Fatal(err)
}
```

### Compiling a Filter

Parse and validate a filter expression:

```go
filter, err := wirefilter.Compile(expression, schema)
if err != nil {
    log.Fatal(err)
}
```

If `schema` is `nil`, field validation is skipped.

### Execution Context

Set runtime values for evaluation:

```go
ctx := wirefilter.NewExecutionContext().
    SetStringField("http.host", "example.com").
    SetIntField("http.status", 200).
    SetBoolField("http.secure", true).
    SetIPField("ip.src", "192.168.1.1").
    SetBytesField("body", []byte("raw data"))
```

All setter methods return `*ExecutionContext` for method chaining.

#### Initializing with Field Maps

Like `NewSchema`, `NewExecutionContext` accepts optional field maps for bulk initialization.
Multiple maps can be provided and will be merged:

```go
ctx := wirefilter.NewExecutionContext(
    map[string]wirefilter.Value{
        "http.host":   wirefilter.StringValue("example.com"),
        "http.status": wirefilter.IntValue(200),
    },
    map[string]wirefilter.Value{
        "http.secure": wirefilter.BoolValue(true),
    },
)
```

For a generic value or custom type:

```go
ctx := wirefilter.NewExecutionContext().
    SetField("custom", wirefilter.StringValue("value"))
```

#### Setting Map Fields

For map fields with string values:

```go
ctx := wirefilter.NewExecutionContext().
    SetMapField("user.attributes", map[string]string{
        "region": "us-west",
        "role":   "admin",
    })
```

For map fields with mixed value types:

```go
ctx := wirefilter.NewExecutionContext().
    SetMapFieldValues("config", map[string]wirefilter.Value{
        "timeout": wirefilter.IntValue(30),
        "host":    wirefilter.StringValue("localhost"),
        "enabled": wirefilter.BoolValue(true),
    })
```

#### Setting Array Fields

For array fields with string values:

```go
ctx := wirefilter.NewExecutionContext().
    SetArrayField("tags", []string{"admin", "user", "guest"})
```

For array fields with integer values:

```go
ctx := wirefilter.NewExecutionContext().
    SetIntArrayField("ports", []int64{80, 443, 8080})
```

For float fields:

```go
ctx := wirefilter.NewExecutionContext().
    SetFloatField("score", 3.14)
```

#### Setting Time and Duration Fields

```go
ctx := wirefilter.NewExecutionContext().
    SetTimeField("created_at", time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)).
    SetDurationField("ttl", 30*time.Minute)
```

#### Injecting a Clock for now()

By default, `now()` returns the current UTC time. Use `WithNow` to inject a fixed clock for deterministic testing:

```go
fixed := time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)
ctx := wirefilter.NewExecutionContext().
    WithNow(func() time.Time { return fixed }).
    SetTimeField("expires_at", fixed.Add(time.Hour))
```

For map fields where each key maps to an array of values (e.g., HTTP headers):

```go
ctx := wirefilter.NewExecutionContext().
    SetMapArrayField("http.headers", map[string][]wirefilter.Value{
        "Accept": {wirefilter.StringValue("text/html"), wirefilter.StringValue("application/json")},
        "X-Forwarded-For": {wirefilter.StringValue("10.0.0.1")},
    })
```

#### Setting Custom Lists

Custom lists are referenced in expressions with `$list_name` syntax:

```go
ctx := wirefilter.NewExecutionContext().
    SetStringField("role", "admin").
    SetList("admin_roles", []string{"admin", "superuser", "root"})

// Expression: role in $admin_roles
```

For IP address lists:

```go
ctx := wirefilter.NewExecutionContext().
    SetIPField("ip.src", "192.168.1.100").
    SetIPList("blocked_ips", []string{"10.0.0.1", "192.168.1.100", "172.16.0.0/12"})

// Expression: ip.src in $blocked_ips
// Expression: ip.src not in $blocked_ips
```

#### Retrieving Values

Retrieve fields, lists, tables, and functions from the execution context:

```go
val, ok := ctx.GetField("http.host")       // (Value, bool)
list, ok := ctx.GetList("admin_roles")      // (ArrayValue, bool)
table, ok := ctx.GetTable("geo")            // (MapValue, bool)
handler, ok := ctx.GetFunc("get_score")     // (FuncHandler, bool)
```

#### Retrieving the Go Context

```go
goCtx := ctx.Context() // returns context.Background() if not set via WithContext
```

#### Cache Inspection

```go
ctx.EnableCache()
// ... execute filters ...
n := ctx.CacheLen() // number of cached function results
ctx.ResetCache()    // clear cache
```

### User-Defined Functions

Register custom functions for domain-specific logic that runs at evaluation time.
Functions are registered in two steps: declare the signature on the schema (optional,
for compile-time validation), then bind the handler on the execution context.

```go
// Step 1: Register signature on schema (optional, enables compile-time validation)
schema := wirefilter.NewSchema().
    AddField("smtp.sender.domain", wirefilter.TypeString).
    AddField("src.ip", wirefilter.TypeIP).
    RegisterFunction("maintenance", wirefilter.TypeBool, nil).
    RegisterFunction("get_domain_score", wirefilter.TypeFloat, []wirefilter.Type{wirefilter.TypeString}).
    RegisterFunction("is_tor", wirefilter.TypeBool, []wirefilter.Type{wirefilter.TypeIP})

// Step 2: Compile the expression
filter, _ := wirefilter.Compile(
    `not is_tor(src.ip) and get_domain_score(smtp.sender.domain) > 5.0`,
    schema,
)

// Step 3: Bind handlers on the execution context
ctx := wirefilter.NewExecutionContext().
    SetIPField("src.ip", srcIP).
    SetStringField("smtp.sender.domain", senderDomain).
    SetFunc("maintenance", func(ctx context.Context, args []wirefilter.Value) (wirefilter.Value, error) {
        return wirefilter.BoolValue(isMaintenanceWindow()), nil
    }).
    SetFunc("get_domain_score", func(ctx context.Context, args []wirefilter.Value) (wirefilter.Value, error) {
        domain := string(args[0].(wirefilter.StringValue))
        score, err := reputationDB.GetScore(ctx, domain)
        return wirefilter.FloatValue(score), err
    }).
    SetFunc("is_tor", func(ctx context.Context, args []wirefilter.Value) (wirefilter.Value, error) {
        ip := args[0].(wirefilter.IPValue).IP
        return wirefilter.BoolValue(torExitNodes.Contains(ip)), nil
    })

result, _ := filter.Execute(ctx)
```

Functions can return any value type and work with all operators:

```go
// Return ArrayValue of CIDRValue for use with "in"
// ip.src in get_spf_cidrs(smtp.sender.domain)
ctx.SetFunc("get_spf_cidrs", func(ctx context.Context, args []wirefilter.Value) (wirefilter.Value, error) {
    domain := string(args[0].(wirefilter.StringValue))
    cidrs := spfResolver.GetCIDRs(domain)
    arr := make(wirefilter.ArrayValue, 0, len(cidrs))
    for _, c := range cidrs {
        _, ipNet, _ := net.ParseCIDR(c)
        arr = append(arr, wirefilter.CIDRValue{IPNet: ipNet})
    }
    return arr, nil
})

// Return CIDRValue for direct "in" check
// ip.src in get_network(zone)
ctx.SetFunc("get_network", func(ctx context.Context, args []wirefilter.Value) (wirefilter.Value, error) {
    // ...
    return wirefilter.CIDRValue{IPNet: ipNet}, nil
})

// Use with arithmetic
// get_score(domain) * 2 + get_ip_score(src.ip) > 10.0

// Receive MapValue (e.g., HTTP headers with map[string][]string)
// is_valid_headers(http.headers)
ctx.SetFunc("is_valid_headers", func(ctx context.Context, args []wirefilter.Value) (wirefilter.Value, error) {
    headers := args[0].(wirefilter.MapValue)
    // inspect headers...
    return wirefilter.BoolValue(true), nil
})
```

If a function handler is not bound at runtime, it returns nil (same as a missing field).

UDF results are cached when `EnableCache()` is active on the execution context.
Same function with same arguments is only called once across multiple rule evaluations.

### Expression Hash

Compiled filters expose a canonical hash for deduplication. The hash ignores
whitespace differences and operator aliases (`and` vs `&&`, `or` vs `||`,
`not` vs `!`, `xor` vs `^^`, `matches` vs `~`):

```go
f1, _ := wirefilter.Compile(`name == "test" and status >= 400`, schema)
f2, _ := wirefilter.Compile(`name   ==   "test"  &&  status  >=  400`, schema)

f1.Hash() == f2.Hash() // true - semantically identical
```

This is useful for deduplicating rules in large rule sets.

### Executing a Filter

Evaluate the filter against the context:

```go
result, err := filter.Execute(ctx)
if err != nil {
    log.Fatal(err)
}

if result {
    fmt.Println("Filter matched")
}
```

### Execution Timeout

Use `context.Context` for cancellation and timeout support:

```go
goCtx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
defer cancel()

ctx := wirefilter.NewExecutionContext().
    WithContext(goCtx).
    SetStringField("name", "test")

result, err := filter.Execute(ctx) // returns context.DeadlineExceeded on timeout
```

### Evaluation Tracing

Enable tracing to debug which sub-expressions matched:

```go
ctx := wirefilter.NewExecutionContext().
    EnableTrace().
    SetStringField("name", "test").
    SetIntField("status", 500)

result, _ := filter.Execute(ctx)
trace := ctx.Trace() // returns *TraceNode tree with expression, result, duration
```

### Result Caching

Cache user-defined function results across multiple rule evaluations:

```go
ctx := wirefilter.NewExecutionContext().
    EnableCache().              // default max 1024 entries
    SetCacheMaxSize(5000).      // or set custom limit
    SetFunc("get_score", handler)

// Same function+args only calls handler once across all rules
filter1.Execute(ctx)
filter2.Execute(ctx) // get_score results served from cache

ctx.ResetCache()     // clear cache between request batches
```

### Rule Metadata

Attach metadata to compiled filters for rule management:

```go
filter, _ := wirefilter.Compile(expr, schema)
filter.SetMeta(wirefilter.RuleMeta{
    ID:   "WAF-1001",
    Tags: map[string]string{"severity": "high", "category": "xss"},
})

meta := filter.Meta()
fmt.Println(meta.ID)              // "WAF-1001"
fmt.Println(meta.Tags["severity"]) // "high"
```

### Exporting for Audit Logs

Export schema and execution context as JSON-friendly structures for audit logging.

#### Schema Export

Returns a flat map of field names to their types:

```go
schema := wirefilter.NewSchema().
    AddField("http.host", wirefilter.TypeString).
    AddField("http.status", wirefilter.TypeInt).
    AddField("ip.src", wirefilter.TypeIP)

exported := schema.Export()
// map[string]wirefilter.Type{
//     "http.host":   wirefilter.TypeString,
//     "http.status": wirefilter.TypeInt,
//     "ip.src":      wirefilter.TypeIP,
// }
```

#### Context Export

Returns a flat map of field values, directly compatible with `json.Marshal`:

```go
ctx := wirefilter.NewExecutionContext().
    SetStringField("http.host", "example.com").
    SetIntField("http.status", 200).
    SetIPField("ip.src", "192.0.2.1").
    SetArrayField("tags", []string{"production", "v2"}).
    SetMapField("headers", map[string]string{"content-type": "application/json"})

exported := ctx.Export()
// map[string]any{
//     "http.host":   "example.com",
//     "http.status": int64(200),
//     "ip.src":      "192.0.2.1",
//     "tags":        []any{"production", "v2"},
//     "headers":     map[string]any{"content-type": "application/json"},
// }
```

#### Lists Export

Returns a flat map of list names to their values:

```go
ctx := wirefilter.NewExecutionContext().
    SetList("allowed_roles", []string{"viewer", "editor"}).
    SetIPList("blocked_nets", []string{"192.0.2.0/24", "198.51.100.1"})

exported := ctx.ExportLists()
// map[string]any{
//     "allowed_roles": []any{"viewer", "editor"},
//     "blocked_nets":  []any{"192.0.2.0/24", "198.51.100.1"},
// }
```

## Data Types

| Type | Description | Example |
|------|-------------|---------|
| `TypeString` | String values | `"example.com"` |
| `TypeInt` | Integer values | `200`, `-5` |
| `TypeFloat` | Floating-point values | `3.14`, `-2.5` |
| `TypeBool` | Boolean values | `true`, `false` |
| `TypeIP` | IP addresses (IPv4/IPv6) | `192.168.1.1`, `2001:db8::1` |
| `TypeCIDR` | CIDR network ranges | `192.168.0.0/24`, `2001:db8::/32` |
| `TypeBytes` | Byte arrays | `[]byte("data")` |
| `TypeArray` | Arrays of values | `{1, 2, 3}` |
| `TypeMap` | Map of string keys to values | `{"key": "value"}` |
| `TypeTime` | RFC 3339 timestamps | `2026-03-19T10:00:00Z` |
| `TypeDuration` | Duration values | `30m`, `7d`, `1h30m` |

### Value Interface

All value types implement the `Value` interface:

```go
type Value interface {
    Type() Type
    Equal(other Value) bool
    String() string
    IsTruthy() bool
}
```

Concrete value types: `StringValue`, `IntValue`, `FloatValue`, `BoolValue`,
`IPValue`, `CIDRValue`, `BytesValue`, `ArrayValue`, `MapValue`, `TimeValue`,
`DurationValue`.

Additional types used internally and in advanced scenarios:

- `IntervalValue` - non-materialized range for `in` membership checks (created via `NewIntInterval`, `NewTimeInterval`, `NewDurationInterval`)
- `UnpackedArrayValue` - intermediate type for `array[*]` unpack operations

### Value Type Methods

Some value types provide additional methods beyond the `Value` interface:

| Type | Method | Description |
|------|--------|-------------|
| `ArrayValue` | `Contains(v Value) bool` | Check if array contains a value |
| `MapValue` | `Get(key string) (Value, bool)` | Retrieve a value by key |
| `CIDRValue` | `Contains(ip net.IP) bool` | Check if IP is in the CIDR range |
| `TimeValue` | `GoTime() time.Time` | Convert to `time.Time` |

### Value Constructors

```go
// Time values
tv := wirefilter.NewTimeValue(time.Now())  // TimeValue from time.Time
t := tv.GoTime()                            // back to time.Time

// Interval values (for range membership checks)
wirefilter.NewIntInterval(wirefilter.IntValue(1), wirefilter.IntValue(100))
wirefilter.NewTimeInterval(start, end)
wirefilter.NewDurationInterval(start, end)
```

## Operators

### Comparison Operators

| Operator | Description | Example |
|----------|-------------|---------|
| `==` | Equal | `status == 200` |
| `!=` | Not equal | `status != 404` |
| `<` | Less than | `status < 400` |
| `>` | Greater than | `status > 300` |
| `<=` | Less than or equal | `status <= 299` |
| `>=` | Greater than or equal | `status >= 500` |

### Logical Operators

| Operator | Description | Example |
|----------|-------------|---------|
| `and`, `&&` | Logical AND | `a and b`, `a && b` |
| `or`, `\|\|` | Logical OR | `a or b`, `a \|\| b` |
| `xor`, `^^` | Logical XOR (exclusive OR) | `a xor b`, `a ^^ b` |
| `not`, `!` | Logical NOT | `not a`, `! a` |

### Membership Operators

| Operator | Description | Example |
|----------|-------------|---------|
| `in` | Value in array, IP in CIDR, array ANY match | `port in {80, 443}` |
| `not in` | Negated membership | `ip not in {10.0.0.0/8}` |
| `contains` | Substring or array ALL match | `path contains "/api"` |
| `not contains` | Negated containment | `path not contains "/admin"` |
| `matches`, `~` | Regex match | `ua matches "^Mozilla"` |

### Wildcard Operators

| Operator | Description | Example |
|----------|-------------|---------|
| `wildcard` | Glob match (case-insensitive) | `host wildcard "*.ex.com"` |
| `strict wildcard` | Glob (case-sensitive) | `host strict wildcard "*.a"` |

Wildcard patterns support:

- `*` matches any sequence of characters (including empty)
- `?` matches any single character

### Arithmetic Operators

| Operator | Description | Example |
|----------|-------------|---------|
| `+` | Addition | `x + 1` |
| `-` | Subtraction | `x - 5` |
| `*` | Multiplication | `x * 2` |
| `/` | Division (nil on zero) | `x / 3` |
| `%` | Modulo (nil on zero) | `x % 2` |

Arithmetic works on Int, Float, Time, and Duration types. Mixed Int/Float produces Float results.
Standard precedence: `*`, `/`, `%` bind tighter than `+`, `-`.

Temporal arithmetic rules:

| Expression | Result Type | Example |
|-----------|------------|---------|
| `time + duration` | Time | `created_at + 1h` |
| `time - duration` | Time | `expires_at - 30m` |
| `duration + time` | Time | `1h + created_at` |
| `time - time` | Duration | `end - start` |
| `duration + duration` | Duration | `ttl + 30m` |
| `duration - duration` | Duration | `ttl - 5m` |
| `duration * int/float` | Duration | `ttl * 2` |
| `int/float * duration` | Duration | `2 * ttl` |
| `duration / int/float` | Duration | `ttl / 2` |
| `duration / duration` | Int | `ttl / 30m` |
| `duration % duration` | Duration | `ttl % 1h` |

### Array Operators

| Operator | Description | Example |
|----------|-------------|---------|
| `===` | All elements equal | `tags === "prod"` |
| `!==` | Any element not equal | `tags !== "test"` |

## Functions

Wirefilter provides built-in functions for transforming and inspecting values.

### String Functions

| Function | Description | Example |
|----------|-------------|---------|
| `lower(String)` | Convert to lowercase | `lower(http.host) == "example.com"` |
| `upper(String)` | Convert to uppercase | `upper(method) == "GET"` |
| `len(String)` | String length in bytes | `len(path) > 100` |
| `starts_with(String, String)` | Check prefix | `starts_with(path, "/api/")` |
| `ends_with(String, String)` | Check suffix | `ends_with(file, ".json")` |
| `substring(String, Int [, Int])` | Extract substring | `substring(s, 0, 4)` |
| `concat(String...)` | Concatenate strings | `concat(scheme, "://", host)` |
| `split(String, String)` | Split into array | `split(header, ",")[0]` |
| `url_decode(String)` | URL decode | `url_decode(query)` |
| `trim(String)` | Trim whitespace from both ends | `trim(name)` |
| `trim_left(String)` | Trim whitespace from left | `trim_left(name)` |
| `trim_right(String)` | Trim whitespace from right | `trim_right(name)` |
| `replace(String, String, String)` | Replace all occurrences | `replace(path, "/", "-")` |
| `regex_replace(String, String, String)` | Replace regex matches | `regex_replace(s, "[0-9]+", "X")` |
| `regex_extract(String, String)` | Extract first regex match | `regex_extract(path, "/v([0-9]+)/")` |
| `contains_word(String, String)` | Word boundary match | `contains_word(msg, "admin")` |

### Array Functions

| Function | Description | Example |
|----------|-------------|---------|
| `len(Array)` | Array element count | `len(tags) > 0` |
| `count(Array)` | Count truthy elements | `count(tags) >= 3` |
| `any(expression)` | Any element matches | `any(tags[*] == "admin")` |
| `all(expression)` | All elements match | `all(ports[*] > 0)` |
| `has_value(Array, Value)` | Array contains value | `has_value(tags, "admin")` |
| `join(Array, String)` | Join array elements | `join(tags, ",")` |
| `contains_any(Array, Array)` | Any element from second in first | `contains_any(tags, required)` |
| `contains_all(Array, Array)` | All elements from second in first | `contains_all(tags, required)` |
| `intersection(Array, Array)` | Elements in both arrays | `intersection(a, b)` |
| `union(Array, Array)` | Combined unique elements | `union(a, b)` |
| `difference(Array, Array)` | Elements in first not in second | `difference(a, b)` |

### Map Functions

| Function | Description | Example |
|----------|-------------|---------|
| `len(Map)` | Map key count | `len(headers) > 0` |
| `has_key(Map, String)` | Check key exists | `has_key(headers, "Auth")` |

### IP Functions

| Function | Description | Example |
|----------|-------------|---------|
| `cidr(IP, Int)` | Apply CIDR mask for IPv4 | `cidr(ip, 24)` |
| `cidr6(IP, Int)` | Apply CIDR mask for IPv6 | `cidr6(ip, 64)` |
| `is_ipv4(IP)` | Check if IPv4 | `is_ipv4(ip)` |
| `is_ipv6(IP)` | Check if IPv6 | `is_ipv6(ip)` |
| `is_loopback(IP)` | Check if loopback | `is_loopback(ip)` |

### Time Functions

| Function | Description | Example |
|----------|-------------|---------|
| `now()` | Current time (injectable clock) | `created_at >= now() - 1h` |

The `now()` function is always available regardless of function mode settings.
Use `WithNow` on the execution context to inject a fixed clock for testing.

### Math Functions

| Function | Description | Example |
|----------|-------------|---------|
| `abs(Int\|Float)` | Absolute value | `abs(x) > 5` |
| `ceil(Float)` | Ceiling (round up) | `ceil(x) == 4` |
| `floor(Float)` | Floor (round down) | `floor(x) == 3` |
| `round(Float)` | Round to nearest | `round(x) == 4` |

### Utility Functions

| Function | Description | Example |
|----------|-------------|---------|
| `coalesce(Value...)` | First non-nil value | `coalesce(a, b, "default")` |
| `exists(Value)` | Check if field is set | `exists(http.referer)` |

### Function Examples

```go
// Case-insensitive comparison
lower(http.host) == "example.com"

// Check path prefix
starts_with(http.path, "/api/v1/")

// Check file extension
ends_with(request.file, ".pdf")

// URL decode and search
url_decode(http.query) contains "admin"

// Check if any tag matches
any(tags[*] contains "prod")

// Check if all ports are valid
all(ports[*] > 0 and ports[*] < 65536)

// Build URL from parts
concat(scheme, "://", host, path) == "https://api.example.com/users"

// Parse CSV header
split(header, ",")[0] == "value1"

// Check map key exists
has_key(request.headers, "X-Auth-Token")

// Apply /24 CIDR mask to IPv4 (returns CIDRValue)
cidr(ip.src, 24) == 192.168.1.0/24

// Apply /64 CIDR mask for IPv6 networks
cidr6(ip.src, 64) == "2001:db8::/64"

// Check if two IPs are in the same /24 network
cidr(ip.src, 24) == cidr(ip.dst, 24)

// Check if IP is in the network of another IP
ip.src in cidr(ip.dst, 24)

// Check if field exists
exists(http.referer)
```

## Advanced Examples

### HTTP Request Filtering

```go
schema := wirefilter.NewSchema().
    AddField("http.method", wirefilter.TypeString).
    AddField("http.host", wirefilter.TypeString).
    AddField("http.path", wirefilter.TypeString).
    AddField("http.status", wirefilter.TypeInt)

expression := `
    http.method == "GET" and
    http.host == "api.example.com" and
    http.path contains "/v1/" and
    http.status >= 200 and http.status < 300
`

filter, _ := wirefilter.Compile(expression, schema)

ctx := wirefilter.NewExecutionContext().
    SetStringField("http.method", "GET").
    SetStringField("http.host", "api.example.com").
    SetStringField("http.path", "/v1/users").
    SetIntField("http.status", 200)

matched, _ := filter.Execute(ctx)
```

### Network Traffic Filtering

```go
schema := wirefilter.NewSchema().
    AddField("ip.src", wirefilter.TypeIP).
    AddField("ip.dst", wirefilter.TypeIP).
    AddField("port.dst", wirefilter.TypeInt).
    AddField("protocol", wirefilter.TypeString)

expression := `
    ip.src in "10.0.0.0/8" and
    port.dst in {80, 443, 8080..8090} and
    protocol == "tcp"
`

filter, _ := wirefilter.Compile(expression, schema)

ctx := wirefilter.NewExecutionContext().
    SetIPField("ip.src", "10.1.2.3").
    SetIPField("ip.dst", "192.168.1.1").
    SetIntField("port.dst", 443).
    SetStringField("protocol", "tcp")

matched, _ := filter.Execute(ctx)
```

### Tag-based Filtering

```go
schema := wirefilter.NewSchema().
    AddField("tags", wirefilter.TypeArray).
    AddField("environment", wirefilter.TypeString)

expression := `
    environment == "production" and
    tags === "critical"
`

filter, _ := wirefilter.Compile(expression, schema)

tags := wirefilter.ArrayValue{
    wirefilter.StringValue("critical"),
    wirefilter.StringValue("monitored"),
}

ctx := wirefilter.NewExecutionContext().
    SetField("tags", tags).
    SetStringField("environment", "production")

matched, _ := filter.Execute(ctx)
```

### Field-to-Field and Map Access

```go
schema := wirefilter.NewSchema().
    AddField("user.attributes", wirefilter.TypeMap).
    AddField("device.vars", wirefilter.TypeMap).
    AddField("user.login", wirefilter.TypeString).
    AddField("device.owner", wirefilter.TypeString)

// Compare map values from different fields
expression := `
    user.attributes["region"] == device.vars["region"] and
    user.login == device.owner
`

filter, _ := wirefilter.Compile(expression, schema)

ctx := wirefilter.NewExecutionContext().
    SetMapField("user.attributes", map[string]string{"region": "us-west"}).
    SetMapField("device.vars", map[string]string{"region": "us-west"}).
    SetStringField("user.login", "john").
    SetStringField("device.owner", "john")

matched, _ := filter.Execute(ctx) // true
```

### Wildcard Host Matching

```go
schema := wirefilter.NewSchema().
    AddField("http.host", wirefilter.TypeString)

// Case-insensitive wildcard matching
filter, _ := wirefilter.Compile(`http.host wildcard "*.example.com"`, schema)

ctx := wirefilter.NewExecutionContext().
    SetStringField("http.host", "API.EXAMPLE.COM")

matched, _ := filter.Execute(ctx) // true (case-insensitive)

// Case-sensitive matching
expr := `http.host strict wildcard "*.Example.com"`
filterStrict, _ := wirefilter.Compile(expr, schema)

ctx2 := wirefilter.NewExecutionContext().
    SetStringField("http.host", "api.Example.com")

matched2, _ := filterStrict.Execute(ctx2) // true
```

### XOR Logic for Mutual Exclusion

```go
schema := wirefilter.NewSchema().
    AddField("user.is_admin", wirefilter.TypeBool).
    AddField("user.is_guest", wirefilter.TypeBool)

// XOR: user must be either admin or guest, but not both
filter, _ := wirefilter.Compile(`user.is_admin xor user.is_guest`, schema)

ctx := wirefilter.NewExecutionContext().
    SetBoolField("user.is_admin", true).
    SetBoolField("user.is_guest", false)

matched, _ := filter.Execute(ctx) // true
```

### Array Index and Unpack Operations

```go
schema := wirefilter.NewSchema().
    AddField("tags", wirefilter.TypeArray).
    AddField("ports", wirefilter.TypeArray)

// Access specific array element
filter1, _ := wirefilter.Compile(`tags[0] == "primary"`, schema)

// Check if ANY element matches (unpack)
filter2, _ := wirefilter.Compile(`tags[*] contains "admin"`, schema)

// Check if ANY port is in a dangerous range
filter3, _ := wirefilter.Compile(`ports[*] > 1000 and ports[*] < 2000`, schema)

ctx := wirefilter.NewExecutionContext().
    SetArrayField("tags", []string{"primary", "admin-role", "active"}).
    SetIntArrayField("ports", []int64{80, 443, 1500})

matched1, _ := filter1.Execute(ctx) // true (tags[0] == "primary")
matched2, _ := filter2.Execute(ctx) // true (admin-role contains "admin")
matched3, _ := filter3.Execute(ctx) // true (1500 is between 1000 and 2000)
```

### Custom Lists for Dynamic Filtering

```go
schema := wirefilter.NewSchema().
    AddField("user.role", wirefilter.TypeString).
    AddField("ip.src", wirefilter.TypeIP)

// Filter using custom lists
expression := `user.role in $privileged_roles and not (ip.src in $blocked_ips)`

filter, _ := wirefilter.Compile(expression, schema)

// Lists can be updated at runtime without recompiling the filter
ctx := wirefilter.NewExecutionContext().
    SetStringField("user.role", "admin").
    SetIPField("ip.src", "10.0.0.50").
    SetList("privileged_roles", []string{"admin", "superuser", "operator"}).
    SetIPList("blocked_ips", []string{"192.168.1.1", "10.0.0.100"})

matched, _ := filter.Execute(ctx) // true (admin is privileged, IP not blocked)
```

### Time-based Filtering

```go
schema := wirefilter.NewSchema().
    AddField("created_at", wirefilter.TypeTime).
    AddField("ttl", wirefilter.TypeDuration).
    AddField("status", wirefilter.TypeString)

// Filter for recently created active items with sufficient TTL
expression := `
    created_at >= now() - 1h and
    ttl > 30m and
    status == "active"
`

filter, _ := wirefilter.Compile(expression, schema)

fixed := time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)
ctx := wirefilter.NewExecutionContext().
    WithNow(func() time.Time { return fixed }).
    SetTimeField("created_at", fixed.Add(-30*time.Minute)).
    SetDurationField("ttl", 2*time.Hour).
    SetStringField("status", "active")

matched, _ := filter.Execute(ctx) // true
```

### Raw Strings for Complex Patterns

```go
schema := wirefilter.NewSchema().
    AddField("file.path", wirefilter.TypeString).
    AddField("log.message", wirefilter.TypeString)

// Raw strings make regex patterns cleaner
expr1 := `file.path matches r"^C:\Windows\System32\.*\.dll$"`
filter1, _ := wirefilter.Compile(expr1, schema)
expr2 := `log.message matches r"error code: \d{4}"`
filter2, _ := wirefilter.Compile(expr2, schema)

ctx := wirefilter.NewExecutionContext().
    SetStringField("file.path", `C:\Windows\System32\kernel32.dll`).
    SetStringField("log.message", "error code: 1234")

matched1, _ := filter1.Execute(ctx) // true
matched2, _ := filter2.Execute(ctx) // true
```

## Binary Serialization

Compiled filters can be serialized to bytes and restored later without re-parsing.
This is useful for pre-compiling large rule sets and loading them from external
storage (databases, caches, files) at runtime.

### Saving a Compiled Filter

```go
filter, err := wirefilter.Compile(`ip.src not in $blocked and status < 500`, schema)
if err != nil {
    log.Fatal(err)
}

data, err := filter.MarshalBinary()
if err != nil {
    log.Fatal(err)
}

// Store data in Redis, database, file, etc.
```

### Loading a Compiled Filter

```go
// Load data from external storage
filter := &wirefilter.Filter{}
if err := filter.UnmarshalBinary(data); err != nil {
    log.Fatal(err)
}

// Use the filter directly - no re-parsing needed
result, err := filter.Execute(ctx)
```

The `Filter` type implements the standard `encoding.BinaryMarshaler` and
`encoding.BinaryUnmarshaler` interfaces.

Regex and CIDR caches are rebuilt lazily on first use after deserialization.
Schema validation is not re-applied; the filter is assumed to have been
validated at compile time.

## Helper Functions

Utility functions for working with IP addresses and regex patterns:

```go
// Normalize an IP address to 16-byte representation
ip := wirefilter.NormalizeIP(net.ParseIP("192.168.1.1"))

// Check IP version
wirefilter.IsIPv4(ip) // true
wirefilter.IsIPv6(ip) // false

// Check if an IP is in a CIDR range
match, err := wirefilter.IPInCIDR(ip, "192.168.0.0/16") // true, nil

// Match a string against a regex pattern
match, err := wirefilter.MatchesRegex("hello world", "^hello.*") // true, nil
```

## Lexer and Parser

The lexer and parser are exported for advanced use cases such as custom validation,
AST inspection, or building tools on top of wirefilter expressions.

### Lexer

Tokenize a filter expression:

```go
lexer := wirefilter.NewLexer(`http.host == "example.com" and status >= 400`)

for {
    tok := lexer.NextToken()
    if tok.Type == wirefilter.TokenEOF {
        break
    }
    fmt.Printf("%s: %q\n", tok.Type, tok.Value)
}
```

### Parser

Parse tokens into an AST:

```go
lexer := wirefilter.NewLexer(expression)
parser := wirefilter.NewParser(lexer)

expr, err := parser.Parse()
if err != nil {
    // err may contain multiple parse errors (multi-error recovery)
    log.Fatal(err)
}

// Access individual parse errors
for _, e := range parser.Errors() {
    fmt.Println(e)
}
```

### AST Node Types

The parser produces an AST composed of the following expression types:

| Type | Description | Example Expression |
|------|-------------|--------------------|
| `BinaryExpr` | Binary operation (left, operator, right) | `a == b`, `x and y` |
| `UnaryExpr` | Unary operation (operator, operand) | `not x`, `!a` |
| `FieldExpr` | Field reference | `http.host` |
| `LiteralExpr` | Literal value | `"hello"`, `200`, `true` |
| `ArrayExpr` | Array literal | `{1, 2, 3}` |
| `RangeExpr` | Range expression (start, end) | `1..100` |
| `IndexExpr` | Index access (object, index) | `map["key"]`, `arr[0]` |
| `UnpackExpr` | Array unpack (array) | `tags[*]` |
| `ListRefExpr` | Custom list reference | `$blocked_hosts` |
| `FunctionCallExpr` | Function call (name, arguments) | `lower(name)` |

All expression types implement the `Expression` interface.

### Operator Precedence

From lowest to highest:

| Level | Operators |
|-------|-----------|
| `OR` | `or`, `\|\|` |
| `XOR` | `xor`, `^^` |
| `AND` | `and`, `&&` |
| `PREFIX` | `not`, `!` |
| `EQUALS` | `==`, `!=`, `===`, `!==` |
| `COMPARE` | `<`, `>`, `<=`, `>=` |
| `MEMBERSHIP` | `in`, `contains`, `matches`, `wildcard`, `strict wildcard` |
| `SUM` | `+`, `-` |
| `PRODUCT` | `*`, `/`, `%` |

### Function Signature Type

The `FuncSignature` struct describes a user-defined function's compile-time signature:

```go
type FuncSignature struct {
    ArgTypes   []Type // expected argument types (nil means any count/type)
    ReturnType Type   // return type for schema validation
}
```

When `ArgTypes` is `nil`, argument validation is skipped. Otherwise, the compiler
checks both the argument count and types at compile time.

## Performance

The filter engine is designed for high performance:

- Filters are compiled once and can be executed multiple times
- Schema validation happens at compile time, not runtime
- Efficient AST-based evaluation with zero-alloc hot paths
- No runtime reflection
- Binary deserialization is ~2x faster than compiling from string
- Zero-allocation comparisons for string, int, float, bool, time, duration types
- Non-materialized integer ranges (`{200..299}`) use O(1) interval checks instead of array allocation
- Stack-buffered arrays (up to 8 elements) and function args (up to 4 args) avoid heap allocation
- Switch-based built-in function dispatch (no map allocation per call)
- Time values stored as int64 nanoseconds to avoid interface boxing overhead

For optimal performance, compile filters once and reuse them across multiple
executions. For large rule sets, pre-compile and store the binary representation
for fast loading.

## Error Handling

The library returns errors for:

- Malformed filter expressions
- Unknown field references (when schema is provided)
- Operator-type mismatches (when schema is provided)
- Expression complexity limits exceeded
- Invalid regex patterns
- Disabled regex usage (when `DisableRegex()` is set)
- Context cancellation or timeout (`context.DeadlineExceeded`, `context.Canceled`)
- User-defined function errors

The parser supports **multi-error recovery**: when a syntax error is encountered,
it synchronizes to the next logical operator or closing delimiter and continues
parsing to report as many errors as possible in a single pass.

```go
filter, err := wirefilter.Compile(expression, schema)
if err != nil {
    // err.Error() may contain multiple parse errors
    log.Printf("Compilation error: %v", err)
    return
}

result, err := filter.Execute(ctx)
if err != nil {
    log.Printf("Execution error: %v", err)
    return
}
```
