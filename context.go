package wirefilter

import (
	"context"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// FuncHandler is the type for user-defined function handlers.
// The context parameter carries cancellation and deadline signals
// from ExecutionContext.WithContext, enabling handlers to propagate
// request-scoped timeouts to downstream operations (e.g., database queries, HTTP calls).
type FuncHandler func(ctx context.Context, args []Value) (Value, error)

// TraceNode represents the evaluation trace of a single expression node.
type TraceNode struct {
	Expression string        `json:"expression"`
	Result     interface{}   `json:"result"`
	Duration   time.Duration `json:"duration,omitempty"`
	Children   []*TraceNode  `json:"children,omitempty"`
}

func (t *TraceNode) addChild(child *TraceNode) {
	t.Children = append(t.Children, child)
}

// ExecutionContext holds the runtime values for fields that are evaluated during filter execution.
// ExecutionContext is safe for concurrent use across goroutines. Multiple filters can be
// executed concurrently against the same context. Setup methods (Set*) can be called
// concurrently with each other and with Execute calls.
//
// For maximum evaluation performance, call Snapshot() to create a frozen, lock-free copy
// of the context. Snapshots are immutable and skip all mutex operations on the hot path.
type ExecutionContext struct {
	mu     sync.RWMutex
	frozen bool // when true, skip all mutex operations (immutable snapshot)
	fields map[string]Value
	lists  map[string]ArrayValue
	sets   map[string]SetValue // auto-promoted lists for O(1) membership
	tables map[string]MapValue
	funcs  map[string]FuncHandler

	nowFunc func() time.Time // injectable clock for now()

	// Evaluation options
	goCtx context.Context // cancellation/timeout

	traceMu    sync.Mutex
	traceRoot  *TraceNode   // trace tree root
	traceStack []*TraceNode // current trace path

	cacheMu      sync.Mutex
	cacheEnabled bool             // enable result caching
	cacheMaxSize int              // max cache entries (0 = default 1024)
	cache        map[string]Value // cached function results
}

const defaultCacheMaxSize = 1024

// NewExecutionContext creates a new execution context.
// If field maps are provided, initializes the context with those field values.
// Multiple field maps can be provided and will be merged.
// Otherwise, creates an empty context.
func NewExecutionContext(fields ...map[string]Value) *ExecutionContext {
	ctx := &ExecutionContext{
		fields: make(map[string]Value),
		lists:  make(map[string]ArrayValue),
		sets:   make(map[string]SetValue),
		tables: make(map[string]MapValue),
	}
	for _, fieldMap := range fields {
		for name, value := range fieldMap {
			ctx.fields[name] = value
		}
	}
	return ctx
}

// Snapshot creates a frozen, immutable copy of the execution context.
// The snapshot skips all mutex operations during evaluation, providing
// maximum performance for the common pattern of evaluating many filters
// against the same request data.
//
// The snapshot shares the same Go context, clock function, and cache settings.
// Cache state is independent (a new cache is created if caching was enabled).
// Tracing is not carried over; call EnableTrace on the snapshot if needed.
//
// The snapshot is read-only. Calling any Set* method on a snapshot is a no-op.
func (ctx *ExecutionContext) Snapshot() *ExecutionContext {
	ctx.readLock()
	fields := make(map[string]Value, len(ctx.fields))
	for k, v := range ctx.fields {
		fields[k] = v
	}
	lists := make(map[string]ArrayValue, len(ctx.lists))
	for k, v := range ctx.lists {
		lists[k] = v
	}
	sets := make(map[string]SetValue, len(ctx.sets))
	for k, v := range ctx.sets {
		sets[k] = v
	}
	tables := make(map[string]MapValue, len(ctx.tables))
	for k, v := range ctx.tables {
		tables[k] = v
	}
	var funcs map[string]FuncHandler
	if ctx.funcs != nil {
		funcs = make(map[string]FuncHandler, len(ctx.funcs))
		for k, v := range ctx.funcs {
			funcs[k] = v
		}
	}
	nowFunc := ctx.nowFunc
	goCtx := ctx.goCtx
	ctx.readUnlock()

	ctx.cacheMu.Lock()
	cacheEnabled := ctx.cacheEnabled
	cacheMaxSize := ctx.cacheMaxSize
	ctx.cacheMu.Unlock()

	snap := &ExecutionContext{
		frozen:       true,
		fields:       fields,
		lists:        lists,
		sets:         sets,
		tables:       tables,
		funcs:        funcs,
		nowFunc:      nowFunc,
		goCtx:        goCtx,
		cacheEnabled: cacheEnabled,
		cacheMaxSize: cacheMaxSize,
	}
	if cacheEnabled {
		snap.cache = make(map[string]Value)
	}
	return snap
}

// Frozen returns true if this context is an immutable snapshot.
func (ctx *ExecutionContext) Frozen() bool {
	return ctx.frozen
}

// writeLock acquires the write lock. Returns false if the context is frozen (read-only).
func (ctx *ExecutionContext) writeLock() bool {
	if ctx.frozen {
		return false
	}
	ctx.mu.Lock()
	return true
}

func (ctx *ExecutionContext) readLock() {
	if !ctx.frozen {
		ctx.mu.RLock()
	}
}
func (ctx *ExecutionContext) readUnlock() {
	if !ctx.frozen {
		ctx.mu.RUnlock()
	}
}

// SetField sets a field value in the execution context.
// Returns the context to allow method chaining.
// No-op if called on a frozen snapshot.
func (ctx *ExecutionContext) SetField(name string, value Value) *ExecutionContext {
	if !ctx.writeLock() {
		return ctx
	}
	ctx.fields[name] = value
	ctx.mu.Unlock()
	return ctx
}

// SetStringField sets a string field value in the execution context.
// Returns the context to allow method chaining.
func (ctx *ExecutionContext) SetStringField(name string, value string) *ExecutionContext {
	if !ctx.writeLock() {
		return ctx
	}
	ctx.fields[name] = StringValue(value)
	ctx.mu.Unlock()
	return ctx
}

// SetIntField sets an integer field value in the execution context.
// Returns the context to allow method chaining.
func (ctx *ExecutionContext) SetIntField(name string, value int64) *ExecutionContext {
	if !ctx.writeLock() {
		return ctx
	}
	ctx.fields[name] = IntValue(value)
	ctx.mu.Unlock()
	return ctx
}

// SetFloatField sets a floating-point field value in the execution context.
// Returns the context to allow method chaining.
func (ctx *ExecutionContext) SetFloatField(name string, value float64) *ExecutionContext {
	if !ctx.writeLock() {
		return ctx
	}
	ctx.fields[name] = FloatValue(value)
	ctx.mu.Unlock()
	return ctx
}

// SetBoolField sets a boolean field value in the execution context.
// Returns the context to allow method chaining.
func (ctx *ExecutionContext) SetBoolField(name string, value bool) *ExecutionContext {
	if !ctx.writeLock() {
		return ctx
	}
	ctx.fields[name] = BoolValue(value)
	ctx.mu.Unlock()
	return ctx
}

// SetIPField sets an IP address field value in the execution context.
// The value string will be parsed as an IP address.
// Returns the context to allow method chaining.
func (ctx *ExecutionContext) SetIPField(name string, value string) *ExecutionContext {
	ip := NormalizeIP(net.ParseIP(value))
	if ip != nil {
		if !ctx.writeLock() {
			return ctx
		}
		ctx.fields[name] = IPValue{IP: ip}
		ctx.mu.Unlock()
	}
	return ctx
}

// SetBytesField sets a bytes field value in the execution context.
// Returns the context to allow method chaining.
func (ctx *ExecutionContext) SetBytesField(name string, value []byte) *ExecutionContext {
	if !ctx.writeLock() {
		return ctx
	}
	ctx.fields[name] = BytesValue(value)
	ctx.mu.Unlock()
	return ctx
}

// SetTimeField sets a time field value in the execution context.
// Returns the context to allow method chaining.
func (ctx *ExecutionContext) SetTimeField(name string, value time.Time) *ExecutionContext {
	if !ctx.writeLock() {
		return ctx
	}
	ctx.fields[name] = NewTimeValue(value.UTC())
	ctx.mu.Unlock()
	return ctx
}

// SetDurationField sets a duration field value in the execution context.
// Returns the context to allow method chaining.
func (ctx *ExecutionContext) SetDurationField(name string, value time.Duration) *ExecutionContext {
	if !ctx.writeLock() {
		return ctx
	}
	ctx.fields[name] = DurationValue(value)
	ctx.mu.Unlock()
	return ctx
}

// WithNow sets an injectable clock function for the now() built-in.
// If not set, now() returns time.Now().UTC().
// Returns the context to allow method chaining.
func (ctx *ExecutionContext) WithNow(fn func() time.Time) *ExecutionContext {
	if !ctx.writeLock() {
		return ctx
	}
	ctx.nowFunc = fn
	ctx.mu.Unlock()
	return ctx
}

// now returns the current time from the injectable clock or time.Now().UTC().
func (ctx *ExecutionContext) now() time.Time {
	ctx.readLock()
	fn := ctx.nowFunc
	ctx.readUnlock()
	if fn != nil {
		return fn()
	}
	return time.Now().UTC()
}

// SetMapField sets a map field value in the execution context.
// Accepts map[string]string and converts values to StringValue.
// Returns the context to allow method chaining.
func (ctx *ExecutionContext) SetMapField(name string, value map[string]string) *ExecutionContext {
	m := make(MapValue, len(value))
	for k, v := range value {
		m[k] = StringValue(v)
	}
	if !ctx.writeLock() {
		return ctx
	}
	ctx.fields[name] = m
	ctx.mu.Unlock()
	return ctx
}

// SetMapFieldValues sets a map field with Value types in the execution context.
// Returns the context to allow method chaining.
func (ctx *ExecutionContext) SetMapFieldValues(name string, value map[string]Value) *ExecutionContext {
	if !ctx.writeLock() {
		return ctx
	}
	ctx.fields[name] = MapValue(value)
	ctx.mu.Unlock()
	return ctx
}

// SetMapArrayField sets a map field where each key maps to an array of Values.
// This supports any value types in the arrays (strings, ints, floats, IPs, CIDRs, etc.).
// Useful for HTTP headers, ACL rules, and similar map[string][]T structures.
// Returns the context to allow method chaining.
func (ctx *ExecutionContext) SetMapArrayField(name string, value map[string][]Value) *ExecutionContext {
	m := make(MapValue, len(value))
	for k, values := range value {
		m[k] = ArrayValue(values)
	}
	if !ctx.writeLock() {
		return ctx
	}
	ctx.fields[name] = m
	ctx.mu.Unlock()
	return ctx
}

// GetField retrieves a field value from the execution context.
// Returns the value and true if found, or nil and false if not found.
func (ctx *ExecutionContext) GetField(name string) (Value, bool) {
	ctx.readLock()
	val, ok := ctx.fields[name]
	ctx.readUnlock()
	return val, ok
}

// SetArrayField sets an array of string values as an ArrayValue field.
// Returns the context to allow method chaining.
func (ctx *ExecutionContext) SetArrayField(name string, values []string) *ExecutionContext {
	arr := make(ArrayValue, len(values))
	for i, v := range values {
		arr[i] = StringValue(v)
	}
	if !ctx.writeLock() {
		return ctx
	}
	ctx.fields[name] = arr
	ctx.mu.Unlock()
	return ctx
}

// SetIntArrayField sets an array of integer values as an ArrayValue field.
// Returns the context to allow method chaining.
func (ctx *ExecutionContext) SetIntArrayField(name string, values []int64) *ExecutionContext {
	arr := make(ArrayValue, len(values))
	for i, v := range values {
		arr[i] = IntValue(v)
	}
	if !ctx.writeLock() {
		return ctx
	}
	ctx.fields[name] = arr
	ctx.mu.Unlock()
	return ctx
}

// SetList sets a string list in the execution context.
// Lists with 16 or more elements are automatically indexed for O(1) membership lookups.
// Returns the context to allow method chaining.
func (ctx *ExecutionContext) SetList(name string, values []string) *ExecutionContext {
	arr := make(ArrayValue, len(values))
	for i, v := range values {
		arr[i] = StringValue(v)
	}
	if !ctx.writeLock() {
		return ctx
	}
	ctx.lists[name] = arr
	if len(arr) >= setAutoPromoteThreshold {
		ctx.sets[name] = NewSetValue(arr)
	} else {
		delete(ctx.sets, name)
	}
	ctx.mu.Unlock()
	return ctx
}

// SetIPList sets an IP address list in the execution context.
// Values can be plain IPs (e.g., "10.0.0.1") or CIDR ranges (e.g., "10.0.0.0/8").
// Lists with 16 or more elements are automatically indexed for O(1) membership lookups.
// Returns the context to allow method chaining.
func (ctx *ExecutionContext) SetIPList(name string, values []string) *ExecutionContext {
	arr := make(ArrayValue, 0, len(values))
	for _, v := range values {
		if _, ipNet, err := net.ParseCIDR(v); err == nil {
			arr = append(arr, CIDRValue{IPNet: ipNet})
			continue
		}
		if ip := NormalizeIP(net.ParseIP(v)); ip != nil {
			arr = append(arr, IPValue{IP: ip})
		}
	}
	if !ctx.writeLock() {
		return ctx
	}
	ctx.lists[name] = arr
	if len(arr) >= setAutoPromoteThreshold {
		ctx.sets[name] = NewSetValue(arr)
	} else {
		delete(ctx.sets, name)
	}
	ctx.mu.Unlock()
	return ctx
}

// GetList retrieves a list from the execution context.
// Returns the list and true if found, or nil and false if not found.
func (ctx *ExecutionContext) GetList(name string) (ArrayValue, bool) {
	ctx.readLock()
	val, ok := ctx.lists[name]
	ctx.readUnlock()
	return val, ok
}

// getSet retrieves a set-indexed list from the execution context.
// Returns the set and true if found, or zero value and false if not found.
func (ctx *ExecutionContext) getSet(name string) (SetValue, bool) {
	ctx.readLock()
	val, ok := ctx.sets[name]
	ctx.readUnlock()
	return val, ok
}

// SetTable sets a lookup table with string values in the execution context.
// Tables are referenced in expressions with $table_name[field] syntax.
// Returns the context to allow method chaining.
func (ctx *ExecutionContext) SetTable(name string, data map[string]string) *ExecutionContext {
	m := make(MapValue, len(data))
	for k, v := range data {
		m[k] = StringValue(v)
	}
	if !ctx.writeLock() {
		return ctx
	}
	ctx.tables[name] = m
	ctx.mu.Unlock()
	return ctx
}

// SetTableValues sets a lookup table with mixed value types.
// Returns the context to allow method chaining.
func (ctx *ExecutionContext) SetTableValues(name string, data map[string]Value) *ExecutionContext {
	if !ctx.writeLock() {
		return ctx
	}
	ctx.tables[name] = MapValue(data)
	ctx.mu.Unlock()
	return ctx
}

// SetTableList sets a lookup table where each key maps to a string array.
// Returns the context to allow method chaining.
func (ctx *ExecutionContext) SetTableList(name string, data map[string][]string) *ExecutionContext {
	m := make(MapValue, len(data))
	for k, values := range data {
		arr := make(ArrayValue, len(values))
		for i, v := range values {
			arr[i] = StringValue(v)
		}
		m[k] = arr
	}
	if !ctx.writeLock() {
		return ctx
	}
	ctx.tables[name] = m
	ctx.mu.Unlock()
	return ctx
}

// SetTableIPList sets a lookup table where each key maps to an IP/CIDR array.
// Values can be plain IPs or CIDR ranges.
// Returns the context to allow method chaining.
func (ctx *ExecutionContext) SetTableIPList(name string, data map[string][]string) *ExecutionContext {
	m := make(MapValue, len(data))
	for k, values := range data {
		arr := make(ArrayValue, 0, len(values))
		for _, v := range values {
			if _, ipNet, err := net.ParseCIDR(v); err == nil {
				arr = append(arr, CIDRValue{IPNet: ipNet})
				continue
			}
			if ip := NormalizeIP(net.ParseIP(v)); ip != nil {
				arr = append(arr, IPValue{IP: ip})
			}
		}
		m[k] = arr
	}
	if !ctx.writeLock() {
		return ctx
	}
	ctx.tables[name] = m
	ctx.mu.Unlock()
	return ctx
}

// GetTable retrieves a lookup table from the execution context.
// Returns the table and true if found, or nil and false if not found.
func (ctx *ExecutionContext) GetTable(name string) (MapValue, bool) {
	ctx.readLock()
	val, ok := ctx.tables[name]
	ctx.readUnlock()
	return val, ok
}

// SetFunc registers a user-defined function handler in the execution context.
// The handler will be called when the function is invoked in a filter expression.
// Returns the context to allow method chaining.
func (ctx *ExecutionContext) SetFunc(name string, handler FuncHandler) *ExecutionContext {
	if !ctx.writeLock() {
		return ctx
	}
	if ctx.funcs == nil {
		ctx.funcs = make(map[string]FuncHandler)
	}
	ctx.funcs[strings.ToLower(name)] = handler
	ctx.mu.Unlock()
	return ctx
}

// GetFunc retrieves a user-defined function handler from the execution context.
// Returns the handler and true if found, or nil and false if not found.
func (ctx *ExecutionContext) GetFunc(name string) (FuncHandler, bool) {
	ctx.readLock()
	defer ctx.readUnlock()
	if ctx.funcs == nil {
		return nil, false
	}
	fn, ok := ctx.funcs[strings.ToLower(name)]
	return fn, ok
}

// WithContext sets a Go context for cancellation and timeout support.
// The evaluator checks for context cancellation at key evaluation points.
// Returns the context to allow method chaining.
func (ctx *ExecutionContext) WithContext(goCtx context.Context) *ExecutionContext {
	if !ctx.writeLock() {
		return ctx
	}
	ctx.goCtx = goCtx
	ctx.mu.Unlock()
	return ctx
}

// EnableTrace enables expression evaluation tracing.
// After Execute, call Trace() to retrieve the evaluation trace tree.
//
// Tracing is designed for single-execution use. When tracing is enabled on a shared
// context, concurrent Execute calls will interleave their trace entries, producing
// a corrupted trace tree. For concurrent evaluation with tracing, use a separate
// ExecutionContext per goroutine.
//
// Returns the context to allow method chaining.
func (ctx *ExecutionContext) EnableTrace() *ExecutionContext {
	ctx.traceMu.Lock()
	ctx.traceRoot = &TraceNode{Expression: "root"}
	ctx.traceStack = []*TraceNode{ctx.traceRoot}
	ctx.traceMu.Unlock()
	return ctx
}

// Trace returns the evaluation trace tree after Execute completes.
// Returns nil if tracing was not enabled.
func (ctx *ExecutionContext) Trace() *TraceNode {
	ctx.traceMu.Lock()
	root := ctx.traceRoot
	ctx.traceMu.Unlock()
	return root
}

// EnableCache enables result caching for user-defined function calls.
// Repeated calls to the same function with the same arguments return cached results.
// The cache persists across multiple Execute() calls on the same context,
// which is useful for evaluating many rules against the same request.
// Default max size is 1024 entries; use SetCacheMaxSize to change.
// Returns the context to allow method chaining.
func (ctx *ExecutionContext) EnableCache() *ExecutionContext {
	ctx.cacheMu.Lock()
	ctx.cacheEnabled = true
	ctx.cacheMaxSize = defaultCacheMaxSize
	ctx.cache = make(map[string]Value)
	ctx.cacheMu.Unlock()
	return ctx
}

// SetCacheMaxSize sets the maximum number of cached function results.
// When the cache is full, new entries are not cached (existing entries are kept).
// Must be called after EnableCache. Zero resets to default (1024).
// Returns the context to allow method chaining.
func (ctx *ExecutionContext) SetCacheMaxSize(size int) *ExecutionContext {
	if size <= 0 {
		size = defaultCacheMaxSize
	}
	ctx.cacheMu.Lock()
	ctx.cacheMaxSize = size
	ctx.cacheMu.Unlock()
	return ctx
}

// ResetCache clears all cached function results.
// Useful between batches of rule evaluations to free memory.
// Returns the context to allow method chaining.
func (ctx *ExecutionContext) ResetCache() *ExecutionContext {
	ctx.cacheMu.Lock()
	if ctx.cache != nil {
		clear(ctx.cache)
	}
	ctx.cacheMu.Unlock()
	return ctx
}

// CacheLen returns the number of entries currently in the cache.
func (ctx *ExecutionContext) CacheLen() int {
	ctx.cacheMu.Lock()
	n := len(ctx.cache)
	ctx.cacheMu.Unlock()
	return n
}

// Context returns the Go context associated with this execution context.
// Returns context.Background() if no context was set via WithContext.
func (ctx *ExecutionContext) Context() context.Context {
	ctx.readLock()
	goCtx := ctx.goCtx
	ctx.readUnlock()
	if goCtx == nil {
		return context.Background()
	}
	return goCtx
}

// checkContext checks if the Go context has been cancelled or timed out.
func (ctx *ExecutionContext) checkContext() error {
	ctx.readLock()
	goCtx := ctx.goCtx
	ctx.readUnlock()
	if goCtx == nil {
		return nil
	}
	select {
	case <-goCtx.Done():
		return goCtx.Err()
	default:
		return nil
	}
}

// traceEnabled returns true if tracing is active.
func (ctx *ExecutionContext) traceEnabled() bool {
	ctx.traceMu.Lock()
	enabled := ctx.traceRoot != nil
	ctx.traceMu.Unlock()
	return enabled
}

// pushTrace starts tracing a sub-expression.
func (ctx *ExecutionContext) pushTrace(expr string) {
	ctx.traceMu.Lock()
	node := &TraceNode{Expression: expr}
	parent := ctx.traceStack[len(ctx.traceStack)-1]
	parent.addChild(node)
	ctx.traceStack = append(ctx.traceStack, node)
	ctx.traceMu.Unlock()
}

// popTrace completes tracing a sub-expression with its result.
func (ctx *ExecutionContext) popTrace(result Value, dur time.Duration) {
	ctx.traceMu.Lock()
	node := ctx.traceStack[len(ctx.traceStack)-1]
	ctx.traceStack = ctx.traceStack[:len(ctx.traceStack)-1]
	if result != nil {
		node.Result = result.String()
	}
	node.Duration = dur
	ctx.traceMu.Unlock()
}

// getCached retrieves a cached function result.
func (ctx *ExecutionContext) getCached(key string) (Value, bool) {
	ctx.cacheMu.Lock()
	if !ctx.cacheEnabled {
		ctx.cacheMu.Unlock()
		return nil, false
	}
	v, ok := ctx.cache[key]
	ctx.cacheMu.Unlock()
	return v, ok
}

// setCache stores a function result in the cache, respecting max size.
func (ctx *ExecutionContext) setCache(key string, val Value) {
	ctx.cacheMu.Lock()
	if !ctx.cacheEnabled || len(ctx.cache) >= ctx.cacheMaxSize {
		ctx.cacheMu.Unlock()
		return
	}
	ctx.cache[key] = val
	ctx.cacheMu.Unlock()
}

// Export returns a flat map of field names to their values for use in audit logs.
// The output uses native Go types that json.Marshal handles directly.
func (ctx *ExecutionContext) Export() map[string]any {
	ctx.readLock()
	result := make(map[string]any, len(ctx.fields))
	for name, val := range ctx.fields {
		result[name] = exportValue(val)
	}
	ctx.readUnlock()
	return result
}

// ExportLists returns a flat map of list names to their values for use in audit logs.
func (ctx *ExecutionContext) ExportLists() map[string]any {
	ctx.readLock()
	result := make(map[string]any, len(ctx.lists))
	for name, list := range ctx.lists {
		result[name] = exportValue(list)
	}
	ctx.readUnlock()
	return result
}

// exportValue converts a Value to a JSON-friendly representation.
func exportValue(v Value) any {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case StringValue:
		return string(val)
	case IntValue:
		return int64(val)
	case FloatValue:
		return float64(val)
	case BoolValue:
		return bool(val)
	case IPValue:
		return val.IP.String()
	case CIDRValue:
		return val.IPNet.String()
	case BytesValue:
		return string(val)
	case TimeValue:
		return val.GoTime().Format(time.RFC3339Nano)
	case DurationValue:
		return val.String()
	case ArrayValue:
		items := make([]any, len(val))
		for i, elem := range val {
			items[i] = exportValue(elem)
		}
		return items
	case MapValue:
		m := make(map[string]any, len(val))
		for k, elem := range val {
			m[k] = exportValue(elem)
		}
		return m
	default:
		return v.String()
	}
}

