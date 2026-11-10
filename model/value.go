// Package model holds the types of the Grenton mobile interface (spec §3):
// the dynamically typed [Value], the state and action references, and the
// interface DTOs with their tolerant decoder. It does no I/O.
package model

import (
	"math"
	"strconv"
	"strings"
)

// Kind is the type of a [Value].
type Kind uint8

// The five kinds of spec §3.1.
const (
	KindNull Kind = iota
	KindBool
	KindInt
	KindDouble
	KindString
)

func (k Kind) String() string {
	switch k {
	case KindBool:
		return "bool"
	case KindInt:
		return "int"
	case KindDouble:
		return "double"
	case KindString:
		return "string"
	default:
		return "null"
	}
}

// Value is a Grenton value: null, bool, a 64-bit int, a double or a string
// (spec §3.1). The zero Value is null. Values compare with ==; a double
// compares by value, so NaN never equals itself (the wire never yields one).
type Value struct {
	kind Kind
	b    bool
	i    int64
	f    float64
	s    string
}

// Null is the null value (the CLU's nil).
func Null() Value { return Value{} }

// Bool returns a bool value.
func Bool(b bool) Value { return Value{kind: KindBool, b: b} }

// Int returns an int value.
func Int(i int64) Value { return Value{kind: KindInt, i: i} }

// Double returns a double value.
func Double(f float64) Value { return Value{kind: KindDouble, f: f} }

// String returns a string value.
func String(s string) Value { return Value{kind: KindString, s: s} }

// Kind reports the value's type.
func (v Value) Kind() Kind { return v.kind }

// IsNull reports whether the value is null.
func (v Value) IsNull() bool { return v.kind == KindNull }

// AsBool returns the bool and whether the value is one.
func (v Value) AsBool() (bool, bool) { return v.b, v.kind == KindBool }

// AsInt returns the int and whether the value is one.
func (v Value) AsInt() (int64, bool) { return v.i, v.kind == KindInt }

// AsDouble returns the double and whether the value is one.
func (v Value) AsDouble() (float64, bool) { return v.f, v.kind == KindDouble }

// AsString returns the string and whether the value is one.
func (v Value) AsString() (string, bool) { return v.s, v.kind == KindString }

// Number returns an int or a double as a float64, and whether the value is
// numeric. A string is never parsed (spec §6.2: casting happened on receipt).
func (v Value) Number() (float64, bool) {
	switch v.kind {
	case KindInt:
		return float64(v.i), true
	case KindDouble:
		return v.f, true
	}
	return 0, false
}

// String renders the value for logs: nil, true, 42, 21.5 or the text.
func (v Value) String() string {
	switch v.kind {
	case KindBool:
		return strconv.FormatBool(v.b)
	case KindInt:
		return strconv.FormatInt(v.i, 10)
	case KindDouble:
		return strconv.FormatFloat(v.f, 'g', -1, 64)
	case KindString:
		return v.s
	default:
		return "nil"
	}
}

// ParseWireValue casts a value from the wire (spec §3.1): trim, strip every
// surrounding double quote, then nil, true/false, a 64-bit int, a finite
// double in the strict ASCII grammar, or else the string itself.
func ParseWireValue(text string) Value {
	s := strings.Trim(TrimSpace(text), `"`)
	switch {
	case strings.EqualFold(s, "nil"):
		return Null()
	case strings.EqualFold(s, "true"):
		return Bool(true)
	case strings.EqualFold(s, "false"):
		return Bool(false)
	}
	if isIntegerText(s) {
		if i, err := strconv.ParseInt(s, 10, 64); err == nil {
			return Int(i)
		}
		// Int64 overflow: the same digits as a double (point 4).
		if f, err := strconv.ParseFloat(s, 64); err == nil && !math.IsInf(f, 0) {
			return Double(f)
		}
		return String(s)
	}
	if isDoubleText(s) {
		// ParseFloat reports ErrRange with ±Inf for an overflow; only a
		// finite result is a double (point 5).
		if f, err := strconv.ParseFloat(s, 64); err == nil && !math.IsInf(f, 0) {
			return Double(f)
		}
	}
	return String(s)
}

// TrimSpace trims the whitespace the wire format trims: ASCII space, tab,
// CR, LF, vertical tab and form feed (spec §3.1, §5.1).
func TrimSpace(s string) string {
	return strings.Trim(s, " \t\r\n\v\f")
}

// isIntegerText matches ^-?[0-9]+$.
func isIntegerText(s string) bool {
	if strings.HasPrefix(s, "-") {
		s = s[1:]
	}
	return s != "" && allDigits(s)
}

// isDoubleText matches ^-?([0-9]+\.[0-9]*|\.[0-9]+|[0-9]+)([eE][+-]?[0-9]+)?$
// with a dot or an exponent present.
func isDoubleText(s string) bool {
	s = strings.TrimPrefix(s, "-")
	mantissa, exponent, hasExp := s, "", false
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		mantissa, exponent, hasExp = s[:i], s[i+1:], true
	}
	whole, frac, hasDot := mantissa, "", false
	if i := strings.IndexByte(mantissa, '.'); i >= 0 {
		whole, frac, hasDot = mantissa[:i], mantissa[i+1:], true
	}
	switch {
	case !hasDot && !hasExp, whole == "" && frac == "":
		return false
	case whole != "" && !allDigits(whole), frac != "" && !allDigits(frac):
		return false
	}
	if hasExp {
		if exponent != "" && (exponent[0] == '+' || exponent[0] == '-') {
			exponent = exponent[1:]
		}
		if exponent == "" || !allDigits(exponent) {
			return false
		}
	}
	return true
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
