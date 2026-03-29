package wirefilter

import (
	"fmt"
	"math"
	"net"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Type represents the data type of a value in the filter system.
type Type uint8

const (
	TypeString Type = iota
	TypeInt
	TypeFloat
	TypeBool
	TypeIP
	TypeCIDR
	TypeBytes
	TypeArray
	TypeMap
	TypeTime
	TypeDuration
)

// String returns the string representation of a data type.
func (t Type) String() string {
	switch t {
	case TypeString:
		return "string"
	case TypeInt:
		return "int"
	case TypeFloat:
		return "float"
	case TypeBool:
		return "bool"
	case TypeIP:
		return "ip"
	case TypeCIDR:
		return "cidr"
	case TypeBytes:
		return "bytes"
	case TypeArray:
		return "array"
	case TypeMap:
		return "map"
	case TypeTime:
		return "time"
	case TypeDuration:
		return "duration"
	default:
		return "unknown"
	}
}

// Value is the interface that all value types must implement.
type Value interface {
	Type() Type
	Equal(other Value) bool
	String() string
	IsTruthy() bool
}

// StringValue represents a string value.
type StringValue string

func (s StringValue) Type() Type     { return TypeString }
func (s StringValue) String() string { return string(s) }
func (s StringValue) IsTruthy() bool { return true }
func (s StringValue) Equal(v Value) bool {
	if v.Type() != TypeString {
		return false
	}
	return string(s) == string(v.(StringValue))
}

// IntValue represents an integer value.
type IntValue int64

func (i IntValue) Type() Type     { return TypeInt }
func (i IntValue) String() string { return fmt.Sprintf("%d", i) }
func (i IntValue) IsTruthy() bool { return true }
func (i IntValue) Equal(v Value) bool {
	if v.Type() != TypeInt {
		return false
	}
	return int64(i) == int64(v.(IntValue))
}

// FloatValue represents a floating-point value.
type FloatValue float64

func (f FloatValue) Type() Type     { return TypeFloat }
func (f FloatValue) String() string { return fmt.Sprintf("%g", f) }
func (f FloatValue) IsTruthy() bool { return true }
func (f FloatValue) Equal(v Value) bool {
	if v.Type() != TypeFloat {
		return false
	}
	return float64(f) == float64(v.(FloatValue))
}

// BoolValue represents a boolean value.
type BoolValue bool

func (b BoolValue) Type() Type     { return TypeBool }
func (b BoolValue) String() string { return fmt.Sprintf("%t", b) }
func (b BoolValue) IsTruthy() bool { return bool(b) }
func (b BoolValue) Equal(v Value) bool {
	if v.Type() != TypeBool {
		return false
	}
	return bool(b) == bool(v.(BoolValue))
}

// IPValue represents an IP address value (IPv4 or IPv6).
type IPValue struct {
	IP net.IP
}

func (ip IPValue) Type() Type     { return TypeIP }
func (ip IPValue) String() string { return ip.IP.String() }
func (ip IPValue) IsTruthy() bool { return true }
func (ip IPValue) Equal(v Value) bool {
	if v.Type() != TypeIP {
		return false
	}
	return ip.IP.Equal(v.(IPValue).IP)
}

// CIDRValue represents a CIDR network range (e.g., 192.168.0.0/24).
type CIDRValue struct {
	IPNet *net.IPNet
}

func (c CIDRValue) Type() Type     { return TypeCIDR }
func (c CIDRValue) String() string { return c.IPNet.String() }
func (c CIDRValue) IsTruthy() bool { return true }
func (c CIDRValue) Equal(v Value) bool {
	if v.Type() != TypeCIDR {
		return false
	}
	other := v.(CIDRValue)
	return c.IPNet.IP.Equal(other.IPNet.IP) && c.IPNet.Mask.String() == other.IPNet.Mask.String()
}

// Contains checks if an IP address is within this CIDR range.
func (c CIDRValue) Contains(ip net.IP) bool {
	return c.IPNet.Contains(ip)
}

// BytesValue represents a byte array value.
type BytesValue []byte

func (b BytesValue) Type() Type     { return TypeBytes }
func (b BytesValue) String() string { return string(b) }
func (b BytesValue) IsTruthy() bool { return true }
func (b BytesValue) Equal(v Value) bool {
	if v.Type() != TypeBytes {
		return false
	}
	other := v.(BytesValue)
	if len(b) != len(other) {
		return false
	}
	for i := range b {
		if b[i] != other[i] {
			return false
		}
	}
	return true
}

// ArrayValue represents an array of values.
type ArrayValue []Value

func (a ArrayValue) Type() Type     { return TypeArray }
func (a ArrayValue) IsTruthy() bool { return true }
func (a ArrayValue) String() string {
	parts := make([]string, len(a))
	for i, v := range a {
		if v == nil {
			parts[i] = "nil"
		} else {
			parts[i] = v.String()
		}
	}
	return fmt.Sprintf("[%s]", strings.Join(parts, ", "))
}
func (a ArrayValue) Equal(v Value) bool {
	if v == nil || v.Type() != TypeArray {
		return false
	}
	other := v.(ArrayValue)
	if len(a) != len(other) {
		return false
	}
	for i := range a {
		if a[i] == nil && other[i] == nil {
			continue
		}
		if a[i] == nil || other[i] == nil {
			return false
		}
		if !a[i].Equal(other[i]) {
			return false
		}
	}
	return true
}

// Contains checks if the array contains the specified value.
func (a ArrayValue) Contains(v Value) bool {
	for _, item := range a {
		if item == nil {
			if v == nil {
				return true
			}
			continue
		}
		if v == nil {
			continue // item is non-nil, v is nil - no match
		}
		if item.Equal(v) {
			return true
		}
	}
	return false
}

// setAutoPromoteThreshold is the minimum array size for automatic promotion to SetValue.
const setAutoPromoteThreshold = 16

// SetValue wraps an ArrayValue with a hash map for O(1) membership tests.
// It implements the Value interface with TypeArray for compatibility.
// IP and CIDR values require linear scan and are not indexed in the hash map.
type SetValue struct {
	Array  ArrayValue
	index  map[string]struct{}
	hasNet bool // true if array contains IP or CIDR values (require linear scan)
}

// NewSetValue creates a SetValue from an ArrayValue.
// String, Int, Float, Bool, Time, Duration values are indexed for O(1) lookup.
// IP and CIDR values are kept in the array for linear containment checks.
func NewSetValue(arr ArrayValue) SetValue {
	idx := make(map[string]struct{}, len(arr))
	var hasNet bool
	for _, v := range arr {
		if v == nil {
			continue
		}
		switch v.Type() {
		case TypeIP, TypeCIDR:
			hasNet = true
		default:
			idx[setKey(v)] = struct{}{}
		}
	}
	return SetValue{Array: arr, index: idx, hasNet: hasNet}
}

func setKey(v Value) string { return fmt.Sprintf("%s:%s", v.Type(), v.String()) }

func (s SetValue) Type() Type     { return TypeArray }
func (s SetValue) IsTruthy() bool { return true }
func (s SetValue) String() string { return s.Array.String() }
func (s SetValue) Equal(v Value) bool {
	if other, ok := v.(SetValue); ok {
		return s.Array.Equal(other.Array)
	}
	return s.Array.Equal(v)
}

// Contains checks if the set contains the specified value.
// Uses O(1) hash lookup for indexable types, falls back to linear scan for IP/CIDR.
func (s SetValue) Contains(v Value) bool {
	if v == nil {
		return s.Array.Contains(nil)
	}
	switch v.Type() {
	case TypeIP:
		if _, ok := s.index[setKey(v)]; ok {
			return true
		}
		if s.hasNet {
			ipVal := v.(IPValue)
			for _, elem := range s.Array {
				if elem != nil && elem.Type() == TypeCIDR {
					if elem.(CIDRValue).Contains(ipVal.IP) {
						return true
					}
				}
			}
		}
		return false
	case TypeCIDR:
		return s.Array.Contains(v)
	default:
		_, ok := s.index[setKey(v)]
		return ok
	}
}

// MapValue represents a map of string keys to Value.
type MapValue map[string]Value

func (m MapValue) Type() Type     { return TypeMap }
func (m MapValue) IsTruthy() bool { return true } // Present maps are truthy (field presence semantics)
func (m MapValue) String() string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(m))
	for _, k := range keys {
		v := m[k]
		if v == nil {
			parts = append(parts, fmt.Sprintf("%q: nil", k))
		} else {
			parts = append(parts, fmt.Sprintf("%q: %s", k, v.String()))
		}
	}
	return fmt.Sprintf("{%s}", strings.Join(parts, ", "))
}
func (m MapValue) Equal(v Value) bool {
	if v == nil || v.Type() != TypeMap {
		return false
	}
	other := v.(MapValue)
	if len(m) != len(other) {
		return false
	}
	for k, val := range m {
		otherVal, ok := other[k]
		if !ok {
			return false
		}
		if val == nil && otherVal == nil {
			continue
		}
		if val == nil || otherVal == nil {
			return false
		}
		if !val.Equal(otherVal) {
			return false
		}
	}
	return true
}