var cacheKeyPool = sync.Pool{
	New: func() any { return &strings.Builder{} },
}

// cacheKey builds a cache key for a function call.
// It includes value types at every level of the value tree to prevent
// collisions across different types, including nested elements in
// arrays and maps.
func cacheKey(name string, args []Value) string {
	sb := cacheKeyPool.Get().(*strings.Builder)
	sb.Reset()
	sb.WriteString(name)
	for _, arg := range args {
		sb.WriteByte(':')
		writeCacheKeyValue(sb, arg)
	}
	key := sb.String()
	cacheKeyPool.Put(sb)
	return key
}

// writeCacheKeyValue writes a type-tagged canonical representation of a value
// into a string builder, recursively handling arrays and maps.
func writeCacheKeyValue(sb *strings.Builder, v Value) {
	if v == nil {
		sb.WriteString("nil")
		return
	}
	sb.WriteString(v.Type().String())
	sb.WriteByte('=')
	switch val := v.(type) {
	case ArrayValue:
		sb.WriteByte('[')
		for i, elem := range val {
			if i > 0 {
				sb.WriteString(", ")
			}
			writeCacheKeyValue(sb, elem)
		}
		sb.WriteByte(']')
	case MapValue:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		sb.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(k)
			sb.WriteByte(':')
			writeCacheKeyValue(sb, val[k])
		}
		sb.WriteByte('}')
	default:
		sb.WriteString(v.String())
	}
}
