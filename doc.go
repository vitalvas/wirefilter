// Package wirefilter implements a filtering expression language and execution engine.
// It allows you to compile and evaluate filter expressions against runtime data.
//
// # Operators
//
// Logical (short-circuit where applicable):
//
//	and, &&    - logical AND (short-circuits on false)
//	or,  ||    - logical OR  (short-circuits on true)
//	not, !     - logical NOT (unary)
//	xor, ^^    - logical XOR (evaluates both sides)
//
// Comparison:
//
//	==, !=, <, >, <=, >=
//
// Array comparison:
//
//	===    - all elements equal
//	!==    - any element not equal
//
// Membership and containment:
//
//	in               - value in array, interval, or CIDR
//	contains         - containment check
//	matches, ~       - regex matching (can be disabled via DisableRegex)
//	wildcard         - case-insensitive glob matching
//	strict wildcard  - case-sensitive glob matching
//
// Arithmetic:
//
//	+, -, *, /, %
//
// # Data Types
//
// The following value types are supported:
//
//   - StringValue: UTF-8 strings
//   - IntValue: 64-bit signed integers
//   - FloatValue: 64-bit floating-point numbers
//   - BoolValue: boolean true/false
//   - IPValue: IPv4 and IPv6 addresses (normalized to 16-byte representation)
//   - CIDRValue: IP network ranges (e.g., "10.0.0.0/8")
//   - BytesValue: raw byte sequences
//   - ArrayValue: ordered collections of any value type
//   - MapValue: string-keyed maps of any value type
//   - TimeValue: nanosecond-precision timestamps
//   - DurationValue: time durations with day-level granularity (e.g., "2d4h30m")
//
// # Temporal Arithmetic
//
// Time and duration types support arithmetic operations:
//
//	time +/- duration = time
//	time - time       = duration
//	duration +/- duration = duration
//	duration * (int|float) = duration
//	(int|float) * duration = duration
//	duration / (int|float) = duration
//	duration / duration    = int
//	duration % duration    = duration
//
// Duration literals support negative values (e.g., -30m, -2d4h).
//
// # Expression Syntax
//
// Field access:
//
//	field_name           - direct field reference
//	map["key"]           - map value by string key
//	array[0]             - array element by index (negative indices and out-of-bounds return nil)
//	array[*]             - unpack array for element-wise operations (ANY semantics)
//
// Range expressions:
//
//	{1..10}              - integer/time/duration interval for use with "in" operator
//
// Custom lists (populated via SetList or SetIPList):
//
//	$list_name           - reference a named list for membership checks
//
// Lookup tables (populated via SetTable, SetTableValues, SetTableList, SetTableIPList):
//
//	$table_name[field]   - resolve a per-key value from a lookup table
//
// Raw strings:
//
//	r"no escape\nprocessing"
//
// # Built-in Functions
//
// String functions:
//
//	lower(str), upper(str)
//	len(str|array|map|bytes)
//	starts_with(str, prefix), ends_with(str, suffix)
//	concat(str...), substring(str, start [, end])
//	split(str, sep), join(array, sep)
//	trim(str), trim_left(str), trim_right(str)
//	replace(str, old, new), url_decode(str)
//	regex_replace(str, pattern, replacement)  - disabled by DisableRegex
//	regex_extract(str, pattern)               - disabled by DisableRegex
//	contains_word(str, word)                  - disabled by DisableRegex
//
// IP/network functions:
//
//	is_ipv4(ip), is_ipv6(ip), is_loopback(ip)
//	cidr(ip, bits)   - apply IPv4 CIDR mask
//	cidr6(ip, bits)  - apply IPv6 CIDR mask
//
// Math functions:
//
//	abs(int|float), ceil(float), floor(float), round(float)
//
// Array set operations:
//
//	intersection(array, array), union(array, array), difference(array, array)
//	contains_any(array, array), contains_all(array, array)
//	count(array)         - count truthy elements
//	has_value(array, value), has_key(map, key)
//
// Utility functions:
//
//	coalesce(val...)     - first non-nil value
//	exists(val)          - true if value is not nil
//	now()                - current time (injectable via WithNow)
//	any(expr)            - true if any unpacked element matches
//	all(expr)            - true if all unpacked elements match
//
// # Schema
//
// A Schema defines the structure and types of fields available in filter expressions.
// It provides compile-time validation of field references, operator compatibility,
// and expression complexity.
//
//	schema := wirefilter.NewSchema().
//	    AddField("http.host", wirefilter.TypeString).
//	    AddField("http.status", wirefilter.TypeInt).
//	    AddArrayField("tags", wirefilter.TypeString).
//	    AddMapField("headers", wirefilter.TypeString)
//
// Function availability can be controlled via allowlist or blocklist modes:
//
//	schema.SetFunctionMode(wirefilter.FunctionModeAllowlist).
//	    EnableFunctions("lower", "upper", "len")
//
// Or in blocklist mode (default), disable specific functions:
//
//	schema.DisableFunctions("regex_replace", "regex_extract")
//
// Regex support can be entirely disabled:
//
//	schema.DisableRegex()
//
// Expression complexity limits prevent resource exhaustion:
//
//	schema.SetMaxDepth(10).SetMaxNodes(100)
//
// User-defined functions are registered on the schema for compile-time type validation:
//
//	schema.RegisterFunction("normalize", wirefilter.TypeString, []wirefilter.Type{wirefilter.TypeString})
//
// # Compilation and Execution
//
// Compile parses a filter expression and validates it against the schema:
//
//	filter, err := wirefilter.Compile(`http.host == "example.com" and http.status >= 400`, schema)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// Execute evaluates the filter against an ExecutionContext:
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
//
// Both Filter and ExecutionContext are safe for concurrent use across goroutines.
//
// # User-Defined Functions
//
// Register the function signature on the schema and bind the handler on the context:
//
//	schema.RegisterFunction("normalize", wirefilter.TypeString, []wirefilter.Type{wirefilter.TypeString})
//
//	filter, _ := wirefilter.Compile(`normalize(http.host) == "example.com"`, schema)
//
//	ctx := wirefilter.NewExecutionContext().
//	    SetStringField("http.host", "EXAMPLE.COM").
//	    SetFunc("normalize", func(_ context.Context, args []wirefilter.Value) (wirefilter.Value, error) {
//	        return wirefilter.StringValue(strings.ToLower(string(args[0].(wirefilter.StringValue)))), nil
//	    })
//
// The FuncHandler receives a Go context from WithContext, enabling timeout propagation
// to downstream operations (e.g., database queries, HTTP calls).
//
// # Custom Lists
//
// String and IP/CIDR lists for membership checks:
//
//	filter, _ := wirefilter.Compile(`http.host in $blocked_hosts or ip.src in $blocked_ips`, schema)
//
//	ctx := wirefilter.NewExecutionContext().
//	    SetList("blocked_hosts", []string{"malware.example.com", "phishing.test.net"}).
//	    SetIPList("blocked_ips", []string{"203.0.113.0/24", "10.0.0.1"})
//
// # Lookup Tables
//
// Tables resolve per-key values at runtime:
//
//	filter, _ := wirefilter.Compile(`$roles[user.id] == "admin"`, schema)
//
//	ctx := wirefilter.NewExecutionContext().
//	    SetStringField("user.id", "user-42").
//	    SetTable("roles", map[string]string{"user-42": "admin"})
//
// Tables also support mixed value types (SetTableValues), string arrays (SetTableList),
// and IP/CIDR arrays (SetTableIPList).
//
// # Execution Timeout
//
// Attach a Go context to enable cancellation and deadline support:
//
//	goCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
//	defer cancel()
//
//	ctx := wirefilter.NewExecutionContext().
//	    WithContext(goCtx)
//
// The evaluator checks for context cancellation at key evaluation points.
//
// # Injectable Clock
//
// Override the now() function for deterministic evaluation:
//
//	ctx := wirefilter.NewExecutionContext().
//	    WithNow(func() time.Time { return fixedTime })
//
// # Evaluation Tracing
//
// Capture a full evaluation tree with per-node timing:
//
//	ctx := wirefilter.NewExecutionContext().
//	    EnableTrace()
//	result, _ := filter.Execute(ctx)
//	trace := ctx.Trace() // *TraceNode with Expression, Result, Duration, Children
//
// # Function Result Caching
//
// Cache results of user-defined function calls across multiple Execute calls
// on the same context (useful for evaluating many rules against one request):
//
//	ctx := wirefilter.NewExecutionContext().
//	    EnableCache().
//	    SetCacheMaxSize(2048)  // default is 1024
//
//	// After batch evaluation:
//	ctx.ResetCache()
//
// Cache keys are type-aware at every nesting level to prevent collisions.
//
// # Rule Metadata
//
// Attach an ID and string tags to compiled filters:
//
//	filter.SetMeta(wirefilter.RuleMeta{
//	    ID:   "rule-1",
//	    Tags: map[string]string{"severity": "high"},
//	})
//	meta := filter.Meta()
//
// Tags are defensively copied to prevent external mutation.
//
// # Filter Hashing
//
// Compute a deterministic FNV-128a hash of the compiled AST for deduplication:
//
//	hash := filter.Hash()
//
// Semantically identical expressions produce the same hash regardless of
// whitespace, operator aliases (and vs &&), or formatting differences.
//
// # Binary Serialization
//
// Serialize compiled filters for storage or network transfer:
//
//	data, err := filter.MarshalBinary()  // "WF" magic header + version byte + AST
//	restored := &wirefilter.Filter{}
//	err = restored.UnmarshalBinary(data)
//
// # Context Export
//
// Export field and list values as JSON-friendly maps for audit logging:
//
//	fields := ctx.Export()       // map[string]any
//	lists  := ctx.ExportLists()  // map[string]any
//
// # Type Coercion
//
// Equality comparisons support automatic type coercion:
//
//	IP == string:   parses string as IP address
//	CIDR == string: parses string as CIDR range
//	Time == string: parses string as RFC3339Nano
//
// Coercion is bidirectional.
//
// # IP Matching Semantics
//
//	IP in CIDR:        direct containment check
//	IP in array:       matches against IP or CIDR elements
//	array in array:    OR logic (any element matches)
//	array contains array: AND logic (all elements present)
//
// # Null Handling
//
//	nil == nil returns true.
//	Absent fields return nil.
//	nil with most operators returns false or nil.
//
// # Concurrency
//
// Both Filter and ExecutionContext are safe for concurrent use across goroutines.
// Filter protects regex and CIDR caches with sync.RWMutex.
// ExecutionContext protects data maps (fields, lists, tables, funcs) with sync.RWMutex,
// and cache and trace state with dedicated sync.Mutex locks.
// Multiple filters can be executed concurrently against the same context.
package wirefilter