// Get retrieves a value from the map by key.
// Returns the value and true if found, or nil and false if not found.
func (m MapValue) Get(key string) (Value, bool) {
	val, ok := m[key]
	return val, ok
}

// TimeValue represents a point in time stored as nanoseconds since Unix epoch.
// Using int64 internally avoids heap allocations when boxed as a Value interface.
type TimeValue int64

// NewTimeValue creates a TimeValue from a time.Time.
func NewTimeValue(t time.Time) TimeValue {
	return TimeValue(t.UnixNano())
}

// GoTime converts the TimeValue back to a time.Time in UTC.
func (t TimeValue) GoTime() time.Time {
	return time.Unix(0, int64(t)).UTC()
}

func (t TimeValue) Type() Type     { return TypeTime }
func (t TimeValue) IsTruthy() bool { return true }
func (t TimeValue) String() string { return t.GoTime().Format(time.RFC3339Nano) }
func (t TimeValue) Equal(v Value) bool {
	if v.Type() != TypeTime {
		return false
	}
	return int64(t) == int64(v.(TimeValue))
}

// DurationValue represents a duration of time.
type DurationValue time.Duration

func (d DurationValue) Type() Type     { return TypeDuration }
func (d DurationValue) IsTruthy() bool { return true }
func (d DurationValue) Equal(v Value) bool {
	if v.Type() != TypeDuration {
		return false
	}
	return time.Duration(d) == time.Duration(v.(DurationValue))
}

// String returns a human-readable duration with day support and sub-second precision.
func (d DurationValue) String() string {
	dur := time.Duration(d)
	if dur == 0 {
		return "0s"
	}

	var sb strings.Builder
	if dur < 0 {
		sb.WriteByte('-')
		if dur == math.MinInt64 {
			dur = math.MaxInt64
		} else {
			dur = -dur
		}
	}

	days := dur / (24 * time.Hour)
	dur -= days * 24 * time.Hour
	hours := dur / time.Hour
	dur -= hours * time.Hour
	minutes := dur / time.Minute
	dur -= minutes * time.Minute
	seconds := dur / time.Second
	dur -= seconds * time.Second
	millis := dur / time.Millisecond
	dur -= millis * time.Millisecond
	micros := dur / time.Microsecond
	dur -= micros * time.Microsecond
	nanos := dur

	if days > 0 {
		fmt.Fprintf(&sb, "%dd", days)
	}
	if hours > 0 {
		fmt.Fprintf(&sb, "%dh", hours)
	}
	if minutes > 0 {
		fmt.Fprintf(&sb, "%dm", minutes)
	}
	if seconds > 0 {
		fmt.Fprintf(&sb, "%ds", seconds)
	}
	if millis > 0 {
		fmt.Fprintf(&sb, "%dms", millis)
	}
	if micros > 0 {
		fmt.Fprintf(&sb, "%dus", micros)
	}
	if nanos > 0 {
		fmt.Fprintf(&sb, "%dns", nanos)
	}

	if sb.Len() == 0 || (sb.Len() == 1 && sb.String()[0] == '-') {
		return "0s"
	}

	return sb.String()
}

// IntervalValue represents a non-materialized range for membership tests.
// Start and End are stored as raw int64 to avoid interface boxing allocations.
type IntervalValue struct {
	start    int64
	end      int64
	elemType Type
}

// NewTimeInterval creates an interval from two TimeValues.
func NewTimeInterval(start, end TimeValue) IntervalValue {
	return IntervalValue{start: int64(start), end: int64(end), elemType: TypeTime}
}

// NewDurationInterval creates an interval from two DurationValues.
func NewDurationInterval(start, end DurationValue) IntervalValue {
	return IntervalValue{start: int64(start), end: int64(end), elemType: TypeDuration}
}

// NewIntInterval creates an interval from two IntValues.
func NewIntInterval(start, end IntValue) IntervalValue {
	return IntervalValue{start: int64(start), end: int64(end), elemType: TypeInt}
}

func (iv IntervalValue) Type() Type     { return iv.elemType }
func (iv IntervalValue) IsTruthy() bool { return true }
func (iv IntervalValue) String() string {
	switch iv.elemType {
	case TypeInt:
		return fmt.Sprintf("%d..%d", iv.start, iv.end)
	case TypeTime:
		return fmt.Sprintf("%s..%s", TimeValue(iv.start).String(), TimeValue(iv.end).String())
	case TypeDuration:
		return fmt.Sprintf("%s..%s", DurationValue(iv.start).String(), DurationValue(iv.end).String())
	}
	return ""
}
func (iv IntervalValue) Equal(v Value) bool {
	other, ok := v.(IntervalValue)
	if !ok {
		return false
	}
	return iv.start == other.start && iv.end == other.end && iv.elemType == other.elemType
}

// Contains checks if a value falls within the interval [start, end].
func (iv IntervalValue) Contains(v Value) bool {
	switch iv.elemType {
	case TypeInt:
		if v.Type() != TypeInt {
			return false
		}
		n := int64(v.(IntValue))
		return n >= iv.start && n <= iv.end
	case TypeTime:
		if v.Type() != TypeTime {
			return false
		}
		n := int64(v.(TimeValue))
		return n >= iv.start && n <= iv.end
	case TypeDuration:
		if v.Type() != TypeDuration {
			return false
		}
		n := int64(v.(DurationValue))
		return n >= iv.start && n <= iv.end
	}
	return false
}

// NormalizeIP returns the canonical form of an IP address per RFC 4291.
// IPv4-mapped IPv6 addresses (::ffff:x.x.x.x) are normalized to their
// 4-byte IPv4 form. Pure IPv6 addresses are returned as 16-byte form.
// Returns nil if the input is nil.
func NormalizeIP(ip net.IP) net.IP {
	if ip == nil {
		return nil
	}
	if ip4 := ip.To4(); ip4 != nil {
		return ip4
	}
	return ip.To16()
}

// IPInCIDR checks if an IP address is within the specified CIDR range.
func IPInCIDR(ip net.IP, cidr string) (bool, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return false, err
	}
	return ipNet.Contains(ip), nil
}

// IsIPv6 checks if an IP address is IPv6.
func IsIPv6(ip net.IP) bool {
	return ip.To4() == nil && ip.To16() != nil
}

// IsIPv4 checks if an IP address is IPv4.
func IsIPv4(ip net.IP) bool {
	return ip.To4() != nil
}

// MatchesRegex checks if a value matches the specified regular expression pattern.
func MatchesRegex(value string, pattern string) (bool, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false, err
	}
	return re.MatchString(value), nil
}

// UnpackedArrayValue wraps an ArrayValue to indicate it should be unpacked in operations.
// When used in comparisons, the operation is applied to each element.
type UnpackedArrayValue struct {
	Array ArrayValue
}

func (u UnpackedArrayValue) Type() Type     { return TypeArray }
func (u UnpackedArrayValue) String() string { return u.Array.String() }
func (u UnpackedArrayValue) IsTruthy() bool { return len(u.Array) > 0 }
func (u UnpackedArrayValue) Equal(v Value) bool {
	if uv, ok := v.(UnpackedArrayValue); ok {
		return u.Array.Equal(uv.Array)
	}
	return u.Array.Equal(v)
}
